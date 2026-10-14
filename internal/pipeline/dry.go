package pipeline

import (
	"context"
	"fmt"
	"time"

	"github.com/glw907/drafter/internal/ledger"
)

const defaultPoll = 30 * time.Second

func newTimer(d time.Duration) *time.Timer { return time.NewTimer(d) }

// dryRun assembles each page's first stage, counts the tokens of its requests, and prices them at
// their worst case. It submits nothing and writes no ledger line, run directory, or state.
func (r *runner) dryRun(ctx context.Context) (Summary, error) {
	var s Summary
	for _, p := range r.pages {
		p.ps.Status = StatusRunning
	}
	for _, p := range r.pages {
		stg, err := p.advance(ctx)
		if err != nil {
			return s, err
		}
		if stg == nil {
			continue
		}
		if s.EstimateStage == "" {
			s.EstimateStage = stg.name
		}
		for _, q := range stg.reqs {
			n, err := r.api.CountTokens(ctx, q)
			if err != nil {
				return s, fmt.Errorf("count tokens for %s: %w", q.CustomID, err)
			}
			w, err := ledger.Worst(r.cfg.Prices, q.Model, n, q.MaxTokens, r.batchMode())
			if err != nil {
				return s, fmt.Errorf("worst case of %s: %w", q.CustomID, err)
			}
			s.Estimates = append(s.Estimates, Estimate{Seat: seatOf(q.CustomID), CustomID: q.CustomID, Tokens: n, MaxTokens: q.MaxTokens, Worst: w})
			s.EstimateTotal += w
		}
	}
	for _, p := range r.pages {
		res := PageResult{Path: p.h.Page.Path, Status: p.ps.Status}
		if p.w.Rec.Escalation != nil {
			res.Rule = p.w.Rec.Escalation.Rule
		}
		s.Pages = append(s.Pages, res)
	}
	return s, nil
}
