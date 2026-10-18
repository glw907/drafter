package blueprint

import (
	"slices"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/plan"
)

const sinceBase = `version: 1
argument: |
  Keys rotate in three steps.
introduction:
  covers: |
    Rotating a key.
  priorKnowledge: |
    None.
  notCovered: |
    Revoking.
  facts: [f:intro]
framing:
  readers: |
    Operators.
  background: |
    Background that cites ` + "`f:frame`" + `.
orderReason: |
  Steps run in order.
sections:
  - id: first
    heading: First
    why: |
      It comes first.
    opens: |
      Start here.
    items:
      - id: first-fact
        kind: fact
        fact: f:one
      - id: first-step
        kind: step
        text: |
          Do the first thing.
  - id: second
    heading: Second
    why: |
      Next.
    opens: |
      Then this.
    items:
      - id: second-fact
        kind: fact
        fact: f:two
  - id: third
    heading: Third
    level: 3
    why: |
      Last.
    opens: |
      Finish.
    items:
      - id: third-sentence
        kind: sentence
        mode: claim
        text: |
          Done.
        facts: [f:three]
`

const sinceFacts = "## Facts\n\n- `f:intro` The intro fact. Source: `a/b.go`. [verified]\n" +
	"- `f:frame` The framing fact. [verified]\n- `f:one` One. [verified]\n- `f:two` Two. [verified]\n- `f:three` Three. [verified]\n"

var sinceEntry = Page{Path: "docs/a.md", Slug: "a", Title: "A", Type: "task guide", Job: "Do A.", Managed: true}

func sinceRev(bp, facts string, entry *Page) Revision {
	return Revision{Blueprint: []byte(bp), Entry: entry, Facts: plan.ParseFacts(map[string]string{"facts.md": facts}, "/nowhere")}
}

func TestCompare(t *testing.T) {
	edit := func(old, new string) string { return strings.Replace(sinceBase, old, new, 1) }
	changedEntry := sinceEntry
	changedEntry.Job = "Do A differently."

	tests := []struct {
		name  string
		bp    string
		facts string
		entry *Page
		// oldBP is the earlier blueprint; sinceBase when empty.
		oldBP string
		want  Scope
	}{
		{"unchanged", sinceBase, sinceFacts, &sinceEntry, "", Scope{Kind: ScopeUnchanged}},
		{"reformatted", strings.Replace(sinceBase, "    heading: First\n", "    heading:   \"First\"\n", 1), sinceFacts, &sinceEntry, "", Scope{Kind: ScopeUnchanged}},
		{"an item edit", edit("Do the first thing.", "Do the first thing, carefully."), sinceFacts, &sinceEntry, "", Scope{Kind: ScopeScoped, IDs: []string{"first"}}},
		{"a new section", sinceBase + "  - id: fourth\n    heading: Fourth\n    why: |\n      More.\n    opens: |\n      Also.\n    items:\n      - id: fourth-step\n        kind: step\n        text: |\n          Extra.\n", sinceFacts, &sinceEntry, "", Scope{Kind: ScopeScoped, IDs: []string{"fourth"}}},
		{"a removed section", sinceBase[:strings.Index(sinceBase, "  - id: third")], sinceFacts, &sinceEntry, "", Scope{Kind: ScopeWhole, Why: "removed"}},
		{"a reorder", strings.Replace(strings.Replace(strings.Replace(sinceBase, "id: first\n", "id: TMP\n", 1), "id: second\n", "id: first\n", 1), "id: TMP\n", "id: second\n", 1), sinceFacts, &sinceEntry, "", Scope{Kind: ScopeWhole, Why: "order"}},
		{"a level change", edit("    heading: Second\n", "    heading: Second\n    level: 3\n"), sinceFacts, &sinceEntry, "", Scope{Kind: ScopeWhole, Why: "level"}},
		{"a field outside sections", edit("Steps run in order.", "Steps run in a new order."), sinceFacts, &sinceEntry, "", Scope{Kind: ScopeWhole, Why: "outside sections"}},
		{"an outline entry change", sinceBase, sinceFacts, &changedEntry, "", Scope{Kind: ScopeWhole, Why: "outline entry"}},
		{"undecodable at the earlier revision", sinceBase, sinceFacts, &sinceEntry, "sections: [", Scope{Kind: ScopeWhole, Why: "undecodable"}},
		{"a fact bullet change marks each section that cites it", sinceBase, strings.Replace(sinceFacts, "One.", "One, revised.", 1), &sinceEntry, "", Scope{Kind: ScopeScoped, IDs: []string{"first"}}},
		{"a changed fact in a facts list", sinceBase, strings.Replace(sinceFacts, "Three.", "Three, revised.", 1), &sinceEntry, "", Scope{Kind: ScopeScoped, IDs: []string{"third"}}},
		{"a changed fact cited from the framing", sinceBase, strings.Replace(sinceFacts, "The framing fact.", "A new framing fact.", 1), &sinceEntry, "", Scope{Kind: ScopeWhole, Why: "cited outside"}},
		{"a changed fact cited from the introduction", sinceBase, strings.Replace(sinceFacts, "The intro fact.", "A new intro fact.", 1), &sinceEntry, "", Scope{Kind: ScopeWhole, Why: "cited outside"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldBP := tt.oldBP
			if oldBP == "" {
				oldBP = sinceBase
			}
			got := Compare("a.blueprint.yaml", sinceRev(oldBP, sinceFacts, &sinceEntry), sinceRev(tt.bp, tt.facts, tt.entry))
			if got.Kind != tt.want.Kind || !slices.Equal(got.IDs, tt.want.IDs) {
				t.Errorf("Compare = %+v, want kind %s ids %v", got, tt.want.Kind, tt.want.IDs)
			}
			if !strings.Contains(got.Why, tt.want.Why) {
				t.Errorf("Why = %q, want it to mention %q", got.Why, tt.want.Why)
			}
		})
	}
}
