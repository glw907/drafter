package e2e

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/blueprint"
)

// bodyOf is the prompt text of the first request of kind the stub got.
func (e *draftEnv) bodyOf(kind string) string {
	e.t.Helper()
	kinds, bodies := e.stub.sent(), e.stub.sentBodies()
	i := slices.Index(kinds, kind)
	if i < 0 {
		e.t.Fatalf("no %s request among %v", kind, kinds)
	}
	return requestTextOf(bodies[i])
}

// TestPageRunWritesNoFactsAndTheNextPageRefuses: the read stage inside drafter page accepts the
// blueprint and writes accepted, never facts, so the next drafter page refuses the entry.
func TestPageRunWritesNoFactsAndTheNextPageRefuses(t *testing.T) {
	e := newDraftEnv(t)
	r := e.page(draftPage)
	if r.code != 0 {
		t.Fatalf("first run: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	if got := e.lockEntry(); got.Accepted == "" || got.Facts != nil {
		t.Fatalf("lock entry = %+v, want accepted and no facts", got)
	}
	e.gate()
	e.commit("first draft", draftPage, draftBrief, draftLock)

	e.stub.reset(0)
	r = e.page(draftPage)
	want := draftPage + ": the accepted read lapsed; no facts recorded. Run drafter read " + draftBP + "."
	if r.code != 2 || !strings.Contains(r.stderr, want) {
		t.Errorf("second run: exit %d, want 2 with %q\n%s%s", r.code, want, r.stdout, r.stderr)
	}
	if n := len(e.stub.sent()); n != 0 {
		t.Errorf("second run sent %d requests", n)
	}
}

// TestLapseReadAndPage walks a fact edit through the lapse rule: drafter page refuses, drafter read
// judges only the changed fact and records the new facts beside the same accepted id, and then a
// read is whole again and drafter page no longer reports a lapse.
func TestLapseReadAndPage(t *testing.T) {
	e := newDraftEnv(t)
	e.firstDraft()
	before := e.lockEntry()
	e.write(draftFacts, strings.Replace(e.read(draftFacts), "in the current directory.", "in the directory where you run it.", 1))
	e.commit("edit the shared fact", draftFacts)

	e.stub.reset(0)
	r := e.page(draftPage)
	if r.code != 2 || !strings.Contains(r.stderr, draftPage+": the accepted read lapsed; facts changed since it: tally-state-file. Run drafter read "+draftBP+".") {
		t.Fatalf("page: exit %d, want 2 with the lapse line\n%s%s", r.code, r.stdout, r.stderr)
	}
	if n := len(e.stub.sent()); n != 0 {
		t.Fatalf("a refused page sent %d requests", n)
	}

	e.stub.reset(0)
	r = e.readBP()
	s := e.barSummary(r)
	if r.code != 0 || s.Pages[0].Status != "accepted" || s.Pages[0].Scope == nil || s.Pages[0].Scope.Kind != blueprint.ScopeScoped {
		t.Fatalf("lapse read: exit %d, page %+v, want accepted on a scoped read\n%s", r.code, s.Pages[0], r.stderr)
	}
	if sent := e.stub.sent(); !slices.Equal(sent, []string{"consistency", "read", "facttruth"}) {
		t.Fatalf("lapse read sent %v, want the three seats", sent)
	}
	if body := e.bodyOf("facttruth"); !strings.Contains(body, "tally-state-file") || strings.Contains(body, "tally-count-cmd") {
		t.Errorf("the fact-truth request should name only the changed fact:\n%s", body)
	}
	if body := e.bodyOf("read"); !strings.Contains(body, "changed since the accepted read: ") {
		t.Errorf("the blueprint request lacks the changed-facts block:\n%s", body)
	}
	got := e.lockEntry()
	if got.Accepted != before.Accepted || len(got.Facts) != len(before.Facts) || maps.Equal(got.Facts, before.Facts) {
		t.Fatalf("lock entry facts %v accepted %s, want the same accepted id and facts that differ from %v", got.Facts, got.Accepted, before.Facts)
	}
	e.commit("the lapse read's accept", draftLock)

	e.stub.reset(0)
	r = e.readBP()
	s = e.barSummary(r)
	if r.code != 0 || s.Pages[0].Scope != nil {
		t.Errorf("read after the lapse accept: exit %d, scope %+v, want a whole read", r.code, s.Pages[0].Scope)
	}
	r = e.drafter("page", "--sync", "--dry-run", "--json", draftPage)
	if strings.Contains(r.stdout+r.stderr, "lapsed") {
		t.Errorf("page --dry-run after the lapse accept still reports a lapse:\n%s%s", r.stdout, r.stderr)
	}
}

func TestReadMode(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		env       []string
		code      int
		wantPath  string
		avoidPath string
	}{
		{"no flag runs sync", []string{"read", "--json", draftBP}, nil, 0, "/v1/messages", "/v1/messages/batches"},
		{"DRAFTER_MODE=batch with no flag still runs sync", []string{"read", "--json", draftBP}, []string{"DRAFTER_MODE=batch"}, 0, "/v1/messages", "/v1/messages/batches"},
		{"--sync runs sync", []string{"read", "--sync", "--json", draftBP}, nil, 0, "/v1/messages", "/v1/messages/batches"},
		{"--batch runs a batch", []string{"read", "--batch", "--json", draftBP}, nil, 1, "/v1/messages/batches", "/v1/messages"},
		{"--sync with --batch is a usage error", []string{"read", "--sync", "--batch", draftBP}, nil, 2, "", "/v1/messages"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newDraftEnv(t)
			r := runEnv(t, e.dir, append(slices.Clone(e.env), tt.env...), tt.args...)
			if r.code != tt.code {
				t.Errorf("exit %d, want %d\n%s%s", r.code, tt.code, r.stdout, r.stderr)
			}
			paths := e.stub.sentPaths()
			if tt.wantPath != "" && !slices.Contains(paths, tt.wantPath) {
				t.Errorf("the stub saw %v, want a POST to %s", paths, tt.wantPath)
			}
			if slices.Contains(paths, tt.avoidPath) {
				t.Errorf("the stub saw %v, want no POST to %s", paths, tt.avoidPath)
			}
		})
	}
}

// TestStoppedBatchReadPrintsAResumeLineThatResumes: a batch read that a cycle ceiling stops prints
// a resume command carrying --batch, and that command resumes the run instead of failing the
// mode check.
func TestStoppedBatchReadPrintsAResumeLineThatResumes(t *testing.T) {
	e := newDraftEnv(t)
	cfg := filepath.Join(strings.TrimPrefix(e.env[0], "XDG_CONFIG_HOME="), "drafter", "config.toml")
	data, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte(strings.Replace(string(data), "cycle_ceiling = 100", "cycle_ceiling = 0.0001", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	r := e.drafter("read", "--batch", "--json", draftBP)
	var s struct {
		Stop   string `json:"stop"`
		Resume string `json:"resume"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &s); err != nil || r.code != 3 || s.Stop == "" {
		t.Fatalf("exit %d, summary %q (%v), want a stopped run\n%s", r.code, r.stdout, err, r.stderr)
	}
	fields := strings.Fields(s.Resume)
	if len(fields) < 5 || fields[0] != "drafter" || fields[1] != "read" || fields[2] != "--batch" || fields[3] != "--resume" {
		t.Fatalf("resume line = %q, want drafter read --batch --resume <id> <blueprint>", s.Resume)
	}
	r = e.drafter(append(fields[1:3:3], append([]string{"--json"}, fields[3:]...)...)...)
	if r.code != 3 || strings.Contains(r.stderr, "mode") {
		t.Errorf("resume: exit %d, want the run to resume and stop again\n%s%s", r.code, r.stdout, r.stderr)
	}
}
