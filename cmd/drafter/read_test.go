package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/glw907/drafter/internal/blueprint"
	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/handoff"
	"github.com/glw907/drafter/internal/pipeline"
	"github.com/glw907/drafter/internal/prompts"
)

// readProject is a committed project with a blueprint root, an outline of two pages, and a lock.
func readProject(t *testing.T) (root string, c *config.Config) {
	t.Helper()
	root = t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("blueprints/outline.yaml", `version: 1
types:
  - name: task guide
    anatomy: Task guide
pages:
  - path: docs/a.md
    slug: a
    title: Page A
    type: task guide
    job: |
      Do A.
    covers:
      - A.
  - path: docs/b.md
    slug: b
    title: Page B
    type: task guide
    job: |
      Do B.
`)
	write("blueprints/docs/a.blueprint.yaml", "version: 1\n")
	write("blueprints/docs/b.blueprint.yaml", "version: 1\n")
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-q", "-m", "fixture"}} {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return root, &config.Config{Blueprints: config.Blueprints{Root: "blueprints", Outline: "blueprints/outline.yaml"}}
}

func lockBytes(t *testing.T, root string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "blueprints/drafted.lock"))
	if err != nil {
		return ""
	}
	return string(b)
}

func TestRecordAccepted(t *testing.T) {
	root, c := readProject(t)
	two := pipeline.Summary{Pages: []pipeline.PageResult{
		{Path: "docs/a.md", Status: pipeline.StatusAccepted, Accepted: "aaa"},
		{Path: "docs/b.md", Status: pipeline.StatusEscalated, Rule: "plan"},
	}}

	for name, tc := range map[string]struct {
		s   pipeline.Summary
		dry bool
	}{
		"a stopped run": {pipeline.Summary{Stop: "cycle cap", Pages: two.Pages}, false},
		"a dry run":     {two, true},
	} {
		t.Run(name+" writes no lock", func(t *testing.T) {
			if err := recordAccepted(c, root, tc.s, tc.dry); err != nil {
				t.Fatal(err)
			}
			if got := lockBytes(t, root); got != "" {
				t.Errorf("lock = %q, want none", got)
			}
		})
	}

	t.Run("one accepted and one set aside leaves exactly one new accepted id", func(t *testing.T) {
		if err := recordAccepted(c, root, two, false); err != nil {
			t.Fatal(err)
		}
		l, err := blueprint.ReadLock(filepath.Join(root, "blueprints/drafted.lock"))
		if err != nil {
			t.Fatal(err)
		}
		if len(l.Pages) != 1 || l.Pages["docs/a.md"].Accepted != "aaa" {
			t.Errorf("lock pages = %+v, want one entry, docs/a.md accepted aaa", l.Pages)
		}
	})

	t.Run("an accepted id already recorded leaves the file as it was", func(t *testing.T) {
		before := lockBytes(t, root)
		if err := recordAccepted(c, root, two, false); err != nil {
			t.Fatal(err)
		}
		if got := lockBytes(t, root); got != before {
			t.Errorf("lock changed:\n%s\nto\n%s", before, got)
		}
	})

	t.Run("a fix keeps an earlier accepted id and a new acceptance keeps the entry's other fields", func(t *testing.T) {
		l, _ := blueprint.ReadLock(filepath.Join(root, "blueprints/drafted.lock"))
		l.Pages["docs/b.md"] = blueprint.LockEntry{Page: "pb", Blueprint: "bb", Accepted: "old"}
		if err := blueprint.WriteLock(filepath.Join(root, "blueprints/drafted.lock"), l); err != nil {
			t.Fatal(err)
		}
		s := pipeline.Summary{Pages: []pipeline.PageResult{
			{Path: "docs/a.md", Status: pipeline.StatusAccepted, Accepted: "aaa2"},
			{Path: "docs/b.md", Status: pipeline.StatusEscalated, Rule: "plan"},
		}}
		if err := recordAccepted(c, root, s, false); err != nil {
			t.Fatal(err)
		}
		got, _ := blueprint.ReadLock(filepath.Join(root, "blueprints/drafted.lock"))
		if got.Pages["docs/b.md"].Accepted != "old" || got.Pages["docs/b.md"].Page != "pb" || got.Pages["docs/a.md"].Accepted != "aaa2" {
			t.Errorf("lock pages = %+v", got.Pages)
		}
	})
}

func TestBlueprintHandoffs(t *testing.T) {
	root, c := readProject(t)
	head, err := gitText(root, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	hs, err := blueprintHandoffs(c, root, head, []string{filepath.Join(root, "blueprints/docs/a.blueprint.yaml")})
	if err != nil {
		t.Fatal(err)
	}
	h := hs[0]
	if h.Page.Path != "docs/a.md" || h.Page.Brief != "blueprints/docs/a.brief.json" || h.Page.Type != "task guide" || strings.TrimSpace(h.Inputs.Job) != "Do A." {
		t.Errorf("handoff page = %+v, job %q", h.Page, h.Inputs.Job)
	}
	if h.Plan != "blueprints/docs/a.blueprint.yaml" || h.Framing != "" {
		t.Errorf("plan %q, framing %q", h.Plan, h.Framing)
	}
	if err := h.Verify(); err != nil {
		t.Errorf("Verify: %v", err)
	}
	var ctx struct {
		Resolved struct {
			Pages []map[string]any `json:"pages"`
		} `json:"resolved"`
	}
	if err := json.Unmarshal(h.Context, &ctx); err != nil || len(ctx.Resolved.Pages) != 1 || ctx.Resolved.Pages[0]["path"] != "docs/a.md" || ctx.Resolved.Pages[0]["pageType"] != "task guide" {
		t.Errorf("context = %s, %v", h.Context, err)
	}

	for name, arg := range map[string]string{
		"not a blueprint":  filepath.Join(root, "blueprints/outline.yaml"),
		"outside the root": filepath.Join(t.TempDir(), "x.blueprint.yaml"),
	} {
		if _, err := blueprintHandoffs(c, root, head, []string{arg}); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

// TestRealModuleRendersTheBlueprintSeat loads the shipped prompts module the way drafter loads it
// for a read, with the host a read's handoff builds and no chain args, and renders the seat.
func TestRealModuleRendersTheBlueprintSeat(t *testing.T) {
	root, c := readProject(t)
	head, _ := gitText(root, "rev-parse", "HEAD")
	hs, err := blueprintHandoffs(c, root, head, []string{filepath.Join(root, "blueprints/docs/a.blueprint.yaml")})
	if err != nil {
		t.Fatal(err)
	}
	var ctx struct {
		Resolved json.RawMessage `json:"resolved"`
	}
	if err := json.Unmarshal(hs[0].Context, &ctx); err != nil {
		t.Fatal(err)
	}
	modulePath := "../../claude/.claude/workflows/docs-page-chain.js"
	src, err := os.ReadFile(modulePath)
	if err != nil {
		t.Fatal(err)
	}
	mod, err := prompts.Load(filepath.Base(modulePath), src, prompts.Host{Args: json.RawMessage("{}"), Resolved: ctx.Resolved})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	page, _ := json.Marshal(map[string]any{"path": hs[0].Page.Path, "job": hs[0].Inputs.Job})
	out, err := mod.Render("blueprint", prompts.Context{Page: page, Inputs: json.RawMessage("{}"), Round: prompts.Round1})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{"Blueprint read.", "Reader state.", "A `use` names scope only."} {
		if !strings.Contains(out, want) {
			t.Errorf("the prompt lacks %q", want)
		}
	}
	if strings.Contains(out, "docs/a.md") || strings.Contains(out, ".blueprint.yaml") {
		t.Error("the prompt names the page or a blueprint path, which the request would inline")
	}
	golden, err := os.ReadFile("testdata/blueprint-prompt.golden.txt")
	if err != nil {
		t.Fatal(err)
	}
	if out != string(golden) {
		t.Error("the unscoped prompt differs from testdata/blueprint-prompt.golden.txt, the prompt as it was before scoped reads")
	}
	if strings.Contains(out, "Change read.") {
		t.Error("the unscoped prompt carries the change-read paragraph")
	}

	scoped, err := mod.Render("blueprint", prompts.Context{Page: page, Inputs: json.RawMessage("{}"), Round: prompts.Round1, Scope: "intro,steps"})
	if err != nil {
		t.Fatalf("render with a scope: %v", err)
	}
	if !strings.HasPrefix(scoped, out) {
		t.Error("the scoped prompt does not start with the unscoped one")
	}
	for _, want := range []string{
		"Change read.", "These sections changed: intro,steps.", "Already present:", `Verdict "fix" if any blocking finding exists`,
		"Sometimes you have to look at the whole file to be sure that the change actually makes sense",
		"favor approving a CL once it is in a state where it definitely improves the overall code health",
		"encourage the author to file a bug", "google.github.io/eng-practices/review/reviewer/standard.html",
	} {
		if !strings.Contains(strings.Join(strings.Fields(scoped), " "), want) {
			t.Errorf("the scoped prompt lacks %q", want)
		}
	}
	if strings.Contains(scoped, "Scope. Only these sections") {
		t.Error("the scoped prompt still narrows the read to sections")
	}

	pageOnly, err := mod.Render("blueprint", prompts.Context{Page: page, Inputs: json.RawMessage("{}"), Round: prompts.Round1, Scope: "page"})
	if err != nil {
		t.Fatalf("render with the page scope: %v", err)
	}
	if !strings.Contains(pageOnly, "Change read.") || !strings.Contains(pageOnly, "No section's own content changed") || strings.Contains(pageOnly, "These sections changed") {
		t.Error("the page scope does not render the change read without section ids")
	}
}

const sinceOutline = `version: 1
types:
  - name: task guide
    anatomy: Task guide
pages:
  - path: docs/a.md
    slug: a
    title: Page A
    type: task guide
    job: |
      Do A.
  - path: docs/b.md
    slug: b
    title: Page B
    type: task guide
    job: |
      Do B.
`

const sinceBlueprint = `version: 1
argument: |
  Do the thing in two steps.
introduction:
  covers: |
    The thing.
  priorKnowledge: |
    Nothing.
  notCovered: |
    Other things.
orderReason: |
  The steps run in order.
sections:
  - id: one
    heading: One
    why: |
      It comes first.
    opens: |
      Start here.
    items:
      - id: one-start
        kind: handoff
        text: |
          Begin.
  - id: two
    heading: Two
    why: |
      It comes second.
    opens: |
      Then this.
    items:
      - id: two-start
        kind: handoff
        text: |
          Continue.
`

// readStub is an API stub for drafter read in sync mode. It records each Messages request body and,
// from failFrom on, refuses every one at a spend limit.
type readStub struct {
	*httptest.Server
	mu       sync.Mutex
	bodies   []string
	failFrom int
}

func newReadStub(t *testing.T) *readStub {
	t.Helper()
	s := &readStub{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("anthropic-workspace-id", "wrkspc_test")
			_, _ = w.Write([]byte(`{"data":[],"has_more":false,"first_id":null,"last_id":null}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/messages/count_tokens":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"input_tokens":100}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/messages":
			body, _ := io.ReadAll(r.Body)
			s.mu.Lock()
			s.bodies = append(s.bodies, string(body))
			refuse := s.failFrom > 0 && len(s.bodies) >= s.failFrom
			s.mu.Unlock()
			if refuse {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"You have reached your specified workspace API usage limits."}}`))
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			for _, ev := range [][2]string{
				{"message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"model-a","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":1}}}`},
				{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
				{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"{\"verdict\":\"ok\",\"findings\":[],\"summary\":\"read\"}"}}`},
				{"content_block_stop", `{"type":"content_block_stop","index":0}`},
				{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5}}`},
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

func (s *readStub) requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.bodies)
}

// sinceProject is a committed two-page blueprint project whose configuration points at the stub.
type sinceProject struct {
	t    *testing.T
	root string
	stub *readStub
}

func (p *sinceProject) git(args ...string) {
	p.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", p.root}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		p.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func (p *sinceProject) write(rel, body string) {
	p.t.Helper()
	path := filepath.Join(p.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		p.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		p.t.Fatal(err)
	}
}

// commit edits page a's blueprint and commits it.
func (p *sinceProject) commit(edit func(string) string) {
	p.t.Helper()
	rel := "blueprints/docs/a.blueprint.yaml"
	data, err := os.ReadFile(filepath.Join(p.root, rel))
	if err != nil {
		p.t.Fatal(err)
	}
	p.write(rel, edit(string(data)))
	p.git("add", rel)
	p.git("commit", "-q", "-m", "edit a")
}

func newSinceProject(t *testing.T) *sinceProject {
	t.Helper()
	p := &sinceProject{t: t, root: t.TempDir(), stub: newReadStub(t)}
	p.write("blueprints/outline.yaml", sinceOutline)
	p.write("blueprints/docs/a.blueprint.yaml", sinceBlueprint)
	p.write("blueprints/docs/b.blueprint.yaml", sinceBlueprint)
	p.write("prompts.js", `return { draft: (c) => "Draft", blueprint: (c) => "Read the blueprint." + (c.scope ? "\nScope: " + c.scope : "") };`+"\n")
	p.write("drafter.toml", `prompts = "prompts.js"
seats = ["draft"]

[gate]
command = "true"

[seat.draft]
model = "model-a"
effort = "high"
max_tokens = 4000

[seat.blueprint]
model = "model-a"
effort = "medium"
max_tokens = 4000
`)
	p.git("init", "-q", "-b", "main")
	p.git("add", "-A")
	p.git("commit", "-q", "-m", "fixture")
	cfg := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cfg, "drafter"), 0o755); err != nil {
		t.Fatal(err)
	}
	user := fmt.Sprintf("workspace_id = \"wrkspc_test\"\npage_cap = 10\ncycle_ceiling = 100\napi_base_url = %q\nstate_dir = %q\n\n[prices.models.model-a]\ninput = 1\noutput = 5\n", p.stub.URL, t.TempDir())
	if err := os.WriteFile(filepath.Join(cfg, "drafter", "config.toml"), []byte(user), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("DRAFTER_API_KEY", "sk-test")
	return p
}

// read runs drafter read in sync mode and returns its exit status and output.
func (p *sinceProject) read(args ...string) (code int, stdout, stderr string) {
	p.t.Helper()
	cmd := newReadCmd()
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(append([]string{"--sync"}, args...))
	if err := cmd.Execute(); err != nil {
		ee, ok := errors.AsType[exitError](err)
		if !ok {
			p.t.Fatalf("read: %v", err)
		}
		return ee.code, out.String(), errOut.String() + ee.msg
	}
	return 0, out.String(), errOut.String()
}

func (p *sinceProject) bp(page string) string {
	return filepath.Join(p.root, "blueprints/docs", page+".blueprint.yaml")
}

func TestRevisionAtFactsDirNotUnderRoot(t *testing.T) {
	p := newSinceProject(t)
	c := &config.Config{}
	c.Blueprints.Outline = "blueprints/outline.yaml"
	c.Facts.Dir = filepath.Join(t.TempDir(), "facts")
	r := revisionAt(c, p.root, "HEAD", handoff.Handoff{Plan: "blueprints/docs/a.blueprint.yaml"})
	if r.Blueprint != nil || r.Entry != nil || r.Facts != nil {
		t.Errorf("revision = %+v, want empty so the page reads whole", r)
	}
}

func editItem(s string) string { return strings.Replace(s, "Continue.", "Continue, carefully.", 1) }

func TestReadSince(t *testing.T) {
	t.Run("an unchanged page makes no API call and writes no lock", func(t *testing.T) {
		p := newSinceProject(t)
		code, out, errOut := p.read("--since", "HEAD", p.bp("a"))
		if code != 0 || !strings.Contains(out, "docs/a.md: nothing changed since HEAD") {
			t.Errorf("exit %d, stdout %q, stderr %q, want 0 and the unchanged line", code, out, errOut)
		}
		if n := len(p.stub.requests()); n != 0 {
			t.Errorf("%d API calls for an unchanged page", n)
		}
		if got := lockBytes(t, p.root); got != "" {
			t.Errorf("lock = %q, want none", got)
		}
	})

	t.Run("an unresolvable revision exits 2", func(t *testing.T) {
		p := newSinceProject(t)
		if code, _, errOut := p.read("--since", "no-such-rev", p.bp("a")); code != 2 || !strings.Contains(errOut, "no-such-rev") {
			t.Errorf("exit %d, stderr %q, want 2 naming the revision", code, errOut)
		}
	})

	t.Run("--since with --resume exits 2", func(t *testing.T) {
		p := newSinceProject(t)
		if code, _, errOut := p.read("--since", "HEAD", "--resume", "20260101-aaaaaaaa", p.bp("a")); code != 2 || !strings.Contains(errOut, "--since") {
			t.Errorf("exit %d, stderr %q, want 2 refusing the pair", code, errOut)
		}
	})

	t.Run("a scoped accept names its sections, sends them, and writes no lock", func(t *testing.T) {
		p := newSinceProject(t)
		p.commit(editItem)
		code, out, errOut := p.read("--since", "HEAD~1", p.bp("a"))
		if code != 0 {
			t.Fatalf("exit %d\nstdout %s\nstderr %s", code, out, errOut)
		}
		if !strings.Contains(out, "docs/a.md: accepted") || !strings.Contains(out, "[scoped: two]") {
			t.Errorf("stdout %q lacks the accepted page and its scope", out)
		}
		if !strings.Contains(errOut, "docs/a.md: accepted on a scoped read; drafted.lock keeps its earlier entry") {
			t.Errorf("stderr %q lacks the no-lock line", errOut)
		}
		if reqs := p.stub.requests(); len(reqs) != 1 || !strings.Contains(reqs[0], `Scope: two`) {
			t.Errorf("requests = %d, want one carrying the scope line", len(reqs))
		} else if text := requestText(reqs[0]); !strings.Contains(text, "=== blueprint diff ===") || strings.Contains(text, "=== outline entry change ===") {
			t.Errorf("the change block should hold the diff and no entry change:\n%s", text)
		}
		if got := lockBytes(t, p.root); got != "" {
			t.Errorf("lock = %q, want none after a scoped accept", got)
		}
	})

	t.Run("the scope is in the JSON", func(t *testing.T) {
		p := newSinceProject(t)
		p.commit(editItem)
		code, out, errOut := p.read("--json", "--since", "HEAD~1", p.bp("a"), p.bp("b"))
		if code != 0 {
			t.Fatalf("exit %d\nstdout %s\nstderr %s", code, out, errOut)
		}
		var got struct {
			Pages []struct {
				Path   string
				Status string
				Scope  struct {
					Kind string
					IDs  []string
					Why  string
				}
			}
		}
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("stdout is not JSON: %v\n%s", err, out)
		}
		kinds := map[string]string{}
		for _, pg := range got.Pages {
			kinds[pg.Path] = pg.Scope.Kind
			if pg.Path == "docs/a.md" && !slices.Equal(pg.Scope.IDs, []string{"two"}) {
				t.Errorf("docs/a.md scope ids = %v, want [two]", pg.Scope.IDs)
			}
		}
		if kinds["docs/a.md"] != "scoped" || kinds["docs/b.md"] != "unchanged" {
			t.Errorf("scope kinds = %v, want a scoped and b unchanged", kinds)
		}
	})

	t.Run("a whole accept under --since writes the lock", func(t *testing.T) {
		p := newSinceProject(t)
		rel := "blueprints/docs/a.blueprint.yaml"
		p.git("rm", "-q", rel)
		p.git("commit", "-q", "-m", "remove a")
		p.write(rel, sinceBlueprint)
		p.git("add", rel)
		p.git("commit", "-q", "-m", "restore a")
		code, out, errOut := p.read("--since", "HEAD~1", p.bp("a"))
		if code != 0 || !strings.Contains(out, "[whole: the blueprint is absent or undecodable at the earlier revision]") {
			t.Fatalf("exit %d, stdout %q, stderr %q, want 0 and a whole scope", code, out, errOut)
		}
		if reqs := p.stub.requests(); len(reqs) != 1 || strings.Contains(reqs[0], "Scope:") || strings.Contains(requestText(reqs[0]), "blueprint diff") {
			t.Errorf("a whole read sent a scope line or a change block")
		}
		l, err := blueprint.ReadLock(filepath.Join(p.root, "blueprints/drafted.lock"))
		if err != nil || l.Pages["docs/a.md"].Accepted == "" {
			t.Errorf("lock pages = %+v, %v, want docs/a.md accepted", l.Pages, err)
		}
	})

	t.Run("a removed section is a change read that writes no lock", func(t *testing.T) {
		p := newSinceProject(t)
		p.commit(func(s string) string { return s[:strings.Index(s, "  - id: two")] })
		code, out, errOut := p.read("--since", "HEAD~1", p.bp("a"))
		if code != 0 || !strings.Contains(out, "docs/a.md: accepted [scoped: page]") {
			t.Fatalf("exit %d, stdout %q, stderr %q, want an accepted page scoped to the page", code, out, errOut)
		}
		if got := lockBytes(t, p.root); got != "" {
			t.Errorf("lock = %q, want none after a removed-section accept", got)
		}
	})

	t.Run("the change block holds the diff, the changed fact, and the entry change", func(t *testing.T) {
		p := newSinceProject(t)
		p.write("blueprints/facts/keys.md", "## Facts\n\n- `f:one` One is old. [verified]\n- `f:other` Other. [verified]\n")
		p.write("blueprints/docs/a.blueprint.yaml", strings.Replace(sinceBlueprint, "        text: |\n          Begin.\n", "        text: |\n          Begin.\n      - id: one-fact\n        kind: fact\n        fact: f:one\n        use: |\n          Only the age.\n", 1))
		p.git("add", "-A")
		p.git("commit", "-q", "-m", "facts and a fact item")
		p.write("blueprints/facts/keys.md", "## Facts\n\n- `f:one` One is new. [verified]\n- `f:other` Other, revised. [verified]\n")
		p.write("blueprints/outline.yaml", strings.Replace(sinceOutline, "Do A.", "Do A well.", 1))
		p.commit(editItem)
		p.git("add", "-A")
		p.git("commit", "-q", "-m", "change everything", "--amend", "--no-edit")
		code, out, errOut := p.read("--since", "HEAD~1", p.bp("a"))
		if code != 0 {
			t.Fatalf("exit %d\nstdout %s\nstderr %s", code, out, errOut)
		}
		reqs := p.stub.requests()
		if len(reqs) != 1 {
			t.Fatalf("%d requests, want 1", len(reqs))
		}
		text := requestText(reqs[0])
		for _, want := range []string{
			"=== blueprint diff ===", "-          Continue.", "+          Continue, carefully.",
			"=== changed facts ===", "- f:one", "was: [verified] One is old.", "now: [verified] One is new.",
			"=== outline entry change ===", `"job": "Do A.\n"`, `"job": "Do A well.\n"`,
		} {
			if !strings.Contains(text, want) {
				t.Errorf("the request lacks %q\n%s", want, text)
			}
		}
		if strings.Contains(text, "f:other") && strings.Contains(text[strings.Index(text, "=== changed facts ==="):], "f:other") {
			t.Error("the change block lists a fact bullet the page never cites")
		}
		if end, diff := strings.Index(text, "=== end facts ==="), strings.Index(text, "=== blueprint diff ==="); end < 0 || diff < end {
			t.Error("the change block does not follow the facts block")
		}
		if strings.Contains(text, "=== outline entry change ===") != true {
			t.Error("the changed outline entry is missing from the block")
		}
	})

	t.Run("an outline-only change reads as the page, with its entry change", func(t *testing.T) {
		p := newSinceProject(t)
		p.write("blueprints/outline.yaml", strings.Replace(sinceOutline, "Do A.", "Do A well.", 1))
		p.git("add", "-A")
		p.git("commit", "-q", "-m", "reword the job")
		code, out, errOut := p.read("--since", "HEAD~1", p.bp("a"))
		if code != 0 || !strings.Contains(out, "[scoped: page]") {
			t.Fatalf("exit %d, stdout %q, stderr %q, want a page scope", code, out, errOut)
		}
		reqs := p.stub.requests()
		if len(reqs) != 1 || !strings.Contains(reqs[0], "Scope: page") || strings.Contains(requestText(reqs[0]), "=== blueprint diff ===") || !strings.Contains(requestText(reqs[0]), "=== outline entry change ===") {
			t.Errorf("requests = %d, want one with scope page, the entry change, and no blueprint diff", len(reqs))
		}
	})

	t.Run("a stopped run resumes with its scope, never needing --since", func(t *testing.T) {
		p := newSinceProject(t)
		p.commit(editItem)
		p.stub.failFrom = 1
		code, out, errOut := p.read("--since", "HEAD~1", p.bp("a"), p.bp("b"))
		if code != 3 {
			t.Fatalf("exit %d, want 3 for a stopped run\nstdout %s\nstderr %s", code, out, errOut)
		}
		fields := strings.Fields(strings.TrimSpace(out[strings.Index(out, "resume with:"):]))
		id := fields[len(fields)-1]
		p.stub.mu.Lock()
		p.stub.failFrom, p.stub.bodies = 0, nil
		p.stub.mu.Unlock()

		code, out, errOut = p.read("--resume", id, p.bp("a"), p.bp("b"))
		if code != 0 {
			t.Fatalf("resume exit %d\nstdout %s\nstderr %s", code, out, errOut)
		}
		if !strings.Contains(out, "docs/a.md: accepted") || !strings.Contains(out, "docs/b.md: unchanged (not in run)") {
			t.Errorf("stdout %q, want a accepted and b unchanged (not in run)", out)
		}
		if reqs := p.stub.requests(); len(reqs) != 1 || !strings.Contains(reqs[0], "Scope: two") {
			t.Errorf("the resume sent %d requests, want one that still carries the scope", len(reqs))
		}
		if got := lockBytes(t, p.root); got != "" {
			t.Errorf("lock = %q, want none after a resumed scoped accept", got)
		}
	})
}

// stockSummaryConfig loads the configuration of a machine with no config files, so every
// setting is the stock default.
func stockSummaryConfig(t *testing.T) *config.Config {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	c, err := config.Load("", config.Flags{})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestSummaryCountsDemotedAndAlreadyPresent(t *testing.T) {
	s := pipeline.Summary{Pages: []pipeline.PageResult{{
		Path: "docs/a.md", Status: pipeline.StatusAccepted, Cost: 0.5, Demoted: 1, AlreadyPresent: 2,
		Scope: &blueprint.Scope{Kind: blueprint.ScopeScoped, Facts: []string{}},
	}}}
	var text, js bytes.Buffer
	c := stockSummaryConfig(t)
	if err := writeSummaryText(&text, c, s, false); err != nil {
		t.Fatal(err)
	}
	if want := "prices: 2026-10-18 (default)\ndocs/a.md: accepted [scoped: page] already present: 2 demoted conflicts: 1 $0.5000\n"; text.String() != want {
		t.Errorf("text summary = %q, want %q", text.String(), want)
	}
	if err := writeSummaryJSON(&js, c, s); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"demoted": 1`, `"already_present": 2`, `"facts": []`, `"prices_date": "2026-10-18"`, `"prices_date_source": "default"`} {
		if !strings.Contains(js.String(), want) {
			t.Errorf("JSON summary lacks %q\n%s", want, js.String())
		}
	}
}

// requestText is the text of every string in a request body, which is how the model reads it.
func requestText(body string) string {
	var v any
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		return body
	}
	var b strings.Builder
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			b.WriteString(x)
			b.WriteByte('\n')
		case []any:
			for _, e := range x {
				walk(e)
			}
		case map[string]any:
			for _, k := range slices.Sorted(maps.Keys(x)) {
				walk(x[k])
			}
		}
	}
	walk(v)
	return b.String()
}
