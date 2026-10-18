package config

import (
	"strings"
	"testing"
)

func TestValidateSeat(t *testing.T) {
	c := &Config{
		Prices: Prices{Models: map[string]Price{"m": {Input: 1, Output: 2}}},
		Seat:   map[string]Seat{"blueprint": {Model: "m", Effort: "medium", MaxTokens: 100}, "partial": {Model: "m"}},
	}
	if err := c.ValidateSeat("blueprint"); err != nil {
		t.Errorf("a complete seat: %v", err)
	}
	err := c.ValidateSeat("partial")
	if err == nil || !strings.Contains(err.Error(), "seat.partial.effort is not set") || !strings.Contains(err.Error(), "seat.partial.max_tokens is not set") {
		t.Errorf("an incomplete seat: %v", err)
	}
	if err := c.ValidateSeat("absent"); err == nil {
		t.Error("a seat with no table validated")
	}
}

func TestDefaultSeatsLeaveOutTheBlueprintSeat(t *testing.T) {
	for _, s := range defaultSeats {
		if s == "blueprint" {
			t.Fatal("the default seats include blueprint, which would change drafter page")
		}
	}
	if len(defaultSeats) != 7 {
		t.Errorf("default seats = %v, want the original seven", defaultSeats)
	}
}
