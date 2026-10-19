package stock

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/plan"
	"github.com/glw907/drafter/internal/prompts"
)

var update = flag.Bool("update", false, "rewrite the golden renderings under testdata")

type stockMatrix struct {
	Host struct {
		Args     json.RawMessage `json:"args"`
		Resolved json.RawMessage `json:"resolved"`
	} `json:"host"`
	Cases []stockCase `json:"cases"`
}

type stockCase struct {
	Name    string          `json:"name"`
	Seat    string          `json:"seat"`
	Context prompts.Context `json:"context"`
}

func loadStockMatrix(t *testing.T) (stockMatrix, *prompts.Module) {
	t.Helper()
	return loadMatrix(t, filepath.Join("testdata", "matrix.json"))
}

// fixtureMatrix is the tally fixture's case list, which check-prompts also reads.
var fixtureMatrix = filepath.Join("..", "e2e", "testdata", "cli", "prompts-matrix.json")

func loadMatrix(t *testing.T, path string) (stockMatrix, *prompts.Module) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m stockMatrix
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	mod, err := prompts.Load("drafter:prompts.js", Prompts, prompts.Host{Args: m.Host.Args, Resolved: m.Host.Resolved})
	if err != nil {
		t.Fatal(err)
	}
	return m, mod
}

func TestStockModuleSeats(t *testing.T) {
	_, mod := loadStockMatrix(t)
	want := []string{"blueprint", "coherence", "draft", "editor", "fact", "reader", "structure"}
	if got := mod.Seats(); !slices.Equal(got, want) {
		t.Errorf("seats = %v, want %v", got, want)
	}
}

// TestStockRenderings compares each rendering of the matrix with its golden file. Run with
// -update to rewrite them after a deliberate prompt change.
func TestStockRenderings(t *testing.T) {
	m, mod := loadStockMatrix(t)
	if len(m.Cases) != 6 {
		t.Fatalf("matrix has %d cases, want 6", len(m.Cases))
	}
	for _, c := range m.Cases {
		t.Run(c.Name, func(t *testing.T) {
			got, err := mod.Render(c.Seat, c.Context)
			if err != nil {
				t.Fatal(err)
			}
			golden := filepath.Join("testdata", c.Name+".txt")
			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Errorf("rendering differs from %s; rerun with -update if the change is deliberate\n got: %s", golden, got)
			}
		})
	}
}

func TestStockRenderingContent(t *testing.T) {
	m, mod := loadStockMatrix(t)
	tests := []struct {
		name    string
		token   string
		change  bool
		mention bool // whether the text may name a register
	}{
		{"whole-anatomy", "`drafter:anatomies.md#task-guide`", false, false},
		{"whole-register", "`docs/house-style.md#the-page-anatomies`", false, true},
		{"whole-neither", "`drafter:anatomies.md`", false, false},
		{"change-anatomy", "`drafter:anatomies.md#task-guide`", true, false},
		{"change-register", "`docs/house-style.md#the-page-anatomies`", true, true},
		{"change-neither", "`drafter:anatomies.md`", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := slices.IndexFunc(m.Cases, func(c stockCase) bool {
				return c.Name == tt.name
			})
			if i < 0 {
				t.Fatalf("no case %s in the matrix", tt.name)
			}
			got, err := mod.Render("blueprint", m.Cases[i].Context)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(got, tt.token) {
				t.Errorf("rendering lacks the anatomy token %s", tt.token)
			}
			if !tt.mention && strings.Contains(strings.ToLower(got), "register") {
				t.Errorf("rendering with no register names one")
			}
			for _, s := range []string{`starting "Nit:"`, `start its finding with
"Already present:"`, "Change read."} {
				if strings.Contains(got, s) != tt.change {
					t.Errorf("contains %q = %v, want %v", s, !tt.change, tt.change)
				}
			}
		})
	}
}

// TestStockBarRenderings compares each change read with a bar, from testdata/bar-matrix.json,
// with its golden file, and checks what the bar paragraph must carry: every finding, the rule
// that an unfixed listed finding blocks whatever its class, and nothing else changed, since the
// rendering is the same change read without findings with the paragraph after it. Run with -update
// to rewrite the goldens after a deliberate prompt change.
func TestStockBarRenderings(t *testing.T) {
	m, mod := loadMatrix(t, filepath.Join("testdata", "bar-matrix.json"))
	if len(m.Cases) != 2 {
		t.Fatalf("bar matrix has %d cases, want 2", len(m.Cases))
	}
	for _, c := range m.Cases {
		t.Run(c.Name, func(t *testing.T) {
			got, err := mod.Render(c.Seat, c.Context)
			if err != nil {
				t.Fatal(err)
			}
			golden := filepath.Join("testdata", c.Name+".txt")
			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Errorf("rendering differs from %s; rerun with -update if the change is deliberate\n got: %s", golden, got)
			}
			bare := c.Context
			bare.Findings = ""
			plain, err := mod.Render(c.Seat, bare)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(got, plain+"\n\nEarlier findings.") {
				t.Errorf("the bar paragraph does not follow the change read unchanged")
			}
			flat := strings.Join(strings.Fields(got), " ")
			for _, s := range []string{
				"The earlier findings:\n" + c.Context.Findings, `its text starts "Still open:"`, "It blocks whatever its class",
				"A defect the change adds or causes blocks", `its text starts "Already present:"`,
			} {
				if !strings.Contains(got, s) && !strings.Contains(flat, s) {
					t.Errorf("rendering lacks %q", s)
				}
			}
			if !strings.HasSuffix(got, c.Context.Findings) {
				t.Error("the findings are not quoted whole at the end")
			}
		})
	}
}

// TestStockAnatomySlugs proves the module's anatomy token names the heading drafter's slugger
// gives each stock anatomy, so the excerpt the token asks for exists.
func TestStockAnatomySlugs(t *testing.T) {
	var level2 []string
	for _, h := range plan.Headings(anatomies) {
		if h.Level == 2 {
			level2 = append(level2, h.Slug)
		}
	}
	if len(level2) != len(Anatomies) {
		t.Fatalf("anatomies.md has %d level-2 headings, want %d", len(level2), len(Anatomies))
	}
	for i, name := range Anatomies {
		t.Run(name, func(t *testing.T) {
			resolved := fmt.Sprintf(`{"pages":[{"path":"a.md","anatomy":%q}]}`, name)
			mod, err := prompts.Load("drafter:prompts.js", Prompts, prompts.Host{Resolved: json.RawMessage(resolved)})
			if err != nil {
				t.Fatal(err)
			}
			got, err := mod.Render("blueprint", prompts.Context{Page: json.RawMessage(`{"path":"a.md"}`), Round: prompts.Round1})
			if err != nil {
				t.Fatal(err)
			}
			if want := "`drafter:anatomies.md#" + level2[i] + "`"; !strings.Contains(got, want) {
				t.Errorf("rendering lacks %s", want)
			}
		})
	}
}

func TestStockAnatomyHeadings(t *testing.T) {
	var got []string
	sc := bufio.NewScanner(bytes.NewReader(anatomies))
	for sc.Scan() {
		if name, ok := strings.CutPrefix(sc.Text(), "## "); ok {
			got = append(got, strings.TrimSpace(name))
		}
	}
	if !slices.Equal(got, Anatomies) {
		t.Errorf("anatomies.md ## headings = %q, want %q", got, Anatomies)
	}
}

func TestStockFiles(t *testing.T) {
	tests := []struct {
		name string
		want []byte
		ok   bool
	}{
		{"prompts.js", Prompts, true},
		{"anatomies.md", anatomies, true},
		{"introductions.md", introductions, true},
		{"register.md", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Files(tt.name)
			if ok != tt.ok || !bytes.Equal(got, tt.want) {
				t.Errorf("Files(%q) = %d bytes, %v; want %d bytes, %v", tt.name, len(got), ok, len(tt.want), tt.ok)
			}
		})
	}
}

// renderAll renders every case of the matrix at path, keyed by case name.
func renderAll(t *testing.T, path string) map[string]string {
	t.Helper()
	m, mod := loadMatrix(t, path)
	out := make(map[string]string, len(m.Cases))
	for _, c := range m.Cases {
		got, err := mod.Render(c.Seat, c.Context)
		if err != nil {
			t.Fatalf("render %s: %v", c.Name, err)
		}
		out[c.Name] = got
	}
	return out
}

// TestFixtureMatrixRenders renders every case of the fixture's matrix and compares the first case
// of each drafting and review seat with its golden file. Run with -update to rewrite them.
func TestFixtureMatrixRenders(t *testing.T) {
	m, mod := loadMatrix(t, fixtureMatrix)
	golden := map[string]bool{}
	for _, c := range m.Cases {
		t.Run(c.Name, func(t *testing.T) {
			got, err := mod.Render(c.Seat, c.Context)
			if err != nil {
				t.Fatal(err)
			}
			if c.Seat == "blueprint" || golden[c.Seat] {
				return
			}
			golden[c.Seat] = true
			path := filepath.Join("testdata", c.Name+".txt")
			if *update {
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Errorf("rendering differs from %s; rerun with -update if the change is deliberate\n got: %s", path, got)
			}
		})
	}
	want := []string{"coherence", "draft", "editor", "fact", "reader", "structure"}
	if got := slices.Sorted(maps.Keys(golden)); !slices.Equal(got, want) {
		t.Errorf("seats with a golden = %v, want %v", got, want)
	}
}

func TestStockDenylist(t *testing.T) {
	renderings := renderAll(t, fixtureMatrix)
	maps.Copy(renderings, renderAll(t, filepath.Join("testdata", "matrix.json")))
	for _, s := range []string{"cairn", "docs/internal", "docs-register", "sveltekit", "cloudflare", "wrangler", "glw907", "npm run check:", "option-map"} {
		t.Run(s, func(t *testing.T) {
			for name, got := range renderings {
				if strings.Contains(strings.ToLower(got), s) {
					t.Errorf("%s holds %q", name, s)
				}
			}
		})
	}
}

// TestStockNoRegister proves no fixture rendering names a register, since the fixture's page has
// none.
func TestStockNoRegister(t *testing.T) {
	for name, got := range renderAll(t, fixtureMatrix) {
		if strings.Contains(strings.ToLower(got), "register") {
			t.Errorf("%s names a register", name)
		}
	}
}

// promptTokens splits text as assemble's tokens does, on whitespace, backticks, quotes,
// parentheses, brackets, angle brackets, and commas, and trims the trailing punctuation
// check-prompts trims.
func promptTokens(text string) []string {
	var out []string
	for _, tok := range strings.FieldsFunc(text, func(c rune) bool {
		return c == ' ' || c == '\t' || c == '\n' || c == '\r' || strings.ContainsRune("`'\"()[]<>,", c)
	}) {
		out = append(out, strings.TrimRight(tok, ".,;:"))
	}
	return out
}

// drafterTokenOK reports whether a drafter: token, with any #slug cut, names a file Files serves.
func drafterTokenOK(tok string) bool {
	name, _, _ := strings.Cut(strings.TrimPrefix(tok, "drafter:"), "#")
	_, ok := Files(name)
	return ok
}

func TestDrafterTokenOK(t *testing.T) {
	tests := []struct {
		tok  string
		want bool
	}{
		{"drafter:google-rubric.md", true},
		{"drafter:anatomies.md#task-guide", true},
		{"drafter:anatomies.md#no-such-heading", true},
		{"drafter:register.md", false},
		{"drafter:", false},
	}
	for _, tt := range tests {
		t.Run(tt.tok, func(t *testing.T) {
			if got := drafterTokenOK(tt.tok); got != tt.want {
				t.Errorf("drafterTokenOK(%q) = %v, want %v", tt.tok, got, tt.want)
			}
		})
	}
}

func TestStockDrafterTokens(t *testing.T) {
	renderings := renderAll(t, fixtureMatrix)
	maps.Copy(renderings, renderAll(t, filepath.Join("testdata", "matrix.json")))
	for name, got := range renderings {
		for _, tok := range promptTokens(got) {
			if strings.HasPrefix(tok, "drafter:") && !drafterTokenOK(tok) {
				t.Errorf("%s names %q, which drafter doesn't serve", name, tok)
			}
		}
	}
}

// TestStockSeatContent checks the text each targeted rendering must carry.
func TestStockSeatContent(t *testing.T) {
	renderings := renderAll(t, fixtureMatrix)
	tests := []struct {
		name string
		want []string
	}{
		{"draft-whole", []string{
			"`drafter:google-description.md`", "`drafter:google-tells.md`", "`drafter:google-corpus.md`",
			"`drafter:introductions.md`", "`drafter:anatomies.md#task-guide`",
			"blueprints/docs/count-items.brief.json",
			`one key, "sentences"`, `{"text": "the sentence as the page has`, `"id" is the bare string "no-claim"`,
			"Every sentence of the page's prose is in the brief, and every brief sentence is on the page",
			"never holds \"no-claim\"", "with the tag verified, external, or vendor",
			"A cited sentence states nothing its facts don't", "numbers and versions are matched in the",
			`A "no-claim" sentence states none of those`,
			"is cited by at least one sentence in that item's section",
		}},
		{"draft-section", []string{"Change only the sections in scope: `add-items`.", "Finding 1 (blueprint, blocking)"}},
		{"editor-whole", []string{"`drafter:google-rubric.md`", "`drafter:google-tells.md`"}},
		{"structure-whole", []string{"`drafter:anatomies.md#task-guide`"}},
		{"fact-whole", []string{"source excerpt", "verified, external, or vendor"}},
		{"coherence-read", []string{"changed the sections `add-items`", "A finding may lie anywhere on the page"}},
		{"coherence-retest", []string{"changed the sections `add-items`", "A finding may lie anywhere on the page", "This is the retest"}},
		{"reader-turn2", []string{"Count a list of items with tally and read the total."}},
		{"reader-retest", []string{"This is the retest after the final redraft"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := renderings[tt.name]
			if !ok {
				t.Fatalf("no case %s in the fixture matrix", tt.name)
			}
			for _, s := range tt.want {
				if !strings.Contains(got, s) {
					t.Errorf("rendering lacks %q", s)
				}
			}
		})
	}
}

// TestStockEditTurns proves every draft turn but the whole draft asks for edits, never a page.
func TestStockEditTurns(t *testing.T) {
	renderings := renderAll(t, fixtureMatrix)
	tests := []struct {
		name  string
		whole bool
	}{
		{"draft-whole", true},
		{"draft-section", false},
		{"draft-redraft", false},
		{"draft-final", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderings[tt.name]
			if strings.Contains(got, "Answer with the whole page as Markdown") != tt.whole {
				t.Errorf("asks for the whole page = %v, want %v", !tt.whole, tt.whole)
			}
			if strings.Contains(got, "Findings:\n") == tt.whole {
				t.Errorf("carries findings = %v, want %v", tt.whole, !tt.whole)
			}
		})
	}
}

// sections splits a Markdown page at its level-2 headings and returns each section's text after
// the heading line, the page's text before the first heading left out.
func sections(page []byte) []string {
	parts := strings.Split("\n"+string(page), "\n## ")
	return parts[1:]
}

var retrieved = regexp.MustCompile(`(?m)^Retrieved: \d{4}-\d{2}-\d{2}$`)

func TestGooglePackage(t *testing.T) {
	tests := []struct {
		name  string
		file  []byte
		min   int
		check func(sec string) string
	}{
		{"google-description.md", googleDescription, 1, func(sec string) string { return "" }},
		{"google-rubric.md", googleRubric, 10, func(sec string) string {
			if !strings.Contains(sec, "(https://developers.google.com/style/") {
				return "no developers.google.com/style URL"
			}
			return ""
		}},
		{"google-tells.md", googleTells, 5, func(sec string) string {
			if strings.Count(sec, "\nBefore: ") != 1 || strings.Count(sec, "\nAfter: ") != 1 {
				return "not one before-and-after pair"
			}
			return ""
		}},
		{"google-corpus.md", googleCorpus, 3, func(sec string) string {
			switch {
			case !strings.Contains(sec, "\nURL: <https://"):
				return "no URL"
			case !strings.Contains(sec, "\nLicense: Creative Commons Attribution 4.0"):
				return "no license"
			case !retrieved.MatchString(sec):
				return "no retrieval date"
			case !strings.Contains(sec, "\n> "):
				return "no excerpt"
			}
			return ""
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Files(tt.name)
			if !ok || !bytes.Equal(got, tt.file) || len(bytes.TrimSpace(got)) == 0 {
				t.Fatalf("Files(%q) serves %d bytes, %v; want the non-empty embedded file", tt.name, len(got), ok)
			}
			secs := sections(got)
			if len(secs) < tt.min {
				t.Errorf("%d sections, want at least %d", len(secs), tt.min)
			}
			for _, sec := range secs {
				if msg := tt.check(sec); msg != "" {
					heading, _, _ := strings.Cut(sec, "\n")
					t.Errorf("section %q: %s", heading, msg)
				}
			}
		})
	}
	if !strings.Contains(string(googleDescription), "https://developers.google.com/style") ||
		!strings.Contains(string(googleDescription), "Creative Commons Attribution 4.0 License") {
		t.Errorf("google-description.md doesn't name the guide's URL and license")
	}
}
