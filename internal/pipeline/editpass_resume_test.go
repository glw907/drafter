package pipeline

import (
	"slices"
	"testing"
)

func TestResumeAdoptsTheEditRequestAfterACrash(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"), withDrafting)
	h.api.crash = crashAfterCreate("_s2e_edit_1")
	if !dies(func() { _, _ = h.runDrafting(drafting{narrow: true}) }) {
		t.Fatal("the run did not die after the edit request was created")
	}
	id := h.runID()
	h.api.crash = nil
	s, err := h.resume(id)
	if err != nil {
		t.Fatalf("resume: %v\nstderr: %s", err, h.err.String())
	}
	mustAccept(t, h, s)
	if got, want := sequence(h), []string{"s2.draft", "s2e.edit"}; !slices.Equal(got, want) {
		t.Errorf("requests = %v, want the edit adopted, not sent again: %v", got, want)
	}
	assertSettledOnce(t, h)
}
