package main

import (
	"fmt"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// conn holds a tmux server of its own. The first conn brings it up with
// one window, home, running conn, and attaches; a later conn attaches
// to what is there. Home is a rail on the left, which is the watch, and
// a slot on the right, which is the process reached from it. Work lives
// in the server as windows of its own, out of sight; reaching a process
// swaps its pane into the slot and the slot's last pane back out to
// where it came from, so a process stays when the client goes. q
// detaches; the server and everything in it keep on. The conn that
// attached stays behind the client, to give the terminal its own colors
// back when the client returns, which tmux does not.
//
// The socket is under the state directory, or where CONN_SOCKET says,
// which is how a test brings up a server of its own.

const (
	sessionName = "conn"
	homeWindow  = "home"
	railWidth   = 48 // the rail's columns; the slot has the rest
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
	// A server that is up but has lost its home window gets one back.
	if _, err := s.run("has-session", "-t", "="+sessionName); err == nil {
		if !s.hasHome() {
			if _, err := s.run("new-window", "-d", "-t", sessionName+":", "-n", homeWindow, "-c", home, "exec "+shellQuote(self)); err != nil {
				return 0, err
			}
		}
	}
	cmd := exec.Command(s.tmux, "-S", s.socket, "-f", conf,
		"new-session", "-A", "-s", sessionName, "-n", homeWindow, "-c", home, "exec "+shellQuote(self))
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	// A tmux inside another tmux refuses to attach while TMUX is set; the
	// terminal is this conn's to give.
	cmd.Env = withoutTmux(os.Environ())
	// The terminal takes the ground and the ink for its own before the
	// client has it, so the padding around the client is the ground too.
	// The conn in the pane asks the same of tmux, which keeps it to the
	// pane; the terminal outside hears it from here.
	fmt.Print(oscColors())
	err := cmd.Run()
	// The client is gone and the terminal is ours again: the colors conn
	// asked it to take go back to its own.
	fmt.Print(oscOwnColors)
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode(), nil
	}
	if err != nil {
		return 0, err
	}
	return 0, nil
}

// oscColors asks the terminal to take conn's ink and ground for its
// own, and oscOwnColors gives it its own back. tmux keeps what the conn
// in a pane asks for to the pane, so the terminal outside hears it from
// the conn that attached, which is also there to take it back.
func oscColors() string {
	return fmt.Sprintf("\x1b]10;%s\x1b\\\x1b]11;%s\x1b\\", hex(inkColor), hex(groundColor))
}

const oscOwnColors = "\x1b]110\x1b\\\x1b]111\x1b\\"

// hex is a color as a terminal wants it written.
func hex(c color.RGBA) string {
	return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
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

// hasHome says whether the session has its home window.
func (s *server) hasHome() bool {
	out, err := s.run("list-windows", "-t", sessionName, "-F", "#{window_name}")
	if err != nil {
		return false
	}
	for _, name := range strings.Split(out, "\n") {
		if name == homeWindow {
			return true
		}
	}
	return false
}

// A pane of the server: its id, which holds through swaps; the terminal
// it holds; its size; and whether it is a hold.
type pane struct {
	id, tty       string
	width, height int
	hold          bool
}

const paneFormat = "#{pane_id}\t#{pane_tty}\t#{pane_width}\t#{pane_height}\t#{@conn_hold}"

// panes is every pane in the server, by the terminal it holds.
func (s *server) panes() (map[string]pane, error) {
	out, err := s.run("list-panes", "-a", "-F", paneFormat)
	if err != nil {
		return nil, err
	}
	return parsePanes(out), nil
}

// parsePanes reads list-panes in paneFormat, with each terminal's /dev/
// dropped to match how a process names its own.
func parsePanes(out string) map[string]pane {
	panes := map[string]pane{}
	for _, l := range strings.Split(out, "\n") {
		f := strings.Split(l, "\t")
		if len(f) != 5 || f[0] == "" {
			continue
		}
		p := pane{id: f[0], tty: strings.TrimPrefix(f[1], "/dev/"), hold: f[4] == "1"}
		p.width, _ = strconv.Atoi(f[2])
		p.height, _ = strconv.Atoi(f[3])
		panes[p.tty] = p
	}
	return panes
}

// The rail is the pane this conn runs in; tmux names it in TMUX_PANE.
func (s *server) rail() string {
	return os.Getenv("TMUX_PANE")
}

// slot is the pane beside the rail in the home window, when there is
// one.
func (s *server) slot() (pane, bool, error) {
	out, err := s.run("list-panes", "-t", s.rail(), "-F", paneFormat)
	if err != nil {
		return pane{}, false, err
	}
	for _, p := range parsePanes(out) {
		if p.id != s.rail() {
			return p, true, nil
		}
	}
	return pane{}, false, nil
}

// splitSlot opens the slot beside the rail, with a hold in it, and sets
// the rail to its width. Focus stays on the rail.
func (s *server) splitSlot(home, self string) error {
	id, err := s.run("split-window", "-h", "-d", "-P", "-F", "#{pane_id}", "-t", s.rail(), "-c", home, "exec "+shellQuote(self)+" hold")
	if err != nil {
		return err
	}
	if _, err := s.run("set-option", "-p", "-t", strings.TrimSpace(id), "@conn_hold", "1"); err != nil {
		return err
	}
	_, err = s.run("resize-pane", "-t", s.rail(), "-x", strconv.Itoa(railWidth))
	return err
}

// show puts a pane in the slot and focus on it. The pane that was in the
// slot goes back to where this one came from, and its window takes the
// slot's size so it keeps its shape; a hold that leaves the slot is
// done with.
func (s *server) show(target pane) error {
	slot, ok, err := s.slot()
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("home has no slot")
	}
	if target.id != slot.id {
		args := []string{"swap-pane", "-d", "-s", target.id, "-t", slot.id}
		if slot.width > 0 && slot.height > 0 {
			args = append(args, ";", "resize-window", "-t", slot.id, "-x", strconv.Itoa(slot.width), "-y", strconv.Itoa(slot.height))
		}
		if slot.hold {
			args = append(args, ";", "kill-pane", "-t", slot.id)
		}
		if _, err := s.run(args...); err != nil {
			return err
		}
	}
	_, err = s.run("select-pane", "-t", target.id)
	return err
}

// open opens a shell at a directory, in a window of its own, and shows
// it in the slot.
func (s *server) open(dir string) error {
	id, err := s.run("new-window", "-d", "-P", "-F", "#{pane_id}", "-c", dir)
	if err != nil {
		return err
	}
	return s.show(pane{id: strings.TrimSpace(id)})
}

// focusRail puts focus on the rail.
func (s *server) focusRail() error {
	_, err := s.run("select-pane", "-t", s.rail())
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
set-environment -g COLORTERM truecolor
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
# The keys under the prefix: w to the watch, C-Space to the other pane.
bind w select-pane -L
bind C-Space last-pane
`
}

// shellQuote quotes a path for a tmux command line.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
