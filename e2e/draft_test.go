package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/glw907/drafter/internal/blueprint"
)

const (
	draftPage  = "docs/count-items.md"
	draftBP    = "blueprints/docs/count-items.blueprint.yaml"
	draftBrief = "blueprints/docs/count-items.brief.json"
	draftLock  = "blueprints/drafted.lock"
	draftFacts = "blueprints/facts/tally.md"
	draftOld   = "Pick the items you want to count."
	draftNew   = "Pick the items you want to count, and have them ready."
)

// draftStub is a scripted Messages API. It answers a request by the output schema the request
// carries, so each seat gets a well-formed answer without the stub reading a prompt. The answers
// are canned files under testdata/cli-answers, and the test sets the edits answer per row.
type draftStub struct {
	*httptest.Server
	mu       sync.Mutex
	kinds    []string
	bodies   []string
	answers  map[string]string
	failFrom int // refuse every Messages request from the nth on, as a spend limit does
}

func newDraftStub(t *testing.T) *draftStub {
	t.Helper()
	s := &draftStub{answers: map[string]string{}}
	answer := func(name string) string {
		data, err := os.ReadFile(filepath.Join("testdata", "cli-answers", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	page, brief := answer("page.md"), answer("brief.json")
	draft, err := json.Marshal(map[string]any{"page": page, "brief": brief, "friction": []string{}})
	if err != nil {
		t.Fatal(err)
	}
	s.answers["draft"] = string(draft)
	s.answers["read"] = `{"verdict":"ok","findings":[],"summary":"read","friction":[]}`
	s.answers["reader1"] = `{"paraphrase":"The page says how to count items with tally.","summary":"counting","friction":[]}`
	s.answers["consistency"] = `{"conflicts":[]}`
	s.answers["coverage"] = answer("coverage.json")
	s.answers["edits"] = answer("edits-sentence.json")
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("anthropic-workspace-id", "wrkspc_test")
			_, _ = w.Write([]byte(`{"data":[],"has_more":false,"first_id":null,"last_id":null}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/messages/count_tokens":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"input_tokens":2000}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/messages":
			body, _ := io.ReadAll(r.Body)
			kind := schemaKind(body)
			s.mu.Lock()
			s.kinds = append(s.kinds, kind)
			s.bodies = append(s.bodies, string(body))
			refuse := s.failFrom > 0 && len(s.kinds) >= s.failFrom
			text := s.answers[kind]
			s.mu.Unlock()
			if kind == "facttruth" && text == "" {
				text = holdsAnswer(body)
			}
			if refuse {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"You have reached your specified workspace API usage limits."}}`))
				return
			}
			if text == "" {
				http.Error(w, "no canned answer for "+kind, http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			quoted, _ := json.Marshal(text)
			for _, ev := range [][2]string{
				{"message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5-5","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":2000,"output_tokens":1}}}`},
				{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
				{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":` + string(quoted) + `}}`},
				{"content_block_stop", `{"type":"content_block_stop","index":0}`},
				{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":300}}`},
				{"message_stop", `{"type":"message_stop"}`},
			} {
				_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev[0], ev[1])
			}
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

var factListed = regexp.MustCompile("(?:\\\\n|\n)- `([^`]+)` ")

// holdsAnswer is the fact-truth answer that finds true every fact the request body lists.
func holdsAnswer(body []byte) string {
	facts := []map[string]string{}
	for _, m := range factListed.FindAllSubmatch(body, -1) {
		facts = append(facts, map[string]string{"id": string(m[1]), "verdict": "holds", "excerpt": "", "fix": ""})
	}
	b, _ := json.Marshal(map[string]any{"facts": facts})
	return string(b)
}

// schemaKind names the answer a request asks for by the properties of its output schema.
func schemaKind(body []byte) string {
	var req struct {
		OutputConfig struct {
			Format struct {
				Schema struct {
					Properties map[string]json.RawMessage `json:"properties"`
				} `json:"schema"`
			} `json:"format"`
		} `json:"output_config"`
	}
	if json.Unmarshal(body, &req) != nil {
		return "unknown"
	}
	p := req.OutputConfig.Format.Schema.Properties
	switch {
	case p["page"] != nil && p["brief"] != nil:
		return "draft"
	case p["edits"] != nil && p["resolutions"] != nil:
		return "edits"
	case p["verdict"] != nil:
		return "read"
	case p["paraphrase"] != nil:
		return "reader1"
	case p["conflicts"] != nil:
		return "consistency"
	case p["facts"] != nil:
		return "facttruth"
	case p["items"] != nil:
		return "coverage"
	}
	return "unknown"
}

func (s *draftStub) sent() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.kinds)
}

// sentBodies returns the body of every Messages request, in the order sent.
func (s *draftStub) sentBodies() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.bodies)
}

func (s *draftStub) reset(failFrom int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.kinds, s.bodies, s.failFrom = nil, nil, failFrom
}

func (s *draftStub) setAnswer(kind, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.answers[kind] = text
}

// draftEnv is the fixture in a fresh git repository, run by the built binary against a stub.
type draftEnv struct {
	t     *testing.T
	dir   string
	state string
	env   []string
	stub  *draftStub
}

func newDraftEnv(t *testing.T) *draftEnv {
	t.Helper()
	e := &draftEnv{t: t, stub: newDraftStub(t), state: t.TempDir()}
	e.dir = filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(e.dir, 0o755); err != nil {
		t.Fatal(err)
	}
	copyDir(t, filepath.Join("testdata", "cli"), e.dir)
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-q", "-m", "fixture"}} {
		gitRun(t, e.dir, args...)
	}
	xdg := t.TempDir()
	if err := os.MkdirAll(filepath.Join(xdg, "drafter"), 0o755); err != nil {
		t.Fatal(err)
	}
	user := fmt.Sprintf("workspace_id = \"wrkspc_test\"\npage_cap = 10\ncycle_ceiling = 100\nkey_env = \"DRAFTER_API_KEY\"\napi_base_url = %q\nstate_dir = %q\n", e.stub.URL, e.state)
	if err := os.WriteFile(filepath.Join(xdg, "drafter", "config.toml"), []byte(user), 0o644); err != nil {
		t.Fatal(err)
	}
	// The stock gate shells out to drafter, so the built binary's directory leads PATH.
	e.env = []string{
		"XDG_CONFIG_HOME=" + xdg, "DRAFTER_API_KEY=" + pageKey,
		"PATH=" + filepath.Dir(binary) + string(os.PathListSeparator) + os.Getenv("PATH"),
	}
	return e
}

func (e *draftEnv) drafter(args ...string) result {
	e.t.Helper()
	return runEnv(e.t, e.dir, e.env, args...)
}

func (e *draftEnv) page(args ...string) result {
	e.t.Helper()
	return e.drafter(append([]string{"page", "--sync", "--json"}, args...)...)
}

func (e *draftEnv) read(rel string) string {
	e.t.Helper()
	data, err := os.ReadFile(filepath.Join(e.dir, filepath.FromSlash(rel)))
	if err != nil {
		e.t.Fatal(err)
	}
	return string(data)
}

func (e *draftEnv) write(rel, body string) {
	e.t.Helper()
	writeFile(e.t, filepath.Join(e.dir, filepath.FromSlash(rel)), body)
}

func (e *draftEnv) commit(msg string, files ...string) {
	e.t.Helper()
	gitRun(e.t, e.dir, append([]string{"add"}, files...)...)
	gitRun(e.t, e.dir, "commit", "-q", "-m", msg)
}

// draftSummary is the part of drafter page --json the rows read.
type draftSummary struct {
	RunID string `json:"run_id"`
	Stop  string `json:"stop"`
	Pages []struct {
		Path     string  `json:"path"`
		Status   string  `json:"status"`
		Rule     string  `json:"rule"`
		Accepted string  `json:"accepted"`
		Cost     float64 `json:"cost"`
		Scope    *struct {
			Kind string   `json:"kind"`
			Why  string   `json:"why"`
			IDs  []string `json:"ids"`
		} `json:"scope"`
	} `json:"pages"`
}

func (e *draftEnv) summary(r result) draftSummary {
	e.t.Helper()
	var s draftSummary
	if err := json.Unmarshal([]byte(r.stdout), &s); err != nil {
		e.t.Fatalf("no JSON summary (exit %d): %v\nstdout: %s\nstderr: %s", r.code, err, r.stdout, r.stderr)
	}
	return s
}

func (e *draftEnv) lockEntry() blueprint.LockEntry {
	e.t.Helper()
	l, err := blueprint.ReadLock(filepath.Join(e.dir, draftLock))
	if err != nil {
		e.t.Fatal(err)
	}
	return l.Pages[draftPage]
}

func (e *draftEnv) runDirs() []string {
	e.t.Helper()
	entries, err := os.ReadDir(filepath.Join(e.state, "runs", filepath.Base(e.dir)))
	if err != nil {
		return nil
	}
	var ids []string
	for _, en := range entries {
		ids = append(ids, en.Name())
	}
	return ids
}

// git runs git in the project and returns its trimmed output.
func (e *draftEnv) git(args ...string) string {
	e.t.Helper()
	out, err := exec.Command("git", append([]string{"-C", e.dir}, args...)...).Output()
	if err != nil {
		e.t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

// gate checks that the stock gate passes on the page and ran provenance.
func (e *draftEnv) gate() {
	e.t.Helper()
	r := e.drafter("gate", "--page", draftPage)
	if r.code != 0 {
		e.t.Fatalf("drafter gate exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	out := r.stdout + r.stderr
	if strings.Contains(out, "provenance check skipped") || !strings.Contains(out, "provenance:") {
		e.t.Errorf("the gate did not run provenance:\n%s", out)
	}
}

// acceptedPage fails the test unless the summary holds the page, accepted, and returns it.
func acceptedPage(t *testing.T, s draftSummary) (scopeKind, why string, ids []string) {
	t.Helper()
	for _, p := range s.Pages {
		if p.Path != draftPage {
			continue
		}
		if p.Status != "accepted" {
			t.Fatalf("%s is %s (%s)", draftPage, p.Status, p.Rule)
		}
		if p.Scope == nil {
			return "", "", nil
		}
		return p.Scope.Kind, p.Scope.Why, p.Scope.IDs
	}
	t.Fatalf("the summary has no %s: %+v", draftPage, s)
	return "", "", nil
}
