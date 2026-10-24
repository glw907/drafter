package pipeline

import (
	"strings"
	"testing"
)

// batchHolding returns the fake batch holding a request whose custom_id contains sub.
func (f *fakeAPI) batchHolding(sub string) *fakeBatch {
	for _, b := range f.batches {
		for _, id := range ids(b.reqs) {
			if strings.Contains(id, sub) {
				return b
			}
		}
	}
	return nil
}

// slowPage makes the batches whose custom_ids contain a sub key in late take that many polls to
// end, and every other batch end at its first poll.
func slowPage(late map[string]int) func([]string) int {
	return func(ids []string) int {
		for _, id := range ids {
			for sub, n := range late {
				if strings.Contains(id, sub) {
					return n
				}
			}
		}
		return 0
	}
}

func TestPageAdvancesBeforeASiblingBatchEnds(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("a", "b"))
	bFirst := hash12Of(pageB) + "_s2_draft_1"
	h.api.pollsFor = slowPage(map[string]int{bFirst: 3})
	aNext := 0
	bPending := false
	h.api.onSubmit = func(submitted []string) {
		for _, id := range submitted {
			if !strings.Contains(id, hash12Of(pageA)) || strings.Contains(id, "_s2_") {
				continue
			}
			aNext++
			if aNext == 1 {
				b := h.api.batchHolding(bFirst)
				bPending = b != nil && b.polls <= h.api.pollsFor(ids(b.reqs))
			}
		}
	}
	s := h.mustRun()
	mustAccept(t, h, s)
	if got := statusOf(s, pageB); got != StatusAccepted {
		t.Errorf("page B = %s, want accepted", got)
	}
	if aNext == 0 {
		t.Fatal("page A never submitted a second stage")
	}
	if !bPending {
		t.Error("page A's second stage waited for page B's first batch to end")
	}
}

func TestResumeAfterACrashBesideASlowSibling(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("a", "b"))
	h.api.pollsFor = slowPage(map[string]int{hash12Of(pageB) + "_s2_draft_1": 3})
	h.api.crash = func(call string, ids []string) string {
		if call == "create" && strings.Contains(ids[0], hash12Of(pageA)) && !strings.Contains(ids[0], "_s2_") {
			return "before"
		}
		return ""
	}
	if !dies(func() { _, _ = h.run() }) {
		t.Fatal("the run did not die at page A's second-stage create")
	}
	h.api.crash = nil
	s, err := h.resume(h.runID())
	if err != nil {
		t.Fatalf("resume: %v\nstderr: %s", err, h.err.String())
	}
	mustAccept(t, h, s)
	if got := statusOf(s, pageB); got != StatusAccepted {
		t.Errorf("after the resume page B = %s, want accepted", got)
	}
}

func TestSiblingHoldWhileTheSiblingFixBatchIsLate(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("b", "a"))
	h.api.pollsFor = slowPage(map[string]int{
		hash12Of(pageB) + "_s2_draft_2": 3,
		hash12Of(pageA) + "_s2_draft_1": 1,
	})
	h.api.script(s2, "draft",
		okText(draftText(goodPageB+"\nBROKEN\n", goodBriefB)),
		okText(draftText(goodPage, goodBrief)),
		okText(answer("BROKEN", "fixed", 1)))
	s := h.mustRun()
	mustAccept(t, h, s)
	if got := statusOf(s, pageB); got != StatusAccepted {
		t.Errorf("page B = %s, want accepted after its fix batch", got)
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
