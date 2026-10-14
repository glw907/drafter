package batch

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCredentials(t *testing.T) {
	const envSentinel = "ENV-SENTINEL-must-not-leak"
	tests := []struct {
		name, env, value string
	}{
		{"api key", "ANTHROPIC_API_KEY", envSentinel},
		{"auth token", "ANTHROPIC_AUTH_TOKEN", envSentinel},
		{"base url", "ANTHROPIC_BASE_URL", "http://" + envSentinel + ".invalid"},
		{"missing profile", "ANTHROPIC_PROFILE", envSentinel + "-profile"},
		{"custom headers", "ANTHROPIC_CUSTOM_HEADERS", "X-Leak: " + envSentinel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newAPI(t, func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, 200, batchJSON("msgbatch_1", "in_progress", time.Now()))
			})
			clearEnv(t)
			t.Setenv(tt.env, tt.value)
			c := New(testOptions(a))
			if err := c.CheckWorkspace(context.Background()); err != nil {
				t.Fatalf("CheckWorkspace: %v", err)
			}
			if _, err := c.Get(context.Background(), "msgbatch_1"); err != nil {
				t.Fatalf("Get: %v", err)
			}
			reqs := a.all()
			if len(reqs) != 2 {
				t.Fatalf("server saw %d requests, want 2", len(reqs))
			}
			for _, r := range reqs {
				if got := r.header.Get("X-Api-Key"); got != sentinelKey {
					t.Errorf("%s: X-Api-Key = %q, want the Options key", r.path, got)
				}
				if got := r.header.Get("Authorization"); got != "" {
					t.Errorf("%s: Authorization header present", r.path)
				}
				for name, vals := range r.header {
					for _, v := range vals {
						if strings.Contains(v, envSentinel) {
							t.Errorf("%s: header %s holds the environment sentinel", r.path, name)
						}
					}
				}
			}
		})
	}
}

func TestWorkspace(t *testing.T) {
	ctx := context.Background()

	t.Run("billable calls refuse before the check", func(t *testing.T) {
		a := newAPI(t, nil)
		c, _ := newClient(t, a, nil)
		if _, err := c.CountTokens(ctx, sampleRequest("a")); !errors.Is(err, ErrNoWorkspaceCheck) {
			t.Errorf("CountTokens err = %v, want ErrNoWorkspaceCheck", err)
		}
		if _, err := c.Create(ctx, []Request{sampleRequest("a")}); !errors.Is(err, ErrNoWorkspaceCheck) {
			t.Errorf("Create err = %v, want ErrNoWorkspaceCheck", err)
		}
		if _, err := c.Send(ctx, sampleRequest("a")); !errors.Is(err, ErrNoWorkspaceCheck) {
			t.Errorf("Send err = %v, want ErrNoWorkspaceCheck", err)
		}
		if n := a.total(); n != 0 {
			t.Errorf("server saw %d requests, want 0", n)
		}
	})

	t.Run("one models call", func(t *testing.T) {
		a := newAPI(t, nil)
		c, _ := newClient(t, a, nil)
		if err := c.CheckWorkspace(ctx); err != nil {
			t.Fatal(err)
		}
		if n := a.total(); n != 1 {
			t.Fatalf("server saw %d requests, want 1", n)
		}
		r := a.last(t, "GET", "/v1/models")
		if !strings.Contains(r.query, "limit=1") {
			t.Errorf("models query = %q, want limit=1", r.query)
		}
	})

	other := "wrkspc_other"
	tests := []struct {
		name string
		got  *string
	}{
		{"mismatched header", &other},
		{"absent header", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newAPI(t, nil)
			a.workspaceHeader = tt.got
			c, _ := newClient(t, a, nil)
			err := c.CheckWorkspace(ctx)
			if err == nil {
				t.Fatal("CheckWorkspace passed, want an error")
			}
			if !strings.Contains(err.Error(), testWorkspace) {
				t.Errorf("error %q does not name the configured ID", err)
			}
			if tt.got != nil && !strings.Contains(err.Error(), *tt.got) {
				t.Errorf("error %q does not name the received ID", err)
			}
			if _, err := c.Create(ctx, []Request{sampleRequest("a")}); !errors.Is(err, ErrNoWorkspaceCheck) {
				t.Errorf("Create after a failed check: err = %v, want ErrNoWorkspaceCheck", err)
			}
			if n := a.hits("POST", "/v1/messages/batches"); n != 0 {
				t.Errorf("Create reached the server %d times after a failed check", n)
			}
		})
	}

	t.Run("every request names the workspace", func(t *testing.T) {
		a := newAPI(t, func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "count_tokens"):
				writeJSON(w, 200, `{"input_tokens":5}`)
			case r.Method == "POST" && r.URL.Path == "/v1/messages":
				goodStream(w, "end_turn", 0)
			case r.Method == "POST":
				writeJSON(w, 200, batchJSON("msgbatch_1", "in_progress", time.Now()))
			default:
				writeJSON(w, 200, batchJSON("msgbatch_1", "ended", time.Now()))
			}
		})
		c, _ := checkedClient(t, a, nil)
		if _, err := c.CountTokens(ctx, sampleRequest("a")); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Create(ctx, []Request{sampleRequest("a")}); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Get(ctx, "msgbatch_1"); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Send(ctx, sampleRequest("a")); err != nil {
			t.Fatal(err)
		}
		reqs := a.all()
		if len(reqs) != 5 {
			t.Fatalf("server saw %d requests, want 5", len(reqs))
		}
		for _, r := range reqs {
			if got := r.header.Get("anthropic-workspace-id"); got != testWorkspace {
				t.Errorf("%s %s: workspace header = %q, want %q", r.method, r.path, got, testWorkspace)
			}
		}
	})
}

func TestSDKRetriesOff(t *testing.T) {
	a := newAPI(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 500, errBody("api_error", "boom"))
	})
	c, _ := checkedClient(t, a, nil)
	_, err := c.Create(context.Background(), []Request{sampleRequest("a")})
	if !errors.Is(err, ErrMaybeSent) {
		t.Fatalf("Create err = %v, want ErrMaybeSent", err)
	}
	if n := a.hits("POST", "/v1/messages/batches"); n != 1 {
		t.Errorf("Create reached the server %d times, want 1", n)
	}
}

// flaky answers the first n requests (models probe excluded) with fail and the rest with ok.
func flaky(n int, fail, ok http.HandlerFunc) http.HandlerFunc {
	var count atomic.Int32
	return func(w http.ResponseWriter, r *http.Request) {
		if int(count.Add(1)) <= n {
			fail(w, r)
			return
		}
		ok(w, r)
	}
}

func status(code int, typ string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, code, errBody(typ, "synthetic "+typ))
	}
}

func retryAfter(code int, seconds string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("retry-after", seconds)
		writeJSON(w, code, errBody("rate_limit_error", "slow down"))
	}
}

func okBatch(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, batchJSON("msgbatch_1", "ended", time.Now()))
}

const resultsJSONL = `{"custom_id":"a","result":{"type":"canceled"}}` + "\n"

// okAny answers every endpoint the retry rows use.
func okAny(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasSuffix(r.URL.Path, "/results"):
		w.Header().Set("content-type", "application/x-jsonl")
		w.WriteHeader(200)
		w.Write([]byte(resultsJSONL))
	case r.Method == "GET" && r.URL.Path == "/v1/messages/batches":
		writeJSON(w, 200, `{"data":[],"has_more":false,"first_id":null,"last_id":null}`)
	case strings.HasSuffix(r.URL.Path, "count_tokens"):
		writeJSON(w, 200, `{"input_tokens":5}`)
	default:
		okBatch(w, r)
	}
}

func TestInPlaceRetries(t *testing.T) {
	ctx := context.Background()

	calls := map[string]func(*Client) error{
		"Get": func(c *Client) error { _, err := c.Get(ctx, "msgbatch_1"); return err },
		"Results": func(c *Client) error {
			_, err := c.Results(ctx, "msgbatch_1")
			return err
		},
		"List": func(c *Client) error {
			_, err := c.List(ctx, time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC))
			return err
		},
		"CountTokens": func(c *Client) error { _, err := c.CountTokens(ctx, sampleRequest("a")); return err },
		"Create": func(c *Client) error {
			_, err := c.Create(ctx, []Request{sampleRequest("a")})
			return err
		},
	}

	t.Run("a reset then success", func(t *testing.T) {
		a := newAPI(t, flaky(1, reset, okBatch))
		c, _ := checkedClient(t, a, nil)
		if _, err := c.Get(ctx, "msgbatch_1"); err != nil {
			t.Fatalf("Get: %v", err)
		}
		if n := a.hits("GET", "/v1/messages/batches/msgbatch_1"); n < 2 {
			t.Errorf("server saw %d Get requests, want at least 2", n)
		}
	})

	t.Run("resets past the wall time", func(t *testing.T) {
		a := newAPI(t, reset)
		c, clk := checkedClient(t, a, func(o *Options) { o.WallTime = 10 * time.Second })
		_, err := c.Get(ctx, "msgbatch_1")
		if !errors.Is(err, ErrTransport) {
			t.Fatalf("Get err = %v, want ErrTransport", err)
		}
		if len(clk.waits()) == 0 {
			t.Error("no backoff wait happened before giving up")
		}
		var total time.Duration
		for _, d := range clk.waits() {
			total += d
		}
		if total > 10*time.Second {
			t.Errorf("waited %v in total, more than the wall time", total)
		}
	})

	t.Run("a retry-after longer than the wall time left", func(t *testing.T) {
		a := newAPI(t, retryAfter(429, "100"))
		c, clk := checkedClient(t, a, func(o *Options) { o.WallTime = 30 * time.Second })
		_, err := c.Get(ctx, "msgbatch_1")
		if !errors.Is(err, ErrTransport) {
			t.Fatalf("Get err = %v, want ErrTransport", err)
		}
		if n := a.hits("GET", "/v1/messages/batches/msgbatch_1"); n != 1 {
			t.Errorf("server saw %d Get requests, want 1", n)
		}
		if w := clk.waits(); len(w) != 0 {
			t.Errorf("slept %v, want no sleep", w)
		}
	})

	t.Run("a 429 with retry-after honors it", func(t *testing.T) {
		a := newAPI(t, flaky(1, retryAfter(429, "2"), okBatch))
		c, clk := checkedClient(t, a, nil)
		if _, err := c.Get(ctx, "msgbatch_1"); err != nil {
			t.Fatalf("Get: %v", err)
		}
		if w := clk.waits(); len(w) != 1 || w[0] != 2*time.Second {
			t.Errorf("waits = %v, want [2s]", w)
		}
	})

	t.Run("a bare 429 on Get", func(t *testing.T) {
		a := newAPI(t, flaky(1, status(429, "rate_limit_error"), okBatch))
		c, clk := checkedClient(t, a, nil)
		if _, err := c.Get(ctx, "msgbatch_1"); err != nil {
			t.Fatalf("Get: %v", err)
		}
		if n := a.hits("GET", "/v1/messages/batches/msgbatch_1"); n != 2 {
			t.Errorf("server saw %d Get requests, want 2", n)
		}
		if len(clk.waits()) != 1 {
			t.Errorf("waits = %v, want one backoff", clk.waits())
		}
	})

	for _, code := range []int{500, 529} {
		for _, name := range []string{"Get", "Results", "List", "CountTokens"} {
			t.Run(name+" "+http.StatusText(code), func(t *testing.T) {
				typ := "api_error"
				if code == 529 {
					typ = "overloaded_error"
				}
				a := newAPI(t, flaky(1, status(code, typ), okAny))
				c, _ := checkedClient(t, a, nil)
				if err := calls[name](c); err != nil {
					t.Fatalf("%s: %v", name, err)
				}
			})
		}
	}

	t.Run("Create retries a 429 that is not a spend limit", func(t *testing.T) {
		a := newAPI(t, flaky(1, retryAfter(429, "1"), okAny))
		c, clk := checkedClient(t, a, nil)
		if err := calls["Create"](c); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if n := a.hits("POST", "/v1/messages/batches"); n != 2 {
			t.Errorf("server saw %d Create requests, want 2", n)
		}
		if w := clk.waits(); len(w) != 1 || w[0] != time.Second {
			t.Errorf("waits = %v, want [1s]", w)
		}
	})

	t.Run("Create to a closed port is a dial failure", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addr := ln.Addr().String()
		ln.Close()
		a := newAPI(t, nil)
		c, clk := newClient(t, a, func(o *Options) {
			o.BaseURL = "http://" + addr
			o.WallTime = 10 * time.Second
		})
		c.checked = true
		_, err = c.Create(ctx, []Request{sampleRequest("a")})
		if !errors.Is(err, ErrTransport) {
			t.Fatalf("Create err = %v, want ErrTransport", err)
		}
		if errors.Is(err, ErrMaybeSent) {
			t.Error("a dial failure must not be ErrMaybeSent")
		}
		if len(clk.waits()) == 0 {
			t.Error("the dial failure was not retried")
		}
	})
}

func TestMaybeSent(t *testing.T) {
	ctx := context.Background()

	t.Run("Create reset after the body", func(t *testing.T) {
		a := newAPI(t, reset)
		c, _ := checkedClient(t, a, nil)
		_, err := c.Create(ctx, []Request{sampleRequest("a")})
		if !errors.Is(err, ErrMaybeSent) {
			t.Fatalf("Create err = %v, want ErrMaybeSent", err)
		}
		if n := a.hits("POST", "/v1/messages/batches"); n != 1 {
			t.Errorf("server saw %d Create requests, want 1", n)
		}
	})

	t.Run("Send reset mid-stream", func(t *testing.T) {
		a := newAPI(t, func(w http.ResponseWriter, r *http.Request) {
			sseStart(w)
			sseEvent(w, "message_start", messageStart)
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				conn.Close()
			}
		})
		c, _ := checkedClient(t, a, nil)
		_, err := c.Send(ctx, sampleRequest("a"))
		if !errors.Is(err, ErrMaybeSent) {
			t.Fatalf("Send err = %v, want ErrMaybeSent", err)
		}
		if n := a.hits("POST", "/v1/messages"); n != 1 {
			t.Errorf("server saw %d Send requests, want 1", n)
		}
	})

	t.Run("Send SSE error event after the 200", func(t *testing.T) {
		a := newAPI(t, func(w http.ResponseWriter, r *http.Request) {
			sseStart(w)
			sseEvent(w, "message_start", messageStart)
			sseEvent(w, "error", `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`)
		})
		c, _ := checkedClient(t, a, nil)
		_, err := c.Send(ctx, sampleRequest("a"))
		if !errors.Is(err, ErrMaybeSent) {
			t.Fatalf("Send err = %v, want ErrMaybeSent", err)
		}
		var ce *CallError
		if !errors.As(err, &ce) || ce.Type != "overloaded_error" {
			t.Errorf("error type = %+v, want overloaded_error kept", ce)
		}
		if n := a.hits("POST", "/v1/messages"); n != 1 {
			t.Errorf("server saw %d Send requests, want 1", n)
		}
	})

	t.Run("Send stream that ends without message_stop", func(t *testing.T) {
		a := newAPI(t, func(w http.ResponseWriter, r *http.Request) {
			sseStart(w)
			sseEvent(w, "message_start", messageStart)
		})
		c, _ := checkedClient(t, a, nil)
		if _, err := c.Send(ctx, sampleRequest("a")); !errors.Is(err, ErrMaybeSent) {
			t.Fatalf("Send err = %v, want ErrMaybeSent", err)
		}
	})
}

func TestSpendLimits(t *testing.T) {
	ctx := context.Background()
	shapes := []struct {
		name string
		h    http.HandlerFunc
		msg  string
	}{
		{"429 enforced", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 429, spendLimit429Body()) }, spendLimitMsg},
		{"400 organization", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, 400, errBody("invalid_request_error", orgLimitMsg))
		}, orgLimitMsg},
		{"400 workspace", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, 400, errBody("invalid_request_error", wsLimitMsg))
		}, wsLimitMsg},
	}
	calls := map[string]func(*Client) error{
		"Create": func(c *Client) error { _, err := c.Create(ctx, []Request{sampleRequest("a")}); return err },
		"Send": func(c *Client) error {
			res, err := c.Send(ctx, sampleRequest("a"))
			if err != nil && res.Kind != "" {
				return errors.New("Send returned a result and an error")
			}
			if err == nil {
				return errors.New("Send returned a result for a spend limit")
			}
			return err
		},
		"CountTokens": func(c *Client) error { _, err := c.CountTokens(ctx, sampleRequest("a")); return err },
		"Get":         func(c *Client) error { _, err := c.Get(ctx, "msgbatch_1"); return err },
	}
	for _, s := range shapes {
		for name, call := range calls {
			t.Run(s.name+" "+name, func(t *testing.T) {
				a := newAPI(t, s.h)
				c, clk := checkedClient(t, a, nil)
				err := call(c)
				if !errors.Is(err, ErrSpendLimit) {
					t.Fatalf("err = %v, want ErrSpendLimit", err)
				}
				if !strings.Contains(err.Error(), s.msg) {
					t.Errorf("error %q lost the API's message", err)
				}
				if n := a.total() - 1; n != 1 {
					t.Errorf("server saw %d calls after the check, want 1", n)
				}
				if len(clk.waits()) != 0 {
					t.Errorf("slept %v before giving up on a spend limit", clk.waits())
				}
			})
		}
	}

	t.Run("an errored batch result", func(t *testing.T) {
		for _, msg := range []string{orgLimitMsg, wsLimitMsg} {
			r := Result{Kind: KindErrored, ErrType: "invalid_request_error", ErrMessage: msg}
			if got := Classify(r); got != ActionSpendLimit {
				t.Errorf("Classify(%q) = %v, want ActionSpendLimit", msg, got)
			}
		}
	})
}

func TestTimeouts(t *testing.T) {
	ctx := context.Background()

	t.Run("a stream slower than RequestTimeout but within StreamTimeout", func(t *testing.T) {
		a := newAPI(t, func(w http.ResponseWriter, r *http.Request) {
			goodStream(w, "end_turn", 100*time.Millisecond)
		})
		c, _ := checkedClient(t, a, func(o *Options) {
			o.RequestTimeout = 80 * time.Millisecond
			o.StreamTimeout = 5 * time.Second
		})
		res, err := c.Send(ctx, sampleRequest("a"))
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
		if res.Text != "Hello" {
			t.Errorf("Text = %q, want Hello", res.Text)
		}
	})

	t.Run("a unary call past RequestTimeout", func(t *testing.T) {
		a := newAPI(t, func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-time.After(3 * time.Second):
			}
		})
		c, _ := checkedClient(t, a, func(o *Options) { o.RequestTimeout = 80 * time.Millisecond })
		start := time.Now()
		_, err := c.Create(ctx, []Request{sampleRequest("a")})
		if !errors.Is(err, ErrMaybeSent) {
			t.Fatalf("Create err = %v, want ErrMaybeSent", err)
		}
		if time.Since(start) > 2*time.Second {
			t.Error("Create waited for the server instead of timing out")
		}
	})
}

func TestErrorText(t *testing.T) {
	ctx := context.Background()
	servers := map[string]http.HandlerFunc{
		"400":         status(400, "invalid_request_error"),
		"500":         status(500, "api_error"),
		"reset":       reset,
		"spend limit": func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 429, spendLimit429Body()) },
	}
	calls := map[string]func(*Client) error{
		"Create":      func(c *Client) error { _, err := c.Create(ctx, []Request{sampleRequest("a")}); return err },
		"Send":        func(c *Client) error { _, err := c.Send(ctx, sampleRequest("a")); return err },
		"CountTokens": func(c *Client) error { _, err := c.CountTokens(ctx, sampleRequest("a")); return err },
		"Get":         func(c *Client) error { _, err := c.Get(ctx, "msgbatch_1"); return err },
		"Results":     func(c *Client) error { _, err := c.Results(ctx, "msgbatch_1"); return err },
		"List":        func(c *Client) error { _, err := c.List(ctx, time.Now()); return err },
	}
	for sname, h := range servers {
		for cname, call := range calls {
			t.Run(sname+" "+cname, func(t *testing.T) {
				a := newAPI(t, h)
				c, _ := checkedClient(t, a, func(o *Options) { o.WallTime = 3 * time.Second })
				err := call(c)
				if err == nil {
					return
				}
				for _, text := range []string{fmt.Sprintf("%v", err), fmt.Sprintf("%+v", err)} {
					if strings.Contains(text, sentinelKey) {
						t.Errorf("error text holds the key: %q", text)
					}
				}
			})
		}
	}
}
