package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	term "github.com/charmbracelet/x/term"
	"github.com/w0zro/conn/internal/shell"
	"github.com/w0zro/conn/internal/tmux"
)

// conn holds a tmux server of its own, with one session. Its first
// window is conn; every process conn starts is a window beside it, and
// ctrl-space comes back to conn from any of them. tmux is left as it
// comes otherwise, prefix and all.
const (
	sessionName = "conn"
	connWindow  = "conn"
	backKey     = "C-Space"
	// The mark on a pane conn started a conversation in: its id.
	sessionMark = "@conn_session"
)

// socketPath is where conn's server listens: CONN_SOCKET, or tmux.sock
// in conn's state directory.
func socketPath(home string) string {
	if p := os.Getenv("CONN_SOCKET"); p != "" {
		return p
	}
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		state = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(state, "conn", "tmux.sock")
}

// findServer is conn's server on this machine's tmux.
func findServer(home string) (*tmux.Server, error) {
	bin, err := exec.LookPath("tmux")
	if err != nil {
		return nil, fmt.Errorf("conn runs in tmux, and there is no tmux on the PATH")
	}
	return &tmux.Server{Tmux: bin, Socket: socketPath(home)}, nil
}

// insideConn says whether this process runs in a pane of conn's server:
// tmux tells its panes the socket in TMUX, before the first comma.
func insideConn(socket string) bool {
	sock, _, _ := strings.Cut(os.Getenv("TMUX"), ",")
	return sock != "" && filepath.Clean(sock) == filepath.Clean(socket)
}

// generation names this conn's arrangement of the server. A server
// arranged by an earlier conn is not this one's to rearrange.
const generation = "2"

// conf is the server's configuration, written beside the socket on
// each start and sourced into a server already up. The key back to
// conn is bound once conn's window is known, by its id: a window's name
// is whatever runs in it, unless it was given one.
func conf() string {
	return `# conn's tmux server. Written by conn on each start; edits do not keep.
set -g @conn_generation ` + generation + `
set -g prefix C-b
set -g mouse on
set -g mode-keys vi
set -g history-limit 10000
set -g escape-time 10
set -g focus-events on
set -g set-clipboard on
set -g default-terminal tmux-256color
set -as terminal-features ",*:RGB"
set-environment -g COLORTERM truecolor
set-environment -g CLAUDE_CODE_TMUX_TRUECOLOR 1
set-environment -g CONN 1
set -g status-left "#[bold] conn #[nobold] "
set -g status-left-length 20
set -g status-right " ctrl-space: back to conn "
set -g status-right-length 40
`
}

// errEarlier is a server up under an earlier conn's arrangement.
var errEarlier = fmt.Errorf("an earlier conn's server is up")

// attach brings the server up if it is down, with conn in its first
// window, and puts this terminal on it until the client leaves. A
// server already up gets the configuration again, and conn's window
// again where it is gone or runs another build.
func attach(srv *tmux.Server, self, home string) (int, error) {
	if err := os.MkdirAll(filepath.Dir(srv.Socket), 0o700); err != nil {
		return 0, err
	}
	path := filepath.Join(filepath.Dir(srv.Socket), "tmux.conf")
	if err := os.WriteFile(path, []byte(conf()), 0o600); err != nil {
		return 0, err
	}
	panel := "exec " + shell.Quote(self) + " panel"
	if srv.Up() {
		if g, _ := srv.Global("@conn_generation"); g != generation {
			return 0, errEarlier
		}
		if _, err := srv.Do(tmux.SourceFile(path)); err != nil {
			return 0, err
		}
	}
	if !srv.HasSession(sessionName) {
		// Made at this terminal's size: made at tmux's own 80 by 24, the
		// panel can start before the client's size reaches it and miss
		// the change.
		args := []string{"-f", path, "new-session", "-d", "-s", sessionName, "-n", connWindow, "-c", home}
		if w, h, err := term.GetSize(os.Stdout.Fd()); err == nil {
			args = append(args, "-x", strconv.Itoa(w), "-y", strconv.Itoa(h))
		}
		if _, err := srv.Run(append(args, panel)...); err != nil {
			return 0, err
		}
	}
	win, err := ensurePanel(srv, home, self, panel)
	if err != nil {
		return 0, err
	}
	if _, err := srv.Do(
		tmux.Cmd{"bind", "-n", backKey, "select-window", "-t", win},
		tmux.Cmd{"select-window", "-t", win},
	); err != nil {
		return 0, err
	}
	return srv.AttachClient(path, sessionName, connWindow, home, panel)
}

// ensurePanel answers conn's window: conn's pane started again where
// it has ended or runs another build, and made where there is none.
func ensurePanel(srv *tmux.Server, home, self, panel string) (string, error) {
	own, err := findPanel(srv, self)
	if err != nil {
		return "", err
	}
	if own.pane == "" {
		out, err := srv.Run("new-window", "-d", "-t", sessionName+":", "-n", connWindow, "-c", home, "-P", "-F", "#{window_id} #{pane_id}", panel)
		if err != nil {
			return "", err
		}
		own.window, own.pane, _ = strings.Cut(strings.TrimSpace(out), " ")
	} else if own.dead || !own.ours {
		if _, err := srv.Do(tmux.Respawn(own.pane, panel)); err != nil {
			return "", err
		}
	}
	_, err = srv.Do(tmux.SetPane(own.pane, panelMark, "1"))
	return own.window, err
}

// panelMark is on conn's own pane. A window's name will not do to find
// it by: a window conn starts is named for its project, and a project
// can be called conn.
const panelMark = "@conn_panel"

// A panelPane is where conn's pane is, whether it has ended, and
// whether it runs this build.
type panelPane struct {
	window, pane string
	dead, ours   bool
}

// findPanel is conn's pane: the one marked as conn's, or, before it is
// marked, the one this build was started in. self is this build, or
// blank where any build's pane will do.
func findPanel(srv *tmux.Server, self string) (panelPane, error) {
	out, err := srv.Run("list-panes", "-s", "-t", sessionName, "-F", "#{window_id} #{pane_id} #{pane_dead} #{"+panelMark+"} #{pane_start_command}")
	if err != nil {
		return panelPane{}, err
	}
	for _, l := range strings.Split(out, "\n") {
		f := strings.SplitN(l, " ", 5)
		if len(f) < 5 {
			continue
		}
		ours := self != "" && strings.Contains(f[4], shell.Quote(self)+" panel")
		if f[3] == "1" || ours {
			return panelPane{window: f[0], pane: f[1], dead: f[2] == "1", ours: ours}, nil
		}
	}
	return panelPane{}, nil
}

// endServer ends the server and everything running in it.
func endServer(srv *tmux.Server) error { return srv.Down() }

// A pane in conn's server, as the panel reads it.
type pane struct {
	ID      string
	Window  string // the window's id
	Dead    bool
	TTY     string // without /dev/
	Session string // the conversation conn started in it
	Command string // what tmux says runs in it now
	Dir     string
}

// The fields are separated by a space: tmux turns a tab into an
// underscore when it prints to a pipe in the C locale. Every field but
// the last is one token; the directory, which can hold a space, is last
// and taken whole.
const paneFormat = "#{pane_id} #{window_id} #{pane_dead} #{pane_tty} #{" + sessionMark + "} #{pane_current_command} #{pane_current_path}"

// panes is every pane in the server.
func panes(srv *tmux.Server) ([]pane, error) {
	out, err := srv.Run("list-panes", "-s", "-t", sessionName, "-F", paneFormat)
	if err != nil {
		return nil, err
	}
	return parsePanes(out), nil
}

func parsePanes(out string) []pane {
	var ps []pane
	for _, l := range strings.Split(out, "\n") {
		f := strings.SplitN(l, " ", 7)
		if len(f) < 7 {
			continue
		}
		ps = append(ps, pane{
			ID: f[0], Window: f[1], Dead: f[2] == "1",
			TTY: strings.TrimPrefix(f[3], "/dev/"), Session: f[4], Command: f[5], Dir: f[6],
		})
	}
	return ps
}

// openWindow starts a command in a window of its own at a directory,
// marks it with what it holds, and puts the client on it.
func openWindow(srv *tmux.Server, name, dir, cmd, session string) error {
	id, err := srv.Run("new-window", "-t", sessionName+":", "-n", name, "-c", dir, "-P", "-F", "#{pane_id}", cmd)
	if err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	if session != "" {
		if _, err := srv.Do(tmux.SetPane(id, sessionMark, session)); err != nil {
			return err
		}
	}
	return nil // new-window without -d has put the client on it
}

// goTo puts the client on a pane's window, and the keys in the pane.
func goTo(srv *tmux.Server, p pane) error {
	_, err := srv.Do(tmux.Cmd{"select-window", "-t", p.Window}, tmux.SelectPane(p.ID))
	return err
}

// resumeCommand picks a conversation back up in Claude Code.
func resumeCommand(id string) string {
	return "exec claude --resume " + shell.Quote(id)
}

// windowName is a short name for a window, from a project's name.
func windowName(name string) string {
	if len(name) > 20 {
		name = name[:20]
	}
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == ':' || r == '.' {
			return '-'
		}
		return r
	}, name)
}
