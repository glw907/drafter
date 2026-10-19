package blueprint

import "testing"

func TestBriefPath(t *testing.T) {
	tests := []struct{ name, root, template, page, want string }{
		{"a top-level page", "blueprints", "", "docs/guide.md", "blueprints/docs/guide.brief.json"},
		{"a nested page", "blueprints", "", "docs/a/b/page.md", "blueprints/docs/a/b/page.brief.json"},
		{"a custom root", "plans/bp", "", "x.md", "plans/bp/x.brief.json"},
		{"a root with a trailing slash", "blueprints/", "", "docs/guide.md", "blueprints/docs/guide.brief.json"},
		{"a template, one directory", "blueprints", "notes/briefs/{dir}/{name}.json", "docs/guide/start.md", "notes/briefs/guide/start.json"},
		{"a template, another directory", "blueprints", "notes/briefs/{dir}/{name}.json", "docs/ops/deploy.md", "notes/briefs/ops/deploy.json"},
		{"a template without {dir}", "blueprints", "briefs/{name}.brief.json", "docs/ops/deploy.md", "briefs/deploy.brief.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BriefPath(tt.root, tt.template, tt.page); got != tt.want {
				t.Errorf("BriefPath(%q, %q, %q) = %q, want %q", tt.root, tt.template, tt.page, got, tt.want)
			}
		})
	}
}
