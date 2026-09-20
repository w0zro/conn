package main

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
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
		{"--theme takes its name with it", []string{"--theme", "datum", "down"}, []string{"down"}, nil},
		{"--theme beside a ground", []string{"--light", "--theme", "datum"}, nil, boolPtr(false)},
	} {
		t.Run(c.name, func(t *testing.T) {
			rest, o, err := parseModeFlags(c.args)
			got := o.dark
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

	// --theme names a theme conn has, and answers it.
	for _, c := range []struct {
		name  string
		args  []string
		theme string
	}{
		{"alone", []string{"--theme", "datum"}, "datum"},
		{"conn by name", []string{"--theme", "conn"}, "conn"},
		{"said twice, the same", []string{"--theme", "datum", "--theme", "datum"}, "datum"},
		{"not said", []string{"--dark"}, ""},
	} {
		if _, o, err := parseModeFlags(c.args); err != nil || o.theme != c.theme {
			t.Errorf("%s: parseModeFlags(%v) = theme %q, %v; want %q", c.name, c.args, o.theme, err, c.theme)
		}
	}
	for _, c := range []struct {
		name string
		args []string
		says string
	}{
		{"with no name", []string{"--theme"}, "conn and datum"},
		{"with a name conn does not have", []string{"--theme", "solarized"}, "conn has no theme solarized; it has conn and datum"},
		{"twice, with two names", []string{"--theme", "conn", "--theme", "datum"}, "contradiction"},
	} {
		if _, _, err := parseModeFlags(c.args); err == nil || !strings.Contains(err.Error(), c.says) {
			t.Errorf("--theme %s: %v; want it to say %q", c.name, err, c.says)
		}
	}
}

// askMode takes what the flags said over the terminal and the
// configuration, when they said anything; the theme is the
// configuration's, and conn's own where it names none.
func TestAskMode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	light, dark := false, true
	if m := askMode(override{dark: &light}, home); m.dark || m.theme != defaultTheme {
		t.Errorf("askMode(--light) = %+v", m)
	}
	if m := askMode(override{dark: &dark}, home); !m.dark || m.theme != defaultTheme {
		t.Errorf("askMode(--dark) = %+v", m)
	}
	// With nothing said and no terminal to ask (a test has none), askMode
	// falls back the same way detectDark does: dark.
	if m := askMode(override{}, home); !m.dark || m.theme != defaultTheme {
		t.Errorf("askMode(nothing) with no terminal = %+v", m)
	}
	// The configuration names the theme; a flag names it over the file;
	// a file naming a theme conn does not have names conn's own.
	home = writeConfig(t, `{"roots": ["~"], "theme": "datum"}`)
	if m := askMode(override{dark: &dark}, home); m.theme != "datum" {
		t.Errorf("askMode with datum configured = %+v", m)
	}
	if m := askMode(override{dark: &dark, theme: "conn"}, home); m.theme != "conn" {
		t.Errorf("askMode(--theme conn) with datum configured = %+v", m)
	}
	if m := serverMode(filepath.Join(t.TempDir(), "sock"), home); m.theme != "datum" || !m.dark {
		t.Errorf("serverMode with no file and datum configured = %+v", m)
	}
	home = writeConfig(t, `{"roots": ["~"], "theme": "solarized"}`)
	if m := askMode(override{dark: &dark}, home); m.theme != defaultTheme {
		t.Errorf("askMode with a theme conn does not have configured = %+v", m)
	}
	// A file naming a ground stands in front of the terminal: an
	// operator who wrote one wants that ground wherever they are. A flag
	// stands in front of the file, as it does for the theme.
	home = writeConfig(t, `{"roots": ["~"], "ground": "light"}`)
	if m := askMode(override{}, home); m.dark {
		t.Errorf("askMode with light configured = %+v", m)
	}
	if m := askMode(override{dark: &dark}, home); !m.dark {
		t.Errorf("askMode(--dark) with light configured = %+v", m)
	}
	if m := serverMode(filepath.Join(t.TempDir(), "sock"), home); m.dark {
		t.Errorf("serverMode with no file and light configured = %+v", m)
	}
	// A word that is neither ground is no ground at all: the terminal is
	// asked, which with no terminal to ask is dark, and the console says
	// the file names one conn does not have.
	home = writeConfig(t, `{"roots": ["~"], "ground": "grey"}`)
	if m := askMode(override{}, home); !m.dark {
		t.Errorf("askMode with a ground conn does not have configured = %+v", m)
	}
}

// connOn is conn's own theme on one ground: what every server was
// before there was another theme to be in.
func connOn(dark bool) mode { return mode{theme: defaultTheme, dark: dark} }

// holdMode keeps the ground the test binary is on. applyMode sets the
// colors package-wide and conn calls it once at start, where a test
// binary runs every test in the one process: a test that puts conn on
// the other ground leaves it there for whatever runs next, and the
// tests that read the dark defaults run later in the file order. The
// hazard is worth naming because it is not always visible at the call
// — dressProgram applies a mode of its own on the way to writing a
// theme — so a test that touches the ground at all takes this.
func holdMode(t *testing.T) {
	t.Helper()
	was := current
	t.Cleanup(func() { applyMode(was) })
}

// applyMode puts every color on one ground or the other.
func TestApplyModeSwitchesTheGround(t *testing.T) {
	holdMode(t)

	applyMode(connOn(true))
	if hex(groundColor) != "#15130F" || hex(inkColor) != "#E6DFD0" || cursorHex != "#E85D2F" ||
		borderHex != "#2A2620" || grayHex != "#8B8272" || themeBase != "dark-ansi" || vimBackground != "dark" {
		t.Errorf("dark: ground=%s ink=%s cursor=%s border=%s gray=%s base=%s vim=%s",
			hex(groundColor), hex(inkColor), cursorHex, borderHex, grayHex, themeBase, vimBackground)
	}
	if scheme != connTheme.dark.scheme {
		t.Errorf("dark scheme is not connTheme.dark.scheme: %v", scheme)
	}

	applyMode(connOn(false))
	if hex(groundColor) != "#EFE9DB" || hex(inkColor) != "#1A1611" || cursorHex != "#BD3A1D" ||
		borderHex != "#D8D0BD" || grayHex != "#6F6656" || themeBase != "light-ansi" || vimBackground != "light" {
		t.Errorf("light: ground=%s ink=%s cursor=%s border=%s gray=%s base=%s vim=%s",
			hex(groundColor), hex(inkColor), cursorHex, borderHex, grayHex, themeBase, vimBackground)
	}
	if current.dark || current.theme != defaultTheme {
		t.Errorf("applyMode does not say what it put conn in: %+v", current)
	}
	if scheme != connTheme.light.scheme {
		t.Errorf("light scheme is not connTheme.light.scheme: %v", scheme)
	}
	// Every diff wash and band moves with the ground too, not just the
	// sixteen and the two grounds, and so does the faint conn dims with.
	if diffAddedBg != connTheme.light.diffAddedBg || diffRemovedBg != connTheme.light.diffRemovedBg ||
		diffAddedDim != connTheme.light.diffAddedDim || diffRemovedDim != connTheme.light.diffRemovedDim ||
		diffAddedWord != connTheme.light.diffAddedWord || diffRemovedWord != connTheme.light.diffRemovedWord ||
		messageHoverBg != connTheme.light.messageHoverBg || toolBg != connTheme.light.toolBg ||
		faintHex != connTheme.light.faint {
		t.Error("a wash, a band or the faint was left on the dark ground")
	}
}

// The light scheme is as sound as the dark one: sixteen slots, none
// alike, structure and type and what can be run apart from each other.
func TestTheLightSchemeIsSixteenToo(t *testing.T) {
	seen := map[string]int{}
	for i, c := range connTheme.light.scheme {
		if was, dup := seen[c]; dup {
			t.Errorf("slot %d is slot %d again: %s", i, was, c)
		}
		seen[c] = i
	}
	blue, magenta, cyan := connTheme.light.scheme[4], connTheme.light.scheme[5], connTheme.light.scheme[6]
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
		{hex(connTheme.light.ground), 0, connTheme.light.scheme, "light black"},
		{hex(connTheme.light.ground), 7, connTheme.light.scheme, "light white"},
		{hex(connTheme.light.ground), 15, connTheme.light.scheme, "light bright white"},
		{hex(connTheme.dark.ground), 7, connTheme.dark.scheme, "dark white"},
		{hex(connTheme.dark.ground), 15, connTheme.dark.scheme, "dark bright white"},
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
		{"dark", connTheme.dark.accent, hex(connTheme.dark.ground)},
		{"light", connTheme.light.accent, hex(connTheme.light.ground)},
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
		{"light bright black", connTheme.light.scheme[8], hex(connTheme.light.ground)},
		{"dark bright black", connTheme.dark.scheme[8], hex(connTheme.dark.ground)},
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

// A server's mode is conn's dark until one is written, is what was
// written once one is, and conn down clears it so the next one rises
// fresh. A file from before conn had themes names the ground alone and
// reads as conn's; a file naming a theme conn does not have reads as
// conn's too, on the ground it says.
func TestModeFileRoundTrip(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "state", "tmux.sock")
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // no configuration of the machine's own

	if m := serverMode(socket, home); m != connOn(true) {
		t.Errorf("a server with no mode file yet is %+v, not conn's dark", m)
	}
	if _, ok := readModeFile(socket); ok {
		t.Error("readModeFile found a file nobody wrote")
	}

	if err := writeMode(socket, connOn(false)); err != nil {
		t.Fatal(err)
	}
	if m := serverMode(socket, home); m != connOn(false) {
		t.Errorf("a server written conn's light reads back %+v", m)
	}
	if m, ok := readModeFile(socket); !ok || m != connOn(false) {
		t.Errorf("readModeFile = (%+v, %v), want (conn light, true)", m, ok)
	}

	if err := writeMode(socket, connOn(true)); err != nil {
		t.Fatal(err)
	}
	if m := serverMode(socket, home); m != connOn(true) {
		t.Errorf("a server written conn's dark reads back %+v", m)
	}

	for _, c := range []struct {
		name, file string
		want       mode
	}{
		{"a file from before themes, light", "light", connOn(false)},
		{"a file from before themes, dark", "dark\n", connOn(true)},
		{"a theme conn does not have", "light solarized\n", connOn(false)},
		{"an empty file", "", connOn(true)},
	} {
		if err := os.WriteFile(modePath(socket), []byte(c.file), 0o600); err != nil {
			t.Fatal(err)
		}
		if m := serverMode(socket, home); m != c.want {
			t.Errorf("%s reads as %+v, want %+v", c.name, m, c.want)
		}
	}

	if err := os.Remove(modePath(socket)); err != nil {
		t.Fatal(err)
	}
	if m := serverMode(socket, home); m != connOn(true) {
		t.Errorf("a cleared mode file falls back to %+v, not conn's dark", m)
	}
}

// holdMode puts the ground back where it found it, which is the point
// of taking it rather than calling applyMode(connOn(true)) by hand: a test that
// restores to dark is right only for as long as dark is what it was.
func TestHoldModePutsTheGroundBack(t *testing.T) {
	holdMode(t)
	for _, was := range []bool{true, false} {
		applyMode(connOn(was))
		t.Run("", func(t *testing.T) {
			holdMode(t)
			applyMode(connOn(!was))
		})
		if current != connOn(was) {
			t.Errorf("a test on %v ground left conn in %+v", was, current)
		}
	}
}

// dressProgram writes a theme for the ground the server on this machine
// is on, which means it applies a mode: a caller that had conn on the
// other ground does not have it any more. Nothing at the call says so,
// which is why the tests that make it take holdMode, and this is that
// reason written down where it can fail.
func TestDressingAProgramSetsTheGround(t *testing.T) {
	holdMode(t)
	applyMode(connOn(false))
	// A home with no server beside it has no mode file, and a ground
	// that was never asked for is dark.
	if _, ok := dressProgram([]string{"vim"}, t.TempDir(), nil); !ok {
		t.Fatal("the colorscheme was not written")
	}
	if !current.dark {
		t.Error("dressProgram left conn on the ground its caller chose")
	}
}
