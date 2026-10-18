package plan

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestCheckPageLinks(t *testing.T) {
	files := map[string]string{
		"docs/other.md":        "# Other page\n\n## Second heading\n",
		"docs/sub/deep.md":     "# Deep\n",
		"docs/a b.md":          "# Spaced\n",
		"README.md":            "# Root readme\n",
		"docs/img/diagram.png": "x",
	}
	tests := []struct {
		name string
		page string
		want []string // "line: message fragment" per finding
	}{
		{"no links", "# Page\n\nText.\n", nil},
		{"valid relative link and anchor", "# Page\n\nSee [other](other.md#second-heading).\n", nil},
		{"valid root link", "# Page\n\nSee [readme](/README.md#root-readme).\n", nil},
		{"url schemes are not checked", "# Page\n\n[a](https://example.com/x.md#nope) [b](mailto:a@example.com) [c](//example.com/y)\n", nil},
		{"bare anchor matches a heading", "# Page\n\n## Steps\n\n[s](#steps)\n", nil},
		{"percent-encoded path", "# Page\n\n[s](a%20b.md)\n", nil},
		{"existing image", "# Page\n\n![d](img/diagram.png)\n", nil},
		{"link to a directory", "# Page\n\n[d](sub/)\n", nil},
		{"missing file", "# Page\n\nText\n\n[x](missing.md)\n", []string{"5: target docs/missing.md does not exist"}},
		{"missing image", "# Page\n\n![x](img/none.png)\n", []string{"3: target docs/img/none.png does not exist"}},
		{"anchor matching no heading in the target", "# Page\n\n[x](other.md#nope)\n", []string{"3: anchor #nope matches no heading in docs/other.md"}},
		{"bare anchor matching no heading", "# Page\n\n[x](#nope)\n", []string{"3: anchor #nope matches no heading on this page"}},
		{"leaves the repository", "# Page\n\n[x](../../outside.md)\n", []string{"3: target ../../outside.md leaves the repository"}},
		{"anchor on a directory", "# Page\n\n[x](sub/#top)\n", []string{"3: target docs/sub is a directory"}},
		{"link inside a code fence is not a link", "# Page\n\n```md\n[x](missing.md)\n```\n", nil},
		{"front matter shifts nothing", "---\ntitle: T\n---\n# Page\n\n[x](missing.md)\n", []string{"6: target docs/missing.md does not exist"}},
		{"link in a list item", "# Page\n\n- one\n- [x](missing.md)\n", []string{"4: target docs/missing.md does not exist"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for name, body := range files {
				writePageLinksFile(t, root, name, body)
			}
			writePageLinksFile(t, root, "docs/page.md", tt.page)
			got, err := CheckPageLinks(root, "docs/page.md")
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d findings %+v, want %d", len(got), got, len(tt.want))
			}
			for i, f := range got {
				line, frag, _ := strings.Cut(tt.want[i], ": ")
				if f.File != "docs/page.md" || f.Check != "links" || f.Severity != SeverityError ||
					strconv.Itoa(f.Line) != line || !strings.Contains(f.Message, frag) {
					t.Errorf("finding %d = %+v, want line %s with %q", i, f, line, frag)
				}
			}
		})
	}
}

func TestCheckPageLinksMissingPage(t *testing.T) {
	if _, err := CheckPageLinks(t.TempDir(), "docs/none.md"); err == nil {
		t.Error("a missing page gave no error")
	}
}

func writePageLinksFile(t *testing.T, root, name, body string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
