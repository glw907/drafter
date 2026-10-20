package blueprint

import (
	"slices"
	"strings"
	"testing"
)

func TestCompareRawChanged(t *testing.T) {
	revised := func(from, to string) string { return strings.Replace(sinceFacts, from, to, 1) }
	tests := []struct {
		name        string
		facts       string
		want        []string
		wantBullets []string
	}{
		{"a tag qualifier alone", revised("`f:one` One. [verified]", "`f:one` One. [verified: 2026-10-01]"), []string{"f:one"}, nil},
		{"a parenthesized anchor alone", revised("Source: `a/b.go`.", "Source: `a/b.go` (`Run`)."), []string{"f:intro"}, nil},
		{"a cited bullet's text", revised("`f:two` Two.", "`f:two` Two, revised."), []string{"f:two"}, []string{"f:two"}},
		{"a bullet the page never cites", revised("`f:other` Other.", "`f:other` Other, revised."), nil, []string{"f:other"}},
		{"nothing", sinceFacts, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A step edit keeps the page scoped whatever the facts do.
			bp := strings.Replace(sinceBase, "Do the first thing.", "Do it.", 1)
			got := Compare("a.blueprint.yaml", sinceRev(sinceBase, sinceFacts, &sinceEntry), sinceRev(bp, tt.facts, &sinceEntry))
			if got.Kind != ScopeScoped {
				t.Fatalf("kind = %s, want scoped", got.Kind)
			}
			if !slices.Equal(got.RawChanged, tt.want) {
				t.Errorf("RawChanged = %#v, want %#v", got.RawChanged, tt.want)
			}
			if !slices.Equal(got.Bullets, append([]string{}, tt.wantBullets...)) {
				t.Errorf("Bullets = %#v, want %#v: RawChanged must catch what Bullets misses", got.Bullets, tt.wantBullets)
			}
		})
	}
}
