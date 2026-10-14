package pipeline

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/glw907/drafter/internal/batch"
	"github.com/glw907/drafter/internal/ledger"
	"github.com/glw907/drafter/internal/run"
)

// stopAfterDraft makes the run stop on the cycle ceiling once the draft has settled: the draft
// reserves 0.042 at worst and settles at 0.007, and the reads that follow reserve more than the
// ceiling leaves.
func stopAfterDraft(h *harness) {
	h.cfg.CycleCeiling = 0.06
}

func raiseCeiling(h *harness) { h.cfg.CycleCeiling = 100 }

func mustStop(t *testing.T, h *harness, want string) string {
	t.Helper()
	s := h.mustRun()
	if s.Stop != want {
		t.Fatalf("Stop = %q, want %q\nstderr: %s", s.Stop, want, h.err.String())
	}
	return s.RunID
}

func countSettles(h *harness) (lines int, unique int) {
	keys := map[string]bool{}
	for _, l := range h.ledgerLines("settle") {
		lines++
		k, _ := l["key"].(string)
		keys[k] = true
	}
	return lines, len(keys)
}

func TestResumeAdoptsACreateWithNoRecordedBatchID(t *testing.T) {
	crashAt := func(substr, phase string) func(string, []string) string {
		return func(call string, ids []string) string {
			if call == "create" && strings.Contains(ids[0], substr) {
				return phase
			}
			return ""
		}
	}
	t.Run("the batch exists and decoys surround it", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		h.api.crash = crashAt("_s2_draft_1", "after")
		if !dies(func() { _, _ = h.run() }) {
			t.Fatal("the run did not die")
		}
		id := h.runID()
		h.api.addDecoy("decoy_created_after", testNow.Add(time.Minute))
		h.api.addDecoy("decoy_created_before", testNow.Add(-time.Minute))
		h.api.crash = nil
		s, err := h.resume(id)
		if err != nil {
			t.Fatal(err)
		}
		if got := statusOf(s, pageB); got != StatusAccepted {
			t.Errorf("status = %s (%s)", got, ruleOf(s, pageB))
		}
		if n := len(requestsFor(h, "s2", "draft")); n != 1 {
			t.Errorf("draft requests = %d, want the adopted one only", n)
		}
		if h.settleKinds()["void"] != 0 {
			t.Errorf("an adopted batch was voided: %v", h.settleKinds())
		}
	})
	t.Run("no batch carries the intent's ids, so it resubmits once", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		h.api.crash = crashAt("_s2_draft_1", "before")
		if !dies(func() { _, _ = h.run() }) {
			t.Fatal("the run did not die")
		}
		id := h.runID()
		h.api.addDecoy("decoy_created_after", testNow.Add(time.Minute))
		h.api.crash = nil
		s, err := h.resume(id)
		if err != nil {
			t.Fatal(err)
		}
		if got := statusOf(s, pageB); got != StatusAccepted {
			t.Errorf("status = %s (%s)", got, ruleOf(s, pageB))
		}
		if n := len(requestsFor(h, "s2", "draft")); n != 1 {
			t.Errorf("draft requests = %d, want the one resubmission", n)
		}
		if h.settleKinds()["void"] != 1 {
			t.Errorf("void lines = %d, want the first intent voided", h.settleKinds()["void"])
		}
	})
	t.Run("an ended earlier batch in the margin is never adopted", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("a"))
		h.api.script("s7", "reader", okText(`{"paraphrase":"p","summary":"s"}`),
			okText(readText("fix", finding("Rotate the key", "Say which key."))))
		h.api.script("s7", "draft", okText(answer("Create a new key.", "Create a new key in the console.", 1)))
		h.api.crash = crashAt("_s7_reader_3", "before")
		if !dies(func() { _, _ = h.run() }) {
			t.Fatal("the run did not die at the retest")
		}
		id := h.runID()
		h.api.crash = nil
		s, err := h.resume(id)
		if err != nil {
			t.Fatal(err)
		}
		if got := statusOf(s, pageA); got != StatusAccepted {
			t.Errorf("status = %s (%s)", got, ruleOf(s, pageA))
		}
		if n := len(requestsFor(h, "s7", "reader")); n != 4 {
			t.Errorf("reader requests = %d, want 2 for the read and 2 for the retest, none adopted", n)
		}
		if h.settleKinds()["void"] != 1 {
			t.Errorf("void lines = %d, want 1", h.settleKinds()["void"])
		}
	})
}

func TestResumeSettlesAnEndedBatchOncePerCustomID(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("b"))
	n := 0
	h.api.crash = func(call string, _ []string) string {
		if call == "results" && n < 2 {
			n++
			return "before"
		}
		return ""
	}
	if !dies(func() { _, _ = h.run() }) {
		t.Fatal("the run did not die")
	}
	id := h.runID()
	if !dies(func() { _, _ = h.resume(id) }) {
		t.Fatal("the first resume did not die")
	}
	s, err := h.resume(id)
	if err != nil {
		t.Fatal(err)
	}
	if got := statusOf(s, pageB); got != StatusAccepted {
		t.Fatalf("status = %s (%s)", got, ruleOf(s, pageB))
	}
	lines, unique := countSettles(h)
	if lines != unique || lines != len(h.api.requests) {
		t.Errorf("settle lines = %d (%d unique), requests = %d, want one per custom_id", lines, unique, len(h.api.requests))
	}
}

func TestResumeAfterCycleCapSettlesTheBatchInFlight(t *testing.T) {
	h := newHarness(t, withoutFacts)
	h.cfg.CycleCeiling = 0.05
	id := mustStop(t, h, StopCycleCap)
	if open := h.openIntents(id); len(open) != 1 {
		t.Fatalf("open intents = %d, want the other page's batch in flight", len(open))
	}
	raiseCeiling(h)
	s, err := h.resume(id)
	if err != nil {
		t.Fatal(err)
	}
	for _, pg := range []string{pageA, pageB} {
		if got := statusOf(s, pg); got != StatusAccepted {
			t.Errorf("%s = %s (%s)", pg, got, ruleOf(s, pg))
		}
	}
	lines, unique := countSettles(h)
	if lines != unique || lines != len(h.api.requests) {
		t.Errorf("settle lines = %d (%d unique), requests = %d", lines, unique, len(h.api.requests))
	}
	if open := h.openIntents(id); len(open) != 0 {
		t.Errorf("open intents after the resume: %v", open)
	}
}

func TestWriteInterruptedBetweenRecordAndWrite(t *testing.T) {
	t.Run("pre-write hashes redo the write from the saved answer", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		dir := filepath.Join(h.root, "docs", "guides")
		if err := os.Chmod(dir, 0o555); err != nil {
			t.Fatal(err)
		}
		_, err := h.run()
		if err == nil {
			_ = os.Chmod(dir, 0o755)
			t.Fatal("a write into a read-only directory succeeded")
		}
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		id := h.runID()
		st := h.state(id)
		w := st.Pages[pageKey(pageB)].Writes
		if len(w) != 1 || w[0].Post != nil {
			t.Fatalf("writes = %+v, want one pre-write record", w)
		}
		s2Lines := func() int {
			n := 0
			for _, l := range h.ledgerLines("") {
				if intent, _ := l["intent"].(string); strings.Contains(intent, "-s2-") {
					n++
				}
			}
			return n
		}
		callsBefore, linesBefore := len(h.api.calls), s2Lines()
		s, err := h.resume(id)
		if err != nil {
			t.Fatal(err)
		}
		if got := statusOf(s, pageB); got != StatusAccepted {
			t.Fatalf("status = %s (%s)", got, ruleOf(s, pageB))
		}
		if n := len(requestsFor(h, "s2", "draft")); n != 1 {
			t.Errorf("draft requests = %d, want 1: the write was redone, not the draft", n)
		}
		if h.read(pageB) != goodPageB {
			t.Error("the page is not the saved answer")
		}
		// The redo costs nothing: the resume's first call is the workspace check, and its
		// second is the token count of the next stage, with no ledger line for the draft's
		// intent beyond the two it had.
		if got := h.api.calls[callsBefore : callsBefore+2]; got[0] != "check" || got[1] != "count" {
			t.Errorf("calls at the start of the resume = %v, want check then count", got)
		}
		if got := s2Lines(); got != linesBefore {
			t.Errorf("stage-2 ledger lines went from %d to %d", linesBefore, got)
		}
	})
	t.Run("post-write hashes continue with no rewrite", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		stopAfterDraft(h)
		id := mustStop(t, h, StopCycleCap)
		statePath := filepath.Join(h.runDir(id), "state.json")
		data, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatal(err)
		}
		var st run.State
		if err := json.Unmarshal(data, &st); err != nil {
			t.Fatal(err)
		}
		ps := st.Pages[pageKey(pageB)]
		ps.Writes[len(ps.Writes)-1].Post = nil
		var w map[string]any
		if err := json.Unmarshal(ps.Work, &w); err != nil {
			t.Fatal(err)
		}
		w["pending_gate"] = "draft"
		ps.Work, _ = json.Marshal(w)
		if err := st.Save(h.runDir(id)); err != nil {
			t.Fatal(err)
		}
		before, err := os.Stat(filepath.Join(h.root, pageB))
		if err != nil {
			t.Fatal(err)
		}
		raiseCeiling(h)
		s, err := h.resume(id)
		if err != nil {
			t.Fatal(err)
		}
		if got := statusOf(s, pageB); got != StatusAccepted {
			t.Fatalf("status = %s (%s)", got, ruleOf(s, pageB))
		}
		after, err := os.Stat(filepath.Join(h.root, pageB))
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(before, after) {
			t.Error("the page was rewritten though it was at its post-write hash")
		}
	})
}

func TestResumeFinishesARestore(t *testing.T) {
	refused := answer("THIS TEXT IS NOT ON THE PAGE", "x", 1)
	h := newHarness(t, withoutFacts, withPages("b"))
	pre := `{"sentences":[]}` + "\n"
	writeFile(t, h.root, pageB, "# Old page\n\nOld text.\n")
	writeFile(t, h.root, briefB, pre)
	gitIn(t, h.root, "add", "-A")
	gitIn(t, h.root, "commit", "-q", "-m", "an existing page")
	h.api.script("s4", "structure", okText(readText("fix", finding("Revoke the key", "Add a note."))))
	h.api.script("s5", "draft", okText(refused), okText(refused), okText(refused))
	s := h.mustRun()
	if ruleOf(s, pageB) != "contract" {
		t.Fatalf("rule = %q", ruleOf(s, pageB))
	}
	// A crash after the page was restored and before the brief was: the brief holds the draft.
	writeFile(t, h.root, briefB, goodBriefB)
	statePath := filepath.Join(h.runDir(s.RunID), "state.json")
	data, _ := os.ReadFile(statePath)
	var st run.State
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatal(err)
	}
	st.Pages[pageKey(pageB)].Restored = false
	st.Stopped = StopTransport
	if err := st.Save(h.runDir(s.RunID)); err != nil {
		t.Fatal(err)
	}
	if _, err := h.resume(s.RunID); err != nil {
		t.Fatal(err)
	}
	if got := h.read(briefB); got != pre {
		t.Errorf("brief = %q, want it restored to %q", got, pre)
	}
	if !h.state(s.RunID).Pages[pageKey(pageB)].Restored {
		t.Error("the page is not marked restored")
	}
}

func TestResumeRefusesAChangedFile(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("b"))
	stopAfterDraft(h)
	id := mustStop(t, h, StopCycleCap)
	writeFile(t, h.root, pageB, "edited by hand\n")
	raiseCeiling(h)
	before := len(h.api.requests)
	_, err := h.resume(id)
	if err == nil || !strings.Contains(err.Error(), pageB) {
		t.Fatalf("resume error = %v, want one naming %s", err, pageB)
	}
	if h.read(pageB) != "edited by hand\n" {
		t.Error("the resume overwrote a file changed by hand")
	}
	if len(h.api.requests) != before {
		t.Error("the refused resume sent requests")
	}
}

func TestResumeCountsEarlierPageSpend(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("b"))
	stopAfterDraft(h)
	id := mustStop(t, h, StopCycleCap)
	raiseCeiling(h)
	// The draft cost 0.007; the three reads reserve 0.126 more, which the first run's cap of 10
	// allowed and a cap of 0.05 does not.
	h.cfg.PageCap = 0.05
	s, err := h.resume(id)
	if err != nil {
		t.Fatal(err)
	}
	if statusOf(s, pageB) != StatusEscalated || ruleOf(s, pageB) != "cap" {
		t.Errorf("page = %s (%s), want escalated for cap", statusOf(s, pageB), ruleOf(s, pageB))
	}
}

func TestResumeSyncIntents(t *testing.T) {
	t.Run("a lost response settles estimated and is sent again, once per attempt", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"), withMode("sync"))
		n := 0
		h.api.crash = func(call string, _ []string) string {
			if call == "send" && n < 2 {
				n++
				return "before"
			}
			return ""
		}
		if !dies(func() { _, _ = h.run() }) {
			t.Fatal("the run did not die")
		}
		id := h.runID()
		if !dies(func() { _, _ = h.resume(id) }) {
			t.Fatal("the first resume did not die")
		}
		s, err := h.resume(id)
		if err != nil {
			t.Fatal(err)
		}
		if got := statusOf(s, pageB); got != StatusAccepted {
			t.Fatalf("status = %s (%s)", got, ruleOf(s, pageB))
		}
		if got := h.settleKinds()["estimated"]; got != 2 {
			t.Errorf("estimated lines = %d, want one per lost attempt", got)
		}
		lines, unique := countSettles(h)
		if lines != unique {
			t.Errorf("settle lines = %d, unique keys = %d", lines, unique)
		}
	})
	t.Run("a transport failure left a void line, not an estimate", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"), withMode("sync"))
		h.api.sendErrs = map[int]error{1: errTransport()}
		id := mustStop(t, h, StopTransport)
		h.api.sendErrs = nil
		s, err := h.resume(id)
		if err != nil {
			t.Fatal(err)
		}
		if got := statusOf(s, pageB); got != StatusAccepted {
			t.Fatalf("status = %s (%s)", got, ruleOf(s, pageB))
		}
		kinds := h.settleKinds()
		if kinds["void"] != 1 || kinds["estimated"] != 0 {
			t.Errorf("settle kinds = %v, want one void and no estimate", kinds)
		}
		if n := len(requestsFor(h, "s2", "draft")); n != 1 {
			t.Errorf("draft requests = %d, want the one new attempt", n)
		}
	})
}

func TestResumeVoidsAnOpenReservationNoIntentNames(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("b"))
	stopAfterDraft(h)
	id := mustStop(t, h, StopCycleCap)
	led := ledger.Open(h.stateDir)
	orphan := ledger.Reservation{Intent: "orphan-intent", Requests: 1, Worst: 0.01}
	orphan.RunID, orphan.Repo, orphan.Page = id, "x", pageB
	orphan.Seat, orphan.Model, orphan.Mode = "draft", "model-a", "batch"
	if err := led.Reserve(orphan, ledger.Caps{Page: 100, Cycle: 100, CycleStart: testNow.AddDate(0, -1, 0)}); err != nil {
		t.Fatal(err)
	}
	raiseCeiling(h)
	if _, err := h.resume(id); err != nil {
		t.Fatal(err)
	}
	if open := h.openIntents(id); len(open) != 0 {
		t.Errorf("open intents after the resume: %v", open)
	}
	found := false
	for _, l := range h.ledgerLines("settle") {
		found = found || (l["intent"] == "orphan-intent" && l["kind"] == "void")
	}
	if !found {
		t.Error("the orphan reservation was not voided")
	}
}

func TestResumeSettlesAnExpiredBatchEstimated(t *testing.T) {
	h := newHarness(t, withoutFacts)
	h.cfg.CycleCeiling = 0.05
	id := mustStop(t, h, StopCycleCap)
	raiseCeiling(h)
	opts := h.options()
	opts.Resume = id
	opts.Now = func() time.Time { return testNow.AddDate(0, 0, 30) }
	// The fake stamped its batches with the real clock, so the 30 days are measured from it.
	st := h.state(id)
	for _, ps := range st.Pages {
		for i := range ps.Intents {
			ps.Intents[i].Created = testNow.AddDate(0, 0, -1)
		}
	}
	if err := st.Save(h.runDir(id)); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(t.Context(), opts); err != nil {
		t.Fatal(err)
	}
	if got := h.settleKinds()["estimated"]; got < 1 {
		t.Errorf("estimated lines = %d, want the expired batch settled at its reservation", got)
	}
	if open := h.openIntents(id); len(open) != 0 {
		t.Errorf("open intents: %v", open)
	}
}

func TestResumeRefusesAnEditedPlan(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("b"))
	stopAfterDraft(h)
	id := mustStop(t, h, StopCycleCap)
	writeFile(t, h.root, "plans/revoke-a-key.plan.yaml", h.read("plans/revoke-a-key.plan.yaml")+"# edited\n")
	raiseCeiling(h)
	_, err := h.resume(id)
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Errorf("resume error = %v, want a checksum refusal", err)
	}
}

func TestRunDirectoryCopiesWin(t *testing.T) {
	setup := func(t *testing.T) (*harness, string, string) {
		h := newHarness(t, withoutFacts, withPages("b"))
		module := filepath.Join(t.TempDir(), "module.js")
		src, err := os.ReadFile(h.cfg.Prompts)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(module, src, 0o644); err != nil {
			t.Fatal(err)
		}
		h.cfg.Prompts = module
		agent := filepath.Join(t.TempDir(), "draft.md")
		asrc, err := os.ReadFile(h.cfg.Seat["draft"].Agent)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(agent, asrc, 0o644); err != nil {
			t.Fatal(err)
		}
		s := h.cfg.Seat["draft"]
		s.Agent = agent
		h.cfg.Seat["draft"] = s
		return h, module, agent
	}
	// control is the request text an unedited run sends for the seat and stage.
	control := func(t *testing.T, stage, seat string) (system, text string) {
		h, _, _ := setup(t)
		structureFixes(h)
		h.api.script("s5", "draft", okText(answer("Revoke the old key after the grace period.", "Revoke the old key once the grace period ends.", 1)))
		h.mustRun()
		r := requestsFor(h, stage, seat)[0]
		return r.System, userText(r)
	}
	start := func(t *testing.T) (*harness, string, string, string) {
		h, module, agent := setup(t)
		structureFixes(h)
		h.api.script("s5", "draft", okText(answer("Revoke the old key after the grace period.", "Revoke the old key once the grace period ends.", 1)))
		stopAfterDraft(h)
		id := mustStop(t, h, StopCycleCap)
		raiseCeiling(h)
		return h, id, module, agent
	}
	edit := func(t *testing.T, path, from, to string) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), from) {
			t.Fatalf("%s has no %q to edit", path, from)
		}
		if err := os.WriteFile(path, []byte(strings.Replace(string(data), from, to, 1)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("an edit that changes no prompt passes", func(t *testing.T) {
		h, id, module, _ := start(t)
		edit(t, module, "const resolved", "// a comment\nconst resolved")
		s, err := h.resume(id)
		if err != nil {
			t.Fatal(err)
		}
		if got := statusOf(s, pageB); got != StatusAccepted {
			t.Errorf("status = %s (%s)", got, ruleOf(s, pageB))
		}
	})
	t.Run("an edit to a seat's prompt changes nothing in the run", func(t *testing.T) {
		wantSys, wantText := control(t, "s4", "editor")
		h, id, module, _ := start(t)
		hashBefore := h.state(id).Inputs["prompts"].Hash
		edit(t, module, "Editor read of", "Rewritten read of")
		if _, err := h.resume(id); err != nil {
			t.Fatal(err)
		}
		r := requestsFor(h, "s4", "editor")[0]
		if r.System != wantSys || userText(r) != wantText {
			t.Errorf("the resumed editor request differs from the original:\n%s\nwant\n%s", userText(r), wantText)
		}
		if got := h.state(id).Inputs["prompts"].Hash; got != hashBefore {
			t.Errorf("the run directory's module hash changed: %s to %s", hashBefore, got)
		}
	})
	t.Run("an agent definition edit changes nothing in the run", func(t *testing.T) {
		wantSys, _ := control(t, "s5", "draft")
		h, id, _, agent := start(t)
		edit(t, agent, "You draft documentation pages.", "You write poems.")
		if _, err := h.resume(id); err != nil {
			t.Fatal(err)
		}
		r := requestsFor(h, "s5", "draft")[0]
		if r.System != wantSys || strings.Contains(r.System, "poems") {
			t.Errorf("the resumed redraft's system prompt is %q, want the original %q", r.System, wantSys)
		}
	})
}

func TestResumeAndStartRefusals(t *testing.T) {
	t.Run("another branch", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		stopAfterDraft(h)
		id := mustStop(t, h, StopCycleCap)
		gitIn(t, h.root, "checkout", "-q", "-b", "elsewhere")
		raiseCeiling(h)
		if _, err := h.resume(id); err == nil || !strings.Contains(err.Error(), "branch") {
			t.Errorf("error = %v, want a branch refusal", err)
		}
	})
	t.Run("a HEAD reset to the recorded commit's parent", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		writeFile(t, h.root, "docs/extra.md", "# Extra\n")
		gitIn(t, h.root, "add", "-A")
		gitIn(t, h.root, "commit", "-q", "-m", "second commit")
		stopAfterDraft(h)
		id := mustStop(t, h, StopCycleCap)
		gitIn(t, h.root, "reset", "-q", "--hard", "HEAD~1")
		raiseCeiling(h)
		if _, err := h.resume(id); err == nil || !strings.Contains(err.Error(), "descendant") {
			t.Errorf("error = %v, want a HEAD refusal", err)
		}
	})
	t.Run("a descendant HEAD resumes", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		stopAfterDraft(h)
		id := mustStop(t, h, StopCycleCap)
		writeFile(t, h.root, "docs/extra.md", "# Extra\n")
		gitIn(t, h.root, "add", "docs/extra.md")
		gitIn(t, h.root, "commit", "-q", "-m", "later commit")
		raiseCeiling(h)
		if _, err := h.resume(id); err != nil {
			t.Errorf("error = %v, want the resume to go on", err)
		}
	})
	t.Run("another state schema", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		stopAfterDraft(h)
		id := mustStop(t, h, StopCycleCap)
		if err := os.WriteFile(filepath.Join(h.runDir(id), "state.json"), []byte(`{"schema": 1}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := h.resume(id); err == nil || !strings.Contains(err.Error(), "schema") {
			t.Errorf("error = %v, want a schema refusal", err)
		}
	})
	t.Run("a dirty worktree at start", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		writeFile(t, h.root, "gate.sh", h.read("gate.sh")+"# touched\n")
		_, err := h.run()
		if !errors.Is(err, ErrDirty) || !strings.Contains(err.Error(), "gate.sh") {
			t.Errorf("error = %v, want ErrDirty naming gate.sh", err)
		}
	})
	t.Run("an untracked file at start", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		writeFile(t, h.root, "scratch.txt", "x")
		_, err := h.run()
		if !errors.Is(err, ErrDirty) || !strings.Contains(err.Error(), "scratch.txt") {
			t.Errorf("error = %v, want ErrDirty naming scratch.txt", err)
		}
	})
	t.Run("a busy worktree lock names its holder", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		out, err := exec.Command("git", "-C", h.root, "rev-parse", "--absolute-git-dir").Output()
		if err != nil {
			t.Fatal(err)
		}
		l, err := run.TryLock(filepath.Join(strings.TrimSpace(string(out)), "drafter.lock"))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = l.Release() }()
		_, err = h.run()
		if err == nil || !strings.Contains(err.Error(), "held by pid") {
			t.Errorf("error = %v, want a lock refusal naming the pid", err)
		}
	})
}

func TestWorkspaceMismatchStopsBeforeAnyRequest(t *testing.T) {
	mismatch := errors.New("workspace mismatch: expected ws-test, the API reported ws-other")
	check := func(t *testing.T, err error) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), "ws-test") || !strings.Contains(err.Error(), "ws-other") {
			t.Errorf("error = %v, want the mismatch naming both IDs", err)
		}
	}
	t.Run("at start", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		h.api.checkErr = mismatch
		_, err := h.run()
		check(t, err)
		if h.api.counts+h.api.creates+h.api.sends != 0 {
			t.Errorf("counts %d, creates %d, sends %d, want none", h.api.counts, h.api.creates, h.api.sends)
		}
	})
	t.Run("on resume", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		stopAfterDraft(h)
		id := mustStop(t, h, StopCycleCap)
		raiseCeiling(h)
		counts, creates, sends := h.api.counts, h.api.creates, h.api.sends
		h.api.checkErr = mismatch
		_, err := h.resume(id)
		check(t, err)
		if h.api.counts != counts || h.api.creates != creates || h.api.sends != sends {
			t.Error("the refused resume sent requests")
		}
	})
}

func TestLongestCustomIDMatchesThePattern(t *testing.T) {
	p := &page{key: "docs--guides--a-very-long-page-name-that-goes-on", r: &runner{suffix: "0123abcd"}}
	id := p.customID("s7", "consistency", 999)
	if !regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`).MatchString(id) {
		t.Errorf("custom_id %q does not match the API's pattern", id)
	}
	if len(id) > 64 {
		t.Errorf("custom_id is %d characters", len(id))
	}
}

func TestCopiedConfigurationHoldsNoCredentials(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("b"))
	h.cfg.KeyCommand = "op read secret-vault-item"
	s := h.mustRun()
	data, err := os.ReadFile(filepath.Join(h.runDir(s.RunID), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"secret-vault-item", key} {
		if strings.Contains(string(data), bad) {
			t.Errorf("the copied configuration holds %q", bad)
		}
	}
}

func TestStartPrunesFinishedOldRuns(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("b"))
	repo, err := run.Repo(h.root)
	if err != nil {
		t.Fatal(err)
	}
	mk := func(id, stopped string) string {
		dir, err := run.Create(h.stateDir, repo, id)
		if err != nil {
			t.Fatal(err)
		}
		st := &run.State{Schema: run.StateSchema, RunID: id, Stopped: stopped, Pages: map[string]*run.PageState{"p": {Status: "accepted"}}}
		if err := st.Save(dir); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	finished := mk("20260101T000000Z-aaaaaaaa", "")
	stopped := mk("20260101T000000Z-bbbbbbbb", StopCycleCap)
	h.mustRun()
	if _, err := os.Stat(finished); err == nil {
		t.Error("a finished run past retention was not pruned")
	}
	if _, err := os.Stat(stopped); err != nil {
		t.Error("a stopped run was pruned")
	}
}

func errTransport() error {
	return &batch.CallError{Kind: batch.ErrTransport, Cause: "timeout"}
}
