package main

import (
	"fmt"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

// conn holds a tmux server of its own. The first conn brings it up with
// one window, home, running conn, and attaches; a later conn attaches
// to what is there. Home is a panel on the left, which is the processes
// view, and a bay on the right, which is the process reached from it.
// Work lives in the server as windows of its own, out of sight;
// reaching a process swaps its pane into the bay and the bay's last
// pane back out to where it came from, so a process stays when the
// client goes. q detaches; the server and everything in it keep on. The
// conn that attached stays behind the client, to give the terminal its
// own colors back when the client returns, which tmux does not.
//
// The socket is under the state directory, or where CONN_SOCKET says,
// which is how a test brings up a server of its own.

const (
	sessionName   = "conn"
	homeWindow    = "home"
	panelWidth    = 44 // the panel's columns; the bay has the rest
	defaultPrefix = "C-Space"
)

// prefix is the key conn's chords come under: ctrl+space, or what
// CONN_PREFIX says, in tmux's spelling of a key.
func prefix() string {
	if p := os.Getenv("CONN_PREFIX"); p != "" {
		return p
	}
	return defaultPrefix
}

// prefixLabel is the prefix as a legend writes it: C-SPACE, C-A.
func prefixLabel(p string) string {
	return strings.ToUpper(p)
}

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

// attach brings the server up if it is down, with the processes view
// window running conn, and puts this terminal on it until the client
// detaches or the server ends. It answers how the client exited; an
// error is one of its own, before the client had the terminal.
//
// The server's ground is what a mode file beside the socket says, or
// the terminal's own the first time a server rises, written down so it
// holds across attaches. --light or --dark says it instead, and says it
// whenever it is given: a server already up is put on the other ground
// where it stands, rather than keeping what it rose on until conn down.
// tmux does not re-read -f on an attach, so that takes sourcing the
// configuration again; see reground.
func (s *server) attach(self, home string, override *bool) (int, error) {
	if err := os.MkdirAll(filepath.Dir(s.socket), 0o700); err != nil {
		return 0, err
	}
	dark, ok := readModeFile(s.socket)
	asked := false
	switch {
	case !ok:
		dark = askDark(override)
		_ = writeMode(s.socket, dark)
	case override != nil && *override != dark:
		dark = *override
		_ = writeMode(s.socket, dark)
		asked = true
	}
	applyMode(dark)
	refreshClaudeTheme(home)
	conf := filepath.Join(filepath.Dir(s.socket), "tmux.conf")
	if err := os.WriteFile(conf, []byte(tmuxConf(prefix())), 0o600); err != nil {
		return 0, err
	}
	if asked {
		if err := s.reground(conf); err != nil {
			return 0, err
		}
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

// reground puts a server already up onto the ground the mode file now
// says. Sourcing the configuration again is what tmux has instead of
// re-reading -f: every set -g in it lands on the live server, so each
// pane takes the new sixteen and the new ground without going down.
//
// The panes conn draws itself are the exception. They are conn, and
// conn reads the mode once, when it starts; the palette each is
// painting from is the one it rose on. So they are started again — the
// panel, and a hold if one is standing in the bay — and read the mode
// afresh. A pane with work in it draws in its own colors and keeps
// them; what it asks for by name it now gets from the new sixteen.
func (s *server) reground(conf string) error {
	if !s.up() {
		return nil
	}
	if _, err := s.run("source-file", conf); err != nil {
		return err
	}
	panes, err := s.panes()
	if err != nil {
		return err
	}
	for _, p := range panes {
		if p.hold {
			if _, err := s.run("respawn-pane", "-k", "-t", p.id); err != nil {
				return err
			}
		}
	}
	_, err = s.run("respawn-pane", "-k", "-t", sessionName+":"+homeWindow+".0")
	return err
}

// oscColors asks the terminal to take conn's ink and ground for its
// own, and oscOwnColors gives it its own back. tmux keeps what the conn
// in a pane asks for to the pane, so the terminal outside hears it from
// the conn that attached, which is also there to take it back. The
// cursor is the other way about: tmux does put the server's on the
// terminal, and leaves it there when the client goes, so conn asks for
// nothing and takes it back all the same.
func oscColors() string {
	return fmt.Sprintf("\x1b]10;%s\x1b\\\x1b]11;%s\x1b\\", hex(inkColor), hex(groundColor))
}

const oscOwnColors = "\x1b]110\x1b\\\x1b]111\x1b\\\x1b]112\x1b\\"

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
// it holds; its size; whether it is conn's own furniture and whether
// that furniture is a readout; and whether remain-on-exit is the only
// thing keeping it up, its process already gone.
//
// A readout is furniture too — everything true of a hold is true of it,
// so it carries the hold's own mark and everything that acts on holds
// acts on it — but i has to tell the two apart to know whether it is
// opening a page or closing one, and a mark of its own is how.
type pane struct {
	id, tty       string
	width, height int
	hold          bool
	readout       bool
	dead          bool
}

const paneFormat = "#{pane_id}\t#{pane_tty}\t#{pane_width}\t#{pane_height}\t#{@conn_hold}\t#{pane_dead}\t#{@conn_readout}"

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
		if len(f) != 7 || f[0] == "" {
			continue
		}
		p := pane{id: f[0], tty: strings.TrimPrefix(f[1], "/dev/"),
			hold: f[4] == "1", dead: f[5] == "1", readout: f[6] == "1"}
		p.width, _ = strconv.Atoi(f[2])
		p.height, _ = strconv.Atoi(f[3])
		panes[p.tty] = p
	}
	return panes
}

// The panel is the pane this conn runs in; tmux names it in TMUX_PANE.
func (s *server) panel() string {
	return os.Getenv("TMUX_PANE")
}

// bay is the pane beside the panel in the home window, when there is
// one.
func (s *server) bay() (pane, bool, error) {
	out, err := s.run("list-panes", "-t", s.panel(), "-F", paneFormat)
	if err != nil {
		return pane{}, false, err
	}
	for _, p := range parsePanes(out) {
		if p.id != s.panel() {
			return p, true, nil
		}
	}
	return pane{}, false, nil
}

// splitBay opens the bay beside the panel, with a hold in it, and sets
// the panel to its width. Focus stays on the panel.
func (s *server) splitBay(home, self string) error {
	id, err := s.run("split-window", "-h", "-d", "-P", "-F", "#{pane_id}", "-t", s.panel(), "-c", home, "exec "+shellQuote(self)+" hold")
	if err != nil {
		return err
	}
	if _, err := s.run("set-option", "-p", "-t", strings.TrimSpace(id), "@conn_hold", "1"); err != nil {
		return err
	}
	// A pane in this window whose process ends stays instead of
	// closing, so a killed shell does not collapse the window to the
	// panel alone before conn re-splits it: the bay holds its project,
	// dead, until conn puts a hold there. It is the window's option
	// and not the server's, since the bay is the only place conn has a
	// layout to protect. Everywhere else a window whose work has ended
	// is a window that is done.
	if _, err := s.run("set-option", "-w", "-t", strings.TrimSpace(id), "remain-on-exit", "on"); err != nil {
		return err
	}
	return s.holdPanel()
}

// holdPanel sets the panel to its width. tmux keeps the panes in
// proportion when the window is resized, so the panel is put back each
// time it is not its width.
func (s *server) holdPanel() error {
	_, err := s.run("resize-pane", "-t", s.panel(), "-x", strconv.Itoa(panelWidth))
	return err
}

// reviveBay puts a hold in a bay whose pane has died: remain-on-exit
// kept it there, its process gone, so this is a swap into the bay's
// own shape rather than a split — nothing about the window's layout
// moves. Without a bay at all, which a swap has nothing to land in,
// it falls back to splitBay.
func (s *server) reviveBay(home, self string) error {
	bay, ok, err := s.bay()
	if err != nil {
		return err
	}
	if !ok {
		return s.splitBay(home, self)
	}
	return s.holdBay(home, self, bay)
}

// hideReadout puts the bay back to a hold, which is what closing the
// readout leaves behind: the bay is conn's, and an empty one says so.
func (s *server) hideReadout(home, self string) error {
	bay, ok, err := s.bay()
	if err != nil || !ok {
		return err
	}
	return s.holdBay(home, self, bay)
}

// holdBay puts a hold in the bay, in the bay's own shape, and is rid
// of whatever was there. It is a swap rather than a split so nothing
// about the window's layout moves, and the panel never has to give up
// its width and take it back.
func (s *server) holdBay(home, self string, bay pane) error {
	id, err := s.run("new-window", "-d", "-P", "-F", "#{pane_id}", "-c", home, "exec "+shellQuote(self)+" hold")
	if err != nil {
		return err
	}
	hold := strings.TrimSpace(id)
	if _, err := s.run("set-option", "-p", "-t", hold, "@conn_hold", "1"); err != nil {
		return err
	}
	_, err = s.run("swap-pane", "-d", "-s", hold, "-t", bay.id, ";", "kill-pane", "-t", bay.id)
	return err
}

// showReadout puts the readout in the bay, and leaves focus on the
// panel. It is furniture rather than work — it runs nothing of yours,
// and reaching anything else is meant to be rid of it — so it is marked
// the way a hold is: killed when a real pane takes the bay, and
// respawned where it stands when the ground changes. Focus stays where
// it was because the readout is a reading, not a project to be.
//
// It is opened on no pid, which is the readout's word for "whatever the
// panel's cursor is on". One page then serves the whole list, j and k
// carrying it along, where a page opened per row would spawn a window a
// keystroke and blank the bay between each.
func (s *server) showReadout(home, self string) error {
	id, err := s.run("new-window", "-d", "-P", "-F", "#{pane_id}", "-c", home,
		"exec "+shellQuote(self)+" readout")
	if err != nil {
		return err
	}
	readout := strings.TrimSpace(id)
	if _, err := s.run("set-option", "-p", "-t", readout, "@conn_hold", "1"); err != nil {
		return err
	}
	if _, err := s.run("set-option", "-p", "-t", readout, "@conn_readout", "1"); err != nil {
		return err
	}
	bay, ok, err := s.bay()
	if err != nil {
		return err
	}
	if !ok {
		// Nothing to swap into. Split one first — swapping against the panel
		// instead would put the processes view in the window the readout came
		// from and the readout where the processes view belongs.
		if err := s.splitBay(home, self); err != nil {
			return err
		}
		if bay, ok, err = s.bay(); err != nil {
			return err
		} else if !ok {
			return fmt.Errorf("home has no bay")
		}
	}
	args := []string{"swap-pane", "-d", "-s", readout, "-t", bay.id}
	if bay.width > 0 && bay.height > 0 {
		args = append(args, ";", "resize-window", "-t", bay.id, "-x", strconv.Itoa(bay.width), "-y", strconv.Itoa(bay.height))
	}
	// What was in the bay goes back to a window of its own, still
	// running, unless it was conn's own furniture and has nothing to go
	// back to.
	if bay.hold {
		args = append(args, ";", "kill-pane", "-t", bay.id)
	}
	if _, err := s.run(args...); err != nil {
		return err
	}
	return s.focusPanel()
}

// show puts a pane in the bay and focus on it. The pane that was in the
// bay goes back to where this one came from, and its window takes the
// bay's size so it keeps its shape; a hold that leaves the bay is
// done with, and so is a pane whose process has ended, which
// remain-on-exit kept only so the bay would hold its project.
func (s *server) show(target pane) error {
	bay, ok, err := s.bay()
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("home has no bay")
	}
	if target.id != bay.id {
		args := []string{"swap-pane", "-d", "-s", target.id, "-t", bay.id}
		if bay.width > 0 && bay.height > 0 {
			args = append(args, ";", "resize-window", "-t", bay.id, "-x", strconv.Itoa(bay.width), "-y", strconv.Itoa(bay.height))
		}
		if bay.hold || bay.dead {
			args = append(args, ";", "kill-pane", "-t", bay.id)
		}
		if _, err := s.run(args...); err != nil {
			return err
		}
	}
	_, err = s.run("select-pane", "-t", target.id)
	return err
}

// A shell conn opened, as tmux answers when it makes the window: the
// pane it is in and the process in it. What tmux says the process is
// called at that instant is tmux itself, before the shell has taken
// over, so the name is left to the process table.
type shell struct {
	pane pane
	pid  int
}

// open opens a shell at a directory, in a window of its own, and shows
// it in the bay.
func (s *server) open(dir string) (shell, error) {
	return s.openCmd(dir, "")
}

// openCmd is open, running a command instead of the directory's own
// shell — what a opens claude with.
func (s *server) openCmd(dir, cmd string) (shell, error) {
	args := []string{"new-window", "-d", "-P", "-F", "#{pane_id}\t#{pane_pid}\t#{pane_tty}", "-c", dir}
	if cmd != "" {
		args = append(args, cmd)
	}
	out, err := s.run(args...)
	if err != nil {
		return shell{}, err
	}
	sh := parseOpened(out)
	return sh, s.show(sh.pane)
}

// parseOpened reads what new-window printed for the pane it made.
func parseOpened(out string) shell {
	f := strings.Split(strings.TrimSpace(out), "\t")
	for len(f) < 3 {
		f = append(f, "")
	}
	sh := shell{pane: pane{id: f[0], tty: strings.TrimPrefix(f[2], "/dev/")}}
	sh.pid, _ = strconv.Atoi(f[1])
	return sh
}

// wide gives the panel the whole window, which is what the console
// wants: it is a page, not a panel. narrow gives the bay its side back.
// tmux has one key for both, so each looks first at how the window
// stands; on a home with no bay yet there is nothing to zoom and both
// are nothing.
func (s *server) wide() error { return s.zoom(true) }

func (s *server) narrow() error { return s.zoom(false) }

func (s *server) zoom(on bool) error {
	out, err := s.run("display-message", "-p", "-t", s.panel(), "#{window_zoomed_flag}")
	if err != nil {
		return err
	}
	if (strings.TrimSpace(out) == "1") == on {
		return nil
	}
	_, err = s.run("resize-pane", "-Z", "-t", s.panel())
	return err
}

// focusPanel puts focus on the panel.
func (s *server) focusPanel() error {
	return s.focusPane(s.panel())
}

// focusPane puts the keys in a pane by its id. Where the pane is gone
// tmux says so and nothing moves, which is the right answer: the pane
// the keys came from can have ended while the list was up.
func (s *server) focusPane(id string) error {
	_, err := s.run("select-pane", "-t", id)
	return err
}

// detach lets the client go; the server keeps on.
func (s *server) detach() error {
	_, err := s.run("detach-client")
	return err
}

// The terminal's chrome, beyond the ground and the ink the console is
// drawn in: the orange for the cursor, and the console's border color,
// which draws the line between the panel and the bay and sits behind a
// selection. Dark until applyMode says otherwise; see mode.go.
var (
	cursorHex = darkCursorHex
	borderHex = darkScheme[0]
)

// The sixteen colors a program asks for by name, as conn draws them.
// Most are the console's own tokens: the two oranges for the reds, the
// parchment and the ink for the whites, the faint for bright black, the
// border for black. The blue and the magenta are conn's own, added so
// that the slots a shell theme leans on — structure, type, what can be
// run — stay apart from one another instead of collapsing into the
// orange and the teal. Normal, then bright. Dark until applyMode says
// otherwise; see mode.go for the light table and both grounds.
var scheme = darkScheme

// tmuxConf is the server's configuration: the prefix with its chords,
// and how every pane is drawn. tmux's own prefix table is emptied, so
// none of its keys or actions are reachable through conn; prefix then -
// puts focus in the processes view from wherever the keys are, and
// prefix then q detaches the way q does from the processes view itself,
// without first coming back to it. The drawing is the console's: every
// pane on the ground, in the ink, with the sixteen colors a program
// asks for by name drawn from conn's scheme, with CONN set in the
// server so a program that draws in its own hex can tell where it is
// and dress to match, the cursor in the orange and a selection on the
// border color, and between the panel and the bay a line in that color
// too, the same whichever side has focus.
func tmuxConf(prefix string) string {
	var b strings.Builder
	b.WriteString(`# conn's tmux server. Written by conn on each start; edits do not keep.
# Ten chords under the prefix: to the processes view, to the list, to the
# other process, to the one that has waited longest, down and up the ones
# that can be reached at all, to a shell, to a contact and to the sessions
# at the project the panel is looking at, and to detach; tmux's own are
# unbound.
set -g prefix ` + prefix + `
set -g prefix2 None
unbind -a -T prefix
bind - select-pane -t ` + sessionName + ":" + homeWindow + `.0
bind p set -gF @conn_from "#{pane_id}" \; select-pane -t ` + sessionName + ":" + homeWindow + `.0 \; send-keys -t ` + sessionName + ":" + homeWindow + `.0 M-p
bind ` + prefix + ` select-pane -t ` + sessionName + ":" + homeWindow + `.0 \; send-keys -t ` + sessionName + ":" + homeWindow + `.0 M-o
bind Tab select-pane -t ` + sessionName + ":" + homeWindow + `.0 \; send-keys -t ` + sessionName + ":" + homeWindow + `.0 M-Tab
bind j select-pane -t ` + sessionName + ":" + homeWindow + `.0 \; send-keys -t ` + sessionName + ":" + homeWindow + `.0 M-j
bind k select-pane -t ` + sessionName + ":" + homeWindow + `.0 \; send-keys -t ` + sessionName + ":" + homeWindow + `.0 M-k
bind s select-pane -t ` + sessionName + ":" + homeWindow + `.0 \; send-keys -t ` + sessionName + ":" + homeWindow + `.0 M-s
bind a select-pane -t ` + sessionName + ":" + homeWindow + `.0 \; send-keys -t ` + sessionName + ":" + homeWindow + `.0 C-a
bind M-a set -gF @conn_from "#{pane_id}" \; select-pane -t ` + sessionName + ":" + homeWindow + `.0 \; send-keys -t ` + sessionName + ":" + homeWindow + `.0 M-a
bind q detach-client
set -g mouse on
# The panel's width is conn's to hold; a drag of the border would only be
# put back.
unbind -n MouseDrag1Border
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
# TERM says tmux-256color, which is what tmux draws with; a program that
# reads it and stops there paints conn's scheme in the 256 palette, where
# the warm dark end of it does not exist. Claude Code is one, and takes
# this for an answer.
set-environment -g CLAUDE_CODE_TMUX_TRUECOLOR 1
# A program in a pane can tell it is in conn, and dress accordingly.
set-environment -g CONN 1
# The terminal a client is attached from announces itself in
# TERM_PROGRAM, which tmux then sets to its own name inside a pane. The
# server is told to keep the client's own answer current as clients
# come and go, so the console can say what is actually drawing the
# screen rather than saying tmux twice. The list is added to rather
# than set, since what is in it already is tmux's own business.
set -ga update-environment " TERM_PROGRAM TERM_PROGRAM_VERSION"
set -g allow-passthrough on
set -g display-time 3000
# remain-on-exit is not set here. It is for the bay, and it is set on
# the home window, where the bay is, when conn opens it. Set for the
# whole server it also kept every window of work standing after its
# work ended: a pane parked out of the bay whose process finished left
# a dead window that nothing in conn ever showed and nothing but conn
# down ever cleared.
`)
	ground, ink := hex(groundColor), hex(inkColor)
	fmt.Fprintf(&b, "set -g window-style \"bg=%s,fg=%s\"\n", ground, ink)
	fmt.Fprintf(&b, "set -g cursor-colour \"%s\"\n", cursorHex)
	fmt.Fprintf(&b, "set -g mode-style \"bg=%s,fg=%s\"\n", borderHex, ink)
	for i, c := range scheme {
		fmt.Fprintf(&b, "set -g pane-colours[%d] \"%s\"\n", i, c)
	}
	b.WriteString("set -g pane-border-lines single\n")
	fmt.Fprintf(&b, "set -g pane-border-style \"fg=%s,bg=%s\"\n", borderHex, ground)
	fmt.Fprintf(&b, "set -g pane-active-border-style \"fg=%s,bg=%s\"\n", borderHex, ground)
	b.WriteString("set -g pane-border-indicators off\n")
	b.WriteString(statusLine())
	return b.String()
}

// The status line is tmux's status line, and it is an annunciator
// panel: dark at rest, lit by what would be worth turning for. It has
// two halves, each at a fixed position, so the eye learns where to
// glance and an empty position is itself a reading — the discipline of
// the 3270's operator information area, where the wait symbol was
// always in the same cell.
//
// On the left, where the keys are. A chord hanging and a pane in copy
// mode are the client's business and tmux's to know, and no amount of
// drawing on the panel will tell you either; tmux has them for nothing.
// A question conn has armed is the third, being not a state you are in
// but a thing waiting on you that takes the next key whatever it is.
// The fourth is the panel view the keys are in — PROCS, PROJECTS,
// SESSIONS — which conn knows and tmux does not.
//
// The view's word was left off for a while, on the reasoning that a
// word saying PROCS while you are looking at the processes view is
// furniture. It is not: the three views are worked by different keys,
// and a letter that narrows the rows in two of them runs a command in
// the third, so which one has the keys is exactly the kind of state the
// other three words say. The position is dark when the keys are in the
// bay, which is the reading that was wanted all along.
//
// Each is a block of the orange with the word knocked out of it, flush
// to the edge: a block is not read but seen, and one that starts where
// the screen starts is seen first. All four take the one color, since
// all four say the same fact — the keys are here, doing this — and the
// orange is what "you, here" is said in everywhere else in conn. The
// word inside says which, and says it more plainly than a hue can.
//
// The right is empty. It carried one lamp per row of the processes
// view, a strip of them in the corner of the eye after the Lisp
// machine's run bars, each in the color of how its process stood. A
// row of dots says how many things are running and which one wants
// you, which is what the processes view says in words a glance to the
// left, and it says it in a shape that has to be counted against that
// list to mean anything. The list is the reading; the dots were a
// second, worse copy of it.
//
// The line stands on the raised ground — the one a chosen row sits on
// everywhere else in conn — and keeps it whether or not anything is
// lit. A status line the color of the window reads as the last line of
// whatever pane is over it, and a status line that comes and goes is
// not somewhere to look.
//
// conn writes the one thing only it knows, where its own keys are, into
// an option of its own and only when it changes, which is on a
// keypress: nothing but a key moves the keys between views or arms a
// question. Never on a beat.
func statusLine() string {
	var b strings.Builder
	b.WriteString(`set -g status on
set -g status-position bottom
set -g status-justify left
set -g status-left-length 200
set -g status-right-length 200
# Nothing on the status line is read on a beat: conn sets its option when
# what it says changes, and nothing else on the line changes at all.
set -g status-interval 0
# conn has no tabs, so the middle of the line is nothing.
set -g window-status-format ""
set -g window-status-current-format ""
`)
	fmt.Fprintf(&b, "set -g status-style \"bg=%s,fg=%s\"\n", borderHex, grayHex)
	// Where the keys are, which is tmux's to know: the panel is the first
	// pane of the home window and everything else is work. A question is
	// armed on the panel and answered there, so it shows only while the
	// keys are on the panel to answer it.
	onPanel := fmt.Sprintf("#{&&:#{==:#{window_name},%s},#{==:#{pane_index},0}}", homeWindow)
	fmt.Fprintf(&b, "set -g status-left \"#{?client_prefix,%s,#{?pane_in_mode,%s,#{?%s,#{@conn_keys},}}}\"\n",
		statusLineBlock("PREFIX"), statusLineBlock("COPY"), onPanel)
	b.WriteString("set -g status-right \"\"\n")
	return b.String()
}

// statusLineBlock is a mode as the status line wears it: the ground
// knocked out of a block of the orange, flush to the edge, the word
// keeping its own space inside. The ground and not a fixed white, so it
// inverts with everything else — the orange on paper is a dark brick,
// and black would go out on it.
//
// One color for every mode. The orange is "you, here" everywhere else
// in conn — the cursor is drawn in it, and so is the kind of the row
// the bay holds — and where the keys are is the same fact about the
// same operator. A color apiece was tried: copy mode in the blue, the
// question in the waiting color, the view in a teal. It made four
// colors the eye had to learn and then read, in a position whose whole
// job is to be seen rather than read, and the word in the block says
// which mode it is more plainly than a hue ever did. What the position
// has to carry is lit or dark, and the word answers the rest.
//
// The attributes of a style are parted by spaces and not by commas: a
// comma inside a style is a comma to the conditional around it, and tmux
// would read the style as the branches of the question.
func statusLineBlock(word string) string {
	if word == "" {
		return ""
	}
	return fmt.Sprintf("#[bg=%s fg=%s bold] %s ", cursorHex, hex(groundColor), word)
}

// statusLineSay is what conn says beside a block: on the status line's
// own ground, in the parchment conn titles with, two spaces off the
// block. A hash is tmux's own character on this line and is doubled to
// be shown.
func statusLineSay(text string) string {
	return fmt.Sprintf("#[bg=%s fg=%s nobold]  %s", borderHex, scheme[7], strings.ReplaceAll(text, "#", "##"))
}

// say puts what conn knows about its own keys on the server, and asks
// the clients to draw, so the status line never lags what changed it.
func (s *server) say(keys string) error {
	_, err := s.run("set-option", "-g", "@conn_keys", keys,
		";", "refresh-client", "-S")
	return err
}

// shellQuote quotes a path for a tmux command line.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// A window of the server, for the report of what conn down ends: its
// name and the directory its pane is in.
type window struct {
	name, path string
}

// windows is every window in the server.
func (s *server) windows() ([]window, error) {
	out, err := s.run("list-windows", "-a", "-F", "#{window_name}\t#{pane_current_path}")
	if err != nil {
		return nil, err
	}
	return parseWindows(out), nil
}

// parseWindows reads list-windows: a name and a path per line.
func parseWindows(out string) []window {
	var ws []window
	for _, l := range strings.Split(out, "\n") {
		name, path, ok := strings.Cut(l, "\t")
		if !ok {
			continue
		}
		ws = append(ws, window{name: name, path: path})
	}
	return ws
}

// up says whether the server is up.
func (s *server) up() bool {
	_, err := s.run("has-session")
	return err == nil
}

// down ends the server and everything in it, and clears the ground it
// came up on, so the next one to rise picks fresh.
func (s *server) down() error {
	_, err := s.run("kill-server")
	if err == nil {
		_ = os.Remove(modePath(s.socket))
	}
	return err
}

// takeDown is conn down: it takes the server down and says what went
// with it, a line for each window and one for the server, the way
// docker compose down does. With no server up it says so, and that is
// not a failure. It answers what to say and whether it went well.
func takeDown(srv *server, home string) (string, bool) {
	if srv == nil {
		return "conn: tmux is not on PATH; there is no server to take down\n", false
	}
	if !srv.up() {
		return fmt.Sprintf("conn: no server up on %s\n", tilde(srv.socket, home)), true
	}
	ws, err := srv.windows()
	if err != nil {
		return fmt.Sprintf("conn: %v\n", err), false
	}
	if err := srv.down(); err != nil {
		return fmt.Sprintf("conn: %v\n", err), false
	}
	return downReport(ws, srv.socket, home), true
}

// downReport is what conn down says of what it ended.
func downReport(ws []window, socket, home string) string {
	var lines []string
	for _, w := range ws {
		lines = append(lines, "Window "+join("  ", w.name, tilde(w.path, home)))
	}
	lines = append(lines, "Server "+tilde(socket, home))
	width := 0
	for _, l := range lines {
		width = max(width, utf8.RuneCountInString(l))
	}
	var b strings.Builder
	for _, l := range lines {
		fmt.Fprintf(&b, " ✔ %-*s  ended\n", width, l)
	}
	return b.String()
}
