package main

import (
	"fmt"
	"io"
	"path"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/glw907/drafter/internal/blueprint"
	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/handoff"
	"github.com/glw907/drafter/internal/pipeline"
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
func recordReads(lock *blueprint.Lock, start string, s pipeline.Summary, bases map[string]string) (changed bool, kept map[string]string) {
	for _, r := range s.Pages {
		e := lock.Pages[r.Path]
		switch {
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
			if e.Accepted == r.Accepted && e.Blocked == nil {
				continue
			}
			e.Accepted, e.Blocked = r.Accepted, nil
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

// scopedRefusal is why a scoped accept cannot advance the entry e, whose read diffed from the
// blueprint blob base, or empty when it can.
func scopedRefusal(e blueprint.LockEntry, base string) string {
	if e.Blocked != nil {
		return "it holds a bar the scoped read did not judge"
	}
	if e.Accepted == "" || e.Accepted != base {
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

// barOf is the bar a blocked whole read leaves: its blocking findings, in order. It is nil when
// none blocks, since a bar with no finding holds the next read to nothing.
func barOf(start string, fs []pipeline.FindingResult) *blueprint.Blocked {
	var out []blueprint.BlockedFinding
	for _, f := range fs {
		if f.Blocking {
			out = append(out, blueprint.BlockedFinding{Location: f.Location, Text: f.Text})
		}
	}
	if len(out) == 0 {
		return nil
	}
	return &blueprint.Blocked{Commit: start, Findings: out}
}

// renderBar is the bar as the blueprint seat gets it: one line per finding, numbered from 1.
func renderBar(b *blueprint.Blocked) string {
	lines := make([]string, len(b.Findings))
	for i, f := range b.Findings {
		lines[i] = fmt.Sprintf("%d. [%s] %s", i+1, f.Location, f.Text)
	}
	return strings.Join(lines, "\n")
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
		r.Findings = append(r.Findings, pipeline.FindingResult{Location: f.Location, Text: f.Text, Blocking: true})
	}
	return r
}
