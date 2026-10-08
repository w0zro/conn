// Package room is the tmux server conn holds of its own, as conn
// arranges it. The first conn
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
package room

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/w0zro/conn/internal/config"
	"github.com/w0zro/conn/internal/shell"
	"github.com/w0zro/conn/internal/tmux"
)

// New is conn's server on a tmux server.
func New(t *tmux.Server) *Server { return &Server{tmux: t, Socket: t.Socket} }

// A Server is conn's tmux server: tmux, as conn arranges it.
type Server struct {
	tmux   *tmux.Server
	Socket string
	// One change to the bay at a time. Each is a run of tmux commands
	// that reads the bay and then swaps against it, and two of them at
	// once — the page being put in as the operator opens a shell — each
	// read the bay before the other's swap landed, and the second swapped
	// against a pane the first had killed. The panel runs each off its
	// loop, so nothing but this keeps them apart.
	swaps sync.Mutex
}

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

// Find is the server as this machine has it: no tmux, no server.
func Find(home string) *Server {
	bin, err := exec.LookPath("tmux")
	if err != nil {
		return nil
	}
	return New(&tmux.Server{Tmux: bin, Socket: SocketPath(home)})
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

// Attach brings the server up if it is down, with the processes view
// window running conn, and puts this terminal on it until the client
// detaches or the server ends. It answers how the client exited; an
// error is one of its own, before the client had the terminal.
//
// conf is the configuration it comes up on, written beside the socket.
// bg, where it is given, is the surface of a mode the server already up
// is to be put on where it stands: tmux does not re-read -f on an
// attach, so that takes sourcing the configuration again; see Reground.
func (s *Server) Attach(self, home, conf, bg string) (int, error) {
	if err := os.MkdirAll(filepath.Dir(s.Socket), 0o700); err != nil {
		return 0, err
	}
	path := confPath(s.Socket)
	if err := os.WriteFile(path, []byte(conf), 0o600); err != nil {
		return 0, err
	}
	// A server on another build is relieved, which sources the
	// configuration and starts the panel again as a reground would.
	relieved, err := s.Relieve(path, self)
	if err != nil {
		return 0, err
	}
	if bg != "" && !relieved {
		if err := s.Reground(path, bg, "", self, true); err != nil {
			return 0, err
		}
	}
	if err := s.RestoreHome(home, self); err != nil {
		return 0, err
	}
	return s.tmux.AttachClient(path, SessionName, HomeWindow, home, "exec "+shell.Quote(self))
}

// Rewear puts the server into a mode conn has just taken on. It is
// reground for a conn already in the server: the mode is written down
// first, since the panes that come up again read it, and the panel is
// left standing — it is told to wear the mode where it stands, and
// killing it to change color would take the view the operator is
// working with it.
func (s *Server) Rewear(conf, bg, except, self string) error {
	if err := os.WriteFile(confPath(s.Socket), []byte(conf), 0o600); err != nil {
		return err
	}
	return s.Reground(confPath(s.Socket), bg, except, self, false)
}

// WriteConf writes the configuration for the server as it stands, in
// the mode it is in, for a conn that did not bring it up or attach to
// it: one typed in a pane of the station. It answers where.
func (s *Server) WriteConf(conf string) (string, error) {
	path := confPath(s.Socket)
	return path, os.WriteFile(path, []byte(conf), 0o600)
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
//
// self is the conn each pane is started again from. Left to itself,
// respawn-pane runs the command the pane first rose on, which is the
// path of whatever conn started it, and that path need not still be
// there: a conn run with go run is the build cache's, and Go trims the
// cache of what it has not run in days, so the pane came up dead with
// the shell saying the file was not found.
func (s *Server) Reground(conf, bg, except, self string, panel bool) error {
	if !s.tmux.Up() {
		return nil
	}
	if _, err := s.tmux.Do(tmux.SourceFile(conf)); err != nil {
		return err
	}
	panes, err := s.Panes()
	if err != nil {
		return err
	}
	if err := s.respawnOwn(panes, except, self); err != nil {
		return err
	}
	// The panel's pane is painted on the new ground's surface here as
	// well as by the conn that comes up in it, so the window is right
	// in the same breath as the rest and not a moment after.
	//
	if err := s.tmux.Paint(panelTarget, bg); err != nil {
		return err
	}
	if !panel {
		return nil
	}
	_, err = s.tmux.Do(tmux.Respawn(panelTarget, "exec "+shell.Quote(self)))
	return err
}

// respawnOwn starts every page of conn's own in the workspace again
// from self, each as the command it is — the readout, the manual, the
// settings or a hold — except the pane named, which asked and has
// seen to itself.
func (s *Server) respawnOwn(panes map[string]Pane, except, self string) error {
	for _, p := range panes {
		if p.Hold && p.ID != except {
			if _, err := s.tmux.Do(tmux.Respawn(p.ID, "exec "+shell.Quote(self)+" "+ownCommand(p))); err != nil {
				return err
			}
		}
	}
	return nil
}

// ownCommand is the command of conn's a page of its own runs, read off
// the page's marks.
func ownCommand(p Pane) string {
	switch {
	case p.Readout:
		return "readout"
	case p.Help:
		return "manual"
	case p.Settings:
		return "settings"
	}
	return "hold"
}

// The server is on a build: the binary every pane of conn's own runs,
// which the panel names on the server as it comes up, by Fingerprint.
// A conn of another build that reaches the server relieves it — puts
// its own panes on the new binary — and leaves the operator's alone, so
// an update is had by running the new conn, and not by conn down, which
// takes the work with it.
const (
	buildOption  = "@conn_build"
	resumeOption = "@conn_resume"
)

// Fingerprint names a binary by its contents. Its path will not do: an
// install writes the new conn over the old one's path, and go run puts
// an unchanged build back at the path it had.
func Fingerprint(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

// Build is the fingerprint the panel named on the server, blank where
// no panel has: a server brought up before conn named its builds.
func (s *Server) Build() string {
	out, _ := s.tmux.Global(buildOption)
	return out
}

// SayBuild is the panel naming the build it runs.
func (s *Server) SayBuild(print string) error {
	_, err := s.tmux.Do(tmux.SetGlobal(buildOption, print))
	return err
}

// Relieve puts the server on self where it runs another build. The
// configuration is sourced again, since a new build may write another;
// every page of conn's own is started again as the command it is; and
// the panel is started again told to resume, so it comes up on the
// processes view and not on the console. Every other pane — the work —
// is left running. It answers whether there was anything to relieve.
func (s *Server) Relieve(conf, self string) (bool, error) {
	print, err := Fingerprint(self)
	if err != nil {
		return false, err
	}
	if !s.tmux.Up() || s.Build() == print {
		return false, nil
	}
	if _, err := s.tmux.Do(tmux.SourceFile(conf)); err != nil {
		return false, err
	}
	panes, err := s.Panes()
	if err != nil {
		return false, err
	}
	if err := s.respawnOwn(panes, "", self); err != nil {
		return false, err
	}
	// A home that is gone is RestoreHome's to put back, and the conn it
	// starts is this one.
	if !s.hasHome() {
		return true, nil
	}
	_, err = s.tmux.Do(tmux.SetGlobal(resumeOption, "1"), tmux.Respawn(panelTarget, "exec "+shell.Quote(self)))
	return true, err
}

// Resuming says whether the panel coming up was started by Relieve, and
// clears it, so the panel started after it by anything else comes up
// on the console as it always has.
func (s *Server) Resuming() bool {
	out, err := s.tmux.Global(resumeOption)
	if err != nil || out != "1" {
		return false
	}
	_, _ = s.tmux.Do(tmux.UnsetGlobal(resumeOption))
	return true
}

// IsPanel says whether a pane, by id, is the panel's.
func (s *Server) IsPanel(pane string) bool {
	out, err := s.tmux.Display(panelTarget, "#{pane_id}")
	return err == nil && out == pane
}

// panelTarget is the panel's pane as tmux is asked for it from outside
// the server: the first pane of the home window, where conn put it.
var panelTarget = SessionName + ":" + HomeWindow + ".0"

// RestoreHome puts the home window back on a server that is up and has
// lost it, and starts the panel again where the window stands but the
// panel in it has died: once the bay has opened the window keeps a
// pane whose process has ended, so a panel that ended stays, dead, and
// an attach would land on it. Either starts from the conn attaching,
// the one binary known to be there, and not from what the pane first
// rose on; see Reground for why that may be gone.
func (s *Server) RestoreHome(home, self string) error {
	if !s.tmux.HasSession(SessionName) {
		return nil
	}
	if !s.hasHome() {
		return s.tmux.NewNamedWindow(SessionName, HomeWindow, home, "exec "+shell.Quote(self))
	}
	if !s.panelDead() {
		return nil
	}
	_, err := s.tmux.Do(tmux.Respawn(panelTarget, "exec "+shell.Quote(self)))
	return err
}

// panelDead says whether the panel's pane stands with its process gone.
func (s *Server) panelDead() bool {
	out, err := s.tmux.Display(panelTarget, "#{pane_dead}")
	return err == nil && out == "1"
}

// hasHome says whether the session has its home window.
func (s *Server) hasHome() bool {
	names, err := s.tmux.WindowNames(SessionName)
	if err != nil {
		return false
	}
	for _, name := range names {
		if name == HomeWindow {
			return true
		}
	}
	return false
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
	id, err := s.tmux.SplitRight(s.Panel(), home, "exec "+shell.Quote(self)+" hold")
	if err != nil {
		return err
	}
	if _, err := s.tmux.Do(tmux.SetPane(id, holdMark, "1")); err != nil {
		return err
	}
	// A pane in this window whose process ends stays instead of
	// closing, so a killed shell does not collapse the window to the
	// panel alone before conn re-splits it: the bay holds its project,
	// dead, until conn puts a hold there. It is the window's option
	// and not the server's, since the bay is the only place conn has a
	// layout to protect. Everywhere else a window whose work has ended
	// is a window that is done.
	if _, err := s.tmux.Do(tmux.SetWindow(id, "remain-on-exit", "on")); err != nil {
		return err
	}
	return s.HoldPanel()
}

// HoldPanel sets the panel to its width. tmux keeps the panes in
// proportion when the window is resized, so the panel is put back each
// time it is not its width.
func (s *Server) HoldPanel() error {
	_, err := s.tmux.Do(tmux.ResizeWidth(s.Panel(), PanelWidth))
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
	sh, err := s.tmux.NewWindow(home, "exec "+shell.Quote(self)+" hold")
	if err != nil {
		return err
	}
	hold := sh.Pane.ID
	if _, err := s.tmux.Do(tmux.SetPane(hold, holdMark, "1")); err != nil {
		return err
	}
	_, err = s.tmux.Do(tmux.Swap(hold, bay.ID), tmux.Kill(bay.ID))
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
	return s.showOwn(home, self, "readout", readoutMark, false)
}

// ShowHelp puts the manual in the workspace, with the keys in it: it is
// a page to be read, and a page that cannot be scrolled cannot be read.
// The panel says HELP while it stands, so where the keys have gone is
// not left to be guessed at.
func (s *Server) ShowHelp(home, self string) error {
	return s.showOwn(home, self, "manual", helpMark, true)
}

// ShowSettings puts the settings in the workspace, with the keys in it.
// The workspace is where conn puts what is being worked on, and the
// configuration is that while it is open: the panel is the list of
// what is running and has no room to be a form as well. The panel says
// SETTINGS while it stands and goes on reading the machine beside it.
func (s *Server) ShowSettings(home, self string) error {
	return s.showOwn(home, self, "settings", settingsMark, true)
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
	sh, err := s.tmux.NewWindow(home, "exec "+shell.Quote(self)+" "+cmd)
	if err != nil {
		return err
	}
	page := sh.Pane.ID
	for _, opt := range []string{holdMark, mark} {
		if _, err := s.tmux.Do(tmux.SetPane(page, opt, "1")); err != nil {
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
	cmds := []tmux.Cmd{tmux.Swap(page, bay.ID)}
	if bay.Width > 0 && bay.Height > 0 {
		cmds = append(cmds, tmux.ResizeWindow(bay.ID, bay.Width, bay.Height))
	}
	if bay.Hold {
		cmds = append(cmds, tmux.Kill(bay.ID))
	}
	if _, err := s.tmux.Do(cmds...); err != nil {
		return err
	}
	if keys {
		return s.tmux.Select(page)
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
	return s.show(target, true)
}

// Preview puts a pane in the bay and leaves the keys on the panel: the
// pane is being looked at, the way the readout is, and not gone into.
// A pane already in the bay is left as it is.
func (s *Server) Preview(target Pane) error {
	return s.show(target, false)
}

// show is Show and Preview: the swap, and the keys with it or not.
func (s *Server) show(target Pane, focus bool) error {
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
	var cmds []tmux.Cmd
	if target.ID != bay.ID {
		cmds = append(cmds, tmux.Swap(target.ID, bay.ID))
		if bay.Width > 0 && bay.Height > 0 && !inHome {
			cmds = append(cmds, tmux.ResizeWindow(bay.ID, bay.Width, bay.Height))
		}
		if bay.Hold || bay.Dead {
			cmds = append(cmds, tmux.Kill(bay.ID))
		}
	}
	if focus {
		cmds = append(cmds, tmux.SelectPane(target.ID))
	}
	if len(cmds) == 0 {
		return nil
	}
	_, err = s.tmux.Do(cmds...)
	return err
}

// Open opens a shell at a directory, in a window of its own, and shows
// it in the bay.
func (s *Server) Open(dir string) (Shell, error) {
	return s.OpenCmd(dir, "")
}

// OpenCmd is open, running a command instead of the directory's own
// shell — what a opens claude with.
func (s *Server) OpenCmd(dir, cmd string) (Shell, error) {
	sh, err := s.open(dir, cmd)
	if err != nil {
		return Shell{}, err
	}
	return sh, s.Show(sh.Pane)
}

// OpenWatching opens a pane conn made to watch a container, marked with
// the container it is watching. The mark is what makes the pane the
// terminal the container's row stands on, and what keeps the watcher
// itself off the view: a docker logs listed beside the service it is
// showing would be the same thing twice.
func (s *Server) OpenWatching(dir, cmd, id string) (Shell, error) {
	sh, err := s.openMarked(dir, cmd, containerMark, id)
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
	sh, err := s.openMarked(dir, cmd, shellInMark, id)
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
//
// command is the declaration's own, as written, and name what it is
// called; the pane runs declaredLine around it.
func (s *Server) RaiseDeclared(dir, command, name, mark, replace string, show bool) (Shell, error) {
	if replace != "" {
		_, _ = s.tmux.Do(tmux.Kill(replace))
	}
	sh, err := s.openMarked(dir, declaredLine(command, name, s.tmux.Tmux), declaredMark, mark)
	if err != nil {
		return Shell{}, err
	}
	sh.Pane.Declared = mark
	if show {
		return sh, s.Show(sh.Pane)
	}
	return sh, nil
}

// declaredLine is what runs in a declaration's pane: the command as
// written, then the pane told how it ended, a word of that for the
// operator, and the hold, so the last output stays up to be read. On
// lines of their own, not after semicolons: a comment or an & on the
// end of the operator's line would otherwise take the rest with it.
// tmux tells a pane its own id in TMUX_PANE, and is told it back: a
// client with no terminal is otherwise pointed at whichever pane the
// server counts as current, which is not this one.
func declaredLine(command, name, bin string) string {
	return command + "\n" +
		shell.Quote(bin) + " set-option -p -t \"$TMUX_PANE\" " + exitMark + " \"$?\"\n" +
		"printf '\\n[" + name + " exited]\\n'\n" +
		shell.HoldOpen
}

// openMarked opens a pane running a command and sets one option on it,
// which is how conn remembers what it opened a pane for.
func (s *Server) openMarked(dir, cmd, option, value string) (Shell, error) {
	sh, err := s.open(dir, cmd)
	if err != nil {
		return Shell{}, err
	}
	if _, err := s.tmux.Do(tmux.SetPane(sh.Pane.ID, option, value)); err != nil {
		return Shell{}, err
	}
	return sh, nil
}

// Wide gives the panel the whole window, which is what the console
// wants: it is a page, not a panel. Narrow gives the bay its side back.
// tmux has one key for both, so each looks first at how the window
// stands; on a home with no bay yet there is nothing to zoom and both
// are nothing.
func (s *Server) Wide() error { return s.zoom(true) }

func (s *Server) Narrow() error { return s.zoom(false) }

func (s *Server) zoom(on bool) error {
	out, err := s.tmux.Display(s.Panel(), "#{window_zoomed_flag}")
	if err != nil {
		return err
	}
	if (out == "1") == on {
		return nil
	}
	_, err = s.tmux.Do(tmux.ToggleZoom(s.Panel()))
	return err
}

// FocusPanel puts focus on the panel.
func (s *Server) FocusPanel() error {
	return s.tmux.Select(s.Panel())
}

// A Dress is what the server wears, as conn chooses it: the colors
// the panes, the copy mode and the line are drawn in, as hexes; the
// sixteen a program asks for by name; and what the band and the key bar
// say while a pane is in copy mode, already styled. The choosing is
// conn's; this package only writes it down in tmux's words.
type Dress struct {
	Ground, Ink, Accent, Border, Gray, Surface string
	Block, OnBlock                             string // the accent as a fill, and the letters on it
	Scheme                                     []string
	CopyBand, CopyBar                          string
}

// Conf is the server's configuration: the panel key, and how
// every pane is drawn, in a dress. tmux has no prefix here, so none of its keys or
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
func Conf(key string, d Dress, env []string) string {
	return confKeys(key) + confTerminal() + confEnvironment(env) + confLook(d) + statusLine(d)
}

// confKeys is the server's keys: the one key it takes for conn, the
// mouse, and copy mode's.
func confKeys(key string) string {
	return `# conn's tmux server. Written by conn on each start; edits do not keep.
# One key, from anywhere in the station: to the panel, which says where
# the keys came from. No prefix, so nothing of tmux's own is reachable,
# and the panel or the process answers every other key. There is no key
# for the page: in the processes view the page is what the workspace
# holds, and nothing is pressed for it.
set -g prefix None
set -g prefix2 None
bind -n ` + key + ` set -gF @conn_from "#{pane_id}" \; select-pane -t ` + SessionName + ":" + HomeWindow + `.0 \; send-keys -t ` + SessionName + ":" + HomeWindow + `.0 ` + Arrived.key + `
set -g mouse on
# The panel's width is conn's to hold; a drag of the border would only be
# put back.
unbind -n MouseDrag1Border
set -g mode-keys vi
# Copy mode selects and copies as vim does, which is what the bar says:
# v a selection, V lines, ctrl-v a block, y copied. tmux's own vi table
# begins a selection on space and copies on enter, has v toggle a block
# without beginning anything, and leaves y unbound. A copy goes to the
# terminal's clipboard by set-clipboard.
bind -T copy-mode-vi v send-keys -X begin-selection
bind -T copy-mode-vi C-v send-keys -X rectangle-toggle
bind -T copy-mode-vi y send-keys -X copy-pipe-and-cancel
`
}

// confTerminal is how the server meets the terminal it is drawn on.
func confTerminal() string {
	return `set -g history-limit 10000
set -g window-size latest
set -g set-clipboard on
set -g set-titles on
set -g set-titles-string "conn"
set -g escape-time 10
set -g focus-events on
set -g default-terminal tmux-256color
set -as terminal-features ",*:RGB"
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
`
}

// confEnvironment is what a program in a pane is told: that the colors
// are true, that it is in conn, and whatever env says besides - a
// program's own word for something the server cannot say in general,
// as NAME=VALUE.
func confEnvironment(env []string) string {
	var b strings.Builder
	b.WriteString(`set-environment -g COLORTERM truecolor
# A program in a pane can tell it is in conn, and dress accordingly.
set-environment -g CONN 1
`)
	for _, kv := range env {
		name, value, _ := strings.Cut(kv, "=")
		fmt.Fprintf(&b, "set-environment -g %s %s\n", name, value)
	}
	return b.String()
}

// confLook is how every pane is drawn, in a dress.
func confLook(d Dress) string {
	var b strings.Builder
	ground, ink := d.Ground, d.Ink
	fmt.Fprintf(&b, "set -g window-style \"bg=%s,fg=%s\"\n", ground, ink)
	fmt.Fprintf(&b, "set -g cursor-colour \"%s\"\n", d.Accent)
	// The cursor keeps the accent in copy mode. tmux draws a pane in a
	// mode from a screen of the mode's own, which carries no cursor
	// colour, and sends the terminal a reset the moment the mode comes
	// on: the cursor went to whatever the terminal's own colour is for
	// as long as the pane was in copy mode. The option is applied to
	// whichever screen a pane is showing at the time, so it is applied
	// again, a beat after the mode has changed, when the mode's screen
	// is the one showing, and unset again so no pane carries a colour
	// of its own past the theme's. Deferred, since the hook fires before
	// the mode's screen is in place; a beat of nothing is the next turn
	// of tmux's loop, after the reset has gone. tmux then sends the
	// accent itself, from its own account of the terminal. Established
	// against tmux 3.5a with a client logged under a pty.
	fmt.Fprintf(&b, "set-hook -g pane-mode-changed \"run-shell -b -d 0 -C \\\"set -p -t '#{hook_pane}' cursor-colour '%s' ; set -pu -t '#{hook_pane}' cursor-colour\\\"\"\n", d.Accent)
	fmt.Fprintf(&b, "set -g mode-style \"bg=%s,fg=%s\"\n", d.Border, ink)
	// A search's matches, in copy mode: every saying of the text in a
	// quiet mark, and the one the cursor is on in the accent's block,
	// the way the cursor is everywhere else. tmux's own are a cyan and
	// a magenta, which would be the two colors on the station that are
	// nobody's.
	fmt.Fprintf(&b, "set -g copy-mode-match-style \"bg=%s,fg=%s\"\n", d.Gray, ground)
	fmt.Fprintf(&b, "set -g copy-mode-current-match-style \"bg=%s,fg=%s,bold\"\n", d.Block, d.OnBlock)
	for i, c := range d.Scheme {
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
func statusLine(d Dress) string {
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
	fmt.Fprintf(&b, "set -g status-style \"bg=%s,fg=%s\"\n", d.Border, d.Gray)
	// Two rows across the foot. The upper is the band: a mode tmux knows
	// itself first — COPY in copy mode — then where the keys are, by conn's word, while the keys are on the
	// panel, and the station's word when they are not; and at the right
	// edge the clock. The lower is the key bar: the keys that work where
	// the cursor is, or a question armed, and at the right the station's
	// designation.
	onPanel := fmt.Sprintf("#{&&:#{==:#{window_name},%s},#{==:#{pane_index},0}}", HomeWindow)
	fmt.Fprintf(&b, "set -g status-left \"#{?pane_in_mode,%s,#{?%s,#{@conn_keys},#{@conn_station}}}\"\n",
		d.CopyBand, onPanel)
	b.WriteString("set -g status-right \"#{@conn_up}\"\n")
	b.WriteString("set -g status-format[0] \"#[align=left]#{T:status-left}#[align=right]#{T:status-right}\"\n")
	// The key bar is on the surface, the panel's own ground, so the two
	// rows are two things: the band the window's frame, the bar the
	// panel's footer.
	//
	// While a pane is in copy mode the bar says copy mode's keys, since
	// the keys in the pane are tmux's and not the process's, and a
	// hand that has not learned them is in a screen it cannot scroll
	// or leave. It goes by tmux's own word for the mode, as the band
	// does, so a pane scrolled with the wheel says its keys too.
	fmt.Fprintf(&b, "set -g status-format[1] \"#[fill=%s bg=%s]#{?pane_in_mode,%s,#{@conn_bar}}#[align=right]#{@conn_ident}\"\n",
		d.Surface, d.Surface, d.CopyBar)
	return b.String()
}

// Say puts what conn knows about its own keys on the server, and asks
// the clients to draw, so the status line never lags what changed it.
func (s *Server) Say(keys, station, up, bar, ident string) error {
	_, err := s.tmux.Do(tmux.SetGlobal("@conn_keys", keys), tmux.SetGlobal("@conn_station", station),
		tmux.SetGlobal("@conn_up", up), tmux.SetGlobal("@conn_bar", bar), tmux.SetGlobal("@conn_ident", ident),
		tmux.RefreshStatus())
	return err
}

// SayBand is say without the key bar, for the panel while a page of
// conn's own has the keys and is writing that position itself.
func (s *Server) SayBand(keys, station, up, ident string) error {
	_, err := s.tmux.Do(tmux.SetGlobal("@conn_keys", keys), tmux.SetGlobal("@conn_station", station),
		tmux.SetGlobal("@conn_up", up), tmux.SetGlobal("@conn_ident", ident),
		tmux.RefreshStatus())
	return err
}

// SayBar is the key bar alone, for a page of conn's own that has the
// keys: what the keys do there is its own to say, since the bar says
// the keys that work where the cursor is and the cursor is in that
// pane. The panel leaves the position alone while such a page stands;
// see saying in tui.go.
func (s *Server) SayBar(bar string) error {
	_, err := s.tmux.Do(tmux.SetGlobal("@conn_bar", bar), tmux.RefreshStatus())
	return err
}

// tellPanel sends the panel a key. A page of conn's own is a conn in a
// pane of its own, and the only way it has to speak to the panel is
// the way the panel key does: a key, sent to it.
//
// The keys it sends are the signals the panel answers to and nothing
// else does.
func (s *Server) tellPanel(sig Signal) error {
	_, err := s.tmux.Do(tmux.SendKeys(panelTarget, sig.key))
	return err
}

// CameFrom is the pane the keys were in when the panel key brought
// them to the panel, read once and cleared; nothing where they were on
// the panel already. The key writes it as it fires, in its binding in
// Conf.
func (s *Server) CameFrom() string {
	from, err := s.tmux.Global(fromOption)
	if err != nil {
		return ""
	}
	_, _ = s.tmux.Do(tmux.UnsetGlobal(fromOption))
	if from != s.Panel() {
		return from
	}
	return ""
}

// fromOption is where the panel key writes the pane it was pressed in.
const fromOption = "@conn_from"

// LeaveHelp tells the panel the manual is done with, and leaveSettings
// the same of the settings.
//
// Each says so rather than ending and letting the panel notice. The
// panel notices on its next reading, which is a second or two away and
// only happens at all while the processes view has the keys — so a
// page that just ended left a dead pane standing in the workspace,
// which is the one thing the workspace should never be showing.
func (s *Server) LeaveHelp() error { return s.tellPanel(LeftHelp) }

func (s *Server) LeaveSettings() error { return s.tellPanel(LeftSettings) }

// WearMode tells the panel the mode has changed under it: the settings
// have just written one and put it on the server, and the panel draws
// in colors of its own that it read when it came up.
//
// The panel is not respawned for it, the way the panes conn fills with
// furniture are. Respawning the panel is a fresh conn, which comes up
// on the console — the operator picked a theme and would be handed
// back the boot screen — so the panel reads the mode file again where
// it stands and wears what it now says.
func (s *Server) WearMode() error { return s.tellPanel(ModeChanged) }

// A Signal is a word said to the panel as a key: by the panel key, as
// it brings the keys there, and by a page of conn's own. Each is an alt
// key, because the panel answers those wherever the keys are and
// whatever view it is in, and none is otherwise pressed: nobody reaches
// for alt-escape or alt-comma on a list of processes. A signal is
// spelled twice, as tmux sends the key and as the panel's loop hears
// it, and both spellings are here so that the two cannot part.
type Signal struct{ key, heard string }

// The signals: the keys arrived on the panel; the manual and the
// settings left; the mode changed under the panel.
var (
	Arrived      = Signal{"M--", "alt+-"}
	LeftHelp     = Signal{"M-Escape", "alt+esc"}
	LeftSettings = Signal{"M-,", "alt+,"}
	ModeChanged  = Signal{"M-w", "alt+w"}
)

// Heard is the signal as the panel's loop hears it.
func (s Signal) Heard() string { return s.heard }

// ContactNote is what conn tells a contact it starts about where it is,
// on the server at a socket: how to put work in a window of its own.
// A contact that backgrounds a dev server leaves it with no terminal:
// no pane to attach to, no scrollback to read, and its output wherever
// the contact happened to send it. A window of its own costs the
// contact nothing and makes the server a process like any other here —
// conn shows it, you reach it, and its log is the pane you are looking
// at. The contact is told to keep the pane's id rather than the
// window's name, because conn swaps panes into the workspace and a
// window's name stays where it was: a respawn-window aimed at the name
// once killed the session sitting in the workspace before the swap.
//
// conn says this to the contacts it starts rather than writing it into
// anybody's settings. It travels with conn, so a conn on another
// machine tells its contacts the same thing, and a machine conn is gone
// from is as conn found it.
func ContactNote(socket string) string {
	return "You are running inside conn, which holds this terminal as a tmux pane " +
		"and watches the processes working this project. Start anything long-lived " +
		"— a dev server, a file watcher, a build that stays up — in a window of its " +
		"own rather than detached in the background:\n\n" +
		"  tmux -S " + socket + " new-window -d -P -F '#{pane_id}' -n NAME -c DIR 'COMMAND'\n\n" +
		"It then holds a terminal of its own, so conn lists it as a process " +
		"of its own, it can be attached to, and its output is the pane's scrollback. " +
		"The command prints the pane's id, %N; read, restart and stop it by that id:\n\n" +
		"  tmux -S " + socket + " capture-pane -p -t %N\n" +
		"  tmux -S " + socket + " respawn-pane -k -t %N\n" +
		"  tmux -S " + socket + " kill-pane -t %N\n\n" +
		"Never by the window's name: conn moves panes between windows to show them, " +
		"so the name can come to hold another process, and a command sent to it lands there.\n\n" +
		"Something you background instead holds no terminal and has no pane, and can " +
		"only be read through whatever file its output was sent to."
}
