// Package stock holds the files drafter ships inside its binary: the stock prompts module and the
// reference pages a seat or a person reads.
//
// Each file is both the page a person reads in the repository and the bytes a seat receives, so
// the two never drift.
package stock

import (
	_ "embed"
)

// Prompts is the stock prompts module, used when a project names none.
//
//go:embed prompts.js
var Prompts []byte

//go:embed anatomies.md
var anatomies []byte

//go:embed introductions.md
var introductions []byte

// Anatomies lists the stock anatomy names, one per "##" heading of anatomies.md.
var Anatomies = []string{
	"Concept page",
	"Task guide",
	"Tutorial",
	"Reference entry",
	"Troubleshooting page",
}

// Files returns the embedded file name holds, for the "drafter:" prompt token.
func Files(name string) ([]byte, bool) {
	switch name {
	case "prompts.js":
		return Prompts, true
	case "anatomies.md":
		return anatomies, true
	case "introductions.md":
		return introductions, true
	}
	return nil, false
}
