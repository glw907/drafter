package edits

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/glw907/drafter/internal/coverage"
)

// Applied is the result of an accepted answer: the edited texts and the
// union of the prior and new resolutions, ordered by finding number.
type Applied struct {
	Page        string
	Brief       string
	Resolutions []Resolution
}

// Apply applies a's edits to page and brief in order and checks the answer
// against the edit contract. The rules on a's own edits and resolutions
// are checked on a alone; the rule that every finding has a resolution is
// checked over the union of prior and a's resolutions, where a later
// resolution of a finding overrides an earlier one. The brief's JSON
// validity is checked only when the answer edits the brief.
//
// With a non-nil b, an answer whose edits, once applied, change the page or
// brief outside b's allowed sections is refused as well. The bound is
// checked only when every edit applied.
//
// A refused answer returns a *Refusal listing every problem found and
// applies nothing.
func Apply(page, brief string, fs []Finding, prior []Resolution, a Answer, b *Bound) (Applied, error) {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	oldPage, oldBrief := page, brief
	texts := map[string]*string{FilePage: &page, FileBrief: &brief}
	briefEdited := false
	for i, e := range a.Edits {
		text, ok := texts[e.File]
		if !ok {
			add("edit %d: file %q is neither \"page\" nor \"brief\"", i, e.File)
			continue
		}
		if e.Old == e.New {
			add("edit %d: old and new text are equal", i)
			continue
		}
		n := 0
		if e.Old != "" {
			n = strings.Count(*text, e.Old)
		}
		switch {
		case n == 0:
			add("edit %d: old text not found in the %s", i, e.File)
		case n > 1:
			add("edit %d: old text occurs %d times in the %s; it must occur exactly once", i, n, e.File)
		default:
			*text = strings.Replace(*text, e.Old, e.New, 1)
			briefEdited = briefEdited || e.File == FileBrief
		}
	}
	if b != nil && len(problems) == 0 {
		problems = append(problems, b.problems(oldPage, oldBrief, page, brief)...)
	}

	known := make(map[int]Finding, len(fs))
	for _, f := range fs {
		known[f.N] = f
	}
	claimed := make([]bool, len(a.Edits))
	seen := make(map[int]bool, len(a.Resolutions))
	for _, r := range a.Resolutions {
		f, ok := known[r.Finding]
		if !ok {
			add("resolution names finding %d, which does not exist", r.Finding)
		}
		if seen[r.Finding] {
			add("finding %d has more than one resolution in this answer", r.Finding)
		}
		seen[r.Finding] = true
		for _, idx := range r.Edits {
			if idx < 0 || idx >= len(a.Edits) {
				add("resolution for finding %d names edit %d, which does not exist (this answer has %d edits, indexed from 0)", r.Finding, idx, len(a.Edits))
				continue
			}
			claimed[idx] = true
		}
		if ok && f.Blocking && r.Status == StatusEdited && len(r.Edits) == 0 {
			add("finding %d is blocking and resolved edited but names no edit", r.Finding)
		}
	}
	for i, c := range claimed {
		if !c {
			add("edit %d: no resolution claims it", i)
		}
	}

	union := make(map[int]Resolution, len(prior)+len(a.Resolutions))
	for _, r := range prior {
		union[r.Finding] = r
	}
	for _, r := range a.Resolutions {
		union[r.Finding] = r
	}
	for _, f := range fs {
		if _, ok := union[f.N]; !ok {
			add("finding %d has no resolution", f.N)
		}
	}

	if briefEdited && !json.Valid([]byte(brief)) {
		add("the brief is not valid JSON after the edits")
	}

	if len(problems) > 0 {
		return Applied{}, &Refusal{Problems: problems}
	}

	merged := make([]Resolution, 0, len(union))
	for _, r := range union {
		merged = append(merged, r)
	}
	slices.SortFunc(merged, func(x, y Resolution) int { return x.Finding - y.Finding })
	return Applied{Page: page, Brief: brief, Resolutions: merged}, nil
}

// problems lists how the edits took the page or brief, from oldPage and
// oldBrief to newPage and newBrief, outside the sections b allows. Every
// heading must be found after the edits exactly when it was found before,
// and no allowed section may gain a heading line that a later section's
// heading matches, so the sections still cut the same way. The preamble,
// every heading line, and every section b does not allow must then be
// identical line for line. A brief sentence located outside the allowed
// sections, and in none of them, must keep its text and id, and no
// sentence may be added whose text lies only outside them. A brief that is
// not valid JSON on either side is left to the JSON check.
func (b *Bound) problems(oldPage, oldBrief, newPage, newBrief string) []string {
	allowed := make([]bool, len(b.Headings))
	var names []string
	for i, h := range b.Headings {
		if slices.Contains(b.Allowed, h) {
			allowed[i] = true
			names = append(names, fmt.Sprintf("%q", h))
		}
	}
	scope := "this answer may change only the sections " + strings.Join(names, ", ")
	if len(names) == 0 {
		scope = "this answer may change no section"
	}

	oldLines, newLines := strings.Split(oldPage, "\n"), strings.Split(newPage, "\n")
	oldPre, oldSecs := coverage.Spans(b.Headings, []byte(oldPage))
	newPre, newSecs := coverage.Spans(b.Headings, []byte(newPage))
	var out []string
	for i, h := range b.Headings {
		switch o, n := oldSecs[i], newSecs[i]; {
		case o.Found && !n.Found:
			out = append(out, fmt.Sprintf("the edits remove the heading of section %q or hide it in a code fence; %s", h, scope))
		case !o.Found && n.Found:
			out = append(out, fmt.Sprintf("the edits add a heading for section %q, which the page did not have; %s", h, scope))
		case n.Found && allowed[i]:
			_, later := coverage.Spans(b.Headings[i+1:], []byte(strings.Join(newLines[n.Start:n.End], "\n")))
			for j, s := range later {
				if s.Found {
					out = append(out, fmt.Sprintf("the edits add a heading to section %q that repeats the heading of section %q; %s", h, b.Headings[i+1+j], scope))
					break
				}
			}
		}
	}
	if len(out) > 0 {
		return out
	}

	if d, ok := firstDiff(oldLines[oldPre.Start:oldPre.End], newLines[newPre.Start:newPre.End], oldPre.Start, newPre.Start); ok {
		out = append(out, fmt.Sprintf("the edits change the text above the first section, where %s; %s", d, scope))
	}
	for i, h := range b.Headings {
		o, n := oldSecs[i], newSecs[i]
		if !o.Found {
			continue
		}
		if oldLines[o.Heading] != newLines[n.Heading] {
			out = append(out, fmt.Sprintf("the edits change the heading line of section %q, where line %d now reads %q; %s", h, n.Heading+1, newLines[n.Heading], scope))
		}
		if allowed[i] {
			continue
		}
		if d, ok := firstDiff(oldLines[o.Start:o.End], newLines[n.Start:n.End], o.Start, n.Start); ok {
			out = append(out, fmt.Sprintf("the edits change section %q, where %s; %s", h, d, scope))
		}
	}

	oldSent, okOld := briefSentences(oldBrief)
	newSent, okNew := briefSentences(newBrief)
	if !okOld || !okNew {
		return out
	}
	oldWhere := locator(oldLines, oldPre, oldSecs, allowed)
	newWhere := locator(newLines, newPre, newSecs, allowed)
	have := map[sentence]int{}
	for _, s := range newSent {
		have[s]++
	}
	for _, s := range oldSent {
		if !oldWhere(s.text) {
			continue
		}
		if have[s] == 0 {
			out = append(out, fmt.Sprintf("the edits change or remove the brief sentence %q, which lies outside the sections this answer may change; %s", s.text, scope))
			continue
		}
		have[s]--
	}
	had := map[sentence]int{}
	for _, s := range oldSent {
		had[s]++
	}
	for _, s := range newSent {
		if had[s] > 0 {
			had[s]--
			continue
		}
		if newWhere(s.text) {
			out = append(out, fmt.Sprintf("the edits add the brief sentence %q, which lies only outside the sections this answer may change; %s", s.text, scope))
		}
	}
	return out
}

// firstDiff describes the first line where new differs from old, the lines
// of one part of the page before and after the edits, which begin at the
// 0-based page lines oldAt and newAt.
func firstDiff(old, new []string, oldAt, newAt int) (string, bool) {
	for i := range max(len(old), len(new)) {
		switch {
		case i >= len(new):
			return fmt.Sprintf("line %d, %q, is removed", oldAt+i+1, old[i]), true
		case i >= len(old) || old[i] != new[i]:
			return fmt.Sprintf("line %d now reads %q", newAt+i+1, new[i]), true
		}
	}
	return "", false
}

// sentence is one brief sentence as the bound compares it: its normalized
// text and its id as compact JSON.
type sentence struct{ text, id string }

func briefSentences(brief string) ([]sentence, bool) {
	var b struct {
		Sentences []struct {
			Text string          `json:"text"`
			ID   json.RawMessage `json:"id"`
		} `json:"sentences"`
	}
	if err := json.Unmarshal([]byte(brief), &b); err != nil {
		return nil, false
	}
	out := make([]sentence, 0, len(b.Sentences))
	for _, s := range b.Sentences {
		var id bytes.Buffer
		if len(s.ID) > 0 && json.Compact(&id, s.ID) != nil {
			return nil, false
		}
		out = append(out, sentence{coverage.Normalize(s.Text), id.String()})
	}
	return out, true
}

// locator returns a function that reports whether a normalized sentence
// lies outside the allowed sections, in the preamble or a section the bound
// does not allow, and in none of the allowed ones.
func locator(lines []string, pre coverage.Span, secs []coverage.Span, allowed []bool) func(string) bool {
	norm := func(s coverage.Span) string { return coverage.Normalize(strings.Join(lines[s.Start:s.End], "\n")) }
	outside := []string{norm(pre)}
	var inside []string
	for i, s := range secs {
		switch {
		case !s.Found:
		case allowed[i]:
			inside = append(inside, norm(s))
		default:
			outside = append(outside, norm(s))
		}
	}
	return func(text string) bool {
		if text == "" {
			return false
		}
		in := func(parts []string) bool {
			return slices.ContainsFunc(parts, func(p string) bool { return strings.Contains(p, text) })
		}
		return in(outside) && !in(inside)
	}
}
