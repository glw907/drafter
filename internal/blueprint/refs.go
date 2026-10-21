package blueprint

import (
	"cmp"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/glw907/drafter/internal/plan"
)

// Ref is one place a blueprint places or cites a fact: the blueprint file, the line of the field
// that names the fact, and the id and kind of the item that holds it. A framing citation has the
// item id _framing-<part> and kind example, the introduction's facts list has the id introduction
// and kind introduction, and a disposition has the id dispositions and kind disposition.
//
// In a markdown blueprint the line is the one that carries the :fact tag. A bullet's item id is
// <section id>.p<n>, a row's is <section id>.row<n>, both of kind example, and the :::reader
// block's is reader with kind reader.
type Ref struct {
	File string
	Line int
	Item string
	Kind string
}

// Refs lists every place, in every blueprint under the blueprint root, that places or cites the
// fact id: fact items, every facts list, the introduction's facts, dispositions, and framing
// citations, or in a markdown blueprint every :fact tag. The id may be written with or without the
// store's "f:" prefix. Results come back sorted by file, then line. Held reports whether the
// facts store holds a bullet with the id, so a caller can tell an unknown id from an unreferenced
// one; it is false when the project has no facts store.
//
// A blueprint that fails to decode is read as far as it decoded, since a fact's placements matter
// most while the blueprints are changing. The error means the project's shared inputs could not
// be read, as for Check.
func (p Project) Refs(id string) (refs []Ref, held bool, err error) {
	l, err := p.load()
	if err != nil {
		return nil, false, err
	}
	id = resolveFact(l.facts, id)
	for _, rel := range l.files {
		data, err := os.ReadFile(p.abs(rel))
		if err != nil {
			return nil, false, err
		}
		src, _ := Load(rel, data)
		switch b := src.(type) {
		case *Markdown:
			refs = append(refs, markdownRefs(rel, b, l.facts, id)...)
		case *Blueprint:
			pagePath, _ := PagePath(p.BlueprintRoot, rel)
			pg, ok := l.pages[pagePath]
			if !ok {
				pg = Page{Path: pagePath}
			}
			refs = append(refs, refsIn(rel, View(b, pg, l.facts), id)...)
		}
	}
	slices.SortStableFunc(refs, func(a, b Ref) int {
		return cmp.Or(cmp.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line))
	})
	return refs, l.facts != nil && l.facts.Count(id) > 0, nil
}

// markdownRefs lists each line of m that carries a :fact tag for id, once per line.
func markdownRefs(file string, m *Markdown, facts *plan.Facts, id string) []Ref {
	cites := func(tag string) bool {
		return strings.TrimPrefix(resolveFact(facts, tag), "f:") == strings.TrimPrefix(id, "f:")
	}
	var refs []Ref
	for _, c := range m.Reader.Cites {
		if cites(c.ID) {
			refs = append(refs, Ref{file, c.Line, "reader", "reader"})
		}
	}
	for _, s := range m.Sections {
		for i, l := range s.Lines {
			if slices.ContainsFunc(l.Facts, cites) {
				refs = append(refs, Ref{file, l.Line, fmt.Sprintf("%s.p%d", s.ID, i+1), "example"})
			}
		}
		for i, r := range s.Rows {
			var seen []int
			for _, c := range r.Cites {
				if cites(c.ID) && !slices.Contains(seen, c.Line) {
					seen = append(seen, c.Line)
					refs = append(refs, Ref{file, c.Line, fmt.Sprintf("%s.row%d", s.ID, i+1), "example"})
				}
			}
		}
	}
	return refs
}

func refsIn(file string, v *plan.Plan, id string) []Ref {
	var refs []Ref
	if slices.Contains(v.Introduction.Facts, id) {
		refs = append(refs, Ref{file, v.Introduction.Lines["facts"], "introduction", "introduction"})
	}
	for _, s := range v.Sections {
		for _, it := range s.Items {
			if it.Kind == "fact" && it.Fact == id {
				refs = append(refs, Ref{file, it.Lines["fact"], it.ID, it.Kind})
			}
			if slices.Contains(it.Facts, id) {
				refs = append(refs, Ref{file, it.Lines["facts"], it.ID, it.Kind})
			}
		}
	}
	for _, d := range v.Dispositions {
		if d.Fact == id {
			refs = append(refs, Ref{file, d.Lines["fact"], "dispositions", "disposition"})
		}
	}
	return refs
}
