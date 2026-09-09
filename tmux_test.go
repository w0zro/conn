package main

import (
	"reflect"
	"strings"
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
	out := "%0\t/dev/ttys004\t48\t40\t\n%1\t/dev/ttys007\t138\t40\t1\n%5\t/dev/ttys008\t138\t40\t\n\n"
	want := map[string]pane{
		"ttys004": {id: "%0", tty: "ttys004", width: 48, height: 40},
		"ttys007": {id: "%1", tty: "ttys007", width: 138, height: 40, hold: true},
		"ttys008": {id: "%5", tty: "ttys008", width: 138, height: 40},
	}
	if got := parsePanes(out); !reflect.DeepEqual(got, want) {
		t.Errorf("panes: %v", got)
	}
	if got := parsePanes(""); len(got) != 0 {
		t.Errorf("no panes parsed as %v", got)
	}
}

// The configuration carries the prefix, the look, and the keys back to
// the watch; a path with a quote in it survives quoting.
func TestTheConfigurationHolds(t *testing.T) {
	conf := tmuxConf()
	for _, s := range []string{
		"set -g prefix C-Space", "set -g status off", "set -g mouse on",
		`set -g window-style "bg=#15130F,fg=#E6DFD0"`, `set -g pane-colours[15] "#E6DFD0"`,
		"set -g default-terminal tmux-256color", "set-environment -g COLORTERM truecolor", "bind w select-pane -L", "bind C-Space last-pane",
	} {
		if !strings.Contains(conf, s) {
			t.Errorf("configuration lacks %q", s)
		}
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
	if oscOwnColors != "\x1b]110\x1b\\\x1b]111\x1b\\" {
		t.Errorf("colors given back: %q", oscOwnColors)
	}
	if want := "bg=" + hex(groundColor) + ",fg=" + hex(inkColor); !strings.Contains(tmuxConf(), want) {
		t.Errorf("the panes are not drawn in %s", want)
	}
}
