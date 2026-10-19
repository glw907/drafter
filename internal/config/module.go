package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/glw907/drafter/stock"
)

// StockModuleName names the stock prompts module wherever a module's name is shown.
const StockModuleName = "drafter:prompts.js"

// ModuleName returns the name of the module a run uses: the path prompts names, or the stock
// module's name when prompts is unset. Unlike Module, it reads nothing.
func (c *Config) ModuleName() string {
	if c.Prompts == "" {
		return StockModuleName
	}
	return c.Prompts
}

// Module returns the prompts module a run uses: the file prompts names, or the stock module when
// prompts is unset. It does not apply require_prompts, which Validate does.
func (c *Config) Module() (name string, src []byte, err error) {
	if c.Prompts == "" {
		return StockModuleName, stock.Prompts, nil
	}
	src, err = os.ReadFile(c.Prompts)
	if err != nil {
		return "", nil, fmt.Errorf("read prompts module: %w", err)
	}
	return c.Prompts, src, nil
}

// ShortHash returns "sha256:" and the first 12 hex digits of src's SHA-256, the form the prompts
// row and a run report show.
func ShortHash(src []byte) string {
	sum := sha256.Sum256(src)
	return "sha256:" + hex.EncodeToString(sum[:])[:12]
}

// moduleRow is the prompts row: the module's source and short hash, or the path and "(missing)"
// when the file can't be read.
func (c *Config) moduleRow() Source {
	if c.Prompts == "" {
		return Source{Key: "prompts", Value: StockModuleName + " " + ShortHash(stock.Prompts), From: "default"}
	}
	r := c.row("prompts")
	src, err := os.ReadFile(c.Prompts)
	if err != nil {
		r.Value = c.Prompts + " (missing)"
		return r
	}
	r.Value = c.Prompts + " " + ShortHash(src)
	return r
}
