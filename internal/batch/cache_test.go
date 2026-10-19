package batch

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCachedPartsWire(t *testing.T) {
	ctx := context.Background()
	a := newAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "count_tokens") {
			writeJSON(w, 200, `{"input_tokens":42}`)
			return
		}
		writeJSON(w, 200, batchJSON("msgbatch_1", "in_progress", time.Now()))
	})
	c, _ := checkedClient(t, a, nil)
	ephemeral := func() map[string]any { return map[string]any{"type": "ephemeral"} }
	tests := []struct {
		name string
		msg  Message
		want []any
	}{
		{
			name: "cached parts lead the plain block",
			msg:  Message{Role: "user", Cached: []string{"one", "two"}, Text: "plain"},
			want: []any{
				map[string]any{"type": "text", "text": "one", "cache_control": ephemeral()},
				map[string]any{"type": "text", "text": "two", "cache_control": ephemeral()},
				map[string]any{"type": "text", "text": "plain"},
			},
		},
		{
			name: "no cached parts sends today's single block",
			msg:  Message{Role: "user", Text: "plain"},
			want: []any{map[string]any{"type": "text", "text": "plain"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := sampleRequest("p1-a")
			req.Messages = []Message{tt.msg}
			if _, err := c.CountTokens(ctx, req); err != nil {
				t.Fatal(err)
			}
			if _, err := c.Create(ctx, []Request{req}); err != nil {
				t.Fatal(err)
			}
			count := decodeBody(t, a.last(t, "POST", "/v1/messages/count_tokens").body)
			create := decodeBody(t, a.last(t, "POST", "/v1/messages/batches").body)
			reqs, _ := create["requests"].([]any)
			item, _ := reqs[0].(map[string]any)
			params, _ := item["params"].(map[string]any)
			for name, body := range map[string]map[string]any{"batch": params, "count_tokens": count} {
				msgs, _ := body["messages"].([]any)
				m, _ := msgs[0].(map[string]any)
				if !reflect.DeepEqual(m["content"], tt.want) {
					t.Errorf("%s content = %v\nwant %v", name, m["content"], tt.want)
				}
			}
		})
	}
}
