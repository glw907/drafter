package blueprint

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPathRule(t *testing.T) {
	tests := []struct {
		page, blueprint string
	}{
		{"docs/extend/x.md", "blueprints/docs/extend/x.blueprint.yaml"},
		{"README.md", "blueprints/README.blueprint.yaml"},
		{"docs/a/b/c/deep.md", "blueprints/docs/a/b/c/deep.blueprint.yaml"},
	}
	for _, tt := range tests {
		t.Run(tt.page, func(t *testing.T) {
			if got := BlueprintPath("blueprints", tt.page); got != tt.blueprint {
				t.Errorf("BlueprintPath = %q, want %q", got, tt.blueprint)
			}
			if got, ok := PagePath("blueprints", tt.blueprint); !ok || got != tt.page {
				t.Errorf("PagePath = %q, %v, want %q", got, ok, tt.page)
			}
		})
	}
	if got := BlueprintPath("docs-bp", "docs/x.md"); got != "docs-bp/docs/x.blueprint.yaml" {
		t.Errorf("a custom root gave %q", got)
	}
	for _, bad := range []string{"blueprints/outline.yaml", "blueprints/docs/x.yaml", "other/docs/x.blueprint.yaml", "blueprints/.blueprint.yaml", "blueprints/docs/.blueprint.yaml"} {
		if got, ok := PagePath("blueprints", bad); ok {
			t.Errorf("PagePath(%q) = %q, want false", bad, got)
		}
	}
}

func TestLockRoundTrip(t *testing.T) {
	want := &Lock{Version: Version, Pages: map[string]LockEntry{
		"docs/z.md": {Page: "p3", Blueprint: "b3", Accepted: "b3", Sections: map[string]string{IntroKey: "sha256:aa", "two": "sha256:bb", "one": "sha256:cc"}},
		"docs/a.md": {Page: "p1", Blueprint: "b1"},
		"docs/m.md": {Accepted: "b2"},
	}}
	file := filepath.Join(t.TempDir(), "drafted.lock")
	if err := WriteLock(file, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadLock(file)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("read back %+v, want %+v", got, want)
	}
	raw, _ := os.ReadFile(file)
	text := string(raw)
	if a, m, z := strings.Index(text, "docs/a.md"), strings.Index(text, "docs/m.md"), strings.Index(text, "docs/z.md"); a >= m || m >= z {
		t.Errorf("entries are not in path order (a=%d m=%d z=%d):\n%s", a, m, z, text)
	}
	if one, two := strings.Index(text, "one:"), strings.Index(text, "two:"); one > two {
		t.Errorf("section keys are not sorted:\n%s", text)
	}
	if err := WriteLock(file, got); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(file)
	if string(again) != text {
		t.Errorf("a second write changed the bytes:\n%s\n%s", text, again)
	}
}

func TestReadLockErrors(t *testing.T) {
	dir := t.TempDir()
	if l, err := ReadLock(filepath.Join(dir, "absent.lock")); err != nil || l.Version != Version || len(l.Pages) != 0 {
		t.Errorf("an absent lock = %+v, %v; want an empty lock", l, err)
	}
	tests := []struct{ name, body, want string }{
		{"not YAML", "pages: [\n", "lock"},
		{"an unknown field", "version: 1\nextra: 1\n", "extra"},
		{"a wrong version", "version: 9\n", "unsupported version 9"},
		{"a wrong shape", "version: 1\npages: 3\n", "lock"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := filepath.Join(dir, "x.lock")
			if err := os.WriteFile(file, []byte(tt.body), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := ReadLock(file); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("ReadLock error = %v, want one containing %q", err, tt.want)
			}
		})
	}
}
