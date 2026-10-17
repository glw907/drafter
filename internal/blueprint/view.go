package blueprint

import (
	"regexp"
	"slices"

	"github.com/glw907/drafter/internal/plan"
)

var codeSpan = regexp.MustCompile("`([^`\n]+)`")

// framingParts lists the framing fields in order, by their yaml names.
func framingParts(f *Framing) []struct{ name, text string } {
	return []struct{ name, text string }{
		{"readers", f.Readers}, {"background", f.Background}, {"placeInDocSet", f.PlaceInDocSet}, {"introPlan", f.IntroPlan},
	}
}

// View projects a blueprint and its outline entry onto a plan, keeping line numbers, so the plan
// checks, the consistency prompt, and the coverage split run on a blueprint unchanged. The page's
// path, title, and type come from the entry, and a finding on one cites line 1 of the blueprint.
//
// Each framing part that cites facts becomes one example item named _framing-<part>, appended to
// the first section, whose facts are the part's citations and whose line is the part's. A
// backticked token is a citation when facts.Cites says so; with a nil facts store no part cites
// anything.
func View(b *Blueprint, entry Page, facts *plan.Facts) *plan.Plan {
	p := &plan.Plan{
		Version:       plan.Version,
		Page:          plan.Page{Path: entry.Path, Title: entry.Title, Type: entry.Type, Line: 1, Lines: map[string]int{"path": 1, "title": 1, "type": 1}},
		Argument:      b.Argument,
		Introduction:  b.Introduction,
		OrderReason:   b.OrderReason,
		Preconditions: b.Preconditions,
		Dispositions:  b.Dispositions,
		Constraints:   b.Constraints,
		Line:          b.Line,
		Lines:         b.Lines,
	}
	for _, s := range b.Sections {
		ps := plan.Section{Heading: s.Heading, Level: s.Level, Anchor: s.Anchor, Opens: s.Opens, Line: s.Line, Lines: s.Lines}
		for _, it := range s.Items {
			ps.Items = append(ps.Items, planItem(it))
		}
		p.Sections = append(p.Sections, ps)
	}
	if facts == nil || len(p.Sections) == 0 {
		return p
	}
	for _, part := range framingParts(&b.Framing) {
		var cited []string
		for _, m := range codeSpan.FindAllStringSubmatch(part.text, -1) {
			if facts.Cites(m[1]) && !slices.Contains(cited, m[1]) {
				cited = append(cited, m[1])
			}
		}
		if len(cited) == 0 {
			continue
		}
		line := b.Framing.Lines[part.name]
		p.Sections[0].Items = append(p.Sections[0].Items, plan.Item{
			ID: "_framing-" + part.name, Kind: "example", Text: part.text, Facts: cited, Line: line,
			Lines: map[string]int{"id": line, "kind": line, "text": line, "facts": line},
		})
	}
	return p
}

func planItem(it Item) plan.Item {
	return plan.Item{
		ID: it.ID, Kind: it.Kind, Fact: it.Fact, Use: it.Use, Mode: it.Mode, Text: it.Text, Facts: it.Facts,
		Source: it.Source, Target: it.Target, Requires: it.Requires, Produces: it.Produces, CanFail: it.CanFail,
		Optional: it.Optional, From: it.From, Symptom: it.Symptom, Cause: it.Cause, HandledBy: it.HandledBy,
		Line: it.Line, Lines: it.Lines,
	}
}
