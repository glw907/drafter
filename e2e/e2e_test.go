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
	return runEnv(t, dir, nil, args...)
}

// runEnv is run with extra environment entries. The environment has every DRAFTER_ variable
// stripped and HOME and XDG_CONFIG_HOME set to empty temporary directories, so no setting of the
// machine running the tests reaches the binary; an entry in extra overrides them.
func runEnv(t *testing.T, dir string, extra []string, args ...string) result {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "DRAFTER_") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir(),
		"GIT_CEILING_DIRECTORIES="+filepath.Dir(dir))
	cmd.Env = append(cmd.Env, extra...)
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

// A target counts as planned when any of the outlines lists it.
func TestCheckPlanSeveralOutlines(t *testing.T) {
	other := filepath.Join(t.TempDir(), "other.json")
	if err := os.WriteFile(other, []byte(`{"pages":[{"path":"docs/guides/unrelated.md"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	base := []string{"--facts", "docs/facts", "--root", "."}
	r := checkPlan(t, project(t), cleanPlan, append(base, "--outline", other, "--outline", "docs/outline.json")...)
	got := lines(r.stdout)
	if r.code != 0 || len(got) != 1 || !strings.HasPrefix(got[0], cleanPlan+":32: warning: links: ") {
		t.Errorf("both outlines: exit %d, stdout %q", r.code, r.stdout)
	}
	r = checkPlan(t, project(t), cleanPlan, append(base, "--outline", other)...)
	if r.code != 1 || !strings.Contains(r.stdout, cleanPlan+":32: error: links: ") {
		t.Errorf("only the other outline: exit %d, stdout %q", r.code, r.stdout)
	}
	missing := filepath.Join(t.TempDir(), "absent.json")
	r = checkPlan(t, project(t), cleanPlan, append(base, "--outline", "docs/outline.json", "--outline", missing)...)
	if r.code != 2 || !strings.Contains(r.stderr, missing) {
		t.Errorf("a missing second outline: exit %d, stderr %q", r.code, r.stderr)
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
		{"outline not JSON", root, []string{"check-plan", "--root", ".", "--outline", notJSON, cleanPlan}, notJSON},
		{"outline without pages", root, []string{"check-plan", "--root", ".", "--outline", noPages, cleanPlan}, noPages + " has no pages array"},
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

func TestAnchors(t *testing.T) {
	const page = "docs/reference/anchors.md"
	want := "anchors-page setup setup-1 custom-row"
	r := run(t, project(t), "anchors", page)
	if r.code != 0 || r.stderr != "" || strings.Join(lines(r.stdout), " ") != want {
		t.Errorf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}

	r = run(t, project(t), "anchors", "--json", page)
	var got []string
	if err := json.Unmarshal([]byte(r.stdout), &got); err != nil || r.code != 0 {
		t.Fatalf("exit %d, stdout is not a JSON array: %v\n%s", r.code, err, r.stdout)
	}
	if strings.Join(got, " ") != want {
		t.Errorf("json anchors = %q, want %q", got, want)
	}

	r = run(t, project(t), "anchors", page, "docs/reference/keys.md")
	if r.code != 0 || !strings.HasPrefix(r.stdout, page+"#anchors-page\n") || !strings.Contains(r.stdout, "docs/reference/keys.md#") {
		t.Errorf("several files: exit %d, stdout %q", r.code, r.stdout)
	}
}

func TestAnchorsNone(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plain.md"), []byte("Just text.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := run(t, dir, "anchors", "plain.md"); r.code != 0 || r.stdout != "" {
		t.Errorf("text: exit %d, stdout %q", r.code, r.stdout)
	}
	if r := run(t, dir, "anchors", "--json", "plain.md"); r.code != 0 || r.stdout != "[]\n" {
		t.Errorf("json: exit %d, stdout %q", r.code, r.stdout)
	}
}

func TestAnchorsErrors(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		code     int
		contains string
	}{
		{"missing file", []string{"anchors", "docs/nope.md"}, 1, "docs/nope.md"},
		{"no file argument", []string{"anchors"}, 2, "Usage:"},
		{"unknown flag", []string{"anchors", "--bogus", "docs/reference/keys.md"}, 2, "Usage:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := run(t, project(t), tt.args...)
			if r.code != tt.code || r.stdout != "" || !strings.Contains(r.stderr, tt.contains) {
				t.Errorf("exit %d, stdout %q, stderr %q; want exit %d and stderr containing %q", r.code, r.stdout, r.stderr, tt.code, tt.contains)
			}
		})
	}
}

// cfgProject is a drafter project under test: a git repository copied from testdata/config, with
// a user config under its own XDG_CONFIG_HOME.
type cfgProject struct {
	dir, xdg string
}

func gitEnv() []string {
	return []string{
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
	}
}

func newCfgProject(t *testing.T) cfgProject {
	t.Helper()
	src, err := filepath.Abs("testdata/config")
	if err != nil {
		t.Fatal(err)
	}
	p := cfgProject{dir: filepath.Join(t.TempDir(), "project"), xdg: t.TempDir()}
	if err := os.CopyFS(p.dir, os.DirFS(src)); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", p.dir, "init", "-q")
	cmd.Env = append(os.Environ(), gitEnv()...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return p
}

const completeUser = `workspace_id = "wrkspc_test"
page_cap = 10
cycle_ceiling = 100

[prices.models.model-a]
input = 1
output = 5
`

func writeFile(t *testing.T, name, body string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (p cfgProject) user(t *testing.T, body string) {
	t.Helper()
	dir := filepath.Join(p.xdg, "drafter")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "config.toml"), body)
}

func (p cfgProject) config(t *testing.T, env []string, args ...string) result {
	t.Helper()
	env = append(append(gitEnv(), "HOME="+p.xdg, "XDG_CONFIG_HOME="+p.xdg), env...)
	return runEnv(t, p.dir, env, append([]string{"config"}, args...)...)
}

type cfgRow struct {
	Key     string   `json:"key"`
	Value   string   `json:"value"`
	Source  string   `json:"source"`
	Sources []string `json:"sources"`
}

func (p cfgProject) rows(t *testing.T, env []string, args ...string) map[string]cfgRow {
	t.Helper()
	r := p.config(t, env, append([]string{"--json"}, args...)...)
	if r.code != 0 {
		t.Fatalf("config --json: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	var list []cfgRow
	if err := json.Unmarshal([]byte(r.stdout), &list); err != nil {
		t.Fatalf("stdout is not a JSON array: %v\n%s", err, r.stdout)
	}
	m := map[string]cfgRow{}
	for _, row := range list {
		m[row.Key] = row
	}
	return m
}

func TestConfigModeAtEveryLayer(t *testing.T) {
	p := newCfgProject(t)
	proj := filepath.Join(p.dir, "drafter.toml")
	base, err := os.ReadFile(proj)
	if err != nil {
		t.Fatal(err)
	}
	p.user(t, completeUser)

	check := func(name, value, source string, env []string, args ...string) {
		t.Helper()
		got := p.rows(t, env, args...)["mode"]
		if got.Value != value || got.Source != source {
			t.Errorf("%s: mode = %q from %q, want %q from %q", name, got.Value, got.Source, value, source)
		}
	}
	check("default", "batch", "default", nil)
	p.user(t, "mode = \"sync\"\n"+completeUser)
	check("user", "sync", "user", nil)
	writeFile(t, proj, "mode = \"batch\"\n"+string(base))
	check("drafter.toml", "batch", "drafter.toml", nil)
	writeFile(t, filepath.Join(p.dir, "drafter.local.toml"), "mode = \"sync\"\n")
	check("drafter.local.toml", "sync", "drafter.local.toml", nil)
	check("environment", "batch", "env DRAFTER_MODE", []string{"DRAFTER_MODE=batch"})
	check("flag", "sync", "flag --sync", []string{"DRAFTER_MODE=batch"}, "--sync")
}

func TestConfigCapAtTwoLayers(t *testing.T) {
	p := newCfgProject(t)
	p.user(t, completeUser)
	writeFile(t, filepath.Join(p.dir, "drafter.local.toml"), "page_cap = 5\n")

	r := p.config(t, nil)
	if r.code != 0 {
		t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	var line string
	for _, l := range lines(r.stdout) {
		if strings.HasPrefix(l, "page_cap = ") {
			line = l
		}
	}
	for _, want := range []string{"page_cap = 5  (drafter.local.toml", "user: 10", "drafter.local.toml: 5"} {
		if !strings.Contains(line, want) {
			t.Errorf("page_cap line %q lacks %q", line, want)
		}
	}
	got := p.rows(t, nil)["page_cap"]
	if got.Value != "5" || got.Source != "drafter.local.toml" || strings.Join(got.Sources, "|") != "user: 10|drafter.local.toml: 5" {
		t.Errorf("json page_cap = %+v", got)
	}
}

func TestConfigJSONMatchesText(t *testing.T) {
	p := newCfgProject(t)
	p.user(t, completeUser)
	writeFile(t, filepath.Join(p.dir, "drafter.local.toml"), "page_cap = 5\n")

	text := lines(p.config(t, nil).stdout)
	var list []cfgRow
	if err := json.Unmarshal([]byte(p.config(t, nil, "--json").stdout), &list); err != nil {
		t.Fatal(err)
	}
	if len(text) != len(list) {
		t.Fatalf("text has %d rows, json %d", len(text), len(list))
	}
	for i, row := range list {
		if row.Key == "output" {
			continue // --json is what sets it, so its source differs between the two runs
		}
		if want := row.Key + " = " + row.Value + "  (" + row.Source; !strings.HasPrefix(text[i], want) {
			t.Errorf("row %d: text %q, json wants prefix %q", i, text[i], want)
		}
	}
}

func TestConfigRefusal(t *testing.T) {
	p := newCfgProject(t)
	p.user(t, completeUser)
	proj := filepath.Join(p.dir, "drafter.toml")
	base, err := os.ReadFile(proj)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, proj, string(base)+"key_command = \"op read x\"\n")
	line := strings.Count(string(base), "\n") + 1

	r := p.config(t, nil)
	if r.code != 1 || r.stdout != "" || !strings.Contains(r.stderr, "key_command") ||
		!strings.Contains(r.stderr, fmt.Sprintf("drafter.toml:%d", line)) {
		t.Errorf("exit %d, stdout %q, stderr %q; want exit 1 and a refusal naming key_command at line %d", r.code, r.stdout, r.stderr, line)
	}
}

func TestConfigMissingSetting(t *testing.T) {
	p := newCfgProject(t)
	p.user(t, strings.Replace(completeUser, "cycle_ceiling = 100\n", "", 1))

	r := p.config(t, nil)
	if r.code != 1 || !strings.Contains(r.stderr, "cycle_ceiling is not set") {
		t.Errorf("exit %d, stderr %q", r.code, r.stderr)
	}
	if !strings.Contains(r.stdout, "workspace_id = wrkspc_test  (user)") || !strings.Contains(r.stdout, "cycle_ceiling =   (unset)") {
		t.Errorf("the table did not print before the error:\n%s", r.stdout)
	}
}

func TestConfigOutsideGitTree(t *testing.T) {
	xdg := t.TempDir()
	if err := os.MkdirAll(filepath.Join(xdg, "drafter"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(xdg, "drafter", "config.toml"), completeUser)
	r := runEnv(t, t.TempDir(), []string{"XDG_CONFIG_HOME=" + xdg}, "config")
	if r.code != 0 || !strings.Contains(r.stdout, "page_cap = 10  (user;") {
		t.Errorf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
}
