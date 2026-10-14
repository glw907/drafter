package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/ledger"
)

func withBothReviewers(h *harness) {
	for i := range h.handoffs {
		h.handoffs[i].Context = []byte(strings.Replace(string(h.handoffs[i].Context), `"args":{`, `"args":{"bothReviewers":true,`, 1))
	}
}

func withStructureInputs(inputs ...string) harnessOption {
	return func(h *harness) {
		s := h.cfg.Seat["structure"]
		s.Inputs = inputs
		h.cfg.Seat["structure"] = s
	}
}

// structureFixes scripts a round-1 structure read with one blocking finding, so a redraft follows.
func structureFixes(h *harness) {
	h.api.script("s4", "structure", okText(readText("fix", finding("Rotate the key", "Name the key."))))
}

func TestGateFixAndReaderRedraftRepeatNoCustomID(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("a"))
	h.api.script("s2", "draft",
		okText(draftText(goodPage+"\nBROKEN\n", goodBrief)),
		okText(answer("BROKEN", "BROKEN too", 1)),
		okText(answer("BROKEN too", "Fixed.", 2)))
	h.api.script("s7", "reader",
		okText(`{"paraphrase":"p","summary":"s"}`),
		okText(readText("fix", finding("Rotate the key", "Say which key."))))
	h.api.script("s7", "draft", okText(answer("Fixed.", "Fixed, and the new key signs the build.", 1)))
	s := h.mustRun()
	if got := statusOf(s, pageA); got != StatusAccepted {
		t.Fatalf("status = %s (%s)\n%s", got, ruleOf(s, pageA), h.err.String())
	}
	if !h.api.allUnique {
		t.Error("a custom_id repeated")
	}
	if n := len(requestsFor(h, "s2", "draft")); n != 3 {
		t.Errorf("draft and gate-fix requests = %d, want 3", n)
	}
	if n := len(requestsFor(h, "s7", "reader")); n != 4 {
		t.Errorf("reader requests = %d, want the read and the retest, two turns each", n)
	}
	seen := map[string]bool{}
	for _, in := range h.state(s.RunID).Pages[pageKey(pageA)].Intents {
		for _, rq := range in.Requests {
			if seen[rq.CustomID] {
				t.Errorf("state names %s twice", rq.CustomID)
			}
			seen[rq.CustomID] = true
		}
	}
	rec := h.record(s.RunID, pageA)
	if len(rec.Resolutions) == 0 || len(rec.Findings) == 0 {
		t.Errorf("record resolutions %d, findings %d, want both recorded", len(rec.Resolutions), len(rec.Findings))
	}
	if !strings.Contains(h.read(pageA), "Fixed, and the new key signs the build.") {
		t.Error("the final redraft is not on disk")
	}
}

func TestStageFourAndSevenBatchesCarryDistinctIDs(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("a"))
	h.api.script("s7", "reader",
		okText(`{"paraphrase":"p","summary":"s"}`),
		okText(readText("fix", finding("Rotate the key", "Say which key."))))
	h.api.script("s7", "draft", okText(answer("Create a new key.", "Create a new key in the console.", 1)))
	h.mustRun()
	for _, stage := range []string{"s4", "s7"} {
		ids := map[string]bool{}
		for _, r := range h.api.requests {
			if st, _ := stageSeat(r.CustomID); st == stage {
				if ids[r.CustomID] {
					t.Errorf("%s repeats %s", stage, r.CustomID)
				}
				ids[r.CustomID] = true
			}
		}
		if len(ids) < 3 {
			t.Errorf("stage %s sent %d requests, want a batch of several seats", stage, len(ids))
		}
	}
}

func TestEditContractBudget(t *testing.T) {
	refused := answer("THIS TEXT IS NOT ON THE PAGE", "x", 1)
	t.Run("two refusals then a valid answer", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		h.api.script("s4", "structure", okText(readText("fix", finding("Revoke the key", "Add a note."))))
		h.api.script("s5", "draft", okText(refused), okText(refused),
			okText(answer("Revoke the old key after the grace period.", "Revoke the old key once the grace period ends.", 1)))
		s := h.mustRun()
		if got := statusOf(s, pageB); got != StatusAccepted {
			t.Errorf("status = %s (%s)", got, ruleOf(s, pageB))
		}
	})
	t.Run("a third refusal escalates for the contract", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		h.api.script("s4", "structure", okText(readText("fix", finding("Revoke the key", "Add a note."))))
		h.api.script("s5", "draft", okText(refused), okText(refused), okText(refused))
		s := h.mustRun()
		if ruleOf(s, pageB) != "contract" {
			t.Errorf("rule = %q, want contract", ruleOf(s, pageB))
		}
		if h.exists(pageB) {
			t.Error("an escalated page's draft was left on disk")
		}
		rec := h.record(s.RunID, pageB)
		if rec.Escalation == nil || rec.Escalation.Rule != "contract" {
			t.Errorf("record escalation = %+v", rec.Escalation)
		}
		if !hasFault(rec.Faults, "no resolution") {
			t.Errorf("faults = %v, want the blocking finding left without a resolution", rec.Faults)
		}
	})
	t.Run("a red gate after applied edits spends the same budget", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		h.api.script("s4", "structure", okText(readText("fix", finding("Revoke the key", "Add a note."))))
		old := "Revoke the old key after the grace period."
		h.api.script("s5", "draft",
			okText(refused),
			okText(answer(old, old+" BROKEN", 1)),
			okText(answer("BROKEN", "BROKEN two", 2)))
		s := h.mustRun()
		if ruleOf(s, pageB) != "contract" {
			t.Errorf("rule = %q, want contract", ruleOf(s, pageB))
		}
		if n := len(requestsFor(h, "s5", "draft")); n != 3 {
			t.Errorf("redraft requests = %d, want 3 (refusal, red gate, red gate)", n)
		}
	})
	t.Run("one red gate then a fix is accepted", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		h.api.script("s4", "structure", okText(readText("fix", finding("Revoke the key", "Add a note."))))
		old := "Revoke the old key after the grace period."
		h.api.script("s5", "draft",
			okText(answer(old, old+" BROKEN", 1)),
			okText(answer(" BROKEN", "", 2)))
		s := h.mustRun()
		if got := statusOf(s, pageB); got != StatusAccepted {
			t.Errorf("status = %s (%s)", got, ruleOf(s, pageB))
		}
	})
}

func hasFault(faults []string, sub string) bool {
	for _, f := range faults {
		if strings.Contains(f, sub) {
			return true
		}
	}
	return false
}

func TestRedGateAfterDraftBudget(t *testing.T) {
	bad := draftText(goodPageB+"\nBROKEN\n", goodBriefB)
	t.Run("two gate-fix turns fit the budget", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		h.api.script("s2", "draft", okText(bad),
			okText(answer("BROKEN", "BROKEN again", 1)),
			okText(answer("BROKEN again", "Fine.", 2)))
		s := h.mustRun()
		if got := statusOf(s, pageB); got != StatusAccepted {
			t.Errorf("status = %s (%s)", got, ruleOf(s, pageB))
		}
	})
	t.Run("a third red gate escalates", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		h.api.script("s2", "draft", okText(bad),
			okText(answer("BROKEN", "BROKEN again", 1)),
			okText(answer("BROKEN again", "BROKEN thrice", 2)))
		s := h.mustRun()
		if ruleOf(s, pageB) != "red gate" {
			t.Errorf("rule = %q, want red gate", ruleOf(s, pageB))
		}
		if h.exists(pageB) || h.exists(briefB) {
			t.Error("an escalated page's files were left on disk")
		}
	})
}

func TestCoverageMisses(t *testing.T) {
	missing := okText(jsonText(map[string]any{"items": []map[string]string{
		{"id": "r-publish", "quote": ""},
		{"id": "r-verify", "quote": "The new key's ID is the one the create step printed."},
	}}))
	t.Run("a miss after the redraft", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("a"))
		structureFixes(h)
		h.api.script("s5", "draft", okText(answer("Create a new key.", "Create a new key in the console.", 1)))
		h.api.script("s5", "coverage", missing)
		s := h.mustRun()
		if ruleOf(s, pageA) != "second fix verdict" {
			t.Errorf("rule = %q, want second fix verdict", ruleOf(s, pageA))
		}
	})
	t.Run("a miss after the final reader redraft", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("a"))
		h.api.script("s7", "reader", okText(`{"paraphrase":"p","summary":"s"}`),
			okText(readText("fix", finding("Rotate the key", "Say which key."))))
		h.api.script("s7", "draft", okText(answer("Create a new key.", "Create a new key in the console.", 1)))
		h.api.script("s7", "coverage", missing)
		s := h.mustRun()
		if ruleOf(s, pageA) != "fix after the reader redraft" {
			t.Errorf("rule = %q", ruleOf(s, pageA))
		}
	})
	t.Run("a miss in the first coverage check becomes a blocking finding", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("a"))
		h.api.script("s3", "coverage", missing)
		h.api.script("s5", "draft", okText(answer("Create a new key.", "Create a new key in the console.", 1)))
		s := h.mustRun()
		if got := statusOf(s, pageA); got != StatusAccepted {
			t.Errorf("status = %s (%s)", got, ruleOf(s, pageA))
		}
		rec := h.record(s.RunID, pageA)
		found := false
		for _, f := range rec.Findings {
			found = found || (f.Seat == "coverage" && f.Blocking)
		}
		if !found {
			t.Errorf("record findings = %+v, want a blocking coverage finding", rec.Findings)
		}
	})
}

func TestFinalReadOutcomes(t *testing.T) {
	fixRead := okText(readText("fix", finding("Rotate the key", "Say which key.")))
	paraphrase := okText(`{"paraphrase":"p","summary":"s"}`)
	redraft := okText(answer("Create a new key.", "Create a new key in the console.", 1))
	t.Run("a scoped re-read returns fix", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("a"))
		h.api.script("s7", "reader", paraphrase, fixRead)
		h.api.script("s7", "draft", redraft)
		h.api.script("s7", "editor", okText(readText("fix", finding("Rotate the key", "Still unclear."))))
		s := h.mustRun()
		if r := ruleOf(s, pageA); r != "fix after the reader redraft" {
			t.Errorf("rule = %q", r)
		}
		if rec := h.record(s.RunID, pageA); rec.Escalation == nil || rec.Escalation.Seat != "editor" || len(rec.Escalation.Findings) != 1 {
			t.Errorf("escalation = %+v, want the editor's finding", rec.Escalation)
		}
	})
	t.Run("the retest returns fix", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("a"))
		h.api.script("s7", "reader", paraphrase, fixRead, paraphrase, fixRead)
		h.api.script("s7", "draft", redraft)
		s := h.mustRun()
		if r := ruleOf(s, pageA); r != "fix after the reader redraft" {
			t.Errorf("rule = %q", r)
		}
	})
	t.Run("the clean twin is accepted", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("a"))
		h.api.script("s7", "reader", paraphrase, fixRead)
		h.api.script("s7", "draft", redraft)
		s := h.mustRun()
		if got := statusOf(s, pageA); got != StatusAccepted {
			t.Errorf("status = %s (%s)", got, ruleOf(s, pageA))
		}
	})
}

func TestSecondRoundFixEscalates(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("b"))
	structureFixes(h)
	h.api.script("s5", "draft", okText(answer("Revoke the old key after the grace period.", "Revoke the old key once the grace period ends.", 1)))
	h.api.script("s6", "structure", okText(readText("fix", finding("Revoke the key", "Still wrong."))))
	s := h.mustRun()
	if r := ruleOf(s, pageB); r != "second fix verdict" {
		t.Errorf("rule = %q", r)
	}
	if rec := h.record(s.RunID, pageB); rec.Escalation == nil || rec.Escalation.Seat != "structure" {
		t.Errorf("escalation = %+v", rec.Escalation)
	}
}

func TestDeclinedFindingReachesNextRead(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("b"))
	structureFixes(h)
	h.api.script("s5", "draft", okText(declined(1, "the page already says it")))
	s := h.mustRun()
	if got := statusOf(s, pageB); got != StatusAccepted {
		t.Fatalf("status = %s (%s)", got, ruleOf(s, pageB))
	}
	reqs := requestsFor(h, "s6", "structure")
	if len(reqs) != 1 {
		t.Fatalf("round-2 structure requests = %d", len(reqs))
	}
	text := userText(reqs[0])
	if !strings.Contains(text, "Declined finding 1") || !strings.Contains(text, "the page already says it") {
		t.Errorf("round-2 request lacks the declined finding and its reason:\n%s", text)
	}
	if other := requestsFor(h, "s6", "editor"); len(other) != 0 {
		t.Error("a seat that accepted in round 1 read again in round 2")
	}
}

func TestRoundTwoInlinesATwoSidedDiff(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("b"),
		withStructureInputs("git cat-file blob {baseline} | diff -u --label round-1 --label round-2 - {page}"))
	structureFixes(h)
	h.api.script("s5", "draft", okText(answer("Revoke the old key after the grace period.", "Revoke the old key once the grace period ends.", 1)))
	h.mustRun()
	reqs := requestsFor(h, "s6", "structure")
	if len(reqs) != 1 {
		t.Fatalf("round-2 requests = %d", len(reqs))
	}
	text := userText(reqs[0])
	for _, want := range []string{"--- round-1", "+++ round-2", "-Revoke the old key after the grace period.", "+Revoke the old key once the grace period ends."} {
		if !strings.Contains(text, want) {
			t.Errorf("round-2 request lacks %q:\n%s", want, text)
		}
	}
	if round1 := requestsFor(h, "s4", "structure"); strings.Contains(userText(round1[0]), "=== command:") {
		t.Error("the round-1 request carries a command block, so it was narrowed")
	}
}

func TestBothReviewersSendsEverySeatsRoundOneRead(t *testing.T) {
	t.Run("bothReviewers", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"), withBothReviewers)
		structureFixes(h)
		h.api.script("s5", "draft", okText(answer("Revoke the old key after the grace period.", "Revoke the old key once the grace period ends.", 1)))
		h.mustRun()
		for _, seat := range []string{"structure", "editor", "fact"} {
			reqs := requestsFor(h, "s6", seat)
			if len(reqs) != 1 {
				t.Fatalf("%s round-2 requests = %d, want 1", seat, len(reqs))
			}
		}
		editor := userText(requestsFor(h, "s6", "editor")[0])
		if !strings.Contains(editor, "Your round 1 verdict: ok") {
			t.Errorf("the editor's round-1 accept did not reach its round-2 read as narrow.own:\n%s", editor)
		}
		structure := userText(requestsFor(h, "s6", "structure")[0])
		if !strings.Contains(structure, "Your round 1 verdict: fix") {
			t.Errorf("structure's own read missing:\n%s", structure)
		}
	})
	t.Run("without it only the fix seat reads again", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		structureFixes(h)
		h.api.script("s5", "draft", okText(answer("Revoke the old key after the grace period.", "Revoke the old key once the grace period ends.", 1)))
		h.mustRun()
		if n := len(requestsFor(h, "s6", "editor")); n != 0 {
			t.Errorf("editor read again %d times", n)
		}
	})
}

func TestHeldPageSpendsNoFixTurn(t *testing.T) {
	h := newHarness(t, withoutFacts)
	// Page A is written first and fails the gate; page B's gate then reports A's file.
	h.api.script("s2", "draft", okText(draftText(goodPage+"\nBROKEN\n", goodBrief)),
		okText(draftText(goodPageB, goodBriefB)))
	h.api.script("s2", "draft", okText(answer("BROKEN", "Fixed.", 1)))
	s := h.mustRun()
	for _, pg := range []string{pageA, pageB} {
		if got := statusOf(s, pg); got != StatusAccepted {
			t.Errorf("%s = %s (%s)\n%s", pg, got, ruleOf(s, pg), h.err.String())
		}
	}
	if n := len(requestsFor(h, "s2", "draft")); n != 3 {
		t.Errorf("draft requests = %d, want A's draft, A's fix, and B's draft only", n)
	}
	bFixes := 0
	for _, r := range requestsFor(h, "s2", "draft") {
		if strings.Contains(r.CustomID, hash12Of(pageB)) {
			bFixes++
		}
	}
	if bFixes != 1 {
		t.Errorf("page B sent %d s2 requests, want 1 (held, no fix turn)", bFixes)
	}
}

func TestForeignRedThatPersistsStopsTheRun(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("b"))
	writeFile(t, h.root, "gate.sh", "#!/bin/sh\necho 'docs/elsewhere.md: broken'\nexit 1\n")
	gitIn(t, h.root, "add", "-A")
	gitIn(t, h.root, "commit", "-q", "-m", "red gate")
	s := h.mustRun()
	if s.Stop != StopTreeRed {
		t.Fatalf("Stop = %q, want %q", s.Stop, StopTreeRed)
	}
	if statusOf(s, pageB) == StatusEscalated {
		t.Error("a foreign red escalated the page")
	}
	if !strings.Contains(h.err.String(), "--resume") {
		t.Errorf("no resume line: %q", h.err.String())
	}
	if n := len(requestsFor(h, "s2", "draft")); n != 1 {
		t.Errorf("draft requests = %d, want 1: the page held and spent no fix turn", n)
	}
}

func TestGateVanishStopsTheRun(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("b"))
	bin := t.TempDir()
	writeFile(t, bin, "cairn-run-gate", "#!/bin/sh\necho 'gate exit: 1 (vanished 3 times)'\n")
	if err := os.Chmod(filepath.Join(bin, "cairn-run-gate"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	h.cfg.Gate.Protocol = "cairn-run-gate"
	s := h.mustRun()
	if s.Stop != StopInfrastructure {
		t.Fatalf("Stop = %q, want %q", s.Stop, StopInfrastructure)
	}
	if statusOf(s, pageB) == StatusEscalated {
		t.Error("a gate vanish escalated the page")
	}
}

func TestFaults(t *testing.T) {
	t.Run("a read that returned nothing", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		h.api.script("s4", "structure", absent, absent)
		s := h.mustRun()
		rec := h.record(s.RunID, pageB)
		if !hasFault(rec.Faults, "structure read") {
			t.Errorf("faults = %v, want the missing read", rec.Faults)
		}
		if rec.FinalStatus == StatusAccepted {
			t.Error("a read that returned nothing counted as accept")
		}
	})
	t.Run("a refusal the answer did not cause", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		bad := draftText(goodPageB+"\nBROKEN\n", goodBriefB)
		h.api.script("s2", "draft", okText(bad),
			okText(jsonText(map[string]any{
				"edits":       []map[string]string{{"file": "brief", "old": "not json", "new": "still not json"}},
				"resolutions": []map[string]any{{"finding": 1, "status": "edited", "edits": []int{0}, "reason": "r"}},
			})))
		h.api.onSubmit = func(ids []string) {
			if strings.Contains(ids[0], "_s2_draft_2") {
				writeFile(t, h.root, briefB, "not json")
			}
		}
		// The brief on disk was corrupted by something other than the answer; the answer's brief
		// edit cannot match, and the refusal lands in faults.
		h.api.script("s2", "draft", okText(`{"edits":[],"resolutions":[]}`), okText(`{"edits":[],"resolutions":[]}`))
		s := h.mustRun()
		rec := h.record(s.RunID, pageB)
		if rec.Escalation == nil {
			t.Fatalf("page %s", statusOf(s, pageB))
		}
		if !hasFault(rec.Faults, "not the answer's doing") {
			t.Errorf("faults = %v, want the refusal the answer did not cause", rec.Faults)
		}
	})
}

func TestEscalationRestoresTheTree(t *testing.T) {
	refused := answer("THIS TEXT IS NOT ON THE PAGE", "x", 1)
	escalate := func(h *harness) {
		h.api.script("s4", "structure", okText(readText("fix", finding("Revoke the key", "Add a note."))))
		h.api.script("s5", "draft", okText(refused), okText(refused), okText(refused))
	}
	t.Run("a page that existed", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		writeFile(t, h.root, pageB, "# Old page\n\nOld text.\n")
		writeFile(t, h.root, briefB, `{"sentences":[]}`+"\n")
		gitIn(t, h.root, "add", "-A")
		gitIn(t, h.root, "commit", "-q", "-m", "an existing page")
		escalate(h)
		s := h.mustRun()
		if ruleOf(s, pageB) != "contract" {
			t.Fatalf("rule = %q (%s)", ruleOf(s, pageB), statusOf(s, pageB))
		}
		if h.read(pageB) != "# Old page\n\nOld text.\n" || h.read(briefB) != `{"sentences":[]}`+"\n" {
			t.Errorf("the page or brief is not back to its pre-run bytes: %q", h.read(pageB))
		}
		rec := h.record(s.RunID, pageB)
		dir := filepath.Join(h.runDir(s.RunID), "docs--guides--revoke-a-key")
		final, err := os.ReadFile(filepath.Join(dir, "final.page.md"))
		if err != nil {
			t.Fatal(err)
		}
		if string(final) != goodPageB {
			t.Errorf("final.page.md is not the draft: %q", final)
		}
		if rec.FinalHashes["final.page.md"] != hashBytes(final) {
			t.Errorf("final hash %q does not match the saved bytes", rec.FinalHashes["final.page.md"])
		}
		if _, err := os.Stat(filepath.Join(dir, "final.brief.json")); err != nil {
			t.Error("final.brief.json is missing")
		}
	})
	t.Run("a page drafter created", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		escalate(h)
		s := h.mustRun()
		if ruleOf(s, pageB) != "contract" {
			t.Fatalf("rule = %q", ruleOf(s, pageB))
		}
		if h.exists(pageB) || h.exists(briefB) {
			t.Error("a page drafter created is still on disk")
		}
		if _, err := os.Stat(filepath.Join(h.runDir(s.RunID), "docs--guides--revoke-a-key", "final.page.md")); err != nil {
			t.Error("the draft is not kept in the record")
		}
	})
	t.Run("an index file the link command changed", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		writeFile(t, h.root, "docs/index.md", "# Index\n")
		gitIn(t, h.root, "add", "-A")
		gitIn(t, h.root, "commit", "-q", "-m", "index")
		h.cfg.Gate.IndexLinkCommand = "printf '%s\\n' {page} >> docs/index.md"
		escalate(h)
		s := h.mustRun()
		if ruleOf(s, pageB) != "contract" {
			t.Fatalf("rule = %q", ruleOf(s, pageB))
		}
		if got := h.read("docs/index.md"); got != "# Index\n" {
			t.Errorf("index = %q, want it restored", got)
		}
	})
	t.Run("an accepted page keeps its index link", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		writeFile(t, h.root, "docs/index.md", "# Index\n")
		gitIn(t, h.root, "add", "-A")
		gitIn(t, h.root, "commit", "-q", "-m", "index")
		h.cfg.Gate.IndexLinkCommand = "printf '%s\\n' {page} >> docs/index.md"
		s := h.mustRun()
		if statusOf(s, pageB) != StatusAccepted {
			t.Fatalf("status = %s", statusOf(s, pageB))
		}
		if got := h.read("docs/index.md"); !strings.Contains(got, pageB) {
			t.Errorf("index = %q, want the link", got)
		}
	})
}

func TestNoKeyInAnyOutput(t *testing.T) {
	h := newHarness(t, withoutFacts)
	h.cfg.KeyCommand = "echo secret-key-command"
	h.api.createErrs = map[int]error{1: callError(400, "invalid_request_error", "bad request")}
	h.api.script("s2", "draft", absent)
	s, err := h.run()
	if err != nil {
		t.Fatal(err)
	}
	second := newHarness(t, withoutFacts, withPages("b"))
	second.api.createErrs = map[int]error{1: maybeSent()}
	second.api.createErr = transportErr()
	_, _ = second.run()
	third := newHarness(t, withoutFacts, withPages("b"))
	third.api.createErr = spendErr()
	_, _ = third.run()
	for _, hh := range []*harness{h, second, third} {
		for _, root := range []string{hh.stateDir, hh.root} {
			_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() || strings.Contains(p, "/.git/") {
					return nil
				}
				data, rerr := os.ReadFile(p)
				if rerr != nil {
					return nil
				}
				if strings.Contains(string(data), key) {
					t.Errorf("%s holds the key sentinel", p)
				}
				if strings.Contains(string(data), "secret-key-command") {
					t.Errorf("%s holds the key command", p)
				}
				return nil
			})
		}
		if strings.Contains(hh.out.String()+hh.err.String(), key) {
			t.Error("stdout or stderr holds the key sentinel")
		}
	}
	_ = s
}

func TestRecordGolden(t *testing.T) {
	h := newHarness(t, withPages("a"))
	s := h.mustRun()
	rec := h.record(s.RunID, pageA)
	spend, err := ledger.Open(h.stateDir).PageSpend(s.RunID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Cost != spend || spend == 0 {
		t.Errorf("record cost %v, ledger page spend %v, want equal and nonzero", rec.Cost, spend)
	}
	got := normalizeRecord(t, h.runDir(s.RunID), pageA)
	goldenPath := filepath.Join("testdata", "record.golden.json")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("record.json differs from %s; rerun with UPDATE_GOLDEN=1 after checking the change\n%s", goldenPath, got)
	}
}
