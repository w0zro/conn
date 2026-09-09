package main

import (
	"strings"
	"testing"
)

func TestAStatusChipWashesTheLineAfterIt(t *testing.T) {
	chip := statusChip(tp.amber, "PRE#FIX")
	for _, want := range []string{"fg=" + tp.amber, "bg=" + tp.chip, ",bold] PRE##FIX ", "fill=" + tp.wash} {
		if !strings.Contains(chip, want) {
			t.Errorf("chip %q lacks %q", chip, want)
		}
	}
}

func TestTheBrandChipIsTheOneInvertedGround(t *testing.T) {
	chip := brandChip()
	if !strings.Contains(chip, "bg="+tp.orange) || !strings.Contains(chip, "fg="+tp.ground) {
		t.Errorf("chip %q, want CONN in the ground's color on the orange", chip)
	}
}

func TestDotsJoinTheFactsThatAreThere(t *testing.T) {
	if got := dots("pid 4402", "", ":3000", "2h"); got != "pid 4402 · :3000 · 2h" {
		t.Errorf("dots = %q", got)
	}
}
