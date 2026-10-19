package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite the golden request bodies under testdata")

// cliFixture is the synthetic tally project of e2e/testdata/cli, copied into a fresh git
// repository with its configuration pointed at stub.
type cliFixture struct {
	t    *testing.T
	root string
	stub *countStub
}

// countStub is an API stub that records the body of every count_tokens request.
type countStub struct {
	*httptest.Server
	mu     sync.Mutex
	counts []string
}

func newCountStub(t *testing.T) *countStub {
	t.Helper()
	s := &countStub{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("anthropic-workspace-id", "wrkspc_test")
			_, _ = w.Write([]byte(`{"data":[],"has_more":false,"first_id":null,"last_id":null}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/messages/count_tokens":
			body, _ := io.ReadAll(r.Body)
			s.mu.Lock()
			s.counts = append(s.counts, string(body))
			s.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"input_tokens":100}`))
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func newCLIFixture(t *testing.T) *cliFixture {
	t.Helper()
	f := &cliFixture{t: t, root: t.TempDir(), stub: newCountStub(t)}
	from := filepath.Join("..", "..", "e2e", "testdata", "cli")
	err := filepath.Walk(from, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		dst := filepath.Join(f.root, rel)
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
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-q", "-m", "fixture"}} {
		f.git(args...)
	}
	cfg := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cfg, "drafter"), 0o755); err != nil {
		t.Fatal(err)
	}
	user := fmt.Sprintf("workspace_id = \"wrkspc_test\"\npage_cap = 10\ncycle_ceiling = 100\napi_base_url = %q\nstate_dir = %q\n", f.stub.URL, t.TempDir())
	if err := os.WriteFile(filepath.Join(cfg, "drafter", "config.toml"), []byte(user), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("DRAFTER_API_KEY", "sk-test")
	return f
}

func (f *cliFixture) git(args ...string) {
	f.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", f.root}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		f.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func (f *cliFixture) blueprint() string {
	return filepath.Join(f.root, "blueprints", "docs", "count-items.blueprint.yaml")
}

// read runs drafter read in sync mode on the fixture and returns its exit status and output.
func (f *cliFixture) read(args ...string) (code int, stdout, stderr string) {
	f.t.Helper()
	cmd := newReadCmd()
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(append([]string{"--sync"}, args...))
	if err := cmd.Execute(); err != nil {
		ee, ok := errors.AsType[exitError](err)
		if !ok {
			f.t.Fatalf("read: %v", err)
		}
		return ee.code, out.String(), errOut.String() + ee.msg
	}
	return 0, out.String(), errOut.String()
}

// TestReadDryRunRequestGolden pins the count_tokens bodies that drafter read --dry-run sends for the
// fixture blueprint as it stood when the golden was captured, frozen in testdata so the fixture can
// grow. The golden was captured before the drafting pass touched the pipeline or the handoff
// builder, so a later change that moves the blueprint read's bytes fails here.
func TestReadDryRunRequestGolden(t *testing.T) {
	f := newCLIFixture(t)
	frozen, err := os.ReadFile(filepath.Join("testdata", "read-golden.blueprint.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	f.write(tallyBP, string(frozen))
	f.commit("freeze the blueprint the golden was captured on", tallyBP)
	if code, out, errOut := f.read("--dry-run", f.blueprint()); code != 0 {
		t.Fatalf("read --dry-run exit %d\n%s%s", code, out, errOut)
	}
	if len(f.stub.counts) != 2 {
		t.Fatalf("count_tokens requests = %d, want the consistency read and the blueprint read", len(f.stub.counts))
	}
	var got string
	for _, body := range f.stub.counts {
		got += strings.TrimSpace(body) + "\n"
	}
	golden := filepath.Join("testdata", "read-dryrun-count.golden.json")
	if *updateGolden {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("the count_tokens body differs from %s\n got: %s\nwant: %s", golden, got, want)
	}
}
