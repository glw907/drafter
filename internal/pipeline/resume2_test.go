package pipeline

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/run"
)

func TestResumeRefusesAChangedConfiguration(t *testing.T) {
	t.Run("another mode", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		stopAfterDraft(h)
		id := mustStop(t, h, StopCycleCap)
		raiseCeiling(h)
		h.cfg.Mode = "sync"
		_, err := h.resume(id)
		if err == nil || !strings.Contains(err.Error(), "mode") {
			t.Errorf("error = %v, want a refusal naming mode", err)
		}
	})
	t.Run("a seat with an agent it did not have", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		stopAfterDraft(h)
		id := mustStop(t, h, StopCycleCap)
		raiseCeiling(h)
		s := h.cfg.Seat["structure"]
		s.Agent = h.cfg.Seat["draft"].Agent
		h.cfg.Seat["structure"] = s
		_, err := h.resume(id)
		if err == nil || !strings.Contains(err.Error(), "seat.structure.agent") {
			t.Errorf("error = %v, want a refusal naming seat.structure.agent", err)
		}
	})
	t.Run("the caps and the workspace are read fresh", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		stopAfterDraft(h)
		id := mustStop(t, h, StopCycleCap)
		raiseCeiling(h)
		h.cfg.PageCap = 9
		if _, err := h.resume(id); err != nil {
			t.Errorf("error = %v, want a resume with a changed cap to go on", err)
		}
	})
	t.Run("the run's copy of the configuration was edited", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		stopAfterDraft(h)
		id := mustStop(t, h, StopCycleCap)
		raiseCeiling(h)
		writeFile(t, h.runDir(id), "fingerprint.json", "{}")
		if _, err := h.resume(id); err == nil || !strings.Contains(err.Error(), "changed") {
			t.Errorf("error = %v, want a refusal of the edited copy", err)
		}
	})
}

func TestResumeRefusesFilesNotAsTheRunLeftThem(t *testing.T) {
	setup := func(t *testing.T) (*harness, string) {
		h := newHarness(t, withoutFacts, withPages("b"))
		stopAfterDraft(h)
		id := mustStop(t, h, StopCycleCap)
		raiseCeiling(h)
		return h, id
	}
	t.Run("a page deleted by hand", func(t *testing.T) {
		h, id := setup(t)
		if err := os.Remove(filepath.Join(h.root, pageB)); err != nil {
			t.Fatal(err)
		}
		if _, err := h.resume(id); err == nil || !strings.Contains(err.Error(), pageB) {
			t.Errorf("error = %v, want a refusal naming %s", err, pageB)
		}
	})
	t.Run("a tracked page put back with git checkout", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		writeFile(t, h.root, pageB, "# Old\n")
		writeFile(t, h.root, briefB, `{"sentences":[]}`+"\n")
		gitIn(t, h.root, "add", "-A")
		gitIn(t, h.root, "commit", "-q", "-m", "an existing page")
		stopAfterDraft(h)
		id := mustStop(t, h, StopCycleCap)
		raiseCeiling(h)
		gitIn(t, h.root, "checkout", "--", pageB, briefB)
		if _, err := h.resume(id); err == nil || !strings.Contains(err.Error(), pageB) {
			t.Errorf("error = %v, want a refusal: git checkout left the page at a hash the state does not expect", err)
		}
	})
	t.Run("an index file edited by hand", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		writeFile(t, h.root, "docs/index.md", "# Index\n")
		gitIn(t, h.root, "add", "-A")
		gitIn(t, h.root, "commit", "-q", "-m", "index")
		h.cfg.Gate.IndexLinkCommand = "printf '%s\\n' {page} >> docs/index.md"
		stopAfterDraft(h)
		id := mustStop(t, h, StopCycleCap)
		raiseCeiling(h)
		writeFile(t, h.root, "docs/index.md", h.read("docs/index.md")+"hand edit\n")
		if _, err := h.resume(id); err == nil || !strings.Contains(err.Error(), "docs/index.md") {
			t.Errorf("error = %v, want a refusal naming docs/index.md", err)
		}
	})
	t.Run("a file that is not the run's", func(t *testing.T) {
		h, id := setup(t)
		writeFile(t, h.root, "stray.txt", "x")
		if _, err := h.resume(id); err == nil || !strings.Contains(err.Error(), "stray.txt") {
			t.Errorf("error = %v, want a refusal naming stray.txt", err)
		}
	})
}

func TestResumeReadsSavedResponsesWithoutTheAPI(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("b"))
	n := 0
	h.api.crash = func(call string, _ []string) string {
		if call == "results" && n == 0 {
			n++
			return "before"
		}
		return ""
	}
	if !dies(func() { _, _ = h.run() }) {
		t.Fatal("the run did not die")
	}
	id := h.runID()
	// The state of a crash after the responses were saved and before they were ledgered.
	st := h.state(id)
	ps := st.Pages[pageKey(pageB)]
	in := &ps.Intents[len(ps.Intents)-1]
	dir := filepath.Join(h.runDir(id), pageKey(pageB), "responses")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, res := range h.api.batches[in.BatchID].results {
		data, err := json.Marshal(res)
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, dir, res.CustomID+".json", string(data))
	}
	in.Saved = true
	if err := st.Save(h.runDir(id)); err != nil {
		t.Fatal(err)
	}
	callsBefore := len(h.api.calls)
	s, err := h.resume(id)
	if err != nil {
		t.Fatal(err)
	}
	if got := statusOf(s, pageB); got != StatusAccepted {
		t.Fatalf("status = %s (%s)", got, ruleOf(s, pageB))
	}
	for _, c := range h.api.calls[callsBefore:] {
		if c == "results" && h.api.calls[callsBefore] != "check" {
			t.Error("the resume downloaded results it already had")
		}
	}
	first := h.api.calls[callsBefore:]
	if len(first) < 2 || first[0] != "check" || first[1] != "count" {
		t.Errorf("calls at the start of the resume = %v, want check then count: the saved draft response was used", first[:min(3, len(first))])
	}
	if n := len(requestsFor(h, "s2", "draft")); n != 1 {
		t.Errorf("draft requests = %d, want 1", n)
	}
	lines, unique := countSettles(h)
	if lines != len(h.api.requests) || lines != unique {
		t.Errorf("settle lines = %d (%d unique), requests = %d", lines, unique, len(h.api.requests))
	}
}

func TestResumeSettlesAnAdoptedBatchWithNoRecordedIDWhenItsResultsExpired(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("b"))
	h.api.crash = func(call string, ids []string) string {
		if call == "create" && strings.Contains(ids[0], "_s2_draft_1") {
			return "after"
		}
		return ""
	}
	if !dies(func() { _, _ = h.run() }) {
		t.Fatal("the run did not die")
	}
	id := h.runID()
	st := h.state(id)
	ps := st.Pages[pageKey(pageB)]
	ps.Intents[0].Time = testNow.AddDate(0, 0, -40)
	if err := st.Save(h.runDir(id)); err != nil {
		t.Fatal(err)
	}
	h.api.crash = nil
	if _, err := h.resume(id); err != nil {
		t.Fatal(err)
	}
	if h.settleKinds()["estimated"] < 1 {
		t.Errorf("settle kinds = %v, want the old intent settled at its reservation", h.settleKinds())
	}
	if contains(h.api.calls, "list") {
		t.Error("the resume searched for a batch that could not exist any more")
	}
	if open := h.openIntents(id); len(open) != 0 {
		t.Errorf("open intents: %v", open)
	}
}

func TestResumeRefusesAnIDThatIsNotARunID(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("b"))
	if _, err := h.resume("../../etc"); err == nil || !strings.Contains(err.Error(), "run ID") {
		t.Errorf("error = %v", err)
	}
}

func TestResumeTakesTheLocksBeforeReadingState(t *testing.T) {
	tests := []struct {
		name string
		lock func(t *testing.T, h *harness, id string) string
	}{
		{"the run lock", func(t *testing.T, h *harness, id string) string {
			return filepath.Join(h.runDir(id), "run.lock")
		}},
		{"the worktree lock", func(t *testing.T, h *harness, id string) string {
			out, err := exec.Command("git", "-C", h.root, "rev-parse", "--absolute-git-dir").Output()
			if err != nil {
				t.Fatal(err)
			}
			return filepath.Join(strings.TrimSpace(string(out)), "drafter.lock")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, withoutFacts, withPages("b"))
			stopAfterDraft(h)
			id := mustStop(t, h, StopCycleCap)
			raiseCeiling(h)
			// A state file nobody can parse makes a resume that reads it first fail with a
			// parse error, so only a resume that takes the lock first names the holder.
			if err := os.WriteFile(filepath.Join(h.runDir(id), "state.json"), []byte("{"), 0o600); err != nil {
				t.Fatal(err)
			}
			l, err := run.TryLock(tt.lock(t, h, id))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = l.Release() }()
			_, err = h.resume(id)
			if err == nil || !strings.Contains(err.Error(), "held by pid") {
				t.Errorf("error = %v, want a refusal naming the holder of %s", err, tt.name)
			}
		})
	}
}
