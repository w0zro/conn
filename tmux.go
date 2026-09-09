package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// conn holds a tmux server of its own. The first conn brings it up with
// one window, watch, running conn, and attaches; a later conn attaches
// to what is there. Work opened from the watch lives in the server as
// windows, so a row can be reached and left, and stays when the client
// goes. q detaches; the server and everything in it keep on. The conn
// that attached stays behind the client, to give the terminal its own
// colors back when the client returns, which tmux does not.
//
// The socket is under the state directory, or where CONN_SOCKET says,
// which is how a test brings up a server of its own.

const (
	sessionName = "conn"
	watchWindow = "watch"
)

// A server is conn's tmux server: the tmux program and the socket.
type server struct {
	tmux   string
	socket string
}

// findServer is the server as this machine has it: no tmux, no server.
func findServer(home string) *server {
	tmux := lookPath("tmux")
	if tmux == "" {
		return nil
	}
	return &server{tmux: tmux, socket: socketPath(home)}
}

// socketPath is where the server listens: CONN_SOCKET, or tmux.sock in
// the state directory.
func socketPath(home string) string {
	if p := os.Getenv("CONN_SOCKET"); p != "" {
		return p
	}
	return filepath.Join(stateHome(home), "conn", "tmux.sock")
}

// stateHome is where state goes: XDG_STATE_HOME, or ~/.local/state.
func stateHome(home string) string {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return dir
	}
	return filepath.Join(home, ".local", "state")
}

// insideConn says whether this process runs in a pane of conn's server:
// tmux tells its panes the socket in TMUX, before the first comma.
func insideConn(tmuxEnv, socket string) bool {
	sock, _, _ := strings.Cut(tmuxEnv, ",")
	return sock != "" && filepath.Clean(sock) == filepath.Clean(socket)
}

// run runs a tmux command against the server and answers what it
// printed.
func (s *server) run(args ...string) (string, error) {
	out, err := exec.Command(s.tmux, append([]string{"-S", s.socket}, args...)...).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("tmux %s: %s", args[0], strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("tmux %s: %w", args[0], err)
	}
	return string(out), nil
}

// attach brings the server up if it is down, with the watch window
// running conn, and puts this terminal on it until the client detaches
// or the server ends. It answers how the client exited; an error is one
// of its own, before the client had the terminal.
func (s *server) attach(self, home string) (int, error) {
	if err := os.MkdirAll(filepath.Dir(s.socket), 0o700); err != nil {
		return 0, err
	}
	conf := filepath.Join(filepath.Dir(s.socket), "tmux.conf")
	if err := os.WriteFile(conf, []byte(tmuxConf()), 0o600); err != nil {
		return 0, err
	}
	// A server that is up but has lost its watch window gets one back.
	if _, err := s.run("has-session", "-t", "="+sessionName); err == nil {
		if !s.hasWatch() {
			if _, err := s.run("new-window", "-d", "-t", sessionName+":", "-n", watchWindow, "-c", home, "exec "+shellQuote(self)); err != nil {
				return 0, err
			}
		}
	}
	cmd := exec.Command(s.tmux, "-S", s.socket, "-f", conf,
		"new-session", "-A", "-s", sessionName, "-n", watchWindow, "-c", home, "exec "+shellQuote(self))
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	// A tmux inside another tmux refuses to attach while TMUX is set; the
	// terminal is this conn's to give.
	cmd.Env = withoutTmux(os.Environ())
	err := cmd.Run()
	// The client is gone and the terminal is ours again: the colors conn
	// asked it to take go back to its own.
	fmt.Print("\x1b]110\x1b\\\x1b]111\x1b\\")
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode(), nil
	}
	if err != nil {
		return 0, err
	}
	return 0, nil
}

// withoutTmux is an environment with tmux's own variables dropped.
func withoutTmux(env []string) []string {
	out := env[:0:0]
	for _, kv := range env {
		if strings.HasPrefix(kv, "TMUX=") || strings.HasPrefix(kv, "TMUX_PANE=") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// hasWatch says whether the session has its watch window.
func (s *server) hasWatch() bool {
	out, err := s.run("list-windows", "-t", sessionName, "-F", "#{window_name}")
	if err != nil {
		return false
	}
	for _, name := range strings.Split(out, "\n") {
		if name == watchWindow {
			return true
		}
	}
	return false
}

// panes is every pane in the server by the terminal it holds, as a
// target a command can be aimed at.
func (s *server) panes() (map[string]string, error) {
	out, err := s.run("list-panes", "-a", "-F", "#{pane_tty}\t#{session_name}:#{window_index}.#{pane_index}")
	if err != nil {
		return nil, err
	}
	return parsePanes(out), nil
}

// parsePanes reads list-panes: a terminal and a target per line, with
// the terminal's /dev/ dropped to match how a process names its own.
func parsePanes(out string) map[string]string {
	panes := map[string]string{}
	for _, l := range strings.Split(out, "\n") {
		tty, target, ok := strings.Cut(l, "\t")
		if !ok {
			continue
		}
		panes[strings.TrimPrefix(tty, "/dev/")] = target
	}
	return panes
}

// reach puts the client on a pane.
func (s *server) reach(target string) error {
	if _, err := s.run("select-window", "-t", target); err != nil {
		return err
	}
	_, err := s.run("select-pane", "-t", target)
	return err
}

// open opens a shell in a new window at a directory and puts the client
// on it.
func (s *server) open(dir string) error {
	_, err := s.run("new-window", "-c", dir)
	return err
}

// detach lets the client go; the server keeps on.
func (s *server) detach() error {
	_, err := s.run("detach-client")
	return err
}

// tmuxConf is the server's configuration: the prefix, the look, and the
// keys that come back to the watch. The look is the console's: every
// pane on the ground, in the ink, with the sixteen colors a program asks
// for by name drawn from the same palette.
func tmuxConf() string {
	return `# conn's tmux server. Written by conn on each start; edits do not keep.
set -g prefix C-Space
unbind C-b
set -g status off
set -g mouse on
set -g history-limit 10000
set -g window-size latest
set -g set-clipboard on
set -g mode-keys vi
set -g set-titles on
set -g set-titles-string "conn"
set -g escape-time 10
set -g focus-events on
set -g default-terminal tmux-256color
set -as terminal-features ",*:RGB"
set -g allow-passthrough on
set -g display-time 3000
set -g window-style "bg=#15130F,fg=#E6DFD0"
set -g pane-colours[0] "#100E0B"
set -g pane-colours[1] "#FF7847"
set -g pane-colours[2] "#93C98B"
set -g pane-colours[3] "#E3A94F"
set -g pane-colours[4] "#7FC7BD"
set -g pane-colours[5] "#E85D2F"
set -g pane-colours[6] "#7FC7BD"
set -g pane-colours[7] "#E6DFD0"
set -g pane-colours[8] "#8B8272"
set -g pane-colours[9] "#FF7847"
set -g pane-colours[10] "#93C98B"
set -g pane-colours[11] "#E3A94F"
set -g pane-colours[12] "#7FC7BD"
set -g pane-colours[13] "#E85D2F"
set -g pane-colours[14] "#7FC7BD"
set -g pane-colours[15] "#E6DFD0"
set -g pane-border-style "fg=#15130F,bg=#15130F"
set -g pane-active-border-style "fg=#15130F,bg=#15130F"
set -g pane-border-indicators off
# The keys under the prefix: w back to the watch, C-Space to where you were.
bind w select-window -t :=watch
bind C-Space last-window
`
}

// shellQuote quotes a path for a tmux command line.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
