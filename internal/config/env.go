package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// envName is the variable that carries a setting: DRAFTER_ plus the key upper-cased, with each
// dot written as an underscore.
func envName(key string) string {
	return "DRAFTER_" + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
}

// fromEnv reads the environment layer. An empty variable counts as unset, and any DRAFTER_
// variable that is not a setting's is ignored, so DRAFTER_API_KEY and DRAFTER_DIR change
// nothing. Relative paths resolve from the working directory.
func fromEnv(home string) (*layer, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("resolve working directory: %w", err)
	}
	l := &layer{
		label: "env",
		dir:   dir,
		home:  home,
		f: &file{
			Retries: &retriesFile{},
			Gate:    &gateFile{},
			Facts:   &factsFile{},
		},
	}
	f := l.f
	for _, v := range []struct {
		key string
		dst **string
	}{
		{"workspace_id", &f.WorkspaceID},
		{"key_env", &f.KeyEnv},
		{"key_command", &f.KeyCommand},
		{"api_base_url", &f.APIBaseURL},
		{"mode", &f.Mode},
		{"state_dir", &f.StateDir},
		{"output", &f.Output},
		{"prompts", &f.Prompts},
		{"gate.command", &f.Gate.Command},
		{"gate.index_link_command", &f.Gate.IndexLinkCommand},
		{"gate.protocol", &f.Gate.Protocol},
		{"gate.lane", &f.Gate.Lane},
		{"gate.section_pattern", &f.Gate.SectionPattern},
		{"gate.failure_pattern", &f.Gate.FailurePattern},
		{"facts.dir", &f.Facts.Dir},
		{"facts.bullet_format", &f.Facts.BulletFormat},
	} {
		if s := os.Getenv(envName(v.key)); s != "" {
			*v.dst = &s
		}
	}
	for _, v := range []struct {
		key string
		dst **int
	}{
		{"billing_cycle_day", &f.BillingCycleDay},
		{"retention_days", &f.RetentionDays},
		{"pages_in_flight", &f.PagesInFlight},
		{"retries.gate_fix", &f.Retries.GateFix},
		{"retries.edit_contract", &f.Retries.EditContract},
	} {
		s := os.Getenv(envName(v.key))
		if s == "" {
			continue
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return nil, l.fail(v.key, "must be a whole number, not %q", s)
		}
		*v.dst = &n
	}
	for _, v := range []struct {
		key string
		dst **float64
	}{
		{"page_cap", &f.PageCap},
		{"cycle_ceiling", &f.CycleCeiling},
	} {
		s := os.Getenv(envName(v.key))
		if s == "" {
			continue
		}
		n, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, l.fail(v.key, "must be a number, not %q", s)
		}
		*v.dst = &n
	}
	return l, nil
}
