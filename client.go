package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/charmbracelet/x/ansi"
)

// conn's side of the server. It never touches a pty and never draws a
// buffer: tmux does both, in the pane under the tabline. This file is the
// session that talks to the server: asking what it holds, moving the
// buffer to show into the pane under the tabline and the last one back
// out, moving focus to a buffer, and hearing when buffers come and go.

// socketPath is where conn's tmux server listens. It is per user and outside
// any project, because one server holds the shells for every repository. It
// honors XDG_STATE_HOME, and falls back to ~/.local/state; the config
// honors XDG_CONFIG_HOME.
func socketPath() string {
	if p := os.Getenv("CONN_SOCKET"); p != "" {
		return p
	}
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		home, _ := os.UserHomeDir()
		state = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(state, "conn", "tmux-"+strconv.Itoa(os.Getuid())+".sock")
}

// sessionInfo is a shell the server is holding. Name is what the project
// that asked for it calls it, and empty for a shell opened by hand. Shown
// says it is the buffer in the pane under the tabline; Wanted that a chord
// opened it and asked for it to be shown.
type sessionInfo struct {
	PID   int
	Dir   string
	Name  string
	Run   string // the command the shell was started with; empty for a shell opened by hand
	Exit  string // how the command the shell was started with ended, once it has
	Ended string // when, as seconds since the epoch
	// Summary is what the transcript said of the run, and Recorded that the
	// ending is on the record: both kept on the pane once known, so a
	// navigator starting beside the shell neither reads nor records the
	// ending again.
	Summary  string
	Recorded bool
	Shown    bool
	Wanted   bool
}

// remoteTerm is a shell the server is holding, as the navigator sees it.
type remoteTerm struct {
	pid  int
	dir  string
	name string    // what the project calls it, if a project asked for it
	run  string    // the command it was started with, which is the run's identity; the name is its label
	exit string    // how the command it was started with ended, once it has: "0", "1"…
	at   time.Time // when it ended, when the pane recorded that too
	// summary is what its transcript said of the run, read once it ended
	// and the shell was at its prompt: 3 failed, 12 passed.
	summary string

	// ended orders the exits the navigator has learned of, so the latest
	// of several shells for one entry is the one that speaks for it.
	// settled says the exit has been seen with the shell at its prompt;
	// dropped says the shell was used by hand after, and its exit is
	// history — not to be learned again from a list that still carries it.
	ended   int
	settled bool
	dropped bool
	// recorded says the ending has been written to the runs file, which
	// happens once, when the transcript has been read for what it said.
	recorded bool
	// hangUp says a kill asked for the shell as well as what ran in it:
	// once nothing runs in it any more it is hung up, buffer and all.
	hangUp bool
	// killed says the ending was asked for with x: an exit you asked for
	// is not a wrong one, and carries no mark.
	killed bool
}

// live reports whether the shell is still running what it was started
// with — or was started with nothing, and is a shell for its own sake.
func (t *remoteTerm) live() bool { return t.exit == "" }

// learn takes what a later report says of a shell already known: where it
// was opened and what the project called it. The window is made and its
// options set in two commands, and tmux announces the window between them,
// so the first list of a new shell can carry neither; the reports that
// know come after. A blank is a report that came early, not a shell that
// lost its name, and is not taken. The model learns how the command
// ended, since it orders the endings.
func (t *remoteTerm) learn(dir, name, run string) {
	if dir != "" {
		t.dir = dir
	}
	if name != "" {
		t.name = name
	}
	if run != "" {
		t.run = run
	}
}

// Messages the session raises for the model.
type (
	// serverReadyMsg says the session is up, or explains why it is not.
	serverReadyMsg struct {
		session *session
		err     error
	}

	// termOpenedMsg is the shell this navigator just asked for.
	termOpenedMsg struct {
		pid  int
		dir  string
		name string
		run  string
	}

	// sessionsMsg is the shells the server is holding.
	sessionsMsg struct {
		sessions []sessionInfo
	}

	// termGoneMsg says a shell has finished.
	termGoneMsg struct{ pid int }

	// serverLostMsg says the server hung this session up. With no error it is
	// the ordinary end of holding nothing — the last shell closed, the
	// session went, and the session object keeps watching for a new one.
	serverLostMsg struct{}

	// serverErrorMsg says the server could not do something it was asked to.
	// This is a report about one ask, not about the server.
	serverErrorMsg struct{ err error }
)

// reconnectMsg asks for another go at connecting, for the one failure the
// session cannot chase from inside: tmux itself missing.
type reconnectMsg struct{}

const (
	reconnectWait = 300 * time.Millisecond
	reconnectMax  = 5 * time.Second
)

// reconnect connects again after the wait.
func reconnect(after time.Duration) tea.Cmd {
	return tea.Tick(after, func(time.Time) tea.Msg { return reconnectMsg{} })
}

// probeEvery is how often a session with no server watches for one to
// appear — another window may hold shells this one cannot see yet.
const probeEvery = 2 * time.Second

// pane is one held shell as the session tracks it: which tmux pane it is,
// and what it is called.
type pane struct {
	id       string // "%3"
	pid      int
	dir      string
	name     string
	run      string // the command the shell was started with, recorded on the pane at open
	exit     string // the command's exit status, recorded on the pane when it ended
	ended    string // when it ended, as seconds since the epoch, recorded with it
	summary  string // what the transcript said of the run, once the navigator read it
	recorded string // "1" once the ending is on the record
	cmd      string // what is in the pane's foreground: the command, or the shell at its prompt
	shown    bool   // in the home window, under the tabline
	wanted   bool   // opened by a chord that asked for it to be shown
}

// placement is one request to arrange the home window: the buffer to put
// under the tabline — none, to leave conn the whole window — and whether
// focus should go to it.
type placement struct {
	pid   int
	focus bool
}

// session is the navigator's connection to the tmux server holding the
// shells.
type session struct {
	events chan tea.Msg

	// run is one tmux command against conn's server; a seam for the tests,
	// which put a recorder here.
	run func(args ...string) (string, error)

	mu      sync.Mutex
	ctl     *ctlClient
	panes   map[int]*pane      // by the pid of the shell in the pane
	byPane  map[string]int     // pane id → that pid
	nav     string             // conn's own pane, "%0"
	placing latest[placement]  // the arrangement asked for, made one at a time
	saying  latest[statusText] // what the status line is to read, said one at a time
	listing sync.Mutex         // one list is read and told at a time, so the newer is heard last
	probing bool
	closed  bool
	waiting bool          // a waiter is listening for endings on the server
	stopped chan struct{} // closed by close, so a watcher stops in its tracks

	// probe is how long the watch waits between looks for a server; the
	// tests set a pace that suits a test.
	probe time.Duration

	// settle is how long after a window is announced the list is read a
	// second time: the client that made the window dresses it — its
	// directory, its name — right after, and tmux announces an option to
	// nobody. The tests set a pace that suits a test.
	settle time.Duration
}

func newSession() *session {
	return &session{
		events:  make(chan tea.Msg, 64),
		run:     tmuxCommand,
		panes:   map[int]*pane{},
		byPane:  map[string]int{},
		probe:   probeEvery,
		settle:  windowSettle,
		stopped: make(chan struct{}),
	}
}

// windowSettle is how long a window's maker gets to dress it before the
// list is read again: the options are set in the command after the
// window's making, milliseconds later on a machine that is not busy.
const windowSettle = time.Second

// connectServer builds the session and has it find whatever is already held.
// The one unrecoverable failure is tmux not being installed: everything else
// the session chases on its own.
func connectServer() tea.Cmd {
	return func() tea.Msg {
		if _, err := exec.LookPath("tmux"); err != nil {
			return serverReadyMsg{err: errors.New("tmux is not installed")}
		}
		s := newSession()
		go s.connect()
		return serverReadyMsg{session: s}
	}
}

// connect attaches to a session that exists, or begins watching for one —
// shells opened by another window appear on their own.
func (s *session) connect() {
	if s.refreshList() {
		s.ensureCtl()
		return
	}
	s.watchForServer()
}

// watchForServer polls for a session appearing, one probe at a time. It is
// the quiet state of a navigator holding nothing.
func (s *session) watchForServer() {
	s.mu.Lock()
	if s.probing || s.closed {
		s.mu.Unlock()
		return
	}
	s.probing = true
	s.mu.Unlock()

	go func() {
		tick := time.NewTicker(s.probe)
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
			case <-s.stopped:
			}
			// Whether the probe is over is decided in the same breath as
			// the probing flag is put down. A control client that hangs up
			// between the two — attached to a session that went in the
			// same instant — would ask for a probe while this one still
			// claimed to be running, and then nobody would be probing.
			s.mu.Lock()
			done := s.closed || s.ctl != nil
			if done {
				s.probing = false
			}
			s.mu.Unlock()
			if done {
				return
			}
			if _, err := s.run("has-session", "-t", tmuxSession); err == nil {
				s.refreshList()
				s.ensureCtl()
				// Not the end of the probe: the next pass is what sees
				// whether the client stayed attached.
			}
		}
	}()
}

// close lets go of the server. The shells stay held — this is a navigator
// finishing with them, not an end to them — and the session stops watching,
// stops probing, and will not attach again.
func (s *session) close() {
	s.mu.Lock()
	if !s.closed {
		close(s.stopped)
	}
	s.closed = true
	ctl := s.ctl
	s.ctl = nil
	s.mu.Unlock()
	if ctl != nil {
		ctl.close()
	}
}

// ensureCtl attaches the control-mode client if it is not already attached.
func (s *session) ensureCtl() {
	s.mu.Lock()
	if s.ctl != nil || s.closed {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	ctl, err := startCtl(s.notify)
	if err != nil {
		s.watchForServer()
		return
	}
	s.mu.Lock()
	closed := s.closed
	if !closed {
		s.ctl = ctl
	}
	s.mu.Unlock()
	if closed {
		ctl.close()
		return
	}
	s.watchEndings()
}

// notify is what the control stream tells the session.
func (s *session) notify(n ctlNote) {
	switch n.kind {
	case noteWindows:
		// Now, and again once the window's maker has dressed it: a window
		// made by another client — a chord, conn run from a shell — is
		// announced before its directory and name are set, and nothing is
		// announced when they are. A navigator that read the first list
		// alone saw a bare shell for good.
		go s.refreshList()
		go func() {
			select {
			case <-time.After(s.settle):
				s.refreshList()
			case <-s.stopped:
			}
		}()
	case noteError:
		s.events <- serverErrorMsg{err: errors.New(n.err)}
	case noteExit:
		s.mu.Lock()
		if s.ctl != nil {
			s.ctl.close()
			s.ctl = nil
		}
		s.mu.Unlock()
		s.events <- serverLostMsg{}
		s.watchForServer()
	}
}

// listFormat is the one line per pane the session reads the server's state
// through. The directory a shell was opened in is a pane option conn set;
// a pane conn never dressed — opened through a bare tmux attach — falls
// back to where it is working now. The navigator's own pane is told apart
// by its option, and a pane in the home window beside it is the shell
// shown there; the home window's option reaches its panes. A chord that
// opened a shell to be shown says so in the window's name, the one mark
// that is set in the same breath as the window is made — an option set
// after would race the refresh the new window sets off.
const listFormat = "#{pane_id}\t#{pane_pid}\t#{@conn_dir}\t#{@conn_name}\t#{pane_current_path}\t#{@conn_nav}\t#{@conn_home}\t#{window_name}\t#{@conn_exit}\t#{@conn_ended}\t#{pane_current_command}\t#{@conn_run}\t#{@conn_summary}\t#{@conn_recorded}"

// wantName is the window name that asks the navigator to show the shell
// in it; heldName is what the window is called once it has.
const (
	wantName = "conn-want"
	heldName = "shell"
)

// parseListing reads what list-panes said in listFormat: one record per
// shell, and the navigator's own pane, "" when there is none. Both readers
// of the server's state come through here, so the format is written once
// and read once.
func parseListing(out string) (held []*pane, nav string) {
	for line := range strings.SplitSeq(out, "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 8 {
			continue
		}
		if f[5] == "1" {
			nav = f[0]
			continue
		}
		pid, err := strconv.Atoi(f[1])
		if err != nil {
			continue
		}
		// A pane conn never dressed reports no directory of its own, and
		// falls back to where it is working now.
		dir := f[2]
		if dir == "" {
			dir = f[4]
		}
		p := &pane{id: f[0], pid: pid, dir: dir, name: f[3],
			shown: f[6] == "1", wanted: f[6] != "1" && f[7] == wantName}
		if len(f) > 8 {
			p.exit = f[8]
		}
		if len(f) > 9 {
			p.ended = f[9]
		}
		if len(f) > 10 {
			p.cmd = f[10]
		}
		if len(f) > 11 {
			p.run = f[11]
		}
		if len(f) > 13 {
			p.summary, p.recorded = f[12], f[13]
		}
		held = append(held, p)
	}
	return held, nav
}

// info is the pane as the model hears about it.
func (p *pane) info() sessionInfo {
	return sessionInfo{PID: p.pid, Dir: p.dir, Name: p.name, Run: p.run, Exit: p.exit, Ended: p.ended,
		Summary: p.summary, Recorded: p.recorded == "1", Shown: p.shown, Wanted: p.wanted}
}

// refreshList reads what the server holds and tells the model, reporting
// whether a session was there to read. Shells that left are named going:
// the model clears its rows by the pid, not by the list.
func (s *session) refreshList() bool {
	// Held from the read to the telling: two reads in flight, each told
	// after its own unlock, could reach the model in the other's order,
	// and the model takes the list it hears last as the truth.
	s.listing.Lock()
	defer s.listing.Unlock()

	out, err := s.run("list-panes", "-a", "-F", listFormat)
	if err != nil && !errors.Is(err, errNoServer) {
		// A list that could not be read says nothing about the shells: a
		// tmux slow enough to time out under load is still holding them.
		// The last list stands, and the model hears nothing until the next
		// one is read.
		s.mu.Lock()
		held := len(s.panes) > 0
		s.mu.Unlock()
		return held
	}
	if err != nil {
		// No server is the one answer that means every shell is gone.
		s.mu.Lock()
		was := s.panes
		s.panes, s.byPane, s.nav = map[int]*pane{}, map[string]int{}, ""
		s.mu.Unlock()
		for pid := range was {
			s.events <- termGoneMsg{pid: pid}
		}
		s.events <- sessionsMsg{}
		return false
	}

	held := map[int]*pane{}
	byPane := map[string]int{}
	var infos []sessionInfo
	listed, nav := parseListing(out)
	for _, p := range listed {
		held[p.pid] = p
		byPane[p.id] = p.pid
		infos = append(infos, p.info())
	}

	// Which shells left is worked out under the lock and carried out as a
	// list of pids. Reading the new map after publishing it is reading a map
	// an open may be writing to in the same breath.
	var gone []int
	s.mu.Lock()
	for pid := range s.panes {
		if _, ok := held[pid]; !ok {
			gone = append(gone, pid)
		}
	}
	s.panes, s.byPane, s.nav = held, byPane, nav
	s.mu.Unlock()

	for _, pid := range gone {
		s.events <- termGoneMsg{pid: pid}
	}
	s.events <- sessionsMsg{sessions: infos}
	return true
}

// nextEvent waits for the session's next word. Exactly one of these is in
// flight, the same discipline the refresh tick keeps: each schedules the next.
func nextEvent(s *session) tea.Cmd {
	if s == nil {
		return nil
	}
	return func() tea.Msg {
		return <-s.events
	}
}

// paneBirth is what an open asks for back: the pane and the shell's pid.
const paneBirth = "#{pane_id} #{pane_pid}"

// runner is one tmux command against conn's server: a session's seam, or
// tmuxCommand itself for a one-shot caller.
type runner func(args ...string) (string, error)

// birth is a shell just opened: its pane, and the pid of the shell in it.
type birth struct {
	pane string
	pid  int
}

// createWindow opens a window in conn's session holding a shell — or, handed
// a command, that command with a shell waiting behind it — in dir, and pins
// the name and the opening directory on its pane so the list can tell a
// plan's web apart from a shell that wandered there. The window is where
// the shell waits until the navigator shows it; wanted asks the navigator
// to, as soon as it sees the window. The launcher is what brings the
// server up; a session that has gone in the meantime is made again around
// the first shell, without a home window, which `conn home` supplies when
// it is next asked for.
func createWindow(run runner, dir, command, name string, wanted bool) (birth, error) {
	cmd := ""
	if command != "" {
		// Under a shell, so the command is found on the user's own PATH,
		// and one that exits leaves the shell rather than the row
		// vanishing. The shell is the pane's own $SHELL, which tmux sets
		// to its default-shell whatever the asker's environment says: a
		// chord runs under run-shell, where SHELL is /bin/sh. How the
		// command ended is recorded on the pane as it does, for the
		// navigator to read: inside the pane, tmux finds its server and
		// the pane by the environment tmux gave it.
		cmd = command + recordExit() + `; exec "$SHELL"`
	}
	winName := heldName
	if wanted {
		winName = wantName
	}

	args := []string{"new-window", "-d", "-P", "-t", tmuxSession + ":",
		"-F", paneBirth, "-n", winName, "-c", dir}
	if _, err := run("has-session", "-t", tmuxSession); err != nil {
		// The session is made again around a placeholder, and the shell
		// opens in a new window of it, not the first: a session's first
		// window cannot carry the terminal's name, and a new window can. The
		// placeholder is a sleep, not a shell, and goes once the shell is
		// there. Two askers finding no session in the same instant make
		// one between them, and the other's shell still opens in it.
		_ = os.MkdirAll(filepath.Dir(socketPath()), 0o700)
		_, err := run("new-session", "-d", "-s", tmuxSession, "-n", bornName, "-c", "/", "sleep 30")
		if err == nil {
			defer func() { _, _ = run("kill-window", "-t", tmuxSession+":"+bornName) }()
		} else if !errors.Is(err, errDuplicateSession) {
			return birth{}, err
		}
	}
	// The pane is told which terminal is around tmux, for the programs
	// that speak to one. tmux sets the names itself, after the session's
	// environment, so they go with the window rather than the server —
	// which is asked once it is there to ask.
	for _, kv := range outerTerminal(run) {
		args = append(args, "-e", kv)
	}
	if cmd != "" {
		args = append(args, cmd)
	}

	out, err := run(args...)
	if err != nil {
		return birth{}, err
	}
	f := strings.Fields(out)
	if len(f) != 2 {
		return birth{}, errors.New("tmux said " + out)
	}
	pid, err := strconv.Atoi(f[1])
	if err != nil {
		return birth{}, errors.New("tmux said " + out)
	}
	// The pane is told where it was opened, what the plan calls it, and
	// what it was started with: the command is the run's identity, kept on
	// the pane so every reader — the navigator, conn ls, the runs file —
	// says the same, and a plan that renames or changes the entry does
	// not change what this shell ran.
	_, _ = run("set", "-p", "-t", f[0], "@conn_dir", dir, ";",
		"set", "-p", "-t", f[0], "@conn_name", name, ";",
		"set", "-p", "-t", f[0], "@conn_run", command)
	return birth{pane: f[0], pid: pid}, nil
}

// recordExit is the shell fragment that sets the pane's exit option to the
// status of the command before it, and the ended option to the moment,
// then says so on the endings channel — nothing when tmux cannot be found
// by path, and then the exit goes unrecorded rather than the shell
// failing. The pane is named: a tmux run inside a pane knows its own pane
// by the environment, but set -p without a target goes to the session's
// active pane, not the one it was run in. The status is the first word
// expanded, before the date's substitution could run anything.
func recordExit() string {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		return ""
	}
	return "; " + shellQuote(tmux) + ` set -p -t "$TMUX_PANE" @conn_exit "$?" \; set -p -t "$TMUX_PANE" @conn_ended "$(date +%s)" \; wait-for -S ` + endedChannel + ` 2>/dev/null`
}

// endedChannel is the wait-for channel a command's ending is announced
// on. tmux announces nothing when a pane's option is set, so the dying
// command says it itself, from inside the pane, at the moment: the one
// moment conn is in the process at its death, and the event most worth
// hearing at once.
const endedChannel = "conn-ended"

// watchEndings listens for endings on the server, one waiter at a time,
// and reads the list each time one is announced: the navigator hears a
// command end rather than finding the shell at its prompt on a later
// scan. The waiter is a tmux client blocked on the channel, without the
// timeout the one-shot commands carry; it is let go when the session
// closes, and a server that goes ends it, for the next ensureCtl to start
// another.
func (s *session) watchEndings() {
	s.mu.Lock()
	if s.waiting || s.closed {
		s.mu.Unlock()
		return
	}
	s.waiting = true
	s.mu.Unlock()
	go func() {
		defer func() {
			s.mu.Lock()
			s.waiting = false
			s.mu.Unlock()
		}()
		for {
			if !s.awaitEnding() {
				return
			}
			s.refreshList()
		}
	}()
}

// awaitEnding blocks until an ending is announced, and reports whether one
// was: false when the server went, or the session closed.
func (s *session) awaitEnding() bool {
	cmd := exec.Command("tmux", "-S", socketPath(), "wait-for", endedChannel)
	cmd.Dir = "/"
	if err := cmd.Start(); err != nil {
		return false
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err == nil
	case <-s.stopped:
		_ = cmd.Process.Kill()
		<-done
		return false
	}
}

// noteOutcome keeps what the transcript said of a run on its pane, and
// noteRecorded that the ending is on the record: the next navigator to
// read the list has both, and reads and records nothing again.
func (s *session) noteOutcome(pid int, summary string) {
	if s == nil || summary == "" {
		return
	}
	p := s.pane(pid)
	if p == nil {
		return
	}
	go func() { _, _ = s.run("set", "-p", "-t", p.id, "@conn_summary", summary) }()
}

func (s *session) noteRecorded(pid int) {
	if s == nil {
		return
	}
	p := s.pane(pid)
	if p == nil {
		return
	}
	go func() { _, _ = s.run("set", "-p", "-t", p.id, "@conn_recorded", "1") }()
}

// open starts a shell — or handed a command, that command with a shell
// waiting behind it — in dir, held by the server, and tells the model when
// it is there.
func (s *session) open(dir, run, name string) {
	if s == nil {
		return
	}
	go func() {
		b, err := createWindow(s.run, dir, run, name, false)
		if err != nil {
			s.events <- serverErrorMsg{err: err}
			return
		}

		s.mu.Lock()
		s.panes[b.pid] = &pane{id: b.pane, pid: b.pid, dir: dir, name: name, run: run}
		s.byPane[b.pane] = b.pid
		s.mu.Unlock()

		s.refreshList()
		s.ensureCtl()
		s.events <- termOpenedMsg{pid: b.pid, dir: dir, name: name, run: run}
	}()
}

// list asks for what the server holds, which comes back as a sessionsMsg.
func (s *session) list() {
	if s == nil {
		return
	}
	go s.refreshList()
}

// show puts a buffer in the pane under the tabline and moves focus to it.
// tmux draws it there; conn keeps its rows.
func (s *session) show(pid int) {
	if s == nil {
		return
	}
	s.place(placement{pid: pid, focus: true})
}

// showQuiet puts a buffer in the pane under the tabline and keeps focus
// with conn: a dead buffer is readable beneath, and answers to conn's keys.
func (s *session) showQuiet(pid int) {
	if s == nil {
		return
	}
	s.place(placement{pid: pid})
	s.home()
}

// home moves focus to conn, from wherever it is: its window, and its pane
// in it.
func (s *session) home() {
	if s == nil {
		return
	}
	s.mu.Lock()
	nav := s.nav
	s.mu.Unlock()
	if nav == "" {
		return
	}
	go func() {
		if _, err := s.run("select-window", "-t", nav, ";", "select-pane", "-t", nav); err != nil {
			s.events <- serverErrorMsg{err: err}
		}
	}()
}

// park puts the buffer shown under the tabline back in a window of its own
// and leaves conn the whole window: conn has focus, and has something of
// its own to draw there.
func (s *session) park() {
	if s == nil {
		return
	}
	s.place(placement{})
}

// place asks for one arrangement of the home window, coalescing: one is made
// at a time, and an ask arriving while one is being made replaces any ask
// still waiting. A cursor moving down a column of shells becomes a handful
// of arrangements, each superseding the last, rather than one per row it
// crossed — and they are made in the order asked, so the pane ends up
// holding the shell the cursor stopped on.
func (s *session) place(p placement) {
	s.placing.ask(p, func(p placement) {
		if err := s.arrange(p); err != nil {
			s.events <- serverErrorMsg{err: err}
		}
	})
}

// latest holds the requests for one kind of work that is done one at a
// time and only ever needs its newest: one is done at a time, and a
// request arriving while one is being done replaces any still waiting.
type latest[T any] struct {
	mu   sync.Mutex
	want *T   // the request not yet done
	busy bool // a request is being done
}

// ask queues v, superseding any ask still waiting, and sees to it that do
// is run — on its own goroutine, once per ask that was not superseded, in
// the order asked.
func (l *latest[T]) ask(v T, do func(T)) {
	l.mu.Lock()
	l.want = &v
	if l.busy {
		l.mu.Unlock()
		return
	}
	l.busy = true
	l.mu.Unlock()

	go func() {
		for {
			l.mu.Lock()
			next := l.want
			l.want = nil
			if next == nil {
				l.busy = false
				l.mu.Unlock()
				return
			}
			l.mu.Unlock()
			do(*next)
		}
	}()
}

// arrange makes one arrangement: the buffer asked for moves into the pane
// under the tabline, and whichever buffer was there moves into the window
// it left. What the home window holds is asked of tmux as it
// stands rather than remembered, so an arrangement made by a chord or by
// a shell closing is built on, not fought.
func (s *session) arrange(p placement) error {
	s.mu.Lock()
	nav := s.nav
	target := ""
	if p.pid != 0 {
		if t := s.panes[p.pid]; t != nil {
			target = t.id
		}
	}
	s.mu.Unlock()
	if nav == "" {
		return errors.New("no pane of conn's to arrange around")
	}
	if p.pid != 0 && target == "" {
		return nil // gone since it was asked for; the list will say so
	}
	if err := showPane(s.run, nav, target); err != nil {
		return err
	}
	if p.focus && target != "" {
		_, err := s.run("select-window", "-t", nav, ";", "select-pane", "-t", target)
		return err
	}
	return nil
}

// showPane puts the target pane under conn's pane nav — or, with no
// target, puts whatever is there back in a window of its own. It reads the
// home window first, so it is right about what is there whoever last
// changed it. The layout is main-horizontal with conn's pane as the main
// one, and that holds the chrome's height when the window is resized: the
// configuration re-applies it on every resize.
//
// A buffer keeps its size as it moves. It joins at the slot's height in
// one step rather than at half the window and then the slot, and the
// window it goes back to is sized to the slot as it goes — a window sized
// by hand stays that size whatever the client does — so a program in it
// sees no change in its terminal, and has nothing to repaint, as it is
// shown and shown again. A buffer's first showing is the one resize: its
// window was made at the client's size. The slot is the window past the
// chrome's rows and the border under them.
func showPane(run runner, nav, target string) error {
	out, err := run("list-panes", "-t", nav, "-F", "#{pane_id}\t#{@conn_nav}\t#{window_width}\t#{window_height}")
	if err != nil {
		return err
	}
	shown := ""
	var slot []string // -x width -y height of the slot, when known
	for line := range strings.SplitSeq(out, "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 4 || f[0] == "" {
			continue
		}
		if f[1] != "1" && shown == "" {
			shown = f[0]
		}
		w, werr := strconv.Atoi(f[2])
		h, herr := strconv.Atoi(f[3])
		// The slot is the window past the chrome, the heading row and
		// the buffer's own border row.
		if werr == nil && herr == nil && w > 0 && h > chromeRows+2 {
			slot = []string{"-x", strconv.Itoa(w), "-y", strconv.Itoa(h - chromeRows - 2)}
		}
	}
	// park sizes the window a pane has just gone back to: the pane names
	// its window, and the window takes the slot's size.
	park := func(pane string) []string {
		if slot == nil {
			return nil
		}
		return append([]string{";", "resize-window", "-t", pane}, slot...)
	}
	switch {
	case target == shown:
		return nil
	case target == "":
		args := []string{"break-pane", "-d", "-n", heldName, "-s", shown}
		_, err = run(append(args, park(shown)...)...)
	case shown == "":
		// Shown is what a window named for wanting asked; the name goes
		// first, while the pane is still there to name the window by —
		// joined, its window closes behind it.
		args := []string{"rename-window", "-t", target, heldName, ";",
			"join-pane", "-v", "-d"}
		if slot != nil {
			args = append(args, "-l", slot[3])
		}
		args = append(args, "-s", target, "-t", nav, ";",
			"select-layout", "-t", nav, "main-horizontal")
		_, err = run(args...)
	default:
		// -d: focus stays where it is. Without it the pane swapped in
		// becomes the active one whatever was active before; where focus
		// goes is the arrangement's focus to say, after the swap.
		args := []string{"rename-window", "-t", target, heldName, ";",
			"swap-pane", "-d", "-s", target, "-t", shown}
		_, err = run(append(args, park(shown)...)...)
	}
	return err
}

// help shows the keys in a popup over the window; the popup takes the
// next key and goes.
func (s *session) help() {
	if s == nil {
		return
	}
	go func() {
		if err := showKeys(s.run, connExe(), ""); err != nil {
			s.events <- serverErrorMsg{err: err}
		}
	}()
}

// finder shows the finder in a popup over the client that spoke last, the
// way help does: the front door, from anywhere.
func (s *session) finder() {
	if s == nil {
		return
	}
	go func() {
		if err := showFinder(s.run, connExe(), "", ""); err != nil {
			s.events <- serverErrorMsg{err: err}
		}
	}()
}

// showRests keeps the listing of the conversations at rest for the page
// and opens the finder on it, over the client that spoke last.
func (s *session) showRests(snap finderSnapshot) error {
	if err := writeRests(snap); err != nil {
		return err
	}
	return showFinder(s.run, connExe(), "", finderRests)
}

// environment shows the environment of the run pid heads — the server's,
// for 0 — in a popup over the client that spoke last, the way help does.
func (s *session) environment(pid int) {
	if s == nil {
		return
	}
	go func() {
		if err := showEnv(s.run, connExe(), "", pid); err != nil {
			s.events <- serverErrorMsg{err: err}
		}
	}()
}

// popupSize is the room a popup running a command gets: enough for the
// installer's lines, cut to the client when the client is smaller.
const popupWidth, popupHeight = 72, 16

// popup runs a command in a popup over the client that spoke last, titled,
// closing when the command does.
func (s *session) popup(title, command string) {
	if s == nil {
		return
	}
	go func() {
		if err := popup(s.run, "", title, popupWidth, popupHeight, command); err != nil {
			s.events <- serverErrorMsg{err: err}
		}
	}()
}

// leave detaches the client this navigator is drawn in. The shells keep
// running; `conn` attaches again.
func (s *session) leave() {
	if s == nil {
		return
	}
	go func() { _, _ = s.run("detach-client") }()
}

// closeTerm ends a shell the server holds. Killing the pane takes its
// terminal away, which is the hangup everything in it listens to.
func (s *session) closeTerm(pid int) {
	if s == nil {
		return
	}
	p := s.pane(pid)
	if p == nil {
		return
	}
	go func() { _, _ = s.run("kill-pane", "-t", p.id) }()
}

// respawn runs a command again in a held shell's pane, in place: the pane
// keeps its id, its place in the layout and its options — its directory,
// its name, what it ran — and gets a new shell running the command with a
// shell waiting behind it, the ending recorded as it goes, as the first
// run's was. The old ending is taken off the pane first.
func (s *session) respawn(pid int, command string) {
	if s == nil {
		return
	}
	p := s.pane(pid)
	if p == nil {
		return
	}
	go func() {
		_, _ = s.run("set", "-pu", "-t", p.id, "@conn_exit", ";", "set", "-pu", "-t", p.id, "@conn_ended", ";",
			"set", "-pu", "-t", p.id, "@conn_summary", ";", "set", "-pu", "-t", p.id, "@conn_recorded", ";",
			"respawn-pane", "-k", "-t", p.id, command+recordExit()+`; exec "$SHELL"`)
		s.refreshList()
	}()
}

// typeInto types a line at a held shell's prompt, and enter after it: the
// way gf opens the editor in the buffer's own shell.
func (s *session) typeInto(pid int, text string) {
	if s == nil {
		return
	}
	p := s.pane(pid)
	if p == nil {
		return
	}
	go func() { _, _ = s.run("send-keys", "-t", p.id, "-l", text, ";", "send-keys", "-t", p.id, "Enter") }()
}

// replace ends the server and every shell it holds. Nothing reaches this by
// accident: it takes the same second key any other kill takes.
func (s *session) replace() {
	if s == nil {
		return
	}
	go func() { _, _ = s.run("kill-server") }()
}

// dress names a shell's pane — its place, what runs there, and its mark —
// for the terminal's title while it has focus.
func (s *session) dress(pid int, name string) {
	if s == nil {
		return
	}
	p := s.pane(pid)
	if p == nil {
		return
	}
	go func() { _, _ = s.run("set", "-p", "-t", p.id, "@conn_title", name) }()
}

// statusText is what conn has tmux read for it: on the status line the
// mode, when it has one to name, and what it has to say; and under the
// tabline the shown buffer's heading.
type statusText struct {
	mode, msg string
	heading   string // the heading in tmux's styling, empty while no buffer is shown
}

// say hands tmux conn's part of the status line. Said one at a time and
// superseded while waiting, so a query being typed lands in the order it
// was typed and the line ends up reading the last of it; and the line is
// refreshed at once rather than on its next tick, because a mode that
// lags the keys is a mode that lies.
func (s *session) say(t statusText) {
	if s == nil {
		return
	}
	s.saying.ask(t, func(t statusText) {
		_, _ = s.run("set", "-g", modeOption, t.mode, ";",
			"set", "-g", msgOption, t.msg, ";",
			"set", "-g", headingOption, t.heading, ";",
			"refresh-client", "-S")
	})
}

func (s *session) pane(pid int) *pane {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.panes[pid]
}

// tail is the last n lines a held shell has shown, the pane and what has
// scrolled off above it together, in the colors the shell drew them, with
// the pane's unused rows below the last line left out. Wrapped lines are
// read joined, so the pane that draws them wraps or cuts them at its own
// width rather than tmux's.
func (s *session) tail(pid, n int) []string {
	p := s.pane(pid)
	if p == nil {
		return nil
	}
	out, err := s.run("capture-pane", "-p", "-e", "-J", "-t", p.id, "-S", "-"+strconv.Itoa(n))
	if err != nil {
		return nil
	}
	lines := strings.Split(out, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t\r")
	}
	for len(lines) > 0 && strings.TrimSpace(ansi.Strip(lines[len(lines)-1])) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

// running reports a pane still running the command it was started with:
// no exit recorded, and something other than the shell in the foreground.
// A shell at its prompt with no exit recorded — the recording missed, or a
// shell started with nothing — is not running an entry, and is not in the
// way of running it again.
func (p *pane) running() bool {
	if p.exit != "" {
		return false
	}
	return p.cmd == "" || !isShell(p.cmd)
}

// forgetExit takes the recorded exit off a shell's pane: the shell has
// been used by hand since, and the exit is history.
func (s *session) forgetExit(pid int) {
	p := s.pane(pid)
	if p == nil {
		return
	}
	go func() {
		_, _ = s.run("set", "-pu", "-t", p.id, "@conn_exit", ";", "set", "-pu", "-t", p.id, "@conn_ended", ";",
			"set", "-pu", "-t", p.id, "@conn_summary", ";", "set", "-pu", "-t", p.id, "@conn_recorded")
	}()
}

// scrollbackLines is how many lines of transcript each shell keeps once they
// scroll off its pane. It is written into the server's configuration at
// launch, so raising it takes R — a fresh server — to reach anything.
var scrollbackLines = 10000

// applyScrollback sets the transcript cap from the config. Zero — unset —
// leaves the default standing.
func applyScrollback(n int) {
	if n > 0 {
		scrollbackLines = n
	}
}

// bornName is the placeholder window a session is remade around, gone
// as soon as the first shell is in.
const bornName = "conn-born"

// outerTerminal is the terminal around tmux, as the TERM_PROGRAM and
// TERM_PROGRAM_VERSION the server was started under: the launcher's, kept
// in the server's global environment. tmux tells every pane its terminal
// is tmux, and Claude Code takes it at its word — the progress it reports
// while it works, which Ghostty draws in the title bar, is offered only to
// a terminal it knows will. Told the outer name, it sees through tmux and
// wraps what it says for passing through, which the configuration allows.
// A server started inside another tmux has nothing to pass on.
func outerTerminal(run runner) []string {
	var kvs []string
	for _, name := range []string{"TERM_PROGRAM", "TERM_PROGRAM_VERSION"} {
		out, err := run("show-environment", "-g", name)
		if err != nil {
			continue
		}
		_, v, ok := strings.Cut(strings.TrimSpace(out), "=")
		if !ok || v == "" {
			continue
		}
		if name == "TERM_PROGRAM" && v == "tmux" {
			return nil
		}
		kvs = append(kvs, name+"="+v)
	}
	return kvs
}
