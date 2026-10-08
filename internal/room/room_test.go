package room

import (
	"strings"
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
		p    Pane
		want bool
	}{
		"work":        {Pane{ID: "%4"}, true},
		"no pane":     {Pane{}, false},
		"a hold":      {Pane{ID: "%4", Hold: true}, false},
		"the readout": {Pane{ID: "%4", Readout: true}, false},
		"ended":       {Pane{ID: "%4", Dead: true}, false},
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

// A pane is read for what conn marked it as. A readout carries the
// hold's own mark as well as its own: it is furniture like a hold, and
// everything that acts on holds acts on it; only the panel has to tell
// the two apart. The manual and the settings are furniture the same
// way. A pane conn opened for a container says which, and a shell
// inside one says which on the other mark: it is work of the
// operator's own, not the service being read. A pane opened for a
// declared process says which, and how the process ended once it has.
func TestAPaneIsReadByItsMarks(t *testing.T) {
	base := tmux.Pane{ID: "%7", TTY: "ttys009", Width: 138, Height: 40, Active: true}
	with := func(opts map[string]string) tmux.Pane { p := base; p.Opts = opts; return p }
	for name, c := range map[string]struct {
		in   tmux.Pane
		want Pane
	}{
		"work": {with(nil), Pane{ID: "%7", TTY: "ttys009", Width: 138, Height: 40, Active: true}},
		"furniture": {with(map[string]string{holdMark: "1", readoutMark: "1", helpMark: "1", settingsMark: "1"}),
			Pane{ID: "%7", TTY: "ttys009", Width: 138, Height: 40, Active: true, Hold: true, Readout: true, Help: true, Settings: true}},
		"container": {with(map[string]string{containerMark: "9f1c2d3e4a5b"}),
			Pane{ID: "%7", TTY: "ttys009", Width: 138, Height: 40, Active: true, Container: "9f1c2d3e4a5b"}},
		"shell in": {with(map[string]string{shellInMark: "9f1c2d3e4a5b"}),
			Pane{ID: "%7", TTY: "ttys009", Width: 138, Height: 40, Active: true, ShellIn: "9f1c2d3e4a5b"}},
		"declared": {with(map[string]string{declaredMark: "web@%2FUsers%2Fw0zro%2Fapp", exitMark: "1"}),
			Pane{ID: "%7", TTY: "ttys009", Width: 138, Height: 40, Active: true, Declared: "web@%2FUsers%2Fw0zro%2Fapp", Exit: "1"}},
	} {
		if got := paneOf(c.in); got != c.want {
			t.Errorf("%s: %+v, want %+v", name, got, c.want)
		}
	}
}

// The line run in the pane is the command as written, then the exit
// recorded and the hold, each on a line of its own.
func TestTheLineRunInThePane(t *testing.T) {
	got := declaredLine("npm run dev # dev", "web", "/opt/bin/tmux")
	want := "npm run dev # dev\n'/opt/bin/tmux' set-option -p -t \"$TMUX_PANE\" @conn_exit \"$?\"\nprintf '\\n[web exited]\\n'\nexec cat"
	if got != want {
		t.Errorf("line:\n%s\nwant:\n%s", got, want)
	}
}

// What the note says has to name the socket, since a contact told to
// open a window and not told which server would be guessing; and it
// says how to put work in a window and read it back.
func TestTheContactIsToldWhereItIs(t *testing.T) {
	for _, socket := range []string{"/Users/w0zro/.local/state/conn/tmux.sock", "/tmp/it's here/conn.sock"} {
		note := ContactNote(socket)
		for _, want := range []string{socket, "new-window", "capture-pane"} {
			if !strings.Contains(note, want) {
				t.Errorf("the note says nothing of %q:\n%s", want, note)
			}
		}
	}
}
