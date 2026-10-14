package pipeline

import (
	"errors"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/batch"
)

func transportErr() error { return &batch.CallError{Kind: batch.ErrTransport, Cause: "timeout"} }

func spendErr() error {
	return &batch.CallError{Kind: batch.ErrSpendLimit, Status: 400, Message: "You have reached your specified workspace API usage limits."}
}

func TestCountTokensFailures(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantStop string
		wantRule string
	}{
		{"spend limit", spendErr(), StopSpendLimit, ""},
		{"transport", transportErr(), StopTransport, ""},
		{"a 4xx refusal", callError(400, "invalid_request_error", "too long"), "", "api error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, withoutFacts, withPages("b"))
			h.api.countErr = tt.err
			s := h.mustRun()
			if s.Stop != tt.wantStop {
				t.Errorf("Stop = %q, want %q", s.Stop, tt.wantStop)
			}
			if tt.wantRule != "" && ruleOf(s, pageB) != tt.wantRule {
				t.Errorf("rule = %q, want %q", ruleOf(s, pageB), tt.wantRule)
			}
			if tt.wantRule == "" && statusOf(s, pageB) == StatusEscalated {
				t.Error("a stop escalated the page")
			}
			if n := len(h.ledgerLines("reserve")); n != 0 {
				t.Errorf("reserve lines = %d, want none before a count succeeds", n)
			}
			if h.api.creates != 0 {
				t.Errorf("creates = %d", h.api.creates)
			}
		})
	}
	t.Run("a missing workspace check is an error", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		h.api.countErr = &batch.CallError{Kind: batch.ErrNoWorkspaceCheck}
		if _, err := h.run(); !errors.Is(err, batch.ErrNoWorkspaceCheck) {
			t.Errorf("error = %v", err)
		}
	})
}

func TestAdoptionFailureLeavesTheIntentOpen(t *testing.T) {
	t.Run("a failed list", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		h.api.createErrs = map[int]error{1: maybeSent()}
		h.api.listErr = transportErr()
		id := mustStop(t, h, StopTransport)
		if got := h.settleKinds()["void"]; got != 0 {
			t.Errorf("void lines = %d, want none: the search proved nothing", got)
		}
		if open := h.openIntents(id); len(open) != 1 {
			t.Errorf("open intents = %d, want the one the search could not settle", len(open))
		}
		h.api.listErr = nil
		s, err := h.resume(id)
		if err != nil {
			t.Fatal(err)
		}
		if got := statusOf(s, pageB); got != StatusAccepted {
			t.Errorf("status = %s (%s)", got, ruleOf(s, pageB))
		}
		if h.settleKinds()["void"] != 1 {
			t.Errorf("settle kinds = %v, want the unmatched intent voided by the resume", h.settleKinds())
		}
	})
	t.Run("a failed results download", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		h.api.createSentErr = maybeSent()
		h.api.resultsErr = transportErr()
		id := mustStop(t, h, StopTransport)
		if h.settleKinds()["void"] != 0 || len(h.openIntents(id)) != 1 {
			t.Errorf("void %d, open %d, want no void and one open intent", h.settleKinds()["void"], len(h.openIntents(id)))
		}
		h.api.resultsErr = nil
		s, err := h.resume(id)
		if err != nil {
			t.Fatal(err)
		}
		if got := statusOf(s, pageB); got != StatusAccepted {
			t.Errorf("status = %s", got)
		}
		if n := len(requestsFor(h, "s2", "draft")); n != 1 {
			t.Errorf("draft requests = %d, want the adopted batch only", n)
		}
	})
}

func TestSecondUnknownOutcomeStopsTheRun(t *testing.T) {
	t.Run("batch", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"))
		h.api.createErrs = map[int]error{1: maybeSent(), 2: maybeSent()}
		s := h.mustRun()
		if s.Stop != StopTransport {
			t.Fatalf("Stop = %q", s.Stop)
		}
		if got := h.settleKinds()["void"]; got != 2 {
			t.Errorf("void lines = %d, want both intents voided", got)
		}
		if open := h.openIntents(s.RunID); len(open) != 0 {
			t.Errorf("open intents: %v", open)
		}
	})
	t.Run("sync", func(t *testing.T) {
		h := newHarness(t, withoutFacts, withPages("b"), withMode("sync"))
		h.api.sendErrs = map[int]error{1: maybeSent(), 2: maybeSent()}
		s := h.mustRun()
		if s.Stop != StopTransport {
			t.Fatalf("Stop = %q", s.Stop)
		}
		if got := h.settleKinds()["estimated"]; got != 2 {
			t.Errorf("estimated lines = %d, want one per lost attempt", got)
		}
	})
}

func TestSyncSendFailures(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantStop string
		wantRule string
	}{
		{"a 400", callError(400, "invalid_request_error", "bad"), "", "api error"},
		{"transport", transportErr(), StopTransport, ""},
		{"spend limit", spendErr(), StopSpendLimit, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, withoutFacts, withPages("b"), withMode("sync"))
			h.api.sendErrs = map[int]error{1: tt.err}
			s := h.mustRun()
			if s.Stop != tt.wantStop || (tt.wantRule != "" && ruleOf(s, pageB) != tt.wantRule) {
				t.Errorf("Stop %q, rule %q, want %q and %q", s.Stop, ruleOf(s, pageB), tt.wantStop, tt.wantRule)
			}
			if got := h.settleKinds()["void"]; got != 1 {
				t.Errorf("void lines = %d, want 1", got)
			}
			if open := h.openIntents(s.RunID); len(open) != 0 {
				t.Errorf("open intents: %v", open)
			}
		})
	}
}

func TestPollingWaitsForABatchToEnd(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("b"))
	h.api.pollsToEnd = 2
	s := h.mustRun()
	if got := statusOf(s, pageB); got != StatusAccepted {
		t.Fatalf("status = %s", got)
	}
	gets := 0
	for _, c := range h.api.calls {
		if c == "get" {
			gets++
		}
	}
	if gets < 3 {
		t.Errorf("get calls = %d, want the polls before the first batch ended", gets)
	}
}

func TestPollFailureStopsAndResumeReattaches(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("b"))
	h.api.getErr = transportErr()
	id := mustStop(t, h, StopTransport)
	if open := h.openIntents(id); len(open) != 1 {
		t.Fatalf("open intents = %d, want the batch the poll could not read", len(open))
	}
	h.api.getErr = nil
	s, err := h.resume(id)
	if err != nil {
		t.Fatal(err)
	}
	if got := statusOf(s, pageB); got != StatusAccepted {
		t.Errorf("status = %s", got)
	}
	if n := len(requestsFor(h, "s2", "draft")); n != 1 {
		t.Errorf("draft requests = %d, want the one batch reattached", n)
	}
}

func TestEveryIntentIsReservedBeforeItIsSent(t *testing.T) {
	for _, mode := range []string{"batch", "sync"} {
		t.Run(mode, func(t *testing.T) {
			h := newHarness(t, withoutFacts, withPages("b"), withMode(mode))
			h.api.onSubmit = func(ids []string) {
				intent := ""
				for _, ps := range h.stateOnDisk().Pages {
					for _, in := range ps.Intents {
						for _, rq := range in.Requests {
							if rq.CustomID == ids[0] {
								intent = in.ID
							}
						}
					}
				}
				if intent == "" {
					t.Errorf("no state intent names %s at submission time", ids[0])
					return
				}
				for _, l := range h.ledgerLines("reserve") {
					if l["intent"] == intent {
						return
					}
				}
				t.Errorf("no reserve line for intent %s of %s at submission time", intent, ids[0])
			}
			h.mustRun()
		})
	}
}

func TestResolutionsOfEveryExchangeStayInTheRecord(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("a"))
	structureFixes(h)
	h.api.script("s5", "draft", okText(answer("Create a new key.", "Create a new key in the console.", 1)))
	h.api.script("s7", "reader", okText(`{"paraphrase":"p","summary":"s"}`),
		okText(readText("fix", finding("Rotate the key", "Say which key."))))
	h.api.script("s7", "draft", okText(answer("in the console.", "in the console, then publish.", 1)))
	s := h.mustRun()
	if got := statusOf(s, pageA); got != StatusAccepted {
		t.Fatalf("status = %s (%s)", got, ruleOf(s, pageA))
	}
	stages := map[string]bool{}
	for _, r := range h.record(s.RunID, pageA).Resolutions {
		stages[r.Stage] = true
	}
	if !stages["s5"] || !stages["s7"] {
		t.Errorf("record resolutions cover stages %v, want s5 and s7", stages)
	}
}

func TestDeclinedReaderFindingReachesTheRetest(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("a"))
	h.api.script("s7", "reader", okText(`{"paraphrase":"p","summary":"s"}`),
		okText(readText("fix", finding("Rotate the key", "Say which key."))))
	h.api.script("s7", "draft", okText(declined(1, "the page names the key in step 1")))
	h.mustRun()
	retest := requestsFor(h, "s7", "reader")
	if len(retest) != 4 {
		t.Fatalf("reader requests = %d, want 4", len(retest))
	}
	if text := userText(retest[3]); !strings.Contains(text, "Declined finding 1") || !strings.Contains(text, "names the key in step 1") {
		t.Errorf("the retest lacks the declined finding:\n%s", text)
	}
	if strings.Contains(userText(retest[2]), "Declined finding") {
		t.Error("the retest's paraphrase turn carries the declined finding, which would show the reader the findings")
	}
}

func TestSharedIndexFileKeepsAnotherPagesLink(t *testing.T) {
	h := newHarness(t, withoutFacts)
	writeFile(t, h.root, "docs/index.md", "# Index\n")
	gitIn(t, h.root, "add", "-A")
	gitIn(t, h.root, "commit", "-q", "-m", "index")
	h.cfg.Gate.IndexLinkCommand = "printf '%s\\n' {page} >> docs/index.md"
	// Page A's round-1 read comes first in the queue: it asks for a redraft the model refuses
	// three times, so A escalates after page B's link command has run and B is accepted.
	h.api.answer = func(r batch.Request) string {
		if stage, seat := stageSeat(r.CustomID); stage == "s4" && seat == "structure" && strings.Contains(r.CustomID, hash12Of(pageA)) {
			return readText("fix", finding("Rotate the key", "Name the key."))
		}
		return ""
	}
	refused := answer("THIS TEXT IS NOT ON THE PAGE", "x", 1)
	h.api.script("s5", "draft", okText(refused), okText(refused), okText(refused))
	s := h.mustRun()
	if ruleOf(s, pageA) != "contract" || statusOf(s, pageB) != StatusAccepted {
		t.Fatalf("A = %s (%s), B = %s", statusOf(s, pageA), ruleOf(s, pageA), statusOf(s, pageB))
	}
	index := h.read("docs/index.md")
	if !strings.Contains(index, pageB) {
		t.Errorf("index = %q, want the accepted page's link kept", index)
	}
	rec := h.record(s.RunID, pageA)
	if !hasFault(rec.Friction, "not restored") {
		t.Errorf("friction = %v, want the index file left as it is, said so", rec.Friction)
	}
	writes := h.state(s.RunID).Pages[pageKey(pageA)].Writes
	last := writes[len(writes)-1]
	if !last.Restore || last.Post == nil {
		t.Errorf("last write = %+v, want a finished restore recorded with its hashes", last)
	}
	if h.exists(pageA) {
		t.Error("the escalated page is still on disk")
	}
}
