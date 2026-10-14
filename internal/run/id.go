// Package run holds what drafter keeps for one run: its ID, its directory,
// its locks, its state file, and the retention that prunes finished runs.
package run

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const idTimeLayout = "20060102T150405Z"

var idPattern = regexp.MustCompile(`^(\d{8}T\d{6}Z)-[0-9a-f]{8}$`)

// NewID returns a run ID: the UTC time in the form 20261013T142233Z, a
// hyphen, and 8 hex characters from crypto/rand.
func NewID(now time.Time) string {
	var b [4]byte
	// rand.Read never returns an error as of Go 1.24.
	_, _ = rand.Read(b[:])
	return now.UTC().Format(idTimeLayout) + "-" + hex.EncodeToString(b[:])
}

// idTime returns the timestamp of a run ID, and false when name isn't one.
func idTime(name string) (time.Time, bool) {
	m := idPattern.FindStringSubmatch(name)
	if m == nil {
		return time.Time{}, false
	}
	t, err := time.Parse(idTimeLayout, m[1])
	return t, err == nil
}

// Repo returns the repo name that runs and the ledger file under: the base
// name of the main checkout, so every worktree of one repo shares it.
func Repo(root string) (string, error) {
	cmd := exec.Command("git", "-C", root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("find git directory of %s: %w", root, err)
	}
	common := strings.TrimSpace(string(out))
	return filepath.Base(filepath.Dir(common)), nil
}

// PageKey returns a page's directory name in a run: its repo-relative path
// with .md dropped and each / written as --.
func PageKey(path string) string {
	return strings.ReplaceAll(strings.TrimSuffix(path, ".md"), "/", "--")
}

// Create makes runs/<repo>/<id>/ under stateDir and returns its path. It
// fails when the directory already exists.
func Create(stateDir, repo, id string) (string, error) {
	parent := filepath.Join(stateDir, "runs", repo)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", parent, err)
	}
	dir := filepath.Join(parent, id)
	if err := os.Mkdir(dir, 0o755); err != nil {
		return "", fmt.Errorf("create run directory: %w", err)
	}
	return dir, nil
}
