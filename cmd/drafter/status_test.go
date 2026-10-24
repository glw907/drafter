package main

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	runstate "github.com/glw907/drafter/internal/run"
)

// statusEnv is a git repo with a user configuration pointed at a stub batch endpoint.
type statusEnv struct {
	t        *testing.T
	repo     string
	stateDir string
}

func newStatusEnv(t *testing.T, h http.HandlerFunc) *statusEnv {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	repo := filepath.Join(t.TempDir(), "proj")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", repo, "init", "-q", "-b", "main").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	cfg := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cfg, "drafter"), 0o755); err != nil {
		t.Fatal(err)
	}
	e := &statusEnv{t: t, repo: repo, stateDir: t.TempDir()}
	user := fmt.Sprintf("workspace_id = \"wrkspc_test\"\napi_base_url = %q\nstate_dir = %q\n", srv.URL, e.stateDir)
	if err := os.WriteFile(filepath.Join(cfg, "drafter", "config.toml"), []byte(user), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("DRAFTER_API_KEY", "sk-test")
	t.Chdir(repo)
	return e
}

// writeRun saves a run with one page whose intents are the given batches; a batch id prefixed
// "saved:" is one the run has already read.
func (e *statusEnv) writeRun(schema int, batches ...string) {
	e.t.Helper()
	ps := &runstate.PageState{Status: "running"}
	for i, b := range batches {
		in := runstate.Intent{ID: fmt.Sprintf("i%d", i), Stage: "draft"}
		in.BatchID, in.Saved = strings.CutPrefix(b, "saved:")
		ps.Intents = append(ps.Intents, in)
	}
	repo, err := runstate.Repo(e.repo)
	if err != nil {
		e.t.Fatal(err)
	}
	dir, err := runstate.Create(e.stateDir, repo, "r1")
	if err != nil {
		e.t.Fatal(err)
	}
	st := &runstate.State{Schema: schema, RunID: "r1", Pages: map[string]*runstate.PageState{"docs--guide": ps}}
	if err := st.Save(dir); err != nil {
		e.t.Fatal(err)
	}
}

func (e *statusEnv) status(args ...string) (code int, stdout, stderr string) {
	e.t.Helper()
	cmd := newStatusCmd()
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		ee, ok := errors.AsType[exitError](err)
		if !ok {
			e.t.Fatalf("status: %v", err)
		}
		return ee.code, out.String(), errOut.String() + ee.msg
	}
	return 0, out.String(), errOut.String()
}

func batchBody(id, status string, created time.Time, succeeded int) string {
	return fmt.Sprintf(`{"id":%q,"type":"message_batch","processing_status":%q,"created_at":%q,"expires_at":%q,"ended_at":null,"archived_at":null,"cancel_initiated_at":null,"results_url":null,"request_counts":{"processing":0,"succeeded":%d,"errored":0,"canceled":0,"expired":0}}`,
		id, status, created.Format(time.RFC3339), created.Add(24*time.Hour).Format(time.RFC3339), succeeded)
}

func TestStatus(t *testing.T) {
	created := time.Now().Add(-10 * time.Minute)
	api := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/messages/batches/msgbatch_open":
			_, _ = w.Write([]byte(batchBody("msgbatch_open", "in_progress", created, 0)))
		case "/v1/messages/batches/msgbatch_done":
			_, _ = w.Write([]byte(batchBody("msgbatch_done", "ended", created, 3)))
		default:
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`))
		}
	}
	tests := []struct {
		name     string
		setup    func(e *statusEnv)
		args     []string
		wantCode int
		want     []string
		wantErr  []string
	}{
		{
			name:  "an open batch",
			setup: func(e *statusEnv) { e.writeRun(runstate.StateSchema, "saved:msgbatch_old", "msgbatch_open") },
			args:  []string{"r1"},
			want:  []string{"batch msgbatch_open (docs--guide): in_progress, 10m old"},
			wantErr: []string{
				"--sync",
			},
		},
		{
			name:  "an ended batch shows counts",
			setup: func(e *statusEnv) { e.writeRun(runstate.StateSchema, "msgbatch_done") },
			args:  []string{"r1"},
			want:  []string{"batch msgbatch_done (docs--guide): ended, 10m old, 3 succeeded, 0 errored, 0 canceled, 0 expired"},
		},
		{
			name:  "json prints one object per batch",
			setup: func(e *statusEnv) { e.writeRun(runstate.StateSchema, "msgbatch_open", "msgbatch_done") },
			args:  []string{"--json", "r1"},
			want:  []string{`"batch":"msgbatch_open"`, `"status":"in_progress"`, `"batch":"msgbatch_done"`, `"succeeded":3`},
		},
		{
			name:  "a run with every batch read",
			setup: func(e *statusEnv) { e.writeRun(runstate.StateSchema, "saved:msgbatch_done") },
			args:  []string{"r1"},
			want:  []string{"no open batch"},
		},
		{
			name:     "a missing run",
			setup:    func(e *statusEnv) {},
			args:     []string{"nope"},
			wantCode: 2,
			wantErr:  []string{"state.json"},
		},
		{
			name:     "another state schema",
			setup:    func(e *statusEnv) { e.writeRun(7, "msgbatch_open") },
			args:     []string{"r1"},
			wantCode: 2,
			wantErr:  []string{"schema 7"},
		},
		{
			name:     "an API error",
			setup:    func(e *statusEnv) { e.writeRun(runstate.StateSchema, "msgbatch_gone") },
			args:     []string{"r1"},
			wantCode: 2,
			wantErr:  []string{"authentication_error", "invalid x-api-key"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newStatusEnv(t, api)
			tt.setup(e)
			code, out, errOut := e.status(tt.args...)
			if code != tt.wantCode {
				t.Fatalf("exit %d, want %d\n%s%s", code, tt.wantCode, out, errOut)
			}
			for _, w := range tt.want {
				if !strings.Contains(out, w) {
					t.Errorf("stdout lacks %q:\n%s", w, out)
				}
			}
			for _, w := range tt.wantErr {
				if !strings.Contains(errOut, w) {
					t.Errorf("stderr lacks %q:\n%s", w, errOut)
				}
			}
		})
	}
}
