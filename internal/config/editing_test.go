package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditingPreferences(t *testing.T) {
	sb := newSandbox(t)
	writeFile(t, sb.project(), "[editing]\npreferences = \"docs/editing.md\"\n")
	c, err := Load(sb.root, Flags{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Editing.Preferences != "docs/editing.md" {
		t.Errorf("Editing.Preferences = %q", c.Editing.Preferences)
	}
	if r := row(t, c, "editing.preferences"); r.From != "drafter.toml" || r.Value != "docs/editing.md" {
		t.Errorf("row = %+v", r)
	}
}

func TestEditingPreferencesUnset(t *testing.T) {
	sb := newSandbox(t)
	writeFile(t, sb.project(), "mode = \"batch\"\n")
	c, err := Load(sb.root, Flags{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if r := row(t, c, "editing.preferences"); r.From != "unset" || r.Value != "" {
		t.Errorf("row = %+v, want unset", r)
	}
}

func TestEditingRefusals(t *testing.T) {
	tests := []struct {
		name, layer, body, want string
	}{
		{"user layer", "user", "[editing]\npreferences = \"p.md\"\n", "editing is not allowed here; move it to drafter.toml"},
		{"outside the project", "project", "[editing]\npreferences = \"../p.md\"\n", "editing.preferences must be a path inside the project, relative to its root, not \"../p.md\""},
		{"absolute", "project", "[editing]\npreferences = \"/p.md\"\n", "editing.preferences must be a path inside the project, relative to its root, not \"/p.md\""},
		{"unknown key", "project", "[editing]\nfile = \"p.md\"\n", "unknown key editing.file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sb := newSandbox(t)
			writeFile(t, sb.project(), "mode = \"batch\"\n")
			if tt.layer == "user" {
				writeFile(t, sb.user, tt.body)
			} else {
				writeFile(t, sb.project(), tt.body)
			}
			_, err := Load(sb.root, Flags{})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Load error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestValidateRefusesMissingPreferences(t *testing.T) {
	root := t.TempDir()
	c := complete()
	c.Root = root
	c.Editing.Preferences = "docs/editing.md"
	err := c.Validate(ScopeRun)
	if err == nil || !strings.Contains(err.Error(), "editing.preferences names a file that cannot be read") {
		t.Fatalf("Validate = %v, want the missing-file refusal", err)
	}
	if err := c.Validate(ScopeMoney); err != nil {
		t.Errorf("money scope refused a missing preferences file: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "editing.md"), []byte("Prefer short words.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(ScopeRun); err != nil {
		t.Errorf("Validate with the file present = %v", err)
	}
}
