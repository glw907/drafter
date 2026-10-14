package batch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// sentinelKey stands in for the API key; no real key shape appears in this package.
const sentinelKey = "SENTINEL-KEY-for-tests-only"

const testWorkspace = "wrkspc_test"

// seen is one request the fake API received.
type seen struct {
	method string
	path   string
	query  string
	header http.Header
	body   []byte
}

// api is an httptest server that records every request and answers the models probe itself.
type api struct {
	srv *httptest.Server

	mu   sync.Mutex
	reqs []seen

	// workspaceHeader is the anthropic-workspace-id the models probe answers with; nil omits it.
	workspaceHeader *string
}

func newAPI(t *testing.T, h http.HandlerFunc) *api {
	t.Helper()
	a := &api{}
	ws := testWorkspace
	a.workspaceHeader = &ws
	a.srv = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		a.mu.Lock()
		a.reqs = append(a.reqs, seen{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Clone(), body})
		a.mu.Unlock()
		r.Body = io.NopCloser(strings.NewReader(string(body)))
		if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
			if a.workspaceHeader != nil {
				w.Header().Set("anthropic-workspace-id", *a.workspaceHeader)
			}
			w.Header().Set("content-type", "application/json")
			io.WriteString(w, `{"data":[],"has_more":false,"first_id":null,"last_id":null}`)
			return
		}
		if h == nil {
			http.Error(w, "no handler", http.StatusInternalServerError)
			return
		}
		h(w, r)
	}))
	a.srv.Config.SetKeepAlivesEnabled(false)
	a.srv.Start()
	t.Cleanup(a.srv.Close)
	return a
}

// hits counts the requests to method and path, models probe included.
func (a *api) hits(method, path string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	n := 0
	for _, r := range a.reqs {
		if r.method == method && r.path == path {
			n++
		}
	}
	return n
}

func (a *api) total() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.reqs)
}

func (a *api) all() []seen {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]seen(nil), a.reqs...)
}

// last returns the newest request to method and path.
func (a *api) last(t *testing.T, method, path string) seen {
	t.Helper()
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := len(a.reqs) - 1; i >= 0; i-- {
		if a.reqs[i].method == method && a.reqs[i].path == path {
			return a.reqs[i]
		}
	}
	t.Fatalf("no %s %s request seen", method, path)
	return seen{}
}

// fakeClock is the injected backoff clock: Sleep advances Now and records the wait.
type fakeClock struct {
	mu    sync.Mutex
	now   time.Time
	slept []time.Duration
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) Sleep(ctx context.Context, d time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.slept = append(f.slept, d)
	f.now = f.now.Add(d)
	return ctx.Err()
}

func (f *fakeClock) waits() []time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]time.Duration(nil), f.slept...)
}

// clearEnv removes every variable the SDK or the key code could read, restoring them on cleanup.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "ANTHROPIC_") || strings.HasPrefix(k, "CLAUDE_CODE_") || k == "DRAFTER_API_KEY" {
			t.Setenv(k, "")
			os.Unsetenv(k)
		}
	}
}

func testOptions(a *api) Options {
	return Options{
		Key:            sentinelKey,
		BaseURL:        a.srv.URL,
		WorkspaceID:    testWorkspace,
		RequestTimeout: 2 * time.Second,
		StreamTimeout:  5 * time.Second,
		WallTime:       30 * time.Second,
	}
}

// newClient returns a client on a with the fake clock injected and the environment cleared.
func newClient(t *testing.T, a *api, mut func(*Options)) (*Client, *fakeClock) {
	t.Helper()
	clearEnv(t)
	o := testOptions(a)
	if mut != nil {
		mut(&o)
	}
	clk := &fakeClock{now: time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)}
	c := New(o)
	c.Sleep = clk.Sleep
	c.Now = clk.Now
	return c, clk
}

// checkedClient is newClient after a passing CheckWorkspace.
func checkedClient(t *testing.T, a *api, mut func(*Options)) (*Client, *fakeClock) {
	t.Helper()
	c, clk := newClient(t, a, mut)
	if err := c.CheckWorkspace(context.Background()); err != nil {
		t.Fatalf("CheckWorkspace: %v", err)
	}
	return c, clk
}

func sampleRequest(id string) Request {
	return Request{
		CustomID:  id,
		Model:     "test-model",
		System:    "You are a test.",
		Messages:  []Message{{Role: "user", Text: "hello"}},
		MaxTokens: 100,
		Effort:    "high",
		Schema:    json.RawMessage(`{"type":"object","properties":{"a":{"type":"string"}},"required":["a"],"additionalProperties":false}`),
	}
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	io.WriteString(w, body)
}

func errBody(typ, msg string) string {
	b, _ := json.Marshal(map[string]any{
		"type":       "error",
		"error":      map[string]any{"type": typ, "message": msg},
		"request_id": "req_test",
	})
	return string(b)
}

const spendLimitMsg = "You have reached your API usage limits: your organization has crossed its monthly API usage threshold."

func spendLimit429Body() string {
	return `{"type":"error","error":{"type":"rate_limit_error","message":"` + spendLimitMsg + `","details":{"error_code":"enforced_spend_limit_reached"}},"request_id":"req_test"}`
}

const (
	orgLimitMsg = "You have reached your specified API usage limits. You will regain access on 2026-11-01 at 00:00 UTC."
	wsLimitMsg  = "You have reached your specified workspace API usage limits. You will regain access on 2026-11-01 at 00:00 UTC."
)

// batchJSON renders a message batch.
func batchJSON(id, status string, created time.Time) string {
	ended := "null"
	if status == "ended" {
		ended = `"` + created.Add(time.Hour).Format(time.RFC3339) + `"`
	}
	return fmt.Sprintf(`{"id":%q,"type":"message_batch","processing_status":%q,"created_at":%q,"expires_at":%q,"ended_at":%s,"archived_at":null,"cancel_initiated_at":null,"results_url":null,"request_counts":{"processing":0,"succeeded":0,"errored":0,"canceled":0,"expired":0}}`,
		id, status, created.Format(time.RFC3339), created.Add(24*time.Hour).Format(time.RFC3339), ended)
}

// reset reads the request body, then closes the connection without answering.
func reset(w http.ResponseWriter, r *http.Request) {
	io.Copy(io.Discard, r.Body)
	conn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return
	}
	conn.Close()
}

func sseEvent(w http.ResponseWriter, event, data string) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	w.(http.Flusher).Flush()
}

func sseStart(w http.ResponseWriter) {
	w.Header().Set("content-type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	w.(http.Flusher).Flush()
}

const messageStart = `{"type":"message_start","message":{"id":"msg_test1","type":"message","role":"assistant","model":"test-model","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"cache_creation_input_tokens":5,"cache_read_input_tokens":3,"cache_creation":{"ephemeral_5m_input_tokens":2,"ephemeral_1h_input_tokens":3},"output_tokens":1}}}`

// goodStream writes a complete streamed message whose text is "Hello" and whose usage is
// {10 input, 7 output, 2 five-minute write, 3 one-hour write, 3 read}.
func goodStream(w http.ResponseWriter, stop string, pause time.Duration) {
	sseStart(w)
	sseEvent(w, "message_start", messageStart)
	sseEvent(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
	sseEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hel"}}`)
	time.Sleep(pause)
	sseEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"lo"}}`)
	time.Sleep(pause)
	sseEvent(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
	sseEvent(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"`+stop+`","stop_sequence":null},"usage":{"output_tokens":7}}`)
	sseEvent(w, "message_stop", `{"type":"message_stop"}`)
}
