package main

import (
	"fmt"
	"reflect"
	"strings"
	"syscall"
	"testing"
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
	// The last field is tmux's word for where the keys are in the
	// window: the panel has them here, and nothing else does.
	out := "%0 /dev/ttys004 48 40       1\n" +
		"%1 /dev/ttys007 138 40 1      0\n" +
		"%5 /dev/ttys008 138 40  1     0\n" +
		// A readout carries the hold's own mark as well as its own: it is
		// furniture like a hold, and everything that acts on holds acts on
		// it. Only the panel has to tell the two apart. The manual is furniture the
		// same way; this pane wears every mark at once, so the parse is read
		// for all of them together.
		"%7 /dev/ttys009 138 40 1  1   1 0\n" +
		// A pane conn opened for a container says which: it is the
		// terminal that container has not got, and its row is reached
		// through it.
		"%9 /dev/ttys010 138 40    9f1c2d3e4a5b   0\n" +
		// A pane holding a shell inside a container says which container,
		// on the other mark: it is work of the operator's own, not the
		// service being read.
		"%11 /dev/ttys011 138 40     9f1c2d3e4a5b  0\n\n"
	want := map[string]pane{
		"ttys004": {id: "%0", tty: "ttys004", width: 48, height: 40, active: true},
		"ttys007": {id: "%1", tty: "ttys007", width: 138, height: 40, hold: true},
		"ttys008": {id: "%5", tty: "ttys008", width: 138, height: 40, dead: true},
		"ttys009": {id: "%7", tty: "ttys009", width: 138, height: 40, hold: true, readout: true, help: true},
		"ttys010": {id: "%9", tty: "ttys010", width: 138, height: 40, container: "9f1c2d3e4a5b"},
		"ttys011": {id: "%11", tty: "ttys011", width: 138, height: 40, shellIn: "9f1c2d3e4a5b"},
	}
	if got := parsePanes(out); !reflect.DeepEqual(got, want) {
		t.Errorf("panes: %v", got)
	}
	if got := parsePanes(""); len(got) != 0 {
		t.Errorf("no panes parsed as %v", got)
	}
}

// The configuration sets the prefix, empties tmux's prefix table, and
// binds two chords under it — - to the processes view, q to detach; it
// carries the readout; a path with a quote in it survives quoting.
func TestTheConfigurationHolds(t *testing.T) {
	conf := tmuxConf("C-Space")
	for _, s := range []string{
		"set -g prefix C-Space", "set -g prefix2 None", "unbind -a -T prefix", "bind - select-pane -t conn:home.0",
		`bind p set -gF @conn_from "#{pane_id}" \; select-pane -t conn:home.0 \; send-keys -t conn:home.0 M-p`,
		"bind q detach-client",
		"bind Tab select-pane -t conn:home.0 \\; send-keys -t conn:home.0 M-Tab",
		"bind j select-pane -t conn:home.0 \\; send-keys -t conn:home.0 M-j",
		"bind k select-pane -t conn:home.0 \\; send-keys -t conn:home.0 M-k",
		"bind s select-pane -t conn:home.0 \\; send-keys -t conn:home.0 M-s",
		"bind a select-pane -t conn:home.0 \\; send-keys -t conn:home.0 M-c",
		`bind ? set -gF @conn_from "#{pane_id}" \; select-pane -t conn:home.0 \; send-keys -t conn:home.0 M-?`,
		`bind M-a set -gF @conn_from "#{pane_id}" \; select-pane -t conn:home.0 \; send-keys -t conn:home.0 M-a`,
		"set -g status on", "set -g status-position bottom", "set -g mouse on", "unbind -n MouseDrag1Border",
		// The status line stands on the raised ground, which is what a chosen
		// row sits on: a surface of its own and not the last line of the pane
		// over it. Its text begins where the panel's does.
		`set -g status-style "bg=` + borderHex + `,fg=#8B8272"`,
		// A mode is a block of its color with the ground knocked out of
		// it. The attributes are parted by spaces rather than commas so
		// the conditional around a mode is not cut in two.
		"#[bg=" + cursorHex + " fg=" + hex(groundColor) + " bold] PREFIX ",
		"#[bg=" + cursorHex + " fg=" + hex(groundColor) + " bold] COPY ",
		`set -g window-status-format ""`,
		`set -g window-style "bg=#15130F,fg=#E6DFD0"`, `set -g pane-colours[15] "#E6DFD0"`,
		`set -g cursor-colour "#E85D2F"`, `set -g mode-style "bg=#2A2620,fg=#E6DFD0"`,
		`set -g pane-border-style "fg=#2A2620,bg=#15130F"`,
		"set-environment -g CONN 1", "set-environment -g CLAUDE_CODE_TMUX_TRUECOLOR 1",
		`set -ga update-environment " TERM_PROGRAM TERM_PROGRAM_VERSION"`, "set -g default-terminal tmux-256color", "set-environment -g COLORTERM truecolor",

		`set -g pane-border-style "fg=#2A2620,bg=#15130F"`, `set -g pane-active-border-style "fg=#2A2620,bg=#15130F"`,
	} {
		if !strings.Contains(conf, s) {
			t.Errorf("configuration lacks %q", s)
		}
	}
	if strings.Count(conf, "\nbind ") != 11 || strings.Contains(conf, "C-b") || strings.Contains(tmuxConf("C-a"), "C-Space") {
		t.Errorf("configuration binds more than the eleven chords, or ignores the prefix given:\n%s", conf)
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
	ws := parseWindows("home /Users/w0zro\nzsh /Users/w0zro/projects/w0zro/conn\nclaude /Users/w0zro/projects/w0zro/vim.pro\n")
	// The name is one token and the path is whatever is left of the
	// line, so a path with a space in it arrives whole.
	if w := parseWindows("claude /Users/w0zro/my notes\n"); len(w) != 1 || w[0].path != "/Users/w0zro/my notes" {
		t.Errorf("a path with a space in it: %+v", w)
	}
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

// The status line is dark at rest and lit by what cannot be seen from
// the panel: the two modes only tmux can know for nothing, and what
// conn says of its own keys, read out of an option and shown only while
// the keys are on the panel. The right of the line is empty, and
// nothing on it is re-read on a beat.
func TestOnlyTmuxDrawsTheStatusLine(t *testing.T) {
	conf := tmuxConf("C-Space")
	for _, gone := range []string{"@conn_in", "@conn_note", "@conn_rail", "@conn_slot", "@conn_lamps", "status-interval 1"} {
		if strings.Contains(conf, gone) {
			t.Errorf("the status line still asks conn for %q", gone)
		}
	}
	for _, want := range []string{
		"#{?client_prefix,", "#{?pane_in_mode,", "#{@conn_keys}",
		"set -g status-right \"\"",
		"#{&&:#{==:#{window_name},home},#{==:#{pane_index},0}}",
		"status-interval 0",
		// Every mode a block of the orange, the ground knocked out of it.
		"#[bg=" + cursorHex + " fg=" + hex(groundColor) + " bold] PREFIX ",
		"#[bg=" + cursorHex + " fg=" + hex(groundColor) + " bold] COPY ",
	} {
		if !strings.Contains(conf, want) {
			t.Errorf("the status line lacks %q:\n%s", want, conf)
		}
	}
	// The conf itself names no mode of conn's: PREFIX and COPY are tmux's
	// to know, and the word for the view is conn's, written into
	// @conn_keys when it changes, with @conn_station for the one state
	// that outlives the panel having the keys. The left is dark where
	// both are empty.
	left := conf[strings.Index(conf, "set -g status-left "):]
	left = left[:strings.Index(left, "\n")]
	for _, gone := range []string{"CONN", "PROCS", "PROJECTS", "SESSIONS", "CONSOLE"} {
		if strings.Contains(left, gone) {
			t.Errorf("the status line says %q at rest", gone)
		}
	}
	// Off the panel the line falls through to the station's own word,
	// which is empty unless the manual is up: dark at rest either way.
	if !strings.HasSuffix(left, ",#{@conn_station}}}}\"") {
		t.Errorf("the left of the status line is not dark at rest: %s", left)
	}
}

// The left of the status line says which view has the keys, or the
// question armed over it, and nothing else of conn's. It is written
// when it changes and not again for the same view.
func TestConnLightsTheStatusLine(t *testing.T) {
	m := newModel(plain)
	m.inside, m.srv = true, &server{tmux: "/nonexistent/tmux", socket: "/tmp/none"}
	m.projects = []project{{path: "/w", entries: []entry{
		{pid: 11, kind: kindContact, command: "claude", tty: "ttys004", status: statusWaiting},
	}}}

	// Each panel view wears its own word, and the console wears none: it
	// covers the window and says which page it is itself.
	for v, want := range map[int]string{
		viewProcesses: "PROCS",
		viewProjects:  "PROJECTS",
		viewSessions:  "SESSIONS",
	} {
		m.view = v
		if keys := m.keys(); keys != statusLineBlock(want) {
			t.Errorf("the %s view lights %q, not %s", want, keys, want)
		}
	}
	m.view = viewConsole
	if keys := m.keys(); keys != "" {
		t.Errorf("the console lights %q", keys)
	}
	// A question armed takes the next key whatever it is, and wears the
	// waiting color, which is the one thing waiting on you is said in.
	// It comes ahead of the view's word: while it stands, the view under
	// it cannot be worked, and its word would be a lie.
	m.view = viewProcesses
	// The question itself stands beside the block, on the status line's
	// own ground, with tmux's own character doubled so it is shown.
	m.kill = &pendingKill{pid: 11, command: "claude", sig: syscall.SIGTERM, prompt: "END CLAUDE 11 · #1"}
	if ask := m.keys(); !strings.HasPrefix(ask, statusLineBlock("CONFIRM")) || !strings.Contains(ask, "bg="+cursorHex) ||
		!strings.HasSuffix(ask, "  END CLAUDE 11 · ##1") || !strings.Contains(ask, "bg="+borderHex+" fg="+scheme[7]) ||
		strings.Contains(ask, "PROCS") {
		t.Errorf("a question armed lights %q", ask)
	}
	m.kill = nil

	// How the processes stand is the processes view's to say, in words,
	// and nothing of it reaches the status line.
	m.projects = []project{
		{path: "/w", entries: []entry{
			{pid: 11, kind: kindShell, status: statusIdle},
			{pid: 12, kind: kindContact, status: statusWorking, depth: 1},
		}},
		{path: "/x", entries: []entry{
			{pid: 21, kind: kindContact, status: statusWaiting},
			{pid: 22, kind: kindShell, status: statusStopped, fault: true},
		}},
	}
	m.view = viewProcesses
	waiting, _ := m.saying()
	quiet := m
	quiet.projects[1].entries[0].status = statusIdle
	still, _ := quiet.saying()
	if waiting.saidKeys != still.saidKeys {
		t.Errorf("a contact waiting changed the status line: %q against %q", waiting.saidKeys, still.saidKeys)
	}

	// The first writing goes out whatever the server holds: the option
	// outlives the conn that set it, and a reground respawns the panel
	// under a fresh one that has said nothing yet.
	first := m
	first.said, first.saidKeys = false, m.keys()
	if _, cmd := first.saying(); cmd == nil {
		t.Error("a conn that has said nothing yet left the status line as it found it")
	}

	// Written when it changes, and not again for the same view.
	next, cmd := m.saying()
	if cmd == nil {
		t.Fatal("what conn had not said was not put on the status line")
	}
	if _, again := next.saying(); again != nil {
		t.Error("the same word was written to the status line twice")
	}
	moved := next
	moved.view = viewProjects
	if _, changed := moved.saying(); changed == nil {
		t.Error("the keys moving to another view did not go out on the status line")
	}

	// Outside the server there is no status line to write to.
	out := m
	out.inside = false
	if _, cmd := out.saying(); cmd != nil {
		t.Error("conn wrote to the status line outside its server")
	}
}

// Every format conn hands tmux is printable ASCII. tmux sanitizes what
// it prints to something that is not a terminal, and what counts as
// printable is the locale's: in the C locale it turns a tab in the
// answer into an underscore, and conn read no panes at all. A space
// tells the fields apart in every locale there is.
func TestNoFormatAsksTmuxForAControlCharacter(t *testing.T) {
	for _, f := range []string{paneFormat, openFormat, windowFormat, statusLine(), tmuxConf("C-Space")} {
		for i, r := range f {
			if r == '\n' || r == '\t' && f == tmuxConf("C-Space") {
				continue // the configuration is a file of lines, not a format
			}
			if r < 0x20 || r == 0x7f {
				t.Errorf("a format asks tmux for %q at %d: %q", r, i, f)
			}
		}
	}
}
