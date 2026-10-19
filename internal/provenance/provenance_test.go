package provenance

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/plan"
)

const (
	testPage  = "docs/tally.md"
	testBrief = "blueprints/docs/tally.brief.json"
)

func loadStore(t *testing.T, name string) *plan.Facts {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return plan.ParseFacts(map[string]string{name: string(data)}, t.TempDir())
}

// sent is one brief sentence: its text and its id, which may be any JSON value.
type sent struct {
	text string
	id   any
}

// briefOf renders sentences one field per line, so a sentence has its own line to cite.
func briefOf(t *testing.T, ss ...sent) string {
	t.Helper()
	var list []map[string]any
	for _, s := range ss {
		list = append(list, map[string]any{"text": s.text, "id": s.id})
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(map[string]any{"page": testPage, "sentences": list}); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// pageOf is a page that carries each sentence as a paragraph.
func pageOf(ss ...sent) string {
	var texts []string
	for _, s := range ss {
		texts = append(texts, s.text)
	}
	return "# Tally\n\n" + strings.Join(texts, "\n\n") + "\n"
}

func run(facts *plan.Facts, page, brief string, items ...FactItem) Result {
	return Check(Input{Page: testPage, PageText: page, BriefFile: testBrief, Brief: []byte(brief), Facts: facts, FactItems: items})
}

func lineHolding(text, sub string) int {
	for i, l := range strings.Split(text, "\n") {
		if strings.Contains(l, sub) {
			return i + 1
		}
	}
	return 0
}

// wantDefect fails unless res holds exactly one finding that contains want, or none when want is empty.
func wantDefect(t *testing.T, res Result, want string) {
	t.Helper()
	if want == "" {
		if len(res.Findings) != 0 {
			t.Errorf("findings %+v, want none", res.Findings)
		}
		return
	}
	if len(res.Findings) != 1 || !strings.Contains(res.Findings[0].Message, want) {
		t.Errorf("findings %+v, want one containing %q", res.Findings, want)
	}
}

func TestProvenanceShape(t *testing.T) {
	tests := []struct {
		name, brief, want string
	}{
		{"invalid JSON", `{"sentences": [`, "the brief is not valid JSON"},
		{"an empty file", ``, "the brief is not valid JSON"},
		{"an array, not an object", `[]`, "the brief is not a JSON object"},
		{"a string, not an object", `"x"`, "the brief is not a JSON object"},
		{"no sentences key", `{"page": "docs/tally.md"}`, `the brief has no "sentences" key`},
		{"sentences null", `{"sentences": null}`, `"sentences" is not an array`},
		{"sentences an object", `{"sentences": {}}`, `"sentences" is not an array`},
		{"sentences a string", `{"sentences": "x"}`, `"sentences" is not an array`},
		{"a sentence that is a string", `{"sentences": ["x"]}`, "sentence 1 is not an object with text and id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := run(loadStore(t, "facts.md"), "# T\n\nSome prose here.\n", tt.brief)
			if len(res.Findings) == 0 || !strings.Contains(res.Findings[0].Message, tt.want) {
				t.Fatalf("findings %+v, want one containing %q", res.Findings, tt.want)
			}
			f := res.Findings[0]
			if f.File != testBrief || f.Line != 1 || f.Severity != plan.SeverityError || f.Check != "provenance" || f.Fix == "" {
				t.Errorf("finding %+v, want an error of the brief at line 1 with a fix", f)
			}
		})
	}
	t.Run("a shape defect stops the page reading", func(t *testing.T) {
		res := run(nil, "# T\n\nSome prose here.\n", `{"page": "x"}`)
		if len(res.Findings) != 1 {
			t.Errorf("findings %+v, want only the shape defect", res.Findings)
		}
	})
	t.Run("other fields are ignored", func(t *testing.T) {
		res := run(nil, "# T\n", `{"page": "x", "cuts": 5, "local": "no", "sentences": []}`)
		wantDefect(t, res, "")
	})
}

func TestProvenanceClassification(t *testing.T) {
	tests := []struct {
		name string
		s    sent
		want string
	}{
		{"empty text", sent{"", "no-claim"}, "sentence 1: missing its text"},
		{"blank text", sent{" \t", "no-claim"}, "sentence 1: missing its text"},
		{"a number for an id", sent{"Tally keeps a count.", 5}, "unclassified, it has no usable id"},
		{"an object for an id", sent{"Tally keeps a count.", map[string]any{"a": 1}}, "unclassified, it has no usable id"},
		{"a missing id", sent{"Tally keeps a count.", nil}, "unclassified, it has no usable id"},
		{"no-claim in a list", sent{"Tally keeps a count.", []string{"t-state", "no-claim"}}, `"no-claim" is not valid in an id list`},
		{"a bare no-claim list", sent{"Tally keeps a count.", []string{"no-claim"}}, `"no-claim" is not valid in an id list`},
		{"an empty list", sent{"Tally keeps a count.", []string{}}, "an empty id list"},
		{"a number in a list", sent{"Tally keeps a count.", []any{7}}, "7 is not a fact id"},
		{"a bare id with a space", sent{"Tally keeps a count.", "t state"}, `"t state" is neither a fact id nor "no-claim"`},
		{"a bare id with a backtick", sent{"Tally keeps a count.", "t`state"}, "is neither a fact id nor"},
		{"an empty bare id", sent{"Tally keeps a count.", ""}, "is neither a fact id nor"},
		{"a list id with a space", sent{"Tally keeps a count.", []string{"t state"}}, `"t state" is not a fact id`},
		{"a list id that is empty", sent{"Tally keeps a count.", []string{""}}, `"" is not a fact id`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantDefect(t, run(loadStore(t, "facts.md"), pageOf(tt.s), briefOf(t, tt.s)), tt.want)
		})
	}
	t.Run("a punctuated id of the store is a fact id", func(t *testing.T) {
		s := sent{"Tally keeps a count.", "t-state"}
		wantDefect(t, run(loadStore(t, "facts.md"), pageOf(s), briefOf(t, s)), "")
	})
}

func TestProvenanceResolution(t *testing.T) {
	tests := []struct {
		name string
		s    sent
		want string
	}{
		{"an id the store lacks", sent{"Tally keeps a count.", "t-gone"}, "t-gone resolves to no fact in the store"},
		{"a skipped section's bullet", sent{"Tally keeps a count.", "t-skipped"}, "t-skipped resolves to no fact in the store"},
		{"an id defined twice", sent{"Tally keeps a count.", "t-twice"}, "t-twice names 2 store bullets, and an id must name one"},
		{"a candidate", sent{"Tally keeps a count.", "t-cand"}, "tagged [candidate]: a candidate is not citable until it is traced and retagged"},
		{"a rejected claim", sent{"Tally keeps a count.", "t-rej"}, "tagged [rejected]: a rejected claim is not citable"},
		{"a docs-drift fact", sent{"Tally keeps a count.", "t-drift"}, "tagged [docs-drift]: a docs-drift fact is not citable until the drift is resolved and retagged"},
		{"a bullet with no tag", sent{"Tally keeps a count.", "t-notag"}, "its status tag does not parse: it has no status tag from the vocabulary"},
		{"a verified fact", sent{"Tally keeps a count.", "t-state"}, ""},
		{"an external fact", sent{"Tally keeps a count.", "t-date"}, ""},
		{"a vendor fact", sent{"Tally keeps a count.", "t-url"}, ""},
		{"one good id and one missing in a list", sent{"Tally keeps a count.", []string{"t-state", "t-gone"}}, "t-gone resolves to no fact"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := run(loadStore(t, "facts.md"), pageOf(tt.s), briefOf(t, tt.s))
			wantDefect(t, res, tt.want)
			if tt.want != "" && !strings.Contains(res.Findings[0].Message, "sentence 1") {
				t.Errorf("message %q does not name the sentence", res.Findings[0].Message)
			}
		})
	}
	t.Run("an uncitable tag names its bullet's location", func(t *testing.T) {
		s := sent{"Tally keeps a count.", "t-cand"}
		res := run(loadStore(t, "facts.md"), pageOf(s), briefOf(t, s))
		if !strings.Contains(res.Findings[0].Message, "facts.md:") {
			t.Errorf("message %q lacks the bullet's file and line", res.Findings[0].Message)
		}
	})
}

func TestProvenanceContainment(t *testing.T) {
	tests := []struct {
		name string
		s    sent
		want string
	}{
		{"a date in the claim", sent{"Support for the old format ended 2026-03-14.", "t-date"}, ""},
		{"a date the bullet lacks", sent{"Support for the old format ended 2026-03-15.", "t-date"}, `the date "2026-03-15" is in no cited fact`},
		{"a version in the claim", sent{"Tally needs version 2.4.1.", "t-limit"}, ""},
		{"a version the bullet lacks", sent{"Tally needs version 2.4.2.", "t-limit"}, `the version "2.4.2" is in no cited fact`},
		{"a numeral in the claim", sent{"Tally stores at most 1,284 items.", "t-limit"}, ""},
		{"a numeral with a comma the bullet lacks", sent{"Tally stores at most 1,285 items.", "t-limit"}, `the numeral "1285" is in no cited fact`},
		{"a numeral only in the source pointer", sent{"Tally stores 5 items.", "t-limit"}, `the numeral "5" is in no cited fact`},
		{"a URL in the claim", sent{"The notes are at https://example.com/tally/notes.", "t-url"}, ""},
		{"a URL the bullet lacks", sent{"The notes are at https://example.com/other.", "t-url"}, `the path "https://example.com/other" is in no cited fact`},
		{"a path in the source pointers", sent{"The count lives in `src/state.go`.", "t-state"}, ""},
		{"a path the bullet lacks", sent{"The count lives in `src/other.go`.", "t-state"}, `the path "src/other.go" is in no cited fact`},
		{"a derived command", sent{"Run `tally count` to add an item.", "t-count"}, ""},
		{"a generic command the bullet lacks", sent{"Run `git status` to look.", "t-count"}, `the command "git status" is in no cited fact`},
		{"a prose command in the claim", sent{"Install it with npm install.", "t-npm"}, ""},
		{"a prose command the bullet lacks", sent{"Install it with npm run build.", "t-npm"}, `the command "npm run build" is in no cited fact`},
		{"a flag in the claim", sent{"Pass --by to set the amount.", "t-count"}, ""},
		{"a flag the bullet lacks", sent{"Pass --to to set the amount.", "t-count"}, `the flag "--to" is in no cited fact`},
		{"a name in the claim", sent{"The count is held in `stateFile`.", "t-state"}, ""},
		{"a name the bullet lacks", sent{"The count is held in `otherFile`.", "t-state"}, `the name "otherFile" is in no cited fact`},
		{"a token in the second of two cited facts", sent{"Pass --by to set the amount, up to 1,284 items.", []string{"t-count", "t-limit"}}, ""},
		{"a token in neither of two cited facts", sent{"Pass --by to set the amount, up to 9 items.", []string{"t-count", "t-limit"}}, `the numeral "9" is in no cited fact`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := run(loadStore(t, "facts.md"), pageOf(tt.s), briefOf(t, tt.s))
			if tt.want == "" {
				wantDefect(t, res, "")
				return
			}
			wantDefect(t, res, tt.want)
			if !strings.Contains(res.Findings[0].Message, "cites t-") {
				t.Errorf("message %q does not name the cited facts", res.Findings[0].Message)
			}
		})
	}
}

func TestProvenanceDerivedCommandHead(t *testing.T) {
	s := sent{"Run `tally reset` to start over.", "t-state"}
	t.Run("a head derived from the store reads the span as a command", func(t *testing.T) {
		wantDefect(t, run(loadStore(t, "facts.md"), pageOf(s), briefOf(t, s)), `the command "tally reset" is in no cited fact`)
	})
	t.Run("a store with no such span leaves it unread", func(t *testing.T) {
		s := sent{"Run `tally reset` to start over.", "p-reset"}
		wantDefect(t, run(loadStore(t, "facts-plain.md"), pageOf(s), briefOf(t, s)), "")
	})
}

func TestProvenanceNoClaim(t *testing.T) {
	tests := []struct {
		name string
		s    sent
		want string
	}{
		{"an introduction with no specific", sent{"Here is how counting works.", "no-claim"}, ""},
		{"a numeral", sent{"There are 3 steps.", "no-claim"}, `marked no-claim but states the numeral "3"`},
		{"a derived command", sent{"Run `tally count` first.", "no-claim"}, `marked no-claim but states the command "tally count"`},
		{"a flag", sent{"Pass --by first.", "no-claim"}, `marked no-claim but states the flag "--by"`},
		{"a path", sent{"Open `src/a.go` first.", "no-claim"}, `marked no-claim but states the path "src/a.go"`},
		{"a date", sent{"It ended 2026-03-14.", "no-claim"}, `marked no-claim but states the date "2026-03-14"`},
		{"a version", sent{"It needs 1.2.3.", "no-claim"}, `marked no-claim but states the version "1.2.3"`},
		{"a name", sent{"Call `startUp` first.", "no-claim"}, `marked no-claim but states the name "startUp"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantDefect(t, run(loadStore(t, "facts.md"), pageOf(tt.s), briefOf(t, tt.s)), tt.want)
		})
	}
}

func TestProvenanceCoverage(t *testing.T) {
	a := sent{"Tally keeps a count.", "t-state"}
	b := sent{"Here is how counting works.", "no-claim"}
	long := sent{"Tally keeps a count in a file.", "t-state"}
	tests := []struct {
		name  string
		page  string
		brief string
		want  []string
	}{
		{"every sentence on the page", pageOf(a, b), briefOf(t, a, b), nil},
		{"a brief sentence not on the page", pageOf(a), briefOf(t, a, b), []string{`"Here is how counting works." is not on the page`}},
		{"page text the brief lacks", pageOf(a, b), briefOf(t, a), []string{`unclassified page text: "Here is how counting works."`}},
		{"emphasis on the page", "# Tally\n\nTally **keeps** a _count_.\n", briefOf(t, a), nil},
		{"code marks only on the page", "# Tally\n\nTally `keeps` a count.\n", briefOf(t, a), nil},
		{"hard-wrapped page text", "# Tally\n\nTally keeps\na count.\n", briefOf(t, a), nil},
		{"front matter is set aside", "---\ntitle: Anything at all\n---\n# Tally\n\nTally keeps a count.\n", briefOf(t, a), nil},
		{"a fenced block is set aside", "# Tally\n\nTally keeps a count.\n\n```sh\ntally count stuff\n```\n", briefOf(t, a), nil},
		{"a tilde fence is set aside", "# Tally\n\nTally keeps a count.\n\n~~~~\nraw text\n~~~~\n", briefOf(t, a), nil},
		{"an indented fence is set aside", "# Tally\n\n- Step.\n\n  ```sh\n  raw text\n  ```\n", briefOf(t, sent{"Step.", "no-claim"}), nil},
		{"an unclosed fence stays prose", "# Tally\n\nTally keeps a count.\n\n```sh\nraw text\n", briefOf(t, a), []string{"unclassified page text: \"sh raw text\""}},
		{"a comment is set aside", "# Tally\n\n<!-- a note\nfor editors -->\nTally keeps a count.\n", briefOf(t, a), nil},
		{"details tags are set aside, their text stays", "# Tally\n\n<details>\n<summary>Tally keeps a count.</summary>\n</details>\n", briefOf(t, a), nil},
		{"an image is set aside", "# Tally\n\n![diagram of the count](img/count.png)\n\nTally keeps a count.\n", briefOf(t, a), nil},
		{"headings are set aside", "# Tally\n\n## More\n\nTally keeps a count.\n", briefOf(t, a), nil},
		{"list markers are not text", "# Tally\n\n- Tally keeps a count.\n- Here is how counting works.\n", briefOf(t, a, b), nil},
		{"list numbers are not text", "# Tally\n\n1. Tally keeps a count.\n2. Here is how counting works.\n", briefOf(t, a, b), nil},
		{"a table's rules are not text", "# Tally\n\n| Tally keeps a count. |\n|---|\n", briefOf(t, a), nil},
		{"a quote mark is not text", "# Tally\n\n> Tally keeps a count.\n", briefOf(t, a), nil},
		{"a table cell is prose", "# Tally\n\n| Tally keeps a count. | Extra words |\n|---|---|\n", briefOf(t, a), []string{`unclassified page text: "Extra words"`}},
		{"a short sentence inside a long one is consumed after it", pageOf(long), briefOf(t, long, a), []string{`"Tally keeps a count." is not on the page`}},
		{"a repeated sentence needs two page copies", pageOf(a), briefOf(t, a, a), []string{`"Tally keeps a count." is not on the page`}},
		{"a repeated sentence on the page twice", pageOf(a, a), briefOf(t, a, a), nil},
		{"punctuation alone is not text", "# Tally\n\nTally keeps a count.\n\n---\n\n* * *\n", briefOf(t, a), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := run(loadStore(t, "facts.md"), tt.page, tt.brief)
			var got []string
			for _, f := range res.Findings {
				got = append(got, f.Message)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("findings %q, want %q", got, tt.want)
			}
			for i, w := range tt.want {
				if got[i] != w {
					t.Errorf("finding %d = %q, want %q", i, got[i], w)
				}
			}
		})
	}
}

func TestProvenanceEmptySentencesOnAPageWithProse(t *testing.T) {
	res := run(loadStore(t, "facts.md"), "# Tally\n\nTally keeps a count.\n", `{"sentences": []}`)
	if len(res.Findings) != 1 || res.Findings[0].Message != `unclassified page text: "Tally keeps a count."` || res.Findings[0].File != testPage {
		t.Errorf("findings %+v, want one unclassified page text on the page, and no shape defect", res.Findings)
	}
	wantDefect(t, run(loadStore(t, "facts.md"), "# Tally\n", `{"sentences": []}`), "")
}

func TestProvenanceFactItems(t *testing.T) {
	a := sent{"Tally keeps a count.", "t-state"}
	b := sent{"Pass --by to set the amount.", []string{"t-count", "t-state"}}
	tests := []struct {
		name  string
		items []FactItem
		want  string
	}{
		{"an item cited by a bare id", []FactItem{{"i1", "t-state"}}, ""},
		{"an item cited in a list", []FactItem{{"i2", "t-count"}}, ""},
		{"an item cited by nobody", []FactItem{{"i3", "t-limit"}}, "fact item i3 places t-limit, but no brief sentence cites it"},
		{"every item checked", []FactItem{{"i1", "t-state"}, {"i2", "t-count"}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := run(loadStore(t, "facts.md"), pageOf(a, b), briefOf(t, a, b), tt.items...)
			wantDefect(t, res, tt.want)
			if tt.want != "" && (res.Findings[0].File != testBrief || res.Findings[0].Line != 1) {
				t.Errorf("finding %+v, want the brief at line 1", res.Findings[0])
			}
		})
	}
	t.Run("a no-claim sentence cites no item", func(t *testing.T) {
		n := sent{"Here is how counting works.", "no-claim"}
		res := run(loadStore(t, "facts.md"), pageOf(n), briefOf(t, n), FactItem{"i1", "t-state"})
		wantDefect(t, res, "fact item i1 places t-state")
	})
}

func TestProvenanceNoFactsDir(t *testing.T) {
	cited := sent{"Tally keeps a count.", "t-state"}
	list := sent{"Tally keeps a count.", []string{"t-state", "t-count"}}
	n := sent{"Here is how counting works.", "no-claim"}
	t.Run("a cited id is a defect", func(t *testing.T) {
		wantDefect(t, run(nil, pageOf(cited), briefOf(t, cited)), "t-state resolves to no fact: the project sets no facts.dir")
	})
	t.Run("each id of a list is a defect", func(t *testing.T) {
		res := run(nil, pageOf(list), briefOf(t, list))
		if len(res.Findings) != 2 {
			t.Errorf("findings %+v, want one per id", res.Findings)
		}
	})
	t.Run("a no-claim sentence is clean", func(t *testing.T) {
		res := run(nil, pageOf(n), briefOf(t, n))
		wantDefect(t, res, "")
		if res.NoClaim != 1 {
			t.Errorf("no-claim count %d, want 1", res.NoClaim)
		}
	})
}

func TestProvenanceLines(t *testing.T) {
	first := sent{"Tally keeps a count.", "t-state"}
	html := sent{"Compare a<b>&c with `stateFile` in 7 places.", "t-state"}
	last := sent{"Tally keeps a count too.", "t-gone"}
	page := pageOf(first, html, last)
	brief := briefOf(t, first, html, last)
	res := run(loadStore(t, "facts.md"), page, brief)
	if len(res.Findings) != 2 {
		t.Fatalf("findings %+v, want two", res.Findings)
	}
	wantHTML := lineHolding(brief, "a<b>&c")
	wantLast := lineHolding(brief, "too.")
	if wantHTML < 5 || res.Findings[0].Line != wantHTML || res.Findings[0].File != testBrief {
		t.Errorf("html sentence cited at %s:%d, want %s:%d", res.Findings[0].File, res.Findings[0].Line, testBrief, wantHTML)
	}
	if res.Findings[1].Line != wantLast || res.Findings[1].Line <= wantHTML {
		t.Errorf("last sentence cited at line %d, want %d", res.Findings[1].Line, wantLast)
	}
	t.Run("a sentence missing from the page is cited in the brief", func(t *testing.T) {
		amp := sent{"Compare a<b>&c now.", "no-claim"}
		res := run(loadStore(t, "facts.md"), pageOf(first), briefOf(t, first, amp))
		if len(res.Findings) != 1 || res.Findings[0].File != testBrief || res.Findings[0].Line != lineHolding(briefOf(t, first, amp), "a<b>&c") {
			t.Errorf("findings %+v, want the brief at the sentence's line", res.Findings)
		}
	})
	t.Run("page text is cited in the page at its first words", func(t *testing.T) {
		page := "# Tally\n\nTally keeps a count.\n\nThen these extra words stand alone here.\n"
		res := run(loadStore(t, "facts.md"), page, briefOf(t, first))
		if len(res.Findings) != 1 || res.Findings[0].File != testPage || res.Findings[0].Line != 5 {
			t.Errorf("findings %+v, want the page at line 5", res.Findings)
		}
	})
	t.Run("page text whose first words wrap is cited at line 1", func(t *testing.T) {
		page := "# Tally\n\nTally keeps a count.\n\nThen these extra\nwords stand alone here.\n"
		res := run(loadStore(t, "facts.md"), page, briefOf(t, first))
		if len(res.Findings) != 1 || res.Findings[0].File != testPage || res.Findings[0].Line != 1 {
			t.Errorf("findings %+v, want the page at line 1", res.Findings)
		}
	})
	t.Run("a brief on one line cites line 1", func(t *testing.T) {
		res := run(loadStore(t, "facts.md"), pageOf(last), `{"sentences":[{"text":"Tally keeps a count too.","id":"t-gone"}]}`)
		if len(res.Findings) != 1 || res.Findings[0].Line != 1 {
			t.Errorf("findings %+v, want line 1", res.Findings)
		}
	})
}

func TestProvenanceCleanCounts(t *testing.T) {
	ss := []sent{
		{"Tally keeps a count.", "t-state"},
		{"Pass --by to set the amount.", []string{"t-count", "t-limit"}},
		{"Here is how counting works.", "no-claim"},
		{"Another lead-in.", "no-claim"},
		{"Support ended 2026-03-14.", "t-date"},
	}
	res := run(loadStore(t, "facts.md"), pageOf(ss...), briefOf(t, ss...), FactItem{"i1", "t-state"})
	wantDefect(t, res, "")
	if res.Sentences != 5 || res.Cited != 3 || res.NoClaim != 2 {
		t.Errorf("counts %d sentences, %d cited, %d no-claim; want 5, 3, 2", res.Sentences, res.Cited, res.NoClaim)
	}
}
