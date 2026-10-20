package config

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// file is one layer's decoded settings. A pointer or nil-able field is nil when the layer
// leaves the setting out, which is how a layer overrides key by key.
type file struct {
	Prices          *pricesFile          `toml:"prices"`
	BillingCycleDay *int                 `toml:"billing_cycle_day"`
	WorkspaceID     *string              `toml:"workspace_id"`
	KeyEnv          *string              `toml:"key_env"`
	KeyCommand      *string              `toml:"key_command"`
	APIBaseURL      *string              `toml:"api_base_url"`
	PageCap         *float64             `toml:"page_cap"`
	CycleCeiling    *float64             `toml:"cycle_ceiling"`
	Mode            *string              `toml:"mode"`
	StateDir        *string              `toml:"state_dir"`
	RetentionDays   *int                 `toml:"retention_days"`
	Output          *string              `toml:"output"`
	PagesInFlight   *int                 `toml:"pages_in_flight"`
	Retries         *retriesFile         `toml:"retries"`
	Gate            *gateFile            `toml:"gate"`
	Prompts         *string              `toml:"prompts"`
	RequirePrompts  *bool                `toml:"require_prompts"`
	Seats           *[]string            `toml:"seats"`
	Seat            map[string]*seatFile `toml:"seat"`
	Facts           *factsFile           `toml:"facts"`
	Register        map[string][]string  `toml:"register"`
	Blueprints      *blueprintsFile      `toml:"blueprints"`
	InlineFiles     *[]string            `toml:"inline_files"`
}

type pricesFile struct {
	Date          *string               `toml:"date"`
	Source        *string               `toml:"source"`
	BatchDiscount *float64              `toml:"batch_discount"`
	Models        map[string]*priceFile `toml:"models"`
}

type rateFile struct {
	Input        *float64 `toml:"input"`
	Output       *float64 `toml:"output"`
	CacheWrite5m *float64 `toml:"cache_write_5m"`
	CacheWrite1h *float64 `toml:"cache_write_1h"`
	CacheRead    *float64 `toml:"cache_read"`
}

type priceFile struct {
	rateFile
	LongContext *tierFile `toml:"long_context"`
}

type tierFile struct {
	rateFile
	Threshold *int64 `toml:"threshold"`
}

type retriesFile struct {
	GateFix      *int `toml:"gate_fix"`
	EditContract *int `toml:"edit_contract"`
}

type gateFile struct {
	Command          *string `toml:"command"`
	IndexLinkCommand *string `toml:"index_link_command"`
	Protocol         *string `toml:"protocol"`
	Lane             *string `toml:"lane"`
	SectionPattern   *string `toml:"section_pattern"`
	FailurePattern   *string `toml:"failure_pattern"`
}

type seatFile struct {
	Agent     *string   `toml:"agent"`
	Model     *string   `toml:"model"`
	Effort    *string   `toml:"effort"`
	MaxTokens *int64    `toml:"max_tokens"`
	Inputs    *[]string `toml:"inputs"`
}

type blueprintsFile struct {
	Root      *string `toml:"root"`
	Required  *bool   `toml:"required"`
	Outline   *string `toml:"outline"`
	BriefPath *string `toml:"brief_path"`
}

type factsFile struct {
	Dir          *string `toml:"dir"`
	BulletFormat *string `toml:"bullet_format"`
}

// decode parses data strictly: an unknown key, a malformed document, and a value of the wrong
// type are each an error naming path and the line.
func decode(path string, data []byte) (*file, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("%s: file is empty", path)
	}
	var f file
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	err := dec.Decode(&f)
	if err == nil {
		return &f, nil
	}
	if missing, ok := errors.AsType[*toml.StrictMissingError](err); ok && len(missing.Errors) > 0 {
		first := missing.Errors[0]
		row, _ := first.Position()
		return nil, fmt.Errorf("%s:%d: unknown key %s", path, row, strings.Join(first.Key(), "."))
	}
	if de, ok := errors.AsType[*toml.DecodeError](err); ok {
		row, _ := de.Position()
		return nil, fmt.Errorf("%s:%d: %s", path, row, de.Error())
	}
	return nil, fmt.Errorf("%s: %w", path, err)
}
