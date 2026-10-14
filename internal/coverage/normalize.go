// Package coverage checks a drafted page against its plan.
//
// The mechanical checks (Check) place a plan's fact, verbatim, and link items on the page by the
// page's own headings. The coverage read (Prompt, Schema, Verify) asks a model to quote the
// passage that carries each claim, handoff, and reason item, and Verify accepts a quote only when
// it occurs inside the section the plan names. A text is located by its Normalize form, so
// Markdown emphasis and code delimiters never decide a match.
package coverage

import "strings"

// Normalize returns s with each run of whitespace collapsed to one space, trimmed, and the
// Markdown emphasis and code delimiters (*, _, and `) removed.
func Normalize(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '*' || r == '_' || r == '`' {
			return -1
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}
