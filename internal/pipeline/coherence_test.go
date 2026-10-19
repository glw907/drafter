package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/run"
)

func TestCoherence(t *testing.T) {
	sectionTurn := okText(answer(stepTwo, stepNow, 1))
	tests := []struct {
		name   string
		script func(h *harness)
		// status and rule are how the page ends; reads is the number of coherence reads.
		status, rule string
		reads        int
		check        func(t *testing.T, h *harness, s Summary)
	}{
		{"ok accepts", func(h *harness) { h.api.script(s2, "draft", sectionTurn) },
			StatusAccepted, "", 1, func(t *testing.T, h *harness, s Summary) {
				rec := h.record(s.RunID, pageA)
				if !slices.Contains(rec.Verdicts, verdictRecord{Stage: s8, Seat: "coherence", Verdict: "ok"}) {
					t.Errorf("verdicts = %v, want coherence ok on s8", rec.Verdicts)
				}
			}},
		{"a malformed read is a fault and is asked again", func(h *harness) {
			h.api.script(s2, "draft", sectionTurn)
			h.api.script(s8, "coherence", okText(`{"verdict":"maybe"}`))
		}, StatusAccepted, "", 2, func(t *testing.T, h *harness, s Summary) {
			if faults := h.record(s.RunID, pageA).Faults; len(faults) != 1 || !strings.Contains(faults[0], "coherence") {
				t.Errorf("faults = %v, want the coherence read's", faults)
			}
		}},
		{"a first fix runs an unbounded exchange, the gate, coverage on s8, and the retest", func(h *harness) {
			h.api.script(s2, "draft", sectionTurn)
			coherenceFixes(h)
			h.api.script(s8, "draft", okText(answer(outOld, outNew, 1)))
		}, StatusAccepted, "", 2, func(t *testing.T, h *harness, s Summary) {
			if !strings.Contains(h.read(pageA), outNew) {
				t.Error("the coherence fix's edit outside the changed sections did not apply")
			}
			if n := len(requestsFor(h, s8, "coverage")); n != 1 {
				t.Errorf("s8 coverage reads = %d, want 1", n)
			}
			if !h.workOf(s.RunID).CoherenceRetest {
				t.Error("coherence_retest is not set after the fix")
			}
		}},
		{"a red gate inside the fix spends its contract budget on the fix step", func(h *harness) {
			h.api.script(s2, "draft", sectionTurn)
			coherenceFixes(h)
			h.api.script(s8, "draft", okText(answer(outOld, "Read BROKEN [key lifetime]", 1)), okText(answer(" BROKEN", "", 2)))
			h.api.onSubmit = func(ids []string) {
				if strings.Contains(ids[0], "_s8_draft_2") {
					if step := h.stateOnDisk().Pages[pageKey(pageA)].Step; step != stCoherenceFix {
						t.Errorf("the gate-fix turn is asked at step %q, want %q", step, stCoherenceFix)
					}
				}
			}
		}, StatusAccepted, "", 2, func(t *testing.T, h *harness, s Summary) {
			if got := h.workOf(s.RunID).Budgets["coherence.contract"]; got != 1 {
				t.Errorf("coherence.contract spent = %d, want 1", got)
			}
			if n := len(requestsFor(h, s8, "draft")); n != 2 {
				t.Errorf("coherence fix turns = %d, want the fix and its gate fix", n)
			}
		}},
		{"a retest fix escalates with rule coherence", func(h *harness) {
			h.api.script(s2, "draft", sectionTurn)
			h.api.script(s8, "coherence", okText(readText("fix", finding("Before you begin", "Join them."))),
				okText(readText("fix", finding("Rotate the key", "Still abrupt."), finding("Before you begin", "Still repeats."))))
			h.api.script(s8, "draft", okText(answer(outOld, outNew, 1)))
		}, StatusEscalated, "coherence", 2, func(t *testing.T, h *harness, s Summary) {
			esc := h.record(s.RunID, pageA).Escalation
			if esc == nil || esc.Seat != "coherence" || !slices.Equal(esc.Findings, []int{1, 2}) {
				t.Errorf("escalation = %+v, want seat coherence with findings 1 and 2", esc)
			}
		}},
		{"a coverage miss after the fix escalates with rule coherence", func(h *harness) {
			h.api.script(s2, "draft", sectionTurn)
			coherenceFixes(h)
			h.api.script(s8, "draft", okText(answer("A key lives for 90 days.", "A key lasts 90 days.", 1)))
		}, StatusEscalated, "coherence", 1, func(t *testing.T, h *harness, s Summary) {
			if esc := h.record(s.RunID, pageA).Escalation; esc == nil || esc.Seat != "coverage" {
				t.Errorf("escalation = %+v, want the coverage seat", esc)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newSectionHarness(t)
			tt.script(h)
			s := h.mustDraft(sectionScope("rotate"))
			if got, rule := statusOf(s, pageA), ruleOf(s, pageA); got != tt.status || rule != tt.rule {
				t.Fatalf("status = %s, rule = %q, want %s, %q\nstderr: %s", got, rule, tt.status, tt.rule, h.err.String())
			}
			if n := len(requestsFor(h, s8, "coherence")); n != tt.reads {
				t.Errorf("coherence reads = %d, want %d", n, tt.reads)
			}
			tt.check(t, h, s)
		})
	}
}

func TestCoherenceNeverRunsOnAWholeDraft(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"), withDrafting)
	mustAccept(t, h, h.mustDraft(drafting{}))
	for _, r := range h.api.requests {
		if stage, _ := stageSeat(r.CustomID); stage == s8 {
			t.Errorf("a whole draft sent %s", r.CustomID)
		}
	}
}

func TestWriteLogRecordsEveryWrite(t *testing.T) {
	h := newSectionHarness(t)
	h.api.script(s2, "draft", okText(answer(inOld, inOld+" BROKEN", 1)), okText(answer(" BROKEN", "", 2)))
	coherenceFixes(h)
	h.api.script(s8, "draft", okText(answer(outOld, outNew, 1)))
	s := h.mustDraft(sectionScope("rotate"))
	mustAccept(t, h, s)
	want := []writeRecord{
		{N: 1, Step: stSection, Kind: exSection},
		{N: 2, Step: stSection, Kind: exSection, GateFix: true},
		{N: 3, Step: stCoherenceFix, Kind: exCoherence},
	}
	got := h.record(s.RunID, pageA).Writes
	if !slices.Equal(got, want) {
		t.Fatalf("writes = %+v, want %+v", got, want)
	}
	dir := filepath.Join(h.runDir(s.RunID), run.PageKey(pageA))
	pages := map[int]string{}
	for _, w := range got {
		for _, file := range []string{"page.md", "brief.json"} {
			data, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("write%d.%s", w.N, file)))
			if err != nil {
				t.Fatalf("write %d saved no %s: %v", w.N, file, err)
			}
			if file == "page.md" {
				pages[w.N] = string(data)
			}
		}
	}
	if !strings.Contains(pages[1], "BROKEN") || strings.Contains(pages[2], "BROKEN") || !strings.Contains(pages[3], outNew) {
		t.Error("the saved writes are not the section write, its gate fix, and the coherence fix, in order")
	}
	if n := len(h.state(s.RunID).Pages[pageKey(pageA)].Writes); n != len(want) {
		t.Errorf("state writes = %d, want one per logged write", n)
	}
}

func TestWriteLogOfAWholeDraft(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"), withDrafting)
	s := h.mustDraft(drafting{})
	mustAccept(t, h, s)
	want := []writeRecord{{N: 1, Step: stDraft}}
	if got := h.record(s.RunID, pageA).Writes; !slices.Equal(got, want) {
		t.Errorf("writes = %+v, want %+v", got, want)
	}
}
