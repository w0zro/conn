package tmux

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/w0zro/conn/internal/theme"
)

// The server's socket is under the state directory unless CONN_SOCKET
// says otherwise; a pane knows it is conn's by the socket in TMUX.
func TestTheServerIsFoundBySocket(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("CONN_SOCKET", "")
	if got := SocketPath("/Users/w0zro"); got != "/Users/w0zro/.local/state/conn/tmux.sock" {
		t.Errorf("socket: %q", got)
	}
	t.Setenv("XDG_STATE_HOME", "/tmp/state")
	if got := SocketPath("/Users/w0zro"); got != "/tmp/state/conn/tmux.sock" {
		t.Errorf("socket under XDG_STATE_HOME: %q", got)
	}
	t.Setenv("CONN_SOCKET", "/tmp/cs/sock")
	if got := SocketPath("/Users/w0zro"); got != "/tmp/cs/sock" {
		t.Errorf("socket by CONN_SOCKET: %q", got)
	}
	for env, in := range map[string]bool{
		"/tmp/cs/sock,4242,0":     true,
		"/tmp/cs//sock,4242,0":    true,
		"/tmp/tmux-501/default,1": false,
		"":                        false,
	} {
		if got := InsideConn(env, "/tmp/cs/sock"); got != in {
			t.Errorf("InsideConn(%q) = %v", env, got)
		}
	}
}

// list-panes, as tmux prints it for the format asked.
func TestPanesAreParsed(t *testing.T) {
	// The last three fields are tmux's word for where the keys are in
	// the window — the panel has them here, and nothing else does —
	// where each pane stands in it, and conn's own mark for the
	// settings.
	out := "%0 /dev/ttys004 48 40         1 0 \n" +
		"%1 /dev/ttys007 138 40 1        0 1 \n" +
		"%5 /dev/ttys008 138 40  1       0 2 \n" +
		// A readout carries the hold's own mark as well as its own: it is
		// furniture like a hold, and everything that acts on holds acts on
		// it. Only the panel has to tell the two apart. The manual and the
		// settings are furniture the same way; this pane wears every mark
		// at once, so the parse is read for all of them together.
		"%7 /dev/ttys009 138 40 1  1   1   0 3 1\n" +
		// A pane conn opened for a container says which: it is the
		// terminal that container has not got, and its row is reached
		// through it.
		"%9 /dev/ttys010 138 40    9f1c2d3e4a5b     0 4 \n" +
		// A pane holding a shell inside a container says which container,
		// on the other mark: it is work of the operator's own, not the
		// service being read.
		"%11 /dev/ttys011 138 40     9f1c2d3e4a5b    0 5 \n" +
		// A pane conn opened for a declared process says which, and
		// how the process ended once it has.
		"%13 /dev/ttys012 138 40       web@%2FUsers%2Fw0zro%2Fapp 1 0 6 \n\n"
	want := map[string]Pane{
		"ttys004": {ID: "%0", TTY: "ttys004", Width: 48, Height: 40, Active: true, index: 0},
		"ttys007": {ID: "%1", TTY: "ttys007", Width: 138, Height: 40, Hold: true, index: 1},
		"ttys008": {ID: "%5", TTY: "ttys008", Width: 138, Height: 40, Dead: true, index: 2},
		"ttys009": {ID: "%7", TTY: "ttys009", Width: 138, Height: 40, Hold: true, Readout: true, Help: true, Settings: true, index: 3},
		"ttys010": {ID: "%9", TTY: "ttys010", Width: 138, Height: 40, Container: "9f1c2d3e4a5b", index: 4},
		"ttys011": {ID: "%11", TTY: "ttys011", Width: 138, Height: 40, ShellIn: "9f1c2d3e4a5b", index: 5},
		"ttys012": {ID: "%13", TTY: "ttys012", Width: 138, Height: 40, Declared: "web@%2FUsers%2Fw0zro%2Fapp", Exit: "1", index: 6},
	}
	if got := parsePanes(out); !reflect.DeepEqual(got, want) {
		t.Errorf("panes: %v", got)
	}
	if got := parsePanes(""); len(got) != 0 {
		t.Errorf("no panes parsed as %v", got)
	}
}

// The configuration takes tmux's prefix away and binds the panel key
// in the root table, and nothing else; it carries the readout; a path
// with a quote in it survives quoting.
func TestTheConfigurationHolds(t *testing.T) {
	g := theme.Conn.Dark
	conf := Conf("C-Space", g)
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
	if strings.Count(conf, "\nbind -n ") != 1 || strings.Contains(conf, "unbind -T") || strings.Contains(conf, "C-b") || strings.Contains(Conf("C-a", g), "C-Space") {
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
	if !strings.Contains(Conf("C-a", g), "bind -n C-a set -gF @conn_from") {
		t.Errorf("the panel key is not bound in the root table:\n%s", Conf("C-a", g))
	}
	t.Setenv("CONN_KEY", "")
	if PanelKey() != "C-Space" {
		t.Errorf("default key: %q", PanelKey())
	}
	t.Setenv("CONN_KEY", "C-a")
	if PanelKey() != "C-a" {
		t.Errorf("key from the environment: %q", PanelKey())
	}
	if got := ShellQuote("/Users/o'brien/conn"); got != `'/Users/o'\''brien/conn'` {
		t.Errorf("quoted: %s", got)
	}
}

// The client's environment carries no tmux of its own.
func TestTheClientEnvironmentDropsTmux(t *testing.T) {
	got := WithoutTmux([]string{"HOME=/h", "TMUX=/tmp/x,1,0", "TERM=xterm", "TMUX_PANE=%3"})
	if !reflect.DeepEqual(got, []string{"HOME=/h", "TERM=xterm"}) {
		t.Errorf("environment: %q", got)
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
	if want := "bg=" + theme.Hex(theme.Conn.Dark.Ground) + ",fg=" + theme.Hex(theme.Conn.Dark.Ink); !strings.Contains(Conf("C-Space", theme.Conn.Dark), want) {
		t.Errorf("the panes are not drawn in %s", want)
	}
}

// The sixteen are sixteen: every slot set, and the slots a shell theme
// leans on — structure, what can be run, type — apart from each other,
// in both the normal colors and the bright.
func TestTheSixteenAreSixteen(t *testing.T) {
	scheme := theme.Conn.Dark.Scheme
	conf := Conf(DefaultKey, theme.Conn.Dark)
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
	conf := Conf("C-Space", g)
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
		`set -g status-format[1] "#[fill=` + g.Surface + ` bg=` + g.Surface + `]#{?pane_in_mode,` + KeyBar(CopyHints, g) + `,#{@conn_bar}}#[align=right]#{@conn_ident}"`,
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

// Every format conn hands tmux is printable ASCII. tmux sanitizes what
// it prints to something that is not a terminal, and what counts as
// printable is the locale's: in the C locale it turns a tab in the
// answer into an underscore, and conn read no panes at all. A space
// tells the fields apart in every locale there is.
func TestNoFormatAsksTmuxForAControlCharacter(t *testing.T) {
	conf := Conf("C-Space", theme.Conn.Dark)
	for _, f := range []string{paneFormat, openFormat, windowFormat, statusLine(theme.Conn.Dark), conf} {
		for i, r := range f {
			if r == '\n' || r == '\t' && f == conf {
				continue // the configuration is a file of lines, not a format
			}
			if r < 0x20 || r == 0x7f {
				t.Errorf("a format asks tmux for %q at %d: %q", r, i, f)
			}
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
	if say := StatusLineSay("HELLO", g); !strings.Contains(say, "fg="+g.Parchment) {
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
