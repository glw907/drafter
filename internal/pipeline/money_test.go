package pipeline

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/batch"
)

func TestPageCapEscalatesBeforeSending(t *testing.T) {
	tests := []struct {
		name      string
		maxTokens int64
		wantCap   bool
	}{
		// 1000 input tokens cost 0.002 in batch mode, under the cap of 1; 300000 output tokens
		// add 3.0, over it.
		{"input fits, input plus max_tokens does not", 300000, true},
		{"the twin with a smaller max_tokens", 4000, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, withoutFacts, withPages("b"), withPageCap(1), withMaxTokens("draft", tt.maxTokens))
			s := h.mustRun()
			if tt.wantCap {
				if got := statusOf(s, pageB); got != StatusEscalated || ruleOf(s, pageB) != "cap" {
					t.Errorf("page = %s (%s), want escalated for cap", got, ruleOf(s, pageB))
				}
				if h.api.creates != 0 || h.api.sends != 0 {
					t.Errorf("sent %d creates and %d sends, want none", h.api.creates, h.api.sends)
				}
				return
			}
			if h.api.creates == 0 {
				t.Error("the twin never submitted")
			}
			if ruleOf(s, pageB) == "cap" {
				t.Error("the twin escalated for cap")
			}
		})
	}
}

func TestCycleCeilingStopsTheRun(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("b"), withCeiling(0.01))
	s := h.mustRun()
	if s.Stop != StopCycleCap {
		t.Fatalf("Stop = %q, want %q", s.Stop, StopCycleCap)
	}
	if st := statusOf(s, pageB); st == StatusEscalated {
		t.Errorf("a cycle stop escalated the page")
	}
	if !strings.Contains(s.ResumeLine, "--resume "+s.RunID) || !strings.Contains(h.err.String(), s.ResumeLine) {
		t.Errorf("resume line %q not printed to stderr %q", s.ResumeLine, h.err.String())
	}
	if h.api.creates != 0 {
		t.Errorf("created %d batches past the ceiling", h.api.creates)
	}
	if st := h.state(s.RunID); st.Stopped != StopCycleCap {
		t.Errorf("state.Stopped = %q", st.Stopped)
	}
}

func TestSpendLimitStopsTheRun(t *testing.T) {
	limit := "You have reached your specified workspace API usage limits."
	tests := []struct {
		name  string
		setup func(h *harness)
	}{
		{"create refused", func(h *harness) {
			h.api.createErr = &batch.CallError{Kind: batch.ErrSpendLimit, Status: 400, Message: limit}
		}},
		{"result errored with the limit", func(h *harness) {
			h.api.script("s2", "draft", errored("invalid_request_error", limit))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, withoutFacts, withPages("b"))
			tt.setup(h)
			s := h.mustRun()
			if s.Stop != StopSpendLimit {
				t.Fatalf("Stop = %q, want %q", s.Stop, StopSpendLimit)
			}
			if statusOf(s, pageB) == StatusEscalated {
				t.Error("a spend limit escalated the page")
			}
			if !strings.Contains(h.err.String(), "--resume") {
				t.Errorf("no resume line in %q", h.err.String())
			}
			if open := h.openIntents(s.RunID); len(open) != 0 {
				t.Errorf("open intents after the stop: %v", open)
			}
		})
	}
}

func TestSubmitFollowsReserveAndStateIntent(t *testing.T) {
	h := newHarness(t, withPages("b"))
	var checked int
	h.api.onSubmit = func(ids []string) {
		checked++
		reserves := h.ledgerLines("reserve")
		if len(reserves) < checked {
			t.Errorf("submission %d had only %d reserve lines before it", checked, len(reserves))
		}
		matches, err := filepath.Glob(filepath.Join(h.stateDir, "runs", "*", "*", "state.json"))
		if err != nil || len(matches) != 1 {
			t.Fatalf("state files = %v, %v", matches, err)
		}
		data, err := os.ReadFile(matches[0])
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range ids {
			if !strings.Contains(string(data), id) {
				t.Errorf("state holds no intent naming %s at submission time", id)
			}
		}
	}
	h.mustRun()
	if checked == 0 {
		t.Fatal("the fake saw no submission")
	}
}

func TestMaybeSentCreate(t *testing.T) {
	draftRequests := func(h *harness) int {
		n := 0
		for _, r := range h.api.requests {
			if stage, seat := stageSeat(r.CustomID); stage == "s2" && seat == "draft" {
				n++
			}
		}
		return n
	}
	t.Run("a batch the fake recorded is adopted", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		h.api.createSentErr = maybeSent()
		h.mustRun()
		if got := draftRequests(h); got != 1 {
			t.Errorf("draft requests = %d, want 1 (adopted, never recreated)", got)
		}
		if h.settleKinds()["void"] != 0 {
			t.Errorf("an adopted batch left a void line: %v", h.settleKinds())
		}
		if !contains(h.api.calls, "list") {
			t.Errorf("adoption never listed batches: %v", h.api.calls)
		}
	})
	t.Run("a batch the fake never recorded is resubmitted once", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		h.api.createErrs = map[int]error{1: maybeSent()}
		h.mustRun()
		if got := draftRequests(h); got != 1 {
			// The failed first create recorded nothing, so only the resubmission is a request.
			t.Errorf("draft requests = %d, want 1", got)
		}
		if got := h.settleKinds()["void"]; got != 1 {
			t.Errorf("void lines = %d, want 1 for the first intent", got)
		}
		if h.api.creates < 2 {
			t.Errorf("creates = %d, want a resubmission", h.api.creates)
		}
	})
}

func TestMaybeSentSendSettlesEstimated(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("b"), withMode("sync"))
	h.api.sendErrs = map[int]error{1: maybeSent()}
	h.mustRun()
	if got := h.settleKinds()["estimated"]; got != 1 {
		t.Errorf("estimated lines = %d, want 1", got)
	}
	reserves := h.ledgerLines("reserve")
	if len(reserves) < 2 || reserves[0]["intent"] == reserves[1]["intent"] {
		t.Fatalf("want a new reserved intent after the estimate, got %v", reserves)
	}
}

func TestCreateRefusals(t *testing.T) {
	tests := []struct {
		name   string
		status int
		typ    string
	}{
		{"400", 400, "invalid_request_error"},
		{"403", 403, "permission_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, withoutFacts, withPages("b"))
			h.api.createErrs = map[int]error{1: callError(tt.status, tt.typ, "refused")}
			s := h.mustRun()
			if statusOf(s, pageB) != StatusEscalated || ruleOf(s, pageB) != "api error" {
				t.Errorf("page = %s (%s), want escalated for api error", statusOf(s, pageB), ruleOf(s, pageB))
			}
			if got := h.settleKinds()["void"]; got != 1 {
				t.Errorf("void lines = %d, want 1", got)
			}
			if open := h.openIntents(s.RunID); len(open) != 0 {
				t.Errorf("open intents: %v", open)
			}
			if h.api.creates != 1 {
				t.Errorf("creates = %d, want 1", h.api.creates)
			}
		})
	}
}

func TestCreateWithoutWorkspaceCheckIsAnError(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("b"))
	h.api.createErr = &batch.CallError{Kind: batch.ErrNoWorkspaceCheck, Message: "check the workspace first"}
	_, err := h.run()
	if !errors.Is(err, batch.ErrNoWorkspaceCheck) {
		t.Fatalf("Run error = %v, want ErrNoWorkspaceCheck", err)
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
