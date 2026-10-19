package assemble

import (
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/plan"
)

func TestDraftingFactsBlockChangesOnlyTheOpening(t *testing.T) {
	view := &plan.Plan{Sections: []plan.Section{{Heading: "One", Items: []plan.Item{{ID: "f", Kind: "fact", Fact: "a-fact"}}}}}
	read, draft := FactsBlock(view, nil), DraftingFactsBlock(view, nil)
	readOpen, readRest, _ := strings.Cut(read, "\n\n")
	draftOpen, draftRest, _ := strings.Cut(draft, "\n\n")
	if readRest != draftRest {
		t.Errorf("the blocks differ past the opening:\n%s\n---\n%s", readRest, draftRest)
	}
	if !strings.Contains(readOpen, "They are everything you read") {
		t.Errorf("the read's opening changed: %q", readOpen)
	}
	if strings.Contains(draftOpen, "everything you read") || draftOpen == readOpen {
		t.Errorf("the drafting opening still says the blocks are everything read: %q", draftOpen)
	}
	if !strings.Contains(draftRest, "- a-fact") {
		t.Errorf("the block lacks the placed fact: %q", draftRest)
	}
}
