package tmux

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// A Cmd is one tmux command, in tmux's words. Commands run alone, or
// several to one invocation with Do, so that nothing another client
// does lands between them: a pane swapped into place and the one it
// displaced killed are one step to tmux, not two.
type Cmd []string

// Do runs commands, in order, as one invocation, and answers what they
// printed.
func (s *Server) Do(cmds ...Cmd) (string, error) {
	var args []string
	for i, c := range cmds {
		if i > 0 {
			args = append(args, ";")
		}
		args = append(args, c...)
	}
	if len(args) == 0 {
		return "", nil
	}
	return s.Run(args...)
}

// SourceFile reads a configuration into the server that is up: every
// set in it lands on the live server.
func SourceFile(path string) Cmd { return Cmd{"source-file", path} }

// Respawn starts a pane again on a command, killing what runs in it.
func Respawn(target, command string) Cmd {
	return Cmd{"respawn-pane", "-k", "-t", target, command}
}

// SetGlobal sets a server-wide option, and UnsetGlobal takes it off.
func SetGlobal(name, value string) Cmd { return Cmd{"set-option", "-g", name, value} }
func UnsetGlobal(name string) Cmd      { return Cmd{"set-option", "-gu", name} }

// SetPane sets an option of a pane's own.
func SetPane(pane, name, value string) Cmd {
	return Cmd{"set-option", "-p", "-t", pane, name, value}
}

// SetWindow sets an option of a window's own.
func SetWindow(target, name, value string) Cmd {
	return Cmd{"set-option", "-w", "-t", target, name, value}
}

// Swap puts one pane where another stands and the other where it stood,
// leaving the keys where they are.
func Swap(source, target string) Cmd { return Cmd{"swap-pane", "-d", "-s", source, "-t", target} }

// Kill takes a pane down.
func Kill(pane string) Cmd { return Cmd{"kill-pane", "-t", pane} }

// SelectPane puts the keys in a pane.
func SelectPane(pane string) Cmd { return Cmd{"select-pane", "-t", pane} }

// ResizeWindow sizes the window a pane is in.
func ResizeWindow(target string, width, height int) Cmd {
	return Cmd{"resize-window", "-t", target, "-x", strconv.Itoa(width), "-y", strconv.Itoa(height)}
}

// ResizeWidth sets a pane to a number of columns.
func ResizeWidth(pane string, cols int) Cmd {
	return Cmd{"resize-pane", "-t", pane, "-x", strconv.Itoa(cols)}
}

// ToggleZoom gives a pane the whole of its window, or gives the window
// back.
func ToggleZoom(pane string) Cmd { return Cmd{"resize-pane", "-Z", "-t", pane} }

// SendKeys types keys into a pane, in tmux's spelling of a key.
func SendKeys(pane string, keys ...string) Cmd {
	return append(Cmd{"send-keys", "-t", pane}, keys...)
}

// RefreshStatus asks the clients to draw the status line now.
func RefreshStatus() Cmd { return Cmd{"refresh-client", "-S"} }

// Global is a server-wide option's value, blank where it is not set.
func (s *Server) Global(name string) (string, error) {
	out, err := s.Run("show-options", "-gqv", name)
	return strings.TrimSpace(out), err
}

// Display is a format as tmux fills it in for a target.
func (s *Server) Display(target, format string) (string, error) {
	out, err := s.Run("display-message", "-p", "-t", target, format)
	return strings.TrimSpace(out), err
}

// HasSession says whether the server has a session by that exact name.
func (s *Server) HasSession(name string) bool {
	_, err := s.Run("has-session", "-t", "="+name)
	return err == nil
}

// WindowNames is the name of every window of a session.
func (s *Server) WindowNames(session string) ([]string, error) {
	out, err := s.Run("list-windows", "-t", session, "-F", "#{window_name}")
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimSpace(out), "\n"), nil
}

// NewNamedWindow opens a window of a session, by a name, out of sight,
// at a directory, running a command.
func (s *Server) NewNamedWindow(session, name, dir, command string) error {
	_, err := s.Run("new-window", "-d", "-t", session+":", "-n", name, "-c", dir, command)
	return err
}

// SplitRight opens a pane to the right of one, at a directory, running a
// command, and leaves the keys where they are. It answers the new pane.
func (s *Server) SplitRight(pane, dir, command string) (string, error) {
	out, err := s.Run("split-window", "-h", "-d", "-P", "-F", "#{pane_id}", "-t", pane, "-c", dir, command)
	return strings.TrimSpace(out), err
}

// AttachClient puts this terminal on a session as a client, making the
// session first if there is none, with a window of a name at a
// directory running a command, the server reading conf if it has to
// rise. It answers how the client exited; an error is one of its own,
// before the client had the terminal.
func (s *Server) AttachClient(conf, session, window, dir, command string) (int, error) {
	cmd := exec.Command(s.Tmux, "-S", s.Socket, "-f", conf,
		"new-session", "-A", "-s", session, "-n", window, "-c", dir, command)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	// A tmux inside another tmux refuses to attach while TMUX is set; the
	// terminal is this process's to give.
	cmd.Env = WithoutTmux(os.Environ())
	err := cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode(), nil
	}
	return 0, err
}
