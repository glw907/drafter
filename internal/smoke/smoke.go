// Package smoke is the body of the live smoke: one batch holding one request per seat shape and
// one sync request, settled through the real ledger. The body is untagged so its assertions run
// offline against a fake API; the tagged live test calls it on the real one.
package smoke

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/glw907/drafter/internal/atomicfile"
	"github.com/glw907/drafter/internal/batch"
	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/ledger"
	"github.com/glw907/drafter/internal/run"
)

const (
	repoName      = "drafter-smoke"
	pageName      = "smoke"
	syncID        = "sync"
	defaultPoll   = 30 * time.Second
	recordDirName = "smoke"
)

// Params is what Run needs besides the shapes. A zero PollInterval is thirty seconds, a nil
// Sleep waits on a timer, and a nil Now is time.Now.
type Params struct {
	Options      batch.Options
	StateDir     string
	Model        string
	Prices       config.Prices
	Caps         ledger.Caps
	PollInterval time.Duration
	Sleep        func(ctx context.Context, d time.Duration) error
	Now          func() time.Time
}

// Report is what a smoke run saw. Problems holds every check that failed; an empty Problems
// with a nil error is a pass.
type Report struct {
	RunID   string
	BatchID string
	Results []batch.Result // the batch's results, in the order the API served them
	Sync    batch.Result
	Counted map[string]int64 // custom_id to the input tokens count_tokens reported
	// Resumed lists the batches of earlier runs this run settled. Voided lists the intents of
	// earlier runs that never reached the API. Estimated lists earlier sync intents whose
	// outcome is unknown, settled at their worst case.
	Resumed   []string
	Voided    []string
	Estimated []string
	Problems  []string
}

// intentRecord is one submission unit as the smoke file keeps it.
type intentRecord struct {
	Intent    string   `json:"intent"`
	BatchID   string   `json:"batch_id,omitempty"`
	Worst     float64  `json:"worst"`
	Requests  int      `json:"requests"`
	CustomIDs []string `json:"custom_ids"`
	Settled   bool     `json:"settled"`
}

// record is <state_dir>/smoke/<run-id>.json. A unit whose Intent is empty was never started.
type record struct {
	RunID string       `json:"run_id"`
	Model string       `json:"model"`
	Batch intentRecord `json:"batch"`
	Sync  intentRecord `json:"sync"`
}

type runner struct {
	p      Params
	c      *batch.Client
	led    *ledger.Ledger
	sleep  func(ctx context.Context, d time.Duration) error
	poll   time.Duration
	report Report
}

// Run checks the workspace, settles what earlier smoke runs left behind, then reserves and
// submits one batch holding one request per shape plus one sync request, polls the batch to its
// end, and settles every result under repo drafter-smoke. A failure after a request may have
// reached the API returns with its reservation open; the next run settles it.
func Run(ctx context.Context, p Params, shapes []Shape) (Report, error) {
	r := &runner{p: p, c: batch.New(p.Options), led: ledger.Open(p.StateDir), sleep: p.Sleep, poll: p.PollInterval}
	if r.sleep == nil {
		r.sleep = timerSleep
	}
	if r.poll == 0 {
		r.poll = defaultPoll
	}
	now := time.Now
	if p.Now != nil {
		now = p.Now
	}
	r.report = Report{RunID: run.NewID(now()), Counted: map[string]int64{}}
	if err := r.c.CheckWorkspace(ctx); err != nil {
		return r.report, fmt.Errorf("check workspace: %w", err)
	}
	if err := r.resume(ctx); err != nil {
		return r.report, err
	}
	if err := r.execute(ctx, shapes); err != nil {
		return r.report, err
	}
	r.verify(shapes)
	return r.report, nil
}

func timerSleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (r *runner) entry(runID, seat, mode string) ledger.Entry {
	return ledger.Entry{RunID: runID, Repo: repoName, Page: pageName, Seat: seat, Model: r.p.Model, Mode: mode}
}

func (r *runner) recordPath(runID string) string {
	return filepath.Join(r.p.StateDir, recordDirName, runID+".json")
}

func (r *runner) save(rec record) error {
	if err := os.MkdirAll(filepath.Join(r.p.StateDir, recordDirName), 0o700); err != nil {
		return fmt.Errorf("create smoke directory: %w", err)
	}
	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	if err := atomicfile.WriteFile(r.recordPath(rec.RunID), append(b, '\n'), 0o600); err != nil {
		return fmt.Errorf("write smoke record: %w", err)
	}
	return nil
}

func (r *runner) execute(ctx context.Context, shapes []Shape) error {
	rec := record{RunID: r.report.RunID, Model: r.p.Model}
	reqs := make([]batch.Request, 0, len(shapes))
	for _, s := range shapes {
		reqs = append(reqs, r.request(s.Seat, s))
	}
	if len(shapes) == 0 {
		return errors.New("no shapes to run")
	}
	syncReq := r.request(syncID, shapes[0])

	var batchWorst float64
	var syncIntent intentRecord
	for _, q := range append(slices.Clip(reqs), syncReq) {
		n, err := r.c.CountTokens(ctx, q)
		if err != nil {
			return fmt.Errorf("count tokens for %s: %w", q.CustomID, err)
		}
		r.report.Counted[q.CustomID] = n
		w, err := ledger.Worst(r.p.Prices, r.p.Model, n, q.MaxTokens, q.CustomID != syncID)
		if err != nil {
			return fmt.Errorf("worst case of %s: %w", q.CustomID, err)
		}
		if q.CustomID == syncID {
			syncIntent = intentRecord{Intent: rec.RunID + "-sync", Worst: w, Requests: 1, CustomIDs: []string{syncID}}
			continue
		}
		batchWorst += w
		rec.Batch.CustomIDs = append(rec.Batch.CustomIDs, q.CustomID)
	}
	rec.Batch.Intent, rec.Batch.Worst, rec.Batch.Requests = rec.RunID+"-batch", batchWorst, len(reqs)

	if err := r.reserve(rec.RunID, rec.Batch, "batch"); err != nil {
		return err
	}
	if err := r.save(rec); err != nil {
		return err
	}
	b, err := r.c.Create(ctx, reqs)
	if err != nil {
		if !errors.Is(err, batch.ErrMaybeSent) {
			// The API never accepted the batch, so nothing was billed.
			if verr := r.void(rec.RunID, rec.Batch, "batch"); verr != nil {
				return errors.Join(err, verr)
			}
		}
		return fmt.Errorf("create smoke batch: %w", err)
	}
	rec.Batch.BatchID = b.ID
	r.report.BatchID = b.ID
	if err := r.save(rec); err != nil {
		return err
	}
	results, err := r.await(ctx, b.ID)
	if err != nil {
		return err
	}
	if err := r.settleBatch(rec.RunID, rec.Batch, results); err != nil {
		return err
	}
	r.report.Results = results
	rec.Batch.Settled = true
	if err := r.save(rec); err != nil {
		return err
	}

	rec.Sync = syncIntent
	if err := r.reserve(rec.RunID, rec.Sync, "sync"); err != nil {
		return err
	}
	if err := r.save(rec); err != nil {
		return err
	}
	res, err := r.c.Send(ctx, syncReq)
	if err != nil {
		return fmt.Errorf("send smoke sync request: %w", err)
	}
	if err := r.settleSync(rec.RunID, rec.Sync, res); err != nil {
		return err
	}
	r.report.Sync = res
	rec.Sync.Settled = true
	return r.save(rec)
}

func (r *runner) request(customID string, s Shape) batch.Request {
	return batch.Request{
		CustomID:  customID,
		Model:     r.p.Model,
		System:    s.System,
		Messages:  []batch.Message{{Role: "user", Text: s.Prompt}},
		MaxTokens: s.MaxTokens,
		Effort:    s.Effort,
		Schema:    s.Schema,
	}
}

func (r *runner) reserve(runID string, it intentRecord, mode string) error {
	seat := pageName
	if mode == "sync" {
		seat = syncID
	}
	err := r.led.Reserve(ledger.Reservation{
		Entry:    r.entry(runID, seat, mode),
		Intent:   it.Intent,
		Requests: it.Requests,
		Worst:    it.Worst,
	}, r.p.Caps)
	if err != nil {
		return fmt.Errorf("reserve %s: %w", it.Intent, err)
	}
	return nil
}

func (r *runner) void(runID string, it intentRecord, mode string) error {
	return r.led.Settle(ledger.Settlement{
		Entry:  r.entry(runID, pageName, mode),
		Intent: it.Intent,
		Key:    it.Intent + "/void",
		Kind:   "void",
	})
}

// await polls batch id until it ends, then downloads its results.
func (r *runner) await(ctx context.Context, id string) ([]batch.Result, error) {
	for {
		b, err := r.c.Get(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("poll batch %s: %w", id, err)
		}
		if b.Ended {
			break
		}
		if err := r.sleep(ctx, r.poll); err != nil {
			return nil, fmt.Errorf("wait for batch %s: %w", id, err)
		}
	}
	res, err := r.c.Results(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("download results of batch %s: %w", id, err)
	}
	return res, nil
}

// settleBatch settles each result under <batch-id>/<custom_id>. An unbilled result settles at
// zero so its intent closes.
func (r *runner) settleBatch(runID string, it intentRecord, results []batch.Result) error {
	for _, res := range results {
		s := ledger.Settlement{
			Entry:  r.entry(runID, res.CustomID, "batch"),
			Intent: it.Intent,
			Key:    it.BatchID + "/" + res.CustomID,
			Kind:   "actual",
		}
		if res.Kind == batch.KindSucceeded {
			cost, err := ledger.Cost(r.p.Prices, r.p.Model, res.Usage, true)
			if err != nil {
				return fmt.Errorf("price %s: %w", res.CustomID, err)
			}
			s.Usage, s.Cost = res.Usage, cost
		}
		if err := r.led.Settle(s); err != nil {
			return fmt.Errorf("settle %s: %w", s.Key, err)
		}
	}
	return nil
}

func (r *runner) settleSync(runID string, it intentRecord, res batch.Result) error {
	s := ledger.Settlement{
		Entry:  r.entry(runID, syncID, "sync"),
		Intent: it.Intent,
		Key:    res.MessageID,
		Kind:   "actual",
	}
	if res.Kind == batch.KindSucceeded {
		cost, err := ledger.Cost(r.p.Prices, r.p.Model, res.Usage, false)
		if err != nil {
			return fmt.Errorf("price sync result: %w", err)
		}
		s.Usage, s.Cost = res.Usage, cost
	}
	if s.Key == "" {
		s.Key = it.Intent + "/" + syncID
	}
	if err := r.led.Settle(s); err != nil {
		return fmt.Errorf("settle %s: %w", s.Key, err)
	}
	return nil
}

// resume settles what earlier runs left in the smoke directory. A batch with an ID is settled
// through Results; an intent that never got a batch ID never reached the API and is voided; a
// sync intent that was reserved but not settled may have been billed, so it settles estimated at
// its worst case.
func (r *runner) resume(ctx context.Context) error {
	files, err := filepath.Glob(filepath.Join(r.p.StateDir, recordDirName, "*.json"))
	if err != nil {
		return err
	}
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read smoke record: %w", err)
		}
		var rec record
		if err := json.Unmarshal(b, &rec); err != nil {
			return fmt.Errorf("parse smoke record %s: %w", path, err)
		}
		if err := r.resumeRecord(ctx, rec); err != nil {
			return fmt.Errorf("resume run %s: %w", rec.RunID, err)
		}
	}
	return nil
}

func (r *runner) resumeRecord(ctx context.Context, rec record) error {
	model := r.p.Model
	r.p.Model = rec.Model
	defer func() { r.p.Model = model }()

	if it := rec.Batch; it.Intent != "" && !it.Settled {
		if it.BatchID == "" {
			if err := r.void(rec.RunID, it, "batch"); err != nil {
				return err
			}
			r.report.Voided = append(r.report.Voided, it.Intent)
		} else {
			results, err := r.await(ctx, it.BatchID)
			if err != nil {
				return err
			}
			if err := r.settleBatch(rec.RunID, it, results); err != nil {
				return err
			}
			r.report.Resumed = append(r.report.Resumed, it.BatchID)
		}
		rec.Batch.Settled = true
		if err := r.save(rec); err != nil {
			return err
		}
	}
	if it := rec.Sync; it.Intent != "" && !it.Settled {
		err := r.led.Settle(ledger.Settlement{
			Entry:  r.entry(rec.RunID, syncID, "sync"),
			Intent: it.Intent,
			Key:    it.Intent + "/" + syncID,
			Cost:   it.Worst,
			Kind:   "estimated",
		})
		if err != nil {
			return fmt.Errorf("settle %s: %w", it.Intent, err)
		}
		r.report.Estimated = append(r.report.Estimated, it.Intent)
		rec.Sync.Settled = true
		return r.save(rec)
	}
	return nil
}

// verify records every way the run failed to match the shapes and the counted figures.
func (r *runner) verify(shapes []Shape) {
	bySeat := map[string]Shape{syncID: shapes[0]}
	for _, s := range shapes {
		bySeat[s.Seat] = s
	}
	if len(r.report.Results) != len(shapes) {
		r.problem("batch returned %d results, want %d", len(r.report.Results), len(shapes))
	}
	for _, res := range append(slices.Clip(r.report.Results), r.report.Sync) {
		id := res.CustomID
		s, ok := bySeat[id]
		if !ok {
			r.problem("result for unknown custom_id %q", id)
			continue
		}
		if res.Kind != batch.KindSucceeded || res.StopReason != "end_turn" {
			r.problem("%s: kind %q, stop reason %q", id, res.Kind, res.StopReason)
			continue
		}
		if err := Check(s, res.Text); err != nil {
			r.problem("%v", err)
		}
		if n := r.report.Counted[id]; res.Usage.InputTokens > n {
			r.problem("%s: input_tokens %d exceeds counted %d", id, res.Usage.InputTokens, n)
		}
	}
}

func (r *runner) problem(format string, a ...any) {
	r.report.Problems = append(r.report.Problems, fmt.Sprintf(format, a...))
}
