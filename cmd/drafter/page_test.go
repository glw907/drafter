package main

import (
	"testing"

	"github.com/glw907/drafter/internal/pipeline"
)

func TestDryExitCode(t *testing.T) {
	tests := []struct {
		name string
		s    pipeline.Summary
		want int
	}{
		{"pages still running", pipeline.Summary{Pages: []pipeline.PageResult{{Status: pipeline.StatusRunning}}}, 0},
		{"a page the plan check escalated", pipeline.Summary{Pages: []pipeline.PageResult{{Status: pipeline.StatusEscalated}}}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dryExitCode(tt.s); got != tt.want {
				t.Errorf("dry exit code = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestExitCodeFor(t *testing.T) {
	accepted := pipeline.PageResult{Status: pipeline.StatusAccepted}
	escalated := pipeline.PageResult{Status: pipeline.StatusEscalated}
	tests := []struct {
		name string
		s    pipeline.Summary
		want int
	}{
		{"every page accepted", pipeline.Summary{Pages: []pipeline.PageResult{accepted, accepted}}, 0},
		{"one page escalated", pipeline.Summary{Pages: []pipeline.PageResult{accepted, escalated}}, 1},
		{"stopped with every page pending", pipeline.Summary{Stop: pipeline.StopCycleCap, Pages: []pipeline.PageResult{{Status: pipeline.StatusPending}}}, 3},
		{"stopped with an escalated page outranks it", pipeline.Summary{Stop: pipeline.StopSpendLimit, Pages: []pipeline.PageResult{escalated, accepted}}, 3},
		{"a page left running with no stop", pipeline.Summary{Pages: []pipeline.PageResult{{Status: pipeline.StatusRunning}}}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := exitCodeFor(tt.s); got != tt.want {
				t.Errorf("exit code = %d, want %d", got, tt.want)
			}
		})
	}
}
