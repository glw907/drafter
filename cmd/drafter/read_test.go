package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/blueprint"
	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/pipeline"
	"github.com/glw907/drafter/internal/prompts"
)

// readProject is a committed project with a blueprint root, an outline of two pages, and a lock.
func readProject(t *testing.T) (root string, c *config.Config) {
	t.Helper()
	root = t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("blueprints/outline.yaml", `version: 1
types:
  - name: task guide
    anatomy: Task guide
pages:
  - path: docs/a.md
    slug: a
    title: Page A
    type: task guide
    job: |
      Do A.
    covers:
      - A.
  - path: docs/b.md
    slug: b
    title: Page B
    type: task guide
    job: |
      Do B.
`)
	write("blueprints/docs/a.blueprint.yaml", "version: 1\n")
	write("blueprints/docs/b.blueprint.yaml", "version: 1\n")
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-q", "-m", "fixture"}} {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return root, &config.Config{Blueprints: config.Blueprints{Root: "blueprints", Outline: "blueprints/outline.yaml"}}
}

func lockBytes(t *testing.T, root string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "blueprints/drafted.lock"))
	if err != nil {
		return ""
	}
	return string(b)
}

func TestRecordAccepted(t *testing.T) {
	root, c := readProject(t)
	two := pipeline.Summary{Pages: []pipeline.PageResult{
		{Path: "docs/a.md", Status: pipeline.StatusAccepted, Accepted: "aaa"},
		{Path: "docs/b.md", Status: pipeline.StatusEscalated, Rule: "plan"},
	}}

	for name, tc := range map[string]struct {
		s   pipeline.Summary
		dry bool
	}{
		"a stopped run": {pipeline.Summary{Stop: "cycle cap", Pages: two.Pages}, false},
		"a dry run":     {two, true},
	} {
		t.Run(name+" writes no lock", func(t *testing.T) {
			if err := recordAccepted(c, root, tc.s, tc.dry); err != nil {
				t.Fatal(err)
			}
			if got := lockBytes(t, root); got != "" {
				t.Errorf("lock = %q, want none", got)
			}
		})
	}

	t.Run("one accepted and one set aside leaves exactly one new accepted id", func(t *testing.T) {
		if err := recordAccepted(c, root, two, false); err != nil {
			t.Fatal(err)
		}
		l, err := blueprint.ReadLock(filepath.Join(root, "blueprints/drafted.lock"))
		if err != nil {
			t.Fatal(err)
		}
		if len(l.Pages) != 1 || l.Pages["docs/a.md"].Accepted != "aaa" {
			t.Errorf("lock pages = %+v, want one entry, docs/a.md accepted aaa", l.Pages)
		}
	})

	t.Run("an accepted id already recorded leaves the file as it was", func(t *testing.T) {
		before := lockBytes(t, root)
		if err := recordAccepted(c, root, two, false); err != nil {
			t.Fatal(err)
		}
		if got := lockBytes(t, root); got != before {
			t.Errorf("lock changed:\n%s\nto\n%s", before, got)
		}
	})

	t.Run("a fix keeps an earlier accepted id and a new acceptance keeps the entry's other fields", func(t *testing.T) {
		l, _ := blueprint.ReadLock(filepath.Join(root, "blueprints/drafted.lock"))
		l.Pages["docs/b.md"] = blueprint.LockEntry{Page: "pb", Blueprint: "bb", Accepted: "old"}
		if err := blueprint.WriteLock(filepath.Join(root, "blueprints/drafted.lock"), l); err != nil {
			t.Fatal(err)
		}
		s := pipeline.Summary{Pages: []pipeline.PageResult{
			{Path: "docs/a.md", Status: pipeline.StatusAccepted, Accepted: "aaa2"},
			{Path: "docs/b.md", Status: pipeline.StatusEscalated, Rule: "plan"},
		}}
		if err := recordAccepted(c, root, s, false); err != nil {
			t.Fatal(err)
		}
		got, _ := blueprint.ReadLock(filepath.Join(root, "blueprints/drafted.lock"))
		if got.Pages["docs/b.md"].Accepted != "old" || got.Pages["docs/b.md"].Page != "pb" || got.Pages["docs/a.md"].Accepted != "aaa2" {
			t.Errorf("lock pages = %+v", got.Pages)
		}
	})
}

func TestBlueprintHandoffs(t *testing.T) {
	root, c := readProject(t)
	head, err := gitText(root, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	hs, err := blueprintHandoffs(c, root, head, []string{filepath.Join(root, "blueprints/docs/a.blueprint.yaml")})
	if err != nil {
		t.Fatal(err)
	}
	h := hs[0]
	if h.Page.Path != "docs/a.md" || h.Page.Brief != "blueprints/docs/a.brief.json" || h.Page.Type != "task guide" || strings.TrimSpace(h.Inputs.Job) != "Do A." {
		t.Errorf("handoff page = %+v, job %q", h.Page, h.Inputs.Job)
	}
	if h.Plan != "blueprints/docs/a.blueprint.yaml" || h.Framing != "" {
		t.Errorf("plan %q, framing %q", h.Plan, h.Framing)
	}
	if err := h.Verify(); err != nil {
		t.Errorf("Verify: %v", err)
	}
	var ctx struct {
		Resolved struct {
			Pages []map[string]any `json:"pages"`
		} `json:"resolved"`
	}
	if err := json.Unmarshal(h.Context, &ctx); err != nil || len(ctx.Resolved.Pages) != 1 || ctx.Resolved.Pages[0]["path"] != "docs/a.md" || ctx.Resolved.Pages[0]["pageType"] != "task guide" {
		t.Errorf("context = %s, %v", h.Context, err)
	}

	for name, arg := range map[string]string{
		"not a blueprint":  filepath.Join(root, "blueprints/outline.yaml"),
		"outside the root": filepath.Join(t.TempDir(), "x.blueprint.yaml"),
	} {
		if _, err := blueprintHandoffs(c, root, head, []string{arg}); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

// TestRealModuleRendersTheBlueprintSeat loads the shipped prompts module the way drafter loads it
// for a read, with the host a read's handoff builds and no chain args, and renders the seat.
func TestRealModuleRendersTheBlueprintSeat(t *testing.T) {
	root, c := readProject(t)
	head, _ := gitText(root, "rev-parse", "HEAD")
	hs, err := blueprintHandoffs(c, root, head, []string{filepath.Join(root, "blueprints/docs/a.blueprint.yaml")})
	if err != nil {
		t.Fatal(err)
	}
	var ctx struct {
		Resolved json.RawMessage `json:"resolved"`
	}
	if err := json.Unmarshal(hs[0].Context, &ctx); err != nil {
		t.Fatal(err)
	}
	modulePath := "../../claude/.claude/workflows/docs-page-chain.js"
	src, err := os.ReadFile(modulePath)
	if err != nil {
		t.Fatal(err)
	}
	mod, err := prompts.Load(filepath.Base(modulePath), src, prompts.Host{Args: json.RawMessage("{}"), Resolved: ctx.Resolved})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	page, _ := json.Marshal(map[string]any{"path": hs[0].Page.Path, "job": hs[0].Inputs.Job})
	out, err := mod.Render("blueprint", prompts.Context{Page: page, Inputs: json.RawMessage("{}"), Round: prompts.Round1})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{"Blueprint read.", "Reader state.", "A `use` names scope only."} {
		if !strings.Contains(out, want) {
			t.Errorf("the prompt lacks %q", want)
		}
	}
	if strings.Contains(out, "docs/a.md") || strings.Contains(out, ".blueprint.yaml") {
		t.Error("the prompt names the page or a blueprint path, which the request would inline")
	}
}
