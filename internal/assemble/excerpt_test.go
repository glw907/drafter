package assemble

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/glw907/drafter/internal/plan"
)

func loadFacts(t *testing.T, f fixture) *plan.Facts {
	t.Helper()
	fs, err := plan.LoadFacts(filepath.Join(f.root, "facts"))
	if err != nil {
		t.Fatal(err)
	}
	return fs
}

func TestExcerpt(t *testing.T) {
	f := newFixture(t)
	r, err := openRepo(context.Background(), f.config())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		source string
		in     []string
		out    []string
	}{
		{"line pointer takes ten lines each way", "src/greet.go:25", []string{"lines 15-35", "func (g *Greeter) Hello()", "// line 31"}, []string{"// line 36", "package greet"}},
		{"span takes five lines each way", "src/greet.go:23-26", []string{"lines 18-31", "func (g *Greeter) Hello()"}, []string{"// line 32"}},
		{"span clamps at the top", "src/greet.go:3-4", []string{"lines 1-9"}, nil},
		{"range past the end clamps", "src/greet.go:45-60", []string{"lines 40-50", "// line 50"}, nil},
		{"comma list yields each", "src/greet.go:3,45", []string{"lines 1-13", "lines 35-50"}, nil},
		{"range entirely past the end", "src/greet.go:200", []string{"no excerpt: range beyond the end of the file"}, nil},
		{"bare file inlined whole", "src/panel.ts", []string{"=== file: src/panel.ts ===", "toggle(): void"}, nil},
		{"directory", "src", []string{"no excerpt: directory"}, nil},
		{"prose source", "docs/guide.md:3", []string{"no excerpt: prose"}, []string{"## Setup"}},
		{"prose source by declaration", "docs/guide.md#Setup", []string{"no excerpt: prose"}, nil},
		{"missing path", "src/nope.go:3", []string{"no excerpt: missing"}, nil},
		{"untracked path", "untracked.txt:1", []string{"no excerpt: untracked"}, []string{"UNTRACKED-CONTENT"}},
		{"path that climbs out of the root", "../outside/secret.txt:1", []string{"no excerpt: untracked"}, []string{"OUTSIDE-SECRET"}},
		{"symlink to a file outside the root", "docs/escape.md:1", []string{"no excerpt: untracked"}, []string{"OUTSIDE-SECRET"}},
		{"Go function declaration, not the mention", "src/greet.go#Shout", []string{"lines 22-50", "func Shout(s string)"}, []string{"Greeting is mentioned"}},
		{"Go method", "src/greet.go#Hello", []string{"lines 18-50", "func (g *Greeter) Hello() string"}, nil},
		{"Go method by dotted name", "src/greet.go#Greeter.Hello", []string{"func (g *Greeter) Hello() string"}, nil},
		{"Go type", "src/greet.go#Greeter", []string{"type Greeter struct"}, nil},
		{"Go const and var", "src/greet.go#Suffix", []string{"var Suffix"}, nil},
		{"imported name excerpt holds the declaration", "src/widget.ts#render", []string{"lines 22-35", "export function render(input: string)"}, []string{"import { render }"}},
		{"export default class", "src/panel.ts#Panel", []string{"export default class Panel"}, nil},
		{"export default async function", "src/widget.ts#load", []string{"export default async function load()"}, nil},
		{"interface", "src/widget.ts#Options", []string{"export interface Options"}, nil},
		{"name with no declaration", "src/widget.ts#unrelatedMention", []string{"no excerpt: declaration not found"}, nil},
		{"name only mentioned in a comment", "src/greet.go#Greeting", []string{"no excerpt: declaration not found"}, nil},
		{"name in a grouped const", "src/group.go#Alpha", []string{"\tAlpha = 1"}, nil},
		{"name in a grouped var", "src/group.go#Gamma", []string{"\tGamma string"}, nil},
		{"name in a grouped type", "src/group.go#Zeta", []string{"\tZeta int"}, nil},
		{"name only mentioned in a group and a function body", "src/group.go#Mention", []string{"no excerpt: declaration not found"}, nil},
		{"field nested in a grouped type", "src/group.go#Inner", []string{"no excerpt: declaration not found"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.Join(r.excerpt("f:x", tt.source), "\n")
			for _, s := range tt.in {
				if !strings.Contains(got, s) {
					t.Errorf("excerpt of %s lacks %q:\n%s", tt.source, s, got)
				}
			}
			for _, s := range tt.out {
				if strings.Contains(got, s) {
					t.Errorf("excerpt of %s holds %q:\n%s", tt.source, s, got)
				}
			}
		})
	}
}

func TestExcerptBareSpecialFile(t *testing.T) {
	f := newFixture(t)
	fifo := filepath.Join(f.root, "src", "widget.ts")
	if err := os.Remove(fifo); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := openRepo(context.Background(), f.config())
	if err != nil {
		t.Fatal(err)
	}
	got := r.excerpt("f:x", "src/widget.ts")
	if len(got) != 1 || !strings.Contains(got[0], "no excerpt: not a regular file") {
		t.Errorf("excerpt = %q, want one block reading no excerpt: not a regular file", got)
	}
}

func TestExcerpterFact(t *testing.T) {
	f := newFixture(t)
	e, err := NewExcerpter(context.Background(), f.config())
	if err != nil {
		t.Fatal(err)
	}
	facts := plan.ParseFacts(map[string]string{"a.md": strings.Join([]string{
		"- `f:dir` A directory only. Source: `src`. [verified]",
		"- `f:line` A line in a code file. Source: `src/greet.go:25`. [verified]",
		"- `f:mixed` A directory and a line. Source: `src`, `src/greet.go:25`. [verified]",
		"- `f:none` No source part. [verified]",
	}, "\n")}, f.root)
	tests := []struct {
		id     string
		blocks int
		real   bool
	}{
		{"f:dir", 1, false},
		{"f:line", 1, true},
		{"f:mixed", 2, true},
		{"f:none", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			blocks, real, err := e.Fact(facts, tt.id)
			if err != nil {
				t.Fatal(err)
			}
			if len(blocks) != tt.blocks || real != tt.real {
				t.Errorf("Fact(%s) = %d blocks, real %v; want %d, %v:\n%s", tt.id, len(blocks), real, tt.blocks, tt.real, strings.Join(blocks, "\n"))
			}
		})
	}
	t.Run("the blocks are the ones a request carries", func(t *testing.T) {
		blocks, _, err := e.Fact(facts, "f:mixed")
		if err != nil {
			t.Fatal(err)
		}
		want, err := e.r.excerptBlocks(facts, []string{"f:mixed"})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(blocks, want) {
			t.Errorf("Fact blocks differ from excerptBlocks:\n%q\n%q", blocks, want)
		}
	})
	t.Run("an unknown id is an error", func(t *testing.T) {
		if _, _, err := e.Fact(facts, "f:absent"); err == nil {
			t.Error("Fact(f:absent) returned no error")
		}
	})
}

func TestBuildWithFacts(t *testing.T) {
	f := newFixture(t)
	in := f.input(f.config(), "Draft the page.")
	in.Facts = loadFacts(t, f)

	in.FactIDs = []string{"f:greet", "f:render", "f:other"}
	got := requestText(t, in)
	for _, want := range []string{
		"=== excerpt: src/greet.go#Hello (fact f:greet) lines 18-50 ===",
		"=== excerpt: src/widget.ts#render (fact f:render) lines 22-35 ===",
		"=== excerpt: src/widget.ts:7 (fact f:render) lines 1-17 ===",
		"=== excerpt: src/panel.ts#Panel (fact f:other) lines 1-8 ===",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("request lacks %q", want)
		}
	}
	if strings.Contains(got, "ignored.ts") || strings.Contains(got, "example.com") {
		t.Errorf("request excerpts a commentary or URL span")
	}

	t.Run("an id held by two bullets is refused", func(t *testing.T) {
		in.FactIDs = []string{"f:dup"}
		if _, err := Build(context.Background(), in); err == nil || !strings.Contains(err.Error(), "f:dup") {
			t.Errorf("error = %v, want one naming f:dup", err)
		}
	})
	t.Run("no excerpts without a facts store", func(t *testing.T) {
		in.Facts, in.FactIDs = nil, []string{"f:greet"}
		if got := requestText(t, in); strings.Contains(got, "=== excerpt:") {
			t.Errorf("request holds an excerpt with no facts store")
		}
	})
}
