package pipeline

import (
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/batch"
)

// codeItems are the rotate blueprint's code items of the snippet tests, one per kind of source.
const codeItems = `
      - id: c-check
        kind: code
        text: |
          The check workflow.
        source: ../../snippets/check.yml
      - id: c-wrap
        kind: code
        text: |
          The wrapper.
        source: ../../snippets/wrap.go#Wrap
      - id: c-url
        kind: code
        text: |
          The upstream workflow.
        source: https://example.com/check.yml
      - id: c-dir
        kind: code
        text: |
          The snippets directory.
        source: ../../snippets
      - id: c-secret
        kind: code
        text: |
          A local file nobody committed.
        source: ../../snippets/local.txt
      - id: c-big
        kind: code
        text: |
          A large file.
        source: ../../snippets/big.txt
`

func TestSnippetInputs(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"), withDrafting)
	writeFile(t, h.root, "snippets/check.yml", "name: SNIPPET-CHECK-YML\n")
	writeFile(t, h.root, "snippets/wrap.go", "package snippets\n\n// SNIPPET-WRAP-HEAD\nfunc Wrap() {}\n\nfunc tail() {} // SNIPPET-WRAP-TAIL\n")
	writeFile(t, h.root, "snippets/big.txt", strings.Repeat("y", 300<<10))
	writeFile(t, h.root, ".gitignore", "snippets/local.txt\n")
	gitIn(t, h.root, "add", "snippets", ".gitignore")
	gitIn(t, h.root, "commit", "-q", "-m", "snippets")
	writeFile(t, h.root, "snippets/local.txt", "SNIPPET-UNTRACKED-SECRET\n")
	h.commitBlueprint(bpA, func(s string) string { return s + strings.TrimPrefix(codeItems, "\n") })
	structureFixes(h)
	h.api.script(s5, "draft", okText(answer(inOld, inNew, 1)))
	mustAccept(t, h, h.mustDraft(drafting{}))

	draft := requestsFor(h, s2, "draft")
	redraft := requestsFor(h, s5, "draft")
	if len(draft) != 1 || len(redraft) == 0 {
		t.Fatalf("draft requests = %d, redraft requests = %d, want 1 and at least 1", len(draft), len(redraft))
	}
	var reviews []batch.Request
	for _, seat := range reviewSeats {
		reviews = append(reviews, requestsFor(h, s4, seat)...)
		reviews = append(reviews, requestsFor(h, s6, seat)...)
	}
	if len(reviews) == 0 {
		t.Fatal("no review requests to check")
	}
	checkYML := "=== file: snippets/check.yml ===\nname: SNIPPET-CHECK-YML\n=== end file: snippets/check.yml ==="
	tests := []struct {
		name    string
		reqs    []batch.Request
		want    []string
		absent  []string
		snippet int // the number of snippets/ file blocks in each request
	}{
		{"the draft request carries a tracked page-relative source", draft, []string{checkYML}, nil, 4},
		{"the redraft exchange's first request carries it too", redraft[:1], []string{checkYML}, nil, 4},
		{"the review requests carry none", reviews, nil, []string{"=== file: snippets/"}, 0},
		{"a #Symbol source inlines the whole file", draft, []string{"SNIPPET-WRAP-HEAD", "SNIPPET-WRAP-TAIL"}, nil, 4},
		{"a URL source and a directory source add nothing", draft, nil, []string{"=== file: snippets ===", "example.com/check.yml ==="}, 4},
		{"an untracked file gets a not-inlined block", draft, []string{"=== file: snippets/local.txt ===\nnot inlined: untracked\n"}, nil, 4},
		{"an oversize file gets the size line", draft, []string{"=== file: snippets/big.txt ===\nnot inlined: 307200 bytes\n"}, nil, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, r := range tt.reqs {
				text := requestText(r)
				for _, w := range tt.want {
					if !strings.Contains(text, w) {
						t.Errorf("%s lacks %q", r.CustomID, w)
					}
				}
				for _, a := range tt.absent {
					if strings.Contains(text, a) {
						t.Errorf("%s holds %q", r.CustomID, a)
					}
				}
				if n := strings.Count(text, "\n=== file: snippets/"); n != tt.snippet {
					t.Errorf("%s holds %d snippet blocks, want %d", r.CustomID, n, tt.snippet)
				}
			}
		})
	}
	t.Run("the untracked file's contents reach no request", func(t *testing.T) {
		for _, r := range h.api.requests {
			for _, m := range r.Messages {
				if strings.Contains(m.Text+strings.Join(m.Cached, ""), "SNIPPET-UNTRACKED-SECRET") {
					t.Errorf("%s holds the untracked file's contents", r.CustomID)
				}
			}
		}
	})
}

// TestMarkdownCodeSourceIsInlined proves a Code: bullet's :source reaches the draft request as a
// YAML code item's source does, and a Figure: bullet's does not.
func TestMarkdownCodeSourceIsInlined(t *testing.T) {
	h := newBlueprintHarness(t, withPages("a"), withDrafting)
	writeFile(t, h.root, "snippets/wrap.go", "package snippets\n\n// SNIPPET-WRAP-HEAD\nfunc Wrap() {}\n")
	writeFile(t, h.root, "snippets/diagram.svg", "<svg>SNIPPET-DIAGRAM</svg>\n")
	gitIn(t, h.root, "add", "snippets")
	gitIn(t, h.root, "commit", "-q", "-m", "snippets")
	path, _ := h.markdownBlueprint()
	h.commitBlueprint(path, func(s string) string {
		return s + "\n- Code: the wrapper. :source[/snippets/wrap.go#Wrap]\n\n```go\nfunc Wrap() {}\n```\n\n- Figure: the flow. :source[/snippets/diagram.svg]\n"
	})
	mustAccept(t, h, h.mustDraft(drafting{narrow: true}))

	draft := requestsFor(h, s2, "draft")
	if len(draft) != 1 {
		t.Fatalf("draft requests = %d, want 1", len(draft))
	}
	text := requestText(draft[0])
	if want := "=== file: snippets/wrap.go ===\npackage snippets\n\n// SNIPPET-WRAP-HEAD\nfunc Wrap() {}\n=== end file: snippets/wrap.go ==="; !strings.Contains(text, want) {
		t.Errorf("the draft request lacks the Code: bullet's source:\n%s", text)
	}
	if strings.Contains(text, "SNIPPET-DIAGRAM") {
		t.Error("the draft request inlines a Figure: bullet's source")
	}
	for _, r := range requestsFor(h, s2e, "edit") {
		if strings.Contains(requestText(r), "=== file: snippets/") {
			t.Errorf("%s carries a snippet block", r.CustomID)
		}
	}
}
