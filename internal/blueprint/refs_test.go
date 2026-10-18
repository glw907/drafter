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
