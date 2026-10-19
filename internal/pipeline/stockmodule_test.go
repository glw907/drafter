package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glw907/drafter/internal/config"
	"github.com/glw907/drafter/stock"
)

func TestFingerprintLeavesOutRequirePrompts(t *testing.T) {
	c := &config.Config{Mode: "batch", RequirePrompts: true}
	if _, ok := fingerprint(c)["require_prompts"]; ok {
		t.Error("the fingerprint holds require_prompts, which would break a resume of every earlier run")
	}
	on, off := *c, *c
	off.RequirePrompts = false
	if got := changedSettings(fingerprint(&off), fingerprint(&on)); len(got) != 0 {
		t.Errorf("flipping require_prompts changes %v, want nothing", got)
	}
}

func TestStockModuleRun(t *testing.T) {
	t.Run("a non-dry blueprint read copies the stock module", func(t *testing.T) {
		h := newBlueprintHarness(t, withoutFacts, withPages("a"))
		h.cfg.Prompts = ""
		s := h.mustRun()
		if got := statusOf(s, pageA); got != StatusAccepted {
			t.Fatalf("status = %s (%s)\nstderr: %s", got, ruleOf(s, pageA), h.err.String())
		}
		copied, err := os.ReadFile(filepath.Join(h.runDir(s.RunID), "prompts.js"))
		if err != nil {
			t.Fatal(err)
		}
		if string(copied) != string(stock.Prompts) {
			t.Error("the run's prompts.js differs from stock.Prompts")
		}
		in := h.state(s.RunID).Inputs["prompts"]
		if in.Hash != hashBytes(stock.Prompts) || in.Source != "drafter:prompts.js" || in.Name != "prompts.js" {
			t.Errorf("state input = %+v, want the stock hash, source drafter:prompts.js, name prompts.js", in)
		}
		if s.Prompts != "drafter:prompts.js" || s.PromptsHash != config.ShortHash(stock.Prompts) {
			t.Errorf("report = %q %q, want drafter:prompts.js and the stock short hash", s.Prompts, s.PromptsHash)
		}
	})

	t.Run("a resume reports the run's copy, not the config's module", func(t *testing.T) {
		h := newBlueprintHarness(t, withoutFacts, withPages("a"))
		h.cfg.Prompts = ""
		h.api.crash = func(call string, ids []string) string {
			if call == "create" && strings.Contains(ids[0], "_s1_") {
				return "after"
			}
			return ""
		}
		if !dies(func() { _, _ = h.run() }) {
			t.Fatal("the run did not die after stage 1 was submitted")
		}
		id := h.runID()
		other := []byte(`return { blueprint: (c) => "a different module" };` + "\n")
		module := filepath.Join(t.TempDir(), "other.js")
		if err := os.WriteFile(module, other, 0o644); err != nil {
			t.Fatal(err)
		}
		h.cfg.Prompts = module
		h.cfg.RequirePrompts = true // a fingerprint saved without it must still pass checkConfig
		h.api.crash = nil
		s, err := h.resume(id)
		if err != nil {
			t.Fatalf("resume: %v\nstderr: %s", err, h.err.String())
		}
		if got := statusOf(s, pageA); got != StatusAccepted {
			t.Fatalf("status = %s (%s)", got, ruleOf(s, pageA))
		}
		if s.Prompts != "drafter:prompts.js" || s.PromptsHash != config.ShortHash(stock.Prompts) {
			t.Errorf("resume report = %q %q, want the run's stock copy %q", s.Prompts, s.PromptsHash, config.ShortHash(stock.Prompts))
		}
		if s.PromptsHash == config.ShortHash(other) {
			t.Error("the resume reported the config's module")
		}
	})

	t.Run("a state saved before the Source field reports the run copy", func(t *testing.T) {
		h := newBlueprintHarness(t, withoutFacts, withPages("a"))
		h.cfg.Prompts = ""
		h.api.crash = func(call string, ids []string) string {
			if call == "create" && strings.Contains(ids[0], "_s1_") {
				return "after"
			}
			return ""
		}
		if !dies(func() { _, _ = h.run() }) {
			t.Fatal("the run did not die")
		}
		id := h.runID()
		path := filepath.Join(h.runDir(id), "state.json")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var st map[string]any
		if err := json.Unmarshal(data, &st); err != nil {
			t.Fatal(err)
		}
		inputs, _ := st["inputs"].(map[string]any)
		prompts, ok := inputs["prompts"].(map[string]any)
		if !ok {
			t.Fatalf("state has no prompts input: %v", inputs)
		}
		delete(prompts, "source")
		data, err = json.Marshal(st)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		h.api.crash = nil
		s, err := h.resume(id)
		if err != nil {
			t.Fatalf("resume: %v", err)
		}
		if s.Prompts != "prompts.js (run copy)" {
			t.Errorf("Prompts = %q, want %q", s.Prompts, "prompts.js (run copy)")
		}
	})
}

func TestModuleSeatsRefuseBeforeSpend(t *testing.T) {
	tests := []struct {
		name    string
		harness func(t *testing.T) *harness
		module  string // a module file body; empty means the stock module
		want    []string
		notWant string
	}{
		{
			"a module with only the blueprint seat cannot draft a page",
			func(t *testing.T) *harness { return newHarness(t, withoutFacts, withPages("b")) },
			`return { blueprint: (c) => "x" };` + "\n",
			[]string{"no seat draft, structure, editor, fact, reader", "docs/guides/revoke-a-key.md"},
			"",
		},
		{
			"a seat left out of seats is not required",
			func(t *testing.T) *harness {
				return newHarness(t, withoutFacts, withPages("b"), withSeats("draft", "structure"))
			},
			`return { blueprint: (c) => "x" };` + "\n",
			[]string{"no seat draft, structure"},
			"editor",
		},
		{
			"a custom module names only the seats it lacks",
			func(t *testing.T) *harness { return newHarness(t, withoutFacts, withPages("b")) },
			`return { draft: (c) => "x" };` + "\n",
			[]string{"no seat structure, editor, fact, reader"},
			"draft,",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := tt.harness(t)
			h.cfg.Prompts = ""
			if tt.module != "" {
				h.cfg.Prompts = filepath.Join(t.TempDir(), "module.js")
				if err := os.WriteFile(h.cfg.Prompts, []byte(tt.module), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			_, err := h.run()
			if err == nil {
				t.Fatal("Run accepted a module that lacks a seat")
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q lacks %q", err, w)
				}
			}
			if tt.notWant != "" && strings.Contains(err.Error(), tt.notWant) {
				t.Errorf("error %q holds %q", err, tt.notWant)
			}
			if n := len(h.api.requests); n != 0 {
				t.Errorf("%d requests were sent before the refusal", n)
			}
			if lines := h.ledgerLines("reserve"); len(lines) != 0 {
				t.Errorf("%d reservations before the refusal", len(lines))
			}
		})
	}
}

func TestStockModuleServesEverySeatOfAPage(t *testing.T) {
	h := newHarness(t, withoutFacts, withPages("b"))
	h.cfg.Prompts = ""
	// Stub builders in the stock module serve every seat a plan page renders, so the load check
	// passes and the run reaches the API.
	if _, err := h.run(); err != nil && strings.Contains(err.Error(), "has no seat") {
		t.Fatalf("the stock module lacks a seat: %v", err)
	}
}

func TestBlueprintPageNeedsOnlyTheBlueprintSeat(t *testing.T) {
	h := newBlueprintHarness(t, withoutFacts, withPages("a"))
	h.cfg.Prompts = ""
	if _, err := h.run(); err != nil {
		t.Fatalf("the stock module's blueprint seat did not serve a blueprint page: %v", err)
	}
}
