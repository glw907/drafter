package blueprint

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"strings"

	"github.com/glw907/drafter/internal/coverage"
)

// IntroKey is the section-hash key for the text above the first section heading.
const IntroKey = "_intro"

// SectionHashes returns the lock's section hashes for page: one "sha256:<hex>" per blueprint
// section whose heading is on the page, keyed on the section id, and IntroKey for the text above
// the first of them. The text of a section is the region below its heading up to the next
// section's heading, as the coverage check splits a page, so a level-3 child's text is the
// child's own. A section with no matching heading has no key.
func SectionHashes(b *Blueprint, page []byte) map[string]string {
	headings := make([]string, len(b.Sections))
	for i := range b.Sections {
		headings[i] = b.Sections[i].Heading
	}
	preamble, text, found := coverage.Regions(headings, page)
	out := map[string]string{IntroKey: hashText(preamble)}
	for i, s := range b.Sections {
		if found[i] {
			out[s.ID] = hashText(text[i])
		}
	}
	return out
}

func hashText(s string) string {
	sum := sha256.Sum256([]byte(s))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// FileBlobID returns the git blob id of the file at path, from git hash-object without -w, so
// nothing is written to the object store. dir is the repository the command runs in.
func FileBlobID(dir, path string) (string, error) {
	return gitOut(dir, "hash-object", "--", path)
}

// CommitBlobID returns the git blob id of path as committed in commit.
func CommitBlobID(dir, commit, path string) (string, error) {
	return gitOut(dir, "rev-parse", commit+":"+path)
}

func gitOut(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	return strings.TrimSpace(string(out)), nil
}
