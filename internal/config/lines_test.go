package config

import "testing"

func TestIndexLines(t *testing.T) {
	doc := `# a comment
mode = "batch"

[gate]
protocol = "plain"
section_pattern = "^(.+)$"

[prices.models."claude-haiku-5-5"]
input = 1
output = 5

[prices.models."claude-haiku-5-5".long_context]
threshold = 200000

[seat.draft]
model = "x"
`
	idx, err := indexLines([]byte(doc))
	if err != nil {
		t.Fatalf("indexLines: %v", err)
	}
	tests := []struct {
		key  []string
		want int
	}{
		{[]string{"mode"}, 2},
		{[]string{"gate"}, 4},
		{[]string{"gate", "protocol"}, 5},
		{[]string{"gate", "section_pattern"}, 6},
		{[]string{"prices", "models", "claude-haiku-5-5"}, 8},
		{[]string{"prices", "models", "claude-haiku-5-5", "input"}, 9},
		{[]string{"prices", "models", "claude-haiku-5-5", "output"}, 10},
		{[]string{"prices", "models", "claude-haiku-5-5", "long_context", "threshold"}, 13},
		{[]string{"seat", "draft", "model"}, 16},
		{[]string{"gate", "absent"}, 4},
		{[]string{"absent"}, 0},
	}
	for _, tt := range tests {
		if got := idx.line(tt.key...); got != tt.want {
			t.Errorf("line(%q) = %d, want %d", tt.key, got, tt.want)
		}
	}
}

func TestIndexLinesDottedKeyAndInlineTable(t *testing.T) {
	doc := "gate.protocol = \"plain\"\nseat = { draft = { model = \"x\" } }\n"
	idx, err := indexLines([]byte(doc))
	if err != nil {
		t.Fatalf("indexLines: %v", err)
	}
	if got := idx.line("gate", "protocol"); got != 1 {
		t.Errorf("dotted key line = %d, want 1", got)
	}
	if got := idx.line("seat", "draft", "model"); got != 2 {
		t.Errorf("inline table key line = %d, want 2 from its table", got)
	}
}
