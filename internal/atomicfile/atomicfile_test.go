package atomicfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFile(t *testing.T) {
	tests := []struct {
		name string
		perm os.FileMode
		old  string
	}{
		{"new file", 0o644, ""},
		{"private file", 0o600, ""},
		{"replaces an existing file", 0o640, "old contents"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "out.txt")
			if tt.old != "" {
				if err := os.WriteFile(path, []byte(tt.old), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := WriteFile(path, []byte("new contents"), tt.perm); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != "new contents" {
				t.Errorf("contents %q, want %q", got, "new contents")
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != tt.perm {
				t.Errorf("mode %v, want %v", info.Mode().Perm(), tt.perm)
			}
			assertNoTemp(t, dir)
		})
	}
}

func TestWriteFileFailureLeavesTargetIntact(t *testing.T) {
	dir := t.TempDir()
	// A non-empty directory at the target makes the final rename fail after the temp file is
	// fully written.
	path := filepath.Join(dir, "target")
	if err := os.MkdirAll(filepath.Join(path, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := WriteFile(path, []byte("data"), 0o644)
	if err == nil {
		t.Fatal("WriteFile replaced a non-empty directory")
	}
	if !strings.Contains(err.Error(), "replace "+path) {
		t.Errorf("error %q does not name the failed replace of %s", err, path)
	}
	if _, err := os.Stat(filepath.Join(path, "child")); err != nil {
		t.Errorf("existing target damaged: %v", err)
	}
	assertNoTemp(t, dir)
}

func TestWriteFileMissingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent", "out.txt")
	if err := WriteFile(path, []byte("x"), 0o644); err == nil {
		t.Fatal("WriteFile succeeded in a directory that does not exist")
	}
}

func assertNoTemp(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}
