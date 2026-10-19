package blueprint

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/glw907/drafter/internal/atomicfile"
)

// DefaultRoot is the directory, relative to the repository root, that holds a project's
// blueprints.
const DefaultRoot = "blueprints"

const blueprintSuffix = ".blueprint.yaml"

// BlueprintPath returns the repo-relative slash path of the blueprint for the page at pagePath:
// root, then the page path with .md replaced by .blueprint.yaml.
func BlueprintPath(root, pagePath string) string {
	return path.Join(root, strings.TrimSuffix(pagePath, ".md")+blueprintSuffix)
}

// PagePath is BlueprintPath's inverse. It reports false when blueprintPath is not a
// .blueprint.yaml file under root.
func PagePath(root, blueprintPath string) (string, bool) {
	rel, ok := strings.CutPrefix(path.Clean(blueprintPath), path.Clean(root)+"/")
	if !ok {
		return "", false
	}
	page, ok := strings.CutSuffix(rel, blueprintSuffix)
	if !ok || page == "" || strings.HasSuffix(page, "/") {
		return "", false
	}
	return page + ".md", true
}

// ReadLock reads the lock at file. A missing file is an empty lock, since a project that has not
// drafted anything has none; a file that does not parse is an error.
func ReadLock(file string) (*Lock, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return &Lock{Version: Version}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read lock: %w", err)
	}
	return DecodeLock(file, data)
}

// DecodeLock decodes lock bytes; file names them in errors.
func DecodeLock(file string, data []byte) (*Lock, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	l := new(Lock)
	if err := dec.Decode(l); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("lock %s: %s", file, strings.TrimPrefix(err.Error(), "yaml: "))
	}
	if l.Version != Version {
		return nil, fmt.Errorf("lock %s: unsupported version %d", file, l.Version)
	}
	return l, nil
}

// WriteLock writes l to file through an atomic rename. Entries and section keys come out sorted,
// so a rewrite that changes nothing leaves the bytes alone and a change shows as a small diff.
func WriteLock(file string, l *Lock) error {
	body, err := yaml.Marshal(l)
	if err != nil {
		return fmt.Errorf("encode lock: %w", err)
	}
	data := append([]byte("# Written by drafter; do not edit by hand.\n"), body...)
	return atomicfile.WriteFile(file, data, 0o644)
}
