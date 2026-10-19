package blueprint

import "testing"

func TestTrackForAndAnatomyOf(t *testing.T) {
	o := &Outline{
		Types: []Type{{Name: "task guide", Anatomy: "Task guide"}, {Name: "draft"}},
		Tracks: []Track{
			{Name: "admin", Paths: []string{"docs/admin/**"}},
			{Name: "dev", Paths: []string{"docs/dev/*.md", "docs/admin/both.md"}, Register: "house.md"},
		},
	}
	tracks := []struct {
		path, want string
		ok         bool
	}{
		{"docs/admin/deep/a.md", "admin", true},
		{"./docs/dev/a.md", "dev", true},
		{"docs/admin/both.md", "", false},
		{"docs/other.md", "", false},
	}
	for _, tt := range tracks {
		t.Run(tt.path, func(t *testing.T) {
			got, ok := o.TrackFor(tt.path)
			if ok != tt.ok || got.Name != tt.want {
				t.Errorf("TrackFor(%q) = %q, %v; want %q, %v", tt.path, got.Name, ok, tt.want, tt.ok)
			}
		})
	}
	for typ, want := range map[string]string{"task guide": "Task guide", "draft": "", "missing": ""} {
		if got := o.AnatomyOf(typ); got != want {
			t.Errorf("AnatomyOf(%q) = %q, want %q", typ, got, want)
		}
	}
}
