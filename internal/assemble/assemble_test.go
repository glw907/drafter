package assemble

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glw907/drafter/internal/batch"
	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/edits"
)

func TestBuildRequestFields(t *testing.T) {
	f := newFixture(t)
	c := f.config()
	c.Seat["draft"] = config.Seat{
		Agent: filepath.Join(f.root, "agents", "draft.md"), Model: "model-x", Effort: "high", MaxTokens: 4000,
	}
	req, err := Build(context.Background(), f.input(c, "Write the page."))
	if err != nil {
		t.Fatal(err)
	}
	if req.Model != "model-x" || req.Effort != "high" || req.MaxTokens != 4000 || req.CustomID != "id-1" {
		t.Errorf("request = %+v, want the seat's model, effort, tokens, and the custom id", req)
	}
	if want := "You are the draft seat.\n\nWrite the page."; req.System != want {
		t.Errorf("System = %q, want the agent body %q", req.System, want)
	}
	if len(req.Messages) != 1 || req.Messages[0].Role != "user" {
		t.Fatalf("messages = %+v, want one user turn", req.Messages)
	}
	text := req.Messages[0].Text
	if !strings.HasPrefix(text, "Files and command outputs") {
		t.Errorf("first turn starts %q, want the inlining note", text[:40])
	}
	for _, want := range []string{"Write the page.", "Do not run a command whose output is not inlined", "a finding", "brief as a JSON string"} {
		if !strings.Contains(text, want) {
			t.Errorf("first turn lacks %q", want)
		}
	}
	if !strings.Contains(string(req.Schema), `"page"`) {
		t.Errorf("Schema = %s, want the draft schema", req.Schema)
	}
}

func TestBuildSystemWithoutAgent(t *testing.T) {
	f := newFixture(t)
	req, err := Build(context.Background(), f.input(f.config(), "p"))
	if err != nil {
		t.Fatal(err)
	}
	if req.System != "" {
		t.Errorf("System = %q, want empty with no agent", req.System)
	}
}

func TestBuildRefusals(t *testing.T) {
	f := newFixture(t)
	tests := []struct {
		name string
		edit func(*Input)
		want string
	}{
		{"unknown seat", func(in *Input) { in.Seat = "nobody" }, "not configured"},
		{"unknown answer", func(in *Input) { in.Answer = "poem" }, "unknown answer"},
		{"no project root", func(in *Input) { in.Config = &config.Config{Seat: in.Config.Seat} }, "no project root"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := f.input(f.config(), "p")
			tt.edit(&in)
			_, err := Build(context.Background(), in)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want one containing %q", err, tt.want)
			}
		})
	}
}

func TestFilesNamedInPrompt(t *testing.T) {
	f := newFixture(t)
	ex := filepath.Join(f.outside, "exemplars")
	missing := filepath.Join(ex, "absent.txt")
	tests := []struct {
		name   string
		prompt string
		in     []string // substrings the request must hold
		out    []string // substrings it must not hold
	}{
		{"untracked path stays out", "Read untracked.txt now.", nil, []string{"UNTRACKED-CONTENT"}},
		{"untracked path by absolute name stays out", "Read " + filepath.Join(f.root, "untracked.txt"), nil, []string{"UNTRACKED-CONTENT"}},
		{"tracked path in backticks and quotes", "See `docs/outline.md`, \"docs/guide.md\".", []string{"OUTLINE-JOB-TEXT", "=== file: docs/guide.md ==="}, nil},
		{"sentence mark after a path", "Read docs/outline.md.", []string{"OUTLINE-JOB-TEXT"}, nil},
		{"outside path no entry allows stays out", "Read " + filepath.Join(f.outside, "secret.txt"), nil, []string{"OUTSIDE-SECRET"}},
		{"allowed out-of-repo path is in", "Read " + ex + "/ok.txt", []string{"ALLOWED-OK", "=== file: " + ex + "/ok.txt ==="}, nil},
		{"allowed path that does not exist", "Read " + missing, []string{"=== file: " + missing + " ===\nnot inlined: missing"}, nil},
		{"oversized file", "Read big.txt", []string{"not inlined: 307200 bytes"}, []string{"xxxxxxxx"}},
		{"a directory is no file", "Look in docs and " + ex, nil, []string{"=== file: docs"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := requestText(t, f.input(f.config(ex), tt.prompt))
			for _, s := range tt.in {
				if !strings.Contains(got, s) {
					t.Errorf("request lacks %q", s)
				}
			}
			for _, s := range tt.out {
				if strings.Contains(got, s) {
					t.Errorf("request holds %q", s)
				}
			}
		})
	}
}

func TestThePageAndBriefAreInlinedWhileUntracked(t *testing.T) {
	f := newFixture(t)
	in := f.input(f.config(), "Review untracked.txt against docs/outline.md and docs/guide.md.")
	if got := requestText(t, in); strings.Contains(got, "UNTRACKED-CONTENT") {
		t.Fatal("a file that is not the page was inlined while untracked")
	}
	in.Page = "untracked.txt"
	got := requestText(t, in)
	if !strings.Contains(got, "UNTRACKED-CONTENT") || !strings.Contains(got, "=== file: untracked.txt ===") {
		t.Errorf("the page under review, untracked, was not inlined:\n%s", got)
	}
}

func TestFileNamedTwiceInlinesOnce(t *testing.T) {
	f := newFixture(t)
	prompt := "Read docs/outline.md and " + filepath.Join(f.root, "docs", "outline.md") + " again."
	got := requestText(t, f.input(f.config(), prompt))
	if n := strings.Count(got, "OUTLINE-JOB-TEXT"); n != 1 {
		t.Errorf("file inlined %d times, want once", n)
	}
	if !strings.Contains(got, "=== file: docs/outline.md ===") {
		t.Errorf("request lacks the repo-relative header")
	}
}

func TestTildeEntryAndPrompt(t *testing.T) {
	f := newFixture(t)
	t.Setenv("HOME", f.outside)
	got := requestText(t, f.input(f.config("~/exemplars"), "Read ~/exemplars/ok.txt and ~/secret.txt"))
	if !strings.Contains(got, "ALLOWED-OK") || strings.Contains(got, "OUTSIDE-SECRET") {
		t.Errorf("request = %q, want the allowed file in and the other out", got)
	}
}

func TestCommands(t *testing.T) {
	f := newFixture(t)
	t.Setenv("DRAFTER_TEST_KEY", "sk-secret-value")
	t.Setenv("ANTHROPIC_API_KEY", "sk-other-secret")
	c := f.config()
	c.Seat["draft"] = config.Seat{Model: "m", Inputs: []string{
		"echo page={page} slug={slug}",
		"echo changed; exit 1",
		"echo against {baseline}",
		`echo "key=[$DRAFTER_TEST_KEY] anthropic=[$ANTHROPIC_API_KEY]"`,
		"pwd",
	}}
	t.Run("without a baseline", func(t *testing.T) {
		got := requestText(t, f.input(c, "go"))
		for _, want := range []string{
			"=== command: echo page='docs/page.md' slug='page' ===\npage=docs/page.md slug=page\n=== end command",
			"changed\n[exit status 1]",
			"key=[] anthropic=[]",
			f.root + "\n=== end command: pwd",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("request lacks %q\n%s", want, got)
			}
		}
		if strings.Contains(got, "against") || strings.Contains(got, "sk-") {
			t.Errorf("request holds a skipped command's output or a key:\n%s", got)
		}
	})
	t.Run("with a baseline", func(t *testing.T) {
		in := f.input(c, "go")
		in.Baseline = "abc123"
		got := requestText(t, in)
		if !strings.Contains(got, "against abc123") {
			t.Errorf("request lacks the baseline command's output:\n%s", got)
		}
	})
}

func TestCommandTimeout(t *testing.T) {
	old, oldDelay := commandTimeout, commandWaitDelay
	commandTimeout, commandWaitDelay = 200*time.Millisecond, 100*time.Millisecond
	t.Cleanup(func() { commandTimeout, commandWaitDelay = old, oldDelay })
	f := newFixture(t)
	c := f.config()
	c.Seat["draft"] = config.Seat{Model: "m", Inputs: []string{"echo before; sleep 30; echo after"}}
	start := time.Now()
	got := requestText(t, f.input(c, "go"))
	if time.Since(start) > 10*time.Second {
		t.Errorf("Build took %v, want the command cut off near its timeout", time.Since(start))
	}
	if !strings.Contains(got, "timed out after 200ms") || strings.Contains(got, "after\n") {
		t.Errorf("request = %q, want a timeout note and no output past the cut", got)
	}
}

func TestCommandsOnlyForDraftAndRead(t *testing.T) {
	f := newFixture(t)
	c := f.config()
	c.Seat["draft"] = config.Seat{Model: "m", Inputs: []string{"echo RAN"}}
	for _, a := range []Answer{AnswerReader1, AnswerReader2, AnswerEdits, AnswerCoverage, AnswerConsistency} {
		in := f.input(c, "go")
		in.Answer = a
		if got := requestText(t, in); strings.Contains(got, "RAN") {
			t.Errorf("%s request ran a seat command", a)
		}
	}
}

func TestReaderAndContinuations(t *testing.T) {
	f := newFixture(t)
	t.Run("reader1 holds the page alone", func(t *testing.T) {
		in := f.input(f.config(), "Read the page. Background is in docs/outline.md and docs/guide.md.")
		in.Answer = AnswerReader1
		got := requestText(t, in)
		if n := strings.Count(got, "\n=== file: "); n != 1 {
			t.Errorf("reader1 holds %d file blocks, want 1:\n%s", n, got)
		}
		if !strings.Contains(got, "=== file: docs/page.md ===") || strings.Contains(got, "OUTLINE-JOB-TEXT") {
			t.Errorf("reader1 request = %q, want the page and no job text", got)
		}
	})
	t.Run("reader2 continues and inlines nothing", func(t *testing.T) {
		in := f.input(f.config(), "Now the job is in docs/outline.md.")
		in.Answer = AnswerReader2
		in.History = []batch.Message{{Role: "user", Text: "turn one"}, {Role: "assistant", Text: "answer one"}}
		req, err := Build(context.Background(), in)
		if err != nil {
			t.Fatal(err)
		}
		if len(req.Messages) != 3 || req.Messages[0].Text != "turn one" || req.Messages[1].Role != "assistant" {
			t.Fatalf("messages = %+v, want the history then one user turn", req.Messages)
		}
		last := req.Messages[2].Text
		if strings.Contains(last, "=== file:") || strings.Contains(last, "OUTLINE-JOB-TEXT") || strings.HasPrefix(last, "Files and command") {
			t.Errorf("reader2 turn = %q, want no files and no second note", last)
		}
		if !strings.HasPrefix(last, "Now the job") {
			t.Errorf("reader2 turn = %q, want the module's prompt first", last)
		}
	})
	t.Run("an edits continuation inlines the page and brief", func(t *testing.T) {
		in := f.input(f.config(), "Fix the findings in docs/outline.md.")
		in.Answer = AnswerEdits
		in.History = []batch.Message{{Role: "user", Text: "u"}, {Role: "assistant", Text: "a"}}
		got := requestText(t, in)
		for _, want := range []string{"=== file: docs/page.md ===", "=== file: docs/page.brief.json ===", "See the widget", edits.Instructions()} {
			if !strings.Contains(got, want) {
				t.Errorf("edits request lacks %q", want)
			}
		}
		if strings.Contains(got, "OUTLINE-JOB-TEXT") {
			t.Errorf("edits request inlined a prompt-named file")
		}
	})
}

func TestCoverageAndConsistencyInlineNothing(t *testing.T) {
	f := newFixture(t)
	for _, a := range []Answer{AnswerCoverage, AnswerConsistency} {
		in := f.input(f.config(), "Check docs/outline.md against docs/guide.md.")
		in.Answer = a
		got := requestText(t, in)
		if strings.Contains(got, "=== file:") || strings.HasPrefix(got, "Files and command") {
			t.Errorf("%s request inlined something or carried the note: %q", a, got)
		}
		if !strings.HasPrefix(got, "Check docs/outline.md") {
			t.Errorf("%s request = %q, want the prompt as built", a, got)
		}
	}
}

func TestFactReadWithoutFacts(t *testing.T) {
	f := newFixture(t)
	c := f.config()
	c.Seat["fact"] = config.Seat{Model: "m"}
	in := f.input(c, "Check the page's claims.")
	in.Seat, in.Answer = "fact", AnswerRead
	got := requestText(t, in)
	for _, want := range []string{"=== file: src/greet.go ===", "func Shout", "=== file: src/widget.ts ==="} {
		if !strings.Contains(got, want) {
			t.Errorf("fact request without facts lacks %q", want)
		}
	}

	t.Run("with a facts store the page's paths are not inlined", func(t *testing.T) {
		in.Facts = loadFacts(t, f)
		in.FactIDs = []string{"f:greet"}
		got := requestText(t, in)
		if strings.Contains(got, "=== file: src/greet.go ===") {
			t.Errorf("fact request with facts inlined a named file whole")
		}
	})
}

func TestDeclinedFindingsReachTheNextRead(t *testing.T) {
	f := newFixture(t)
	in := f.input(f.config(), "Read the page again.")
	in.Answer = AnswerRead
	in.Declined = []edits.DeclinedFinding{{
		Finding: edits.Finding{N: 4, Seat: "read", Location: "Intro", Text: "The intro repeats the title.", Blocking: true},
		Reason:  "the title is quoted on purpose",
	}}
	got := requestText(t, in)
	for _, want := range []string{"Declined finding 4 (Intro): The intro repeats the title.", "Reason declined: the title is quoted on purpose", "accept the decline, or raise the finding again"} {
		if !strings.Contains(got, want) {
			t.Errorf("request lacks %q", want)
		}
	}
	in.Declined = nil
	if got := requestText(t, in); strings.Contains(got, "Declined finding") {
		t.Errorf("request names a declined finding when none was declined")
	}
}
