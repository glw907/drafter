package pipeline

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/glw907/drafter/internal/assemble"
	"github.com/glw907/drafter/internal/batch"
	"github.com/glw907/drafter/internal/blueprint"
	"github.com/glw907/drafter/internal/edits"
	"github.com/glw907/drafter/internal/plan"
)

// factTruthRecord is the fact-truth read's part of a page's record. It is also the read's saved
// state: Planned says the set was decided, once, at the first request, so a resume asks about the
// same facts, and BarFacts are the facts a bar's fact-truth findings named, which the run was
// given and a resume is not.
type factTruthRecord struct {
	BarFacts []string `json:"bar_facts,omitempty"`
	Planned  bool     `json:"planned"`
	// Asked are the facts the request carries and NoExcerpt the facts Go answered itself, for
	// want of a source or of an excerpt that shows one.
	Asked     []string            `json:"asked"`
	NoExcerpt []string            `json:"no_excerpt,omitempty"`
	Verdicts  []factVerdictRecord `json:"verdicts"`
}

// factVerdictRecord is one fact's verdict, holds included.
type factVerdictRecord struct {
	ID      string `json:"id"`
	Verdict string `json:"verdict"`
	Excerpt string `json:"excerpt,omitempty"`
	Fix     string `json:"fix,omitempty"`
}

type factTruthAnswer struct {
	Facts []struct {
		ID      string `json:"id"`
		Verdict string `json:"verdict"`
		Excerpt string `json:"excerpt"`
		Fix     string `json:"fix"`
	} `json:"facts"`
}

// parseFactTruth decodes a fact-truth answer, refusing a verdict the seat does not give.
func parseFactTruth(text string) (factTruthAnswer, error) {
	var a factTruthAnswer
	if err := decodeAnswer(text, &a); err != nil {
		return a, err
	}
	for _, f := range a.Facts {
		if !slices.Contains(assemble.FactVerdicts, f.Verdict) {
			return a, fmt.Errorf("verdict %q for %s is not one of %s", f.Verdict, f.ID, strings.Join(assemble.FactVerdicts, ", "))
		}
	}
	return a, nil
}

// runsFactTruth reports whether the page's read judges facts: a read, never a draft, in a project
// with a facts store.
func (p *page) runsFactTruth() bool {
	return !p.w.Draft && p.r.facts != nil && p.r.cfg.Facts.Dir != ""
}

// askFactTruth reports whether the page's fact-truth request is sent.
func (p *page) askFactTruth() bool {
	ft := p.w.Rec.FactTruth
	return ft != nil && len(ft.Asked) > 0
}

// factTruthSet is the facts the page's read judges: every fact the blueprint places, cites, or
// disposes for a whole read, and for a change read the facts the change touched or joined, plus,
// for a bar read, the facts whose whole bullet changed since the bar and those the bar's own
// fact-truth findings named. A fact the blueprint no longer cites is not judged, since nothing
// the blueprint says rests on it.
func (p *page) factTruthSet(in *bpInputs) []string {
	cited := blueprint.CitedFacts(in.bp, p.r.facts)
	sc := p.w.readScope()
	if sc == nil || sc.Kind != blueprint.ScopeScoped || sc.Facts == nil {
		return cited
	}
	touched := slices.Concat(sc.Facts, sc.Joined)
	if p.w.BarFrom != "" {
		touched = slices.Concat(touched, sc.RawChanged)
		if ft := p.w.Rec.FactTruth; ft != nil {
			touched = slices.Concat(touched, ft.BarFacts)
		}
	}
	return slices.DeleteFunc(cited, func(id string) bool { return !slices.Contains(touched, id) })
}

// planFactTruth decides, once, which facts the request carries. A fact with no source pointer, or
// none that yields an excerpt, gets its no-excerpt verdict from Go and is left out of the request,
// so the seat sees only facts with something to read.
func (p *page) planFactTruth(ctx context.Context, in *bpInputs) error {
	ft := p.w.Rec.FactTruth
	if ft != nil && ft.Planned || !p.runsFactTruth() {
		return nil
	}
	if ft == nil {
		ft = &factTruthRecord{}
		p.w.Rec.FactTruth = ft
	}
	ft.Planned = true
	ids := p.factTruthSet(in)
	if len(ids) == 0 {
		return nil
	}
	ex, err := assemble.NewExcerpter(ctx, p.r.cfg)
	if err != nil {
		return err
	}
	for _, id := range ids {
		f, ok := p.r.facts.Get(id)
		if !ok {
			continue
		}
		real := false
		if len(f.Sources) > 0 {
			if _, real, err = ex.Fact(p.r.facts, id); err != nil {
				return err
			}
		}
		if real {
			ft.Asked = append(ft.Asked, id)
		} else {
			ft.NoExcerpt = append(ft.NoExcerpt, id)
		}
	}
	return nil
}

// factTruthRequest builds the one request that carries every fact the read asks about.
func (p *page) factTruthRequest(ctx context.Context, seat string) (batch.Request, error) {
	ids := p.w.Rec.FactTruth.Asked
	facts := make([]plan.Fact, 0, len(ids))
	for _, id := range ids {
		f, _ := p.r.facts.Get(id)
		facts = append(facts, f)
	}
	return p.buildInput(ctx, s1, seat, assemble.Input{
		Answer: assemble.AnswerFactTruth, Prompt: assemble.FactTruthPrompt(p.h.Page.Path, facts),
		Facts: p.r.facts, FactIDs: ids,
	})
}

// factRank orders the verdicts by how bad they are, for an id answered twice.
func factRank(verdict string) int {
	return slices.Index([]string{assemble.VerdictHolds, assemble.VerdictNoExcerpt, assemble.VerdictWider, assemble.VerdictContradicts}, verdict)
}

// factTruthFindings turns the read's verdicts into findings numbered from first+1, records every
// verdict, and reports whether any finding blocks. A wider or contradicts verdict blocks; a
// no-excerpt verdict blocks a verified fact and passes an external or vendor one. A fact the
// answer leaves out blocks, and an id the request did not carry is dropped with a warning.
func (p *page) factTruthFindings(first int) ([]edits.Finding, bool, error) {
	ft := p.w.Rec.FactTruth
	if ft == nil || len(ft.Asked)+len(ft.NoExcerpt) == 0 {
		return nil, false, nil
	}
	ft.Verdicts = nil
	got := map[string]factVerdictRecord{}
	for _, id := range ft.NoExcerpt {
		got[id] = factVerdictRecord{ID: id, Verdict: assemble.VerdictNoExcerpt, Excerpt: "none",
			Fix: "no pointer of the fact yields an excerpt; point it at the code that shows the claim"}
	}
	if raw, ok := p.w.Got["facttruth"]; ok {
		a, err := parseFactTruth(raw)
		if err != nil {
			return nil, false, err
		}
		for _, f := range a.Facts {
			if !slices.Contains(ft.Asked, f.ID) {
				p.r.warnf("%s: the fact-truth read answered for %s, which the request did not carry; dropped", p.h.Page.Path, f.ID)
				continue
			}
			if old, seen := got[f.ID]; seen && factRank(old.Verdict) >= factRank(f.Verdict) {
				continue
			}
			got[f.ID] = factVerdictRecord{ID: f.ID, Verdict: f.Verdict, Excerpt: f.Excerpt, Fix: f.Fix}
		}
	}
	var fs []edits.Finding
	blocks := false
	for _, id := range slices.Sorted(slices.Values(slices.Concat(ft.Asked, ft.NoExcerpt))) {
		v, answered := got[id]
		if !answered {
			fs = append(fs, edits.Finding{N: first + len(fs) + 1, Seat: "facttruth", Kind: "fact", Location: "fact " + id,
				Text: "the fact-truth read returned no verdict for " + id, Blocking: true})
			blocks = true
			continue
		}
		ft.Verdicts = append(ft.Verdicts, v)
		if v.Verdict == assemble.VerdictHolds {
			continue
		}
		blocking := v.Verdict != assemble.VerdictNoExcerpt
		if !blocking {
			if f, ok := p.r.facts.Get(id); ok {
				blocking = f.Tag == "verified"
			}
		}
		blocks = blocks || blocking
		fs = append(fs, edits.Finding{N: first + len(fs) + 1, Seat: "facttruth", Kind: "fact", Location: "fact " + id,
			Text: fmt.Sprintf("%s: %s (excerpt: %s)", v.Verdict, v.Fix, v.Excerpt), Blocking: blocking})
	}
	return fs, blocks, nil
}

// factTruthDetail names the facts a blocked fact-truth read stopped on, with their verdicts.
func factTruthDetail(fs []edits.Finding) string {
	var parts []string
	for _, f := range fs {
		if f.Blocking {
			verdict, _, _ := strings.Cut(f.Text, ":")
			parts = append(parts, strings.TrimPrefix(f.Location, "fact ")+" ("+verdict+")")
		}
	}
	return "the fact-truth read found claims the source does not bear out: " + strings.Join(parts, ", ")
}
