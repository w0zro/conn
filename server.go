package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
	// The number of conversations waiting on you, for the status line.
	waitingMark = "@conn_waiting"
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
# The panel lists what runs; tmux's own list of windows would say it
# again, by window.
set -g window-status-format ""
set -g window-status-current-format ""
set -g window-status-separator ""
set -g status-left-length 20
set -g status-right "#{?#{` + waitingMark + `},#[fg=colour` + waitingColor + `#,bold]#{` + waitingMark + `} waiting#[default] · ctrl-space then tab goes to it · , }ctrl-space: back to conn "
set -g status-right-length 80
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
		// Made at the size it will have, this terminal less the status
		// line: made at any other, the panel can start before the
		// client's size reaches it and miss the change.
		args := []string{"-f", path, "new-session", "-d", "-s", sessionName, "-n", connWindow, "-c", home}
		if w, h, err := term.GetSize(os.Stdout.Fd()); err == nil && h > 1 {
			args = append(args, "-x", strconv.Itoa(w), "-y", strconv.Itoa(h-1))
		}
		if _, err := srv.Run(append(args, panel)...); err != nil {
			return 0, err
		}
		// The pane just made runs this build; said now, it is not
		// started again over a build it already runs.
		if _, err := srv.Do(tmux.SetGlobal(buildMark, fingerprint(self))); err != nil {
			return 0, err
		}
	}
	win, err := ensurePanel(srv, home, self, panel)
	if err != nil {
		return 0, err
	}
	own, err := findPanel(srv, "")
	if err != nil {
		return 0, err
	}
	if _, err := srv.Do(bindBack(win, own.pane), tmux.Cmd{"select-window", "-t", win}, tmux.SelectPane(own.pane)); err != nil {
		return 0, err
	}
	return srv.AttachClient(path, sessionName, connWindow, home, panel)
}

// bindBack binds the one key conn takes from every window: the keys
// back to the panel, in conn's window.
func bindBack(window, panel string) tmux.Cmd {
	return tmux.Cmd{"bind", "-n", backKey, "select-window", "-t", window, `\;`, "select-pane", "-t", panel}
}

// ensurePanel answers conn's window: conn's pane started again where
// it has ended or runs another build, and made where there is none.
func ensurePanel(srv *tmux.Server, home, self, panel string) (string, error) {
	own, err := findPanel(srv, self)
	if err != nil {
		return "", err
	}
	// A rebuild keeps its path, so the build is told by its contents.
	build := fingerprint(self)
	if was, _ := srv.Global(buildMark); was != build {
		own.ours = false
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
	_, err = srv.Do(tmux.SetPane(own.pane, panelMark, "1"), tmux.SetGlobal(buildMark, build))
	return own.window, err
}

// buildMark is the build conn's pane runs, by fingerprint.
const buildMark = "@conn_build"

// fingerprint names a binary by its contents.
func fingerprint(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
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
	PID     int    // what the pane runs, as started
	TTY     string // without /dev/
	Panel   bool   // conn's own
	Hold    bool   // conn's blank, standing in the workspace
	Session string // the conversation conn started in it
	Name    string // the name conn u started it under
	Exit    string // how what conn u started ended, once it has
	Command string // what tmux says runs in it now
	Dir     string
}

// The marks conn sets on the panes conn u starts: the name, the
// command and the directory it was started with, and how it ended,
// which the pane sets itself.
const (
	nameMark = "@conn_name"
	runMark  = "@conn_run"
	dirMark  = "@conn_dir"
	exitMark = "@conn_exit"
)

// The fields are separated by a space: tmux turns a tab into an
// underscore when it prints to a pipe in the C locale. Every field but
// the last is one token — a name conn u takes has no space in it — and
// the directory, which can hold a space, is last and taken whole.
const paneFormat = "#{pane_id} #{window_id} #{pane_dead} #{pane_pid} #{pane_tty} #{" + panelMark + "} #{" + holdMark + "} #{" + sessionMark + "} #{" + nameMark + "} #{" + exitMark + "} #{pane_current_command} #{pane_current_path}"

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
		f := strings.SplitN(l, " ", 12)
		if len(f) < 12 {
			continue
		}
		pid, _ := strconv.Atoi(f[3])
		ps = append(ps, pane{
			ID: f[0], Window: f[1], Dead: f[2] == "1", PID: pid,
			TTY: strings.TrimPrefix(f[4], "/dev/"), Panel: f[5] == "1", Hold: f[6] == "1", Session: f[7],
			Name: f[8], Exit: f[9], Command: f[10], Dir: f[11],
		})
	}
	return ps
}

// openWindow starts a command, or a shell where there is none, in a
// window of its own at a directory, out of sight, marks it with the
// conversation it holds where it holds one, and answers its pane.
func openWindow(srv *tmux.Server, name, dir, cmd, session string) (string, error) {
	args := []string{"new-window", "-d", "-t", sessionName + ":", "-n", name, "-c", dir, "-P", "-F", "#{pane_id}"}
	if cmd != "" {
		args = append(args, cmd) // with none, tmux runs the shell
	}
	id, err := srv.Run(args...)
	if err != nil {
		return "", err
	}
	id = strings.TrimSpace(id)
	if session != "" {
		if _, err := srv.Do(tmux.SetPane(id, sessionMark, session)); err != nil {
			return "", err
		}
	}
	return id, nil
}

// resumeCommand picks a conversation back up in Claude Code, told what
// conn tells the agents it starts.
func resumeCommand(note, id string) string {
	return "exec claude --append-system-prompt " + shell.Quote(note) + " --resume " + shell.Quote(id)
}

// newCommand starts a conversation in Claude Code under an id conn
// chose, so its pane is marked with it before its transcript exists,
// and with a first prompt where there is one.
func newCommand(note, id, prompt string) string {
	cmd := "exec claude --append-system-prompt " + shell.Quote(note) + " --session-id " + shell.Quote(id)
	if prompt != "" {
		cmd += " " + shell.Quote(prompt)
	}
	return cmd
}

// told is what a conversation started in a project is told: the CLI,
// and what the project's stack and checks are here, as last run
// through conn by whoever ran them. in says which roots are the
// project's.
func told(conn, socket string, in func(string) bool) string {
	starts := readStarted(startedPath(socket))
	return agentNote(conn, established(starts, in, roleStack), established(starts, in, roleCheck))
}

// agentNote is what conn tells the agents it starts: to work the
// project's processes through conn, and what the project runs here. It
// travels with conn rather than being written into anybody's settings.
// conn is how to run this build.
func agentNote(conn string, stack, checks []stacked) string {
	called := ""
	if conn != "conn" {
		called = "conn here is " + conn + "; run it by that path.\n\n"
	}
	here := ""
	if len(stack) > 0 || len(checks) > 0 {
		here = "\n\nIn this project, as last run through conn:\n"
		for _, s := range checks {
			here += "  conn u " + s.name + " -w -- " + s.command + "\n"
		}
		for _, s := range stack {
			here += "  conn u " + s.name + " -- " + s.command + "\n"
		}
		here = strings.TrimRight(here, "\n")
	}
	return "You are running inside conn, which holds this terminal as a tmux pane " +
		"and watches the processes working this project. Work the project's processes " +
		"through conn rather than starting them in the background or finding them with " +
		"ps, lsof and tmux:\n\n" +
		"  conn ls                          this project's processes, and the names they answer to\n" +
		"  conn u NAME -- COMMAND           start a dev server, a watcher, anything long-lived, in a pane of its own\n" +
		"  conn u NAME -w -- COMMAND        run a check and wait: its output, and its exit status as conn's\n" +
		"  conn l NAME|:PORT|PID [-n N]     what a process said\n" +
		"  conn r NAME|:PORT|PID            restart one conn started\n" +
		"  conn x NAME|:PORT|PID            end one\n\n" + called +
		"A process started this way holds a terminal of its own: conn lists it, the operator " +
		"can go into it, and its output is the pane's. Something you background instead holds " +
		"no terminal and has no pane, and can only be read through whatever file its output was sent to. " +
		"conn also keeps what you started as what this project runs: what waited (-w) as its checks, " +
		"the rest as its stack, each under its name, and the operator's keys run them again as you ran them." + here
}

// connCommand is how an agent runs this build: conn, where the PATH
// finds this build by that name, and its whole path otherwise. Setting
// the PATH in tmux's environment would not reach a shell, whose startup
// files build the PATH again.
func connCommand(self string) string {
	if found, err := exec.LookPath("conn"); err == nil && realDir(found) == realDir(self) {
		return "conn"
	}
	return self
}

// newID is a random UUID, the shape Claude Code takes for a session id.
func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// leave lets the terminal go that conn was last used from, and keeps
// everything in conn running. It is named: tmux's current client can
// be conn's own control-mode client, which watches the server.
func leave(srv *tmux.Server) error {
	cs := terminals(srv)
	if len(cs) == 0 {
		return srv.Detach()
	}
	_, err := srv.Do(tmux.Cmd{"detach-client", "-t", cs[0]})
	return err
}

// terminals is the clients on a terminal, not in control mode, the
// last used first.
func terminals(srv *tmux.Server) []string {
	out, err := srv.Run("list-clients", "-F", "#{client_activity} #{client_control_mode} #{client_name}")
	if err != nil {
		return nil
	}
	return parseTerminals(out)
}

// leftWatching detaches the control-mode clients on conn's session.
// Only conn's own pane makes one, to watch the server, and tmux keeps a
// control-mode client on after the process that made it has gone: a
// pane started again, on a new build, leaves its old one behind.
func leftWatching(srv *tmux.Server) {
	out, err := srv.Run("list-clients", "-t", sessionName, "-F", "#{client_control_mode} #{client_name}")
	if err != nil {
		return
	}
	for _, l := range strings.Split(out, "\n") {
		if name, ok := strings.CutPrefix(l, "1 "); ok {
			_, _ = srv.Do(tmux.Cmd{"detach-client", "-t", name})
		}
	}
}

func parseTerminals(out string) []string {
	type client struct {
		at   int
		name string
	}
	var cs []client
	for _, l := range strings.Split(out, "\n") {
		f := strings.SplitN(l, " ", 3)
		if len(f) < 3 || f[1] == "1" {
			continue
		}
		at, _ := strconv.Atoi(f[0])
		cs = append(cs, client{at, f[2]})
	}
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].at > cs[j].at })
	names := make([]string, len(cs))
	for i, c := range cs {
		names[i] = c.name
	}
	return names
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
