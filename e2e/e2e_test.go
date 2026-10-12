package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var binary string

func TestMain(m *testing.M) {
	os.Exit(buildAndRun(m))
}

func buildAndRun(m *testing.M) int {
	dir, err := os.MkdirTemp("", "drafter-e2e")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() { _ = os.RemoveAll(dir) }()
	binary = filepath.Join(dir, "drafter")
	if out, err := exec.Command("go", "build", "-o", binary, "../cmd/drafter").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build: %v\n%s", err, out)
		return 1
	}
	return m.Run()
}

type result struct {
	stdout, stderr string
	code           int
}

// run executes the drafter binary in dir with args.
func run(t *testing.T, dir string, args ...string) result {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CEILING_DIRECTORIES="+filepath.Dir(dir))
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
		return result{stdout.String(), stderr.String(), 0}
	case errors.As(err, &ee):
		return result{stdout.String(), stderr.String(), ee.ExitCode()}
	default:
		t.Fatalf("run %v: %v", args, err)
		return result{}
	}
}

func project(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("testdata/project")
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

const (
	gapsPlan  = "plans/rotate-the-api-key.gaps.plan.yaml"
	cleanPlan = "plans/rotate-the-api-key.clean.plan.yaml"
)

var projectFlags = []string{"--facts", "docs/facts", "--outline", "docs/outline.json", "--root", "."}

func checkPlan(t *testing.T, dir string, plan string, extra ...string) result {
	t.Helper()
	return run(t, dir, append(append([]string{"check-plan"}, extra...), plan)...)
}

func lines(s string) []string {
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

func TestCheckPlanHelp(t *testing.T) {
	r := run(t, t.TempDir(), "check-plan", "--help")
	if r.code != 0 || !strings.Contains(r.stdout, "--facts") {
		t.Errorf("help: exit %d, stdout %q", r.code, r.stdout)
	}
}

// The gaps plan carries the two recovery gaps a final reader found in a real task guide: a
// failed publish while the health check passes, and telling the old key from the new.
func TestCheckPlanGaps(t *testing.T) {
	r := checkPlan(t, project(t), gapsPlan, projectFlags...)
	if r.code != 1 {
		t.Fatalf("exit = %d, want 1\n%s%s", r.code, r.stdout, r.stderr)
	}
	got := lines(r.stdout)
	want := []string{
		gapsPlan + ":32: warning: links: ",
		gapsPlan + ":49: error: recovery: step \"s-publish\" can fail",
		gapsPlan + ":56: error: recovery: step \"s-verify\" can fail",
	}
	if len(got) != len(want) {
		t.Fatalf("stdout has %d lines, want %d:\n%s", len(got), len(want), r.stdout)
	}
	for i := range want {
		if !strings.HasPrefix(got[i], want[i]) {
			t.Errorf("line %d = %q, want prefix %q", i, got[i], want[i])
		}
	}
	if r.stderr != "2 errors, 1 warning\n" {
		t.Errorf("stderr = %q", r.stderr)
	}
}

func TestCheckPlanClean(t *testing.T) {
	r := checkPlan(t, project(t), cleanPlan, projectFlags...)
	got := lines(r.stdout)
	if r.code != 0 || len(got) != 1 || !strings.HasPrefix(got[0], cleanPlan+":32: warning: links: ") {
		t.Errorf("exit %d, stdout %q", r.code, r.stdout)
	}
	if r.stderr != "0 errors, 1 warning\n" {
		t.Errorf("stderr = %q", r.stderr)
	}
}

// The troubleshooting plan uses the optional fields a page of symptoms needs: a level-3
// section, a failure with no step, code and figure items, and a sentence that cites facts.
func TestCheckPlanSymptomPage(t *testing.T) {
	const plan = "plans/key-troubleshooting.plan.yaml"
	r := checkPlan(t, project(t), plan, projectFlags...)
	if r.code != 0 || r.stdout != "" || r.stderr != "0 errors, 0 warnings\n" {
		t.Errorf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
	r = checkPlan(t, project(t), plan, "--root", ".")
	if r.code != 0 || len(lines(r.stdout)) != 2 || !strings.Contains(r.stdout, ":58: warning: facts: the facts list") {
		t.Errorf("without --facts: exit %d, stdout %q", r.code, r.stdout)
	}
}

func TestCheckPlanRootFromGit(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "project")
	if err := os.CopyFS(dir, os.DirFS(project(t))); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	want := checkPlan(t, project(t), cleanPlan, projectFlags...)
	got := checkPlan(t, dir, cleanPlan, "--facts", "docs/facts", "--outline", "docs/outline.json")
	if got != want {
		t.Errorf("without --root: %+v\nwith --root: %+v", got, want)
	}
}

func TestCheckPlanJSON(t *testing.T) {
	r := checkPlan(t, project(t), gapsPlan, append([]string{"--json"}, projectFlags...)...)
	if r.code != 1 {
		t.Fatalf("exit = %d, want 1", r.code)
	}
	var got []map[string]any
	if err := json.Unmarshal([]byte(r.stdout), &got); err != nil {
		t.Fatalf("stdout is not a JSON array: %v\n%s", err, r.stdout)
	}
	wantSeverity := []string{"warning", "error", "error"}
	if len(got) != len(wantSeverity) {
		t.Fatalf("%d objects, want %d", len(got), len(wantSeverity))
	}
	for i, o := range got {
		if o["severity"] != wantSeverity[i] || o["file"] != gapsPlan {
			t.Errorf("object %d = %v", i, o)
		}
		for _, key := range []string{"file", "line", "severity", "check", "message", "fix"} {
			if _, ok := o[key]; !ok {
				t.Errorf("object %d lacks %q", i, key)
			}
		}
	}
}

func TestCheckPlanWithoutFacts(t *testing.T) {
	r := checkPlan(t, project(t), cleanPlan, "--outline", "docs/outline.json", "--root", ".")
	if r.code != 0 {
		t.Fatalf("exit = %d, want 0", r.code)
	}
	got := lines(r.stdout)
	if len(got) != 2 || !strings.Contains(got[0], ":26: warning: facts: ") || !strings.Contains(got[1], ":32: warning: links: ") {
		t.Errorf("stdout = %q, want a facts warning at line 26 and the outline warning", r.stdout)
	}
}

func TestCheckPlanSeveralPlans(t *testing.T) {
	r := run(t, project(t), append([]string{"check-plan"}, append(projectFlags, cleanPlan, gapsPlan)...)...)
	if r.code != 1 || r.stderr != "2 errors, 2 warnings\n" {
		t.Errorf("exit %d, stderr %q", r.code, r.stderr)
	}
}

func TestCheckPlanDecodeFailures(t *testing.T) {
	valid, err := os.ReadFile(filepath.Join(project(t), cleanPlan))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, src, contains string
		line                int
	}{
		{"empty file", "", "the file holds no YAML document", 1},
		{"malformed YAML", "version: 1\npage: [\n", "malformed YAML", 2},
		{"second document", string(valid) + "---\nversion: 1\n", "the file holds a second YAML document", strings.Count(string(valid), "\n") + 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "p.plan.yaml"), []byte(tt.src), 0o644); err != nil {
				t.Fatal(err)
			}
			r := run(t, dir, "check-plan", "--root", dir, "p.plan.yaml")
			if want := fmt.Sprintf("p.plan.yaml:%d: error: decode: %s", tt.line, tt.contains); r.code != 1 || !strings.HasPrefix(r.stdout, want) {
				t.Errorf("exit %d, stdout %q", r.code, r.stdout)
			}
		})
	}
}

func TestCheckPlanCannotRun(t *testing.T) {
	root := project(t)
	tmp := t.TempDir()
	notJSON := filepath.Join(tmp, "outline.txt")
	noPages := filepath.Join(tmp, "nopages.json")
	emptyFacts := filepath.Join(tmp, "facts")
	for path, body := range map[string]string{notJSON: "not json", noPages: `{"sections": []}`} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(emptyFacts, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(tmp, "outside")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, cleanPlan))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "p.plan.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name, dir string
		args      []string
		contains  string
	}{
		{"no plan argument", root, []string{"check-plan"}, "Usage:"},
		{"unknown flag", root, []string{"check-plan", "--bogus", cleanPlan}, "Usage:"},
		{"plan file missing", root, []string{"check-plan", "--root", ".", "plans/missing.plan.yaml"}, "read plan"},
		{"facts directory missing", root, []string{"check-plan", "--root", ".", "--facts", "docs/nope", cleanPlan}, "docs/nope"},
		{"facts directory with no facts file", root, []string{"check-plan", "--root", ".", "--facts", emptyFacts, cleanPlan}, "facts"},
		{"outline missing", root, []string{"check-plan", "--root", ".", "--outline", "docs/nope.json", cleanPlan}, "read outline"},
		{"outline not JSON", root, []string{"check-plan", "--root", ".", "--outline", notJSON, cleanPlan}, "outline"},
		{"outline without pages", root, []string{"check-plan", "--root", ".", "--outline", noPages, cleanPlan}, "no pages array"},
		{"no root and outside a git tree", outside, []string{"check-plan", "p.plan.yaml"}, "not inside a git work tree"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := run(t, tt.dir, tt.args...)
			if r.code != 2 || !strings.Contains(r.stderr, tt.contains) || r.stdout != "" {
				t.Errorf("exit %d, stdout %q, stderr %q; want exit 2 and stderr containing %q", r.code, r.stdout, r.stderr, tt.contains)
			}
		})
	}
}
