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

func TestALineConnDrewReadsTheSameInTmuxsStyling(t *testing.T) {
	line := headingStyle.Render("claude") + " " + hintStyle.Render("in conn · pid 7 · #1") + "\x1b[0m"
	got := tmuxOf(line)
	for _, want := range []string{"#[bold", "fg=" + colorParchment, "claude", "#[default]", "fg=" + colorGray, "in conn · pid 7 · ##1"} {
		if !strings.Contains(strings.ToLower(got), strings.ToLower(want)) {
			t.Errorf("tmuxOf = %q, want it to hold %q", got, want)
		}
	}
	if strings.Contains(got, "\x1b") {
		t.Errorf("tmuxOf = %q, want no escape left in it", got)
	}
	if got := tmuxStyle("22"); got != "nobold,nodim" {
		t.Errorf("tmuxStyle(22) = %q", got)
	}
}
