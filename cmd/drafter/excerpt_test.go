package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/assemble"
	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/plan"
)

// dirFact is a verified fact whose only pointer is a directory, so no read can show its source.
const dirFact = "- `tally-dir-only` Tally ships in the blueprints directory. Source: `blueprints`. [verified]\n"

// dirBlueprint cites tally-dir-only from the first section of the tally blueprint.
func (f *cliFixture) dirBlueprint(tag string) {
	f.t.Helper()
	facts := f.read1(tallyFacts) + strings.Replace(dirFact, "[verified]", "["+tag+"]", 1)
	f.write(tallyFacts, facts)
	bp := strings.Replace(f.read1(tallyBP), "      - id: f-reset\n", "      - id: f-dir\n        kind: fact\n        fact: tally-dir-only\n      - id: f-reset\n", 1)
	f.write(tallyBP, bp)
	f.commit("cite a directory-only fact", tallyFacts, tallyBP)
}

func TestExcerptFactPrintsBulletAndBlocks(t *testing.T) {
	f := newCLIFixture(t)
	out, errs, code := run(t, "excerpt", "--root", f.root, "tally-state-file")
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, errs)
	}
	bullet := "`tally-state-file` Tally keeps its count in a file named .tally in the current directory. Source: `main.go:12-13` (`stateFile`). [verified]"
	if !strings.HasPrefix(out, bullet+"\n\n=== excerpt: main.go (fact tally-state-file) lines 1-50 ===\n") {
		t.Errorf("stdout does not open with the bullet then the block:\n%s", out)
	}
	if !strings.Contains(out, "const stateFile = \".tally\"") || !strings.HasSuffix(out, " ===\n") {
		t.Errorf("stdout lacks the excerpted lines or the block footer:\n%s", out)
	}
}

func TestExcerptExitStatus(t *testing.T) {
	f := newCLIFixture(t)
	f.dirBlueprint("verified")
	tests := []struct {
		name string
		args []string
		code int
		want string
	}{
		{"a fact whose only pointer is a directory", []string{"tally-dir-only"}, 1, "no excerpt: directory"},
		{"a pointer prints one block", []string{"main.go#readTotal"}, 0, "=== excerpt: main.go lines 1-50 ==="},
		{"a pointer to a directory", []string{"blueprints/docs"}, 1, "no excerpt: directory"},
		{"an unknown fact id", []string{"f:nothere"}, 2, "not a fact in the facts store"},
		{"no argument", nil, 2, "exactly one fact id or pointer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, errs, code := run(t, append([]string{"excerpt", "--root", f.root}, tt.args...)...)
			if code != tt.code || !strings.Contains(out+errs, tt.want) {
				t.Errorf("code %d, want %d containing %q:\n%s%s", code, tt.code, tt.want, out, errs)
			}
		})
	}
	t.Run("a pointer prints exactly one block", func(t *testing.T) {
		out, _, _ := run(t, "excerpt", "--root", f.root, "--json", "main.go#readTotal")
		var got excerptResult
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatal(err)
		}
		if len(got.Blocks) != 1 || !got.Excerpt || got.ID != "" || got.Raw != "" {
			t.Errorf("result = %+v, want one block, an excerpt, and no fact", got)
		}
	})
}

func TestCheckWarnsOnNoExcerpt(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		want int
	}{
		{"verified fact with only a directory pointer", "verified", 1},
		{"the same fact tagged external", "external", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newCLIFixture(t)
			f.dirBlueprint(tt.tag)
			for _, args := range [][]string{{"check", "--root", f.root}, {"check", "--root", f.root, f.blueprint()}} {
				out, errs, code := run(t, args...)
				if code != 0 {
					t.Fatalf("check %v exits %d:\n%s%s", args[3:], code, out, errs)
				}
				if got := strings.Count(out, ": warning: no-excerpt: "); got != tt.want {
					t.Errorf("check %v printed %d no-excerpt warnings, want %d:\n%s", args[3:], got, tt.want, out)
				}
			}
			if tt.want == 0 {
				return
			}
			out, _, _ := run(t, "check", "--root", f.root)
			wantLine := "blueprints/facts/tally.md:"
			if !strings.Contains(out, wantLine) || !strings.Contains(out, "`drafter excerpt tally-dir-only` shows what a read sees") {
				t.Errorf("warning does not point at the bullet's file or carry the fix:\n%s", out)
			}
		})
	}
}

// TestExcerptMatchesTheDraftRequest holds the command to the bytes a draft request carries. The
// draft request is built where drafter page builds it, assemble.Build with the page's fact ids,
// because a blueprint page's dry run stops at the reads that precede the draft.
func TestExcerptMatchesTheDraftRequest(t *testing.T) {
	f := newCLIFixture(t)
	facts, err := plan.LoadFacts(filepath.Join(f.root, "blueprints", "facts"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(f.root, config.Flags{})
	if err != nil {
		t.Fatal(err)
	}
	req, err := assemble.Build(context.Background(), assemble.Input{
		Seat: "draft", Answer: assemble.AnswerDraft, Config: cfg, Prompt: "Draft the page.",
		Page: "docs/count-items.md", Brief: "docs/count-items.brief.json", Slug: "count-items", CustomID: "id-1",
		Facts: facts, FactIDs: []string{"tally-count-cmd", "tally-by-flag"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var request strings.Builder
	for _, m := range req.Messages {
		request.WriteString(m.Text + "\n")
		request.WriteString(strings.Join(m.Cached, "\n"))
	}
	for _, id := range []string{"tally-count-cmd", "tally-by-flag"} {
		out, errs, code := run(t, "excerpt", "--root", f.root, "--json", id)
		if code != 0 {
			t.Fatalf("excerpt %s exits %d: %s", id, code, errs)
		}
		var got excerptResult
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatal(err)
		}
		if len(got.Blocks) == 0 {
			t.Fatalf("excerpt %s printed no blocks", id)
		}
		for _, b := range got.Blocks {
			if !strings.Contains(request.String(), b) {
				t.Errorf("the draft request lacks the command's block for %s:\n%s", id, b)
			}
		}
	}
}

func TestExcerptCountsCommentary(t *testing.T) {
	tests := []struct {
		name, source, want string
	}{
		{"no parenthetical pointer", "`blueprints` (no pointer here)", "no block shows source lines; point"},
		{"one parenthetical pointer", "`blueprints` (see `main.go:5`)", "no block shows source lines (1 pointer in parentheses was read as commentary); point"},
		{"two parenthetical pointers", "`blueprints` (see `main.go:5` and `main.go:9`)", "(2 pointers in parentheses were read as commentary)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newCLIFixture(t)
			f.dirBlueprint("verified")
			facts := strings.Replace(f.read1(tallyFacts), "Source: `blueprints`.", "Source: "+tt.source+".", 1)
			f.write(tallyFacts, facts)
			f.commit("add commentary", tallyFacts)
			_, errs, code := run(t, "excerpt", "--root", f.root, "tally-dir-only")
			if code != 1 || !strings.Contains(errs, tt.want) {
				t.Errorf("code %d, stderr %q, want 1 containing %q", code, errs, tt.want)
			}
			out, _, _ := run(t, "check", "--root", f.root)
			if strings.Contains(tt.source, "main.go") && !strings.Contains(out, "yields an excerpt (") {
				t.Errorf("check warning does not count the commentary:\n%s", out)
			}
		})
	}
}
