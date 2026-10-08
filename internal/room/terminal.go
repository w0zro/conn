package room

import "github.com/w0zro/conn/internal/tmux"

// What the panel asks of a terminal the server holds, and of the server
// itself, past arranging them. tmux answers each; the panel asks room.

// Scrollback is everything a pane has written that the server still
// holds, oldest first, and how much of it is history above the screen.
func (s *Server) Scrollback(id string) (lines []string, history int, err error) {
	return s.tmux.Scrollback(id)
}

// Tail is the last n lines a pane holds, oldest first.
func (s *Server) Tail(id string, n int) ([]string, error) { return s.tmux.Tail(id, n) }

// Land puts a pane in copy mode on the k-th saying of a text.
func (s *Server) Land(id string, k int, text string) error { return s.tmux.Land(id, k, text) }

// Unmode takes a pane out of copy mode, where it is in it.
func (s *Server) Unmode(id string) { s.tmux.Unmode(id) }

// Interrupt is ctrl-c in a pane, as a hand types it.
func (s *Server) Interrupt(id string) error { return s.tmux.Interrupt(id) }

// ClosePane takes a pane down.
func (s *Server) ClosePane(id string) error { return s.tmux.ClosePane(id) }

// Paint sets the ground a pane is painted on past what is drawn in it.
func (s *Server) Paint(pane, bg string) error { return s.tmux.Paint(pane, bg) }

// Detach lets the client go; the server keeps on.
func (s *Server) Detach() error { return s.tmux.Detach() }

// Up says whether the server is up.
func (s *Server) Up() bool { return s.tmux.Up() }

// Windows is every window in the server, for what conn down ends.
func (s *Server) Windows() ([]tmux.Window, error) { return s.tmux.Windows() }

// Down ends the server and everything in it.
func (s *Server) Down() error { return s.tmux.Down() }
