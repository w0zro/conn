package main

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/w0zro/conn/internal/theme"
	"github.com/w0zro/conn/internal/tmux"
)

// The server's dress, as conn chooses it on a ground, and the words of
// the station's line written in it.

// The configuration takes tmux's prefix away and binds the panel key
// in the root table, and nothing else; it carries the readout; a path
// with a quote in it survives quoting.
func TestTheConfigurationHolds(t *testing.T) {
	g := theme.Conn.Dark
	conf := serverConfOn("C-Space", g)
	for _, s := range []string{
		"set -g prefix None", "set -g prefix2 None",
		`bind -n C-Space set -gF @conn_from "#{pane_id}" \; select-pane -t conn:home.0 \; send-keys -t conn:home.0 M--`,
		"set -g status on", "set -g status-position bottom", "set -g status 2", "set -g mouse on", "unbind -n MouseDrag1Border",
		// The status line stands on the raised ground, which is what a chosen
		// row sits on: a surface of its own and not the last line of the pane
		// over it. Its text begins where the panel's does.
		`set -g status-style "bg=` + g.Border + `,fg=#8B8272"`,
		`set -g window-status-format ""`,
		`set -g window-style "bg=#15130F,fg=#E6DFD0"`, `set -g pane-colours[15] "#E6DFD0"`,
		`set -g cursor-colour "#E85D2F"`, `set -g mode-style "bg=#2A2620,fg=#E6DFD0"`,
		`set -g pane-border-style "fg=` + theme.Hex(g.Ground) + `,bg=` + theme.Hex(g.Ground) + `"`,
		"set-environment -g CONN 1", "set-environment -g CLAUDE_CODE_TMUX_TRUECOLOR 1",
		`set -ga update-environment " TERM_PROGRAM TERM_PROGRAM_VERSION"`, "set -g default-terminal tmux-256color", "set-environment -g COLORTERM truecolor",

		`set -g pane-border-style "fg=` + theme.Hex(g.Ground) + `,bg=` + theme.Hex(g.Ground) + `"`, `set -g pane-active-border-style "fg=` + theme.Hex(g.Ground) + `,bg=` + theme.Hex(g.Ground) + `"`,
	} {
		if !strings.Contains(conf, s) {
			t.Errorf("configuration lacks %q", s)
		}
	}
	if strings.Count(conf, "\nbind -n ") != 1 || strings.Contains(conf, "unbind -T") || strings.Contains(conf, "C-b") || strings.Contains(serverConfOn("C-a", g), "C-Space") {
		t.Errorf("the root table binds more than the panel key, or ignores the key given:\n%s", conf)
	}
	// Copy mode selects and copies as the bar says, the way vim does.
	for _, s := range []string{
		"bind -T copy-mode-vi v send-keys -X begin-selection",
		"bind -T copy-mode-vi C-v send-keys -X rectangle-toggle",
		"bind -T copy-mode-vi y send-keys -X copy-pipe-and-cancel",
	} {
		if !strings.Contains(conf, s) {
			t.Errorf("configuration lacks %q", s)
		}
	}
	if n := strings.Count(conf, "\nbind "); n != 4 {
		t.Errorf("configuration binds %d keys, not the panel key and copy mode's three", n)
	}
	// The key is bound whatever the key is, and in the root table, or it
	// would not reach through a process.
	if !strings.Contains(serverConfOn("C-a", g), "bind -n C-a set -gF @conn_from") {
		t.Errorf("the panel key is not bound in the root table:\n%s", serverConfOn("C-a", g))
	}
	t.Setenv("CONN_KEY", "")
	if tmux.PanelKey() != "C-Space" {
		t.Errorf("default key: %q", tmux.PanelKey())
	}
	t.Setenv("CONN_KEY", "C-a")
	if tmux.PanelKey() != "C-a" {
		t.Errorf("key from the environment: %q", tmux.PanelKey())
	}
	if got := tmux.ShellQuote("/Users/o'brien/conn"); got != `'/Users/o'\''brien/conn'` {
		t.Errorf("quoted: %s", got)
	}
}

// Before the client has the terminal, the terminal is asked to take the
// ink and a ground for its own: black, on a dark ground and on a light
// one alike, so the padding around the client is the terminal's own
// edge; when the client returns it gets its own colors back. The panes
// themselves are drawn on the ground.
func TestTheTerminalIsAskedForItsPadding(t *testing.T) {
	if got := oscColors(theme.Conn.Dark); got != "\x1b]10;#E6DFD0\x1b\\\x1b]11;#000000\x1b\\" {
		t.Errorf("colors asked for on dark: %q", got)
	}
	if got := oscColors(theme.Conn.Light); got != "\x1b]10;"+theme.Hex(theme.Conn.Light.Ink)+"\x1b\\\x1b]11;#000000\x1b\\" {
		t.Errorf("colors asked for on light: %q", got)
	}
	// The cursor is given back too: the server puts its own on the
	// terminal, and does not take it off.
	if oscOwnColors != "\x1b]110\x1b\\\x1b]111\x1b\\\x1b]112\x1b\\" {
		t.Errorf("colors given back: %q", oscOwnColors)
	}
	if want := "bg=" + theme.Hex(theme.Conn.Dark.Ground) + ",fg=" + theme.Hex(theme.Conn.Dark.Ink); !strings.Contains(serverConfOn("C-Space", theme.Conn.Dark), want) {
		t.Errorf("the panes are not drawn in %s", want)
	}
}

// The sixteen are sixteen: every slot set, and the slots a shell theme
// leans on — structure, what can be run, type — apart from each other,
// in both the normal colors and the bright.
func TestTheSixteenAreSixteen(t *testing.T) {
	scheme := theme.Conn.Dark.Scheme
	conf := serverConfOn(tmux.DefaultKey, theme.Conn.Dark)
	for i, c := range scheme {
		if want := fmt.Sprintf("set -g pane-colours[%d] %q", i, c); !strings.Contains(conf, want) {
			t.Errorf("configuration lacks %q", want)
		}
	}
	for _, slots := range [][3]int{{4, 5, 6}, {12, 13, 14}} {
		blue, magenta, cyan := scheme[slots[0]], scheme[slots[1]], scheme[slots[2]]
		if blue == cyan || blue == magenta || magenta == cyan {
			t.Errorf("slots %v collapse: %s %s %s", slots, blue, magenta, cyan)
		}
	}
	seen := map[string]int{}
	for i, c := range scheme {
		if was, dup := seen[c]; dup {
			t.Errorf("slot %d is slot %d again: %s", i, was, c)
		}
		seen[c] = i
	}
}

// The status line is dark at rest and lit by what cannot be seen from
// the panel: the two modes only tmux can know for nothing, and what
// conn says of its own keys, read out of an option and shown only while
// the keys are on the panel. The right of the line is empty, and
// nothing on it is re-read on a beat.
func TestOnlyTmuxDrawsTheStatusLine(t *testing.T) {
	g := theme.Conn.Dark
	conf := serverConfOn("C-Space", g)
	for _, gone := range []string{"@conn_in", "@conn_note", "@conn_rail", "@conn_slot", "@conn_lamps", "status-interval 1"} {
		if strings.Contains(conf, gone) {
			t.Errorf("the status line still asks conn for %q", gone)
		}
	}
	// The line is the band: a mode tmux knows itself first, then where
	// the keys are by conn's word while they are on the panel and the
	// station's word when they are not, and at the right the clock.
	// The conf itself names no mode of conn's.
	for _, want := range []string{
		"#{?pane_in_mode,", "#{@conn_keys}", "#{@conn_station}",
		"set -g status-right \"#{@conn_up}\"",
		// The bar is conn's word, or copy mode's own keys while a pane
		// is in it, which tmux knows for nothing too.
		`set -g status-format[1] "#[fill=` + g.Surface + ` bg=` + g.Surface + `]#{?pane_in_mode,` + keyBar(copyHints, g) + `,#{@conn_bar}}#[align=right]#{@conn_ident}"`,
		"#{&&:#{==:#{window_name},home},#{==:#{pane_index},0}}",
		"status-interval 0", "set -g pane-border-status off",
		// Every mode a block of the orange, the ground knocked out of it.
		"#[bg=" + g.Accent + " fg=" + theme.Hex(g.Ground) + " bold] COPY ",
		// The cursor keeps the accent in copy mode: the option applied
		// again to the mode's screen a beat after the mode comes on.
		`set-hook -g pane-mode-changed "run-shell -b -d 0 -C \"set -p -t '#{hook_pane}' cursor-colour '` + g.Accent + `' ; set -pu -t '#{hook_pane}' cursor-colour\""`,
	} {
		if !strings.Contains(conf, want) {
			t.Errorf("the status line lacks %q:\n%s", want, conf)
		}
	}
	left := conf[strings.Index(conf, "set -g status-left "):]
	left = left[:strings.Index(left, "\n")]
	for _, gone := range []string{"CONN", "PROCS", "PROJECTS", "SESSIONS", "CONSOLE"} {
		if strings.Contains(left, gone) {
			t.Errorf("the band says %q at rest", gone)
		}
	}
}

// Under datum light, slot 7 is bg2 and slot 15 is bg1 - the whites, as
// ANSI means them, and no ink at all on a light ground. Everything conn
// draws as text reads there all the same, because it reads its inks by
// name: the console's own palette, what conn says on the status line,
// what Claude Code writes in, and the text of the vim colorscheme.
func TestConnReadsUnderDatumLight(t *testing.T) {
	g := theme.Mode{Theme: "datum", Dark: false}.Wear()
	const readable = 4.5
	bg := theme.Hex(g.Ground)
	if c := contrast(g.Scheme[7], bg); c >= readable {
		t.Fatalf("datum light's slot 7 (%s) reads as text at %.2f:1; this test has nothing to show", g.Scheme[7], c)
	}

	if c := contrast(g.Parchment, bg); c < readable {
		t.Errorf("the parchment (%s) is %.2f:1 on datum light", g.Parchment, c)
	}
	if say := statusSay("HELLO", g); !strings.Contains(say, "fg="+g.Parchment) {
		t.Errorf("the status line says its word in %q, not the parchment", say)
	}
	at := map[string]string{}
	for _, grp := range theme.ClaudeTheme(g) {
		for _, tk := range grp {
			at[tk.Name] = tk.Color
		}
	}
	for _, k := range []string{"text", "bashBorder", "claude", "permission", "inactive"} {
		if c := contrast(at[k], bg); c < readable {
			t.Errorf("Claude Code's %s (%s) is %.2f:1 on datum light", k, at[k], c)
		}
	}
	for _, line := range strings.Split(theme.VimColorscheme(g), "\n") {
		for _, group := range []string{"hi Normal ", "hi Delimiter ", "hi StatusLine ", "hi Pmenu ", "hi Title ", "hi @punctuation.delimiter "} {
			if !strings.HasPrefix(line, group) {
				continue
			}
			_, rest, _ := strings.Cut(line, "guifg=")
			fg, _, _ := strings.Cut(rest, " ")
			if c := contrast(fg, bg); c < readable {
				t.Errorf("%s draws text in %s, %.2f:1 on datum light", strings.TrimSpace(group), fg, c)
			}
		}
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

func TestTheStatusLineWordsAreTmuxFormats(t *testing.T) {
	g := theme.Conn.Dark
	ground := theme.Hex(g.Ground)
	// A block is lit in the accent with the ground knocked out of it,
	// and nothing where there is no word; a word stands on the band's
	// own ground, in a colour and a weight; what conn says in words is
	// on the surface in the parchment. A hash is tmux's own character
	// on the line and is doubled wherever conn's text carries one.
	for name, c := range map[string]struct{ got, want string }{
		"block":     {statusBlock("COPY", g), "#[bg=" + g.Accent + " fg=" + ground + " bold] COPY "},
		"no block":  {statusBlock("", g), ""},
		"word":      {statusWord(" CONN ", theme.Hex(g.Ink), true, g), "#[bg=" + g.Border + " fg=" + theme.Hex(g.Ink) + " bold] CONN "},
		"figure":    {statusWord("14:32 ", g.Gray, false, g), "#[bg=" + g.Border + " fg=" + g.Gray + " nobold]14:32 "},
		"hash":      {statusWord("#3", g.Gray, false, g), "#[bg=" + g.Border + " fg=" + g.Gray + " nobold]##3"},
		"said":      {statusSay("kill -TERM 123", g), "#[bg=" + g.Surface + " fg=" + g.Parchment + " nobold] kill -TERM 123"},
		"said hash": {statusSay("#1", g), "#[bg=" + g.Surface + " fg=" + g.Parchment + " nobold] ##1"},
	} {
		if c.got != c.want {
			t.Errorf("%s:\n%s\nwant\n%s", name, c.got, c.want)
		}
	}
}

func TestTheKeyBarIsAKeyAndAWordEach(t *testing.T) {
	g := theme.Conn.Dark
	ink := theme.Hex(g.Ink)
	got := keyBar([]keyHint{{"j k", "Move"}, {"enter", "Go In"}}, g)
	want := " #[bg=" + g.Surface + " fg=" + ink + " bold]j k #[bg=" + g.Surface + " fg=" + g.Gray + " nobold]move" +
		"   #[bg=" + g.Surface + " fg=" + ink + " bold]enter #[bg=" + g.Surface + " fg=" + g.Gray + " nobold]go in"
	if got != want {
		t.Errorf("the bar reads\n%s\nwant\n%s", got, want)
	}
	// The copy-mode bar is written inside a conditional of the format's
	// own, where a comma is the conditional's: no hint carries one, and
	// no style is written with one.
	bar := keyBar(copyHints, g)
	if strings.Contains(bar, ",") {
		t.Errorf("the copy-mode bar carries a comma: %s", bar)
	}
	for _, h := range copyHints {
		if h.key == "" || h.does == "" {
			t.Errorf("a copy hint is missing a half: %+v", h)
		}
	}
}

// serverConfOn is the server's configuration with a given panel key.
func serverConfOn(key string, g theme.Ground) string { return tmux.Conf(key, dressOf(g)) }
