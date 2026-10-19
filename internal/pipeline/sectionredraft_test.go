package pipeline

import (
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/edits"
)

// gateFix answers the gate's finding, number 2 after an exchange's one finding: it takes out the
// BROKEN the turn before added and makes the out-of-scope edit too, which only an unbounded turn
// may apply.
func gateFix() string {
	a := edits.Answer{
		Edits:       []edits.Edit{{File: "page", Old: " BROKEN", New: ""}, {File: "page", Old: outOld, New: outNew}},
		Resolutions: []edits.Resolution{{Finding: 2, Status: "edited", Edits: []int{0, 1}, Reason: "fixed"}},
	}
	return jsonText(a)
}

// readerFixes scripts a final reader that returns fix with one finding at location.
func readerFixes(h *harness, location string) {
	h.api.script(s7, "reader", okText(`{"paraphrase":"p","summary":"s"}`),
		okText(readText("fix", finding(location, "Say which key."))))
}

// coherenceFixes scripts a coherence read that returns fix with one finding outside the changed
// section.
func coherenceFixes(h *harness) {
	h.api.script(s8, "coherence", okText(readText("fix", finding("Before you begin", "Join the two sections."))))
}

func TestRegionBoundPerExchange(t *testing.T) {
	sectionTurn := okText(answer(stepTwo, stepNow, 1))
	tests := []struct {
		name   string
		script func(h *harness)
		// stage is where the exchange's draft turns go.
		stage string
		// refused says the out-of-scope edit is refused, naming its line, and the next turn
		// applies an in-scope one; otherwise the out-of-scope edit applies.
		refused bool
		// bounded is whether each of the exchange's turns carries the decline sentence.
		bounded []bool
	}{
		{"a section turn's out-of-scope edit is refused", func(h *harness) {
			h.api.script(s2, "draft", okText(answer(outOld, outNew, 1)), okText(answer(inOld, inNew, 1)))
		}, s2, true, []bool{true, true}},
		{"a section exchange's gate-fix turn is unbounded", func(h *harness) {
			h.api.script(s2, "draft", okText(answer(inOld, inOld+" BROKEN", 1)), okText(gateFix()))
		}, s2, false, []bool{true, false}},
		{"a redraft's out-of-scope edit is refused", func(h *harness) {
			h.api.script(s2, "draft", sectionTurn)
			structureFixes(h)
			h.api.script(s5, "draft", okText(answer(outOld, outNew, 1)), okText(answer(inOld, inNew, 1)))
		}, s5, true, []bool{true, true}},
		{"a redraft's gate-fix turn is unbounded", func(h *harness) {
			h.api.script(s2, "draft", sectionTurn)
			structureFixes(h)
			h.api.script(s5, "draft", okText(answer(inOld, inOld+" BROKEN", 1)), okText(gateFix()))
		}, s5, false, []bool{true, false}},
		{"a final redraft's out-of-scope edit is refused", func(h *harness) {
			h.api.script(s2, "draft", sectionTurn)
			readerFixes(h, "Rotate the key")
			h.api.script(s7, "draft", okText(answer(outOld, outNew, 1)), okText(answer(stepThree, stepLog, 1)))
		}, s7, true, []bool{true, true}},
		{"a final redraft's gate-fix turn is unbounded", func(h *harness) {
			h.api.script(s2, "draft", sectionTurn)
			readerFixes(h, "Rotate the key")
			h.api.script(s7, "draft", okText(answer(stepThree, stepThree+" BROKEN", 1)), okText(gateFix()))
		}, s7, false, []bool{true, false}},
		{"the coherence fix is unbounded", func(h *harness) {
			h.api.script(s2, "draft", sectionTurn)
			coherenceFixes(h)
			h.api.script(s8, "draft", okText(answer(outOld, outNew, 1)))
		}, s8, false, []bool{false}},
		{"the coherence fix's gate-fix turn is unbounded", func(h *harness) {
			h.api.script(s2, "draft", sectionTurn)
			coherenceFixes(h)
			h.api.script(s8, "draft", okText(answer(inOld, inOld+" BROKEN", 1)), okText(gateFix()))
		}, s8, false, []bool{false, false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newSectionHarness(t)
			tt.script(h)
			s := h.mustDraft(sectionScope("rotate"))
			mustAccept(t, h, s)
			turns := requestsFor(h, tt.stage, "draft")
			if len(turns) != len(tt.bounded) {
				t.Fatalf("%s draft turns = %d, want %d", tt.stage, len(turns), len(tt.bounded))
			}
			for i, want := range tt.bounded {
				if got := strings.Contains(userText(turns[i]), edits.BoundInstructions()); got != want {
					t.Errorf("turn %d carries the decline sentence = %v, want %v", i+1, got, want)
				}
			}
			page := h.read(pageA)
			if tt.refused {
				next := userText(turns[1])
				if !strings.Contains(next, "Your answer was refused") || !strings.Contains(next, outLine) {
					t.Errorf("the turn after the out-of-scope edit does not name its line:\n%s", next)
				}
				if strings.Contains(page, outNew) {
					t.Error("the refused out-of-scope edit reached the page")
				}
				return
			}
			if !strings.Contains(page, outNew) || strings.Contains(page, "BROKEN") {
				t.Errorf("the unbounded out-of-scope edit did not apply:\n%s", page)
			}
		})
	}
}

func TestRegionBoundRefusalSpendsTheContractBudget(t *testing.T) {
	h := newSectionHarness(t)
	out := okText(answer(outOld, outNew, 1))
	h.api.script(s2, "draft", out, out, out)
	s := h.mustDraft(sectionScope("rotate"))
	if got, rule := statusOf(s, pageA), ruleOf(s, pageA); got != StatusEscalated || rule != "contract" {
		t.Fatalf("status = %s, rule = %s, want escalated under contract", got, rule)
	}
	if n := len(requestsFor(h, s2, "draft")); n != 3 {
		t.Errorf("section turns = %d, want the first and the two the budget allows", n)
	}
	if h.read(pageA) != goodPage {
		t.Error("the escalated page was not put back")
	}
}

func TestSectionRedraftDeclinesAnOutOfScopeFinding(t *testing.T) {
	h := newSectionHarness(t)
	h.api.script(s2, "draft", okText(answer(stepTwo, stepNow, 1)))
	readerFixes(h, "Before you begin")
	h.api.script(s7, "draft", okText(declined(1, "It lies outside the sections this redraft may change.")))
	s := h.mustDraft(sectionScope("rotate"))
	mustAccept(t, h, s)
	final := requestsFor(h, s7, "draft")
	if len(final) != 1 {
		t.Fatalf("final redraft turns = %d, want 1", len(final))
	}
	text := userText(final[0])
	contract, decline := strings.Index(text, edits.Instructions()), strings.Index(text, edits.BoundInstructions())
	if contract < 0 || decline < contract {
		t.Errorf("the bounded turn does not carry the decline sentence after the contract (contract at %d, sentence at %d)", contract, decline)
	}
	for _, seat := range []string{"editor", "fact"} {
		if n := len(requestsFor(h, s7, seat)); n != 1 {
			t.Errorf("scoped %s reads = %d, want 1", seat, n)
		}
	}
	readers := requestsFor(h, s7, "reader")
	if len(readers) != 4 || !strings.Contains(userText(readers[3]), "Declined finding 1") {
		t.Error("the declined finding did not reach the reader's retest")
	}
}

func TestSectionRedraftNarrowsEveryReview(t *testing.T) {
	h := newSectionHarness(t)
	h.api.script(s2, "draft", okText(jsonText(edits.Answer{
		Edits: []edits.Edit{{File: "page", Old: outOld, New: outNew}, {File: "page", Old: inOld, New: inNew}},
		Resolutions: []edits.Resolution{
			{Finding: 1, Status: "edited", Edits: []int{0}, Reason: "r"}, {Finding: 2, Status: "edited", Edits: []int{1}, Reason: "r"},
		},
	})))
	structureFixes(h)
	h.api.script(s5, "draft", okText(answer(stepTwo, stepNow, 1)))
	readerFixes(h, "Rotate the key")
	h.api.script(s7, "draft", okText(answer(stepThree, stepLog, 1)))
	s := h.mustDraft(sectionScope("before-you-begin", "rotate"))
	mustAccept(t, h, s)

	const scope = "scope [before-you-begin,rotate]"
	section := requestText(requestsFor(h, s2, "draft")[0])
	for _, want := range []string{"round 1", scope,
		"Finding 1 (blueprint, blocking)\nLocation: Before you begin", "Finding 2 (blueprint, blocking)\nLocation: Rotate the key"} {
		if !strings.Contains(section, want) {
			t.Errorf("the section turn lacks %q", want)
		}
	}
	for _, c := range []struct{ stage, seat, round string }{
		{s4, "structure", "round 1"}, {s4, "editor", "round 1"}, {s4, "fact", "round 1"},
		{s6, "structure", "round 2"}, {s5, "draft", "round 2"}, {s7, "draft", "round final"}, {s8, "coherence", "round 1"},
	} {
		rs := requestsFor(h, c.stage, c.seat)
		if len(rs) == 0 {
			t.Errorf("no %s %s request", c.stage, c.seat)
			continue
		}
		if text := requestText(rs[0]); !strings.Contains(text, scope) || !strings.Contains(text, c.round) {
			t.Errorf("%s %s render lacks %q or %q", c.stage, c.seat, scope, c.round)
		}
	}
	for _, seat := range []string{"editor", "fact"} {
		rs := requestsFor(h, s7, seat)
		if len(rs) != 1 || !strings.Contains(requestText(rs[0]), "scope [Finding 1 (reader, blocking)") {
			t.Errorf("the scoped %s read is not scoped to the cited findings", seat)
		}
	}
	for _, r := range requestsFor(h, s7, "reader") {
		if strings.Contains(requestText(r), "scope [before-you-begin") {
			t.Error("the final reader is narrowed")
		}
	}
	var blueprintFindings int
	for _, f := range h.record(s.RunID, pageA).Findings {
		if f.Stage == s2 && f.Seat == "blueprint" {
			blueprintFindings++
		}
	}
	if blueprintFindings != 2 {
		t.Errorf("blueprint findings at s2 = %d, want one per changed section", blueprintFindings)
	}
}
