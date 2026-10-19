package blueprint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const draftedLock = "blueprints/drafted.lock"

// commitLock writes the lock with entries and commits it with msg, returning the commit.
func commitLock(t *testing.T, root string, entries map[string]LockEntry, msg string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(draftedLock))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteLock(path, &Lock{Version: Version, Pages: entries}); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", msg)
	return git(t, root, "rev-parse", "HEAD")
}

func commitFile(t *testing.T, root, rel, body, msg string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", msg)
	return git(t, root, "rev-parse", "HEAD")
}

func TestLockIntroducedAt(t *testing.T) {
	const page = "docs/a.md"
	drafted := LockEntry{Page: "p1", Blueprint: "b1", Sections: map[string]string{"_intro": "h0", "s": "h1"}}
	other := LockEntry{Page: "p9", Blueprint: "b9", Sections: map[string]string{"_intro": "h9"}}
	redrafted := LockEntry{Page: "p2", Blueprint: "b2", Sections: map[string]string{"_intro": "h0", "s": "h2"}}

	tests := []struct {
		name  string
		build func(t *testing.T, root string) (want string, cur LockEntry)
	}{
		{"the newest commit that wrote the entry, across unrelated commits", func(t *testing.T, root string) (string, LockEntry) {
			commitLock(t, root, map[string]LockEntry{page: drafted}, "lock A")
			commitFile(t, root, "blueprints/facts/f.md", "fact edit\n", "fact edit")
			want := commitLock(t, root, map[string]LockEntry{page: redrafted}, "lock B")
			commitFile(t, root, "README.md", "other\n", "other")
			return want, redrafted
		}},
		{"an unrelated page's entry changing does not move it", func(t *testing.T, root string) (string, LockEntry) {
			want := commitLock(t, root, map[string]LockEntry{page: drafted}, "lock A")
			commitLock(t, root, map[string]LockEntry{page: drafted, "docs/b.md": other}, "lock B adds another page")
			return want, drafted
		}},
		{"accepted alone changing does not move it", func(t *testing.T, root string) (string, LockEntry) {
			want := commitLock(t, root, map[string]LockEntry{page: drafted}, "draft")
			commitFile(t, root, "blueprints/facts/f.md", "fact and blueprint edit\n", "edit")
			read := drafted
			read.Accepted = "b-read"
			commitLock(t, root, map[string]LockEntry{page: read}, "read accepted")
			return want, drafted
		}},
		{"an entry the lock never held finds no commit", func(t *testing.T, root string) (string, LockEntry) {
			commitLock(t, root, map[string]LockEntry{page: drafted}, "lock A")
			return "", redrafted
		}},
		{"a historical lock that does not decode stops the walk", func(t *testing.T, root string) (string, LockEntry) {
			commitFile(t, root, draftedLock, "version: 1\nfuture-field: x\n", "an older format")
			return commitLock(t, root, map[string]LockEntry{page: drafted}, "lock A"), drafted
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := seedRepo(t, "# A\n")
			want, cur := tt.build(t, root)
			got, err := LockIntroducedAt(root, draftedLock, page, cur)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Errorf("commit = %q, want %q", got, want)
			}
		})
	}
}

func TestParentCommitAndBlobBytes(t *testing.T) {
	root := seedRepo(t, "# A\n")
	first := git(t, root, "rev-parse", "HEAD")
	second := commitFile(t, root, "x.txt", "x\n", "second")
	if got := ParentCommit(root, second); got != first {
		t.Errorf("parent of the second commit = %s, want %s", got, first)
	}
	if got := ParentCommit(root, first); got != first {
		t.Errorf("a commit with no parent = %s, want itself %s", got, first)
	}
	blob := git(t, root, "rev-parse", "HEAD:x.txt")
	if got := string(BlobBytes(root, blob)); got != "x\n" {
		t.Errorf("blob bytes = %q", got)
	}
	if BlobBytes(root, strings.Repeat("0", 40)) != nil {
		t.Error("a blob the store lacks should come back nil")
	}
}

func TestDraftedEntry(t *testing.T) {
	conv, _ := convertedFixture(t)
	root := seedRepo(t, seedPageText)
	commitFile(t, root, seedBP, string(conv.Data), "blueprint")
	start := git(t, root, "rev-parse", "HEAD")

	// The page is rewritten after the start commit and left uncommitted, as a run leaves it.
	written := "# Rotate the widget key\n\nNew intro.\n\n## Before you begin\nNew text.\n\n## Verify the key\nCheck it again.\n"
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(convertPage)), []byte(written), 0o644); err != nil {
		t.Fatal(err)
	}
	e, err := DraftedEntry(root, start, seedBP, convertPage, "acc")
	if err != nil {
		t.Fatal(err)
	}
	if want := git(t, root, "hash-object", convertPage); e.Page != want {
		t.Errorf("page = %s, want the written page's blob %s", e.Page, want)
	}
	if want := git(t, root, "rev-parse", start+":"+seedBP); e.Blueprint != want {
		t.Errorf("blueprint = %s, want the blueprint blob at the start commit %s", e.Blueprint, want)
	}
	if e.Accepted != "acc" {
		t.Errorf("accepted = %q", e.Accepted)
	}
	b, _ := Decode(seedBP, conv.Data)
	if want := SectionHashes(b, []byte(written)); len(e.Sections) != len(want) || e.Sections[IntroKey] != want[IntroKey] {
		t.Errorf("sections = %v, want %v", e.Sections, want)
	}
}
