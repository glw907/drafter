// Package scaffold writes the files that start a drafter project.
package scaffold

import (
	"crypto/rand"
	"embed"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"github.com/glw907/drafter/internal/atomicfile"
)

//go:embed templates
var templates embed.FS

// Result reports what Write did. Written lists the paths it wrote, slash-separated and relative
// to the root, in write order.
type Result struct {
	Written  []string
	ValeKept bool
}

type file struct {
	path     string // slash-separated, relative to the root
	template string
}

var files = []file{
	{"drafter.toml", "drafter.toml"},
	{"blueprints/outline.yaml", "outline.yaml"},
	{"blueprints/facts/README.md", "facts-README.md"},
	{"blueprints/facts/project.md", "project.md"},
	{".vale.ini", "vale.ini"},
	{".vale/styles/.gitignore", "vale-gitignore"},
}

// Write creates the starter files under root. It refuses, writing nothing, when drafter.toml or
// the blueprints directory exists. An existing .vale.ini is kept. A write that fails part way
// returns an error that names the files already written.
func Write(root string) (Result, error) {
	for _, name := range []string{"drafter.toml", "blueprints"} {
		_, err := os.Lstat(filepath.Join(root, name))
		if err == nil {
			return Result{}, fmt.Errorf("%s already exists in %s; init starts a new project only", name, root)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return Result{}, err
		}
	}
	id, err := mintID()
	if err != nil {
		return Result{}, err
	}
	for _, dir := range []string{"blueprints/facts", ".vale/styles"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o755); err != nil {
			return Result{}, err
		}
	}
	var res Result
	for _, f := range files {
		target := filepath.Join(root, filepath.FromSlash(f.path))
		if f.path == ".vale.ini" {
			if _, err := os.Lstat(target); err == nil {
				res.ValeKept = true
				continue
			}
		}
		data, err := templates.ReadFile("templates/" + f.template)
		if err != nil {
			return res, err
		}
		data = []byte(strings.ReplaceAll(string(data), "{{id}}", id))
		if err := atomicfile.WriteFile(target, data, 0o644); err != nil {
			return res, fmt.Errorf("write %s (already written: %s): %w", f.path, strings.Join(res.Written, ", "), err)
		}
		res.Written = append(res.Written, f.path)
	}
	return res, nil
}

const base36 = "0123456789abcdefghijklmnopqrstuvwxyz"

// mintID returns six base36 characters drawn from crypto/rand.
func mintID() (string, error) {
	var b [6]byte
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(base36))))
		if err != nil {
			return "", err
		}
		b[i] = base36[n.Int64()]
	}
	return string(b[:]), nil
}
