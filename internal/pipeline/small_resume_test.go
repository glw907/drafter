package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/run"
)

// smallOrigin scripts page A to the small exchange from one of its two origins.
type smallOrigin struct {
	name   string
	script func(h *harness)
}

var smallOrigins = []smallOrigin{
	{"from round 2", func(h *harness) {
		roundTwoOrigin(h, map[string][]map[string]any{"structure": {kinded("link", "Before you begin", "LINK")}})
		h.api.script(s6b, "draft", smallAnswer(1))
	}},
	{"from the coverage check after the redraft", func(h *harness) {
		structureFixes(h)
		h.api.script(s5, "draft", okText(answer(linkedTTL, unlinkedTTL, 1)))
		h.api.script(s6b, "draft", okText(answer(unlinkedTTL, linkedTTL, 1, 2)))
	}},
}

// vanishingGate puts a cairn-run-gate on the PATH that runs the gate as the plain protocol does,
// except that while the file .vanish is in the project it reports a vanished run, once, and
// removes the file.
func vanishingGate(t *testing.T, h *harness) {
	t.Helper()
	bin := t.TempDir()
	writeFile(t, bin, "cairn-run-gate", `#!/bin/sh
[ "$1" = "--fresh" ] && shift
if [ -f .vanish ]; then rm -f .vanish; echo 'gate exit: 1 (vanished 3 times)'; exit 0; fi
sh -c "$1" > "$2" 2>&1
echo "gate exit: $?"
`)
	if err := os.Chmod(filepath.Join(bin, "cairn-run-gate"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	h.cfg.Gate.Protocol = "cairn-run-gate"
}

// touchOnSubmit creates the file rel in the project when a request whose custom_id holds substr
// is submitted.
func touchOnSubmit(t *testing.T, h *harness, substr, rel string) {
	t.Helper()
	h.api.onSubmit = func(ids []string) {
		for _, id := range ids {
			if strings.Contains(id, substr) {
				writeFile(t, h.root, rel, "")
			}
		}
	}
}

func TestSmallGatePendingResumesIntoCovSmall(t *testing.T) {
	for _, o := range smallOrigins {
		t.Run(o.name, func(t *testing.T) {
			ref := newHarness(t, withoutFacts, withPages("a"))
			o.script(ref)
			mustAccept(t, ref, ref.mustRun())
			want := requestsFor(ref, s6b, "coverage")

			h := newHarness(t, withoutFacts, withPages("a"))
			o.script(h)
			vanishingGate(t, h)
			touchOnSubmit(t, h, "_s6b_draft_1", ".vanish")
			s := h.mustRun()
			if s.Stop != StopInfrastructure {
				t.Fatalf("Stop = %q, want the gate's infrastructure stop", s.Stop)
			}
			if w := h.workOf(s.RunID); w.PendingGate != exSmall {
				t.Fatalf("pending_gate = %q, want %q", w.PendingGate, exSmall)
			}
			h.api.onSubmit = nil
			s, err := h.resume(s.RunID)
			if err != nil {
				t.Fatalf("resume: %v\nstderr: %s", err, h.err.String())
			}
			mustAccept(t, h, s)
			if n := len(requestsFor(h, s6b, "draft")); n != 1 {
				t.Errorf("small exchange requests = %d, want 1: the resume runs the gate, not the turn", n)
			}
			got := requestsFor(h, s6b, "coverage")
			if len(got) != 1 || len(want) != 1 {
				t.Fatalf("small-phase coverage reads = %d, reference %d, want 1 each", len(got), len(want))
			}
			if requestText(got[0]) != requestText(want[0]) || string(got[0].Schema) != string(want[0].Schema) {
				t.Errorf("the resumed coverage read differs from the uninterrupted run's:\n%s\n---\n%s", requestText(got[0]), requestText(want[0]))
			}
			if after := afterFirst(h, s6b, "draft"); len(after) != 1 {
				t.Errorf("requests after the small exchange = %d, want the coverage read alone", len(after))
			}
		})
	}
}

// siblingRedGate replaces the project's gate with one that, while the file .sibling-red is in the
// project, reports a red line naming only a sibling page, once, and removes the file: a sibling
// whose red clears before the next gate.
func siblingRedGate(t *testing.T, h *harness) {
	t.Helper()
	writeFile(t, h.root, "gate.sh", `#!/bin/sh
if [ -f .sibling-red ]; then rm -f .sibling-red; echo "docs/guides/sibling.md: contains BROKEN"; exit 1; fi
`+strings.TrimPrefix(gateScript, "#!/bin/sh\n"))
	gitIn(t, h.root, "add", "gate.sh")
	gitIn(t, h.root, "commit", "-q", "-m", "sibling gate")
}

func TestSmallGateHeldOnASiblingReleasesIntoCovSmall(t *testing.T) {
	for _, o := range smallOrigins {
		t.Run(o.name, func(t *testing.T) {
			h := newHarness(t, withoutFacts, withPages("a"))
			o.script(h)
			siblingRedGate(t, h)
			touchOnSubmit(t, h, "_s6b_draft_1", ".sibling-red")
			s := h.mustRun()
			mustAccept(t, h, s)
			if h.exists(".sibling-red") {
				t.Fatal("the gate never saw the sibling's red")
			}
			if n := len(requestsFor(h, s6b, "draft")); n != 1 {
				t.Errorf("small exchange requests = %d, want 1: a held page spends no fix turn", n)
			}
			after := afterFirst(h, s6b, "draft")
			if len(after) != 1 || !strings.Contains(after[0].CustomID, "_s6b_coverage_") {
				var ids []string
				for _, r := range after {
					ids = append(ids, r.CustomID)
				}
				t.Errorf("after the small exchange the run sent %v, want the small phase's coverage read alone", ids)
			}
			if n := len(requestsFor(h, s7, "reader")); n != 0 {
				t.Errorf("reader requests = %d, want none after the small exchange", n)
			}
			if st := h.state(s.RunID).Pages[pageKey(pageA)]; st.Step != stDone && st.Status != StatusAccepted {
				t.Errorf("page ended at step %q, status %q", st.Step, st.Status)
			}
		})
	}
}

func TestPrePassStateWithoutKindsResumes(t *testing.T) {
	stripKinds := func(t *testing.T, ps *run.PageState) {
		editWork(t, ps, func(w map[string]any) {
			for _, key := range []string{"ex", "cov_findings"} {
				var fs []any
				switch v := w[key].(type) {
				case map[string]any:
					fs, _ = v["findings"].([]any)
				case []any:
					fs = v
				}
				for _, f := range fs {
					if m, ok := f.(map[string]any); ok {
						delete(m, "Kind")
					}
				}
			}
		})
	}
	tests := []struct {
		name    string
		script  func(h *harness)
		crashAt string
		rule    string
	}{
		{"an open redraft resumes and accepts", func(h *harness) {
			structureFixes(h)
			h.api.script(s5, "draft", okText(answer(inOld, inNew, 1)))
		}, "_s5_draft_1", ""},
		{"an open round 2 with a kindless fix escalates as before", func(h *harness) {
			structureFixes(h)
			h.api.script(s5, "draft", okText(answer(inOld, inNew, 1)))
			h.api.script(s6, "structure", okText(readText("fix", finding("Before you begin", "Still wrong."))))
		}, "_s6_structure_1", "second fix verdict"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, withoutFacts, withPages("a"))
			tt.script(h)
			h.api.crash = crashAfterCreate(tt.crashAt)
			if !dies(func() { _, _ = h.run() }) {
				t.Fatalf("the run did not die at %s", tt.crashAt)
			}
			id := h.runID()
			h.rewriteState(id, func(st *run.State) { stripKinds(t, st.Pages[pageKey(pageA)]) })
			raw := h.state(id).Pages[pageKey(pageA)].Work
			if strings.Contains(string(raw), `"Kind"`) {
				t.Fatalf("the saved state still holds a kind: %s", raw)
			}
			var w work
			if err := json.Unmarshal(raw, &w); err != nil {
				t.Fatalf("a state with no kinds does not decode: %v", err)
			}
			h.api.crash = nil
			s, err := h.resume(id)
			if err != nil {
				t.Fatalf("resume: %v\nstderr: %s", err, h.err.String())
			}
			if tt.rule == "" {
				mustAccept(t, h, s)
				return
			}
			if r := ruleOf(s, pageA); r != tt.rule {
				t.Errorf("rule = %q, want %q", r, tt.rule)
			}
			if n := len(requestsFor(h, s6b, "draft")); n != 0 {
				t.Errorf("small exchange requests = %d, want none for a kindless finding", n)
			}
		})
	}
}
