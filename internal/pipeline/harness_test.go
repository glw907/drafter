package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glw907/drafter/internal/batch"
	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/handoff"
	"github.com/glw907/drafter/internal/ledger"
	"github.com/glw907/drafter/internal/run"
)

const (
	pageA  = "docs/guides/rotate-the-api-key.md"
	briefA = "docs/guides/rotate-the-api-key.brief.json"
	pageB  = "docs/guides/revoke-a-key.md"
	briefB = "docs/guides/revoke-a-key.brief.json"
)

const gateScript = `#!/bin/sh
fail=0
for f in docs/guides/*.md; do
  [ -f "$f" ] || continue
  if grep -q BROKEN "$f"; then echo "$f: contains BROKEN"; fail=1; fi
done
exit $fail
`

// key is a sentinel shaped like an API key; no output of a run may contain it.
var key = "sk-" + "ant-api03-SENTINEL-do-not-leak-0123456789"

// harness is one synthetic project with a config, a fake API, and the handoffs for its pages.
type harness struct {
	t        *testing.T
	root     string
	stateDir string
	cfg      *config.Config
	api      *fakeAPI
	handoffs []handoff.Handoff
	out, err bytes.Buffer
	opts     Options
}

type harnessOption func(*harness)

func withoutFacts(h *harness) { h.cfg.Facts = config.Facts{} }

func withMode(mode string) harnessOption {
	return func(h *harness) { h.cfg.Mode = mode }
}

func withPageCap(c float64) harnessOption {
	return func(h *harness) { h.cfg.PageCap = c }
}

func withCeiling(c float64) harnessOption {
	return func(h *harness) { h.cfg.CycleCeiling = c }
}

func withMaxTokens(seat string, n int64) harnessOption {
	return func(h *harness) {
		s := h.cfg.Seat[seat]
		s.MaxTokens = n
		h.cfg.Seat[seat] = s
	}
}

func withSeats(seats ...string) harnessOption {
	return func(h *harness) { h.cfg.Seats = seats }
}

func withInFlight(n int) harnessOption {
	return func(h *harness) { h.cfg.PagesInFlight = n }
}

// withPages limits the run to the named pages ("a" or "b").
func withPages(pages ...string) harnessOption {
	return func(h *harness) {
		var keep []handoff.Handoff
		for _, name := range pages {
			if name == "a" {
				keep = append(keep, h.handoffs[0])
			} else {
				keep = append(keep, h.handoffs[1])
			}
		}
		h.handoffs = keep
	}
}

func withoutContext(h *harness) {
	for i := range h.handoffs {
		h.handoffs[i].Context = nil
	}
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func copyTree(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.Walk(from, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		dst := filepath.Join(to, rel)
		if info.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func newHarness(t *testing.T, opts ...harnessOption) *harness {
	t.Helper()
	t.Setenv("DRAFTER_API_KEY", key)
	root := t.TempDir()
	copyTree(t, "testdata/project", root)
	if err := os.WriteFile(filepath.Join(root, "gate.sh"), []byte(gateScript), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "docs/guides"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs/guides/.keep"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, root, "init", "-q", "-b", "main")
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "-q", "-m", "fixture")

	module, err := filepath.Abs("testdata/module.js")
	if err != nil {
		t.Fatal(err)
	}
	agent, err := filepath.Abs("testdata/agents/draft.md")
	if err != nil {
		t.Fatal(err)
	}
	stateDir := t.TempDir()
	seat := func(effort string) config.Seat {
		return config.Seat{Model: "model-a", Effort: effort, MaxTokens: 4000}
	}
	draft := seat("high")
	draft.Agent = agent
	cfg := &config.Config{
		Prices: config.Prices{
			Date: "2026-10-13", Source: "test", BatchDiscount: 0.5,
			Models: map[string]config.Price{"model-a": {Input: 4, Output: 20}},
		},
		BillingCycleDay: 1, WorkspaceID: "ws-test", KeyEnv: "DRAFTER_API_KEY",
		PageCap: 10, CycleCeiling: 100, Mode: "batch", StateDir: stateDir, RetentionDays: 30,
		Output: "human", PagesInFlight: 3,
		Retries: config.Retries{GateFix: 2, EditContract: 2},
		Gate:    config.Gate{Command: "sh gate.sh", Protocol: "plain"},
		Prompts: module,
		Seats:   []string{"draft", "structure", "editor", "fact", "reader", "coverage", "consistency"},
		Seat: map[string]config.Seat{
			"draft": draft, "structure": seat("medium"), "editor": seat("medium"),
			"fact": seat("medium"), "reader": seat("medium"),
			"coverage": seat("low"), "consistency": seat("low"),
		},
		Facts: config.Facts{Dir: filepath.Join(root, "docs/facts")},
		Root:  root,
	}
	h := &harness{t: t, root: root, stateDir: stateDir, cfg: cfg, api: newFakeAPI(t)}
	h.handoffs = []handoff.Handoff{h.handoff(pageA, briefA, "plans/rotate-the-api-key.plan.yaml", "Rotate a key"),
		h.handoff(pageB, briefB, "plans/revoke-a-key.plan.yaml", "Revoke a key")}
	for _, o := range opts {
		o(h)
	}
	return h
}

func (h *harness) handoff(page, brief, plan, job string) handoff.Handoff {
	h.t.Helper()
	ctx := `{"args":{"outline":"docs/outline.json"},"resolved":{"index":"docs/index.md","pages":[` +
		`{"path":"` + pageA + `","job":"Rotate a key"},{"path":"` + pageB + `","job":"Revoke a key"}]}}`
	sum, err := handoff.Checksum(h.root, plan, "")
	if err != nil {
		h.t.Fatal(err)
	}
	return handoff.Handoff{
		Version: handoff.Version, Project: h.root,
		Page: handoff.Page{Path: page, Brief: brief, Type: "task-guide"},
		Plan: plan,
		Inputs: handoff.Inputs{
			Job: job, PageType: "task-guide", RegisterSections: []string{}, ExemplarSources: []string{},
			ClaimInventory: []handoff.Claim{}, FactIDs: []string{"lumen-key-ttl"},
		},
		Context:  json.RawMessage(ctx),
		Checksum: sum,
	}
}

// run runs the pipeline once and returns its summary and error.
func (h *harness) run() (Summary, error) {
	h.t.Helper()
	h.opts = Options{
		Config: h.cfg, Handoffs: h.handoffs, API: h.api, Version: "test",
		Now:   func() time.Time { return time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC) },
		Sleep: func(context.Context, time.Duration) error { return nil }, PollInterval: time.Millisecond,
		Out: &h.out, Err: &h.err,
	}
	return Run(context.Background(), h.opts)
}

func (h *harness) mustRun() Summary {
	h.t.Helper()
	s, err := h.run()
	if err != nil {
		h.t.Fatalf("Run: %v\nstderr: %s", err, h.err.String())
	}
	return s
}

// ledgerLines reads every ledger line of one type.
func (h *harness) ledgerLines(typ string) []map[string]any {
	h.t.Helper()
	data, err := os.ReadFile(filepath.Join(h.stateDir, "ledger.jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		h.t.Fatal(err)
	}
	var out []map[string]any
	for ln := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		if ln == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(ln), &m); err != nil {
			h.t.Fatalf("ledger line %q: %v", ln, err)
		}
		if typ == "" || m["type"] == typ {
			out = append(out, m)
		}
	}
	return out
}

// settleKinds counts the settle lines of each kind.
func (h *harness) settleKinds() map[string]int {
	h.t.Helper()
	kinds := map[string]int{}
	for _, l := range h.ledgerLines("settle") {
		kind, _ := l["kind"].(string)
		kinds[kind]++
	}
	return kinds
}

// openIntents is what the ledger still holds open for the run.
func (h *harness) openIntents(runID string) []ledger.Reservation {
	h.t.Helper()
	open, err := ledger.Open(h.stateDir).OpenIntents(runID)
	if err != nil {
		h.t.Fatal(err)
	}
	return open
}

// runDir returns the run's directory.
func (h *harness) runDir(runID string) string {
	h.t.Helper()
	repo, err := run.Repo(h.root)
	if err != nil {
		h.t.Fatal(err)
	}
	return filepath.Join(h.stateDir, "runs", repo, runID)
}

func (h *harness) state(runID string) *run.State {
	h.t.Helper()
	s, err := run.LoadState(h.runDir(runID))
	if err != nil {
		h.t.Fatal(err)
	}
	return s
}

func (h *harness) read(rel string) string {
	h.t.Helper()
	data, err := os.ReadFile(filepath.Join(h.root, rel))
	if err != nil {
		h.t.Fatal(err)
	}
	return string(data)
}

func (h *harness) exists(rel string) bool {
	_, err := os.Stat(filepath.Join(h.root, rel))
	return err == nil
}

func statusOf(s Summary, page string) string {
	for _, p := range s.Pages {
		if p.Path == page {
			return p.Status
		}
	}
	return "absent"
}

func ruleOf(s Summary, page string) string {
	for _, p := range s.Pages {
		if p.Path == page {
			return p.Rule
		}
	}
	return ""
}

// callError builds the API error the client returns for an HTTP status.
func callError(status int, typ, msg string) error {
	return &batch.CallError{Status: status, Type: typ, Message: msg}
}

func maybeSent() error {
	return &batch.CallError{Kind: batch.ErrMaybeSent, Cause: "connection reset"}
}
