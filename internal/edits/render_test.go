package edits

import (
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	fs := []Finding{
		{N: 1, Seat: "reader", Location: "Rename it", Text: "Vague step.", Rewrite: "Name the field.", Blocking: true},
		{N: 2, Seat: "gate", Location: "line 4", Text: "Heading case.", Rewrite: "Sentence case."},
	}
	want := `Finding 1 (reader, blocking)
Location: Rename it
Text: Vague step.
Rewrite: Name the field.

Finding 2 (gate, advisory)
Location: line 4
Text: Heading case.
Rewrite: Sentence case.
`
	if got := Render(fs); got != want {
		t.Errorf("Render =\n%s\nwant\n%s", got, want)
	}
}

func TestRenderEmpty(t *testing.T) {
	if got := Render(nil); got != "" {
		t.Errorf("Render(nil) = %q, want empty", got)
	}
}

func TestInstructionsNameTheContract(t *testing.T) {
	got := Instructions()
	for _, s := range []string{"0-based", "the first edit is 0", "exactly once", "valid JSON", "page or brief"} {
		if !strings.Contains(got, s) {
			t.Errorf("Instructions lacks %q", s)
		}
	}
}
