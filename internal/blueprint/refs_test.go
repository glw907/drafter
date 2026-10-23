package blueprint

import (
	"slices"
	"strings"
	"testing"
)

func TestRefs(t *testing.T) {
	edited := withEdit(t, "          Only the role requirement.\n",
		"          Only the role requirement.\n"+
			"      - id: needs-admin-again\n        kind: fact\n        fact: f:bbbbbb\n"+
			"      - id: c-admin\n        kind: sentence\n        mode: claim\n        text: |\n          Rotation needs the role.\n        facts: [f:bbbbbb]\n")
	edited = strings.Replace(edited, "the service accepts any key it holds.", "the service accepts any key it holds. Rotation needs `f:bbbbbb`.", 1)
	p := catalogProject(t, map[string]string{guideBlueprint: edited})

	tests := []struct {
		name string
		id   string
		want []string // item/kind pairs in file-and-line order
		held bool
	}{
		{"placed twice, cited in an item, cited in framing", "f:bbbbbb",
			[]string{"_framing-background example", "needs-admin fact", "needs-admin-again fact", "c-admin sentence"}, true},
		{"introduction, framing, and an item", "f:aaaaaa", []string{"introduction introduction", "_framing-readers example", "c-two-keys sentence"}, true},
		{"disposition", "f:dddddd", []string{"dispositions disposition"}, true},
		{"held but placed nowhere", "f:eeeeee", nil, true},
		{"not in the store", "f:nothing", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs, held, err := p.Refs(tt.id)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, r := range refs {
				if r.File != guideBlueprint || r.Line < 1 {
					t.Errorf("ref at %s:%d", r.File, r.Line)
				}
				got = append(got, r.Item+" "+r.Kind)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("refs %v, want %v", got, tt.want)
			}
			if held != tt.held {
				t.Errorf("held = %v, want %v", held, tt.held)
			}
		})
	}
}

func TestRefsLines(t *testing.T) {
	p := catalogProject(t, nil)
	base := readFixture(t, "base.blueprint.yaml")
	refs, _, err := p.Refs("f:bbbbbb")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].Line != lineOf(t, base, "fact: f:bbbbbb") {
		t.Errorf("refs %+v, want one at the fact line %d", refs, lineOf(t, base, "fact: f:bbbbbb"))
	}
}

const refsBrief = `{
  "sentences": [
    {"text": "Rotating a key needs the admin role.", "id": "f:bbbbbb"},
    {"text": "The check returns the id of the key that signed.", "id": ["f:cccccc", "f:aaaaaa"]},
    {"text": "Here is how the rotation works.", "id": "no-claim"},
    {"text": "A preview lives at /preview/<token> & expires.", "id": "f:eeeeee"}
  ]
}
`

func TestRefHitsSection(t *testing.T) {
	p := catalogProject(t, nil)
	hits, _, err := p.RefHits("f:bbbbbb")
	if err != nil || len(hits) != 1 {
		t.Fatalf("RefHits = %+v, %v", hits, err)
	}
	if hits[0].Item != "needs-admin" || hits[0].Section != "before-you-begin" {
		t.Errorf("hit = %+v, want item needs-admin in section before-you-begin", hits[0])
	}
	hits, _, err = p.RefHits("f:aaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hits {
		if (h.Kind == "introduction" || strings.HasPrefix(h.Item, "_framing-")) && h.Section != "" {
			t.Errorf("hit %+v has a section, want none", h)
		}
	}
}

func TestRefHitsBriefs(t *testing.T) {
	tests := []struct {
		name     string
		template string
		briefAt  string
		id       string
		want     []string // item and sentence of each brief hit
	}{
		{"a string id", "", "blueprints/docs/guide.brief.json", "f:bbbbbb", []string{"s1 Rotating a key needs the admin role."}},
		{"an id without its prefix", "", "blueprints/docs/guide.brief.json", "bbbbbb", []string{"s1 Rotating a key needs the admin role."}},
		{"an array of ids", "", "blueprints/docs/guide.brief.json", "f:aaaaaa", []string{"s2 The check returns the id of the key that signed."}},
		{"a non-default brief path", "notes/{dir}/{name}.json", "notes/docs/guide.json", "f:bbbbbb", []string{"s1 Rotating a key needs the admin role."}},
		{"the default path is ignored under a template", "notes/{dir}/{name}.json", "blueprints/docs/guide.brief.json", "f:bbbbbb", nil},
		{"a sentence with angle brackets", "", "blueprints/docs/guide.brief.json", "f:eeeeee", []string{"s4 A preview lives at /preview/<token> & expires."}},
		{"no citing sentence", "", "blueprints/docs/guide.brief.json", "f:dddddd", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := catalogProject(t, map[string]string{tt.briefAt: refsBrief})
			p.BriefPath = tt.template
			hits, _, err := p.RefHits(tt.id)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, h := range hits {
				if h.Kind != "brief" {
					continue
				}
				if h.Brief != tt.briefAt || h.File != tt.briefAt || h.Page != "docs/guide.md" || h.Line < 2 {
					t.Errorf("brief hit = %+v", h)
				}
				got = append(got, h.Item+" "+h.Sentence)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("brief hits %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRefHitsNoBrief(t *testing.T) {
	p := catalogProject(t, nil)
	hits, held, err := p.RefHits("f:eeeeee")
	if err != nil || len(hits) != 0 || !held {
		t.Errorf("RefHits = %+v, held %v, %v; want none, held, no error", hits, held, err)
	}
}
