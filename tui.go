package main

import (
	"maps"
	"os"
	"slices"
	"strings"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Across the foot of the window is the status line, which is tmux's
// status line and an annunciator panel: dark until something conn's
// keys are doing lights it. It is written in tmux.go; conn lights its
// half through saying, below.
//
// The program holds three views. The console comes on first: the header
// at once, from what is known before anything is read; the station is
// read meanwhile, and the readout comes on when it has been read and
// its beat has passed, then the checks one by one, then the verdict, in
// under a second. A key skips to the end; a key at the end continues to
// the processes view. The processes view is what is running, by
// project, read again every two seconds while it is up; j and k move
// the cursor, which follows its process across readings; tab takes it
// to whatever is waiting on you, longest held up first and round again;
// enter goes into the row under the cursor and esc goes back into the
// one you came out of, so a look down the list and back costs nothing;
// the page is the readout on the row under the cursor — what conn knows
// of it past the six columns a row has room for — which the workspace
// holds while the keys are on the panel, beside the processes view
// rather than over it, following the cursor from there, with nothing
// pressed for it; c brings the console back, and any key there returns
// to the processes view. The console is a page: in
// the server it takes the whole window while it is up, and the bay has
// its side again on the way back to the processes view. The words of
// both are said again each second, from what was read and the clock as
// it stands.
//
// p is the list: every project the roots hold, whether anything is
// running in it or not, walked as the view comes on. It is a line typed
// into, so the keys the other views are worked by are characters there;
// enter opens a shell at the row under the cursor and comes back to the
// processes view, where the shell shows, and esc comes back without
// opening anything.
//
// In conn's tmux server, conn is the panel on the left of the home
// window; when the processes view first comes on it opens the bay
// beside it, with a hold in it, and opens it again should it close.
// Enter puts the cursor's process in the bay, when it is in a pane of
// the server; s opens a shell at the cursor's project there; q and
// ctrl+c detach, and the server keeps on. Without the server, q and
// ctrl+c close conn.

// The views.
const (
	viewConsole = iota
	viewProcesses
	viewProjects
	viewSessions
	viewRoots // conn has not been told where the work is, and is asking
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

// processesEvery is how often the processes view reads the process
// table at rest. While it is waiting on a shell conn has just opened,
// it reads again as soon as it can: a shell takes a moment to reach the
// table, and two seconds of the cursor sitting on the old row is the
// shell feeling slow to open. waitForOpened is how long that is worth
// doing before the shell is given up on.
const (
	processesEvery = 2 * time.Second
	processesSoon  = 150 * time.Millisecond
	waitForOpened  = 3 * time.Second
)

// The console's alarms blink like annunciators on a panel: lit for a
// second, dark for half of one. The dark is the shorter half — the
// blink is there to catch the eye, not to take the words away.
const (
	blinkLit  = time.Second
	blinkDark = time.Second / 2
)

type (
	stageMsg     struct{}          // the next stage is due
	clockMsg     struct{}          // the second has turned
	stationMsg   struct{ station } // the station is read
	processesMsg struct {          // the process table is read
		projects   []project
		panes      map[string]pane // the server's panes by terminal
		bay        string          // the terminal in the bay
		noBay      bool            // home has no bay beside the panel
		bayDead    bool            // the bay's pane held on remain-on-exit, its process gone
		bayReadout bool            // the bay holds the readout, so the page is up
		bayHelp    bool            // the bay holds the manual, and the panel says HELP
		bayActive  bool            // the keys are in the bay, by tmux's own word
		err        string
		// The projects' .conn files as this reading found them, kept on
		// the model for the next reading to stat against; see declared.go.
		declared map[string]declared
		// The projects whole, where projects is the fold of them.
		tree []project
		gen  int
		// The processor time every process had used as of this reading,
		// and when it was taken: what the next reading asks against to
		// tell work from waiting.
		cpu   map[int]time.Duration
		cpuAt time.Time
		// Each row as this reading saw it stand, and since when: what
		// the next reading dates a row's status against.
		stood map[int]stood
		acts  map[string]activitySeen
		// The table's record behind each row, for the page; see cursor.go.
		records map[int]record
		// The roots the reading found the file naming, where they are
		// not the ones conn was on: the reading was made on these, and
		// the model goes onto them with it.
		rooted *rooting
	}
	processesTickMsg struct{ gen int }        // the processes view is due to be read again
	openedMsg        struct{ shell shell }    // a shell was opened; the cursor goes to it once it is read
	noticeMsg        struct{ text string }    // something asked of the server was not done, and this is why
	raisedMsg        struct{ shells []shell } // a project's declared processes were brought up, parked
	reachedMsg       struct{ tty string }     // a process was put in the bay
	readoutMsg       struct{ on bool }        // the readout was put in the bay, or taken out of it
	helpMsg          struct{ on bool }        // the manual was put in the bay
	blinkMsg         struct{ gen int }        // the chip's half is up
	projectsMsg      struct {                 // the roots were walked
		projects []projectRow
		err      string
	}
	sessionsMsg struct { // a project's suspended sessions were read
		dirs     []string
		sessions []session
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
	lit      bool // the annunciators are showing this half of the blink
	blinkGen int  // which run of the blink a turn belongs to
	ticking  bool // the blink's tick is in flight, because something annunciates
	projects []project
	cursor   int     // the pid the cursor is on
	cursorAt int     // where in the rows it was, for when the pid goes
	told     subject // the subject as last published for the readout to follow
	// The table's record behind each row as last read, published with
	// the rows for the page; see cursor.go.
	records map[int]record
	// Whether the readout is in the bay, so the page is not asked for
	// twice. conn sets it when it puts the page there or takes it away,
	// and a reading corrects it — asking tmux on every reading would be
	// a process for something conn already knows.
	looking bool
	// helping is whether the manual is the thing in the workspace. While
	// it is, the panel says HELP and no row is under the cursor: the
	// manual is not a process, so there is no row it belongs to and a
	// cursor left sitting on one would say the keys were about that row
	// when they are about reading.
	helping bool
	// Whether the keys are on the panel. conn is told by the terminal
	// when they arrive and when they leave, and knows on its own when
	// its own reaching sent them away, so a terminal that reports no
	// focus does not leave conn guessing where they are.
	focused  bool
	entering bool // the console is waiting on a reading to go to the processes view
	// Whether conn has written the status line once since it started,
	// and the words it last put there, so each is written when it
	// changes and not on every pass through Update.
	// The option outlives the conn that set it — a reground respawns the
	// panel, and the fresh conn inherits whatever the last one left — so
	// an empty saidKeys means "not written yet", not "the server says
	// nothing", and the first writing goes out whatever it holds.
	said     bool
	saidKeys string
	// And the station's own word, for the line to wear while the keys
	// are off the panel; see station.
	saidStation string
	// A shell conn has just opened: the pid the cursor goes to once the
	// process table has it, and how long that is waited for.
	awaited      int
	until        time.Time
	processesErr string
	// What the server would not do, in its own words, said under the
	// rows until the next key. A shell that could not be opened left
	// nothing on the screen at all: the operator pressed a key and
	// nothing happened, which is the one thing conn should never leave
	// them with.
	notice string
	processesGen int // which stay in the processes view the ticks belong to
	// The pane the keys were in when a chord brought them to the panel,
	// for a view there is something to cancel out of. Blank where the
	// keys were already here.
	from string
	// The last reading's processor times, and when they were read: a
	// process is working by what it has spent since, not by what it has
	// spent altogether.
	cpuWas map[int]time.Duration
	stood  map[int]stood
	acts   map[string]activitySeen
	cpuAt  time.Time
	// The list: the projects as the roots were last walked, and the line
	// typed into to narrow them, with the cursor among the rows it leaves.
	walked      []projectRow
	find        typed
	scanning    bool
	projectsErr string
	uid         int
	roots       rooting // where the checkouts are kept, and the finders built on it

	// The sessions view: a project's suspended sessions, as last read,
	// what has narrowed them, and which of the rows the cursor is on.
	sessionsDirs    []string // the directories asked for; a stale answer's guard
	sessionsProject string
	sessions        []session
	sessionsLoading bool
	rfind           typed // the line typed into, and the cursor among the rows it leaves
	// The manual: where the keys were when prefix ? fired, and the row
	// that was under the cursor, so that leaving it puts both back.
	// See leftHelp.
	helpFrom   string
	helpCursor int

	// The asking view: the path being typed, with the cursor among the
	// directories answering it, and what went wrong saving, where
	// something did.
	asking  typed
	rootErr string

	// kill is a kill x has asked for and not yet answered; nothing else
	// binds while it is not nil.
	kill *pendingKill
	// firstG is a g that has been pressed and is nothing on its own: the
	// first half of gg, waiting to see whether the next key is its
	// second. Unlike a kill it asks nothing and says nothing — a motion
	// half typed is not a question — so any other key simply goes on to
	// be the key it is.
	firstG bool

	srv    *server         // conn's tmux server, when there is one
	inside bool            // this conn is the panel of the server's home window
	self   string          // this binary, for the hold
	panes  map[string]pane // the server's panes by terminal, as last read
	bay    string          // the terminal in the bay, as last read
	// The terminal that was in the bay before that one, which is where
	// the other-process chord goes back to.
	lastBay string

	// What docker last said, and the feed that says it. The containers
	// are read beside the process table rather than in it, so a reading
	// merges what is already here and never waits on the daemon; stalled
	// is docker having gone quiet, which the view admits rather than
	// showing yesterday's rows as though they were today's.
	containers []container
	// The projects' .conn files as last read; see declared.go.
	declared map[string]declared
	// The processes as read, whole, and whether the view shows them
	// so: at rest it shows the fold of them; see fold.go.
	tree          []project
	full          bool
	dockerFeed    *dockerFeed
	dockerStalled bool
	// The last terminal the workspace held that was work: where esc
	// goes back into. It is not the bay, because while the keys are on
	// the panel the page is in the bay and the work has been put back
	// in a window of its own; it is what the bay held before the page
	// borrowed it, which is the process the operator was last in.
	lastIn string
}

func newModel(p palette) model {
	home, _ := os.UserHomeDir()
	// A config that will not parse is the view's to report, not the
	// model's to come up on: newModel takes the roots it is left with
	// and the first scan says what is wrong with the file.
	configured, _ := projectRoots(home)
	m := model{
		lit:     true,
		focused: true, // conn comes up with the keys in the panel
		// conn comes up on the console, which annunciates, and Init sets
		// the blink going with everything else.
		ticking: true,

		head: station{build: readBuild(), login: readLogin()},
		now:  time.Now(),
		p:    p,
		uid:  os.Getuid(),
	}
	return m.rooted(rootOn(configured))
}

// A rooting is conn on a set of roots: the directories as they were
// configured, the same as the process table names them, and the two
// finders built on them — which directories are projects, and which
// project holds a directory. The finders remember what they found, so
// a rooting is built once for a set of roots and kept until the set
// changes.
type rooting struct {
	configured, real []string
	isProject        func(string) bool
	rootOf           func(string) string
}

// rootOn is the rooting for a set of configured roots.
func rootOn(configured []string) rooting {
	r := rooting{configured: configured, real: realRoots(configured)}
	r.isProject = projectDirs(r.real)
	r.rootOf = rootFinder(r.isProject)
	return r
}

// rooted puts conn on a rooting. It is where conn comes up, where the
// asking view was answered, and where a reading found the file changed
// under it: the roots are what the processes view names projects by,
// and a view naming them by roots the operator has since edited is a
// view that stopped reading the file it says it reads.
func (m model) rooted(r rooting) model {
	m.roots = r
	return m
}

// report is the console's words as things stand: from the station once
// it is read, from the header's part of it before, and as the terminal
// in hand can hold them. The stages are counted off the report the
// screen will actually draw, so a console that gave up its per-root
// lines does not go on ticking through stages that have no row.
func (m model) report() report {
	st := m.head
	if m.st != nil {
		st = *m.st
	}
	return fitted(compose(st, m.now), m.height)
}

// processesReport is the processes view's words as things stand.
func (m model) processesReport() processesReport {
	w := composeProcesses(m.projects, m.panes, m.bay, m.roots.real, m.head.login.home, m.now, m.processesErr, m.dockerStalled)
	w.inside, w.lit, w.notice = m.inside, m.lit, m.notice
	return w
}

// listRows is the list as it stands before any filter: the projects the
// walk found, each with the processes conn holds a pane for in it under
// it, and the work happening off every project at the foot.
func (m model) listRows() []projectRow {
	return withProcesses(m.walked, m.projects, m.panes, m.roots.real, m.head.login.home)
}

// projectsReport is the list's words as things stand, and projectRows
// the rows the filter leaves, which the cursor is an index into.
func (m model) projectsReport() projectsReport {
	b := composeProjectsAt(m.listRows(), m.find.text, m.roots.configured, m.head.login.home, m.scanning, m.projectsErr)
	b.caret = m.find.cur
	return b
}

func (m model) projectRows() []projectRow {
	return matching(m.listRows(), m.find.text)
}

// atCursor is the row the list's cursor stands on, where there is one,
// and followRow finds that row again once a reading has changed the
// list under it: a process by its pid and a project by its path, and
// where neither is still listed, the place it was. The list is read
// live now, so a cursor that were only an index would walk on its own
// as processes come and go.
func (m model) atCursor() (projectRow, bool) {
	rows := m.projectRows()
	if m.find.at >= len(rows) {
		return projectRow{}, false
	}
	return rows[m.find.at], true
}

func followRow(rows []projectRow, was projectRow, at int) int {
	if was.pid != 0 {
		for i, r := range rows {
			if r.pid == was.pid {
				return i
			}
		}
	}
	// The process has ended. Its project is where the operator was
	// looking, and a process row carries that project's path, so the
	// cursor falls back to the row the work was under rather than to
	// whatever has moved up into its place.
	if was.path != "" {
		for i, r := range rows {
			if r.pid == 0 && r.path == was.path {
				return i
			}
		}
	}
	return clamp(at, len(rows))
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{readStationCmd, startDocker, m.nextStage(), nextSecond(m.now), m.nextBlink()}
	if m.inside {
		cmds = append(cmds, m.serverCmd(func() error { return m.srv.wide() }))
	}
	return tea.Batch(cmds...)
}

func readStationCmd() tea.Msg {
	return stationMsg{readStation()}
}

// readProcesses reads the process table, and in the server its panes
// and the bay, and composes the processes view off them. It reads and
// does nothing else; what the reading calls for is decided when it
// comes back.
func (m model) readProcesses() tea.Cmd {
	gen, uid, roots, isProject := m.processesGen, m.uid, m.roots.rootOf, m.roots.isProject
	home, configured := m.head.login.home, m.roots.configured
	containers := m.containers
	declared, full := m.declared, m.full
	was, wasAt, stoodWas, actsWas := m.cpuWas, m.cpuAt, m.stood, m.acts
	var srv *server
	if m.inside {
		srv = m.srv
	}
	return func() tea.Msg {
		procs, err := readProcesses(uid)
		if err != nil {
			return processesMsg{err: "THE PROCESS TABLE COULD NOT BE READ: " + err.Error(), gen: gen}
		}
		// The roots as the file names them now. The operator edits the
		// file from inside conn, and the list walks it fresh; the
		// reading names projects by the same file, and takes it as it
		// now stands rather than as it stood when conn came up. A file
		// that will not parse changes nothing: the list says so, and
		// conn is not going to name projects by a guess at what it was
		// about to say.
		var rerooted *rooting
		if home != "" {
			if now, err := projectRoots(home); err == nil && !slices.Equal(now, configured) {
				r := rootOn(now)
				rerooted, roots, isProject = &r, r.rootOf, r.isProject
			}
		}
		// How each process stands past what the table says: anything is
		// working by the processor time it spent since the last reading,
		// which is why that reading is kept, and a contact answers for
		// itself instead - working, or waiting on you.
		now, nowAt := cpuOf(procs), time.Now()
		how := map[int]status{}
		for pid := range cpuWorking(was, wasAt, procs, nowAt) {
			how[pid] = status{working: true}
		}
		maps.Copy(how, contactStatuses(procs))
		// The panes come first, because a pane conn opened to watch a
		// container is two things to the reading at once: the terminal
		// that container's row will stand on, and a process that must
		// not stand for itself. A docker logs beside the service it is
		// showing would be the same thing listed twice.
		var panes map[string]pane
		if srv != nil {
			panes, _ = srv.panes()
		}
		// A pane reading a service stands in for that service's
		// terminal and is kept off the view, since a docker logs listed
		// beside the service it is showing is the same thing twice. A
		// pane holding a shell inside a container is neither: it is the
		// operator's own work, it is listed, and it is filed under the
		// service it is inside rather than taking the slot the service's
		// terminal is in.
		paneOf, shellIn, watching := map[string]string{}, map[string]string{}, map[string]bool{}
		for tty, p := range panes {
			if p.container != "" {
				paneOf[p.container], watching[tty] = tty, true
			}
			if p.shellIn != "" {
				shellIn[tty] = p.shellIn
			}
		}
		if len(watching) > 0 {
			kept := procs[:0]
			for _, p := range procs {
				if !watching[p.tty] {
					kept = append(kept, p)
				}
			}
			procs = kept
		}
		projects := projectsFrom(procs, uid, roots, isProject, how)
		records := recordsOf(procs, projects)
		// And what docker is holding up, which the table cannot show: a
		// container is not a process of this machine, and compose says
		// where each belongs by the directory it was started for. What
		// docker last said is already here — the feed brings it as it
		// happens — so this costs the reading nothing and waits on no
		// daemon.
		projects = attachContainers(projects, containers, roots, paneOf, shellIn)
		// And what the projects declare should be working them, which
		// the table has no word for until it is: a stat per project,
		// and a read where a file changed.
		declared = refreshDeclared(declared, declaredPaths(projects, isProject))
		projects = attachDeclared(projects, declared, panes)
		msg := processesMsg{projects: projects, tree: projects, panes: panes, gen: gen, cpu: now, cpuAt: nowAt,
			stood: sinceSeen(projects, stoodWas, wasAt, nowAt), acts: activities(projects, actsWas),
			records: records, rooted: rerooted, declared: declared}
		if !full {
			msg.projects = fold(projects)
		}
		if srv != nil {
			if bay, ok, err := srv.bay(); err == nil && !ok {
				msg.noBay = true
			} else if ok {
				msg.bay, msg.bayDead, msg.bayReadout, msg.bayHelp = bay.tty, bay.dead, bay.readout, bay.help
				msg.bayActive = bay.active
			}
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

// annunciating says whether anything conn is drawing blinks as things
// stand: the console's verdict while the console is up, and a row in
// the processes view that is waiting on you. Nothing else does — a
// fault in the processes view wears a chip and keeps it, since a
// process you suspended yourself is not asking anything of you, and a
// word that blinks all day is a word that is never seen.
func (m model) annunciating() bool {
	switch m.view {
	case viewConsole:
		return true
	case viewProcesses:
		return len(waitingRound(m.projects)) > 0
	default:
		return false
	}
}

// blinked starts the blink's tick when something begins to annunciate
// and lets it stop when nothing does, so a view with nothing held up on
// it is not redrawn a second and a half at a time for nothing. Going
// through here means no view has to remember to start it: what blinks
// is decided in one place and the tick follows.
func (m model) blinked() (model, tea.Cmd) {
	want := m.annunciating()
	if want == m.ticking {
		return m, nil
	}
	// A turn already in flight belongs to the run that is ending, and is
	// dropped when it lands; the lit half is where anything not blinking
	// rests.
	m.ticking, m.lit, m.blinkGen = want, true, m.blinkGen+1
	if !want {
		return m, nil
	}
	return m, m.nextBlink()
}

// nextBlink is the turn of the annunciator's other half, each half its
// own length. A turn from an earlier run of the blink is dropped.
func (m model) nextBlink() tea.Cmd {
	d := blinkLit
	if !m.lit {
		d = blinkDark
	}
	gen := m.blinkGen
	return tea.Tick(d, func(time.Time) tea.Msg { return blinkMsg{gen} })
}

// processesTick is when the processes view reads again: soon while it
// waits on a shell conn opened, and at its own pace otherwise.
func (m model) processesTick() tea.Cmd {
	gen, every := m.processesGen, processesEvery
	if m.awaited != 0 {
		every = processesSoon
	}
	return tea.Tick(every, func(time.Time) tea.Msg { return processesTickMsg{gen} })
}

// Update answers a message and, whatever came of it, publishes where
// the cursor ended up. Every path that moves it — j and k, tab, a
// reading that carried it along, the shell conn just opened — publishes
// by going through here, which is the point of doing it in one place
// rather than at each of them: a move that forgot to say so would leave
// the readout reading a row nobody is looking at.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	if nm, ok := next.(model); ok {
		// A reading says it again whether or not it moved, so a file
		// gone missing — a state directory swept, a server that came
		// back — comes back on the next beat rather than staying gone
		// until somebody presses j.
		_, reading := msg.(processesMsg)
		nm = nm.published(reading)
		nm, said := nm.saying()
		nm, blink := nm.blinked()
		if said != nil || blink != nil {
			return nm, tea.Batch(cmd, said, blink)
		}
		return nm, cmd
	}
	return next, cmd
}

// saying puts what conn knows about its own keys on the status line,
// when it has changed since the last telling. Going through here is the
// point: a question is armed and answered from a handful of keys and
// the views are left and entered from as many more, and every one of
// them would otherwise have to remember to say so.
//
// The status line is tmux's line and conn reaches it by setting an
// option, which is a process — so it is written when what it says
// changes, which is on a keypress, and never on a beat.
func (m model) saying() (model, tea.Cmd) {
	if !m.inside || m.srv == nil {
		return m, nil
	}
	keys, station := m.keys(), m.station()
	if m.said && keys == m.saidKeys && station == m.saidStation {
		return m, nil
	}
	m.said, m.saidKeys, m.saidStation = true, keys, station
	srv := m.srv
	return m, func() tea.Msg { _ = srv.say(keys, station); return nil }
}

// The word each panel view wears on the status line. The console takes
// none: it covers the whole window, and a page that fills the screen
// does not need the foot of it to say which page it is.
var viewWords = map[int]string{
	viewProcesses: "PROCS",
	// viewRoots takes none of its own: the chip in the view says what it
	// is for, at the width the question needs.
	viewProjects: "PROJECTS",
	viewSessions: "SESSIONS",
}

// keys is what conn knows about its own keys, for the left of the
// status line. Two things can be there, and a question armed comes
// first: it takes the next key whatever it is, so while it stands the
// view under it cannot be worked and the view's word would be a lie.
// The question is on the status line rather than the panel because the
// line spans the window, where the panel's forty-four columns cut the
// question before the part that says how to answer it.
//
// Otherwise the word for the view the keys are in. The views are worked
// by different keys — a letter narrows the rows in projects and
// sessions and is a command in processes — so which one has the keys is
// a state the operator is in, the same kind of thing PREFIX and COPY
// say. tmux shows it only while the keys are on the panel, so a bay
// with the keys in it leaves the position dark.
func (m model) keys() string {
	if m.kill != nil {
		return statusLineBlock("CONFIRM") + statusLineSay(m.kill.prompt)
	}
	// Reading the manual is a state the operator is in, like a question
	// armed, and it outranks the view's own word: while the manual is up
	// the panel is not being worked, and saying PROCS would name a view
	// whose keys are not what the operator is using.
	if m.helping && m.view == viewProcesses {
		return statusLineBlock(helpWord)
	}
	// The whole tree is a way of looking at the processes view rather
	// than a view of its own, and the line says so while it is on.
	if m.full && m.view == viewProcesses {
		return statusLineBlock(treeWord)
	}
	return statusLineBlock(viewWords[m.view])
}

// treeWord is what the line says while the processes view shows the
// whole tree.
const treeWord = "TREE"

// helpWord is what the line says while the manual is up.
const helpWord = "HELP"

// station is what the line says while the keys are not on the panel.
// Ordinarily nothing: the keys are in a process, and what that process
// is doing is its own business and is on its own screen. The manual is
// the one thing conn puts the keys into that is conn's own, and it says
// so, so that a page filling the workspace is not mistaken for a
// program the operator opened and has to get out of by guessing.
func (m model) station() string {
	if m.helping {
		return statusLineBlock(helpWord)
	}
	return ""
}

// published tells the cursor where it is, when it has moved since the
// last telling or when a reading is saying it again. The readout
// follows it; nothing else reads it.
func (m model) published(again bool) model {
	at := m.subject()
	// With no subject there is nothing to say. The subject is not
	// unchosen by going to a view with no cursor on anything — the
	// readout goes on reading what it was given — so nothing is said
	// rather than a nothing said.
	//
	// With no home there is nowhere to say it: the path would be a
	// relative one, and conn does not write beside whatever directory
	// it happens to have been started in.
	if at.none() || !m.inside || m.head.login.home == "" {
		return m
	}
	if at != m.told || again {
		m.told = at
		// The reading goes with the subject, as the panel shows it, so
		// the page says what the panel says and asks the machine
		// nothing; see cursor.go.
		// The tree whole, whatever the panel is showing of it: the page
		// says what runs a row and what it runs, folded or not.
		projects := m.tree
		if len(projects) == 0 {
			projects = m.projects
		}
		tellCursor(cursorPath(m.head.login.home), at, &reading{
			projects: projects, records: m.records, panes: m.panes,
			inside: m.inside, containers: m.containers, sessions: m.sessions,
		})
	}
	return m
}

// subject is what the page is about, as the panel has it: the process
// under the cursor in the processes view, and in the list the row the
// cursor is on — a process where the row is one, and the project
// otherwise, since a project is as much a thing to read about as a
// row in it. The other views have no cursor on anything.
func (m model) subject() subject {
	switch m.view {
	case viewProcesses:
		return subject{pid: m.cursor}
	case viewProjects:
		if row, ok := m.atCursor(); ok {
			if row.pid != 0 {
				return subject{pid: row.pid}
			}
			return subject{path: row.path}
		}
	case viewSessions:
		if rows := m.sessionsRows(); m.rfind.at < len(rows) {
			return subject{session: rows[m.rfind.at].ID}
		}
	}
	return subject{}
}

// containerAt is the container a row stands for, where it is one, as the
// panel has it from docker.
func (m model) containerAt(pid int) *container {
	for _, pl := range m.projects {
		for _, e := range pl.entries {
			if e.pid == pid {
				return reading{containers: m.containers}.containerOf(e)
			}
		}
	}
	return nil
}

// recordsOf is the table's record behind each row, for the page: what
// the page reads of a process that the row does not carry, kept for
// the rows alone rather than for the whole table.
func recordsOf(procs []process, projects []project) map[int]record {
	byPid := map[int]process{}
	for _, p := range procs {
		byPid[p.pid] = p
	}
	out := map[int]record{}
	for _, pl := range projects {
		for _, e := range pl.entries {
			if p, ok := byPid[e.pid]; ok {
				out[e.pid] = recordOf(p)
			}
		}
	}
	return out
}

func (m model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// The panel holds its width through a resize of the window, once it
		// is a panel: with the bay beside it, in the processes view or the
		// list.
		if m.inside && m.view != viewConsole && m.bay != "" && m.width != panelWidth {
			return m, m.serverCmd(func() error { return m.srv.holdPanel() })
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
	case noticeMsg:
		m.notice = msg.text
	case openedMsg:
		// The shell is in the bay; the table will have it in a moment, and
		// the cursor goes to it then. Until then the processes view reads
		// soon.
		m = m.slotted(msg.shell.pane.tty)
		m.looking, m.focused = false, false
		m.awaited, m.until = msg.shell.pid, time.Now().Add(waitForOpened)
		m.processesGen++
		return m, m.readProcesses()
	case raisedMsg:
		// The panes are parked and the keys stayed here; the cursor
		// goes to the first of them once the table has it.
		if len(msg.shells) > 0 {
			m.awaited, m.until = msg.shells[0].pid, time.Now().Add(waitForOpened)
		}
		m.processesGen++
		return m, m.readProcesses()
	case dockerReadyMsg:
		// The feed is running; from here conn waits on its word rather
		// than asking docker anything on a beat.
		m.dockerFeed = msg.feed
		return m, nextDocker(m.dockerFeed)
	case dockerMsg:
		// Docker's word, held for the next reading to merge. The rows
		// are drawn again at once rather than on the next beat, which is
		// the whole point of a feed: a container is on its row as it
		// starts, not up to two seconds later.
		m.containers, m.dockerStalled = msg.containers, msg.stalled
		m.processesGen++
		return m, tea.Batch(m.readProcesses(), nextDocker(m.dockerFeed))
	case helpMsg:
		// The manual is up, and no row is under the cursor while it is.
		// The manual is not a process; there is no row it belongs to,
		// and a cursor left sitting on one would say these keys were
		// about that row when they are about reading. Where the cursor
		// was is kept, so j and k carry on from it.
		//
		// The reading is taken again from here, as it is for the page,
		// so one already in flight that saw the workspace as it was
		// cannot land afterwards and say the manual is not up.
		m.helping, m.cursor = msg.on, 0
		m.processesGen++
		return m.published(false), m.readProcesses()
	case readoutMsg:
		// The page is up, or down, and conn knows it without reading the
		// server: the next i is a keypress away and has to decide which
		// way it goes. The reading is taken again from here so a reading
		// already in flight, which saw the bay as it was before, cannot
		// land afterwards and say otherwise.
		m.looking = msg.on
		m.processesGen++
		return m, m.readProcesses()
	case tea.FocusMsg:
		m.focused = true
		return m.keepingPage()
	case tea.BlurMsg:
		// The keys have gone to the workspace, which means into a
		// process: what is in there is that process, put there by
		// whatever sent the keys.
		m.focused = false
	case reachedMsg:
		// The keys went with the pane. conn did that itself and does not
		// wait to be told, so a terminal reporting no focus still leaves
		// the workspace holding the process rather than taking the page
		// back on the next reading.
		m.focused = false
		// The pane is in the bay; conn knows it now and does not have to
		// read the server to find out, so the row says so at once.
		//
		// A pane is the whole tree in it, so reaching one from a row
		// down inside it reaches the head. The cursor goes there too:
		// it was on the row that asked, but the row that answered is
		// the head, and leaving the two apart would put the mark on one
		// row while the cursor sat on another — the sub-process looking
		// picked out for being the one thing in the pane that is not
		// what is in the bay.
		m = m.slotted(msg.tty)
		m.looking = false
		if pid, at, ok := headOf(m.projects, msg.tty); ok {
			m.cursor, m.cursorAt = pid, at
		}
		m.processesGen++
		return m, m.readProcesses()
	case blinkMsg:
		if msg.gen != m.blinkGen || !m.annunciating() {
			m.lit = true
			return m, nil
		}
		m.lit = !m.lit
		return m, m.nextBlink()
	case processesMsg:
		if msg.gen != m.processesGen {
			return m, nil
		}
		// The row the list's cursor was on, before the reading replaces
		// the rows it is an index into.
		wasRow, hadRow := m.atCursor()
		// The reading was made on the roots it found the file naming;
		// the model goes onto them with it, so the rows and the roots
		// they are named by are never of two files.
		if msg.rooted != nil {
			m = m.rooted(*msg.rooted)
		}
		m.projects, m.panes, m.bay, m.processesErr = msg.projects, msg.panes, msg.bay, msg.err
		m.records, m.declared, m.tree = msg.records, msg.declared, msg.tree
		m.looking, m.helping = msg.bayReadout, msg.bayHelp
		// Where the keys are, by the server's own word. conn is told by
		// the terminal when they leave, and knows on its own when its
		// reaching sent them away, but a reading can land between the
		// reaching and the word of it: it then saw a bay with a process
		// in it and no page, under a panel it still believed had the
		// keys, and put the page back over the process. The reading
		// carries tmux's answer, so a bay that has the keys is a panel
		// that does not, whatever conn has yet been told.
		if msg.bayActive {
			m.focused = false
		}
		// Work in the workspace is what esc goes back into, so a conn
		// that came up to a bay it did not fill itself still knows where
		// the operator was. The page and a hold are conn's own furniture
		// and leave standing whatever the bay held before them.
		if reachable(msg.panes[msg.bay]) {
			m.lastIn = msg.bay
		}
		if msg.cpu != nil {
			m.cpuWas, m.cpuAt, m.stood, m.acts = msg.cpu, msg.cpuAt, msg.stood, msg.acts
		}
		// The shell conn opened is the cursor's once the reading has it;
		// one that never comes is given up on when the wait is out.
		if m.awaited != 0 {
			switch {
			case hasPid(m.projects, m.awaited):
				m.cursor, m.awaited = m.awaited, 0
			case time.Now().After(m.until):
				m.awaited = 0
			}
		}
		// follow's job is to keep hold of the row the operator was on
		// while the rows change under it. With the manual up there is no
		// such row, and following would hand one back every couple of
		// seconds: the cursor is cleared on purpose, and stays cleared
		// until the operator moves it themselves.
		if !m.helping {
			m.cursor, m.cursorAt = follow(m.projects, m.cursor, m.cursorAt)
		}
		// The reading the console was waiting on: the processes view goes up
		// with its rows already in it, drawn at the panel's width, and the
		// bay opens beside a frame that is already the shape it will be.
		var cmds []tea.Cmd
		if m.entering {
			m.entering, m.view = false, viewProcesses
			if m.inside {
				cmds = append(cmds, m.serverCmd(func() error { return m.srv.narrow() }))
			}
		}
		if m.view == viewProcesses {
			switch {
			// A home without its bay gets one; the next reading finds it.
			case m.inside && msg.noBay:
				cmds = append(cmds, m.processesTick(), m.openBay())
			// A manual left behind: it says so as it goes, and this is
			// the same answer for a manual that ended without saying —
			// killed from outside, or gone while the keys were in the
			// list and nobody was tending the workspace.
			case m.inside && msg.bayDead && msg.bayHelp:
				mm, cmd := m.leftHelp(true)
				m = mm.(model)
				cmds = append(cmds, m.processesTick(), cmd)
			// A bay whose pane died stays the shape it was; only what is in it
			// is replaced, so the panel never has to give up its width and take
			// it back. What replaces it is the next process conn holds, from the
			// cursor down and round again: the operator was working in the bay,
			// and the work goes on in the one nearest to hand. Only with none to
			// reach does the bay hold a placard.
			case m.inside && msg.bayDead:
				if e, ok := m.nextReachable(); ok {
					cmds = append(cmds, m.processesTick(), m.reach(m.panes[e.tty], e.tty))
				} else {
					cmds = append(cmds, m.processesTick(), m.reviveBay())
				}
			default:
				cmds = append(cmds, m.processesTick())
			}
			// And the page is what the workspace holds while the keys
			// are here, so a view just come on, or one that has just
			// got its first row, has it without anybody asking.
			mm, cmd := m.keepingPage()
			m = mm.(model)
			cmds = append(cmds, cmd)
		}
		// The list holds live processes too, so it is read for as long as
		// it is up: a row that says a process is there is a row enter is
		// about to go into, and one read minutes ago is a promise the
		// machine may not keep.
		if m.view == viewProjects {
			cmds = append(cmds, m.processesTick())
			if rows := m.projectRows(); hadRow {
				m.find.at = followRow(rows, wasRow, m.find.at)
			} else {
				m.find.at = clamp(m.find.at, len(rows))
			}
			// The page is what the workspace holds here too, about the
			// row the cursor is on.
			mm, cmd := m.keepingPage()
			m = mm.(model)
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	case processesTickMsg:
		if msg.gen != m.processesGen || (m.view != viewProcesses && m.view != viewProjects) {
			return m, nil
		}
		return m, m.readProcesses()
	case projectsMsg:
		wasRow, hadRow := m.atCursor()
		m.walked, m.projectsErr, m.scanning = msg.projects, msg.err, false
		if rows := m.projectRows(); hadRow {
			m.find.at = followRow(rows, wasRow, m.find.at)
		} else {
			m.find.at = clamp(m.find.at, len(rows))
		}
		// The walk has given the list its rows, and the page comes up
		// for the one the cursor is on without anybody asking.
		return m.keepingPage()
	case sessionsMsg:
		// Only the sessions view that asked for these dirs wants them; one
		// opened on another project since has moved past the answer.
		if !slices.Equal(msg.dirs, m.sessionsDirs) {
			return m, nil
		}
		m.sessions, m.sessionsLoading = msg.sessions, false
		m.rfind.at = clamp(m.rfind.at, len(m.sessionsRows()))
		// The sessions have landed, and the page comes up for the one
		// the cursor is on without anybody asking.
		return m.keepingPage()
	case killedMsg:
		// A beat for the signal to be acted on, so the row is not read a
		// moment too soon, still there; the processesTick this reuses is a
		// no-op once the stay it belongs to has moved on. Whether the
		// process ended is the processes view's to say.
		gen := m.processesGen
		return m, tea.Tick(killGrace, func(time.Time) tea.Msg { return processesTickMsg{gen: gen} })
	case tea.KeyPressMsg:
		return m.key(msg.String())
	}
	return m, nil
}

// key answers a key: q and ctrl+c detach in the server and close conn
// outside it, from anywhere; on the console a key skips the sequence,
// then continues to the processes view and gives the bay its side back;
// in the processes view c brings the console back over the whole
// window, enter reaches the cursor's process, esc goes back into the
// last process the workspace held, s opens a shell at its project, a
// opens claude there instead, and alt+A opens the sessions view over
// what claude left suspended there. tab goes to what is waiting on you,
// longest first, and round again. x asks to end the cursor's process,
// and arms the question rather than the ending: the next key answers
// it. gg and G are the ends of the list, where j and k are its steps: a
// table long enough to scroll is not walked to its end.
func (m model) key(k string) (tea.Model, tea.Cmd) {
	// A notice stands until the next key, whatever it is: it was read,
	// or it was not going to be.
	m.notice = ""
	// A kill x asked for takes the next key, whatever it is: y confirms
	// it, and anything else cancels, as tmux's own confirmation goes —
	// no other binding fires while the question is on the status line.
	if m.kill != nil {
		req := m.kill
		m.kill = nil
		if k == "y" {
			if req.container != "" {
				return m, m.stopContainer(req.container, req.command)
			}
			if req.pane != "" {
				return m, m.closeHeld(req.pane, req.command)
			}
			return m, m.killEntry(req.pid, req.command, req.sig)
		}
		return m, nil
	}
	// The second g of gg, which is the only key the first one waits for.
	// Every other key clears it and goes on to do what it does, so a g
	// pressed and thought better of costs nothing.
	half := m.firstG
	m.firstG = false
	if half && k == "g" {
		m.cursor, m.cursorAt = follow(m.projects, 0, 0)
		return m, nil
	}
	// The key that opens projects from wherever conn is, which is what
	// the prefix chord sends. p cannot serve: it opens projects from the
	// processes view, where it is a key, but in projects and in sessions
	// it is a letter being typed into the line, and on the console it is
	// one of the any-keys that continue to the processes view. So the
	// chord has a key of its own, and it is the same key wherever it is
	// pressed.
	if k == "alt+p" {
		m.from = m.cameFrom()
		return m.toProjects()
	}
	// The other process, which the prefix twice over sends. It is a key
	// of its own for the same reason: in projects and in sessions every
	// letter is one being typed into the line.
	if k == "alt+o" {
		return m.toOther()
	}
	// The process that has waited longest, which the prefix then tab sends
	// from anywhere: in the processes view tab itself is the key, but on
	// the list and the sessions view tab is nothing and on the console it
	// is one of the any-keys, so the chord has a key of its own.
	if k == "alt+tab" {
		return m.toWaiting()
	}
	// A shell, a contact, and the sessions suspended at the project the
	// panel is looking at, each from anywhere in the server: what the
	// prefix then s, then a, and then A send. Each is a key of its own
	// for the reason the three above are — in the list and in the
	// sessions view a plain s or a is a letter being typed into the
	// line. The letter is the same on every road to the thing: a is a
	// contact and A the sessions in the processes view, alt+a and
	// alt+A on a line typed into, prefix a and prefix A from anywhere.
	//
	// In a line typed into, ctrl is readline's and alt is conn's. The
	// line is edited the way readline edits one, and a ctrl key there
	// means what it means to readline — ctrl+a the start of the line,
	// not a contact, which it was; ctrl+s a search, not a shell, which
	// it was — so conn's own verbs on a line are all on alt, and the
	// alt key is the one the chord sends, so a hand learns one key for
	// one thing wherever it is pressed.
	if k == "alt+s" || k == "alt+a" || k == "alt+shift+a" {
		return m.openAt(k)
	}
	// Everything the project the panel is looking at declares and does
	// not have running, brought up: u in the processes view, and alt+u
	// from the list, where u is a letter being typed.
	if k == "alt+u" || k == "u" && m.view == viewProcesses {
		return m.raiseAt()
	}
	// The manual saying it is done with. It sends this as it goes, so
	// the workspace is filled in the same breath rather than holding a
	// dead pane until the next reading comes round — and so that it is
	// filled at all, the reading only tending the workspace while the
	// processes view has the keys.
	if k == "alt+esc" {
		return m.leftHelp(false)
	}
	// Back to the processes view, which the prefix then - sends. tmux
	// has already put the keys on the panel by the time this arrives;
	// what is left is the manual, if one is up, which this takes down.
	// It is how the operator gets from reading to working without first
	// deciding what they were working on.
	if k == "alt+-" {
		if m.helping {
			// This chord says where to go, so where the manual was asked
			// from stops mattering: it is the one way out of the manual
			// that does not put the keys back, and forgetting is what
			// makes it that.
			m.helping, m.helpFrom = false, ""
			m = m.tookBackRow()
			return m, tea.Batch(m.reviveBay(), m.processesTick())
		}
		return m, nil
	}
	// The manual, which the prefix then ? sends. It goes to the
	// processes view first: the manual stands in the workspace, and the
	// workspace is what that view is a panel beside.
	if k == "alt+?" {
		if m.srv == nil {
			return m, nil // nowhere to put it
		}
		// Up already: the same chord puts it away. The manual is not
		// reachable — it is conn's furniture, and the keys step over
		// furniture — so the chord that opened it is the only thing
		// that can close it, and a manual that could be opened and not
		// closed would be a trap rather than a help.
		if m.helping {
			m.helping = false
			return m, tea.Batch(m.reviveBay(), m.processesTick())
		}
		// Where the keys were when the chord fired, before tmux brought
		// them here. Reading the manual is a detour and not a move, so
		// leaving it puts them back where they were: in the workspace if
		// that is where they were, and on the panel if the operator was
		// working the view.
		m.helpFrom = m.cameFrom()
		mm, cmd := m.toProcesses()
		m = mm.(model)
		// Said here rather than when the manual is up. Opening it is
		// several turns of talking to tmux, and the page would be put in
		// the workspace by a reading landing in the middle of that —
		// the page goes up wherever a row is under the cursor, and it is
		// this that takes the row out from under it.
		// The row is kept rather than dropped. No row is under the
		// cursor while the manual is up, but the operator has not
		// unchosen it: they asked a question about the station and are
		// coming back to whatever they were looking at.
		m.helping, m.helpCursor, m.cursor = true, m.cursor, 0
		return m, tea.Batch(cmd, m.openHelp())
	}
	// Down and up the processes conn can put in the bay, which the
	// prefix then j and then k send. In the processes view j and k walk
	// every row, this walks only what there is somewhere to be sent to;
	// in the list and in the sessions view they are letters being typed.
	if k == "alt+j" || k == "alt+k" {
		return m.toReachable(k == "alt+j")
	}
	switch m.view {
	case viewProjects:
		return m.projectKey(k)
	case viewSessions:
		return m.sessionsKey(k)
	case viewRoots:
		return m.rootsKey(k)
	}
	switch {
	case k == "ctrl+c" || k == "q":
		if m.inside {
			// A detach leaves the server and this conn standing, so the
			// feed keeps its stream: there is something still watching.
			return m, m.serverCmd(func() error { return m.srv.detach() })
		}
		// Going for good takes the feed's stream with it. docker events
		// is a child conn started, and a child outlives the parent that
		// abandons it — it would sit reparented to init until the next
		// container event pushed a write down a pipe nobody holds.
		m.dockerFeed.close()
		return m, tea.Quit
	case m.view == viewConsole && m.stage < lastStage(m.report()):
		m.stage = lastStage(m.report())
		return m, nil
	case m.view == viewConsole:
		// The console holds until the processes view has something to show.
		// Going at once put an empty view up, filled it a tenth of a second
		// later when the table had been read, and moved it to the panel's
		// width after that — three screens to arrive at one. The console is a
		// still page and a moment more of it is not seen, where a view
		// assembling itself is.
		//
		// Only the first time. Coming back from the console the rows of the
		// last stay are still held, a couple of seconds old, and the
		// processes view goes up with them at once while the reading on its
		// way brings them up to date.
		// Told nowhere to look, conn cannot show the processes view at
		// all: it is every project work is happening in, and there are
		// no projects. So it asks, here, where going on from the console
		// would otherwise arrive at an empty list that means three
		// different things.
		if len(m.roots.real) == 0 {
			return m.toRoots()
		}
		m.processesGen++
		if len(m.projects) == 0 && m.processesErr == "" {
			m.entering = true
			return m, m.readProcesses()
		}
		m.view = viewProcesses
		if m.inside {
			return m, tea.Batch(m.readProcesses(), m.serverCmd(func() error { return m.srv.narrow() }))
		}
		return m, m.readProcesses()
	case k == "c":
		// The blink is not started here: what annunciates is decided in
		// one place, and the tick follows the view on its own.
		m.view = viewConsole
		if m.inside {
			return m, m.serverCmd(func() error { return m.srv.wide() })
		}
	case k == "j" || k == "down":
		m.cursor, m.cursorAt = follow(m.projects, 0, m.cursorAt+1)
	case k == "k" || k == "up":
		m.cursor, m.cursorAt = follow(m.projects, 0, max(m.cursorAt-1, 0))
	case k == "g":
		// Nothing yet: g is the half of a motion, and what it means is
		// decided by the key after it.
		m.firstG = true
	case k == "G":
		m.cursor, m.cursorAt = follow(m.projects, 0, rowsIn(m.projects)-1)
	case k == "z":
		// The whole tree, or the fold of it again. The rows are re-made
		// from the reading held, so the change is at once; the cursor
		// keeps its pid where the pid is still shown, and its row
		// otherwise.
		m.full = !m.full
		if len(m.tree) > 0 {
			m.projects = m.tree
			if !m.full {
				m.projects = fold(m.tree)
			}
			m.cursor, m.cursorAt = follow(m.projects, m.cursor, m.cursorAt)
		}
	case k == "enter":
		e, _, ok := m.under()
		if !m.inside || !ok {
			return m, nil
		}
		// A row conn already holds a pane for is gone into; that is enter
		// everywhere. A container has no pane until one is opened for it,
		// and what there is to be in front of is what it has written, so
		// the first enter opens its output and the next goes back into
		// the pane holding it.
		switch {
		case reachable(m.panes[e.tty]):
			return m, m.reach(m.panes[e.tty], e.tty)
		case e.container != "":
			return m, m.watchContainer(e)
		case e.declared != "":
			// A declared process that is down: brought up, and gone into.
			if path, d, ok := m.declarationOf(e); ok {
				return m, m.raise(path, d, "")
			}
		}
	case k == "esc":
		return m.backIn()
	case k == "x":
		e, _, ok := m.under()
		if !ok {
			return m, nil
		}
		// A container is stopped rather than signalled: there is no
		// process here to send anything to, and docker's stop asks it to
		// go before insisting. One already stopped is left alone — the
		// question would be about nothing, and the row is kept only so
		// the service that died beside its siblings can be seen.
		if e.container != "" {
			if e.status == statusEnded || e.fault {
				return m, nil
			}
			// By its service, which is what it is called here. The row's
			// own label carries the ports it publishes, and a question
			// that reads STOP CACHE · :6390 is asking about an address.
			name := e.command
			if c := m.containerAt(e.pid); c != nil {
				name = c.service
			}
			m.kill = &pendingKill{container: e.container, command: name, prompt: stopPrompt(e.container, name)}
			return m, nil
		}
		if e.declared != "" {
			return m.armDeclared(e)
		}
		// A shell whose rows are folded says what it runs, and x on it
		// is x on that: the command is asked to end and the shell is
		// left at its prompt, as it is when the command has a row of
		// its own. Killing the shell for being a shell would take the
		// command with it, from a row that named the command.
		if e.kind == kindShell && e.under != "" {
			if run, ok := m.runsOf(e); ok {
				name := program(run.asTyped())
				m.kill = &pendingKill{pid: run.pid, command: name, sig: syscall.SIGTERM, prompt: killPrompt(name, run.pid, syscall.SIGTERM)}
				return m, nil
			}
		}
		sig := killSignal(e.kind)
		// The question names the program: a contact's whole command
		// line is the note conn handed it, and a question that long is
		// not read.
		name := program(e.asTyped())
		m.kill = &pendingKill{pid: e.pid, command: name, sig: sig, prompt: killPrompt(name, e.pid, sig)}
	case k == "s":
		// A shell here. On a container, here is inside it: the row stands
		// for a machine of its own, and the directory it was started for
		// is not where its work is going on.
		if e, pl, ok := m.under(); m.inside && ok {
			if e.container != "" {
				return m, m.shellInContainer(e)
			}
			if pl.path != "" {
				return m, m.openShell(pl.path)
			}
		}
	case k == "a":
		if _, pl, ok := m.under(); m.inside && ok && pl.path != "" {
			return m, m.startContact(pl.path)
		}
	case k == "A":
		// The sessions at the project: the capital of the contact's
		// key, a session being a contact's to pick back up.
		return m.openAt("alt+shift+a")
	case k == "tab":
		return m.toWaiting()
	case k == "p":
		// Pressed here, so there is nowhere to go back to.
		m.from = ""
		return m.toProjects()
	}
	return m, nil
}

// projectKey answers a key in projects, which is a line typed into;
// see typed for the keys every such line has. What is the list's own:
// enter opens a shell at the row under the cursor and goes back to the
// processes view, which is where the shell will show, or goes into the
// row where it is a process; alt+a opens claude there instead, since
// a plain a is a letter to type; alt+A opens the sessions view over
// what claude left suspended at the row, group included, the same way
// — not a plain A, which would take a letter the line can still be
// typed with, and not ctrl+shift+a, which is not its own chord to any
// terminal at all, alphabetic ctrl combinations being their letter's
// own case already; esc goes back without opening anything, and ctrl+c
// is what it is everywhere.
func (m model) projectKey(k string) (tea.Model, tea.Cmd) {
	rows := m.projectRows()
	switch {
	case m.find.edit(k, len(rows)):
	case k == "ctrl+c":
		if m.inside {
			return m, m.serverCmd(func() error { return m.srv.detach() })
		}
		return m, tea.Quit
	case k == "esc":
		return m.backFrom()
	case k == "enter":
		if !m.inside || m.find.at >= len(rows) {
			return m, nil
		}
		row := rows[m.find.at]
		// A process row is somewhere to go, not something to start: enter
		// puts its pane in the bay and the keys in it, the way enter does
		// on the row in the processes view. That is the whole of what this
		// mode is for on a machine with more processes than rows.
		if row.pid != 0 {
			if reachable(m.panes[row.tty]) {
				mm, cmd := m.toProcesses()
				m = mm.(model)
				return m, tea.Batch(cmd, m.reach(m.panes[row.tty], row.tty))
			}
			return m, nil
		}
		if row.path == "" {
			return m, nil // work off every project: a heading, not a place
		}
		mm, cmd := m.toProcesses()
		m = mm.(model)
		return m, tea.Batch(cmd, m.openShell(row.path))
	}
	return m, nil
}

// slotted takes a terminal into the bay and remembers the one it is
// replacing, so there is an other to go back to.
//
// Only a process is remembered. A hold standing in an empty bay and
// the readout are conn's own furniture rather than somewhere you were
// working, and going back to one would be going back to nothing.
func (m model) slotted(tty string) model {
	// The other is the work before this work, and it is read off lastIn
	// rather than off the bay. The bay is not where the last thing you
	// were in has been since you left it: the page takes the workspace
	// the moment the keys reach the panel, so by the time anything is
	// opened or reached the bay is the page, and a rule that refused to
	// remember furniture — rightly — never remembered anything at all.
	// The chord did nothing for the whole of the page's life.
	//
	// lastIn is only ever work, being set here and, on a reading, only
	// for a bay that can be reached. So one holds what you are in and
	// the other what you were in before it, and the page cannot get
	// between them.
	if m.lastIn != "" && m.lastIn != tty {
		m.lastBay = m.lastIn
	}
	m.bay, m.lastIn = tty, tty
	return m
}

// toOther goes to the process that was in the bay before the one in it
// now, and takes the one in it now as the one to come back to — so
// pressed twice it is where it started, and pressed while working is
// the other thing you are working on. It is what the prefix twice over
// sends, which is the shape that key has everywhere: the one you were
// last in.
//
// The console is left on the way, in the rare case the chord is pressed
// with it up: you cannot be in a pane while the console is over the
// window, so this is a press from the panel, and the answer to it is a
// process.
func (m model) toOther() (tea.Model, tea.Cmd) {
	// Asked as reachable and not merely as held, the way every other
	// road into a pane asks it: a pane whose process has ended is an id
	// conn still has and nowhere to be sent.
	if !m.inside || m.lastBay == "" || !reachable(m.panes[m.lastBay]) {
		return m, nil
	}
	cmds := []tea.Cmd{m.reach(m.panes[m.lastBay], m.lastBay)}
	if m.view == viewConsole {
		m.view, m.entering = viewProcesses, false
		cmds = append(cmds, m.serverCmd(func() error { return m.srv.narrow() }))
	}
	return m, tea.Batch(cmds...)
}

// toWaiting goes to the process that has waited longest: the cursor to
// its row, its pane in the bay, and the keys in it, so one press has
// the operator answering. Pressed again from the panel it goes round
// the ring, longest first. It is what tab does in the processes view
// and what the prefix then tab sends from anywhere. From another view
// the processes view is put up on the way, since the answer is a pane,
// and from the console the bay is given its side back, a pane being
// unreachable with the console over the window.
//
// A process conn holds no pane for is still gone to, on the panel, and
// the keys stay where they are.
func (m model) toWaiting() (tea.Model, tea.Cmd) {
	round := waitingRound(m.projects)
	if len(round) == 0 {
		return m, nil
	}
	next := round[0]
	for i, e := range round {
		if e.pid == m.cursor {
			next = round[(i+1)%len(round)]
			break
		}
	}
	return m.goTo(next)
}

// goTo puts the cursor on a row and the operator in front of it: the
// panel comes back to the processes view if it is somewhere else, and
// the row's pane goes into the bay with the keys, where conn holds
// one. A row conn only reports is gone to on the panel, and the keys
// stay where they are, there being nothing to put them in.
func (m model) goTo(next entry) (tea.Model, tea.Cmd) {
	m.cursor, m.cursorAt = follow(m.projects, next.pid, m.cursorAt)
	var cmds []tea.Cmd
	if m.view != viewProcesses {
		console := m.view == viewConsole
		m.view, m.entering = viewProcesses, false
		m.processesGen++
		cmds = append(cmds, m.readProcesses())
		if console && m.inside {
			cmds = append(cmds, m.serverCmd(func() error { return m.srv.narrow() }))
		}
	}
	if m.inside && reachable(m.panes[next.tty]) {
		cmds = append(cmds, m.reach(m.panes[next.tty], next.tty))
	}
	return m, tea.Batch(cmds...)
}

// reachableRound is every pane conn holds that has work in it, said as
// the one row that stands for it: the head of the tree, since a pane
// holds a whole tree and there is one thing in it to be put in the
// bay. It is the ring alt+j walks down and alt+k up, in the order the
// view stands the rows in, with where each sits among all the rows, so
// that a cursor anywhere in the list knows which way is next. A hold,
// the readout and a pane whose process has ended are not work.
func (m model) reachableRound() (round []entry, at []int) {
	seen := map[string]bool{}
	i := 0
	for _, pl := range m.projects {
		for _, e := range pl.entries {
			p := m.panes[e.tty]
			if reachable(p) && !seen[e.tty] {
				seen[e.tty] = true
				round, at = append(round, e), append(at, i)
			}
			i++
		}
	}
	return round, at
}

// toReachable goes to the next process conn can actually put in front
// of you, down the view with alt+j and up it with alt+k, round again
// from either end. Rows conn only reports are stepped over rather than
// landed on: the keys are going with the cursor, and a row there is no
// pane for is nowhere to send them. With nothing to reach it does
// nothing, which is every row outside conn's own server.
func (m model) toReachable(down bool) (tea.Model, tea.Cmd) {
	round, at := m.reachableRound()
	if len(round) == 0 {
		return m, nil
	}
	// Where the cursor stands among all the rows, which is not where it
	// stands in the ring: it can be on a row conn cannot reach, or on a
	// row inside a tree whose head is the ring's member.
	here, i := -1, 0
	for _, pl := range m.projects {
		for _, e := range pl.entries {
			if e.pid == m.cursor {
				here = i
			}
			i++
		}
	}
	next := round[0]
	if down {
		for j, k := range at {
			if k > here {
				next = round[j]
				break
			}
		}
	} else {
		next = round[len(round)-1]
		for j := len(at) - 1; j >= 0; j-- {
			if at[j] < here {
				next = round[j]
				break
			}
		}
	}
	return m.goTo(next)
}

// cameFrom is the pane the keys were in when a chord brought them to
// the panel, and nothing where they were already here. A chord writes
// it as it fires, because by the time conn reads the key the panel is
// the pane with the keys and nothing on this side can tell where they
// came from. It is read once and cleared, so a chord that was answered
// rather than cancelled leaves nothing behind for the next one.
func (m model) cameFrom() string {
	if !m.inside || m.srv == nil {
		return ""
	}
	out, err := m.srv.run("show-options", "-gqv", "@conn_from")
	if err != nil {
		return ""
	}
	_, _ = m.srv.run("set-option", "-gu", "@conn_from")
	if from := strings.TrimSpace(out); from != m.srv.panel() {
		return from
	}
	return ""
}

// backFrom is the cancel. The panel comes back to the processes view,
// and where a chord brought the keys here out of another pane they go
// back to it: cancelling is putting things as they were, and the pane
// the operator was working in is part of how they were.
//
// Going back to it is reaching it, not selecting it. The pane is not
// where it was: the page takes the workspace while the keys are on the
// panel, and what the page displaced went back to a window of its own.
// Selecting it by id there does select it — in a window the client is
// not looking at, which is nothing happening at all. Reaching it puts
// it back in the workspace first, which is where the operator left it.
//
// A chord pressed on the panel leaves nothing to go back to, and the
// cancel is the view alone: you were not in a pane, so there is no pane
// to be put back in. Work that ended while the list was up is the same
// answer for the same reason.
func (m model) backFrom() (tea.Model, tea.Cmd) {
	from := m.from
	m.from = ""
	mm, cmd := m.toProcesses()
	m = mm.(model)
	if from == "" || !m.inside {
		return m, cmd
	}
	p, tty, ok := m.paneByID(from)
	if !ok || !reachable(p) {
		return m, cmd
	}
	return m, tea.Batch(cmd, m.reach(p, tty))
}

// paneByID is the pane conn holds under that id, and the terminal it is
// on. conn holds its panes by terminal, a terminal being what a row
// is; a chord names the pane it fired from by id, which is what tmux
// knows of it.
func (m model) paneByID(id string) (pane, string, bool) {
	for tty, p := range m.panes {
		if p.id == id {
			return p, tty, true
		}
	}
	return pane{}, "", false
}

// backIn puts the keys back in the process they came out of, which is
// the last one the workspace held. Walking the rows is reading, not
// moving: the cursor goes down the list while the work stands where it
// was, and esc is how the reading ends. Without it the way back is to
// find the row the work is on and press enter, which is the operator
// doing by hand what conn already knows.
//
// It is the cursor that is ignored here, deliberately. enter goes to
// the row you are looking at; esc goes to the process you were in. A
// glance down the list and back costs nothing, and lands where it
// started however far the cursor wandered.
//
// With nothing to go back into — a bay that has only ever held a hold,
// work that has since ended, every row outside conn's own server — it
// does nothing, and the cursor stays where the operator left it.
func (m model) backIn() (tea.Model, tea.Cmd) {
	if !m.inside || m.lastIn == "" {
		return m, nil
	}
	p, ok := m.panes[m.lastIn]
	if !ok || !reachable(p) {
		return m, nil
	}
	return m, m.reach(p, m.lastIn)
}

// toProjects opens the list, from wherever conn is, and walks the roots
// again for it: the list is what could be worked on rather than what is
// being worked on, so it is read when it is asked for and not on a beat.
//
// From the console it gives the bay its side back, the way going to the
// processes view does — the list is a panel view like the processes
// view — and it calls off the console's wait on a reading, or that
// reading would land a moment later and put the processes view up over
// it.
func (m model) toProjects() (tea.Model, tea.Cmd) {
	console := m.view == viewConsole
	m.view, m.scanning = viewProjects, true
	m.find.clear()
	m.entering = false
	// The walk, and the reading: the list holds the processes running in
	// each project as well as the projects, and the reading goes on for
	// as long as it is up.
	m.processesGen++
	cmds := []tea.Cmd{m.scanProjects(), m.readProcesses()}
	if console && m.inside {
		cmds = append(cmds, m.serverCmd(func() error { return m.srv.narrow() }))
	}
	return m, tea.Batch(cmds...)
}

// toProcesses leaves the list for the processes view, which starts
// reading again.
func (m model) toProcesses() (tea.Model, tea.Cmd) {
	// Coming to the view fresh, the page is what the workspace holds
	// again: a close is for the stay it was made in.
	m.view = viewProcesses
	m.processesGen++
	return m, m.readProcesses()
}

// keepingPage puts the page in the workspace, the page being what the
// workspace holds: the keys are on the panel, the panel is in the
// processes view, and there is a row to be about. The operator is
// reading the list and the page is the reading; nothing is pressed for
// it.
//
// It is asked on the keys arriving and on every reading, so a view
// just come on, or one that has just got its first row, gets the page
// without the operator doing anything. looking is set here rather than
// waited for, so the reading a moment later does not ask for a second
// page on top of the first.
func (m model) keepingPage() (tea.Model, tea.Cmd) {
	// The manual is in the workspace on purpose, and the page would put
	// itself there over the top of it. No row is under the cursor while
	// the manual is up, which would stop this on its own; saying it
	// plainly as well means the page cannot come back the moment the
	// cursor does.
	if !m.inside || m.looking || m.helping || !m.focused {
		return m, nil
	}
	if m.view != viewProcesses && m.view != viewProjects && m.view != viewSessions {
		return m, nil
	}
	if m.subject().none() {
		return m, nil
	}
	m.looking = true
	return m, m.openReadout()
}

// atProject is the project the panel has under its eye and the
// directories a session of its own could be filed under: the cursor's
// project in the processes view, the row's in the list, where a group
// answers for the repositories under it, and in the sessions view the
// project those sessions are already about. The console is looking at
// the machine and not at a project, and answers nothing.
func (m model) atProject() (string, []string, bool) {
	switch m.view {
	case viewProcesses:
		if _, pl, ok := m.under(); ok && pl.path != "" {
			return pl.path, []string{pl.path}, true
		}
	case viewProjects:
		if row, ok := m.atCursor(); ok && row.path != "" {
			return row.path, sessionDirs(m.walked, row), true
		}
	case viewSessions:
		if m.sessionsProject != "" {
			return m.sessionsProject, m.sessionsDirs, true
		}
	}
	return "", nil, false
}

// openAt opens a shell, a contact or the sessions view at whatever
// project the panel is looking at. A shell and a contact show in the
// processes view, so the panel comes back to it for them, the way the
// list has always come back for what it opened; the sessions view is
// somewhere to be and is gone to.
func (m model) openAt(k string) (tea.Model, tea.Cmd) {
	path, dirs, ok := m.atProject()
	if !m.inside || !ok {
		return m, nil
	}
	if k == "alt+shift+a" {
		m.from = m.cameFrom()
		return m.openSessions(path, dirs)
	}
	var cmds []tea.Cmd
	if m.view != viewProcesses {
		mm, cmd := m.toProcesses()
		m, cmds = mm.(model), append(cmds, cmd)
	}
	if k == "alt+a" {
		return m, tea.Batch(append(cmds, m.startContact(path))...)
	}
	return m, tea.Batch(append(cmds, m.openShell(path))...)
}

// raiseAt brings up what the project the panel is looking at declares
// and does not have running. From another view the processes view is
// put up on the way, since that is where the rows will show.
func (m model) raiseAt() (tea.Model, tea.Cmd) {
	// The file is read by the raise itself, off the loop, so a project
	// with nothing running — whose file the reading has not read — is
	// brought up from the list all the same.
	path, _, ok := m.atProject()
	if !m.inside || !ok || path == "" {
		return m, nil
	}
	up, held := upAndHeld(m.projects, m.panes, path)
	var cmds []tea.Cmd
	if m.view != viewProcesses {
		mm, cmd := m.toProcesses()
		m, cmds = mm.(model), append(cmds, cmd)
	}
	return m, tea.Batch(append(cmds, m.raiseAll(path, up, held))...)
}

// armDeclared is x on a declared process's row. Down, there is nothing
// to end. Ended and holding its pane, the pane is what goes, and the
// question says close. Up, the process itself is asked to end — the
// command under the sh that started it, so that the sh goes on to
// record the end and hold the output the way an end of its own would;
// the sh itself only until the command is read.
func (m model) armDeclared(e entry) (tea.Model, tea.Cmd) {
	_, name, _ := unmarkDeclared(e.declared)
	switch {
	case e.tty == "":
		return m, nil
	case m.panes[e.tty].exit != "":
		m.kill = &pendingKill{pane: m.panes[e.tty].id, command: name, prompt: closePrompt(m.panes[e.tty].id, name)}
		return m, nil
	}
	pid := e.pid
	if child, ok := m.childOf(e); ok {
		pid = child.pid
	}
	m.kill = &pendingKill{pid: pid, command: name, sig: syscall.SIGTERM, prompt: killPrompt(name, pid, syscall.SIGTERM)}
	return m, nil
}

// runsOf is what a shell runs, in the tree whole: the first row under
// it that is not a shell itself, looking through a bash -c to the
// command it was given, the way the fold does for the shell's own row;
// with nothing but shells under it, the first of those.
func (m model) runsOf(head entry) (entry, bool) {
	projects := m.tree
	if len(projects) == 0 {
		projects = m.projects
	}
	for _, pl := range projects {
		for i, e := range pl.entries {
			if e.pid != head.pid {
				continue
			}
			var first entry
			for _, under := range pl.entries[i+1:] {
				if under.depth <= e.depth {
					break
				}
				if under.kind != kindShell {
					return under, true
				}
				if first.pid == 0 {
					first = under
				}
			}
			return first, first.pid != 0
		}
	}
	return entry{}, false
}

// childOf is the first row under a pane's head: what the head runs,
// in the tree whole, since the fold may have taken it off the panel.
func (m model) childOf(head entry) (entry, bool) {
	projects := m.tree
	if len(projects) == 0 {
		projects = m.projects
	}
	for _, pl := range projects {
		for i, e := range pl.entries {
			if e.pid != head.pid {
				continue
			}
			if i+1 < len(pl.entries) && pl.entries[i+1].tty == head.tty && pl.entries[i+1].depth == head.depth+1 {
				return pl.entries[i+1], true
			}
			return entry{}, false
		}
	}
	return entry{}, false
}

// openSessions opens the sessions view over a project's suspended
// sessions: project is what it is for, and dirs the directories a
// transcript could be filed under, which for a group is a repository
// under it, not the folder that names it.
func (m model) openSessions(project string, dirs []string) (tea.Model, tea.Cmd) {
	m.view = viewSessions
	m.sessionsDirs, m.sessionsProject, m.sessionsLoading = dirs, project, true
	m.sessions = nil
	m.rfind.clear()
	return m, m.scanSessions(dirs)
}

// sessionsRows is the sessions the filter leaves, which the cursor
// is an index into.
func (m model) sessionsRows() []session {
	return matchingSessions(m.sessions, m.rfind.text)
}

// sessionsReport is the sessions view's words as things stand.
func (m model) sessionsReport() sessionsReport {
	b := composeSessionsAt(m.sessions, m.sessionsProject, m.rfind.text, m.head.login.home, m.now, m.sessionsLoading)
	b.caret = m.rfind.cur
	return b
}

// sessionsKey answers a key on the sessions view, which is a line typed
// into the same way the list is; see typed. What is the view's own:
// enter continues the session under the cursor and goes back to the
// processes view, esc goes back without continuing anything, and ctrl+c
// is what it is everywhere.
func (m model) sessionsKey(k string) (tea.Model, tea.Cmd) {
	rows := m.sessionsRows()
	switch {
	case m.rfind.edit(k, len(rows)):
	case k == "ctrl+c":
		if m.inside {
			return m, m.serverCmd(func() error { return m.srv.detach() })
		}
		return m, tea.Quit
	case k == "esc":
		return m.backFrom()
	case k == "enter":
		if m.inside && !m.sessionsLoading && m.rfind.at < len(rows) {
			c := rows[m.rfind.at]
			mm, cmd := m.toProcesses()
			m = mm.(model)
			return m, tea.Batch(cmd, m.openResumed(c.Dir, c.ID))
		}
	}
	return m, nil
}

// clamp holds an index within the rows there are; with no rows it is
// the first, which is no row.
func clamp(at, rows int) int {
	return min(max(at, 0), max(rows-1, 0))
}

// under is the entry and the project under the cursor. nextReachable is
// the first process at or after the cursor, round again from the top,
// that conn holds a live pane for: what the bay takes when what was in
// it ends. A hold, the readout and a pane that has died are not
// processes.
func (m model) nextReachable() (entry, bool) {
	var all []entry
	for _, pl := range m.projects {
		all = append(all, pl.entries...)
	}
	start := 0
	for i, e := range all {
		if e.pid == m.cursor {
			start = i
			break
		}
	}
	for k := range all {
		e := all[(start+k)%len(all)]
		if p := m.panes[e.tty]; reachable(p) {
			return e, true
		}
	}
	return entry{}, false
}

func (m model) under() (entry, project, bool) {
	for _, pl := range m.projects {
		for _, e := range pl.entries {
			if e.pid == m.cursor {
				return e, pl, true
			}
		}
	}
	return entry{}, project{}, false
}

// follow finds the cursor after the rows change: the row of its pid,
// where that is still listed, else the row where it was, held within
// the rows there are. It answers the pid and the row.
func follow(projects []project, pid, at int) (int, int) {
	var pids []int
	for _, pl := range projects {
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

// rowsIn is how many process rows the projects hold: what a motion to
// the end or the middle of them counts against. It counts the processes
// and not the titles above them, which is the list j and k walk.
func rowsIn(projects []project) int {
	n := 0
	for _, pl := range projects {
		n += len(pl.entries)
	}
	return n
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
// on: rows of a later stage are the ground until their turn. cols is
// the width conn draws in, which is the panel's own where conn is a
// panel. Inside the server, off the console, the panel is panelWidth:
// conn holds tmux to that (see the resize in WindowSizeMsg) rather than
// taking whatever width it is given, so it draws to it as well instead
// of waiting to be told the pane has become one. That is what makes
// going to the processes view one change of the screen — the frame conn
// paints is already the shape the pane is about to be, so the split has
// nothing to reflow and no frame is ever drawn to a width that is on
// its way out.
//
// Never wider than the terminal: a window narrower than the panel is
// still the whole of what there is to draw in.
func (m model) cols() int {
	if m.inside && m.view != viewConsole {
		return min(panelWidth, m.width)
	}
	return m.width
}

func (m model) View() tea.View {
	var rows []row
	width := m.cols()
	switch m.view {
	case viewProcesses:
		rows = drawProcesses(m.processesReport(), m.cursor, width, m.height, m.p)
	case viewProjects:
		rows = drawProjects(m.projectsReport(), m.find.at, width, m.height, m.p)
	case viewSessions:
		rows = drawSessions(m.sessionsReport(), m.rfind.at, width, m.height, m.p)
	case viewRoots:
		b := composeRootsAt(m.asking.text, m.head.login.home)
		b.caret = m.asking.cur
		rows = drawRoots(b, m.asking.at, width, m.height, m.p)
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
	// conn is told when the keys arrive in its pane and when they
	// leave, which is how the workspace beside it knows to hold the
	// page: the keys coming back to the panel is the operator asking
	// what a row is, and nothing else announces that.
	v.ReportFocus = true
	v.BackgroundColor = groundColor
	v.ForegroundColor = inkColor
	v.WindowTitle = "conn"
	return v
}

// toRoots is the asking view, which conn goes to instead of the
// processes view when it has no roots. It comes up on the home, which
// is where checkouts usually are and is a directory that certainly
// exists, so the first thing shown is a list rather than nothing.
func (m model) toRoots() (tea.Model, tea.Cmd) {
	m.view, m.asking, m.rootErr = viewRoots, typed{text: "~/"}, ""
	return m, nil
}

// rootsKey is the asking view's keys. The line is typed into like the
// list's; see typed. What is the view's own: tab fills the line in
// with the directory under the cursor, and enter takes it — the config
// is written and conn is working from it before the view is gone. A
// line that changes takes what went wrong saving off the view with it,
// since the error was about what was typed and that is not what is
// typed now.
func (m model) rootsKey(k string) (tea.Model, tea.Cmd) {
	b := composeRoots(m.asking.text, m.head.login.home)
	switch {
	case m.asking.edit(k, len(b.rows)):
		if m.asking.text != b.typed {
			m.rootErr = ""
		}
	case k == "ctrl+c":
		if m.inside {
			return m, m.serverCmd(func() error { return m.srv.detach() })
		}
		return m, tea.Quit
	case k == "tab":
		// Filling the line in is not answering: what is typed becomes
		// the directory under the cursor, with a separator after it, so
		// the next keystroke is already looking inside it.
		if m.asking.at < len(b.rows) {
			m.asking.set(b.rows[m.asking.at] + "/")
		}
	case k == "enter":
		return m.takeRoot(b)
	}
	return m, nil
}

// takeRoot writes the root the operator settled on and puts conn to
// work on it. The root is what the cursor is on where the line has not
// been typed past it, and what was typed otherwise: somebody who typed
// a whole path and pressed enter meant that path, not the first thing
// that happened to be listed under it.
func (m model) takeRoot(b rootsReport) (tea.Model, tea.Cmd) {
	home := m.head.login.home
	root := strings.TrimSpace(m.asking.text)
	if typedIsADir(root, home) {
		// what was typed names a directory of its own: take it
	} else if m.asking.at < len(b.rows) {
		root = b.rows[m.asking.at]
	}
	if root == "" {
		return m, nil
	}
	full := expandHome(root, home)
	if err := saveRoots(home, []string{tilde(full, home)}); err != nil {
		m.rootErr = err.Error()
		return m, nil
	}
	// conn works from it now, not on the next start: the roots the
	// reading names projects by are the ones just written, and the walk
	// and the table are asked again against them.
	m = m.rooted(rootOn([]string{full}))
	m.view, m.processesGen = viewProcesses, m.processesGen+1
	cmds := []tea.Cmd{m.readProcesses(), m.scanProjects()}
	if m.inside {
		cmds = append(cmds, m.serverCmd(func() error { return m.srv.narrow() }))
	}
	return m, tea.Batch(cmds...)
}

// typedIsADir says whether what was typed already names a directory, so
// that a path typed in full is taken as it stands.
func typedIsADir(typed, home string) bool {
	if strings.TrimSpace(typed) == "" {
		return false
	}
	info, err := os.Stat(expandHome(strings.TrimSpace(typed), home))
	return err == nil && info.IsDir()
}

// leftHelp is conn putting things back as the manual found them.
// Reading is a detour: the operator asked a question in the middle of
// something, and the answer to it is not a reason to move them.
//
// So the keys go back where the chord took them from. Pressed in the
// workspace, they go back into that pane — the work is put back in the
// workspace first, since the manual displaced it to a window of its
// own and selecting it there is nothing happening at all. Pressed on
// the panel, they stay on the panel: the operator was working the view,
// and the workspace takes a hold, with the page coming back to it on
// the next reading as it always does.
//
// A manual found dead rather than leaving — killed from outside, or
// gone while nobody was tending the workspace — knows of no chord, and
// falls back on the process the manual was standing in front of.
func (m model) leftHelp(found bool) (tea.Model, tea.Cmd) {
	m.helping = false
	from := m.helpFrom
	m.helpFrom = ""
	// The row the operator was on comes back with them. follow lets it
	// go on the next reading if the process has ended meanwhile, which
	// is what it does for a row nobody ever left.
	m = m.tookBackRow()
	if !m.inside || m.srv == nil {
		return m, nil
	}
	toPanel := tea.Batch(m.reviveBay(), m.serverCmd(func() error { return m.srv.focusPanel() }))
	if from != "" {
		if p, tty, ok := m.paneByID(from); ok && reachable(p) {
			return m, m.reach(p, tty)
		}
		// The pane the chord came from has gone while the manual was up.
		// There is nothing to be put back into, and the panel is where
		// conn is worked from.
		return m, toPanel
	}
	// No chord to go on: the manual ended without saying. Back into the
	// work it was standing in front of, where there is any.
	if found {
		mm, cmd := m.backIn()
		m = mm.(model)
		if cmd != nil {
			return m, cmd
		}
	}
	return m, toPanel
}

// tookBackRow puts the cursor back on the row the manual was asked
// from. Nothing happens where there was none: a manual asked for with
// no row under the cursor leaves with none, which is the same answer.
func (m model) tookBackRow() model {
	if m.helpCursor == 0 {
		return m
	}
	m.cursor, m.helpCursor = m.helpCursor, 0
	return m.published(false)
}
