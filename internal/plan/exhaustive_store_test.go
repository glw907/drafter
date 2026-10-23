package plan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyFactsExhaustive(t *testing.T) {
	tests := []struct {
		name, bullet string
		want         string
	}{
		{"an exhaustive word in the claim", "- `f:ex0001` Only admins rotate keys. Source: `src/long.go:3`. [candidate]", "only"},
		{"a count in the claim", "- `f:ex0001` The tool has three roles. Source: `src/long.go:3`. [external: Example]", "three roles"},
		{"a verified fact is not warned", "- `f:ex0001` Only admins rotate keys. Source: `src/long.go:3`. [verified]", ""},
		{"a verified fact with a note is not warned", "- `f:ex0001` Only admins rotate keys. Source: `src/long.go:3`. [verified: read by hand]", ""},
		{"a rejected fact is not warned", "- `f:ex0001` Only admins rotate keys. Source: `src/long.go:3`. [rejected: wrong]", ""},
		{"a word in a code span", "- `f:ex0001` Run `rotate --all` to start. Source: `src/long.go:3`. [verified]", ""},
		{"a word only in the source part", "- `f:ex0001` Admins rotate keys. Source: `src/long.go:3`, all of it. [verified]", ""},
		{"a claim with none", "- `f:ex0001` Admins rotate keys. Source: `src/long.go:3`. [verified]", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := copyVerifyFixture(t)
			dir := filepath.Join(root, "facts")
			if err := os.WriteFile(filepath.Join(dir, "store.md"), []byte("## Facts\n\n"+tt.bullet+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			findings, _, _, err := VerifyFacts(dir, root, nil)
			if err != nil {
				t.Fatal(err)
			}
			if tt.want == "" {
				if len(findings) != 0 {
					t.Fatalf("findings = %+v, want none", findings)
				}
				return
			}
			if len(findings) != 1 || findings[0].Check != CheckExhaustive || findings[0].Severity != SeverityWarning ||
				findings[0].Line != 3 || !strings.Contains(findings[0].Message, `"`+tt.want+`"`) {
				t.Errorf("findings = %+v, want one exhaustive warning on %q at line 3", findings, tt.want)
			}
		})
	}
}
