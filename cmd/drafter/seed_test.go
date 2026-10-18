package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/blueprint"
	"github.com/spf13/cobra"
)

const (
	seedPage = "docs/guides/rotate-the-widget-key.md"
	seedBP   = "blueprints/docs/guides/rotate-the-widget-key.blueprint.yaml"
)

// runSeedCmd runs the seed command under a root of its own, as root.go mounts it.
func runSeedCmd(t *testing.T, args ...string) (stdout string, code int, msg string) {
	t.Helper()
	root := &cobra.Command{Use: "drafter", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(newSeedCmd())
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"seed"}, args...))
	err := root.Execute()
	if err == nil {
		return out.String(), 0, ""
	}
	ee, ok := errors.AsType[exitError](err)
	if !ok {
		t.Fatalf("error %v is not an exitError", err)
	}
	return out.String(), ee.code, ee.msg
}

// seedProject builds a committed page and blueprint with a lock entry that holds only accepted,
// and returns the root.
func seedProject(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	gitIn(t, root, "init", "-q")
	page := filepath.Join(root, filepath.FromSlash(seedPage))
	if err := os.MkdirAll(filepath.Dir(page), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(page, []byte("# Rotate\n\n## Before you begin\nx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, root, "add", ".")
	gitIn(t, root, "commit", "-q", "-m", "page")

	work := t.TempDir()
	planFile := copyFixture(t, work, "widget.plan.yaml")
	framing := copyFixture(t, work, "widget.framing.md")
	outline := copyFixture(t, work, "widget.outline.json")
	if _, _, err := runConvert(t, "plan", "--root", root, "--framing", framing, "--outline", outline, planFile); err != nil {
		t.Fatal(err)
	}
	gitIn(t, root, "add", ".")
	gitIn(t, root, "commit", "-q", "-m", "blueprint")

	// Conversion seeded a full entry; reduce it to what drafter read leaves behind.
	id, err := blueprint.CommitBlobID(root, "HEAD", seedBP)
	if err != nil {
		t.Fatal(err)
	}
	lock := &blueprint.Lock{Version: blueprint.Version, Pages: map[string]blueprint.LockEntry{seedPage: {Accepted: id}}}
	if err := blueprint.WriteLock(filepath.Join(root, "blueprints", blueprint.LockFile), lock); err != nil {
		t.Fatal(err)
	}
	gitIn(t, root, "add", ".")
	gitIn(t, root, "commit", "-q", "-m", "lock")
	return root
}

func TestSeedCommand(t *testing.T) {
	root := seedProject(t)

	stdout, code, msg := runSeedCmd(t, "--root", root, filepath.Join(root, seedBP))
	if code != 0 || stdout != "seeded "+seedPage+"\n" {
		t.Fatalf("code %d, stdout %q, msg %q", code, stdout, msg)
	}
	lock, err := blueprint.ReadLock(filepath.Join(root, "blueprints", blueprint.LockFile))
	if err != nil {
		t.Fatal(err)
	}
	e := lock.Pages[seedPage]
	if e.Page == "" || e.Blueprint != e.Accepted || len(e.Sections) == 0 {
		t.Errorf("entry = %+v", e)
	}

	_, code, msg = runSeedCmd(t, "--root", root, filepath.Join(root, seedBP))
	if code != exitCannotRun || !strings.Contains(msg, "already records a drafted page") {
		t.Errorf("second seed: code %d, msg %q", code, msg)
	}
	if _, code, _ := runSeedCmd(t, "--root", root); code != exitCannotRun {
		t.Errorf("no blueprint named: code %d", code)
	}
}
