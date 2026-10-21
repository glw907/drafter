package main

import (
	"encoding/json"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/blueprint"
	"github.com/glw907/drafter/internal/config"
)

const (
	tallyPage  = "docs/count-items.md"
	tallyBrief = "blueprints/docs/count-items.brief.json"
	tallyBP    = "blueprints/docs/count-items.blueprint.yaml"
	tallyFacts = "blueprints/facts/tally.md"
	tallyLock  = "blueprints/drafted.lock"
)

var tallyDraft = `# Count items with tally

Tally counts the items you name.

## Before you begin

Tally keeps its count in a file named .tally in the current directory.

Run every tally command in the directory that holds the count file.

## Add items

Run ` + "`" + `tally count` + "`" + ` with the items you want to add.

The command ` + "`" + `tally count` + "`" + ` adds each item you name on the command line to the total.

The flag ` + "`" + `--by` + "`" + ` sets the amount each item adds, and the default is 1.

Pick the items you want to count.

## Read the total

Run ` + "`" + `tally total` + "`" + ` in the same directory.

The command ` + "`" + `tally total` + "`" + ` prints the saved total.

The file is named .tally.

## See also

For the rest of tally, see [the tally readme](../README.md).
`

// tallyBriefJSON is a brief for tallyDraft. extra sentences are appended.
func tallyBriefJSON(t *testing.T, extra ...map[string]any) string {
	t.Helper()
	sentences := []map[string]any{
		{"text": "Tally counts the items you name.", "id": "no-claim"},
		{"text": "Tally keeps its count in a file named .tally in the current directory.", "id": "tally-state-file"},
		{"text": "The command tally count adds each item you name on the command line to the total.", "id": "tally-count-cmd"},
		{"text": "The flag --by sets the amount each item adds, and the default is 1.", "id": "tally-by-flag"},
		{"text": "Pick the items you want to count.", "id": "no-claim"},
		{"text": "The command tally total prints the saved total.", "id": "tally-total-cmd"},
		{"text": "The command tally reset deletes the count file.", "id": "tally-reset-cmd"},
		{"text": "The file is named .tally.", "id": "tally-state-file"},
	}
	data, err := json.Marshal(map[string]any{"sentences": append(sentences, extra...)})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func (f *cliFixture) write(rel, body string) {
	f.t.Helper()
	p := filepath.Join(f.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *cliFixture) read1(rel string) string {
	f.t.Helper()
	data, err := os.ReadFile(filepath.Join(f.root, filepath.FromSlash(rel)))
	if err != nil {
		f.t.Fatal(err)
	}
	return string(data)
}

func (f *cliFixture) commit(msg string, files ...string) {
	f.t.Helper()
	f.git(append([]string{"add"}, files...)...)
	f.git("commit", "-q", "-m", msg)
}

func (f *cliFixture) rev(r string) string {
	f.t.Helper()
	out, err := gitText(f.root, "rev-parse", r)
	if err != nil {
		f.t.Fatal(err)
	}
	return out
}

// draft records the page as drafted from the committed blueprint: the page, its brief, and its
// lock entry go in one commit, as a run's output does.
func (f *cliFixture) draft(brief, accepted string) {
	f.t.Helper()
	f.write(tallyPage, tallyDraft)
	f.write(tallyBrief, brief)
	e, err := blueprint.DraftedEntry(f.root, f.rev("HEAD"), tallyBP, tallyPage, accepted)
	if err != nil {
		f.t.Fatal(err)
	}
	lock := &blueprint.Lock{Version: blueprint.Version, Pages: map[string]blueprint.LockEntry{tallyPage: e}}
	if err := blueprint.WriteLock(filepath.Join(f.root, tallyLock), lock); err != nil {
		f.t.Fatal(err)
	}
	f.commit("draft", tallyPage, tallyBrief, tallyLock)
}

// scope runs the lock scoping on the fixture's blueprint at HEAD.
func (f *cliFixture) scope() draftPlan {
	f.t.Helper()
	d, err := f.scopeWith(false, io.Discard)
	if err != nil {
		f.t.Fatal(err)
	}
	return d
}

// scopeWith runs the lock scoping with --whole set as whole, naming a lost bar commit on warn.
func (f *cliFixture) scopeWith(whole bool, warn io.Writer) (draftPlan, error) {
	f.t.Helper()
	c, err := config.Load(f.root, config.Flags{})
	if err != nil {
		f.t.Fatal(err)
	}
	head := f.rev("HEAD")
	hs, err := blueprintHandoffs(c, f.root, head, []string{f.blueprint()})
	if err != nil {
		f.t.Fatal(err)
	}
	return scopeFromLock(c, f.root, head, hs, whole, true, warn)
}

// editFact rewrites the shared fact's bullet and commits it.
func (f *cliFixture) editFact() {
	f.t.Helper()
	facts := f.read1(tallyFacts)
	f.write(tallyFacts, strings.Replace(facts, "in the current directory.", "in the directory where you run it.", 1))
	f.commit("edit the shared fact", tallyFacts)
}

func sectionIDs(d draftPlan) []string { return d.scopes[tallyPage].IDs }

func TestDraftScopeWholeCases(t *testing.T) {
	tests := []struct {
		name  string
		setup func(f *cliFixture)
		why   string
	}{
		{"no lock entry", func(f *cliFixture) {}, "no drafted entry"},
		{"an entry with only accepted", func(f *cliFixture) {
			lock := &blueprint.Lock{Version: blueprint.Version, Pages: map[string]blueprint.LockEntry{tallyPage: {Accepted: "abc"}}}
			if err := blueprint.WriteLock(filepath.Join(f.root, tallyLock), lock); err != nil {
				t.Fatal(err)
			}
			f.commit("accepted only", tallyLock)
		}, "no drafted entry"},
		{"the page absent at the start commit", func(f *cliFixture) {
			f.draft(tallyBriefJSON(t), "")
			f.git("rm", "-q", tallyPage)
			f.git("commit", "-q", "-m", "drop the page")
		}, "not committed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newCLIFixture(t)
			tt.setup(f)
			d := f.scope()
			sc, ok := d.scopes[tallyPage]
			if !ok || sc.Kind != blueprint.ScopeWhole || !strings.Contains(sc.Why, tt.why) {
				t.Fatalf("scope = %+v (present %v), want whole naming %q", sc, ok, tt.why)
			}
			if len(d.handoffs) != 1 || d.changes[tallyPage] != "" {
				t.Errorf("handoffs %d, change %q, want the page run whole with no change block", len(d.handoffs), d.changes[tallyPage])
			}
		})
	}

	t.Run("no commit holds the entry", func(t *testing.T) {
		f := newCLIFixture(t)
		f.draft(tallyBriefJSON(t), "")
		c, err := config.Load(f.root, config.Flags{})
		if err != nil {
			t.Fatal(err)
		}
		hs, err := blueprintHandoffs(c, f.root, f.rev("HEAD"), []string{f.blueprint()})
		if err != nil {
			t.Fatal(err)
		}
		stranger := blueprint.LockEntry{Page: "deadbeef", Blueprint: "cafe", Sections: map[string]string{"_intro": "sha256:0"}}
		ps := pageScopeFor(c, f.root, f.rev("HEAD"), tallyLock, stranger, hs[0])
		if ps.scope.Kind != blueprint.ScopeWhole || !strings.Contains(ps.scope.Why, "no commit introduced") {
			t.Errorf("scope = %+v, want whole naming the missing commit", ps.scope)
		}
	})
}

func TestDraftScopeFromTheLock(t *testing.T) {
	t.Run("nothing changed is up to date and not run", func(t *testing.T) {
		f := newCLIFixture(t)
		f.draft(tallyBriefJSON(t), "")
		d := f.scope()
		if len(d.handoffs) != 0 || len(d.skipped) != 1 {
			t.Fatalf("handoffs %d, skipped %d, want the page skipped", len(d.handoffs), len(d.skipped))
		}
		if s := d.skipped[0]; s.Status != statusUnchanged || s.Scope.Why != "up to date with drafted.lock" {
			t.Errorf("skipped = %+v", s)
		}
	})

	t.Run("a fact bullet scopes every section that cites it", func(t *testing.T) {
		f := newCLIFixture(t)
		f.draft(tallyBriefJSON(t), "")
		f.editFact()
		d := f.scope()
		sc := d.scopes[tallyPage]
		if sc.Kind != blueprint.ScopeScoped {
			t.Fatalf("scope = %+v, want scoped", sc)
		}
		if want := []string{"before-you-begin", "read-the-total"}; !slices.Equal(sc.IDs, want) {
			t.Errorf("IDs = %v, want the two sections that cite the fact %v", sc.IDs, want)
		}
		if !strings.Contains(d.changes[tallyPage], "tally-state-file") {
			t.Errorf("the change block lacks the changed fact:\n%s", d.changes[tallyPage])
		}
	})

	t.Run("a blueprint edit scopes the edited section only", func(t *testing.T) {
		f := newCLIFixture(t)
		f.draft(tallyBriefJSON(t), "")
		bp := f.read1(tallyBP)
		f.write(tallyBP, strings.Replace(bp, "Pick the items you want to count.", "Pick the items you want to count, and have them ready.", 1))
		f.commit("edit the sentence", tallyBP)
		d := f.scope()
		if want := []string{"add-items"}; !slices.Equal(sectionIDs(d), want) {
			t.Errorf("IDs = %v, want %v", sectionIDs(d), want)
		}
		block := d.changes[tallyPage]
		if !strings.Contains(block, "Blueprint file: "+tallyBP) || !strings.Contains(block, "have them ready") {
			t.Errorf("the change block should open its diff with the blueprint's path and hold the edit:\n%s", block)
		}
	})

	t.Run("a brief sentence in a third section widens the scope to it", func(t *testing.T) {
		f := newCLIFixture(t)
		// The sentence in "Add items" cites the shared fact, which the blueprint cites only from two other sections.
		brief := tallyBriefJSON(t, map[string]any{"text": "Pick the items you want to count.", "id": []string{"tally-state-file"}})
		f.draft(brief, "")
		f.editFact()
		d := f.scope()
		if want := []string{"before-you-begin", "add-items", "read-the-total"}; !slices.Equal(sectionIDs(d), want) {
			t.Errorf("IDs = %v, want %v in blueprint order", sectionIDs(d), want)
		}
	})

	t.Run("a preamble sentence citing the changed fact makes the scope whole", func(t *testing.T) {
		f := newCLIFixture(t)
		brief := tallyBriefJSON(t, map[string]any{"text": "Tally counts the items you name.", "id": "tally-state-file"})
		f.draft(brief, "")
		f.editFact()
		d := f.scope()
		sc := d.scopes[tallyPage]
		if sc.Kind != blueprint.ScopeWhole || !strings.Contains(sc.Why, "tally-state-file") {
			t.Errorf("scope = %+v, want whole naming the fact", sc)
		}
	})

	t.Run("a seeded page with no brief drafts whole after a one-item edit", func(t *testing.T) {
		f := newCLIFixture(t)
		f.draft(tallyBriefJSON(t), "")
		f.git("rm", "-q", tallyBrief)
		f.git("commit", "-q", "-m", "drop the brief, as a seeded page has none")
		bp := f.read1(tallyBP)
		f.write(tallyBP, strings.Replace(bp, "Pick the items you want to count.", "Pick the items you want to count, and have them ready.", 1))
		f.commit("edit the sentence", tallyBP)
		d := f.scope()
		sc := d.scopes[tallyPage]
		if sc.Kind != blueprint.ScopeWhole || !strings.Contains(sc.Why, "no brief") {
			t.Errorf("scope = %+v, want whole naming the missing brief", sc)
		}
	})

	t.Run("the walk ignores accepted", func(t *testing.T) {
		f := newCLIFixture(t)
		f.draft(tallyBriefJSON(t), "")
		f.editFact()
		// drafter read rewrites only accepted, in a commit after the fact edit.
		lock, err := blueprint.ReadLock(filepath.Join(f.root, tallyLock))
		if err != nil {
			t.Fatal(err)
		}
		e := lock.Pages[tallyPage]
		e.Accepted = f.rev("HEAD:" + tallyBP)
		lock.Pages[tallyPage] = e
		if err := blueprint.WriteLock(filepath.Join(f.root, tallyLock), lock); err != nil {
			t.Fatal(err)
		}
		f.commit("read accepted", tallyLock)
		d := f.scope()
		if want := []string{"before-you-begin", "read-the-total"}; !slices.Equal(sectionIDs(d), want) {
			t.Errorf("IDs = %v, want %v although the newest lock commit came after the fact edit", sectionIDs(d), want)
		}
		if !d.skipRead[tallyPage] {
			t.Error("the lock's accepted id equals the blueprint's blob, so the read should be skipped")
		}
	})
}

func TestSkipRead(t *testing.T) {
	tests := []struct {
		name     string
		accepted string
		blob     string
		want     bool
	}{
		{"accepted equals the blueprint at the start", "abc", "abc", true},
		{"a stale accepted id", "old", "abc", false},
		{"no accepted id", "", "abc", false},
		{"no accepted id and no blob", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := skipRead(blueprint.LockEntry{Accepted: tt.accepted}, tt.blob); got != tt.want {
				t.Errorf("skipRead = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSkipReadWithAFactOnlyChange(t *testing.T) {
	f := newCLIFixture(t)
	f.draft(tallyBriefJSON(t), f.rev("HEAD:"+tallyBP))
	f.editFact()
	d := f.scope()
	if !d.skipRead[tallyPage] {
		t.Error("a fact-only change leaves the blueprint equal to accepted, so the read is skipped")
	}

	f = newCLIFixture(t)
	f.draft(tallyBriefJSON(t), "a-stale-id")
	f.editFact()
	d = f.scope()
	if d.skipRead[tallyPage] {
		t.Error("a stale accepted id must not skip the read")
	}
}

func TestWidenIDs(t *testing.T) {
	bp, _ := blueprint.Decode(tallyBP, []byte(readFixture(t, tallyBP)))
	if bp == nil {
		t.Fatal("the fixture blueprint does not decode")
	}
	brief := func(sentences ...map[string]any) []byte {
		data, _ := json.Marshal(map[string]any{"sentences": sentences})
		return data
	}
	changed := blueprint.Scope{Kind: blueprint.ScopeScoped, IDs: []string{"before-you-begin"}, Bullets: []string{"tally-state-file"}}
	tests := []struct {
		name     string
		scope    blueprint.Scope
		brief    []byte
		wantIDs  []string
		wantFact string
	}{
		{"a sentence in another section widens", changed,
			brief(map[string]any{"text": "The command tally total prints the saved total.", "id": "tally-state-file"}),
			[]string{"before-you-begin", "read-the-total"}, ""},
		{"a sentence citing an unchanged fact does not", changed,
			brief(map[string]any{"text": "The command tally total prints the saved total.", "id": "tally-total-cmd"}),
			[]string{"before-you-begin"}, ""},
		{"a preamble sentence returns the fact", changed,
			brief(map[string]any{"text": "Tally counts the items you name.", "id": []string{"tally-total-cmd", "tally-state-file"}}),
			nil, "tally-state-file"},
		{"a sentence found nowhere is ignored", changed,
			brief(map[string]any{"text": "Nothing like this is on the page.", "id": "tally-state-file"}),
			[]string{"before-you-begin"}, ""},
		{"a brief that is not JSON leaves the ids alone", changed, []byte("{"), []string{"before-you-begin"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ids, fact := widenIDs(tt.scope, bp, []byte(tallyDraft), tt.brief)
			if !slices.Equal(ids, tt.wantIDs) || fact != tt.wantFact {
				t.Errorf("widenIDs = %v, %q; want %v, %q", ids, fact, tt.wantIDs, tt.wantFact)
			}
		})
	}
}

func readFixture(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "e2e", "testdata", "cli", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestStockTrackRefusal(t *testing.T) {
	o := func(tracks ...blueprint.Track) *blueprint.Outline { return &blueprint.Outline{Tracks: tracks} }
	google := blueprint.Track{Name: "dev", Standard: "google", Paths: []string{"docs/**"}}
	tests := []struct {
		name    string
		outline *blueprint.Outline
		want    string
	}{
		{"no tracks declared", o(), ""},
		{"a matching google track", o(google), ""},
		{"no track matches the page", o(blueprint.Track{Name: "other", Standard: "google", Paths: []string{"guides/**"}}), "matches no track"},
		{"two tracks match", o(google, blueprint.Track{Name: "again", Standard: "google", Paths: []string{"docs/a.md"}}), "matches no track or more than one"},
		{"another standard", o(blueprint.Track{Name: "ms", Standard: "microsoft", Paths: []string{"docs/**"}}), `names the standard "microsoft"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stockTrackRefusal(tt.outline, path.Clean("docs/a.md"))
			if (tt.want == "") != (got == "") || !strings.Contains(got, tt.want) {
				t.Errorf("refusal = %q, want it to hold %q", got, tt.want)
			}
			if got != "" && !strings.Contains(got, "docs/a.md") {
				t.Errorf("refusal %q should name the page", got)
			}
		})
	}
}
