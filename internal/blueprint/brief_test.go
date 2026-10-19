package blueprint

import "testing"

func TestBriefPath(t *testing.T) {
	tests := []struct{ name, root, page, want string }{
		{"a top-level page", "blueprints", "docs/guide.md", "blueprints/docs/guide.brief.json"},
		{"a nested page", "blueprints", "docs/a/b/page.md", "blueprints/docs/a/b/page.brief.json"},
		{"a custom root", "plans/bp", "x.md", "plans/bp/x.brief.json"},
		{"a root with a trailing slash", "blueprints/", "docs/guide.md", "blueprints/docs/guide.brief.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BriefPath(tt.root, tt.page); got != tt.want {
				t.Errorf("BriefPath(%q, %q) = %q, want %q", tt.root, tt.page, got, tt.want)
			}
		})
	}
}
