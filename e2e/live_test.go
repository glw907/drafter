//go:build live

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/glw907/drafter/internal/blueprint"
	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/coverage"
	"github.com/glw907/drafter/internal/key"
	"github.com/glw907/drafter/internal/pipeline"
	runstate "github.com/glw907/drafter/internal/run"
)

// meterDate is the date the live budget's meter reads from.
const meterDate = "2026-10-19"

const (
	livePage      = "docs/count-items.md"
	liveBlueprint = "blueprints/docs/count-items.blueprint.yaml"
	liveBrief     = "blueprints/docs/count-items.brief.json"
	liveLock      = "blueprints/drafted.lock"
	liveEditedID  = "add-items"
)

// liveProject is the fixture copy a live check drafts in, with a runner that spends within the
// budget. It keeps the user's real HOME, so the user layer's workspace, caps, and key apply and
// the real ledger is the meter.
type liveProject struct {
	t        *testing.T
	dir      string
	baseline float64
	ctx      context.Context
	stateDir string
}

type liveSummary = pipeline.Summary

// liveRecord is the part of a page's record.json the check reads.
type liveRecord struct {
	Verdicts []struct {
		Stage, Seat, Verdict string
	} `json:"verdicts"`
	Writes []struct {
		N       int    `json:"n"`
		Step    string `json:"step"`
		Kind    string `json:"kind"`
		GateFix bool   `json:"gate_fix"`
	} `json:"writes"`
}

func newLiveProject(t *testing.T) *liveProject {
	t.Helper()
	baseline, err := parseBaseline(os.Getenv("DRAFTER_LIVE_BASELINE"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := config.Load("", config.Flags{})
	if err != nil {
		t.Fatalf("load the user configuration: %v", err)
	}
	if _, err := key.Resolve(c); err != nil {
		t.Fatalf("the API key does not resolve: %v", err)
	}
	deadline := time.Now().Add(3 * time.Hour)
	if d, ok := t.Deadline(); ok {
		deadline = d.Add(-2 * time.Minute)
	}
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	t.Cleanup(cancel)

	parent, err := os.MkdirTemp("", "drafter-live-")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(parent, fmt.Sprintf("tally-%d", time.Now().Unix()))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	copyDir(t, filepath.Join("testdata", "cli"), dir)
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-q", "-m", "fixture"}} {
		gitRun(t, dir, args...)
	}
	// A project left behind on failure keeps its run and its open intent resumable; one removed
	// would leave an open reservation at worst case in the ledger for good.
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("the project is kept at %s; resume a stopped run there with a binary built from this branch's head", dir)
			return
		}
		_ = os.RemoveAll(parent)
	})
	return &liveProject{t: t, dir: dir, baseline: baseline, ctx: ctx, stateDir: c.StateDir}
}

// exec runs the freshly built binary in the project. The binary's directory leads PATH, because
// the stock gate shells out to drafter. extra entries override the inherited environment.
func (p *liveProject) exec(extra []string, args ...string) (stdout, stderr string, code int) {
	p.t.Helper()
	cmd := exec.CommandContext(p.ctx, binary, args...)
	cmd.Dir = p.dir
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "DRAFTER_PAGE_CAP=") && !strings.HasPrefix(kv, "PATH=") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, "PATH="+filepath.Dir(binary)+string(os.PathListSeparator)+os.Getenv("PATH"))
	cmd.Env = append(cmd.Env, extra...)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	if ee, ok := errors.AsType[*exec.ExitError](err); ok {
		return out.String(), errOut.String(), ee.ExitCode()
	}
	if err != nil {
		p.t.Fatalf("drafter %v: %v", args, err)
	}
	return out.String(), errOut.String(), 0
}

// meter reads the ledger since the meter date.
func (p *liveProject) meter() meterReading {
	p.t.Helper()
	out, errOut, code := p.exec(nil, "spend", "--since", meterDate, "--json")
	if code != 0 {
		p.t.Fatalf("drafter spend exit %d\n%s", code, errOut)
	}
	r, err := parseMeter([]byte(out))
	if err != nil {
		p.t.Fatal(err)
	}
	return r
}

// repoCost is the ledger's cost for this project's repo name.
func (p *liveProject) repoCost() float64 {
	p.t.Helper()
	out, _, _ := p.exec(nil, "spend", "--since", meterDate, "--json")
	var rep struct {
		Rows []struct {
			Repo string
			Cost float64
		}
	}
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		p.t.Fatal(err)
	}
	var sum float64
	for _, r := range rep.Rows {
		if r.Repo == filepath.Base(p.dir) {
			sum += r.Cost
		}
	}
	return sum
}

// page runs drafter page with --json under the budget left, resuming a stopped run with the same
// arguments, and returns the final summary.
func (p *liveProject) page(args ...string) liveSummary {
	p.t.Helper()
	var s liveSummary
	callArgs := append([]string{"page"}, args...)
	callArgs = append(callArgs, "--json")
	for range 6 {
		left := budgetLeft(liveLimit, p.baseline, p.meter())
		capped, err := pageCapFor(left)
		if err != nil {
			p.t.Fatalf("%v (baseline %.4f)", err, p.baseline)
		}
		out, errOut, code := p.exec([]string{"DRAFTER_PAGE_CAP=" + capped}, callArgs...)
		if err := json.Unmarshal([]byte(out), &s); err != nil {
			p.t.Fatalf("drafter page exit %d printed no summary: %v\nstdout: %s\nstderr: %s", code, err, out, errOut)
		}
		p.t.Logf("drafter page %s: exit %d, run %s, cap $%s", strings.Join(args, " "), code, s.RunID, capped)
		if code == 0 || code == 1 {
			return s
		}
		if code != 3 || s.RunID == "" {
			p.t.Fatalf("drafter page exit %d\nstdout: %s\nstderr: %s", code, out, errOut)
		}
		p.t.Logf("stopped (%s); resuming run %s", s.Stop, s.RunID)
		callArgs = append([]string{"page", "--resume", s.RunID}, append(args, "--json")...)
	}
	p.t.Fatal("the run kept stopping")
	return s
}

func (p *liveProject) gitText(args ...string) string {
	p.t.Helper()
	out, err := exec.Command("git", append([]string{"-C", p.dir}, args...)...).Output()
	if err != nil {
		p.t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

func (p *liveProject) read(rel string) string {
	p.t.Helper()
	data, err := os.ReadFile(filepath.Join(p.dir, filepath.FromSlash(rel)))
	if err != nil {
		p.t.Fatal(err)
	}
	return string(data)
}

func (p *liveProject) lockEntry() blueprint.LockEntry {
	p.t.Helper()
	l, err := blueprint.ReadLock(filepath.Join(p.dir, liveLock))
	if err != nil {
		p.t.Fatal(err)
	}
	return l.Pages[livePage]
}

func (p *liveProject) runIDs() []string {
	p.t.Helper()
	entries, err := os.ReadDir(filepath.Join(p.stateDir, "runs", filepath.Base(p.dir)))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		p.t.Fatal(err)
	}
	var ids []string
	for _, e := range entries {
		ids = append(ids, e.Name())
	}
	slices.Sort(ids)
	return ids
}

func (p *liveProject) commit(msg string, files ...string) {
	p.t.Helper()
	gitRun(p.t, p.dir, append([]string{"add"}, files...)...)
	gitRun(p.t, p.dir, "commit", "-q", "-m", msg)
}

// gate runs the stock gate on the page and checks it ran provenance.
func (p *liveProject) gate() {
	p.t.Helper()
	out, errOut, code := p.exec(nil, "gate", "--page", livePage)
	if code != 0 {
		p.t.Fatalf("drafter gate exit %d\n%s%s", code, out, errOut)
	}
	if strings.Contains(out+errOut, "provenance check skipped") {
		p.t.Errorf("the gate skipped provenance:\n%s%s", out, errOut)
	}
	if !strings.Contains(out+errOut, "provenance:") {
		p.t.Errorf("the gate printed no provenance count line:\n%s%s", out, errOut)
	}
}

func (p *liveProject) accepted(s liveSummary) pipeline.PageResult {
	p.t.Helper()
	for _, r := range s.Pages {
		if r.Path == livePage {
			if r.Status != pipeline.StatusAccepted {
				p.t.Fatalf("%s is %s (%s)\n%+v", livePage, r.Status, r.Rule, r)
			}
			return r
		}
	}
	p.t.Fatalf("the summary has no %s", livePage)
	return pipeline.PageResult{}
}

func TestLiveDraftFromBlueprint(t *testing.T) {
	if os.Getenv("DRAFTER_LIVE") != "1" {
		t.Skip("set DRAFTER_LIVE=1 (make live does) to spend on the live API")
	}
	p := newLiveProject(t)
	before := p.meter()
	t.Logf("meter at start: total %.4f, open worst %.4f, baseline %.4f", before.Total, before.Open.Worst, p.baseline)
	defer func() {
		after := p.meter()
		t.Logf("meter at end: total %.4f, open worst %.4f, spent %.4f of %.2f", after.Total, after.Open.Worst, after.Total-p.baseline, liveLimit)
	}()

	// Row 1: draft the page whole.
	s := p.page(livePage)
	r := p.accepted(s)
	if r.Scope == nil || r.Scope.Kind != blueprint.ScopeWhole || !strings.Contains(r.Scope.Why, "no drafted entry") {
		t.Fatalf("row 1 scope = %+v, want whole naming the missing lock entry", r.Scope)
	}
	t.Logf("row 1: run %s, cost $%.4f", s.RunID, r.Cost)
	bpBlob := p.gitText("rev-parse", "HEAD:"+liveBlueprint)
	b, _ := blueprint.Decode(liveBlueprint, []byte(p.read(liveBlueprint)))
	e := p.lockEntry()
	wantSections := blueprint.SectionHashes(b, []byte(p.read(livePage)))
	if e.Page != p.gitText("hash-object", livePage) || e.Blueprint != bpBlob || e.Accepted != bpBlob {
		t.Fatalf("row 1 lock entry = %+v; want the page's blob, and the blueprint's blob %s as blueprint and accepted", e, bpBlob)
	}
	if len(e.Sections) != len(wantSections) {
		t.Fatalf("row 1 lock sections = %v, want %v", e.Sections, wantSections)
	}
	for k, v := range wantSections {
		if e.Sections[k] != v {
			t.Fatalf("row 1 lock section %s = %s, want %s", k, e.Sections[k], v)
		}
	}
	p.gate()
	firstPage := p.read(livePage)
	p.commit("row 1: the drafted page", livePage, liveBrief, liveLock)

	// Row 2: edit one sentence item, commit, and redraft only that section.
	bp := p.read(liveBlueprint)
	edited := strings.Replace(bp, "Add items one at a time so that each one is counted.", "Add items one at a time, and wait for each count to print before you add the next.", 1)
	if edited == bp {
		t.Fatal("the fixture blueprint lacks the sentence row 2 edits")
	}
	if err := os.WriteFile(filepath.Join(p.dir, filepath.FromSlash(liveBlueprint)), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	p.commit("row 2: edit one sentence item", liveBlueprint)
	s = p.page(livePage)
	r = p.accepted(s)
	if r.Scope == nil || r.Scope.Kind != blueprint.ScopeScoped || !slices.Equal(r.Scope.IDs, []string{liveEditedID}) {
		t.Fatalf("row 2 scope = %+v, want scoped to exactly %s", r.Scope, liveEditedID)
	}
	t.Logf("row 2: run %s, cost $%.4f", s.RunID, r.Cost)
	rec := p.record(s.RunID)
	coherence := false
	for _, v := range rec.Verdicts {
		if v.Seat == "coherence" && v.Stage == "s8" {
			coherence = true
		}
	}
	if !coherence {
		t.Errorf("row 2: record.json holds no coherence verdict on stage s8: %+v", rec.Verdicts)
	}
	p.checkWrites(s.RunID, rec, firstPage)
	e2 := p.lockEntry()
	bp2 := p.gitText("rev-parse", "HEAD:"+liveBlueprint)
	if e2.Page != p.gitText("hash-object", livePage) || e2.Page == e.Page || e2.Blueprint != bp2 || e2.Accepted != e.Accepted {
		t.Errorf("row 2 lock entry = %+v; want a new page and blueprint blob, and accepted kept as %s", e2, e.Accepted)
	}
	p.gate()
	p.commit("row 2: the redrafted page", livePage, liveBrief, liveLock)

	// Row 3: nothing changed, so nothing runs and nothing is spent.
	idsBefore, costBefore := p.runIDs(), p.repoCost()
	s = p.page(livePage)
	if s.RunID != "" || len(s.Pages) != 1 || s.Pages[0].Status != "unchanged" || s.Pages[0].Scope == nil || s.Pages[0].Scope.Why != "up to date with drafted.lock" {
		t.Fatalf("row 3 summary = %+v, want the page up to date", s)
	}
	if got := p.runIDs(); !slices.Equal(got, idsBefore) {
		t.Errorf("row 3 made a run: run ids %v, were %v", got, idsBefore)
	}
	if got := p.repoCost(); got != costBefore {
		t.Errorf("row 3 spent: the repo's cost is %v, was %v", got, costBefore)
	}
}

func (p *liveProject) record(runID string) liveRecord {
	p.t.Helper()
	path := filepath.Join(p.stateDir, "runs", filepath.Base(p.dir), runID, runstate.PageKey(livePage), "record.json")
	data, err := os.ReadFile(path)
	if err != nil {
		p.t.Fatal(err)
	}
	var rec liveRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		p.t.Fatal(err)
	}
	return rec
}

// checkWrites compares each write of the run with the page before it. A bounded write (a section,
// redraft, or final exchange that is not a gate fix) must leave everything outside the edited
// section as it was, and fails the row otherwise. A difference a gate-fix or coherence write
// makes outside it is reported, not failed.
func (p *liveProject) checkWrites(runID string, rec liveRecord, firstPage string) {
	p.t.Helper()
	b, _ := blueprint.Decode(liveBlueprint, []byte(p.read(liveBlueprint)))
	if len(rec.Writes) == 0 {
		p.t.Fatal("row 2: record.json holds no write log")
	}
	var headings []string
	editedAt := -1
	for i, s := range b.Sections {
		headings = append(headings, s.Heading)
		if s.ID == liveEditedID {
			editedAt = i
		}
	}
	before := firstPage
	for _, w := range rec.Writes {
		file := filepath.Join(p.stateDir, "runs", filepath.Base(p.dir), runID, runstate.PageKey(livePage), fmt.Sprintf("write%d.page.md", w.N))
		data, err := os.ReadFile(file)
		if err != nil {
			p.t.Fatal(err)
		}
		after := string(data)
		outside := outsideDiff(headings, editedAt, before, after)
		bounded := !w.GateFix && (w.Kind == "section" || w.Kind == "redraft" || w.Kind == "final")
		switch {
		case outside == "":
		case bounded:
			p.t.Errorf("row 2: bounded write %d (%s, %s) changed the page outside %q:\n%s", w.N, w.Step, w.Kind, liveEditedID, outside)
		default:
			p.t.Logf("row 2: write %d (%s, %s, gate fix %v) changed the page outside %q:\n%s", w.N, w.Step, w.Kind, w.GateFix, liveEditedID, outside)
		}
		before = after
	}
}

// outsideDiff names the parts of the page, outside the section at edited, that differ between
// the two texts: the preamble, the other sections, and the heading lines.
func outsideDiff(headings []string, edited int, before, after string) string {
	preB, textB, _ := coverage.Regions(headings, []byte(before))
	preA, textA, _ := coverage.Regions(headings, []byte(after))
	var diff []string
	if preB != preA {
		diff = append(diff, "preamble")
	}
	for i := range headings {
		if i != edited && (i >= len(textB) || i >= len(textA) || textB[i] != textA[i]) {
			diff = append(diff, "section "+headings[i])
		}
	}
	if headingLines(before) != headingLines(after) {
		diff = append(diff, "heading lines")
	}
	return strings.Join(diff, ", ")
}

func headingLines(page string) string {
	var out []string
	for line := range strings.SplitSeq(page, "\n") {
		if strings.HasPrefix(line, "#") {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}
