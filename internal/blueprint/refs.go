package blueprint

import (
	"cmp"
	"encoding/json"
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
// block's is reader with kind reader and the :::constraints block's is constraints with kind
// constraints. A Code: bullet's kind is code and a Figure: bullet's is figure.
type Ref struct {
	File string
	Line int
	Item string
	Kind string
}

// Hit is a Ref with the context a correction needs. Section is the id of the section the line sits
// in, which is the section a correction redrafts; it is empty for the introduction, the reader and
// constraints blocks, framing citations, and dispositions. A hit from a brief has the kind brief,
// the item id s<n> for the n-th sentence, and File and Brief both naming the brief file. Page is
// the page the brief belongs to and Sentence the text of the citing sentence. Line is the line of
// the sentence in the brief, or 0 when it could not be found.
type Hit struct {
	Ref
	Section  string
	Brief    string
	Page     string
	Sentence string
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
	hits, held, err := p.blueprintHits(l, id)
	for _, h := range hits {
		refs = append(refs, h.Ref)
	}
	return refs, held, err
}

// RefHits is Refs with each place's section, followed by every brief sentence that cites the fact.
// The briefs read are those of the pages the blueprints cover, found through the project's brief
// path; a page with no brief contributes nothing, and a brief that does not parse is skipped.
func (p Project) RefHits(id string) (hits []Hit, held bool, err error) {
	l, err := p.load()
	if err != nil {
		return nil, false, err
	}
	hits, held, err = p.blueprintHits(l, id)
	if err != nil {
		return nil, false, err
	}
	id = resolveFact(l.facts, id)
	var seen []string
	for _, rel := range l.files {
		page, ok := PagePath(p.BlueprintRoot, rel)
		if !ok || slices.Contains(seen, page) {
			continue
		}
		seen = append(seen, page)
		hits = append(hits, p.briefHits(l.facts, page, id)...)
	}
	return hits, held, nil
}

func (p Project) blueprintHits(l *loaded, id string) (hits []Hit, held bool, err error) {
	id = resolveFact(l.facts, id)
	for _, rel := range l.files {
		data, err := os.ReadFile(p.abs(rel))
		if err != nil {
			return nil, false, err
		}
		src, _ := Load(rel, data)
		switch b := src.(type) {
		case *Markdown:
			hits = append(hits, markdownRefs(rel, b, l.facts, id)...)
		case *Blueprint:
			pagePath, _ := PagePath(p.BlueprintRoot, rel)
			pg, ok := l.pages[pagePath]
			if !ok {
				pg = Page{Path: pagePath}
			}
			hits = append(hits, refsIn(rel, View(b, pg, l.facts), b, id)...)
		}
	}
	slices.SortStableFunc(hits, func(a, b Hit) int {
		return cmp.Or(cmp.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line))
	})
	return hits, l.facts != nil && l.facts.Count(id) > 0, nil
}

// briefHits lists the sentences of the page's brief that cite id. A sentence's id field is a
// string or a list of strings.
func (p Project) briefHits(facts *plan.Facts, page, id string) []Hit {
	rel := BriefPath(p.BlueprintRoot, p.BriefPath, page)
	data, err := os.ReadFile(p.abs(rel))
	if err != nil {
		return nil
	}
	var brief struct {
		Sentences []struct {
			Text string          `json:"text"`
			ID   json.RawMessage `json:"id"`
		} `json:"sentences"`
	}
	if json.Unmarshal(data, &brief) != nil {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	var hits []Hit
	for i, s := range brief.Sentences {
		var ids []string
		if json.Unmarshal(s.ID, &ids) != nil {
			var one string
			if json.Unmarshal(s.ID, &one) != nil {
				continue
			}
			ids = []string{one}
		}
		cites := slices.ContainsFunc(ids, func(c string) bool {
			return strings.TrimPrefix(resolveFact(facts, c), "f:") == strings.TrimPrefix(id, "f:")
		})
		if !cites {
			continue
		}
		var quoted strings.Builder
		enc := json.NewEncoder(&quoted)
		enc.SetEscapeHTML(false) // the brief file holds < and > as written
		_ = enc.Encode(s.Text)
		needle := strings.TrimSuffix(quoted.String(), "\n")
		line := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, needle) }) + 1
		hits = append(hits, Hit{Ref: Ref{rel, line, fmt.Sprintf("s%d", i+1), "brief"}, Brief: rel, Page: page, Sentence: s.Text})
	}
	return hits
}

// markdownRefs lists each line of m that carries a :fact tag for id, once per line.
func markdownRefs(file string, m *Markdown, facts *plan.Facts, id string) []Hit {
	cites := func(tag string) bool {
		return strings.TrimPrefix(resolveFact(facts, tag), "f:") == strings.TrimPrefix(id, "f:")
	}
	var hits []Hit
	for _, c := range m.Reader.Cites {
		if cites(c.ID) {
			hits = append(hits, Hit{Ref: Ref{file, c.Line, "reader", "reader"}})
		}
	}
	for _, c := range m.Constraints.Cites {
		if cites(c.ID) {
			hits = append(hits, Hit{Ref: Ref{file, c.Line, "constraints", "constraints"}})
		}
	}
	for _, s := range m.Sections {
		for i, l := range s.Lines {
			if slices.ContainsFunc(l.Facts, cites) {
				hits = append(hits, Hit{Ref: Ref{file, l.Line, fmt.Sprintf("%s.p%d", s.ID, i+1), cmp.Or(l.Kind, "example")}, Section: s.ID})
			}
		}
		for i, r := range s.Rows {
			var seen []int
			for _, c := range r.Cites {
				if cites(c.ID) && !slices.Contains(seen, c.Line) {
					seen = append(seen, c.Line)
					hits = append(hits, Hit{Ref: Ref{file, c.Line, fmt.Sprintf("%s.row%d", s.ID, i+1), "example"}, Section: s.ID})
				}
			}
		}
	}
	return hits
}

// refsIn lists the places v, the view of blueprint b, cites id. View keeps b's section order and
// files the framing citations under the first section, which they do not belong to.
func refsIn(file string, v *plan.Plan, b *Blueprint, id string) []Hit {
	var hits []Hit
	if slices.Contains(v.Introduction.Facts, id) {
		hits = append(hits, Hit{Ref: Ref{file, v.Introduction.Lines["facts"], "introduction", "introduction"}})
	}
	for i, s := range v.Sections {
		for _, it := range s.Items {
			section := b.Sections[i].ID
			if strings.HasPrefix(it.ID, "_framing-") {
				section = ""
			}
			if it.Kind == "fact" && it.Fact == id {
				hits = append(hits, Hit{Ref: Ref{file, it.Lines["fact"], it.ID, it.Kind}, Section: section})
			}
			if slices.Contains(it.Facts, id) {
				hits = append(hits, Hit{Ref: Ref{file, it.Lines["facts"], it.ID, it.Kind}, Section: section})
			}
		}
	}
	for _, d := range v.Dispositions {
		if d.Fact == id {
			hits = append(hits, Hit{Ref: Ref{file, d.Lines["fact"], "dispositions", "disposition"}})
		}
	}
	return hits
}
