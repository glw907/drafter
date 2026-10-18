package pipeline

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/handoff"
	"github.com/glw907/drafter/internal/run"
)

const (
	bpA = "blueprints/docs/guides/rotate-the-api-key.blueprint.yaml"
	bpB = "blueprints/docs/guides/revoke-a-key.blueprint.yaml"
)

// newBlueprintHarness is newHarness with handoffs built the way drafter read builds them: one per
// blueprint, with the blueprint as the plan and the page's outline entry in the context.
func newBlueprintHarness(t *testing.T, opts ...harnessOption) *harness {
	t.Helper()
	h := newHarness(t)
	h.cfg.Blueprints = config.Blueprints{Root: "blueprints", Outline: "blueprints/outline.yaml"}
	seat := h.cfg.Seat["structure"]
	h.cfg.Seat["blueprint"] = seat
	h.handoffs = []handoff.Handoff{h.blueprintHandoff(pageA, briefA, bpA, "Rotate a key"), h.blueprintHandoff(pageB, briefB, bpB, "Revoke a key")}
	for _, o := range opts {
		o(h)
	}
	return h
}

func (h *harness) blueprintHandoff(page, brief, bp, job string) handoff.Handoff {
	h.t.Helper()
	ctx, err := json.Marshal(map[string]any{"resolved": map[string]any{"pages": []any{map[string]any{"path": page, "job": job}}}})
	if err != nil {
		h.t.Fatal(err)
	}
	sum, err := handoff.Checksum(h.root, bp, "")
	if err != nil {
		h.t.Fatal(err)
	}
	return handoff.Handoff{
		Version: handoff.Version, Project: h.root, Page: handoff.Page{Path: page, Brief: brief, Type: "task-guide"},
		Plan: bp, Inputs: handoff.Inputs{Job: job}, Context: ctx, Checksum: sum,
	}
}

// commitBlueprint rewrites a blueprint with edit applied and commits it, then refreshes the
// handoffs' checksums.
func (h *harness) commitBlueprint(bp string, edit func(string) string) {
	h.t.Helper()
	if err := os.WriteFile(filepath.Join(h.root, bp), []byte(edit(h.read(bp))), 0o644); err != nil {
		h.t.Fatal(err)
	}
	gitIn(h.t, h.root, "add", bp)
	gitIn(h.t, h.root, "commit", "-q", "-m", "edit "+bp)
	for i := range h.handoffs {
		sum, err := handoff.Checksum(h.root, h.handoffs[i].Plan, "")
		if err != nil {
			h.t.Fatal(err)
		}
		h.handoffs[i].Checksum = sum
	}
}

func execGit(dir string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	return strings.TrimSpace(string(out)), err
}

func (h *harness) head() string {
	h.t.Helper()
	out, err := execGit(h.root, "rev-parse", "HEAD")
	if err != nil {
		h.t.Fatal(err)
	}
	return out
}

func (h *harness) blobAt(commit, rel string) string {
	h.t.Helper()
	out, err := execGit(h.root, "rev-parse", commit+":"+rel)
	if err != nil {
		h.t.Fatal(err)
	}
	return out
}

func acceptedOf(s Summary, page string) string {
	for _, p := range s.Pages {
		if p.Path == page {
			return p.Accepted
		}
	}
	return ""
}

func TestBlueprintReadAccepts(t *testing.T) {
	h := newBlueprintHarness(t)
	start := h.head()
	s := h.mustRun()
	for _, pg := range []string{pageA, pageB} {
		if got := statusOf(s, pg); got != StatusAccepted {
			t.Fatalf("%s = %s (%s), want accepted\nstderr: %s", pg, got, ruleOf(s, pg), h.err.String())
		}
	}
	if got, want := acceptedOf(s, pageA), h.blobAt(start, bpA); got != want {
		t.Errorf("accepted id = %q, want the blueprint's blob id at the start commit %q", got, want)
	}
	if h.exists(pageA) || h.exists(briefA) {
		t.Error("a read wrote the page or its brief")
	}
	if n := len(requestsFor(h, "s1", "blueprint")); n != 2 {
		t.Errorf("blueprint requests = %d, want one per page", n)
	}
	if n := len(requestsFor(h, "s1", "consistency")); n != 2 {
		t.Errorf("consistency requests = %d, want one per page, in the same stage", n)
	}
	if len(h.api.requests) != 4 {
		t.Errorf("requests = %d, want stage 1 only", len(h.api.requests))
	}
	if open := h.openIntents(s.RunID); len(open) != 0 {
		t.Errorf("open intents after a finished run: %v", open)
	}
}

func TestBlueprintReadWithoutFactsStore(t *testing.T) {
	h := newBlueprintHarness(t, withoutFacts, withPages("a"))
	h.commitBlueprint(bpA, func(s string) string {
		return strings.Replace(strings.Replace(s, "  facts: [lumen-key-ttl]\n", "", 1), "      - id: f-ttl\n        kind: fact\n        fact: lumen-key-ttl\n        use: |\n          Only the lifetime.\n", "      - id: h-start\n        kind: handoff\n        text: |\n          Start here.\n", 1)
	})
	s := h.mustRun()
	if got := statusOf(s, pageA); got != StatusAccepted {
		t.Fatalf("status = %s (%s)\nstderr: %s", got, ruleOf(s, pageA), h.err.String())
	}
	if n := len(requestsFor(h, "s1", "consistency")); n != 0 {
		t.Errorf("consistency requests = %d, want none without a facts store", n)
	}
}

func TestBlueprintReadFixSetsPageAside(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"))
	h.api.script("s1", "blueprint", okText(readText("fix", finding("rotate", "A step the page needs is missing."))))
	s := h.mustRun()
	if got, rule := statusOf(s, pageA), ruleOf(s, pageA); got != StatusEscalated || rule != "plan" {
		t.Fatalf("status = %s, rule = %s, want escalated under plan", got, rule)
	}
	if acceptedOf(s, pageA) != "" {
		t.Error("an escalated page carries an accepted id")
	}
	data, err := os.ReadFile(filepath.Join(h.runDir(s.RunID), run.PageKey(pageA), "record.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rec record
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatal(err)
	}
	if rec.Escalation == nil || rec.Escalation.Rule != "plan" || rec.Escalation.Seat != "blueprint" {
		t.Errorf("escalation = %+v, want rule plan and seat blueprint", rec.Escalation)
	}
	if len(rec.Findings) != 1 || rec.Findings[0].Seat != "blueprint" || rec.Findings[0].Location != "rotate" {
		t.Errorf("findings = %+v, want the blueprint seat's finding", rec.Findings)
	}
}

func TestBlueprintCheckErrorStopsBeforeAnyRequest(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"))
	h.commitBlueprint(bpA, func(s string) string {
		return strings.Replace(s, "    why: |\n      Rotation is the page's job, so it follows the prerequisites.\n", "", 1)
	})
	s := h.mustRun()
	if got, rule := statusOf(s, pageA), ruleOf(s, pageA); got != StatusEscalated || rule != "plan" {
		t.Fatalf("status = %s, rule = %s, want escalated under plan", got, rule)
	}
	if h.api.counts != 0 || len(h.api.requests) != 0 {
		t.Errorf("CountTokens calls = %d, requests = %d, want none before the check passes", h.api.counts, len(h.api.requests))
	}
}

func TestBlueprintReadOneAcceptedOneFix(t *testing.T) {
	h := newBlueprintHarness(t, withInFlight(1))
	h.api.script("s1", "blueprint", okText(readText("ok")), okText(readText("fix", finding("revoke", "Say how long to wait."))))
	s := h.mustRun()
	if statusOf(s, pageA) != StatusAccepted || statusOf(s, pageB) != StatusEscalated {
		t.Fatalf("statuses = %s and %s, want accepted and escalated", statusOf(s, pageA), statusOf(s, pageB))
	}
	n := 0
	for _, p := range s.Pages {
		if p.Accepted != "" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("accepted ids = %d, want exactly one", n)
	}
}

func TestBlueprintReadStopAndResume(t *testing.T) {
	h := newBlueprintHarness(t, withInFlight(1))
	h.api.crash = func(call string, ids []string) string {
		if call == "create" && h.api.creates == 2 {
			return "before"
		}
		return ""
	}
	if !dies(func() { _, _ = h.run() }) {
		t.Fatal("the run did not die before the second page's batch")
	}
	id := h.runID()
	start := h.state(id).Head
	h.api.crash = nil
	// A blueprint edited and committed after the stop must not change what the resume reads.
	h.commitBlueprint(bpB, func(s string) string { return s + "constraints:\n  - Added after the stop.\n" })
	s, err := h.resume(id)
	if err != nil {
		t.Fatalf("resume: %v\nstderr: %s", err, h.err.String())
	}
	for _, pg := range []string{pageA, pageB} {
		if got := statusOf(s, pg); got != StatusAccepted {
			t.Errorf("%s = %s (%s), want accepted", pg, got, ruleOf(s, pg))
		}
	}
	if got, want := acceptedOf(s, pageB), h.blobAt(start, bpB); got != want {
		t.Errorf("accepted id of the edited blueprint = %q, want its blob id at the start commit %q", got, want)
	}
	for _, r := range h.api.requests {
		for _, m := range r.Messages {
			if strings.Contains(m.Text, "Added after the stop.") {
				t.Errorf("request %s holds the blueprint as committed after the stop", r.CustomID)
			}
		}
	}
	if got := acceptedOf(s, pageB); got == h.blobAt("HEAD", bpB) {
		t.Error("the resume stamped the blueprint committed after the stop")
	}
	settles := map[string]int{}
	for _, l := range h.ledgerLines("settle") {
		key, _ := l["key"].(string)
		settles[key]++
	}
	for key, n := range settles {
		if n != 1 {
			t.Errorf("settle key %s appears %d times, want once per custom_id", key, n)
		}
	}
}

func TestBlueprintRequestContents(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"))
	h.mustRun()
	reqs := requestsFor(h, "s1", "blueprint")
	if len(reqs) != 1 {
		t.Fatalf("blueprint requests = %d", len(reqs))
	}
	text := userText(reqs[0])
	raw := h.read(bpA)
	if n := strings.Count(text, strings.TrimRight(raw, "\n")); n != 1 {
		t.Errorf("the blueprint's bytes appear %d times, want once", n)
	}
	for _, want := range []string{"Rotate a Lumen API key without interrupting publishing.", "A Lumen API key lives for 90 days."} {
		if !strings.Contains(text, want) {
			t.Errorf("the request lacks %q", want)
		}
	}
	for _, bad := range []string{"=== file: " + pageA, "=== file: " + bpA, "lumen-grace", "The old key keeps working", "=== file: docs/facts", "=== excerpt"} {
		if strings.Contains(text, bad) {
			t.Errorf("the request holds %q", bad)
		}
	}
	if !strings.Contains(text, "- lumen-key-ttl [verified]") {
		t.Error("the request lacks the bullet of the placed fact")
	}
}

func TestBlueprintEveryCreateFollowsAReserve(t *testing.T) {
	h := newBlueprintHarness(t)
	submits := 0
	h.api.onSubmit = func(ids []string) {
		submits++
		if reserves := len(h.ledgerLines("reserve")); reserves < submits {
			t.Errorf("submission %d (%v) went out with %d reserve lines in the ledger", submits, ids, reserves)
		}
	}
	h.mustRun()
}

func TestBlueprintDryRun(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"))
	h.opts = h.options()
	h.opts.DryRun = true
	s, err := Run(t.Context(), h.opts)
	if err != nil {
		t.Fatal(err)
	}
	if s.EstimateStage != "s1" || len(s.Estimates) != 2 {
		t.Fatalf("stage %q with %d estimates, want s1 with the consistency and blueprint requests", s.EstimateStage, len(s.Estimates))
	}
	seats := []string{s.Estimates[0].Seat, s.Estimates[1].Seat}
	if seats[0] != "consistency" || seats[1] != "blueprint" {
		t.Errorf("seats = %v", seats)
	}
	if len(h.api.batches) != 0 || h.api.creates != 0 || h.api.sends != 0 {
		t.Error("a dry run submitted")
	}
}

func TestResumeHintNamesTheCommand(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"), func(h *harness) { h.cfg.CycleCeiling = 0.0001 })
	h.opts = h.options()
	h.opts.Command = "drafter read"
	s, err := Run(t.Context(), h.opts)
	if err != nil {
		t.Fatal(err)
	}
	if s.Stop == "" || !strings.HasPrefix(s.ResumeLine, "drafter read --resume ") {
		t.Errorf("stop %q, resume line %q, want a stop that names drafter read", s.Stop, s.ResumeLine)
	}
}

func TestPageIgnoresTheBlueprintSeat(t *testing.T) {
	h := newHarness(t)
	h.cfg.Seats = defaultSeatsForTest()
	s := h.mustRun()
	if statusOf(s, pageA) != StatusAccepted {
		t.Fatalf("a v1 handoff no longer runs: %s", statusOf(s, pageA))
	}
	for _, r := range h.api.requests {
		if _, seat := stageSeat(r.CustomID); seat == "blueprint" {
			t.Error("drafter page sent a blueprint request")
		}
	}
}

func defaultSeatsForTest() []string {
	return []string{"draft", "structure", "editor", "fact", "reader", "coverage", "consistency"}
}
