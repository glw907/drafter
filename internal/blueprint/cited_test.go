package blueprint

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCited(t *testing.T) {
	const rel = "blueprints/docs/guide.blueprint.yaml"
	tests := []struct {
		name  string
		extra map[string]string
		want  []string
	}{
		{"the base blueprint", nil, []string{"f:aaaaaa", "f:bbbbbb", "f:cccccc"}},
		{
			"a fact cited only in a framing part",
			map[string]string{rel: withEdit(t, "Keys are long-lived,", "Keys are long-lived (`f:eeeeee`),")},
			[]string{"f:aaaaaa", "f:bbbbbb", "f:cccccc", "f:eeeeee"},
		},
		{
			"a disposition is not a citation",
			map[string]string{rel: strings.Replace(withEdit(t, "f:cccccc", "f:aaaaaa"), "f:bbbbbb", "f:aaaaaa", 1)},
			[]string{"f:aaaaaa"},
		},
		{"a blueprint that does not decode", map[string]string{rel: "version: 1\nargument: [\n"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := catalogProject(t, tt.extra)
			got, err := p.Cited(rel)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Cited = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCitedErrors(t *testing.T) {
	p := catalogProject(t, nil)
	if _, err := p.Cited("blueprints/docs/none.blueprint.yaml"); err == nil {
		t.Error("a missing blueprint gave no error")
	}
	p.Outline = "blueprints/none.yaml"
	if _, err := p.Cited("blueprints/docs/guide.blueprint.yaml"); err == nil {
		t.Error("a missing outline gave no error")
	}
}

func TestCitedInReadsTheBytesItIsGiven(t *testing.T) {
	const rel = "blueprints/docs/guide.blueprint.yaml"
	p := catalogProject(t, map[string]string{rel: withEdit(t, "Keys are long-lived,", "Keys are long-lived (`f:eeeeee`),")})
	outline, err := os.ReadFile(p.abs(p.Outline))
	if err != nil {
		t.Fatal(err)
	}
	base, err := os.ReadFile(filepath.Join("testdata", "catalog", "base.blueprint.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.CitedIn(rel, base, outline)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"f:aaaaaa", "f:bbbbbb", "f:cccccc"}; !slices.Equal(got, want) {
		t.Errorf("CitedIn over the base bytes = %v, want %v although the working tree cites f:eeeeee", got, want)
	}
}
