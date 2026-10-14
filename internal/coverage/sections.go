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

// split cuts page into one region per plan section, in plan order, and the preamble above the
// first planned heading. A plan section matches the first page heading at or after the previous
// match whose normalized text equals its own; headings inside code fences are ignored.
func split(p *plan.Plan, page []byte) (preamble region, regions []region) {
	lines := strings.Split(string(page), "\n")
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

	regions = make([]region, len(p.Sections))
	starts := make([]int, len(p.Sections))
	next := 0
	for i, s := range p.Sections {
		regions[i].heading = s.Heading
		starts[i] = -1
		want := Normalize(s.Heading)
		for j := next; j < len(heads); j++ {
			if heads[j].text == want {
				starts[i] = heads[j].line
				regions[i].found = true
				next = j + 1
				break
			}
		}
	}

	fill := func(r *region, from, to int) {
		r.raw = strings.Join(lines[from:to], "\n")
		r.norm = Normalize(r.raw)
	}
	first := len(lines)
	for _, s := range starts {
		if s >= 0 {
			first = s
			break
		}
	}
	fill(&preamble, 0, first)
	for i := range regions {
		if !regions[i].found {
			continue
		}
		end := len(lines)
		for _, s := range starts[i+1:] {
			if s >= 0 {
				end = s
				break
			}
		}
		fill(&regions[i], starts[i]+1, end)
	}
	return preamble, regions
}
