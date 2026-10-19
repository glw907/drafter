package config

import "testing"

// complete returns a configuration that satisfies ScopeRun: every seat but consistency set up,
// and no facts store.
func complete() *Config {
	c := newConfig("/home/test")
	c.WorkspaceID = "wrkspc_01"
	c.PageCap = 5
	c.CycleCeiling = 100
	c.Gate.Command = "make check"
	c.Prompts = "/project/prompts/index.js"
	c.Prices.Models["claude-haiku-5-5"] = Price{Input: 1, Output: 5}
	for _, name := range seatNames {
		if name == "consistency" {
			continue
		}
		c.Seat[name] = Seat{Model: "claude-haiku-5-5", Effort: "high", MaxTokens: 4000}
	}
	return c
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name  string
		scope Scope
		edit  func(c *Config)
		want  string // the whole error text, one problem per line
	}{
		{"complete money", ScopeMoney, func(c *Config) {}, ""},
		{"complete run", ScopeRun, func(c *Config) {}, ""},
		{"money names a missing cap", ScopeMoney, func(c *Config) { c.PageCap = 0 }, "page_cap is not set"},
		{
			"money names every missing setting", ScopeMoney,
			func(c *Config) { c.WorkspaceID, c.PageCap, c.CycleCeiling = "", 0, 0 },
			"workspace_id is not set\npage_cap is not set\ncycle_ceiling is not set",
		},
		{"money passes without gate.command", ScopeMoney, func(c *Config) { c.Gate.Command = "" }, ""},
		{"money passes without seats", ScopeMoney, func(c *Config) { c.Seat = nil }, ""},
		{"run names missing gate.command", ScopeRun, func(c *Config) { c.Gate.Command = "" }, "gate.command is not set"},
		{"run passes with prompts unset", ScopeRun, func(c *Config) { c.Prompts = "" }, ""},
		{
			"run refuses the stock module under require_prompts", ScopeRun,
			func(c *Config) { c.Prompts, c.RequirePrompts = "", true },
			"prompts is not set, and require_prompts in drafter.toml refuses the stock module",
		},
		{"require_prompts passes with prompts set", ScopeRun, func(c *Config) { c.RequirePrompts = true }, ""},
		{"money ignores require_prompts", ScopeMoney, func(c *Config) { c.Prompts, c.RequirePrompts = "", true }, ""},
		{"run still requires the caps", ScopeRun, func(c *Config) { c.CycleCeiling = 0 }, "cycle_ceiling is not set"},
		{
			"run names a seat whose model has no price", ScopeRun,
			func(c *Config) {
				s := c.Seat["editor"]
				s.Model = "claude-unpriced"
				c.Seat["editor"] = s
			},
			"seat.editor.model \"claude-unpriced\" has no entry in prices.models",
		},
		{
			"run names a seat with nothing set", ScopeRun,
			func(c *Config) { delete(c.Seat, "reader") },
			"seat.reader.model is not set\nseat.reader.effort is not set\nseat.reader.max_tokens is not set",
		},
		{
			"consistency is not required without facts", ScopeRun,
			func(c *Config) { delete(c.Seat, "consistency") },
			"",
		},
		{
			"consistency is required with facts.dir", ScopeRun,
			func(c *Config) { c.Facts.Dir = "/project/docs/facts" },
			"seat.consistency.model is not set\nseat.consistency.effort is not set\nseat.consistency.max_tokens is not set",
		},
		{
			"consistency passes once set, with facts.dir", ScopeRun,
			func(c *Config) {
				c.Facts.Dir = "/project/docs/facts"
				c.Seat["consistency"] = Seat{Model: "claude-haiku-5-5", Effort: "high", MaxTokens: 4000}
			},
			"",
		},
		{
			"a seat left out of seats is not required", ScopeRun,
			func(c *Config) {
				c.Seats = []string{"draft"}
				c.Seat = map[string]Seat{"draft": c.Seat["draft"]}
			},
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := complete()
			tt.edit(c)
			err := c.Validate(tt.scope)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				return
			}
			if err == nil || err.Error() != tt.want {
				t.Errorf("error %v, want %q", err, tt.want)
			}
		})
	}
}
