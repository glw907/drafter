package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/batch"
	"github.com/glw907/drafter/internal/blueprint"
	"github.com/glw907/drafter/internal/run"
)

const factStoreHead = "# Key facts\n\n## Lifetimes\n\n" +
	"- `lumen-key-ttl` A Lumen API key lives for 90 days. Source: `src/keys.js:2`. [verified]\n" +
	"- `lumen-grace` The old key keeps working for one hour after a rotation. [external: the vendor's rotation guide]\n"

// withFactStore replaces the facts store with the fixture's two facts plus extra bullets and
// commits it, so the next run reads it at its start commit.
func withFactStore(extra ...string) harnessOption {
	return func(h *harness) {
		h.t.Helper()
		text := factStoreHead + strings.Join(extra, "")
		if err := os.WriteFile(filepath.Join(h.root, "docs/facts/keys.md"), []byte(text), 0o644); err != nil {
			h.t.Fatal(err)
		}
		gitIn(h.t, h.root, "add", "docs/facts/keys.md")
		gitIn(h.t, h.root, "commit", "-q", "-m", "facts")
	}
}

// sourced is a verified bullet whose pointer shows a line of the fixture's source file.
func sourced(id, claim string) string {
	return "- `" + id + "` " + claim + " Source: `src/keys.js:2`. [verified]\n"
}

// citeFacts rewrites the rotate blueprint's introduction to cite exactly the given facts.
func citeFacts(ids ...string) func(string) string {
	return func(s string) string {
		return strings.Replace(s, "  facts: [lumen-key-ttl]\n", "  facts: ["+strings.Join(ids, ", ")+"]\n", 1)
	}
}

// factTruthIDs lists the facts a fact-truth request carries, in order.
func factTruthIDs(r batch.Request) []string {
	var ids []string
	for _, m := range regexp.MustCompile("(?m)^- `([^`]+)` ").FindAllStringSubmatch(r.Messages[0].Text, -1) {
		ids = append(ids, m[1])
	}
	return ids
}

func recordOf(t *testing.T, h *harness, s Summary, page string) record {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(h.runDir(s.RunID), run.PageKey(page), "record.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rec record
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatal(err)
	}
	return rec
}

func findingsBySeat(s Summary, seat string) []FindingResult {
	var out []FindingResult
	for _, f := range statusPage(s, pageA).Findings {
		if f.Seat == seat {
			out = append(out, f)
		}
	}
	return out
}

func TestFactTruthWholeReadHolds(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"), withFactStore(
		sourced("lumen-a", "Claim a."), sourced("lumen-b", "Claim b."), sourced("lumen-c", "Claim c.")))
	h.commitBlueprint(bpA, citeFacts("lumen-key-ttl", "lumen-a", "lumen-b", "lumen-c"))
	s := h.mustRun()
	if got := statusOf(s, pageA); got != StatusAccepted {
		t.Fatalf("status = %s (%s)\nstderr: %s", got, ruleOf(s, pageA), h.err.String())
	}
	reqs := requestsFor(h, "s1", "facttruth")
	if len(reqs) != 1 {
		t.Fatalf("facttruth requests = %d, want 1", len(reqs))
	}
	if got, want := factTruthIDs(reqs[0]), []string{"lumen-a", "lumen-b", "lumen-c", "lumen-key-ttl"}; !slices.Equal(got, want) {
		t.Errorf("request facts = %v, want %v", got, want)
	}
	text := userText(reqs[0])
	for _, want := range []string{
		"- `lumen-key-ttl` A Lumen API key lives for 90 days. Source: `src/keys.js:2`. [verified]",
		"- `lumen-a` Claim a. Source: `src/keys.js:2`. [verified]",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("request lacks the Raw bullet %q", want)
		}
	}
	if n := strings.Count(text, "=== excerpt: src/keys.js:2 (fact "); n != 4 {
		t.Errorf("request holds %d excerpt blocks, want 4", n)
	}
	pg := statusPage(s, pageA)
	if pg.WholeBlocked || len(pg.Findings) != 0 {
		t.Errorf("page = %+v, want no bar and no findings", pg)
	}
}

func TestFactTruthBlocksAndTagsItsFindings(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"), withFactStore(
		sourced("lumen-a", "Claim a."), sourced("lumen-b", "Claim b.")))
	h.commitBlueprint(bpA, citeFacts("lumen-key-ttl", "lumen-a", "lumen-b"))
	h.api.script("s1", "facttruth", okText(factTruthText(
		[4]string{"lumen-key-ttl", "holds", "", ""},
		[4]string{"lumen-a", "wider", "export const keyLifetimeDays = 90;", "A key lives for 90 days, and the code shows no cap on rotation."},
		[4]string{"lumen-b", "contradicts", "export const keyLifetimeDays = 90;", "A key lives for 90 days, not 30."},
	)))
	h.api.script("s1", "blueprint", okText(jsonText(map[string]any{
		"verdict": "ok", "summary": "read",
		"findings": []map[string]any{{"location": "intro", "finding": "Nit.", "rewrite": "r", "blocking": false}},
	})))
	s := h.mustRun()
	pg := statusPage(s, pageA)
	if pg.Status != StatusEscalated || pg.Rule != "facttruth" || !pg.WholeBlocked {
		t.Fatalf("page = %s rule %q whole_blocked %v, want escalated under facttruth with a whole read that blocked\nstderr: %s", pg.Status, pg.Rule, pg.WholeBlocked, h.err.String())
	}
	ft := findingsBySeat(s, "facttruth")
	if len(ft) != 2 {
		t.Fatalf("facttruth findings = %+v, want 2", ft)
	}
	for i, want := range []struct{ loc, verdict, excerpt, fix string }{
		{"fact lumen-a", "wider", "export const keyLifetimeDays = 90;", "no cap on rotation"},
		{"fact lumen-b", "contradicts", "export const keyLifetimeDays = 90;", "not 30"},
	} {
		f := ft[i]
		if f.Location != want.loc || !f.Blocking || f.Kind != "fact" || !strings.HasPrefix(f.Text, want.verdict+": ") ||
			!strings.Contains(f.Text, "(excerpt: "+want.excerpt+")") || !strings.Contains(f.Text, want.fix) {
			t.Errorf("finding %d = %+v, want %+v", i, f, want)
		}
	}
	bp := findingsBySeat(s, "blueprint")
	if len(bp) != 1 || bp[0].Blocking {
		t.Errorf("blueprint findings = %+v, want its one nit, seat blueprint", bp)
	}
	data, err := json.Marshal(pg)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"seat":"facttruth"`, `"kind":"fact"`, `"seat":"blueprint"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("summary JSON lacks %s: %s", want, data)
		}
	}
}

func TestFactTruthDuplicatesAndStrangers(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"), withFactStore(sourced("lumen-a", "Claim a.")))
	h.commitBlueprint(bpA, citeFacts("lumen-key-ttl", "lumen-a"))
	h.api.script("s1", "facttruth", okText(factTruthText(
		[4]string{"lumen-a", "holds", "", ""},
		[4]string{"lumen-key-ttl", "holds", "", ""},
		[4]string{"lumen-a", "wider", "export const keyLifetimeDays = 90;", "The code shows less."},
		[4]string{"lumen-ghost", "contradicts", "x", "y"},
	)))
	s := h.mustRun()
	ft := findingsBySeat(s, "facttruth")
	if len(ft) != 1 || ft[0].Location != "fact lumen-a" || !strings.HasPrefix(ft[0].Text, "wider: ") {
		t.Fatalf("findings = %+v, want one wider finding on lumen-a, the worse of its two verdicts", ft)
	}
	if got := h.err.String(); !strings.Contains(got, "lumen-ghost") || !strings.Contains(got, "dropped") {
		t.Errorf("stderr = %q, want a warning that lumen-ghost was dropped", got)
	}
}

func TestFactTruthNoExcerptFromGo(t *testing.T) {
	directory := "- `lumen-dir` Claim about a directory. Source: `src`. [verified]\n"
	external := "- `lumen-ext` Claim about a vendor. Source: `src`. [external: the vendor guide]\n"
	t.Run("a verified fact whose only pointer is a directory blocks and is left out of the request", func(t *testing.T) {
		h := newBlueprintHarness(t, withPages("a"), withFactStore(directory))
		h.commitBlueprint(bpA, citeFacts("lumen-key-ttl", "lumen-dir"))
		s := h.mustRun()
		pg := statusPage(s, pageA)
		if pg.Status != StatusEscalated || pg.Rule != "facttruth" {
			t.Fatalf("page = %s rule %q, want escalated under facttruth\nstderr: %s", pg.Status, pg.Rule, h.err.String())
		}
		ft := findingsBySeat(s, "facttruth")
		if len(ft) != 1 || ft[0].Location != "fact lumen-dir" || !ft[0].Blocking || !strings.HasPrefix(ft[0].Text, "no-excerpt: ") {
			t.Errorf("findings = %+v, want one blocking no-excerpt finding on lumen-dir", ft)
		}
		reqs := requestsFor(h, "s1", "facttruth")
		if len(reqs) != 1 || !slices.Equal(factTruthIDs(reqs[0]), []string{"lumen-key-ttl"}) {
			t.Errorf("requests = %d, want one carrying only lumen-key-ttl", len(reqs))
		}
	})
	t.Run("an external fact with the same pointer is a warning that blocks nothing", func(t *testing.T) {
		h := newBlueprintHarness(t, withPages("a"), withFactStore(external))
		h.commitBlueprint(bpA, citeFacts("lumen-key-ttl", "lumen-ext"))
		s := h.mustRun()
		pg := statusPage(s, pageA)
		if pg.Status != StatusAccepted {
			t.Fatalf("status = %s (%s), want accepted\nstderr: %s", pg.Status, pg.Rule, h.err.String())
		}
		ft := findingsBySeat(s, "facttruth")
		if len(ft) != 1 || ft[0].Location != "fact lumen-ext" || ft[0].Blocking || !strings.HasPrefix(ft[0].Text, "no-excerpt: ") {
			t.Errorf("findings = %+v, want one non-blocking no-excerpt finding on lumen-ext", ft)
		}
	})
	t.Run("a set with nothing to read sends no request", func(t *testing.T) {
		h := newBlueprintHarness(t, withPages("a"), withFactStore(directory))
		h.commitBlueprint(bpA, func(s string) string {
			// The framing cites lumen-key-ttl as well, so it moves to the directory fact too.
			return strings.ReplaceAll(citeFacts("lumen-dir")(s), "lumen-key-ttl", "lumen-dir")
		})
		s := h.mustRun()
		if n := len(requestsFor(h, "s1", "facttruth")); n != 0 {
			t.Errorf("facttruth requests = %d, want none", n)
		}
		if got := statusPage(s, pageA); got.Rule != "facttruth" {
			t.Errorf("rule = %q, want facttruth from Go's own verdict", got.Rule)
		}
	})
}

func TestFactTruthOmittedFactBlocks(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"), withFactStore(sourced("lumen-a", "Claim a.")))
	h.commitBlueprint(bpA, citeFacts("lumen-key-ttl", "lumen-a"))
	h.api.script("s1", "facttruth", okText(factTruthText([4]string{"lumen-key-ttl", "holds", "", ""})))
	s := h.mustRun()
	ft := findingsBySeat(s, "facttruth")
	if len(ft) != 1 || !ft[0].Blocking || ft[0].Text != "the fact-truth read returned no verdict for lumen-a" {
		t.Errorf("findings = %+v, want the blocking returned-no-verdict finding for lumen-a", ft)
	}
	if got := statusPage(s, pageA); got.Status != StatusEscalated || got.Rule != "facttruth" {
		t.Errorf("page = %s rule %q, want escalated under facttruth", got.Status, got.Rule)
	}
}

// changeScope is a scoped page as Compare builds it, with the lists a test sets.
func changeScope(facts, joined, rawChanged []string) blueprint.Scope {
	return blueprint.Scope{
		Kind: blueprint.ScopeScoped, IDs: []string{"before-you-begin"}, Facts: append([]string{}, facts...),
		Bullets: []string{}, Joined: append([]string{}, joined...), RawChanged: rawChanged,
	}
}

func TestFactTruthSetOfAChangeRead(t *testing.T) {
	store := withFactStore(sourced("lumen-a", "Claim a."), sourced("lumen-b", "Claim b."), sourced("lumen-c", "Claim c."))
	cite := citeFacts("lumen-key-ttl", "lumen-a", "lumen-b", "lumen-c")
	t.Run("a --since read carries the touched fact and the joined one", func(t *testing.T) {
		h := newBlueprintHarness(t, withPages("a"), store)
		h.commitBlueprint(bpA, cite)
		o := h.options()
		o.Since = barCommit
		o.Scopes = map[string]blueprint.Scope{pageA: changeScope([]string{"lumen-a"}, []string{"lumen-b"}, []string{"lumen-c"})}
		o.Changes = map[string]string{pageA: changeBlockA}
		if _, err := Run(t.Context(), o); err != nil {
			t.Fatalf("Run: %v\nstderr: %s", err, h.err.String())
		}
		reqs := requestsFor(h, "s1", "facttruth")
		if len(reqs) != 1 || !slices.Equal(factTruthIDs(reqs[0]), []string{"lumen-a", "lumen-b"}) {
			t.Errorf("requests = %d, want one carrying lumen-a and lumen-b only", len(reqs))
		}
	})
	t.Run("a change that touches no fact sends no request", func(t *testing.T) {
		h := newBlueprintHarness(t, withPages("a"), store)
		h.commitBlueprint(bpA, cite)
		o := h.options()
		o.Since = barCommit
		o.Scopes = map[string]blueprint.Scope{pageA: changeScope(nil, nil, nil)}
		o.Changes = map[string]string{pageA: changeBlockA}
		s, err := Run(t.Context(), o)
		if err != nil {
			t.Fatalf("Run: %v\nstderr: %s", err, h.err.String())
		}
		if n := len(requestsFor(h, "s1", "facttruth")); n != 0 {
			t.Errorf("facttruth requests = %d, want none", n)
		}
		if got := statusOf(s, pageA); got != StatusAccepted {
			t.Errorf("status = %s, want accepted", got)
		}
	})
	t.Run("a fact the blueprint no longer cites is not judged", func(t *testing.T) {
		h := newBlueprintHarness(t, withPages("a"), store)
		h.commitBlueprint(bpA, cite)
		o := h.options()
		o.Since = barCommit
		o.Scopes = map[string]blueprint.Scope{pageA: changeScope([]string{"lumen-a", "lumen-gone"}, nil, nil)}
		o.Changes = map[string]string{pageA: changeBlockA}
		if _, err := Run(t.Context(), o); err != nil {
			t.Fatalf("Run: %v\nstderr: %s", err, h.err.String())
		}
		reqs := requestsFor(h, "s1", "facttruth")
		if len(reqs) != 1 || !slices.Equal(factTruthIDs(reqs[0]), []string{"lumen-a"}) {
			t.Errorf("requests = %d, want one carrying lumen-a only", len(reqs))
		}
	})
}

func TestFactTruthSetOfABarRead(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"), withFactStore(
		sourced("lumen-a", "Claim a."), sourced("lumen-b", "Claim b."), sourced("lumen-c", "Claim c."), sourced("lumen-d", "Claim d.")))
	h.commitBlueprint(bpA, citeFacts("lumen-key-ttl", "lumen-a", "lumen-b", "lumen-c", "lumen-d"))
	o := h.options()
	// The bar holds a fact-truth finding on lumen-a. The change touches lumen-b, and lumen-c's tag
	// qualifier alone changed since the bar, which only RawChanged sees.
	o.Scopes = map[string]blueprint.Scope{pageA: changeScope([]string{"lumen-b"}, nil, []string{"lumen-c"})}
	o.Changes = map[string]string{pageA: changeBlockA}
	o.Bars, o.BarFrom = map[string]string{pageA: ""}, map[string]string{pageA: barCommit}
	o.BarFacts = map[string][]string{pageA: {"lumen-a"}}
	if _, err := Run(t.Context(), o); err != nil {
		t.Fatalf("Run: %v\nstderr: %s", err, h.err.String())
	}
	reqs := requestsFor(h, "s1", "facttruth")
	if len(reqs) != 1 || !slices.Equal(factTruthIDs(reqs[0]), []string{"lumen-a", "lumen-b", "lumen-c"}) {
		t.Errorf("requests = %d, want one carrying lumen-a, lumen-b, and lumen-c", len(reqs))
	}
}

func TestFactTruthNeverRunsInDrafterPage(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"), withDrafting)
	h.mustDraft(drafting{})
	if n := len(requestsFor(h, "s1", "facttruth")); n != 0 {
		t.Errorf("facttruth requests = %d on a page whose read was not skipped, want none", n)
	}
	if n := len(requestsFor(h, "s1", "blueprint")); n != 1 {
		t.Errorf("blueprint requests = %d, want the read to have run", n)
	}
}

func TestFactTruthRecordKeepsEveryVerdict(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"), withFactStore(sourced("lumen-a", "Claim a.")))
	h.commitBlueprint(bpA, citeFacts("lumen-key-ttl", "lumen-a"))
	h.api.script("s1", "facttruth", okText(factTruthText(
		[4]string{"lumen-key-ttl", "holds", "", ""},
		[4]string{"lumen-a", "wider", "export const keyLifetimeDays = 90;", "Less."},
	)))
	s := h.mustRun()
	rec := recordOf(t, h, s, pageA)
	if rec.FactTruth == nil {
		t.Fatal("record.json has no fact_truth")
	}
	got := map[string]string{}
	for _, v := range rec.FactTruth.Verdicts {
		got[v.ID] = v.Verdict
	}
	if got["lumen-key-ttl"] != "holds" || got["lumen-a"] != "wider" || len(got) != 2 {
		t.Errorf("verdicts = %v, want holds for lumen-key-ttl and wider for lumen-a", got)
	}
	if rec.Escalation == nil || rec.Escalation.Rule != "facttruth" || rec.Escalation.Seat != "facttruth" {
		t.Errorf("escalation = %+v, want rule and seat facttruth", rec.Escalation)
	}
	var seat string
	for _, f := range rec.Findings {
		if f.Seat == "facttruth" {
			seat = f.Kind
		}
	}
	if seat != "fact" {
		t.Errorf("the record's facttruth finding kind = %q, want fact", seat)
	}
}

func TestFactTruthSeatMustBeConfigured(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"))
	delete(h.cfg.Seat, "facttruth")
	_, err := Run(t.Context(), h.options())
	if err == nil || !strings.Contains(err.Error(), "seat.facttruth.model is not set") {
		t.Errorf("Run = %v, want a refusal naming the facttruth seat's missing model", err)
	}
	if len(h.api.requests) != 0 {
		t.Errorf("%d requests went out before the refusal", len(h.api.requests))
	}
}
