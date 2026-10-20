package main

import (
	"encoding/json"
	"fmt"
	"io"
	"path"
	"slices"
	"strings"

	"github.com/glw907/drafter/internal/blueprint"
	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/coverage"
	"github.com/glw907/drafter/internal/handoff"
	"github.com/glw907/drafter/internal/pipeline"
)

// upToDate is the line of a page whose blueprint, facts, and outline entry match its lock entry.
const upToDate = "up to date with drafted.lock"

// draftInputs are the page inputs a blueprint page's drafting seats read: its type, the facts the
// blueprint places or cites, and its exemplar sources, all from the commit the run starts at. A
// blueprint the commit lacks gives none of them; the run's clean-tree check names that fault.
func draftInputs(c *config.Config, root, head, rel string, outline []byte, entry blueprint.Page) (handoff.Inputs, error) {
	in := handoff.Inputs{
		Job: entry.Job, PageType: entry.Type,
		RegisterSections: []string{}, ExemplarSources: []string{}, ClaimInventory: []handoff.Claim{},
	}
	bp, err := gitShow(root, head, rel)
	if err != nil {
		return in, nil
	}
	if b, _ := blueprint.Decode(rel, bp); b != nil {
		for _, ex := range b.Exemplars {
			in.ExemplarSources = append(in.ExemplarSources, ex.Source)
		}
	}
	p := blueprint.Project{Root: root, BlueprintRoot: path.Clean(c.Blueprints.Root), Outline: path.Clean(c.Blueprints.Outline), FactsDir: c.Facts.Dir, Required: c.Blueprints.Required}
	ids, err := p.CitedIn(rel, bp, outline)
	if err != nil {
		return in, err
	}
	in.FactIDs = ids
	return in, nil
}

// draftPlan is what drafter page decides about its blueprint pages before the run: which pages
// run, the scope and change block of each, the pages whose blueprint read is skipped, and the
// read of each page its bar sends back to the change since a blocked read.
type draftPlan struct {
	handoffs []handoff.Handoff
	skipped  []pipeline.PageResult
	scopes   map[string]blueprint.Scope
	changes  map[string]string
	skipRead map[string]bool
	read     barMaps
}

// pageScope is the decision for one page.
type pageScope struct {
	scope    blueprint.Scope
	change   string // the change block of a scoped page
	upToDate bool
}

// wholeOnRequest is the reason a page drafts whole under --whole.
const wholeOnRequest = "drafted whole on request (--whole)"

// scopeFromLock compares each blueprint page with the lock entry committed at head and decides how
// the run treats it. With whole set, every page drafts whole and none is up to date. A page whose
// read is not skipped and whose entry holds a bar reads the change since the bar's commit, while
// its draft keeps the scope decided here; a page its bar holds is not run. A lock that does not
// decode is an error, since every entry, bar, and skipped read would be lost with it.
func scopeFromLock(c *config.Config, root, head string, handoffs []handoff.Handoff, whole bool, warn io.Writer) (draftPlan, error) {
	d := draftPlan{
		scopes: map[string]blueprint.Scope{}, changes: map[string]string{}, skipRead: map[string]bool{},
		read: barMaps{scopes: map[string]blueprint.Scope{}, changes: map[string]string{}, bars: map[string]string{}, barFrom: map[string]string{}, barFacts: map[string][]string{}},
	}
	lock, err := committedLock(c, root, head)
	if err != nil {
		return d, err
	}
	for _, h := range handoffs {
		e := lock.Pages[h.Page.Path]
		ps := wholeBecause(wholeOnRequest)
		if !whole {
			ps = pageScopeFor(c, root, head, lockRel(c), e, h)
		}
		if ps.upToDate {
			d.skipped = append(d.skipped, unchangedPage(h.Page.Path, upToDate))
			continue
		}
		if blob, err := blueprint.CommitBlobID(root, head, h.Plan); err == nil && skipRead(e, blob) {
			d.skipRead[h.Page.Path] = true
		} else if e.Blocked != nil {
			if r := applyBar(c, root, head, h, e.Blocked, d.read, warn); r != nil {
				d.skipped = append(d.skipped, *r)
				continue
			}
		}
		d.handoffs = append(d.handoffs, h)
		d.scopes[h.Page.Path] = ps.scope
		if ps.change != "" {
			d.changes[h.Page.Path] = ps.change
		}
	}
	return d, nil
}

// skipRead reports whether a page's blueprint read can be skipped: the lock records the
// blueprint as read-accepted, and it is the blueprint committed at the start.
func skipRead(e blueprint.LockEntry, blob string) bool {
	return e.Accepted != "" && e.Accepted == blob
}

func wholeBecause(why string) pageScope {
	return pageScope{scope: blueprint.Scope{Kind: blueprint.ScopeWhole, Why: why}}
}

// pageScopeFor decides one page. A page with no drafted entry, or no committed page, drafts
// whole. Otherwise the blueprint, outline entry, and facts the entry's draft saw are rebuilt and
// compared with the committed ones.
func pageScopeFor(c *config.Config, root, head, lockRel string, e blueprint.LockEntry, h handoff.Handoff) pageScope {
	if e.Page == "" {
		return wholeBecause("the page has no drafted entry in " + blueprint.LockFile)
	}
	if _, err := gitShow(root, head, h.Page.Path); err != nil {
		return wholeBecause("the page is not committed at the start commit")
	}
	introduced, err := blueprint.LockIntroducedAt(root, lockRel, h.Page.Path, e)
	if err != nil || introduced == "" {
		return wholeBecause("no commit introduced the page's entry in " + blueprint.LockFile)
	}
	cur := revisionAt(c, root, head, h)
	old := revisionAt(c, root, blueprint.ParentCommit(root, introduced), h)
	old.Blueprint = blueprint.BlobBytes(root, e.Blueprint)
	sc := blueprint.Compare(h.Plan, old, cur)
	switch sc.Kind {
	case blueprint.ScopeUnchanged:
		return pageScope{scope: sc, upToDate: true}
	case blueprint.ScopeWhole:
		return pageScope{scope: sc}
	}

	if len(sc.IDs) > 0 {
		briefBytes, err := gitShow(root, head, h.Page.Brief)
		if err != nil {
			return wholeBecause("the page has no brief at the start commit, so a section redraft cannot cover its sentences")
		}
		pageBytes, _ := gitShow(root, head, h.Page.Path)
		b, _ := blueprint.Decode(h.Plan, cur.Blueprint)
		if b == nil {
			return wholeBecause("the blueprint does not decode")
		}
		ids, fact := widenIDs(sc, b, pageBytes, briefBytes)
		if fact != "" {
			return wholeBecause("a sentence of the page's preamble cites the changed fact " + fact)
		}
		sc.IDs = ids
	}
	blob, err := blueprint.CommitBlobID(root, head, h.Plan)
	if err != nil {
		return wholeBecause("the blueprint's blob could not be read: " + err.Error())
	}
	block, err := changeBlock(root, diffEnds{from: e.Blueprint, to: blob, file: h.Plan, blobs: true}, sc, old, cur)
	if err != nil {
		return wholeBecause("the change could not be built: " + err.Error())
	}
	return pageScope{scope: sc, change: block}
}

// widenIDs adds to the scope's section ids every section holding a brief sentence that cites a
// changed fact bullet, so a bounded redraft can repair that sentence. It returns the ids in
// blueprint order. When a changed bullet is cited by a sentence of the preamble, it returns the
// fact instead, and the page drafts whole. A brief sentence found in no section is ignored here;
// the run's coverage check reports it.
func widenIDs(sc blueprint.Scope, b *blueprint.Blueprint, page, brief []byte) (ids []string, preambleFact string) {
	var br coverage.Brief
	if json.Unmarshal(brief, &br) != nil {
		return sc.IDs, ""
	}
	headings := make([]string, len(b.Sections))
	for i, s := range b.Sections {
		headings[i] = s.Heading
	}
	preamble, text, found := coverage.Regions(headings, page)
	pre := coverage.Normalize(preamble)
	want := map[string]bool{}
	for _, id := range sc.IDs {
		want[id] = true
	}
	for _, s := range br.Sentences {
		changed := slices.DeleteFunc(slices.Clone(s.ID), func(id string) bool { return !slices.Contains(sc.Bullets, id) })
		norm := coverage.Normalize(s.Text)
		if len(changed) == 0 || norm == "" {
			continue
		}
		if strings.Contains(pre, norm) {
			return nil, changed[0]
		}
		for i := range text {
			if found[i] && strings.Contains(coverage.Normalize(text[i]), norm) {
				want[b.Sections[i].ID] = true
			}
		}
	}
	for _, s := range b.Sections {
		if want[s.ID] {
			ids = append(ids, s.ID)
		}
	}
	return ids, ""
}

// stockTrackRefusal is the reason the stock module cannot draft a page, or "". The stock module
// carries only the Google standard, and it needs the page's track to name the standard.
func stockTrackRefusal(o *blueprint.Outline, pagePath string) string {
	if len(o.Tracks) == 0 {
		return ""
	}
	t, ok := o.TrackFor(pagePath)
	if !ok {
		return fmt.Sprintf("%s: the outline declares tracks, and the page matches no track or more than one; the stock module needs one track to name the standard", pagePath)
	}
	if t.Standard != "google" {
		return fmt.Sprintf("%s: the track %q names the standard %q, and the stock module carries only google; set prompts to a module for that standard", pagePath, t.Name, t.Standard)
	}
	return ""
}
