package ledger

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glw907/drafter/internal/batch"
	"github.com/glw907/drafter/internal/config"
)

var t0 = time.Date(2027, 3, 10, 12, 0, 0, 0, time.UTC)

var wide = Caps{Page: 1000, Cycle: 1000}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

// newLedger returns a ledger over a state directory that does not exist yet.
func newLedger(t *testing.T) (*Ledger, *clock, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "state")
	ck := &clock{t: t0}
	l := Open(dir)
	l.Now = ck.now
	l.Warn = &bytes.Buffer{}
	return l, ck, dir
}

func res(run, page, intent string, requests int, worst float64) Reservation {
	return Reservation{
		RunID: run, Repo: "repo", Page: page, Seat: "draft", Model: "plain-s", Mode: "batch",
		Intent:   intent,
		Requests: requests,
		Worst:    worst,
	}
}

func settle(page, intent, key, kind string, cost float64) Settlement {
	return Settlement{
		RunID: "r1", Repo: "repo", Page: page, Seat: "draft", Model: "plain-s", Mode: "batch",
		Intent: intent,
		Key:    key,
		Cost:   cost,
		Kind:   kind,
	}
}

func mustReserve(t *testing.T, l *Ledger, r Reservation, c Caps) {
	t.Helper()
	if err := l.Reserve(r, c); err != nil {
		t.Fatalf("Reserve %s: %v", r.Intent, err)
	}
}

func mustSettle(t *testing.T, l *Ledger, s Settlement) {
	t.Helper()
	if err := l.Settle(s); err != nil {
		t.Fatalf("Settle %s: %v", s.Key, err)
	}
}

func pageSpend(t *testing.T, l *Ledger, run, page string) float64 {
	t.Helper()
	got, err := l.PageSpend(run, page)
	if err != nil {
		t.Fatalf("PageSpend: %v", err)
	}
	return got
}

func lines(t *testing.T, dir string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "ledger.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

func loadPrices(t *testing.T) config.Prices {
	t.Helper()
	data, err := os.ReadFile("testdata/prices.json")
	if err != nil {
		t.Fatal(err)
	}
	var p config.Prices
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-12 }

// The page cap is checked against input plus max_tokens at the output price. With plain-s
// (input 4, output 16 per million): 1,000,000 input tokens cost 4; 500,000 output tokens cost 8,
// so the worst case is 12; 250,000 output tokens cost 4, so the worst case is 8.
func TestWorstThroughReserve(t *testing.T) {
	prices := loadPrices(t)
	tests := []struct {
		name      string
		maxTokens int64
		wantWorst float64
		wantErr   error
	}{
		{"input fits, worst case does not", 500_000, 12, ErrPageCap},
		{"clean twin", 250_000, 8, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, _, dir := newLedger(t)
			worst, err := Worst(prices, "plain-s", 1_000_000, tt.maxTokens, false)
			if err != nil {
				t.Fatal(err)
			}
			if worst != tt.wantWorst {
				t.Fatalf("Worst = %v, want %v", worst, tt.wantWorst)
			}
			err = l.Reserve(res("r1", "p1", "i1", 1, worst), Caps{Page: 10, Cycle: 1000})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Reserve err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				if _, statErr := os.Stat(filepath.Join(dir, "ledger.jsonl")); statErr == nil {
					t.Error("a refused reservation appended a line")
				}
			}
		})
	}
}

func TestPageCapBoundary(t *testing.T) {
	tests := []struct {
		name    string
		second  float64
		wantErr error
	}{
		{"exactly at the cap", 4, nil},
		{"a quarter-dollar over", 4.25, ErrPageCap},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, _, _ := newLedger(t)
			caps := Caps{Page: 10, Cycle: 1000}
			mustReserve(t, l, res("r1", "p1", "i1", 1, 6), caps)
			err := l.Reserve(res("r1", "p1", "i2", 1, tt.second), caps)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			// Another page of the same run is unaffected by this page's total.
			mustReserve(t, l, res("r1", "p2", "i3", 1, 10), caps)
		})
	}
}

func TestCycleCap(t *testing.T) {
	tests := []struct {
		name    string
		third   float64
		wantErr error
	}{
		{"third page would cross", 4, ErrCycleCap},
		{"third page lands exactly on the ceiling", 2, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, _, _ := newLedger(t)
			caps := Caps{Page: 100, Cycle: 10, CycleStart: t0.Add(-time.Hour)}
			mustReserve(t, l, res("r1", "a", "ia", 1, 4), caps)
			mustReserve(t, l, res("r1", "b", "ib", 1, 4), caps)
			err := l.Reserve(res("r1", "c", "ic", 1, tt.third), caps)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestReserveChild(t *testing.T) {
	n := os.Getenv("DRAFTER_LEDGER_CHILD")
	if n == "" {
		t.Skip("runs only as a child of TestTwoProcessesOneLedger")
	}
	peer := map[string]string{"1": "2", "2": "1"}[n]
	ready := os.Getenv("DRAFTER_LEDGER_READY")
	l := Open(os.Getenv("DRAFTER_LEDGER_DIR"))
	l.hook = func() {
		if err := os.WriteFile(filepath.Join(ready, "ready-"+n), nil, 0o600); err != nil {
			t.Error(err)
			return
		}
		deadline := time.Now().Add(500 * time.Millisecond)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(filepath.Join(ready, "ready-"+peer)); err == nil {
				return
			}
			time.Sleep(2 * time.Millisecond)
		}
	}
	err := l.Reserve(res("r1", "p1", "i"+n, 1, 6), Caps{Page: 10, Cycle: 1000})
	switch {
	case err == nil:
		fmt.Println("RESULT ok")
	case errors.Is(err, ErrPageCap):
		fmt.Println("RESULT page-cap")
	default:
		fmt.Println("RESULT error", err)
	}
}

// Each child reserves 6 against a page cap of 10. Under the lock the second child blocks before
// its check, so exactly one passes; without it both pass the check inside the hook window.
func TestTwoProcessesOneLedger(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	ready := t.TempDir()
	cmds := make([]*exec.Cmd, 2)
	outs := make([]*bytes.Buffer, 2)
	for i := range cmds {
		n := string(rune('1' + i))
		cmd := exec.Command(os.Args[0], "-test.run=^TestReserveChild$")
		cmd.Env = append(os.Environ(), "DRAFTER_LEDGER_CHILD="+n, "DRAFTER_LEDGER_DIR="+dir, "DRAFTER_LEDGER_READY="+ready)
		outs[i] = &bytes.Buffer{}
		cmd.Stdout = outs[i]
		cmd.Stderr = outs[i]
		cmds[i] = cmd
	}
	for _, c := range cmds {
		if err := c.Start(); err != nil {
			t.Fatal(err)
		}
	}
	for i, c := range cmds {
		if err := c.Wait(); err != nil {
			t.Fatalf("child %d: %v\n%s", i+1, err, outs[i])
		}
	}
	var ok, refused int
	for _, o := range outs {
		switch {
		case strings.Contains(o.String(), "RESULT ok"):
			ok++
		case strings.Contains(o.String(), "RESULT page-cap"):
			refused++
		default:
			t.Fatalf("unexpected child output: %s", o)
		}
	}
	if ok != 1 || refused != 1 {
		t.Errorf("ok = %d, refused = %d, want 1 and 1", ok, refused)
	}
	if got := len(lines(t, dir)); got != 1 {
		t.Errorf("ledger has %d lines, want 1", got)
	}
}

func TestSettleTwiceAndEmptyKey(t *testing.T) {
	l, _, dir := newLedger(t)
	mustReserve(t, l, res("r1", "p1", "i1", 1, 5), wide)
	s := settle("p1", "i1", "msg_1", "actual", 3)
	mustSettle(t, l, s)
	mustSettle(t, l, s)
	if got := len(lines(t, dir)); got != 2 {
		t.Errorf("lines = %d, want 2 (one reserve, one settle)", got)
	}
	if got := pageSpend(t, l, "r1", "p1"); got != 3 {
		t.Errorf("PageSpend = %v, want 3", got)
	}
	s.Key = ""
	if err := l.Settle(s); err == nil {
		t.Error("Settle with an empty Key succeeded")
	}
}

func TestSettlementKinds(t *testing.T) {
	t.Run("batch requests errored at zero close the intent at the settled sum", func(t *testing.T) {
		l, _, dir := newLedger(t)
		mustReserve(t, l, res("r1", "p1", "i1", 2, 8), wide)
		if got := pageSpend(t, l, "r1", "p1"); got != 8 {
			t.Fatalf("open PageSpend = %v, want 8", got)
		}
		mustSettle(t, l, settle("p1", "i1", "batch_1/a", "actual", 0))
		if got := pageSpend(t, l, "r1", "p1"); got != 8 {
			t.Fatalf("after one of two PageSpend = %v, want 8", got)
		}
		mustSettle(t, l, settle("p1", "i1", "batch_1/b", "actual", 0))
		if got := pageSpend(t, l, "r1", "p1"); got != 0 {
			t.Errorf("closed PageSpend = %v, want 0", got)
		}
		if got := len(lines(t, dir)); got != 3 {
			t.Errorf("lines = %d, want 3", got)
		}
		open, err := l.OpenIntents("r1")
		if err != nil || len(open) != 0 {
			t.Errorf("OpenIntents = %v, %v, want none", open, err)
		}
	})
	t.Run("an intent counts at its worst case until the last request settles", func(t *testing.T) {
		l, _, _ := newLedger(t)
		mustReserve(t, l, res("r1", "p1", "i1", 3, 9), wide)
		mustSettle(t, l, settle("p1", "i1", "b/a", "actual", 1))
		mustSettle(t, l, settle("p1", "i1", "b/b", "actual", 1.5))
		if got := pageSpend(t, l, "r1", "p1"); got != 9 {
			t.Errorf("two of three settled: PageSpend = %v, want 9 (worst case)", got)
		}
		mustSettle(t, l, settle("p1", "i1", "b/c", "actual", 0.25))
		if got := pageSpend(t, l, "r1", "p1"); got != 2.75 {
			t.Errorf("all settled: PageSpend = %v, want 2.75 (settled sum)", got)
		}
	})
	t.Run("estimated sync settlement is idempotent on resume", func(t *testing.T) {
		l, _, dir := newLedger(t)
		mustReserve(t, l, res("r1", "p1", "s1", 1, 3), wide)
		est := settle("p1", "s1", "s1/c1", "estimated", 3)
		mustSettle(t, l, est)
		mustSettle(t, l, est)
		if got := len(lines(t, dir)); got != 2 {
			t.Errorf("lines = %d, want 2", got)
		}
		if got := pageSpend(t, l, "r1", "p1"); got != 3 {
			t.Errorf("PageSpend = %v, want 3, counted once as written", got)
		}
	})
	t.Run("void closes the whole intent at zero", func(t *testing.T) {
		l, _, _ := newLedger(t)
		mustReserve(t, l, res("r1", "p1", "v1", 4, 5), wide)
		mustSettle(t, l, settle("p1", "v1", "v1/void", "void", 0))
		if got := pageSpend(t, l, "r1", "p1"); got != 0 {
			t.Errorf("PageSpend = %v, want 0", got)
		}
		open, _ := l.OpenIntents("r1")
		if len(open) != 0 {
			t.Errorf("OpenIntents = %v, want none", open)
		}
	})
}

func TestCycleStartAndOpenIntents(t *testing.T) {
	cycleStart := t0.Add(-time.Hour)
	caps := Caps{Page: 100, Cycle: 10, CycleStart: cycleStart}
	t.Run("an open intent reserved before CycleStart counts", func(t *testing.T) {
		l, ck, _ := newLedger(t)
		ck.t = cycleStart.Add(-time.Hour)
		mustReserve(t, l, res("r1", "a", "ia", 1, 6), Caps{Page: 100, Cycle: 10, CycleStart: cycleStart.Add(-2 * time.Hour)})
		ck.t = t0
		if err := l.Reserve(res("r1", "b", "ib", 1, 5), caps); !errors.Is(err, ErrCycleCap) {
			t.Errorf("err = %v, want ErrCycleCap", err)
		}
		if err := l.Reserve(res("r1", "b", "ib", 1, 4), caps); err != nil {
			t.Errorf("clean twin: %v", err)
		}
	})
	t.Run("a settle line before CycleStart does not count", func(t *testing.T) {
		l, ck, _ := newLedger(t)
		ck.t = cycleStart.Add(-time.Hour)
		mustReserve(t, l, res("r1", "a", "ia", 1, 8), wide)
		mustSettle(t, l, settle("a", "ia", "m1", "actual", 8))
		ck.t = t0
		if err := l.Reserve(res("r1", "b", "ib", 1, 5), caps); err != nil {
			t.Errorf("old settled spend counted in the new cycle: %v", err)
		}
	})
	t.Run("a settle line at or after CycleStart counts", func(t *testing.T) {
		l, ck, _ := newLedger(t)
		ck.t = cycleStart
		mustReserve(t, l, res("r1", "a", "ia", 1, 8), wide)
		mustSettle(t, l, settle("a", "ia", "m1", "actual", 8))
		ck.t = t0
		if err := l.Reserve(res("r1", "b", "ib", 1, 5), caps); !errors.Is(err, ErrCycleCap) {
			t.Errorf("err = %v, want ErrCycleCap", err)
		}
	})
}

func TestCycleStartFunc(t *testing.T) {
	d := func(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 0, 0, 0, 0, time.UTC) }
	tests := []struct {
		name string
		now  time.Time
		day  int
		want time.Time
	}{
		{"leap February", time.Date(2028, 3, 10, 0, 0, 0, 0, time.UTC), 31, d(2028, 2, 29)},
		{"short February", time.Date(2027, 3, 10, 0, 0, 0, 0, time.UTC), 31, d(2027, 2, 28)},
		{"January's last day", time.Date(2027, 2, 15, 0, 0, 0, 0, time.UTC), 31, d(2027, 1, 31)},
		{"exactly at the start", d(2027, 2, 28), 31, d(2027, 2, 28)},
		{"one nanosecond earlier", d(2027, 2, 28).Add(-time.Nanosecond), 31, d(2027, 1, 31)},
		{"ordinary day, same month", time.Date(2027, 5, 20, 8, 0, 0, 0, time.UTC), 15, d(2027, 5, 15)},
		{"ordinary day, previous month", time.Date(2027, 5, 10, 8, 0, 0, 0, time.UTC), 15, d(2027, 4, 15)},
		{"across the year", time.Date(2027, 1, 3, 8, 0, 0, 0, time.UTC), 15, d(2026, 12, 15)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CycleStart(tt.now, tt.day); !got.Equal(tt.want) {
				t.Errorf("CycleStart(%v, %d) = %v, want %v", tt.now, tt.day, got, tt.want)
			}
		})
	}
}

func TestCost(t *testing.T) {
	prices := loadPrices(t)
	tests := []struct {
		name  string
		model string
		u     batch.Usage
		batch bool
		want  float64
	}{
		// plain-s, 1,000,000 one-hour cache-write tokens at 8 per million = 8.
		{"one-hour cache write", "plain-s", batch.Usage{CacheWrite1h: 1_000_000}, false, 8},
		// haiku-s at exactly 100,000 total input (60,000 + 30,000 + 10,000), standard rates:
		// (60000*1 + 10000*5 + 10000*1.25 + 30000*0.125) / 1e6 = 126250 / 1e6 = 0.12625.
		{"at the threshold", "haiku-s", batch.Usage{InputTokens: 60_000, OutputTokens: 10_000, CacheWrite5m: 10_000, CacheRead: 30_000}, false, 0.12625},
		// The same with 30,001 cache-read tokens: total 100,001, every rate from the tier:
		// (60000*2 + 10000*8 + 10000*2.5 + 30001*0.25) / 1e6 = 232500.25 / 1e6 = 0.23250025.
		{"one over the threshold", "haiku-s", batch.Usage{InputTokens: 60_000, OutputTokens: 10_000, CacheWrite5m: 10_000, CacheRead: 30_001}, false, 0.23250025},
		// Batch mode halves every rate: 0.12625 * 0.5 = 0.063125 and 0.23250025 * 0.5 = 0.116250125.
		{"at the threshold, batch", "haiku-s", batch.Usage{InputTokens: 60_000, OutputTokens: 10_000, CacheWrite5m: 10_000, CacheRead: 30_000}, true, 0.063125},
		{"over the threshold, batch", "haiku-s", batch.Usage{InputTokens: 60_000, OutputTokens: 10_000, CacheWrite5m: 10_000, CacheRead: 30_001}, true, 0.116250125},
		// Cache tokens alone push total input over: 100,001 cache-write-1h tokens at the tier's 4 = 0.400004.
		{"cache write alone crosses", "haiku-s", batch.Usage{CacheWrite1h: 100_001}, false, 0.400004},
		// plain-s has no tier: (1e6*4 + 1e6*16) / 1e6 = 20 sync, 10 batch.
		{"no tier, sync", "plain-s", batch.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000}, false, 20},
		{"no tier, batch", "plain-s", batch.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000}, true, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Cost(prices, tt.model, tt.u, tt.batch)
			if err != nil {
				t.Fatal(err)
			}
			if !near(got, tt.want) {
				t.Errorf("Cost = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWorstFunc(t *testing.T) {
	prices := loadPrices(t)
	tests := []struct {
		name      string
		model     string
		in, max   int64
		batchMode bool
		want      float64
	}{
		{"sync", "plain-s", 1_000_000, 500_000, false, 12},
		{"batch", "plain-s", 1_000_000, 500_000, true, 6},
		// haiku-s, 200,000 input is over the threshold: (200000*2 + 10000*8) / 1e6 = 0.48.
		{"tier", "haiku-s", 200_000, 10_000, false, 0.48},
		// 100,000 input is not: (100000*1 + 10000*5) / 1e6 = 0.15.
		{"at the threshold", "haiku-s", 100_000, 10_000, false, 0.15},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Worst(prices, tt.model, tt.in, tt.max, tt.batchMode)
			if err != nil {
				t.Fatal(err)
			}
			if !near(got, tt.want) {
				t.Errorf("Worst = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUnknownModel(t *testing.T) {
	prices := loadPrices(t)
	if _, err := Cost(prices, "nope", batch.Usage{InputTokens: 1}, false); err == nil {
		t.Error("Cost of an unpriced model returned no error")
	}
	if _, err := Worst(prices, "nope", 1, 1, false); err == nil {
		t.Error("Worst of an unpriced model returned no error")
	}
}

func TestPageSpendAfterResume(t *testing.T) {
	l, _, dir := newLedger(t)
	mustReserve(t, l, res("r1", "p1", "i1", 1, 5), wide)
	mustSettle(t, l, settle("p1", "i1", "m1", "actual", 2))
	mustReserve(t, l, res("r1", "p1", "i2", 1, 4), wide)

	resumed := Open(dir)
	if got := pageSpend(t, resumed, "r1", "p1"); got != 6 {
		t.Errorf("PageSpend after resume = %v, want 6 (2 settled + 4 open)", got)
	}
	if got := pageSpend(t, resumed, "r1", "other"); got != 0 {
		t.Errorf("another page = %v, want 0", got)
	}
	if got := pageSpend(t, resumed, "r2", "p1"); got != 0 {
		t.Errorf("another run = %v, want 0", got)
	}
}

func TestOpenIntentsByRun(t *testing.T) {
	l, _, _ := newLedger(t)
	mustReserve(t, l, res("r1", "p1", "i1", 2, 5), wide)
	mustReserve(t, l, res("r1", "p1", "i2", 1, 3), wide)
	mustReserve(t, l, res("r2", "p1", "i3", 1, 3), wide)
	mustSettle(t, l, settle("p1", "i2", "m2", "actual", 1))
	got, err := l.OpenIntents("r1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Intent != "i1" || got[0].Requests != 2 || got[0].Worst != 5 || got[0].RunID != "r1" {
		t.Errorf("OpenIntents(r1) = %+v, want only i1", got)
	}
}

func TestFileStates(t *testing.T) {
	t.Run("absent and empty read as zero", func(t *testing.T) {
		l, _, dir := newLedger(t)
		rep, err := l.Spend(time.Time{}, time.Time{})
		if err != nil || rep.Total != 0 || len(rep.Rows) != 0 || rep.Open.Count != 0 {
			t.Errorf("absent: %+v, %v", rep, err)
		}
		if got := pageSpend(t, l, "r1", "p1"); got != 0 {
			t.Errorf("absent PageSpend = %v", got)
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "ledger.jsonl"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		rep, err = l.Spend(time.Time{}, time.Time{})
		if err != nil || rep.Total != 0 {
			t.Errorf("empty: %+v, %v", rep, err)
		}
	})
	t.Run("first append creates the directory and file with their modes", func(t *testing.T) {
		l, _, dir := newLedger(t)
		mustReserve(t, l, res("r1", "p1", "i1", 1, 1), wide)
		di, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if di.Mode().Perm() != 0o700 {
			t.Errorf("dir mode = %v, want 0700", di.Mode().Perm())
		}
		fi, err := os.Stat(filepath.Join(dir, "ledger.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("file mode = %v, want 0600", fi.Mode().Perm())
		}
	})
	t.Run("a torn final line is skipped and gets its own line", func(t *testing.T) {
		l, _, dir := newLedger(t)
		mustReserve(t, l, res("r1", "p1", "i1", 1, 2), wide)
		path := filepath.Join(dir, "ledger.jsonl")
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		const frag = `{"v":1,"type":"reserve","intent":"torn`
		if _, err := f.WriteString(frag); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()

		warn := &bytes.Buffer{}
		l.Warn = warn
		if got := pageSpend(t, l, "r1", "p1"); got != 2 {
			t.Errorf("PageSpend with a torn tail = %v, want 2", got)
		}
		if !strings.Contains(warn.String(), "ledger.jsonl") || !strings.Contains(warn.String(), "line 2") {
			t.Errorf("warning %q does not name the file and line", warn.String())
		}
		mustReserve(t, l, res("r1", "p1", "i2", 1, 3), wide)
		got := lines(t, dir)
		found := false
		for _, ln := range got {
			if ln == frag {
				found = true
			}
		}
		if !found {
			t.Errorf("the fragment is not on its own line: %q", got)
		}
		// The ledger keeps working after the repair.
		if got := pageSpend(t, l, "r1", "p1"); got != 5 {
			t.Errorf("PageSpend after repair = %v, want 5", got)
		}
		mustSettle(t, l, settle("p1", "i2", "m", "actual", 1))
	})
	t.Run("a corrupt middle line fails every operation naming the line", func(t *testing.T) {
		l, _, dir := newLedger(t)
		mustReserve(t, l, res("r1", "p1", "i1", 1, 2), wide)
		mustReserve(t, l, res("r1", "p1", "i2", 1, 2), wide)
		path := filepath.Join(dir, "ledger.jsonl")
		ls := lines(t, dir)
		ls[0] = "this is not json"
		if err := os.WriteFile(path, []byte(strings.Join(ls, "\n")+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		ops := map[string]func() error{
			"Reserve": func() error { return l.Reserve(res("r1", "p1", "i3", 1, 1), wide) },
			"Settle":  func() error { return l.Settle(settle("p1", "i2", "m", "actual", 1)) },
			"Spend": func() error {
				_, err := l.Spend(time.Time{}, time.Time{})
				return err
			},
		}
		for name, op := range ops {
			err := op()
			if err == nil {
				t.Errorf("%s succeeded over a corrupt line", name)
				continue
			}
			if !strings.Contains(err.Error(), "ledger.jsonl") || !strings.Contains(err.Error(), "line 1") {
				t.Errorf("%s error %q does not name file and line", name, err)
			}
		}
	})
	t.Run("an unknown field reads", func(t *testing.T) {
		l, _, dir := newLedger(t)
		mustReserve(t, l, res("r1", "p1", "i1", 1, 2), wide)
		line := lines(t, dir)[0]
		line = strings.TrimSuffix(line, "}") + `,"future":"x"}`
		if err := os.WriteFile(filepath.Join(dir, "ledger.jsonl"), []byte(line+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := pageSpend(t, l, "r1", "p1"); got != 2 {
			t.Errorf("PageSpend = %v, want 2", got)
		}
	})
}

func TestLineShape(t *testing.T) {
	l, ck, dir := newLedger(t)
	mustReserve(t, l, res("r1", "p1", "i1", 1, 2), wide)
	ck.t = t0.Add(90 * time.Minute)
	mustSettle(t, l, settle("p1", "i1", "m1", "actual", 1))
	wantTypes := []string{"reserve", "settle"}
	wantTimes := []time.Time{t0, t0.Add(90 * time.Minute)}
	for i, ln := range lines(t, dir) {
		var m map[string]any
		if err := json.Unmarshal([]byte(ln), &m); err != nil {
			t.Fatal(err)
		}
		if m["v"] != float64(1) {
			t.Errorf("line %d v = %v, want 1", i+1, m["v"])
		}
		if m["type"] != wantTypes[i] {
			t.Errorf("line %d type = %v, want %s", i+1, m["type"], wantTypes[i])
		}
		ts, _ := m["time"].(string)
		got, err := time.Parse(time.RFC3339Nano, ts)
		if err != nil || !got.Equal(wantTimes[i]) || !strings.HasSuffix(ts, "Z") {
			t.Errorf("line %d time = %q, want %v in UTC", i+1, ts, wantTimes[i])
		}
	}
}

func TestSpendReport(t *testing.T) {
	l, ck, _ := newLedger(t)
	other := func(r Reservation, repo, seat string) Reservation {
		r.Repo, r.Seat = repo, seat
		return r
	}
	otherS := func(s Settlement, repo, seat string) Settlement {
		s.Repo, s.Seat = repo, seat
		return s
	}
	ck.t = t0
	mustReserve(t, l, other(res("r1", "p1", "i1", 2, 5), "a", "draft"), wide)
	mustSettle(t, l, otherS(settle("p1", "i1", "b/x", "actual", 1.5), "a", "draft"))
	mustSettle(t, l, otherS(settle("p1", "i1", "b/y", "actual", 0.25), "a", "draft"))
	ck.t = t0.Add(time.Hour)
	mustReserve(t, l, other(res("r1", "p2", "i2", 1, 3), "a", "edit"), wide)
	mustSettle(t, l, otherS(settle("p2", "i2", "i2/c1", "estimated", 3), "a", "edit"))
	mustReserve(t, l, other(res("r1", "p3", "i3", 1, 2), "b", "draft"), wide)
	mustSettle(t, l, otherS(settle("p3", "i3", "m3", "actual", 2), "b", "draft"))
	mustReserve(t, l, other(res("r1", "p4", "i4", 2, 7), "b", "draft"), wide) // open

	rep, err := l.Spend(time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	want := []Row{
		{Repo: "a", Seat: "draft", Cost: 1.75, Requests: 2},
		{Repo: "a", Seat: "edit", Cost: 3, Requests: 1, Estimated: 1},
		{Repo: "b", Seat: "draft", Cost: 2, Requests: 1},
	}
	if len(rep.Rows) != len(want) {
		t.Fatalf("rows = %+v, want %+v", rep.Rows, want)
	}
	for i := range want {
		if rep.Rows[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, rep.Rows[i], want[i])
		}
	}
	if rep.Total != 6.75 {
		t.Errorf("Total = %v, want 6.75", rep.Total)
	}
	if rep.Open.Count != 1 || rep.Open.Worst != 7 {
		t.Errorf("Open = %+v, want 1 intent and 7", rep.Open)
	}

	// since is inclusive and until exclusive: only the lines at t0+1h remain.
	rep, err = l.Spend(t0.Add(time.Hour), t0.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if rep.Total != 5 {
		t.Errorf("windowed Total = %v, want 5", rep.Total)
	}
	rep, err = l.Spend(time.Time{}, t0.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if rep.Total != 1.75 {
		t.Errorf("until-exclusive Total = %v, want 1.75", rep.Total)
	}
}
