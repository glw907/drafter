package gate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/config"
)

// The recorded cairn patterns, from testdata/logs/PATTERNS.md.
const (
	cairnSection = `^== (check:\S+) ==$`
	cairnFailure = `✖|defect\(s\)|broken link|target not found|\.json: [^0-9]|^ +\S+\.md$|^ +[0-9]+:[0-9]+ +error |^\S+\.md:[0-9]+: ([^w]|w[^a])`

	pageA  = "docs/guides/widget-setup.md"
	briefA = "docs/briefs/guides/widget-setup.json"
)

var plainEnv = []string{"PATH=/usr/bin:/bin"}

func sectionNames(ss []Section) []string {
	var names []string
	for _, s := range ss {
		names = append(names, s.Name)
	}
	return names
}

func TestRunPlain(t *testing.T) {
	const script = "echo preamble; echo '== one =='; echo fine; echo '== two =='; echo BAD thing; echo to-stderr >&2; exit $CODE"
	tests := []struct {
		name         string
		code         int
		g            config.Gate
		green        bool
		unparsed     bool
		names        []string
		failingTotal int
	}{
		{"green no patterns", 0, config.Gate{}, true, false, []string{""}, 0},
		{"red no patterns", 3, config.Gate{}, false, false, []string{""}, 6},
		{"green both patterns", 0, config.Gate{SectionPattern: `^== (\w+) ==$`, FailurePattern: `BAD`}, true, false, []string{"", "one", "two"}, 0},
		{"red both patterns", 1, config.Gate{SectionPattern: `^== (\w+) ==$`, FailurePattern: `BAD`}, false, false, []string{"", "one", "two"}, 1},
		{"red section pattern only", 1, config.Gate{SectionPattern: `^== (\w+) ==$`}, false, false, []string{"", "one", "two"}, 4},
		{"red failure pattern only", 1, config.Gate{FailurePattern: `BAD`}, false, false, []string{""}, 1},
		{"red failure pattern matches nothing", 1, config.Gate{FailurePattern: `NEVER`}, false, true, []string{""}, 0},
		{"red section pattern matches nothing", 1, config.Gate{SectionPattern: `^## (\w+)$`}, false, true, []string{""}, 6},
		{"explicit plain protocol", 2, config.Gate{Protocol: "plain"}, false, false, []string{""}, 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logPath := filepath.Join(t.TempDir(), "gate.log")
			env := append(slices.Clone(plainEnv), "CODE="+strconv.Itoa(tt.code))
			res, err := Run(context.Background(), tt.g, env, t.TempDir(), script, logPath)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if res.Green != tt.green || res.Unparsed != tt.unparsed || res.Log != logPath {
				t.Errorf("Green=%v Unparsed=%v Log=%q, want %v %v %q", res.Green, res.Unparsed, res.Log, tt.green, tt.unparsed, logPath)
			}
			if got := sectionNames(res.Sections); !slices.Equal(got, tt.names) {
				t.Errorf("section names = %q, want %q", got, tt.names)
			}
			total := 0
			for _, s := range res.Sections {
				total += len(s.Failing)
			}
			if total != tt.failingTotal {
				t.Errorf("failing lines = %d, want %d", total, tt.failingTotal)
			}
			if tt.unparsed && len(res.Tail) != 6 {
				t.Errorf("tail = %q, want the 6 log lines", res.Tail)
			}
			if !tt.unparsed && res.Tail != nil {
				t.Errorf("tail = %q on a parsed log", res.Tail)
			}
		})
	}
}

func TestRunPlainDetails(t *testing.T) {
	root := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "gate.log")
	env := append(slices.Clone(plainEnv), "GATE_VAR=seen")
	res, err := Run(context.Background(), config.Gate{SectionPattern: `^== (\w+) ==$`}, env, root,
		"pwd; echo $GATE_VAR; echo '== s =='; echo a; echo b >&2", logPath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	pre := res.Sections[0]
	if pre.Name != "" || len(pre.Lines) != 2 || pre.Lines[1] != "seen" {
		t.Errorf("preamble = %+v, want an empty name and the pwd and env lines", pre)
	}
	if !strings.HasSuffix(pre.Lines[0], filepath.Base(root)) {
		t.Errorf("ran in %q, want the root %q", pre.Lines[0], root)
	}
	if got := res.Sections[1].Lines; !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("section lines = %q, want stdout and stderr combined", got)
	}
}

func TestRunErrors(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "gate.log")
	tests := []struct {
		name string
		g    config.Gate
		want string
	}{
		{"unknown protocol", config.Gate{Protocol: "bogus"}, "unknown gate protocol"},
		{"bad failure pattern", config.Gate{FailurePattern: "("}, "gate.failure_pattern"},
		{"bad section pattern", config.Gate{SectionPattern: "("}, "gate.section_pattern"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Run(context.Background(), tt.g, plainEnv, t.TempDir(), "true", logPath)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want it to contain %q", err, tt.want)
			}
		})
	}
	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := Run(ctx, config.Gate{}, plainEnv, t.TempDir(), "sleep 5", logPath)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
	})
}

// fakeGate stages the fake cairn-run-gate on a PATH and returns its state directory and the
// environment to run it under. The environment sets RUN_GATE_IF_BUSY, which Run must remove.
func fakeGate(t *testing.T, mode, logFile string, exit int) (dir string, env []string) {
	t.Helper()
	bin, err := filepath.Abs(filepath.Join("testdata", "bin"))
	if err != nil {
		t.Fatal(err)
	}
	dir = t.TempDir()
	logSrc, err := filepath.Abs(filepath.Join("testdata", "logs", logFile))
	if err != nil {
		t.Fatal(err)
	}
	return dir, []string{
		"PATH=" + bin + ":/usr/bin:/bin",
		"FAKE_GATE_DIR=" + dir,
		"FAKE_GATE_MODE=" + mode,
		"FAKE_GATE_LOG=" + logSrc,
		"FAKE_GATE_EXIT=" + strconv.Itoa(exit),
		"RUN_GATE_IF_BUSY=defer",
		"CAIRN_GATE_LANE=heavy",
	}
}

func readCalls(t *testing.T, dir string) [][]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "calls"))
	if err != nil {
		t.Fatal(err)
	}
	var calls [][]string
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		if strings.HasPrefix(line, "args: ") {
			calls = append(calls, nil)
		}
		calls[len(calls)-1] = append(calls[len(calls)-1], line)
	}
	return calls
}

var cairnGate = config.Gate{Protocol: "cairn-run-gate", Lane: "light", SectionPattern: cairnSection, FailurePattern: cairnFailure}

func TestRunCairnGate(t *testing.T) {
	dir, env := fakeGate(t, "", "provenance.log", 1)
	logPath := filepath.Join(t.TempDir(), "gate.log")
	res, err := Run(context.Background(), cairnGate, env, t.TempDir(), "make docs-gate", logPath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Green || res.Unparsed {
		t.Errorf("Green=%v Unparsed=%v, want a red parsed log", res.Green, res.Unparsed)
	}
	calls := readCalls(t, dir)
	if len(calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(calls))
	}
	want := []string{"args: --fresh make docs-gate " + logPath, "lane: light", "busy: unset"}
	if !slices.Equal(calls[0], want) {
		t.Errorf("call = %q, want %q", calls[0], want)
	}
	own, foreign := res.ForPage(pageA, briefA)
	if foreign || !slices.Equal(sectionNames(own), []string{"check:provenance"}) {
		t.Errorf("ForPage = %q foreign=%v, want the provenance section", sectionNames(own), foreign)
	}
	// The wrapper's own lines never reach the log, so none can read as a section line.
	for _, s := range res.Sections {
		for _, l := range s.Lines {
			if strings.HasPrefix(l, "gate exit:") {
				t.Errorf("log carries %q", l)
			}
		}
	}
}

func TestRunCairnGateGreen(t *testing.T) {
	_, env := fakeGate(t, "", "green.log", 0)
	res, err := Run(context.Background(), cairnGate, env, t.TempDir(), "make docs-gate", filepath.Join(t.TempDir(), "g.log"))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.Green {
		t.Error("Green = false on gate exit: 0")
	}
}

func TestRunCairnGateReissue(t *testing.T) {
	dir, env := fakeGate(t, "busy", "vale.log", 1)
	logPath := filepath.Join(t.TempDir(), "gate.log")
	res, err := Run(context.Background(), cairnGate, env, t.TempDir(), "make docs-gate", logPath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Green {
		t.Error("Green = true on gate exit: 1")
	}
	calls := readCalls(t, dir)
	if len(calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(calls))
	}
	if calls[0][0] != "args: --fresh make docs-gate "+logPath {
		t.Errorf("first call = %q, want --fresh", calls[0][0])
	}
	if calls[1][0] != "args: make docs-gate "+logPath {
		t.Errorf("second call = %q, want the same command without --fresh", calls[1][0])
	}
	for _, c := range calls {
		if c[2] != "busy: unset" || c[1] != "lane: light" {
			t.Errorf("call env = %q, want lane light and RUN_GATE_IF_BUSY unset", c[1:])
		}
	}
}

func TestRunCairnGateVanished(t *testing.T) {
	_, env := fakeGate(t, "vanish", "green.log", 1)
	_, err := Run(context.Background(), cairnGate, env, t.TempDir(), "make docs-gate", filepath.Join(t.TempDir(), "g.log"))
	if !errors.Is(err, ErrInfra) {
		t.Errorf("err = %v, want ErrInfra", err)
	}
}

func TestRunCairnGateMissingWrapper(t *testing.T) {
	_, err := Run(context.Background(), cairnGate, plainEnv, t.TempDir(), "make docs-gate", filepath.Join(t.TempDir(), "g.log"))
	if err == nil || errors.Is(err, ErrInfra) {
		t.Errorf("err = %v, want a plain error naming the missing wrapper", err)
	}
}

func TestRunCairnGateNoLane(t *testing.T) {
	dir, env := fakeGate(t, "", "green.log", 0)
	g := cairnGate
	g.Lane = ""
	if _, err := Run(context.Background(), g, env, t.TempDir(), "x", filepath.Join(t.TempDir(), "g.log")); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := readCalls(t, dir)[0][1]; got != "lane: unset" {
		t.Errorf("call = %q, want no CAIRN_GATE_LANE when gate.lane is empty", got)
	}
}

// recorded runs the fake wrapper over one recorded log.
func recorded(t *testing.T, name string, exit int) Result {
	t.Helper()
	_, env := fakeGate(t, "", name, exit)
	res, err := Run(context.Background(), cairnGate, env, t.TempDir(), "make docs-gate", filepath.Join(t.TempDir(), "g.log"))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return res
}

func TestForPageRecordedLogs(t *testing.T) {
	tests := []struct {
		name    string
		log     string
		exit    int
		sects   []string
		foreign bool
	}{
		{"provenance names the brief", "provenance.log", 1, []string{"check:provenance"}, false},
		{"vale names the page", "vale.log", 1, []string{"check:vale", "check:provenance"}, false},
		{"twopage is page B's", "twopage.log", 1, nil, true},
		{"green is nobody's", "green.log", 0, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := recorded(t, tt.log, tt.exit)
			if res.Unparsed {
				t.Fatal("recorded log is unparsed")
			}
			own, foreign := res.ForPage(pageA, briefA)
			if got := sectionNames(own); !slices.Equal(got, tt.sects) || foreign != tt.foreign {
				t.Errorf("ForPage = %q foreign=%v, want %q foreign=%v", got, foreign, tt.sects, tt.foreign)
			}
		})
	}
}

func TestRecordedSections(t *testing.T) {
	res := recorded(t, "provenance.log", 1)
	if len(res.Sections) == 0 || res.Sections[0].Name != "" || res.Sections[0].Lines[0] != "wrote /work/project/dist/admin/styles.css (1024 bytes)" {
		t.Errorf("first section = %+v, want the unnamed preamble", res.Sections[0])
	}
	for _, s := range res.Sections {
		if s.Name == "check:provenance" {
			if len(s.Failing) != 2 || !strings.Contains(s.Failing[1], "unclassified page text") {
				t.Errorf("provenance failing = %q, want the defect count and the brief line", s.Failing)
			}
			return
		}
	}
	t.Error("no check:provenance section")
}

func TestRecordedGreenSectionsNameAAndDoNotFail(t *testing.T) {
	res := recorded(t, "green.log", 0)
	named := 0
	for _, s := range res.Sections {
		if len(s.Failing) != 0 {
			t.Errorf("section %q fails in a green log: %q", s.Name, s.Failing)
		}
		for _, l := range s.Lines {
			if namesPath(l, pageA) || namesPath(l, briefA) {
				named++
			}
		}
	}
	if named == 0 {
		t.Error("no green section names page A")
	}
}

func TestRecordedValeAttribution(t *testing.T) {
	res := recorded(t, "vale.log", 1)
	other, foreign := res.ForPage("docs/guides/other-page.md", "docs/briefs/guides/other-page.json")
	if len(other) != 0 || !foreign {
		t.Errorf("Vale error is page A's: ForPage(B) = %q foreign=%v", sectionNames(other), foreign)
	}
}

func TestUnparsedIsOwn(t *testing.T) {
	var b strings.Builder
	for i := range 80 {
		b.WriteString("line " + strconv.Itoa(i) + "\n")
	}
	logPath := filepath.Join(t.TempDir(), "gate.log")
	env := append(slices.Clone(plainEnv), "CODE=1")
	res, err := Run(context.Background(), config.Gate{FailurePattern: "NEVER"}, env, t.TempDir(),
		"i=0; while [ $i -lt 80 ]; do echo \"line $i\"; i=$((i+1)); done; exit $CODE", logPath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.Unparsed || len(res.Tail) != 60 || res.Tail[0] != "line 20" || res.Tail[59] != "line 79" {
		t.Fatalf("Unparsed=%v tail len=%d, want the last 60 lines", res.Unparsed, len(res.Tail))
	}
	own, foreign := res.ForPage("anything.md", "anything.json")
	if foreign || len(own) != 1 || !slices.Equal(own[0].Lines, res.Tail) || own[0].Name != "" {
		t.Errorf("ForPage = %+v foreign=%v, want the tail as the one own section", own, foreign)
	}
}

func TestForPageBoundaries(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{"bare", "docs/x.md", true},
		{"line number", "docs/x.md:12: bad", true},
		{"indented", "   docs/x.md", true},
		{"after colon", "error:docs/x.md", true},
		{"after quote", `see "docs/x.md"`, true},
		{"leading dot slash", "./docs/x.md", true},
		{"parent dir", "../docs/x.md", false},
		{"inside longer path", "examples/docs/x.md:3: bad", false},
		{"inside longer name", "mydocs/x.md", false},
		{"longer extension", "docs/x.mdx", false},
		{"second occurrence", "examples/docs/x.md and docs/x.md", true},
		{"absent", "nothing here", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := namesPath(tt.line, "docs/x.md"); got != tt.want {
				t.Errorf("namesPath(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
	r := Result{Sections: []Section{{Name: "s", Lines: []string{"examples/docs/x.md:1: bad"}, Failing: []string{"examples/docs/x.md:1: bad"}}}}
	own, foreign := r.ForPage("docs/x.md", "docs/x.json")
	if len(own) != 0 || !foreign {
		t.Errorf("ForPage = %v foreign=%v, want foreign", own, foreign)
	}
	r.Sections[0].Failing = append(r.Sections[0].Failing, "docs/x.json: broken")
	if own, foreign = r.ForPage("docs/x.md", "docs/x.json"); len(own) != 1 || foreign {
		t.Errorf("ForPage = %v foreign=%v, want the brief's section", own, foreign)
	}
}
