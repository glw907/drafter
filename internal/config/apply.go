package config

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// origin is the resolved value of one setting and the source that set it.
type origin struct {
	value string
	from  string
}

// newConfig returns a Config holding the defaults from the spec's settings table.
func newConfig(home string) *Config {
	c := &Config{
		BillingCycleDay: 1,
		KeyEnv:          "DRAFTER_API_KEY",
		APIBaseURL:      "https://api.anthropic.com",
		Mode:            "batch",
		StateDir:        filepath.Join(home, ".local", "state", "drafter"),
		RetentionDays:   30,
		Output:          "human",
		PagesInFlight:   3,
		Retries:         Retries{GateFix: 2, EditContract: 2},
		Gate:            Gate{Protocol: "plain"},
		Blueprints:      Blueprints{Root: "blueprints"},
		Seats:           append([]string(nil), seatNames...),
		Seat:            map[string]Seat{},
		Register:        map[string][]string{},
		src:             map[string]origin{},
		capSets:         map[string][]string{},
	}
	c.Prices.Models = map[string]Price{}
	for key, v := range map[string]any{
		"billing_cycle_day":     c.BillingCycleDay,
		"key_env":               c.KeyEnv,
		"api_base_url":          c.APIBaseURL,
		"mode":                  c.Mode,
		"state_dir":             c.StateDir,
		"retention_days":        c.RetentionDays,
		"output":                c.Output,
		"pages_in_flight":       c.PagesInFlight,
		"retries.gate_fix":      c.Retries.GateFix,
		"retries.edit_contract": c.Retries.EditContract,
		"gate.protocol":         c.Gate.Protocol,
		"blueprints.root":       c.Blueprints.Root,
		"blueprints.required":   c.Blueprints.Required,
		"seats":                 c.Seats,
	} {
		c.record(key, "default", format(v))
	}
	return c
}

func format(v any) string {
	switch v := v.(type) {
	case []string:
		return strings.Join(v, ", ")
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return fmt.Sprint(v)
}

func (c *Config) record(key, from, value string) {
	c.src[key] = origin{value: value, from: from}
}

// put sets *dst from v when the layer sets it, recording the layer as the source.
func put[T any](c *Config, l *layer, key string, dst *T, v *T) {
	if v == nil {
		return
	}
	*dst = *v
	c.record(key, l.from(key), format(*v))
}

// putPath is put for a path, which the layer resolves to an absolute one.
func putPath(c *Config, l *layer, key string, dst *string, v *string) {
	if v == nil {
		return
	}
	p := l.resolve(*v)
	put(c, l, key, dst, &p)
}

// putCap sets a cap to the lowest value any layer sets, and notes every layer that set it.
func putCap(c *Config, l *layer, key string, dst *float64, v *float64) {
	if v == nil {
		return
	}
	c.capSets[key] = append(c.capSets[key], l.from(key)+": "+format(*v))
	if *dst == 0 || *v <= *dst {
		put(c, l, key, dst, v)
	}
}

// apply merges the layer over what lower layers set: each key comes from the highest layer that
// sets it, and a list from a higher layer replaces a lower one.
func (c *Config) apply(l *layer) {
	f := l.f
	if p := f.Prices; p != nil {
		put(c, l, "prices.date", &c.Prices.Date, p.Date)
		put(c, l, "prices.source", &c.Prices.Source, p.Source)
		put(c, l, "prices.batch_discount", &c.Prices.BatchDiscount, p.BatchDiscount)
		for id, pf := range p.Models {
			c.applyPrice(l, id, pf)
		}
	}
	put(c, l, "billing_cycle_day", &c.BillingCycleDay, f.BillingCycleDay)
	put(c, l, "workspace_id", &c.WorkspaceID, f.WorkspaceID)
	put(c, l, "key_env", &c.KeyEnv, f.KeyEnv)
	put(c, l, "key_command", &c.KeyCommand, f.KeyCommand)
	put(c, l, "api_base_url", &c.APIBaseURL, f.APIBaseURL)
	putCap(c, l, "page_cap", &c.PageCap, f.PageCap)
	putCap(c, l, "cycle_ceiling", &c.CycleCeiling, f.CycleCeiling)
	put(c, l, "mode", &c.Mode, f.Mode)
	putPath(c, l, "state_dir", &c.StateDir, f.StateDir)
	put(c, l, "retention_days", &c.RetentionDays, f.RetentionDays)
	put(c, l, "output", &c.Output, f.Output)
	put(c, l, "pages_in_flight", &c.PagesInFlight, f.PagesInFlight)
	if r := f.Retries; r != nil {
		put(c, l, "retries.gate_fix", &c.Retries.GateFix, r.GateFix)
		put(c, l, "retries.edit_contract", &c.Retries.EditContract, r.EditContract)
	}
	if g := f.Gate; g != nil {
		put(c, l, "gate.command", &c.Gate.Command, g.Command)
		put(c, l, "gate.index_link_command", &c.Gate.IndexLinkCommand, g.IndexLinkCommand)
		put(c, l, "gate.protocol", &c.Gate.Protocol, g.Protocol)
		put(c, l, "gate.lane", &c.Gate.Lane, g.Lane)
		put(c, l, "gate.section_pattern", &c.Gate.SectionPattern, g.SectionPattern)
		put(c, l, "gate.failure_pattern", &c.Gate.FailurePattern, g.FailurePattern)
	}
	putPath(c, l, "prompts", &c.Prompts, f.Prompts)
	put(c, l, "seats", &c.Seats, f.Seats)
	for name, sf := range f.Seat {
		s := c.Seat[name]
		key := "seat." + name + "."
		putPath(c, l, key+"agent", &s.Agent, sf.Agent)
		put(c, l, key+"model", &s.Model, sf.Model)
		put(c, l, key+"effort", &s.Effort, sf.Effort)
		put(c, l, key+"max_tokens", &s.MaxTokens, sf.MaxTokens)
		put(c, l, key+"inputs", &s.Inputs, sf.Inputs)
		c.Seat[name] = s
	}
	if d := f.Facts; d != nil {
		putPath(c, l, "facts.dir", &c.Facts.Dir, d.Dir)
		put(c, l, "facts.bullet_format", &c.Facts.BulletFormat, d.BulletFormat)
	}
	if b := f.Blueprints; b != nil {
		put(c, l, "blueprints.root", &c.Blueprints.Root, b.Root)
		put(c, l, "blueprints.required", &c.Blueprints.Required, b.Required)
		put(c, l, "blueprints.outline", &c.Blueprints.Outline, b.Outline)
	}
	for track, sections := range f.Register {
		c.Register[track] = sections
		c.record("register."+track, l.from("register."+track), format(sections))
	}
	if f.InlineFiles != nil {
		var files []string
		for _, p := range *f.InlineFiles {
			files = append(files, l.resolve(p))
		}
		put(c, l, "inline_files", &c.InlineFiles, &files)
	}
}

func (c *Config) applyPrice(l *layer, id string, pf *priceFile) {
	key := "prices.models." + id + "."
	p := c.Prices.Models[id]
	putRates(c, l, key, [5]*float64{&p.Input, &p.Output, &p.CacheWrite5m, &p.CacheWrite1h, &p.CacheRead}, pf.rateFile)
	if tf := pf.LongContext; tf != nil {
		var t Tier
		if p.LongContext != nil {
			t = *p.LongContext
		}
		key := key + "long_context."
		put(c, l, key+"threshold", &t.Threshold, tf.Threshold)
		putRates(c, l, key, [5]*float64{&t.Input, &t.Output, &t.CacheWrite5m, &t.CacheWrite1h, &t.CacheRead}, tf.rateFile)
		p.LongContext = &t
	}
	c.Prices.Models[id] = p
}

func putRates(c *Config, l *layer, key string, dst [5]*float64, r rateFile) {
	names := [5]string{"input", "output", "cache_write_5m", "cache_write_1h", "cache_read"}
	vals := [5]*float64{r.Input, r.Output, r.CacheWrite5m, r.CacheWrite1h, r.CacheRead}
	for i, name := range names {
		put(c, l, key+name, dst[i], vals[i])
	}
}
