// Package provenance checks a drafted page against its brief and the facts store.
//
// The brief maps every sentence of the page's prose to the fact ids it rests on, or to "no-claim".
// The check is deny by default: a sentence that is not classified is a defect, a cited id must
// resolve to one citable bullet, and each number, version, date, path, URL, command, flag, and
// code name in a sentence must appear in a bullet the sentence cites. It reads the page and the
// brief both ways, so a brief sentence the page lacks and page text no sentence covers are each a
// defect. Every defect is an error.
//
// The extraction is deterministic pattern work over a sentence's text, so it cannot tell whether a
// sentence says what its bullet says: negation, scope words ("only", "every"), ordering, and
// causation pass once the tokens are present. Spelled-out numbers, units, short flags, bare
// filenames in prose, and snippets in code spans are not read either. A reviewer who reads the
// brief beside the page owns what the check cannot reach.
package provenance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/glw907/drafter/internal/coverage"
	"github.com/glw907/drafter/internal/plan"
)

// CheckName is the check name the findings carry.
const CheckName = "provenance"

const noClaim = "no-claim"

// uncitable gives the reason each unusable status tag keeps a bullet from being cited.
var uncitable = map[string]string{
	"candidate":  "a candidate is not citable until it is traced and retagged",
	"rejected":   "a rejected claim is not citable",
	"docs-drift": "a docs-drift fact is not citable until the drift is resolved and retagged",
}

var (
	frontMatterRe = regexp.MustCompile(`(?s)\A---\n.*?\n---(?:\n|\z)`)
	commentRe     = regexp.MustCompile(`(?s)<!--.*?-->`)
	headingRe     = regexp.MustCompile(`(?m)^#{1,6}[ \t].*$`)
	listNumberRe  = regexp.MustCompile(`^\d+[.)]$`)
)

// FactItem is a blueprint fact item: the item's id and the fact it places.
type FactItem struct{ ID, Fact string }

// Input is what one check reads. Page and BriefFile are the repo-relative slash paths findings
// cite. Facts is nil when the project sets no facts directory.
type Input struct {
	Page      string
	PageText  string
	BriefFile string
	Brief     []byte
	Facts     *plan.Facts
	FactItems []FactItem
}

// Result is the findings of one check and the brief's counts. The counts are meaningful only when
// Findings is empty.
type Result struct {
	Findings  []plan.Finding
	Sentences int
	Cited     int
	NoClaim   int
}

type sentence struct {
	Text any `json:"text"`
	ID   any `json:"id"`
}

type checker struct {
	in         Input
	briefLines []string
	pageLines  []string
	heads      map[string]bool
	res        Result
}

// Check runs the provenance check. Every defect it finds is an error finding that names the brief
// or the page.
func Check(in Input) Result {
	c := &checker{
		in:         in,
		briefLines: strings.Split(string(in.Brief), "\n"),
		pageLines:  strings.Split(in.PageText, "\n"),
	}
	raw, ok := c.decode()
	if !ok {
		return c.res
	}
	c.heads = commandHeads(c.claims())

	type located struct {
		text string
		line int
	}
	var onPage []located
	cited := map[string]bool{}
	c.res.Sentences = len(raw)
	for i, r := range raw {
		var s sentence
		if err := json.Unmarshal(r, &s); err != nil {
			c.brief(1, fmt.Sprintf("sentence %d is not an object with text and id", i+1), `write each sentence as {"text": "...", "id": ...}`)
			continue
		}
		text, _ := s.Text.(string)
		if strings.TrimSpace(text) == "" {
			c.brief(1, fmt.Sprintf("sentence %d: missing its text", i+1), "give the sentence its text, or drop the entry")
			continue
		}
		line := c.briefLine(text)
		onPage = append(onPage, located{text, line})
		label := fmt.Sprintf("sentence %d (%q)", i+1, preview(text))
		ids, isNoClaim, defects := classify(s.ID)
		for _, d := range defects {
			c.brief(line, label+": "+d, `give it a fact id, a list of fact ids, or "no-claim"`)
		}
		if len(defects) > 0 {
			continue
		}
		if isNoClaim {
			c.res.NoClaim++
			for _, t := range extract(text, c.heads) {
				c.brief(line, fmt.Sprintf("%s: marked no-claim but states the %s %q", label, t.kind, t.value), "cite the fact that states it, or take it out of the sentence")
			}
			continue
		}
		c.res.Cited++
		for _, id := range ids {
			cited[id] = true
		}
		c.cite(line, label, ids, text)
	}

	prose := pageProse(in.PageText)
	longestFirst := make([]int, len(onPage))
	for i := range longestFirst {
		longestFirst[i] = i
	}
	slices.SortStableFunc(longestFirst, func(a, b int) int {
		return len(coverage.Normalize(onPage[b].text)) - len(coverage.Normalize(onPage[a].text))
	})
	missing := map[int]bool{}
	for _, k := range longestFirst {
		want := coverage.Normalize(onPage[k].text)
		at := -1
		if want != "" {
			at = strings.Index(prose, want)
		}
		if at < 0 {
			missing[k] = true
			continue
		}
		prose = prose[:at] + "\x00" + prose[at+len(want):]
	}
	for k, s := range onPage {
		if missing[k] {
			c.brief(s.line, fmt.Sprintf("%q is not on the page", preview(s.text)), "put the sentence on the page word for word, or take it out of the brief")
		}
	}
	for fragment := range strings.SplitSeq(prose, "\x00") {
		if !hasContent(fragment) {
			continue
		}
		shown := strings.Trim(fragment, "-*+>|# \t\n")
		c.page(c.pageLine(shown), fmt.Sprintf("unclassified page text: %q", preview(shown)), "add the text to the brief with a fact id or no-claim, or take it off the page")
	}

	for _, it := range in.FactItems {
		if it.Fact != "" && !cited[it.Fact] {
			c.brief(1, fmt.Sprintf("fact item %s places %s, but no brief sentence cites it", it.ID, it.Fact), "cite the fact in the sentence that carries it")
		}
	}
	return c.res
}

// decode returns the brief's raw sentences. It reports false after a shape defect.
func (c *checker) decode() ([]json.RawMessage, bool) {
	const fix = `write the brief as a JSON object with a "sentences" array`
	if !json.Valid(c.in.Brief) {
		c.brief(1, "the brief is not valid JSON", fix)
		return nil, false
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(c.in.Brief, &top); err != nil {
		c.brief(1, "the brief is not a JSON object", fix)
		return nil, false
	}
	raw, ok := top["sentences"]
	if !ok {
		c.brief(1, `the brief has no "sentences" key`, fix)
		return nil, false
	}
	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err != nil || list == nil {
		c.brief(1, `"sentences" is not an array`, fix)
		return nil, false
	}
	return list, true
}

// classify reads a sentence's id. It returns the fact ids, whether the sentence is no-claim, and
// why it is unclassified, if it is.
func classify(id any) (ids []string, isNoClaim bool, defects []string) {
	switch v := id.(type) {
	case string:
		switch {
		case v == noClaim:
			return nil, true, nil
		case !validID(v):
			return nil, false, []string{fmt.Sprintf("%q is neither a fact id nor %q, so the sentence is unclassified", v, noClaim)}
		}
		return []string{v}, false, nil
	case []any:
		if len(v) == 0 {
			return nil, false, []string{"unclassified, an empty id list"}
		}
		for _, e := range v {
			s, ok := e.(string)
			switch {
			case ok && s == noClaim:
				defects = append(defects, fmt.Sprintf("%q is not valid in an id list, so the sentence is unclassified", noClaim))
			case !ok || !validID(s):
				defects = append(defects, fmt.Sprintf("%s is not a fact id, so the sentence is unclassified", encode(e)))
			default:
				ids = append(ids, s)
			}
		}
		return ids, false, defects
	}
	return nil, false, []string{"unclassified, it has no usable id"}
}

// validID reports whether id has a fact id's shape: non-empty, with no whitespace or backtick.
func validID(id string) bool {
	return id != "" && !strings.ContainsFunc(id, unicode.IsSpace) && !strings.Contains(id, "`")
}

// claims lists the claim text of every bullet in the store.
func (c *checker) claims() []string {
	var out []string
	for _, id := range c.in.Facts.IDs() {
		if f, ok := c.in.Facts.Get(id); ok {
			out = append(out, f.Text)
		}
	}
	return out
}

// cite checks one cited sentence: each id resolves to one citable bullet, and each extracted token
// is in at least one of them.
func (c *checker) cite(line int, label string, ids []string, text string) {
	var found []plan.Fact
	for _, id := range ids {
		if c.in.Facts == nil {
			c.brief(line, fmt.Sprintf("%s: %s resolves to no fact: the project sets no facts.dir", label, id), "set facts.dir in drafter.toml, or mark the sentence no-claim")
			continue
		}
		f, ok := c.in.Facts.Get(id)
		if !ok {
			msg := fmt.Sprintf("%s: %s resolves to no fact in the store", label, id)
			if n := c.in.Facts.Count(id); n > 1 {
				msg = fmt.Sprintf("%s: %s names %d store bullets, and an id must name one", label, id, n)
			}
			c.brief(line, msg, "cite an id the facts store holds")
			continue
		}
		found = append(found, f)
		if f.Citable() {
			continue
		}
		reason := "its status tag is not citable"
		switch {
		case f.Problem != "":
			reason = "its status tag does not parse: " + f.Problem
		case uncitable[f.Tag] != "":
			reason = fmt.Sprintf("tagged [%s]: %s", f.Tag, uncitable[f.Tag])
		}
		c.brief(line, fmt.Sprintf("%s: cites %s (%s:%d), %s", label, id, f.File, f.Line, reason), "cite a verified, external, or vendor fact")
	}
	if len(found) == 0 {
		return
	}
	where := make([]string, len(found))
	for i, f := range found {
		where[i] = fmt.Sprintf("%s, %s:%d", f.ID, f.File, f.Line)
	}
	for _, t := range extract(text, c.heads) {
		contained := false
		for _, f := range found {
			whole := strings.Join(strings.Fields(f.Text+" "+strings.Join(f.Sources, " ")), " ")
			if contains(f.Text, whole, t) {
				contained = true
				break
			}
		}
		if !contained {
			c.brief(line, fmt.Sprintf("%s: the %s %q is in no cited fact (cites %s)", label, t.kind, t.value, strings.Join(where, "; ")), "cite the fact that states it, or take it out of the sentence")
		}
	}
}

// pageProse is the page's text less everything the brief does not classify, normalized as
// coverage.Normalize does: front matter, fenced code, HTML comments, details and summary tags,
// images, and heading lines.
func pageProse(markdown string) string {
	s := strings.ReplaceAll(markdown, "\r\n", "\n")
	s = frontMatterRe.ReplaceAllString(s, "")
	s = stripFences(s)
	s = commentRe.ReplaceAllString(s, " ")
	s = detailsTagRe.ReplaceAllString(s, " ")
	s = imageRe.ReplaceAllString(s, " ")
	s = headingRe.ReplaceAllString(s, " ")
	return coverage.Normalize(s)
}

// stripFences drops each fenced code block, an opening run of three or more backticks or tildes to
// a line that closes with at least as long a run. An unclosed fence stays as prose.
func stripFences(s string) string {
	lines := strings.Split(s, "\n")
	var out []string
	for i := 0; i < len(lines); i++ {
		if run := fenceRun(lines[i]); run != "" {
			closer := -1
			for j := i + 1; j < len(lines); j++ {
				t := strings.TrimSpace(lines[j])
				if strings.HasPrefix(t, run) && strings.Trim(t, run[:1]) == "" {
					closer = j
					break
				}
			}
			if closer >= 0 {
				out = append(out, "")
				i = closer
				continue
			}
		}
		out = append(out, lines[i])
	}
	return strings.Join(out, "\n")
}

// fenceRun returns the opening fence run of line, or "" when line opens none.
func fenceRun(line string) string {
	t := strings.TrimLeft(line, " \t")
	if t == "" || (t[0] != '`' && t[0] != '~') {
		return ""
	}
	run := t[:len(t)-len(strings.TrimLeft(t, t[:1]))]
	if len(run) < 3 {
		return ""
	}
	return run
}

// hasContent reports whether a leftover fragment holds a letter or digit once list numbers and
// Markdown punctuation are set aside.
func hasContent(fragment string) bool {
	var words []string
	for w := range strings.FieldsSeq(fragment) {
		if !listNumberRe.MatchString(w) {
			words = append(words, w)
		}
	}
	for _, r := range strings.Join(words, "") {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// briefLine returns the 1-based line of the brief holding text JSON-encoded, or 1.
func (c *checker) briefLine(text string) int {
	want := encode(text)
	for i, l := range c.briefLines {
		if strings.Contains(l, want) {
			return i + 1
		}
	}
	return 1
}

// pageLine returns the 1-based line of the page whose normalized text holds the first five words
// of fragment, or 1.
func (c *checker) pageLine(fragment string) int {
	words := strings.Fields(fragment)
	want := strings.Join(words[:min(5, len(words))], " ")
	for i, l := range c.pageLines {
		if strings.Contains(coverage.Normalize(l), want) {
			return i + 1
		}
	}
	return 1
}

// encode returns v as JSON without escaping <, >, or &.
func encode(v any) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return ""
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// preview flattens text to one line of at most 60 characters.
func preview(text string) string {
	flat := strings.Join(strings.Fields(text), " ")
	if r := []rune(flat); len(r) > 60 {
		return string(r[:57]) + "..."
	}
	return flat
}

func (c *checker) brief(line int, msg, fix string) { c.add(c.in.BriefFile, line, msg, fix) }

func (c *checker) page(line int, msg, fix string) { c.add(c.in.Page, line, msg, fix) }

func (c *checker) add(file string, line int, msg, fix string) {
	c.res.Findings = append(c.res.Findings, plan.Finding{
		File: file, Line: line, Severity: plan.SeverityError, Check: CheckName, Message: msg, Fix: fix,
	})
}
