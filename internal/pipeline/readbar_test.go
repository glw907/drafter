package pipeline

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/blueprint"
)

const (
	barText   = "1. [rotate] A step the page needs is missing.\n2. [intro] Say what the page leaves out."
	barCommit = "0123456789abcdef0123456789abcdef01234567"
)

// withFindingsModule points the harness at a module whose blueprint seat prints the context's
// scope and findings, so a test can see what the seat was given.
func withFindingsModule(h *harness) {
	src, err := os.ReadFile(h.cfg.Prompts)
	if err != nil {
		h.t.Fatal(err)
	}
	text := strings.Replace(string(src), `blueprint: (c) => "Read the blueprint below.",`,
		`blueprint: (c) => "Read the blueprint below." + (c.scope ? "\nScope: " + c.scope : "") + (c.findings ? "\nFindings:\n" + c.findings : ""),`, 1)
	if text == string(src) {
		h.t.Fatal("the module's blueprint seat moved; update withFindingsModule")
	}
	path := filepath.Join(h.t.TempDir(), "module.js")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		h.t.Fatal(err)
	}
	h.cfg.Prompts = path
}

func blueprintPromptOf(t *testing.T, h *harness) (prompt, extra string) {
	t.Helper()
	reqs := requestsFor(h, "s1", "blueprint")
	if len(reqs) != 1 {
		t.Fatalf("blueprint requests = %d, want 1", len(reqs))
	}
	m := reqs[0].Messages[len(reqs[0].Messages)-1]
	return m.Cached[0], m.Text
}

// TestWholeBlockedRecordsOnlyAWholeRead covers when a read's summary says it judged the whole
// blueprint and blocked: a whole fix does, set before the consistency branch so a conflict's
// escalation still says so; a change read, a bar read, and an accept do not.
func TestWholeBlockedRecordsOnlyAWholeRead(t *testing.T) {
	fix := okText(readText("fix", finding("rotate", "A step the page needs is missing.")))
	tests := []struct {
		name     string
		verdict  outcome
		conflict bool
		set      func(o *Options)
		status   string
		seat     string // the escalation's seat
		want     bool
		barFrom  string
	}{
		{name: "a whole read that returns fix", verdict: fix, status: StatusEscalated, seat: "blueprint", want: true},
		{name: "a whole read that accepts", verdict: okText(readText("ok")), status: StatusAccepted},
		{name: "a whole fix escalated on a consistency conflict", verdict: fix, conflict: true, status: StatusEscalated, seat: "consistency", want: true},
		{name: "a change read that returns fix", verdict: fix, status: StatusEscalated, seat: "blueprint", set: func(o *Options) {
			o.Since = barCommit
			o.Scopes = map[string]blueprint.Scope{pageA: scopedFacts([]string{"rotate"})}
			o.Changes = map[string]string{pageA: changeBlockA}
		}},
		{name: "a bar read that returns fix", verdict: fix, status: StatusEscalated, seat: "blueprint", barFrom: barCommit, set: func(o *Options) {
			o.Scopes = map[string]blueprint.Scope{pageA: scopedFacts([]string{"rotate"})}
			o.Changes = map[string]string{pageA: changeBlockA}
			o.Bars, o.BarFrom = map[string]string{pageA: barText}, map[string]string{pageA: barCommit}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newBlueprintHarness(t, withPages("a"))
			h.api.script("s1", "blueprint", tt.verdict)
			if tt.conflict {
				h.api.script("s1", "consistency", okText(`{"conflicts":[{"facts":["lumen-key-ttl"],"with":"lumen-grace","conflict":"they disagree"}]}`))
			}
			o := h.options()
			if tt.set != nil {
				tt.set(&o)
			}
			s, err := Run(context.Background(), o)
			if err != nil {
				t.Fatal(err)
			}
			got := statusPage(s, pageA)
			if got.Status != tt.status || got.WholeBlocked != tt.want || got.BarFrom != tt.barFrom {
				t.Errorf("status %s, whole_blocked %v, bar_from %q; want %s, %v, %q", got.Status, got.WholeBlocked, got.BarFrom, tt.status, tt.want, tt.barFrom)
			}
			if tt.seat != "" && h.record(s.RunID, pageA).Escalation.Seat != tt.seat {
				t.Errorf("escalated on %s, want %s", h.record(s.RunID, pageA).Escalation.Seat, tt.seat)
			}
			if tt.want && (len(got.Findings) != 1 || !got.Findings[0].Blocking || got.Findings[0].Location != "rotate") {
				t.Errorf("findings = %+v, want the read's blocking finding", got.Findings)
			}
			var w work
			if err := json.Unmarshal(h.state(s.RunID).Pages[pageKey(pageA)].Work, &w); err != nil {
				t.Fatal(err)
			}
			if w.WholeBlocked != tt.want {
				t.Errorf("the saved work's whole_blocked = %v, want %v", w.WholeBlocked, tt.want)
			}
		})
	}
}

// TestBarReadSendsTheBarAndItsChange checks the blueprint request of a bar read: the seat gets the
// bar as its findings and the request the change since the bar's commit, and the page's since is
// that commit.
func TestBarReadSendsTheBarAndItsChange(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"), withFindingsModule)
	o := h.options()
	o.Scopes = map[string]blueprint.Scope{pageA: scopedFacts([]string{"rotate"})}
	o.Changes = map[string]string{pageA: changeBlockA}
	o.Bars, o.BarFrom = map[string]string{pageA: barText}, map[string]string{pageA: barCommit}
	s, err := Run(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	prompt, extra := blueprintPromptOf(t, h)
	if !strings.Contains(prompt, "Findings:\n"+barText) || !strings.Contains(prompt, "Scope: rotate") {
		t.Errorf("the prompt lacks the bar or the scope:\n%s", prompt)
	}
	if !strings.Contains(extra, "These sections changed: rotate.\n\n"+changeBlockA) {
		t.Errorf("the request lacks the change since the bar's commit:\n%s", extra)
	}
	var w work
	if err := json.Unmarshal(h.state(s.RunID).Pages[pageKey(pageA)].Work, &w); err != nil {
		t.Fatal(err)
	}
	if w.Since != barCommit || w.Bar != barText || w.BarFrom != barCommit {
		t.Errorf("work since %q, bar %q, bar_from %q; want the bar's commit and text", w.Since, w.Bar, w.BarFrom)
	}
	if got := statusPage(s, pageA); got.Status != StatusAccepted || got.BarFrom != barCommit || got.ReadScope != nil {
		t.Errorf("result = %+v, want accepted with bar_from and no read scope of its own", got)
	}
}

// TestReadScopeIsKeptApartFromTheDraftScope covers drafter page's read fields: the blueprint read
// judges ReadScopes and ReadChanges and never the draft's Scope and Change, and a whole read scope
// beside a scoped draft sends no change block.
func TestReadScopeIsKeptApartFromTheDraftScope(t *testing.T) {
	const draftChange = "=== blueprint diff ===\n-draft side\n+draft side, changed\n=== end blueprint diff ==="
	tests := []struct {
		name       string
		draft      blueprint.Scope
		read       blueprint.Scope
		readChange string
		bar        bool
		wantScope  string // the scope line the prompt carries, or "" for none
		wantExtra  string // what the request's change block holds, or "" for none
	}{
		{"a scoped bar read beside a whole draft", blueprint.Scope{Kind: blueprint.ScopeWhole, Why: "no drafted entry"}, scopedFacts([]string{"rotate"}), changeBlockA, true,
			"Scope: rotate", "These sections changed: rotate.\n\n" + changeBlockA},
		{"a whole read beside a scoped draft", scopedFacts([]string{"verify"}, "lumen-key-ttl"), blueprint.Scope{Kind: blueprint.ScopeWhole, Why: "the bar's commit is gone"}, "", false,
			"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newBlueprintHarness(t, withPages("a"), withFindingsModule)
			o := h.options()
			o.Scopes = map[string]blueprint.Scope{pageA: tt.draft}
			o.Changes = map[string]string{pageA: draftChange}
			o.ReadScopes = map[string]blueprint.Scope{pageA: tt.read}
			o.ReadChanges = map[string]string{pageA: tt.readChange}
			if tt.bar {
				o.Bars, o.BarFrom = map[string]string{pageA: barText}, map[string]string{pageA: barCommit}
			}
			s, err := Run(context.Background(), o)
			if err != nil {
				t.Fatal(err)
			}
			prompt, extra := blueprintPromptOf(t, h)
			if strings.Contains(extra, "draft side") {
				t.Errorf("the read sent the draft's change block:\n%s", extra)
			}
			if tt.wantScope == "" && strings.Contains(prompt, "Scope:") || tt.wantScope != "" && !strings.Contains(prompt, tt.wantScope) {
				t.Errorf("prompt = %q, want scope line %q", prompt, tt.wantScope)
			}
			if tt.wantExtra == "" && (strings.Contains(extra, "These sections changed") || strings.Contains(extra, "No section's own content changed")) || tt.wantExtra != "" && !strings.Contains(extra, tt.wantExtra) {
				t.Errorf("request text = %q, want change %q", extra, tt.wantExtra)
			}
			if strings.Contains(prompt, "Findings:") != tt.bar {
				t.Errorf("prompt carries findings = %v, want %v", !tt.bar, tt.bar)
			}
			got := statusPage(s, pageA)
			if got.Scope == nil || got.Scope.Kind != tt.draft.Kind || got.ReadScope == nil || got.ReadScope.Kind != tt.read.Kind {
				t.Errorf("scope %+v, read scope %+v; want the draft's and the read's kept apart", got.Scope, got.ReadScope)
			}
		})
	}
}

// TestBarReadResumesWithTheSameBytes stops a bar read before its batch is created and resumes it
// with options that carry no bar: the resume reads the bar and its change from state.
func TestBarReadResumesWithTheSameBytes(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"), withFindingsModule)
	h.api.crash = func(call string, ids []string) string {
		if call == "create" && h.api.creates == 1 {
			return "before"
		}
		return ""
	}
	o := h.options()
	o.Scopes = map[string]blueprint.Scope{pageA: scopedFacts([]string{"rotate"})}
	o.Changes = map[string]string{pageA: changeBlockA}
	o.Bars, o.BarFrom = map[string]string{pageA: barText}, map[string]string{pageA: barCommit}
	if !dies(func() { _, _ = Run(context.Background(), o) }) {
		t.Fatal("the run did not die before its batch")
	}
	h.api.crash = nil
	s, err := h.resume(h.runID())
	if err != nil {
		t.Fatalf("resume: %v\nstderr: %s", err, h.err.String())
	}
	prompt, extra := blueprintPromptOf(t, h)
	if !strings.Contains(prompt, "Findings:\n"+barText) || !strings.Contains(extra, changeBlockA) {
		t.Errorf("the resumed request lost the bar or its change:\nprompt %s\ntext %s", prompt, extra)
	}
	if got := statusPage(s, pageA); got.BarFrom != barCommit {
		t.Errorf("bar_from = %q after the resume, want %q", got.BarFrom, barCommit)
	}
}

// TestStateWithoutBarFieldsDecodes decodes a page's working state as B3's binary saved it, with
// no bar or read fields: the read judges the draft's scope and change, as before.
func TestStateWithoutBarFieldsDecodes(t *testing.T) {
	const b3 = `{"cov":{"phase":""},"reader":{},"gates":0,"rec":{},"since":"abc","scope":{"kind":"scoped","ids":["rotate"],"facts":[]},"change":"the change","draft":true}`
	var w work
	if err := json.Unmarshal([]byte(b3), &w); err != nil {
		t.Fatal(err)
	}
	if w.WholeBlocked || w.Bar != "" || w.BarFrom != "" || w.ReadScope != nil || w.ReadChange != "" {
		t.Errorf("work = %+v, want no bar or read fields", w)
	}
	if sc := w.readScope(); sc == nil || sc.Kind != blueprint.ScopeScoped || w.readChange() != "the change" {
		t.Errorf("read scope %+v, read change %q; want the draft's", sc, w.readChange())
	}
	raw, err := w.marshal()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"whole_blocked", `"bar`, "read_scope", "read_change"} {
		if strings.Contains(string(raw), key) {
			t.Errorf("re-encoded state holds %s: %s", key, raw)
		}
	}
}
