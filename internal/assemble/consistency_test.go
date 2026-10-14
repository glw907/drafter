package assemble

import (
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/plan"
)

func TestConsistencyPrompt(t *testing.T) {
	f := newFixture(t)
	p := &plan.Plan{
		Page:         plan.Page{Path: "docs/page.md", Title: "The page"},
		Introduction: plan.Introduction{Facts: []string{"f:other"}},
		Sections: []plan.Section{
			{Heading: "One", Items: []plan.Item{
				{ID: "a", Kind: "fact", Fact: "f:greet"},
				{ID: "b", Kind: "sentence", Mode: "claim", Facts: []string{"f:render", "f:greet"}},
				{ID: "c", Kind: "link", Target: "guide.md#setup"},
				{ID: "d", Kind: "link", Target: "https://example.com/elsewhere"},
				{ID: "e", Kind: "link", Target: "nope.md"},
				{ID: "f", Kind: "link", Target: "#local"},
				{ID: "g", Kind: "link", Target: "guide.md"},
			}},
		},
	}
	got, err := ConsistencyPrompt(p, loadFacts(t, f), f.root)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"- f:other: Two sources of two kinds.",
		"- f:greet: The greeter says hello with an exclamation mark.",
		"- f:render: The render helper trims its input.",
		"- docs/guide.md\n  # Guide\n  # A heading that opens the doc\n  ## Setup\n  ### Install\n",
		"- missing: docs/nope.md",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt lacks %q:\n%s", want, got)
		}
	}
	for _, bad := range []string{"example.com", "not a heading", "local"} {
		if strings.Contains(got, bad) {
			t.Errorf("prompt holds %q:\n%s", bad, got)
		}
	}
	if n := strings.Count(got, "- f:greet"); n != 1 {
		t.Errorf("f:greet listed %d times, want once", n)
	}
	if n := strings.Count(got, "- docs/guide.md"); n != 1 {
		t.Errorf("guide listed %d times, want once", n)
	}
	if strings.Index(got, "- f:other") > strings.Index(got, "- f:greet") {
		t.Errorf("introduction fact should be listed before section facts")
	}
}

func TestConsistencyPromptWithoutFactsStoreListsIds(t *testing.T) {
	f := newFixture(t)
	p := &plan.Plan{Page: plan.Page{Path: "docs/page.md"}, Sections: []plan.Section{
		{Items: []plan.Item{{ID: "a", Kind: "fact", Fact: "f:greet"}, {ID: "l", Kind: "link", Target: "../../escape.md"}}},
	}}
	got, err := ConsistencyPrompt(p, nil, f.root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "- f:greet\n") {
		t.Errorf("prompt = %s, want the bare id", got)
	}
	if !strings.Contains(got, "missing: ../escape.md") {
		t.Errorf("prompt = %s, want a link out of the root read as missing", got)
	}
}
