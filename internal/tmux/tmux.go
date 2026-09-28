// Package tmux is the tmux server conn holds of its own. The first conn
// brings it up with one window, home, running conn, and attaches; a
// later conn attaches to what is there. Home is a panel on the left,
// which is the processes view, and a bay on the right, which is the
// process reached from it. Work lives in the server as windows of its
// own, out of sight; reaching a process swaps its pane into the bay and
// the bay's last pane back out to where it came from, so a process
// stays when the client goes. q detaches; the server and everything in
// it keep on. The conn that attached stays behind the client, to give
// the terminal its own colors back when the client returns, which tmux
// does not.
//
// The socket is under the state directory, or where CONN_SOCKET says,
// which is how a test brings up a server of its own.
package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/w0zro/conn/internal/config"
	"github.com/w0zro/conn/internal/theme"
)

const (
	SessionName = "conn"
	HomeWindow  = "home"
	PanelWidth  = 44 // the panel's columns; the bay has the rest
	DefaultKey  = "C-Space"
)

// PanelKey is the one key tmux takes for conn from anywhere in the
// station: ctrl+space, or what CONN_KEY says, in tmux's spelling of a
// key. It brings the keys to the panel, and the panel answers the key
// after it.
func PanelKey() string {
	if p := os.Getenv("CONN_KEY"); p != "" {
		return p
	}
	return DefaultKey
}

// A Server is conn's tmux server: the tmux program and the socket.
type Server struct {
	Tmux   string
	Socket string
	// One change to the bay at a time. Each is a run of tmux commands
	// that reads the bay and then swaps against it, and two of them at
	// once — the page being put in as the operator opens a shell — each
	// read the bay before the other's swap landed, and the second swapped
	// against a pane the first had killed. The panel runs each off its
	// loop, so nothing but this keeps them apart.
	swaps sync.Mutex
}

// Find is the server as this machine has it: no tmux, no server.
func Find(home string) *Server {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		return nil
	}
	return &Server{Tmux: tmux, Socket: SocketPath(home)}
}

// SocketPath is where the server listens: CONN_SOCKET, or tmux.sock in
// the state directory.
func SocketPath(home string) string {
	if p := os.Getenv("CONN_SOCKET"); p != "" {
		return p
	}
	return filepath.Join(config.StateHome(home), "conn", "tmux.sock")
}

// InsideConn says whether this process runs in a pane of conn's server:
// tmux tells its panes the socket in TMUX, before the first comma.
func InsideConn(tmuxEnv, socket string) bool {
	sock, _, _ := strings.Cut(tmuxEnv, ",")
	return sock != "" && filepath.Clean(sock) == filepath.Clean(socket)
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

// Attach brings the server up if it is down, with the processes view
// window running conn, and puts this terminal on it until the client
// detaches or the server ends. It answers how the client exited; an
// error is one of its own, before the client had the terminal.
//
// The server's mode - its theme, and the ground it is on - is what a
// mode file beside the socket says, or the terminal's own ground in
// conn's own theme the first time a server rises, written down so it
// holds across attaches. A flag says it instead, and says it whenever
// it is given: a server already up is put on the other ground, or in
// the other theme, where it stands, rather than keeping what it rose
// in until conn down. tmux does not re-read -f on an attach, so that
// takes sourcing the configuration again; see reground.
func (s *Server) Attach(self, home string, o theme.Override) (int, error) {
	if err := os.MkdirAll(filepath.Dir(s.Socket), 0o700); err != nil {
		return 0, err
	}
	have, ok := theme.ReadModeFile(s.Socket)
	want, asked := have, false
	switch {
	case !ok:
		want = theme.AskMode(o, home)
		_ = theme.WriteMode(s.Socket, want)
	case o.Over(have) != have:
		want = o.Over(have)
		_ = theme.WriteMode(s.Socket, want)
		asked = true
	}
	g := want.Wear()
	theme.RefreshClaudeTheme(home, g)
	theme.RefreshVimColorscheme(home, g)
	conf := confPath(s.Socket)
	if err := os.WriteFile(conf, []byte(Conf(PanelKey(), g)), 0o600); err != nil {
		return 0, err
	}
	if asked {
		if err := s.Reground(conf, g.Surface, "", true); err != nil {
			return 0, err
		}
	}
	// A server that is up but has lost its home window gets one back.
	if _, err := s.Run("has-session", "-t", "="+SessionName); err == nil {
		if !s.hasHome() {
			if _, err := s.Run("new-window", "-d", "-t", SessionName+":", "-n", HomeWindow, "-c", home, "exec "+ShellQuote(self)); err != nil {
				return 0, err
			}
		}
	}
	cmd := exec.Command(s.Tmux, "-S", s.Socket, "-f", conf,
		"new-session", "-A", "-s", SessionName, "-n", HomeWindow, "-c", home, "exec "+ShellQuote(self))
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	// A tmux inside another tmux refuses to attach while TMUX is set; the
	// terminal is this conn's to give.
	cmd.Env = WithoutTmux(os.Environ())
	// The terminal takes black and the ink for its own before the
	// client has it, so the padding around the client is the terminal's
	// own edge, on either ground. The conn in the pane asks tmux for
	// the pane's own ground, which tmux keeps to the pane; the terminal
	// outside hears it from here.
	fmt.Print(oscColors(g))
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

// Rewear puts the server into a mode conn has just taken on. It is
// reground for a conn already in the server: the mode is written down
// first, since the panes that come up again read it, and the panel is
// left standing — it is told to wear the mode where it stands, and
// killing it to change color would take the view the operator is
// working with it.
func (s *Server) Rewear(conf, bg, except string, m theme.Mode) error {
	if err := theme.WriteMode(s.Socket, m); err != nil {
		return err
	}
	if err := os.WriteFile(confPath(s.Socket), []byte(conf), 0o600); err != nil {
		return err
	}
	return s.Reground(confPath(s.Socket), bg, except, false)
}

// confPath is the tmux configuration conn writes for its server, beside
// the socket and the mode.
func confPath(socket string) string {
	return filepath.Join(filepath.Dir(socket), "tmux.conf")
}

// Reground puts a server already up into the mode the mode file now
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
//
// bg is the surface the panel's pane is painted, handed in rather than
// read here: the caller has the palette on the loop, and this runs off
// it. panel is whether the panel is one of the panes to start again — a
// conn that attached from outside has to, and a conn already in the
// server, which tells the panel to wear the mode where it stands, must
// not. except is the pane asking, where the asking came from a pane of
// conn's own: it has put the mode on itself already, and starting it
// again would take the operator back to the top of the page they are
// working.
func (s *Server) Reground(conf, bg, except string, panel bool) error {
	if !s.Up() {
		return nil
	}
	if _, err := s.Run("source-file", conf); err != nil {
		return err
	}
	panes, err := s.Panes()
	if err != nil {
		return err
	}
	for _, p := range panes {
		if p.Hold && p.ID != except {
			if _, err := s.Run("respawn-pane", "-k", "-t", p.ID); err != nil {
				return err
			}
		}
	}
	// The panel's pane is painted on the new ground's surface here as
	// well as by the conn that comes up in it, so the window is right
	// in the same breath as the rest and not a moment after.
	//
	// The style is set as the pane's own option rather than with
	// select-pane -P, which paints a pane by first making it the pane
	// the keys are in. That was nothing while the panel was the only
	// conn that asked for a ground — it had the keys already — and
	// with the settings in the workspace it took them out from under
	// the operator mid-page.
	target := SessionName + ":" + HomeWindow + ".0"
	if _, err := s.Run("set-option", "-p", "-t", target, "window-style", "bg="+bg); err != nil {
		return err
	}
	if !panel {
		return nil
	}
	_, err = s.Run("respawn-pane", "-k", "-t", target)
	return err
}

// oscColors asks the terminal to take the ink of a ground and a ground
// for its own, and oscOwnColors gives it its own back. What the terminal paints
// with the ground is the padding around the client, and the padding
// meets the panel and the key bar. It is black, and not the surface:
// the terminal's own edge, the same on every theme and on either
// ground, with the frame standing on it. tmux
// keeps what the conn in a pane asks for to the pane, so the terminal
// outside hears it from the conn that attached, which is also there to
// take it back. The cursor is the other way about: tmux does put the
// server's on the terminal, and leaves it there when the client goes,
// so conn asks for nothing and takes it back all the same.
func oscColors(g theme.Ground) string {
	return fmt.Sprintf("\x1b]10;%s\x1b\\\x1b]11;%s\x1b\\", theme.Hex(g.Ink), paddingHex)
}

// paddingHex is what the terminal is asked to paint around the client:
// black, on either ground.
const paddingHex = "#000000"

const oscOwnColors = "\x1b]110\x1b\\\x1b]111\x1b\\\x1b]112\x1b\\"

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

// hasHome says whether the session has its home window.
func (s *Server) hasHome() bool {
	out, err := s.Run("list-windows", "-t", SessionName, "-F", "#{window_name}")
	if err != nil {
		return false
	}
	for _, name := range strings.Split(out, "\n") {
		if name == HomeWindow {
			return true
		}
	}
	return false
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

// Reachable says whether conn can put a pane in front of you: it holds
// one, that pane has work in it rather than conn's own furniture, and
// the process in it has not ended. It is the one rule, so that what
// enter does, what the ring steps through, what the bay takes when its
// own ends, and what the page reports cannot drift apart into four
// slightly different answers to one question.
func Reachable(p Pane) bool {
	return p.ID != "" && !p.Hold && !p.Readout && !p.Dead
}

// Panel is the pane this conn runs in; tmux names it in TMUX_PANE.
func (s *Server) Panel() string {
	return OwnPane()
}

// OwnPane is the pane this conn runs in, whichever conn it is: the
// panel for the one that draws the panel, and its own pane for a page
// of conn's own standing in the workspace. A page asking the server to
// change color names it, so that it is the one pane not started again.
func OwnPane() string {
	return os.Getenv("TMUX_PANE")
}

// home is every pane of the home window, in the order they stand.
func (s *Server) home() ([]Pane, error) {
	out, err := s.Run("list-panes", "-t", s.Panel(), "-F", paneFormat)
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

// Bay is the pane beside the panel in the home window, when there is
// one: the first pane after the panel, by where it stands. The panes
// were read out of a map, in whatever order the map gave them, which
// was one answer while home had two panes and a coin toss once it had
// three — and a swap against the wrong one of the three, both in
// home, sized home to the bay's width and left the operator a window
// one column wide.
func (s *Server) Bay() (Pane, bool, error) {
	panes, err := s.home()
	if err != nil {
		return Pane{}, false, err
	}
	for _, p := range panes {
		if p.ID != s.Panel() {
			return p, true, nil
		}
	}
	return Pane{}, false, nil
}

// SplitBay opens the bay beside the panel, with a hold in it, and sets
// the panel to its width. Focus stays on the panel.
func (s *Server) SplitBay(home, self string) error {
	s.swaps.Lock()
	defer s.swaps.Unlock()
	return s.split(home, self)
}

// split is splitBay for a caller that already holds the bay. A bay
// that is there already is left alone: two readings that each found
// home without one, before either's split had landed, each split one
// in, and home stood with three panes — the second hold beside the
// first, and whatever came next swapped against whichever of the two
// the lookup happened on.
func (s *Server) split(home, self string) error {
	if _, ok, err := s.Bay(); err != nil {
		return err
	} else if ok {
		return nil
	}
	id, err := s.Run("split-window", "-h", "-d", "-P", "-F", "#{pane_id}", "-t", s.Panel(), "-c", home, "exec "+ShellQuote(self)+" hold")
	if err != nil {
		return err
	}
	if _, err := s.Run("set-option", "-p", "-t", strings.TrimSpace(id), "@conn_hold", "1"); err != nil {
		return err
	}
	// A pane in this window whose process ends stays instead of
	// closing, so a killed shell does not collapse the window to the
	// panel alone before conn re-splits it: the bay holds its project,
	// dead, until conn puts a hold there. It is the window's option
	// and not the server's, since the bay is the only place conn has a
	// layout to protect. Everywhere else a window whose work has ended
	// is a window that is done.
	if _, err := s.Run("set-option", "-w", "-t", strings.TrimSpace(id), "remain-on-exit", "on"); err != nil {
		return err
	}
	return s.HoldPanel()
}

// HoldPanel sets the panel to its width. tmux keeps the panes in
// proportion when the window is resized, so the panel is put back each
// time it is not its width.
func (s *Server) HoldPanel() error {
	_, err := s.Run("resize-pane", "-t", s.Panel(), "-x", strconv.Itoa(PanelWidth))
	return err
}

// ReviveBay puts a hold in a bay whose pane has died: remain-on-exit
// kept it there, its process gone, so this is a swap into the bay's
// own shape rather than a split — nothing about the window's layout
// moves. Without a bay at all, which a swap has nothing to land in,
// it falls back to splitBay.
func (s *Server) ReviveBay(home, self string) error {
	s.swaps.Lock()
	defer s.swaps.Unlock()
	bay, ok, err := s.Bay()
	if err != nil {
		return err
	}
	if !ok {
		return s.split(home, self)
	}
	return s.holdBay(home, self, bay)
}

// holdBay puts a hold in the bay, in the bay's own shape, and is rid
// of whatever was there. It is a swap rather than a split so nothing
// about the window's layout moves, and the panel never has to give up
// its width and take it back.
func (s *Server) holdBay(home, self string, bay Pane) error {
	id, err := s.Run("new-window", "-d", "-P", "-F", "#{pane_id}", "-c", home, "exec "+ShellQuote(self)+" hold")
	if err != nil {
		return err
	}
	hold := strings.TrimSpace(id)
	if _, err := s.Run("set-option", "-p", "-t", hold, "@conn_hold", "1"); err != nil {
		return err
	}
	_, err = s.Run("swap-pane", "-d", "-s", hold, "-t", bay.ID, ";", "kill-pane", "-t", bay.ID)
	return err
}

// ShowReadout puts the readout in the bay, and leaves focus on the
// panel. Focus stays where it was because the readout is a reading,
// not a project to be.
//
// It is opened on no pid, which is the readout's word for "whatever the
// panel's cursor is on". One page then serves the whole list, j and k
// carrying it along, where a page opened per row would spawn a window a
// keystroke and blank the bay between each.
func (s *Server) ShowReadout(home, self string) error {
	return s.showOwn(home, self, "readout", "@conn_readout", false)
}

// ShowHelp puts the manual in the workspace, with the keys in it: it is
// a page to be read, and a page that cannot be scrolled cannot be read.
// The panel says HELP while it stands, so where the keys have gone is
// not left to be guessed at.
func (s *Server) ShowHelp(home, self string) error {
	return s.showOwn(home, self, "manual", "@conn_help", true)
}

// ShowSettings puts the settings in the workspace, with the keys in it.
// The workspace is where conn puts what is being worked on, and the
// configuration is that while it is open: the panel is the list of
// what is running and has no room to be a form as well. The panel says
// SETTINGS while it stands and goes on reading the machine beside it.
func (s *Server) ShowSettings(home, self string) error {
	return s.showOwn(home, self, "settings", "@conn_settings", true)
}

// showOwn puts one of conn's own pages in the bay: the readout, the
// manual, the settings. Each is a conn of its own, run as a command of
// this binary in a window of its own and swapped into the bay, so the
// row it would otherwise take in the processes view is not taken and
// the keys in it are conn's to decide.
//
// Every one is furniture rather than work — it runs nothing of yours,
// and reaching anything else is meant to be rid of it. So each carries
// the hold's own mark, and everything that acts on holds acts on it:
// killed when a real pane takes the bay, respawned where it stands
// when the ground changes. Each carries a mark of its own besides,
// which is how the panel tells which page is standing. What is left to
// the caller is the keys: a page to be read or worked takes them, and
// a reading does not.
//
// What was in the bay goes back to a window of its own, still running,
// unless it was conn's own furniture and has nothing to go back to.
func (s *Server) showOwn(home, self, cmd, mark string, keys bool) error {
	s.swaps.Lock()
	defer s.swaps.Unlock()
	id, err := s.Run("new-window", "-d", "-P", "-F", "#{pane_id}", "-c", home,
		"exec "+ShellQuote(self)+" "+cmd)
	if err != nil {
		return err
	}
	page := strings.TrimSpace(id)
	for _, opt := range []string{"@conn_hold", mark} {
		if _, err := s.Run("set-option", "-p", "-t", page, opt, "1"); err != nil {
			return err
		}
	}
	bay, ok, err := s.Bay()
	if err != nil {
		return err
	}
	if !ok {
		// Nothing to swap into. Split one first — swapping against the panel
		// instead would put the processes view in the window the page came
		// from and the page where the processes view belongs.
		if err := s.split(home, self); err != nil {
			return err
		}
		if bay, ok, err = s.Bay(); err != nil {
			return err
		} else if !ok {
			return fmt.Errorf("home has no bay")
		}
	}
	args := []string{"swap-pane", "-d", "-s", page, "-t", bay.ID}
	if bay.Width > 0 && bay.Height > 0 {
		args = append(args, ";", "resize-window", "-t", bay.ID, "-x", strconv.Itoa(bay.Width), "-y", strconv.Itoa(bay.Height))
	}
	if bay.Hold {
		args = append(args, ";", "kill-pane", "-t", bay.ID)
	}
	if _, err := s.Run(args...); err != nil {
		return err
	}
	if keys {
		return s.focusPane(page)
	}
	return s.FocusPanel()
}

// Show puts a pane in the bay and focus on it. The pane that was in the
// bay goes back to where this one came from, and its window takes the
// bay's size so it keeps its shape; a hold that leaves the bay is
// done with, and so is a pane whose process has ended, which
// remain-on-exit kept only so the bay would hold its project.
//
// The swap and the select are one command to tmux, so there is no
// moment at which the pane is in the bay and the keys are still on the
// panel. There was one, and a reading that landed in it saw a bay with
// no page in it under a panel that still had the keys, and put the
// page back over the process the operator had just gone into.
func (s *Server) Show(target Pane) error {
	s.swaps.Lock()
	defer s.swaps.Unlock()
	bay, ok, err := s.Bay()
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("home has no bay")
	}
	// Whether the target is itself in home, which it is only when home
	// has more panes than it should: the pane it displaces then stays
	// in home too, and sizing that pane's window to the bay would size
	// home, which is the window the operator is looking at.
	inHome := false
	if panes, err := s.home(); err == nil {
		for _, p := range panes {
			inHome = inHome || p.ID == target.ID
		}
	}
	var args []string
	if target.ID != bay.ID {
		args = []string{"swap-pane", "-d", "-s", target.ID, "-t", bay.ID}
		if bay.Width > 0 && bay.Height > 0 && !inHome {
			args = append(args, ";", "resize-window", "-t", bay.ID, "-x", strconv.Itoa(bay.Width), "-y", strconv.Itoa(bay.Height))
		}
		if bay.Hold || bay.Dead {
			args = append(args, ";", "kill-pane", "-t", bay.ID)
		}
		args = append(args, ";")
	}
	_, err = s.Run(append(args, "select-pane", "-t", target.ID)...)
	return err
}

// A Shell conn opened, as tmux answers when it makes the window: the
// pane it is in and the process in it. What tmux says the process is
// called at that instant is tmux itself, before the shell has taken
// over, so the name is left to the process table.
type Shell struct {
	Pane Pane
	PID  int
}

// Open opens a shell at a directory, in a window of its own, and shows
// it in the bay.
func (s *Server) Open(dir string) (Shell, error) {
	return s.OpenCmd(dir, "")
}

// OpenCmd is open, running a command instead of the directory's own
// shell — what a opens claude with.
func (s *Server) OpenCmd(dir, cmd string) (Shell, error) {
	args := []string{"new-window", "-d", "-P", "-F", openFormat, "-c", dir}
	if cmd != "" {
		args = append(args, cmd)
	}
	out, err := s.Run(args...)
	if err != nil {
		return Shell{}, err
	}
	sh := parseOpened(out)
	return sh, s.Show(sh.Pane)
}

// OpenWatching opens a pane conn made to watch a container, marked with
// the container it is watching. The mark is what makes the pane the
// terminal the container's row stands on, and what keeps the watcher
// itself off the view: a docker logs listed beside the service it is
// showing would be the same thing twice.
func (s *Server) OpenWatching(dir, cmd, id string) (Shell, error) {
	sh, err := s.openMarked(dir, cmd, "@conn_container", id)
	if err != nil {
		return Shell{}, err
	}
	sh.Pane.Container = id
	return sh, s.Show(sh.Pane)
}

// OpenShellIn opens a pane running a shell inside a container. It is
// marked as a shell in that container and not as the container's own
// terminal: the service is read in one pane and worked in from another,
// and only the reader stands in for the terminal the service has not
// got.
func (s *Server) OpenShellIn(dir, cmd, id string) (Shell, error) {
	sh, err := s.openMarked(dir, cmd, "@conn_shell_in", id)
	if err != nil {
		return Shell{}, err
	}
	sh.Pane.ShellIn = id
	return sh, s.Show(sh.Pane)
}

// RaiseDeclared opens a declared process's pane, marked as the
// declaration, in place of the pane that last held it where one is
// still standing with its last output in it. Shown, it goes into the
// bay with the keys in it; not shown, it is parked in a window of its
// own for the reading to list, which is how a project is brought up
// whole without the bay ending on whichever pane opened last.
func (s *Server) RaiseDeclared(dir, cmd, mark, replace string, show bool) (Shell, error) {
	if replace != "" {
		_, _ = s.Run("kill-pane", "-t", replace)
	}
	sh, err := s.openMarked(dir, cmd, "@conn_declared", mark)
	if err != nil {
		return Shell{}, err
	}
	sh.Pane.Declared = mark
	if show {
		return sh, s.Show(sh.Pane)
	}
	return sh, nil
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

// openMarked opens a pane running a command and sets one option on it,
// which is how conn remembers what it opened a pane for.
func (s *Server) openMarked(dir, cmd, option, value string) (Shell, error) {
	args := []string{"new-window", "-d", "-P", "-F", openFormat, "-c", dir}
	if cmd != "" {
		args = append(args, cmd)
	}
	out, err := s.Run(args...)
	if err != nil {
		return Shell{}, err
	}
	sh := parseOpened(out)
	if _, err := s.Run("set-option", "-p", "-t", sh.Pane.ID, option, value); err != nil {
		return Shell{}, err
	}
	return sh, nil
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

// Wide gives the panel the whole window, which is what the console
// wants: it is a page, not a panel. Narrow gives the bay its side back.
// tmux has one key for both, so each looks first at how the window
// stands; on a home with no bay yet there is nothing to zoom and both
// are nothing.
func (s *Server) Wide() error { return s.zoom(true) }

func (s *Server) Narrow() error { return s.zoom(false) }

func (s *Server) zoom(on bool) error {
	out, err := s.Run("display-message", "-p", "-t", s.Panel(), "#{window_zoomed_flag}")
	if err != nil {
		return err
	}
	if (strings.TrimSpace(out) == "1") == on {
		return nil
	}
	_, err = s.Run("resize-pane", "-Z", "-t", s.Panel())
	return err
}

// FocusPanel puts focus on the panel.
func (s *Server) FocusPanel() error {
	return s.focusPane(s.Panel())
}

// focusPane puts the keys in a pane by its id, which reaches only a
// pane of the window the client is looking at — so it is the panel's
// own way back, and not a way to somewhere else. Putting the keys in a
// process means reaching it: show brings the pane into the workspace
// first, and the window the client is on is the one it was already on.
func (s *Server) focusPane(id string) error {
	_, err := s.Run("select-pane", "-t", id)
	return err
}

// Detach lets the client go; the server keeps on.
func (s *Server) Detach() error {
	_, err := s.Run("detach-client")
	return err
}

// Conf is the server's configuration: the panel key, and how
// every pane is drawn, on a ground. tmux has no prefix here, so none of its keys or
// actions are reachable through conn; the one key it takes is the
// panel's, bound in the root table so that it works from inside a
// process, and what it does is bring the keys to the panel and say so
// — the pane they came out of first, so that the panel can put its row
// under the cursor and a detour begun from here can end back in it.
// Every key of the panel's is then reachable from inside a process
// with the panel key before it, and the panel's own table is the only
// one there is. The drawing is the console's: every
// pane on the ground, in the ink, with the sixteen colors a program
// asks for by name drawn from conn's scheme, with CONN set in the
// server so a program that draws in its own hex can tell where it is
// and dress to match, the cursor in the orange and a selection on the
// border color, and between the panel and the bay a line in that color
// too, the same whichever side has focus.
func Conf(key string, g theme.Ground) string {
	var b strings.Builder
	b.WriteString(`# conn's tmux server. Written by conn on each start; edits do not keep.
# One key, from anywhere in the station: to the panel, which says where
# the keys came from. No prefix, so nothing of tmux's own is reachable,
# and the panel or the process answers every other key. There is no key
# for the page: in the processes view the page is what the workspace
# holds, and nothing is pressed for it.
set -g prefix None
set -g prefix2 None
bind -n ` + key + ` set -gF @conn_from "#{pane_id}" \; select-pane -t ` + SessionName + ":" + HomeWindow + `.0 \; send-keys -t ` + SessionName + ":" + HomeWindow + `.0 M--
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
	ground, ink := theme.Hex(g.Ground), theme.Hex(g.Ink)
	fmt.Fprintf(&b, "set -g window-style \"bg=%s,fg=%s\"\n", ground, ink)
	fmt.Fprintf(&b, "set -g cursor-colour \"%s\"\n", g.Accent)
	fmt.Fprintf(&b, "set -g mode-style \"bg=%s,fg=%s\"\n", g.Border, ink)
	for i, c := range g.Scheme {
		fmt.Fprintf(&b, "set -g pane-colours[%d] \"%s\"\n", i, c)
	}
	// The seam between the panel and the bay is the panel's surface
	// meeting the bay's ground, and needs no line drawn on it: the
	// border column is painted in the ground, line and all, so it is a
	// column of the bay's own air between the two, and the panel ends a
	// column short of where the bay begins.
	b.WriteString("set -g pane-border-lines single\n")
	fmt.Fprintf(&b, "set -g pane-border-style \"fg=%s,bg=%s\"\n", ground, ground)
	fmt.Fprintf(&b, "set -g pane-active-border-style \"fg=%s,bg=%s\"\n", ground, ground)
	b.WriteString("set -g pane-border-indicators off\n")
	b.WriteString(statusLine(g))
	return b.String()
}

// The status line is tmux's status line, and it is an annunciator
// panel: dark at rest, lit by what would be worth turning for. It has
// two halves, each at a fixed position, so the eye learns where to
// glance and an empty position is itself a reading — the discipline of
// the 3270's operator information area, where the wait symbol was
// always in the same cell.
//
// On the left, where the keys are. A pane in copy mode is the client's
// business and tmux's to know, and no amount of drawing on the panel
// will tell you; tmux has it for nothing. A question conn has armed is
// the other, being not a state you are in
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
func statusLine(g theme.Ground) string {
	var b strings.Builder
	b.WriteString(`set -g status on
set -g status-position bottom
set -g status 2
set -g status-justify left
set -g status-left-length 200
set -g status-right-length 100
# Nothing on the status line is read on a beat: conn sets its options
# when what they say changes, and nothing else on the line changes at all.
set -g status-interval 0
# conn has no tabs, so the middle of the line is nothing.
set -g window-status-format ""
set -g window-status-current-format ""
set -g pane-border-status off
`)
	fmt.Fprintf(&b, "set -g status-style \"bg=%s,fg=%s\"\n", g.Border, g.Gray)
	// Two rows across the foot. The upper is the band: a mode tmux knows
	// itself first — COPY in copy mode — then where the keys are, by conn's word, while the keys are on the
	// panel, and the station's word when they are not; and at the right
	// edge the clock. The lower is the key bar: the keys that work where
	// the cursor is, or a question armed, and at the right the station's
	// designation.
	onPanel := fmt.Sprintf("#{&&:#{==:#{window_name},%s},#{==:#{pane_index},0}}", HomeWindow)
	fmt.Fprintf(&b, "set -g status-left \"#{?pane_in_mode,%s,#{?%s,#{@conn_keys},#{@conn_station}}}\"\n",
		StatusLineBlock("COPY", g), onPanel)
	b.WriteString("set -g status-right \"#{@conn_up}\"\n")
	b.WriteString("set -g status-format[0] \"#[align=left]#{T:status-left}#[align=right]#{T:status-right}\"\n")
	// The key bar is on the surface, the panel's own ground, so the two
	// rows are two things: the band the window's frame, the bar the
	// panel's footer.
	fmt.Fprintf(&b, "set -g status-format[1] \"#[fill=%s bg=%s]#{@conn_bar}#[align=right]#{@conn_ident}\"\n", g.Surface, g.Surface)
	return b.String()
}

// StatusLineBlock is a mode as the status line wears it: the ground
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
func StatusLineBlock(word string, g theme.Ground) string {
	if word == "" {
		return ""
	}
	return fmt.Sprintf("#[bg=%s fg=%s bold] %s ", g.Accent, theme.Hex(g.Ground), word)
}

// StatusLineSay is what conn says on the key bar in words, a question
// armed: on the bar's own ground, the surface, in the parchment conn
// titles with, one space in where the keys begin. A hash is tmux's own
// character on this line and is doubled to be shown.
func StatusLineSay(text string, g theme.Ground) string {
	return fmt.Sprintf("#[bg=%s fg=%s nobold] %s", g.Surface, g.Parchment, strings.ReplaceAll(text, "#", "##"))
}

// Say puts what conn knows about its own keys on the server, and asks
// the clients to draw, so the status line never lags what changed it.
func (s *Server) Say(keys, station, up, bar, ident string) error {
	_, err := s.Run("set-option", "-g", "@conn_keys", keys,
		";", "set-option", "-g", "@conn_station", station,
		";", "set-option", "-g", "@conn_up", up,
		";", "set-option", "-g", "@conn_bar", bar,
		";", "set-option", "-g", "@conn_ident", ident,
		";", "refresh-client", "-S")
	return err
}

// SayBand is say without the key bar, for the panel while a page of
// conn's own has the keys and is writing that position itself.
func (s *Server) SayBand(keys, station, up, ident string) error {
	_, err := s.Run("set-option", "-g", "@conn_keys", keys,
		";", "set-option", "-g", "@conn_station", station,
		";", "set-option", "-g", "@conn_up", up,
		";", "set-option", "-g", "@conn_ident", ident,
		";", "refresh-client", "-S")
	return err
}

// SayBar is the key bar alone, for a page of conn's own that has the
// keys: what the keys do there is its own to say, since the bar says
// the keys that work where the cursor is and the cursor is in that
// pane. The panel leaves the position alone while such a page stands;
// see saying in tui.go.
func (s *Server) SayBar(bar string) error {
	_, err := s.Run("set-option", "-g", "@conn_bar", bar, ";", "refresh-client", "-S")
	return err
}

// StatusLineWord is a word on the line's own ground: the wordmark in
// the ink and bold, or a figure in the gray.
func StatusLineWord(text, color string, bold bool, g theme.Ground) string {
	weight := "nobold"
	if bold {
		weight = "bold"
	}
	return fmt.Sprintf("#[bg=%s fg=%s %s]%s", g.Border, color, weight, strings.ReplaceAll(text, "#", "##"))
}

// tellPanel sends the panel a key. A page of conn's own is a conn in a
// pane of its own, and the only way it has to speak to the panel is
// the way the panel key does: a key, sent to it.
//
// The keys it sends are alt keys the panel answers to and nothing else
// does; see leaveKey and worn.
func (s *Server) tellPanel(key string) error {
	_, err := s.Run("send-keys", "-t", SessionName+":"+HomeWindow+".0", key)
	return err
}

// LeaveHelp tells the panel the manual is done with, and leaveSettings
// the same of the settings.
//
// Each says so rather than ending and letting the panel notice. The
// panel notices on its next reading, which is a second or two away and
// only happens at all while the processes view has the keys — so a
// page that just ended left a dead pane standing in the workspace,
// which is the one thing the workspace should never be showing.
func (s *Server) LeaveHelp() error     { return s.tellPanel(leaveHelpKey) }
func (s *Server) LeaveSettings() error { return s.tellPanel(leaveSettingsKey) }

// WearMode tells the panel the mode has changed under it: the settings
// have just written one and put it on the server, and the panel draws
// in colors of its own that it read when it came up.
//
// The panel is not respawned for it, the way the panes conn fills with
// furniture are. Respawning the panel is a fresh conn, which comes up
// on the console — the operator picked a theme and would be handed
// back the boot screen — so the panel reads the mode file again where
// it stands and wears what it now says.
func (s *Server) WearMode() error { return s.tellPanel(wearModeKey) }

// The keys a page of conn's own sends the panel. They are alt keys
// because the panel answers those wherever the keys are and whatever
// view it is in, and these three are not otherwise pressed: nobody
// reaches for alt-escape or alt-comma on a list of processes.
const (
	leaveHelpKey     = "M-Escape"
	leaveSettingsKey = "M-,"
	wearModeKey      = "M-w"
)

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
	if err == nil {
		_ = os.Remove(theme.ModePath(s.Socket))
	}
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
