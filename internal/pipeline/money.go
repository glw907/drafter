package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/glw907/drafter/internal/atomicfile"
	"github.com/glw907/drafter/internal/batch"
	"github.com/glw907/drafter/internal/ledger"
	"github.com/glw907/drafter/internal/run"
)

// flight is a batch the run has submitted and not yet read.
type flight struct {
	page *page
	idx  int // the intent's index in the page's state
}

func (f *flight) intent() *run.Intent { return &f.page.ps.Intents[f.idx] }

// seatOf reads the seat out of a custom_id.
func seatOf(customID string) string {
	parts := strings.Split(customID, "_")
	if len(parts) < 5 {
		return ""
	}
	return parts[3]
}

func attemptOf(customID string) int {
	parts := strings.Split(customID, "_")
	n, _ := strconv.Atoi(parts[len(parts)-1])
	return n
}

func (r *runner) caps() ledger.Caps {
	return ledger.Caps{
		Page: r.cfg.PageCap, Cycle: r.cfg.CycleCeiling,
		CycleStart: ledger.CycleStart(r.now(), r.cfg.BillingCycleDay),
	}
}

func (r *runner) entry(p *page, seat, model string) ledger.Entry {
	return ledger.Entry{
		RunID: r.state.RunID, Repo: r.state.Repo, Page: p.h.Page.Path, Seat: seat, Model: model, Mode: r.cfg.Mode,
	}
}

func (r *runner) batchMode() bool { return r.cfg.Mode != "sync" }

// submit sends a stage: one batch in batch mode, one request at a time in sync mode.
func (r *runner) submit(ctx context.Context, stg *stage) error {
	p := stg.page
	if r.batchMode() {
		return r.submitUnit(ctx, p, stg.name, stg.reqs)
	}
	for _, req := range stg.reqs {
		if p.ps.Status != StatusRunning || r.stop != "" {
			return nil
		}
		if err := r.submitUnit(ctx, p, stg.name, []batch.Request{req}); err != nil {
			return err
		}
	}
	return nil
}

// reissue gives each request a new attempt number, so a resubmission never reuses a custom_id.
func (p *page) reissue(stage string, reqs []batch.Request) []batch.Request {
	out := make([]batch.Request, len(reqs))
	for i, q := range reqs {
		seat := seatOf(q.CustomID)
		q.CustomID = p.customID(stage, seat, p.nextAttempt(stage, seat))
		out[i] = q
	}
	return out
}

// submitUnit counts, reserves, records, and sends one submission unit. The order is the money
// path's contract: a request is never sent before its reservation is in the ledger and its
// intent is in the state.
func (r *runner) submitUnit(ctx context.Context, p *page, name string, reqs []batch.Request) error {
	var worst float64
	var seats []string
	for _, q := range reqs {
		n, err := r.api.CountTokens(ctx, q)
		if err != nil {
			return r.countFailed(p, err)
		}
		w, err := ledger.Worst(r.cfg.Prices, q.Model, n, q.MaxTokens, r.batchMode())
		if err != nil {
			return fmt.Errorf("worst case of %s: %w", q.CustomID, err)
		}
		worst += w
		seats = append(seats, seatOf(q.CustomID))
	}
	intentID := fmt.Sprintf("%s-%s-%s-%d", r.state.RunID, p.hash12(), name, len(p.ps.Intents)+1)
	res := ledger.Reservation{
		Entry:    r.entry(p, strings.Join(seats, "+"), reqs[0].Model),
		Intent:   intentID,
		Requests: len(reqs),
		Worst:    worst,
	}
	if err := r.led.Reserve(res, r.caps()); err != nil {
		switch {
		case errors.Is(err, ledger.ErrPageCap):
			return p.escalate("cap", "", nil, err.Error())
		case errors.Is(err, ledger.ErrCycleCap):
			r.warnf("%v", err)
			r.halt(StopCycleCap)
			return nil
		}
		return fmt.Errorf("reserve %s: %w", intentID, err)
	}
	in := run.Intent{ID: intentID, Stage: name, Time: r.now(), Mode: r.cfg.Mode, Worst: worst}
	for _, q := range reqs {
		in.Requests = append(in.Requests, run.IntentRequest{CustomID: q.CustomID, Seat: seatOf(q.CustomID)})
		in.Attempt = max(in.Attempt, attemptOf(q.CustomID))
		if err := p.saveRequest(q); err != nil {
			return err
		}
	}
	p.ps.Intents = append(p.ps.Intents, in)
	idx := len(p.ps.Intents) - 1
	if err := r.save(); err != nil {
		return err
	}
	if r.batchMode() {
		return r.create(ctx, p, idx, name, reqs)
	}
	return r.send(ctx, p, idx, name, reqs[0])
}

// countFailed handles a failed CountTokens, which bills nothing and holds no reservation.
func (r *runner) countFailed(p *page, err error) error {
	switch {
	case errors.Is(err, batch.ErrSpendLimit):
		r.warnf("%v", err)
		r.halt(StopSpendLimit)
	case errors.Is(err, batch.ErrTransport):
		r.halt(StopTransport)
	case errors.Is(err, batch.ErrNoWorkspaceCheck):
		return err
	default:
		if ce, ok := errors.AsType[*batch.CallError](err); ok && ce.Status >= 400 && ce.Status < 500 {
			return p.escalate("api error", "", nil, ce.Error())
		}
		return fmt.Errorf("count tokens: %w", err)
	}
	return nil
}

func (p *page) saveRequest(q batch.Request) error {
	if p.dir == "" {
		return nil
	}
	dir := filepath.Join(p.dir, "requests")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create request directory: %w", err)
	}
	b, err := json.MarshalIndent(q, "", "  ")
	if err != nil {
		return fmt.Errorf("encode request %s: %w", q.CustomID, err)
	}
	return atomicfile.WriteFile(filepath.Join(dir, q.CustomID+".json"), b, 0o644)
}

func (p *page) saveResponse(res batch.Result) error {
	if p.dir == "" {
		return nil
	}
	dir := filepath.Join(p.dir, "responses")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create response directory: %w", err)
	}
	b, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return fmt.Errorf("encode response %s: %w", res.CustomID, err)
	}
	return atomicfile.WriteFile(filepath.Join(dir, res.CustomID+".json"), b, 0o644)
}

// create sends the batch of intent idx.
func (r *runner) create(ctx context.Context, p *page, idx int, name string, reqs []batch.Request) error {
	b, err := r.api.Create(ctx, reqs)
	switch {
	case err == nil:
		in := &p.ps.Intents[idx]
		in.BatchID, in.Created = b.ID, b.CreatedAt
		if err := r.save(); err != nil {
			return err
		}
		r.inflight = append(r.inflight, &flight{page: p, idx: idx})
		return nil
	case errors.Is(err, batch.ErrMaybeSent):
		return r.recover(ctx, p, idx, name, reqs)
	}
	return r.failedSubmit(p, idx, err)
}

// recover finds out whether a Create whose outcome is unknown made a batch. A batch that carries
// the intent's custom_ids is adopted; with none, the intent is voided and the unit is sent again
// as a new intent.
func (r *runner) recover(ctx context.Context, p *page, idx int, name string, reqs []batch.Request) error {
	in := &p.ps.Intents[idx]
	id, err := r.findBatch(ctx, in)
	if err != nil {
		return r.failedSubmit(p, idx, err)
	}
	if id != "" {
		in.BatchID = id
		if err := r.save(); err != nil {
			return err
		}
		r.inflight = append(r.inflight, &flight{page: p, idx: idx})
		return nil
	}
	if err := r.voidIntent(p, idx); err != nil {
		return err
	}
	if p.w.spend(name+".maybesent") > 1 {
		r.halt(StopTransport)
		return nil
	}
	return r.submitUnit(ctx, p, name, p.reissue(name, reqs))
}

// findBatch lists the workspace's batches created since the intent, waits for each to end, and
// returns the ID of the one whose results carry the intent's custom_ids, or empty.
func (r *runner) findBatch(ctx context.Context, in *run.Intent) (string, error) {
	cands, err := r.api.List(ctx, in.Time)
	if err != nil {
		return "", err
	}
	want := map[string]bool{}
	for _, rq := range in.Requests {
		want[rq.CustomID] = true
	}
	for _, c := range cands {
		if err := r.waitEnded(ctx, c.ID); err != nil {
			return "", err
		}
		results, err := r.api.Results(ctx, c.ID)
		if err != nil {
			return "", err
		}
		for _, res := range results {
			if want[res.CustomID] {
				return c.ID, nil
			}
		}
	}
	return "", nil
}

func (r *runner) waitEnded(ctx context.Context, id string) error {
	for {
		b, err := r.api.Get(ctx, id)
		if err != nil {
			return err
		}
		if b.Ended {
			return nil
		}
		if err := r.sleep(ctx); err != nil {
			return err
		}
	}
}

func (r *runner) sleep(ctx context.Context) error {
	d := r.o.PollInterval
	if d == 0 {
		d = defaultPoll
	}
	if r.o.Sleep != nil {
		return r.o.Sleep(ctx, d)
	}
	t := newTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// send sends the single request of intent idx in sync mode and settles its result.
func (r *runner) send(ctx context.Context, p *page, idx int, name string, req batch.Request) error {
	res, err := r.api.Send(ctx, req)
	switch {
	case err == nil:
		in := &p.ps.Intents[idx]
		if err := p.saveResponse(res); err != nil {
			return err
		}
		in.Saved = true
		if err := r.settleResults(p, idx, []batch.Result{res}); err != nil {
			return err
		}
		return p.handle(ctx, &p.ps.Intents[idx], []batch.Result{res})
	case errors.Is(err, batch.ErrMaybeSent):
		// The response is lost and cannot be recovered, so the attempt settles at its worst
		// case, and the request goes out again as a new attempt.
		if err := r.settleEstimated(p, idx); err != nil {
			return err
		}
		if p.w.spend(name+".maybesent") > 1 {
			r.halt(StopTransport)
			return nil
		}
		return r.submitUnit(ctx, p, name, p.reissue(name, []batch.Request{req}))
	}
	return r.failedSubmit(p, idx, err)
}

// voidIntent closes an intent the API never billed, at zero.
func (r *runner) voidIntent(p *page, idx int) error {
	in := &p.ps.Intents[idx]
	err := r.led.Settle(ledger.Settlement{
		Entry:  r.entry(p, strings.Join(seatsOf(in), "+"), r.modelOf(in.Requests[0].Seat)),
		Intent: in.ID, Key: in.ID + "/void", Kind: "void",
	})
	if err != nil {
		return fmt.Errorf("void %s: %w", in.ID, err)
	}
	in.Settlement = "void"
	return r.save()
}

func seatsOf(in *run.Intent) []string {
	var out []string
	for _, rq := range in.Requests {
		out = append(out, rq.Seat)
	}
	return out
}

func (r *runner) modelOf(seat string) string { return r.cfg.Seat[seat].Model }

// settleEstimated closes an intent whose outcome is unknown at its reservation, one line per
// request.
func (r *runner) settleEstimated(p *page, idx int) error {
	in := &p.ps.Intents[idx]
	for _, rq := range in.Requests {
		err := r.led.Settle(ledger.Settlement{
			Entry:  r.entry(p, rq.Seat, r.modelOf(rq.Seat)),
			Intent: in.ID, Key: in.ID + "/" + rq.CustomID,
			Cost: in.Worst / float64(len(in.Requests)), Kind: "estimated",
		})
		if err != nil {
			return fmt.Errorf("settle %s: %w", in.ID, err)
		}
	}
	in.Settlement = "estimated"
	return r.save()
}

// failedSubmit handles a Create or Send that failed with proof the request was not billed: the
// intent is voided at once, then the error takes its action.
func (r *runner) failedSubmit(p *page, idx int, err error) error {
	if verr := r.voidIntent(p, idx); verr != nil {
		return errors.Join(err, verr)
	}
	switch {
	case errors.Is(err, batch.ErrSpendLimit):
		r.warnf("%v", err)
		r.halt(StopSpendLimit)
		return nil
	case errors.Is(err, batch.ErrTransport):
		r.halt(StopTransport)
		return nil
	case errors.Is(err, batch.ErrNoWorkspaceCheck):
		return err
	}
	if ce, ok := errors.AsType[*batch.CallError](err); ok && ce.Status >= 400 && ce.Status < 500 {
		return p.escalate("api error", "", nil, ce.Error())
	}
	return fmt.Errorf("submit for %s: %w", p.h.Page.Path, err)
}

// settleResults writes one settle line per result of the intent, an unbilled result at zero, and
// settles any request the API returned no result for at its share of the reservation.
func (r *runner) settleResults(p *page, idx int, results []batch.Result) error {
	in := &p.ps.Intents[idx]
	got := map[string]bool{}
	kind := "actual"
	for _, res := range results {
		seat := seatOf(res.CustomID)
		key := res.MessageID
		if r.batchMode() {
			key = in.BatchID + "/" + res.CustomID
		} else if key == "" {
			key = in.ID + "/" + res.CustomID
		}
		s := ledger.Settlement{
			Entry: r.entry(p, seat, r.modelOf(seat)), Intent: in.ID, Key: key, Kind: "actual",
		}
		if res.Kind == batch.KindSucceeded {
			cost, err := ledger.Cost(r.cfg.Prices, r.modelOf(seat), res.Usage, r.batchMode())
			if err != nil {
				return fmt.Errorf("price %s: %w", res.CustomID, err)
			}
			s.Usage, s.Cost = res.Usage, cost
		}
		if err := r.led.Settle(s); err != nil {
			return fmt.Errorf("settle %s: %w", key, err)
		}
		got[res.CustomID] = true
	}
	for _, rq := range in.Requests {
		if got[rq.CustomID] {
			continue
		}
		kind = "estimated"
		err := r.led.Settle(ledger.Settlement{
			Entry:  r.entry(p, rq.Seat, r.modelOf(rq.Seat)),
			Intent: in.ID, Key: in.ID + "/" + rq.CustomID,
			Cost: in.Worst / float64(len(in.Requests)), Kind: "estimated",
		})
		if err != nil {
			return fmt.Errorf("settle %s: %w", rq.CustomID, err)
		}
	}
	in.Settlement = kind
	return r.save()
}

// await polls every in-flight batch until each has ended and been read, or the run stops.
func (r *runner) await(ctx context.Context) error {
	for len(r.inflight) > 0 && r.stop == "" {
		ended := false
		for i := 0; i < len(r.inflight); i++ {
			fl := r.inflight[i]
			in := fl.intent()
			b, err := r.api.Get(ctx, in.BatchID)
			if err != nil {
				return r.pollFailed(err)
			}
			if !b.Ended {
				continue
			}
			ended = true
			results, err := r.api.Results(ctx, in.BatchID)
			if err != nil {
				return r.pollFailed(err)
			}
			r.inflight = append(r.inflight[:i], r.inflight[i+1:]...)
			i--
			for _, res := range results {
				if err := fl.page.saveResponse(res); err != nil {
					return err
				}
			}
			if err := r.settleResults(fl.page, fl.idx, results); err != nil {
				return err
			}
			if err := fl.page.handle(ctx, fl.intent(), results); err != nil {
				return err
			}
			if err := r.save(); err != nil {
				return err
			}
			if r.stop != "" {
				return nil
			}
		}
		if !ended && len(r.inflight) > 0 {
			if err := r.sleep(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

// pollFailed maps a failed poll or download to a stop; the batch keeps running on the server,
// so a resume reattaches to it.
func (r *runner) pollFailed(err error) error {
	switch {
	case errors.Is(err, batch.ErrTransport):
		r.halt(StopTransport)
		return nil
	case errors.Is(err, batch.ErrSpendLimit):
		r.halt(StopSpendLimit)
		return nil
	}
	return fmt.Errorf("poll batches: %w", err)
}
