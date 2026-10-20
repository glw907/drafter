package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/blueprint"
	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/pipeline"
	runstate "github.com/glw907/drafter/internal/run"
)

const barStart = "1a2b3c4d5e6f1a2b3c4d5e6f1a2b3c4d5e6f1a2b"

func scopeOf(kind string) *blueprint.Scope { return &blueprint.Scope{Kind: kind} }

func TestRecordReads(t *testing.T) {
	findings := []pipeline.FindingResult{
		{Location: "steps", Text: "A step is missing.", Blocking: true},
		{Location: "intro", Text: "Nit: tighten the opening.", Blocking: false},
		{Location: "outro", Text: "Say what comes next.", Blocking: true},
	}
	wantBar := &blueprint.Blocked{Commit: barStart, Findings: []blueprint.BlockedFinding{{Location: "steps", Text: "A step is missing."}, {Location: "outro", Text: "Say what comes next."}}}
	oldBar := &blueprint.Blocked{Commit: "0000000000000000000000000000000000000000", Findings: []blueprint.BlockedFinding{{Location: "steps", Text: "An older finding."}}}
	drafted := blueprint.LockEntry{Page: "p1", Blueprint: "b1", Accepted: "b0", Sections: map[string]string{"_intro": "h"}}
	withBar := func(e blueprint.LockEntry, b *blueprint.Blocked) blueprint.LockEntry { e.Blocked = b; return e }

	tests := []struct {
		name    string
		prior   *blueprint.LockEntry // nil: the page has no entry
		result  pipeline.PageResult
		changed bool
		want    *blueprint.LockEntry // nil: the page still has no entry
		base    string               // the blueprint's blob at a scoped read's base
		kept    bool                 // a scoped accept left the entry and gave a reason
	}{
		{"a whole read that blocks gives a page with no entry a bar-only entry, its blocking findings in order",
			nil, pipeline.PageResult{Status: pipeline.StatusEscalated, WholeBlocked: true, Findings: findings}, true, &blueprint.LockEntry{Blocked: wantBar}, "", false},
		{"a whole read that blocks keeps a drafted entry's other fields",
			&drafted, pipeline.PageResult{Status: pipeline.StatusEscalated, WholeBlocked: true, Findings: findings}, true, new(withBar(drafted, wantBar)), "", false},
		{"a whole read that blocks replaces an older bar",
			new(withBar(drafted, oldBar)), pipeline.PageResult{Status: pipeline.StatusEscalated, WholeBlocked: true, Findings: findings}, true, new(withBar(drafted, wantBar)), "", false},
		{"a fix with no blocking finding records no bar",
			nil, pipeline.PageResult{Status: pipeline.StatusEscalated, WholeBlocked: true, Findings: findings[1:2]}, false, nil, "", false},
		{"a change read that blocks records no bar",
			nil, pipeline.PageResult{Status: pipeline.StatusEscalated, Scope: scopeOf(blueprint.ScopeScoped), Findings: findings}, false, nil, "", false},
		{"a bar read that blocks leaves the bar as it was",
			new(withBar(drafted, oldBar)), pipeline.PageResult{Status: pipeline.StatusEscalated, BarFrom: oldBar.Commit, Scope: scopeOf(blueprint.ScopeScoped), Findings: findings}, false, new(withBar(drafted, oldBar)), "", false},
		{"an unscoped accept writes accepted and clears the bar",
			new(withBar(drafted, oldBar)), pipeline.PageResult{Status: pipeline.StatusAccepted, Accepted: "b2"}, true, &blueprint.LockEntry{Page: "p1", Blueprint: "b1", Accepted: "b2", Sections: drafted.Sections}, "", false},
		{"an accept on a bar read writes accepted and clears the bar",
			new(withBar(drafted, oldBar)), pipeline.PageResult{Status: pipeline.StatusAccepted, Accepted: "b2", BarFrom: oldBar.Commit, Scope: scopeOf(blueprint.ScopeScoped)}, true, &blueprint.LockEntry{Page: "p1", Blueprint: "b1", Accepted: "b2", Sections: drafted.Sections}, "", false},
		{"a later accept of a bar-only entry leaves only accepted",
			&blueprint.LockEntry{Blocked: oldBar}, pipeline.PageResult{Status: pipeline.StatusAccepted, Accepted: "b2", BarFrom: oldBar.Commit, Scope: scopeOf(blueprint.ScopeScoped)}, true, &blueprint.LockEntry{Accepted: "b2"}, "", false},
		{"a scoped accept advances accepted and drops blocked when the entry accepted the blueprint at the scope's base",
			&drafted, pipeline.PageResult{Status: pipeline.StatusAccepted, Accepted: "b2", Scope: scopeOf(blueprint.ScopeScoped)}, true, &blueprint.LockEntry{Page: "p1", Blueprint: "b1", Accepted: "b2", Sections: drafted.Sections}, "b0", false},
		{"a scoped accept keeps the entry when its accepted is not the blueprint at the scope's base",
			&drafted, pipeline.PageResult{Status: pipeline.StatusAccepted, Accepted: "b2", Scope: scopeOf(blueprint.ScopeScoped)}, false, &drafted, "b1", true},
		{"a scoped accept keeps the entry when the base is unknown",
			&drafted, pipeline.PageResult{Status: pipeline.StatusAccepted, Accepted: "b2", Scope: scopeOf(blueprint.ScopeScoped)}, false, &drafted, "", true},
		{"a scoped accept keeps a page with no entry without one",
			nil, pipeline.PageResult{Status: pipeline.StatusAccepted, Accepted: "b2", Scope: scopeOf(blueprint.ScopeScoped)}, false, nil, "", true},
		{"a scoped accept against an entry with a bar keeps its bar",
			new(withBar(drafted, oldBar)), pipeline.PageResult{Status: pipeline.StatusAccepted, Accepted: "b2", Scope: scopeOf(blueprint.ScopeScoped)}, false, new(withBar(drafted, oldBar)), "b0", true},
		{"a scoped read beside a whole draft advances accepted under the same condition",
			&drafted, pipeline.PageResult{Status: pipeline.StatusAccepted, Accepted: "b2", Scope: scopeOf(blueprint.ScopeWhole), ReadScope: scopeOf(blueprint.ScopeScoped)}, true, &blueprint.LockEntry{Page: "p1", Blueprint: "b1", Accepted: "b2", Sections: drafted.Sections}, "b0", false},
		{"a read that fell back to whole beside a scoped draft writes accepted",
			&drafted, pipeline.PageResult{Status: pipeline.StatusAccepted, Accepted: "b2", Scope: scopeOf(blueprint.ScopeScoped), ReadScope: scopeOf(blueprint.ScopeWhole)}, true, &blueprint.LockEntry{Page: "p1", Blueprint: "b1", Accepted: "b2", Sections: drafted.Sections}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lock := &blueprint.Lock{Version: blueprint.Version}
			if tt.prior != nil {
				lock.Pages = map[string]blueprint.LockEntry{"docs/a.md": *tt.prior}
			}
			tt.result.Path = "docs/a.md"
			changed, kept := recordReads(lock, barStart, pipeline.Summary{Pages: []pipeline.PageResult{tt.result}}, map[string]string{"docs/a.md": tt.base})
			if changed != tt.changed {
				t.Errorf("changed = %v, want %v", changed, tt.changed)
			}
			if _, ok := kept["docs/a.md"]; ok != tt.kept {
				t.Errorf("kept = %v, want a reason %v", kept, tt.kept)
			}
			got, ok := lock.Pages["docs/a.md"]
			if tt.want == nil {
				if ok {
					t.Errorf("entry = %+v, want none", got)
				}
				return
			}
			if !reflect.DeepEqual(got, *tt.want) {
				t.Errorf("entry = %+v (bar %+v), want %+v (bar %+v)", got, got.Blocked, *tt.want, tt.want.Blocked)
			}
		})
	}
}

func TestRecordAcceptedWritesNoBarOnADryOrStoppedRun(t *testing.T) {
	blocked := []pipeline.PageResult{{Path: "docs/a.md", Status: pipeline.StatusEscalated, WholeBlocked: true,
		Findings: []pipeline.FindingResult{{Location: "steps", Text: "A step is missing.", Blocking: true}}}}
	tests := []struct {
		name string
		s    pipeline.Summary
		dry  bool
		want bool
	}{
		{"a dry run", pipeline.Summary{Pages: blocked}, true, false},
		{"a stopped run", pipeline.Summary{Stop: pipeline.StopSpendLimit, Pages: blocked}, false, false},
		{"a finished run", pipeline.Summary{Pages: blocked}, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, c := readProject(t)
			if _, err := recordAccepted(c, root, barStart, tt.s, nil, tt.dry); err != nil {
				t.Fatal(err)
			}
			got := strings.Contains(lockBytes(t, root), "commit: "+barStart)
			if got != tt.want {
				t.Errorf("bar written = %v, want %v\n%s", got, tt.want, lockBytes(t, root))
			}
		})
	}
}

func TestScopedAccept(t *testing.T) {
	tests := []struct {
		name string
		r    pipeline.PageResult
		want bool
	}{
		{"a scoped accept", pipeline.PageResult{Status: pipeline.StatusAccepted, Accepted: "b", Scope: scopeOf(blueprint.ScopeScoped)}, true},
		{"a whole accept", pipeline.PageResult{Status: pipeline.StatusAccepted, Accepted: "b", Scope: scopeOf(blueprint.ScopeWhole)}, false},
		{"an accept with no scope", pipeline.PageResult{Status: pipeline.StatusAccepted, Accepted: "b"}, false},
		{"a bar accept", pipeline.PageResult{Status: pipeline.StatusAccepted, Accepted: "b", Scope: scopeOf(blueprint.ScopeScoped), BarFrom: barStart}, false},
		{"a whole read beside a scoped draft", pipeline.PageResult{Status: pipeline.StatusAccepted, Accepted: "b", Scope: scopeOf(blueprint.ScopeScoped), ReadScope: scopeOf(blueprint.ScopeWhole)}, false},
		{"a scoped read beside a whole draft", pipeline.PageResult{Status: pipeline.StatusAccepted, Accepted: "b", Scope: scopeOf(blueprint.ScopeWhole), ReadScope: scopeOf(blueprint.ScopeScoped)}, true},
		{"an escalated page", pipeline.PageResult{Status: pipeline.StatusEscalated, Scope: scopeOf(blueprint.ScopeScoped)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scopedAccept(tt.r); got != tt.want {
				t.Errorf("scopedAccept = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRenderBarAndHeldPage(t *testing.T) {
	b := &blueprint.Blocked{Commit: "abcdef0123456789", Findings: []blueprint.BlockedFinding{{Location: "steps", Text: "A step is missing."}, {Location: "intro", Text: "Say what the page leaves out."}}}
	if got, want := renderBar(b), "1. [steps] A step is missing.\n2. [intro] Say what the page leaves out."; got != want {
		t.Errorf("renderBar = %q, want %q", got, want)
	}
	r := heldPage("docs/a.md", b)
	if r.Status != statusHeld || r.Scope.Why != "unchanged since the blocked read at abcdef0; fix its 2 findings and commit" {
		t.Errorf("held = %s %q", r.Status, r.Scope.Why)
	}
	if len(r.Findings) != 2 || !r.Findings[1].Blocking || r.Findings[1].Location != "intro" {
		t.Errorf("held findings = %+v, want the bar's, blocking", r.Findings)
	}
}

// barRead is the read answer of a blueprint read that blocks on two findings and passes a third.
const barRead = `{"verdict":"fix","findings":[` +
	`{"location":"two","finding":"Say why the reader continues.","blocking":true,"rewrite":"Add a reason."},` +
	`{"location":"one","finding":"Nit: tighten the opening.","blocking":false,"rewrite":""},` +
	`{"location":"one","finding":"Name what the reader needs first.","blocking":true,"rewrite":"Add it."}` +
	`],"summary":"two blocking findings"}`

// readJSON is the part of drafter read --json the bar tests read.
type readJSON struct {
	RunID string `json:"run_id"`
	Pages []struct {
		Path         string
		Status       string
		WholeBlocked bool   `json:"whole_blocked"`
		BarFrom      string `json:"bar_from"`
		Accepted     string
		Scope        *blueprint.Scope
		Findings     []pipeline.FindingResult
	}
}

func (p *sinceProject) readJSON(args ...string) (int, readJSON, string) {
	p.t.Helper()
	code, out, errOut := p.read(append([]string{"--json"}, args...)...)
	var got readJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		p.t.Fatalf("exit %d, stdout is not JSON: %v\n%s\n%s", code, err, out, errOut)
	}
	return code, got, errOut
}

func (p *sinceProject) lock() *blueprint.Lock {
	p.t.Helper()
	l, err := blueprint.ReadLock(filepath.Join(p.root, "blueprints/drafted.lock"))
	if err != nil {
		p.t.Fatal(err)
	}
	return l
}

func (p *sinceProject) rev(r string) string {
	p.t.Helper()
	out, err := gitText(p.root, "rev-parse", r)
	if err != nil {
		p.t.Fatal(err)
	}
	return out
}

func (p *sinceProject) setAnswer(answer string) {
	p.stub.mu.Lock()
	defer p.stub.mu.Unlock()
	p.stub.answer, p.stub.bodies, p.stub.failFrom = answer, nil, 0
}

// blockA reads page a whole with the stub blocking it and commits the lock the read wrote. It
// returns the commit the bar records.
func (p *sinceProject) blockA() string {
	p.t.Helper()
	start := p.rev("HEAD")
	p.setAnswer(barRead)
	if code, out, errOut := p.read(p.bp("a")); code != 1 {
		p.t.Fatalf("blocking read: exit %d, want 1\n%s%s", code, out, errOut)
	}
	p.git("add", "blueprints/drafted.lock")
	p.git("commit", "-q", "-m", "the blocked read's bar")
	p.setAnswer("")
	return start
}

func (p *sinceProject) writeLock(l *blueprint.Lock) {
	p.t.Helper()
	if err := blueprint.WriteLock(filepath.Join(p.root, "blueprints/drafted.lock"), l); err != nil {
		p.t.Fatal(err)
	}
	p.git("add", "blueprints/drafted.lock")
	p.git("commit", "-q", "-m", "write the lock")
}

func TestReadWritesTheBarOnAWholeBlock(t *testing.T) {
	p := newSinceProject(t)
	start := p.rev("HEAD")
	p.setAnswer(barRead)
	code, got, errOut := p.readJSON(p.bp("a"))
	if code != 1 || len(got.Pages) != 1 || !got.Pages[0].WholeBlocked || got.Pages[0].Scope != nil {
		t.Fatalf("exit %d, pages %+v, want 1 with a whole read that blocked\n%s", code, got.Pages, errOut)
	}
	want := &blueprint.Blocked{Commit: start, Findings: []blueprint.BlockedFinding{
		{Location: "two", Text: "Say why the reader continues.", Seat: "blueprint"}, {Location: "one", Text: "Name what the reader needs first.", Seat: "blueprint"}}}
	e := p.lock().Pages["docs/a.md"]
	if !reflect.DeepEqual(e.Blocked, want) || e.Accepted != "" || e.Page != "" {
		t.Errorf("entry = %+v (bar %+v), want a bar-only entry %+v", e, e.Blocked, want)
	}
}

func TestReadWithABar(t *testing.T) {
	t.Run("the bar read's since is the bar's commit, its prompt holds the bar, and an accept clears it", func(t *testing.T) {
		p := newSinceProject(t)
		bar := p.blockA()
		p.commit(editItem)
		code, got, errOut := p.readJSON(p.bp("a"))
		if code != 0 || got.Pages[0].Status != "accepted" || got.Pages[0].BarFrom != bar || got.Pages[0].Scope == nil || got.Pages[0].Scope.Kind != blueprint.ScopeScoped {
			t.Fatalf("exit %d, page %+v, want accepted on a scoped read from %s\n%s", code, got.Pages, bar, errOut)
		}
		reqs := p.stub.requests()
		if len(reqs) != 1 {
			t.Fatalf("%d requests, want 1", len(reqs))
		}
		text := requestText(reqs[0])
		for _, want := range []string{"Findings:\n1. [two] Say why the reader continues.\n2. [one] Name what the reader needs first.", "Scope: two", "=== blueprint diff ===", "+          Continue, carefully."} {
			if !strings.Contains(text, want) {
				t.Errorf("the request lacks %q\n%s", want, text)
			}
		}
		if strings.Contains(errOut, "accepted on a scoped read") {
			t.Errorf("a bar accept was reported as a scoped accept: %s", errOut)
		}
		e := p.lock().Pages["docs/a.md"]
		if e.Accepted != p.rev("HEAD:blueprints/docs/a.blueprint.yaml") || e.Blocked != nil {
			t.Errorf("entry = %+v, want accepted at HEAD and no bar", e)
		}
		st := readState(t, p, got.RunID)
		if st.Since != bar {
			t.Errorf("the page's since = %q, want the bar's commit %s", st.Since, bar)
		}
	})

	t.Run("a bar read that blocks leaves the bar", func(t *testing.T) {
		p := newSinceProject(t)
		bar := p.blockA()
		before := p.lock().Pages["docs/a.md"].Blocked
		p.commit(editItem)
		p.setAnswer(`{"verdict":"fix","findings":[{"location":"two","finding":"Still open: say why the reader continues.","blocking":true,"rewrite":"x"}],"summary":"s"}`)
		code, got, _ := p.readJSON(p.bp("a"))
		if code != 1 || got.Pages[0].WholeBlocked || got.Pages[0].BarFrom != bar {
			t.Fatalf("exit %d, page %+v, want 1 on a bar read", code, got.Pages)
		}
		if after := p.lock().Pages["docs/a.md"].Blocked; !reflect.DeepEqual(after, before) {
			t.Errorf("bar = %+v, want it unchanged %+v", after, before)
		}
	})

	t.Run("an unchanged blueprint is held, sends no request, and exits 1", func(t *testing.T) {
		p := newSinceProject(t)
		bar := p.blockA()
		code, out, errOut := p.read(p.bp("a"), p.bp("b"))
		want := "docs/a.md: held: unchanged since the blocked read at " + bar[:7] + "; fix its 2 findings and commit\n  blocking: two: Say why the reader continues."
		if code != 1 || !strings.Contains(out, want) {
			t.Errorf("exit %d, stdout %q, want 1 and %q\n%s", code, out, want, errOut)
		}
		if reqs := p.stub.requests(); len(reqs) != 1 || strings.Contains(reqs[0], "Findings:") {
			t.Errorf("%d requests, want only page b's, with no bar", len(reqs))
		}
	})

	t.Run("a held page alone exits 1 with no request", func(t *testing.T) {
		p := newSinceProject(t)
		p.blockA()
		code, got, _ := p.readJSON(p.bp("a"))
		if code != 1 || got.Pages[0].Status != statusHeld || len(got.Pages[0].Findings) != 2 {
			t.Errorf("exit %d, page %+v, want 1 and held with the bar's findings", code, got.Pages)
		}
		if n := len(p.stub.requests()); n != 0 {
			t.Errorf("%d requests for a held page", n)
		}
	})

	t.Run("--since ignores the bar", func(t *testing.T) {
		p := newSinceProject(t)
		p.blockA()
		p.commit(editItem)
		code, got, _ := p.readJSON("--since", "HEAD~1", p.bp("a"))
		if code != 0 || got.Pages[0].BarFrom != "" {
			t.Fatalf("exit %d, page %+v, want an ordinary scoped accept", code, got.Pages)
		}
		if reqs := p.stub.requests(); len(reqs) != 1 || strings.Contains(reqs[0], "Findings:") {
			t.Errorf("the --since read sent the bar")
		}
		if p.lock().Pages["docs/a.md"].Blocked == nil {
			t.Error("a scoped accept under --since cleared the bar")
		}
	})

	for _, tt := range []struct {
		name  string
		setup func(p *sinceProject) string // returns the bar's commit
		line  string                       // the line stderr must hold, or ""
		why   string
	}{
		{"a commit not in the repository reads whole and says so", func(p *sinceProject) string {
			return "0123456789abcdef0123456789abcdef01234567"
		}, "docs/a.md: the blocked read's commit 0123456 is not in this repository; reading whole", "is not in this repository"},
		{"a blueprint absent at the bar's commit reads whole", func(p *sinceProject) string {
			p.git("rm", "-q", "blueprints/docs/a.blueprint.yaml")
			p.git("commit", "-q", "-m", "remove a")
			at := p.rev("HEAD")
			p.write("blueprints/docs/a.blueprint.yaml", sinceBlueprint)
			p.git("add", "blueprints/docs/a.blueprint.yaml")
			p.git("commit", "-q", "-m", "restore a")
			return at
		}, "", "absent or undecodable"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := newSinceProject(t)
			commit := tt.setup(p)
			p.writeLock(&blueprint.Lock{Version: blueprint.Version, Pages: map[string]blueprint.LockEntry{"docs/a.md": {
				Blocked: &blueprint.Blocked{Commit: commit, Findings: []blueprint.BlockedFinding{{Location: "two", Text: "Say why."}}}}}})
			code, got, errOut := p.readJSON(p.bp("a"))
			if code != 0 || got.Pages[0].BarFrom != "" || got.Pages[0].Scope == nil || got.Pages[0].Scope.Kind != blueprint.ScopeWhole || !strings.Contains(got.Pages[0].Scope.Why, tt.why) {
				t.Fatalf("exit %d, page %+v, want a whole accept naming %q\n%s", code, got.Pages, tt.why, errOut)
			}
			if tt.line != "" && !strings.Contains(errOut, tt.line) {
				t.Errorf("stderr %q lacks %q", errOut, tt.line)
			}
			if reqs := p.stub.requests(); len(reqs) != 1 || strings.Contains(reqs[0], "Findings:") || strings.Contains(reqs[0], "Scope:") {
				t.Errorf("the whole read sent a bar or a scope")
			}
			if e := p.lock().Pages["docs/a.md"]; e.Accepted == "" || e.Blocked != nil {
				t.Errorf("entry = %+v, want accepted and the bar cleared", e)
			}
		})
	}

	t.Run("two pages with bars from different commits each get their own since", func(t *testing.T) {
		p := newSinceProject(t)
		first := p.rev("HEAD")
		bpB := "blueprints/docs/b.blueprint.yaml"
		p.write(bpB, strings.Replace(sinceBlueprint, "Begin.", "Begin now.", 1))
		p.git("add", bpB)
		p.git("commit", "-q", "-m", "edit b before a's edit")
		p.commit(editItem)
		second := p.rev("HEAD")
		p.write(bpB, strings.Replace(strings.Replace(sinceBlueprint, "Begin.", "Begin now.", 1), "Continue.", "Continue, slowly.", 1))
		p.git("add", bpB)
		p.git("commit", "-q", "-m", "edit b after a's edit")
		bar := func(c string) *blueprint.Blocked {
			return &blueprint.Blocked{Commit: c, Findings: []blueprint.BlockedFinding{{Location: "two", Text: "Say why."}}}
		}
		p.writeLock(&blueprint.Lock{Version: blueprint.Version, Pages: map[string]blueprint.LockEntry{"docs/a.md": {Blocked: bar(first)}, "docs/b.md": {Blocked: bar(second)}}})
		code, got, errOut := p.readJSON(p.bp("a"), p.bp("b"))
		if code != 0 {
			t.Fatalf("exit %d\n%s", code, errOut)
		}
		from := map[string]string{}
		for _, pg := range got.Pages {
			from[pg.Path] = pg.BarFrom
		}
		if from["docs/a.md"] != first || from["docs/b.md"] != second {
			t.Errorf("bar_from = %v, want a from %s and b from %s", from, first, second)
		}
		var textB string
		for _, r := range p.stub.requests() {
			if strings.Contains(requestText(r), "b.blueprint.yaml") && strings.Contains(requestText(r), "=== blueprint diff ===") {
				textB = requestText(r)
			}
		}
		if !strings.Contains(textB, "+          Continue, slowly.") || strings.Contains(textB, "+          Begin now.") {
			t.Errorf("page b's change is not the change since its own bar's commit:\n%s", textB)
		}
	})
}

// readState returns the saved working state of page a in the run.
func readState(t *testing.T, p *sinceProject, runID string) struct{ Since string } {
	t.Helper()
	repo, err := runstate.Repo(p.root)
	if err != nil {
		t.Fatal(err)
	}
	st, err := runstate.LoadState(filepath.Join(p.state, "runs", repo, runID))
	if err != nil {
		t.Fatal(err)
	}
	var w struct{ Since string }
	if err := json.Unmarshal(st.Pages[runstate.PageKey("docs/a.md")].Work, &w); err != nil {
		t.Fatal(err)
	}
	return w
}

func TestReadFailedBarWriteIsWrittenByTheResume(t *testing.T) {
	p := newSinceProject(t)
	p.setAnswer(barRead)
	dir := filepath.Join(p.root, "blueprints")
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	code, out, errOut := p.read(p.bp("a"))
	if code != 2 || !strings.Contains(out, "docs/a.md: escalated (plan)") || !strings.Contains(out, "blocking: two: Say why the reader continues.") {
		t.Fatalf("exit %d, stdout %q, want 2 with the summary printed first\n%s", code, out, errOut)
	}
	i := strings.Index(errOut, "--resume ")
	if i < 0 || !strings.Contains(errOut, "drafted.lock") {
		t.Fatalf("stderr %q lacks the lock error and the resume line", errOut)
	}
	id := strings.Fields(errOut[i+len("--resume "):])[0]
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p.setAnswer("")
	code, out, errOut = p.read("--resume", id, p.bp("a"))
	if code != 1 {
		t.Fatalf("resume exit %d, want 1 for the escalated page\n%s%s", code, out, errOut)
	}
	if n := len(p.stub.requests()); n != 0 {
		t.Errorf("the resume of a finished run sent %d requests", n)
	}
	if e := p.lock().Pages["docs/a.md"]; e.Blocked == nil || len(e.Blocked.Findings) != 2 {
		t.Errorf("entry = %+v, want the bar the resume wrote", e)
	}
}

func TestReadRefusesALockItCannotDecode(t *testing.T) {
	p := newSinceProject(t)
	p.write("blueprints/drafted.lock", "version: 1\nfuture-field: x\n")
	p.git("add", "blueprints/drafted.lock")
	p.git("commit", "-q", "-m", "a lock from a later format")
	code, out, errOut := p.read(p.bp("a"))
	if code != 2 || !strings.Contains(errOut, "blueprints/drafted.lock") {
		t.Errorf("exit %d, want 2 naming the lock\n%s%s", code, out, errOut)
	}
	if n := len(p.stub.requests()); n != 0 {
		t.Errorf("%d requests before the refusal", n)
	}
}

// pageBar commits a lock entry for the tally page holding a bar at commit, keeping any entry
// the lock holds.
func (f *cliFixture) pageBar(commit string) {
	f.t.Helper()
	file := filepath.Join(f.root, tallyLock)
	l, err := blueprint.ReadLock(file)
	if err != nil {
		f.t.Fatal(err)
	}
	if l.Pages == nil {
		l.Pages = map[string]blueprint.LockEntry{}
	}
	e := l.Pages[tallyPage]
	e.Blocked = &blueprint.Blocked{Commit: commit, Findings: []blueprint.BlockedFinding{{Location: "add-items", Text: "Say what the reader needs."}}}
	l.Pages[tallyPage] = e
	if err := blueprint.WriteLock(file, l); err != nil {
		f.t.Fatal(err)
	}
	f.commit("the blocked read's bar", tallyLock)
}

func editTallySentence(f *cliFixture) {
	f.write(tallyBP, strings.Replace(f.read1(tallyBP), "Pick the items you want to count.", "Pick the items you want to count, and have them ready.", 1))
	f.commit("edit the sentence", tallyBP)
}

func TestPageWhole(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(f *cliFixture)
		without  string // the scope kind without --whole, "" when the page is up to date
		skipRead bool
	}{
		{"a converted page whose blueprint is unchanged drafts whole instead of up to date", func(f *cliFixture) { f.draft(tallyBriefJSON(t), "") }, "", false},
		{"a seeded page whose blueprint changed drafts whole instead of a section redraft", func(f *cliFixture) {
			f.draft(tallyBriefJSON(t), "")
			editTallySentence(f)
		}, blueprint.ScopeScoped, false},
		{"the read stays skipped when accepted matches", func(f *cliFixture) {
			f.draft(tallyBriefJSON(t), f.rev("HEAD:"+tallyBP))
		}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newCLIFixture(t)
			tt.setup(f)
			d := f.scope()
			if got := d.scopes[tallyPage].Kind; got != tt.without || (tt.without == "") != (len(d.skipped) == 1) {
				t.Fatalf("without --whole: scope %q, skipped %d; want %q", got, len(d.skipped), tt.without)
			}
			d, err := f.scopeWith(true, io.Discard)
			if err != nil {
				t.Fatal(err)
			}
			sc := d.scopes[tallyPage]
			if len(d.handoffs) != 1 || len(d.skipped) != 0 || sc.Kind != blueprint.ScopeWhole || sc.Why != wholeOnRequest || d.changes[tallyPage] != "" {
				t.Errorf("with --whole: handoffs %d, skipped %d, scope %+v; want the page drafted whole naming the flag", len(d.handoffs), len(d.skipped), sc)
			}
			if d.skipRead[tallyPage] != tt.skipRead {
				t.Errorf("skip read = %v, want %v", d.skipRead[tallyPage], tt.skipRead)
			}
		})
	}
}

func TestPageWholeUsage(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"with --resume", []string{"--whole", "--resume", "20260101-aaaaaaaa", tallyPage}, "--resume"},
		{"with a handoff file", []string{"--whole", "handoff.json"}, "handoff files"},
		{"with --workflow-run", []string{"--whole", "--workflow-run", "run.json"}, "--workflow-run"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newCLIFixture(t)
			code, out, errOut := f.page(tt.args...)
			if code != exitCannotRun || !strings.Contains(out+errOut, tt.want) || !strings.Contains(out+errOut, "--whole") {
				t.Errorf("exit %d, want %d naming --whole and %q\n%s%s", code, exitCannotRun, tt.want, out, errOut)
			}
			if n := len(f.stub.counts); n != 0 {
				t.Errorf("%d API calls for a usage error", n)
			}
		})
	}
}

func TestPageScopeWithABar(t *testing.T) {
	t.Run("a bar page reads the change since the bar while its draft keeps its own scope", func(t *testing.T) {
		f := newCLIFixture(t)
		f.draft(tallyBriefJSON(t), "")
		bar := f.rev("HEAD")
		f.pageBar(bar)
		editTallySentence(f)
		for _, whole := range []bool{false, true} {
			d, err := f.scopeWith(whole, io.Discard)
			if err != nil {
				t.Fatal(err)
			}
			draft, read := d.scopes[tallyPage], d.read.scopes[tallyPage]
			wantDraft := blueprint.ScopeScoped
			if whole {
				wantDraft = blueprint.ScopeWhole
			}
			if draft.Kind != wantDraft || read.Kind != blueprint.ScopeScoped || !slices.Equal(read.IDs, []string{"add-items"}) {
				t.Errorf("whole %v: draft scope %+v, read scope %+v; want the draft %s and the read scoped to add-items", whole, draft, read, wantDraft)
			}
			if d.read.barFrom[tallyPage] != bar || d.read.bars[tallyPage] != "1. [add-items] Say what the reader needs." || !strings.Contains(d.read.changes[tallyPage], "have them ready") {
				t.Errorf("whole %v: bar from %q, bar %q, change %q", whole, d.read.barFrom[tallyPage], d.read.bars[tallyPage], d.read.changes[tallyPage])
			}
		}
	})

	t.Run("a skipped read ignores the bar", func(t *testing.T) {
		f := newCLIFixture(t)
		f.draft(tallyBriefJSON(t), "")
		bar := f.rev("HEAD")
		editTallySentence(f)
		l, _ := blueprint.ReadLock(filepath.Join(f.root, tallyLock))
		e := l.Pages[tallyPage]
		e.Accepted = f.rev("HEAD:" + tallyBP)
		l.Pages[tallyPage] = e
		if err := blueprint.WriteLock(filepath.Join(f.root, tallyLock), l); err != nil {
			t.Fatal(err)
		}
		f.commit("accept by hand", tallyLock)
		f.pageBar(bar)
		d := f.scope()
		if !d.skipRead[tallyPage] || len(d.read.scopes) != 0 || len(d.read.bars) != 0 {
			t.Errorf("skip read %v, read scopes %v, bars %v; want the read skipped and no bar", d.skipRead[tallyPage], d.read.scopes, d.read.bars)
		}
	})

	t.Run("an unchanged blueprint is held: drafter page sends nothing and exits 1", func(t *testing.T) {
		f := newCLIFixture(t)
		f.pageBar(f.rev("HEAD"))
		t.Setenv("DRAFTER_API_KEY", "")
		code, out, errOut := f.page(tallyPage)
		if code != 1 || !strings.Contains(out, tallyPage+": held: unchanged since the blocked read at") {
			t.Errorf("exit %d, want 1 with the held line\n%s%s", code, out, errOut)
		}
		if n := len(f.stub.counts); n != 0 {
			t.Errorf("%d API calls for a held page", n)
		}
	})

	t.Run("a lock that does not decode is refused, naming the file", func(t *testing.T) {
		f := newCLIFixture(t)
		c, err := config.Load(f.root, config.Flags{})
		if err != nil {
			t.Fatal(err)
		}
		hs, err := blueprintHandoffs(c, f.root, f.rev("HEAD"), []string{f.blueprint()})
		if err != nil {
			t.Fatal(err)
		}
		f.write(tallyLock, "version: 1\nfuture-field: x\n")
		f.commit("a lock from a later format", tallyLock)
		head := f.rev("HEAD")
		for name, call := range map[string]func() error{
			"drafter page's scopeFromLock": func() error { _, err := scopeFromLock(c, f.root, head, hs, false, io.Discard); return err },
			"drafter read's bar lookup": func() error {
				_, _, err := readBars(c, f.root, head, hs, barMaps{}, io.Discard)
				return err
			},
		} {
			if err := call(); err == nil || !strings.Contains(err.Error(), "lock "+tallyLock+":") {
				t.Errorf("%s: err = %v, want one naming %s", name, err, tallyLock)
			}
		}
	})
}

func TestScopedRefusal(t *testing.T) {
	bar := &blueprint.Blocked{Commit: "c", Findings: []blueprint.BlockedFinding{{Location: "steps", Text: "x"}}}
	tests := []struct {
		name  string
		entry blueprint.LockEntry
		base  string
		want  string
	}{
		{"a bar the read did not judge", blueprint.LockEntry{Accepted: "b0", Blocked: bar}, "b0", "it holds a bar the scoped read did not judge"},
		{"no accepted blueprint yet", blueprint.LockEntry{}, "b0", "it has no accepted blueprint yet"},
		{"an accept that is not the base", blueprint.LockEntry{Accepted: "b1"}, "b0", "the blueprint it accepted is not the one at the scoped read's base"},
		{"an accept at the base", blueprint.LockEntry{Accepted: "b0"}, "b0", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scopedRefusal(tt.entry, tt.base); got != tt.want {
				t.Errorf("scopedRefusal = %q, want %q", got, tt.want)
			}
		})
	}
}
