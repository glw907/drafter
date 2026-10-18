package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/glw907/drafter/internal/assemble"
	"github.com/glw907/drafter/internal/batch"
	"github.com/glw907/drafter/internal/blueprint"
	"github.com/glw907/drafter/internal/plan"
	"github.com/glw907/drafter/internal/prompts"
)

// bpInputs is what a blueprint page's stage 1 reads, all of it from the run's start commit so one
// run reads one version of the blueprint and its outline entry.
type bpInputs struct {
	raw       []byte
	bp        *blueprint.Blueprint
	entryJSON []byte
	view      *plan.Plan
	findings  []plan.Finding
}

// atHead returns the committed content of the repo-relative path at the run's start commit.
func (p *page) atHead(rel string) ([]byte, error) {
	out, err := p.r.gitOut("show", p.r.state.Head+":"+rel)
	if err != nil {
		return nil, err
	}
	return []byte(out), nil
}

// outlineEntry is the outline entry as the blueprint read sees it.
type outlineEntry struct {
	Path       string   `json:"path"`
	Slug       string   `json:"slug,omitempty"`
	Title      string   `json:"title"`
	Type       string   `json:"type"`
	Job        string   `json:"job"`
	Covers     []string `json:"covers,omitempty"`
	OutOfScope []string `json:"outOfScope,omitempty"`
	Figure     bool     `json:"figure,omitempty"`
}

// EntryJSON is an outline entry as the blueprint read shows it.
func EntryJSON(e blueprint.Page) ([]byte, error) {
	return json.MarshalIndent(outlineEntry{
		Path: e.Path, Slug: e.Slug, Title: e.Title, Type: e.Type, Job: e.Job,
		Covers: e.Covers, OutOfScope: e.OutOfScope, Figure: e.Figure,
	}, "", "  ")
}

// changeRead reports whether the page's read judges a change: its comparison is scoped and carries
// the facts it touched. A scoped state from before Scope.Facts existed reads as whole.
func (p *page) changeRead() bool {
	return p.w.Scope != nil && p.w.Scope.Kind == blueprint.ScopeScoped && p.w.Scope.Facts != nil
}

// blueprintInputs loads the page's blueprint and outline at the start commit and runs the
// blueprint checks on them, once.
func (p *page) blueprintInputs() (*bpInputs, error) {
	if p.bp != nil {
		return p.bp, nil
	}
	raw, err := p.atHead(p.h.Plan)
	if err != nil {
		return nil, fmt.Errorf("read the blueprint of %s at the run's start commit: %w", p.h.Page.Path, err)
	}
	outline, err := p.atHead(p.r.cfg.Blueprints.Outline)
	if err != nil {
		return nil, fmt.Errorf("read the outline at the run's start commit: %w", err)
	}
	proj := blueprint.Project{
		Root: p.r.root, BlueprintRoot: p.r.cfg.Blueprints.Root, Outline: p.r.cfg.Blueprints.Outline,
		FactsDir: p.r.cfg.Facts.Dir,
	}
	bp, entry, findings, err := proj.CheckSource(p.h.Plan, raw, outline)
	if err != nil {
		return nil, err
	}
	in := &bpInputs{raw: raw, bp: bp, findings: findings}
	if bp != nil {
		in.view = blueprint.View(bp, entry, p.r.facts)
		if in.entryJSON, err = EntryJSON(entry); err != nil {
			return nil, err
		}
	}
	p.bp = in
	return in, nil
}

// doBlueprintCheck is stage 1's local half for a blueprint: the blueprint checks, offline, on the
// version committed at the run's start.
func (p *page) doBlueprintCheck() error {
	in, err := p.blueprintInputs()
	if err != nil {
		return err
	}
	var errs []string
	for _, f := range in.findings {
		line := fmt.Sprintf("%s:%d: %s: %s", f.File, f.Line, f.Check, f.Message)
		if f.Severity == plan.SeverityError {
			errs = append(errs, line)
		} else {
			p.w.Rec.warn(line)
		}
	}
	if len(errs) > 0 {
		return p.escalate("plan", "check", nil, strings.Join(errs, "\n"))
	}
	p.ps.Step = stBpRead
	return nil
}

// stageBlueprintRead sends the consistency read, when the project has a facts store, and the
// blueprint read in one stage-1 batch, asking again only the seats with no accepted answer.
func (p *page) stageBlueprintRead(ctx context.Context) (*stage, error) {
	in, err := p.blueprintInputs()
	if err != nil {
		return nil, err
	}
	var reqs []batch.Request
	for _, seat := range p.pending(p.stepSeats()) {
		var req batch.Request
		switch seat {
		case "consistency":
			prompt, err := assemble.ConsistencyPrompt(in.view, p.r.facts, p.r.root)
			if err != nil {
				return nil, err
			}
			req, err = p.build(ctx, s1, seat, assemble.AnswerConsistency, prompt, nil, nil)
			if err != nil {
				return nil, err
			}
		default:
			c := p.context(prompts.Round1)
			extra := assemble.BlueprintBlock(in.view, in.raw, in.entryJSON, p.r.facts)
			if p.changeRead() {
				c.Scope = "page"
				if len(p.w.Scope.IDs) > 0 {
					c.Scope = strings.Join(p.w.Scope.IDs, ",")
				}
				extra += "\n\n" + p.w.Change
			}
			prompt, err := p.render(seat, c)
			if err != nil {
				return nil, err
			}
			req, err = p.buildInput(ctx, s1, seat, assemble.Input{Answer: assemble.AnswerRead, Prompt: prompt, Extra: extra})
			if err != nil {
				return nil, err
			}
		}
		reqs = append(reqs, req)
	}
	return &stage{page: p, name: s1, reqs: reqs}, nil
}

// untouched reports whether a consistency conflict, between the facts ids and with, lies wholly
// outside what a change read touched. It fails closed: a conflict that names no fact, a fact the
// store lacks, or a with that is neither a store fact nor a page the plan links is touched. with
// names a page as page#heading, so the heading is cut before the page is looked up.
func (p *page) untouched(in *bpInputs, ids []string, with string) bool {
	if !p.changeRead() || p.r.facts == nil || len(ids) == 0 {
		return false
	}
	touched := p.w.Scope.Facts
	for _, id := range ids {
		if p.r.facts.Count(id) == 0 || slices.Contains(touched, id) {
			return false
		}
	}
	if p.r.facts.Count(with) > 0 {
		return !slices.Contains(touched, with)
	}
	file, _, _ := strings.Cut(with, "#")
	return slices.Contains(assemble.LinkedPages(in.view), file)
}

// completeBlueprintRead decides the page from the two stage-1 reads. A consistency conflict or a
// blueprint read that says fix sets the page aside under the plan rule, and an accepted page
// records the blob id of the blueprint the read saw. When both fire, the escalation names the
// consistency seat. The blueprint read's findings are in the record whatever its verdict. A change
// read demotes a conflict that lies outside what the change touched to a warning, and the page
// goes on to the blueprint verdict.
func (p *page) completeBlueprintRead() error {
	a, err := parseRead(p.w.Got["blueprint"])
	if err != nil {
		return err
	}
	p.w.Rec.verdict(s1, "blueprint", a.Verdict)
	p.w.Rec.addFriction("blueprint", a.Friction)
	fs := a.findings("blueprint", 0)
	p.w.Rec.findings(s1, fs)
	if raw, ok := p.w.Got["consistency"]; ok {
		var c consistencyAnswer
		if err := decodeAnswer(raw, &c); err != nil {
			return err
		}
		if len(c.Conflicts) > 0 {
			in, err := p.blueprintInputs()
			if err != nil {
				return err
			}
			var parts []string
			touched := false
			for _, x := range c.Conflicts {
				parts = append(parts, fmt.Sprintf("%s conflicts with %s: %s", strings.Join(x.Facts, ", "), x.With, x.Conflict))
				touched = touched || !p.untouched(in, x.Facts, x.With)
			}
			if touched {
				p.w.Rec.verdict(s1, "consistency", "conflicts")
				return p.escalate("plan", "consistency", nil, strings.Join(parts, "; "))
			}
			p.w.Rec.verdict(s1, "consistency", "demoted")
			p.w.Demoted = len(parts)
			for _, part := range parts {
				p.w.Rec.warn(alreadyPresentPrefix + " " + part)
			}
		} else {
			p.w.Rec.verdict(s1, "consistency", "ok")
		}
	}
	if a.Verdict == "fix" {
		ns := make([]int, 0, len(fs))
		for _, f := range fs {
			ns = append(ns, f.N)
		}
		return p.escalate("plan", "blueprint", ns, "the blueprint read returned fix")
	}
	id, err := p.r.gitOut("rev-parse", p.r.state.Head+":"+p.h.Plan)
	if err != nil {
		return errors.Join(errors.New("find the accepted blueprint's blob id"), err)
	}
	p.w.Accepted = strings.TrimSpace(id)
	return p.accept()
}
