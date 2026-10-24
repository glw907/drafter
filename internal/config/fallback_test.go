package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// worktreeOf makes a git repository named repo with one commit and a linked worktree of it, and
// returns the worktree's path. The repository holds a drafter.toml and ignores drafter.local.toml,
// so the worktree starts with no local file.
func worktreeOf(t *testing.T, repo string) string {
	t.Helper()
	main := filepath.Join(t.TempDir(), repo)
	if err := os.MkdirAll(main, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, main, "init", "-q")
	writeFile(t, filepath.Join(main, "drafter.toml"), "require_prompts = true\n")
	writeFile(t, filepath.Join(main, ".gitignore"), "drafter.local.toml\n")
	runGit(t, main, "add", ".")
	runGit(t, main, "commit", "-q", "-m", "start")
	wt := filepath.Join(t.TempDir(), "wt")
	runGit(t, main, "worktree", "add", "-q", wt)
	return wt
}

func TestLocalFallbackInAWorktree(t *testing.T) {
	sb := newSandbox(t)
	wt := worktreeOf(t, "mainrepo")
	fallback := filepath.Join(filepath.Dir(sb.user), "mainrepo.local.toml")

	t.Run("a worktree with no local file reads the user-scope copy", func(t *testing.T) {
		writeFile(t, fallback, "mode = \"sync\"\npage_cap = 7\nprompts = \"prompts.js\"\n")
		c, err := Load(wt, Flags{})
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if c.Mode != "sync" || c.PageCap != 7 {
			t.Errorf("mode %q, page_cap %v; want sync and 7 from the fallback layer", c.Mode, c.PageCap)
		}
		if c.LocalFallback != fallback {
			t.Errorf("LocalFallback = %q, want %q", c.LocalFallback, fallback)
		}
		if got := row(t, c, "mode").From; got != fallback {
			t.Errorf("mode comes from %q, want the fallback path %q", got, fallback)
		}
	})

	t.Run("the fallback resolves a relative path from its own directory", func(t *testing.T) {
		c, err := Load(wt, Flags{})
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if want := filepath.Join(filepath.Dir(fallback), "prompts.js"); c.Prompts != want {
			t.Errorf("prompts = %q, want %q", c.Prompts, want)
		}
	})

	t.Run("the worktree's own drafter.local.toml wins", func(t *testing.T) {
		writeFile(t, filepath.Join(wt, "drafter.local.toml"), "mode = \"batch\"\n")
		c, err := Load(wt, Flags{})
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if c.Mode != "batch" || c.PageCap == 7 {
			t.Errorf("mode %q, page_cap %v; want the local file alone", c.Mode, c.PageCap)
		}
	})
}

func TestLocalFallbackNeedsGit(t *testing.T) {
	sb := newSandbox(t)
	t.Setenv("GIT_CEILING_DIRECTORIES", sb.root+"/..")
	writeFile(t, sb.project(), "require_prompts = true\n")
	writeFile(t, filepath.Join(filepath.Dir(sb.user), filepath.Base(sb.root)+".local.toml"), "mode = \"sync\"\n")
	c, err := Load(sb.root, Flags{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.LocalFallback != "" || c.Mode == "sync" {
		t.Errorf("fallback %q, mode %q; want no fallback outside git", c.LocalFallback, c.Mode)
	}
}

func TestStockRefusalNamesBothPaths(t *testing.T) {
	c := &Config{LocalFallback: "/u/drafter/mainrepo.local.toml"}
	got := c.StockRefusal()
	for _, want := range []string{RefusedStock, "drafter.local.toml", "/u/drafter/mainrepo.local.toml"} {
		if !strings.Contains(got, want) {
			t.Errorf("StockRefusal = %q, want it to contain %q", got, want)
		}
	}
}
