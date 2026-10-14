package edits

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
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
// A refused answer returns a *Refusal listing every problem found and
// applies nothing.
func Apply(page, brief string, fs []Finding, prior []Resolution, a Answer) (Applied, error) {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

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
