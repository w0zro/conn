package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

// The server's socket is under the state directory unless CONN_SOCKET
// says otherwise; a pane knows it is conn's by the socket in TMUX.
func TestTheServerIsFoundBySocket(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("CONN_SOCKET", "")
	if got := socketPath("/Users/w0zro"); got != "/Users/w0zro/.local/state/conn/tmux.sock" {
		t.Errorf("socket: %q", got)
	}
	t.Setenv("XDG_STATE_HOME", "/tmp/state")
	if got := socketPath("/Users/w0zro"); got != "/tmp/state/conn/tmux.sock" {
		t.Errorf("socket under XDG_STATE_HOME: %q", got)
	}
	t.Setenv("CONN_SOCKET", "/tmp/cs/sock")
	if got := socketPath("/Users/w0zro"); got != "/tmp/cs/sock" {
		t.Errorf("socket by CONN_SOCKET: %q", got)
	}
	for env, in := range map[string]bool{
		"/tmp/cs/sock,4242,0":     true,
		"/tmp/cs//sock,4242,0":    true,
		"/tmp/tmux-501/default,1": false,
		"":                        false,
	} {
		if got := insideConn(env, "/tmp/cs/sock"); got != in {
			t.Errorf("insideConn(%q) = %v", env, got)
		}
	}
}

// list-panes, as tmux prints it for the format asked.
func TestPanesAreParsed(t *testing.T) {
	out := "%0\t/dev/ttys004\t48\t40\t\t\t\n" +
		"%1\t/dev/ttys007\t138\t40\t1\t\t\n" +
		"%5\t/dev/ttys008\t138\t40\t\t1\t\n" +
		// A look carries the hold's own mark as well as its own: it is
		// furniture like a hold, and everything that acts on holds acts
		// on it. Only i has to tell the two apart.
		"%7\t/dev/ttys009\t138\t40\t1\t\t1\n\n"
	want := map[string]pane{
		"ttys004": {id: "%0", tty: "ttys004", width: 48, height: 40},
		"ttys007": {id: "%1", tty: "ttys007", width: 138, height: 40, hold: true},
		"ttys008": {id: "%5", tty: "ttys008", width: 138, height: 40, dead: true},
		"ttys009": {id: "%7", tty: "ttys009", width: 138, height: 40, hold: true, look: true},
	}
	if got := parsePanes(out); !reflect.DeepEqual(got, want) {
		t.Errorf("panes: %v", got)
	}
	if got := parsePanes(""); len(got) != 0 {
		t.Errorf("no panes parsed as %v", got)
	}
}

// The configuration sets the prefix, empties tmux's prefix table, and
// binds two chords under it — - to the watch, q to detach; it carries
// the look; a path with a quote in it survives quoting.
func TestTheConfigurationHolds(t *testing.T) {
	conf := tmuxConf("C-Space")
	for _, s := range []string{
		"set -g prefix C-Space", "set -g prefix2 None", "unbind -a -T prefix", "bind - select-pane -t conn:home.0",
		"bind p select-pane -t conn:home.0 \\; send-keys -t conn:home.0 M-p",
		"bind q detach-client",
		"set -g status on", "set -g status-position bottom", "set -g mouse on", "unbind -n MouseDrag1Border",
		// The bar stands on the raised ground, which is what a chosen row
		// sits on: a surface of its own and not the last line of the pane
		// over it. Its text begins where the rail's does.
		`set -g status-style "bg=` + borderHex + `,fg=#8B8272"`, `set -g status-left "   `,
		// A mode wears its color as a ground, parted by spaces rather
		// than commas so the conditional around it is not cut in two.
		"#[bg=" + cursorHex + " fg=" + hex(groundColor) + " bold] PREFIX ",
		"#[bg=" + scheme[12] + " fg=" + hex(groundColor) + " bold] COPY ",
		// The lamps are what only tmux can know; the row is conn's, and
		// tmux drops it while the keys are on the rail, where the watch
		// says all of it and more.
		"#{?client_prefix,", "#{?pane_in_mode,", "#{@conn_in}",
		"#{&&:#{==:#{window_name},home},#{==:#{pane_index},0}}",
		`set -g window-status-format ""`,
		`set -g window-style "bg=#15130F,fg=#E6DFD0"`, `set -g pane-colours[15] "#E6DFD0"`,
		`set -g cursor-colour "#E85D2F"`, `set -g mode-style "bg=#2A2620,fg=#E6DFD0"`,
		`set -g pane-border-style "fg=#2A2620,bg=#15130F"`,
		"set-environment -g CONN 1", "set-environment -g CLAUDE_CODE_TMUX_TRUECOLOR 1", "set -g default-terminal tmux-256color", "set-environment -g COLORTERM truecolor",
		"set -g remain-on-exit on",
		`set -g pane-border-style "fg=#2A2620,bg=#15130F"`, `set -g pane-active-border-style "fg=#2A2620,bg=#15130F"`,
	} {
		if !strings.Contains(conf, s) {
			t.Errorf("configuration lacks %q", s)
		}
	}
	if strings.Count(conf, "\nbind ") != 4 || strings.Contains(conf, "C-b") || strings.Contains(tmuxConf("C-a"), "C-Space") {
		t.Errorf("configuration binds more than the four chords, or ignores the prefix given:\n%s", conf)
	}
	// The prefix twice over is the other process, and the chord is the
	// prefix whatever the prefix is.
	if !strings.Contains(tmuxConf("C-a"), "bind C-a select-pane -t conn:home.0 \\; send-keys -t conn:home.0 M-o") {
		t.Errorf("the prefix is not bound under itself:\n%s", tmuxConf("C-a"))
	}
	t.Setenv("CONN_PREFIX", "")
	if prefix() != "C-Space" || prefixLabel(prefix()) != "C-SPACE" {
		t.Errorf("default prefix: %q %q", prefix(), prefixLabel(prefix()))
	}
	t.Setenv("CONN_PREFIX", "C-a")
	if prefix() != "C-a" {
		t.Errorf("prefix from the environment: %q", prefix())
	}
	if got := shellQuote("/Users/o'brien/conn"); got != `'/Users/o'\''brien/conn'` {
		t.Errorf("quoted: %s", got)
	}
}

// The client's environment carries no tmux of its own.
func TestTheClientEnvironmentDropsTmux(t *testing.T) {
	got := withoutTmux([]string{"HOME=/h", "TMUX=/tmp/x,1,0", "TERM=xterm", "TMUX_PANE=%3"})
	if !reflect.DeepEqual(got, []string{"HOME=/h", "TERM=xterm"}) {
		t.Errorf("environment: %q", got)
	}
}

// Before the client has the terminal, the terminal is asked to take the
// ground and the ink for its own, so its padding around the client is
// the ground; when the client returns it gets its own colors back. The
// colors are the ones the panes are drawn in.
func TestTheTerminalIsAskedForTheGround(t *testing.T) {
	if got := oscColors(); got != "\x1b]10;#E6DFD0\x1b\\\x1b]11;#15130F\x1b\\" {
		t.Errorf("colors asked for: %q", got)
	}
	// The cursor is given back too: the server puts its own on the
	// terminal, and does not take it off.
	if oscOwnColors != "\x1b]110\x1b\\\x1b]111\x1b\\\x1b]112\x1b\\" {
		t.Errorf("colors given back: %q", oscOwnColors)
	}
	if want := "bg=" + hex(groundColor) + ",fg=" + hex(inkColor); !strings.Contains(tmuxConf("C-Space"), want) {
		t.Errorf("the panes are not drawn in %s", want)
	}
}

// conn down says what it ended, a line for each window and one for the
// server, the columns aligned; a window's path is written from ~.
func TestDownSaysWhatItEnded(t *testing.T) {
	ws := parseWindows("home\t/Users/w0zro\nzsh\t/Users/w0zro/projects/w0zro/conn\nclaude\t/Users/w0zro/projects/w0zro/vim.pro\n")
	if len(ws) != 3 || ws[1] != (window{name: "zsh", path: "/Users/w0zro/projects/w0zro/conn"}) {
		t.Errorf("windows: %+v", ws)
	}
	got := downReport(ws, "/Users/w0zro/.local/state/conn/tmux.sock", "/Users/w0zro")
	want := "" +
		" ✔ Window home  ~                           ended\n" +
		" ✔ Window zsh  ~/projects/w0zro/conn        ended\n" +
		" ✔ Window claude  ~/projects/w0zro/vim.pro  ended\n" +
		" ✔ Server ~/.local/state/conn/tmux.sock     ended\n"
	if got != want {
		t.Errorf("report:\n%s\nwant:\n%s", got, want)
	}
	if got := downReport(nil, "/tmp/cs/sock", "/Users/w0zro"); got != " ✔ Server /tmp/cs/sock  ended\n" {
		t.Errorf("report with no windows: %q", got)
	}
}

// The sixteen are sixteen: every slot set, and the slots a shell theme
// leans on — structure, what can be run, type — apart from each other,
// in both the normal colors and the bright.
func TestTheSixteenAreSixteen(t *testing.T) {
	conf := tmuxConf(defaultPrefix)
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

// The bar is the row you are standing inside, and conn writes it only
// when it changes: setting an option on the server is a process, and the
// slot changes hands rarely. It is nothing at all when the slot holds
// conn's own furniture, since a hold in an empty slot is not somewhere
// you are working.
func TestTheBarIsTheRowYouAreIn(t *testing.T) {
	m := newModel(plain)
	m.view, m.inside, m.now = viewWatch, true, watchNow
	m.srv = &server{tmux: "/nonexistent/tmux", socket: "/tmp/none"}
	m.projRoots = testProjRoots
	m.head.session.home = "/Users/w0zro"
	m.panes = map[string]pane{
		"ttys004": {id: "%1", tty: "ttys004"},
		"ttys009": {id: "%9", tty: "ttys009", hold: true},
	}
	m.places = []place{{path: "/Users/w0zro/projects/w0zro/conn", entries: []entry{
		{pid: 11, kind: kindAgent, command: "claude --resume", tty: "ttys004",
			started: watchNow.Add(-47 * time.Minute), status: statusWaiting},
	}}}

	// Nothing in the slot: nothing to say, and nothing written.
	if _, cmd := m.saying(); cmd != nil {
		t.Error("conn wrote a bar with nothing in the slot")
	}

	// A hold standing in an empty slot is conn's own furniture.
	m.slot = "ttys009"
	if row := m.slotRow(); row != "" {
		t.Errorf("a hold in the slot is said to be somewhere you are: %q", row)
	}

	// The process: the place it works in, then the row as the watch has
	// it, with the age in its largest unit alone.
	m.slot = "ttys004"
	row := m.slotRow()
	for _, want := range []string{"w0zro/conn", kindAgent, "claude --resume", "47M", statusWaiting} {
		if !strings.Contains(row, want) {
			t.Errorf("the bar's row lacks %q:\n%s", want, row)
		}
	}
	next, cmd := m.saying()
	if cmd == nil || next.saidIn != row {
		t.Errorf("the row was not put on the bar: %q", next.saidIn)
	}
	// Said once. The same row again is not a second process.
	if _, cmd := next.saying(); cmd != nil {
		t.Error("the same row was written to the bar twice")
	}
	// A minute later the coarse age has not moved, so neither has the row.
	later := next
	later.now = watchNow.Add(30 * time.Second)
	if _, cmd := later.saying(); cmd != nil {
		t.Error("the bar was written again for a figure that had not changed")
	}

	// Outside the server there is no bar to write to.
	out := m
	out.inside = false
	if _, cmd := out.saying(); cmd != nil {
		t.Error("conn wrote a bar outside its server")
	}
}
