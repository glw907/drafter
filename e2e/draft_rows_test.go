package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/blueprint"
	"github.com/glw907/drafter/internal/coverage"
	"github.com/glw907/drafter/internal/plan"
	runstate "github.com/glw907/drafter/internal/run"
)

// resumeLine finds the run id in the resume command a failed lock write prints.
var resumeLine = regexp.MustCompile(`--resume (\S+)`)

// draftedRecord is the part of a page's record.json the rows read.
type draftedRecord struct {
	Verdicts []struct{ Stage, Seat, Verdict string } `json:"verdicts"`
	Writes   []struct {
		N       int    `json:"n"`
		Step    string `json:"step"`
		Kind    string `json:"kind"`
		GateFix bool   `json:"gate_fix"`
	} `json:"writes"`
}

func (e *draftEnv) record(runID string) (draftedRecord, string) {
	e.t.Helper()
	dir := filepath.Join(e.state, "runs", filepath.Base(e.dir), runID, runstate.PageKey(draftPage))
	data, err := os.ReadFile(filepath.Join(dir, "record.json"))
	if err != nil {
		e.t.Fatal(err)
	}
	var rec draftedRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		e.t.Fatal(err)
	}
	return rec, dir
}

// sectionsOutside names the parts of the page, outside the section at edited, that differ.
func sectionsOutside(headings []string, edited int, before, after string) []string {
	preB, textB, _ := coverage.Regions(headings, []byte(before))
	preA, textA, _ := coverage.Regions(headings, []byte(after))
	var diff []string
	if preB != preA {
		diff = append(diff, "preamble")
	}
	for i := range headings {
		if i != edited && textB[i] != textA[i] {
			diff = append(diff, headings[i])
		}
	}
	return diff
}

// firstDraft runs row 1 and checks the lock entry and the gate. It commits the page, brief, and lock.
func (e *draftEnv) firstDraft() (firstPage string) {
	e.t.Helper()
	r := e.page(draftPage)
	if r.code != 0 {
		e.t.Fatalf("row 1: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	kind, why, _ := acceptedPage(e.t, e.summary(r))
	if kind != blueprint.ScopeWhole || !strings.Contains(why, "no drafted entry") {
		e.t.Fatalf("row 1 scope = %s %q, want whole naming the missing entry", kind, why)
	}
	blob := e.git("rev-parse", "HEAD:"+draftBP)
	b, _ := blueprint.Decode(draftBP, []byte(e.read(draftBP)))
	got := e.lockEntry()
	want := blueprint.SectionHashes(b, []byte(e.read(draftPage)))
	if got.Page != e.git("hash-object", draftPage) || got.Blueprint != blob || got.Accepted != blob {
		e.t.Fatalf("row 1 lock entry = %+v, want the page's blob and %s as blueprint and accepted", got, blob)
	}
	if len(got.Sections) != len(want) {
		e.t.Fatalf("row 1 lock sections = %v, want %v", got.Sections, want)
	}
	for k, v := range want {
		if got.Sections[k] != v {
			e.t.Fatalf("row 1 lock section %s = %s, want %s", k, got.Sections[k], v)
		}
	}
	if got.Facts != nil {
		e.t.Fatalf("row 1 lock entry holds facts %v, but a drafter page run never writes them", got.Facts)
	}
	e.gate()
	e.recordFacts()
	e.commit("row 1", draftPage, draftBrief, draftLock)
	return e.read(draftPage)
}

// recordFacts writes the lock entry's facts map, the way a drafter read that judged the facts does.
// A drafter page run's own read never writes it, and the next drafter page refuses a page that
// has none.
func (e *draftEnv) recordFacts() {
	e.t.Helper()
	b, _ := blueprint.Decode(draftBP, []byte(e.read(draftBP)))
	facts, err := plan.LoadFacts(filepath.Dir(filepath.Join(e.dir, draftFacts)))
	if err != nil {
		e.t.Fatal(err)
	}
	file := filepath.Join(e.dir, draftLock)
	l, err := blueprint.ReadLock(file)
	if err != nil {
		e.t.Fatal(err)
	}
	entry := l.Pages[draftPage]
	entry.Facts = blueprint.FactsHashes(blueprint.CitedFacts(b, facts), facts)
	l.Pages[draftPage] = entry
	if err := blueprint.WriteLock(file, l); err != nil {
		e.t.Fatal(err)
	}
}

func TestDraftFixtureDraftsRedraftsAndSettles(t *testing.T) {
	e := newDraftEnv(t)
	firstPage := e.firstDraft()

	// Row 2: edit one sentence item and redraft only its section.
	bp := e.read(draftBP)
	edited := strings.Replace(bp, draftOld, draftNew, 1)
	if edited == bp {
		t.Fatal("the fixture blueprint lacks the sentence row 2 edits")
	}
	e.write(draftBP, edited)
	e.commit("row 2: edit one sentence item", draftBP)
	e.stub.reset(0)
	e.stub.setAnswer("coverage", strings.Replace(mustRead(t, filepath.Join("testdata", "cli-answers", "coverage.json")), draftOld, draftNew, 1))
	r := e.page(draftPage)
	if r.code != 0 {
		t.Fatalf("row 2: exit %d sent %v\n%s%s", r.code, e.stub.sent(), r.stdout, r.stderr)
	}
	s := e.summary(r)
	kind, _, ids := acceptedPage(t, s)
	if kind != blueprint.ScopeScoped || !slices.Equal(ids, []string{"add-items"}) {
		t.Fatalf("row 2 scope = %s %v, want scoped to add-items", kind, ids)
	}
	rec, dir := e.record(s.RunID)
	coherence := false
	for _, v := range rec.Verdicts {
		coherence = coherence || (v.Seat == "coherence" && v.Stage == "s8")
	}
	if !coherence {
		t.Errorf("row 2: no coherence verdict on stage s8: %+v", rec.Verdicts)
	}
	b, _ := blueprint.Decode(draftBP, []byte(edited))
	var headings []string
	for _, sec := range b.Sections {
		headings = append(headings, sec.Heading)
	}
	before := firstPage
	for _, w := range rec.Writes {
		data, err := os.ReadFile(filepath.Join(dir, "write"+itoa(w.N)+".page.md"))
		if err != nil {
			t.Fatal(err)
		}
		if diff := sectionsOutside(headings, 1, before, string(data)); len(diff) > 0 {
			t.Errorf("row 2: write %d (%s %s, gate fix %v) changed %v outside Add items", w.N, w.Step, w.Kind, w.GateFix, diff)
		}
		before = string(data)
	}
	if len(rec.Writes) == 0 {
		t.Error("row 2: the write log is empty")
	}
	if !strings.Contains(e.read(draftPage), draftNew) {
		t.Error("row 2: the page lacks the edited sentence")
	}
	entry := e.lockEntry()
	if entry.Page != e.git("hash-object", draftPage) || entry.Blueprint != e.git("rev-parse", "HEAD:"+draftBP) {
		t.Errorf("row 2 lock entry = %+v, want the new page and blueprint blobs", entry)
	}
	e.gate()
	e.commit("row 2: redrafted", draftPage, draftBrief, draftLock)

	// Row 3: nothing changed, so nothing runs and nothing is spent.
	idsBefore := e.runDirs()
	e.stub.reset(0)
	r = e.page(draftPage)
	if r.code != 0 {
		t.Fatalf("row 3: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	s = e.summary(r)
	if s.RunID != "" || len(s.Pages) != 1 || s.Pages[0].Status != "unchanged" || s.Pages[0].Scope.Why != "up to date with drafted.lock" || s.Pages[0].Cost != 0 {
		t.Errorf("row 3 summary = %+v, want the page up to date at zero cost", s)
	}
	if n := len(e.stub.sent()); n != 0 {
		t.Errorf("row 3 sent %d requests", n)
	}
	if got := e.runDirs(); !slices.Equal(got, idsBefore) {
		t.Errorf("row 3 made a run: %v, was %v", got, idsBefore)
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestDraftFactEditSkipsTheBlueprintRead(t *testing.T) {
	e := newDraftEnv(t)
	e.firstDraft()
	facts := e.read(draftFacts)
	e.write(draftFacts, strings.Replace(facts, "in the current directory.", "in the directory where you run it.", 1))
	e.commit("edit the shared fact", draftFacts)
	e.recordFacts()
	e.commit("record the fact read", draftLock)
	e.stub.reset(0)
	e.stub.setAnswer("edits", mustRead(t, filepath.Join("testdata", "cli-answers", "edits-fact.json")))
	e.stub.setAnswer("coverage", strings.Replace(mustRead(t, filepath.Join("testdata", "cli-answers", "coverage.json")), "in the current directory.", "in the directory where you run it.", 1))
	r := e.page(draftPage)
	if r.code != 0 {
		t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	kind, _, ids := acceptedPage(t, e.summary(r))
	if kind != blueprint.ScopeScoped || !slices.Equal(ids, []string{"before-you-begin", "read-the-total"}) {
		t.Fatalf("scope = %s %v, want scoped to the two sections that cite the fact", kind, ids)
	}
	sent := e.stub.sent()
	if len(sent) == 0 || sent[0] != "edits" || slices.Contains(sent, "consistency") {
		t.Errorf("requests = %v, want the draft from the check with no stage-1 request", sent)
	}
	if !strings.Contains(e.read(draftPage), "in the directory where you run it.") {
		t.Error("the page lacks the corrected sentence")
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestDraftStoppedRunResumesAndWritesTheLock(t *testing.T) {
	e := newDraftEnv(t)
	startBlob := e.git("rev-parse", "HEAD:"+draftBP)
	e.stub.reset(6)
	r := e.page(draftPage)
	if r.code != 3 {
		t.Fatalf("stopped run: exit %d, want 3\n%s%s", r.code, r.stdout, r.stderr)
	}
	s := e.summary(r)
	if s.RunID == "" || s.Stop == "" {
		t.Fatalf("summary = %+v, want a stopped run with an id", s)
	}
	if _, err := os.Stat(filepath.Join(e.dir, draftLock)); err == nil {
		t.Fatal("a stopped run wrote drafted.lock")
	}
	e.stub.reset(0)
	r = e.page("--resume", s.RunID, draftPage)
	if r.code != 0 {
		t.Fatalf("resume: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	acceptedPage(t, e.summary(r))
	if got := e.lockEntry(); got.Blueprint != startBlob || got.Page != e.git("hash-object", draftPage) {
		t.Errorf("lock entry = %+v, want the start commit's blueprint blob %s", got, startBlob)
	}
}

func TestDraftFailedLockWriteIsWrittenByTheResume(t *testing.T) {
	e := newDraftEnv(t)
	// A read-only blueprint root lets the pages write below it but not the lock beside the outline.
	root := filepath.Join(e.dir, "blueprints")
	if err := os.Chmod(root, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })
	r := e.page(draftPage)
	if r.code != 2 {
		t.Fatalf("exit %d, want 2 for a lock that could not be written\n%s%s", r.code, r.stdout, r.stderr)
	}
	if !strings.Contains(r.stdout, `"status": "accepted"`) {
		t.Errorf("stdout lacks the summary:\n%s", r.stdout)
	}
	m := resumeLine.FindStringSubmatch(r.stderr)
	if m == nil {
		t.Fatalf("stderr lacks the resume command:\n%s", r.stderr)
	}
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	e.stub.reset(0)
	r = e.page("--resume", m[1], draftPage)
	if r.code != 0 {
		t.Fatalf("resume: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	if n := len(e.stub.sent()); n != 0 {
		t.Errorf("the resume of a finished run sent %d requests", n)
	}
	if got := e.lockEntry(); got.Page == "" || got.Blueprint != e.git("rev-parse", "HEAD:"+draftBP) {
		t.Errorf("lock entry = %+v, want the page and start blueprint", got)
	}
}
