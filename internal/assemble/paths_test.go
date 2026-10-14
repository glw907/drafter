package assemble

import (
	"path/filepath"
	"strings"
	"testing"
)

// The inline_files path-safety rows. Each hostile row names a file whose content holds a marker
// and requires the marker to stay out of the request; the twin rows show the same setup admits a
// file the entry really covers.
func TestInlineFilesPathSafety(t *testing.T) {
	f := newFixture(t)
	ex := filepath.Join(f.outside, "exemplars")
	tests := []struct {
		name   string
		prompt string
		marker string
		want   bool
	}{
		{"allowed file is in", "Read " + ex + "/ok.txt now.", "ALLOWED-OK", true},
		{"dotdot climbs out of the allowed directory", "Read " + ex + "/../secret.txt now.", "OUTSIDE-SECRET", false},
		{"symlink in the allowed directory points outside", "Read " + ex + "/link.txt now.", "OUTSIDE-SECRET", false},
		{"sibling directory shares the entry's prefix", "Read " + filepath.Join(f.outside, "exemplars-old", "f.txt") + " now.", "OLD-SECRET", false},
		{"tracked symlink points outside the root", "Read docs/escape.md now.", "OUTSIDE-SECRET", false},
		{"tracked symlink into an allowed directory is in", "Read docs/allowed-link.md now.", "ALLOWED-OK", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := requestText(t, f.input(f.config(ex), tt.prompt))
			if has := strings.Contains(got, tt.marker); has != tt.want {
				t.Errorf("request holds %q = %v, want %v", tt.marker, has, tt.want)
			}
		})
	}
}
