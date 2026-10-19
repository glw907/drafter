package coverage

import (
	"regexp"
	"strings"

	"github.com/glw907/drafter/internal/plan"
)

var (
	headingLine = regexp.MustCompile(`^ {0,3}#{1,6}[ \t]+(.+?)[ \t]*$`)
	fenceLine   = regexp.MustCompile("^ {0,3}(```|~~~)")
)

// region is the page text that belongs to one plan section: everything below its heading up to the
// next planned heading, so an unplanned heading never splits it.
type region struct {
	heading string
	found   bool
	raw     string
	norm    string
}

// Span is where one part of a page lies, as 0-based indexes into the page's lines, the page split
// on "\n": the heading's line, and the lines below it from Start up to End, exclusive. The
// preamble's Span is always Found, with Heading -1. A section whose heading is not found has Found
// false, Heading -1, and no lines.
type Span struct {
	Found               bool
	Heading, Start, End int
}

// Spans cuts page by the given section headings as Regions does and returns where each part lies:
// the preamble, then one Span per heading, in order.
func Spans(headings []string, page []byte) (preamble Span, sections []Span) {
	return spans(headings, strings.Split(string(page), "\n"))
}

// spans is the one rule every cut of a page follows. A heading matches the first page heading at
// or after the previous match whose normalized text equals its own; headings inside code fences
// are ignored. A section runs to the next matched heading.
func spans(headings, lines []string) (preamble Span, sections []Span) {
	type heading struct {
		line int
		text string
	}
	var heads []heading
	fenced := false
	for i, l := range lines {
		if fenceLine.MatchString(l) {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		if m := headingLine.FindStringSubmatch(l); m != nil {
			heads = append(heads, heading{i, Normalize(m[1])})
		}
	}

	sections = make([]Span, len(headings))
	next := 0
	for i, h := range headings {
		sections[i].Heading = -1
		want := Normalize(h)
		for j := next; j < len(heads); j++ {
			if heads[j].text == want {
				sections[i].Heading = heads[j].line
				sections[i].Found = true
				next = j + 1
				break
			}
		}
	}

	first := len(lines)
	for _, s := range sections {
		if s.Found {
			first = s.Heading
			break
		}
	}
	preamble = Span{Found: true, Heading: -1, End: first}
	for i := range sections {
		if !sections[i].Found {
			continue
		}
		end := len(lines)
		for _, s := range sections[i+1:] {
			if s.Found {
				end = s.Heading
				break
			}
		}
		sections[i].Start, sections[i].End = sections[i].Heading+1, end
	}
	return preamble, sections
}

// split cuts page into one region per plan section, in plan order, and the preamble above the
// first planned heading.
func split(p *plan.Plan, page []byte) (preamble region, regions []region) {
	headings := make([]string, len(p.Sections))
	for i, s := range p.Sections {
		headings[i] = s.Heading
	}
	lines := strings.Split(string(page), "\n")
	pre, sections := spans(headings, lines)
	fill := func(r *region, s Span) {
		r.raw = strings.Join(lines[s.Start:s.End], "\n")
		r.norm = Normalize(r.raw)
	}
	fill(&preamble, pre)
	regions = make([]region, len(sections))
	for i, s := range sections {
		regions[i].heading = headings[i]
		if s.Found {
			regions[i].found = true
			fill(&regions[i], s)
		}
	}
	return preamble, regions
}

// Regions cuts page by the given section headings in the way the checks here do, and returns the
// text above the first matched heading, the text under each heading, and whether each heading was
// found. The text under a heading runs to the next matched heading and omits the heading line;
// a heading that is not found has no text.
func Regions(headings []string, page []byte) (preamble string, text []string, found []bool) {
	lines := strings.Split(string(page), "\n")
	pre, sections := spans(headings, lines)
	text = make([]string, len(sections))
	found = make([]bool, len(sections))
	for i, s := range sections {
		if s.Found {
			text[i], found[i] = strings.Join(lines[s.Start:s.End], "\n"), true
		}
	}
	return strings.Join(lines[pre.Start:pre.End], "\n"), text, found
}
