package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/blueprint"
)

const (
	// barFinding is the blocking finding the stub's blocking read gives.
	barFinding = "Say what the reader has once the items are counted."
	barRead    = `{"verdict":"fix","findings":[` +
		`{"location":"add-items","finding":"` + barFinding + `","blocking":true,"rewrite":"Name the saved total."},` +
		`{"location":"see-also","finding":"Nit: the link text could be shorter.","blocking":false,"rewrite":""}` +
		`],"summary":"one blocking finding","friction":[]}`
	okRead  = `{"verdict":"ok","findings":[],"summary":"read","friction":[]}`
	whyOld  = "Adding items is the page's job, so it follows the setup."
	whyNew  = "Adding items is the page's job, so it follows the setup and comes before the total."
	barLine = "1. [add-items] " + barFinding
)

// barSummary is the part of a read or page --json summary the bar rows read.
type barSummary struct {
	Pages []struct {
		Path         string           `json:"path"`
		Status       string           `json:"status"`
		Accepted     string           `json:"accepted"`
		WholeBlocked bool             `json:"whole_blocked"`
		BarFrom      string           `json:"bar_from"`
		Scope        *blueprint.Scope `json:"scope"`
		ReadScope    *blueprint.Scope `json:"read_scope"`
	} `json:"pages"`
}

func (e *draftEnv) barSummary(r result) barSummary {
	e.t.Helper()
	var s barSummary
	if err := json.Unmarshal([]byte(r.stdout), &s); err != nil || len(s.Pages) != 1 {
		e.t.Fatalf("no one-page JSON summary (exit %d): %v\nstdout: %s\nstderr: %s", r.code, err, r.stdout, r.stderr)
	}
	return s
}

// readBP runs drafter read on the fixture's blueprint in sync mode with JSON output.
func (e *draftEnv) readBP() result {
	e.t.Helper()
	return e.drafter("read", "--sync", "--json", draftBP)
}

// fixWhy commits the blueprint edit that answers the bar: a section's why, which no drafted
// sentence carries, so the canned page still covers the blueprint.
func (e *draftEnv) fixWhy() {
	e.t.Helper()
	bp := e.read(draftBP)
	if !strings.Contains(bp, whyOld) {
		e.t.Fatal("the fixture blueprint lacks the why the fix edits")
	}
	e.write(draftBP, strings.Replace(bp, whyOld, whyNew, 1))
	e.commit("answer the blocked read", draftBP)
}

// blueprintReadBody is the body of the first read request the stub got, the blueprint read's.
func (e *draftEnv) blueprintReadBody() string {
	e.t.Helper()
	kinds, bodies := e.stub.sent(), e.stub.sentBodies()
	if i := slices.Index(kinds, "read"); i >= 0 {
		return bodies[i]
	}
	e.t.Fatalf("no read request among %v", kinds)
	return ""
}

// TestReadBarMechanismOffline proves the read bar end to end with the built binary and a stub:
// a whole read that blocks writes the bar, the next read after a fix judges the change against
// it and clears it, --whole drafts the page whole, and drafter page writes and reads the bar too.
func TestReadBarMechanismOffline(t *testing.T) {
	e := newDraftEnv(t)

	// 1. A whole read that blocks writes the bar at the run's start commit.
	start := e.git("rev-parse", "HEAD")
	e.stub.setAnswer("read", barRead)
	r := e.readBP()
	s := e.barSummary(r)
	if r.code != 1 || !s.Pages[0].WholeBlocked || s.Pages[0].Scope != nil {
		t.Fatalf("step 1: exit %d, page %+v, want 1 and a whole read that blocked\n%s", r.code, s.Pages[0], r.stderr)
	}
	want := &blueprint.Blocked{Commit: start, Findings: []blueprint.BlockedFinding{{Location: "add-items", Text: barFinding}}}
	if got := e.lockEntry(); got.Blocked == nil || got.Blocked.Commit != want.Commit || !slices.Equal(got.Blocked.Findings, want.Findings) || got.Accepted != "" {
		t.Fatalf("step 1: entry = %+v, want only the bar %+v", got, want)
	}
	e.commit("the blocked read's bar", draftLock)

	// 2. After a fix, the read judges the change since the bar against its findings and accepts.
	e.fixWhy()
	e.stub.reset(0)
	e.stub.setAnswer("read", okRead)
	r = e.readBP()
	s = e.barSummary(r)
	if r.code != 0 || s.Pages[0].Status != "accepted" || s.Pages[0].BarFrom != start || s.Pages[0].Scope == nil || s.Pages[0].Scope.Kind != blueprint.ScopeScoped {
		t.Fatalf("step 2: exit %d, page %+v, want accepted on a scoped read from %s\n%s", r.code, s.Pages[0], start, r.stderr)
	}
	body := requestTextOf(e.blueprintReadBody())
	for _, w := range []string{"Earlier findings.", "The earlier findings:\n" + barLine, "=== blueprint diff ===", "+      " + whyNew} {
		if !strings.Contains(body, w) {
			t.Errorf("step 2: the blueprint read lacks %q", w)
		}
	}
	blob := e.git("rev-parse", "HEAD:"+draftBP)
	if got := e.lockEntry(); got.Accepted != blob || got.Blocked != nil {
		t.Fatalf("step 2: entry = %+v, want accepted %s and no bar", got, blob)
	}
	e.commit("the accepted read", draftLock)

	// 3. --whole skips the accepted read and drafts the page whole.
	e.stub.reset(0)
	r = e.page("--whole", draftPage)
	if r.code != 0 {
		t.Fatalf("step 3: exit %d sent %v\n%s%s", r.code, e.stub.sent(), r.stdout, r.stderr)
	}
	kind, why, _ := acceptedPage(t, e.summary(r))
	if kind != blueprint.ScopeWhole || why != "drafted whole on request (--whole)" {
		t.Errorf("step 3: scope %s %q, want whole naming the flag", kind, why)
	}
	if sent := e.stub.sent(); len(sent) == 0 || sent[0] != "draft" || slices.Contains(sent, "consistency") {
		t.Errorf("step 3: requests %v, want the draft first and no stage-1 read", sent)
	}
	if got := e.lockEntry(); got.Page == "" || got.Accepted != blob || got.Blocked != nil {
		t.Errorf("step 3: entry = %+v, want the drafted page, accepted %s", got, blob)
	}

	// 4. The page-mode path: drafter page blocks and writes the bar, then reads with it after a
	// fix, accepts, drafts, and writes the entry with accepted.
	p := newDraftEnv(t)
	pstart := p.git("rev-parse", "HEAD")
	p.stub.setAnswer("read", barRead)
	r = p.page(draftPage)
	if r.code != 1 || !p.barSummary(r).Pages[0].WholeBlocked {
		t.Fatalf("step 4: exit %d, want 1 on a whole read that blocked\n%s%s", r.code, r.stdout, r.stderr)
	}
	if got := p.lockEntry(); got.Blocked == nil || got.Blocked.Commit != pstart || got.Page != "" {
		t.Fatalf("step 4: entry = %+v, want a bar-only entry at %s", got, pstart)
	}
	if _, err := os.Stat(filepath.Join(p.dir, draftPage)); err == nil {
		t.Error("step 4: a blocked read drafted the page")
	}
	p.commit("the blocked read's bar", draftLock)
	p.fixWhy()
	p.stub.reset(0)
	p.stub.setAnswer("read", okRead)
	r = p.page(draftPage)
	if r.code != 0 {
		t.Fatalf("step 4: bar read exit %d sent %v\n%s%s", r.code, p.stub.sent(), r.stdout, r.stderr)
	}
	ps := p.barSummary(r).Pages[0]
	if ps.BarFrom != pstart || ps.ReadScope == nil || ps.ReadScope.Kind != blueprint.ScopeScoped || ps.Scope == nil || ps.Scope.Kind != blueprint.ScopeWhole {
		t.Errorf("step 4: page %+v, want a scoped bar read beside a whole draft", ps)
	}
	if body := requestTextOf(p.blueprintReadBody()); !strings.Contains(body, "The earlier findings:\n"+barLine) {
		t.Error("step 4: the blueprint read lacks the bar")
	}
	pblob := p.git("rev-parse", "HEAD:"+draftBP)
	if got := p.lockEntry(); got.Page != p.git("hash-object", draftPage) || got.Accepted != pblob || got.Blocked != nil {
		t.Errorf("step 4: entry = %+v, want the drafted page, accepted %s, no bar", got, pblob)
	}
}

// TestPageBarReadBesideASectionRedraft runs a bar page whose draft scope is a section redraft: the
// read judges the change since the bar's commit, with the bar, while the draft rewrites only the
// changed section.
func TestPageBarReadBesideASectionRedraft(t *testing.T) {
	e := newDraftEnv(t)
	e.firstDraft()
	bar := e.git("rev-parse", "HEAD")
	l, err := blueprint.ReadLock(filepath.Join(e.dir, draftLock))
	if err != nil {
		t.Fatal(err)
	}
	entry := l.Pages[draftPage]
	entry.Blocked = &blueprint.Blocked{Commit: bar, Findings: []blueprint.BlockedFinding{{Location: "add-items", Text: barFinding}}}
	l.Pages[draftPage] = entry
	if err := blueprint.WriteLock(filepath.Join(e.dir, draftLock), l); err != nil {
		t.Fatal(err)
	}
	e.commit("a bar from a later blocked read", draftLock)
	e.write(draftBP, strings.Replace(e.read(draftBP), draftOld, draftNew, 1))
	e.commit("edit one sentence item", draftBP)
	e.stub.reset(0)
	e.stub.setAnswer("coverage", strings.Replace(mustRead(t, filepath.Join("testdata", "cli-answers", "coverage.json")), draftOld, draftNew, 1))
	r := e.page(draftPage)
	if r.code != 0 {
		t.Fatalf("exit %d sent %v\n%s%s", r.code, e.stub.sent(), r.stdout, r.stderr)
	}
	ps := e.barSummary(r).Pages[0]
	if ps.Scope == nil || ps.Scope.Kind != blueprint.ScopeScoped || !slices.Equal(ps.Scope.IDs, []string{"add-items"}) {
		t.Errorf("draft scope = %+v, want a section redraft of add-items", ps.Scope)
	}
	if ps.BarFrom != bar || ps.ReadScope == nil || ps.ReadScope.Kind != blueprint.ScopeScoped {
		t.Errorf("bar_from %q, read scope %+v; want the bar read from %s", ps.BarFrom, ps.ReadScope, bar)
	}
	if body := requestTextOf(e.blueprintReadBody()); !strings.Contains(body, "The earlier findings:\n"+barLine) || !strings.Contains(body, "+          "+draftNew) {
		t.Error("the blueprint read lacks the bar or the change since its commit")
	}
	if sent := e.stub.sent(); slices.Contains(sent, "draft") || !slices.Contains(sent, "edits") {
		t.Errorf("requests %v, want a section redraft through edits and no whole draft", sent)
	}
	if got := e.lockEntry(); got.Accepted != e.git("rev-parse", "HEAD:"+draftBP) || got.Blocked != nil {
		t.Errorf("entry = %+v, want accepted at HEAD and no bar", got)
	}
}

// requestTextOf is the text of every string in a request body, which is how the model reads it.
func requestTextOf(body string) string {
	var v any
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		return body
	}
	var b strings.Builder
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			b.WriteString(x)
			b.WriteByte('\n')
		case []any:
			for _, e := range x {
				walk(e)
			}
		case map[string]any:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(v)
	return b.String()
}
