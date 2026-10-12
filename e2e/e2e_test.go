package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

var binary string

func TestMain(m *testing.M) {
	os.Exit(buildAndRun(m))
}

func buildAndRun(m *testing.M) int {
	dir, err := os.MkdirTemp("", "drafter-e2e")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() { _ = os.RemoveAll(dir) }()
	binary = filepath.Join(dir, "drafter")
	if out, err := exec.Command("go", "build", "-o", binary, "../cmd/drafter").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build: %v\n%s", err, out)
		return 1
	}
	return m.Run()
}

func TestCheckPlanHelp(t *testing.T) {
	out, err := exec.Command(binary, "check-plan", "--help").CombinedOutput()
	if err != nil {
		t.Fatalf("check-plan --help: %v\n%s", err, out)
	}
}
