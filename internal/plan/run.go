package plan

import (
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

// GitRoot returns the top level of the git work tree that contains dir.
func GitRoot(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("%s is not inside a git work tree; pass --root", filepath.Clean(dir))
	}
	return strings.TrimSpace(string(out)), nil
}
