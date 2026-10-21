package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Scope names the settings a command needs.
type Scope int

const (
	// ScopeMoney requires workspace_id, page_cap, and cycle_ceiling. It serves commands that
	// resolve money settings outside a project.
	ScopeMoney Scope = iota + 1
	// ScopeRun adds gate.command, the require_prompts guard, and, for each running seat, its
	// model, effort, max_tokens, and a price entry for its model.
	ScopeRun
)

// RefusedStock is the refusal for a project that sets require_prompts and names no prompts module.
const RefusedStock = "prompts is not set, and require_prompts in drafter.toml refuses the stock module"

// Validate returns one error naming every problem in s, or nil. The consistency seat runs, and
// so is required, only when facts.dir is set.
func (c *Config) Validate(s Scope) error {
	var problems []error
	missing := func(key string) {
		problems = append(problems, fmt.Errorf("%s is not set", key))
	}
	if c.WorkspaceID == "" {
		missing("workspace_id")
	}
	if c.PageCap == 0 {
		missing("page_cap")
	}
	if c.CycleCeiling == 0 {
		missing("cycle_ceiling")
	}
	if s == ScopeRun {
		if c.Gate.Command == "" {
			missing("gate.command")
		}
		if c.Prompts == "" && c.RequirePrompts {
			problems = append(problems, errors.New(RefusedStock))
		}
		if p := c.Editing.Preferences; p != "" && c.Root != "" {
			if _, err := os.Stat(filepath.Join(c.Root, filepath.FromSlash(p))); err != nil {
				problems = append(problems, fmt.Errorf("editing.preferences names a file that cannot be read: %w", err))
			}
		}
		for _, name := range c.Seats {
			if name == "consistency" && c.Facts.Dir == "" {
				continue
			}
			problems = append(problems, c.validateSeat(name)...)
		}
	}
	return errors.Join(problems...)
}

func (c *Config) validateSeat(name string) []error {
	seat := c.Seat[name]
	key := "seat." + name + "."
	var problems []error
	if seat.Model == "" {
		problems = append(problems, fmt.Errorf("%smodel is not set", key))
	}
	if seat.Effort == "" {
		problems = append(problems, fmt.Errorf("%seffort is not set", key))
	}
	if seat.MaxTokens == 0 {
		problems = append(problems, fmt.Errorf("%smax_tokens is not set", key))
	}
	if _, ok := c.Prices.Models[seat.Model]; seat.Model != "" && !ok {
		problems = append(problems, fmt.Errorf("%smodel %q has no entry in prices.models", key, seat.Model))
	}
	return problems
}

// ValidateSeat reports why the named seat cannot run: a missing model, effort, or max_tokens, or
// a model with no price. It lets a command that runs a seat outside the seats list, such as
// drafter read with the blueprint seat, refuse to start on an incomplete one.
func (c *Config) ValidateSeat(name string) error {
	return errors.Join(c.validateSeat(name)...)
}
