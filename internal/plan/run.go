package plan

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CheckFile reads the plan at file, decodes it, and runs the plan checks when it decoded
// without error. The returned error means the file could not be read; every fault in the plan
// itself comes back as a finding.
func CheckFile(file string, opts CheckOptions) ([]Finding, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read plan: %w", err)
	}
	p, findings := Decode(file, data)
	if p == nil || HasError(findings) {
		return findings, nil
	}
	return append(findings, Check(file, p, opts)...), nil
}

// ErrNotGitTree is wrapped by GitRoot when dir is not inside a git work tree, as distinct from
// git failing for some other reason.
var ErrNotGitTree = errors.New("not inside a git work tree")

// notGitTreeLine reports whether line, the first line of git's stderr under LC_ALL=C, is one of
// the two messages git prints when it finds no repository. Prefix matches keep the check
// independent of the mount point git names; other fatal lines (inside .git, a directory that
// is missing, a worktree whose repository moved) are different failures.
func notGitTreeLine(line string) bool {
	return strings.HasPrefix(line, "fatal: not a git repository (or any of the parent directories)") ||
		strings.HasPrefix(line, "fatal: not a git repository (or any parent up to mount point")
}

// GitRoot returns the top level of the git work tree that contains dir. A dir outside any work
// tree yields an error wrapping ErrNotGitTree; git failing to run, or failing for another
// reason, yields an error that says so.
func GitRoot(dir string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel")
	// git localizes its messages, and the classification below reads them.
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(out)), nil
	}
	ee, ok := errors.AsType[*exec.ExitError](err)
	if !ok {
		return "", fmt.Errorf("git could not run: %w", err)
	}
	line, _, _ := strings.Cut(strings.TrimSpace(stderr.String()), "\n")
	if ee.ExitCode() == 128 && notGitTreeLine(line) {
		return "", fmt.Errorf("%s is %w; pass --root", filepath.Clean(dir), ErrNotGitTree)
	}
	if line == "" {
		return "", fmt.Errorf("git rev-parse in %s: %v", filepath.Clean(dir), ee)
	}
	return "", fmt.Errorf("git rev-parse in %s: %s", filepath.Clean(dir), line)
}
