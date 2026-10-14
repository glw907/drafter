package plan

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNotGitTreeLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{"parent directories form", "fatal: not a git repository (or any of the parent directories): .git", true},
		{"mount point form", "fatal: not a git repository (or any parent up to mount point /)", true},
		{"mount point form on another mount", "fatal: not a git repository (or any parent up to mount point /mnt/data)", true},
		{"inside the git directory", "fatal: this operation must be run in a work tree", false},
		{"missing directory", "fatal: cannot change to '/no/such/dir': No such file or directory", false},
		{"moved repository", "fatal: gitfile does not point to a valid repository: /tmp/wt/.git", false},
		{"bare repository paraphrase", "fatal: not a git repository: '/tmp/x'", false},
		{"no fatal prefix", "not a git repository (or any parent up to mount point /)", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := notGitTreeLine(tt.line); got != tt.want {
				t.Errorf("notGitTreeLine(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

// gitSandbox clears the locale variables git reads and sets a ceiling so a temp directory never
// finds the repository the tests run inside.
func gitSandbox(t *testing.T, ceiling string) {
	t.Helper()
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANGUAGE"} {
		t.Setenv(name, "")
		_ = os.Unsetenv(name)
	}
	t.Setenv("GIT_CEILING_DIRECTORIES", ceiling)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

func initRepo(t *testing.T, dir string) {
	t.Helper()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
}

func TestGitRootNotGitTree(t *testing.T) {
	dir := t.TempDir()
	gitSandbox(t, filepath.Dir(dir))
	_, err := GitRoot(dir)
	if !errors.Is(err, ErrNotGitTree) {
		t.Fatalf("err = %v, want one wrapping ErrNotGitTree", err)
	}
	if want := filepath.Clean(dir) + " is not inside a git work tree; pass --root"; err.Error() != want {
		t.Errorf("message %q, want %q", err, want)
	}
}

// The German row is vacuous on a host with no German message catalog: git prints English either
// way, and the row passes with or without LC_ALL=C.
func TestGitRootNotGitTreeInGerman(t *testing.T) {
	dir := t.TempDir()
	gitSandbox(t, filepath.Dir(dir))
	t.Setenv("LANG", "de_DE.UTF-8")
	t.Setenv("LANGUAGE", "de")
	_, err := GitRoot(dir)
	if !errors.Is(err, ErrNotGitTree) {
		t.Fatalf("err = %v, want one wrapping ErrNotGitTree", err)
	}
}

func TestGitRootInsideRepo(t *testing.T) {
	dir := t.TempDir()
	gitSandbox(t, filepath.Dir(dir))
	initRepo(t, dir)
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := GitRoot(dir)
	if err != nil {
		t.Fatalf("GitRoot: %v", err)
	}
	if got != want {
		t.Errorf("GitRoot = %q, want %q", got, want)
	}
}

func TestGitRootInsideGitDir(t *testing.T) {
	dir := t.TempDir()
	gitSandbox(t, filepath.Dir(dir))
	initRepo(t, dir)
	_, err := GitRoot(filepath.Join(dir, ".git"))
	if err == nil {
		t.Fatal("GitRoot succeeded inside .git")
	}
	if errors.Is(err, ErrNotGitTree) {
		t.Errorf("err = %v wraps ErrNotGitTree; git named a different failure", err)
	}
	if !strings.Contains(err.Error(), "this operation must be run in a work tree") {
		t.Errorf("err = %q, want git's own message", err)
	}
}

func TestGitRootMissingDir(t *testing.T) {
	dir := t.TempDir()
	gitSandbox(t, filepath.Dir(dir))
	_, err := GitRoot(filepath.Join(dir, "nonexistent"))
	if err == nil || errors.Is(err, ErrNotGitTree) {
		t.Fatalf("err = %v, want a failure that is not ErrNotGitTree", err)
	}
	if !strings.Contains(err.Error(), "cannot change to") {
		t.Errorf("err = %q, want git's own message", err)
	}
}

func TestGitRootGitMissing(t *testing.T) {
	dir := t.TempDir()
	gitSandbox(t, filepath.Dir(dir))
	t.Setenv("PATH", t.TempDir())
	_, err := GitRoot(dir)
	if err == nil || errors.Is(err, ErrNotGitTree) {
		t.Fatalf("err = %v, want a failure that is not ErrNotGitTree", err)
	}
	if !strings.Contains(err.Error(), "git could not run") {
		t.Errorf("err = %q, want it to say git could not run", err)
	}
}
