package main

import (
	"maps"
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

// Across the foot of the window is the bar, which is tmux's status line
// and conn's line: a chip for what the keys are doing, what conn has to
// say, and the station and the clock. It is written in tmux.go; conn
// puts its half of it there through saying, below.
//
// The program holds three views. The console comes on first: the header
// at once, from what is known before anything is read; the station is
// read meanwhile, and the readout comes on when it is in hand and its
// beat has passed, then the checks one by one, then the verdict, in
// under a second. A key skips to the end; a key at the end continues to
// the watch. The watch is what is running, by place, read again every
// two seconds while it is up; j and k move the cursor, which follows
// its process across readings; tab takes it to whatever is waiting on
// you, longest held up first and round again; i opens the look on the
// row under the cursor — what conn knows of it past the six columns a
// row has room for — which opens in the slot, beside the watch rather
// than over it, follows the cursor from there, and closes on i again; c brings the console back, and any key there returns
// to the watch. The console is a page: in the server it
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
		slotLook bool            // the slot holds the look, which i closes rather than opens
		err      string
		gen      int
		// The processor time every process had used as of this reading,
		// and when it was taken: what the next reading asks against to
		// tell work from waiting.
		cpu   map[int]time.Duration
		cpuAt time.Time
	}
	watchTickMsg struct{ gen int }     // the watch is due to be read again
	openedMsg    struct{ shell shell } // a shell was opened; the cursor goes to it once it is read
	reachedMsg   struct{ tty string }  // a process was put in the slot
	lookedMsg    struct{ on bool }     // the look was put in the slot, or taken out of it
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
	told     int // the cursor as last published for the look to follow
	// Whether the look is in the slot, which is what makes i a toggle.
	// conn sets it when it puts the page there or takes it away, and a
	// reading corrects it — asking tmux on the keypress would be a
	// process between the key and what it does, for something conn
	// already knows.
	looking  bool
	entering bool // the console is waiting on a reading to go to the watch
	// What conn last put on the bar, so it is written when the words
	// change and not on every pass through Update.
	saidNote, saidMode string
	// A shell conn has just opened: the pid the cursor goes to once the
	// process table has it, and how long that is waited for.
	awaited  int
	until    time.Time
	watchErr string
	watchGen int // which stay on the watch the ticks belong to
	// The last reading's processor times, and when they were read: a
	// process is working by what it has spent since, not by what it has
	// spent altogether.
	cpuWas map[int]time.Duration
	cpuAt  time.Time
	// The list: the projects as the roots were last walked, what has been
	// typed to narrow them, and which of the rows the cursor is on.
	projects    []project
	filter      string
	pcursor     int
	scanning    bool
	projectsErr string
	uid         int
	roots       func(string) string
	isProject   func(string) bool
	projRoots   []string // where the checkouts are kept, for naming places by

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
	home, _ := os.UserHomeDir()
	roots := realRoots(projectRoots(home))
	isProject := projectDirs(roots)
	return model{
		lit:  true,
		told: -1, // nothing published yet; the first cursor is news

		head:      station{build: readBuild(), session: readSession()},
		now:       time.Now(),
		p:         p,
		uid:       os.Getuid(),
		roots:     placeRoots(isProject),
		isProject: isProject,
		projRoots: roots,
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
	w := composeWatch(m.places, m.panes, m.slot, m.projRoots, m.head.session.home, m.now, m.watchErr)
	w.inside = m.inside
	return w
}

// projectsReport is the list's words as things stand, and projectRows
// the rows the filter leaves, which the cursor is an index into.
func (m model) projectsReport() projectsReport {
	return composeProjects(m.projects, m.filter, projectRoots(m.head.session.home), m.head.session.home, m.scanning, m.projectsErr)
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
	gen, uid, roots, isProject := m.watchGen, m.uid, m.roots, m.isProject
	was, wasAt := m.cpuWas, m.cpuAt
	var srv *server
	if m.inside {
		srv = m.srv
	}
	return func() tea.Msg {
		procs, err := readProcesses(uid)
		if err != nil {
			return watchMsg{err: "THE PROCESS TABLE COULD NOT BE READ: " + err.Error(), gen: gen}
		}
		// How each process stands past what the table says: anything is
		// working by the processor time it spent since the last reading,
		// which is why that reading is kept, and an agent answers for
		// itself instead - working, or waiting on you.
		now, nowAt := cpuOf(procs), time.Now()
		how := map[int]standing{}
		for pid := range cpuWorking(was, wasAt, procs, nowAt) {
			how[pid] = standing{working: true}
		}
		maps.Copy(how, agentStandings(procs))
		msg := watchMsg{places: watch(procs, uid, roots, isProject, how), gen: gen, cpu: now, cpuAt: nowAt}
		if srv != nil {
			if slot, ok, err := srv.slot(); err == nil && !ok {
				msg.noSlot = true
			} else if ok {
				msg.slot, msg.slotDead, msg.slotLook = slot.tty, slot.dead, slot.look
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

// Update answers a message and, whatever came of it, publishes where
// the cursor ended up. Every path that moves it — j and k, tab, a
// reading that carried it along, the shell conn just opened — publishes
// by going through here, which is the point of doing it in one place
// rather than at each of them: a move that forgot to say so would leave
// the look reading a row nobody is looking at.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	if nm, ok := next.(model); ok {
		// A reading says it again whether or not it moved, so a file
		// gone missing — a state directory swept, a server that came
		// back — comes back on the next beat rather than staying gone
		// until somebody presses j.
		_, reading := msg.(watchMsg)
		nm = nm.published(reading)
		nm, said := nm.saying()
		if said != nil {
			return nm, tea.Batch(cmd, said)
		}
		return nm, cmd
	}
	return next, cmd
}

// saying puts what conn has to say on the bar, and the mode its chip
// shows, when either has changed since the last telling. Going through
// here is the point: a note is set from a dozen places and every one of
// them would otherwise have to remember to say so.
//
// The bar is tmux's line and conn reaches it by setting an option on the
// server, which is a process — so it is written when the words change,
// which is on a keypress and rarely, and never on a beat. Off the loop,
// since a process between a key and what it does is a key that feels
// slow.
func (m model) saying() (model, tea.Cmd) {
	if !m.inside || m.srv == nil {
		return m, nil
	}
	mode := ""
	if m.kill != nil {
		// The kill's question is the one thing conn does that takes the
		// next key whatever it is, and the chip is where a mode that
		// swallows keys belongs.
		mode = "CONFIRM"
	}
	if m.note == m.saidNote && mode == m.saidMode {
		return m, nil
	}
	m.saidNote, m.saidMode = m.note, mode
	note, srv := m.note, m.srv
	return m, func() tea.Msg { _ = srv.say(note, mode); return nil }
}

// published tells the cursor where it is, when it has moved since the
// last telling or when a reading is saying it again. The look follows
// it; nothing else reads it.
func (m model) published(again bool) model {
	pid := m.cursor
	// Off the watch there is no cursor on a process. The subject is not
	// unchosen by going to the list to open something — the look goes
	// on reading the row it was given — so nothing is said rather than
	// a nothing said.
	//
	// With no home there is nowhere to say it: the path would be a
	// relative one, and conn does not write beside whatever directory
	// it happens to have been started in.
	if m.view != viewWatch || !m.inside || m.head.session.home == "" {
		return m
	}
	if pid != m.told || again {
		m.told = pid
		tellCursor(cursorPath(m.head.session.home), pid)
	}
	return m
}

func (m model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		m.slot, m.looking = msg.shell.pane.tty, false
		m.awaited, m.until = msg.shell.pid, time.Now().Add(waitForOpened)
		m.watchGen++
		return m, m.readWatch()
	case lookedMsg:
		// The page is up, or down, and conn knows it without reading the
		// server: the next i is a keypress away and has to decide which
		// way it goes. The reading is taken again from here so a reading
		// already in flight, which saw the slot as it was before, cannot
		// land afterwards and say otherwise.
		m.looking = msg.on
		m.watchGen++
		return m, m.readWatch()
	case reachedMsg:
		// The pane is in the slot; conn knows it now and does not have to
		// read the server to find out, so the row says so at once.
		//
		// A pane is the whole tree in it, so reaching one from a row
		// down inside it reaches the head. The cursor goes there too:
		// it was on the row that asked, but the row that answered is
		// the head, and leaving the two apart would put the mark on one
		// row while the cursor sat on another — the sub-process looking
		// picked out for being the one thing in the pane that is not
		// what is in the slot.
		m.slot, m.looking = msg.tty, false
		if pid, at, ok := headOf(m.places, msg.tty); ok {
			m.cursor, m.cursorAt = pid, at
		}
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
		m.looking = msg.slotLook
		if msg.cpu != nil {
			m.cpuWas, m.cpuAt = msg.cpu, msg.cpuAt
		}
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
		// The reading the console was waiting on: the watch goes up with
		// its rows already in it, drawn at the rail's width, and the slot
		// opens beside a frame that is already the shape it will be.
		var cmds []tea.Cmd
		if m.entering {
			m.entering, m.view = false, viewWatch
			if m.inside {
				cmds = append(cmds, m.serverCmd(func() error { return m.srv.narrow() }, ""))
			}
		}
		if m.view == viewWatch {
			switch {
			// A home without its slot gets one; the next reading finds it.
			case m.inside && msg.noSlot:
				cmds = append(cmds, m.watchTick(), m.openSlot())
			// A slot whose pane died stays the shape it was; only what is
			// in it is replaced, so the rail never has to give up its
			// width and take it back.
			case m.inside && msg.slotDead:
				cmds = append(cmds, m.watchTick(), m.reviveSlot())
			default:
				cmds = append(cmds, m.watchTick())
			}
		}
		return m, tea.Batch(cmds...)
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
// opens claude there instead, and alt+a opens the picker over what
// claude left suspended there. tab goes to what is waiting on you,
// longest first, and round again; i looks at the cursor's row. x asks
// to end the cursor's process,
// and arms the question rather than the ending: the next key answers
// it.
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
	// The list's own key, which reaches it from wherever conn is and is
	// what the prefix chord sends. p cannot serve: it is the list's key
	// on the watch, where it is a key, but on the list and the picker it
	// is a letter being typed into the line, and on the console it is one
	// of the any-keys that continue to the watch. So the chord has a key
	// of its own, and it is the same key wherever it is pressed.
	if k == "alt+p" {
		return m.toProjects()
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
		// The console holds until the watch has something to show. Going
		// at once put an empty watch up, filled it a tenth of a second
		// later when the table had been read, and moved it to the rail's
		// width after that — three screens to arrive at one. The console
		// is a still page and a moment more of it is not seen, where a
		// watch assembling itself is.
		//
		// Only the first time. Coming back from the console the rows of
		// the last stay are still in hand, a couple of seconds old, and
		// the watch goes up with them at once while the reading on its
		// way brings them up to date.
		m.watchGen++
		if len(m.places) == 0 && m.watchErr == "" {
			m.entering = true
			return m, m.readWatch()
		}
		m.view = viewWatch
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
	case k == "alt+a":
		_, pl, ok := m.under()
		switch {
		case !m.inside:
			m.note = "NOTHING CAN BE OPENED OUTSIDE CONN'S TMUX SERVER"
		case !ok || pl.path == "":
			m.note = "NO PLACE UNDER THE CURSOR"
		default:
			return m.openResume(pl.path, []string{pl.path})
		}
	case k == "i":
		// i is the key for the page, and the key for the page is what a
		// reader reaches for to be rid of it. Closing wants no row under
		// the cursor: the page is there whatever the cursor is on, and
		// refusing to close it because the watch has emptied would leave
		// it stuck.
		//
		// Where the row is one conn holds, closing goes to it. The page
		// is a reading of that row and the row is right there in a pane
		// — read about it, then be in it — and an empty slot is a worse
		// answer than the thing the page was about. What cannot be
		// reached closes to the empty slot as before.
		e, _, ok := m.under()
		switch {
		case !m.inside:
			m.note = "NOTHING CAN BE SHOWN OUTSIDE CONN'S TMUX SERVER"
		case m.looking && ok && m.panes[e.tty].id != "":
			return m, m.reach(m.panes[e.tty], e.tty)
		case m.looking:
			return m, m.closeLook()
		case !ok:
			m.note = "NOTHING UNDER THE CURSOR"
		default:
			return m, m.openLook()
		}
	case k == "tab":
		// The ring of what is waiting on you, longest held up first: the
		// first press goes to the one that has waited longest, and each
		// after it to the next, round and back. It is the one question
		// the watch asks of you, so it gets the one key that means go to
		// what wants me.
		round := waitingRound(m.places)
		if len(round) == 0 {
			m.note = "NO AGENT IS WAITING ON YOU"
			return m, nil
		}
		next := round[0]
		for i, e := range round {
			if e.pid == m.cursor {
				next = round[(i+1)%len(round)]
				break
			}
		}
		m.cursor, m.cursorAt = follow(m.places, next.pid, m.cursorAt)
	case k == "p":
		return m.toProjects()
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
// letter to type; alt+a opens the picker over what claude left
// suspended at the row, group included, the same way — not a plain A,
// which would take a letter the filter can still be typed with, and
// not ctrl+shift+a, which is not its own chord to any terminal at all,
// alphabetic ctrl combinations being their letter's own case already;
// esc goes back without opening anything, and ctrl+c is what it is
// everywhere.
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
	case k == "alt+a":
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

// toProjects opens the list, from wherever conn is, and walks the roots
// again for it: the list is what could be worked on rather than what is
// being worked on, so it is read when it is asked for and not on a beat.
//
// From the console it gives the slot its side back, the way going to the
// watch does — the list is a rail view like the watch — and it calls off
// the console's wait on a reading, or that reading would land a moment
// later and put the watch up over it.
func (m model) toProjects() (tea.Model, tea.Cmd) {
	console := m.view == viewConsole
	m.view, m.filter, m.pcursor, m.scanning = viewProjects, "", 0, true
	m.entering = false
	if console && m.inside {
		return m, tea.Batch(m.scanProjects(), m.serverCmd(func() error { return m.srv.narrow() }, ""))
	}
	return m, m.scanProjects()
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
	return composeResume(m.convos, m.convosPlace, m.rfilter, m.head.session.home, m.now, m.convosLoading)
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
// cols is the width conn draws in, which is the rail's own where conn is
// a rail. Inside the server, off the console, the rail is railWidth: conn
// holds tmux to that (see the resize in WindowSizeMsg) rather than taking
// whatever width it is given, so it draws to it as well instead of
// waiting to be told the pane has become one. That is what makes going
// to the watch one change of the screen — the frame conn paints is
// already the shape the pane is about to be, so the split has nothing to
// reflow and no frame is ever drawn to a width that is on its way out.
//
// Never wider than the terminal: a window narrower than the rail is
// still the whole of what there is to draw in.
func (m model) cols() int {
	if m.inside && m.view != viewConsole {
		return min(railWidth, m.width)
	}
	return m.width
}

func (m model) View() tea.View {
	var rows []row
	width := m.cols()
	switch m.view {
	case viewWatch:
		rows = drawWatch(m.watchReport(), m.cursor, width, m.height, m.p)
	case viewProjects:
		rows = drawProjects(m.projectsReport(), m.pcursor, width, m.height, m.p)
	case viewResume:
		rows = drawResume(m.resumeReport(), m.rcursor, width, m.height, m.p)
	default:
		r := m.report()
		r.lit = m.lit
		rows = screen(r, width, m.height, m.p)
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
