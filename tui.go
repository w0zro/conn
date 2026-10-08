package main

import (
	"cmp"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/w0zro/conn/internal/console"
	"github.com/w0zro/conn/internal/draw"
	"github.com/w0zro/conn/internal/room"
	"github.com/w0zro/conn/internal/work"

	"github.com/w0zro/conn/internal/station"

	"github.com/w0zro/conn/internal/theme"

	"github.com/w0zro/conn/internal/config"

	tea "charm.land/bubbletea/v2"
)

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
// A view is what the panel is showing.
type view int

const (
	viewConsole view = iota
	viewProcesses
	viewProjects
	viewSessions
	viewRoots  // the first root being typed, on a conn told nowhere to look
	viewLog    // the panel over time, which l puts up; see log.go
	viewOutput // a project's output searched, which / puts up; see output.go
)

// reads says whether the table is read again on a beat while the view
// is up: the views whose rows are the processes, or lean on them. The
// processes view is the table; the list holds live processes under its
// projects; the log says whether a line's row is still there; the
// output view searches panes the table names. The console reads the
// station instead, the sessions view and the roots view nothing.
func (v view) reads() bool {
	switch v {
	case viewProcesses, viewProjects, viewLog, viewOutput:
		return true
	}
	return false
}

// paged says whether the page follows the view's cursor in the
// workspace: the views with a cursor on something the page can be
// about. The output view has the bay follow its cursor itself, with
// the match's pane; see subject.
func (v view) paged() bool {
	switch v {
	case viewProcesses, viewProjects, viewSessions, viewLog, viewOutput:
		return true
	}
	return false
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

// awaited is a process conn has just started — a shell, or the first
// of what a bring-up opened — which the cursor goes to once the process
// table has it, and until when that is waited for. A zero pid waits on
// nothing.
type awaited struct {
	pid   int
	until time.Time
}

// awaiting is a wait on pid, for as long as an opened process is worth
// waiting for.
func awaiting(pid int) awaited {
	return awaited{pid: pid, until: time.Now().Add(waitForOpened)}
}

// found is a reading's answer to the wait: over, with the pid where the
// rows now have it, or with none where the wait has run out.
func (w awaited) found(projects []work.Project, now time.Time) (pid int, over bool) {
	switch {
	case w.pid == 0:
		return 0, false
	case hasPid(projects, w.pid):
		return w.pid, true
	case now.After(w.until):
		return 0, true
	}
	return 0, false
}

// The console's alarms blink like annunciators on a panel: lit for a
// second, dark for half of one. The dark is the shorter half — the
// blink is there to catch the eye, not to take the words away.
const (
	blinkLit  = time.Second
	blinkDark = time.Second / 2
)

// A working row's spinner turns a frame at a time: eight frames a
// turn, a turn a second. It turned with the clock before, a frame a
// second, and a turn eight seconds long read as a glyph changing now
// and then rather than as anything moving.
const spinEvery = 125 * time.Millisecond

type (
	stageMsg   struct{} // the next stage is due
	clockMsg   struct{} // the second has turned
	stationMsg struct { // the station is read
		station.Station
		gen int // the run of the station's beat that read it
	}
	stationTickMsg struct{ gen int } // the station is due to be read again
	processesMsg   struct {          // the process table is read
		projects   []work.Project
		panes      map[string]room.Pane // the server's panes by terminal
		bay        string               // the terminal in the bay
		noBay      bool                 // home has no bay beside the panel
		bayDead    bool                 // the bay's pane held on remain-on-exit, its process gone
		bayReadout bool                 // the bay holds the readout, so the page is up
		bayDetour  detourTo             // the page of conn's own the bay holds, if any
		bayActive  bool                 // the keys are in the bay, by tmux's own word
		err        string
		// The projects' .conn files as this reading found them, kept on
		// the model for the next reading to stat against; see declared.go.
		declared map[string]work.Declared
		// The projects whole, where projects is the fold of them.
		tree []work.Project
		gen  int
		// What this reading leaves for the next to read against.
		trace *trace
		// The table's record behind each row, for the page; see cursor.go.
		records map[int]record
		// The rooting the reading was made on: the roots it found the
		// file naming, and which directories were projects as it read
		// them. The model goes onto it with the rows it filed.
		rooted *rooting
	}
	processesTickMsg struct{ gen int }             // the processes view is due to be read again
	openedMsg        struct{ shell room.Shell }    // a shell was opened; the cursor goes to it once it is read
	noticeMsg        struct{ text string }         // something asked of the server was not done, and this is why
	raisedMsg        struct{ shells []room.Shell } // declared processes were brought up, parked: a project's, or one
	reachedMsg       struct{ tty string }          // a process was put in the bay
	readoutMsg       struct{ on bool }             // the readout was put in the bay, or taken out of it
	detourMsg        struct{ to detourTo }         // the manual or the settings were put in the bay
	blinkMsg         struct{ gen int }             // the chip's half is up
	spinMsg          struct{ gen int }             // the spinner's next frame is due
	projectsMsg      struct {                      // the roots were walked
		projects []projectRow
		err      string
	}
	previewTickMsg struct{ gen int } // the output view's cursor has rested, and the bay follows it
	sessionsMsg    struct {          // a project's suspended sessions were read, or every project's
		dirs     []string
		recent   bool
		sessions []work.Session
	}
)

type model struct {
	head          station.Station // what the header needs: the build and who is at the station
	console       consoleOn       // the console, as far as it has come on
	now           time.Time
	width, height int
	p             draw.Palette
	g             theme.Ground // the ground conn is on, which the palette is built off and the status line is written from

	view     view
	lit      bool // the annunciators are showing this half of the blink
	blink    beat // the blink's tick, in flight while something annunciates
	spin     beat // the spinner's, in flight while a row is working
	survey   beat // the station's reading, in flight while the console is up
	projects []work.Project
	cursor   int     // the pid the cursor is on
	cursorAt int     // where in the rows it was, for when the pid goes
	told     subject // the subject as last published for the readout to follow
	// The table's record behind each row as last read, published with
	// the rows for the page; see cursor.go.
	records map[int]record
	// The manual or the settings, where one is the thing in the
	// workspace; see detour.go.
	detour detour
	// Whether the keys are on the panel. conn is told by the terminal
	// when they arrive and when they leave, and knows on its own when
	// its own reaching sent them away, so a terminal that reports no
	// focus does not leave conn guessing where they are.
	focused  bool
	entering bool // the console is waiting on a reading to go to the processes view
	// This panel came up relieving one of another build; see resuming.
	// resumed holds the console dark until the first reading lands, and
	// relieved has the band say so until the next key.
	resumed, relieved bool
	// The words conn last put on the status line, so they are written
	// when they change and not on every pass through Update. The
	// option outlives the conn that set it — a reground respawns the
	// panel, and the fresh conn inherits whatever the last one left — so
	// nil means "not written yet", not "the server says nothing", and
	// the first writing goes out whatever it holds.
	said         *band
	awaited      awaited // a process conn has just started, which the cursor goes to
	processesErr string
	// What the server would not do, in its own words, said under the
	// rows until the next key. A shell that could not be opened left
	// nothing on the screen at all: the operator pressed a key and
	// nothing happened, which is the one thing conn should never leave
	// them with.
	notice       string
	processesGen int // which stay in the processes view the ticks belong to
	// The pane the keys were in when the panel key brought them here
	// and a detour was begun with the next key, for a view there is
	// something to cancel out of. Blank where the keys were already
	// here.
	from string
	// The pane the keys came out of at the panel key just pressed,
	// held for the one key after it: p, ? and A begin a detour, and a
	// detour ends where the keys were before it. Any other key is the
	// operator working the view, and the arrival is over.
	came  string
	trace trace // what the last reading left for the next to read against

	list  projectList // the list, which p puts up
	uid   int
	roots rooting // where the checkouts are kept, and the finders built on it

	sessions sessionList // the sessions view, which A puts up

	// The log, which l puts up, and the rows as the last reading filed
	// them, which the next is read against for it; see log.go.
	log     logList
	seen    []work.Project
	seenAny bool

	out outList // the output view, which / puts up; see output.go

	// The tail of every held pane as last read, by pane id, for what
	// each has said since; see said.go.
	tails map[string][]string

	// The asking view: the first root being typed. It is the first start
	// alone — a root changed on a conn already at work is typed in the
	// settings, which are a pane of conn's own.
	asking rootLine

	// kill is a kill x has asked for and not yet answered; nothing else
	// binds while it is not nil.
	kill *pendingKill
	// firstG is a g that has been pressed and is nothing on its own: the
	// first half of gg, waiting to see whether the next key is its
	// second. Unlike a kill it asks nothing and says nothing — a motion
	// half typed is not a question — so any other key simply goes on to
	// be the key it is.
	firstG bool

	srv    *room.Server         // conn's tmux server, when there is one
	inside bool                 // this conn is the panel of the server's home window
	self   string               // this binary, for the hold
	panes  map[string]room.Pane // the server's panes by terminal, as last read
	bay    bay                  // what is in the bay and what it has held; see bay.go

	// What docker last said, and the feed that says it. The containers
	// are read beside the process table rather than in it, so a reading
	// merges what is already here and never waits on the daemon; stalled
	// is docker having gone quiet, which the view admits rather than
	// showing yesterday's rows as though they were today's.
	containers []work.Container
	// The projects' .conn files as last read; see declared.go.
	declared map[string]work.Declared
	// The processes as read, whole, and whether the view shows them
	// so: at rest it shows the fold of them; see fold.go.
	tree          []work.Project
	full          bool
	up            time.Time // when this conn came up, for the band's clock
	dockerFeed    *work.DockerFeed
	dockerStalled bool
	// What brew last said of its services, merged into every reading
	// while any project declares one; see brew.go.
	brews []work.BrewService
}

// newModel is conn on a ground: drawn in that ground's palette on the
// surface, the panel's own, and speaking to tmux in its colors.
func newModel(g theme.Ground) model {
	home, _ := os.UserHomeDir()
	// A config that will not parse is the view's to report, not the
	// model's to come up on: newModel takes the roots it is left with
	// and the first scan says what is wrong with the file.
	configured, _ := config.Roots(home)
	m := model{
		up:      time.Now(),
		lit:     true,
		focused: true, // conn comes up with the keys in the panel
		// conn comes up on the console, which annunciates, and Init sets
		// the blink going with everything else.
		blink: beat{on: true},
		// And the station is read with it, and again while the console
		// stands; see surveyed.
		survey: beat{on: true},

		head: station.Station{Build: station.ReadBuild(), Login: station.ReadLogin()},
		now:  time.Now(),
		p:    draw.Colored(g).OnSurface(),
		g:    g,
		uid:  os.Getuid(),
	}
	return m.rooted(rootOn(configured))
}

// A rooting is conn on a set of roots: the directories as they were
// configured, the same as the process table names them, and the two
// finders built on them — which directories are projects, and which
// project holds a directory. The finders remember what they found, so
// one reading asks the disk once a directory; each reading builds its
// own, since what they found is true of the disk as it stood then.
type rooting struct {
	configured, real []string
	isProject        func(string) bool
	rootOf           func(string) string
}

// rootOn is the rooting for a set of configured roots.
func rootOn(configured []string) rooting {
	r := rooting{configured: configured, real: realRoots(configured)}
	r.isProject = work.ProjectDirs(r.real)
	r.rootOf = work.RootFinder(r.isProject)
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

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{readStation(m.survey.gen), work.StartDocker, m.nextStage(), nextSecond(m.now), m.nextBlink()}
	if work.BrewPath != "" {
		cmds = append(cmds, work.NextBrew())
	}
	if m.inside {
		// The panel names the build it runs, which is how a conn of
		// another one knows to relieve it; see room.Relieve.
		srv, self := m.srv, m.self
		cmds = append(cmds, m.serverCmd(func() error {
			print, err := room.Fingerprint(self)
			if err != nil {
				return err
			}
			return srv.SayBuild(print)
		}))
		if m.resumed {
			cmds = append(cmds, m.readProcesses())
		} else {
			cmds = append(cmds, m.serverCmd(func() error { return m.srv.Wide() }))
		}
	}
	return tea.Batch(cmds...)
}

// resuming is the panel come up in place of one of another build. The
// operator was at work and asked for nothing but the new build, so it
// goes to the processes view on its first reading, the cursor on the
// row the last panel had it on, and the console stays dark: the
// workspace stays the shape it was rather than giving the window to a
// page nobody asked for. Told nowhere to look, there is no processes
// view to go to, and conn comes up on the console as it always has.
func (m model) resuming(home string) model {
	if len(m.roots.real) == 0 {
		return m
	}
	at, _ := askCursor(cursorPath(home))
	m.cursor = at.pid
	m.processesGen++
	m.resumed, m.relieved, m.entering = true, true, true
	return m
}

// readStation reads the station off the loop, for a run of the
// station's beat.
func readStation(gen int) tea.Cmd {
	return func() tea.Msg { return stationMsg{station.Read(), gen} }
}

// stationEvery is how often the station is read again while the console
// is up: the table's own beat, so the console's memory, load, disk and
// power move with the rows. A reading runs a handful of programs and
// takes under half a second, and runs only while the console stands.
const stationEvery = processesEvery

// surveyed starts the station's beat when the console comes up and lets
// it stop when it goes. The console read the station once, as conn came
// up, and said it again against the clock from then on: memory, swap,
// load, disk and power as they stood at the start of a stay that can
// run for days. Coming up it reads at once, so the console brought back
// with c says the machine as it is and not as it was.
func (m model) surveyed() (model, tea.Cmd) {
	if !m.survey.set(m.view == viewConsole) || !m.survey.on {
		return m, nil
	}
	return m, readStation(m.survey.gen)
}

// nextSecond ticks on the turn of the second, not a second after the
// last tick, so no second is skipped.
func nextSecond(now time.Time) tea.Cmd {
	return tea.Tick(time.Until(now.Truncate(time.Second).Add(time.Second)), func(time.Time) tea.Msg { return clockMsg{} })
}

// A beat is a tick that runs only while something needs it: on is
// whether its tick is in flight, and gen which run of it a tick belongs
// to, so a tick from a run that has ended is dropped when it lands
// rather than starting a second one beside the new.
type beat struct {
	gen int
	on  bool
}

// set starts the beat or lets it stop, as it is wanted, and says
// whether that changed anything. A tick already in flight belongs to
// the run that has ended.
func (b *beat) set(want bool) bool {
	if want == b.on {
		return false
	}
	b.on, b.gen = want, b.gen+1
	return true
}

// annunciating says whether anything conn is drawing blinks as things
// stand: the console's verdict while the console is up, and a row in
// the processes view that is waiting on you, or carrying more context
// than is good for it. Nothing else does — a
// fault in the processes view wears a chip and keeps it, since a
// process you suspended yourself is not asking anything of you, and a
// word that blinks all day is a word that is never seen.
func (m model) annunciating() bool {
	switch m.view {
	case viewConsole:
		return true
	case viewProcesses:
		return len(work.WaitingRound(m.projects)) > 0 || slices.ContainsFunc(m.projects, func(pl work.Project) bool {
			return slices.ContainsFunc(pl.Entries, func(e work.Entry) bool { return heavy(e) != "" })
		})
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
	if !m.blink.set(m.annunciating()) {
		return m, nil
	}
	// The lit half is where anything not blinking rests.
	m.lit = true
	if !m.blink.on {
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
	gen := m.blink.gen
	return tea.Tick(d, func(time.Time) tea.Msg { return blinkMsg{gen} })
}

// working says whether a spinner is turning on the panel as things
// stand: the processes view filed by state, with a row at work on it.
// The tree on z draws no spinner, and no other view does.
func (m model) working() bool {
	if m.view != viewProcesses || m.full {
		return false
	}
	for _, pl := range m.projects {
		for _, e := range pl.Entries {
			if e.Status == work.StatusWorking {
				return true
			}
		}
	}
	return false
}

// turned starts the spinner's tick when a row begins to work and lets
// it stop when none does, the way blinked does for the blink: eight
// redraws a second are nothing while something is seen to move, and
// too many while nothing is.
func (m model) turned() (model, tea.Cmd) {
	if !m.spin.set(m.working()) || !m.spin.on {
		return m, nil
	}
	return m, m.nextSpin()
}

// nextSpin is the spinner's next frame. A frame from an earlier run of
// the spinner is dropped.
func (m model) nextSpin() tea.Cmd {
	gen := m.spin.gen
	return tea.Tick(spinEvery, func(time.Time) tea.Msg { return spinMsg{gen} })
}

// Update answers a message and, whatever came of it, publishes where
// the cursor ended up. Every path that moves it — j and k, tab, a
// reading that carried it along, the shell conn just opened — publishes
// by going through here, which is the point of doing it in one place
// rather than at each of them: a move that forgot to say so would leave
// the readout reading a row nobody is looking at.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m, cmd := m.update(msg)
	// A reading says it again whether or not it moved, so a file gone
	// missing — a state directory swept, a server that came back — comes
	// back on the next beat rather than staying gone until somebody
	// presses j.
	_, reading := msg.(processesMsg)
	m = m.published(reading)
	m, said := m.saying()
	m, blink := m.blinked()
	m, spin := m.turned()
	m, survey := m.surveyed()
	if said != nil || blink != nil || spin != nil || survey != nil {
		return m, tea.Batch(cmd, said, blink, spin, survey)
	}
	return m, cmd
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
	if !m.inside || m.head.Login.Home == "" {
		return m
	}
	// The processes view with no row at all has no subject either, and
	// the last row gone is what the readout has to hear: told nothing,
	// it went on wording the row out of the reading it last had, with
	// the panel beside it saying NO PROCESSES. So the reading is said
	// again under the subject last told, and the readout, not finding
	// the row in it, says the row is gone.
	if at.none() && m.view == viewProcesses && !m.told.none() {
		at, again = m.told, true
	}
	if at.none() {
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
		tellCursor(cursorPath(m.head.Login.Home), at, &reading{
			projects: projects, records: m.records, panes: m.panes,
			inside: m.inside, containers: m.containers, brews: m.brews, sessions: m.sessions.read,
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
		if c, ok := m.sessions.at(); ok {
			return subject{session: c.ID}
		}
	case viewLog:
		// A line about a row still on the panel is about that row; a
		// line about one that is gone is about nothing the page can say.
		if e, ok := m.log.logAt(); ok {
			if r, ok := m.logEntry(e); ok {
				return subject{pid: r.PID}
			}
		}
	}
	// The output view has the bay follow its cursor itself, with the
	// match's pane rather than the page, and has no subject for the page
	// to take the bay with.
	return subject{}
}

// brewAt is the brew service of a formula, as the panel has it from
// brew, where brew has reported it.
func (m model) brewAt(formula string) *work.BrewService {
	return work.BrewServiceNamed(m.brews, formula)
}

// containerAt is the container a row stands for, where it is one, as the
// panel has it from docker.
func (m model) containerAt(pid int) *work.Container {
	for _, pl := range m.projects {
		for _, e := range pl.Entries {
			if e.PID == pid {
				return reading{containers: m.containers}.containerOf(e)
			}
		}
	}
	return nil
}

func (m model) update(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// The panel holds its width through a resize of the window, once it
		// is a panel: with the bay beside it, in the processes view or the
		// list.
		if m.inside && m.view != viewConsole && m.bay.tty != "" && m.width != room.PanelWidth {
			return m, m.serverCmd(func() error { return m.srv.HoldPanel() })
		}
	case stationMsg:
		// A reading is the station as it is, whichever run asked for it;
		// only the run in flight asks for the next.
		var next tea.Cmd
		if msg.gen == m.survey.gen && m.survey.on {
			gen := m.survey.gen
			next = tea.Tick(stationEvery, func(time.Time) tea.Msg { return stationTickMsg{gen} })
		}
		m, cmd := m.stationRead(msg.Station)
		return m, tea.Batch(cmd, next)
	case stationTickMsg:
		if msg.gen != m.survey.gen || !m.survey.on {
			return m, nil
		}
		return m, readStation(msg.gen)
	case stageMsg:
		return m.stageDue()
	case clockMsg:
		m.now = time.Now()
		return m, nextSecond(m.now)
	case noticeMsg:
		m.notice = msg.text
	case openedMsg:
		// The shell is in the bay; the table will have it in a moment, and
		// the cursor goes to it then. Until then the processes view reads
		// soon.
		m.bay.slotted(msg.shell.Pane.TTY)
		m.focused = false
		m.awaited = awaiting(msg.shell.PID)
		m.processesGen++
		return m, m.readProcesses()
	case raisedMsg:
		// The panes are parked and the keys stayed here; the cursor
		// goes to the first of them once the table has it.
		if len(msg.shells) > 0 {
			m.awaited = awaiting(msg.shells[0].PID)
		}
		m.processesGen++
		return m, m.readProcesses()
	case work.DockerReadyMsg:
		// The feed is running; from here conn waits on its word rather
		// than asking docker anything on a beat.
		m.dockerFeed = msg.Feed
		return m, work.NextDocker(m.dockerFeed)
	case work.DockerMsg:
		// Docker's word, held for the next reading to merge. The rows
		// are drawn again at once rather than on the next beat, which is
		// the whole point of a feed: a container is on its row as it
		// starts, not up to two seconds later.
		m.containers, m.dockerStalled = msg.Containers, msg.Stalled
		m.processesGen++
		return m, tea.Batch(m.readProcesses(), work.NextDocker(m.dockerFeed))
	case work.BrewTickMsg:
		// Brew is asked only while some project declares a service of
		// its own; otherwise the beat passes.
		if work.BrewDeclared(m.declared) {
			return m, work.ReadBrew
		}
		return m, work.NextBrew()
	case work.BrewMsg:
		// Brew's word, drawn again at once, as docker's is. An answer
		// that failed leaves what it last said standing.
		if msg.Err == nil {
			m.brews = msg.Services
		}
		m.processesGen++
		return m, tea.Batch(m.readProcesses(), work.NextBrew())
	case detourMsg:
		// The page is up, and no row is under the cursor while it is;
		// see detour. Where the cursor was is kept, so j and k carry on
		// from it.
		//
		// The reading is taken again from here, as it is for the
		// readout, so one already in flight that saw the workspace as it
		// was cannot land afterwards and say the page is not up.
		m.detour.to, m.cursor = msg.to, 0
		m.processesGen++
		return m.published(false), m.readProcesses()
	case readoutMsg:
		// The page is up, or down, and conn knows it without reading the
		// server: the next i is a keypress away and has to decide which
		// way it goes. The reading is taken again from here so a reading
		// already in flight, which saw the bay as it was before, cannot
		// land afterwards and say otherwise.
		m.bay.readout = msg.on
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
		m.bay.slotted(msg.tty)
		if pid, at, ok := headOf(m.projects, msg.tty); ok {
			m.cursor, m.cursorAt = pid, at
		}
		m.processesGen++
		return m, m.readProcesses()
	case blinkMsg:
		if msg.gen != m.blink.gen || !m.annunciating() {
			m.lit = true
			return m, nil
		}
		m.lit = !m.lit
		return m, m.nextBlink()
	case spinMsg:
		if msg.gen != m.spin.gen || !m.working() {
			return m, nil
		}
		m.now = time.Now()
		return m, m.nextSpin()
	case processesMsg:
		if msg.gen != m.processesGen {
			return m, nil
		}
		return m.landed(msg)
	case processesTickMsg:
		if msg.gen != m.processesGen || !m.view.reads() {
			return m, nil
		}
		return m, m.readProcesses()
	case logMsg:
		return m.landedLog(msg), nil
	case outMsg:
		return m.landedOutput(msg)
	case saidMsg:
		return m.heard(msg)
	case previewTickMsg:
		// The moves have stopped, if this is the tick the last of them
		// set going; an earlier one is passed over.
		if msg.gen != m.out.previews || m.view != viewOutput {
			return m, nil
		}
		return m.preview()
	case projectsMsg:
		wasRow, hadRow := m.atCursor()
		m.list.walked, m.list.err, m.list.scanning = msg.projects, msg.err, false
		m.list.kept(m.projectRows(), wasRow, hadRow)
		// The walk has given the list its rows, and the page comes up
		// for the one the cursor is on without anybody asking.
		return m.keepingPage()
	case sessionsMsg:
		// Only the sessions view that asked for these dirs wants them; one
		// opened on another project since has moved past the answer.
		if !m.sessions.landed(msg) {
			return m, nil
		}
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
	case tea.MouseClickMsg:
		return m.click(msg)
	}
	return m, nil
}

// key answers a key. What every view shares is answered here: a
// question armed takes the key whatever it is, the panel key and the
// alt keys work from any view, and a page of conn's own speaks to the
// panel on keys of its own. Anything else is the view's, and goes to
// the view that has the keys.
func (m model) key(k string) (model, tea.Cmd) {
	// A notice stands until the next key, whatever it is: it was read,
	// or it was not going to be.
	m.notice = ""
	m.relieved = false
	// So does the arrival: the pane the keys came out of is for the key
	// after the panel key and no other, whatever that key is.
	came := m.came
	m.came = ""
	// A kill x asked for takes the next key, whatever it is: y confirms
	// it, and anything else cancels, as tmux's own confirmation goes —
	// no other binding fires while the question is on the status line.
	if m.kill != nil {
		req := m.kill
		m.kill = nil
		if k == "y" {
			return m, req.end
		}
		return m, nil
	}
	// The second g of gg, which is the only key the first one waits for.
	// Every other key clears it and goes on to do what it does, so a g
	// pressed and thought better of costs nothing.
	half := m.firstG
	m.firstG = false
	if half && k == "g" {
		if m.view == viewLog {
			m.log.at = 0
			return m, nil
		}
		m = m.onRow(0, 0)
		return m, nil
	}
	// The keys arriving on the panel, which the panel key sends after
	// bringing them: from inside a process, from the manual, or from
	// the panel itself. On the console it is any key, and the console
	// answers it as it answers any key.
	if k == "alt+-" && m.view != viewConsole {
		return m.arrived(m.cameFrom())
	}
	// A shell, a contact, and the sessions suspended at the project the
	// panel is looking at, from a line typed into. In the list and in
	// the sessions view a plain s or a is a letter being typed into the
	// line, so what a letter does in the processes view is done there
	// with alt. The letter is the same on every road to the thing: a is
	// a contact and A the sessions in the processes view, alt+a and
	// alt+A on a line typed into.
	//
	// In a line typed into, ctrl is readline's and alt is conn's. The
	// line is edited the way readline edits one, and a ctrl key there
	// means what it means to readline — ctrl+a the start of the line,
	// not a contact, which it was; ctrl+s a search, not a shell, which
	// it was — so conn's own verbs on a line are all on alt, so a hand
	// learns one key for one thing wherever it is pressed.
	if k == "alt+s" || k == "alt+a" || k == "alt+shift+a" {
		return m.openAt(k, came)
	}
	// Every project's suspended sessions, newest first: r in the
	// processes view, and alt+r on a line typed into. A is the one
	// project's; see openRecent.
	if k == "alt+r" || k == "r" && m.view == viewProcesses {
		return m.openRecent(came)
	}
	// The log: l in the processes view, and alt+l on a line typed into.
	if k == "alt+l" || k == "l" && m.view == viewProcesses {
		return m.openLog(came)
	}
	// What is under the cursor, brought up: u in the processes view,
	// and alt+u from the list, where u is a letter being typed. On a
	// process's row it is that one process, and the keys stay on the
	// panel, so u pressed down the rows brings them up one at a time;
	// on a project's row, in the list, it is the project. U, and alt+U,
	// bring up everything the row's project declares and does not have
	// running, wherever in it the cursor is.
	if k == "alt+u" || k == "u" && m.view == viewProcesses {
		return m.raiseUnder()
	}
	if k == "alt+shift+u" || k == "U" && m.view == viewProcesses {
		return m.raiseAt()
	}
	// A page of conn's own speaking to the panel. The manual and the
	// settings each say as they go that they are done with, so the
	// workspace is filled in the same breath rather than holding a
	// dead pane until the next reading comes round — and so that it is
	// filled at all, the reading only tending the workspace while the
	// processes view has the keys. The settings say the other thing
	// too: they have put the server in a mode, and the panel draws in
	// colors it read when it came up.
	switch k {
	case "alt+esc", "alt+,":
		return m.leftDetour(false)
	case "alt+w":
		return m.worn()
	}
	switch m.view {
	case viewConsole:
		return m.consoleKey(k)
	case viewProjects:
		return m.projectKey(k)
	case viewSessions:
		return m.sessionsKey(k)
	case viewRoots:
		return m.rootsKey(k)
	case viewLog:
		return m.logKey(k)
	case viewOutput:
		return m.outputKey(k)
	}
	return m.processesKey(k, came)
}

// leave is ctrl+c, from any view, and q where q is not a letter being
// typed: a detach in the server, and conn closing outside it.
func (m model) leave() (model, tea.Cmd) {
	if m.inside {
		// A detach leaves the server and this conn standing, so the
		// feed keeps its stream: there is something still watching.
		return m, m.serverCmd(func() error { return m.srv.Detach() })
	}
	// Going for good takes the feed's stream with it. docker events
	// is a child conn started, and a child outlives the parent that
	// abandons it — it would sit reparented to init until the next
	// container event pushed a write down a pipe nobody holds.
	m.dockerFeed.Close()
	return m, tea.Quit
}

// arrived is the keys having come to the panel by the panel key, from
// the pane they were in, which tmux has already left for the panel by
// the time this is read. The panel is put on the processes view, which
// is what the panel is: the list and the sessions view are left, and
// the manual is put away, its own key being the only thing that can
// reach it. Come out of a pane, the pane's row goes under the cursor,
// so the key after this one is about the process the operator was just
// in — x ends it, s opens a shell beside it — and the pane is held for
// that key, so a detour begun with it ends back in the pane.
//
// Pressed on the processes view itself, where the keys already were,
// it is the other process: the one worked in before this one. So from
// inside a process, twice over is the other process, which is the
// shape that key has everywhere.
//
// The asking view is left alone: there are no processes to show until
// it has been answered.
func (m model) arrived(from string) (model, tea.Cmd) {
	if m.detour.to != noDetour {
		// The key says where to go, so where the page was asked from
		// stops mattering: it is the one way out that does not put the
		// keys back, and forgetting is what makes it that.
		m = m.endDetour()
		return m, tea.Batch(m.reviveBay(), m.processesTick())
	}
	if m.view == viewRoots {
		return m, nil
	}
	var cmds []tea.Cmd
	switch {
	case m.view != viewProcesses:
		// Come to the panel, whatever the list was begun from: the key
		// says where to go.
		m.from = ""
		var cmd tea.Cmd
		m, cmd = m.toProcesses()
		cmds = append(cmds, cmd)
	case from == "":
		return m.toOther()
	}
	if from != "" {
		m.came = from
		if _, tty, ok := m.paneByID(from); ok {
			if pid, at, ok := headOf(m.projects, tty); ok {
				m.cursor, m.cursorAt = pid, at
			}
		}
	}
	return m, tea.Batch(cmds...)
}

// toOther goes to the process that was in the bay before the one in it
// now, and takes the one in it now as the one to come back to — so
// pressed twice it is where it started, and pressed while working is
// the other thing you are working on. It is what the panel key does
// pressed on the panel, so from inside a process it is the panel key
// twice over, which is the shape that key has everywhere: the one you
// were last in.
//
// The console is left on the way, in the rare case it is asked for
// with the console up: you cannot be in a pane while the console is
// over the window, so this is a press from the panel, and the answer
// to it is a process.
func (m model) toOther() (model, tea.Cmd) {
	// Asked as reachable and not merely as held, the way every other
	// road into a pane asks it: a pane whose process has ended is an id
	// conn still has and nowhere to be sent.
	if !m.inside || m.bay.other == "" || !room.Reachable(m.panes[m.bay.other]) {
		return m, nil
	}
	cmds := []tea.Cmd{m.reach(m.panes[m.bay.other], m.bay.other)}
	if m.view == viewConsole {
		m.view, m.entering = viewProcesses, false
		cmds = append(cmds, m.serverCmd(func() error { return m.srv.Narrow() }))
	}
	return m, tea.Batch(cmds...)
}

// cameFrom is the pane the keys were in when the panel key brought
// them to the panel, and nothing where they were already here. The key
// writes it as it fires, because by the time conn reads the key the
// panel is the pane with the keys and nothing on this side can tell
// where they came from. It is read once and cleared, so a press that
// was answered rather than cancelled leaves nothing behind for the
// next one.
func (m model) cameFrom() string {
	if !m.inside || m.srv == nil {
		return ""
	}
	return m.srv.CameFrom()
}

// backFrom is the cancel. The panel comes back to the processes view,
// and where the panel key brought the keys here out of another pane
// and the list was the next key, they go back to it: cancelling is
// putting things as they were, and the pane the operator was working
// in is part of how they were.
//
// Going back to it is reaching it, not selecting it. The pane is not
// where it was: the page takes the workspace while the keys are on the
// panel, and what the page displaced went back to a window of its own.
// Selecting it by id there does select it — in a window the client is
// not looking at, which is nothing happening at all. Reaching it puts
// it back in the workspace first, which is where the operator left it.
//
// A list opened from the panel leaves nothing to go back to, and the
// cancel is the view alone: you were not in a pane, so there is no pane
// to be put back in. Work that ended while the list was up is the same
// answer for the same reason.
func (m model) backFrom() (model, tea.Cmd) {
	from := m.from
	m.from = ""
	m, cmd := m.toProcesses()
	if from == "" || !m.inside {
		return m, cmd
	}
	p, tty, ok := m.paneByID(from)
	if !ok || !room.Reachable(p) {
		return m, cmd
	}
	return m, tea.Batch(cmd, m.reach(p, tty))
}

// paneByID is the pane conn holds under that id, and the terminal it is
// on. conn holds its panes by terminal, a terminal being what a row
// is; the panel key names the pane it fired from by id, which is what
// tmux knows of it.
func (m model) paneByID(id string) (room.Pane, string, bool) {
	for tty, p := range m.panes {
		if p.ID == id {
			return p, tty, true
		}
	}
	return room.Pane{}, "", false
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
func (m model) backIn() (model, tea.Cmd) {
	if !m.inside || m.bay.work == "" {
		return m, nil
	}
	p, ok := m.panes[m.bay.work]
	if !ok || !room.Reachable(p) {
		return m, nil
	}
	return m, m.reach(p, m.bay.work)
}

// toProcesses leaves the list for the processes view, which starts
// reading again.
func (m model) toProcesses() (model, tea.Cmd) {
	// The output view had the bay showing a pane in copy mode; left by
	// any road, the pane is taken out of it, so the one way out that
	// keeps it in, going in, says so before coming here.
	var left tea.Cmd
	if m.view == viewOutput {
		m, left = m.leaveOutput()
	}
	// Coming to the view fresh, the page is what the workspace holds
	// again: a close is for the stay it was made in.
	m.view = viewProcesses
	m.processesGen++
	return m, tea.Batch(left, m.readProcesses())
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
func (m model) keepingPage() (model, tea.Cmd) {
	// The manual and the settings are in the workspace on purpose, and
	// the page would put itself there over the top of either. No row is
	// under the cursor while one of them is up, which would stop this
	// on its own; saying it plainly as well means the page cannot come
	// back the moment the cursor does.
	if !m.inside || m.bay.readout || m.detour.to != noDetour || !m.focused {
		return m, nil
	}
	if !m.view.paged() {
		return m, nil
	}
	if m.subject().none() {
		return m, nil
	}
	m.bay.readout = true
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
		if _, pl, ok := m.under(); ok && pl.Path != "" {
			return pl.Path, []string{pl.Path}, true
		}
	case viewProjects:
		if row, ok := m.atCursor(); ok && row.path != "" {
			return row.path, sessionDirs(m.list.walked, row), true
		}
	case viewSessions:
		if m.sessions.project != "" {
			return m.sessions.project, m.sessions.dirs, true
		}
		// The recent view is for every project, so the project is the
		// row's: the one that holds where its session was had.
		if c, ok := m.sessions.at(); m.sessions.recent && ok {
			root := c.Dir
			if m.roots.rootOf != nil {
				root = cmp.Or(m.roots.rootOf(c.Dir), c.Dir)
			}
			return root, []string{root}, true
		}
	}
	return "", nil, false
}

// openAt opens a shell, a contact or the sessions view at whatever
// project the panel is looking at. A shell and a contact show in the
// processes view, so the panel comes back to it for them, the way the
// list has always come back for what it opened; the sessions view is
// somewhere to be and is gone to, and takes with it the pane the panel
// key just brought the keys out of, where it did, so that leaving it
// puts them back.
func (m model) openAt(k, came string) (model, tea.Cmd) {
	path, dirs, ok := m.atProject()
	if !m.inside || !ok {
		return m, nil
	}
	if k == "alt+shift+a" {
		m.from = came
		return m.openSessions(path, dirs)
	}
	var cmds []tea.Cmd
	if m.view != viewProcesses {
		var cmd tea.Cmd
		m, cmd = m.toProcesses()
		cmds = append(cmds, cmd)
	}
	if k == "alt+a" {
		return m, tea.Batch(append(cmds, m.startContact(path))...)
	}
	return m, tea.Batch(append(cmds, m.openShell(path))...)
}

// raiseAt brings up what the project the panel is looking at declares
// and does not have running. From another view the processes view is
// put up on the way, since that is where the rows will show.
func (m model) raiseAt() (model, tea.Cmd) {
	// The file is read by the raise itself, off the loop, so a project
	// with nothing running — whose file the reading has not read — is
	// brought up from the list all the same.
	path, _, ok := m.atProject()
	if !m.inside || !ok || path == "" {
		return m, nil
	}
	up, held := work.UpAndHeld(m.projects, declaredPanes(m.panes), path)
	var cmds []tea.Cmd
	if m.view != viewProcesses {
		var cmd tea.Cmd
		m, cmd = m.toProcesses()
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(append(cmds, m.raiseAll(path, up, held))...)
}

// runsOf is what a shell runs, in the tree whole: the first row under
// it that is not a shell itself, looking through a bash -c to the
// command it was given, the way the fold does for the shell's own row;
// with nothing but shells under it, the first of those.
func (m model) runsOf(head work.Entry) (work.Entry, bool) {
	projects := m.tree
	if len(projects) == 0 {
		projects = m.projects
	}
	for _, pl := range projects {
		for i, e := range pl.Entries {
			if e.PID != head.PID {
				continue
			}
			var first work.Entry
			for _, under := range pl.Entries[i+1:] {
				if under.Depth <= e.Depth {
					break
				}
				if under.Kind != work.KindShell {
					return under, true
				}
				if first.PID == 0 {
					first = under
				}
			}
			return first, first.PID != 0
		}
	}
	return work.Entry{}, false
}

// clamp holds an index within the rows there are; with no rows it is
// the first, which is no row.
func clamp(at, rows int) int {
	return min(max(at, 0), max(rows-1, 0))
}

// ring is a row index wrapped round the rows there are. The rows are a
// ring to j and k: the row after the last is the first, and the row
// before the first is the last, so a hand cycling through the
// processes is never stopped at an end. With no rows it is the first.
func ring(at, rows int) int {
	if rows <= 0 {
		return 0
	}
	return ((at % rows) + rows) % rows
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
		return min(room.PanelWidth, m.width)
	}
	return m.width
}

func (m model) View() tea.View {
	var rows []draw.Row
	width := m.cols()
	switch {
	// The manual is in the workspace with the keys in it, and the panel
	// holds the keys themselves: what a hand looking for one has to
	// read, in the half of the window where they are pressed.
	case m.detour.to == toManual:
		rows = drawKeys(panelKeys(keyWord(room.PanelKey())), "processes", width, m.height, m.p)
	case m.view == viewProcesses:
		rows = drawProcesses(m.processesReport(), m.cursor, width, m.height, m.p)
	case m.view == viewProjects:
		rows = drawProjects(m.projectsReport(), m.list.find.at, width, m.height, m.p)
	case m.view == viewSessions:
		rows = drawSessions(m.sessions.report(m.head.Login.Home, m.roots.real, m.now, m.roots.rootOf), m.sessions.find.at, width, m.height, m.p)
	case m.view == viewRoots:
		rows = drawRoots(m.asking.report(m.head.Login.Home), m.asking.line.at, width, m.height, m.p)
	case m.view == viewLog:
		rows = drawLog(m.logReport(), m.log.at, width, m.height, m.p)
	case m.view == viewOutput:
		rows = drawOutput(m.outputReport(), m.out.find.at, width, m.height, m.p)
	default:
		r := m.report()
		r.Lit = m.lit
		rows = console.Screen(r, width, m.height, m.p)
	}
	ground := rows[0].Text // the first row is blank, on the ground, at the rows' width
	texts := make([]string, 0, len(rows))
	for i, r := range rows {
		if i >= m.height && m.height > 0 {
			break
		}
		if m.view == viewConsole && (m.resumed || r.Stage > m.console.stage) {
			texts = append(texts, ground)
		} else {
			texts = append(texts, r.Text)
		}
	}
	v := tea.NewView(strings.Join(texts, "\n"))
	v.AltScreen = true
	// conn is told when the keys arrive in its pane and when they
	// leave, which is how the workspace beside it knows to hold the
	// page: the keys coming back to the panel is the operator asking
	// what a row is, and nothing else announces that.
	v.ReportFocus = true
	// The mouse: a press on a row is a way to the row. tmux keeps the
	// rest of the mouse, the wheel and a drag into copy mode among it,
	// and passes conn the presses in its pane.
	v.MouseMode = tea.MouseModeCellMotion
	v.BackgroundColor = m.g.Ground
	v.ForegroundColor = m.g.Ink
	v.WindowTitle = "conn"
	return v
}

// worn is the settings saying they have put the server in a mode. The
// panel reads the mode file where it stands and wears what it says: a
// fresh conn in this pane would be the console, and the operator
// picked a theme rather than asking to start again.
//
// Nothing is asked of the server. The pane conn draws is painted by
// the reground the settings asked for, and the sixteen every other
// pane draws from went with it; what is left is the colors this conn
// holds in memory.
func (m model) worn() (model, tea.Cmd) {
	if m.srv == nil {
		return m, nil
	}
	want, ok := theme.ReadModeFile(m.srv.Socket)
	if !ok {
		return m, nil
	}
	m.g = want.Wear()
	m.p = draw.Colored(m.g).OnSurface()
	return m, nil
}
