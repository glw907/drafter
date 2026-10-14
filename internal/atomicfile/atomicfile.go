// Package atomicfile writes a file so that a crash or a failed write leaves either the old
// contents or the new, never a partial file.
package atomicfile

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// WriteFile writes data to path through a temporary file in the same directory, syncs it, and
// renames it over path. The file's mode is perm exactly, whatever the umask. A failed write
// leaves any existing file at path as it was and removes the temporary file.
func WriteFile(path string, data []byte, perm fs.FileMode) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		return fmt.Errorf("write %s: %w", tmp.Name(), err)
	}
	if err = tmp.Chmod(perm); err != nil {
		return fmt.Errorf("set mode of %s: %w", tmp.Name(), err)
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("sync %s: %w", tmp.Name(), err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmp.Name(), err)
	}
	if err = os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}
