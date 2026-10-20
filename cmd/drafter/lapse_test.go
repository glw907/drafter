package main

import (
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/blueprint"
	"github.com/glw907/drafter/internal/pipeline"
)

// setEntry rewrites the fixture page's lock entry with edit and commits the lock.
func (f *cliFixture) setEntry(edit func(*blueprint.LockEntry)) {
	f.t.Helper()
	file := filepath.Join(f.root, tallyLock)
	lock, err := blueprint.ReadLock(file)
	if err != nil {
		f.t.Fatal(err)
	}
	e := lock.Pages[tallyPage]
	edit(&e)
	lock.Pages[tallyPage] = e
	if err := blueprint.WriteLock(file, lock); err != nil {
		f.t.Fatal(err)
	}
	f.commit("edit the lock entry", tallyLock)
}

// editFactAnchor changes only the parenthesized anchor of the first fact's bullet, which the
// fact's text and sources do not carry.
func (f *cliFixture) editFactAnchor() {
	f.t.Helper()
	f.write(tallyFacts, strings.Replace(f.read1(tallyFacts), "(`stateFile`)", "(`stateFile` constant)", 1))
	f.commit("edit the anchor of one fact", tallyFacts)
}

func TestRecordReadsFacts(t *testing.T) {
	oldFacts := map[string]string{"f:a": "aaaaaaaaaaaa"}
	newFacts := map[string]string{"f:a": "bbbbbbbbbbbb", "f:b": "cccccccccccc"}
	oldBar := &blueprint.Blocked{Commit: "0000000000000000000000000000000000000000", Findings: []blueprint.BlockedFinding{{Location: "steps", Text: "An older finding.", Seat: "blueprint"}}}
	seatless := &blueprint.Blocked{Commit: oldBar.Commit, Findings: []blueprint.BlockedFinding{{Location: "steps", Text: "An older finding."}}}
	accepted := func(e blueprint.LockEntry, facts map[string]string) *blueprint.LockEntry {
		e.Facts = facts
		return &e
	}
	drafted := blueprint.LockEntry{Page: "p1", Blueprint: "b1", Accepted: "b0", Sections: map[string]string{"_intro": "h"}}
	at := func(e blueprint.LockEntry, accepted string, facts map[string]string, bar *blueprint.Blocked) blueprint.LockEntry {
		e.Accepted, e.Facts, e.Blocked = accepted, facts, bar
		return e
	}
	scoped := scopeOf(blueprint.ScopeScoped)

	tests := []struct {
		name    string
		prior   blueprint.LockEntry
		result  pipeline.PageResult
		base    string
		changed bool
		want    *blueprint.LockEntry
	}{
		{"a whole accept writes accepted and facts",
			drafted, pipeline.PageResult{Status: pipeline.StatusAccepted, Accepted: "b2", Facts: newFacts}, "", true, accepted(at(drafted, "b2", nil, nil), newFacts)},
		{"an entry whose accepted equals the blob and holds no facts gains them on a whole accept",
			at(drafted, "b2", nil, nil), pipeline.PageResult{Status: pipeline.StatusAccepted, Accepted: "b2", Facts: newFacts}, "", true, accepted(at(drafted, "b2", nil, nil), newFacts)},
		{"a whole accept that changes nothing writes nothing",
			at(drafted, "b2", newFacts, nil), pipeline.PageResult{Status: pipeline.StatusAccepted, Accepted: "b2", Facts: newFacts}, "", false, accepted(at(drafted, "b2", nil, nil), newFacts)},
		{"a whole accept of a blueprint that cites no fact clears the facts",
			at(drafted, "b2", oldFacts, nil), pipeline.PageResult{Status: pipeline.StatusAccepted, Accepted: "b2", Facts: map[string]string{}}, "", true, accepted(at(drafted, "b2", nil, nil), nil)},
		{"an accept inside drafter page leaves the facts as they were",
			at(drafted, "b0", oldFacts, nil), pipeline.PageResult{Status: pipeline.StatusAccepted, Accepted: "b2"}, "", true, accepted(at(drafted, "b2", nil, nil), oldFacts)},
		{"a bar accept on a bar whose findings carry a seat writes accepted and facts",
			at(drafted, "", nil, oldBar), pipeline.PageResult{Status: pipeline.StatusAccepted, Accepted: "b2", BarFrom: oldBar.Commit, Scope: scoped, Facts: newFacts}, "", true, accepted(at(drafted, "b2", nil, nil), newFacts)},
		{"a bar accept on a bar with no seat writes accepted, clears the bar, and writes no facts",
			at(drafted, "", oldFacts, seatless), pipeline.PageResult{Status: pipeline.StatusAccepted, Accepted: "b2", BarFrom: oldBar.Commit, Scope: scoped, Facts: newFacts}, "", true, accepted(at(drafted, "b2", nil, nil), oldFacts)},
		{"a lapse accept writes the facts and keeps accepted",
			at(drafted, "b2", oldFacts, nil), pipeline.PageResult{Status: pipeline.StatusAccepted, Accepted: "b2", Lapse: true, Scope: scoped, Facts: newFacts}, "b2", true, accepted(at(drafted, "b2", nil, nil), newFacts)},
		{"a lapse accept against an entry that no longer holds its accepted id writes nothing",
			at(drafted, "b9", oldFacts, nil), pipeline.PageResult{Status: pipeline.StatusAccepted, Accepted: "b2", Lapse: true, Scope: scoped, Facts: newFacts}, "b2", false, accepted(at(drafted, "b9", nil, nil), oldFacts)},
		{"a lapse read that blocks leaves the entry as it was",
			at(drafted, "b2", oldFacts, nil), pipeline.PageResult{Status: pipeline.StatusEscalated, Lapse: true, Scope: scoped, Facts: newFacts, Findings: []pipeline.FindingResult{{Location: "steps", Text: "Broken.", Blocking: true}}}, "b2", false, accepted(at(drafted, "b2", nil, nil), oldFacts)},
		{"a scoped accept advances accepted and leaves the facts",
			at(drafted, "b0", oldFacts, nil), pipeline.PageResult{Status: pipeline.StatusAccepted, Accepted: "b2", Since: "abc", Scope: scoped}, "b0", true, accepted(at(drafted, "b2", nil, nil), oldFacts)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lock := &blueprint.Lock{Version: blueprint.Version, Pages: map[string]blueprint.LockEntry{"docs/a.md": tt.prior}}
			tt.result.Path = "docs/a.md"
			changed, _ := recordReads(lock, barStart, pipeline.Summary{Pages: []pipeline.PageResult{tt.result}}, map[string]string{"docs/a.md": tt.base})
			if changed != tt.changed {
				t.Errorf("changed = %v, want %v", changed, tt.changed)
			}
			if got := lock.Pages["docs/a.md"]; !reflect.DeepEqual(got, *tt.want) {
				t.Errorf("entry = %+v, want %+v", got, *tt.want)
			}
		})
	}
}

func TestReadLapse(t *testing.T) {
	const stateFile = "tally-state-file"
	tests := []struct {
		name       string
		setup      func(f *cliFixture)
		scoped     bool
		requests   int
		facttruth  []string // ids the fact-truth request names
		notTruth   []string // ids it leaves out
		blueprint  []string // text the blueprint request carries
		consistent bool     // the consistency request is sent
	}{
		{"an accepted entry with no facts reads whole", func(f *cliFixture) {
			f.draft(tallyBriefJSON(t), f.rev("HEAD:"+tallyBP))
			f.setEntry(func(e *blueprint.LockEntry) { e.Facts = nil })
		}, false, 3, []string{stateFile, "tally-count-cmd"}, nil, nil, true},
		{"a map that matches reads whole", func(f *cliFixture) {
			f.draft(tallyBriefJSON(t), f.rev("HEAD:"+tallyBP))
		}, false, 3, []string{stateFile, "tally-count-cmd"}, nil, nil, true},
		{"a map that differs by one bullet's anchor span reads that fact", func(f *cliFixture) {
			f.draft(tallyBriefJSON(t), f.rev("HEAD:"+tallyBP))
			f.editFactAnchor()
		}, true, 3, []string{stateFile}, []string{"tally-count-cmd"}, []string{"changed since the accepted read: ", "(`stateFile` constant)"}, true},
		{"a map that names an id the blueprint no longer cites lists it and does not send it to the fact-truth seat", func(f *cliFixture) {
			f.draft(tallyBriefJSON(t), f.rev("HEAD:"+tallyBP))
			f.editFactAnchor()
			f.setEntry(func(e *blueprint.LockEntry) { e.Facts["tally-gone"] = "000000000000" })
		}, true, 3, []string{stateFile}, []string{"tally-gone"}, []string{"tally-gone", "no longer cited by the blueprint"}, true},
		{"a map whose only change is an id no longer cited sends no fact-truth request", func(f *cliFixture) {
			f.draft(tallyBriefJSON(t), f.rev("HEAD:"+tallyBP))
			f.setEntry(func(e *blueprint.LockEntry) { e.Facts["tally-gone"] = "000000000000" })
		}, true, 2, nil, nil, []string{"tally-gone", "no longer cited by the blueprint"}, true},
		{"an entry whose accepted id is stale reads whole whatever its facts", func(f *cliFixture) {
			f.draft(tallyBriefJSON(t), "a-stale-id")
			f.editFactAnchor()
		}, false, 3, []string{stateFile, "tally-count-cmd"}, nil, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newCLIFixture(t)
			tt.setup(f)
			code, out, errOut := f.read("--dry-run", f.blueprint())
			if code != 0 {
				t.Fatalf("exit %d\n%s%s", code, out, errOut)
			}
			if got := strings.Contains(out, "scoped:"); got != tt.scoped {
				t.Errorf("scoped = %v, want %v\n%s", got, tt.scoped, out)
			}
			if len(f.stub.counts) != tt.requests {
				t.Fatalf("%d requests, want %d", len(f.stub.counts), tt.requests)
			}
			bodies := f.stub.counts
			if tt.facttruth != nil {
				for _, id := range tt.facttruth {
					if !strings.Contains(bodies[2], id) {
						t.Errorf("the fact-truth request lacks %s", id)
					}
				}
			}
			for _, id := range tt.notTruth {
				if len(bodies) > 2 && strings.Contains(bodies[2], id) {
					t.Errorf("the fact-truth request names %s", id)
				}
			}
			for _, s := range tt.blueprint {
				if !strings.Contains(bodies[1], s) {
					t.Errorf("the blueprint request lacks %q", s)
				}
			}
			if tt.consistent != strings.Contains(bodies[0], "Check a documentation page plan for conflicts") {
				t.Errorf("consistency request sent = %v, want %v", !tt.consistent, tt.consistent)
			}
		})
	}
}

// addOtherPage adds a second managed page, a copy of the tally page, with its own lock entry.
func (f *cliFixture) addOtherPage(facts map[string]string) (blueprintFile string) {
	f.t.Helper()
	const rel = "blueprints/docs/other.blueprint.yaml"
	f.write(rel, f.read1(tallyBP))
	f.write("blueprints/outline.yaml", strings.Replace(f.read1("blueprints/outline.yaml"), "tracks:\n", "  - path: docs/other.md\n    slug: other\n    title: Other\n    type: task guide\n    job: |\n      Count more.\n    covers:\n      - Add items to the count.\ntracks:\n", 1))
	f.commit("add a second page", rel, "blueprints/outline.yaml")
	file := filepath.Join(f.root, tallyLock)
	lock, err := blueprint.ReadLock(file)
	if err != nil {
		f.t.Fatal(err)
	}
	lock.Pages["docs/other.md"] = blueprint.LockEntry{Accepted: f.rev("HEAD:" + rel), Facts: facts}
	if err := blueprint.WriteLock(file, lock); err != nil {
		f.t.Fatal(err)
	}
	f.commit("lock the second page", tallyLock)
	return filepath.Join(f.root, filepath.FromSlash(rel))
}

func TestPageRefusesLapsedPages(t *testing.T) {
	const stateLine = tallyPage + ": the accepted read lapsed; facts changed since it: tally-state-file. Run drafter read " + tallyBP + "."
	tests := []struct {
		name  string
		setup func(f *cliFixture)
		want  string // the refusal, or "" when the page drafts as before
	}{
		{"a fact changed since the accepted read", func(f *cliFixture) {
			f.draft(tallyBriefJSON(t), f.rev("HEAD:"+tallyBP))
			f.editFact()
		}, stateLine},
		{"a stale accepted id with a differing map", func(f *cliFixture) {
			f.draft(tallyBriefJSON(t), "a-stale-id")
			f.editFact()
		}, stateLine},
		{"an accepted entry with no facts recorded", func(f *cliFixture) {
			f.draft(tallyBriefJSON(t), f.rev("HEAD:"+tallyBP))
			f.setEntry(func(e *blueprint.LockEntry) { e.Facts = nil })
		}, tallyPage + ": the accepted read lapsed; no facts recorded. Run drafter read " + tallyBP + "."},
		{"a map that matches", func(f *cliFixture) {
			f.draft(tallyBriefJSON(t), f.rev("HEAD:"+tallyBP))
		}, ""},
		{"a stale accepted id with no facts is not a lapse", func(f *cliFixture) {
			f.draft(tallyBriefJSON(t), "a-stale-id")
			f.setEntry(func(e *blueprint.LockEntry) { e.Facts = nil })
		}, ""},
		{"an accepted entry with no facts in a project with no facts directory", func(f *cliFixture) {
			f.draft(tallyBriefJSON(t), f.rev("HEAD:"+tallyBP))
			f.setEntry(func(e *blueprint.LockEntry) { e.Facts = nil })
			f.git("rm", "-rq", "blueprints/facts")
			f.git("commit", "-q", "-m", "drop the facts directory")
		}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newCLIFixture(t)
			tt.setup(f)
			d, err := f.scopeWith(false, io.Discard)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("scopeFromLock refused: %v", err)
				}
				if len(d.handoffs)+len(d.skipped) != 1 {
					t.Errorf("handoffs %d, skipped %d, want the page decided", len(d.handoffs), len(d.skipped))
				}
				return
			}
			if err == nil || err.Error() != tt.want {
				t.Errorf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestPageCommandRefusesBeforeAnyRequest(t *testing.T) {
	t.Run("exit 2 with the one-line refusal and no request", func(t *testing.T) {
		f := newCLIFixture(t)
		f.draft(tallyBriefJSON(t), f.rev("HEAD:"+tallyBP))
		f.editFact()
		code, out, errOut := f.page(tallyPage)
		want := tallyPage + ": the accepted read lapsed; facts changed since it: tally-state-file. Run drafter read " + tallyBP + "."
		if code != exitCannotRun || !strings.Contains(out+errOut, want) {
			t.Errorf("exit %d, want %d with %q\n%s%s", code, exitCannotRun, want, out, errOut)
		}
		if n := len(f.stub.counts); n != 0 {
			t.Errorf("%d API calls for a refused call", n)
		}
	})

	t.Run("one lapsed page refuses the call and names only that page", func(t *testing.T) {
		f := newCLIFixture(t)
		f.draft(tallyBriefJSON(t), f.rev("HEAD:"+tallyBP))
		other := f.addOtherPage(map[string]string{"tally-state-file": "000000000000"})
		code, out, errOut := f.page(f.blueprint(), other)
		msg := out + errOut
		if code != exitCannotRun || !strings.Contains(msg, "docs/other.md: the accepted read lapsed") || strings.Contains(msg, tallyPage+":") {
			t.Errorf("exit %d, want %d naming only docs/other.md\n%s", code, exitCannotRun, msg)
		}
		if n := len(f.stub.counts); n != 0 {
			t.Errorf("%d API calls for a refused call", n)
		}
	})
}

func TestReadRejectsSyncWithBatch(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string // the usage error, or "" for a run
	}{
		{"--sync with --batch", []string{"--sync", "--batch", "x.blueprint.yaml"}, "--sync and --batch cannot be used together"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newReadCmd()
			cmd.SilenceUsage, cmd.SilenceErrors = true, true
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want one naming %q", err, tt.want)
			}
		})
	}
}
