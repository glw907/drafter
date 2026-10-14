package config

import (
	"strings"
	"testing"
)

// localRoot writes a drafter.toml and a drafter.local.toml into a fresh sandbox root.
func localRoot(t *testing.T) sandbox {
	t.Helper()
	sb := newSandbox(t)
	writeFile(t, sb.project(), "mode = \"batch\"\n")
	writeFile(t, sb.local(), "mode = \"sync\"\n")
	return sb
}

func TestLocalFileGitFailures(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T, sb sandbox)
		contains string
	}{
		{"root is not a git tree", func(t *testing.T, sb sandbox) {
			t.Setenv("GIT_CEILING_DIRECTORIES", sb.root+"/..")
		}, "not a git repository"},
		{"git cannot run", func(t *testing.T, sb sandbox) {
			t.Setenv("PATH", t.TempDir())
		}, "git could not run"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sb := localRoot(t)
			tt.setup(t, sb)
			_, err := Load(sb.root, Flags{})
			if err == nil {
				t.Fatal("Load accepted a drafter.local.toml it could not check")
			}
			if !strings.Contains(err.Error(), tt.contains) {
				t.Errorf("error %q, want it to contain %q", err, tt.contains)
			}
			if strings.Contains(err.Error(), "not ignored") {
				t.Errorf("error %q blames the ignore state; git failed instead", err)
			}
		})
	}
}

func TestLocalFileIgnoredLoads(t *testing.T) {
	sb := localRoot(t)
	runGit(t, sb.root, "init", "-q")
	writeFile(t, sb.root+"/.gitignore", "drafter.local.toml\n")
	c, err := Load(sb.root, Flags{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Mode != "sync" {
		t.Errorf("mode = %q, want sync from the local layer", c.Mode)
	}
}
