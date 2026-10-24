package blueprint

import (
	"slices"
	"strings"
	"testing"
)

func TestRefHitsPageWithoutBlueprint(t *testing.T) {
	p := catalogProject(t, map[string]string{guideBlueprint: "", "blueprints/docs/guide.brief.json": refsBrief})
	hits, _, err := p.RefHits("f:bbbbbb")
	if err != nil || len(hits) != 1 || hits[0].Kind != "brief" || hits[0].Page != "docs/guide.md" {
		t.Errorf("RefHits = %+v, %v; want the one brief hit on the page with no blueprint", hits, err)
	}
}

const pageRefsMD = `---
page: Rotate the widget key
type: task guide
track: operate
---

# Rotate the widget key

:::reader
**Who reads it.** An operator who rotates keys. :fact[aaaaaa]
:::

## Before you begin

*Gets the reader ready to rotate.*

- See [the key reference](keys.md#lengths).
- See [the vendor site](https://example.com/keys.md).
- See [another page](other.md).
`

func TestPageRefs(t *testing.T) {
	outline := strings.Replace(readFixture(t, "outline.yaml"), "pages:\n",
		"pages:\n  - path: docs/keys.md\n    slug: keys\n    managed: false\n  - path: docs/other.md\n    managed: false\n", 1) +
		tracksBlock + "crossLinks:\n  - from: guide\n    to: keys\n    why: The guide sends readers to the reference.\n"
	p := mdProject(t, pageRefsMD, map[string]string{"blueprints/outline.yaml": outline})

	for _, ref := range []string{"keys", "docs/keys.md"} {
		refs, listed, err := p.PageRefs(ref)
		if err != nil || !listed {
			t.Fatalf("PageRefs(%s) = %v, listed %v, %v", ref, refs, listed, err)
		}
		want := []Ref{
			{guideMD, lineOf(t, pageRefsMD, "keys.md#lengths"), "before-you-begin.p1", "link"},
			{"blueprints/outline.yaml", lineOf(t, outline, "- from: guide"), "crossLinks", "crossLink"},
		}
		if !slices.Equal(refs, want) {
			t.Errorf("PageRefs(%s) = %+v, want %+v", ref, refs, want)
		}
	}
	if refs, listed, err := p.PageRefs("docs/guide.md"); err != nil || !listed || len(refs) != 0 {
		t.Errorf("PageRefs(guide) = %v, listed %v, %v; want none, listed", refs, listed, err)
	}
	if _, listed, err := p.PageRefs("nowhere"); err != nil || listed {
		t.Errorf("PageRefs(nowhere) listed %v, %v; want unlisted", listed, err)
	}
}

func TestSourceKinds(t *testing.T) {
	more := "# More\n\n" +
		"- `f:ffffff` Both. Source: `src/a.go`, https://example.com/x, read 2026-10-01. [verified]\n" +
		"- `f:gggggg` A doc. Source: `docs/notes.md:1-3`. [verified]\n" +
		"- `f:hhhhhh` A configured prose file. Source: `docs/notes.txt`. [verified]\n" +
		"- `f:iiiiii` A claim naming a URL. https://example.com/claim [verified]\n"
	p := catalogProject(t, map[string]string{"blueprints/facts/more.md": more})
	p.ProseSources = []string{"./docs/notes.txt"}
	tests := []struct {
		id   string
		want []string
	}{
		{"f:bbbbbb", []string{"code"}},
		{"bbbbbb", []string{"code"}},
		{"f:aaaaaa", []string{"url"}},
		{"f:ffffff", []string{"code", "url"}},
		{"f:gggggg", []string{"prose"}},
		{"f:hhhhhh", []string{"prose"}},
		{"f:iiiiii", nil},
		{"f:nothing", nil},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got, err := p.SourceKinds(tt.id)
			if err != nil || !slices.Equal(got, tt.want) {
				t.Errorf("SourceKinds = %v, %v; want %v", got, err, tt.want)
			}
		})
	}
}
