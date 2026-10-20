package pipeline

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/batch"
	"github.com/glw907/drafter/internal/edits"
)

// kinded is a blocking review finding with a kind.
func kinded(kind, location, text string) map[string]any {
	f := finding(location, text)
	f["kind"] = kind
	return f
}

// advisory is a non-blocking review finding with a kind.
func advisory(kind, location, text string) map[string]any {
	f := kinded(kind, location, text)
	f["blocking"] = false
	return f
}

var renderedFinding = regexp.MustCompile(`(?m)^Finding \d+ \(`)

// The texts the round-1 reads and the round-2 reads of the small-path tests raise, so a test can
// tell which findings reached a request.
const (
	round1Text = "ROUND-ONE: name the key."
	smallFix   = "2. Publish the site with the new key."
	smallFixed = "2. Publish the site with the new key now."
)

// roundTwoOrigin scripts page A to a round-2 verdict: each seat named in round2 answers fix in
// round 1, the redraft answers every round-1 finding, and the seat then answers fix in round 2
// with the findings given.
func roundTwoOrigin(h *harness, round2 map[string][]map[string]any) {
	var ns []int
	for _, seat := range reviewSeats {
		if _, ok := round2[seat]; !ok {
			continue
		}
		h.api.script(s4, seat, okText(readText("fix", kinded("claim", "Rotate the key", round1Text))))
		ns = append(ns, len(ns)+1)
		h.api.script(s6, seat, okText(readText("fix", round2[seat]...)))
	}
	h.api.script(s5, "draft", okText(answer(inOld, inNew, ns...)))
}

// smallAnswer answers the small exchange's n findings with one edit.
func smallAnswer(n int) outcome {
	var ns []int
	for i := 1; i <= n; i++ {
		ns = append(ns, i)
	}
	return okText(answer(smallFix, smallFixed, ns...))
}

// afterFirst returns the requests sent after the first one of stage and seat, or nil when there
// was none.
func afterFirst(h *harness, stage, seat string) []batch.Request {
	for i, r := range h.api.requests {
		if st, se := stageSeat(r.CustomID); st == stage && se == seat {
			return h.api.requests[i+1:]
		}
	}
	return nil
}

// assertSmallPathEnds checks that the small exchange ran once with exactly n findings, none of
// round 1's, and that nothing after it asked a review seat or the reader.
func assertSmallPathEnds(t *testing.T, h *harness, n int) {
	t.Helper()
	reqs := requestsFor(h, s6b, "draft")
	if len(reqs) != 1 {
		t.Fatalf("small exchange requests = %d, want 1", len(reqs))
	}
	text := userText(reqs[0])
	if got := len(renderedFinding.FindAllString(text, -1)); got != n {
		t.Errorf("the small exchange carries %d findings, want %d:\n%s", got, n, text)
	}
	if strings.Contains(text, round1Text) {
		t.Errorf("the small exchange carries a round-1 finding:\n%s", text)
	}
	after := afterFirst(h, s6b, "draft")
	for _, r := range after {
		if _, seat := stageSeat(r.CustomID); seat != "draft" && seat != "coverage" {
			t.Errorf("after the small exchange the run asked %s", r.CustomID)
		}
	}
	if len(requestsFor(h, s6b, "coverage")) != 1 {
		t.Error("no coverage read in the small phase")
	}
}

func TestSmallFindingRuleAtRoundTwo(t *testing.T) {
	tests := []struct {
		name   string
		round2 map[string][]map[string]any
		rule   string // "" accepts through the small exchange
		seat   string
		small  int
	}{
		{"only link and handoff findings take the small exchange", map[string][]map[string]any{
			"structure": {kinded("link", "Before you begin", "LINK"), kinded("handoff", "Rotate the key", "HANDOFF")},
		}, "", "", 2},
		{"a voice finding beside a link escalates", map[string][]map[string]any{
			"structure": {kinded("voice", "Rotate the key", "VOICE"), kinded("link", "Before you begin", "LINK")},
		}, "second fix verdict", "structure", 0},
		{"every seat is weighed: a small structure fix beside a fact claim escalates", map[string][]map[string]any{
			"structure": {kinded("link", "Before you begin", "LINK")},
			"fact":      {kinded("claim", "Rotate the key", "CLAIM")},
		}, "second fix verdict", "fact", 0},
		{"non-blocking findings beside one blocking heading take the small exchange", map[string][]map[string]any{
			"structure": {advisory("voice", "Rotate the key", "Nit: tighten it."), advisory("other", "intro", "Already present: old."), kinded("heading", "Rotate the key", "HEADING")},
		}, "", "", 1},
		{"a fix with no blocking finding escalates", map[string][]map[string]any{
			"structure": {advisory("link", "Before you begin", "Nit: a shorter link text.")},
		}, "second fix verdict", "structure", 0},
		{"an empty kind is not small", map[string][]map[string]any{
			"structure": {kinded("", "Before you begin", "EMPTY")},
		}, "second fix verdict", "structure", 0},
		{"a finding with no kind field is not small", map[string][]map[string]any{
			"structure": {finding("Before you begin", "NONE")},
		}, "second fix verdict", "structure", 0},
		{"an unknown kind is not small", map[string][]map[string]any{
			"structure": {kinded("typo", "Before you begin", "UNKNOWN")},
		}, "second fix verdict", "structure", 0},
		{"a kind spelled Link is small", map[string][]map[string]any{
			"structure": {kinded("Link", "Before you begin", "LINK")},
		}, "", "", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, withoutFacts, withPages("a"))
			roundTwoOrigin(h, tt.round2)
			if tt.small > 0 {
				h.api.script(s6b, "draft", smallAnswer(tt.small))
			}
			s := h.mustRun()
			if tt.rule != "" {
				if r := ruleOf(s, pageA); r != tt.rule {
					t.Fatalf("rule = %q, want %q", r, tt.rule)
				}
				if rec := h.record(s.RunID, pageA); rec.Escalation == nil || rec.Escalation.Seat != tt.seat {
					t.Errorf("escalation = %+v, want seat %s", rec.Escalation, tt.seat)
				}
				if n := len(requestsFor(h, s6b, "draft")); n != 0 {
					t.Errorf("small exchange requests = %d, want none", n)
				}
				return
			}
			mustAccept(t, h, s)
			assertSmallPathEnds(t, h, tt.small)
			if !strings.Contains(h.read(pageA), smallFixed) {
				t.Error("the small exchange's edit is not on the page")
			}
		})
	}
}

// The redraft edits of the coverage-origin tests.
const (
	linkedTTL   = "A key lives for 90 days. See [key lifetime](../reference/keys.md#key-lifetime)"
	unlinkedTTL = "A key lasts. See key lifetime"
)

func TestSmallFindingRuleAtCoverage(t *testing.T) {
	briefEdit := func(old, new string, ns ...int) outcome {
		a := edits.Answer{Edits: []edits.Edit{{File: "brief", Old: old, New: new}}}
		for _, n := range ns {
			a.Resolutions = append(a.Resolutions, edits.Resolution{Finding: n, Status: "edited", Edits: []int{0}, Reason: "fixed"})
		}
		return okText(jsonText(a))
	}
	tests := []struct {
		name    string
		opts    []harnessOption
		redraft outcome
		small   []outcome
		rule    string
		detail  string
		smallN  int
	}{
		{"link and brief misses take the small exchange", []harnessOption{withoutFacts},
			okText(answer(linkedTTL, unlinkedTTL, 1)),
			[]outcome{okText(answer(unlinkedTTL, linkedTTL, 1, 2))}, "", "", 2},
		{"a heading-not-found miss escalates", []harnessOption{withoutFacts},
			okText(answer("## Before you begin", "## Before you start", 1)),
			nil, "second fix verdict", "l-lifetime: heading not found", 0},
		{"a cites-fact miss is a citation and takes the small exchange", nil,
			briefEdit(`"id":"lumen-key-ttl"`, `"id":"lumen-grace"`, 1),
			[]outcome{briefEdit(`"id":"lumen-grace"`, `"id":"lumen-key-ttl"`, 1)}, "", "", 1},
		{"a miss after the small fix escalates even on small kinds", []harnessOption{withoutFacts},
			okText(answer(linkedTTL, unlinkedTTL, 1)),
			[]outcome{okText(jsonText(edits.Answer{Edits: []edits.Edit{}, Resolutions: []edits.Resolution{
				{Finding: 1, Status: "declined", Edits: []int{}, Reason: "no"},
				{Finding: 2, Status: "declined", Edits: []int{}, Reason: "no"},
			}}))}, "second fix verdict", "after the small-finding fix:", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, append([]harnessOption{withPages("a")}, tt.opts...)...)
			structureFixes(h)
			h.api.script(s5, "draft", tt.redraft)
			h.api.script(s6b, "draft", tt.small...)
			s := h.mustRun()
			if n := len(requestsFor(h, s6, "structure")); n != 0 {
				t.Errorf("round-2 requests = %d, want none", n)
			}
			if tt.rule != "" {
				if r := ruleOf(s, pageA); r != tt.rule {
					t.Fatalf("rule = %q, want %q", r, tt.rule)
				}
				rec := h.record(s.RunID, pageA)
				if rec.Escalation == nil || rec.Escalation.Seat != "coverage" || !strings.Contains(rec.Escalation.Detail, tt.detail) {
					t.Errorf("escalation = %+v, want seat coverage and detail holding %q", rec.Escalation, tt.detail)
				}
				if strings.HasPrefix(tt.detail, "after") && !strings.HasPrefix(rec.Escalation.Detail, tt.detail) {
					t.Errorf("detail = %q, want it to open %q", rec.Escalation.Detail, tt.detail)
				}
				if got := len(requestsFor(h, s6b, "draft")); got != len(tt.small) {
					t.Errorf("small exchange requests = %d, want %d", got, len(tt.small))
				}
				return
			}
			mustAccept(t, h, s)
			assertSmallPathEnds(t, h, tt.smallN)
		})
	}
}

func TestSmallExchangeGateRedPastContract(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("a"))
	roundTwoOrigin(h, map[string][]map[string]any{"structure": {kinded("link", "Before you begin", "LINK")}})
	h.api.script(s6b, "draft",
		okText(answer(smallFix, "BROKEN", 1)),
		okText(answer("BROKEN", "BROKEN again", 2)),
		okText(answer("BROKEN again", "BROKEN thrice", 3)))
	s := h.mustRun()
	if r := ruleOf(s, pageA); r != "contract" {
		t.Fatalf("rule = %q, want contract", r)
	}
	if n := len(requestsFor(h, s6b, "draft")); n != 3 {
		t.Errorf("small exchange requests = %d, want the turn and two gate-fix turns", n)
	}
}

func TestSmallPathRecord(t *testing.T) {
	h := newHarness(t, withPages("a"))
	h.api.script(s4, "structure", okText(readText("fix", kinded("step", "Rotate the key", "Name the key."))))
	h.api.script(s5, "draft", okText(answer(linkedTTL, unlinkedTTL, 1)))
	h.api.script(s6b, "draft", okText(answer(unlinkedTTL, linkedTTL, 1, 2, 3)))
	s := h.mustRun()
	mustAccept(t, h, s)
	rec := h.record(s.RunID, pageA)
	var coverageKinds []string
	for _, f := range rec.Findings {
		if f.Kind == "" {
			t.Errorf("finding %+v has no kind", f)
		}
		if f.Seat == "coverage" {
			coverageKinds = append(coverageKinds, f.Kind)
		}
	}
	slices.Sort(coverageKinds)
	if want := []string{"citation", "citation", "link"}; !slices.Equal(coverageKinds, want) {
		t.Errorf("coverage finding kinds = %v, want %v", coverageKinds, want)
	}
	if !slices.ContainsFunc(rec.Writes, func(w writeRecord) bool { return w.Kind == exSmall && w.Step == stSmall }) {
		t.Errorf("writes = %+v, want one from the small exchange", rec.Writes)
	}
	if !slices.ContainsFunc(rec.Stages, func(st stageRecord) bool { return st.Stage == s6b && st.Seat == "draft" }) {
		t.Errorf("stages = %+v, want the small exchange's s6b", rec.Stages)
	}
}

func TestIsSmall(t *testing.T) {
	tests := []struct {
		kind string
		want bool
	}{
		{"link", true}, {"handoff", true}, {"heading", true}, {"citation", true},
		{"Link", true}, {"HANDOFF", true},
		{"fact", false}, {"claim", false}, {"code", false}, {"voice", false}, {"step", false},
		{"placement", false}, {"other", false}, {"", false}, {"typo", false},
	}
	for _, tt := range tests {
		if got := isSmall(tt.kind); got != tt.want {
			t.Errorf("isSmall(%q) = %v, want %v", tt.kind, got, tt.want)
		}
	}
}

func TestMissKind(t *testing.T) {
	tests := []struct {
		miss missRecord
		want string
	}{
		{missRecord{Kind: "link", Reason: "no link to x.md in this section"}, "link"},
		{missRecord{Kind: "handoff", Reason: "the coverage read found no passage for this item"}, "handoff"},
		{missRecord{Kind: "brief", Reason: "brief sentence not found on the page"}, "citation"},
		{missRecord{Kind: "fact", Reason: "no brief sentence in this section cites fact f"}, "citation"},
		{missRecord{Kind: "link", Reason: "heading not found"}, "placement"},
		{missRecord{Kind: "fact", Reason: "heading not found"}, "placement"},
		{missRecord{Kind: "verbatim", Reason: "verbatim sentence not found in this section"}, "code"},
		{missRecord{Kind: "reason", Reason: "the coverage read found no passage for this item"}, "claim"},
		{missRecord{Kind: "claim", Reason: "the quoted passage is not in this section"}, "claim"},
	}
	for _, tt := range tests {
		if got := missFinding(tt.miss).Kind; got != tt.want {
			t.Errorf("missFinding(%+v).Kind = %q, want %q", tt.miss, got, tt.want)
		}
	}
}
