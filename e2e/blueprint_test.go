package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const bpOutline = `version: 1
types:
  - name: task guide
    anatomy: Task guide
pages:
  - path: docs/guides/rotate-the-api-key.md
    slug: rotate
    title: Rotate the API key
    type: task guide
    job: |
      Rotate a key.
`

const bpFacts = "# Facts\n\n- `f:aaaaaa` A key lives for 90 days. Source: `drafter.toml`. [verified]\n"

const bpBlueprint = `version: 1
argument: |
  Rotate the key before it expires.
introduction:
  covers: |
    How to rotate a key.
  priorKnowledge: |
    You can publish a site.
  notCovered: |
    Revoking a key.
  facts: [f:aaaaaa]
orderReason: |
  The page has one section.
sections:
  - id: rotate
    heading: Rotate the key
    why: |
      Rotation is the page's job.
    opens: |
      Rotate the key before it expires.
    items:
      - id: f-life
        kind: fact
        fact: f:aaaaaa
        use: |
          Only the lifetime.
`

const bpJSONOutline = `{"pages":[{"path":"docs/a.md","slug":"a","title":"A","pageType":"task guide","job":"Do A."}],"crossLinks":[]}`

func TestBlueprintCommands(t *testing.T) {
	stub := newAPIStub(t)
	p := newPageEnv(t, stub)
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(p.dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, path, body)
	}
	write("blueprints/outline.yaml", bpOutline)
	write("blueprints/facts/keys.md", bpFacts)
	write("blueprints/docs/guides/rotate-the-api-key.blueprint.yaml", bpBlueprint)
	gitRun(t, p.dir, "add", "-A")
	gitRun(t, p.dir, "commit", "-q", "-m", "blueprints")

	t.Run("check passes on a valid project", func(t *testing.T) {
		if r := runEnv(t, p.dir, p.env, "check"); r.code != 0 {
			t.Errorf("exit %d\nstdout: %s\nstderr: %s", r.code, r.stdout, r.stderr)
		}
	})

	t.Run("check exits 1 on an error and 2 when it cannot run", func(t *testing.T) {
		bad := strings.Replace(bpBlueprint, "    why: |\n      Rotation is the page's job.\n", "", 1)
		write("blueprints/docs/guides/rotate-the-api-key.blueprint.yaml", bad)
		defer write("blueprints/docs/guides/rotate-the-api-key.blueprint.yaml", bpBlueprint)
		r := runEnv(t, p.dir, p.env, "check")
		if r.code != 1 || !strings.Contains(r.stdout, "why") {
			t.Errorf("exit %d, stdout %q, want 1 and a why finding", r.code, r.stdout)
		}
		empty := t.TempDir()
		gitRun(t, empty, "init", "-q", "-b", "main")
		if r := runEnv(t, empty, p.env, "check"); r.code != 2 {
			t.Errorf("exit %d without a blueprint root, want 2\nstderr: %s", r.code, r.stderr)
		}
	})

	t.Run("refs lists placements and exits 1 for a fact nothing places", func(t *testing.T) {
		r := runEnv(t, p.dir, p.env, "refs", "f:aaaaaa")
		if r.code != 0 || strings.Count(strings.TrimSpace(r.stdout), "\n") != 1 {
			t.Errorf("exit %d, stdout %q, want two lines (the introduction and the fact item)", r.code, r.stdout)
		}
		if r := runEnv(t, p.dir, p.env, "refs", "f:nothing"); r.code != 1 {
			t.Errorf("exit %d for an unknown fact, want 1", r.code)
		}
	})

	t.Run("convert outline writes outline.yaml", func(t *testing.T) {
		in := filepath.Join(t.TempDir(), "outline.json")
		writeFile(t, in, bpJSONOutline)
		out := filepath.Join(t.TempDir(), "outline.yaml")
		if r := runEnv(t, p.dir, p.env, "convert", "outline", "--out", out, in); r.code != 0 {
			t.Fatalf("exit %d\nstderr: %s", r.code, r.stderr)
		}
		data, err := os.ReadFile(out)
		if err != nil || !strings.Contains(string(data), "task guide") {
			t.Errorf("outline.yaml = %q, %v", data, err)
		}
		if r := runEnv(t, p.dir, p.env, "convert", "outline", "--out", out, in); r.code == 0 {
			t.Error("convert outline overwrote an existing file")
		}
	})

	t.Run("read refuses to start when the prompts lack the blueprint seat", func(t *testing.T) {
		r := runEnv(t, p.dir, p.env, "read", "--dry-run", "blueprints/docs/guides/rotate-the-api-key.blueprint.yaml")
		if r.code != 2 || !strings.Contains(r.stderr, `no seat "blueprint"`) {
			t.Errorf("exit %d, stderr %q, want 2 naming the blueprint seat", r.code, r.stderr)
		}
	})

	t.Run("read dry-run estimates the stage and writes nothing", func(t *testing.T) {
		toml, err := os.ReadFile(filepath.Join(p.dir, "drafter.toml"))
		if err != nil {
			t.Fatal(err)
		}
		write("drafter.toml", string(toml)+"\n[seat.blueprint]\nmodel = \"model-a\"\neffort = \"medium\"\nmax_tokens = 4000\n")
		write("prompts.js", `return { draft: (c) => "Draft " + c.page.path, blueprint: () => "Read the blueprint." };`+"\n")
		gitRun(t, p.dir, "add", "-A")
		gitRun(t, p.dir, "commit", "-q", "-m", "seat")
		r := runEnv(t, p.dir, p.env, "read", "--dry-run", "blueprints/docs/guides/rotate-the-api-key.blueprint.yaml")
		if r.code != 0 || !strings.Contains(r.stdout, "blueprint: 5000 input tokens") {
			t.Errorf("exit %d\nstdout: %s\nstderr: %s", r.code, r.stdout, r.stderr)
		}
		if got := stub.submitted(); len(got) != 0 {
			t.Errorf("a dry run submitted %v", got)
		}
		if _, err := os.Stat(filepath.Join(p.dir, "blueprints/drafted.lock")); err == nil {
			t.Error("a dry run wrote drafted.lock")
		}
	})
}
