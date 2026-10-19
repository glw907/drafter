package e2e

import (
	"strings"
	"testing"
)

func TestBudgetLeftAndPageCap(t *testing.T) {
	const baseline = 9.4436516
	reading := func(total, worst float64) meterReading {
		var r meterReading
		r.Total = total
		r.Open.Worst = worst
		return r
	}
	tests := []struct {
		name    string
		reading meterReading
		left    float64
		cap     string
		refuse  bool
	}{
		{"nothing spent since launch", reading(baseline, 0), 20, "20.00", false},
		{"spend over the baseline counts", reading(baseline+4.25, 0), 15.75, "15.75", false},
		{"an open reservation counts at its worst case", reading(baseline+4.25, 1.5), 14.25, "14.25", false},
		{"the cap rounds down to the cent", reading(baseline+0.0049, 0), 19.9951, "19.99", false},
		{"spend by another session counts", reading(baseline+19.5, 0), 0.5, "0.50", false},
		{"exactly spent is refused", reading(baseline+20, 0), 0, "", true},
		{"overspent is refused", reading(baseline+20.5, 0), -0.5, "", true},
		{"a sliver under a cent is refused", reading(baseline+19.995, 0), 0.005, "", true},
		{"open reservations alone can spend it", reading(baseline, 20), 0, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			left := budgetLeft(liveLimit, baseline, tt.reading)
			if d := left - tt.left; d > 1e-9 || d < -1e-9 {
				t.Errorf("budget left = %v, want %v", left, tt.left)
			}
			got, err := pageCapFor(left)
			if tt.refuse {
				if err == nil || !strings.Contains(err.Error(), "spent") {
					t.Errorf("pageCapFor(%v) = %q, %v; want a refusal", left, got, err)
				}
				return
			}
			if err != nil || got != tt.cap {
				t.Errorf("pageCapFor(%v) = %q, %v; want %q", left, got, err, tt.cap)
			}
		})
	}
}

func TestParseBaseline(t *testing.T) {
	tests := []struct {
		in   string
		want float64
		bad  bool
	}{
		{"9.4436516", 9.4436516, false},
		{"0", 0, false},
		{"", 0, true},
		{"nine", 0, true},
		{"-1", 0, true},
		{"NaN", 0, true},
		{"Inf", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseBaseline(tt.in)
			if (err != nil) != tt.bad || (!tt.bad && got != tt.want) {
				t.Errorf("parseBaseline(%q) = %v, %v", tt.in, got, err)
			}
		})
	}
}

func TestParseMeter(t *testing.T) {
	r, err := parseMeter([]byte(`{"rows":[{"repo":"x","seat":"draft","cost":1.5,"requests":2,"estimated":0}],"total":1.5,"open":{"count":1,"worst":0.25}}`))
	if err != nil || r.Total != 1.5 || r.Open.Worst != 0.25 {
		t.Errorf("parseMeter = %+v, %v", r, err)
	}
	if _, err := parseMeter([]byte("not json")); err == nil {
		t.Error("a report that is not JSON was accepted")
	}
}
