package plan

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// copyVerifyFixture copies testdata/verify into a temp directory and returns it. Nothing there
// is a git work tree, so the base name index walks the directory.
func copyVerifyFixture(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	src := filepath.Join("testdata", "verify")
	err := filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

// verifyLineOf returns the 1-based line of the fixture bullet that contains needle.
func verifyLineOf(t *testing.T, store, needle string) int {
	t.Helper()
	for i, line := range strings.Split(store, "\n") {
		if strings.HasPrefix(line, "- ") && strings.Contains(line, needle) {
			return i + 1
		}
	}
	t.Fatalf("no bullet contains %q", needle)
	return 0
}

func TestVerifyFactsStore(t *testing.T) {
	root := copyVerifyFixture(t)
	// A dot directory holds a second solo.go; the walk must leave it out.
	if err := os.MkdirAll(filepath.Join(root, ".hidden"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".hidden", "solo.go"), []byte("package hidden\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "facts", "store.md"))
	if err != nil {
		t.Fatal(err)
	}
	store := string(data)
	findings, checked, skipped, err := VerifyFacts(filepath.Join(root, "facts"), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	byLine := map[int][]Finding{}
	for _, f := range findings {
		if f.Check != "store" || f.File != "facts/store.md" {
			t.Errorf("finding %+v: want check store on facts/store.md", f)
		}
		byLine[f.Line] = append(byLine[f.Line], f)
	}

	tests := []struct {
		needle   string
		severity string // empty: the bullet passes or is skipped, and has no finding
		message  string
	}{
		{"`f:moved1`", "error", `names ["farWord"], not found within 10 lines`},
		{"`f:near01`", "warning", `"nearWord" on line 12`},
		{"`f:missng`", "error", `names ["nowhereWord"]`},
		{"The first bullet of a repeated id", "error", `fact "f:dupe01" is on 2 bullets (facts/store.md:`},
		{"The second bullet of a repeated id", "error", `fact "f:dupe01" is on 2 bullets (facts/store.md:`},
		{"`f:over01`", "error", "out of range (src/long.go has 31 lines)"},
		{"`f:nopath`", "error", "no file at src/gone.go"},
		{"`f:ambig1`", "error", "matches 2 files by name (src/dup/a/same.go, src/dup/b/same.go)"},
		{"`f:nosrc1`", "error", `no "Source:"`},
		{"`f:badtag`", "error", "outside the vocabulary"},
		{"`f:nodecl`", "error", "no declaration of Missing in src/doc.go"},
		{"`f:tsout1`", "error", `names ["tokenInOtherFunction"], not found in the declaration (lines 1-4)`},
		{"`f:goout1`", "error", `names ["job"], not found in the declaration`},
		{"`f:backwd`", "error", "runs backward"},
		{"`f:dirptr`", "error", "names a directory"},
		{"`f:badfile`", "error", "out of range (src/notes.md has 4 lines)"},
		{"A claim without an id", "error", "no leading fact id"},
		{"`f:prov01`", "error", "Provenance bullet carries the fact id"},
		{"`f:barewn`", "warning", `source "src/gone/" does not exist`},

		{"`f:edge01`", "", ""},
		{"`f:substr`", "", ""},
		{"`f:uniqbn`", "", ""},
		{"`f:gometh`", "", ""},
		{"`f:gogrp1`", "", ""},
		{"`f:tsfunc`", "", ""},
		{"`f:tsind1`", "", ""},
		{"`f:tsglob`", "", ""},
		{"`f:bareok`", "", ""},
		{"`f:multi1`", "", ""},
		{"`f:parens1`", "", ""},
		{"`f:dotted`", "", ""},
		{"`f:mdhead`", "", ""},
		{"`f:route1`", "", ""},
		{"`harvested-page.md`", "", ""},
	}
	for _, tt := range tests {
		t.Run(strings.Trim(tt.needle, "`"), func(t *testing.T) {
			got := byLine[verifyLineOf(t, store, tt.needle)]
			if tt.severity == "" {
				if len(got) != 0 {
					t.Errorf("findings = %+v, want none", got)
				}
				return
			}
			if len(got) != 1 || got[0].Severity != tt.severity || !strings.Contains(got[0].Message, tt.message) || got[0].Fix == "" {
				t.Errorf("findings = %+v, want one %s containing %q", got, tt.severity, tt.message)
			}
		})
	}
	failing := 0
	for _, tt := range tests {
		if tt.severity != "" {
			failing++
		}
	}
	if len(findings) != failing {
		t.Errorf("%d findings, want %d: %+v", len(findings), failing, findings)
	}
	if checked != 29 || skipped != 3 {
		t.Errorf("checked %d, skipped %d, want 29 and 3", checked, skipped)
	}
}

func TestVerifyFactsIDs(t *testing.T) {
	root := copyVerifyFixture(t)
	dir := filepath.Join(root, "facts")
	tests := []struct {
		name             string
		ids              []string
		findings         int
		checked, skipped int
	}{
		{"a repeated id selects both bullets", []string{"f:dupe01"}, 2, 2, 0},
		{"a clean fact", []string{"f:substr"}, 0, 1, 0},
		{"a skipped pointer", []string{"f:dotted", "f:mdhead"}, 0, 0, 2},
		{"an id in no bullet", []string{"f:absent"}, 0, 0, 0},
		{"no ids selects nothing", []string{}, 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings, checked, skipped, err := VerifyFacts(dir, root, tt.ids)
			if err != nil {
				t.Fatal(err)
			}
			if len(findings) != tt.findings || checked != tt.checked || skipped != tt.skipped {
				t.Errorf("findings %d, checked %d, skipped %d, want %d, %d, %d: %+v",
					len(findings), checked, skipped, tt.findings, tt.checked, tt.skipped, findings)
			}
		})
	}

	findings, _, _, err := VerifyFacts(dir, root, []string{"f:dupe01"})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range findings {
		if strings.Count(f.Message, "facts/store.md:") != 2 {
			t.Errorf("message %q does not name both places", f.Message)
		}
	}
}

func TestVerifyFactsGitIndex(t *testing.T) {
	root := copyVerifyFixture(t)
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	line := func(findings []Finding) []int {
		var lines []int
		for _, f := range findings {
			lines = append(lines, f.Line)
		}
		return lines
	}
	data, err := os.ReadFile(filepath.Join(root, "facts", "store.md"))
	if err != nil {
		t.Fatal(err)
	}
	ambiguous := verifyLineOf(t, string(data), "`f:ambig1`")

	findings, _, _, err := VerifyFacts(filepath.Join(root, "facts"), root, []string{"f:ambig1"})
	if err != nil || !slices.Equal(line(findings), []int{ambiguous}) {
		t.Fatalf("untracked files count: findings %+v, err %v", findings, err)
	}

	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("/src/dup/b/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	findings, _, _, err = VerifyFacts(filepath.Join(root, "facts"), root, []string{"f:ambig1"})
	if err != nil || len(findings) != 0 {
		t.Errorf("an ignored copy still counts: findings %+v, err %v", findings, err)
	}
}

func TestVerifyFactsUnreadableSource(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root reads any file")
	}
	root := copyVerifyFixture(t)
	file := filepath.Join(root, "src", "long.go")
	if err := os.Chmod(file, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(file, 0o644) })
	findings, _, _, err := VerifyFacts(filepath.Join(root, "facts"), root, []string{"f:near01"})
	if err != nil || len(findings) != 1 || !strings.Contains(findings[0].Message, "cannot be read") {
		t.Errorf("findings %+v, err %v, want one cannot-be-read error", findings, err)
	}
}

func TestVerifyFactsMissingStore(t *testing.T) {
	root := t.TempDir()
	if _, _, _, err := VerifyFacts(filepath.Join(root, "facts"), root, nil); err == nil {
		t.Error("a missing facts directory returned no error")
	}
}

func TestDeclarationSpan(t *testing.T) {
	tests := []struct {
		name, lang, symbol string
		lines              []string
		start, end         int
		ok                 bool
	}{
		{"go func to the next declaration", "go", "A", []string{"package p", "func A() {", "}", "", "func B() {}"}, 1, 4, true},
		{"go method", "go", "M", []string{"func (t *T) M() {", "}"}, 0, 2, true},
		{"go grouped var", "go", "x", []string{"var (", "\tx = 1", "\ty = 2", ")", "func F() {}"}, 1, 4, true},
		{"go prefix is not a match", "go", "Run", []string{"func Runner() {}"}, 0, 0, false},
		{"ts class", "ts", "C", []string{"export abstract class C {", "}", "export const D = 1;"}, 0, 2, true},
		{"ts const enum", "ts", "E", []string{"export const enum E { A }"}, 0, 1, true},
		{"ts dollar name", "ts", "$store", []string{"export const $store = 1;", "export const other = 2;"}, 0, 1, true},
		{"ts indented only when no column 0 matches", "ts", "x", []string{"  const x = 1;", "const x = 2;"}, 1, 2, true},
		{"ts indented fallback", "ts", "x", []string{"it('a', () => {", "  const x = 1;", "});", "declare global {", "}"}, 1, 3, true},
		{"ts mention is not a declaration", "ts", "x", []string{"call(x);"}, 0, 0, false},
		{"other language", "py", "x", []string{"x = 1"}, 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, ok := declarationSpan(tt.lines, tt.lang, tt.symbol)
			if ok != tt.ok || (ok && (start != tt.start || end != tt.end)) {
				t.Errorf("declarationSpan = (%d, %d, %v), want (%d, %d, %v)", start, end, ok, tt.start, tt.end, tt.ok)
			}
		})
	}
}

func TestParsePointer(t *testing.T) {
	tests := []struct {
		span string
		ok   bool
		kind pointerKind
		lo   int
		hi   int
	}{
		{"src/a.go", true, pointerBare, 0, 0},
		{"src/a.go:3", true, pointerLines, 3, 3},
		{"src/a.go:3,10-12", true, pointerLines, 3, 12},
		{"src/a.go#Name", true, pointerSymbol, 0, 0},
		{"https://example.com/a.go:3", false, 0, 0, 0},
		{"go test ./...", false, 0, 0, 0},
		{"../outside.go:1", false, 0, 0, 0},
		{"/abs/a.go:1", false, 0, 0, 0},
		{"a.go:one", false, 0, 0, 0},
		{"f:abc123", false, 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.span, func(t *testing.T) {
			p, ok := parsePointer(tt.span)
			if ok != tt.ok || (ok && (p.kind != tt.kind || (p.kind == pointerLines && (p.lo != tt.lo || p.hi != tt.hi)))) {
				t.Errorf("parsePointer(%q) = %+v, %v", tt.span, p, ok)
			}
		})
	}
}
