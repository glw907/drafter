package plan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFactsCites(t *testing.T) {
	dir := t.TempDir()
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name  string
		facts string
		token string
		want  bool
	}{
		{"an id the store holds", "- `f:aaaaaa` One. [verified]\n", "f:aaaaaa", true},
		{"a mistyped id with the shared prefix", "- `f:aaaaaa` One. [verified]\n- `f:bbbbbb` Two. [verified]\n", "f:zzzzzz", true},
		{"code that is no id", "- `f:aaaaaa` One. [verified]\n", "--key", false},
		{"the bare prefix", "- `f:aaaaaa` One. [verified]\n", "f:", false},
		{"ids with no shared prefix", "- `f:aaaaaa` One. [verified]\n- `g:bbbbbb` Two. [verified]\n", "f:zzzzzz", false},
		{"ids with no colon", "- `aaaaaa` One. [verified]\n", "zzzzzz", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			write(tt.facts)
			fs, err := LoadFacts(dir)
			if err != nil {
				t.Fatal(err)
			}
			if got := fs.Cites(tt.token); got != tt.want {
				t.Errorf("Cites(%q) = %v, want %v", tt.token, got, tt.want)
			}
		})
	}

	write("- `f:aaaaaa` One. [verified]\n- `f:aaaaaa` Again. [verified]\n")
	fs, err := LoadFacts(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := fs.Get("f:aaaaaa"); ok || fs.Count("f:aaaaaa") != 2 || fs.Count("f:none") != 0 {
		t.Errorf("a doubled id: Get hides it, and Count = %d, %d; want 2, 0", fs.Count("f:aaaaaa"), fs.Count("f:none"))
	}
}
