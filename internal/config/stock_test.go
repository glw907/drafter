package config

import (
	"path/filepath"
	"testing"
)

const stockUserLayer = `key_env = "EXAMPLE_KEY"
workspace_id = "wrkspc_example"
page_cap = 10
cycle_ceiling = 100
`

func TestStockDefaultsRunWithoutSeatOrGateSettings(t *testing.T) {
	sb := newSandbox(t)
	writeFile(t, sb.user, stockUserLayer)
	writeFile(t, sb.project(), "prompts = \"prompts.js\"\n")
	writeFile(t, filepath.Join(sb.root, "prompts.js"), "return {}\n")
	c, err := Load(sb.root, Flags{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := c.Validate(ScopeRun); err != nil {
		t.Errorf("Validate(ScopeRun) = %v, want nil on the stock defaults", err)
	}
	tests := []struct {
		key, value, from string
	}{
		{"gate.command", "drafter gate --page {page}", "default"},
		{"gate.protocol", "plain", "default"},
		{"seat.draft.model", "claude-opus-5-5", "default"},
		{"seat.draft.effort", "high", "default"},
		{"seat.draft.max_tokens", "64000", "default"},
		{"seat.coverage.effort", "low", "default"},
		{"seat.consistency.effort", "low", "default"},
		{"seat.structure.effort", "medium", "default"},
		{"seat.blueprint.max_tokens", "32000", "default"},
		{"seat.reader.max_tokens", "32000", "default"},
		{"prices.date", "2026-10-18", "default"},
		{"prices.source", "https://platform.claude.com/docs/en/about-claude/pricing", "default"},
		{"prices.batch_discount", "0.5", "default"},
		{"prices.models.claude-opus-5-5.input", "4", "default"},
		{"prices.models.claude-opus-5-5.output", "20", "default"},
		{"prices.models.claude-opus-5-5.cache_write_5m", "5", "default"},
		{"prices.models.claude-opus-5-5.cache_write_1h", "8", "default"},
		{"prices.models.claude-opus-5-5.cache_read", "0.2", "default"},
		{"prompts", sb.root + "/prompts.js " + ShortHash([]byte("return {}\n")), "drafter.toml"},
		{"key_env", "EXAMPLE_KEY", "user"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if r := row(t, c, tt.key); r.Value != tt.value || r.From != tt.from {
				t.Errorf("%s = %q from %q, want %q from %q", tt.key, r.Value, r.From, tt.value, tt.from)
			}
		})
	}
	for _, name := range seatNames {
		if seat := c.Seat[name]; seat.Agent != "" || seat.Inputs != nil {
			t.Errorf("seat %s = %+v, want agent and inputs unset", name, seat)
		}
	}
	if c.Prices.Models["claude-opus-5-5"].LongContext != nil {
		t.Error("the stock table holds a long_context tier the pricing page does not list")
	}
}

func TestUserRateOverridesOneStockRate(t *testing.T) {
	sb := newSandbox(t)
	writeFile(t, sb.user, stockUserLayer+`
[prices.models."claude-opus-5-5"]
output = 25
`)
	c, err := Load("", Flags{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	tests := []struct {
		key, value, from string
	}{
		{"prices.models.claude-opus-5-5.output", "25", "user"},
		{"prices.models.claude-opus-5-5.input", "4", "default"},
		{"prices.models.claude-opus-5-5.cache_write_5m", "5", "default"},
		{"prices.models.claude-opus-5-5.cache_write_1h", "8", "default"},
		{"prices.models.claude-opus-5-5.cache_read", "0.2", "default"},
		{"prices.date", "2026-10-18", "default"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if r := row(t, c, tt.key); r.Value != tt.value || r.From != tt.from {
				t.Errorf("%s = %q from %q, want %q from %q", tt.key, r.Value, r.From, tt.value, tt.from)
			}
		})
	}
}

func TestPricesDate(t *testing.T) {
	tests := []struct {
		name     string
		user     string
		wantDate string
		wantFrom string
	}{
		{"stock", "", "2026-10-18", "default"},
		{"user rate without a date", "[prices.models.\"claude-opus-5-5\"]\noutput = 25\n", "2026-10-18", "default"},
		{"user date", "[prices]\ndate = \"2026-11-02\"\n", "2026-11-02", "user"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sb := newSandbox(t)
			if tt.user != "" {
				writeFile(t, sb.user, tt.user)
			}
			c, err := Load("", Flags{})
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if date, from := c.PricesDate(); date != tt.wantDate || from != tt.wantFrom {
				t.Errorf("PricesDate = %q from %q, want %q from %q", date, from, tt.wantDate, tt.wantFrom)
			}
		})
	}
}
