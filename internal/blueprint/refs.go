package blueprint

import (
	"cmp"
	"os"
	"slices"

	"github.com/glw907/drafter/internal/plan"
)

// Ref is one place a blueprint places or cites a fact: the blueprint file, the line of the field
// that names the fact, and the id and kind of the item that holds it. A framing citation has the
// item id _framing-<part> and kind example, the introduction's facts list has the id introduction
// and kind introduction, and a disposition has the id dispositions and kind disposition.
type Ref struct {
	File string
	Line int
	Item string
	Kind string
}

// Refs lists every place, in every blueprint under the blueprint root, that places or cites the
// fact id: fact items, every facts list, the introduction's facts, dispositions, and framing
// citations. Results come back sorted by file, then line. Held reports whether the facts store
// holds a bullet with the id, so a caller can tell an unknown id from an unreferenced one; it is
// false when the project has no facts store.
//
// A blueprint that fails to decode is read as far as it decoded, since a fact's placements matter
// most while the blueprints are changing. The error means the project's shared inputs could not
// be read, as for Check.
func (p Project) Refs(id string) (refs []Ref, held bool, err error) {
	l, err := p.load()
	if err != nil {
		return nil, false, err
	}
	for _, rel := range l.files {
		data, err := os.ReadFile(p.abs(rel))
		if err != nil {
			return nil, false, err
		}
		b, _ := Decode(rel, data)
		if b == nil {
			continue
		}
		pagePath, _ := PagePath(p.BlueprintRoot, rel)
		pg, ok := l.pages[pagePath]
		if !ok {
			pg = Page{Path: pagePath}
		}
		refs = append(refs, refsIn(rel, View(b, pg, l.facts), id)...)
	}
	slices.SortStableFunc(refs, func(a, b Ref) int {
		return cmp.Or(cmp.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line))
	})
	return refs, l.facts != nil && l.facts.Count(id) > 0, nil
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
