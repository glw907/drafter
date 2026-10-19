package pipeline

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"github.com/glw907/drafter/internal/blueprint"
)

func TestSummaryCarriesTheReadsFindings(t *testing.T) {
	const ttl, grace = "lumen-key-ttl", "lumen-grace"
	nit := map[string]any{"location": "intro", "finding": "Nit: the opening repeats the title.", "rewrite": "cut it", "blocking": false}
	older := map[string]any{"location": "rotate", "finding": "Already present: the step has no timeout.", "rewrite": "add one", "blocking": false}
	tests := []struct {
		name       string
		blueprint  string
		consistent string
		scope      blueprint.Scope
		status     string
		findings   []FindingResult
		warnings   []string
		detail     bool
	}{
		{
			name:       "an accepted change read keeps its non-blocking findings and its warning",
			blueprint:  readText("ok", nit, older),
			consistent: `{"conflicts":[{"facts":[],"with":"","conflict":"scope difference"}]}`,
			scope:      scopedFacts(nil),
			status:     StatusAccepted,
			findings: []FindingResult{
				{Location: "intro", Text: "Nit: the opening repeats the title."},
				{Location: "rotate", Text: "Already present: the step has no timeout."},
			},
			warnings: []string{"Unplaced: scope difference"},
		},
		{
			name:      "a blocking finding keeps its flag",
			blueprint: readText("fix", finding("rotate", "A step the page needs is missing."), nit),
			scope:     scopedFacts(nil),
			status:    StatusEscalated,
			findings: []FindingResult{
				{Location: "rotate", Text: "A step the page needs is missing.", Blocking: true},
				{Location: "intro", Text: "Nit: the opening repeats the title."},
			},
		},
		{
			name:       "an escalated consistency read carries its detail",
			blueprint:  readText("ok"),
			consistent: `{"conflicts":[{"facts":["` + ttl + `"],"with":"` + grace + `","conflict":"they disagree"}]}`,
			scope:      blueprint.Scope{Kind: blueprint.ScopeWhole, Why: "x"},
			status:     StatusEscalated,
			detail:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newBlueprintHarness(t, withPages("a"))
			h.api.script("s1", "blueprint", okText(tt.blueprint))
			if tt.consistent != "" {
				h.api.script("s1", "consistency", okText(tt.consistent))
			}
			o := h.options()
			o.Since = h.head()
			o.Scopes = map[string]blueprint.Scope{pageA: tt.scope}
			s, err := Run(context.Background(), o)
			if err != nil {
				t.Fatalf("Run: %v\nstderr: %s", err, h.err.String())
			}
			got := statusPage(s, pageA)
			if got.Status != tt.status {
				t.Fatalf("status = %s, want %s", got.Status, tt.status)
			}
			if !reflect.DeepEqual(got.Findings, tt.findings) {
				t.Errorf("findings = %+v, want %+v", got.Findings, tt.findings)
			}
			if !slices.Equal(got.Warnings, tt.warnings) {
				t.Errorf("warnings = %q, want %q", got.Warnings, tt.warnings)
			}
			if want := h.record(s.RunID, pageA).Escalation; tt.detail && (want == nil || want.Detail == "" || got.Detail != want.Detail) {
				t.Errorf("detail = %q, want the record's %+v", got.Detail, want)
			}
		})
	}
}

func TestResumeLineNamesSyncAndPages(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"), func(h *harness) { h.cfg.CycleCeiling = 0.0001; h.cfg.Mode = "sync" })
	h.opts = h.options()
	h.opts.Command = "drafter read"
	h.opts.Handoffs[0].File = "blueprints/a.blueprint.yaml"
	s, err := Run(t.Context(), h.opts)
	if err != nil {
		t.Fatal(err)
	}
	if want := "drafter read --sync --resume " + s.RunID + " blueprints/a.blueprint.yaml"; s.ResumeLine != want {
		t.Errorf("ResumeLine = %q, want %q", s.ResumeLine, want)
	}
}
