package main

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// --light and --dark, read off the front of conn's own arguments, pick
// a ground before anything is asked; together they contradict, and
// anything else - a command's name, an argument of its own - ends the
// reading right there, whole.
func TestParseModeFlags(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }

	for _, c := range []struct {
		name string
		args []string
		rest []string
		want *bool // nil means no override
	}{
		{"nothing", nil, nil, nil},
		{"a command alone", []string{"down"}, []string{"down"}, nil},
		{"--dark alone", []string{"--dark"}, nil, boolPtr(true)},
		{"--light alone", []string{"--light"}, nil, boolPtr(false)},
		{"--dark before a command", []string{"--dark", "theme", "claude"}, []string{"theme", "claude"}, boolPtr(true)},
		{"--light before a command", []string{"--light", "down"}, []string{"down"}, boolPtr(false)},
		{"--dark said twice", []string{"--dark", "--dark"}, nil, boolPtr(true)},
		{"a command first leaves the flag its own", []string{"theme", "--light", "claude"}, []string{"theme", "--light", "claude"}, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			rest, got, err := parseModeFlags(c.args)
			if err != nil {
				t.Fatalf("parseModeFlags(%v): %v", c.args, err)
			}
			if len(rest) != len(c.rest) {
				t.Fatalf("parseModeFlags(%v) rest = %v, want %v", c.args, rest, c.rest)
			}
			for i := range rest {
				if rest[i] != c.rest[i] {
					t.Errorf("parseModeFlags(%v) rest = %v, want %v", c.args, rest, c.rest)
				}
			}
			if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
				t.Errorf("parseModeFlags(%v) override = %v, want %v", c.args, got, c.want)
			}
		})
	}

	if _, _, err := parseModeFlags([]string{"--dark", "--light"}); err == nil {
		t.Error("--dark and --light together was not a contradiction")
	}
	if _, _, err := parseModeFlags([]string{"--light", "--dark"}); err == nil {
		t.Error("--light and --dark together was not a contradiction")
	}
}

// askDark takes an override over the terminal, when there is one.
func TestAskDark(t *testing.T) {
	light, dark := false, true
	if askDark(&light) {
		t.Error("askDark(&light) is dark")
	}
	if !askDark(&dark) {
		t.Error("askDark(&dark) is light")
	}
	// With no override and no terminal to ask (a test has none), askDark
	// falls back the same way detectDark does: dark.
	if !askDark(nil) {
		t.Error("askDark(nil) with no terminal is not dark")
	}
}

// applyMode puts every color on one ground or the other, and nothing
// else touches these package vars mid-test, so a light call is always
// undone before another test reads the dark defaults.
func TestApplyModeSwitchesTheGround(t *testing.T) {
	t.Cleanup(func() { applyMode(true) })

	applyMode(true)
	if hex(groundColor) != "#15130F" || hex(inkColor) != "#E6DFD0" || cursorHex != "#E85D2F" ||
		borderHex != "#2A2620" || grayHex != "#8B8272" || themeBase != "dark-ansi" || vimBackground != "dark" {
		t.Errorf("dark: ground=%s ink=%s cursor=%s border=%s gray=%s base=%s vim=%s",
			hex(groundColor), hex(inkColor), cursorHex, borderHex, grayHex, themeBase, vimBackground)
	}
	if scheme != darkScheme {
		t.Errorf("dark scheme is not darkScheme: %v", scheme)
	}

	applyMode(false)
	if hex(groundColor) != "#EFE9DB" || hex(inkColor) != "#1A1611" || cursorHex != "#BD3A1D" ||
		borderHex != "#D8D0BD" || grayHex != "#6F6656" || themeBase != "light-ansi" || vimBackground != "light" {
		t.Errorf("light: ground=%s ink=%s cursor=%s border=%s gray=%s base=%s vim=%s",
			hex(groundColor), hex(inkColor), cursorHex, borderHex, grayHex, themeBase, vimBackground)
	}
	if scheme != lightScheme {
		t.Errorf("light scheme is not lightScheme: %v", scheme)
	}
	// Every diff wash and band moves with the ground too, not just the
	// sixteen and the two grounds, and so does the faint conn dims with.
	if diffAddedBg != lightDiffAddedBg || diffRemovedBg != lightDiffRemovedBg ||
		diffAddedDim != lightDiffAddedDim || diffRemovedDim != lightDiffRemovedDim ||
		diffAddedWord != lightDiffAddedWord || diffRemovedWord != lightDiffRemovedWord ||
		messageHoverBg != lightMessageHoverBg || toolBg != lightToolBg ||
		faintHex != lightFaintHex {
		t.Error("a wash, a band or the faint was left on the dark ground")
	}
}

// The light scheme is as sound as the dark one: sixteen slots, none
// alike, structure and type and what can be run apart from each other.
func TestTheLightSchemeIsSixteenToo(t *testing.T) {
	seen := map[string]int{}
	for i, c := range lightScheme {
		if was, dup := seen[c]; dup {
			t.Errorf("slot %d is slot %d again: %s", i, was, c)
		}
		seen[c] = i
	}
	blue, magenta, cyan := lightScheme[4], lightScheme[5], lightScheme[6]
	if blue == cyan || blue == magenta || magenta == cyan {
		t.Errorf("slots 4/5/6 collapse: %s %s %s", blue, magenta, cyan)
	}
}

// contrast is the WCAG ratio between two hexes, which is how every
// color on a ground here was chosen.
func contrast(a, b string) float64 {
	lum := func(h string) float64 {
		var r, g, bl int
		if _, err := fmt.Sscanf(h, "#%02x%02x%02x", &r, &g, &bl); err != nil {
			return 0
		}
		part := func(v int) float64 {
			c := float64(v) / 255
			if c <= 0.03928 {
				return c / 12.92
			}
			return math.Pow((c+0.055)/1.055, 2.4)
		}
		return 0.2126*part(r) + 0.7152*part(g) + 0.0722*part(bl)
	}
	hi, lo := lum(a), lum(b)
	if hi < lo {
		hi, lo = lo, hi
	}
	return (hi + 0.05) / (lo + 0.05)
}

// A slot a program writes ordinary text in has to be readable on the
// ground it is written against. Which slots those are is the
// convention, not conn's to pick: on paper black is text, and on a
// dark ground the whites are. conn's light scheme once had black at
// #D8D0BD - an edge, not an ink, and 1.27:1 against its own ground -
// which left the unchanged lines of a Claude Code diff all but blank.
func TestTheSlotsATextIsWrittenInAreReadable(t *testing.T) {
	const readable = 4.5 // WCAG AA for body text
	for _, c := range []struct {
		ground string
		slot   int
		scheme [16]string
		name   string
	}{
		{hex(lightGround), 0, lightScheme, "light black"},
		{hex(lightGround), 7, lightScheme, "light white"},
		{hex(lightGround), 15, lightScheme, "light bright white"},
		{hex(darkGround), 7, darkScheme, "dark white"},
		{hex(darkGround), 15, darkScheme, "dark bright white"},
	} {
		if r := contrast(c.scheme[c.slot], c.ground); r < readable {
			t.Errorf("%s (slot %d, %s) is %.2f:1 on %s; %.1f:1 is what reading it takes",
				c.name, c.slot, c.scheme[c.slot], r, c.ground, readable)
		}
	}
	// Every mode on the status line is a block of the orange with the
	// ground knocked out of it, so the word is only as readable as the
	// orange stands off the ground it is drawn against.
	for _, c := range []struct{ name, hex, ground string }{
		{"dark", darkCursorHex, hex(darkGround)},
		{"light", lightCursorHex, hex(lightGround)},
	} {
		if r := contrast(c.hex, c.ground); r < readable {
			t.Errorf("the %s block (%s) is %.2f:1 on %s; the word knocked out of it takes %.1f:1",
				c.name, c.hex, r, c.ground, readable)
		}
	}
	// The gray a program dims with is meant to be quieter than text,
	// and is held to no more than that, but it is still a color and
	// not the ground.
	for _, c := range []struct{ name, hex, ground string }{
		{"light bright black", lightScheme[8], hex(lightGround)},
		{"dark bright black", darkScheme[8], hex(darkGround)},
	} {
		if r := contrast(c.hex, c.ground); r < 2 {
			t.Errorf("%s (%s) is %.2f:1 on %s, which is the ground again", c.name, c.hex, r, c.ground)
		}
	}
}

// OSC 11's reply is read as dark or light by its luminance, however
// many hex digits a channel came in and whichever way it is
// terminated; a reply conn cannot read leaves ok false.
func TestParseBackground(t *testing.T) {
	for _, c := range []struct {
		name  string
		reply string
		dark  bool
		ok    bool
	}{
		{"dark, ST, 4 digits", "\x1b]11;rgb:1512/1312/0f0f\x1b\\", true, true},
		{"light, BEL, 4 digits", "\x1b]11;rgb:efef/e9e9/dbdb\a", false, true},
		{"dark, 2 digits", "\x1b]11;rgb:15/13/0f\x1b\\", true, true},
		{"light, 2 digits", "\x1b]11;rgb:ef/e9/db\x1b\\", false, true},
		{"white is light", "\x1b]11;rgb:ffff/ffff/ffff\x1b\\", false, true},
		{"black is dark", "\x1b]11;rgb:0000/0000/0000\x1b\\", true, true},
		{"garbage", "not an OSC reply", false, false},
		{"empty", "", false, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			dark, ok := parseBackground([]byte(c.reply))
			if ok != c.ok || (ok && dark != c.dark) {
				t.Errorf("parseBackground(%q) = (%v, %v), want (%v, %v)", c.reply, dark, ok, c.dark, c.ok)
			}
		})
	}
}

// readOSCReply stops at BEL, at ST, or at 64 bytes, whichever comes
// first, and takes nothing past what the terminal actually sent.
func TestReadOSCReply(t *testing.T) {
	for _, c := range []struct {
		name string
		sent string
		want string
	}{
		{"BEL", "\x1b]11;rgb:1512/1312/0f0f\a\x1b]after", "\x1b]11;rgb:1512/1312/0f0f\a"},
		{"ST", "\x1b]11;rgb:1512/1312/0f0f\x1b\\after", "\x1b]11;rgb:1512/1312/0f0f\x1b\\"},
		{"nothing sent", "", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := readOSCReply(bytes.NewReader([]byte(c.sent)))
			if string(got) != c.want {
				t.Errorf("readOSCReply(%q) = %q, want %q", c.sent, got, c.want)
			}
		})
	}
}

// A server's mode is dark until one is written, is what was written
// once one is, and conn down clears it so the next one rises fresh.
func TestModeFileRoundTrip(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "state", "tmux.sock")

	if !serverMode(socket) {
		t.Error("a server with no mode file yet is not dark")
	}
	if _, ok := readModeFile(socket); ok {
		t.Error("readModeFile found a file nobody wrote")
	}

	if err := writeMode(socket, false); err != nil {
		t.Fatal(err)
	}
	if serverMode(socket) {
		t.Error("a server written light reads back dark")
	}
	dark, ok := readModeFile(socket)
	if !ok || dark {
		t.Errorf("readModeFile = (%v, %v), want (false, true)", dark, ok)
	}

	if err := writeMode(socket, true); err != nil {
		t.Fatal(err)
	}
	if !serverMode(socket) {
		t.Error("a server written dark reads back light")
	}

	if err := os.Remove(modePath(socket)); err != nil {
		t.Fatal(err)
	}
	if !serverMode(socket) {
		t.Error("a cleared mode file does not fall back to dark")
	}
}
