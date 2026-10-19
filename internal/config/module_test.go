package config

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glw907/drafter/stock"
)

func TestModule(t *testing.T) {
	dir := t.TempDir()
	custom := filepath.Join(dir, "custom.js")
	writeFile(t, custom, "return {};\n")
	tests := []struct {
		name     string
		prompts  string
		wantName string
		wantSrc  []byte
		wantErr  string
	}{
		{"unset gives the stock module", "", "drafter:prompts.js", stock.Prompts, ""},
		{"set gives the file", custom, custom, []byte("return {};\n"), ""},
		{"a missing file is an error", filepath.Join(dir, "absent.js"), "", nil, "read prompts module"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newConfig("/home/test")
			c.Prompts = tt.prompts
			name, src, err := c.Module()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Module error %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Module: %v", err)
			}
			if name != tt.wantName || !bytes.Equal(src, tt.wantSrc) {
				t.Errorf("Module = %q, %d bytes, want %q, %d bytes", name, len(src), tt.wantName, len(tt.wantSrc))
			}
			if c.ModuleName() != tt.wantName {
				t.Errorf("ModuleName = %q, want %q", c.ModuleName(), tt.wantName)
			}
		})
	}
}

func TestPromptsRow(t *testing.T) {
	tests := []struct {
		name      string
		project   string
		files     map[string]string
		wantValue func(root string) string
		wantFrom  string
	}{
		{
			"stock module from default", "", nil,
			func(string) string { return "drafter:prompts.js " + ShortHash(stock.Prompts) }, "default",
		},
		{
			"a file shows path and hash", "prompts = \"p.js\"\n", map[string]string{"p.js": "return {};\n"},
			func(root string) string { return filepath.Join(root, "p.js") + " " + ShortHash([]byte("return {};\n")) },
			"drafter.toml",
		},
		{
			"a missing file shows missing", "prompts = \"p.js\"\n", nil,
			func(root string) string { return filepath.Join(root, "p.js") + " (missing)" }, "drafter.toml",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sb := newSandbox(t)
			writeFile(t, sb.user, stockUserLayer)
			if tt.project != "" {
				writeFile(t, sb.project(), tt.project)
			}
			for name, content := range tt.files {
				writeFile(t, filepath.Join(sb.root, name), content)
			}
			c, err := Load(sb.root, Flags{})
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			r := row(t, c, "prompts")
			if want := tt.wantValue(sb.root); r.Value != want || r.From != tt.wantFrom {
				t.Errorf("prompts = %q from %q, want %q from %q", r.Value, r.From, want, tt.wantFrom)
			}
		})
	}
}

func TestShortHash(t *testing.T) {
	// sha256 of the empty input.
	if got, want := ShortHash(nil), "sha256:e3b0c44298fc"; got != want {
		t.Errorf("ShortHash(nil) = %q, want %q", got, want)
	}
}

func TestRequirePromptsLayers(t *testing.T) {
	const misplaced = "require_prompts is not allowed here; move it to drafter.toml"
	tests := []struct {
		name    string
		layer   string // user, project, or local
		env     string
		wantErr string
		want    bool
	}{
		{"the project layer sets it", "project", "", "", true},
		{"the user layer is refused", "user", "", misplaced, false},
		{"the local layer is refused", "local", "", misplaced, false},
		{"the environment cannot unset it", "project", "false", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sb := newSandbox(t)
			const body = "require_prompts = true\n"
			writeFile(t, sb.user, stockUserLayer)
			writeFile(t, sb.project(), "mode = \"batch\"\n")
			switch tt.layer {
			case "user":
				writeFile(t, sb.user, stockUserLayer+body)
			case "project":
				writeFile(t, sb.project(), body)
			case "local":
				sb.ignoreLocal(t)
				writeFile(t, sb.local(), body)
			}
			if tt.env != "" {
				t.Setenv("DRAFTER_REQUIRE_PROMPTS", tt.env)
			}
			c, err := Load(sb.root, Flags{})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Load error %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if c.RequirePrompts != tt.want {
				t.Errorf("RequirePrompts = %v, want %v", c.RequirePrompts, tt.want)
			}
			if err := c.Validate(ScopeRun); (err != nil) != tt.want {
				t.Errorf("Validate(ScopeRun) = %v, want a refusal exactly when require_prompts is true", err)
			}
		})
	}
}
