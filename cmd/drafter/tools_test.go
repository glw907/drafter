package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/plan"
)

func TestCheckFactVerifiesOnlyTheNamedFacts(t *testing.T) {
	// f:bbbbbb's pointer is broken; f:cccccc's is sound. The store check alone reports the first.
	broken := strings.Replace(readCatalog(t, "facts/facts.md"), verifyStoreRotateSource, "Source: `src/gone.go:3`. [verified]", 1)
	root := project(t, map[string]string{"blueprints/facts/facts.md": broken})
	tests := []struct {
		name     string
		args     []string
		code     int
		want     string // a substring of stdout
		stderr   string // a substring of stderr
		wantNone string // a substring stdout must lack
	}{
		{"a broken fact", []string{"--fact", "f:bbbbbb"}, 1, "src/gone.go", "facts store: ", ""},
		{"the id without its prefix", []string{"--fact", "bbbbbb"}, 1, "src/gone.go", "", ""},
		{"a sound fact", []string{"--fact", "f:cccccc"}, 0, "", "0 errors, 0 warnings", "src/gone.go"},
		{"several facts", []string{"--fact", "f:cccccc", "--fact", "f:bbbbbb"}, 1, "src/gone.go", "", ""},
		{"an id the store lacks", []string{"--fact", "f:nothere"}, 1, "fact f:nothere is not in the facts store", "", "src/gone.go"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, errs, code := run(t, append([]string{"check", "--root", root}, tt.args...)...)
			if code != tt.code || !strings.Contains(out, tt.want) || !strings.Contains(errs, tt.stderr) {
				t.Errorf("code %d, stdout %q, stderr %q; want code %d, stdout containing %q, stderr containing %q", code, out, errs, tt.code, tt.want, tt.stderr)
			}
			if tt.wantNone != "" && strings.Contains(out, tt.wantNone) {
				t.Errorf("stdout %q holds %q", out, tt.wantNone)
			}
		})
	}

	t.Run("no blueprint check runs", func(t *testing.T) {
		bad := project(t, map[string]string{"blueprints/docs/guide.blueprint.yaml": "not: a blueprint\n"})
		out, errs, code := run(t, "check", "--root", bad, "--fact", "f:cccccc")
		if code != 0 || out != "" {
			t.Errorf("code %d, stdout %q, stderr %q; want a clean fact-only run", code, out, errs)
		}
		if _, _, code := run(t, "check", "--root", bad); code != 1 {
			t.Errorf("the whole check passed a malformed blueprint: code %d", code)
		}
	})
	t.Run("a named blueprint is a usage error", func(t *testing.T) {
		if _, errs, code := run(t, "check", "--root", root, "--fact", "f:cccccc", "blueprints/docs/guide.blueprint.yaml"); code != 2 || !strings.Contains(errs, "takes no blueprint") {
			t.Errorf("code %d, stderr %q", code, errs)
		}
	})
	t.Run("json", func(t *testing.T) {
		out, _, code := run(t, "check", "--root", root, "--json", "--fact", "f:nothere")
		var fs []plan.Finding
		if err := json.Unmarshal([]byte(out), &fs); err != nil || code != 1 || len(fs) != 1 || fs[0].Check != "store" {
			t.Errorf("json %q, code %d, err %v", out, code, err)
		}
	})
}

func TestOutline(t *testing.T) {
	root := project(t, nil)
	t.Run("yaml by path", func(t *testing.T) {
		out, _, code := run(t, "outline", "--root", root, "docs/guide.md")
		if code != 0 || !strings.Contains(out, "path: docs/guide.md\n") || !strings.Contains(out, "title: Rotate the widget key\n") || !strings.Contains(out, "type: task guide\n") {
			t.Errorf("code %d, stdout %q", code, out)
		}
	})
	t.Run("json by slug", func(t *testing.T) {
		out, _, code := run(t, "outline", "--root", root, "--json", "guide")
		var entry map[string]any
		if err := json.Unmarshal([]byte(out), &entry); err != nil {
			t.Fatalf("stdout %q: %v", out, err)
		}
		if code != 0 || entry["path"] != "docs/guide.md" || entry["type"] != "task guide" || entry["managed"] != true {
			t.Errorf("code %d, entry %v", code, entry)
		}
		if _, ok := entry["covers"].([]any); !ok {
			t.Errorf("covers = %v, want a list", entry["covers"])
		}
	})
	t.Run("a page the outline lacks", func(t *testing.T) {
		if _, errs, code := run(t, "outline", "--root", root, "docs/other.md"); code != 1 || !strings.Contains(errs, "not a slug or path the outline lists") {
			t.Errorf("code %d, stderr %q", code, errs)
		}
	})
	t.Run("no argument", func(t *testing.T) {
		if _, errs, code := run(t, "outline", "--root", root); code != 2 || !strings.Contains(errs, "exactly one slug or page path") {
			t.Errorf("code %d, stderr %q", code, errs)
		}
	})
}

func TestVersionFlag(t *testing.T) {
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"--version"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if want := "drafter version " + version() + "\n"; out.String() != want {
		t.Errorf("--version printed %q, want %q", out.String(), want)
	}
}

func TestExcerptURLChecklist(t *testing.T) {
	tests := []struct {
		raw  string
		want []string
	}{
		{"`f:a` A claim. Source: https://example.com/docs/keys, read 2026-10-01. [external: Example]",
			[]string{"- [ ] https://example.com/docs/keys (read 2026-10-01)"}},
		{"`f:a` A claim. Source: `https://example.com/a` read 2026-09-30, https://example.com/b. [vendor]",
			[]string{"- [ ] https://example.com/a (read 2026-09-30)", "- [ ] https://example.com/b (no read date)"}},
		{"`f:a` A claim. Source: `src/main.go`. [verified]", nil},
	}
	for _, tt := range tests {
		got := urlChecklist(tt.raw)
		if strings.Join(got, "|") != strings.Join(tt.want, "|") {
			t.Errorf("urlChecklist(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestExcerptVendorProseAndLength(t *testing.T) {
	f := newCLIFixture(t)
	big := "package big\n" + strings.Repeat("// line of text\n", 300)
	f.write("big.go", big)
	f.write("notes.md", "A statement of the owner.\n")
	facts := f.read1(tallyFacts) +
		"- `tally-vendor` Tally runs anywhere. Source: https://example.com/tally, read 2026-10-02. [vendor: Example]\n" +
		"- `tally-prose` Tally was designed for speed. Source: `notes.md:1`. [verified]\n"
	f.write(tallyFacts, facts)
	f.commit("add a vendor fact, a prose fact, and a long file", tallyFacts, "big.go", "notes.md")

	t.Run("a URL-sourced fact prints a checklist and exits 0", func(t *testing.T) {
		out, errs, code := run(t, "excerpt", "--root", f.root, "tally-vendor")
		if code != 0 || !strings.Contains(out, "- [ ] https://example.com/tally (read 2026-10-02)") {
			t.Errorf("code %d, stdout %q, stderr %q", code, out, errs)
		}
	})
	t.Run("a prose-sourced fact says to verify by hand", func(t *testing.T) {
		_, errs, code := run(t, "excerpt", "--root", f.root, "tally-prose")
		if code != 1 || !strings.Contains(errs, "verify the claim by hand, or drop it where the project's rules exclude prose sources") {
			t.Errorf("code %d, stderr %q", code, errs)
		}
	})
	t.Run("a long excerpt stops at 200 lines unless --full", func(t *testing.T) {
		out, _, code := run(t, "excerpt", "--root", f.root, "big.go")
		lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
		if code != 0 || len(lines) != 201 || !strings.HasPrefix(lines[200], "... (") || !strings.Contains(lines[200], "more lines; --full prints them") {
			t.Fatalf("code %d, %d lines, last %q", code, len(lines), lines[len(lines)-1])
		}
		full, _, code := run(t, "excerpt", "--root", f.root, "--full", "big.go")
		if code != 0 || strings.Contains(full, "--full prints them") || strings.Count(full, "line of text") < 300 {
			t.Errorf("--full: code %d, %d lines of text", code, strings.Count(full, "line of text"))
		}
	})
}
