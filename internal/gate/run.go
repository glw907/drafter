package gate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/glw907/drafter/internal/config"
)

// ErrInfra marks a gate run that cairn-run-gate gave up on after its run vanished three times.
// The run stops, resumable, and no page escalates for it.
var ErrInfra = errors.New("gate infrastructure failure")

const tailLines = 60

// Section is one part of a gate log, opened by a line matching the section pattern.
type Section struct {
	Name    string // the pattern's first capture group; empty for the preamble or an unsplit log
	Lines   []string
	Failing []string // the lines of a red log that match the failure pattern
}

// Result is a finished gate run.
type Result struct {
	Green    bool
	Log      string // the log file's path
	Sections []Section
	Unparsed bool     // a red log the configured patterns could not read
	Tail     []string // the log's last 60 lines when Unparsed
}

// Run runs command once under the protocol g names, from root with env as the child's
// environment, and parses the log it leaves at logPath. A red gate is a Result, not an error.
func Run(ctx context.Context, g config.Gate, env []string, root, command, logPath string) (Result, error) {
	var green bool
	var err error
	switch g.Protocol {
	case "", "plain":
		green, err = runPlain(ctx, env, root, command, logPath)
	case "cairn-run-gate":
		green, err = runCairn(ctx, g.Lane, env, root, command, logPath)
	default:
		return Result{}, fmt.Errorf("unknown gate protocol %q", g.Protocol)
	}
	if err != nil {
		return Result{}, err
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		return Result{}, fmt.Errorf("read gate log: %w", err)
	}
	res, err := parse(g, string(data), green)
	if err != nil {
		return Result{}, err
	}
	res.Log = logPath
	return res, nil
}

func runPlain(ctx context.Context, env []string, root, command, logPath string) (bool, error) {
	f, err := os.Create(logPath)
	if err != nil {
		return false, fmt.Errorf("create gate log: %w", err)
	}
	defer func() { _ = f.Close() }()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	cmd.Dir = root
	// A nil Env makes exec inherit the parent's environment, key included.
	if env == nil {
		env = []string{}
	}
	cmd.Env = env
	cmd.Stdout = f
	cmd.Stderr = f
	err = cmd.Run()
	if err == nil {
		return true, nil
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if _, ok := errors.AsType[*exec.ExitError](err); ok {
		return false, nil
	}
	return false, fmt.Errorf("run gate: %w", err)
}

// runCairn follows cairn-run-gate's protocol. Its result is the "gate exit:" line on standard
// output, which the wrapper never writes to the log; exit 75 means the run is still in flight.
func runCairn(ctx context.Context, lane string, env []string, root, command, logPath string) (bool, error) {
	env = wrapperEnv(env, lane)
	bin, err := lookPath("cairn-run-gate", env)
	if err != nil {
		return false, err
	}
	args := []string{"--fresh", command, logPath}
	for {
		var stdout bytes.Buffer
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Dir = root
		cmd.Env = env
		cmd.Stdout = &stdout
		runErr := cmd.Run()
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		if line, ok := exitLine(stdout.String()); ok {
			if strings.Contains(line, "vanished 3 times") {
				return false, fmt.Errorf("%w: %s", ErrInfra, line)
			}
			n, err := strconv.Atoi(strings.Fields(line)[2])
			if err != nil {
				return false, fmt.Errorf("read gate exit line %q", line)
			}
			return n == 0, nil
		}
		if ee, ok := errors.AsType[*exec.ExitError](runErr); !ok || ee.ExitCode() != 75 {
			return false, fmt.Errorf("cairn-run-gate ended with no gate exit line: %v", runErr)
		}
		// Reattaching to the run in flight is never affected by --fresh, so the re-issue drops it.
		args = []string{command, logPath}
	}
}

func exitLine(out string) (string, bool) {
	for line := range strings.SplitSeq(out, "\n") {
		if strings.HasPrefix(line, "gate exit:") && len(strings.Fields(line)) > 2 {
			return line, true
		}
	}
	return "", false
}

// wrapperEnv sets CAIRN_GATE_LANE from lane and drops RUN_GATE_IF_BUSY, so the wrapper never
// answers with the deferred exit 76.
func wrapperEnv(env []string, lane string) []string {
	out := []string{}
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if name == "RUN_GATE_IF_BUSY" || name == "CAIRN_GATE_LANE" {
			continue
		}
		out = append(out, kv)
	}
	if lane != "" {
		out = append(out, "CAIRN_GATE_LANE="+lane)
	}
	return out
}

// lookPath finds name on the PATH that env carries, which exec.Command would not consult.
func lookPath(name string, env []string) (string, error) {
	for _, kv := range env {
		if p, ok := strings.CutPrefix(kv, "PATH="); ok {
			for _, dir := range filepath.SplitList(p) {
				path := filepath.Join(dir, name)
				if st, err := os.Stat(path); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
					return path, nil
				}
			}
			return "", fmt.Errorf("%s not found on the gate environment's PATH", name)
		}
	}
	return exec.LookPath(name)
}

func parse(g config.Gate, log string, green bool) (Result, error) {
	var sectionRe, failRe *regexp.Regexp
	var err error
	if g.SectionPattern != "" {
		if sectionRe, err = regexp.Compile(g.SectionPattern); err != nil {
			return Result{}, fmt.Errorf("gate.section_pattern: %w", err)
		}
	}
	if g.FailurePattern != "" {
		if failRe, err = regexp.Compile(g.FailurePattern); err != nil {
			return Result{}, fmt.Errorf("gate.failure_pattern: %w", err)
		}
	}
	var lines []string
	if log != "" {
		lines = strings.Split(strings.TrimSuffix(log, "\n"), "\n")
	}

	res := Result{Green: green}
	var cur Section
	opened := false // a section line has matched
	flush := func() {
		if opened || len(cur.Lines) > 0 {
			res.Sections = append(res.Sections, cur)
		}
	}
	failed := false
	for _, line := range lines {
		if sectionRe != nil {
			if m := sectionRe.FindStringSubmatch(line); m != nil {
				flush()
				name := m[0]
				if len(m) > 1 {
					name = m[1]
				}
				cur = Section{Name: name}
				opened = true
				continue
			}
		}
		cur.Lines = append(cur.Lines, line)
		if !green && (failRe == nil || failRe.MatchString(line)) {
			cur.Failing = append(cur.Failing, line)
			failed = true
		}
	}
	flush()

	if !green && (!failed || (sectionRe != nil && !opened)) {
		res.Unparsed = true
		res.Tail = lines[max(0, len(lines)-tailLines):]
	}
	return res, nil
}

// ForPage returns the sections with a failing line that names page or brief, and whether the
// gate is red with no such line. A path matches only at a path boundary: its left neighbor is the
// line's start, whitespace, or a character that cannot be part of a path, and its right neighbor
// does not continue a file name, so docs/x.md matches neither examples/docs/x.md nor docs/x.mdx.
// An unparsed red log counts as the page's own, its tail standing as the one section, since
// drafter cannot show it is foreign.
func (r Result) ForPage(page, brief string) (own []Section, foreign bool) {
	if r.Green {
		return nil, false
	}
	if r.Unparsed {
		return []Section{{Lines: r.Tail, Failing: r.Tail}}, false
	}
	for _, s := range r.Sections {
		for _, line := range s.Failing {
			if namesPath(line, page) || namesPath(line, brief) {
				own = append(own, s)
				break
			}
		}
	}
	return own, len(own) == 0
}

func namesPath(line, path string) bool {
	if path == "" {
		return false
	}
	for from := 0; ; {
		i := strings.Index(line[from:], path)
		if i < 0 {
			return false
		}
		start := from + i
		end := start + len(path)
		if leftBoundary(line, start) && (end == len(line) || !nameByte(line[end])) {
			return true
		}
		from = start + 1
	}
}

// leftBoundary reports whether a match at start begins a path, allowing one leading "./".
func leftBoundary(line string, start int) bool {
	if strings.HasSuffix(line[:start], "./") {
		start -= 2
	}
	return start == 0 || !pathByte(line[start-1])
}

// pathByte reports whether c can be part of a path to the left of a match.
func pathByte(c byte) bool {
	return nameByte(c) || c == '/' || c == '.' || c == '~' || c == '+' || c == '@' || c == '%'
}

// nameByte reports whether c continues a file name to the right of a match.
func nameByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-'
}
