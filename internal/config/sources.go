package config

import (
	"maps"
	"slices"
)

// settingKeys lists every setting that has a fixed key, in the order of the spec's settings
// table. Sources lists a setting with no value as unset, so a reader sees what is missing.
var settingKeys = []string{
	"prices.date", "prices.source", "prices.batch_discount",
	"billing_cycle_day", "workspace_id", "key_env", "key_command", "api_base_url",
	"page_cap", "cycle_ceiling", "mode", "state_dir", "retention_days", "output",
	"pages_in_flight", "retries.gate_fix", "retries.edit_contract",
	"gate.command", "gate.index_link_command", "gate.protocol", "gate.lane",
	"gate.section_pattern", "gate.failure_pattern",
	"prompts", "require_prompts", "seats", "facts.dir", "facts.bullet_format", "inline_files",
	"blueprints.root", "blueprints.required", "blueprints.outline", "blueprints.brief_path",
}

// Sources returns one row per setting with its value and the layer, variable, or flag that set
// it, and for each cap every source that set it. The fixed settings come first in table order,
// then the per-model, per-seat, and per-track keys sorted. A row never carries a key.
func (c *Config) Sources() []Source {
	var rows []Source
	for _, key := range settingKeys {
		if key == "prompts" {
			rows = append(rows, c.moduleRow())
			continue
		}
		rows = append(rows, c.row(key))
	}
	for _, key := range slices.Sorted(maps.Keys(c.src)) {
		if !slices.Contains(settingKeys, key) {
			rows = append(rows, c.row(key))
		}
	}
	return rows
}

func (c *Config) row(key string) Source {
	o, ok := c.src[key]
	if !ok {
		return Source{Key: key, From: "unset"}
	}
	return Source{Key: key, Value: o.value, From: o.from, Sets: c.capSets[key]}
}

// PricesDate returns the price table's date and the layer that set it. The merge is key by key,
// so a layer that overrides a rate without setting the date leaves the date to a lower layer.
func (c *Config) PricesDate() (date, from string) {
	r := c.row("prices.date")
	return r.Value, r.From
}
