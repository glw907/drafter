package plan

import (
	"slices"
	"strings"
	"testing"
)

func TestDispositionNamesOneTarget(t *testing.T) {
	const cut = "  - fact: legacy-format\n    disposition: cut\n"
	tests := []struct {
		name, repl, want string
	}{
		{"a covers disposition", "  - covers: key-sizes\n    disposition: cut\n", ""},
		{"both a fact and a covers id", "  - fact: legacy-format\n    covers: key-sizes\n    disposition: cut\n", "names both a fact and a covers id"},
		{"neither", "  - disposition: cut\n", "names neither a fact nor a covers id"},
		{"a covers id that is not a token", "  - covers: Key Sizes\n    disposition: cut\n", "is not a lowercase token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, fs := Decode("x.plan.yaml", []byte(mutate(t, cut, tt.repl)))
			if tt.want == "" {
				if len(fs) != 0 || p.Dispositions[1].Covers != "key-sizes" || p.Dispositions[1].Fact != "" {
					t.Errorf("findings = %v, disposition = %+v; want none and a covers disposition", fs, p.Dispositions[1])
				}
				return
			}
			if !slices.ContainsFunc(fs, func(f Finding) bool { return strings.Contains(f.Message, tt.want) }) {
				t.Errorf("findings = %v, want one containing %q", fs, tt.want)
			}
		})
	}
}
