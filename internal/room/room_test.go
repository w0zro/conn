package room

import (
	"testing"

	"github.com/w0zro/conn/internal/tmux"
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

func TestThePanelKeyIsTheOperatorsToSet(t *testing.T) {
	t.Setenv("CONN_KEY", "")
	if got := PanelKey(); got != DefaultKey {
		t.Errorf("the key is %q by default, want %q", got, DefaultKey)
	}
	t.Setenv("CONN_KEY", "M-a")
	if got := PanelKey(); got != "M-a" {
		t.Errorf("CONN_KEY names %q, want M-a", got)
	}
}

func TestOnlyWorkStillRunningIsReachable(t *testing.T) {
	for name, c := range map[string]struct {
		p    tmux.Pane
		want bool
	}{
		"work":        {tmux.Pane{ID: "%4"}, true},
		"no pane":     {tmux.Pane{}, false},
		"a hold":      {tmux.Pane{ID: "%4", Hold: true}, false},
		"the readout": {tmux.Pane{ID: "%4", Readout: true}, false},
		"ended":       {tmux.Pane{ID: "%4", Dead: true}, false},
	} {
		if got := Reachable(c.p); got != c.want {
			t.Errorf("%s: Reachable = %v, want %v", name, got, c.want)
		}
	}
}

// Every format conn hands tmux is printable ASCII. tmux sanitizes what
// it prints to something that is not a terminal, and what counts as
// printable is the locale's: in the C locale it turns a tab in the
// answer into an underscore, and conn read no panes at all. A space
// tells the fields apart in every locale there is.
func TestNoFormatAsksTmuxForAControlCharacter(t *testing.T) {
	d := Dress{Ground: "#15130F", Ink: "#E6DFD0", Accent: "#E85D2F", Border: "#2A2620", Gray: "#8B8272", Surface: "#1D1A16", Scheme: []string{"#000000"}, CopyBand: "#[bg=#E85D2F fg=#15130F bold] COPY ", CopyBar: " #[bg=#1D1A16 fg=#E6DFD0 bold]q #[bg=#1D1A16 fg=#8B8272 nobold]leave"}
	conf := Conf("C-Space", d)
	for _, f := range []string{statusLine(d), conf} {
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
