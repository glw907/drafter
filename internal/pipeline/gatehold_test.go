package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/edits"
)

// cairnFailurePattern is cairn's gate.failure_pattern before the check-symbols defect line's shape
// joined it, and cairnSymbolsLine that shape, copied as literals so the test pins the pattern the
// project sets.
const (
	cairnFailurePattern = `✖|defect\(s\)|broken link|target not found|\.json: [^0-9]|^ +\S+\.md$|^ +[0-9]+:[0-9]+ +error |^\S+\.md:[0-9]+: ([^w]|w[^a])`
	cairnSymbolsLine    = `^  \S+:[0-9]+  \[[a-z-]+\]  `
	cairnSectionPattern = `^== (check:\S+) ==$`
)

// siblingSymbolsLog is a full-length docs-gate log, red only in check:symbols, on a line naming a
// sibling page near the log's top; check:snippets lists lines naming the page itself, which pass.
func siblingSymbolsLog() string {
	var b strings.Builder
	line := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	line("> synthetic-site check:docs-gate")
	line("== check:links ==")
	for i := range 44 {
		line("  checked docs/guides/page-%02d.md: %d links resolve", i, i%7+1)
	}
	line("== check:symbols ==")
	line("  docs/extend/add-a-sibling.md:297  [file-path]  lib/sibling.config.js")
	line("check:symbols: 1 problem in 1 file")
	line("== check:snippets ==")
	for i := range 80 {
		line("  snippet %d of %s matches its source (%d lines)", i+1, pageA, i%11+3)
	}
	line("== check:provenance ==")
	for i := range 85 {
		line("  sentence %d cites a verified fact", i+1)
	}
	line("docs gate: 1 component failed")
	return b.String()
}

// logGate replaces the project's gate with one that prints log and fails while the file .red is
// in the project, once, removing the file, and otherwise runs the fixture's gate.
func logGate(t *testing.T, h *harness, log string) {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "gate-red.log")
	if err := os.WriteFile(logPath, []byte(log), 0o644); err != nil {
		t.Fatal(err)
	}
	writeFile(t, h.root, "gate.sh", "#!/bin/sh\nif [ -f .red ]; then rm -f .red; cat '"+logPath+"'; exit 1; fi\n"+
		strings.TrimPrefix(gateScript, "#!/bin/sh\n"))
	gitIn(t, h.root, "add", "gate.sh")
	gitIn(t, h.root, "commit", "-q", "-m", "log gate")
}

func TestSiblingSymbolsLineHoldsThePage(t *testing.T) {
	log := siblingSymbolsLog()
	if n := strings.Count(log, "\n"); n < 200 {
		t.Fatalf("the synthetic log has %d lines, want 200 or more", n)
	}
	tests := []struct {
		name    string
		pattern string
		drafts  int
		held    bool
	}{
		{"cairn's pattern with the symbols line holds the page", "(?:" + cairnFailurePattern + ")|" + cairnSymbolsLine, 1, true},
		{"without the symbols line the log is unparsed and the page spends a fix turn", cairnFailurePattern, 2, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, withoutFacts, withPages("a"))
			h.cfg.Gate.SectionPattern, h.cfg.Gate.FailurePattern = cairnSectionPattern, tt.pattern
			logGate(t, h, log)
			touchOnSubmit(t, h, "_s2_draft_1", ".red")
			// The fix turn an unparsed red costs declines its one finding, since nothing on the page is wrong.
			h.api.script(s2, "draft", okText(draftText(goodPage, goodBrief)), okText(jsonText(edits.Answer{Edits: []edits.Edit{}, Resolutions: []edits.Resolution{
				{Finding: 1, Status: "declined", Edits: []int{}, Reason: "the red names a sibling"},
			}})))
			s := h.mustRun()
			mustAccept(t, h, s)
			if h.exists(".red") {
				t.Fatal("the gate never printed the red log")
			}
			if n := len(requestsFor(h, s2, "draft")); n != tt.drafts {
				t.Errorf("draft requests = %d, want %d", n, tt.drafts)
			}
			rec := h.record(s.RunID, pageA)
			unparsed := false
			for _, e := range rec.GateExcerpts {
				unparsed = unparsed || e == "gate_log: unparsed"
			}
			if tt.held && len(rec.GateExcerpts) != 0 || !tt.held && !unparsed {
				t.Errorf("gate excerpts = %q, want none for a held page and the unparsed mark otherwise", rec.GateExcerpts)
			}
		})
	}
}

func TestHeldPageReleasedWhenTheSiblingEscalates(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("b", "a"))
	h.api.script(s2, "draft",
		okText(draftText(goodPageB+"\nBROKEN\n", goodBriefB)),
		okText(draftText(goodPage, goodBrief)),
		okText(answer("BROKEN", "BROKEN again", 1)),
		okText(answer("BROKEN again", "BROKEN thrice", 2)))
	s := h.mustRun()
	if r := ruleOf(s, pageB); r != "red gate" {
		t.Fatalf("page B rule = %q, want red gate", r)
	}
	mustAccept(t, h, s)
	if h.exists(pageB) {
		t.Error("the escalated sibling's page was left on disk")
	}
	if w := h.workOf(s.RunID); w.Gates < 2 {
		t.Errorf("page A ran its gate %d times, want a hold and a regate", w.Gates)
	}
	aDrafts := 0
	for _, r := range requestsFor(h, s2, "draft") {
		if strings.Contains(r.CustomID, hash12Of(pageA)) {
			aDrafts++
		}
	}
	if aDrafts != 1 {
		t.Errorf("page A sent %d s2 requests, want 1 (held, no fix turn)", aDrafts)
	}
}

func TestHoldBesideAnAcceptedSiblingStopsTreeRed(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("b", "a"), withInFlight(1))
	h.api.onSubmit = func(ids []string) {
		for _, id := range ids {
			if strings.Contains(id, hash12Of(pageA)+"_s2_draft_1") {
				writeFile(t, h.root, "docs/guides/elsewhere.md", "BROKEN\n")
			}
		}
	}
	s := h.mustRun()
	if s.Stop != StopTreeRed {
		t.Fatalf("Stop = %q, want %q", s.Stop, StopTreeRed)
	}
	if got := statusOf(s, pageB); got != StatusAccepted {
		t.Fatalf("page B = %s, want accepted before page A held", got)
	}
	if ps := h.state(s.RunID).Pages[pageKey(pageA)]; ps.Status != StatusHeld {
		t.Fatalf("page A = %s, want held", ps.Status)
	}
	h.api.onSubmit = nil
	if err := os.Remove(filepath.Join(h.root, "docs/guides/elsewhere.md")); err != nil {
		t.Fatal(err)
	}
	s, err := h.resume(s.RunID)
	if err != nil {
		t.Fatalf("resume: %v\nstderr: %s", err, h.err.String())
	}
	mustAccept(t, h, s)
	if got := statusOf(s, pageB); got != StatusAccepted {
		t.Errorf("after the resume page B = %s, want its accepted entry", got)
	}
	if rec := h.record(s.RunID, pageB); rec.FinalStatus != StatusAccepted {
		t.Errorf("page B's record final_status = %q, want accepted", rec.FinalStatus)
	}
	if ps := h.state(s.RunID).Pages[pageKey(pageB)]; ps.Status != StatusAccepted {
		t.Errorf("page B's state = %s, want accepted", ps.Status)
	}
}
