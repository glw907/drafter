package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/edits"
	"github.com/glw907/drafter/internal/gate"
	"github.com/glw907/drafter/internal/plan"
)

// hasReadItems reports whether the plan has an item the coverage read answers: a claim
// sentence, a handoff, or a reason.
func hasReadItems(p *plan.Plan) bool {
	for _, s := range p.Sections {
		for _, it := range s.Items {
			if it.Kind == "handoff" || it.Kind == "reason" || (it.Kind == "sentence" && it.Mode == "claim") {
				return true
			}
		}
	}
	return false
}

// doPlan is stage 1's local half: the plan check. A markdown plan skips it with a warning.
func (p *page) doPlan() error {
	if p.markdown {
		p.w.Rec.warn("the plan is markdown, so the plan-dependent checks are skipped")
		p.ps.Step = stDraft
		return nil
	}
	opts := plan.CheckOptions{Root: p.r.root, FactsDir: p.r.cfg.Facts.Dir}
	var files []string
	var args struct {
		Args struct {
			Outline string `json:"outline"`
		} `json:"args"`
	}
	if len(p.h.Context) > 0 {
		if err := json.Unmarshal(p.h.Context, &args); err == nil && args.Args.Outline != "" {
			files = append(files, p.abs(args.Args.Outline))
		}
	}
	var pages []string
	for _, h := range p.r.o.Handoffs {
		pages = append(pages, h.Page.Path)
	}
	outline, err := plan.OutlineWithPages(files, pages)
	if err != nil {
		return err
	}
	opts.Outline = outline
	findings, err := plan.CheckFile(p.abs(p.h.Plan), opts)
	if err != nil {
		return err
	}
	var errs []string
	for _, f := range findings {
		line := fmt.Sprintf("%s:%d: %s: %s", relTo(p.r.root, f.File), f.Line, f.Check, f.Message)
		if f.Severity == plan.SeverityError {
			errs = append(errs, line)
		} else {
			p.w.Rec.warn(line)
		}
	}
	if len(errs) > 0 {
		return p.escalate("plan", "check-plan", nil, strings.Join(errs, "\n"))
	}
	if p.runs("consistency") {
		p.ps.Step = stConsistency
	} else {
		p.ps.Step = stDraft
	}
	return nil
}

// gateStep runs the index-link command and the gate after a write and moves the page on: to the
// coverage check on a green gate, to a hold on a red one with nothing of the page's own, and to
// a fix exchange on a red one that names the page.
func (p *page) gateStep(ctx context.Context, kind string) error {
	err := p.gateStepOnce(ctx, kind)
	if err == nil && p.r.stop == "" {
		p.w.PendingGate = ""
	}
	return err
}

func (p *page) gateStepOnce(ctx context.Context, kind string) error {
	if err := p.indexLink(ctx); err != nil {
		return err
	}
	res, own, foreign, err := p.runGate(ctx)
	if err != nil || p.r.stop != "" {
		return err
	}
	switch {
	case res.Green:
		p.toCoverage(coveragePhase(kind))
		p.w.Held = nil
	case foreign:
		p.w.Held = &held{Kind: kind, OnGreen: coveragePhase(kind)}
		p.ps.Status = StatusHeld
	default:
		return p.redGate(kind, own)
	}
	return nil
}

// coveragePhase names the coverage check that follows a write of the exchange kind. A section
// exchange takes the place of the draft, so its check is the initial one.
func coveragePhase(kind string) string {
	switch kind {
	case exRedraft:
		return covRedraft
	case exFinal:
		return covFinal
	case exCoherence:
		return covCoherence
	}
	return covInitial
}

// exchangeStep names the step that asks the exchange kind's next turn, a gate-fix turn included.
func exchangeStep(kind string) string {
	switch kind {
	case exRedraft:
		return stRedraft
	case exFinal:
		return stFinalRedraft
	case exSection:
		return stSection
	case exCoherence:
		return stCoherenceFix
	}
	return stGateFix
}

// runGate runs the project's gate once and splits a red result into the sections that name this
// page and whether the red names nothing of the page's.
func (p *page) runGate(ctx context.Context) (res gate.Result, own []gate.Section, foreign bool, err error) {
	p.w.Gates++
	logDir := p.dir
	if logDir == "" {
		logDir = os.TempDir()
	}
	log := filepath.Join(logDir, fmt.Sprintf("gate-%d.log", p.w.Gates))
	cmd := gate.Command(p.r.cfg.Gate.Command, p.h.Page.Path, p.h.Page.Brief, p.slug(), p.w.Baseline)
	res, err = gate.Run(ctx, p.r.cfg.Gate, gate.Env(p.r.cfg), p.r.root, cmd, log)
	if errors.Is(err, gate.ErrInfra) {
		p.r.halt(StopInfrastructure)
		return res, nil, false, nil
	}
	if err != nil {
		return res, nil, false, fmt.Errorf("run the gate for %s: %w", p.h.Page.Path, err)
	}
	if res.Green {
		return res, nil, false, nil
	}
	own, foreign = res.ForPage(p.h.Page.Path, p.h.Page.Brief)
	if res.Unparsed {
		p.w.Rec.GateExcerpts = append(p.w.Rec.GateExcerpts, "gate_log: unparsed")
	}
	for _, s := range own {
		p.w.Rec.GateExcerpts = append(p.w.Rec.GateExcerpts, excerpt(s))
	}
	return res, own, foreign, nil
}

// excerpt is a section's first lines, for the record.
func excerpt(s gate.Section) string {
	ls := s.Failing
	if len(ls) == 0 {
		ls = s.Lines
	}
	if len(ls) > 20 {
		ls = ls[:20]
	}
	return strings.Join(ls, "\n")
}

// gateFindings turns the failing sections into blocking findings, numbered from first.
func gateFindings(own []gate.Section, first int) []edits.Finding {
	var out []edits.Finding
	for _, s := range own {
		text := excerpt(s)
		loc := s.Name
		if loc == "" {
			loc = "the gate log"
		}
		out = append(out, edits.Finding{
			N: first + len(out), Seat: "gate", Location: loc, Text: text,
			Rewrite: "Change the page or brief so this gate section passes.", Blocking: true,
		})
	}
	return out
}

// redGate opens or continues a fix exchange for a gate that names the page, inside the budget
// of its stop-table row.
func (p *page) redGate(kind string, own []gate.Section) error {
	if kind == exDraft {
		if p.w.spend("s2.gatefix") > p.r.cfg.Retries.GateFix {
			return p.escalate("red gate", "gate", nil, strings.Join(p.w.Rec.GateExcerpts, "\n"))
		}
		if p.w.Ex == nil {
			p.w.Ex = &exchange{Kind: exDraft, History: p.w.DraftHist}
		}
	} else if p.w.spend(kind+".contract") > p.r.cfg.Retries.EditContract {
		return p.escalate("contract", "gate", nil, "the gate stayed red after the edits applied")
	}
	ex := p.w.Ex
	ex.GateFix = true
	first := 1
	for _, f := range ex.Findings {
		first = max(first, f.N+1)
	}
	fs := gateFindings(own, first)
	ex.Findings = append(ex.Findings, fs...)
	p.w.Rec.findings(exchangeStage(kind), fs)
	ex.Turn = "The gate is red with the page and brief as they now stand, which are inlined below. " +
		"Findings from the gate:\n\n" + edits.Render(fs) +
		"\nResolutions you gave earlier stand. Resolve each new finding."
	p.w.Got = nil
	p.ps.Step = exchangeStep(kind)
	return nil
}

// regate runs a held page's gate again. It reports whether the page left its hold.
func (p *page) regate(ctx context.Context) (bool, error) {
	h := p.w.Held
	if h == nil {
		p.ps.Status = StatusRunning
		return true, nil
	}
	res, own, foreign, err := p.runGate(ctx)
	if err != nil || p.r.stop != "" {
		return false, err
	}
	switch {
	case res.Green:
		p.ps.Status = StatusRunning
		p.w.Held = nil
		p.toCoverage(h.OnGreen)
	case foreign:
		return false, nil
	default:
		p.ps.Status = StatusRunning
		p.w.Held = nil
		if err := p.redGate(h.Kind, own); err != nil {
			return false, err
		}
	}
	return true, nil
}

// porcelain reads the project's changed files as path to status code.
func (p *page) porcelain() (map[string]string, error) {
	out, err := p.r.gitOut("status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	for _, ln := range lines(out) {
		if len(ln) < 4 {
			continue
		}
		path := ln[3:]
		if _, to, ok := strings.Cut(path, " -> "); ok {
			path = to
		}
		m[strings.Trim(path, `"`)] = ln[:2]
	}
	return m, nil
}

// indexLink runs the index-link command, if the project sets one, and keeps what each file it
// changed was before, so an escalation can put it back.
func (p *page) indexLink(ctx context.Context) error {
	tmpl := p.r.cfg.Gate.IndexLinkCommand
	if tmpl == "" {
		return nil
	}
	before, err := p.porcelain()
	if err != nil {
		return err
	}
	prior := map[string]*string{}
	for path := range before {
		if data, ok, err := p.readFile(path); err != nil {
			return err
		} else if ok {
			prior[path] = &data
		} else {
			prior[path] = nil
		}
	}
	logDir := p.dir
	if logDir == "" {
		logDir = os.TempDir()
	}
	cmd := gate.Command(tmpl, p.h.Page.Path, p.h.Page.Brief, p.slug(), p.w.Baseline)
	res, err := gate.Run(ctx, config.Gate{Protocol: "plain"}, gate.Env(p.r.cfg), p.r.root, cmd, filepath.Join(logDir, "index-link.log"))
	if err != nil {
		return fmt.Errorf("run the index-link command for %s: %w", p.h.Page.Path, err)
	}
	if !res.Green {
		p.w.Rec.Friction = append(p.w.Rec.Friction, "index-link: the command exited nonzero")
	}
	after, err := p.porcelain()
	if err != nil {
		return err
	}
	for path, code := range after {
		if path == p.h.Page.Path || path == p.h.Page.Brief {
			continue
		}
		now, _, err := p.readFile(path)
		if err != nil {
			return err
		}
		if pre, was := prior[path]; was && pre != nil && *pre == now {
			continue
		}
		if p.w.IndexPost == nil {
			p.w.IndexPost = map[string]string{}
		}
		p.w.IndexPost[path] = hashBytes([]byte(now))
		if _, recorded := p.w.IndexPre[path]; recorded {
			continue
		}
		if p.w.IndexPre == nil {
			p.w.IndexPre = map[string]*string{}
		}
		switch pre, was := prior[path]; {
		case was:
			p.w.IndexPre[path] = pre
		case code == "??":
			p.w.IndexPre[path] = nil
		default:
			head, err := p.r.gitOut("show", "HEAD:"+path)
			if err != nil {
				return err
			}
			p.w.IndexPre[path] = &head
		}
	}
	return nil
}
