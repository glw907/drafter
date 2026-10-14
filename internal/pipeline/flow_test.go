package pipeline

import (
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/batch"
	"github.com/glw907/drafter/internal/edits"
)

// answer builds an edit-contract answer: one edit of the page, claimed by every finding named.
func answer(old, new string, findings ...int) string {
	a := edits.Answer{Edits: []edits.Edit{{File: "page", Old: old, New: new}}}
	for _, n := range findings {
		a.Resolutions = append(a.Resolutions, edits.Resolution{Finding: n, Status: "edited", Edits: []int{0}, Reason: "fixed"})
	}
	return jsonText(a)
}

func declined(n int, reason string) string {
	return jsonText(edits.Answer{Edits: []edits.Edit{}, Resolutions: []edits.Resolution{{Finding: n, Status: "declined", Edits: []int{}, Reason: reason}}})
}

func requestsFor(h *harness, stage, seat string) []batch.Request {
	var out []batch.Request
	for _, r := range h.api.requests {
		if s, st := stageSeat(r.CustomID); s == stage && st == seat {
			out = append(out, r)
		}
	}
	return out
}

func userText(r batch.Request) string { return r.Messages[len(r.Messages)-1].Text }

func TestSyntheticRunAccepts(t *testing.T) {
	for _, mode := range []string{"batch", "sync"} {
		t.Run(mode, func(t *testing.T) {
			h := newHarness(t, withMode(mode))
			s := h.mustRun()
			for _, pg := range []string{pageA, pageB} {
				if got := statusOf(s, pg); got != StatusAccepted {
					t.Errorf("%s = %s (%s), want accepted\nstderr: %s", pg, got, ruleOf(s, pg), h.err.String())
				}
			}
			if s.Stop != "" {
				t.Errorf("stopped: %s", s.Stop)
			}
			if open := h.openIntents(s.RunID); len(open) != 0 {
				t.Errorf("open intents after a finished run: %v", open)
			}
			if got := h.read(pageA); got != goodPage {
				t.Errorf("page A on disk differs from the draft")
			}
			settles := h.ledgerLines("settle")
			if len(settles) != len(h.api.requests) {
				t.Errorf("settle lines = %d, requests = %d, want one line per request", len(settles), len(h.api.requests))
			}
			for _, r := range h.api.requests {
				found := false
				for _, l := range settles {
					key, _ := l["key"].(string)
					// A batch result is keyed by batch ID and custom_id; a sync result by its
					// message ID, which the fake numbers per send.
					found = found || strings.HasSuffix(key, r.CustomID) || (mode == "sync" && strings.HasPrefix(key, "msg_"))
				}
				if !found {
					t.Errorf("no settle line for %s", r.CustomID)
				}
			}
		})
	}
}

func TestDraftAnswerFailures(t *testing.T) {
	tests := []struct {
		name string
		bad  outcome
	}{
		{"truncated", stopped("max_tokens")},
		{"unparseable", okText("not json at all")},
		{"brief is not JSON", okText(draftText(goodPageB, "not json"))},
	}
	for _, tt := range tests {
		t.Run(tt.name+" once is retried", func(t *testing.T) {
			h := newHarness(t, withoutFacts, withPages("b"))
			h.api.script("s2", "draft", tt.bad)
			s := h.mustRun()
			if got := statusOf(s, pageB); got != StatusAccepted {
				t.Errorf("status = %s (%s), want accepted after one retry", got, ruleOf(s, pageB))
			}
			if got := len(requestsFor(h, "s2", "draft")); got != 2 {
				t.Errorf("draft requests = %d, want 2", got)
			}
		})
		t.Run(tt.name+" twice escalates", func(t *testing.T) {
			h := newHarness(t, withoutFacts, withPages("b"))
			h.api.script("s2", "draft", tt.bad, tt.bad)
			s := h.mustRun()
			if statusOf(s, pageB) != StatusEscalated || ruleOf(s, pageB) != "unreadable draft" {
				t.Errorf("page = %s (%s), want escalated for unreadable draft", statusOf(s, pageB), ruleOf(s, pageB))
			}
			if h.exists(pageB) {
				t.Error("an unreadable draft was written")
			}
		})
	}
}

func TestRound1ReadFailures(t *testing.T) {
	tests := []struct {
		name string
		bad  outcome
	}{
		{"max_tokens", stopped("max_tokens")},
		{"schema failure", okText(`{"verdict":"maybe","findings":[],"summary":"x"}`)},
		{"missing", absent},
	}
	for _, tt := range tests {
		t.Run(tt.name+" once is retried", func(t *testing.T) {
			h := newHarness(t, withoutFacts, withPages("b"))
			h.api.script("s4", "structure", tt.bad)
			s := h.mustRun()
			if got := statusOf(s, pageB); got != StatusAccepted {
				t.Errorf("status = %s (%s)", got, ruleOf(s, pageB))
			}
			// Only the structure seat is asked again.
			if len(requestsFor(h, "s4", "structure")) != 2 || len(requestsFor(h, "s4", "editor")) != 1 {
				t.Errorf("structure asked %d times, editor %d", len(requestsFor(h, "s4", "structure")), len(requestsFor(h, "s4", "editor")))
			}
		})
		t.Run(tt.name+" twice escalates", func(t *testing.T) {
			h := newHarness(t, withoutFacts, withPages("b"))
			h.api.script("s4", "structure", tt.bad, tt.bad)
			s := h.mustRun()
			if ruleOf(s, pageB) != "unreadable structure" {
				t.Errorf("rule = %q", ruleOf(s, pageB))
			}
			if h.exists(pageB) {
				t.Error("an escalated page was left on disk")
			}
		})
	}
}

func TestNoRetryReads(t *testing.T) {
	round2 := func(h *harness, o outcome) {
		h.api.script("s4", "structure", okText(readText("fix", finding("Revoke the key", "Add a note."))))
		h.api.script("s5", "draft", okText(answer("Revoke the old key after the grace period.", "Revoke the old key after one hour.", 1)))
		h.api.script("s6", "structure", o)
	}
	final := func(h *harness, o outcome) {
		h.api.script("s7", "reader", okText(`{"paraphrase":"p","summary":"s"}`), o)
	}
	tests := []struct {
		name     string
		setup    func(*harness, outcome)
		bad      outcome
		wantRule string
		stage    string
		seat     string
	}{
		{"round 2 missing", round2, absent, "unreadable structure", "s6", "structure"},
		{"round 2 unreadable", round2, okText("garbage"), "unreadable structure", "s6", "structure"},
		{"final read missing", final, absent, "unreadable reader", "s7", "reader"},
		{"final read unreadable", final, okText("garbage"), "unreadable reader", "s7", "reader"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, withoutFacts, withPages("b"))
			tt.setup(h, tt.bad)
			s := h.mustRun()
			if ruleOf(s, pageB) != tt.wantRule {
				t.Errorf("rule = %q, want %q (status %s)", ruleOf(s, pageB), tt.wantRule, statusOf(s, pageB))
			}
			// A read with no budget is never asked again: the final reader's two turns are two
			// requests, and a round-2 read is one.
			want := 1
			if tt.seat == "reader" {
				want = 2
			}
			if got := len(requestsFor(h, tt.stage, tt.seat)); got != want {
				t.Errorf("%s %s requests = %d, want %d", tt.stage, tt.seat, got, want)
			}
		})
	}
}

func TestTransientErrors(t *testing.T) {
	tests := []struct {
		name string
		mode string
		bad  outcome
	}{
		{"api_error in a batch", "batch", errored("api_error", "boom")},
		{"overloaded_error in a batch", "batch", errored("overloaded_error", "busy")},
		{"sync 500", "sync", errored("api_error", "server error")},
		{"sync 529", "sync", errored("overloaded_error", "overloaded")},
	}
	for _, tt := range tests {
		t.Run(tt.name+" once is resubmitted", func(t *testing.T) {
			h := newHarness(t, withoutFacts, withPages("b"), withMode(tt.mode))
			h.api.script("s2", "draft", tt.bad)
			s := h.mustRun()
			if got := statusOf(s, pageB); got != StatusAccepted {
				t.Errorf("status = %s (%s)", got, ruleOf(s, pageB))
			}
			if got := len(requestsFor(h, "s2", "draft")); got != 2 {
				t.Errorf("draft requests = %d, want 2", got)
			}
		})
		t.Run(tt.name+" twice escalates", func(t *testing.T) {
			h := newHarness(t, withoutFacts, withPages("b"), withMode(tt.mode))
			h.api.script("s2", "draft", tt.bad, tt.bad)
			s := h.mustRun()
			if statusOf(s, pageB) != StatusEscalated || ruleOf(s, pageB) != "api error" {
				t.Errorf("page = %s (%s)", statusOf(s, pageB), ruleOf(s, pageB))
			}
		})
	}
	for _, k := range []string{batch.KindCanceled, batch.KindExpired} {
		t.Run(k+" once is resubmitted", func(t *testing.T) {
			h := newHarness(t, withoutFacts, withPages("b"))
			h.api.script("s2", "draft", kind(k))
			s := h.mustRun()
			if got := statusOf(s, pageB); got != StatusAccepted {
				t.Errorf("status = %s", got)
			}
		})
		t.Run(k+" twice stops the run as infrastructure", func(t *testing.T) {
			h := newHarness(t, withoutFacts, withPages("b"))
			h.api.script("s2", "draft", kind(k), kind(k))
			s := h.mustRun()
			if s.Stop != StopInfrastructure {
				t.Fatalf("Stop = %q, want %q", s.Stop, StopInfrastructure)
			}
			if statusOf(s, pageB) == StatusEscalated {
				t.Error("an infrastructure stop escalated the page")
			}
			if !strings.Contains(h.err.String(), "--resume") {
				t.Errorf("no resume line: %q", h.err.String())
			}
			if open := h.openIntents(s.RunID); len(open) != 0 {
				t.Errorf("open intents: %v", open)
			}
		})
	}
}

func TestRefusalEscalates(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("b"))
	h.api.script("s2", "draft", outcome{res: batch.Result{Kind: batch.KindSucceeded, StopReason: "refusal", Text: ""}})
	s := h.mustRun()
	if ruleOf(s, pageB) != "refusal" {
		t.Errorf("rule = %q, want refusal", ruleOf(s, pageB))
	}
}

func TestTransportStopsTheRun(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("b"))
	h.api.createErr = &batch.CallError{Kind: batch.ErrTransport, Cause: "timeout"}
	s := h.mustRun()
	if s.Stop != StopTransport {
		t.Fatalf("Stop = %q, want %q", s.Stop, StopTransport)
	}
	if statusOf(s, pageB) == StatusEscalated {
		t.Error("a transport stop escalated the page")
	}
	if !strings.Contains(h.err.String(), "--resume") {
		t.Errorf("no resume line: %q", h.err.String())
	}
	if open := h.openIntents(s.RunID); len(open) != 0 {
		t.Errorf("open intents: %v", open)
	}
}

func TestPlanStage(t *testing.T) {
	t.Run("pages A and B together pass", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withoutContext)
		s := h.mustRun()
		if got := statusOf(s, pageA); got != StatusAccepted {
			t.Errorf("A = %s (%s)\n%s", got, ruleOf(s, pageA), h.err.String())
		}
	})
	t.Run("A alone escalates for its link to B", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withoutContext, withPages("a"))
		s := h.mustRun()
		if statusOf(s, pageA) != StatusEscalated || ruleOf(s, pageA) != "plan" {
			t.Errorf("A = %s (%s), want escalated for plan", statusOf(s, pageA), ruleOf(s, pageA))
		}
		if h.api.creates != 0 {
			t.Errorf("a plan escalation spent a request: %d creates", h.api.creates)
		}
	})
	t.Run("a link to a page the outline lists but no run page writes warns", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("a"))
		s := h.mustRun()
		if got := statusOf(s, pageA); got != StatusAccepted {
			t.Errorf("A = %s (%s)", got, ruleOf(s, pageA))
		}
		if rec := h.record(s.RunID, pageA); !hasFault(rec.Warnings, "revoke-a-key.md") {
			t.Errorf("warnings = %v, want the link to the unwritten page named", rec.Warnings)
		}
	})
}

func TestMarkdownPlanSkipsStageOne(t *testing.T) {
	h := newHarness(t, withPages("b"))
	h.handoffs[0].Plan = "plans/revoke.plan.md"
	h.handoffs[0].Checksum = "" // recomputed below
	writeFile(t, h.root, "plans/revoke.plan.md", "# Plan\n")
	gitIn(t, h.root, "add", "-A")
	gitIn(t, h.root, "commit", "-q", "-m", "markdown plan")
	sum, err := checksum(h.root, "plans/revoke.plan.md")
	if err != nil {
		t.Fatal(err)
	}
	h.handoffs[0].Checksum = sum
	s := h.mustRun()
	if got := statusOf(s, pageB); got != StatusAccepted {
		t.Fatalf("status = %s (%s)", got, ruleOf(s, pageB))
	}
	if n := len(requestsFor(h, "s1", "consistency")); n != 0 {
		t.Errorf("a markdown plan sent %d consistency reads", n)
	}
	rec := h.record(s.RunID, pageB)
	if rec.PlanFormat != "markdown" || len(rec.Warnings) == 0 {
		t.Errorf("record plan_format = %q, warnings = %v", rec.PlanFormat, rec.Warnings)
	}
}

func TestWithoutAFactsStore(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("b"))
	s := h.mustRun()
	if n := len(requestsFor(h, "s1", "consistency")); n != 0 {
		t.Errorf("sent %d consistency reads without a facts store", n)
	}
	if rec := h.record(s.RunID, pageB); rec.Facts != "none" {
		t.Errorf("record facts = %q, want none", rec.Facts)
	}
	twin := newHarness(t, withPages("b"))
	ts := twin.mustRun()
	if n := len(requestsFor(twin, "s1", "consistency")); n != 1 {
		t.Errorf("a facts store sent %d consistency reads, want 1", n)
	}
	if rec := twin.record(ts.RunID, pageB); rec.Facts != "store" {
		t.Errorf("twin record facts = %q", rec.Facts)
	}
}

func TestConsistencyConflictEscalates(t *testing.T) {
	h := newHarness(t, withPages("b"))
	h.api.script("s1", "consistency", okText(`{"conflicts":[{"facts":["lumen-key-ttl"],"with":"lumen-grace","conflict":"they disagree"}]}`))
	s := h.mustRun()
	if ruleOf(s, pageB) != "plan" {
		t.Errorf("rule = %q, want plan", ruleOf(s, pageB))
	}
	if n := len(requestsFor(h, "s2", "draft")); n != 0 {
		t.Errorf("a conflicting plan spent %d draft requests", n)
	}
}

func TestSeatLeftOut(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("b"), withSeats("draft", "structure", "fact", "reader", "coverage"))
	s := h.mustRun()
	if got := statusOf(s, pageB); got != StatusAccepted {
		t.Fatalf("status = %s (%s)", got, ruleOf(s, pageB))
	}
	if n := len(requestsFor(h, "s4", "editor")); n != 0 {
		t.Errorf("the editor seat, left out, was asked %d times", n)
	}
	rec := h.record(s.RunID, pageB)
	found := false
	for _, seat := range rec.SeatsNotRun {
		found = found || seat == "editor"
	}
	if !found {
		t.Errorf("record seats_not_run = %v, want editor named", rec.SeatsNotRun)
	}
}

func TestPagesInFlight(t *testing.T) {
	t.Run("one page at a time", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withInFlight(1))
		h.mustRun()
		// Page A runs every stage first: B's first create comes after A's last.
		lastA := 0
		for i, r := range h.api.requests {
			if strings.Contains(r.CustomID, hash12Of(pageA)) {
				lastA = i
			}
		}
		firstBIdx := -1
		for i, r := range h.api.requests {
			if strings.Contains(r.CustomID, hash12Of(pageB)) {
				firstBIdx = i
				break
			}
		}
		if firstBIdx < lastA {
			t.Errorf("page B's first request (%d) came before page A's last (%d)", firstBIdx, lastA)
		}
	})
	t.Run("both pages start together", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withInFlight(3))
		h.mustRun()
		firstGet := indexOf(h.api.calls, "get")
		creates := 0
		for _, c := range h.api.calls[:firstGet] {
			if c == "create" {
				creates++
			}
		}
		if creates != 2 {
			t.Errorf("creates before the first poll = %d, want both pages' first stage", creates)
		}
	})
}

func indexOf(ss []string, s string) int {
	for i, x := range ss {
		if x == s {
			return i
		}
	}
	return len(ss)
}
