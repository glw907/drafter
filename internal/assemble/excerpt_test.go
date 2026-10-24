package assemble

import (
	"context"
	"fmt"
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

// numbered returns a file of n lines, each "// line K".
func numbered(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "// line %d\n", i)
	}
	return b.String()
}

// addTracked writes a file into the fixture and tracks it.
func (f fixture) addTracked(t *testing.T, rel, text string) {
	t.Helper()
	f.write(t, filepath.Join(f.root, rel), text)
	f.git(t, "add", rel)
}

func TestExcerpt(t *testing.T) {
	f := newFixture(t)
	f.addTracked(t, "src/big.go", numbered(400))
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
		{"line pointer takes ten lines each way", "src/big.go:25", []string{"lines 15-35", "// line 31"}, []string{"// line 36", "// line 14"}},
		{"span takes five lines each way", "src/big.go:23-26", []string{"lines 18-31"}, []string{"// line 32"}},
		{"span clamps at the top", "src/big.go:3-4", []string{"lines 1-9"}, nil},
		{"range past the end clamps", "src/big.go:395-410", []string{"lines 390-400", "// line 400"}, nil},
		{"comma list yields each", "src/big.go:3,45", []string{"lines 1-13", "lines 35-55"}, nil},
		{"range entirely past the end", "src/greet.go:200", []string{"no excerpt: range beyond the end of the file"}, nil},
		{"bare file inlined whole", "src/panel.ts", []string{"=== file: src/panel.ts ===", "toggle(): void"}, nil},
		{"directory", "src", []string{"no excerpt: directory"}, nil},
		{"prose source", "docs/guide.md:3", []string{"no excerpt: prose"}, []string{"## Setup"}},
		{"prose source by declaration", "docs/guide.md#Setup", []string{"no excerpt: prose"}, nil},
		{"missing path", "src/nope.go:3", []string{"no excerpt: missing"}, nil},
		{"untracked path", "untracked.txt:1", []string{"no excerpt: untracked"}, []string{"UNTRACKED-CONTENT"}},
		{"path that climbs out of the root", "../outside/secret.txt:1", []string{"no excerpt: untracked"}, []string{"OUTSIDE-SECRET"}},
		{"symlink to a file outside the root", "docs/escape.md:1", []string{"no excerpt: untracked"}, []string{"OUTSIDE-SECRET"}},
		{"Go function declaration, not the mention", "src/greet.go#Shout", []string{"lines 1-50", "func Shout(s string)"}, nil},
		{"Go method", "src/greet.go#Hello", []string{"lines 1-50", "func (g *Greeter) Hello() string"}, nil},
		{"Go method by dotted name", "src/greet.go#Greeter.Hello", []string{"func (g *Greeter) Hello() string"}, nil},
		{"Go type", "src/greet.go#Greeter", []string{"type Greeter struct"}, nil},
		{"Go const and var", "src/greet.go#Suffix", []string{"var Suffix"}, nil},
		{"imported name excerpt holds the declaration", "src/widget.ts#render", []string{"lines 1-35", "export function render(input: string)"}, nil},
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
			got := strings.Join(r.excerpt("f:x", tt.source, map[string]bool{}), "\n")
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
	got := r.excerpt("f:x", "src/widget.ts", map[string]bool{})
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
		"=== excerpt: src/greet.go (fact f:greet) lines 1-50 ===",
		"=== excerpt: src/widget.ts (fact f:render) lines 1-35 ===",
		"=== excerpt: src/panel.ts (fact f:other) lines 1-8 ===",
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

// tsFixture is a TypeScript file over the whole-file limit, so every pointer into it gets a
// declaration span. pad lines of filler separate the declarations.
func tsFixture(pad int) string {
	filler := strings.Repeat("// filler\n", pad)
	body60 := strings.Repeat("  step();\n", 60)
	body250 := strings.Repeat("  step();\n", 250)
	return "export class Panel {\n" +
		"  open = false;\n\n" +
		"  /** Flips the panel. */\n" +
		"  toggle(): void {\n    this.open = !this.open;\n  }\n\n" +
		"  readonly opts = {\n    size: 4,\n    label: 'x',\n  };\n" +
		"}\n\n" + filler +
		"export function outer(a: number): number {\n" +
		"  function helper(b: number): number {\n    return b + 1;\n  }\n" +
		"  const local = helper(a);\n  return local;\n}\n\n" +
		"export const a = {\n  b: {\n    c: {\n      deep: 1,\n    },\n  },\n};\n\n" +
		"/**\n * Documented.\n */\nexport function documented(): void {\n" + body60 + "}\n\n" +
		"export function huge(): void {\n" + body250 + "}\n"
}

func TestExcerptAnchors(t *testing.T) {
	f := newFixture(t)
	f.addTracked(t, "src/big.ts", tsFixture(150))
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
		{"method of a class", "src/big.ts#Panel.toggle", []string{"/** Flips the panel. */", "toggle(): void {", "this.open = !this.open;"}, []string{"export class Panel", "readonly opts"}},
		{"object property", "src/big.ts#Panel.opts", []string{"readonly opts = {", "label: 'x',"}, []string{"toggle()"}},
		{"nested function", "src/big.ts#outer.helper", []string{"function helper(b: number)", "return b + 1;"}, []string{"const local"}},
		{"three links", "src/big.ts#a.b.c", []string{"c: {", "deep: 1,"}, []string{"b: {"}},
		{"inner member absent", "src/big.ts#Panel.missing", []string{"no excerpt: declaration not found"}, nil},
		{"outer absent", "src/big.ts#Nope.toggle", []string{"no excerpt: declaration not found"}, nil},
		{"doc comment through last line", "src/big.ts#documented", []string{" * Documented.", "export function documented()", "step();\n}"}, []string{"more lines", "export function huge"}},
		{"250-line declaration is capped", "src/big.ts#huge", []string{"export function huge()", "… (52 more lines)"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.Join(r.excerpt("f:x", tt.source, map[string]bool{}), "\n")
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
	t.Run("a returned named function", func(t *testing.T) {
		g := newFixture(t)
		factory := "export function createGuard(config: Config): Handle {\n  const name = config.name;\n" +
			"  return async function handle({ event }: Input): Promise<Response> {\n    return respond(event, name);\n  };\n}\n"
		g.addTracked(t, "src/guard.ts", factory+numbered(160))
		rg, err := openRepo(context.Background(), g.config())
		if err != nil {
			t.Fatal(err)
		}
		got := strings.Join(rg.excerpt("f:x", "src/guard.ts#createGuard.handle", map[string]bool{}), "\n")
		if !strings.Contains(got, "return async function handle(") || !strings.Contains(got, "return respond(event, name);") || strings.Contains(got, "const name") {
			t.Errorf("excerpt is not the returned function:\n%s", got)
		}
	})
	t.Run("the cap is 200 lines plus the closing line", func(t *testing.T) {
		got := r.excerpt("f:x", "src/big.ts#huge", map[string]bool{})
		if len(got) != 1 {
			t.Fatalf("blocks = %d, want 1", len(got))
		}
		// Header, 200 body lines, the closing line, and the footer make 203 lines, 202 newlines.
		if n := strings.Count(got[0], "\n"); n != 202 {
			t.Errorf("block has %d newlines, want 202:\n%s", n, got[0])
		}
	})
}

func TestExcerptWholeSmallFile(t *testing.T) {
	f := newFixture(t)
	f.addTracked(t, "src/mid.go", numbered(120))
	f.addTracked(t, "src/long.go", numbered(400))
	r, err := openRepo(context.Background(), f.config())
	if err != nil {
		t.Fatal(err)
	}
	t.Run("a 120-line file shows whole for a line pointer", func(t *testing.T) {
		got := strings.Join(r.excerpt("f:x", "src/mid.go:60", map[string]bool{}), "\n")
		if !strings.Contains(got, "lines 1-120") || !strings.Contains(got, "// line 1\n") || !strings.Contains(got, "// line 120") {
			t.Errorf("excerpt is not the whole file:\n%s", got)
		}
	})
	t.Run("a 400-line file keeps the window", func(t *testing.T) {
		got := strings.Join(r.excerpt("f:x", "src/long.go:200", map[string]bool{}), "\n")
		if !strings.Contains(got, "lines 190-210") || strings.Contains(got, "// line 189\n") {
			t.Errorf("excerpt is not the plus-or-minus-ten window:\n%s", got)
		}
	})
	t.Run("two pointers into one small file show it once", func(t *testing.T) {
		facts := plan.ParseFacts(map[string]string{"a.md": "- `f:two` Two pointers. Source: `src/mid.go:5`, `src/mid.go:100`. [verified]\n"}, f.root)
		e := &Excerpter{r: r}
		blocks, real, err := e.Fact(facts, "f:two")
		if err != nil || !real || len(blocks) != 1 {
			t.Errorf("Fact = %d blocks, real %v, err %v; want 1 block, real", len(blocks), real, err)
		}
	})
}

func TestExcerptProseSources(t *testing.T) {
	f := newFixture(t)
	f.addTracked(t, "notes.md", numbered(20))
	f.addTracked(t, "docs/guide/page.md", numbered(20))
	c := f.config()
	c.Facts.ProseSources = []string{"notes.md"}
	r, err := openRepo(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		source string
		in     []string
		out    []string
	}{
		{"a span into a listed file yields its window", "notes.md:3-5", []string{"lines 1-10", "// line 10"}, []string{"// line 11", "lines 1-20"}},
		{"a line into a listed file yields its window", "notes.md:15", []string{"lines 5-20"}, nil},
		{"a bare listed file stays prose", "notes.md", []string{"no excerpt: prose"}, nil},
		{"an anchor into a listed file stays prose", "notes.md#Setup", []string{"no excerpt: prose"}, nil},
		{"a line into an unlisted prose file stays prose", "docs/guide/page.md:4", []string{"no excerpt: prose"}, []string{"// line 4"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.Join(r.excerpt("f:x", tt.source, map[string]bool{}), "\n")
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

func TestExcerptPointer(t *testing.T) {
	f := newFixture(t)
	e, err := NewExcerpter(context.Background(), f.config())
	if err != nil {
		t.Fatal(err)
	}
	blocks, real := e.Pointer("src/greet.go#Hello")
	if len(blocks) != 1 || !real || !strings.Contains(blocks[0], "=== excerpt: src/greet.go lines 1-50 ===") {
		t.Errorf("Pointer = %q, real %v; want one real block labeled src/greet.go lines 1-50", blocks, real)
	}
	if _, real := e.Pointer("src"); real {
		t.Error("Pointer(src) reports a real excerpt for a directory")
	}
}

func TestExcerptNodeModules(t *testing.T) {
	f := newFixture(t)
	f.write(t, filepath.Join(f.root, "node_modules/pkg/index.js"), "export function pick(a) {\n  return a;\n}\n")
	f.write(t, filepath.Join(f.root, "build/out.js"), "export function pick(a) {\n  return a;\n}\n")
	e, err := NewExcerpter(context.Background(), f.config())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, source string
		real         bool
		in           string
	}{
		{"line pointer", "node_modules/pkg/index.js:2", true, "=== excerpt: node_modules/pkg/index.js (read from disk: untracked) lines 1-3 ==="},
		{"declaration pointer", "node_modules/pkg/index.js#pick", true, "return a;"},
		{"bare file", "node_modules/pkg/index.js", true, "=== file: node_modules/pkg/index.js (read from disk: untracked) ==="},
		{"missing file", "node_modules/pkg/gone.js:1", false, "no excerpt: missing"},
		{"other untracked file", "build/out.js:1", false, "no excerpt: untracked"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks, real := e.Pointer(tt.source)
			if real != tt.real || !strings.Contains(strings.Join(blocks, "\n"), tt.in) {
				t.Errorf("Pointer(%q) = %q, real %v; want real %v with %q", tt.source, blocks, real, tt.real, tt.in)
			}
		})
	}
}
