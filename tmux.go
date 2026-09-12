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
	sessionName   = "conn"
	homeWindow    = "home"
	railWidth     = 44 // the rail's columns; the slot has the rest
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

// attach brings the server up if it is down, with the watch window
// running conn, and puts this terminal on it until the client detaches
// or the server ends. It answers how the client exited; an error is one
// of its own, before the client had the terminal.
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
// says. Sourcing the configuration again is what tmux has in place of
// re-reading -f: every set -g in it lands on the live server, so each
// pane takes the new sixteen and the new ground without going down.
//
// The panes conn draws itself are the exception. They are conn, and
// conn reads the mode once, when it starts; the palette each is
// painting from is the one it rose on. So they are started again — the
// rail, and a hold if one is standing in the slot — and read the mode
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
// that furniture is a look; and whether remain-on-exit is the only
// thing keeping it up, its process already gone.
//
// A look is furniture too — everything true of a hold is true of it, so
// it carries the hold's own mark and everything that acts on holds acts
// on it — but i has to tell the two apart to know whether it is opening
// a page or closing one, and a mark of its own is how.
type pane struct {
	id, tty       string
	width, height int
	hold          bool
	look          bool
	dead          bool
}

const paneFormat = "#{pane_id}\t#{pane_tty}\t#{pane_width}\t#{pane_height}\t#{@conn_hold}\t#{pane_dead}\t#{@conn_look}"

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
			hold: f[4] == "1", dead: f[5] == "1", look: f[6] == "1"}
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
	return s.holdRail()
}

// holdRail sets the rail to its width. tmux keeps the panes in
// proportion when the window is resized, so the rail is put back each
// time it is not its width.
func (s *server) holdRail() error {
	_, err := s.run("resize-pane", "-t", s.rail(), "-x", strconv.Itoa(railWidth))
	return err
}

// reviveSlot puts a hold in a slot whose pane has died: remain-on-exit
// kept it there, its process gone, so this is a swap into the slot's
// own shape rather than a split — nothing about the window's layout
// moves. Without a slot at all, which a swap has nothing to land in,
// it falls back to splitSlot.
func (s *server) reviveSlot(home, self string) error {
	slot, ok, err := s.slot()
	if err != nil {
		return err
	}
	if !ok {
		return s.splitSlot(home, self)
	}
	return s.holdSlot(home, self, slot)
}

// hideLook puts the slot back to a hold, which is what closing the look
// leaves behind: the slot is conn's, and an empty one says so.
func (s *server) hideLook(home, self string) error {
	slot, ok, err := s.slot()
	if err != nil || !ok {
		return err
	}
	return s.holdSlot(home, self, slot)
}

// holdSlot puts a hold in the slot, in the slot's own shape, and is rid
// of whatever was there. It is a swap rather than a split so nothing
// about the window's layout moves, and the rail never has to give up
// its width and take it back.
func (s *server) holdSlot(home, self string, slot pane) error {
	id, err := s.run("new-window", "-d", "-P", "-F", "#{pane_id}", "-c", home, "exec "+shellQuote(self)+" hold")
	if err != nil {
		return err
	}
	hold := strings.TrimSpace(id)
	if _, err := s.run("set-option", "-p", "-t", hold, "@conn_hold", "1"); err != nil {
		return err
	}
	_, err = s.run("swap-pane", "-d", "-s", hold, "-t", slot.id, ";", "kill-pane", "-t", slot.id)
	return err
}

// showLook puts the look in the slot, and leaves focus on the rail. It
// is furniture rather than work — it runs nothing of yours, and
// reaching anything else is meant to be rid of it — so it is marked the
// way a hold is: killed when a real pane takes the slot, and respawned
// where it stands when the ground changes. Focus stays where it was
// because the look is a reading, not a place to be.
//
// It is opened on no pid, which is the look's word for "whatever the
// rail's cursor is on". One page then serves the whole list, j and k
// carrying it along, where a page opened per row would spawn a window a
// keystroke and blank the slot between each.
func (s *server) showLook(home, self string) error {
	id, err := s.run("new-window", "-d", "-P", "-F", "#{pane_id}", "-c", home,
		"exec "+shellQuote(self)+" look")
	if err != nil {
		return err
	}
	look := strings.TrimSpace(id)
	if _, err := s.run("set-option", "-p", "-t", look, "@conn_hold", "1"); err != nil {
		return err
	}
	if _, err := s.run("set-option", "-p", "-t", look, "@conn_look", "1"); err != nil {
		return err
	}
	slot, ok, err := s.slot()
	if err != nil {
		return err
	}
	if !ok {
		// Nothing to swap into. Split one first — swapping against the
		// rail instead would put the watch in the window the look came
		// from and the look where the watch belongs.
		if err := s.splitSlot(home, self); err != nil {
			return err
		}
		if slot, ok, err = s.slot(); err != nil {
			return err
		} else if !ok {
			return fmt.Errorf("home has no slot")
		}
	}
	args := []string{"swap-pane", "-d", "-s", look, "-t", slot.id}
	if slot.width > 0 && slot.height > 0 {
		args = append(args, ";", "resize-window", "-t", slot.id, "-x", strconv.Itoa(slot.width), "-y", strconv.Itoa(slot.height))
	}
	// What was in the slot goes back to a window of its own, still
	// running, unless it was conn's own furniture and has nothing to go
	// back to.
	if slot.hold {
		args = append(args, ";", "kill-pane", "-t", slot.id)
	}
	if _, err := s.run(args...); err != nil {
		return err
	}
	return s.focusRail()
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

// A shell conn opened, as tmux answers when it makes the window: the
// pane it is in and the process in it. What tmux says the process is
// called at that instant is tmux itself, before the shell has taken
// over, so the name is left to the process table.
type shell struct {
	pane pane
	pid  int
}

// open opens a shell at a directory, in a window of its own, and shows
// it in the slot.
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

// wide gives the rail the whole window, which is what the console
// wants: it is a page, not a rail. narrow gives the slot its side back.
// tmux has one key for both, so each looks first at how the window
// stands; on a home with no slot yet there is nothing to zoom and both
// are nothing.
func (s *server) wide() error { return s.zoom(true) }

func (s *server) narrow() error { return s.zoom(false) }

func (s *server) zoom(on bool) error {
	out, err := s.run("display-message", "-p", "-t", s.rail(), "#{window_zoomed_flag}")
	if err != nil {
		return err
	}
	if (strings.TrimSpace(out) == "1") == on {
		return nil
	}
	_, err = s.run("resize-pane", "-Z", "-t", s.rail())
	return err
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

// The terminal's chrome, beyond the ground and the ink the console is
// drawn in: the orange for the cursor, and the console's border color,
// which draws the line between the rail and the slot and sits behind a
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
// and the look. tmux's own prefix table is emptied, so none of its
// keys or actions are reachable through conn; prefix then - puts focus
// on the watch from wherever the keys are, and prefix then q detaches
// the way q does from the watch itself, without first coming back to
// it. The look is the console's: every pane on the ground, in the ink,
// with the sixteen colors a program asks for by name drawn from conn's
// scheme, with CONN set in the server so a program that draws in its
// own hex can tell where it is and dress to match, the cursor in the
// orange and a selection on the border color, and between the rail
// and the slot a line in that color too, the same whichever side has
// focus.
func tmuxConf(prefix string) string {
	var b strings.Builder
	b.WriteString(`# conn's tmux server. Written by conn on each start; edits do not keep.
# Four chords under the prefix: to the watch, to the list, to the other
# process, and to detach; tmux's own are unbound.
set -g prefix ` + prefix + `
set -g prefix2 None
unbind -a -T prefix
bind - select-pane -t ` + sessionName + ":" + homeWindow + `.0
bind p select-pane -t ` + sessionName + ":" + homeWindow + `.0 \; send-keys -t ` + sessionName + ":" + homeWindow + `.0 M-p
bind ` + prefix + ` select-pane -t ` + sessionName + ":" + homeWindow + `.0 \; send-keys -t ` + sessionName + ":" + homeWindow + `.0 M-o
bind q detach-client
set -g mouse on
# The rail's width is conn's to hold; a drag of the border would only be
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
set -g allow-passthrough on
set -g display-time 3000
# A pane whose process ends stays instead of closing, so a killed shell
# does not collapse the window to the rail alone before conn re-splits
# it: the slot holds its place, dead, until conn puts a hold there.
set -g remain-on-exit on
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
	b.WriteString(bar())
	return b.String()
}

// The bar is tmux's status line, and it is an annunciator panel rather
// than a status line: dark, saying nothing at all, until something
// lights it. A row that always says something is a row nobody reads, and
// most of what a status line carries — where the keys are, who you are,
// what time it is — you already know or do not need. What conn puts
// there instead is what you would want to be interrupted for, and
// nothing else ever.
//
// It is the only instrument conn has that works on peripheral vision.
// The rail cannot catch your eye: when you are working your eyes are in
// the slot, and the watch is beside them unread. The bar spans the
// window under both, and a dark row that lights is seen without being
// looked at. That is what the row is worth, and it is only worth it
// while the row is dark the rest of the time — so its ground is the
// window's own, and at rest there is nothing there to tell it from the
// padding around the client.
//
// On the left are the lamps for what the keys are doing, which is the
// half conn cannot see from inside its own pane — a chord hanging, a
// pane in copy mode, a kill waiting on its second key — and beside them
// what conn has to say, which is the answer to a key just pressed and
// stands until the next one.
//
// On the right is the one question conn asks of you: an agent stopped on
// something it put to you and cannot go on without. It blinks, because a
// lamp that blinks is the one thing on a screen that reaches the corner
// of an eye.
//
// The blinking is the clock's, not conn's and not the terminal's. The
// attribute was the obvious way and it does not work: the terminfo
// advertises blink and tmux duly sends it, and a terminal is free to
// draw it steady — Ghostty does. So the lamp is lit on an odd second and
// dark on an even one, which tmux can say for itself: the status line is
// run through strftime before the conditionals in it are read, so the
// format can ask what second it is. tmux redraws the line each second to
// do it, which costs no process at all — where conn blinking it would be
// two a cycle for as long as anything waited. The beat is a second
// either way, rather than the console's second and a half, because a
// second is the finest tmux has.
func bar() string {
	var b strings.Builder
	b.WriteString(`set -g status on
set -g status-position bottom
set -g status-justify left
set -g status-left-length 200
set -g status-right-length 60
# The line is drawn each second, which is what the waiting lamp blinks
# on. Every lamp is set when it changes; the beat only re-reads them.
set -g status-interval 1
# conn has no tabs, so the middle of the line is nothing.
set -g window-status-format ""
set -g window-status-current-format ""
`)
	// The window's own ground, so a panel with nothing lit is not a bar.
	fmt.Fprintf(&b, "set -g status-style \"bg=%s,fg=%s\"\n", hex(groundColor), grayHex)
	// The mode lamps, in the order the states shadow one another: a chord
	// hanging covers everything, copy mode covers what conn says of
	// itself, and with none of them there is no lamp. conn's words are
	// the orange, which is "you, here" everywhere else in conn.
	mode := fmt.Sprintf("#{?client_prefix,#[fg=%s bold]PREFIX,"+
		"#{?pane_in_mode,#[fg=%s bold]COPY,"+
		"#{?#{!=:#{@conn_mode},},#[fg=%s bold]#{@conn_mode},}}}", cursorHex, cursorHex, cursorHex)
	fmt.Fprintf(&b, "set -g status-left \"%s#[fg=%s nobold]  #{@conn_note}\"\n", mode, scheme[1])
	// Lit on the odd second, dark on the even: the blink is the clock's.
	fmt.Fprintf(&b, "set -g status-right \"#{?#{m:*[13579],%%S},#[fg=%s bold]#{@conn_owed},}\"\n", scheme[1])
	return b.String()
}

// say lights the panel: what conn has to say, the mode its lamp shows,
// and what is held up on you. It asks the clients to draw, so the bar
// never lags the key that changed it.
func (s *server) say(note, mode, owed string) error {
	_, err := s.run("set-option", "-g", "@conn_note", note,
		";", "set-option", "-g", "@conn_mode", mode,
		";", "set-option", "-g", "@conn_owed", owed,
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
