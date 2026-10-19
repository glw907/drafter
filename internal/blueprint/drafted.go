package blueprint

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
)

// DraftedEntry is the lock entry for a page drafter just wrote: the page as it stands in the
// working tree, uncommitted, by blob id and section hash, and the blueprint as committed at start.
// accepted is the blueprint blob the entry records as read-accepted, which the caller chooses. The
// entry comes out of the same constructor conversion and seeding use.
func DraftedEntry(root, start, bp, pagePath, accepted string) (LockEntry, error) {
	bpBytes, err := gitRaw(root, "show", start+":"+bp)
	if err != nil {
		return LockEntry{}, err
	}
	b, findings := Decode(bp, bpBytes)
	if b == nil || len(findings) > 0 {
		return LockEntry{}, fmt.Errorf("%s at %s does not decode; run drafter check", bp, start)
	}
	bpBlob, err := CommitBlobID(root, start, bp)
	if err != nil {
		return LockEntry{}, err
	}
	pageBlob, err := FileBlobID(root, pagePath)
	if err != nil {
		return LockEntry{}, err
	}
	pageBytes, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(pagePath)))
	if err != nil {
		return LockEntry{}, fmt.Errorf("read the drafted page: %w", err)
	}
	e := newLockEntry(b, pageBlob, pageBytes, bpBlob)
	e.Accepted = accepted
	return e, nil
}

// LockIntroducedAt returns the commit that introduced cur as the lock entry of pagePath, or ""
// when no commit does. It walks the commits that changed the lock at lockRel, newest first, and
// keeps the oldest commit of the unbroken run whose entry for the page has cur's page, blueprint,
// and sections. Accepted is ignored: a read rewrites only it, so a read-accepted lock commit must
// not move the answer past the draft or seed before it. The walk stops at the first commit whose
// entry differs or is absent, whose lock does not decode (an older format), or that deleted the
// lock.
func LockIntroducedAt(root, lockRel, pagePath string, cur LockEntry) (string, error) {
	out, err := gitRaw(root, "log", "--format=%H", "--", lockRel)
	if err != nil {
		return "", err
	}
	found := ""
	for c := range strings.FieldsSeq(string(out)) {
		data, err := gitRaw(root, "show", c+":"+lockRel)
		if err != nil {
			break
		}
		l, err := DecodeLock(lockRel, data)
		if err != nil {
			break
		}
		e, ok := l.Pages[pagePath]
		if !ok || e.Page != cur.Page || e.Blueprint != cur.Blueprint || !maps.Equal(e.Sections, cur.Sections) {
			break
		}
		found = c
	}
	return found, nil
}

// ParentCommit returns the first parent of commit, or commit itself when it has none.
func ParentCommit(root, commit string) string {
	out, err := gitRaw(root, "rev-parse", "--verify", "--quiet", commit+"^")
	if err != nil {
		return commit
	}
	return strings.TrimSpace(string(out))
}

// BlobBytes returns the content of the git object id, or nil when the store lacks it.
func BlobBytes(root, id string) []byte {
	out, err := gitRaw(root, "cat-file", "blob", id)
	if err != nil {
		return nil
	}
	return out
}
