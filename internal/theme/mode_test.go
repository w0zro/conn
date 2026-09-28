package theme

import (
	"fmt"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/w0zro/conn/internal/config"
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
			rest, o, err := ParseFlags(c.args)
			got := o.Dark
			if err != nil {
				t.Fatalf("ParseFlags(%v): %v", c.args, err)
			}
			if len(rest) != len(c.rest) {
				t.Fatalf("ParseFlags(%v) rest = %v, want %v", c.args, rest, c.rest)
			}
			for i := range rest {
				if rest[i] != c.rest[i] {
					t.Errorf("ParseFlags(%v) rest = %v, want %v", c.args, rest, c.rest)
				}
			}
			if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
				t.Errorf("ParseFlags(%v) override = %v, want %v", c.args, got, c.want)
			}
		})
	}

	if _, _, err := ParseFlags([]string{"--dark", "--light"}); err == nil {
		t.Error("--dark and --light together was not a contradiction")
	}
	if _, _, err := ParseFlags([]string{"--light", "--dark"}); err == nil {
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
		if _, o, err := ParseFlags(c.args); err != nil || o.Theme != c.theme {
			t.Errorf("%s: ParseFlags(%v) = theme %q, %v; want %q", c.name, c.args, o.Theme, err, c.theme)
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
		if _, _, err := ParseFlags(c.args); err == nil || !strings.Contains(err.Error(), c.says) {
			t.Errorf("--theme %s: %v; want it to say %q", c.name, err, c.says)
		}
	}
}

// AskMode takes what the flags said over the terminal and the
// configuration, when they said anything; the theme is the
// configuration's, and conn's own where it names none.
func TestAskMode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	light, dark := false, true
	if m := AskMode(Override{Dark: &light}, home); m.Dark || m.Theme != Default {
		t.Errorf("AskMode(--light) = %+v", m)
	}
	if m := AskMode(Override{Dark: &dark}, home); !m.Dark || m.Theme != Default {
		t.Errorf("AskMode(--dark) = %+v", m)
	}
	// With nothing said and no terminal to ask (a test has none), AskMode
	// falls back the same way detectDark does: dark.
	if m := AskMode(Override{}, home); !m.Dark || m.Theme != Default {
		t.Errorf("AskMode(nothing) with no terminal = %+v", m)
	}
	// The configuration names the theme; a flag names it over the file;
	// a file naming a theme conn does not have names conn's own.
	home = writeConfig(t, `{"roots": ["~"], "theme": "datum"}`)
	if m := AskMode(Override{Dark: &dark}, home); m.Theme != "datum" {
		t.Errorf("AskMode with datum configured = %+v", m)
	}
	if m := AskMode(Override{Dark: &dark, Theme: "conn"}, home); m.Theme != "conn" {
		t.Errorf("AskMode(--theme conn) with datum configured = %+v", m)
	}
	if m := ServerMode(filepath.Join(t.TempDir(), "sock"), home); m.Theme != "datum" || !m.Dark {
		t.Errorf("ServerMode with no file and datum configured = %+v", m)
	}
	home = writeConfig(t, `{"roots": ["~"], "theme": "solarized"}`)
	if m := AskMode(Override{Dark: &dark}, home); m.Theme != Default {
		t.Errorf("AskMode with a theme conn does not have configured = %+v", m)
	}
	// A file naming a ground stands in front of the terminal: an
	// operator who wrote one wants that ground wherever they are. A flag
	// stands in front of the file, as it does for the theme.
	home = writeConfig(t, `{"roots": ["~"], "ground": "light"}`)
	if m := AskMode(Override{}, home); m.Dark {
		t.Errorf("AskMode with light configured = %+v", m)
	}
	if m := AskMode(Override{Dark: &dark}, home); !m.Dark {
		t.Errorf("AskMode(--dark) with light configured = %+v", m)
	}
	if m := ServerMode(filepath.Join(t.TempDir(), "sock"), home); m.Dark {
		t.Errorf("ServerMode with no file and light configured = %+v", m)
	}
	// A word that is neither ground is no ground at all: the terminal is
	// asked, which with no terminal to ask is dark, and the console says
	// the file names one conn does not have.
	home = writeConfig(t, `{"roots": ["~"], "ground": "grey"}`)
	if m := AskMode(Override{}, home); !m.Dark {
		t.Errorf("AskMode with a ground conn does not have configured = %+v", m)
	}
}

// connOn is conn's own theme on one ground: what every server was
// before there was another theme to be in.
func connOn(dark bool) Mode { return Mode{Theme: Default, Dark: dark} }

// A mode wears a ground: its theme's, dark or light, and the ground
// says what follows from it for the programs conn dresses. A theme
// conn does not have is conn's own, on the ground asked for.
func TestAModeWearsAGround(t *testing.T) {
	for _, c := range []struct {
		m         Mode
		want      Ground
		base, vim string
	}{
		{connOn(true), Conn.Dark, "dark-ansi", "dark"},
		{connOn(false), Conn.Light, "light-ansi", "light"},
		{Mode{Theme: "datum", Dark: true}, Datum.Dark, "dark-ansi", "dark"},
		{Mode{Theme: "datum", Dark: false}, Datum.Light, "light-ansi", "light"},
		{Mode{Theme: "gone", Dark: false}, Conn.Light, "light-ansi", "light"},
	} {
		g := c.m.Wear()
		if g != c.want {
			t.Errorf("%+v wears %s, not the ground asked for", c.m, Hex(g.Ground))
		}
		if g.claudeBase() != c.base || g.vimBackground() != c.vim {
			t.Errorf("%+v tells Claude Code %q and nvim %q", c.m, g.claudeBase(), g.vimBackground())
		}
	}
	if Hex(connOn(true).Wear().Ground) != "#15130F" || connOn(true).Wear().Accent != "#E85D2F" ||
		Hex(connOn(false).Wear().Ground) != "#EFE9DB" || connOn(false).Wear().Accent != "#BD3A1D" {
		t.Error("conn's own grounds are not the ones the tables say")
	}
}

// The light scheme is as sound as the dark one: sixteen slots, none
// alike, structure and type and what can be run apart from each other.
func TestTheLightSchemeIsSixteenToo(t *testing.T) {
	seen := map[string]int{}
	for i, c := range Conn.Light.Scheme {
		if was, dup := seen[c]; dup {
			t.Errorf("slot %d is slot %d again: %s", i, was, c)
		}
		seen[c] = i
	}
	blue, magenta, cyan := Conn.Light.Scheme[4], Conn.Light.Scheme[5], Conn.Light.Scheme[6]
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
		{Hex(Conn.Light.Ground), 0, Conn.Light.Scheme, "light black"},
		{Hex(Conn.Light.Ground), 7, Conn.Light.Scheme, "light white"},
		{Hex(Conn.Light.Ground), 15, Conn.Light.Scheme, "light bright white"},
		{Hex(Conn.Dark.Ground), 7, Conn.Dark.Scheme, "dark white"},
		{Hex(Conn.Dark.Ground), 15, Conn.Dark.Scheme, "dark bright white"},
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
		{"dark", Conn.Dark.Accent, Hex(Conn.Dark.Ground)},
		{"light", Conn.Light.Accent, Hex(Conn.Light.Ground)},
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
		{"light bright black", Conn.Light.Scheme[8], Hex(Conn.Light.Ground)},
		{"dark bright black", Conn.Dark.Scheme[8], Hex(Conn.Dark.Ground)},
	} {
		if r := contrast(c.hex, c.ground); r < 2 {
			t.Errorf("%s (%s) is %.2f:1 on %s, which is the ground again", c.name, c.hex, r, c.ground)
		}
	}
}

// A ground is read as dark or light by its luminance: conn's own two
// grounds, as a terminal on them answers OSC 11, and the ends.
func TestIsDark(t *testing.T) {
	for _, c := range []struct {
		name    string
		r, g, b uint16
		dark    bool
	}{
		{"conn's dark", 0x1512, 0x1312, 0x0f0f, true},
		{"conn's light", 0xefef, 0xe9e9, 0xdbdb, false},
		{"white", 0xffff, 0xffff, 0xffff, false},
		{"black", 0, 0, 0, true},
		{"a saturated blue", 0x2020, 0x4040, 0xffff, true},
	} {
		if got := isDark(color.RGBA64{R: c.r, G: c.g, B: c.b, A: 0xffff}); got != c.dark {
			t.Errorf("IsDark(%s) = %v, want %v", c.name, got, c.dark)
		}
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

	if m := ServerMode(socket, home); m != connOn(true) {
		t.Errorf("a server with no mode file yet is %+v, not conn's dark", m)
	}
	if _, ok := ReadModeFile(socket); ok {
		t.Error("ReadModeFile found a file nobody wrote")
	}

	if err := WriteMode(socket, connOn(false)); err != nil {
		t.Fatal(err)
	}
	if m := ServerMode(socket, home); m != connOn(false) {
		t.Errorf("a server written conn's light reads back %+v", m)
	}
	if m, ok := ReadModeFile(socket); !ok || m != connOn(false) {
		t.Errorf("ReadModeFile = (%+v, %v), want (conn light, true)", m, ok)
	}

	if err := WriteMode(socket, connOn(true)); err != nil {
		t.Fatal(err)
	}
	if m := ServerMode(socket, home); m != connOn(true) {
		t.Errorf("a server written conn's dark reads back %+v", m)
	}

	for _, c := range []struct {
		name, file string
		want       Mode
	}{
		{"a file from before themes, light", "light", connOn(false)},
		{"a file from before themes, dark", "dark\n", connOn(true)},
		{"a theme conn does not have", "light solarized\n", connOn(false)},
		{"an empty file", "", connOn(true)},
	} {
		if err := os.WriteFile(ModePath(socket), []byte(c.file), 0o600); err != nil {
			t.Fatal(err)
		}
		if m := ServerMode(socket, home); m != c.want {
			t.Errorf("%s reads as %+v, want %+v", c.name, m, c.want)
		}
	}

	if err := os.Remove(ModePath(socket)); err != nil {
		t.Fatal(err)
	}
	if m := ServerMode(socket, home); m != connOn(true) {
		t.Errorf("a cleared mode file falls back to %+v, not conn's dark", m)
	}
}

// writeConfig puts a config file where conn will look for it, under a
// config home of the test's own, and answers the home it was written
// for.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	home := t.TempDir()
	path := config.Path(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}
