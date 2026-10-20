package pipeline

import (
	"strings"

	"github.com/glw907/drafter/internal/edits"
)

// smallKinds are the finding kinds one scoped fix may answer without another review: a link, a
// handoff, a heading, or a brief citation.
var smallKinds = []string{"link", "handoff", "heading", "citation"}

// isSmall reports whether kind is a small kind. Structured outputs do not promise an enum value's
// capitalization, so the match ignores case. An empty or unknown kind, as in a state saved before
// kinds existed, is not small.
func isSmall(kind string) bool {
	for _, k := range smallKinds {
		if strings.EqualFold(kind, k) {
			return true
		}
	}
	return false
}

// allSmall reports whether every finding of fs is small. It is true for an empty fs, so a caller
// checks for at least one finding first.
func allSmall(fs []edits.Finding) bool {
	for _, f := range fs {
		if !isSmall(f.Kind) {
			return false
		}
	}
	return true
}

func blockingOnly(fs []edits.Finding) []edits.Finding {
	var out []edits.Finding
	for _, f := range fs {
		if f.Blocking {
			out = append(out, f)
		}
	}
	return out
}

// openSmall opens the small exchange on fs, numbered afresh, in place of an escalation. Its gate
// and coverage check are the only guards on what it writes: after it the page accepts or
// escalates, and no path leads to a second small exchange.
func (p *page) openSmall(fs []edits.Finding) {
	for i := range fs {
		fs[i].N = i + 1
	}
	p.w.Ex = &exchange{Kind: exSmall, Findings: fs}
	p.w.Rec.findings(s6b, fs)
	p.w.Got = nil
	p.ps.Step = stSmall
}
