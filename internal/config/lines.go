package config

import (
	"slices"

	"github.com/pelletier/go-toml/v2/unstable"
)

// lineIndex maps each key a TOML document sets, table headers included, to its 1-indexed line.
// go-toml's strict mode reports a line for an unknown key only; a key that decodes but a layer
// refuses needs this index for its line.
type lineIndex []keyLine

type keyLine struct {
	key  []string // the full dotted key, with quoted parts unquoted
	line int
}

// indexLines walks data with go-toml's unstable parser, tracking the current table header.
func indexLines(data []byte) (lineIndex, error) {
	var p unstable.Parser
	p.Reset(data)
	var idx lineIndex
	var table []string
	for p.NextExpression() {
		expr := p.Expression()
		var key []string
		line := 0
		for it := expr.Key(); it.Next(); {
			part := it.Node()
			if line == 0 {
				line = p.Shape(part.Raw).Start.Line
			}
			key = append(key, string(part.Data))
		}
		switch expr.Kind {
		case unstable.Table, unstable.ArrayTable:
			table = key
		case unstable.KeyValue:
			key = append(slices.Clone(table), key...)
		}
		idx = append(idx, keyLine{key: key, line: line})
	}
	if err := p.Error(); err != nil {
		return nil, err
	}
	return idx, nil
}

// line returns the line of the key made of parts. A key set through an inline table has no entry
// of its own, so the lookup falls back to the longest prefix that does. It returns 0 when
// nothing matches.
func (x lineIndex) line(parts ...string) int {
	for n := len(parts); n > 0; n-- {
		for _, e := range x {
			if slices.Equal(e.key, parts[:n]) {
				return e.line
			}
		}
	}
	return 0
}
