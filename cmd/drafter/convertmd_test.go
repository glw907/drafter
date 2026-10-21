package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConvertMarkdownCommand(t *testing.T) {
	root := seedProject(t)
	outline := copyFixture(t, t.TempDir(), "widget.outline.json")
	if _, _, err := runConvert(t, "outline", "--out", filepath.Join(root, "blueprints", "outline.yaml"), outline); err != nil {
		t.Fatal(err)
	}
	gitIn(t, root, "add", ".")
	gitIn(t, root, "commit", "-q", "-m", "outline")

	stdout, stderr, err := runConvert(t, "markdown", "--root", root, filepath.Join(root, filepath.FromSlash(seedBP)))
	if err != nil {
		t.Fatalf("convert markdown: %v\n%s", err, stderr)
	}
	md := strings.TrimSuffix(seedBP, ".yaml") + ".md"
	if strings.TrimSpace(stdout) != md {
		t.Errorf("stdout = %q, want %q", stdout, md)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(md))); err != nil {
		t.Errorf("no markdown blueprint: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(seedBP))); err == nil {
		t.Error("the YAML blueprint remains")
	}
	if !strings.Contains(stderr, seedPage+": ") {
		t.Errorf("stderr lacks the judgment lines: %q", stderr)
	}

	if _, _, err := runConvert(t, "markdown", "--root", root, filepath.Join(root, filepath.FromSlash(md))); err == nil {
		t.Error("converting a markdown blueprint was not refused")
	}
}
