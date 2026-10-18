package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/plan"
	"github.com/spf13/cobra"
)

// initCmdRoot builds a cobra root that holds init and check, since root.go registers init later.
func initCmdRoot() *cobra.Command {
	root := &cobra.Command{Use: "drafter", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(newInitCmd(), newCheckCmd())
	return root
}

// runInitCmd executes the command line and returns stdout, stderr, and the exit status.
func runInitCmd(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	root := initCmdRoot()
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs(args)
	err := root.Execute()
	if err == nil {
		return out.String(), errb.String(), 0
	}
	ee, ok := errors.AsType[exitError](err)
	if !ok {
		t.Fatalf("error %v is not an exitError", err)
	}
	return out.String(), errb.String() + ee.msg + "\n", ee.code
}

func initGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// initRepo returns an empty git work tree with HOME and XDG_CONFIG_HOME set to temp directories.
func initRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	initGit(t, dir, "init", "-q")
	return dir
}

// snapshot maps every path under dir, other than .git, to its content ("" for a directory).
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if rel == ".git" {
			return filepath.SkipDir
		}
		if d.IsDir() {
			snap[rel+"/"] = ""
			return nil
		}
		data, err := os.ReadFile(p)
		snap[rel] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

var seedID = regexp.MustCompile("- `(f:[0-9a-z]{6})` ")

func seedIDOf(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "blueprints/facts/project.md"))
	if err != nil {
		t.Fatal(err)
	}
	m := seedID.FindStringSubmatch(string(data))
	if m == nil {
		t.Fatalf("no seed id in %q", data)
	}
	return m[1]
}

func TestInitThenCheck(t *testing.T) {
	dir := initRepo(t)
	stdout, stderr, code := runInitCmd(t, "init", "--root", dir)
	if code != 0 {
		t.Fatalf("init exit %d, stderr %q", code, stderr)
	}
	wantPaths := []string{
		"drafter.toml",
		"blueprints/outline.yaml",
		"blueprints/facts/README.md",
		"blueprints/facts/project.md",
		".vale.ini",
		".vale/styles/.gitignore",
	}
	if got := strings.Fields(stdout); !slices.Equal(got, wantPaths) {
		t.Errorf("stdout paths = %v, want %v", got, wantPaths)
	}
	if !strings.HasSuffix(stderr, "next: run vale sync, then drafter check\n") {
		t.Errorf("stderr = %q", stderr)
	}

	stdout, stderr, code = runInitCmd(t, "check", "--root", dir)
	if code != 0 {
		t.Fatalf("check exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if stdout != "" {
		t.Errorf("check findings = %q, want none", stdout)
	}
	if !strings.Contains(stderr, "facts store: 1 pointers checked") || !strings.Contains(stderr, "0 errors, 0 warnings") {
		t.Errorf("check stderr = %q", stderr)
	}
}

func TestInitMintsDifferentIDs(t *testing.T) {
	seen := map[string]bool{}
	for range 2 {
		dir := initRepo(t)
		if _, stderr, code := runInitCmd(t, "init", "--root", dir); code != 0 {
			t.Fatalf("init exit %d: %s", code, stderr)
		}
		seen[seedIDOf(t, dir)] = true
	}
	if len(seen) != 2 {
		t.Errorf("two runs minted the same id: %v", seen)
	}
}

func TestInitRefusals(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T) string
		want  string
	}{
		{"outside a git work tree", func(t *testing.T) string {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			return t.TempDir()
		}, "git work tree"},
		{"drafter.toml exists", func(t *testing.T) string {
			dir := initRepo(t)
			if err := os.WriteFile(filepath.Join(dir, "drafter.toml"), []byte("# mine\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			return dir
		}, "drafter.toml already exists"},
		{"blueprints exists", func(t *testing.T) string {
			dir := initRepo(t)
			if err := os.MkdirAll(filepath.Join(dir, "blueprints"), 0o755); err != nil {
				t.Fatal(err)
			}
			return dir
		}, "blueprints already exists"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.setup(t)
			before := snapshot(t, dir)
			stdout, stderr, code := runInitCmd(t, "init", "--root", dir)
			if code != 2 {
				t.Fatalf("exit %d, want 2 (stderr %q)", code, stderr)
			}
			if !strings.Contains(stderr, tt.want) {
				t.Errorf("stderr = %q, want %q", stderr, tt.want)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want none", stdout)
			}
			after := snapshot(t, dir)
			if len(after) != len(before) {
				t.Errorf("tree changed: before %v, after %v", before, after)
			}
			for k, v := range before {
				if after[k] != v {
					t.Errorf("%s changed", k)
				}
			}
		})
	}
}

func TestInitKeepsExistingValeINI(t *testing.T) {
	dir := initRepo(t)
	mine := "StylesPath = elsewhere\n"
	if err := os.WriteFile(filepath.Join(dir, ".vale.ini"), []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := runInitCmd(t, "init", "--root", dir)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".vale.ini"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != mine {
		t.Errorf(".vale.ini = %q, want it unchanged", got)
	}
	if strings.Contains(stdout, ".vale.ini") {
		t.Errorf("stdout lists .vale.ini: %q", stdout)
	}
	if !strings.Contains(stderr, "kept the existing .vale.ini") {
		t.Errorf("stderr = %q", stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, ".vale/styles/.gitignore")); err != nil {
		t.Errorf("styles .gitignore missing: %v", err)
	}
}

func TestInitFactsReadmeAnchorExists(t *testing.T) {
	dir := initRepo(t)
	if _, stderr, code := runInitCmd(t, "init", "--root", dir); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	readme, err := os.ReadFile(filepath.Join(dir, "blueprints/facts/README.md"))
	if err != nil {
		t.Fatal(err)
	}
	const link = "https://github.com/glw907/drafter/blob/main/docs/blueprint-format.md#the-facts-store"
	if !strings.Contains(string(readme), link) {
		t.Fatalf("README lacks %s", link)
	}
	doc, err := os.ReadFile("../../docs/blueprint-format.md")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(plan.Anchors(doc), "the-facts-store") {
		t.Error("docs/blueprint-format.md has no the-facts-store anchor")
	}
}

func TestInitValeFilesLeaveTreeClean(t *testing.T) {
	dir := initRepo(t)
	if _, stderr, code := runInitCmd(t, "init", "--root", dir); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	ini, err := os.ReadFile(filepath.Join(dir, ".vale.ini"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ini), "https://github.com/vale-cli/Google/releases/download/v0.7.1/Google.zip") {
		t.Errorf(".vale.ini = %q, want the v0.7.1 release URL", ini)
	}
	ignore, err := os.ReadFile(filepath.Join(dir, ".vale/styles/.gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(strings.Fields(string(ignore)), "/Google/") {
		t.Errorf("styles .gitignore = %q, want /Google/", ignore)
	}
	initGit(t, dir, "add", ".")
	initGit(t, dir, "commit", "-q", "-m", "init")
	if err := os.MkdirAll(filepath.Join(dir, ".vale/styles/Google"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".vale/styles/Google/Rule.yml"), []byte("extends: existence\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := initGit(t, dir, "status", "--porcelain"); got != "" {
		t.Errorf("git status --porcelain = %q, want empty", got)
	}
}
