// Package vale checks that a project can run Vale and runs it on a page.
package vale

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// builtinStyle is the style Vale ships inside its binary, so it has no directory.
const builtinStyle = "Vale"

// config is the part of `vale ls-config` output the preflight reads.
type config struct {
	RootINI     string
	Paths       []string
	GBaseStyles []string
	SBaseStyles map[string][]string
}

// Preflight reports why Vale cannot lint the project at root, or nil when it can. It runs
// `vale ls-config` from root and requires that Vale loaded the project's own .vale.ini, not a
// global one or one in a parent directory, and that every style the config names has a
// directory under a styles path inside root. Vale itself exits 0 for a missing package, and a
// global styles directory can satisfy a style the project never pinned.
func Preflight(root string) error {
	if _, err := exec.LookPath("vale"); err != nil {
		return errors.New("vale is not on PATH; install Vale (https://vale.sh/docs/install)")
	}
	cmd := exec.Command("vale", "ls-config")
	cmd.Dir = root
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := firstLine(stderr.String(), err.Error())
		if strings.Contains(msg, "E201") {
			msg += " (a style the config names is not installed; run vale sync)"
		}
		return fmt.Errorf("vale ls-config failed: %s", msg)
	}
	var c config
	if err := json.Unmarshal(stdout.Bytes(), &c); err != nil {
		return fmt.Errorf("vale ls-config printed output that is not JSON: %v", err)
	}
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve project root: %w", err)
	}
	want, wantErr := filepath.EvalSymlinks(filepath.Join(rootReal, ".vale.ini"))
	got := ""
	if c.RootINI != "" {
		got, _ = filepath.EvalSymlinks(c.RootINI)
	}
	if wantErr != nil || got == "" || got != want {
		return fmt.Errorf("the project has no .vale.ini at %s; run drafter init", root)
	}
	var local []string
	for _, p := range c.Paths {
		real, err := filepath.EvalSymlinks(p)
		if err != nil {
			continue
		}
		if rel, err := filepath.Rel(rootReal, real); err == nil && filepath.IsLocal(rel) {
			local = append(local, real)
		}
	}
	styles := slices.Clone(c.GBaseStyles)
	for _, list := range c.SBaseStyles {
		styles = append(styles, list...)
	}
	slices.Sort(styles)
	for _, s := range slices.Compact(styles) {
		if s != builtinStyle && !hasStyleDir(local, s) {
			return fmt.Errorf("the Vale style %s has no directory under the project's styles path; run vale sync", s)
		}
	}
	return nil
}

func hasStyleDir(dirs []string, style string) bool {
	for _, d := range dirs {
		if info, err := os.Stat(filepath.Join(d, style)); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

// Lint runs Vale on page, a path relative to root, reporting error-level alerts only. Vale's
// alerts, one page:line:col:rule:message line each, are written to out. The code is Vale's exit
// status: 0 clean, 1 for an error-level alert. On a runtime error (exit 2) the error quotes the
// first line of Vale's stderr.
func Lint(root, page string, out io.Writer) (code int, err error) {
	cmd := exec.Command("vale", "--output=line", "--minAlertLevel=error", page)
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = out, &stderr
	runErr := cmd.Run()
	if runErr == nil {
		return 0, nil
	}
	ee, ok := errors.AsType[*exec.ExitError](runErr)
	if !ok {
		return 0, fmt.Errorf("run vale: %w", runErr)
	}
	if ee.ExitCode() == 1 {
		return 1, nil
	}
	return ee.ExitCode(), fmt.Errorf("vale exited %d: %s", ee.ExitCode(), firstLine(stderr.String(), ee.Error()))
}

func firstLine(s, fallback string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	if line == "" {
		return fallback
	}
	return line
}
