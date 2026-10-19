package e2e

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
)

// liveLimit is the dollars a live check may spend, measured from the baseline recorded at launch.
const liveLimit = 20.0

// meterReading is the part of `drafter spend --json` the budget reads: the ledger's total since
// the meter date, and the worst case of the reservations still open.
type meterReading struct {
	Total float64 `json:"total"`
	Open  struct {
		Worst float64 `json:"worst"`
	} `json:"open"`
}

func parseMeter(data []byte) (meterReading, error) {
	var r meterReading
	if err := json.Unmarshal(data, &r); err != nil {
		return r, fmt.Errorf("spend report is not JSON: %w", err)
	}
	return r, nil
}

// parseBaseline reads DRAFTER_LIVE_BASELINE, the meter's total at launch.
func parseBaseline(s string) (float64, error) {
	if s == "" {
		return 0, errors.New("DRAFTER_LIVE_BASELINE is not set; give the meter's total at launch")
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("DRAFTER_LIVE_BASELINE %q is not a dollar amount", s)
	}
	return v, nil
}

// budgetLeft is the limit minus what the pass has spent: the meter's total over the baseline, plus
// the worst case of the open reservations.
func budgetLeft(limit, baseline float64, r meterReading) float64 {
	return limit - (r.Total - baseline + r.Open.Worst)
}

// pageCapFor is the value of DRAFTER_PAGE_CAP for a call that may spend left dollars. It rounds
// down to the cent so the cap never exceeds the budget, and refuses a budget at or below the
// smallest cap a run can hold.
func pageCapFor(left float64) (string, error) {
	capped := math.Floor(left*100) / 100
	if capped <= 0 {
		return "", fmt.Errorf("the live budget is spent: $%.4f left", left)
	}
	return strconv.FormatFloat(capped, 'f', 2, 64), nil
}
