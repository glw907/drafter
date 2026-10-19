package pipeline

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/batch"
	"github.com/glw907/drafter/internal/blueprint"
)

// healthQuote is the goodPage sentence that carries the rotate blueprint's handoff item.
const healthQuote = "A passing health check shows only that the service is up, so read the publish log."

// The edits the drafting tests script against goodPage: one in the rotate section, which a section
// redraft of rotate may change, and one in "Before you begin", which it may not.
const (
	inOld     = "1. Create a new key."
	inNew     = "1. Create a new key in the console."
	stepTwo   = "2. Publish the site with the new key."
	stepNow   = "2. Publish the site with the new key now."
	stepThree = "3. Check which key signed the build."
	stepLog   = "3. Check which key signed the build log."
	outOld    = "See [key lifetime]"
	outNew    = "Read [key lifetime]"
	outLine   = `now reads "A key lives for 90 days. Read [key lifetime]`
	changeIn  = "=== synthetic change ===\nthe rotate section's items changed\n=== end synthetic change ==="
)

// withDrafting sets a blueprint harness up to draft: the echo module, a coherence seat, and a
// coverage answer that quotes the rotate blueprint's handoff item.
func withDrafting(h *harness) {
	h.t.Helper()
	module, err := filepath.Abs("testdata/draftmodule.js")
	if err != nil {
		h.t.Fatal(err)
	}
	h.cfg.Prompts = module
	h.cfg.Seat["coherence"] = h.cfg.Seat["structure"]
	h.api.answer = func(r batch.Request) string {
		if strings.Contains(string(r.Schema), `"quote"`) {
			return jsonText(map[string]any{"items": []map[string]string{{"id": "h-retry", "quote": healthQuote}}})
		}
		return ""
	}
}

// drafting is what a drafting run adds to the harness's options.
type drafting struct {
	scopes   map[string]blueprint.Scope
	changes  map[string]string
	skipRead map[string]bool
}

// sectionScope is a drafting run that scopes page A to the named sections of its blueprint.
func sectionScope(ids ...string) drafting {
	return drafting{
		scopes: map[string]blueprint.Scope{pageA: {
			Kind: blueprint.ScopeScoped, IDs: ids, Facts: []string{}, Bullets: []string{}, Joined: []string{},
		}},
		changes: map[string]string{pageA: changeIn},
	}
}

func (h *harness) runDrafting(d drafting) (Summary, error) {
	h.t.Helper()
	h.opts = h.options()
	h.opts.Draft, h.opts.Scopes, h.opts.Changes, h.opts.SkipRead = true, d.scopes, d.changes, d.skipRead
	if d.scopes != nil {
		h.opts.Since = h.head()
	}
	return Run(context.Background(), h.opts)
}

func (h *harness) mustDraft(d drafting) Summary {
	h.t.Helper()
	s, err := h.runDrafting(d)
	if err != nil {
		h.t.Fatalf("Run: %v\nstderr: %s", err, h.err.String())
	}
	return s
}

// commitPageA commits page A and its brief, the page a section redraft starts from.
func (h *harness) commitPageA(page, brief string) {
	h.t.Helper()
	writeFile(h.t, h.root, pageA, page)
	writeFile(h.t, h.root, briefA, brief)
	gitIn(h.t, h.root, "add", pageA, briefA)
	gitIn(h.t, h.root, "commit", "-q", "-m", "page A")
}

// newSectionHarness is a drafting blueprint harness for page A with goodPage committed, ready for
// a section redraft.
func newSectionHarness(t *testing.T, opts ...harnessOption) *harness {
	t.Helper()
	h := newBlueprintHarness(t, append([]harnessOption{withPages("a"), withDrafting}, opts...)...)
	h.commitPageA(goodPage, goodBrief)
	return h
}

// workOf reads page A's working memory from the run's state on disk.
func (h *harness) workOf(runID string) work {
	h.t.Helper()
	var w work
	if err := json.Unmarshal(h.state(runID).Pages[pageKey(pageA)].Work, &w); err != nil {
		h.t.Fatal(err)
	}
	return w
}

// requestText is every part of a request's last user turn, cached parts first.
func requestText(r batch.Request) string {
	m := r.Messages[len(r.Messages)-1]
	return strings.Join(append(slices.Clone(m.Cached), m.Text), "\n\n")
}

func isEditsRequest(r batch.Request) bool { return strings.Contains(string(r.Schema), `"resolutions"`) }

func mustAccept(t *testing.T, h *harness, s Summary) {
	t.Helper()
	if got := statusOf(s, pageA); got != StatusAccepted {
		t.Fatalf("status = %s (%s)\nstderr: %s", got, ruleOf(s, pageA), h.err.String())
	}
}

func TestDraftPathAcceptsAWholeDraft(t *testing.T) {
	t.Run("a Draft page reads, drafts whole, gates, covers, reviews, and accepts", func(t *testing.T) {
		h := newBlueprintHarness(t, withPages("a"), withDrafting)
		start := h.head()
		s := h.mustDraft(drafting{})
		mustAccept(t, h, s)
		if got, want := acceptedOf(s, pageA), h.blobAt(start, bpA); got != want {
			t.Errorf("accepted = %q, want the blueprint's blob at the start commit %q", got, want)
		}
		if h.read(pageA) != goodPage {
			t.Error("the page on disk is not the draft")
		}
		for _, c := range []struct {
			stage, seat string
			n           int
		}{
			{s1, "blueprint", 1}, {s2, "draft", 1}, {s3, "coverage", 1},
			{s4, "structure", 1}, {s4, "editor", 1}, {s4, "fact", 1}, {s7, "reader", 2}, {s8, "coherence", 0},
		} {
			if got := len(requestsFor(h, c.stage, c.seat)); got != c.n {
				t.Errorf("%s %s requests = %d, want %d", c.stage, c.seat, got, c.n)
			}
		}
		if d := requestsFor(h, s2, "draft"); len(d) == 1 && isEditsRequest(d[0]) {
			t.Error("a whole draft asked for edits")
		}
	})
	t.Run("without Draft the page accepts at the read (regression)", func(t *testing.T) {
		h := newBlueprintHarness(t, withPages("a"), withDrafting)
		s := h.mustRun()
		mustAccept(t, h, s)
		if len(h.api.requests) != 2 || h.exists(pageA) {
			t.Errorf("requests = %d, page written = %v, want stage 1 only and no page", len(h.api.requests), h.exists(pageA))
		}
	})
}

func TestDraftPathSkipReadSendsNoStageOneRequest(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"), withDrafting)
	s := h.mustDraft(drafting{skipRead: map[string]bool{pageA: true}})
	mustAccept(t, h, s)
	for _, r := range h.api.requests {
		if stage, _ := stageSeat(r.CustomID); stage == s1 {
			t.Errorf("a skipped read sent %s", r.CustomID)
		}
	}
	if len(h.api.requests) == 0 || !strings.Contains(h.api.requests[0].CustomID, "_s2_draft_") {
		t.Errorf("the first request is not the draft: %v", ids(h.api.requests))
	}
	if acceptedOf(s, pageA) != "" {
		t.Error("a page whose read was skipped carries an accepted id")
	}
}

func TestDraftPathPlanDocIsTheBlueprintView(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"), withDrafting)
	o := h.options()
	o.Draft, o.DryRun = true, true
	r, err := newRunner(t.Context(), o)
	if err != nil {
		t.Fatal(err)
	}
	defer r.release()
	p := r.pages[0]
	doc, err := p.planDoc()
	if err != nil {
		t.Fatal(err)
	}
	in, err := p.blueprintInputs()
	if err != nil {
		t.Fatal(err)
	}
	if doc == nil || doc != in.view {
		t.Fatal("planDoc is not the blueprint's view")
	}
	var headings []string
	for _, s := range doc.Sections {
		headings = append(headings, s.Heading)
	}
	if !slices.Equal(headings, []string{"Before you begin", "Rotate the key"}) {
		t.Errorf("view headings = %v", headings)
	}
}

func TestDraftPathRequestBlocks(t *testing.T) {
	const draftingOpening = "The page must carry what they say"
	const readOpening = "They are everything you read"
	h := newSectionHarness(t)
	h.api.script(s2, "draft", okText(answer(inOld, inNew, 1)))
	mustAccept(t, h, h.mustDraft(sectionScope("rotate")))

	read := requestsFor(h, s1, "blueprint")
	if len(read) != 1 || !strings.Contains(requestText(read[0]), readOpening) || strings.Contains(requestText(read[0]), draftingOpening) {
		t.Error("the blueprint read lost its own facts block opening")
	}
	carry := map[string][]batch.Request{
		"section draft": requestsFor(h, s2, "draft"), "structure": requestsFor(h, s4, "structure"),
		"editor": requestsFor(h, s4, "editor"), "fact": requestsFor(h, s4, "fact"), "coherence": requestsFor(h, s8, "coherence"),
	}
	for name, rs := range carry {
		if len(rs) == 0 {
			t.Errorf("no %s request", name)
			continue
		}
		m := rs[0].Messages[len(rs[0].Messages)-1]
		if len(m.Cached) != 2 || !strings.Contains(m.Cached[1], "=== facts placed or cited ===") || !strings.Contains(m.Cached[1], draftingOpening) {
			t.Errorf("%s request's cached parts lack the drafting facts block: %q", name, m.Cached)
		}
		if strings.Contains(requestText(rs[0]), readOpening) {
			t.Errorf("%s request carries the read's facts block opening", name)
		}
		if !strings.Contains(m.Text, "=== blueprint ===") || !strings.Contains(m.Text, "=== outline entry ===") {
			t.Errorf("%s request lacks the blueprint block", name)
		}
		hasChange := strings.Contains(m.Text, "These sections changed: rotate.") && strings.Contains(m.Text, changeIn)
		if want := name == "section draft"; hasChange != want {
			t.Errorf("%s request carries the scope sentence and change block = %v, want %v", name, hasChange, want)
		}
	}
	for _, r := range requestsFor(h, s7, "reader") {
		if text := requestText(r); strings.Contains(text, "=== facts placed or cited ===") || strings.Contains(text, "=== blueprint ===") {
			t.Errorf("reader request %s carries a blueprint block", r.CustomID)
		}
	}

	whole := newBlueprintHarness(t, withPages("a"), withDrafting)
	mustAccept(t, whole, whole.mustDraft(drafting{}))
	d := requestsFor(whole, s2, "draft")
	if len(d) != 1 || !strings.Contains(requestText(d[0]), draftingOpening) || !strings.Contains(requestText(d[0]), "=== blueprint ===") {
		t.Error("the whole draft's request lacks the blocks")
	}
	if strings.Contains(requestText(d[0]), "These sections changed") {
		t.Error("the whole draft's request carries a scope sentence")
	}
}

// TestDraftRunKeepsTheBlueprintReadGolden pins the blueprint read of a drafting run to the golden
// captured before drafting existed: setting Draft must not change the read's bytes.
func TestDraftRunKeepsTheBlueprintReadGolden(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"))
	if _, err := h.runDrafting(drafting{}); err != nil {
		t.Fatal(err)
	}
	reqs := requestsFor(h, s1, "blueprint")
	if len(reqs) != 1 {
		t.Fatalf("blueprint requests = %d, want 1", len(reqs))
	}
	r := reqs[0]
	r.CustomID = ""
	got, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "blueprint-read-request.golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(append(got, '\n')) != string(want) {
		t.Errorf("a drafting run's blueprint read differs from the golden\n got: %s", got)
	}
}

func TestSectionRedraftDecision(t *testing.T) {
	tests := []struct {
		name    string
		page    string // committed before the run; empty commits nothing
		run     drafting
		section bool
		// readSays is a sentence the blueprint read's request must carry, the change read's.
		readSays string
	}{
		{"every changed heading found makes a section redraft", goodPage, sectionScope("rotate"), true, "These sections changed: rotate."},
		{"a missing heading drafts whole and keeps the change read",
			strings.Replace(goodPage, "## Rotate the key", "## Rotation", 1), sectionScope("rotate"), false, "These sections changed: rotate."},
		{"empty IDs draft whole", goodPage, sectionScope(), false, "No section's own content changed"},
		{"an id the blueprint lacks drafts whole", goodPage, sectionScope("gone"), false, "These sections changed: gone."},
		{"a page absent at the start drafts whole", "", sectionScope("rotate"), false, "These sections changed: rotate."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newBlueprintHarness(t, withPages("a"), withDrafting)
			if tt.page != "" {
				h.commitPageA(tt.page, goodBrief)
			}
			if tt.section {
				h.api.script(s2, "draft", okText(answer(inOld, inNew, 1)))
			}
			s := h.mustDraft(tt.run)
			mustAccept(t, h, s)
			if len(h.record(s.RunID, pageA).Faults) != 0 {
				t.Errorf("faults: %v", h.record(s.RunID, pageA).Faults)
			}
			if got := h.workOf(s.RunID).Section; got != tt.section {
				t.Errorf("work.section = %v, want %v", got, tt.section)
			}
			d := requestsFor(h, s2, "draft")
			if len(d) == 0 || isEditsRequest(d[0]) != tt.section {
				t.Errorf("the first draft request asks for edits = %v, want %v", len(d) > 0 && isEditsRequest(d[0]), tt.section)
			}
			if got := len(requestsFor(h, s8, "coherence")) > 0; got != tt.section {
				t.Errorf("coherence ran = %v, want %v", got, tt.section)
			}
			if read := requestsFor(h, s1, "blueprint"); len(read) != 1 || !strings.Contains(requestText(read[0]), tt.readSays) {
				t.Errorf("the blueprint read is not the change read saying %q", tt.readSays)
			}
		})
	}
}

func TestMissingSeatsForDraftingPages(t *testing.T) {
	const noCoherence = `return { blueprint: (c) => "b", draft: (c) => "d", structure: (c) => "s", editor: (c) => "e", fact: (c) => "f", reader: (c) => "r" };` + "\n"
	const noBlueprint = `return { draft: (c) => "d", structure: (c) => "s", editor: (c) => "e", fact: (c) => "f", reader: (c) => "r" };` + "\n"
	tests := []struct {
		name   string
		module string
		run    drafting
		want   string // the refusal names this; empty means no seat is missing
	}{
		{"a scoped Draft page with IDs needs coherence", noCoherence, sectionScope("rotate"), "no seat coherence"},
		{"a scoped Draft page with no IDs does not", noCoherence, sectionScope(), ""},
		{"a whole Draft page does not", noCoherence, drafting{}, ""},
		{"a Draft page needs the blueprint seat for its read", noBlueprint, drafting{}, "no seat blueprint"},
		{"a skipped read does not", noBlueprint, drafting{skipRead: map[string]bool{pageA: true}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newSectionHarness(t)
			h.cfg.Prompts = filepath.Join(t.TempDir(), "module.js")
			if err := os.WriteFile(h.cfg.Prompts, []byte(tt.module), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := h.runDrafting(tt.run)
			if tt.want == "" {
				if err != nil && strings.Contains(err.Error(), "has no seat") {
					t.Errorf("refused: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want one naming %q", err, tt.want)
			}
			if len(h.api.requests) != 0 || len(h.ledgerLines("reserve")) != 0 {
				t.Errorf("%d requests and %d reservations before the refusal", len(h.api.requests), len(h.ledgerLines("reserve")))
			}
		})
	}
}
