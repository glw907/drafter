package smoke

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glw907/drafter/internal/batch"
	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/ledger"
)

const (
	testWorkspace = "wrkspc_smoke_test"
	testModel     = "test-model"
	countedTokens = 500
)

var wantSeats = []string{"draft", "structure", "editor", "fact", "reader", "coverage", "consistency"}

var syncUsage = batch.Usage{InputTokens: 77, OutputTokens: 9, CacheRead: 4}

// batchUsage is the usage the fake reports for the i-th request of a batch.
func batchUsage(i int) batch.Usage {
	return batch.Usage{InputTokens: int64(40 + i), OutputTokens: int64(20 + i), CacheWrite5m: 1, CacheWrite1h: 2, CacheRead: int64(i)}
}

type wireRequest struct {
	CustomID string `json:"custom_id"`
	Params   struct {
		MaxTokens    int64 `json:"max_tokens"`
		OutputConfig struct {
			Effort string `json:"effort"`
			Format struct {
				Schema map[string]any `json:"schema"`
			} `json:"format"`
		} `json:"output_config"`
	} `json:"params"`
}

type fakeBatch struct {
	reqs  []wireRequest
	polls int
}

// fake is an httptest stand-in for the Claude API.
type fake struct {
	srv *httptest.Server

	mu         sync.Mutex
	batches    map[string]*fakeBatch
	creates    int // accepted Create calls
	rejected   int // Create calls refused for repeated custom_ids
	pollsToEnd int
	hold       bool // batches never end
	failCreate bool // Create answers 500 after reading the body
	overCount  bool // results report more input than count_tokens did
	syncCalls  int  // streamed calls served; each gets its own message ID
}

func newFake(t *testing.T) *fake {
	t.Helper()
	f := &fake{batches: map[string]*fakeBatch{}, pollsToEnd: 2}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fake) set(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn()
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func errBody(typ, msg string) string {
	b, _ := json.Marshal(map[string]any{"type": "error", "error": map[string]any{"type": typ, "message": msg}})
	return string(b)
}

func batchJSON(id, status string) string {
	created := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	ended := "null"
	if status == "ended" {
		ended = `"` + created.Add(time.Hour).Format(time.RFC3339) + `"`
	}
	return fmt.Sprintf(`{"id":%q,"type":"message_batch","processing_status":%q,"created_at":%q,"expires_at":%q,"ended_at":%s,"archived_at":null,"cancel_initiated_at":null,"results_url":null,"request_counts":{"processing":0,"succeeded":0,"errored":0,"canceled":0,"expired":0}}`,
		id, status, created.Format(time.RFC3339), created.Add(24*time.Hour).Format(time.RFC3339), ended)
}

func (f *fake) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	defer f.mu.Unlock()
	path := r.URL.Path
	switch {
	case r.Method == http.MethodGet && path == "/v1/models":
		w.Header().Set("anthropic-workspace-id", testWorkspace)
		writeJSON(w, 200, `{"data":[],"has_more":false,"first_id":null,"last_id":null}`)
	case path == "/v1/messages/count_tokens":
		writeJSON(w, 200, fmt.Sprintf(`{"input_tokens":%d}`, countedTokens))
	case r.Method == http.MethodPost && path == "/v1/messages/batches":
		f.create(w, body)
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/results"):
		f.results(w, strings.TrimSuffix(strings.TrimPrefix(path, "/v1/messages/batches/"), "/results"))
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/v1/messages/batches/"):
		id := strings.TrimPrefix(path, "/v1/messages/batches/")
		b, ok := f.batches[id]
		if !ok {
			writeJSON(w, 404, errBody("not_found_error", "no such batch"))
			return
		}
		b.polls++
		status := "in_progress"
		if !f.hold && b.polls > f.pollsToEnd {
			status = "ended"
		}
		writeJSON(w, 200, batchJSON(id, status))
	case r.Method == http.MethodPost && path == "/v1/messages":
		f.stream(w, body)
	default:
		writeJSON(w, 404, errBody("not_found_error", path))
	}
}

func (f *fake) create(w http.ResponseWriter, body []byte) {
	var in struct {
		Requests []wireRequest `json:"requests"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		writeJSON(w, 400, errBody("invalid_request_error", err.Error()))
		return
	}
	seen := map[string]bool{}
	for _, r := range in.Requests {
		if seen[r.CustomID] {
			f.rejected++
			writeJSON(w, 400, errBody("invalid_request_error", "requests: custom_id must be unique for each request within the Message Batch"))
			return
		}
		seen[r.CustomID] = true
	}
	if f.failCreate {
		writeJSON(w, 500, errBody("api_error", "internal"))
		return
	}
	f.creates++
	id := fmt.Sprintf("msgbatch_%d", f.creates)
	f.batches[id] = &fakeBatch{reqs: in.Requests}
	writeJSON(w, 200, batchJSON(id, "in_progress"))
}

func (f *fake) results(w http.ResponseWriter, id string) {
	b, ok := f.batches[id]
	if !ok {
		writeJSON(w, 404, errBody("not_found_error", "no such batch"))
		return
	}
	w.Header().Set("content-type", "application/binary")
	for i, r := range b.reqs {
		u := batchUsage(i)
		if f.overCount {
			u.InputTokens = countedTokens + 1
		}
		text, _ := json.Marshal(instance(r.Params.OutputConfig.Format.Schema))
		msg := fmt.Sprintf(`{"id":"msg_%s_%s","type":"message","role":"assistant","model":%q,"content":[{"type":"text","text":%q}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":%d,"cache_creation_input_tokens":%d,"cache_read_input_tokens":%d,"cache_creation":{"ephemeral_5m_input_tokens":%d,"ephemeral_1h_input_tokens":%d},"output_tokens":%d}}`,
			id, r.CustomID, testModel, string(text), u.InputTokens, u.CacheWrite5m+u.CacheWrite1h, u.CacheRead, u.CacheWrite5m, u.CacheWrite1h, u.OutputTokens)
		_, _ = fmt.Fprintf(w, `{"custom_id":%q,"result":{"type":"succeeded","message":%s}}`+"\n", r.CustomID, msg)
	}
}

func (f *fake) stream(w http.ResponseWriter, body []byte) {
	f.syncCalls++
	var in wireRequest
	_ = json.Unmarshal(body, &in.Params)
	text, _ := json.Marshal(instance(in.Params.OutputConfig.Format.Schema))
	w.Header().Set("content-type", "text/event-stream")
	w.WriteHeader(200)
	ev := func(name, data string) {
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data)
	}
	ev("message_start", fmt.Sprintf(`{"type":"message_start","message":{"id":"msg_sync%d","type":"message","role":"assistant","model":%q,"content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":%d,"cache_creation_input_tokens":0,"cache_read_input_tokens":%d,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":0},"output_tokens":1}}}`,
		f.syncCalls, testModel, syncUsage.InputTokens, syncUsage.CacheRead))
	ev("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
	delta, _ := json.Marshal(map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": string(text)}})
	ev("content_block_delta", string(delta))
	ev("content_block_stop", `{"type":"content_block_stop","index":0}`)
	ev("message_delta", fmt.Sprintf(`{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":%d}}`, syncUsage.OutputTokens))
	ev("message_stop", `{"type":"message_stop"}`)
}

// instance builds a value that satisfies schema: required properties only, first enum value,
// and an empty array.
func instance(schema map[string]any) any {
	if e, ok := schema["enum"].([]any); ok && len(e) > 0 {
		return e[0]
	}
	switch schema["type"] {
	case "object":
		out := map[string]any{}
		props, _ := schema["properties"].(map[string]any)
		req, _ := schema["required"].([]any)
		for _, name := range req {
			k, _ := name.(string)
			sub, _ := props[k].(map[string]any)
			out[k] = instance(sub)
		}
		return out
	case "array":
		return []any{}
	case "integer":
		return 1
	case "boolean":
		return true
	}
	return "ok"
}

func (f *fake) batch(t *testing.T, id string) *fakeBatch {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.batches[id]
	if !ok {
		t.Fatalf("fake holds no batch %q", id)
	}
	return b
}

func prices() config.Prices {
	return config.Prices{
		Date: "2026-10-13", Source: "synthetic", BatchDiscount: 0.5,
		Models: map[string]config.Price{testModel: {Input: 1, Output: 5, CacheWrite5m: 1.25, CacheWrite1h: 2, CacheRead: 0.1}},
	}
}

func params(f *fake, stateDir string) Params {
	return Params{
		Options:      batch.Options{Key: "synthetic-test-key", BaseURL: f.srv.URL, WorkspaceID: testWorkspace, WallTime: 5 * time.Second},
		StateDir:     stateDir,
		Model:        testModel,
		Prices:       prices(),
		Caps:         ledger.Caps{Page: 100, Cycle: 1000, CycleStart: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
		PollInterval: time.Millisecond,
		Sleep:        func(context.Context, time.Duration) error { return nil },
	}
}

// ledgerLine is the part of a ledger line the tests read.
type ledgerLine struct {
	Type   string      `json:"type"`
	Repo   string      `json:"repo"`
	Seat   string      `json:"seat"`
	Intent string      `json:"intent"`
	Key    string      `json:"key"`
	Kind   string      `json:"kind"`
	Cost   float64     `json:"cost"`
	Usage  batch.Usage `json:"usage"`
}

func settleLines(t *testing.T, stateDir string) []ledgerLine {
	t.Helper()
	fh, err := os.Open(filepath.Join(stateDir, "ledger.jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fh.Close() }()
	var out []ledgerLine
	sc := bufio.NewScanner(fh)
	for sc.Scan() {
		var l ledgerLine
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			t.Fatalf("ledger line %q: %v", sc.Text(), err)
		}
		if l.Type == "settle" {
			out = append(out, l)
		}
	}
	return out
}

func TestShapes(t *testing.T) {
	shapes := Shapes()
	var seats []string
	for _, s := range shapes {
		seats = append(seats, s.Seat)
	}
	if !slices.Equal(seats, wantSeats) {
		t.Fatalf("seats = %v, want %v", seats, wantSeats)
	}
	for _, s := range shapes {
		t.Run(s.Seat, func(t *testing.T) {
			if s.MaxTokens != 4096 {
				t.Errorf("max_tokens = %d, want 4096", s.MaxTokens)
			}
			if s.Effort == "" || s.System == "" || s.Prompt == "" {
				t.Errorf("effort %q, system %q, or prompt %q is empty", s.Effort, s.System, s.Prompt)
			}
			var schema map[string]any
			if err := json.Unmarshal(s.Schema, &schema); err != nil {
				t.Fatalf("schema: %v", err)
			}
			if bad := openObjects(schema, "$"); len(bad) > 0 {
				t.Errorf("objects without additionalProperties false: %v", bad)
			}
			if err := Check(s, mustJSON(instance(schema))); err != nil {
				t.Errorf("a satisfying instance fails Check: %v", err)
			}
		})
	}
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// openObjects returns the paths of objects in a schema that do not set additionalProperties
// to false.
func openObjects(node any, path string) []string {
	var bad []string
	switch n := node.(type) {
	case map[string]any:
		if n["type"] == "object" && n["additionalProperties"] != false {
			bad = append(bad, path)
		}
		for k, v := range n {
			bad = append(bad, openObjects(v, path+"."+k)...)
		}
	case []any:
		for i, v := range n {
			bad = append(bad, openObjects(v, fmt.Sprintf("%s[%d]", path, i))...)
		}
	}
	return bad
}

func TestCheck(t *testing.T) {
	s := Shapes()[0]
	var schema map[string]any
	if err := json.Unmarshal(s.Schema, &schema); err != nil {
		t.Fatal(err)
	}
	good, ok := instance(schema).(map[string]any)
	if !ok {
		t.Fatal("instance of an object schema is not an object")
	}
	extra := map[string]any{"surprise": 1}
	maps.Copy(extra, good)
	tests := []struct {
		name string
		text string
		ok   bool
	}{
		{"satisfying", mustJSON(good), true},
		{"extra property", mustJSON(extra), false},
		{"missing required", "{}", false},
		{"not json", "{nope", false},
		{"not an object", `["a"]`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Check(s, tt.text)
			if (err == nil) != tt.ok {
				t.Errorf("Check = %v, want ok=%v", err, tt.ok)
			}
		})
	}
}

func TestRun(t *testing.T) {
	f := newFake(t)
	dir := t.TempDir()
	shapes := Shapes()
	rep, err := Run(context.Background(), params(f, dir), shapes)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rep.Problems) > 0 {
		t.Fatalf("problems: %v", rep.Problems)
	}

	t.Run("ledger gains one settle per request plus the sync request", func(t *testing.T) {
		lines := settleLines(t, dir)
		if len(lines) != len(shapes)+1 {
			t.Fatalf("settle lines = %d, want %d", len(lines), len(shapes)+1)
		}
		byKey := map[string]ledgerLine{}
		for _, l := range lines {
			if l.Repo != "drafter-smoke" {
				t.Errorf("repo = %q, want drafter-smoke", l.Repo)
			}
			if l.Kind != "actual" {
				t.Errorf("%s kind = %q, want actual", l.Key, l.Kind)
			}
			if l.Cost <= 0 {
				t.Errorf("%s cost = %v, want above zero", l.Key, l.Cost)
			}
			byKey[l.Key] = l
		}
		b := f.batch(t, rep.BatchID)
		for i, r := range b.reqs {
			l, ok := byKey[rep.BatchID+"/"+r.CustomID]
			if !ok {
				t.Errorf("no settle line for %s", r.CustomID)
				continue
			}
			if want := batchUsage(i); l.Usage != want {
				t.Errorf("%s usage = %+v, want %+v", r.CustomID, l.Usage, want)
			}
		}
		l, ok := byKey["msg_sync1"]
		if !ok {
			t.Fatalf("no settle line keyed on the sync message ID; keys %v", keysOf(byKey))
		}
		if l.Usage != syncUsage {
			t.Errorf("sync usage = %+v, want %+v", l.Usage, syncUsage)
		}
	})

	t.Run("seven distinct custom ids", func(t *testing.T) {
		b := f.batch(t, rep.BatchID)
		seen := map[string]bool{}
		for _, r := range b.reqs {
			seen[r.CustomID] = true
		}
		if len(b.reqs) != 7 || len(seen) != 7 {
			t.Errorf("requests = %d, distinct custom_ids = %d, want 7 and 7", len(b.reqs), len(seen))
		}
	})

	t.Run("each request carries its shape", func(t *testing.T) {
		b := f.batch(t, rep.BatchID)
		for i, r := range b.reqs {
			s := shapes[i]
			if r.Params.MaxTokens != 4096 {
				t.Errorf("%s max_tokens = %d", r.CustomID, r.Params.MaxTokens)
			}
			if r.Params.OutputConfig.Effort != s.Effort {
				t.Errorf("%s effort = %q, want %q", r.CustomID, r.Params.OutputConfig.Effort, s.Effort)
			}
			if bad := openObjects(r.Params.OutputConfig.Format.Schema, "$"); len(bad) > 0 {
				t.Errorf("%s sent open objects %v", r.CustomID, bad)
			}
		}
	})

	t.Run("each result parses to its schema and stays within the counted input", func(t *testing.T) {
		if len(rep.Results) != len(shapes) {
			t.Fatalf("results = %d, want %d", len(rep.Results), len(shapes))
		}
		bySeat := map[string]Shape{}
		for _, s := range shapes {
			bySeat[s.Seat] = s
		}
		for _, r := range rep.Results {
			s, ok := bySeat[r.CustomID]
			if !ok {
				t.Errorf("result for unknown custom_id %q", r.CustomID)
				continue
			}
			if err := Check(s, r.Text); err != nil {
				t.Errorf("%s: %v", r.CustomID, err)
			}
			if r.Usage.InputTokens > rep.Counted[r.CustomID] || rep.Counted[r.CustomID] != countedTokens {
				t.Errorf("%s input_tokens %d, counted %d", r.CustomID, r.Usage.InputTokens, rep.Counted[r.CustomID])
			}
		}
		if err := Check(shapes[0], rep.Sync.Text); err != nil {
			t.Errorf("sync result: %v", err)
		}
	})

	t.Run("nothing is left open and the record is settled", func(t *testing.T) {
		open, err := ledger.Open(dir).OpenIntents(rep.RunID)
		if err != nil || len(open) != 0 {
			t.Errorf("open intents = %v, err %v", open, err)
		}
		rec := readRecord(t, dir, rep.RunID)
		if rec.Batch.BatchID != rep.BatchID || !rec.Batch.Settled || !rec.Sync.Settled {
			t.Errorf("record = %+v", rec)
		}
	})
}

func keysOf(m map[string]ledgerLine) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	slices.Sort(ks)
	return ks
}

func readRecord(t *testing.T, dir, runID string) record {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "smoke", runID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var rec record
	if err := json.Unmarshal(b, &rec); err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestRunProblems(t *testing.T) {
	f := newFake(t)
	f.set(func() { f.overCount = true })
	rep, err := Run(context.Background(), params(f, t.TempDir()), Shapes())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rep.Problems) != 7 {
		t.Errorf("problems = %v, want one per request over its counted figure", rep.Problems)
	}
}

func TestFakeRejectsRepeatedCustomIDs(t *testing.T) {
	f := newFake(t)
	c := batch.New(params(f, t.TempDir()).Options)
	if err := c.CheckWorkspace(context.Background()); err != nil {
		t.Fatal(err)
	}
	mk := func(id string) batch.Request {
		s := Shapes()[0]
		return batch.Request{CustomID: id, Model: testModel, Messages: []batch.Message{{Role: "user", Text: s.Prompt}}, MaxTokens: 4096, Schema: s.Schema}
	}
	tests := []struct {
		name string
		ids  []string
		ok   bool
	}{
		{"distinct", []string{"a", "b"}, true},
		{"repeated", []string{"a", "a"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rs []batch.Request
			for _, id := range tt.ids {
				rs = append(rs, mk(id))
			}
			_, err := c.Create(context.Background(), rs)
			if (err == nil) != tt.ok {
				t.Errorf("Create err = %v, want ok=%v", err, tt.ok)
			}
		})
	}
	if f.rejected != 1 {
		t.Errorf("fake rejected %d creates, want 1", f.rejected)
	}
}

func TestResume(t *testing.T) {
	t.Run("an unsettled batch is settled once by the next run", func(t *testing.T) {
		f := newFake(t)
		f.set(func() { f.hold = true })
		dir := t.TempDir()
		p := params(f, dir)
		ctx, cancel := context.WithCancel(context.Background())
		p.Sleep = func(ctx context.Context, _ time.Duration) error { cancel(); return ctx.Err() }
		if _, err := Run(ctx, p, Shapes()); !errors.Is(err, context.Canceled) {
			t.Fatalf("first Run err = %v, want context.Canceled", err)
		}
		if n := len(settleLines(t, dir)); n != 0 {
			t.Fatalf("settle lines after the interrupted run = %d, want 0", n)
		}

		f.set(func() { f.hold = false })
		rep, err := Run(context.Background(), params(f, dir), Shapes())
		if err != nil {
			t.Fatalf("second Run: %v", err)
		}
		if !slices.Equal(rep.Resumed, []string{"msgbatch_1"}) {
			t.Errorf("resumed = %v, want [msgbatch_1]", rep.Resumed)
		}
		count := func() (old, all int) {
			for _, l := range settleLines(t, dir) {
				all++
				if strings.HasPrefix(l.Key, "msgbatch_1/") {
					old++
				}
			}
			return old, all
		}
		if old, all := count(); old != 7 || all != 7+8 {
			t.Errorf("after resume: old-batch lines %d, all %d, want 7 and 15", old, all)
		}
		rep, err = Run(context.Background(), params(f, dir), Shapes())
		if err != nil {
			t.Fatalf("third Run: %v", err)
		}
		if len(rep.Resumed) != 0 {
			t.Errorf("third run resumed %v, want nothing", rep.Resumed)
		}
		if old, all := count(); old != 7 || all != 7+8+8 {
			t.Errorf("after third run: old-batch lines %d, all %d, want 7 and 23", old, all)
		}
	})

	t.Run("an intent that never reached the API is voided", func(t *testing.T) {
		f := newFake(t)
		f.set(func() { f.failCreate = true })
		dir := t.TempDir()
		_, err := Run(context.Background(), params(f, dir), Shapes())
		if !errors.Is(err, batch.ErrMaybeSent) {
			t.Fatalf("first Run err = %v, want ErrMaybeSent", err)
		}
		files, _ := filepath.Glob(filepath.Join(dir, "smoke", "*.json"))
		if len(files) != 1 {
			t.Fatalf("record files = %v", files)
		}
		runID := strings.TrimSuffix(filepath.Base(files[0]), ".json")
		if open, _ := ledger.Open(dir).OpenIntents(runID); len(open) != 1 {
			t.Fatalf("open intents = %v, want the one reservation", open)
		}

		f.set(func() { f.failCreate = false })
		rep, err := Run(context.Background(), params(f, dir), Shapes())
		if err != nil {
			t.Fatalf("second Run: %v", err)
		}
		if len(rep.Voided) != 1 {
			t.Errorf("voided = %v, want one intent", rep.Voided)
		}
		if open, _ := ledger.Open(dir).OpenIntents(runID); len(open) != 0 {
			t.Errorf("open intents after resume = %v, want none", open)
		}
		var voids int
		for _, l := range settleLines(t, dir) {
			if l.Kind == "void" {
				voids++
				if l.Cost != 0 || !strings.HasSuffix(l.Key, "/void") {
					t.Errorf("void line = %+v", l)
				}
			}
		}
		if voids != 1 {
			t.Errorf("void lines = %d, want 1", voids)
		}
	})
}
