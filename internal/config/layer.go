package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Layers a setting may appear in, as a bit set.
const (
	inUser uint8 = 1 << iota
	inProject
	inLocal
	inAny = inUser | inProject | inLocal
)

// allowedIn is the spec's settings table: the file layers each top-level setting may appear in.
// The environment and the flags are not file layers and are never restricted.
var allowedIn = map[string]uint8{
	"prices":            inUser | inLocal,
	"billing_cycle_day": inUser | inLocal,
	"workspace_id":      inUser | inLocal,
	"key_env":           inUser | inLocal,
	"key_command":       inUser | inLocal,
	"api_base_url":      inUser | inLocal,
	"page_cap":          inUser | inLocal,
	"cycle_ceiling":     inUser | inLocal,
	"state_dir":         inUser,
	"retention_days":    inUser | inLocal,
	"inline_files":      inUser | inLocal,
	"mode":              inAny,
	"output":            inAny,
	"pages_in_flight":   inAny,
	"retries":           inAny,
	"gate":              inProject | inLocal,
	"prompts":           inProject | inLocal,
	"require_prompts":   inProject,
	"seats":             inProject | inLocal,
	"seat":              inProject | inLocal,
	"facts":             inProject | inLocal,
	"blueprints":        inProject | inLocal,
	"editing":           inProject,
	"register":          inProject | inLocal,
}

// layer is one source of settings: a file, the environment, or the flags.
type layer struct {
	label string // user, drafter.toml, drafter.local.toml, env, or flag
	bit   uint8  // the file layer's bit in allowedIn; 0 for env and flag
	path  string // the file, empty for env and flag
	dir   string // relative paths resolve from here
	home  string // what a leading ~ expands to; empty when ~ is not expanded
	f     *file
	idx   lineIndex
}

// readLayer reads and checks the file at path. It returns nil and no error when the file does
// not exist.
func readLayer(label string, bit uint8, path, home string) (*layer, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	f, err := decode(path, data)
	if err != nil {
		return nil, err
	}
	idx, err := indexLines(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	l := &layer{label: label, bit: bit, path: path, dir: filepath.Dir(path), f: f, idx: idx}
	if bit != inProject {
		l.home = home
	}
	if err := l.checkAllowed(); err != nil {
		return nil, err
	}
	return l, nil
}

// checkAllowed refuses the first key the settings table does not allow in this file layer.
func (l *layer) checkAllowed() error {
	for _, e := range l.idx {
		if allowedIn[e.key[0]]&l.bit != 0 {
			continue
		}
		return fmt.Errorf("%s:%d: %s is not allowed here; move it to %s",
			l.path, e.line, e.key[0], moveTargets(allowedIn[e.key[0]]))
	}
	return nil
}

func moveTargets(mask uint8) string {
	var to []string
	if mask&inUser != 0 {
		to = append(to, "the user config")
	}
	if mask&inProject != 0 {
		to = append(to, "drafter.toml")
	}
	if mask&inLocal != 0 {
		to = append(to, "drafter.local.toml")
	}
	return strings.Join(to, " or ")
}

// where names a key for an error: file and line for a file layer, the variable for the
// environment.
func (l *layer) where(key string) string {
	if l.path == "" {
		return envName(key)
	}
	return fmt.Sprintf("%s:%d", l.path, l.idx.line(strings.Split(key, ".")...))
}

// from names the layer as a setting's source. The environment and flag layers name the variable
// or flag that carried the key.
func (l *layer) from(key string) string {
	switch l.label {
	case "env":
		return "env " + envName(key)
	case "flag":
		if key == "output" {
			return "flag --json"
		}
		return "flag --sync"
	}
	return l.label
}

// resolve returns p as an absolute, clean path: ~ expands where the layer allows it, and a
// relative path resolves from the layer's directory.
func (l *layer) resolve(p string) string {
	if l.home != "" && (p == "~" || strings.HasPrefix(p, "~/")) {
		p = filepath.Join(l.home, p[1:])
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(l.dir, p)
	}
	return filepath.Clean(p)
}

// fail builds a refusal for key in the layer's own terms.
func (l *layer) fail(key, format string, args ...any) error {
	return fmt.Errorf("%s: %s %s", l.where(key), key, fmt.Sprintf(format, args...))
}
