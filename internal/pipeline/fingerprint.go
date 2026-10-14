package pipeline

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/glw907/drafter/internal/config"
)

// fingerprint lists the settings a run's requests and costs depend on, as text. A resume reads
// the caps, the key source, and workspace_id fresh and requires every setting here to be what it
// was at start; the prompts module and the agent definitions come from the run directory.
func fingerprint(c *config.Config) map[string]string {
	f := map[string]string{
		"mode":                  c.Mode,
		"seats":                 strings.Join(c.Seats, ","),
		"retries.gate_fix":      strconv.Itoa(c.Retries.GateFix),
		"retries.edit_contract": strconv.Itoa(c.Retries.EditContract),
		"gate.command":          c.Gate.Command,
		"gate.index_link":       c.Gate.IndexLinkCommand,
		"gate.protocol":         c.Gate.Protocol,
		"gate.lane":             c.Gate.Lane,
		"gate.section_pattern":  c.Gate.SectionPattern,
		"gate.failure_pattern":  c.Gate.FailurePattern,
		"facts.dir":             c.Facts.Dir,
		"facts.bullet_format":   c.Facts.BulletFormat,
		"inline_files":          strings.Join(c.InlineFiles, ","),
	}
	prices, err := json.Marshal(c.Prices)
	if err != nil {
		panic("pipeline: prices do not marshal: " + err.Error())
	}
	f["prices"] = string(prices)
	for name, s := range c.Seat {
		k := "seat." + name + "."
		f[k+"model"], f[k+"effort"] = s.Model, s.Effort
		f[k+"max_tokens"] = strconv.FormatInt(s.MaxTokens, 10)
		f[k+"inputs"] = strings.Join(s.Inputs, "\n")
		f[k+"agent"] = strconv.FormatBool(s.Agent != "")
	}
	return f
}

// changedSettings names each setting of now that differs from was, sorted.
func changedSettings(was, now map[string]string) []string {
	var out []string
	for k, v := range now {
		if prior, ok := was[k]; !ok || prior != v {
			out = append(out, k)
		}
	}
	for k := range was {
		if _, ok := now[k]; !ok {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func fingerprintJSON(c *config.Config) ([]byte, error) {
	b, err := json.MarshalIndent(fingerprint(c), "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode the configuration fingerprint: %w", err)
	}
	return append(b, '\n'), nil
}
