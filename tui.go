package main

import (
	"os"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// The ground and the ink, as the terminal is asked to take them for its
// own while conn is up, so its padding is the ground too. Dark until
// applyMode says otherwise; see mode.go.
var (
	groundColor = darkGround
	inkColor    = darkInk
)

// The program holds three views. The console comes on first: the header
// at once, from what is known before anything is read; the station is
// read meanwhile, and the readout comes on when it is in hand and its
// beat has passed, then the checks one by one, then the verdict, in
// under a second. A key skips to the end; a key at the end continues to
// the watch. The watch is what is running, by place, read again every
// two seconds while it is up; j and k move the cursor, which follows
// its process across readings; c brings the console back, and any key
// there returns to the watch. The console is a page: in the server it
// takes the whole window while it is up, and the slot has its side
// again on the way back to the watch. The words of both are said
// again each second, from what was read and the clock as it stands.
//
// p is the list: every project the roots hold, whether anything is
// running in it or not, walked as the view comes on. It is a line typed
// into, so the keys the other views are worked by are characters there;
// enter opens a shell at the row under the cursor and comes back to the
// watch, where the shell shows, and esc comes back without opening
// anything.
//
// In conn's tmux server, conn is the rail on the left of the home
// window; when the watch first comes on it opens the slot beside it,
// with a hold in it, and opens it again should it close. Enter puts
// the cursor's process in the slot, when it is in a pane of the server;
// s opens a shell at the cursor's place there; q and ctrl+c detach, and
// the server keeps on. Without the server, q and ctrl+c close conn.

// The views.
const (
	viewConsole = iota
	viewWatch
	viewProjects
	viewResume
)

// The time before each stage after the header: a beat for the readout
// and the verdict, less for each check.
func (m model) stageDelay(stage int) time.Duration {
	switch stage {
	case stageReadout, lastStage(m.report()):
		return 150 * time.Millisecond
	default:
		return 80 * time.Millisecond
	}
}

// watchEvery is how often the watch reads the process table at rest.
// While it is waiting on a shell conn has just opened, it reads again as
// soon as it can: a shell takes a moment to reach the table, and two
// seconds of the cursor sitting on the old row is the shell feeling
// slow to open. waitForOpened is how long that is worth doing before
// the shell is given up on.
const (
	watchEvery    = 2 * time.Second
	watchSoon     = 150 * time.Millisecond
	waitForOpened = 3 * time.Second
)

// The console's alarms blink like annunciators on a panel: lit for a
// second, dark for half of one. The dark is the shorter half — the
// blink is there to catch the eye, not to take the words away.
const (
	blinkLit  = time.Second
	blinkDark = time.Second / 2
)

type (
	stageMsg   struct{}          // the next stage is due
	clockMsg   struct{}          // the second has turned
	stationMsg struct{ station } // the station is read
	watchMsg   struct {          // the process table is read
		places   []place
		panes    map[string]pane // the server's panes by terminal
		slot     string          // the terminal in the slot
		noSlot   bool            // home has no slot beside the rail
		slotDead bool            // the slot's pane held on remain-on-exit, its process gone
		err      string
		gen      int
	}
	watchTickMsg struct{ gen int }     // the watch is due to be read again
	openedMsg    struct{ shell shell } // a shell was opened; the cursor goes to it once it is read
	reachedMsg   struct{ tty string }  // a process was put in the slot
	blinkMsg     struct{ gen int }     // the chip's half is up
	noteMsg      struct{ note string }
	projectsMsg  struct { // the roots were walked
		projects []project
		err      string
	}
	convosMsg struct { // a place's suspended conversations were read
		dirs   []string
		convos []conversation
	}
)

type model struct {
	head          station  // what the header needs: the build and who is at the station
	st            *station // the station, once read
	now           time.Time
	stage         int  // the stage the console has come on to
	due           bool // the readout's beat has passed and it waits on the station
	width, height int
	p             palette

	view     int
	lit      bool // the verdict's chip is showing this half of the blink
	blinkGen int  // which stay on the console the blink belongs to
	places   []place
	cursor   int // the pid the cursor is on
	cursorAt int // where in the rows it was, for when the pid goes
	// A shell conn has just opened: the pid the cursor goes to once the
	// process table has it, and how long that is waited for.
	awaited  int
	until    time.Time
	watchErr string
	watchGen int // which stay on the watch the ticks belong to
	// The list: the projects as the roots were last walked, what has been
	// typed to narrow them, and which of the rows the cursor is on.
	projects    []project
	filter      string
	pcursor     int
	scanning    bool
	projectsErr string
	uid         int
	roots       func(string) string

	// The picker: a place's suspended conversations, as last read, what
	// has narrowed them, and which of the rows the cursor is on.
	convosDirs    []string // the directories asked for; a stale answer's guard
	convosPlace   string
	convos        []conversation
	convosLoading bool
	rfilter       string
	rcursor       int

	// kill is a kill x has asked for and not yet answered; nothing else
	// binds while it is not nil.
	kill *pendingKill

	srv    *server         // conn's tmux server, when there is one
	inside bool            // this conn is the rail of the server's home window
	self   string          // this binary, for the hold
	panes  map[string]pane // the server's panes by terminal, as last read
	slot   string          // the terminal in the slot, as last read
	note   string          // a word on the bottom row, until the next key
}

func newModel(p palette) model {
	return model{
		lit:   true,
		head:  station{build: readBuild(), session: readSession()},
		now:   time.Now(),
		p:     p,
		uid:   os.Getuid(),
		roots: placeRoots(),
	}
}

// report is the console's words as things stand: from the station once
// it is read, from the header's part of it before.
func (m model) report() report {
	if m.st != nil {
		return compose(*m.st, m.now)
	}
	return compose(m.head, m.now)
}

// watchReport is the watch's words as things stand.
func (m model) watchReport() watchReport {
	r := m.report()
	w := composeWatch(m.places, m.panes, m.slot, m.head.session.home, m.now, r.station, r.clock, m.watchErr)
	w.inside, w.note = m.inside, m.note
	return w
}

// projectsReport is the list's words as things stand, and projectRows
// the rows the filter leaves, which the cursor is an index into.
func (m model) projectsReport() projectsReport {
	b := composeProjects(m.projects, m.filter, projectRoots(m.head.session.home), m.head.session.home, m.scanning, m.projectsErr)
	b.note = m.note
	return b
}

func (m model) projectRows() []project {
	return matching(m.projects, m.filter)
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{readStationCmd, m.nextStage(), nextSecond(m.now), m.nextBlink()}
	if m.inside {
		cmds = append(cmds, m.serverCmd(func() error { return m.srv.wide() }, ""))
	}
	return tea.Batch(cmds...)
}

func readStationCmd() tea.Msg {
	return stationMsg{readStation()}
}

// readWatch reads the process table, and in the server its panes and
// the slot, and composes the watch off them. It reads and does nothing
// else; what the reading calls for is decided when it comes back.
func (m model) readWatch() tea.Cmd {
	gen, uid, roots := m.watchGen, m.uid, m.roots
	var srv *server
	if m.inside {
		srv = m.srv
	}
	return func() tea.Msg {
		procs, err := readProcesses(uid)
		if err != nil {
			return watchMsg{err: "THE PROCESS TABLE COULD NOT BE READ: " + err.Error(), gen: gen}
		}
		msg := watchMsg{places: watch(procs, uid, roots), gen: gen}
		if srv != nil {
			if slot, ok, err := srv.slot(); err == nil && !ok {
				msg.noSlot = true
			} else if ok {
				msg.slot, msg.slotDead = slot.tty, slot.dead
			}
			msg.panes, _ = srv.panes()
		}
		return msg
	}
}

func (m model) nextStage() tea.Cmd {
	return tea.Tick(m.stageDelay(m.stage+1), func(time.Time) tea.Msg { return stageMsg{} })
}

// nextSecond ticks on the turn of the second, not a second after the
// last tick, so no second is skipped.
func nextSecond(now time.Time) tea.Cmd {
	return tea.Tick(time.Until(now.Truncate(time.Second).Add(time.Second)), func(time.Time) tea.Msg { return clockMsg{} })
}

// nextBlink is the turn of the chip's other half, each half its own
// length. The blink belongs to a stay on the console: a turn that comes
// after the console is left, or from an earlier stay, is dropped.
func (m model) nextBlink() tea.Cmd {
	d := blinkLit
	if !m.lit {
		d = blinkDark
	}
	gen := m.blinkGen
	return tea.Tick(d, func(time.Time) tea.Msg { return blinkMsg{gen} })
}

// watchTick is when the watch reads again: soon while it waits on a
// shell conn opened, and at its own pace otherwise.
func (m model) watchTick() tea.Cmd {
	gen, every := m.watchGen, watchEvery
	if m.awaited != 0 {
		every = watchSoon
	}
	return tea.Tick(every, func(time.Time) tea.Msg { return watchTickMsg{gen} })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// The rail holds its width through a resize of the window, once it
		// is a rail: with the slot beside it, on the watch or the list.
		if m.inside && m.view != viewConsole && m.slot != "" && m.width != railWidth {
			return m, m.serverCmd(func() error { return m.srv.holdRail() }, "")
		}
	case stationMsg:
		st := msg.station
		m.st = &st
		if m.due {
			m.due = false
			return m.advance()
		}
	case stageMsg:
		if m.stage+1 == stageReadout && m.st == nil {
			m.due = true
			return m, nil
		}
		return m.advance()
	case clockMsg:
		m.now = time.Now()
		return m, nextSecond(m.now)
	case openedMsg:
		// The shell is in the slot; the table will have it in a moment,
		// and the cursor goes to it then. Until then the watch reads soon.
		m.slot = msg.shell.pane.tty
		m.awaited, m.until = msg.shell.pid, time.Now().Add(waitForOpened)
		m.watchGen++
		return m, m.readWatch()
	case reachedMsg:
		// The pane is in the slot; conn knows it now and does not have to
		// read the server to find out, so the row says so at once.
		m.slot = msg.tty
		m.watchGen++
		return m, m.readWatch()
	case blinkMsg:
		if msg.gen != m.blinkGen || m.view != viewConsole {
			m.lit = true
			return m, nil
		}
		m.lit = !m.lit
		return m, m.nextBlink()
	case watchMsg:
		if msg.gen != m.watchGen {
			return m, nil
		}
		m.places, m.panes, m.slot, m.watchErr = msg.places, msg.panes, msg.slot, msg.err
		// The shell conn opened is the cursor's once the reading has it;
		// one that never comes is given up on when the wait is out.
		if m.awaited != 0 {
			switch {
			case hasPid(m.places, m.awaited):
				m.cursor, m.awaited = m.awaited, 0
			case time.Now().After(m.until):
				m.awaited = 0
			}
		}
		m.cursor, m.cursorAt = follow(m.places, m.cursor, m.cursorAt)
		if m.view == viewWatch {
			switch {
			// A home without its slot gets one; the next reading finds it.
			case m.inside && msg.noSlot:
				return m, tea.Batch(m.watchTick(), m.openSlot())
			// A slot whose pane died stays the shape it was; only what is
			// in it is replaced, so the rail never has to give up its
			// width and take it back.
			case m.inside && msg.slotDead:
				return m, tea.Batch(m.watchTick(), m.reviveSlot())
			}
			return m, m.watchTick()
		}
	case watchTickMsg:
		if msg.gen != m.watchGen || m.view != viewWatch {
			return m, nil
		}
		return m, m.readWatch()
	case projectsMsg:
		m.projects, m.projectsErr, m.scanning = msg.projects, msg.err, false
		m.pcursor = clamp(m.pcursor, len(m.projectRows()))
	case convosMsg:
		// Only the picker that asked for these dirs wants them; one opened
		// on another place since has moved past the answer.
		if !slices.Equal(msg.dirs, m.convosDirs) {
			return m, nil
		}
		m.convos, m.convosLoading = msg.convos, false
		m.rcursor = clamp(m.rcursor, len(m.resumeRows()))
	case killedMsg:
		m.note = killNote(msg)
		// A beat for the signal to be acted on, so the row is not read a
		// moment too soon, still there; the watchTick this reuses is a
		// no-op once the stay it belongs to has moved on.
		gen := m.watchGen
		return m, tea.Tick(killGrace, func(time.Time) tea.Msg { return watchTickMsg{gen: gen} })
	case noteMsg:
		m.note = msg.note
	case tea.KeyPressMsg:
		m.note = ""
		return m.key(msg.String())
	}
	return m, nil
}

// key answers a key: q and ctrl+c detach in the server and close conn
// outside it, from anywhere; on the console a key skips the sequence,
// then continues to the watch and gives the slot its side back; on the
// watch c brings the console back over the whole window,
// enter reaches the cursor's process, s opens a shell at its place, a
// opens claude there instead, and A opens the picker over what claude
// left suspended there. x asks to end the cursor's process, and arms
// the question rather than the ending: the next key answers it.
func (m model) key(k string) (tea.Model, tea.Cmd) {
	// A kill x asked for takes the next key, whatever it is: x, y or
	// enter confirms it, and anything else cancels — no other binding
	// fires while the question is on the bottom row.
	if m.kill != nil {
		req := m.kill
		m.kill = nil
		switch k {
		case "x", "y", "enter":
			return m, m.killEntry(req.pid, req.command, req.sig)
		default:
			m.note = "KILL CANCELLED"
			return m, nil
		}
	}
	switch m.view {
	case viewProjects:
		return m.projectKey(k)
	case viewResume:
		return m.resumeKey(k)
	}
	switch {
	case k == "ctrl+c" || k == "q":
		if m.inside {
			return m, m.serverCmd(func() error { return m.srv.detach() }, "")
		}
		return m, tea.Quit
	case m.view == viewConsole && m.stage < lastStage(m.report()):
		m.stage = lastStage(m.report())
		return m, nil
	case m.view == viewConsole:
		m.view = viewWatch
		m.watchGen++
		if m.inside {
			return m, tea.Batch(m.readWatch(), m.serverCmd(func() error { return m.srv.narrow() }, ""))
		}
		return m, m.readWatch()
	case k == "c":
		m.view = viewConsole
		m.lit, m.blinkGen = true, m.blinkGen+1
		if m.inside {
			return m, tea.Batch(m.nextBlink(), m.serverCmd(func() error { return m.srv.wide() }, ""))
		}
		return m, m.nextBlink()
	case k == "j" || k == "down":
		m.cursor, m.cursorAt = follow(m.places, 0, m.cursorAt+1)
	case k == "k" || k == "up":
		m.cursor, m.cursorAt = follow(m.places, 0, max(m.cursorAt-1, 0))
	case k == "enter":
		e, _, ok := m.under()
		switch {
		case !m.inside:
			m.note = "NOTHING CAN BE REACHED OUTSIDE CONN'S TMUX SERVER"
		case !ok:
			m.note = "NOTHING UNDER THE CURSOR"
		case m.panes[e.tty].id == "":
			m.note = "NOT IN A PANE OF CONN'S SERVER"
		default:
			return m, m.reach(m.panes[e.tty], e.tty)
		}
	case k == "x":
		e, _, ok := m.under()
		if !ok {
			m.note = "NOTHING UNDER THE CURSOR"
			return m, nil
		}
		sig := killSignal(e.kind)
		m.kill = &pendingKill{pid: e.pid, command: e.command, sig: sig}
		m.note = killPrompt(e.command, e.pid, sig)
	case k == "s":
		_, pl, ok := m.under()
		switch {
		case !m.inside:
			m.note = "NOTHING CAN BE OPENED OUTSIDE CONN'S TMUX SERVER"
		case !ok || pl.path == "":
			m.note = "NO PLACE UNDER THE CURSOR"
		default:
			return m, m.openShell(pl.path)
		}
	case k == "a":
		_, pl, ok := m.under()
		switch {
		case !m.inside:
			m.note = "NOTHING CAN BE OPENED OUTSIDE CONN'S TMUX SERVER"
		case !ok || pl.path == "":
			m.note = "NO PLACE UNDER THE CURSOR"
		default:
			return m, m.openAgent(pl.path)
		}
	case k == "A":
		_, pl, ok := m.under()
		switch {
		case !m.inside:
			m.note = "NOTHING CAN BE OPENED OUTSIDE CONN'S TMUX SERVER"
		case !ok || pl.path == "":
			m.note = "NO PLACE UNDER THE CURSOR"
		default:
			return m.openResume(pl.path, []string{pl.path})
		}
	case k == "p":
		m.view, m.filter, m.pcursor, m.scanning = viewProjects, "", 0, true
		return m, m.scanProjects()
	}
	return m, nil
}

// projectKey answers a key on the list, which is a line typed into: a
// key that stands for a character goes to the filter, so the letters the
// other views are worked by are themselves here. Up and down move the
// cursor, and ctrl+n and ctrl+p do too, since a hand on a filter is a
// hand that cannot reach j and k; enter opens a shell at the row under
// the cursor and goes back to the watch, which is where the shell will
// show, and ctrl+a opens claude there instead, since a plain a is a
// letter to type; A, or ctrl+shift+a beside ctrl+a's own chord, opens
// the picker over what claude left suspended at the row, group
// included — a plain A is not a letter anyone types into a project's
// name; esc goes back without opening anything, and
// ctrl+c is what it is everywhere.
func (m model) projectKey(k string) (tea.Model, tea.Cmd) {
	rows := m.projectRows()
	switch {
	case k == "ctrl+c":
		if m.inside {
			return m, m.serverCmd(func() error { return m.srv.detach() }, "")
		}
		return m, tea.Quit
	case k == "esc":
		return m.toWatch()
	case k == "enter":
		switch {
		case !m.inside:
			m.note = "NOTHING CAN BE OPENED OUTSIDE CONN'S TMUX SERVER"
		case m.pcursor >= len(rows):
			m.note = "NO PROJECT UNDER THE CURSOR"
		default:
			path := rows[m.pcursor].path
			mm, cmd := m.toWatch()
			m = mm.(model)
			return m, tea.Batch(cmd, m.openShell(path))
		}
	case k == "ctrl+a":
		switch {
		case !m.inside:
			m.note = "NOTHING CAN BE OPENED OUTSIDE CONN'S TMUX SERVER"
		case m.pcursor >= len(rows):
			m.note = "NO PROJECT UNDER THE CURSOR"
		default:
			path := rows[m.pcursor].path
			mm, cmd := m.toWatch()
			m = mm.(model)
			return m, tea.Batch(cmd, m.openAgent(path))
		}
	case k == "A" || k == "ctrl+shift+a":
		switch {
		case !m.inside:
			m.note = "NOTHING CAN BE OPENED OUTSIDE CONN'S TMUX SERVER"
		case m.pcursor >= len(rows):
			m.note = "NO PROJECT UNDER THE CURSOR"
		default:
			row := rows[m.pcursor]
			return m.openResume(row.path, convoDirs(m.projects, row))
		}
	case k == "up" || k == "ctrl+p":
		m.pcursor = clamp(m.pcursor-1, len(rows))
	case k == "down" || k == "ctrl+n":
		m.pcursor = clamp(m.pcursor+1, len(rows))
	case k == "backspace":
		if r := []rune(m.filter); len(r) > 0 {
			m.filter = string(r[:len(r)-1])
		}
		m.pcursor = 0
	case k == "ctrl+u":
		m.filter, m.pcursor = "", 0
	case k == "space":
		m.filter, m.pcursor = m.filter+" ", 0
	case utf8.RuneCountInString(k) == 1:
		m.filter, m.pcursor = m.filter+k, 0
	}
	return m, nil
}

// toWatch leaves the list for the watch, which starts reading again.
func (m model) toWatch() (tea.Model, tea.Cmd) {
	m.view = viewWatch
	m.watchGen++
	return m, m.readWatch()
}

// openResume opens the picker over a place's suspended conversations:
// place is what it is for, and dirs the directories a transcript could
// be filed under, which for a group is a repository under it, not the
// folder that names it.
func (m model) openResume(place string, dirs []string) (tea.Model, tea.Cmd) {
	m.view = viewResume
	m.convosDirs, m.convosPlace, m.convosLoading = dirs, place, true
	m.convos, m.rfilter, m.rcursor = nil, "", 0
	return m, m.scanConvos(dirs)
}

// resumeRows is the conversations the filter leaves, which the cursor
// is an index into.
func (m model) resumeRows() []conversation {
	return matchingConvos(m.convos, m.rfilter)
}

// resumeReport is the picker's words as things stand.
func (m model) resumeReport() resumeReport {
	b := composeResume(m.convos, m.convosPlace, m.rfilter, m.head.session.home, m.now, m.convosLoading)
	b.note = m.note
	return b
}

// resumeKey answers a key on the picker, which is a line typed into the
// same way the list is: letters narrow it, up and down move the cursor
// and ctrl+n and ctrl+p do too, enter continues the conversation under
// the cursor and goes back to the watch, esc goes back without
// continuing anything, and ctrl+c is what it is everywhere.
func (m model) resumeKey(k string) (tea.Model, tea.Cmd) {
	rows := m.resumeRows()
	switch {
	case k == "ctrl+c":
		if m.inside {
			return m, m.serverCmd(func() error { return m.srv.detach() }, "")
		}
		return m, tea.Quit
	case k == "esc":
		return m.toWatch()
	case k == "enter":
		switch {
		case !m.inside:
			m.note = "NOTHING CAN BE OPENED OUTSIDE CONN'S TMUX SERVER"
		case m.convosLoading:
			m.note = "STILL LOOKING"
		case m.rcursor >= len(rows):
			m.note = "NO CONVERSATION UNDER THE CURSOR"
		default:
			c := rows[m.rcursor]
			mm, cmd := m.toWatch()
			m = mm.(model)
			return m, tea.Batch(cmd, m.openResumed(c.Dir, c.ID))
		}
	case k == "up" || k == "ctrl+p":
		m.rcursor = clamp(m.rcursor-1, len(rows))
	case k == "down" || k == "ctrl+n":
		m.rcursor = clamp(m.rcursor+1, len(rows))
	case k == "backspace":
		if r := []rune(m.rfilter); len(r) > 0 {
			m.rfilter = string(r[:len(r)-1])
		}
		m.rcursor = 0
	case k == "ctrl+u":
		m.rfilter, m.rcursor = "", 0
	case k == "space":
		m.rfilter, m.rcursor = m.rfilter+" ", 0
	case utf8.RuneCountInString(k) == 1:
		m.rfilter, m.rcursor = m.rfilter+k, 0
	}
	return m, nil
}

// clamp holds an index within the rows there are; with no rows it is
// the first, which is no row.
func clamp(at, rows int) int {
	return min(max(at, 0), max(rows-1, 0))
}

// under is the entry and the place under the cursor.
func (m model) under() (entry, place, bool) {
	for _, pl := range m.places {
		for _, e := range pl.entries {
			if e.pid == m.cursor {
				return e, pl, true
			}
		}
	}
	return entry{}, place{}, false
}

// follow finds the cursor after the rows change: the row of its pid,
// where that is still on watch, else the row where it was, held within
// the rows there are. It answers the pid and the row.
func follow(places []place, pid, at int) (int, int) {
	var pids []int
	for _, pl := range places {
		for _, e := range pl.entries {
			pids = append(pids, e.pid)
		}
	}
	if len(pids) == 0 {
		return 0, 0
	}
	for i, p := range pids {
		if pid != 0 && p == pid {
			return p, i
		}
	}
	at = min(max(at, 0), len(pids)-1)
	return pids[at], at
}

// advance brings the next stage on and sets the one after it going.
func (m model) advance() (tea.Model, tea.Cmd) {
	last := lastStage(m.report())
	if m.stage < last {
		m.stage++
	}
	if m.stage < last {
		return m, m.nextStage()
	}
	return m, nil
}

// View is the view that is up. The console shows as far as it has come
// on: rows of a later stage are the ground until their turn.
func (m model) View() tea.View {
	var rows []row
	switch m.view {
	case viewWatch:
		rows = drawWatch(m.watchReport(), m.cursor, m.width, m.height, m.p)
	case viewProjects:
		rows = drawProjects(m.projectsReport(), m.pcursor, m.width, m.height, m.p)
	case viewResume:
		rows = drawResume(m.resumeReport(), m.rcursor, m.width, m.height, m.p)
	default:
		r := m.report()
		r.lit = m.lit
		rows = screen(r, m.width, m.height, m.p)
	}
	ground := rows[0].text // the first row is blank, on the ground, at the rows' width
	texts := make([]string, 0, len(rows))
	for i, r := range rows {
		if i >= m.height && m.height > 0 {
			break
		}
		if m.view == viewConsole && r.stage > m.stage {
			texts = append(texts, ground)
		} else {
			texts = append(texts, r.text)
		}
	}
	v := tea.NewView(strings.Join(texts, "\n"))
	v.AltScreen = true
	v.BackgroundColor = groundColor
	v.ForegroundColor = inkColor
	v.WindowTitle = "conn"
	return v
}
