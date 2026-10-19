package stock

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/plan"
	"github.com/glw907/drafter/internal/prompts"
)

var update = flag.Bool("update", false, "rewrite the golden renderings under testdata")

type stockMatrix struct {
	Host struct {
		Args     json.RawMessage `json:"args"`
		Resolved json.RawMessage `json:"resolved"`
	} `json:"host"`
	Cases []stockCase `json:"cases"`
}

type stockCase struct {
	Name    string          `json:"name"`
	Seat    string          `json:"seat"`
	Context prompts.Context `json:"context"`
}

func loadStockMatrix(t *testing.T) (stockMatrix, *prompts.Module) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "matrix.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m stockMatrix
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	mod, err := prompts.Load("drafter:prompts.js", Prompts, prompts.Host{Args: m.Host.Args, Resolved: m.Host.Resolved})
	if err != nil {
		t.Fatal(err)
	}
	return m, mod
}

func TestStockModuleSeats(t *testing.T) {
	_, mod := loadStockMatrix(t)
	want := []string{"blueprint", "coherence", "draft", "editor", "fact", "reader", "structure"}
	if got := mod.Seats(); !slices.Equal(got, want) {
		t.Errorf("seats = %v, want %v", got, want)
	}
}

// TestStockRenderings compares each rendering of the matrix with its golden file. Run with
// -update to rewrite them after a deliberate prompt change.
func TestStockRenderings(t *testing.T) {
	m, mod := loadStockMatrix(t)
	if len(m.Cases) != 6 {
		t.Fatalf("matrix has %d cases, want 6", len(m.Cases))
	}
	for _, c := range m.Cases {
		t.Run(c.Name, func(t *testing.T) {
			got, err := mod.Render(c.Seat, c.Context)
			if err != nil {
				t.Fatal(err)
			}
			golden := filepath.Join("testdata", c.Name+".txt")
			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Errorf("rendering differs from %s; rerun with -update if the change is deliberate\n got: %s", golden, got)
			}
		})
	}
}

func TestStockRenderingContent(t *testing.T) {
	m, mod := loadStockMatrix(t)
	tests := []struct {
		name    string
		token   string
		change  bool
		mention bool // whether the text may name a register
	}{
		{"whole-anatomy", "`drafter:anatomies.md#task-guide`", false, false},
		{"whole-register", "`docs/house-style.md#the-page-anatomies`", false, true},
		{"whole-neither", "`drafter:anatomies.md`", false, false},
		{"change-anatomy", "`drafter:anatomies.md#task-guide`", true, false},
		{"change-register", "`docs/house-style.md#the-page-anatomies`", true, true},
		{"change-neither", "`drafter:anatomies.md`", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := slices.IndexFunc(m.Cases, func(c stockCase) bool {
				return c.Name == tt.name
			})
			if i < 0 {
				t.Fatalf("no case %s in the matrix", tt.name)
			}
			got, err := mod.Render("blueprint", m.Cases[i].Context)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(got, tt.token) {
				t.Errorf("rendering lacks the anatomy token %s", tt.token)
			}
			if !tt.mention && strings.Contains(strings.ToLower(got), "register") {
				t.Errorf("rendering with no register names one")
			}
			for _, s := range []string{`starting "Nit:"`, `start its finding with
"Already present:"`, "Change read."} {
				if strings.Contains(got, s) != tt.change {
					t.Errorf("contains %q = %v, want %v", s, !tt.change, tt.change)
				}
			}
		})
	}
}

// TestStockAnatomySlugs proves the module's anatomy token names the heading drafter's slugger
// gives each stock anatomy, so the excerpt the token asks for exists.
func TestStockAnatomySlugs(t *testing.T) {
	var level2 []string
	for _, h := range plan.Headings(anatomies) {
		if h.Level == 2 {
			level2 = append(level2, h.Slug)
		}
	}
	if len(level2) != len(Anatomies) {
		t.Fatalf("anatomies.md has %d level-2 headings, want %d", len(level2), len(Anatomies))
	}
	for i, name := range Anatomies {
		t.Run(name, func(t *testing.T) {
			resolved := fmt.Sprintf(`{"pages":[{"path":"a.md","anatomy":%q}]}`, name)
			mod, err := prompts.Load("drafter:prompts.js", Prompts, prompts.Host{Resolved: json.RawMessage(resolved)})
			if err != nil {
				t.Fatal(err)
			}
			got, err := mod.Render("blueprint", prompts.Context{Page: json.RawMessage(`{"path":"a.md"}`), Round: prompts.Round1})
			if err != nil {
				t.Fatal(err)
			}
			if want := "`drafter:anatomies.md#" + level2[i] + "`"; !strings.Contains(got, want) {
				t.Errorf("rendering lacks %s", want)
			}
		})
	}
}

func TestStockAnatomyHeadings(t *testing.T) {
	var got []string
	sc := bufio.NewScanner(bytes.NewReader(anatomies))
	for sc.Scan() {
		if name, ok := strings.CutPrefix(sc.Text(), "## "); ok {
			got = append(got, strings.TrimSpace(name))
		}
	}
	if !slices.Equal(got, Anatomies) {
		t.Errorf("anatomies.md ## headings = %q, want %q", got, Anatomies)
	}
}

func TestStockFiles(t *testing.T) {
	tests := []struct {
		name string
		want []byte
		ok   bool
	}{
		{"prompts.js", Prompts, true},
		{"anatomies.md", anatomies, true},
		{"introductions.md", introductions, true},
		{"register.md", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Files(tt.name)
			if ok != tt.ok || !bytes.Equal(got, tt.want) {
				t.Errorf("Files(%q) = %d bytes, %v; want %d bytes, %v", tt.name, len(got), ok, len(tt.want), tt.ok)
			}
		})
	}
}
