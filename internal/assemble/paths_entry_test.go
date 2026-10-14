package assemble

import (
	"path/filepath"
	"strings"
	"testing"
)

// Rows whose inline_files entry is not the plain allowed directory: an entry above the repo, and
// an entry spelled through a symlink.
func TestInlineFilesEntrySpelling(t *testing.T) {
	f := newFixture(t)
	ex := filepath.Join(f.outside, "exemplars")
	f.link(t, f.root, filepath.Join(f.outside, "repolink"))
	f.link(t, ex, filepath.Join(f.outside, "exlink"))
	tests := []struct {
		name   string
		entry  string
		prompt string
		marker string
		want   bool
	}{
		{"untracked repo file reached by a link under an entry above the repo", filepath.Dir(f.root), filepath.Join(f.outside, "repolink", "untracked.txt"), "UNTRACKED-CONTENT", false},
		{"entry spelled through a symlink admits the real file", filepath.Join(f.outside, "exlink"), ex + "/ok.txt", "ALLOWED-OK", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := requestText(t, f.input(f.config(tt.entry), "Read "+tt.prompt+" now."))
			if has := strings.Contains(got, tt.marker); has != tt.want {
				t.Errorf("request holds %q = %v, want %v", tt.marker, has, tt.want)
			}
		})
	}
}
