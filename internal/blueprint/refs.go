package blueprint

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"regexp"
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
// <section id>.p<n>, a row's is <section id>.row<n>, both of kind bullet, and the :::reader
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
// The briefs read are those of every page the outline lists and of every page a blueprint covers,
// found through the project's brief path, so a published page's brief counts before its blueprint
// exists; a page with no brief contributes nothing, and a brief that does not parse is skipped.
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
	var pages []string
	for _, pg := range l.outline.Pages {
		pages = append(pages, path.Clean(pg.Path))
	}
	for _, rel := range l.files {
		if page, ok := PagePath(p.BlueprintRoot, rel); ok {
			pages = append(pages, page)
		}
	}
	var seen []string
	for _, page := range pages {
		if slices.Contains(seen, page) {
			continue
		}
		seen = append(seen, page)
		hits = append(hits, p.briefHits(l.facts, page, id)...)
	}
	return hits, held, nil
}

// PageRefs lists each link item of every blueprint, and each outline crossLinks entry, whose
// target is the page that ref names by slug or path. A blueprint link is a See or link item whose
// target, resolved from the blueprint's page, is that page; a crossLinks entry is one whose to
// names it, and sits in the outline file. The kind is link or crossLink, the item of a link is
// its item id, and results are sorted by file, then line. Listed is false when the outline lists
// no such page.
func (p Project) PageRefs(ref string) (refs []Ref, listed bool, err error) {
	l, err := p.load()
	if err != nil {
		return nil, false, err
	}
	target, ok := l.resolvePage(ref)
	if !ok {
		return nil, false, nil
	}
	for _, rel := range l.files {
		data, err := os.ReadFile(p.abs(rel))
		if err != nil {
			return nil, false, err
		}
		src, _ := Load(rel, data)
		if src == nil {
			continue
		}
		from, _ := PagePath(p.BlueprintRoot, rel)
		pg, ok := l.pages[from]
		if !ok {
			pg = Page{Path: from}
		}
		for _, s := range src.View(pg, l.facts).Sections {
			for _, it := range s.Items {
				file, _, _ := strings.Cut(it.Target, "#")
				if it.Kind != "link" || file == "" || strings.Contains(file, "://") {
					continue
				}
				dest := path.Join(path.Dir(from), file)
				if rest, ok := strings.CutPrefix(file, "/"); ok {
					dest = path.Clean(rest)
				}
				if dest == target {
					refs = append(refs, Ref{rel, it.Lines["target"], it.ID, "link"})
				}
			}
		}
	}
	for _, c := range l.outline.CrossLinks {
		if to, ok := l.resolvePage(c.To); ok && to == target {
			refs = append(refs, Ref{p.Outline, c.Line, "crossLinks", "crossLink"})
		}
	}
	slices.SortStableFunc(refs, func(a, b Ref) int {
		return cmp.Or(cmp.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line))
	})
	return refs, true, nil
}

// resolvePage returns the path of the outline page that ref names, by path or by slug.
func (l *loaded) resolvePage(ref string) (string, bool) {
	if pg, ok := l.pages[path.Clean(ref)]; ok {
		return pg.Path, true
	}
	for _, pg := range l.pages {
		if pg.Slug != "" && pg.Slug == ref {
			return pg.Path, true
		}
	}
	return "", false
}

var sourceURL = regexp.MustCompile("https?://[^\\s`)\\]>]+")

// SourceKinds names the kinds of source the fact's Source: part holds, in the order code, url,
// prose: code for a pointer that is a repository path, url for a URL, and prose for a pointer to
// a .md file or under the project's facts.prose_sources. A fact the store does not hold, or a
// project with no facts store, has none.
func (p Project) SourceKinds(id string) ([]string, error) {
	if p.FactsDir == "" {
		return nil, nil
	}
	facts, err := plan.LoadFacts(p.FactsDir)
	if err != nil {
		return nil, err
	}
	fact, ok := facts.Get(resolveFact(facts, id))
	if !ok {
		return nil, nil
	}
	var code, prose bool
	for _, src := range fact.Sources {
		file, _, _ := strings.Cut(src, ":")
		file, _, _ = strings.Cut(file, "#")
		file = path.Clean(file)
		if strings.HasSuffix(file, ".md") || slices.ContainsFunc(p.ProseSources, func(ps string) bool { return path.Clean(ps) == file }) {
			prose = true
		} else {
			code = true
		}
	}
	_, afterSource, _ := strings.Cut(fact.Raw, "Source:")
	var kinds []string
	if code {
		kinds = append(kinds, "code")
	}
	if sourceURL.MatchString(afterSource) {
		kinds = append(kinds, "url")
	}
	if prose {
		kinds = append(kinds, "prose")
	}
	return kinds, nil
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
				hits = append(hits, Hit{Ref: Ref{file, l.Line, fmt.Sprintf("%s.p%d", s.ID, i+1), cmp.Or(l.Kind, "bullet")}, Section: s.ID})
			}
		}
		for i, r := range s.Rows {
			var seen []int
			for _, c := range r.Cites {
				if cites(c.ID) && !slices.Contains(seen, c.Line) {
					seen = append(seen, c.Line)
					hits = append(hits, Hit{Ref: Ref{file, c.Line, fmt.Sprintf("%s.row%d", s.ID, i+1), "bullet"}, Section: s.ID})
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
