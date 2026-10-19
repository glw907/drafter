package blueprint

import (
	"encoding/json"
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
	"- `f:frame` The framing fact. [verified]\n- `f:one` One. [verified]\n- `f:two` Two. [verified]\n- `f:three` Three. [verified]\n- `f:other` Other. [verified]\n"

var sinceEntry = Page{Path: "docs/a.md", Slug: "a", Title: "A", Type: "task guide", Job: "Do A.", Managed: true}

func sinceRev(bp, facts string, entry *Page) Revision {
	return Revision{Blueprint: []byte(bp), Entry: entry, Facts: plan.ParseFacts(map[string]string{"facts.md": facts}, "/nowhere")}
}

func TestCompare(t *testing.T) {
	edit := func(old, new string) string { return strings.Replace(sinceBase, old, new, 1) }
	changedEntry := sinceEntry
	changedEntry.Job = "Do A differently."
	iFirst, iSecond, iThird := strings.Index(sinceBase, "  - id: first"), strings.Index(sinceBase, "  - id: second"), strings.Index(sinceBase, "  - id: third")
	swapped := sinceBase[:iFirst] + sinceBase[iSecond:iThird] + sinceBase[iFirst:iSecond] + sinceBase[iThird:]

	none := []string{}
	withDisp := edit("sections:\n", "dispositions:\n  - fact: f:other\n    disposition: cut\n    reason: |\n      Another page owns it.\nsections:\n")
	tests := []struct {
		name  string
		bp    string
		facts string
		entry *Page
		// oldBP is the earlier blueprint; sinceBase when empty.
		oldBP string
		// oldEntry is the earlier outline entry; sinceEntry when nil.
		oldEntry *Page
		noEntry  bool
		want     Scope
	}{
		{name: "unchanged", bp: sinceBase, facts: sinceFacts, entry: &sinceEntry, want: Scope{Kind: ScopeUnchanged}},
		{name: "reformatted", bp: strings.Replace(sinceBase, "    heading: First\n", "    heading:   \"First\"\n", 1), facts: sinceFacts, entry: &sinceEntry, want: Scope{Kind: ScopeUnchanged}},
		{name: "an item edit", bp: edit("Do the first thing.", "Do the first thing, carefully."), facts: sinceFacts, entry: &sinceEntry, want: Scope{Kind: ScopeScoped, IDs: []string{"first"}, Facts: none}},
		{name: "an item edited from fact A to fact B", bp: edit("fact: f:one", "fact: f:two"), facts: sinceFacts, entry: &sinceEntry, want: Scope{Kind: ScopeScoped, IDs: []string{"first"}, Facts: []string{"f:one", "f:two"}}},
		{name: "a new section", bp: sinceBase + "  - id: fourth\n    heading: Fourth\n    why: |\n      More.\n    opens: |\n      Also.\n    items:\n      - id: fourth-step\n        kind: step\n        text: |\n          Extra.\n", facts: sinceFacts, entry: &sinceEntry, want: Scope{Kind: ScopeScoped, IDs: []string{"fourth"}, Facts: none}},
		{name: "a removed section", bp: sinceBase[:strings.Index(sinceBase, "  - id: third")], facts: sinceFacts, entry: &sinceEntry, want: Scope{Kind: ScopeScoped, Facts: []string{"f:three"}}},
		{name: "a reorder", bp: swapped, facts: sinceFacts, entry: &sinceEntry, want: Scope{Kind: ScopeScoped, Facts: none}},
		{name: "a level change", bp: edit("    heading: Second\n", "    heading: Second\n    level: 3\n"), facts: sinceFacts, entry: &sinceEntry, want: Scope{Kind: ScopeScoped, IDs: []string{"second"}, Facts: none}},
		{name: "a field outside sections", bp: edit("Steps run in order.", "Steps run in a new order."), facts: sinceFacts, entry: &sinceEntry, want: Scope{Kind: ScopeScoped, Facts: none}},
		{name: "a framing edit", bp: edit("Background that cites", "Revised background that cites"), facts: sinceFacts, entry: &sinceEntry, want: Scope{Kind: ScopeScoped, Facts: []string{"f:frame"}}},
		{name: "a changed fact cited from the dispositions", bp: withDisp, oldBP: withDisp, facts: strings.Replace(sinceFacts, "Other.", "Other, revised.", 1), entry: &sinceEntry, want: Scope{Kind: ScopeScoped, Facts: []string{"f:other"}}},
		{name: "an outline entry change", bp: sinceBase, facts: sinceFacts, entry: &changedEntry, want: Scope{Kind: ScopeScoped, Facts: none}},
		{name: "absent at the earlier revision", bp: sinceBase, facts: sinceFacts, entry: &sinceEntry, oldBP: "-", want: Scope{Kind: ScopeWhole, Why: "absent or undecodable"}},
		{name: "undecodable at the earlier revision", bp: sinceBase, facts: sinceFacts, entry: &sinceEntry, oldBP: "sections: [", want: Scope{Kind: ScopeWhole, Why: "absent or undecodable"}},
		{name: "undecodable now", bp: "sections: [", facts: sinceFacts, entry: &sinceEntry, want: Scope{Kind: ScopeWhole, Why: "does not decode"}},
		{name: "the page missing from the outline at the earlier revision", bp: sinceBase, facts: sinceFacts, entry: &sinceEntry, noEntry: true, want: Scope{Kind: ScopeWhole, Why: "does not list"}},
		{name: "a fact bullet change marks each section that cites it", bp: sinceBase, facts: strings.Replace(sinceFacts, "One.", "One, revised.", 1), entry: &sinceEntry, want: Scope{Kind: ScopeScoped, IDs: []string{"first"}, Facts: []string{"f:one"}}},
		{name: "a changed fact in a facts list", bp: sinceBase, facts: strings.Replace(sinceFacts, "Three.", "Three, revised.", 1), entry: &sinceEntry, want: Scope{Kind: ScopeScoped, IDs: []string{"third"}, Facts: []string{"f:three"}}},
		{name: "a changed fact cited from the framing", bp: sinceBase, facts: strings.Replace(sinceFacts, "The framing fact.", "A new framing fact.", 1), entry: &sinceEntry, want: Scope{Kind: ScopeScoped, Facts: []string{"f:frame"}}},
		{name: "a changed fact cited from the introduction", bp: sinceBase, facts: strings.Replace(sinceFacts, "The intro fact.", "A new intro fact.", 1), entry: &sinceEntry, want: Scope{Kind: ScopeScoped, Facts: []string{"f:intro"}}},
		{name: "a changed fact the page never cites", bp: sinceBase, facts: strings.Replace(sinceFacts, "Other.", "Other, revised.", 1), entry: &sinceEntry, want: Scope{Kind: ScopeUnchanged}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldBP := tt.oldBP
			switch oldBP {
			case "":
				oldBP = sinceBase
			case "-":
				oldBP = ""
			}
			oldEntry := &sinceEntry
			if tt.noEntry {
				oldEntry = nil
			}
			got := Compare("a.blueprint.yaml", sinceRev(oldBP, sinceFacts, oldEntry), sinceRev(tt.bp, tt.facts, tt.entry))
			if got.Kind != tt.want.Kind || !slices.Equal(got.IDs, tt.want.IDs) {
				t.Errorf("Compare = %+v, want kind %s ids %v", got, tt.want.Kind, tt.want.IDs)
			}
			if !strings.Contains(got.Why, tt.want.Why) {
				t.Errorf("Why = %q, want it to mention %q", got.Why, tt.want.Why)
			}
			if tt.want.Kind != ScopeScoped {
				return
			}
			if got.Facts == nil || !slices.Equal(got.Facts, tt.want.Facts) {
				t.Errorf("Facts = %#v, want %#v (non-nil)", got.Facts, tt.want.Facts)
			}
		})
	}
}

// TestCompareBulletsAndJoined covers the two lists the consistency rule reads: the facts whose
// bullet changed anywhere in the store, and the facts the page cites now that it did not cite before.
func TestCompareBulletsAndJoined(t *testing.T) {
	edit := func(old, new string) string { return strings.Replace(sinceBase, old, new, 1) }
	const itemEdit = "Do the first thing, carefully."
	withItem := func(bp string) string { return strings.Replace(bp, "Do the first thing.", itemEdit, 1) }
	revised := func(id, from, to string) string {
		return strings.Replace(sinceFacts, "`"+id+"` "+from, "`"+id+"` "+to, 1)
	}
	tests := []struct {
		name        string
		bp          string
		facts       string
		wantBullets []string
		wantJoined  []string
	}{
		{name: "an item edit citing nothing new", bp: withItem(sinceBase), facts: sinceFacts},
		{name: "a fact added to a sentence", bp: edit("facts: [f:three]", "facts: [f:three, f:other]"), facts: sinceFacts, wantJoined: []string{"f:other"}},
		{name: "a fact placed in a new item", bp: edit("      - id: first-step\n", "      - id: first-other\n        kind: fact\n        fact: f:other\n      - id: first-step\n"), facts: sinceFacts, wantJoined: []string{"f:other"}},
		{name: "a new section placing a new fact", bp: sinceBase + "  - id: fourth\n    heading: Fourth\n    why: |\n      More.\n    opens: |\n      Also.\n    items:\n      - id: fourth-fact\n        kind: fact\n        fact: f:other\n", facts: sinceFacts, wantJoined: []string{"f:other"}},
		{name: "a fact the page already cites, cited again", bp: edit("facts: [f:three]", "facts: [f:three, f:one]"), facts: sinceFacts},
		{name: "a fact the framing cites, cited again in an item", bp: edit("facts: [f:three]", "facts: [f:three, f:frame]"), facts: sinceFacts},
		{name: "a fact removed from a sentence", bp: edit("        facts: [f:three]\n", ""), facts: sinceFacts},
		{name: "a section removed", bp: sinceBase[:strings.Index(sinceBase, "  - id: third")], facts: sinceFacts},
		{name: "a framing edit citing a new fact", bp: edit("Background that cites `f:frame`.", "Background that cites `f:frame` and `f:other`."), facts: sinceFacts, wantJoined: []string{"f:other"}},
		{name: "a cited bullet edited", bp: withItem(sinceBase), facts: revised("f:one", "One.", "One, revised."), wantBullets: []string{"f:one"}},
		{name: "a bullet's tag changed", bp: withItem(sinceBase), facts: revised("f:two", "Two. [verified]", "Two. [external]"), wantBullets: []string{"f:two"}},
		{name: "an uncited bullet edited", bp: withItem(sinceBase), facts: revised("f:other", "Other.", "Other, revised."), wantBullets: []string{"f:other"}},
		{name: "a cited bullet removed from the store", bp: withItem(sinceBase), facts: strings.Replace(sinceFacts, "- `f:one` One. [verified]\n", "", 1), wantBullets: []string{"f:one"}},
		{name: "a bullet edit and a new citation", bp: edit("facts: [f:three]", "facts: [f:three, f:other]"), facts: revised("f:one", "One.", "One, revised."), wantBullets: []string{"f:one"}, wantJoined: []string{"f:other"}},
		{name: "a bullet edited and the fact newly cited", bp: edit("facts: [f:three]", "facts: [f:three, f:other]"), facts: revised("f:other", "Other.", "Other, revised."), wantBullets: []string{"f:other"}, wantJoined: []string{"f:other"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Compare("a.blueprint.yaml", sinceRev(sinceBase, sinceFacts, &sinceEntry), sinceRev(tt.bp, tt.facts, &sinceEntry))
			if got.Kind != ScopeScoped {
				t.Fatalf("kind = %s, want scoped", got.Kind)
			}
			if got.Bullets == nil || !slices.Equal(got.Bullets, append([]string{}, tt.wantBullets...)) {
				t.Errorf("Bullets = %#v, want %#v (non-nil)", got.Bullets, tt.wantBullets)
			}
			if got.Joined == nil || !slices.Equal(got.Joined, append([]string{}, tt.wantJoined...)) {
				t.Errorf("Joined = %#v, want %#v (non-nil)", got.Joined, tt.wantJoined)
			}
		})
	}
}

// TestCompareWithoutAFactsStore checks that a project with no store still gets non-nil lists.
func TestCompareWithoutAFactsStore(t *testing.T) {
	old := Revision{Blueprint: []byte(sinceBase), Entry: &sinceEntry}
	cur := Revision{Blueprint: []byte(strings.Replace(sinceBase, "Do the first thing.", "Do it.", 1)), Entry: &sinceEntry}
	got := Compare("a.blueprint.yaml", old, cur)
	if got.Kind != ScopeScoped || got.Bullets == nil || got.Joined == nil {
		t.Errorf("Compare = %+v, want scoped with non-nil Bullets and Joined", got)
	}
}

// TestScopeKeepsEmptyListsAcrossJSON checks that an empty Bullets and Joined survive state.json as
// empty, not nil, since nil marks a scope recorded before the fields existed.
func TestScopeKeepsEmptyListsAcrossJSON(t *testing.T) {
	in := Scope{Kind: ScopeScoped, Facts: []string{}, Bullets: []string{}, Joined: []string{}}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out Scope
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out.Facts == nil || out.Bullets == nil || out.Joined == nil {
		t.Errorf("round trip of %s gave %+v, want every list non-nil", data, out)
	}
	var before Scope
	if err := json.Unmarshal([]byte(`{"kind":"scoped","facts":[]}`), &before); err != nil {
		t.Fatal(err)
	}
	if before.Bullets != nil || before.Joined != nil {
		t.Errorf("a state from before F2 decoded to %+v, want nil lists", before)
	}
}
