package blueprint

import (
	"bytes"
	"maps"
	"slices"

	"github.com/glw907/drafter/internal/plan"
	"go.yaml.in/yaml/v3"
)

// The kinds of Scope.
const (
	// ScopeUnchanged means nothing a read sees differs from the earlier revision.
	ScopeUnchanged = "unchanged"
	// ScopeScoped means the page has an earlier blueprint to diff against and something in it differs.
	ScopeScoped = "scoped"
	// ScopeWhole means the page cannot be diffed, so a read judges all of it.
	ScopeWhole = "whole"
)

// Scope is what changed in one page's blueprint between two revisions. IDs holds the changed
// section ids, in blueprint order, and may be empty for a scoped page: a removed section, a
// reorder, a field outside sections, and an outline-entry change name no section. Facts holds the
// ids of the facts the change touched and is never nil on a scoped page, so a nil Facts marks a
// state recorded before the field existed. Bullets holds the facts whose bullet changed and Joined
// the facts the current blueprint places or cites that the earlier one did not; a conflict between
// facts in neither was already on the page. Both are never nil on a scoped page, and their tags
// carry no omitempty because an empty slice written without its key would read back as nil, the
// mark of a state recorded before the fields existed. Why says what made a page whole.
type Scope struct {
	Kind    string   `json:"kind"`
	IDs     []string `json:"ids,omitempty"`
	Facts   []string `json:"facts"`
	Bullets []string `json:"bullets"`
	Joined  []string `json:"joined"`
	Why     string   `json:"why,omitempty"`
}

// Revision is one version of a page's inputs: its blueprint file, its outline entry (nil when the
// outline did not list the page), and the facts store (nil when the project has none).
type Revision struct {
	Blueprint []byte
	Entry     *Page
	Facts     *plan.Facts
}

// Compare says what changed from old to cur for the blueprint at file. A section is changed when
// its id is new, its decoded content differs, or it places or cites a fact whose bullet changed;
// formatting alone changes nothing. The page is whole only where no diff can be built: the
// blueprint is absent or does not decode at the earlier revision, the current blueprint does not
// decode (which leaves its fault to the blueprint check), or the outline lists the page at neither
// revision. Every other difference makes the page scoped, a removed section, an order change, a
// field outside sections, and an outline-entry change among them.
//
// Scope.Facts is the union, over the old and new versions of every new, removed, or differing item
// (matched by id, which is unique across sections and items) and of the fields outside sections
// when they differ, of the facts each places or cites, plus every fact the page places or cites at
// either revision whose bullet changed. Scope.Bullets lists every fact in either store whose bullet
// changed, cited or not, so a conflict with a changed bullet never reads as already present.
func Compare(file string, old, cur Revision) Scope {
	whole := func(why string) Scope { return Scope{Kind: ScopeWhole, Why: why} }
	ob, ofs := Decode(file, old.Blueprint)
	if ob == nil || plan.HasError(ofs) {
		return whole("the blueprint is absent or undecodable at the earlier revision")
	}
	cb, cfs := Decode(file, cur.Blueprint)
	if cb == nil || plan.HasError(cfs) {
		return whole("the blueprint does not decode")
	}
	if old.Entry == nil || cur.Entry == nil {
		return whole("the outline does not list the page at both revisions")
	}

	facts := map[string]bool{}
	addFacts := func(ids []string) {
		for _, id := range ids {
			if id != "" {
				facts[id] = true
			}
		}
	}
	differs := !sameYAML(old.Entry, cur.Entry)

	if !sameYAML(withoutSections(ob), withoutSections(cb)) {
		differs = true
		// Only a field that differs contributes the facts it cites; the rest of the page-level
		// fields cite none.
		if !sameYAML(ob.Introduction, cb.Introduction) {
			addFacts(ob.Introduction.Facts)
			addFacts(cb.Introduction.Facts)
		}
		if !sameYAML(ob.Dispositions, cb.Dispositions) {
			addFacts(dispositionFacts(ob))
			addFacts(dispositionFacts(cb))
		}
		if !sameYAML(ob.Framing, cb.Framing) {
			addFacts(framingFacts(ob, old.Facts))
			addFacts(framingFacts(cb, cur.Facts))
		}
	}

	oldSec, curSec := map[string]*Section{}, map[string]*Section{}
	oldItems, curItems := map[string]*Item{}, map[string]*Item{}
	for i := range ob.Sections {
		oldSec[ob.Sections[i].ID] = &ob.Sections[i]
		for j := range ob.Sections[i].Items {
			oldItems[ob.Sections[i].Items[j].ID] = &ob.Sections[i].Items[j]
		}
	}
	var sharedOld, sharedCur []string
	for i := range cb.Sections {
		curSec[cb.Sections[i].ID] = &cb.Sections[i]
		for j := range cb.Sections[i].Items {
			curItems[cb.Sections[i].Items[j].ID] = &cb.Sections[i].Items[j]
		}
		if _, ok := oldSec[cb.Sections[i].ID]; ok {
			sharedCur = append(sharedCur, cb.Sections[i].ID)
		}
	}
	for _, s := range ob.Sections {
		if _, ok := curSec[s.ID]; ok {
			sharedOld = append(sharedOld, s.ID)
		} else {
			differs = true
		}
	}
	if !slices.Equal(sharedOld, sharedCur) {
		differs = true
	}

	for id, it := range oldItems {
		if n, ok := curItems[id]; !ok || !sameYAML(it, n) {
			addFacts(itemFacts(it))
		}
	}
	for id, it := range curItems {
		if o, ok := oldItems[id]; !ok || !sameYAML(o, it) {
			addFacts(itemFacts(it))
		}
	}

	bulletChanged := func(id string) bool { return FactChanged(old.Facts, cur.Facts, id) }
	for _, ids := range [][]string{citedFacts(ob, old.Facts), citedFacts(cb, cur.Facts)} {
		for _, id := range ids {
			if bulletChanged(id) {
				facts[id] = true
				differs = true
			}
		}
	}

	var bullets []string
	for _, id := range slices.Compact(slices.Sorted(slices.Values(slices.Concat(old.Facts.IDs(), cur.Facts.IDs())))) {
		if bulletChanged(id) {
			bullets = append(bullets, id)
		}
	}
	atBase := citedFacts(ob, old.Facts)
	var joined []string
	for _, id := range citedFacts(cb, cur.Facts) {
		if id != "" && !slices.Contains(atBase, id) && !slices.Contains(joined, id) {
			joined = append(joined, id)
		}
	}
	slices.Sort(joined)

	var changed []string
	for i := range cb.Sections {
		s := &cb.Sections[i]
		if o, ok := oldSec[s.ID]; ok && sameYAML(o, s) && !sectionCites(s, bulletChanged) {
			continue
		}
		changed = append(changed, s.ID)
	}
	if len(changed) == 0 && !differs {
		return Scope{Kind: ScopeUnchanged}
	}
	return Scope{
		Kind:    ScopeScoped,
		IDs:     changed,
		Facts:   append([]string{}, slices.Sorted(maps.Keys(facts))...),
		Bullets: append([]string{}, bullets...),
		Joined:  append([]string{}, joined...),
	}
}

// withoutSections is the blueprint with its sections dropped, the fields Compare holds apart.
func withoutSections(b *Blueprint) *Blueprint {
	c := *b
	c.Sections = nil
	return &c
}

// sameYAML compares two values as yaml marshals them, which leaves out the line numbers a decode
// records and so ignores formatting.
func sameYAML(a, b any) bool {
	x, errA := yaml.Marshal(a)
	y, errB := yaml.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(x, y)
}

// FactChanged reports whether the bullet that holds id differs between the two stores. A project
// with no store has no citations to go stale.
func FactChanged(old, cur *plan.Facts, id string) bool {
	if old == nil || cur == nil {
		return false
	}
	o, okOld := old.Get(id)
	c, okCur := cur.Get(id)
	if okOld != okCur || old.Count(id) != cur.Count(id) {
		return true
	}
	if !okOld {
		return false
	}
	return o.Tag != c.Tag || o.Problem != c.Problem || o.Text != c.Text || !slices.Equal(o.Sources, c.Sources)
}

// sectionCites reports whether any item of s places or cites a fact that reports as changed.
func sectionCites(s *Section, changed func(id string) bool) bool {
	for i := range s.Items {
		if slices.ContainsFunc(itemFacts(&s.Items[i]), changed) {
			return true
		}
	}
	return false
}

// itemFacts lists the facts an item places or cites.
func itemFacts(it *Item) []string {
	ids := slices.Clone(it.Facts)
	if it.Kind == "fact" {
		ids = append(ids, it.Fact)
	}
	return ids
}

func dispositionFacts(b *Blueprint) []string {
	var ids []string
	for _, d := range b.Dispositions {
		ids = append(ids, d.Fact)
	}
	return ids
}

// framingFacts lists the facts the framing cites. It cites a fact as a backticked id, which
// counts only against a store to recognise it.
func framingFacts(b *Blueprint, facts *plan.Facts) []string {
	if facts == nil {
		return nil
	}
	var ids []string
	for _, part := range framingParts(&b.Framing) {
		for _, m := range codeSpan.FindAllStringSubmatch(part.text, -1) {
			if facts.Cites(m[1]) {
				ids = append(ids, m[1])
			}
		}
	}
	return ids
}

// pageFacts lists the facts the introduction, the dispositions, and the framing place or cite.
func pageFacts(b *Blueprint, facts *plan.Facts) []string {
	return slices.Concat(b.Introduction.Facts, dispositionFacts(b), framingFacts(b, facts))
}

// citedFacts lists every fact the page places or cites, wherever it does.
func citedFacts(b *Blueprint, facts *plan.Facts) []string {
	ids := pageFacts(b, facts)
	for i := range b.Sections {
		for j := range b.Sections[i].Items {
			ids = append(ids, itemFacts(&b.Sections[i].Items[j])...)
		}
	}
	return ids
}
