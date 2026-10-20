package main

import (
	"fmt"
	"io"
	"maps"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/glw907/drafter/internal/blueprint"
	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/handoff"
	"github.com/glw907/drafter/internal/pipeline"
	"github.com/glw907/drafter/internal/plan"
	runstate "github.com/glw907/drafter/internal/run"
)

// statusHeld is the status of a page whose read was not sent because its blueprint has not
// changed since the blocked read its bar records: a read would judge nothing new, and the bar's
// findings still stand.
const statusHeld = "held"

// lockRel is the repo-relative path of drafted.lock, beside the outline.
func lockRel(c *config.Config) string {
	return path.Join(path.Dir(c.Blueprints.Outline), blueprint.LockFile)
}

// committedLock decodes drafted.lock as committed at rev. A commit with no lock gives an empty
// one. A lock that does not decode is an error naming the file: read as empty, it would drop
// every bar, accepted id, and entry without a word.
func committedLock(c *config.Config, root, rev string) (*blueprint.Lock, error) {
	rel := lockRel(c)
	data, err := gitShow(root, rev, rel)
	if err != nil {
		return &blueprint.Lock{Version: blueprint.Version}, nil
	}
	l, err := blueprint.DecodeLock(rel, data)
	if err != nil {
		return nil, fmt.Errorf("%w; a drafter that knows its format must read it", err)
	}
	return l, nil
}

// updateLock reads drafted.lock under the worktree lock, lets edit change it, and writes it
// when edit reports a change, so a run that changes nothing leaves the file as it was.
func updateLock(c *config.Config, root string, edit func(*blueprint.Lock) (bool, error)) error {
	gitDir, err := gitText(root, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return err
	}
	l, err := runstate.TryLock(filepath.Join(gitDir, "drafter.lock"))
	if err != nil {
		return fmt.Errorf("another drafter run holds this worktree, so drafted.lock was not written: %w", err)
	}
	defer func() { _ = l.Release() }()
	file := filepath.Join(root, filepath.FromSlash(lockRel(c)))
	lock, err := blueprint.ReadLock(file)
	if err != nil {
		return err
	}
	changed, err := edit(lock)
	if err != nil || !changed {
		return err
	}
	return blueprint.WriteLock(file, lock)
}

// recordReads writes into lock what each page's blueprint read decided, and reports whether an
// entry changed. An accept that judged the whole blueprint, or the change since a bar, records
// the accepted id and clears the bar. A whole read that blocked records its blocking findings as
// the page's bar, with start, the run's start commit, replacing any bar the entry held. A change
// read that blocked, a bar read among them, leaves the entry alone, so a bar stays at the first
// blocked whole read and each later read judges every change since it.
//
// A scoped accept judged only the change since its base, so it records the accepted id only when
// that change is all that is new: the entry holds no bar, which the read never saw, and its
// accepted id is bases[path], the blueprint's blob at the commit the read diffed from. Otherwise
// the entry stays, and kept maps the page to the reason.
//
// The entry's facts follow finding 12 of the blueprint-first plan: they change only on an accept
// that judged every bullet they hold, so a result's Facts is set only by drafter read. A whole
// accept and a lapse accept write them; a bar accept writes them only when the bar's findings
// carry a seat, since a bar written before the fact-truth seat existed leaves bullets no seat
// ever judged; a scoped accept and any accept inside drafter page leave them as they were. A lapse
// accept keeps the accepted id, and counts only while the entry still holds the id the read
// started from.
func recordReads(lock *blueprint.Lock, start string, s pipeline.Summary, bases map[string]string) (changed bool, kept map[string]string) {
	for _, r := range s.Pages {
		e := lock.Pages[r.Path]
		switch {
		case r.Lapse && r.Status == pipeline.StatusAccepted && r.Facts != nil:
			if e.Accepted != r.Accepted || e.Blocked != nil || maps.Equal(e.Facts, r.Facts) {
				continue
			}
			e.Facts = nilIfEmpty(r.Facts)
		case scopedAccept(r):
			if why := scopedRefusal(e, bases[r.Path]); why != "" {
				if kept == nil {
					kept = map[string]string{}
				}
				kept[r.Path] = why
				continue
			}
			if e.Accepted == r.Accepted {
				continue
			}
			e.Accepted = r.Accepted
		case r.Status == pipeline.StatusAccepted && r.Accepted != "":
			facts := e.Facts
			if r.Facts != nil && (r.BarFrom == "" || barJudgedFacts(e.Blocked)) {
				facts = nilIfEmpty(r.Facts)
			}
			if e.Accepted == r.Accepted && e.Blocked == nil && maps.Equal(e.Facts, facts) {
				continue
			}
			e.Accepted, e.Blocked, e.Facts = r.Accepted, nil, facts
		case r.WholeBlocked:
			b := barOf(start, r.Findings)
			if b == nil || reflect.DeepEqual(e.Blocked, b) {
				continue
			}
			e.Blocked = b
		default:
			continue
		}
		if lock.Pages == nil {
			lock.Pages = map[string]blueprint.LockEntry{}
		}
		lock.Pages[r.Path] = e
		changed = true
	}
	return changed, kept
}

// barJudgedFacts reports whether the bar was written by a read with the fact-truth seat, which sets
// the seat on every finding it writes.
func barJudgedFacts(b *blueprint.Blocked) bool {
	return b != nil && slices.ContainsFunc(b.Findings, func(f blueprint.BlockedFinding) bool { return f.Seat != "" })
}

// nilIfEmpty is m, or nil when it holds nothing, which is how an entry holds no facts.
func nilIfEmpty(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	return m
}

// scopedRefusal is why a scoped accept cannot advance the entry e, whose read diffed from the
// blueprint blob base, or empty when it can.
func scopedRefusal(e blueprint.LockEntry, base string) string {
	if e.Blocked != nil {
		return "it holds a bar the scoped read did not judge"
	}
	if e.Accepted == "" {
		return "it has no accepted blueprint yet"
	}
	if e.Accepted != base {
		return "the blueprint it accepted is not the one at the scoped read's base"
	}
	return ""
}

// scopedKept prints the line for each page whose scoped accept left its entry as it was, in the
// order the run lists the pages.
func scopedKept(w io.Writer, s pipeline.Summary, kept map[string]string) {
	for _, r := range s.Pages {
		if why, ok := kept[r.Path]; ok {
			_, _ = fmt.Fprintf(w, "%s: accepted on a scoped read; drafted.lock keeps its earlier entry, since %s\n", r.Path, why)
		}
	}
}

// barOf is the bar a blocked whole read leaves: its blocking findings, in order, each with the
// seat and kind that raised it. It is nil when none blocks, since a bar with no finding holds the
// next read to nothing.
func barOf(start string, fs []pipeline.FindingResult) *blueprint.Blocked {
	var out []blueprint.BlockedFinding
	for _, f := range fs {
		if f.Blocking {
			out = append(out, blueprint.BlockedFinding{Location: f.Location, Text: f.Text, Seat: f.Seat, Kind: f.Kind})
		}
	}
	if len(out) == 0 {
		return nil
	}
	return &blueprint.Blocked{Commit: start, Findings: out}
}

// renderBar is the bar as the blueprint seat gets it: one line per finding, numbered from 1. The
// fact-truth seat's findings are left out, since the blueprint seat has no excerpts to judge them
// by; the fact-truth read gets those facts through barFactIDs.
func renderBar(b *blueprint.Blocked) string {
	var lines []string
	for _, f := range b.Findings {
		if f.Seat != factTruthSeat {
			lines = append(lines, fmt.Sprintf("%d. [%s] %s", len(lines)+1, f.Location, f.Text))
		}
	}
	return strings.Join(lines, "\n")
}

// factTruthSeat is the Seat of the findings the fact-truth read raises, whose Location is "fact "
// and the fact's id.
const factTruthSeat = "facttruth"

// barFactIDs lists the ids of the facts the bar's fact-truth findings name, in order.
func barFactIDs(b *blueprint.Blocked) []string {
	var ids []string
	for _, f := range b.Findings {
		if id, ok := strings.CutPrefix(f.Location, "fact "); ok && f.Seat == factTruthSeat {
			ids = append(ids, id)
		}
	}
	return ids
}

// shortSHA is the abbreviated form of a commit sha a status line shows.
func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// barMaps are the maps a bar page's read is recorded in: the run's Scopes, Changes, Bars, and
// BarFrom under drafter read, and its read scopes and changes under drafter page, whose Scopes
// belong to the draft.
type barMaps struct {
	scopes  map[string]blueprint.Scope
	changes map[string]string
	bars    map[string]string
	barFrom map[string]string
	// barFacts holds the fact ids each bar page's fact-truth findings name.
	barFacts map[string][]string
}

// applyBar decides the read of a page whose lock entry holds the bar b, and records it in m.
// The page reads the change since the bar's commit, compared as drafter read --since compares,
// with the bar as the blueprint seat's findings. A blueprint unchanged since that commit is not
// read: applyBar returns its held result. When the commit is gone, as after a squash or rebase
// merge, or the comparison falls back to whole, the page reads whole with no bar, and a gone
// commit is named on warn.
func applyBar(c *config.Config, root, head string, h handoff.Handoff, b *blueprint.Blocked, m barMaps, warn io.Writer) *pipeline.PageResult {
	p := h.Page.Path
	if _, err := gitText(root, "cat-file", "-e", b.Commit+"^{commit}"); err != nil {
		_, _ = fmt.Fprintf(warn, "%s: the blocked read's commit %s is not in this repository; reading whole\n", p, shortSHA(b.Commit))
		m.scopes[p] = blueprint.Scope{Kind: blueprint.ScopeWhole, Why: "the blocked read's commit " + shortSHA(b.Commit) + " is not in this repository"}
		return nil
	}
	sc, change := comparePage(c, root, b.Commit, head, h)
	switch sc.Kind {
	case blueprint.ScopeUnchanged:
		r := heldPage(p, b)
		return &r
	case blueprint.ScopeScoped:
		m.changes[p], m.bars[p], m.barFrom[p] = change, renderBar(b), b.Commit
		if ids := barFactIDs(b); len(ids) > 0 {
			m.barFacts[p] = ids
		}
	}
	m.scopes[p] = sc
	return nil
}

// heldPage is the result of a page held by its bar: the line to print, and the bar's findings,
// so a fix can start from the output.
func heldPage(path string, b *blueprint.Blocked) pipeline.PageResult {
	noun := "findings"
	if len(b.Findings) == 1 {
		noun = "finding"
	}
	why := fmt.Sprintf("unchanged since the blocked read at %s; fix its %d %s and commit", shortSHA(b.Commit), len(b.Findings), noun)
	r := pipeline.PageResult{Path: path, Status: statusHeld, Scope: &blueprint.Scope{Kind: blueprint.ScopeUnchanged, Why: why}}
	for _, f := range b.Findings {
		r.Findings = append(r.Findings, pipeline.FindingResult{Location: f.Location, Text: f.Text, Blocking: true, Seat: f.Seat, Kind: f.Kind})
	}
	return r
}

// readSet is a blueprint's facts as committed at a start commit: the facts store, every id the
// blueprint places, cites, or disposes, and the hash of each.
type readSet struct {
	bp     *blueprint.Blueprint
	facts  *plan.Facts
	ids    []string
	hashes map[string]string
}

// readSetAt builds the read set of h at rev. It reports false when the blueprint at rev does not
// decode, which the blueprint check reports on its own.
func readSetAt(c *config.Config, root, rev string, h handoff.Handoff) (readSet, bool) {
	r := revisionAt(c, root, rev, h)
	b, _ := blueprint.Decode(h.Plan, r.Blueprint)
	if b == nil {
		return readSet{}, false
	}
	ids := blueprint.CitedFacts(b, r.Facts)
	return readSet{bp: b, facts: r.Facts, ids: ids, hashes: blueprint.FactsHashes(ids, r.Facts)}, true
}

// lapseRead decides whether a page's read is a lapse read, and builds its scope and change block
// when it is. The page's entry holds no bar, its accepted id is the blueprint committed at head,
// and it holds a facts map that differs from the read set at head. An entry with no facts map
// reads whole, once, so it is not a lapse read.
func lapseRead(c *config.Config, root, head string, h handoff.Handoff, e blueprint.LockEntry) (blueprint.Scope, string, bool) {
	if e.Blocked != nil || len(e.Facts) == 0 || e.Accepted == "" {
		return blueprint.Scope{}, "", false
	}
	if blob, err := blueprint.CommitBlobID(root, head, h.Plan); err != nil || blob != e.Accepted {
		return blueprint.Scope{}, "", false
	}
	rs, ok := readSetAt(c, root, head, h)
	if !ok || blueprint.FactsMatch(e.Facts, rs.hashes) {
		return blueprint.Scope{}, "", false
	}
	changed := blueprint.ChangedFacts(e.Facts, rs.hashes)
	held := []string{}
	for _, id := range changed {
		if _, ok := rs.hashes[id]; ok {
			held = append(held, id)
		}
	}
	sc := blueprint.Scope{
		Kind: blueprint.ScopeScoped, IDs: blueprint.SectionsCiting(rs.bp, held),
		Facts: held, Bullets: slices.Clone(held), Joined: []string{},
	}
	return sc, lapseBlock(changed, rs), true
}

// lapseBlock is the change block of a lapse read: the facts that changed since the accepted read,
// each with its current bullet, or the note that the blueprint no longer cites it.
func lapseBlock(changed []string, rs readSet) string {
	var b strings.Builder
	b.WriteString("The change this read judges follows. The blueprint is the one the last accepted read judged, and the facts below changed after that read. It adds to the blueprint and the facts above.\n\n=== changed facts ===\n")
	for _, id := range changed {
		fmt.Fprintf(&b, "- %s\n", id)
		switch _, held := rs.hashes[id]; {
		case held:
			f, _ := rs.facts.Get(id)
			fmt.Fprintf(&b, "  changed since the accepted read: %s\n", f.Raw)
		case slices.Contains(rs.ids, id):
			b.WriteString("  changed since the accepted read: the facts store no longer holds it as one bullet\n")
		default:
			b.WriteString("  no longer cited by the blueprint\n")
		}
	}
	b.WriteString("=== end changed facts ===")
	return b.String()
}

// lapseResult reports whether a drafter read result is the result of a lapse read: a scoped read
// that compared against no revision and no bar.
func lapseResult(r pipeline.PageResult) bool {
	return r.Scope != nil && r.Scope.Kind == blueprint.ScopeScoped && r.Since == "" && r.BarFrom == "" && r.ReadScope == nil
}

// stampFacts sets Lapse and Facts on each accepted result of a drafter read whose read judged
// every fact the lock would record: a whole read, a bar read, and a lapse read. A change read
// judged only the facts it touched, so its result carries none.
func stampFacts(c *config.Config, root, head string, handoffs []handoff.Handoff, s *pipeline.Summary) {
	for i := range s.Pages {
		r := &s.Pages[i]
		if r.Status != pipeline.StatusAccepted || r.Accepted == "" {
			continue
		}
		r.Lapse = lapseResult(*r)
		if scopedAccept(*r) && !r.Lapse {
			continue
		}
		for _, h := range handoffs {
			if h.Page.Path != r.Path {
				continue
			}
			if rs, ok := readSetAt(c, root, head, h); ok {
				r.Facts = rs.hashes
				if r.Facts == nil {
					r.Facts = map[string]string{}
				}
			}
		}
	}
}

// lapsedPages lists, one line per page, the pages of a drafter page run whose lock entry no longer
// matches the facts at head: an entry holding a facts map that differs from the read set whatever
// its accepted id, and an entry whose accepted id is the committed blueprint but records no facts
// while the blueprint has some.
func lapsedPages(c *config.Config, root, head string, handoffs []handoff.Handoff, lock *blueprint.Lock) []string {
	var lines []string
	for _, h := range handoffs {
		e := lock.Pages[h.Page.Path]
		rs, ok := readSetAt(c, root, head, h)
		if !ok {
			continue
		}
		blob, _ := blueprint.CommitBlobID(root, head, h.Plan)
		var why string
		switch {
		case len(e.Facts) > 0 && !blueprint.FactsMatch(e.Facts, rs.hashes):
			why = "facts changed since it: " + strings.Join(blueprint.ChangedFacts(e.Facts, rs.hashes), ", ")
		case len(e.Facts) == 0 && e.Accepted != "" && e.Accepted == blob && len(rs.hashes) > 0:
			why = "no facts recorded"
		default:
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: the accepted read lapsed; %s. Run drafter read %s.", h.Page.Path, why, h.Plan))
	}
	return lines
}
