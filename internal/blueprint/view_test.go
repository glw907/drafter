package blueprint

import (
	"slices"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/plan"
)

// placed lists the fact ids a plan places or cites, as the consistency prompt gathers them: the
// introduction's list, then each item's fact and facts.
func placed(p *plan.Plan) []string {
	var ids []string
	ids = append(ids, p.Introduction.Facts...)
	for _, s := range p.Sections {
		for _, it := range s.Items {
			if it.Fact != "" {
				ids = append(ids, it.Fact)
			}
			ids = append(ids, it.Facts...)
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}

func TestViewPlacedFacts(t *testing.T) {
	src := readFixture(t, "base.blueprint.yaml")
	src = strings.Replace(src, "  background: |\n    Keys are long-lived, and the service accepts any key it holds.\n",
		"  background: |\n    Keys are long-lived. See `f:eeeeee`, the mistyped `f:zzzzzz`, and the flag `--key`.\n", 1)
	src = strings.Replace(src, "      - id: code-add\n", "      - id: e-staging\n        kind: example\n        text: |\n          A staging rotation first.\n        facts: [f:dddddd]\n      - id: code-add\n", 1)
	b := decodeOK(t, src)
	facts, err := plan.LoadFacts("testdata/catalog/facts")
	if err != nil {
		t.Fatal(err)
	}
	o, fs := DecodeOutline("outline.yaml", []byte(readFixture(t, "outline.yaml")))
	if len(fs) > 0 {
		t.Fatal(fs)
	}

	p := View(b, o.Pages[0], facts)
	want := []string{"f:aaaaaa", "f:bbbbbb", "f:cccccc", "f:dddddd", "f:eeeeee", "f:zzzzzz"}
	if got := placed(p); !slices.Equal(got, want) {
		t.Errorf("placed facts = %v, want %v (fact items, every facts list, the example item, and the framing citations, but not the flag)", got, want)
	}
	if p.Page.Path != "docs/guide.md" || p.Page.Title != "Rotate the widget key" || p.Page.Type != "task guide" || p.Page.Lines["path"] != 1 {
		t.Errorf("page = %+v, want the outline entry's, cited at line 1", p.Page)
	}
	var synthetic []plan.Item
	for _, it := range p.Sections[0].Items {
		if strings.HasPrefix(it.ID, "_framing-") {
			synthetic = append(synthetic, it)
		}
	}
	if len(synthetic) != 2 || synthetic[0].ID != "_framing-readers" || synthetic[1].ID != "_framing-background" {
		t.Fatalf("synthetic items = %+v, want _framing-readers and _framing-background", synthetic)
	}
	if bg := synthetic[1]; bg.Kind != "example" || bg.Line != b.Framing.Lines["background"] || !slices.Equal(bg.Facts, []string{"f:eeeeee", "f:zzzzzz"}) {
		t.Errorf("background item = %+v, want its facts cited at the framing field's line %d", bg, b.Framing.Lines["background"])
	}
	if slices.ContainsFunc(b.Sections[0].Items, func(it Item) bool { return strings.HasPrefix(it.ID, "_framing-") }) {
		t.Error("View wrote its synthetic items into the blueprint")
	}

	findings := plan.Check("x.blueprint.yaml", p, plan.CheckOptions{Root: t.TempDir(), FactsDir: "testdata/catalog/facts"})
	var typo *plan.Finding
	for i := range findings {
		if findings[i].Check == "facts" && strings.Contains(findings[i].Message, "f:zzzzzz") {
			typo = &findings[i]
		}
	}
	if typo == nil || typo.Severity != plan.SeverityError || !strings.Contains(typo.Message, "in no facts file") || typo.Line != b.Framing.Lines["background"] {
		t.Errorf("the mistyped framing id gave %+v, want a facts error at the framing field's line", typo)
	}
}

func TestViewWithoutFacts(t *testing.T) {
	b := decodeOK(t, readFixture(t, "base.blueprint.yaml"))
	p := View(b, Page{Path: "docs/guide.md", Title: "T", Type: "x"}, nil)
	for _, it := range p.Sections[0].Items {
		if strings.HasPrefix(it.ID, "_framing-") {
			t.Errorf("a view with no facts store built %s", it.ID)
		}
	}
	if len(p.Sections) != len(b.Sections) || p.Sections[1].Items[0].ID != "s-generate" || p.Sections[1].Lines["heading"] != b.Sections[1].Lines["heading"] {
		t.Errorf("sections did not carry over with their lines: %+v", p.Sections)
	}
}
