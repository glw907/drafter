package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/glw907/drafter/internal/blueprint"
	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/internal/handoff"
	"github.com/glw907/drafter/internal/pipeline"
)

// page runs drafter page in sync mode from the fixture's root and returns its exit status and
// output.
func (f *cliFixture) page(args ...string) (code int, stdout, stderr string) {
	f.t.Helper()
	f.t.Chdir(f.root)
	cmd := newPageCmd()
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(append([]string{"--sync"}, args...))
	if err := cmd.Execute(); err != nil {
		ee, ok := errors.AsType[exitError](err)
		if !ok {
			f.t.Fatalf("page: %v", err)
		}
		return ee.code, out.String(), errOut.String() + ee.msg
	}
	return 0, out.String(), errOut.String()
}

func TestPageArgumentModes(t *testing.T) {
	tests := []struct {
		name string
		args []string
		code int
		want string
	}{
		{"a blueprint routes to blueprint mode", []string{"blueprints/docs/ghost.blueprint.yaml"}, 2, "does not list as managed"},
		{"a page maps to its blueprint", []string{"docs/ghost.md"}, 2, "docs/ghost.md, which the outline does not list as managed"},
		{"anything else is a handoff file", []string{"nothing.json"}, 2, "read handoff nothing.json"},
		{"a page and a handoff file are a usage error", []string{"docs/count-items.md", "h.json"}, 2, "not both"},
		{"a blueprint and a handoff file are a usage error", []string{"h.json", "blueprints/docs/count-items.blueprint.yaml"}, 2, "not both"},
		{"blueprints with --workflow-run are a usage error", []string{"--workflow-run", "run.json", "docs/count-items.md"}, 2, "or --workflow-run, not both"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newCLIFixture(t)
			code, out, errOut := f.page(tt.args...)
			if code != tt.code || !strings.Contains(out+errOut, tt.want) {
				t.Errorf("exit %d, want %d with %q\n%s%s", code, tt.code, tt.want, out, errOut)
			}
		})
	}
}

// toolsOnPath points PATH at a directory holding only the named tools, plus a stand-in drafter
// when withDrafter is set, so a test controls what the preflight finds.
func toolsOnPath(t *testing.T, withDrafter bool, names ...string) {
	t.Helper()
	bin := t.TempDir()
	for _, n := range names {
		p, err := exec.LookPath(n)
		if err != nil {
			t.Fatalf("%s is required to run this test: %v", n, err)
		}
		if err := os.Symlink(p, filepath.Join(bin, n)); err != nil {
			t.Fatal(err)
		}
	}
	if withDrafter {
		if err := os.WriteFile(filepath.Join(bin, "drafter"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
}

func TestPageAllUpToDateNeedsNothing(t *testing.T) {
	f := newCLIFixture(t)
	f.draft(tallyBriefJSON(t), "")
	// No key source, and neither Vale nor drafter on PATH: the run must not reach either.
	t.Setenv("DRAFTER_API_KEY", "")
	toolsOnPath(t, false, "git")
	for _, mode := range []string{"text", "json"} {
		args := []string{tallyPage}
		if mode == "json" {
			args = append(args, "--json")
		}
		code, out, errOut := f.page(args...)
		if code != 0 {
			t.Fatalf("%s: exit %d\n%s%s", mode, code, out, errOut)
		}
		if !strings.Contains(out, "up to date with drafted.lock") {
			t.Errorf("%s: stdout lacks the up to date line:\n%s", mode, out)
		}
		if mode == "json" {
			var got struct {
				Prompts string `json:"prompts"`
			}
			if err := json.Unmarshal([]byte(out), &got); err != nil || got.Prompts != "drafter:prompts.js" {
				t.Errorf("json summary: prompts %q, err %v", got.Prompts, err)
			}
		}
	}
	if n := len(f.stub.counts); n != 0 {
		t.Errorf("%d API calls for an up to date page", n)
	}
}

func TestPageDryRunDraftsFromTheBlueprint(t *testing.T) {
	f := newCLIFixture(t)
	valePath, err := exec.LookPath("vale")
	if err != nil {
		t.Fatalf("vale is required to run this test: %v", err)
	}
	toolsOnPath(t, true, "git", filepath.Base(valePath))
	code, out, errOut := f.page("--dry-run", tallyPage)
	if code != 0 {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	if !strings.Contains(out, "prompts: drafter:prompts.js") || !strings.Contains(out, tallyPage+": running [whole: ") {
		t.Errorf("stdout lacks the stock prompts line and the whole-scope page:\n%s", out)
	}
	if n := len(f.stub.counts); n != 2 {
		t.Errorf("%d token counts, want the consistency read and the blueprint read", n)
	}
	if _, err := os.Stat(filepath.Join(f.root, tallyLock)); err == nil {
		t.Error("a dry run wrote drafted.lock")
	}
}

func TestPageRefusesTheStockModuleOnATrackMismatch(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(string) string
		want   string
	}{
		{"no track matches the page", func(s string) string { return strings.Replace(s, "- docs/**", "- guides/**", 1) }, "matches no track"},
		{"a standard other than google", func(s string) string { return strings.Replace(s, "standard: google", "standard: microsoft", 1) }, `names the standard "microsoft"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newCLIFixture(t)
			f.write("blueprints/outline.yaml", tt.mutate(f.read1("blueprints/outline.yaml")))
			f.commit("change the track", "blueprints/outline.yaml")
			code, out, errOut := f.page("--dry-run", tallyPage)
			if code != 2 || !strings.Contains(out+errOut, tt.want) || !strings.Contains(out+errOut, tallyPage) {
				t.Errorf("exit %d, want 2 naming the page and %q\n%s%s", code, tt.want, out, errOut)
			}
			if n := len(f.stub.counts); n != 0 {
				t.Errorf("%d API calls before the refusal", n)
			}
		})
	}

	t.Run("a project module is not checked", func(t *testing.T) {
		f := newCLIFixture(t)
		f.write("blueprints/outline.yaml", strings.Replace(f.read1("blueprints/outline.yaml"), "standard: google", "standard: microsoft", 1))
		f.write("prompts.js", "return {};\n")
		f.write("drafter.toml", "prompts = \"prompts.js\"\n")
		f.commit("a project module", "blueprints/outline.yaml", "prompts.js", "drafter.toml")
		c, err := config.Load(f.root, config.Flags{})
		if err != nil {
			t.Fatal(err)
		}
		hs, err := blueprintHandoffs(c, f.root, f.rev("HEAD"), []string{f.blueprint()})
		if err != nil {
			t.Fatal(err)
		}
		if err := checkStockTracks(c, f.root, f.rev("HEAD"), hs); err != nil {
			t.Errorf("a project module was refused: %v", err)
		}
	})
}

func TestBlueprintHandoffsCarryTheDraftingInputs(t *testing.T) {
	f := newCLIFixture(t)
	bp := f.read1(tallyBP)
	f.write(tallyBP, strings.Replace(bp, "orderReason:", "exemplars:\n  - source: https://example.com/guide\n    take: |\n      The shape of its steps.\norderReason:", 1))
	f.commit("add an exemplar", tallyBP)
	committed := f.rev("HEAD")
	// A fact the working tree cites and the start commit does not must not reach the inputs.
	f.write(tallyBP, strings.Replace(f.read1(tallyBP), "fact: tally-total-cmd", "fact: tally-total-cmd\n      - id: f-extra\n        kind: fact\n        fact: tally-reset-cmd", 1))
	c, err := config.Load(f.root, config.Flags{})
	if err != nil {
		t.Fatal(err)
	}
	hs, err := blueprintHandoffs(c, f.root, committed, []string{f.blueprint()})
	if err != nil {
		t.Fatal(err)
	}
	in := hs[0].Inputs
	if in.PageType != "task guide" {
		t.Errorf("PageType = %q", in.PageType)
	}
	want := []string{"tally-count-cmd", "tally-count-creates", "tally-reset-cmd", "tally-state-file", "tally-total-cmd", "tally-total-empty"}
	if !slices.Equal(in.FactIDs, want) {
		t.Errorf("FactIDs = %v, want %v from the start commit's bytes", in.FactIDs, want)
	}
	if !slices.Equal(in.ExemplarSources, []string{"https://example.com/guide"}) {
		t.Errorf("ExemplarSources = %v", in.ExemplarSources)
	}
	if in.RegisterSections == nil || in.ClaimInventory == nil {
		t.Error("RegisterSections and ClaimInventory should be empty lists, not null")
	}
	if err := hs[0].Verify(); err != nil {
		t.Errorf("Verify: %v", err)
	}
}

func TestEntryJSONCarriesTheBriefStandardAndReader(t *testing.T) {
	page := blueprint.Page{Path: "docs/a.md", Type: "task guide"}
	tests := []struct {
		name     string
		outline  *blueprint.Outline
		standard any
		reader   any
	}{
		{"no tracks means google", &blueprint.Outline{}, "google", nil},
		{"a matching track gives its standard and reader", &blueprint.Outline{Tracks: []blueprint.Track{{Name: "d", Standard: "google", Reader: "A developer.", Paths: []string{"docs/**"}}}}, "google", "A developer."},
		{"tracks that do not match give neither", &blueprint.Outline{Tracks: []blueprint.Track{{Name: "d", Standard: "google", Reader: "A developer.", Paths: []string{"other/**"}}}}, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := entryJSON(tt.outline, page, "blueprints/docs/a.brief.json")
			if e["brief"] != "blueprints/docs/a.brief.json" || e["standard"] != tt.standard || e["reader"] != tt.reader {
				t.Errorf("brief %v, standard %v, reader %v; want the brief path, %v, %v", e["brief"], e["standard"], e["reader"], tt.standard, tt.reader)
			}
		})
	}
}

// acceptedSummary is a finished run's summary with one accepted tally page.
func acceptedSummary(accepted string, scope *blueprint.Scope) pipeline.Summary {
	return pipeline.Summary{Pages: []pipeline.PageResult{{Path: tallyPage, Status: pipeline.StatusAccepted, Accepted: accepted, Scope: scope}}}
}

func TestRecordDrafted(t *testing.T) {
	// setup returns a fixture whose page is written and uncommitted, as a run leaves it.
	setup := func(t *testing.T) (*cliFixture, *config.Config, []handoff.Handoff, string) {
		t.Helper()
		f := newCLIFixture(t)
		f.write(tallyPage, tallyDraft)
		c, err := config.Load(f.root, config.Flags{})
		if err != nil {
			t.Fatal(err)
		}
		head := f.rev("HEAD")
		hs, err := blueprintHandoffs(c, f.root, head, []string{f.blueprint()})
		if err != nil {
			t.Fatal(err)
		}
		return f, c, hs, head
	}
	readLock := func(t *testing.T, f *cliFixture) blueprint.LockEntry {
		t.Helper()
		l, err := blueprint.ReadLock(filepath.Join(f.root, tallyLock))
		if err != nil {
			t.Fatal(err)
		}
		return l.Pages[tallyPage]
	}
	writeEntry := func(t *testing.T, f *cliFixture, e blueprint.LockEntry) {
		t.Helper()
		l := &blueprint.Lock{Version: blueprint.Version, Pages: map[string]blueprint.LockEntry{tallyPage: e}}
		if err := blueprint.WriteLock(filepath.Join(f.root, tallyLock), l); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("an accepted page's entry has the four fields", func(t *testing.T) {
		f, c, hs, head := setup(t)
		blob := f.rev("HEAD:" + tallyBP)
		var warn bytes.Buffer
		if err := recordDrafted(c, f.root, head, hs, acceptedSummary(blob, nil), false, &warn); err != nil {
			t.Fatal(err)
		}
		e := readLock(t, f)
		pageBlob, err := gitText(f.root, "hash-object", tallyPage)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := blueprint.Decode(tallyBP, []byte(f.read1(tallyBP)))
		want := blueprint.SectionHashes(b, []byte(tallyDraft))
		if e.Page != pageBlob || e.Blueprint != blob || e.Accepted != blob || len(e.Sections) != len(want) || e.Sections["add-items"] != want["add-items"] {
			t.Errorf("entry = %+v, want page %s, blueprint and accepted %s, sections %v", e, pageBlob, blob, want)
		}
		if warn.Len() != 0 {
			t.Errorf("warning with HEAD at the start commit: %s", warn.String())
		}
	})

	t.Run("a scoped accept keeps accepted", func(t *testing.T) {
		f, c, hs, head := setup(t)
		writeEntry(t, f, blueprint.LockEntry{Accepted: "earlier"})
		scoped := &blueprint.Scope{Kind: blueprint.ScopeScoped}
		if err := recordDrafted(c, f.root, head, hs, acceptedSummary(f.rev("HEAD:"+tallyBP), scoped), false, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
		if e := readLock(t, f); e.Accepted != "earlier" || e.Page == "" {
			t.Errorf("entry = %+v, want accepted kept and the page recorded", e)
		}
	})

	t.Run("a skipped read keeps accepted", func(t *testing.T) {
		f, c, hs, head := setup(t)
		writeEntry(t, f, blueprint.LockEntry{Accepted: "earlier"})
		if err := recordDrafted(c, f.root, head, hs, acceptedSummary("", nil), false, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
		if e := readLock(t, f); e.Accepted != "earlier" || e.Page == "" {
			t.Errorf("entry = %+v, want accepted kept", e)
		}
	})

	t.Run("an escalated page leaves its entry alone", func(t *testing.T) {
		f, c, hs, head := setup(t)
		writeEntry(t, f, blueprint.LockEntry{Accepted: "earlier"})
		before, _ := os.ReadFile(filepath.Join(f.root, tallyLock))
		s := pipeline.Summary{Pages: []pipeline.PageResult{{Path: tallyPage, Status: pipeline.StatusEscalated, Accepted: "x"}}}
		if err := recordDrafted(c, f.root, head, hs, s, false, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
		if after, _ := os.ReadFile(filepath.Join(f.root, tallyLock)); !bytes.Equal(before, after) {
			t.Errorf("an escalated page changed the lock:\n%s", after)
		}
	})

	t.Run("a dry run and a stopped run write nothing", func(t *testing.T) {
		for name, tc := range map[string]struct {
			dry  bool
			stop string
		}{"dry run": {dry: true}, "stopped run": {stop: pipeline.StopSpendLimit}} {
			f, c, hs, head := setup(t)
			s := acceptedSummary(f.rev("HEAD:"+tallyBP), nil)
			s.Stop = tc.stop
			if err := recordDrafted(c, f.root, head, hs, s, tc.dry, &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(f.root, tallyLock)); err == nil {
				t.Errorf("%s wrote drafted.lock", name)
			}
		}
	})

	t.Run("a write that changes nothing leaves the file alone", func(t *testing.T) {
		f, c, hs, head := setup(t)
		s := acceptedSummary(f.rev("HEAD:"+tallyBP), nil)
		if err := recordDrafted(c, f.root, head, hs, s, false, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
		past := time.Now().Add(-time.Hour).Truncate(time.Second)
		if err := os.Chtimes(filepath.Join(f.root, tallyLock), past, past); err != nil {
			t.Fatal(err)
		}
		if err := recordDrafted(c, f.root, head, hs, s, false, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(filepath.Join(f.root, tallyLock))
		if err != nil {
			t.Fatal(err)
		}
		if !info.ModTime().Equal(past) {
			t.Errorf("an unchanged entry rewrote the lock (mtime %v, want %v)", info.ModTime(), past)
		}
	})

	t.Run("HEAD past the start commit by a fact edit warns and names the file", func(t *testing.T) {
		f, c, hs, head := setup(t)
		f.editFact()
		var warn bytes.Buffer
		if err := recordDrafted(c, f.root, head, hs, acceptedSummary(f.rev("HEAD:"+tallyBP), nil), false, &warn); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(warn.String(), tallyFacts) || !strings.Contains(warn.String(), "warning") {
			t.Errorf("warning = %q, want one naming %s", warn.String(), tallyFacts)
		}
	})
}
