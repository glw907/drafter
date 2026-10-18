package blueprint

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/glw907/drafter/internal/run"
)

// LockFile is the name of the lock inside the blueprint root.
const LockFile = "drafted.lock"

// Installed reports what InstallConverted wrote.
type Installed struct {
	// Blueprint is the blueprint's path relative to the repository root, with slashes.
	Blueprint string
	// Seeded is true when the page's drafted.lock entry was written, which happens only when the
	// page file exists.
	Seeded bool
}

// Layout says where a project keeps its blueprints and its lock, both as slash paths relative to
// the repository root.
type Layout struct {
	BlueprintRoot string
	LockPath      string
}

// InstallConverted writes data as the blueprint of the page at pagePath, under the layout's
// blueprint root in the repository at root. It never overwrites a file: a blueprint already there
// is an error. When the page file exists it also seeds the page's lock entry from the page as
// committed at HEAD, so the next draft treats the page as drafted from this blueprint; it refuses
// a page with uncommitted changes and a page the lock already has an entry for.
//
// The blueprint and the lock are two writes. A crash between them leaves a blueprint with no
// entry, and the recovery is to delete the blueprint and convert again.
func InstallConverted(root string, layout Layout, pagePath string, data []byte) (Installed, error) {
	rel := BlueprintPath(layout.BlueprintRoot, pagePath)
	dest := filepath.Join(root, filepath.FromSlash(rel))
	res := Installed{Blueprint: rel}

	_, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(pagePath)))
	pageExists := statErr == nil
	if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
		return res, fmt.Errorf("stat page: %w", statErr)
	}
	if _, err := os.Lstat(dest); err == nil {
		return res, fmt.Errorf("%s already exists; convert never overwrites a blueprint", rel)
	}
	if !pageExists {
		return res, writeExclusive(dest, data)
	}

	gitDir, err := gitRaw(root, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return res, err
	}
	// The worktree lock keeps a running drafter from reading a lock this call is about to change.
	l, err := run.TryLock(filepath.Join(strings.TrimSpace(string(gitDir)), "drafter.lock"))
	if err != nil {
		return res, fmt.Errorf("another drafter run holds this worktree: %w", err)
	}
	defer func() { _ = l.Release() }()

	status, err := gitRaw(root, "status", "--porcelain", "--", pagePath)
	if err != nil {
		return res, err
	}
	if len(bytes.TrimSpace(status)) > 0 {
		return res, fmt.Errorf("%s has uncommitted changes; commit them first so the seed records the page as it is", pagePath)
	}
	lockFile := filepath.Join(root, filepath.FromSlash(layout.LockPath))
	lock, err := ReadLock(lockFile)
	if err != nil {
		return res, err
	}
	if _, has := lock.Pages[pagePath]; has {
		return res, fmt.Errorf("%s already has an entry for %s; remove it to convert again", LockFile, pagePath)
	}
	b, findings := Decode(rel, data)
	if b == nil || len(findings) > 0 {
		return res, errors.New("the blueprint to install does not decode")
	}
	pageBlob, err := CommitBlobID(root, "HEAD", pagePath)
	if err != nil {
		return res, err
	}
	pageBytes, err := gitRaw(root, "show", "HEAD:"+pagePath)
	if err != nil {
		return res, err
	}

	if err := writeExclusive(dest, data); err != nil {
		return res, err
	}
	bpBlob, err := FileBlobID(root, rel)
	if err != nil {
		return res, err
	}
	if lock.Pages == nil {
		lock.Pages = map[string]LockEntry{}
	}
	lock.Pages[pagePath] = LockEntry{Page: pageBlob, Blueprint: bpBlob, Sections: SectionHashes(b, pageBytes)}
	if err := WriteLock(lockFile, lock); err != nil {
		return res, fmt.Errorf("%w; the blueprint %s is written, so delete it and convert again", err, rel)
	}
	res.Seeded = true
	return res, nil
}

// writeExclusive creates dest with data and fails if dest exists. It writes a temporary file and
// links it into place, so dest is never seen half-written and never replaced.
func writeExclusive(dest string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create blueprint directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".convert-*")
	if err != nil {
		return fmt.Errorf("write blueprint: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write blueprint: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write blueprint: %w", err)
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return fmt.Errorf("write blueprint: %w", err)
	}
	if err := os.Link(tmp.Name(), dest); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("%s already exists; convert never overwrites a blueprint", filepath.Base(dest))
		}
		return fmt.Errorf("write blueprint: %w", err)
	}
	return nil
}

// RepoRoot returns the top level of the git repository that holds dir.
func RepoRoot(dir string) (string, error) {
	out, err := gitRaw(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// gitRaw runs git in dir and returns its stdout untrimmed, since a hash over page bytes depends on
// the final newline.
func gitRaw(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}
