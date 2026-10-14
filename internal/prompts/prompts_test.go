package prompts

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

var testHost = Host{
	Args:     json.RawMessage(`{"outline":true}`),
	Resolved: json.RawMessage(`{"pages":[{"path":"docs/a.md"}],"index":"docs/README.md"}`),
}

func loadFixture(t *testing.T) *Module {
	t.Helper()
	src, err := os.ReadFile("testdata/chain.js")
	if err != nil {
		t.Fatal(err)
	}
	m, err := Load("chain.js", src, testHost)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestLoadSyntheticChain(t *testing.T) {
	m := loadFixture(t)
	want := []string{"draft", "echo", "number", "reader", "throws"}
	if got := m.Seats(); !slices.Equal(got, want) {
		t.Errorf("Seats() = %v, want %v", got, want)
	}
	src, err := os.ReadFile("testdata/chain.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "// The phrase export const meta appears") {
		t.Error("fixture header comment missing")
	}
}

func TestRenderSeats(t *testing.T) {
	m := loadFixture(t)
	page := json.RawMessage(`{"path":"docs/a.md"}`)
	tests := []struct {
		name string
		seat string
		c    Context
		want string
	}{
		{"draft round 1", "draft", Context{Page: page, Round: Round1}, "Draft docs/a.md in docs/README.md."},
		{"draft round 2", "draft", Context{Page: page, Round: Round2, Findings: "1. fix"}, "Redraft docs/a.md: 1. fix"},
		{"draft after final", "draft", Context{Page: page, Round: RoundFinal, AfterFinalRead: true, Findings: "2. fix"}, "Redraft after the final read: 2. fix"},
		{"reader turn 1", "reader", Context{Page: page, Round: Round1, Turn: 1}, "Steps 1 and 2 for docs/a.md."},
		{"reader turn 2", "reader", Context{Page: page, Inputs: json.RawMessage(`{"job":"do it"}`), Round: Round1, Turn: 2}, "Step 3: do it"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := m.Render(tt.seat, tt.c)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("Render = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderErrors(t *testing.T) {
	m := loadFixture(t)
	page := json.RawMessage(`{"path":"docs/a.md"}`)
	tests := []struct {
		name    string
		seat    string
		wantErr string
	}{
		{"clean twin", "draft", ""},
		{"unknown seat", "nope", `no seat "nope"`},
		{"throwing builder", "throws", "builder broke"},
		{"number result", "number", "not a string"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := m.Render(tt.seat, Context{Page: page, Round: Round1})
			if tt.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), "chain.js") {
				t.Errorf("err = %v, want it to name chain.js and contain %q", err, tt.wantErr)
			}
		})
	}
	_, err := m.Render("throws", Context{Round: Round1})
	if err == nil || !strings.Contains(err.Error(), "chain.js:") {
		t.Errorf("throwing builder error %v does not carry a line", err)
	}
}

func TestEchoContext(t *testing.T) {
	m := loadFixture(t)
	c := Context{
		Page: json.RawMessage(`{"path":"docs/a.md","brief":"b.json","type":"task-guide"}`),
		Inputs: json.RawMessage(`{"job":"j","pageType":"task guide","claimInventory":[` +
			`{"claim":"c","disposition":"carried","factId":"f:abc123","section":"s"}],"factIds":["f:abc123"]}`),
		Round:          RoundFinal,
		Findings:       "1. a finding",
		AfterFinalRead: true,
		Scope:          "the scope",
		Where:          "section two",
		Narrow: json.RawMessage(`{"baseline":"obj1","own":{"verdict":"fix","summary":"s",` +
			`"findings":[{"finding":"f1"}]}}`),
		Turn:  2,
		Facts: true,
	}
	got, err := m.Render("echo", c)
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var g, w any
	if err := json.Unmarshal([]byte(got), &g); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(want, &w); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(g, w) {
		t.Errorf("echo = %s\nwant   %s", got, want)
	}
	for _, key := range []string{`"round":"final"`, `"afterFinalRead":true`, `"turn":2`, `"facts":true`} {
		if !strings.Contains(got, key) {
			t.Errorf("echo %s lacks %s", got, key)
		}
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		host    Host
		wantErr []string
	}{
		{"clean twin", "return { a: (c) => 'x' };", testHost, nil},
		{"agent at load", "await agent('probe');\nreturn {};", testHost, []string{"mod.js", "agent"}},
		{"require", "require('fs');\nreturn {};", testHost, []string{"ReferenceError", "require"}},
		{"fetch", "fetch('x');\nreturn {};", testHost, []string{"ReferenceError", "fetch"}},
		{"process", "process.exit();\nreturn {};", testHost, []string{"ReferenceError", "process"}},
		{"console", "console.log(1);\nreturn {};", testHost, []string{"ReferenceError", "console"}},
		{"setTimeout", "setTimeout(() => 1, 1);\nreturn {};", testHost, []string{"ReferenceError", "setTimeout"}},
		{"syntax error", "const a = 1;\nconst b = ;\nreturn {};", testHost, []string{"mod.js", "Line 2:"}},
		{"load-time throw", "const a = 1;\n\nthrow new Error('boom');", testHost, []string{"mod.js", "boom", ":3:"}},
		{"never settles", "await new Promise(() => {});\nreturn {};", testHost, []string{"mod.js", "never settles"}},
		{"no resolved", "return {};", Host{Args: json.RawMessage(`{}`)}, []string{"mod.js", "chain context"}},
		{"null resolved", "return {};", Host{Resolved: json.RawMessage(`null`)}, []string{"chain context"}},
		{"not a seat map", "return 5;", testHost, []string{"no seat map"}},
		{"seat not a function", "return { a: 1 };", testHost, []string{`seat "a"`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load("mod.js", []byte(tt.src), tt.host)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil {
				t.Fatal("Load succeeded, want error")
			}
			for _, w := range tt.wantErr {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("err = %q, want it to contain %q", err, w)
				}
			}
		})
	}
}

func TestExportMetaRewrite(t *testing.T) {
	src := "// export const meta in a comment\nexport const meta = { n: 1 };\nreturn { m: () => String(meta.n) };"
	m, err := Load("mod.js", []byte(src), testHost)
	if err != nil {
		t.Fatal(err)
	}
	got, err := m.Render("m", Context{Round: Round1})
	if err != nil || got != "1" {
		t.Errorf("Render = %q, %v; want 1", got, err)
	}
}

func TestHostGlobals(t *testing.T) {
	src := "return { a: () => JSON.stringify([args, DRAFTER_HOST.resolved.index]) };"
	m, err := Load("mod.js", []byte(src), testHost)
	if err != nil {
		t.Fatal(err)
	}
	got, err := m.Render("a", Context{Round: Round1})
	if err != nil {
		t.Fatal(err)
	}
	if want := `[{"outline":true},"docs/README.md"]`; got != want {
		t.Errorf("globals = %s, want %s", got, want)
	}
}

func TestRoundJSON(t *testing.T) {
	tests := []struct {
		r    Round
		json string
	}{{Round1, "1"}, {Round2, "2"}, {RoundFinal, `"final"`}}
	for _, tt := range tests {
		b, err := json.Marshal(tt.r)
		if err != nil || string(b) != tt.json {
			t.Errorf("Marshal(%d) = %s, %v; want %s", tt.r, b, err, tt.json)
		}
		var r Round
		if err := json.Unmarshal([]byte(tt.json), &r); err != nil || r != tt.r {
			t.Errorf("Unmarshal(%s) = %d, %v; want %d", tt.json, r, err, tt.r)
		}
	}
	var r Round
	if err := json.Unmarshal([]byte(`3`), &r); err == nil {
		t.Error("Unmarshal(3) succeeded")
	}
	if _, err := json.Marshal(Round(0)); err == nil {
		t.Error("Marshal(zero Round) succeeded")
	}
}
