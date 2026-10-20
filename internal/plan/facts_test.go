package plan

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestFactsIDs(t *testing.T) {
	fs := ParseFacts(map[string]string{"facts.md": "- `f:b` B. [verified]\n- `f:a` A. [verified]\n- `f:b` B again. [verified]\n"}, "/nowhere")
	if got := fs.IDs(); !slices.Equal(got, []string{"f:a", "f:b"}) {
		t.Errorf("IDs = %v, want [f:a f:b]", got)
	}
	var none *Facts
	if got := none.IDs(); len(got) != 0 {
		t.Errorf("IDs of a nil store = %v, want none", got)
	}
}

func TestCheckTag(t *testing.T) {
	tests := []struct {
		name, text, tag, problem string
	}{
		{"plain tag", "A claim. Source: x. [verified]", "verified", ""},
		{"qualified tag", "A claim. [candidate: sourced to a page only]", "candidate", ""},
		{"trailing space after the tag", "A claim. [external: vendor]  ", "external", ""},
		{"bracket in a code span", "A claim about `deploy-bot[bot]` and `['a', 'b']`. [verified]", "verified", ""},
		{"bracket in a quoted span", `A claim quoting "[...path]" here. [vendor: link]`, "vendor", ""},
		{"quoted text inside the tag", `A claim. [docs-drift: page says "two hours"]`, "docs-drift", ""},
		{"rejected", "A claim. [rejected: false]", "rejected", ""},
		{"no tag", "A claim. Source: x.", "", "no status tag"},
		{"outside the vocabulary", "A claim. [maybe: unsure]", "", "outside the vocabulary"},
		{"space qualifier", "A claim. [verified checked]", "", "space qualifier"},
		{"two tags", "A claim. [verified] [external: both]", "", "2 status tags"},
		{"tag not last", "A claim. [verified] and more.", "", "not the last thing"},
		{"bracket before the tag", "A claim [note] then [verified]", "", "second bracket"},
		{"a tag word inside code is no tag", "A claim about `[verified]`.", "", "no status tag"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tag, problem := checkTag(tt.text)
			if tag != tt.tag || (tt.problem == "") != (problem == "") || !strings.Contains(problem, tt.problem) {
				t.Errorf("checkTag = (%q, %q), want tag %q and a problem containing %q", tag, problem, tt.tag, tt.problem)
			}
		})
	}
}

func TestExtractBullets(t *testing.T) {
	md := "# Title\n\n- before any section\n\n## First\n\n- one\n  wrapped  \n- two\n\n  stray indented line\n" +
		"* not a bullet\n## Second\n- three\n  four\n##Not a heading\n"
	want := []bullet{
		{"", "before any section", 3},
		{"First", "one wrapped", 7},
		{"First", "two", 9},
		{"Second", "three four ##Not a heading", 14},
	}
	got := extractBullets(md)
	if len(got) != len(want) {
		t.Fatalf("bullets = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("bullet %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestLoadFacts(t *testing.T) {
	fs, err := LoadFacts("testdata/repo/docs/facts")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		id           string
		n            int
		file         string
		line         int
		tag, problem string
		citable      bool
	}{
		{"signing-key-ttl", 1, "keys.md", 5, "verified", "", true},
		{"old-key-grace", 1, "keys.md", 6, "external", "", true},
		{"vendor-fact", 1, "keys.md", 10, "vendor", "", true},
		{"legacy-format", 1, "keys.md", 8, "rejected", "", false},
		{"wrapped-tag", 1, "edge.md", 5, "external", "", true},
		{"code-bracket", 1, "edge.md", 7, "verified", "", true},
		{"quoted-bracket", 1, "edge.md", 9, "verified", "", true},
		{"cand-excluded", 1, "edge.md", 13, "candidate", "", false},
		{"drift-fact", 1, "edge.md", 15, "docs-drift", "", false},
		{"no-tag", 1, "edge.md", 17, "", "no status tag", false},
		{"bad-tag", 1, "edge.md", 18, "", "outside the vocabulary", false},
		{"dup-id", 2, "edge.md", 21, "verified", "", true},
		{"provenance-only", 0, "", 0, "", "", false},
		{"readme-only", 0, "", 0, "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := fs.byID[tt.id]
			if len(got) != tt.n {
				t.Fatalf("%d bullets, want %d: %+v", len(got), tt.n, got)
			}
			if tt.n == 0 {
				return
			}
			f := got[0]
			if f.File != tt.file || f.Line != tt.line || f.Tag != tt.tag || !strings.Contains(f.Problem, tt.problem) || f.Citable() != tt.citable {
				t.Errorf("fact = %+v, want %s:%d tag %q problem %q citable %v", f, tt.file, tt.line, tt.tag, tt.problem, tt.citable)
			}
		})
	}
	if got := fs.byID["dup-id"]; got[1].Line != 22 {
		t.Errorf("second dup-id bullet at line %d, want 22", got[1].Line)
	}
}

func TestCheckFactsDir(t *testing.T) {
	onlyReadme := t.TempDir()
	if err := os.WriteFile(filepath.Join(onlyReadme, "README.md"), []byte("- `x` [verified]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	notMarkdown := t.TempDir()
	if err := os.WriteFile(filepath.Join(notMarkdown, "facts.txt"), []byte("- `x` [verified]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, dir, want string
	}{
		{"a facts directory", "testdata/repo/docs/facts", ""},
		{"missing", filepath.Join(t.TempDir(), "absent"), "facts directory"},
		{"a file", "testdata/repo/docs/facts/keys.md", "not a directory"},
		{"only a README", onlyReadme, "holds no facts file"},
		{"no markdown file", notMarkdown, "holds no facts file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckFactsDir(tt.dir)
			if tt.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want one containing %q", err, tt.want)
			}
		})
	}
}

func TestFactRaw(t *testing.T) {
	md := "## Facts\n\n- `f:wrap` The key  rotates\n  every 90 days. Source: `src/key.ts:12-18`, `src/key.ts#rotate`.\n    [verified: 2026-10-01]\n"
	f, ok := ParseFacts(map[string]string{"a.md": md}, "/nowhere").Get("f:wrap")
	if !ok {
		t.Fatal("Get(f:wrap) = false")
	}
	want := "`f:wrap` The key rotates every 90 days. Source: `src/key.ts:12-18`, `src/key.ts#rotate`. [verified: 2026-10-01]"
	if f.Raw != want {
		t.Errorf("Raw = %q, want %q", f.Raw, want)
	}
	if f.Tag != "verified" {
		t.Errorf("Tag = %q, want verified", f.Tag)
	}
}

func TestFactSourcesAndGet(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "docs", "facts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	md := strings.Join([]string{
		"- `f:one` The widget retries twice. Source: `src/x.ts:100-110,163-166`; `src/y.go#Retry`. [verified]",
		"- `f:two` Wrapped claim. Source: `src/z.ts:7` (see `src/ignored.ts:1` and `npm run check`), `npm run build`,",
		"  `https://example.com/page`, `package.json`, `bare`. [external: Source: https://example.com `src/out.ts:2`]",
		"- `f:dup` First holder. [verified]",
		"- `f:dup` Second holder. [verified]",
		"- `f:bare` No source part here. [verified]",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	fs, err := LoadFacts(dir)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, id, text string
		sources        []string
	}{
		{"comma list of ranges and a declaration", "f:one", "The widget retries twice.", []string{"src/x.ts:100-110,163-166", "src/y.go#Retry"}},
		{"parentheses, commands, URL, bare name, external part", "f:two", "Wrapped claim.", []string{"src/z.ts:7", "package.json"}},
		{"no source part", "f:bare", "No source part here.", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, ok := fs.Get(tt.id)
			if !ok {
				t.Fatalf("Get(%q) = false", tt.id)
			}
			if f.Text != tt.text {
				t.Errorf("Text = %q, want %q", f.Text, tt.text)
			}
			if !slices.Equal(f.Sources, tt.sources) {
				t.Errorf("Sources = %q, want %q", f.Sources, tt.sources)
			}
		})
	}
	for _, id := range []string{"f:dup", "f:absent"} {
		if _, ok := fs.Get(id); ok {
			t.Errorf("Get(%q) = true, want false", id)
		}
	}
}
