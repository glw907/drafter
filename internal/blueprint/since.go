package blueprint

import (
	"bytes"
	"slices"

	"github.com/glw907/drafter/internal/plan"
	"go.yaml.in/yaml/v3"
)

// The kinds of Scope.
const (
	// ScopeUnchanged means nothing a read sees differs from the earlier revision.
	ScopeUnchanged = "unchanged"
	// ScopeScoped means the sections in Scope.IDs changed and nothing else did.
	ScopeScoped = "scoped"
	// ScopeWhole means the change reaches beyond named sections, or cannot be bounded.
	ScopeWhole = "whole"
)

// Scope is what changed in one page's blueprint between two revisions. IDs holds the changed
// section ids, in blueprint order, for a scoped page, and Why says what made a page whole.
type Scope struct {
	Kind string   `json:"kind"`
	IDs  []string `json:"ids,omitempty"`
	Why  string   `json:"why,omitempty"`
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
// formatting alone changes nothing. The page is whole when a section was removed, the shared
// sections changed order or level, anything outside sections changed (a fact cited from the
// framing, the introduction, or the dispositions included), the outline entry changed, or the
// earlier blueprint or entry is missing or does not decode. An undecodable current blueprint is
// whole too, which leaves its fault to the blueprint check.
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
	if old.Entry == nil || cur.Entry == nil || !sameYAML(old.Entry, cur.Entry) {
		return whole("the outline entry changed")
	}
	if !sameYAML(withoutSections(ob), withoutSections(cb)) {
		return whole("a field outside sections changed")
	}
	oldAt := map[string]*Section{}
	for i := range ob.Sections {
		oldAt[ob.Sections[i].ID] = &ob.Sections[i]
	}
	curIDs := make([]string, len(cb.Sections))
	for i, s := range cb.Sections {
		curIDs[i] = s.ID
	}
	var shared []string
	for _, s := range ob.Sections {
		if !slices.Contains(curIDs, s.ID) {
			return whole("section " + s.ID + " was removed")
		}
		shared = append(shared, s.ID)
	}
	var sharedCur []string
	for _, id := range curIDs {
		if _, ok := oldAt[id]; ok {
			sharedCur = append(sharedCur, id)
		}
	}
	if !slices.Equal(shared, sharedCur) {
		return whole("the order of the sections changed")
	}
	if cites(cb, cur.Facts, func(id string) bool { return factChanged(old.Facts, cur.Facts, id) }) {
		return whole("a fact cited outside the sections changed")
	}
	var changed []string
	for i := range cb.Sections {
		s := &cb.Sections[i]
		o, ok := oldAt[s.ID]
		switch {
		case !ok:
		case o.Level != s.Level:
			return whole("section " + s.ID + " changed level")
		case !sameYAML(o, s) || sectionCites(s, func(id string) bool { return factChanged(old.Facts, cur.Facts, id) }):
		default:
			continue
		}
		changed = append(changed, s.ID)
	}
	if len(changed) == 0 {
		return Scope{Kind: ScopeUnchanged}
	}
	return Scope{Kind: ScopeScoped, IDs: changed}
}

// withoutSections is the blueprint with its sections dropped, the fields Compare holds to whole.
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

// factChanged reports whether the bullet that holds id differs between the two stores. A project
// with no store has no citations to go stale.
func factChanged(old, cur *plan.Facts, id string) bool {
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

// sectionCites reports whether any item of s places or cites a fact that changed reports as changed.
func sectionCites(s *Section, changed func(id string) bool) bool {
	for _, it := range s.Items {
		if (it.Kind == "fact" && changed(it.Fact)) || slices.ContainsFunc(it.Facts, changed) {
			return true
		}
	}
	return false
}

// cites reports whether the framing, the introduction, or the dispositions place or cite a fact
// that changed reports as changed.
func cites(b *Blueprint, facts *plan.Facts, changed func(id string) bool) bool {
	if slices.ContainsFunc(b.Introduction.Facts, changed) {
		return true
	}
	for _, d := range b.Dispositions {
		if changed(d.Fact) {
			return true
		}
	}
	if facts == nil {
		return false
	}
	for _, part := range framingParts(&b.Framing) {
		for _, m := range codeSpan.FindAllStringSubmatch(part.text, -1) {
			if facts.Cites(m[1]) && changed(m[1]) {
				return true
			}
		}
	}
	return false
}
