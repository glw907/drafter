package blueprint

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/glw907/drafter/internal/run"
)

// Seed gives each page of the named blueprints the lock entry conversion would have written, so a
// page that existed before its blueprint keeps its prose on the first run. A blueprint path is
// relative to the repository root at root, with slashes. It returns the page paths seeded, in
// argument order.
//
// Every blueprint is checked before any entry is written, and the first refusal names the page and
// what to do, leaving the lock untouched. A page qualifies when its entry holds an accepted id
// equal to the blueprint's blob at HEAD and no page id, and when neither file has uncommitted
// changes. The entry keeps accepted.
func Seed(root string, layout Layout, blueprints []string) ([]string, error) {
	gitDir, err := gitRaw(root, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return nil, err
	}
	// The worktree lock keeps a running drafter from reading a lock this call is about to change.
	l, err := run.TryLock(filepath.Join(strings.TrimSpace(string(gitDir)), "drafter.lock"))
	if err != nil {
		return nil, fmt.Errorf("another drafter run holds this worktree: %w", err)
	}
	defer func() { _ = l.Release() }()

	lockFile := filepath.Join(root, filepath.FromSlash(layout.LockPath))
	lock, err := ReadLock(lockFile)
	if err != nil {
		return nil, err
	}

	pages := make([]string, 0, len(blueprints))
	entries := make([]LockEntry, 0, len(blueprints))
	for _, bp := range blueprints {
		page, entry, err := seedEntry(root, layout, lock, bp)
		if err != nil {
			return nil, err
		}
		pages = append(pages, page)
		entries = append(entries, entry)
	}
	for i, page := range pages {
		lock.Pages[page] = entries[i]
	}
	if err := WriteLock(lockFile, lock); err != nil {
		return nil, err
	}
	return pages, nil
}

// seedEntry checks one blueprint against Seed's conditions and returns its page and new entry.
func seedEntry(root string, layout Layout, lock *Lock, bp string) (string, LockEntry, error) {
	page, ok := PagePath(layout.BlueprintRoot, bp)
	if !ok {
		return "", LockEntry{}, fmt.Errorf("%s is not a blueprint; name a .blueprint.md or .blueprint.yaml file under %s", bp, layout.BlueprintRoot)
	}
	for _, f := range []string{page, bp} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(f))); errors.Is(err, fs.ErrNotExist) {
			return "", LockEntry{}, fmt.Errorf("%s does not exist; seed needs the page and its blueprint", f)
		} else if err != nil {
			return "", LockEntry{}, fmt.Errorf("stat %s: %w", f, err)
		}
	}
	old, has := lock.Pages[page]
	switch {
	case !has || old.Accepted == "":
		return "", LockEntry{}, fmt.Errorf("%s has no accepted blueprint in %s; run drafter read first", page, LockFile)
	case old.Page != "":
		return "", LockEntry{}, fmt.Errorf("%s already records a drafted page; seed only gives an entry to a page drafter has not drafted", page)
	}
	status, err := gitRaw(root, "status", "--porcelain", "--", page, bp)
	if err != nil {
		return "", LockEntry{}, err
	}
	if len(bytes.TrimSpace(status)) > 0 {
		return "", LockEntry{}, fmt.Errorf("%s or %s has uncommitted changes; commit them first so the seed records both as they are", page, bp)
	}
	bpBlob, err := CommitBlobID(root, "HEAD", bp)
	if err != nil {
		return "", LockEntry{}, err
	}
	if bpBlob != old.Accepted {
		return "", LockEntry{}, fmt.Errorf("%s has changed since drafter read accepted it; run drafter read again", bp)
	}
	bpData, err := gitRaw(root, "show", "HEAD:"+bp)
	if err != nil {
		return "", LockEntry{}, err
	}
	b, findings := Load(bp, bpData)
	if b == nil || len(findings) > 0 {
		return "", LockEntry{}, fmt.Errorf("%s does not decode; run drafter check", bp)
	}
	pageBlob, pageBytes, err := headPage(root, page)
	if err != nil {
		return "", LockEntry{}, err
	}
	entry := newLockEntry(b, pageBlob, pageBytes, bpBlob)
	entry.Accepted = old.Accepted
	return page, entry, nil
}
