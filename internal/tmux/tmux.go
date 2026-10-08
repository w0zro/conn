// Package tmux is how conn talks to a tmux server: a command run on its
// socket and the answer read back, the panes and windows it holds, what
// a pane has written and how copy mode is put on a line of it, and the
// markup its status line is drawn in. It knows tmux's syntax and nothing
// of conn's own arrangement, which is internal/room's.
package tmux

import (
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// A Server is a tmux server: the tmux program and the socket.
type Server struct {
	Tmux   string
	Socket string
}

// Run runs a tmux command against the server and answers what it
// printed.
func (s *Server) Run(args ...string) (string, error) {
	out, err := exec.Command(s.Tmux, append([]string{"-S", s.Socket}, args...)...).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("tmux %s: %s", args[0], strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("tmux %s: %w", args[0], err)
	}
	return string(out), nil
}

// Paint sets the ground a pane is painted on past what is drawn in it.
//
// The style is set as the pane's own option rather than with
// select-pane -P, which paints a pane by first making it the pane the
// keys are in. That was nothing while the panel only came up with the
// keys already in it. With the settings in the workspace it took them
// out from under the operator mid-page, and a panel started again on a
// new build took them out of the process they were working in.
func (s *Server) Paint(pane, bg string) error {
	_, err := s.Run("set-option", "-p", "-t", pane, "window-style", "bg="+bg)
	return err
}

// WithoutTmux is an environment with tmux's own variables dropped.
func WithoutTmux(env []string) []string {
	out := env[:0:0]
	for _, kv := range env {
		if strings.HasPrefix(kv, "TMUX=") || strings.HasPrefix(kv, "TMUX_PANE=") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// A Pane of the server: its id, which holds through swaps; the terminal
// it holds; its size; whether it is conn's own furniture and whether
// that furniture is a readout; and whether remain-on-exit is the only
// thing keeping it up, its process already gone.
//
// A readout is furniture too — everything true of a hold is true of it,
// so it carries the hold's own mark and everything that acts on holds
// acts on it — but the panel has to tell the two apart to know whether
// the page is up, and a mark of its own is how.
type Pane struct {
	ID, TTY       string
	Width, Height int
	Hold          bool
	Readout       bool
	Dead          bool
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
	// Where it stands in its window, which is how the bay is told from
	// anything else beside the panel: home has two panes, the panel at
	// 0 and the bay at 1, and a third is a mistake to be mended rather
	// than a pane to be guessed between.
	index int
}

// What conn asks tmux for, and how it reads the answer back. The
// fields are separated by a space and not by a tab, because tmux
// sanitizes its output when it prints to something that is not a
// terminal, and what counts as printable is the locale's business: in
// the C locale, which is what a container and a bare service manager
// leave you in, tmux turns every tab in the answer into an underscore.
// conn then read no panes at all, could not find its own bay, and the
// whole of the server layer stopped working on a machine that had done
// nothing wrong. Every field asked for here is one token — an id, a
// device, a number, a flag conn itself set — so a space tells them
// apart and never appears inside one. A field with nothing in it is an
// empty string between two spaces and keeps its place, which is why
// these are split and not fielded.
const (
	openFormat   = "#{pane_id} #{pane_pid} #{pane_tty}"
	windowFormat = "#{window_name} #{pane_current_path}"
)

// paneFields is what conn asks tmux of a pane, a field at a time: what
// tmux fills in, and where the answer goes. paneFormat is these in
// order and parsePanes reads them back in the same order, so a field is
// added in one place and the two cannot fall out of step.
var paneFields = []struct {
	format string
	read   func(p *Pane, v string)
}{
	{"#{pane_id}", func(p *Pane, v string) { p.ID = v }},
	{"#{pane_tty}", func(p *Pane, v string) { p.TTY = strings.TrimPrefix(v, "/dev/") }},
	{"#{pane_width}", func(p *Pane, v string) { p.Width, _ = strconv.Atoi(v) }},
	{"#{pane_height}", func(p *Pane, v string) { p.Height, _ = strconv.Atoi(v) }},
	{"#{@conn_hold}", func(p *Pane, v string) { p.Hold = v == "1" }},
	{"#{pane_dead}", func(p *Pane, v string) { p.Dead = v == "1" }},
	{"#{@conn_readout}", func(p *Pane, v string) { p.Readout = v == "1" }},
	{"#{@conn_container}", func(p *Pane, v string) { p.Container = v }},
	{"#{@conn_shell_in}", func(p *Pane, v string) { p.ShellIn = v }},
	{"#{@conn_help}", func(p *Pane, v string) { p.Help = v == "1" }},
	{"#{@conn_declared}", func(p *Pane, v string) { p.Declared = v }},
	{"#{@conn_exit}", func(p *Pane, v string) { p.Exit = v }},
	{"#{pane_active}", func(p *Pane, v string) { p.Active = v == "1" }},
	{"#{pane_index}", func(p *Pane, v string) { p.index, _ = strconv.Atoi(v) }},
	{"#{@conn_settings}", func(p *Pane, v string) { p.Settings = v == "1" }},
}

// paneFormat is the fields, as list-panes is asked for them.
var paneFormat = func() string {
	formats := make([]string, len(paneFields))
	for i, f := range paneFields {
		formats[i] = f.format
	}
	return strings.Join(formats, " ")
}()

// Panes is every pane in the server, by the terminal it holds.
func (s *Server) Panes() (map[string]Pane, error) {
	out, err := s.Run("list-panes", "-a", "-F", paneFormat)
	if err != nil {
		return nil, err
	}
	return parsePanes(out), nil
}

// parsePanes reads list-panes in paneFormat, with each terminal's /dev/
// dropped to match how a process names its own.
func parsePanes(out string) map[string]Pane {
	panes := map[string]Pane{}
	for _, l := range strings.Split(out, "\n") {
		f := strings.Split(l, " ")
		if len(f) != len(paneFields) || f[0] == "" {
			continue
		}
		var p Pane
		for i, field := range paneFields {
			field.read(&p, f[i])
		}
		panes[p.TTY] = p
	}
	return panes
}

// WindowPanes is every pane of the window a target is in, in the order
// they stand.
func (s *Server) WindowPanes(target string) ([]Pane, error) {
	out, err := s.Run("list-panes", "-t", target, "-F", paneFormat)
	if err != nil {
		return nil, err
	}
	var panes []Pane
	for _, p := range parsePanes(out) {
		panes = append(panes, p)
	}
	sort.Slice(panes, func(i, j int) bool { return panes[i].index < panes[j].index })
	return panes, nil
}

// Scrollback is everything a pane has written that tmux still holds:
// its history and its screen, a line each, oldest first, with the
// count of history lines among them. The lines are the pane's own
// rows and not joined where a long line wrapped, so a line's index is
// the line copy mode counts to.
func (s *Server) Scrollback(id string) (lines []string, history int, err error) {
	out, err := s.Run("display-message", "-p", "-t", id, "#{history_size}")
	if err != nil {
		return nil, 0, err
	}
	history, _ = strconv.Atoi(strings.TrimSpace(out))
	out, err = s.Run("capture-pane", "-p", "-S", "-", "-E", "-", "-t", id)
	if err != nil {
		return nil, 0, err
	}
	lines = strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	return lines, history, nil
}

// Tail is the last n lines a pane holds, history and screen, oldest
// first: what it has said lately, for a listener that reads it again
// on a beat and wants the end and not the whole.
func (s *Server) Tail(id string, n int) ([]string, error) {
	out, err := s.Run("capture-pane", "-p", "-S", "-"+strconv.Itoa(n), "-E", "-", "-t", id)
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimSuffix(out, "\n"), "\n"), nil
}

// Land puts a pane in copy mode with its cursor on the k-th saying of
// a text counted back from the pane's end, the text as the search in
// hand, and the line in the middle of the pane with its context either
// side. It is tmux's own copy mode and search, which the operator
// already knows how to leave; conn only says where to start, so n and N
// go on from there as in any pane.
//
// The match is found by searching and not by a line's number: a pane
// that takes the bay's width wraps its history again, and a number
// counted before the move lands a line off, where the k-th saying of a
// text is the same saying at any width. tmux puts a match it had to
// scroll to a quarter up from the foot; the pane is then scrolled to
// put the line in the middle, and the search run again from the end
// of the line above it, which finds that saying and no other and
// leaves the search in hand.
func (s *Server) Land(id string, k int, text string) error {
	s.Unmode(id)
	if _, err := s.Run(landSearch(id, k, text)...); err != nil {
		return err
	}
	out, err := s.Run("display-message", "-p", "-t", id, "#{copy_cursor_y} #{scroll_position} #{history_size} #{pane_height}")
	if err != nil {
		return err
	}
	var cy, oy, hs, sy int
	if _, err := fmt.Sscan(out, &cy, &oy, &hs, &sy); err != nil {
		return err
	}
	scroll, row := centering(cy, oy, hs, sy)
	_, err = s.Run(landCenter(id, scroll, row, text)...)
	return err
}

// landSearch is the search that finds the saying: copy mode from the
// pane's end, back to the last saying of the text, and k more back.
func landSearch(id string, k int, text string) []string {
	args := []string{"copy-mode", "-t", id,
		";", "send-keys", "-t", id, "-X", "history-bottom",
		";", "send-keys", "-t", id, "-X", "end-of-line",
		";", "send-keys", "-t", id, "-X", "search-backward-text", text}
	if k > 0 {
		args = append(args, ";", "send-keys", "-t", id, "-X", "-N", strconv.Itoa(k), "search-again")
	}
	return args
}

// centering is where to scroll a pane so the cursor's line sits in the
// middle, and which row the line is on once it does: the cursor is at
// row cy with the view scrolled oy lines up its history of hs lines on
// a pane sy rows tall. Copy mode counts its scroll from the bottom, and
// cannot scroll past either end, so a line near the top of a short
// history sits as near the middle as the history allows.
func centering(cy, oy, hs, sy int) (scroll, row int) {
	target := (sy - 1) / 2
	scroll = min(max(oy-cy+target, 0), hs)
	return scroll, cy + scroll - oy
}

// landCenter scrolls the pane to the centring and puts the cursor on
// the saying again: the view to the scroll, the cursor to the end of
// the row above the line, and the search forward from there. The
// first row has no row above it, and the cursor starts at its start.
func landCenter(id string, scroll, row int, text string) []string {
	args := []string{"send-keys", "-t", id, "-X", "goto-line", strconv.Itoa(scroll),
		";", "send-keys", "-t", id, "-X", "top-line"}
	switch {
	case row > 1:
		args = append(args, ";", "send-keys", "-t", id, "-X", "-N", strconv.Itoa(row-1), "cursor-down")
		fallthrough
	case row == 1:
		args = append(args, ";", "send-keys", "-t", id, "-X", "end-of-line")
	default:
		args = append(args, ";", "send-keys", "-t", id, "-X", "start-of-line")
	}
	return append(args, ";", "send-keys", "-t", id, "-X", "search-forward-text", text)
}

// Unmode takes a pane out of copy mode, where it is in it. A pane left
// in copy mode by a preview the keys never went into would stand frozen
// on its screen for whoever next went in; a pane not in a mode is left
// alone, tmux's word that it is not being nothing to do.
func (s *Server) Unmode(id string) {
	_, _ = s.Run("send-keys", "-t", id, "-X", "cancel")
}

// A Shell conn opened, as tmux answers when it makes the window: the
// pane it is in and the process in it. What tmux says the process is
// called at that instant is tmux itself, before the shell has taken
// over, so the name is left to the process table.
type Shell struct {
	Pane Pane
	PID  int
}

// PaneExit is what a declared process's pane has recorded of its end:
// the code, or nothing while it is still going. An error is a pane
// that is not there to ask.
func (s *Server) PaneExit(id string) (string, error) {
	out, err := s.Run("display-message", "-p", "-t", id, "#{@conn_exit}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// Interrupt is ctrl-c in a pane, as tmux types it: what a hand does to
// stop the thing it ran there.
func (s *Server) Interrupt(id string) error {
	_, err := s.Run("send-keys", "-t", id, "C-c")
	return err
}

// ClosePane takes a pane down: a declared process's, once its output
// has been read, or once it was asked to end.
func (s *Server) ClosePane(id string) error {
	_, err := s.Run("kill-pane", "-t", id)
	return err
}

// NewWindow opens a window out of sight, at a directory, running a
// command or, with none, the directory's own shell, and answers the
// pane it made.
func (s *Server) NewWindow(dir, cmd string) (Shell, error) {
	args := []string{"new-window", "-d", "-P", "-F", openFormat, "-c", dir}
	if cmd != "" {
		args = append(args, cmd)
	}
	out, err := s.Run(args...)
	if err != nil {
		return Shell{}, err
	}
	return parseOpened(out), nil
}

// parseOpened reads what new-window printed for the pane it made.
func parseOpened(out string) Shell {
	f := strings.Split(strings.TrimSpace(out), " ")
	for len(f) < 3 {
		f = append(f, "")
	}
	sh := Shell{Pane: Pane{ID: f[0], TTY: strings.TrimPrefix(f[2], "/dev/")}}
	sh.PID, _ = strconv.Atoi(f[1])
	return sh
}

// Select puts the keys in a pane by its id, which reaches only a
// pane of the window the client is looking at — so it is the panel's
// own way back, and not a way to somewhere else. Putting the keys in a
// process means reaching it: show brings the pane into the workspace
// first, and the window the client is on is the one it was already on.
func (s *Server) Select(id string) error {
	_, err := s.Run("select-pane", "-t", id)
	return err
}

// Detach lets the client go; the server keeps on.
func (s *Server) Detach() error {
	_, err := s.Run("detach-client")
	return err
}

// ShellQuote quotes a path for a tmux command line.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// A Window of the server, for the report of what conn down ends: its
// name and the directory its pane is in.
type Window struct {
	Name, Path string
}

// Windows is every window in the server.
func (s *Server) Windows() ([]Window, error) {
	out, err := s.Run("list-windows", "-a", "-F", windowFormat)
	if err != nil {
		return nil, err
	}
	return ParseWindows(out), nil
}

// ParseWindows reads list-windows: a name and a path per line.
func ParseWindows(out string) []Window {
	var ws []Window
	for _, l := range strings.Split(out, "\n") {
		// The name is one token and the path is whatever is left, so a
		// path with a space in it arrives whole.
		name, path, ok := strings.Cut(l, " ")
		if !ok {
			continue
		}
		ws = append(ws, Window{Name: name, Path: path})
	}
	return ws
}

// Up says whether the server is up.
func (s *Server) Up() bool {
	_, err := s.Run("has-session")
	return err == nil
}

// Down ends the server and everything in it, and clears the ground it
// came up on, so the next one to rise picks fresh.
func (s *Server) Down() error {
	_, err := s.Run("kill-server")
	return err
}

// HoldOpen keeps a pane standing after what it was opened for has
// finished. cat with nothing to read waits on the terminal for as long
// as the pane is there, which is exactly as long as wanted: the operator
// leaves by going somewhere else, and the pane goes when its work is
// replaced in the workspace.
//
// It is for a pane with something left to read in it — a log that ended,
// an error docker printed. A pane whose work is over and has left
// nothing behind should go, and a shell is that.
const HoldOpen = "exec cat"
