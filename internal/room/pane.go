package room

import "github.com/w0zro/conn/internal/tmux"

// A Pane of conn's server: what tmux says of it, and the marks conn
// set on it when it opened it, which say what the pane is to conn.
//
// Hold is conn's own furniture: a pane in the bay that holds its place
// with nothing to show. A readout is furniture too — everything true of
// a hold is true of it, so it carries the hold's own mark and
// everything that acts on holds acts on it — but the panel has to tell
// the two apart to know whether the page is up, and a mark of its own
// is how.
type Pane struct {
	ID, TTY       string
	Width, Height int
	Hold          bool
	Readout       bool
	// Whether remain-on-exit is the only thing keeping it up, its
	// process already gone.
	Dead bool
	// The container this pane is watching, where it is one conn opened
	// to read a service's output. A container has no terminal of its
	// own, so this is how its row comes to have one: the pane conn
	// opened for it stands in for the terminal it has not got, and from
	// there the row is reached and left like any other.
	//
	// Exactly one pane stands for a service this way. A shell conn
	// opened inside the container is marked shellIn instead, because it
	// is not the service being read — it is work of the operator's own
	// that happens to be running in there, and a second pane claiming
	// to be the service's terminal would leave the row pointing at
	// whichever of the two a map ranged over last.
	Container string
	ShellIn   string
	// The declaration this pane was opened for, as declared.go marks
	// it, and, once the command in it has ended, the status it ended
	// with, which the pane's own line records. A pane that is a
	// declaration's is the work itself and is listed like any other.
	Declared string
	Exit     string
	// The manual, which ? puts in the workspace. It is conn's
	// own furniture like the readout: it carries the hold's mark as
	// well, so everything that steps over furniture steps over it, and
	// this says which furniture it is.
	Help bool
	// The settings, which , puts in the workspace. Furniture again,
	// and marked apart from the manual for the same reason the manual
	// is marked apart from the readout: the panel says which of them
	// is standing, and answers for the keys that are in it.
	Settings bool
	// Whether this is its window's active pane, which is tmux's word
	// for where the keys are in that window. The panel is told by the
	// terminal when the keys leave it and knows on its own when its
	// reaching sent them away; this is the same fact read off the
	// server, for a reading that lands between the two.
	Active bool
}

// The marks conn sets on the panes it opens, as tmux user options.
const (
	holdMark      = "@conn_hold"
	readoutMark   = "@conn_readout"
	helpMark      = "@conn_help"
	settingsMark  = "@conn_settings"
	containerMark = "@conn_container"
	shellInMark   = "@conn_shell_in"
	declaredMark  = "@conn_declared"
	exitMark      = "@conn_exit"
)

// marks is every mark, as a pane is read with them.
var marks = []string{holdMark, readoutMark, helpMark, settingsMark, containerMark, shellInMark, declaredMark, exitMark}

// paneOf is a pane of tmux's as conn reads it, by its marks.
func paneOf(p tmux.Pane) Pane {
	return Pane{
		ID: p.ID, TTY: p.TTY, Width: p.Width, Height: p.Height, Dead: p.Dead, Active: p.Active,
		Hold: p.Opts[holdMark] == "1", Readout: p.Opts[readoutMark] == "1",
		Help: p.Opts[helpMark] == "1", Settings: p.Opts[settingsMark] == "1",
		Container: p.Opts[containerMark], ShellIn: p.Opts[shellInMark],
		Declared: p.Opts[declaredMark], Exit: p.Opts[exitMark],
	}
}

// Panes is every pane in the server, by the terminal it holds, with
// conn's marks read.
func (s *Server) Panes() (map[string]Pane, error) {
	ps, err := s.tmux.Panes(marks...)
	if err != nil {
		return nil, err
	}
	out := make(map[string]Pane, len(ps))
	for tty, p := range ps {
		out[tty] = paneOf(p)
	}
	return out, nil
}

// home is every pane of the home window, in the order they stand.
func (s *Server) home() ([]Pane, error) {
	ps, err := s.tmux.WindowPanes(s.Panel(), marks...)
	if err != nil {
		return nil, err
	}
	out := make([]Pane, len(ps))
	for i, p := range ps {
		out[i] = paneOf(p)
	}
	return out, nil
}

// A Shell conn opened, as tmux answers when it makes the window: the
// pane it is in and the process in it.
type Shell struct {
	Pane Pane
	PID  int
}

// open opens a window out of sight, at a directory, running a command
// or the directory's own shell, and answers the shell as conn holds it.
func (s *Server) open(dir, cmd string) (Shell, error) {
	sh, err := s.tmux.NewWindow(dir, cmd)
	return Shell{Pane: paneOf(sh.Pane), PID: sh.PID}, err
}

// PaneExit is what a declared process's pane has recorded of its end:
// the code, or nothing while it is still going. An error is a pane
// that is not there to ask.
func (s *Server) PaneExit(id string) (string, error) {
	return s.tmux.Display(id, "#{"+exitMark+"}")
}
