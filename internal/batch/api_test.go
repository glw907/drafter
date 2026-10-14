package batch

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const resultsBody = `{"custom_id":"p1-a","result":{"type":"succeeded","message":{"id":"msg_a","type":"message","role":"assistant","model":"test-model","content":[{"type":"text","text":"{\"a\":\"x\"}"}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":11,"cache_creation_input_tokens":5,"cache_read_input_tokens":3,"cache_creation":{"ephemeral_5m_input_tokens":2,"ephemeral_1h_input_tokens":3},"output_tokens":7}}}}
{"custom_id":"p1-b","result":{"type":"errored","error":{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"},"request_id":"req_x"}}}
{"custom_id":"p1-c","result":{"type":"expired"}}
`

func TestBatchRoundTrip(t *testing.T) {
	ctx := context.Background()
	var polls atomic.Int32
	created := time.Date(2026, 10, 13, 11, 0, 0, 0, time.UTC)
	a := newAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST":
			writeJSON(w, 200, batchJSON("msgbatch_1", "in_progress", created))
		case strings.HasSuffix(r.URL.Path, "/results"):
			w.Header().Set("content-type", "application/x-jsonl")
			w.Write([]byte(resultsBody))
		default:
			status := "in_progress"
			if polls.Add(1) >= 3 {
				status = "ended"
			}
			writeJSON(w, 200, batchJSON("msgbatch_1", status, created))
		}
	})
	c, _ := checkedClient(t, a, nil)

	b, err := c.Create(ctx, []Request{sampleRequest("p1-a"), sampleRequest("p1-b"), sampleRequest("p1-c")})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if b.ID != "msgbatch_1" || b.Ended || !b.CreatedAt.Equal(created) {
		t.Errorf("Create = %+v, want msgbatch_1 in progress created %v", b, created)
	}
	for i := 0; !b.Ended; i++ {
		if i > 5 {
			t.Fatal("batch never ended")
		}
		if b, err = c.Get(ctx, b.ID); err != nil {
			t.Fatalf("Get: %v", err)
		}
	}

	got, err := c.Results(ctx, b.ID)
	if err != nil {
		t.Fatalf("Results: %v", err)
	}
	want := []Result{
		{
			CustomID: "p1-a", MessageID: "msg_a", Kind: KindSucceeded, StopReason: "end_turn", Text: `{"a":"x"}`,
			Usage: Usage{InputTokens: 11, OutputTokens: 7, CacheWrite5m: 2, CacheWrite1h: 3, CacheRead: 3},
		},
		{CustomID: "p1-b", Kind: KindErrored, ErrType: "overloaded_error", ErrMessage: "Overloaded"},
		{CustomID: "p1-c", Kind: KindExpired},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Results = %+v\nwant %+v", got, want)
	}
}

func TestList(t *testing.T) {
	ctx := context.Background()
	since := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	margin := 10 * time.Minute
	// Newest first, as the API serves them, two to a page.
	served := []struct {
		id      string
		created time.Time
	}{
		{"b1", since.Add(2 * time.Hour)},
		{"b2", since.Add(time.Hour)},
		{"b3", since},
		{"b4", since.Add(-30 * time.Second)},
		{"b5", since.Add(-margin)},
		{"b6", since.Add(-margin - time.Second)},
		{"b7", since.Add(-2 * time.Hour)},
	}
	var pages atomic.Int32
	a := newAPI(t, func(w http.ResponseWriter, r *http.Request) {
		pages.Add(1)
		// The fake ignores every filter and the page size; it honors only the cursor.
		start := 0
		if after := r.URL.Query().Get("after_id"); after != "" {
			for i, s := range served {
				if s.id == after {
					start = i + 1
				}
			}
		}
		end := min(start+2, len(served))
		var items []string
		for _, s := range served[start:end] {
			items = append(items, batchJSON(s.id, "ended", s.created))
		}
		last := "null"
		if end > start {
			last = `"` + served[end-1].id + `"`
		}
		writeJSON(w, 200, `{"data":[`+strings.Join(items, ",")+`],"has_more":`+boolStr(end < len(served))+`,"first_id":null,"last_id":`+last+`}`)
	})
	c, _ := checkedClient(t, a, nil)
	got, err := c.List(ctx, since)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var ids []string
	for _, b := range got {
		ids = append(ids, b.ID)
	}
	if want := []string{"b1", "b2", "b3", "b4", "b5"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("List ids = %v, want %v", ids, want)
	}
	if n := pages.Load(); n != 3 {
		t.Errorf("fake served %d pages, want 3 (stop once a page's oldest predates the margin)", n)
	}
	for _, b := range got {
		if !b.Ended {
			t.Errorf("batch %s not marked ended", b.ID)
		}
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func TestSend(t *testing.T) {
	a := newAPI(t, func(w http.ResponseWriter, r *http.Request) { goodStream(w, "end_turn", 0) })
	c, _ := checkedClient(t, a, nil)
	req := sampleRequest("sync-1")
	res, err := c.Send(context.Background(), req)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	want := Result{
		CustomID: "sync-1", MessageID: "msg_test1", Kind: KindSucceeded, StopReason: "end_turn", Text: "Hello",
		Usage: Usage{InputTokens: 10, OutputTokens: 7, CacheWrite5m: 2, CacheWrite1h: 3, CacheRead: 3},
	}
	if res != want {
		t.Errorf("Send = %+v\nwant %+v", res, want)
	}
	body := decodeBody(t, a.last(t, "POST", "/v1/messages").body)
	if body["stream"] != true {
		t.Errorf("stream = %v, want true", body["stream"])
	}
}

func TestSendHTTPErrorsAreResults(t *testing.T) {
	tests := []struct {
		code    int
		typ     string
		wantAct Action
	}{
		{500, "api_error", ActionResubmit},
		{529, "overloaded_error", ActionResubmit},
		{400, "invalid_request_error", ActionEscalate},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.code), func(t *testing.T) {
			a := newAPI(t, status(tt.code, tt.typ))
			c, _ := checkedClient(t, a, nil)
			res, err := c.Send(context.Background(), sampleRequest("s"))
			if err != nil {
				t.Fatalf("Send err = %v, want an errored Result", err)
			}
			if res.Kind != KindErrored || res.ErrType != tt.typ || res.ErrMessage != "synthetic "+tt.typ || res.CustomID != "s" {
				t.Errorf("Send = %+v, want errored %s", res, tt.typ)
			}
			if got := Classify(res); got != tt.wantAct {
				t.Errorf("Classify = %v, want %v", got, tt.wantAct)
			}
			if n := a.hits("POST", "/v1/messages"); n != 1 {
				t.Errorf("server saw %d Send requests, want 1", n)
			}
		})
	}
}

func decodeBody(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("request body is not JSON: %v\n%s", err, b)
	}
	return m
}

func TestWireShape(t *testing.T) {
	ctx := context.Background()
	a := newAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "count_tokens") {
			writeJSON(w, 200, `{"input_tokens":42}`)
			return
		}
		writeJSON(w, 200, batchJSON("msgbatch_1", "in_progress", time.Now()))
	})
	c, _ := checkedClient(t, a, nil)
	req := sampleRequest("p1-a")
	n, err := c.CountTokens(ctx, req)
	if err != nil || n != 42 {
		t.Fatalf("CountTokens = %d, %v; want 42", n, err)
	}
	if _, err := c.Create(ctx, []Request{req}); err != nil {
		t.Fatal(err)
	}

	count := decodeBody(t, a.last(t, "POST", "/v1/messages/count_tokens").body)
	create := decodeBody(t, a.last(t, "POST", "/v1/messages/batches").body)
	reqs, _ := create["requests"].([]any)
	if len(reqs) != 1 {
		t.Fatalf("batch holds %d requests, want 1", len(reqs))
	}
	item, _ := reqs[0].(map[string]any)
	if item["custom_id"] != "p1-a" {
		t.Errorf("custom_id = %v, want p1-a", item["custom_id"])
	}
	params, _ := item["params"].(map[string]any)

	wantOut := map[string]any{
		"effort": "high",
		"format": map[string]any{
			"type": "json_schema",
			"schema": map[string]any{
				"type":                 "object",
				"properties":           map[string]any{"a": map[string]any{"type": "string"}},
				"required":             []any{"a"},
				"additionalProperties": false,
			},
		},
	}
	if !reflect.DeepEqual(params["output_config"], wantOut) {
		t.Errorf("batch output_config = %v\nwant %v", params["output_config"], wantOut)
	}
	wantSystem := []any{map[string]any{"type": "text", "text": "You are a test."}}
	if !reflect.DeepEqual(params["system"], wantSystem) {
		t.Errorf("batch system = %v, want %v", params["system"], wantSystem)
	}
	if params["max_tokens"] != float64(100) || params["model"] != "test-model" {
		t.Errorf("batch params = %v, want max_tokens 100 and model test-model", params)
	}
	for _, k := range []string{"model", "messages", "system", "output_config"} {
		if !reflect.DeepEqual(count[k], params[k]) {
			t.Errorf("count_tokens %s = %v\nbatch %s = %v", k, count[k], k, params[k])
		}
	}
}

func TestRequestWithoutOptionalFields(t *testing.T) {
	a := newAPI(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, batchJSON("msgbatch_1", "in_progress", time.Now()))
	})
	c, _ := checkedClient(t, a, nil)
	req := Request{CustomID: "x", Model: "test-model", Messages: []Message{{Role: "user", Text: "hi"}}, MaxTokens: 10}
	if _, err := c.Create(context.Background(), []Request{req}); err != nil {
		t.Fatal(err)
	}
	create := decodeBody(t, a.last(t, "POST", "/v1/messages/batches").body)
	item := create["requests"].([]any)[0].(map[string]any)
	params := item["params"].(map[string]any)
	for _, k := range []string{"system", "output_config"} {
		if _, ok := params[k]; ok {
			t.Errorf("params holds %q for a request without it", k)
		}
	}
}

func TestBadSchemaSendsNothing(t *testing.T) {
	a := newAPI(t, nil)
	c, _ := checkedClient(t, a, nil)
	req := sampleRequest("x")
	req.Schema = json.RawMessage(`{not json`)
	if _, err := c.Create(context.Background(), []Request{req}); err == nil {
		t.Fatal("Create accepted an invalid schema")
	} else if errors.Is(err, ErrMaybeSent) {
		t.Error("an invalid schema is not a maybe-sent failure")
	}
	if n := a.hits("POST", "/v1/messages/batches"); n != 0 {
		t.Errorf("server saw %d Create requests, want 0", n)
	}
}
