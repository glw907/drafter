package blueprint

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const (
	seedPageText = "# Rotate the widget key\n\nIntro.\n\n## Before you begin\nYou need access.\n\n## Verify the key\nCheck it.\n"
	seedBP       = "blueprints/docs/guides/rotate-the-widget-key.blueprint.yaml"
)

// seedLockFile is the lock path under root.
func seedLockFile(root string) string {
	return filepath.Join(root, filepath.FromSlash(testLayout.LockPath))
}

// seedTwinB builds a repo holding the page and its blueprint, committed, with a lock entry that
// holds only accepted, which is what drafter read leaves behind.
func seedTwinB(t *testing.T) string {
	t.Helper()
	conv, _ := convertedFixture(t)
	root := seedRepo(t, seedPageText)
	dest := filepath.Join(root, filepath.FromSlash(seedBP))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, conv.Data, 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", "blueprint")
	setSeedEntry(t, root, LockEntry{Accepted: git(t, root, "rev-parse", "HEAD:"+seedBP)})
	return root
}

func setSeedEntry(t *testing.T, root string, e LockEntry) {
	t.Helper()
	l := &Lock{Version: Version, Pages: map[string]LockEntry{convertPage: e}}
	if err := WriteLock(seedLockFile(root), l); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", "lock")
}

func TestSeedMatchesConversion(t *testing.T) {
	conv, _ := convertedFixture(t)
	a := seedRepo(t, seedPageText)
	if _, err := InstallConverted(a, testLayout, convertPage, conv.Data); err != nil {
		t.Fatal(err)
	}
	want, err := ReadLock(seedLockFile(a))
	if err != nil {
		t.Fatal(err)
	}

	b := seedTwinB(t)
	accepted := git(t, b, "rev-parse", "HEAD:"+seedBP)
	pages, err := Seed(b, testLayout, []string{seedBP})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pages, []string{convertPage}) {
		t.Errorf("pages = %v", pages)
	}
	got, err := ReadLock(seedLockFile(b))
	if err != nil {
		t.Fatal(err)
	}
	g, w := got.Pages[convertPage], want.Pages[convertPage]
	if g.Page != w.Page || g.Blueprint != w.Blueprint || !reflect.DeepEqual(g.Sections, w.Sections) {
		t.Errorf("seeded entry %+v, conversion wrote %+v", g, w)
	}
	if g.Accepted != accepted || g.Page == "" {
		t.Errorf("seeded entry %+v should keep accepted %q and set page", g, accepted)
	}
}

func TestSeedRefusals(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, root string)
		args  []string
		want  string
	}{
		{"no entry", func(t *testing.T, root string) {
			if err := WriteLock(seedLockFile(root), &Lock{Version: Version}); err != nil {
				t.Fatal(err)
			}
			git(t, root, "commit", "-qam", "empty lock")
		}, []string{seedBP}, "run drafter read first"},
		{"accepted empty", func(t *testing.T, root string) {
			setSeedEntry(t, root, LockEntry{Blueprint: "x"})
		}, []string{seedBP}, "run drafter read first"},
		{"page already set", func(t *testing.T, root string) {
			setSeedEntry(t, root, LockEntry{Page: "p", Accepted: "a"})
		}, []string{seedBP}, "already records a drafted page"},
		{"blueprint changed after acceptance", func(t *testing.T, root string) {
			setSeedEntry(t, root, LockEntry{Accepted: "0000000000000000000000000000000000000000"})
		}, []string{seedBP}, "changed since drafter read accepted it"},
		{"uncommitted page", func(t *testing.T, root string) {
			if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(convertPage)), []byte("edited\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, []string{seedBP}, "has uncommitted changes"},
		{"uncommitted blueprint", func(t *testing.T, root string) {
			f := filepath.Join(root, filepath.FromSlash(seedBP))
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(f, append(data, "# edit\n"...), 0o644); err != nil {
				t.Fatal(err)
			}
		}, []string{seedBP}, "has uncommitted changes"},
		{"missing page", func(t *testing.T, root string) {
			if err := os.Remove(filepath.Join(root, filepath.FromSlash(convertPage))); err != nil {
				t.Fatal(err)
			}
		}, []string{seedBP}, "does not exist"},
		{"not a blueprint", func(t *testing.T, root string) {}, []string{"README.md"}, "is not a blueprint"},
		{"second blueprint refused", func(t *testing.T, root string) {}, []string{seedBP, "blueprints/docs/other.blueprint.yaml"}, "does not exist"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := seedTwinB(t)
			tt.setup(t, root)
			before, err := os.ReadFile(seedLockFile(root))
			if err != nil {
				t.Fatal(err)
			}
			_, err = Seed(root, testLayout, tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tt.want)
			}
			after, _ := os.ReadFile(seedLockFile(root))
			if string(after) != string(before) {
				t.Errorf("the lock changed on a refusal:\n%s", after)
			}
		})
	}
}
