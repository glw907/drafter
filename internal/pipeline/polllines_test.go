package pipeline

import (
	"strings"
	"testing"
	"time"

	"github.com/glw907/drafter/internal/batch"
	"github.com/glw907/drafter/internal/run"
)

func TestPollLinesPrintOnChange(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("b"))
	h.api.pollsToEnd = 3
	s := h.mustRun()
	if got := statusOf(s, pageB); got != StatusAccepted {
		t.Fatalf("status = %s", got)
	}
	inProgress := map[string]int{}
	ended := map[string]int{}
	hints := 0
	for line := range strings.SplitSeq(h.err.String(), "\n") {
		id, _, _ := strings.Cut(strings.TrimPrefix(line, "batch "), " ")
		switch {
		case strings.HasPrefix(line, "batch ") && strings.Contains(line, "): in_progress, waiting since "):
			inProgress[id]++
		case strings.HasPrefix(line, "batch ") && strings.Contains(line, "): ended, ") && strings.Contains(line, " succeeded, 0 errored, 0 canceled, 0 expired"):
			ended[id]++
		case strings.Contains(line, "--sync"):
			hints++
		}
	}
	if len(inProgress) == 0 {
		t.Fatalf("no in-progress line in:\n%s", h.err.String())
	}
	for id, n := range inProgress {
		if n != 1 || ended[id] != 1 {
			t.Errorf("batch %s printed %d in-progress and %d ended lines, want one each", id, n, ended[id])
		}
	}
	if hints != 1 {
		t.Errorf("--sync named %d times, want once", hints)
	}
	for _, in := range h.state(s.RunID).Pages[run.PageKey(pageB)].Intents {
		if in.Poll == nil || in.Poll.Status != "ended" || in.Poll.Succeeded == 0 {
			t.Errorf("intent %s poll = %+v, want the ended status and counts", in.ID, in.Poll)
		}
	}
}

func TestAdoptionSearchPrintsAndWritesNothing(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("b"))
	h.api.createSentErr = &batch.CallError{Kind: batch.ErrMaybeSent}
	h.api.addDecoy("msgbatch_decoy", time.Now())
	s := h.mustRun()
	if !strings.Contains(h.err.String(), "(adoption search): ended, ") {
		t.Errorf("no adoption line in:\n%s", h.err.String())
	}
	for _, p := range h.state(s.RunID).Pages {
		for _, in := range p.Intents {
			if in.BatchID == "msgbatch_decoy" || (in.Poll != nil && in.Poll.Status == "") {
				t.Errorf("intent %s took the decoy or an empty poll: %+v", in.ID, in)
			}
		}
	}
}
