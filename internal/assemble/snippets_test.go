package assemble

import (
	"context"
	"strings"
	"testing"
)

func TestSnippetBlocks(t *testing.T) {
	f := newFixture(t)
	tests := []struct {
		name    string
		sources []string
		want    []string
		absent  []string
		blocks  int
	}{
		{"a page-relative tracked file is inlined", []string{"../src/greet.go"},
			[]string{"=== file: src/greet.go ===\n// Package greet is a synthetic fixture."}, nil, 1},
		{"a #Symbol source inlines the whole file", []string{"../src/greet.go#Greet"},
			[]string{"=== file: src/greet.go ===\n// Package greet is a synthetic fixture."}, nil, 1},
		{"a root-relative source resolves from the root", []string{"/src/greet.go"},
			[]string{"=== file: src/greet.go ==="}, nil, 1},
		{"a URL adds nothing", []string{"https://example.com/check.yml", "http://example.com/a"}, nil, nil, 0},
		{"a directory adds nothing", []string{"../src", "../src/"}, nil, nil, 0},
		{"an untracked file is named and never inlined", []string{"../untracked.txt"},
			[]string{"=== file: untracked.txt ===\nnot inlined: untracked\n"}, []string{"UNTRACKED-CONTENT"}, 1},
		{"an oversize file gets the size line", []string{"../big.txt"},
			[]string{"=== file: big.txt ===\nnot inlined: 307200 bytes\n"}, nil, 1},
		{"a tracked symlink out of the root is not inlined", []string{"escape.md"},
			[]string{"not inlined: untracked"}, []string{"OUTSIDE-SECRET"}, 1},
		{"a path above the root is not inlined", []string{"../../secret.txt"},
			[]string{"not inlined: outside the repository"}, []string{"OUTSIDE-SECRET"}, 1},
		{"a repeated source adds one block", []string{"../src/greet.go", "../src/greet.go#Hello"}, nil, nil, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks, err := SnippetBlocks(context.Background(), f.config(), "docs/page.md", tt.sources)
			if err != nil {
				t.Fatal(err)
			}
			if len(blocks) != tt.blocks {
				t.Errorf("blocks = %d, want %d:\n%s", len(blocks), tt.blocks, strings.Join(blocks, "\n"))
			}
			got := strings.Join(blocks, "\n")
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("blocks lack %q:\n%s", w, got)
				}
			}
			for _, a := range tt.absent {
				if strings.Contains(got, a) {
					t.Errorf("blocks hold %q:\n%s", a, got)
				}
			}
		})
	}
}
