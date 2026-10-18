package config

import (
	_ "embed"
	"fmt"
)

//go:embed stock.toml
var stockTOML []byte

// stockLayer decodes the embedded defaults as the lowest layer. It is not a file layer, so the
// per-layer key restrictions do not apply and it may hold keys from every layer's set.
func stockLayer() (*layer, error) {
	f, err := decode("stock.toml", stockTOML)
	if err != nil {
		return nil, fmt.Errorf("built-in defaults: %w", err)
	}
	return &layer{label: "default", f: f}, nil
}
