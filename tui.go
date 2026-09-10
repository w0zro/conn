package main

import (
	"image/color"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// The ground and the ink, as the terminal is asked to take them for its
// own while conn is up, so its padding is the ground too.
var (
	groundColor = color.RGBA{R: 21, G: 19, B: 15, A: 255}
	inkColor    = color.RGBA{R: 230, G: 223, B: 208, A: 255}
)

// The program holds two views. The console comes on first: the header
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
// While it is waiting on a process conn has just started, it reads
// again as soon as it can: a shell takes a moment to reach the table,
// and two seconds of the cursor sitting on the old row is the shell
// feeling slow to open. waitForOpened is how long that is worth doing
// before giving the process up.
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
		places []place
		panes  map[string]pane // the server's panes by terminal
		slot   string          // the terminal in the slot
		err    string
		gen    int
	}
	watchTickMsg struct{ gen int } // the watch is due to be read again
	openedMsg    struct {          // a shell was opened, and is the cursor's
		shell shell
		place string
	}
	reachedMsg struct{ tty string } // a process was put in the slot
	blinkMsg   struct{}             // the chip's half is up
	noteMsg    struct{ note string }
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
	places   []place
	cursor   int // the pid the cursor is on
	cursorAt int // where in the rows it was, for when the pid goes
	// A row for a process conn has just started. It is shown from the
	// moment the process is made, out of what tmux said about it, and
	// kept until the process table catches up — or until the wait is
	// out, so a shell that never came up is not a row forever.
	pending  *pendingRow
	watchErr string
	watchGen int // which stay on the watch the ticks belong to
	pid      int // this process
	uid      int
	roots    func(string) string

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
		pid:   os.Getpid(),
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
// the slot, opening the slot when home has none, and composes the watch
// off them.
func (m model) readWatch() tea.Cmd {
	gen, pid, uid, roots := m.watchGen, m.pid, m.uid, m.roots
	var srv *server
	if m.inside {
		srv = m.srv
	}
	home, self := m.head.session.home, m.self
	return func() tea.Msg {
		procs, err := readProcesses(uid)
		if err != nil {
			return watchMsg{err: "THE PROCESS TABLE COULD NOT BE READ: " + err.Error(), gen: gen}
		}
		msg := watchMsg{places: watch(procs, pid, uid, roots), gen: gen}
		if srv != nil {
			if slot, ok, err := srv.slot(); err == nil && !ok {
				_ = srv.splitSlot(home, self)
			} else if ok {
				msg.slot = slot.tty
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
// length.
func (m model) nextBlink() tea.Cmd {
	d := blinkLit
	if !m.lit {
		d = blinkDark
	}
	return tea.Tick(d, func(time.Time) tea.Msg { return blinkMsg{} })
}

// watchTick is when the watch reads again: at once while it waits on a
// process conn started, and at its own pace otherwise.
func (m model) watchTick() tea.Cmd {
	gen, every := m.watchGen, watchEvery
	if m.pending != nil {
		every = watchSoon
	}
	return tea.Tick(every, func(time.Time) tea.Msg { return watchTickMsg{gen} })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// The rail holds its width through a resize of the window, once it
		// is a rail: with the slot beside it, on the watch.
		if m.inside && m.view == viewWatch && m.slot != "" && m.width != railWidth {
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
		// tmux made the shell and said what it is, so the row exists now
		// rather than whenever the process table next says so: conn draws
		// it, puts the cursor on it, and reads again to catch up.
		e := entry{
			pid: msg.shell.pid, kind: kindShell, command: msg.shell.command,
			tty: msg.shell.pane.tty, started: m.now, status: statusIdle,
		}
		m.pending = &pendingRow{row: e, pane: msg.shell.pane, place: msg.place, until: time.Now().Add(waitForOpened)}
		m.places = withRow(m.places, msg.place, e)
		if m.panes == nil {
			m.panes = map[string]pane{}
		}
		m.panes[e.tty], m.slot = msg.shell.pane, e.tty
		m.cursor, m.cursorAt = follow(m.places, e.pid, m.cursorAt)
		m.watchGen++
		return m, m.readWatch()
	case reachedMsg:
		// The pane is in the slot; conn knows it now and does not have to
		// read the server to find out, so the row says so at once.
		m.slot = msg.tty
		m.watchGen++
		return m, m.readWatch()
	case blinkMsg:
		m.lit = !m.lit
		return m, m.nextBlink()
	case watchMsg:
		if msg.gen != m.watchGen {
			return m, nil
		}
		// The row conn made for a shell it opened stands until the reading
		// has the process itself, and the pane and the slot with it.
		if m.pending != nil {
			switch {
			case hasPid(msg.places, m.pending.row.pid), time.Now().After(m.pending.until):
				m.pending = nil
			default:
				msg.places = withRow(msg.places, m.pending.place, m.pending.row)
				if msg.panes == nil {
					msg.panes = map[string]pane{}
				}
				msg.panes[m.pending.row.tty] = m.pending.pane
				msg.slot = m.pending.row.tty
			}
		}
		m.places, m.panes, m.slot, m.watchErr = msg.places, msg.panes, msg.slot, msg.err
		m.cursor, m.cursorAt = follow(m.places, m.cursor, m.cursorAt)
		if m.view == viewWatch {
			return m, m.watchTick()
		}
	case watchTickMsg:
		if msg.gen != m.watchGen || m.view != viewWatch {
			return m, nil
		}
		return m, m.readWatch()
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
// enter reaches the cursor's process, and s opens a shell at its place.
func (m model) key(k string) (tea.Model, tea.Cmd) {
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
		if m.inside {
			return m, m.serverCmd(func() error { return m.srv.wide() }, "")
		}
		return m, nil
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
		case m.panes[e.tty].id == m.srv.rail():
			m.note = "THAT IS THIS WATCH"
		default:
			return m, m.reach(m.panes[e.tty], e.tty)
		}
	case k == "s":
		_, pl, ok := m.under()
		switch {
		case !m.inside:
			m.note = "NOTHING CAN BE OPENED OUTSIDE CONN'S TMUX SERVER"
		case !ok || pl.path == "":
			m.note = "NO PLACE UNDER THE CURSOR"
		default:
			dir := pl.path
			return m, m.openShell(dir, pl.path)
		}
	}
	return m, nil
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

// A row conn made for a process it started, before the process table
// has it: the row, the pane it is in, where it goes, and how long it
// stands.
type pendingRow struct {
	row   entry
	pane  pane
	place string
	until time.Time
}

// reach puts a process in the slot, off the loop, and hands back the
// terminal that is in the slot once it is there.
func (m model) reach(target pane, tty string) tea.Cmd {
	srv := m.srv
	return func() tea.Msg {
		if err := srv.show(target); err != nil {
			return noteMsg{strings.ToUpper(err.Error())}
		}
		return reachedMsg{tty}
	}
}

// openShell opens a shell at a place, off the loop, and hands back what
// tmux said of it, which is a row.
func (m model) openShell(dir, place string) tea.Cmd {
	srv := m.srv
	return func() tea.Msg {
		sh, err := srv.open(dir)
		if err != nil {
			return noteMsg{strings.ToUpper(err.Error())}
		}
		return openedMsg{shell: sh, place: place}
	}
}

// withRow is the places with a row at the top of the place it belongs
// to; a place nothing was happening in yet is made for it. The places
// it is given are left as they were: a reading is composed once and
// held, and a row conn adds to what it shows must not become a row of
// the reading itself.
func withRow(places []place, at string, e entry) []place {
	out := make([]place, len(places))
	copy(out, places)
	for i, pl := range out {
		if pl.path == at {
			out[i].entries = append([]entry{e}, pl.entries...)
			return out
		}
	}
	return append([]place{{path: at, entries: []entry{e}}}, out...)
}

// hasPid says whether a process is among what was read.
func hasPid(places []place, pid int) bool {
	for _, pl := range places {
		for _, e := range pl.entries {
			if e.pid == pid {
				return true
			}
		}
	}
	return false
}

// serverCmd runs a server action off the loop; what goes wrong is said
// on the bottom row.
func (m model) serverCmd(act func() error, done string) tea.Cmd {
	return func() tea.Msg {
		if err := act(); err != nil {
			return noteMsg{strings.ToUpper(err.Error())}
		}
		return noteMsg{done}
	}
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
