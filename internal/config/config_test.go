package config

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// layerFile returns the path a table row's layer name refers to.
func (s sandbox) layerFile(layer string) string {
	switch layer {
	case "user":
		return s.user
	case "project":
		return s.project()
	}
	return s.local()
}

// ignoreLocal makes the sandbox a git repository that ignores drafter.local.toml.
func (s sandbox) ignoreLocal(t *testing.T) {
	t.Helper()
	runGit(t, s.root, "init", "-q")
	writeFile(t, filepath.Join(s.root, ".gitignore"), "drafter.local.toml\n")
}

func TestLoadDefaults(t *testing.T) {
	newSandbox(t)
	c, err := Load("", Flags{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	home := os.Getenv("HOME")
	if c.Mode != "batch" || c.KeyEnv != "DRAFTER_API_KEY" || c.APIBaseURL != "https://api.anthropic.com" ||
		c.BillingCycleDay != 1 || c.RetentionDays != 30 || c.Output != "human" || c.PagesInFlight != 3 ||
		c.Retries != (Retries{GateFix: 2, EditContract: 2}) || c.Gate.Protocol != "plain" {
		t.Errorf("defaults wrong: %+v", c)
	}
	if want := filepath.Join(home, ".local", "state", "drafter"); c.StateDir != want {
		t.Errorf("state_dir = %q, want %q", c.StateDir, want)
	}
	if want := "draft, structure, editor, fact, reader, coverage, consistency"; row(t, c, "seats").Value != want {
		t.Errorf("seats = %q, want %q", row(t, c, "seats").Value, want)
	}
	if c.Root != "" {
		t.Errorf("Root = %q outside a project", c.Root)
	}
	if r := row(t, c, "mode"); r.From != "default" || r.Value != "batch" {
		t.Errorf("mode row = %+v, want the default batch", r)
	}
	if r := row(t, c, "workspace_id"); r.From != "unset" || r.Value != "" {
		t.Errorf("workspace_id row = %+v, want unset", r)
	}
}

func TestLoadRootWithoutProject(t *testing.T) {
	sb := newSandbox(t)
	c, err := Load(sb.root, Flags{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Root != "" {
		t.Errorf("Root = %q for a directory with no drafter.toml", c.Root)
	}

	writeFile(t, sb.local(), "mode = \"sync\"\n")
	_, err = Load(sb.root, Flags{})
	if err == nil {
		t.Fatal("Load accepted a drafter.local.toml outside a project")
	}
	want := sb.local() + ": drafter.local.toml needs a drafter.toml beside it; " + sb.root + " is not a drafter project"
	if err.Error() != want {
		t.Errorf("error %q, want %q", err, want)
	}

	writeFile(t, sb.project(), "mode = \"batch\"\n")
	sb.ignoreLocal(t)
	c, err = Load(sb.root, Flags{})
	if err != nil {
		t.Fatalf("Load with a project: %v", err)
	}
	if c.Root != sb.root {
		t.Errorf("Root = %q, want %q", c.Root, sb.root)
	}
}

func TestOverrideAtEachLayer(t *testing.T) {
	sb := newSandbox(t)
	sb.ignoreLocal(t)
	tests := []struct {
		name     string
		layers   map[string]string // layer name to its pages_in_flight line
		env      string
		want     int
		wantFrom string
	}{
		{"default", nil, "", 3, "default"},
		{"user", map[string]string{"user": "pages_in_flight = 4\n"}, "", 4, "user"},
		{"project over user", map[string]string{"user": "pages_in_flight = 4\n", "project": "pages_in_flight = 5\n"}, "", 5, "drafter.toml"},
		{"local over project", map[string]string{"user": "pages_in_flight = 4\n", "project": "pages_in_flight = 5\n", "local": "pages_in_flight = 6\n"}, "", 6, "drafter.local.toml"},
		{"env over local", map[string]string{"user": "pages_in_flight = 4\n", "project": "pages_in_flight = 5\n", "local": "pages_in_flight = 6\n"}, "7", 7, "env DRAFTER_PAGES_IN_FLIGHT"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, f := range []string{sb.user, sb.project(), sb.local()} {
				_ = os.Remove(f)
			}
			writeFile(t, sb.project(), "mode = \"batch\"\n")
			for layer, content := range tt.layers {
				writeFile(t, sb.layerFile(layer), content)
			}
			t.Setenv("DRAFTER_PAGES_IN_FLIGHT", tt.env)
			c, err := Load(sb.root, Flags{})
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if c.PagesInFlight != tt.want {
				t.Errorf("pages_in_flight = %d, want %d", c.PagesInFlight, tt.want)
			}
			if r := row(t, c, "pages_in_flight"); r.From != tt.wantFrom {
				t.Errorf("source %q, want %q", r.From, tt.wantFrom)
			}
		})
	}
}

func TestFlagsOverrideEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		flags    Flags
		wantMode string
		wantFrom string
		wantOut  string
		outFrom  string
	}{
		{"env alone", map[string]string{"DRAFTER_MODE": "sync"}, Flags{}, "sync", "env DRAFTER_MODE", "human", "default"},
		{"sync flag over env", map[string]string{"DRAFTER_MODE": "batch"}, Flags{Sync: true}, "sync", "flag --sync", "human", "default"},
		{"json flag over env", map[string]string{"DRAFTER_OUTPUT": "human"}, Flags{JSON: true}, "batch", "default", "json", "flag --json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newSandbox(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			c, err := Load("", tt.flags)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if c.Mode != tt.wantMode || row(t, c, "mode").From != tt.wantFrom {
				t.Errorf("mode = %q from %q, want %q from %q", c.Mode, row(t, c, "mode").From, tt.wantMode, tt.wantFrom)
			}
			if c.Output != tt.wantOut || row(t, c, "output").From != tt.outFrom {
				t.Errorf("output = %q from %q, want %q from %q", c.Output, row(t, c, "output").From, tt.wantOut, tt.outFrom)
			}
		})
	}
}

func TestLayerRefusals(t *testing.T) {
	tests := []struct {
		name    string
		layer   string
		content string
		line    string
		key     string
		moveTo  string
	}{
		{"state_dir in project", "project", "mode = \"batch\"\nstate_dir = \"/x\"\n", "2", "state_dir", "the user config"},
		{"state_dir in local", "local", "state_dir = \"/x\"\n", "1", "state_dir", "the user config"},
		{"inline_files in project", "project", "inline_files = [\"/x\"]\n", "1", "inline_files", "the user config or drafter.local.toml"},
		{"gate in user", "user", "mode = \"batch\"\n[gate]\ncommand = \"make\"\n", "2", "gate", "drafter.toml or drafter.local.toml"},
		{"prompts in user", "user", "prompts = \"p.js\"\n", "1", "prompts", "drafter.toml or drafter.local.toml"},
		{"seat in user", "user", "[seat.draft]\nmodel = \"m\"\n", "1", "seat", "drafter.toml or drafter.local.toml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sb := newSandbox(t)
			sb.ignoreLocal(t)
			writeFile(t, sb.project(), "mode = \"batch\"\n")
			writeFile(t, sb.layerFile(tt.layer), tt.content)
			_, err := Load(sb.root, Flags{})
			if err == nil {
				t.Fatal("Load accepted the setting")
			}
			want := sb.layerFile(tt.layer) + ":" + tt.line + ": " + tt.key + " is not allowed here; move it to " + tt.moveTo
			if err.Error() != want {
				t.Errorf("error %q, want %q", err, want)
			}
		})
	}
}

func TestEnvironmentMayCarryStateDir(t *testing.T) {
	newSandbox(t)
	t.Setenv("DRAFTER_STATE_DIR", "/var/tmp/drafter-state")
	c, err := Load("", Flags{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.StateDir != "/var/tmp/drafter-state" {
		t.Errorf("state_dir = %q", c.StateDir)
	}
	if r := row(t, c, "state_dir"); r.From != "env DRAFTER_STATE_DIR" {
		t.Errorf("source %q", r.From)
	}
}

func TestPathsMustStayInsideProject(t *testing.T) {
	tests := []struct {
		name    string
		content string
		line    string
		key     string
		path    string
		ok      bool
	}{
		{"climbing prompts", "prompts = \"../outside.js\"\n", "1", "prompts", "../outside.js", false},
		{"climbing past the root", "mode = \"batch\"\nprompts = \"docs/../../x.js\"\n", "2", "prompts", "docs/../../x.js", false},
		{"absolute prompts", "prompts = \"/etc/prompts.js\"\n", "1", "prompts", "/etc/prompts.js", false},
		{"seat agent", "[seat.editor]\nagent = \"../agents/editor.md\"\n", "2", "seat.editor.agent", "../agents/editor.md", false},
		{"facts dir", "[facts]\ndir = \"../facts\"\n", "2", "facts.dir", "../facts", false},
		{"inside prompts", "prompts = \"prompts/index.js\"\n", "", "", "", true},
		{"inside through a dot dot", "prompts = \"docs/../prompts/index.js\"\n", "", "", "", true},
		{"inside seat agent", "[seat.editor]\nagent = \"agents/editor.md\"\n", "", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sb := newSandbox(t)
			writeFile(t, sb.project(), tt.content)
			c, err := Load(sb.root, Flags{})
			if tt.ok {
				if err != nil {
					t.Fatalf("Load: %v", err)
				}
				if p := c.Prompts + c.Seat["editor"].Agent; !strings.HasPrefix(p, sb.root+string(filepath.Separator)) {
					t.Errorf("path %q is not absolute under the project", p)
				}
				return
			}
			if err == nil {
				t.Fatal("Load accepted a path outside the project")
			}
			want := sb.project() + ":" + tt.line + ": " + tt.key + " must resolve inside the project, but " +
				"\"" + tt.path + "\" does not; a machine-specific path belongs in drafter.local.toml or the user config"
			if err.Error() != want {
				t.Errorf("error %q, want %q", err, want)
			}
		})
	}
}

func TestLocalAndUserPathsMayLeaveTheProject(t *testing.T) {
	sb := newSandbox(t)
	sb.ignoreLocal(t)
	writeFile(t, sb.project(), "mode = \"batch\"\n")
	writeFile(t, sb.local(), "prompts = \"~/chain/prompts.js\"\n[seat.editor]\nagent = \"../agents/editor.md\"\n")
	writeFile(t, sb.user, "inline_files = [\"~/notes.md\", \"rel.md\"]\n")
	c, err := Load(sb.root, Flags{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	home := os.Getenv("HOME")
	if want := filepath.Join(home, "chain", "prompts.js"); c.Prompts != want {
		t.Errorf("prompts = %q, want %q", c.Prompts, want)
	}
	if want := filepath.Join(filepath.Dir(sb.root), "agents", "editor.md"); c.Seat["editor"].Agent != want {
		t.Errorf("agent = %q, want %q", c.Seat["editor"].Agent, want)
	}
	want := []string{filepath.Join(home, "notes.md"), filepath.Join(filepath.Dir(sb.user), "rel.md")}
	if !slices.Equal(c.InlineFiles, want) {
		t.Errorf("inline_files = %q, want %q", c.InlineFiles, want)
	}
}

func TestEnvironmentRelativePathResolvesFromWorkingDirectory(t *testing.T) {
	newSandbox(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	t.Setenv("DRAFTER_PROMPTS", "chain/prompts.js")
	c, err := Load("", Flags{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if want := filepath.Join(dir, "chain", "prompts.js"); c.Prompts != want {
		t.Errorf("prompts = %q, want %q", c.Prompts, want)
	}
}

func TestUnknownKeyAtEachLayer(t *testing.T) {
	tests := []struct {
		name    string
		layer   string
		content string
		line    string
		key     string
	}{
		{"user", "user", "mode = \"batch\"\nbogus = 1\n", "2", "bogus"},
		{"project", "project", "bogus = 1\n", "1", "bogus"},
		{"local", "local", "\n\nbogus = 1\n", "3", "bogus"},
		{"nested", "project", "[gate]\ncommand = \"make\"\nbogus = 1\n", "3", "gate.bogus"},
		{"seat table", "project", "[seat.draft]\neffrt = \"x\"\n", "2", "seat.draft.effrt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sb := newSandbox(t)
			sb.ignoreLocal(t)
			writeFile(t, sb.project(), "mode = \"batch\"\n")
			writeFile(t, sb.layerFile(tt.layer), tt.content)
			_, err := Load(sb.root, Flags{})
			if err == nil {
				t.Fatal("Load accepted an unknown key")
			}
			want := sb.layerFile(tt.layer) + ":" + tt.line + ": unknown key " + tt.key
			if err.Error() != want {
				t.Errorf("error %q, want %q", err, want)
			}
		})
	}
}

func TestEmptyAndMalformedFiles(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string // after the file path
	}{
		{"empty", "", ": file is empty"},
		{"blank", "  \n\n", ": file is empty"},
		{"malformed", "mode = \"batch\"\nmode = = 2\n", ":2: "},
		{"wrong type", "pages_in_flight = \"three\"\n", ":1: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sb := newSandbox(t)
			writeFile(t, sb.project(), tt.content)
			_, err := Load(sb.root, Flags{})
			if err == nil {
				t.Fatal("Load accepted the file")
			}
			if !strings.HasPrefix(err.Error(), sb.project()+tt.want) {
				t.Errorf("error %q, want prefix %q", err, sb.project()+tt.want)
			}
		})
	}

	t.Run("comments only is clean", func(t *testing.T) {
		sb := newSandbox(t)
		writeFile(t, sb.project(), "# nothing yet\n")
		if _, err := Load(sb.root, Flags{}); err != nil {
			t.Fatalf("Load: %v", err)
		}
	})
}

func TestCommandPlaceholders(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name:    "unknown placeholder",
			content: "[gate]\ncommand = \"run {other}\"\n",
			wantErr: ":2: gate.command uses the placeholder {other}; the placeholders are {page}, {brief}, {slug}, {baseline}",
		},
		{
			name:    "after a known one",
			content: "[gate]\nindex_link_command = \"x {page}{other}\"\n",
			wantErr: ":2: gate.index_link_command uses the placeholder {other}; the placeholders are {page}, {brief}, {slug}, {baseline}",
		},
		{
			name:    "seat input",
			content: "[seat.fact]\ninputs = [\"ok {slug}\", \"bad {nope}\"]\n",
			wantErr: ":2: seat.fact.inputs uses the placeholder {nope}; the placeholders are {page}, {brief}, {slug}, {baseline}",
		},
		{
			name:    "shell and awk are left alone",
			content: "[gate]\ncommand = \"for arm in a b; do echo ${arm} {page}; done; awk '{print $1}' {baseline} {brief} {slug}\"\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sb := newSandbox(t)
			writeFile(t, sb.project(), tt.content)
			_, err := Load(sb.root, Flags{})
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Load: %v", err)
				}
				return
			}
			if err == nil || err.Error() != sb.project()+tt.wantErr {
				t.Errorf("error %v, want %q", err, sb.project()+tt.wantErr)
			}
		})
	}
}

func TestEnvironment(t *testing.T) {
	t.Run("empty counts as unset", func(t *testing.T) {
		sb := newSandbox(t)
		writeFile(t, sb.user, "page_cap = 8\n")
		t.Setenv("DRAFTER_PAGE_CAP", "")
		c, err := Load("", Flags{})
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if c.PageCap != 8 || row(t, c, "page_cap").From != "user" {
			t.Errorf("page_cap = %v from %q, want 8 from user", c.PageCap, row(t, c, "page_cap").From)
		}
	})

	tests := []struct {
		name string
		env  string
		val  string
		want string
	}{
		{"non-number cap", "DRAFTER_PAGE_CAP", "abc", "DRAFTER_PAGE_CAP: page_cap must be a number, not \"abc\""},
		{"non-number ceiling", "DRAFTER_CYCLE_CEILING", "12x", "DRAFTER_CYCLE_CEILING: cycle_ceiling must be a number, not \"12x\""},
		{"non-integer count", "DRAFTER_PAGES_IN_FLIGHT", "2.5", "DRAFTER_PAGES_IN_FLIGHT: pages_in_flight must be a whole number, not \"2.5\""},
		{"nested key variable", "DRAFTER_RETRIES_GATE_FIX", "many", "DRAFTER_RETRIES_GATE_FIX: retries.gate_fix must be a whole number, not \"many\""},
		{"bad value names the variable", "DRAFTER_MODE", "turbo", "DRAFTER_MODE: mode must be batch or sync, not \"turbo\""},
		{"cap below one", "DRAFTER_PAGE_CAP", "0", "DRAFTER_PAGE_CAP: page_cap must be above 0, not 0"},
		{"infinite cap", "DRAFTER_PAGE_CAP", "inf", "DRAFTER_PAGE_CAP: page_cap must be above 0, not +Inf"},
		{"not a number cap", "DRAFTER_CYCLE_CEILING", "NaN", "DRAFTER_CYCLE_CEILING: cycle_ceiling must be above 0, not NaN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newSandbox(t)
			t.Setenv(tt.env, tt.val)
			_, err := Load("", Flags{})
			if err == nil || err.Error() != tt.want {
				t.Errorf("error %v, want %q", err, tt.want)
			}
		})
	}

	t.Run("nested variables resolve", func(t *testing.T) {
		newSandbox(t)
		t.Setenv("DRAFTER_GATE_PROTOCOL", "cairn-run-gate")
		t.Setenv("DRAFTER_GATE_INDEX_LINK_COMMAND", "links {page}")
		t.Setenv("DRAFTER_RETRIES_GATE_FIX", "5")
		t.Setenv("DRAFTER_FACTS_BULLET_FORMAT", "v1")
		c, err := Load("", Flags{})
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if c.Gate.Protocol != "cairn-run-gate" || c.Gate.IndexLinkCommand != "links {page}" ||
			c.Retries.GateFix != 5 || c.Facts.BulletFormat != "v1" {
			t.Errorf("env values not applied: %+v %+v %+v", c.Gate, c.Retries, c.Facts)
		}
		if r := row(t, c, "gate.index_link_command"); r.From != "env DRAFTER_GATE_INDEX_LINK_COMMAND" {
			t.Errorf("source %q", r.From)
		}
	})

	t.Run("other variables change nothing", func(t *testing.T) {
		newSandbox(t)
		base, err := Load("", Flags{})
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		t.Setenv("DRAFTER_API_KEY", "sk-test-not-a-key")
		t.Setenv("DRAFTER_DIR", "/somewhere")
		t.Setenv("DRAFTER_SEATS", "editor")
		t.Setenv("DRAFTER_SEAT_DRAFT_MODEL", "m")
		got, err := Load("", Flags{})
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if !reflect.DeepEqual(base, got) {
			t.Errorf("unrelated DRAFTER_ variables changed the configuration:\n%+v\n%+v", base, got)
		}
		for _, r := range got.Sources() {
			if strings.Contains(r.Value, "sk-test-not-a-key") {
				t.Errorf("row %s carries the key", r.Key)
			}
		}
	})
}

func TestValueChecks(t *testing.T) {
	tests := []struct {
		name    string
		layer   string
		content string
		line    string
		msg     string // after "<path>:<line>: "
	}{
		{"mode", "project", "mode = \"turbo\"\n", "1", "mode must be batch or sync, not \"turbo\""},
		{"output", "project", "\noutput = \"yaml\"\n", "2", "output must be human or json, not \"yaml\""},
		{"protocol", "project", "[gate]\nprotocol = \"fancy\"\n", "2", "gate.protocol must be plain or cairn-run-gate, not \"fancy\""},
		{"cycle day low", "user", "billing_cycle_day = 0\n", "1", "billing_cycle_day must be at least 1, not 0"},
		{"cycle day high", "user", "billing_cycle_day = 32\n", "1", "billing_cycle_day must be at most 31, not 32"},
		{"page cap zero", "user", "page_cap = 0\n", "1", "page_cap must be above 0, not 0"},
		{"page cap negative", "user", "page_cap = -2.5\n", "1", "page_cap must be above 0, not -2.5"},
		{"ceiling zero", "user", "cycle_ceiling = 0\n", "1", "cycle_ceiling must be above 0, not 0"},
		{"pages in flight", "project", "pages_in_flight = 0\n", "1", "pages_in_flight must be at least 1, not 0"},
		{"retention", "user", "retention_days = 0\n", "1", "retention_days must be at least 1, not 0"},
		{"gate fix retries", "project", "[retries]\ngate_fix = -1\n", "2", "retries.gate_fix must be at least 0, not -1"},
		{"edit contract retries", "project", "[retries]\nedit_contract = -1\n", "2", "retries.edit_contract must be at least 0, not -1"},
		{"discount one", "user", "[prices]\nbatch_discount = 1\n", "2", "prices.batch_discount must be from 0 up to but not including 1, not 1"},
		{"discount negative", "user", "[prices]\nbatch_discount = -0.1\n", "2", "prices.batch_discount must be from 0 up to but not including 1, not -0.1"},
		{"max tokens", "project", "[seat.draft]\nmax_tokens = 0\n", "2", "seat.draft.max_tokens must be at least 1, not 0"},
		{"seat name", "project", "[seat.writer]\nmodel = \"m\"\n", "1", "seat.writer is not a seat; the seats are draft, structure, editor, fact, reader, coverage, consistency, blueprint, coherence, facttruth"},
		{"seats without draft", "project", "seats = [\"editor\"]\n", "1", "seats must include draft, which always runs"},
		{"seats unknown", "project", "seats = [\"draft\", \"proofread\"]\n", "1", "seats names \"proofread\", which is not a seat; the seats are draft, structure, editor, fact, reader, coverage, consistency, blueprint, coherence, facttruth"},
		{"section pattern syntax", "project", "[gate]\nsection_pattern = \"(\"\n", "2", "gate.section_pattern is not a regular expression: error parsing regexp: missing closing ): `(`"},
		{"section pattern group", "project", "[gate]\nsection_pattern = \"^## .+$\"\n", "2", "gate.section_pattern needs a capture group for the section name"},
		{"failure pattern syntax", "project", "[gate]\nfailure_pattern = \"[\"\n", "2", "gate.failure_pattern is not a regular expression: error parsing regexp: missing closing ]: `[`"},
		{"bullet format", "project", "[facts]\nbullet_format = \"v2\"\n", "2", "facts.bullet_format must be v1, the only bullet format, not \"v2\""},
		{"lane without cairn-run-gate", "project", "[gate]\nprotocol = \"plain\"\nlane = \"light\"\n", "3", "gate.lane needs gate.protocol cairn-run-gate, but the resolved protocol is \"plain\""},
		{"lane with default protocol", "project", "[gate]\nlane = \"light\"\n", "2", "gate.lane needs gate.protocol cairn-run-gate, but the resolved protocol is \"plain\""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sb := newSandbox(t)
			writeFile(t, sb.project(), "mode = \"batch\"\n")
			writeFile(t, sb.layerFile(tt.layer), tt.content)
			_, err := Load(sb.root, Flags{})
			if err == nil {
				t.Fatal("Load accepted the value")
			}
			want := sb.layerFile(tt.layer) + ":" + tt.line + ": " + tt.msg
			if err.Error() != want {
				t.Errorf("error %q, want %q", err, want)
			}
		})
	}

	clean := []struct {
		name    string
		layer   string
		content string
	}{
		{"cycle day edges", "user", "billing_cycle_day = 31\n"},
		{"discount zero", "user", "[prices]\nbatch_discount = 0\n"},
		{"discount half", "user", "[prices]\nbatch_discount = 0.5\n"},
		{"retries zero", "project", "[retries]\ngate_fix = 0\nedit_contract = 0\n"},
		{"seats with draft", "project", "seats = [\"draft\", \"editor\"]\n"},
		{"patterns", "project", "[gate]\nsection_pattern = \"^## (.+)$\"\nfailure_pattern = \"FAIL\"\n"},
		{"bullet v1", "project", "[facts]\nbullet_format = \"v1\"\n"},
		{"lane with cairn-run-gate", "project", "[gate]\nprotocol = \"cairn-run-gate\"\nlane = \"light\"\n"},
		{"integer caps", "user", "page_cap = 5\ncycle_ceiling = 100\n"},
	}
	for _, tt := range clean {
		t.Run("clean "+tt.name, func(t *testing.T) {
			sb := newSandbox(t)
			writeFile(t, sb.project(), "mode = \"batch\"\n")
			writeFile(t, sb.layerFile(tt.layer), tt.content)
			if _, err := Load(sb.root, Flags{}); err != nil {
				t.Fatalf("Load: %v", err)
			}
		})
	}
}

func TestLaneChecksResolvedProtocol(t *testing.T) {
	sb := newSandbox(t)
	sb.ignoreLocal(t)
	writeFile(t, sb.project(), "[gate]\nprotocol = \"cairn-run-gate\"\n")
	writeFile(t, sb.local(), "[gate]\nlane = \"light\"\n")
	c, err := Load(sb.root, Flags{})
	if err != nil {
		t.Fatalf("lane in local over a project protocol: %v", err)
	}
	if c.Gate.Lane != "light" || c.Gate.Protocol != "cairn-run-gate" {
		t.Errorf("gate = %+v", c.Gate)
	}

	t.Setenv("DRAFTER_GATE_PROTOCOL", "plain")
	_, err = Load(sb.root, Flags{})
	want := sb.local() + ":2: gate.lane needs gate.protocol cairn-run-gate, but the resolved protocol is \"plain\""
	if err == nil || err.Error() != want {
		t.Errorf("error %v, want %q", err, want)
	}
}

func TestTablesMergeAcrossLayers(t *testing.T) {
	sb := newSandbox(t)
	sb.ignoreLocal(t)
	writeFile(t, sb.user, `[prices]
date = "2026-10-13"
source = "pricing page"

[prices.models."claude-haiku-5-5"]
input = 1
output = 5
cache_read = 0.1

[prices.models."claude-haiku-5-5".long_context]
threshold = 200000
input = 2
output = 10
`)
	writeFile(t, sb.project(), `seats = ["draft", "editor"]

[seat.draft]
model = "claude-haiku-5-5"
effort = "high"
max_tokens = 4000
inputs = ["cat {page}"]

[register]
docs = ["voice", "tone"]
`)
	writeFile(t, sb.local(), `seats = ["draft"]

[seat.draft]
model = "claude-opus-5"
inputs = ["head {page}"]

[prices.models."claude-haiku-5-5"]
output = 6

[prices.models."claude-haiku-5-5".long_context]
output = 12

[register]
docs = ["voice"]
`)
	c, err := Load(sb.root, Flags{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	seat := c.Seat["draft"]
	if seat.Model != "claude-opus-5" || seat.Effort != "high" || seat.MaxTokens != 4000 {
		t.Errorf("seat draft = %+v, want model from local, effort and max_tokens from the project", seat)
	}
	if !slices.Equal(seat.Inputs, []string{"head {page}"}) {
		t.Errorf("inputs = %q, want the local list to replace the project list", seat.Inputs)
	}
	if !slices.Equal(c.Seats, []string{"draft"}) {
		t.Errorf("seats = %q, want the local list", c.Seats)
	}
	if !slices.Equal(c.Register["docs"], []string{"voice"}) {
		t.Errorf("register.docs = %q, want the local list", c.Register["docs"])
	}
	if c.Prices.Date != "2026-10-13" || c.Prices.Source != "pricing page" {
		t.Errorf("prices header = %+v", c.Prices)
	}
	p := c.Prices.Models["claude-haiku-5-5"]
	if p.Input != 1 || p.Output != 6 || p.CacheRead != 0.1 {
		t.Errorf("price = %+v, want input and cache_read from user, output from local", p)
	}
	if p.LongContext == nil || *p.LongContext != (Tier{Threshold: 200000, Input: 2, Output: 12}) {
		t.Errorf("long context = %+v", p.LongContext)
	}
	for key, want := range map[string]string{
		"seat.draft.model":                      "drafter.local.toml",
		"seat.draft.effort":                     "drafter.toml",
		"seat.draft.inputs":                     "drafter.local.toml",
		"seats":                                 "drafter.local.toml",
		"register.docs":                         "drafter.local.toml",
		"prices.models.claude-haiku-5-5.input":  "user",
		"prices.models.claude-haiku-5-5.output": "drafter.local.toml",
		"prices.models.claude-haiku-5-5.long_context.input":  "user",
		"prices.models.claude-haiku-5-5.long_context.output": "drafter.local.toml",
	} {
		if got := row(t, c, key).From; got != want {
			t.Errorf("%s from %q, want %q", key, got, want)
		}
	}
}

func TestFactsProseSources(t *testing.T) {
	tests := []struct {
		name    string
		project string
		local   string
		want    []string
		printed string
	}{
		{"unset", "mode = \"batch\"\n", "", nil, ""},
		{"project layer", "[facts]\nprose_sources = [\"notes.md\", \"docs/owner.md\"]\n", "", []string{"notes.md", "docs/owner.md"}, "notes.md, docs/owner.md"},
		{"local layer replaces", "[facts]\nprose_sources = [\"notes.md\"]\n", "[facts]\nprose_sources = [\"mine.md\"]\n", []string{"mine.md"}, "mine.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sb := newSandbox(t)
			sb.ignoreLocal(t)
			writeFile(t, sb.project(), tt.project)
			if tt.local != "" {
				writeFile(t, sb.local(), tt.local)
			}
			c, err := Load(sb.root, Flags{})
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if !slices.Equal(c.Facts.ProseSources, tt.want) {
				t.Errorf("facts.prose_sources = %q, want %q", c.Facts.ProseSources, tt.want)
			}
			if got := row(t, c, "facts.prose_sources").Value; got != tt.printed {
				t.Errorf("Sources row value = %q, want %q", got, tt.printed)
			}
		})
	}
}
