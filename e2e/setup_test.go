package e2e

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupEnv is a clean machine: a HOME that holds only the drafter binary and vale on its PATH, no
// XDG variables, and no inherited environment. It is what a new developer's first run sees.
type setupEnv struct {
	home, repo string
	env        []string
}

func newSetupEnv(t *testing.T) *setupEnv {
	t.Helper()
	vale, err := exec.LookPath("vale")
	if err != nil {
		t.Fatalf("vale is required to run this test: install Vale (%v)", err)
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, target := range map[string]string{"drafter": binary, "vale": vale} {
		if err := os.Symlink(target, filepath.Join(bin, name)); err != nil {
			t.Fatal(err)
		}
	}
	repo := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	return &setupEnv{home: home, repo: repo, env: []string{
		"HOME=" + home,
		"PATH=" + bin + ":" + filepath.Dir(git) + ":" + filepath.Dir(sh),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
	}}
}

// do runs name with args in the project and returns its output and exit status.
func (e *setupEnv) do(t *testing.T, name string, args ...string) result {
	t.Helper()
	if name == "drafter" {
		// exec.Command looks a bare name up on the test's PATH, not on the one the command gets.
		name = filepath.Join(e.home, "bin", "drafter")
	}
	cmd := exec.Command(name, args...)
	cmd.Dir = e.repo
	cmd.Env = e.env
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
		t.Fatalf("%s %v: %v", name, args, err)
		return result{}
	}
}

// write puts content at a path relative to the project.
func (e *setupEnv) write(t *testing.T, rel, content string) {
	t.Helper()
	path := filepath.Join(e.repo, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (e *setupEnv) read(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(e.repo, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// mustRun fails the test unless the command exits 0.
func (e *setupEnv) mustRun(t *testing.T, name string, args ...string) result {
	t.Helper()
	r := e.do(t, name, args...)
	if r.code != 0 {
		t.Fatalf("%s %v: exit %d\n%s%s", name, args, r.code, r.stdout, r.stderr)
	}
	return r
}

const (
	setupTool = "package tool\n\n// Run does the work.\nfunc Run() {}\n"

	setupOutlinePage = `  - path: docs/use-the-tool.md
    slug: use-the-tool
    title: Use the tool
    type: task guide
    job: |
      Run the tool once.
    covers:
      - Call Run.
    outOfScope:
      - Installing the tool.
`

	setupBlueprint = `version: 1
argument: |
  The tool runs with one call.
introduction:
  covers: |
    How to run the tool.
  priorKnowledge: |
    The reader has the tool installed.
  notCovered: |
    Installing the tool.
framing:
  readers: |
    A developer who has the tool installed.
  background: |
    The tool exposes one function.
orderReason: |
  The page has one section, so the order is that section.
sections:
  - id: run
    heading: Run the tool
    why: |
      Running the tool is the page's job.
    opens: |
      Call the function.
    items:
      - id: c-run
        kind: sentence
        mode: claim
        text: |
          The tool defines a function named Run.
        facts: [f:abc123]
`

	setupFact = "# Tool facts\n\n- `f:abc123` The tool defines a function named Run. Source: `tool.go:3-4` (`Run`). [verified]\n"

	setupPage = "# Use the tool\n\nSee the [readme](../README.md).\n\n## Run the tool\n\nThe tool defines a function named Run.\n"

	// setupStyle is a one-rule synthetic Google style, so the project's .vale.ini resolves offline.
	setupStyle = "extends: existence\nmessage: \"avoid '%s'\"\nlevel: error\ntokens:\n  - foobarbaz\n"
)

// The new-project path end to end, on a machine with nothing configured: init, check, stock
// defaults, and the stock gate.
func TestSetupFromNothing(t *testing.T) {
	e := newSetupEnv(t)
	e.mustRun(t, "git", "init", "-q")
	e.write(t, "README.md", "# Project\n")
	e.write(t, "tool.go", setupTool)

	// 1. init, then check.
	e.mustRun(t, "drafter", "init")
	r := e.mustRun(t, "drafter", "check")
	if !strings.Contains(r.stderr, "facts store: 1 pointers checked") {
		t.Fatalf("check stderr lacks the pointer count:\n%s", r.stderr)
	}

	// 2. Only a key source, a workspace, and two caps in the user layer.
	userConfig := filepath.Join(e.home, ".config", "drafter", "config.toml")
	if err := os.MkdirAll(filepath.Dir(userConfig), 0o755); err != nil {
		t.Fatal(err)
	}
	user := "key_env = \"DRAFTER_API_KEY\"\nworkspace_id = \"wrkspc_synthetic\"\npage_cap = 5\ncycle_ceiling = 20\n"
	if err := os.WriteFile(userConfig, []byte(user), 0o644); err != nil {
		t.Fatal(err)
	}
	e.write(t, "drafter.toml", e.read(t, "drafter.toml")+"prompts = \"prompts.js\"\n")
	e.write(t, "prompts.js", "// synthetic prompts\n")
	r = e.mustRun(t, "drafter", "config")
	for _, key := range []string{"gate.command", "seat.draft.model", "prices.date"} {
		found := false
		for line := range strings.SplitSeq(r.stdout, "\n") {
			if strings.Contains(line, key) && strings.Contains(line, "default") {
				found = true
			}
		}
		if !found {
			t.Errorf("config row for %s lacks the source default:\n%s", key, r.stdout)
		}
	}

	// 3. A managed page, its blueprint and fact, the synthetic style, then a clean tree and the gate.
	e.write(t, "blueprints/outline.yaml", e.read(t, "blueprints/outline.yaml")+setupOutlinePage)
	e.write(t, "blueprints/docs/use-the-tool.blueprint.yaml", setupBlueprint)
	e.write(t, "blueprints/facts/tool.md", setupFact)
	e.write(t, "docs/use-the-tool.md", setupPage)
	e.write(t, ".vale/styles/Google/Bad.yml", setupStyle)
	e.mustRun(t, "git", "add", ".")
	e.mustRun(t, "git", "commit", "-q", "-m", "setup")
	if r := e.mustRun(t, "git", "status", "--porcelain"); r.stdout != "" {
		t.Fatalf("the tree is not clean after the commit (the style should be ignored):\n%s", r.stdout)
	}
	r = e.mustRun(t, "drafter", "check")
	if !strings.Contains(r.stderr, "facts store: 2 pointers checked") {
		t.Fatalf("check stderr lacks the pointer count:\n%s", r.stderr)
	}
	e.mustRun(t, "drafter", "gate", "--page", "docs/use-the-tool.md")

	// 4. Each red, restored after.
	const page = "docs/use-the-tool.md"
	red := func(name, mutated string, want int, wantOut string) {
		t.Helper()
		e.write(t, page, mutated)
		defer e.write(t, page, setupPage)
		r := e.do(t, "drafter", "gate", "--page", page)
		if r.code != want || !strings.Contains(r.stdout+r.stderr, wantOut) {
			t.Errorf("%s: exit %d, want %d containing %q:\n%s%s", name, r.code, want, wantOut, r.stdout, r.stderr)
		}
	}
	red("style token", setupPage+"\nfoobarbaz\n", 1, page+":")
	red("missing link target", strings.Replace(setupPage, "../README.md", "../MISSING.md", 1), 1, "links")

	e.write(t, "blueprints/facts/tool.md", strings.Replace(setupFact, "(`Run`)", "(`Absent`)", 1))
	if r := e.do(t, "drafter", "gate", "--page", page); r.code != 1 || !strings.Contains(r.stdout, "store") {
		t.Errorf("broken fact anchor: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	e.write(t, "blueprints/facts/tool.md", setupFact)

	if err := os.RemoveAll(filepath.Join(e.repo, ".vale", "styles", "Google")); err != nil {
		t.Fatal(err)
	}
	if r := e.do(t, "drafter", "gate", "--page", page); r.code != 2 || !strings.Contains(r.stdout+r.stderr, "vale sync") {
		t.Errorf("missing style: exit %d, want 2 naming vale sync\n%s%s", r.code, r.stdout, r.stderr)
	}
}
