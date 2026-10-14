package assemble

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/config"
)

// fixture is a synthetic project: a git repo copied from testdata with every file tracked but
// untracked.txt, and an outside directory holding files no prompt may pull in.
type fixture struct {
	root    string // the repo
	outside string // a sibling directory of the repo
}

// newFixture copies testdata into a fresh repo under t.TempDir(), builds the outside tree and the
// symlinks, and tracks everything but untracked.txt.
func newFixture(t *testing.T) fixture {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := fixture{root: filepath.Join(base, "repo"), outside: filepath.Join(base, "outside")}
	if err := os.CopyFS(f.root, os.DirFS("testdata")); err != nil {
		t.Fatal(err)
	}
	f.write(t, filepath.Join(f.outside, "secret.txt"), "OUTSIDE-SECRET\n")
	f.write(t, filepath.Join(f.outside, "exemplars", "ok.txt"), "ALLOWED-OK\n")
	f.write(t, filepath.Join(f.outside, "exemplars-old", "f.txt"), "OLD-SECRET\n")
	f.link(t, filepath.Join(f.outside, "secret.txt"), filepath.Join(f.outside, "exemplars", "link.txt"))
	f.link(t, filepath.Join(f.outside, "secret.txt"), filepath.Join(f.root, "docs", "escape.md"))
	f.link(t, filepath.Join(f.outside, "exemplars", "ok.txt"), filepath.Join(f.root, "docs", "allowed-link.md"))
	f.write(t, filepath.Join(f.root, "big.txt"), strings.Repeat("x", 300<<10))
	f.git(t, "init", "-q")
	f.git(t, "add", ".")
	f.git(t, "rm", "-q", "--cached", "untracked.txt")
	return f
}

func (f fixture) write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f fixture) link(t *testing.T, target, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, name); err != nil {
		t.Fatal(err)
	}
}

func (f fixture) git(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = f.root
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// config returns a Config rooted at the fixture with one draft seat.
func (f fixture) config(inline ...string) *config.Config {
	return &config.Config{
		Root:        f.root,
		KeyEnv:      "DRAFTER_TEST_KEY",
		InlineFiles: inline,
		Seat: map[string]config.Seat{
			"draft": {Model: "model-x", Effort: "high", MaxTokens: 4000},
		},
	}
}

// input returns a draft-seat Input for prompt.
func (f fixture) input(c *config.Config, prompt string) Input {
	return Input{
		Seat: "draft", Answer: AnswerDraft, Config: c, Prompt: prompt,
		Page: "docs/page.md", Brief: "docs/page.brief.json", Slug: "page", CustomID: "id-1",
	}
}

// requestText builds the request and returns every message's text, joined.
func requestText(t *testing.T, in Input) string {
	t.Helper()
	req, err := Build(context.Background(), in)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var b strings.Builder
	for _, m := range req.Messages {
		b.WriteString(m.Text)
		b.WriteString("\n")
	}
	return b.String()
}
