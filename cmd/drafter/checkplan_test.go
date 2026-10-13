package main

import (
	"bytes"
	"testing"

	"github.com/glw907/drafter/internal/plan"
)

func TestWriteJSON(t *testing.T) {
	tests := []struct {
		name     string
		findings []plan.Finding
		want     string
	}{
		{"no findings is an empty array", nil, "[]\n"},
		{"one finding", []plan.Finding{{File: "p.yaml", Line: 3, Severity: "error", Check: "ids", Message: "m", Fix: "f"}},
			"[\n  {\n    \"file\": \"p.yaml\",\n    \"line\": 3,\n    \"severity\": \"error\",\n    \"check\": \"ids\",\n    \"message\": \"m\",\n    \"fix\": \"f\"\n  }\n]\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b bytes.Buffer
			if err := writeJSON(&b, tt.findings); err != nil {
				t.Fatal(err)
			}
			if b.String() != tt.want {
				t.Errorf("got %q, want %q", b.String(), tt.want)
			}
		})
	}
}

func TestPlural(t *testing.T) {
	for n, want := range map[int]string{0: "0 errors", 1: "1 error", 2: "2 errors"} {
		if got := plural(n, "error"); got != want {
			t.Errorf("plural(%d) = %q, want %q", n, got, want)
		}
	}
}
