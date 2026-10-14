package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// spendProject is a state directory holding a synthetic ledger, reached through a temp user
// config.
type spendProject struct {
	work, state, xdg string
}

func newSpendProject(t *testing.T, cycleDay int) spendProject {
	t.Helper()
	p := spendProject{work: t.TempDir(), state: t.TempDir(), xdg: t.TempDir()}
	dir := filepath.Join(p.xdg, "drafter")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf("state_dir = %q\nbilling_cycle_day = %d\n", p.state, cycleDay)
	writeFile(t, filepath.Join(dir, "config.toml"), body)
	return p
}

// ledger writes the lines as the ledger file, one per line.
func (p spendProject) ledger(t *testing.T, lines ...string) {
	t.Helper()
	writeFile(t, filepath.Join(p.state, "ledger.jsonl"), strings.Join(lines, "\n")+"\n")
}

func (p spendProject) spend(t *testing.T, args ...string) result {
	t.Helper()
	return runEnv(t, p.work, []string{"XDG_CONFIG_HOME=" + p.xdg}, append([]string{"spend"}, args...)...)
}

func settleLine(at time.Time, repo, seat, kind string, cost float64) string {
	stamp := at.UTC().Format(time.RFC3339Nano)
	return fmt.Sprintf(`{"v":1,"type":"settle","time":%q,"run_id":"r1","repo":%q,"page":"p","seat":%q,`+
		`"model":"m","mode":"batch","intent":"i-%s-%s-%s","key":"k-%s-%s-%s","usage":{},"cost":%v,"kind":%q}`,
		stamp, repo, seat, repo, seat, stamp, repo, seat, stamp, cost, kind)
}

func reserveLine(at time.Time, intent string, requests int, worst float64) string {
	return fmt.Sprintf(`{"v":1,"type":"reserve","time":%q,"run_id":"r1","repo":"acme","page":"p","seat":"draft",`+
		`"model":"m","mode":"batch","intent":%q,"requests":%d,"worst":%v}`,
		at.UTC().Format(time.RFC3339), intent, requests, worst)
}

type spendJSON struct {
	Rows []struct {
		Repo      string  `json:"repo"`
		Seat      string  `json:"seat"`
		Cost      float64 `json:"cost"`
		Requests  int     `json:"requests"`
		Estimated int     `json:"estimated"`
	} `json:"rows"`
	Total float64 `json:"total"`
	Open  struct {
		Count int     `json:"count"`
		Worst float64 `json:"worst"`
	} `json:"open"`
}

func spendJSONOf(t *testing.T, r result) spendJSON {
	t.Helper()
	if r.code != 0 {
		t.Fatalf("spend --json: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	var got spendJSON
	if err := json.Unmarshal([]byte(r.stdout), &got); err != nil {
		t.Fatalf("spend --json output is not one object: %v\n%s", err, r.stdout)
	}
	return got
}

// flat collapses runs of whitespace so a test need not know the column widths.
func flat(s string) string { return strings.Join(strings.Fields(s), " ") }

var spendDay = time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)

// Two repos by two seats, hand-computed: acme/draft 0.25+0.5 = 0.75 over 2 requests, acme/edit 1,
// beta/draft 2.5; total 4.25. Every amount is exact in binary.
func spendRows(at time.Time) []string {
	return []string{
		settleLine(at, "acme", "draft", "actual", 0.25),
		settleLine(at.Add(time.Second), "acme", "draft", "actual", 0.5),
		settleLine(at, "acme", "edit", "actual", 1),
		settleLine(at, "beta", "draft", "actual", 2.5),
	}
}

func TestSpendRowsAndTotal(t *testing.T) {
	p := newSpendProject(t, 1)
	p.ledger(t, spendRows(spendDay)...)
	r := p.spend(t, "--since", "2026-09-01")
	if r.code != 0 {
		t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	text := flat(r.stdout)
	for _, want := range []string{
		"acme draft 0.7500 2",
		"acme edit 1.0000 1",
		"beta draft 2.5000 1",
		"total 4.2500",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("stdout lacks %q\n%s", want, r.stdout)
		}
	}
	got := spendJSONOf(t, p.spend(t, "--since", "2026-09-01", "--json"))
	if len(got.Rows) != 3 || got.Rows[0].Repo != "acme" || got.Rows[0].Seat != "draft" ||
		got.Rows[0].Cost != 0.75 || got.Rows[0].Requests != 2 || got.Rows[2].Repo != "beta" ||
		got.Rows[2].Cost != 2.5 {
		t.Errorf("rows = %+v", got.Rows)
	}
	if got.Total != 4.25 {
		t.Errorf("total = %v, want 4.25", got.Total)
	}
}

func TestSpendEstimated(t *testing.T) {
	p := newSpendProject(t, 1)
	p.ledger(t,
		settleLine(spendDay, "acme", "draft", "actual", 0.5),
		settleLine(spendDay.Add(time.Second), "acme", "draft", "estimated", 0.25),
		settleLine(spendDay, "beta", "edit", "actual", 1))
	r := p.spend(t, "--since", "2026-09-01")
	var marked, plain int
	for _, ln := range lines(r.stdout) {
		switch {
		case strings.HasPrefix(ln, "acme") && strings.Contains(ln, "estimated"):
			marked++
		case strings.HasPrefix(ln, "beta") && !strings.Contains(ln, "estimated"):
			plain++
		}
	}
	if marked != 1 || plain != 1 {
		t.Errorf("acme line marked %d times, beta line clean %d times\n%s", marked, plain, r.stdout)
	}
	got := spendJSONOf(t, p.spend(t, "--since", "2026-09-01", "--json"))
	if len(got.Rows) != 2 || got.Rows[0].Estimated != 1 || got.Rows[1].Estimated != 0 || got.Rows[0].Cost != 0.75 {
		t.Errorf("rows = %+v", got.Rows)
	}
}

func TestSpendOpenReservation(t *testing.T) {
	p := newSpendProject(t, 1)
	p.ledger(t,
		settleLine(spendDay, "acme", "edit", "actual", 1),
		reserveLine(spendDay, "i-open", 2, 3))
	text := flat(p.spend(t, "--since", "2026-09-01").stdout)
	if !strings.Contains(text, "open 1 reservation, worst 3.0000") {
		t.Errorf("no open line: %q", text)
	}
	if !strings.Contains(text, "total 1.0000") {
		t.Errorf("open reservation leaked into the total: %q", text)
	}
	got := spendJSONOf(t, p.spend(t, "--since", "2026-09-01", "--json"))
	if got.Open.Count != 1 || got.Open.Worst != 3 || got.Total != 1 || len(got.Rows) != 1 {
		t.Errorf("json = %+v", got)
	}
}

func TestSpendDefaultWindow(t *testing.T) {
	const day = 15
	start := cycleStart(time.Now(), day)
	p := newSpendProject(t, day)
	p.ledger(t,
		settleLine(start.Add(-time.Second), "acme", "draft", "actual", 4),
		settleLine(start.Add(time.Second), "acme", "edit", "actual", 0.5))
	got := spendJSONOf(t, p.spend(t, "--json"))
	if got.Total != 0.5 || len(got.Rows) != 1 || got.Rows[0].Seat != "edit" {
		t.Errorf("default window = %+v, want only the line after the cycle start %s", got, start)
	}
}

// cycleStart restates the cycle rule so the test does not call the code it checks: the latest
// day-of-month at or before now, clamped to a short month.
func cycleStart(now time.Time, day int) time.Time {
	now = now.UTC()
	for m := 0; ; m-- {
		first := time.Date(now.Year(), now.Month()+time.Month(m), 1, 0, 0, 0, 0, time.UTC)
		last := first.AddDate(0, 1, -1).Day()
		if s := first.AddDate(0, 0, min(day, last)-1); !s.After(now) {
			return s
		}
	}
}

func TestSpendUntilIsInclusive(t *testing.T) {
	p := newSpendProject(t, 1)
	p.ledger(t,
		settleLine(time.Date(2026, 9, 9, 23, 59, 0, 0, time.UTC), "acme", "draft", "actual", 0.25),
		settleLine(time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC), "acme", "edit", "actual", 4),
		settleLine(time.Date(2026, 9, 4, 23, 59, 0, 0, time.UTC), "acme", "edit", "actual", 8))
	got := spendJSONOf(t, p.spend(t, "--since", "2026-09-05", "--until", "2026-09-09", "--json"))
	if got.Total != 0.25 || len(got.Rows) != 1 || got.Rows[0].Seat != "draft" {
		t.Errorf("window = %+v, want only the 23:59 line on 2026-09-09", got)
	}
}

func TestSpendJSONMatchesText(t *testing.T) {
	p := newSpendProject(t, 1)
	p.ledger(t, spendRows(spendDay)...)
	got := spendJSONOf(t, p.spend(t, "--since", "2026-09-01", "--json"))
	var sum float64
	for _, row := range got.Rows {
		sum += row.Cost
	}
	text := flat(p.spend(t, "--since", "2026-09-01").stdout)
	if sum != got.Total || !strings.Contains(text, fmt.Sprintf("total %.4f", got.Total)) {
		t.Errorf("rows sum %v, json total %v, text %q", sum, got.Total, text)
	}
}

func TestSpendMissingLedger(t *testing.T) {
	p := newSpendProject(t, 1)
	r := p.spend(t)
	if r.code != 0 || !strings.Contains(flat(r.stdout), "total 0.0000") {
		t.Errorf("text: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	j := p.spend(t, "--json")
	got := spendJSONOf(t, j)
	if got.Total != 0 || got.Open.Count != 0 || strings.Contains(j.stdout, `"rows": null`) {
		t.Errorf("json = %+v\n%s", got, j.stdout)
	}
}

func TestSpendCorruptLine(t *testing.T) {
	p := newSpendProject(t, 1)
	p.ledger(t,
		settleLine(spendDay, "acme", "draft", "actual", 0.25),
		"this is not json",
		settleLine(spendDay, "beta", "draft", "actual", 0.5))
	r := p.spend(t, "--since", "2026-09-01")
	if r.code != 1 || !strings.Contains(r.stderr, "line 2") {
		t.Errorf("exit %d, stderr %q, want exit 1 naming line 2", r.code, r.stderr)
	}
}

func TestSpendBadDate(t *testing.T) {
	p := newSpendProject(t, 1)
	r := p.spend(t, "--since", "last week")
	if r.code != 2 || !strings.Contains(r.stderr, "--since") {
		t.Errorf("exit %d, stderr %q", r.code, r.stderr)
	}
}
