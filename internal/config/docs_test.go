package config

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// settingsTableRows is the number of rows in the settings table of the pipeline design. The
// walked keys must reach it, so a walk that stops descending cannot pass.
const settingsTableRows = 23

// mapKeyNames are the placeholders the reference uses for the user-chosen key of each decoded
// map, by the dotted prefix that holds the map.
var mapKeyNames = map[string]string{
	"seat":          "<name>",
	"prices.models": "<model>",
	"register":      "<track>",
}

// walkKeys returns the dotted form of every setting the decode struct t holds, with a
// placeholder where a map's key goes.
func walkKeys(t reflect.Type, prefix string) []string {
	var keys []string
	for f := range t.Fields() {
		if f.Anonymous {
			keys = append(keys, walkKeys(f.Type, prefix)...)
			continue
		}
		tag, _, _ := strings.Cut(f.Tag.Get("toml"), ",")
		if tag == "" || tag == "-" {
			continue
		}
		key := prefix + tag
		ft := f.Type
		for ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		switch ft.Kind() {
		case reflect.Struct:
			keys = append(keys, walkKeys(ft, key+".")...)
		case reflect.Map:
			name, ok := mapKeyNames[key]
			if !ok {
				panic("docs_test: no placeholder for the map at " + key)
			}
			elem := ft.Elem()
			for elem.Kind() == reflect.Pointer {
				elem = elem.Elem()
			}
			if elem.Kind() == reflect.Struct {
				keys = append(keys, walkKeys(elem, key+"."+name+".")...)
			} else {
				keys = append(keys, key+"."+name)
			}
		default:
			keys = append(keys, key)
		}
	}
	return keys
}

// undocumented returns each key that doc does not name in code font.
func undocumented(doc string, keys []string) []string {
	var out []string
	for _, k := range keys {
		if !strings.Contains(doc, "`"+k+"`") {
			out = append(out, k)
		}
	}
	return out
}

func TestWalkKeys(t *testing.T) {
	keys := walkKeys(reflect.TypeFor[file](), "")
	for _, want := range []string{
		"retries.gate_fix", "gate.index_link_command", "seat.<name>.max_tokens",
		"prices.models.<model>.long_context.threshold", "prices.models.<model>.cache_read",
		"register.<track>", "facts.bullet_format", "inline_files",
	} {
		if !slices.Contains(keys, want) {
			t.Errorf("walked keys lack %q", want)
		}
	}
	if len(keys) < settingsTableRows {
		t.Errorf("walked %d keys, want at least the settings table's %d rows", len(keys), settingsTableRows)
	}
}

func TestUndocumented(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		keys []string
		want []string
	}{
		{"all present", "`mode` and `gate.lane`", []string{"mode", "gate.lane"}, nil},
		{"one missing", "`mode`", []string{"mode", "gate.lane"}, []string{"gate.lane"}},
		{"bare text does not count", "mode", []string{"mode"}, []string{"mode"}},
		{"a longer key does not stand in", "`gate.lane_x`", []string{"gate.lane"}, []string{"gate.lane"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := undocumented(tt.doc, tt.keys)
			if !slices.Equal(got, tt.want) {
				t.Errorf("undocumented = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigurationDocNamesEverySetting(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "configuration.md"))
	if err != nil {
		t.Fatal(err)
	}
	keys := walkKeys(reflect.TypeFor[file](), "")
	keys = append(keys, settingKeys...)
	if missing := undocumented(string(data), keys); len(missing) > 0 {
		t.Errorf("docs/configuration.md does not document %d settings: %s", len(missing), strings.Join(missing, ", "))
	}
}
