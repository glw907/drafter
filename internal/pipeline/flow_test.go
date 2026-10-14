package pipeline

import (
	"testing"
)

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
		})
	}
}
