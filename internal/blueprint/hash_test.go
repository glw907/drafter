package blueprint

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func TestSectionHashes(t *testing.T) {
	b := &Blueprint{Sections: []Section{
		{ID: "alpha", Heading: "Alpha", Level: 2},
		{ID: "beta", Heading: "Beta", Level: 3},
		{ID: "gone", Heading: "Not on the page", Level: 2},
	}}
	page := "# Title\n\nIntro text.\n\n## Alpha\n\nAlpha body.\n\n### Beta\n\nBeta body.\n"
	at := func(s string) int { return strings.Index(page, s) }

	got := SectionHashes(b, []byte(page))
	want := map[string]string{
		IntroKey: sha(page[:at("## Alpha")-1]),
		"alpha":  sha(page[at("## Alpha")+len("## Alpha\n") : at("### Beta")-1]),
		"beta":   sha(page[at("### Beta")+len("### Beta\n"):]),
	}
	if len(got) != len(want) {
		t.Errorf("hashes = %v, want exactly the keys %v", got, want)
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("hash[%s] = %s, want %s", k, got[k], w)
		}
	}
	if _, ok := got["gone"]; ok {
		t.Error("a section with no matching heading got a key")
	}
}

func TestBlobIDs(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "page.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "page.md")
	git("commit", "-q", "-m", "page")

	const helloBlob = "ce013625030ba8dba906f756967f9e9ca394464a" // git's id for the blob "hello\n"
	committed, err := CommitBlobID(dir, "HEAD", "page.md")
	if err != nil || committed != helloBlob {
		t.Errorf("CommitBlobID = %q, %v; want %s", committed, err, helloBlob)
	}
	fresh, err := FileBlobID(dir, "page.md")
	if err != nil || fresh != helloBlob {
		t.Errorf("FileBlobID = %q, %v; want %s", fresh, err, helloBlob)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.blueprint.yaml"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if id, err := FileBlobID(dir, "new.blueprint.yaml"); err != nil || id != helloBlob {
		t.Errorf("FileBlobID of an uncommitted file = %q, %v", id, err)
	}
	if _, err := CommitBlobID(dir, "HEAD", "missing.md"); err == nil {
		t.Error("CommitBlobID of a path the commit lacks returned no error")
	}
}
