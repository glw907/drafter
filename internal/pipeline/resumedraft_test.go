package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/run"
)

// crashAfterCreate dies once the batch holding the named custom_id has been created, which leaves
// its intent open with no batch ID in the state.
func crashAfterCreate(substr string) func(string, []string) string {
	return func(call string, ids []string) string {
		if call == "create" && strings.Contains(ids[0], substr) {
			return "after"
		}
		return ""
	}
}

// rewriteState applies edit to the stopped run's saved state.
func (h *harness) rewriteState(runID string, edit func(st *run.State)) {
	h.t.Helper()
	st := h.state(runID)
	edit(st)
	if err := st.Save(h.runDir(runID)); err != nil {
		h.t.Fatal(err)
	}
}

// editWork applies edit to a page's saved working memory, as decoded JSON.
func editWork(t *testing.T, ps *run.PageState, edit func(w map[string]any)) {
	t.Helper()
	var w map[string]any
	if err := json.Unmarshal(ps.Work, &w); err != nil {
		t.Fatal(err)
	}
	edit(w)
	var err error
	if ps.Work, err = json.Marshal(w); err != nil {
		t.Fatal(err)
	}
}

func assertSettledOnce(t *testing.T, h *harness) {
	t.Helper()
	if !h.api.allUnique {
		t.Error("a custom_id was submitted twice")
	}
	if lines, unique := countSettles(h); lines != unique {
		t.Errorf("settle lines = %d for %d keys, want one per key", lines, unique)
	}
}

func TestResumeAcrossNewSteps(t *testing.T) {
	sectionTurn := okText(answer(stepTwo, stepNow, 1))
	fixThenRetest := func(h *harness) {
		h.api.script(s2, "draft", sectionTurn)
		coherenceFixes(h)
		h.api.script(s8, "draft", okText(answer(outOld, outNew, 1)))
	}
	tests := []struct {
		name    string
		script  func(h *harness)
		crashAt string
		step    string
		retest  bool
	}{
		{"an open intent at the section step", func(h *harness) { h.api.script(s2, "draft", sectionTurn) }, "_s2_draft_1", stSection, false},
		{"an open intent at the coherence read", func(h *harness) { h.api.script(s2, "draft", sectionTurn) }, "_s8_coherence_1", stCoherence, false},
		{"an open intent at the coherence retest", fixThenRetest, "_s8_coherence_2", stCoherence, true},
		{"an open intent at the coherence fix", fixThenRetest, "_s8_draft_1", stCoherenceFix, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newSectionHarness(t)
			tt.script(h)
			h.api.crash = crashAfterCreate(tt.crashAt)
			if !dies(func() { _, _ = h.runDrafting(sectionScope("rotate")) }) {
				t.Fatalf("the run did not die at %s", tt.crashAt)
			}
			id := h.runID()
			ps := h.state(id).Pages[pageKey(pageA)]
			if ps.Step != tt.step || ps.Open() == nil {
				t.Fatalf("stopped at step %q with open intent %v, want %q with one open", ps.Step, ps.Open(), tt.step)
			}
			if w := h.workOf(id); !w.Section || w.CoherenceRetest != tt.retest {
				t.Fatalf("work.section = %v, coherence_retest = %v, want true and %v", w.Section, w.CoherenceRetest, tt.retest)
			}
			h.api.crash = nil
			s, err := h.resume(id)
			if err != nil {
				t.Fatalf("resume: %v\nstderr: %s", err, h.err.String())
			}
			mustAccept(t, h, s)
			n := 0
			for _, r := range h.api.requests {
				if strings.Contains(r.CustomID, tt.crashAt) {
					n++
				}
			}
			if n != 1 {
				t.Errorf("requests %s = %d, want the adopted one only", tt.crashAt, n)
			}
			assertSettledOnce(t, h)
		})
	}

	t.Run("a stop after a section write with its gate pending", func(t *testing.T) {
		// Without the consistency read, stage 1 and the section turn fit under the ceiling and the
		// coverage read that follows the write does not.
		h := newSectionHarness(t, withSeats("draft", "structure", "editor", "fact", "reader", "coverage"), withCeiling(0.05))
		h.api.script(s2, "draft", sectionTurn)
		s := h.mustDraft(sectionScope("rotate"))
		if s.Stop != StopCycleCap {
			t.Fatalf("Stop = %q, want the cycle cap\nstderr: %s", s.Stop, h.err.String())
		}
		id := s.RunID
		h.rewriteState(id, func(st *run.State) {
			ps := st.Pages[pageKey(pageA)]
			if len(ps.Writes) != 1 {
				t.Fatalf("writes = %d, want the section write", len(ps.Writes))
			}
			ps.Writes[0].Post = nil
			editWork(t, ps, func(w map[string]any) { w["pending_gate"] = exSection })
		})
		raiseCeiling(h)
		s, err := h.resume(id)
		if err != nil {
			t.Fatalf("resume: %v\nstderr: %s", err, h.err.String())
		}
		mustAccept(t, h, s)
		if n := len(requestsFor(h, s2, "draft")); n != 1 {
			t.Errorf("section turns = %d, want 1: the resume runs the gate, not the turn", n)
		}
		w := h.workOf(id)
		if !w.Section || w.PendingGate != "" {
			t.Errorf("work.section = %v, pending_gate = %q, want a section redraft with its gate run", w.Section, w.PendingGate)
		}
		if n := len(requestsFor(h, s8, "coherence")); n != 1 {
			t.Errorf("coherence reads = %d, want the section redraft's one", n)
		}
		assertSettledOnce(t, h)
	})
}

func TestPreB3StateResumesAsARead(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"), withDrafting)
	start := h.head()
	h.api.crash = crashAfterCreate("_s1_")
	if !dies(func() { _, _ = h.runDrafting(drafting{}) }) {
		t.Fatal("the run did not die after stage 1 was created")
	}
	id := h.runID()
	h.rewriteState(id, func(st *run.State) {
		editWork(t, st.Pages[pageKey(pageA)], func(w map[string]any) {
			if w["draft"] != true {
				t.Fatalf("the drafting run saved draft = %v", w["draft"])
			}
			delete(w, "draft")
		})
	})
	h.api.crash = nil
	s, err := h.resume(id)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	mustAccept(t, h, s)
	if got, want := acceptedOf(s, pageA), h.blobAt(start, bpA); got != want {
		t.Errorf("accepted = %q, want %q", got, want)
	}
	for _, r := range h.api.requests {
		if stage, _ := stageSeat(r.CustomID); stage != s1 {
			t.Errorf("a state with no draft field drafted: %s", r.CustomID)
		}
	}
	if h.exists(pageA) {
		t.Error("a read wrote the page")
	}
}

// TestPreB3FingerprintRefusesToResume holds the documented limit: a run stopped under a
// configuration with no coherence seat, as every run before drafting was, can't resume once the
// seat is configured.
func TestPreB3FingerprintRefusesToResume(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"), withDrafting)
	h.api.crash = crashAfterCreate("_s1_")
	if !dies(func() { _, _ = h.runDrafting(drafting{}) }) {
		t.Fatal("the run did not die after stage 1 was created")
	}
	id := h.runID()
	path := filepath.Join(h.runDir(id), "fingerprint.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fp map[string]string
	if err := json.Unmarshal(data, &fp); err != nil {
		t.Fatal(err)
	}
	removed := 0
	for k := range fp {
		if strings.HasPrefix(k, "seat.coherence.") {
			delete(fp, k)
			removed++
		}
	}
	if removed == 0 {
		t.Fatal("the fingerprint holds no seat.coherence keys to remove")
	}
	frozen, err := json.MarshalIndent(fp, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	frozen = append(frozen, '\n')
	if err := os.WriteFile(path, frozen, 0o644); err != nil {
		t.Fatal(err)
	}
	h.rewriteState(id, func(st *run.State) {
		in := st.Inputs["fingerprint"]
		in.Hash = hashBytes(frozen)
		st.Inputs["fingerprint"] = in
	})
	h.api.crash = nil
	_, err = h.resume(id)
	if err == nil || !strings.Contains(err.Error(), "the configuration changed since the run started (seat.coherence.") {
		t.Errorf("error = %v, want the configuration refusal naming seat.coherence", err)
	}
}
