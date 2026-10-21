package pipeline

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/glw907/drafter/internal/atomicfile"
	"github.com/glw907/drafter/internal/blueprint"
	"github.com/glw907/drafter/internal/gate"
	"github.com/glw907/drafter/internal/handoff"
	"github.com/glw907/drafter/internal/plan"
	"github.com/glw907/drafter/internal/prompts"
	"github.com/glw907/drafter/internal/run"
)

// page is one handoff's page in a run.
type page struct {
	r   *runner
	h   handoff.Handoff
	key string
	ps  *run.PageState
	w   work
	// dir is the page's record directory in the run directory; empty in a dry run.
	dir string
	mod *prompts.Module
	pl  *plan.Plan
	// plan format: markdown plans carry no plan.Plan.
	markdown bool
	// blueprint marks a page whose plan is a blueprint, which stage 1 reads and, unless the run
	// drafts, stops at.
	blueprint bool
	bp        *bpInputs
}

func (r *runner) newPage(h handoff.Handoff) (*page, error) {
	p := &page{r: r, h: h, key: run.PageKey(h.Page.Path)}
	p.blueprint = strings.HasSuffix(h.Plan, ".blueprint.yaml") || p.mdBlueprint()
	p.markdown = !p.blueprint && !strings.HasSuffix(h.Plan, ".yaml") && !strings.HasSuffix(h.Plan, ".yml")
	if !r.o.DryRun {
		p.dir = filepath.Join(r.dir, p.key)
		if err := os.MkdirAll(p.dir, 0o755); err != nil {
			return nil, fmt.Errorf("create record directory of %s: %w", h.Page.Path, err)
		}
	}
	return p, nil
}

// mdBlueprint reports whether the page's plan is a blueprint in the Markdown format.
func (p *page) mdBlueprint() bool { return strings.HasSuffix(p.h.Plan, ".blueprint.md") }

func (p *page) abs(rel string) string { return filepath.Join(p.r.root, filepath.FromSlash(rel)) }

// files lists the page and its brief, the two files the page writes.
func (p *page) files() []string { return []string{p.h.Page.Path, p.h.Page.Brief} }

// readFile returns the file's content and whether it exists.
func (p *page) readFile(rel string) (string, bool, error) {
	data, err := os.ReadFile(p.abs(rel))
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read %s: %w", rel, err)
	}
	return string(data), true, nil
}

func (p *page) hashOf(rel string) (string, error) {
	data, ok, err := p.readFile(rel)
	if err != nil || !ok {
		return "", err
	}
	return hashBytes([]byte(data)), nil
}

// snapshotPre records each file's hash before the run and keeps its bytes in the record
// directory, so an escalation can put the tree back.
func (p *page) snapshotPre() error {
	for i, rel := range p.files() {
		data, ok, err := p.readFile(rel)
		if err != nil {
			return err
		}
		if !ok {
			p.ps.Pre[rel] = ""
			continue
		}
		p.ps.Pre[rel] = hashBytes([]byte(data))
		if p.dir == "" {
			continue
		}
		if err := atomicfile.WriteFile(filepath.Join(p.dir, preName(i)), []byte(data), 0o644); err != nil {
			return fmt.Errorf("snapshot %s: %w", rel, err)
		}
	}
	return nil
}

func preName(i int) string {
	if i == 0 {
		return "pre.page.md"
	}
	return "pre.brief.json"
}

func finalName(i int) string {
	if i == 0 {
		return "final.page.md"
	}
	return "final.brief.json"
}

// initRecord fills the parts of the record known at start.
func (p *page) initRecord() {
	rec := &p.w.Rec
	rec.Page, rec.Brief, rec.Plan = p.h.Page.Path, p.h.Page.Brief, p.h.Plan
	rec.Mode = p.r.cfg.Mode
	rec.PlanFormat = "yaml"
	switch {
	case p.markdown:
		rec.PlanFormat = "markdown"
	case p.blueprint:
		rec.PlanFormat = "blueprint"
	}
	rec.Facts = "store"
	if p.r.cfg.Facts.Dir == "" {
		rec.Facts = "none"
	}
	for _, seat := range []string{"structure", "editor", "fact", "reader", "coverage", "consistency"} {
		if !p.runs(seat) {
			rec.SeatsNotRun = append(rec.SeatsNotRun, seat)
		}
	}
	hash := func(name, rel string) {
		if rel == "" {
			return
		}
		if h, err := hashFile(p.abs(rel)); err == nil {
			rec.InputHashes[name] = h
		}
	}
	hash("plan", p.h.Plan)
	hash("framing", p.h.Framing)
	for role, in := range p.r.state.Inputs {
		rec.InputHashes[role] = in.Hash
	}
}

// runs reports whether the seat is configured to run on this page. The consistency read also
// needs a facts store.
func (p *page) runs(seat string) bool {
	if p.w.Narrow && seat != "draft" {
		return false
	}
	if seat == "consistency" && p.r.cfg.Facts.Dir == "" {
		return false
	}
	return slices.Contains(p.r.cfg.Seats, seat)
}

// missingSeats lists the seats this page renders through its module that the module lacks. A
// blueprint page that does not draft renders only the blueprint seat, since stage 1 reads it and
// stops. One that drafts renders the blueprint seat unless its read is skipped, then every seat a
// drafted page renders. A scoped page that names sections also renders the coherence seat: the run
// checks seats before it decides whether the page is a section redraft, which any such page may
// become.
func (p *page) missingSeats() []string {
	need := []string{"blueprint"}
	if !p.blueprint || p.w.Draft {
		need = nil
		if p.blueprint && !p.w.SkipRead {
			need = append(need, "blueprint")
		}
		need = append(need, "draft")
		for _, seat := range []string{"structure", "editor", "fact", "reader"} {
			if p.runs(seat) {
				need = append(need, seat)
			}
		}
		if p.w.Edit {
			need = append(need, "edit")
		}
		if p.blueprint && !p.w.Narrow && p.w.Scope != nil && p.w.Scope.Kind == blueprint.ScopeScoped && len(p.w.Scope.IDs) > 0 {
			need = append(need, "coherence")
		}
	}
	have := p.mod.Seats()
	var missing []string
	for _, seat := range need {
		if !slices.Contains(have, seat) {
			missing = append(missing, seat)
		}
	}
	return missing
}

func (p *page) slug() string {
	return strings.TrimSuffix(path.Base(p.h.Page.Path), path.Ext(p.h.Page.Path))
}

// planDoc returns the decoded plan, loading it once. A blueprint page's plan is the blueprint's
// projection, as committed at the run's start.
func (p *page) planDoc() (*plan.Plan, error) {
	if p.pl != nil || p.markdown {
		return p.pl, nil
	}
	if p.blueprint {
		in, err := p.blueprintInputs()
		if err != nil {
			return nil, err
		}
		if in.view == nil {
			return nil, fmt.Errorf("blueprint %s does not decode", p.h.Plan)
		}
		p.pl = in.view
		return in.view, nil
	}
	data, err := os.ReadFile(p.abs(p.h.Plan))
	if err != nil {
		return nil, fmt.Errorf("read plan: %w", err)
	}
	doc, fs := plan.Decode(p.h.Plan, data)
	if doc == nil || plan.HasError(fs) {
		return nil, fmt.Errorf("plan %s does not decode", p.h.Plan)
	}
	p.pl = doc
	return doc, nil
}

// writeFiles records the page's write, then writes the page and brief atomically and records the
// hashes they now have. The saved contents let a resume redo an interrupted write at no cost. ex is
// the exchange whose answer the write applies, nil for a whole draft, and the record's write log
// names it.
func (p *page) writeFiles(pageText, briefText string, ex *exchange) error {
	contents := map[string]string{p.h.Page.Path: pageText, p.h.Page.Brief: briefText}
	w := run.Write{Pre: map[string]string{}, Saved: map[string]string{}}
	n := len(p.ps.Writes) + 1
	for i, rel := range p.files() {
		h, err := p.hashOf(rel)
		if err != nil {
			return err
		}
		w.Pre[rel] = h
		name := fmt.Sprintf("write%d.%s", n, strings.TrimPrefix(finalName(i), "final."))
		w.Saved[rel] = name
		if p.dir != "" {
			if err := atomicfile.WriteFile(filepath.Join(p.dir, name), []byte(contents[rel]), 0o644); err != nil {
				return fmt.Errorf("save answer for %s: %w", rel, err)
			}
		}
	}
	p.ps.Writes = append(p.ps.Writes, w)
	logged := writeRecord{N: n, Step: p.ps.Step}
	if ex != nil {
		logged.Kind, logged.GateFix = ex.Kind, ex.GateFix
	}
	p.w.Rec.Writes = append(p.w.Rec.Writes, logged)
	if err := p.r.save(); err != nil {
		return err
	}
	for _, rel := range p.files() {
		if err := os.MkdirAll(filepath.Dir(p.abs(rel)), 0o755); err != nil {
			return fmt.Errorf("create directory for %s: %w", rel, err)
		}
		if err := atomicfile.WriteFile(p.abs(rel), []byte(contents[rel]), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", rel, err)
		}
	}
	post := map[string]string{}
	for _, rel := range p.files() {
		post[rel] = hashBytes([]byte(contents[rel]))
	}
	p.ps.Writes[len(p.ps.Writes)-1].Post = post
	return p.r.save()
}

// current returns the page and brief as they are on disk.
func (p *page) current() (pageText, briefText string, err error) {
	pageText, _, err = p.readFile(p.h.Page.Path)
	if err != nil {
		return "", "", err
	}
	briefText, _, err = p.readFile(p.h.Page.Brief)
	return pageText, briefText, err
}

// finalize ends the page: the final texts and their hashes go to the record directory, an
// escalated page's files go back to what they were, and record.json is written. It is safe to
// run again after a crash.
func (p *page) finalize(status string) error {
	p.ps.Status = status
	p.ps.Step = stDone
	p.w.Rec.FinalStatus = status
	if p.dir == "" {
		return nil
	}
	for i, rel := range p.files() {
		name := finalName(i)
		saved := filepath.Join(p.dir, name)
		if prior, err := os.ReadFile(saved); err == nil {
			// A crash after the finals and before the restore left them; the page files may
			// already be restored, so the saved copy is the final.
			p.w.Rec.FinalHashes[name] = hashBytes(prior)
			continue
		}
		data, ok, err := p.readFile(rel)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if err := atomicfile.WriteFile(saved, []byte(data), 0o644); err != nil {
			return fmt.Errorf("save %s: %w", name, err)
		}
		p.w.Rec.FinalHashes[name] = hashBytes([]byte(data))
	}
	if status == StatusEscalated {
		if err := p.restore(); err != nil {
			return err
		}
	}
	p.ps.Restored = true
	cost, err := p.r.led.PageSpend(p.r.state.RunID, p.h.Page.Path)
	if err != nil {
		return err
	}
	p.w.Rec.Cost = cost
	return writeJSON(filepath.Join(p.dir, "record.json"), p.w.Rec)
}

// restore puts the page and its brief back to their pre-run content, or removes them when the
// run created them, and then does the same for the files the index-link command changed.
func (p *page) restore() error {
	w := run.Write{Pre: map[string]string{}, Saved: map[string]string{}, Restore: true}
	for i, rel := range p.files() {
		h, err := p.hashOf(rel)
		if err != nil {
			return err
		}
		w.Pre[rel] = h
		if p.ps.Pre[rel] != "" {
			w.Saved[rel] = preName(i)
		}
	}
	if len(p.ps.Writes) == 0 || !p.ps.Writes[len(p.ps.Writes)-1].Restore {
		p.ps.Writes = append(p.ps.Writes, w)
	}
	if err := p.r.save(); err != nil {
		return err
	}
	for i, rel := range p.files() {
		if err := p.restoreFile(rel, p.ps.Pre[rel] != "", func() ([]byte, error) {
			return os.ReadFile(filepath.Join(p.dir, preName(i)))
		}); err != nil {
			return err
		}
	}
	for rel, pre := range p.w.IndexPre {
		// Another page's link command may have changed the file since this page's did; the
		// restore then leaves it, which keeps that page's link and records the dangling one.
		now, err := p.hashOf(rel)
		if err != nil {
			return err
		}
		if want, ok := p.w.IndexPost[rel]; ok && now != want {
			p.w.Rec.Friction = append(p.w.Rec.Friction, "index-link: "+rel+" changed after this page's link, so it was not restored")
			continue
		}
		if err := p.restoreFile(rel, pre != nil, func() ([]byte, error) { return []byte(*pre), nil }); err != nil {
			return err
		}
	}
	post := map[string]string{}
	for _, rel := range p.files() {
		h, err := p.hashOf(rel)
		if err != nil {
			return err
		}
		post[rel] = h
	}
	p.ps.Writes[len(p.ps.Writes)-1].Post = post
	return p.r.save()
}

func (p *page) restoreFile(rel string, existed bool, content func() ([]byte, error)) error {
	if !existed {
		if err := os.Remove(p.abs(rel)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", rel, err)
		}
		return nil
	}
	data, err := content()
	if err != nil {
		return fmt.Errorf("read snapshot of %s: %w", rel, err)
	}
	if err := atomicfile.WriteFile(p.abs(rel), data, 0o644); err != nil {
		return fmt.Errorf("restore %s: %w", rel, err)
	}
	return nil
}

// escalate ends the page on a stop rule.
func (p *page) escalate(rule, seat string, findings []int, detail string) error {
	p.w.Rec.Escalation = &escalation{Rule: rule, Seat: seat, Findings: findings, Detail: detail}
	if findings == nil {
		p.w.Rec.Escalation.Findings = []int{}
	}
	p.leftUnresolved()
	return p.finalize(StatusEscalated)
}

// accept ends the page as accepted.
func (p *page) accept() error { return p.finalize(StatusAccepted) }

// leftUnresolved records the blocking findings the page ends without a resolution for.
func (p *page) leftUnresolved() {
	ex := p.w.Ex
	if ex == nil {
		return
	}
	resolved := map[int]bool{}
	for _, r := range ex.Resolutions {
		resolved[r.Finding] = true
	}
	for _, f := range ex.Findings {
		if f.Blocking && !resolved[f.N] {
			p.w.Rec.fault("blocking finding %d (%s) ended with no resolution", f.N, f.Seat)
		}
	}
}

// gitHashObject stores the file in the project's object database and returns its ID.
func (p *page) gitHashObject(file string) (string, error) {
	cmd := exec.Command("git", "-C", p.r.root, "hash-object", "-w", "--", file)
	cmd.Env = gate.Env(p.r.o.Config)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git hash-object: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func marshalRaw(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic("pipeline: value does not marshal: " + err.Error())
	}
	return b
}
