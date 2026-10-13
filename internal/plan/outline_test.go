package plan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadOutline(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	tests := []struct {
		name, file string
		listed     []string
		unlisted   []string
		wantErr    string
	}{
		{"the fixture", "testdata/repo/docs/outline.json", []string{"docs/reference/planned.md"}, []string{"docs/reference/keys.md"}, ""},
		{"paths are cleaned", write("a.json", `{"pages":[{"path":"./docs/a.md"},{"path":"docs//b.md"}]}`), []string{"docs/a.md", "docs/b.md"}, nil, ""},
		{"other keys are ignored", write("b.json", `{"arm":"x","pages":[{"path":"docs/a.md","title":"A"},{"title":"no path"}]}`), []string{"docs/a.md"}, []string{""}, ""},
		{"an empty pages array", write("c.json", `{"pages":[]}`), nil, []string{"docs/a.md"}, ""},
		{"missing file", filepath.Join(dir, "absent.json"), nil, nil, "read outline"},
		{"not json", write("d.json", `pages: [docs/a.md]`), nil, nil, "d.json"},
		{"no pages array", write("e.json", `{"groups":[]}`), nil, nil, "no pages array"},
		{"pages is null", write("f.json", `{"pages":null}`), nil, nil, "no pages array"},
		{"pages is not an array", write("g.json", `{"pages":{"path":"docs/a.md"}}`), nil, nil, "g.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, err := LoadOutline(tt.file)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range tt.listed {
				if !o.lists(p) {
					t.Errorf("outline does not list %q", p)
				}
			}
			for _, p := range tt.unlisted {
				if o.lists(p) {
					t.Errorf("outline lists %q", p)
				}
			}
		})
	}
}

func TestLoadOutlines(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	first := write("first.json", `{"pages":[{"path":"docs/a.md"}]}`)
	second := write("second.json", `{"pages":[{"path":"docs/b.md"}]}`)
	o, err := LoadOutlines([]string{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if !o.lists("docs/a.md") || !o.lists("docs/b.md") || o.lists("docs/c.md") {
		t.Errorf("merged outline lists wrong: %v", o.paths)
	}
	bad := write("bad.json", `{"groups":[]}`)
	if _, err := LoadOutlines([]string{first, bad}); err == nil || !strings.Contains(err.Error(), bad) {
		t.Errorf("error = %v, want one naming %s", err, bad)
	}
}
