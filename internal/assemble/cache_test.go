package assemble

import (
	"context"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/plan"
)

func TestReadRequestCachedParts(t *testing.T) {
	f := newFixture(t)
	c := f.config()
	c.Seat["blueprint"] = config.Seat{Model: "model-x", Effort: "high", MaxTokens: 4000}
	facts := FactsBlock(&plan.Plan{}, nil)
	in := f.input(c, "Read the blueprint. The register is docs/guide.md.")
	in.Seat, in.Answer = "blueprint", AnswerRead
	in.Cached = []string{facts}
	in.Extra = BlueprintBlock([]byte("title: T\n"), []byte(`{"path":"docs/page.md"}`)) +
		"\n\nThese sections changed: intro.\n\n" + ChangeBlock("-a\n+b\n", nil, nil, nil)
	req, err := Build(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("%d messages, want 1", len(req.Messages))
	}
	m := req.Messages[0]
	if len(m.Cached) != 2 {
		t.Fatalf("%d cached parts, want 2", len(m.Cached))
	}
	if !strings.HasPrefix(m.Cached[0], "Files and command outputs") || !strings.HasSuffix(m.Cached[0], "=== end file: docs/guide.md ===") {
		t.Errorf("first block should open with the note and end with the register file:\n%s", m.Cached[0])
	}
	if strings.Contains(m.Cached[0], "=== blueprint ===") {
		t.Error("the blueprint is in the first block")
	}
	if m.Cached[1] != facts || !strings.Contains(facts, "=== facts placed or cited ===") {
		t.Errorf("second block = %q, want the facts block", m.Cached[1])
	}
	for _, want := range []string{"=== blueprint ===", "=== end outline entry ===\n\nThese sections changed: intro.", "=== blueprint diff ===", "Answer with a verdict"} {
		if !strings.Contains(m.Text, want) {
			t.Errorf("third block lacks %q:\n%s", want, m.Text)
		}
	}
	if strings.Contains(m.Text, "=== facts placed") {
		t.Error("the facts are in the third block")
	}
}

func TestNoCachedPartsKeepsOneBlock(t *testing.T) {
	f := newFixture(t)
	req, err := Build(context.Background(), f.input(f.config(), "Write the page."))
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Messages) != 1 || req.Messages[0].Cached != nil {
		t.Errorf("messages = %+v, want one plain block", req.Messages)
	}
}
