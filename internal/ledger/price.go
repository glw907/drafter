package ledger

import (
	"fmt"
	"time"

	"github.com/glw907/drafter/internal/batch"
	"github.com/glw907/drafter/internal/config"
)

// rates are one model's per-million-token rates after the long-context tier is chosen.
type rates struct {
	input, output, write5m, write1h, read float64
}

func ratesFor(p config.Price, totalInput int64) rates {
	if t := p.LongContext; t != nil && totalInput > t.Threshold {
		return rates{t.Input, t.Output, t.CacheWrite5m, t.CacheWrite1h, t.CacheRead}
	}
	return rates{p.Input, p.Output, p.CacheWrite5m, p.CacheWrite1h, p.CacheRead}
}

func (p rates) apply(u batch.Usage, discount float64) float64 {
	sum := float64(u.InputTokens)*p.input +
		float64(u.OutputTokens)*p.output +
		float64(u.CacheWrite5m)*p.write5m +
		float64(u.CacheWrite1h)*p.write1h +
		float64(u.CacheRead)*p.read
	return sum / 1e6 * (1 - discount)
}

// Cost returns the dollar cost of u on model. The long-context tier applies when total input
// (input, cache-read, and cache-write tokens) is over its threshold and its rates replace all
// five; batchMode takes the batch discount off every rate. A model with no price entry is an
// error, never a zero.
func Cost(p config.Prices, model string, u batch.Usage, batchMode bool) (float64, error) {
	price, ok := p.Models[model]
	if !ok {
		return 0, fmt.Errorf("no price for model %q in the price table", model)
	}
	discount := 0.0
	if batchMode {
		discount = p.BatchDiscount
	}
	total := u.InputTokens + u.CacheRead + u.CacheWrite5m + u.CacheWrite1h
	return ratesFor(price, total).apply(u, discount), nil
}

// Worst returns the most a request can cost: inputTokens of input and maxTokens of output at the
// same tier and discount rules as Cost. Input is priced at the higher of the input rate and the
// five-minute cache-write rate, so the figure bounds a request whose input is written to the
// five-minute cache. It does not bound a one-hour write, which drafter never requests.
func Worst(p config.Prices, model string, inputTokens, maxTokens int64, batchMode bool) (float64, error) {
	price, ok := p.Models[model]
	if !ok {
		return 0, fmt.Errorf("no price for model %q in the price table", model)
	}
	discount := 0.0
	if batchMode {
		discount = p.BatchDiscount
	}
	r := ratesFor(price, inputTokens)
	r.input = max(r.input, r.write5m)
	return r.apply(batch.Usage{InputTokens: inputTokens, OutputTokens: maxTokens}, discount), nil
}

// CycleStart returns 00:00 UTC on the most recent day-of-month day at or before now. A month too
// short for day starts on its last day.
func CycleStart(now time.Time, day int) time.Time {
	now = now.UTC()
	start := monthDay(now.Year(), now.Month(), day)
	if start.After(now) {
		start = monthDay(now.Year(), now.Month()-1, day)
	}
	return start
}

// monthDay is day of the month at 00:00 UTC, clamped to the month's last day. A month of 0 is
// the December before.
func monthDay(year int, month time.Month, day int) time.Time {
	last := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
	return time.Date(year, month, min(day, last), 0, 0, 0, 0, time.UTC)
}
