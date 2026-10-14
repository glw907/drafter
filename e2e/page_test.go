package e2e

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// pageKey is a sentinel shaped like an API key. No output of the binary may hold it.
var pageKey = "sk-" + "ant-api03-E2E-SENTINEL-do-not-leak"

const (
	guidePage  = "docs/guides/rotate-the-api-key.md"
	guideBrief = "docs/guides/rotate-the-api-key.brief.json"
	guidePlan  = "plans/rotate-the-api-key.clean.plan.yaml"
)

// apiStub is an httptest server that answers the two free calls a dry run makes and records
// every other path, so a test can assert the run submitted nothing.
type apiStub struct {
	*httptest.Server
	mu    sync.Mutex
	paths []string
}

func newAPIStub(t *testing.T) *apiStub {
	t.Helper()
	s := &apiStub{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.paths = append(s.paths, r.Method+" "+r.URL.Path)
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
			w.Header().Set("anthropic-workspace-id", "wrkspc_test")
			_, _ = w.Write([]byte(`{"data":[],"has_more":false,"first_id":null,"last_id":null}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/messages/count_tokens":
			if got := r.Header.Get("anthropic-workspace-id"); got != "wrkspc_test" {
				http.Error(w, "wrong workspace header "+got, http.StatusBadRequest)
				return
			}
			_, _ = fmt.Fprintf(w, `{"input_tokens":%d}`, countedTokens)
		default:
			http.Error(w, `{"type":"error","error":{"type":"not_found_error","message":"unexpected"}}`, http.StatusNotFound)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

// submitted lists the requests the stub saw beyond the workspace check and the token count.
func (s *apiStub) submitted() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, p := range s.paths {
		if p != "GET /v1/models" && p != "POST /v1/messages/count_tokens" {
			out = append(out, p)
		}
	}
	return out
}

// countedTokens is what the stub reports for every count_tokens call.
const countedTokens = 5000

// pageEnv is a project copy in a git repository with a user configuration pointing at a stub.
type pageEnv struct {
	dir, state, xdg string
	env             []string
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func copyDir(t *testing.T, from, to string) {
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

func newPageEnv(t *testing.T, stub *apiStub) pageEnv {
	t.Helper()
	p := pageEnv{dir: filepath.Join(t.TempDir(), "project"), state: t.TempDir(), xdg: t.TempDir()}
	if err := os.MkdirAll(p.dir, 0o755); err != nil {
		t.Fatal(err)
	}
	copyDir(t, project(t), p.dir)
	writeFile(t, filepath.Join(p.dir, "drafter.toml"), `prompts = "prompts.js"
seats = ["draft"]

[gate]
command = "true"

[seat.draft]
model = "model-a"
effort = "high"
max_tokens = 4000
`)
	writeFile(t, filepath.Join(p.dir, "prompts.js"), `return { draft: (c) => "Draft " + c.page.path };`+"\n")
	gitRun(t, p.dir, "init", "-q", "-b", "main")
	gitRun(t, p.dir, "add", "-A")
	gitRun(t, p.dir, "commit", "-q", "-m", "fixture")

	cfgDir := filepath.Join(p.xdg, "drafter")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(cfgDir, "config.toml"), fmt.Sprintf(`workspace_id = "wrkspc_test"
page_cap = 10
cycle_ceiling = 100
api_base_url = %q
state_dir = %q

[prices.models.model-a]
input = 1
output = 5
`, stub.URL, p.state))
	p.env = []string{"XDG_CONFIG_HOME=" + p.xdg, "DRAFTER_API_KEY=" + pageKey}
	return p
}

// handoffFile writes the handoff file for the guide page and returns its path.
func (p pageEnv) handoffFile(t *testing.T) string {
	t.Helper()
	plan, err := os.ReadFile(filepath.Join(p.dir, guidePlan))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(plan)
	h := map[string]any{
		"version": 1, "project": p.dir,
		"page":   map[string]string{"path": guidePage, "brief": guideBrief, "type": "task-guide"},
		"plan":   guidePlan,
		"inputs": pageInputs(),
		"context": map[string]any{
			"args":     map[string]string{"outline": "docs/outline.json"},
			"resolved": resolvedPages(),
		},
		"checksum": "sha256:" + hex.EncodeToString(sum[:]),
	}
	path := filepath.Join(t.TempDir(), "handoff.json")
	writeJSONFile(t, path, h)
	return path
}

// workflowRunFile writes a chain Workflow run file carrying the same page.
func (p pageEnv) workflowRunFile(t *testing.T) string {
	t.Helper()
	run := map[string]any{"result": map[string]any{"handoff": map[string]any{
		"version": 1, "worktree": p.dir,
		"args":     map[string]string{"outline": "docs/outline.json"},
		"resolved": resolvedPages(),
		"pages": []map[string]any{{
			"page":    map[string]string{"path": guidePage, "brief": guideBrief, "type": "task-guide"},
			"plan":    guidePlan,
			"framing": "",
			"inputs":  pageInputs(),
		}},
	}}}
	path := filepath.Join(t.TempDir(), "wf_run.json")
	writeJSONFile(t, path, run)
	return path
}

func pageInputs() map[string]any {
	return map[string]any{
		"job": "Rotate a key", "pageType": "task-guide",
		"registerSections": []string{}, "exemplarSources": []string{}, "claimInventory": []string{},
	}
}

func resolvedPages() map[string]any {
	return map[string]any{
		"index": "docs/index.md",
		"pages": []map[string]string{{"path": guidePage, "job": "Rotate a key"}},
	}
}

func writeJSONFile(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, string(data))
}

type dryResult struct {
	Pages []struct {
		Path, Status, Rule string
	} `json:"pages"`
	Estimates []struct {
		Seat      string  `json:"seat"`
		Tokens    int64   `json:"tokens"`
		MaxTokens int64   `json:"max_tokens"`
		Worst     float64 `json:"worst"`
	} `json:"estimates"`
	EstimateTotal float64 `json:"estimate_total"`
}

func (p pageEnv) page(t *testing.T, args ...string) result {
	t.Helper()
	return runEnv(t, p.dir, p.env, append([]string{"page"}, args...)...)
}

func parseDry(t *testing.T, r result) dryResult {
	t.Helper()
	var d dryResult
	if err := json.Unmarshal([]byte(r.stdout), &d); err != nil {
		t.Fatalf("stdout is not the dry run's JSON: %v\n%s\n%s", err, r.stdout, r.stderr)
	}
	return d
}

// A draft request of 5000 counted tokens with max_tokens 4000 at $1 in and $5 out per million
// tokens is 5000 x 1/1e6 + 4000 x 5/1e6 = $0.025 in batch mode with no discount set.
const handFigure = 0.025

func TestPageDryRunEstimatesTheFirstStage(t *testing.T) {
	stub := newAPIStub(t)
	p := newPageEnv(t, stub)
	r := p.page(t, "--dry-run", "--json", p.handoffFile(t))
	if r.code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", r.code, r.stdout, r.stderr)
	}
	d := parseDry(t, r)
	if len(d.Estimates) != 1 || d.Estimates[0].Seat != "draft" || d.Estimates[0].Tokens != 5000 {
		t.Fatalf("estimates = %+v, want the one draft request of 5000 tokens", d.Estimates)
	}
	if math.Abs(d.Estimates[0].Worst-handFigure) > 1e-9 || math.Abs(d.EstimateTotal-handFigure) > 1e-9 {
		t.Errorf("worst %v, total %v, want %v", d.Estimates[0].Worst, d.EstimateTotal, handFigure)
	}
	if got := stub.submitted(); len(got) != 0 {
		t.Errorf("the dry run sent %v", got)
	}
	for _, name := range []string{"ledger.jsonl", "runs"} {
		if _, err := os.Stat(filepath.Join(p.state, name)); err == nil {
			t.Errorf("the dry run left %s in the state directory", name)
		}
	}
	if strings.Contains(r.stdout+r.stderr, pageKey) {
		t.Error("the dry run's output holds the key sentinel")
	}
}

func TestPageDryRunTextOutput(t *testing.T) {
	stub := newAPIStub(t)
	p := newPageEnv(t, stub)
	r := p.page(t, "--dry-run", p.handoffFile(t))
	if r.code != 0 {
		t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	for _, want := range []string{"draft: 5000 input tokens, max_tokens 4000, worst case $0.0250", "estimated worst case $0.0250"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, r.stdout)
		}
	}
}

func TestPageWorkflowRunMatchesTheHandoffFile(t *testing.T) {
	stub := newAPIStub(t)
	p := newPageEnv(t, stub)
	fromFile := parseDry(t, p.page(t, "--dry-run", "--json", p.handoffFile(t)))
	fromRun := parseDry(t, p.page(t, "--dry-run", "--json", "--workflow-run", p.workflowRunFile(t)))
	if fromRun.EstimateTotal != fromFile.EstimateTotal || len(fromRun.Estimates) != len(fromFile.Estimates) {
		t.Errorf("workflow run estimate %+v differs from the handoff file's %+v", fromRun, fromFile)
	}
}

func TestPageRefusesAnUncleanWorktree(t *testing.T) {
	stub := newAPIStub(t)
	t.Run("a touched tracked file", func(t *testing.T) {
		p := newPageEnv(t, stub)
		touched := "docs/reference/keys.md"
		writeFile(t, filepath.Join(p.dir, touched), "changed\n")
		r := p.page(t, "--dry-run", p.handoffFile(t))
		if r.code != 2 || !strings.Contains(r.stderr, touched) {
			t.Errorf("exit %d, stderr %q, want 2 naming %s", r.code, r.stderr, touched)
		}
	})
	t.Run("an untracked file", func(t *testing.T) {
		p := newPageEnv(t, stub)
		writeFile(t, filepath.Join(p.dir, "scratch.txt"), "x")
		r := p.page(t, "--dry-run", p.handoffFile(t))
		if r.code != 2 || !strings.Contains(r.stderr, "scratch.txt") {
			t.Errorf("exit %d, stderr %q, want 2 naming scratch.txt", r.code, r.stderr)
		}
	})
	t.Run("a clean worktree", func(t *testing.T) {
		p := newPageEnv(t, stub)
		if r := p.page(t, "--dry-run", p.handoffFile(t)); r.code != 0 {
			t.Errorf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
		}
	})
}

func TestPageUsage(t *testing.T) {
	stub := newAPIStub(t)
	p := newPageEnv(t, stub)
	if r := p.page(t); r.code != 2 || !strings.Contains(r.stderr, "handoff") {
		t.Errorf("no inputs: exit %d, stderr %q", r.code, r.stderr)
	}
	if r := p.page(t, "--workflow-run", "x.json", "y.json"); r.code != 2 {
		t.Errorf("both inputs: exit %d", r.code)
	}
}
