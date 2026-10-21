package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigPrintsTheEditingPreferences(t *testing.T) {
	p := newCfgProject(t)
	p.user(t, completeUser)
	proj := filepath.Join(p.dir, "drafter.toml")
	base, err := os.ReadFile(proj)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, proj, string(base)+"\n[editing]\npreferences = \"docs/editing.md\"\n")
	if err := os.MkdirAll(filepath.Join(p.dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(p.dir, "docs", "editing.md"), "Prefer short words.\n")

	r := p.config(t, nil)
	if r.code != 0 || !strings.Contains(r.stdout, "editing.preferences = docs/editing.md  (drafter.toml)") {
		t.Errorf("exit %d, stdout %q, stderr %q; want the editing.preferences row from drafter.toml", r.code, r.stdout, r.stderr)
	}
}

func TestConfigRefusesAMissingPreferencesFile(t *testing.T) {
	p := newCfgProject(t)
	p.user(t, completeUser)
	proj := filepath.Join(p.dir, "drafter.toml")
	base, err := os.ReadFile(proj)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, proj, string(base)+"\n[editing]\npreferences = \"docs/editing.md\"\n")

	r := p.config(t, nil)
	if r.code != 1 || !strings.Contains(r.stderr, "editing.preferences names a file that cannot be read") {
		t.Errorf("exit %d, stderr %q; want exit 1 and the missing-file refusal", r.code, r.stderr)
	}
	if !strings.Contains(r.stdout, "editing.preferences = docs/editing.md  (drafter.toml)") {
		t.Errorf("the row did not print before the refusal:\n%s", r.stdout)
	}
}
