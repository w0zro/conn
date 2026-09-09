package main

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// procPoll is how often the process list is refreshed. Processes come and go
// constantly, and an lsof sweep is cheap enough to repeat at this rate.
const procPoll = 2 * time.Second

// The agent poll lives in agent.go with the rest of the seam. It is far
// shorter than the process poll because it is a different kind of work: a
// handful of small files rather than an lsof sweep of every process on the
// machine, which is some three orders of magnitude apart. Tying the two
// together made a session that had started working wait up to a process poll
// to say so, which is exactly the moment the marker is for.

// projectEvery is how many process polls pass between repository scans.
// Repositories appear and disappear far more slowly than processes do.
const projectEvery = 15

// repoDetailEvery is how many process polls pass between refreshes of a
// selected repository's details. Those are half a dozen git spawns — git
// status among them, which in a large work checkout is real work — and what
// they report changes at the speed of a person committing, not of a process
// list. A process row keeps the every-poll rate: cpu, memory and ports are
// exactly the numbers that move.
const repoDetailEvery = 5

// tickMsg drives the refresh loop. Exactly one tick is ever in flight: each
// one schedules its successor, so a one-off rescan — after a kill, say — can
// never start a second chain that doubles the polling rate.
type tickMsg struct{}

// tick schedules the next refresh.
func tick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return tickMsg{} })
}

// projectsMsg carries the result of the startup scan: the repositories, the
// groups that hold them, the sub-projects found inside each repository,
// keyed by the repository's path, and the roots they were all found under.
type projectsMsg struct {
	projects []Project
	groups   []Project
	subs     map[string][]Project
	roots    []string
	// tasks and plans are what each place defines, by its path: the
	// tasks it says how to run and the plan it says it needs, read off
	// the render path with the places, for the finder to list.
	tasks map[string][]task
	plans map[string]plan
	err   error
}

// procsMsg carries the running processes. It arrives at startup and again
// whenever the view is narrowed, so the narrowed list reflects what is running
// now rather than at launch.
type procsMsg struct {
	procs []Proc
	err   error
}

// rowKind distinguishes the two things the navigator lists.
type rowKind int

const (
	rowGroup   rowKind = iota // a folder of repositories that make one project
	rowProject                // a repository
	rowSub                    // a sub-project: a directory inside a repository with a manifest
	rowProc                   // a process
	rowRest                   // a conversation at rest: an agent that exited and can be picked back up
)

// navRow is one selectable line: a repository, or a process inside one.
//
// A run of processes that never branches is one row, because it is one thing
// happening: a shell that started an editor is the editor, and an editor that
// forked itself is still the editor. node is the process the row is named for,
// the deepest of that run; chain is the top of it, which a tree kill has to
// cover. For a row that folded nothing the two are the same.
type navRow struct {
	kind    rowKind
	project Project
	name    string       // a place's name as its heading reads: the repository under its group, the sub-project under its repository
	run     []*ProcNode  // the whole folded run, oldest first
	node    *ProcNode    // the one in it the row is named for
	prefix  string       // the indent of the ancestors already drawn
	rest    conversation // for a row at rest, the conversation it stands for
}

// placeName is what a place's row is called: a repository under a group
// is named for both, w0zro/conn, and a sub-project for its repository
// and itself, conn/docs — every place a heading of its own, at the margin.
func (r navRow) placeName() string {
	if r.name != "" {
		return r.name
	}
	return r.project.Name
}

// chain is the top of the run, which a tree kill has to cover.
func (r navRow) chain() *ProcNode {
	if len(r.run) == 0 {
		return r.node
	}
	return r.run[0]
}

// leaf is the bottom of the run. Anything below it branches, so that is where
// the tree carries on.
func (r navRow) leaf() *ProcNode {
	if len(r.run) == 0 {
		return r.node
	}
	return r.run[len(r.run)-1]
}

type model struct {
	// width and height are what conn draws in: its pane. windowRows is
	// the height the terminal last reported; height is held to the
	// chrome's rows while a buffer is shown under it (keepRows).
	width, height, windowRows int

	// host is what the last process scan read, and containers what docker
	// last said (dockerfeed.go); procs is the two merged, the tree's
	// input, remade when either arrives. docker is the feed, and
	// dockerStalled says its last word was that docker is not answering,
	// so the next can tell whether that is still news.
	host, containers []Proc
	docker           *dockerFeed
	dockerStalled    bool

	projects []Project
	err      error

	// procs are the running processes; byPlace groups them under the place
	// they are working in — a repository, or a sub-project inside one — each
	// group already arranged into parent/child trees. parent maps a pid to
	// the one that started it, which is how a process is traced back to the
	// shell it is running inside.
	procs   []Proc
	byPlace map[string][]*ProcNode
	parent  map[int]int
	nodes   map[int]*ProcNode

	// tasks and plans are what each place defines, by its path, as the
	// last scan of the places read them: the finder lists the ones not
	// running.
	tasks map[string][]task
	plans map[string]plan

	// subs are each repository's sub-projects, keyed by the repository's
	// path. They come from the repository scan, not the process scan: a
	// sub-project exists whether or not anything is running in it, which is
	// what lets the filter reach it cold.
	subs map[string][]Project

	// groups are the folders grouping repositories into one project, and
	// grouped files each group's repositories under its path. A project is
	// often several repositories in one directory, worked on at that level.
	groups  []Project
	grouped map[string][]Project

	// unfolded draws every process on a line of its own, including the ones a
	// run would otherwise fold away. The folded view is the reading view; this
	// is for when the shell in the middle is the thing you came to find.
	unfolded bool

	// filter narrows the navigator to the repositories whose name or path
	// matches it, searching every one rather than only those with something
	// running: the point of it is to reach a project you are not working in.
	// typing says the filter has focus rather than the list.
	// query is the line the filter is typed on: a text input with the
	// editing keys every other line has — readline's — which conn does not
	// own a case of. filter mirrors its value for everything that reads it.
	filter string
	typing bool
	query  textinput.Model

	// filterFrom is where the search began: the subject under the cursor.
	// Abandoning the filter with esc puts it back — acting on a result does
	// not, because acting is the point of having looked.
	filterFrom string

	// roots are the config's project directories, as the last scan read
	// them: where a new project goes when nothing under the cursor says.
	roots []string

	// release is the one newer than this build, when one is known: the
	// status line says so whenever nothing else is said, and U takes it.
	release string

	// showAll toggles the everything view between every repository and only
	// those with a process running in them. It starts off: the repositories
	// with something running in them are the ones worth seeing.
	showAll bool

	// all says the everything view is up: conn has the window, and draws
	// the list under the tabline. Off, with nothing shown, the window is
	// the empty workspace.
	all bool

	// from is the buffer a kill preview was asked from, parked while
	// the preview has the window and shown again after — the buffer stays
	// as the record of the ending, or as it was when esc kept it.
	from int

	// history is each named buffer's past runs, newest first, read for the
	// heading's strip once per ending rather than per frame.
	history map[int][]run
	// histories is what each strip was read against — the buffer's last
	// ending — so a strip is read again when the ending changes.
	histories map[int]string

	// deep is what each held agent says of itself when read deeper than
	// the scan does — its branch, its context — for the heading, and for
	// telling two tabs of one name apart; read off the render path.
	deep map[int]agentFacts

	// seen holds the agents whose finished turn has been looked at: the
	// buffer was shown, or had focus when the turn ended. A turn looked at
	// is quiet — the filled mark is for a result you have not seen — and
	// the next turn the agent works clears it.
	seen map[int]bool

	// snapshot is the finder's listing as last written beside the socket,
	// so it is written again only when it changed.
	snapshot string

	// rows is the flattened navigator, rebuilt whenever its inputs change;
	// cursor indexes into it and offset is the first row on screen.
	rows   []navRow
	cursor int
	offset int

	// collapsed holds the subjects whose children are folded away, keyed the
	// same way as details so the state survives a rescan.
	collapsed map[string]bool

	// pendingKill is the kill that has been asked for but not confirmed:
	// the preview has the window until it is. Killing cannot be undone, so
	// it takes a second key.
	pendingKill *killRequest

	// pendingG is a g waiting for the g that makes it mean the top.
	pendingG bool

	// dying holds the processes that have been signalled and are still listed.
	// They keep their place, marked, until a rescan finds them gone: a row that
	// vanished on the keystroke would claim an exit conn has not seen yet.
	// spinning says whether the frame chain is running and frame is its count.
	dying    map[int]dyingProc
	spinning bool
	frame    int

	// refused holds the processes that were signalled and did not go in
	// the time the marker gives them, by pid, named: x on one of them
	// offers SIGKILL rather than the signal it has already ignored. A
	// process is off the list once a scan finds it gone.
	refused map[int]string

	// status is a one-line report of the last action, cleared as soon as the
	// cursor moves on.
	status    string
	statusErr bool

	// ticks counts refresh cycles, so slower work can run every Nth one.
	ticks int

	// scanning says a process scan is out, so another is never started behind
	// it: on a machine where lsof stalls, a poll that kept asking would pile a
	// stalled scan on top of every tick. rescan says one was wanted while one
	// was out — asked for by an event, not the poll — and is owed the moment
	// the answer lands, because that answer predates the event it was about.
	scanning bool
	rescan   bool

	// agents holds every live agent instance currently advertised, of every
	// kind, keyed by pid. It is refreshed on its own fast poll so the
	// navigator can mark the working and the waiting without the cursor
	// having to visit them.
	agents map[int]agent

	// rests is, by place, the newest conversation at rest there — an agent
	// that exited, with its transcript to pick it back up — listed as a
	// dimmed row under the place while the place has work, the way an
	// exited container is kept beside its running siblings. restLive is
	// the live conversations the listing was made against; a change in
	// them is an instance gone or come, and a fresh listing.
	rests    map[string]conversation
	restLive string

	// manifestDirs is, by a process's directory, the sub-project a manifest
	// on the way up to its repository makes of it, or "" for none — kept
	// between scans of the places, which reset it.
	manifestDirs map[string]string

	// terms are the shells the server is holding, keyed by the pid running
	// each one. A repository can hold as many as you open; they tell themselves
	// apart in the navigator because each is its own process in that
	// repository's tree. The navigator owns none of them: tmux holds them,
	// draws the one under the cursor in the pane beside the navigator, and
	// gives it focus.
	terms map[int]*remoteTerm

	// server is the connection to the tmux server holding the shells, and
	// err says so when there is not one.
	server    *session
	serverErr string

	// backoff is the wait before the next attempt to reach a server that went
	// away or could not be reached, doubled per consecutive failure and reset
	// once a server is talking.
	backoff time.Duration

	// pendingReplace is R waiting on its confirmation: ending the server
	// ends the work it holds, so it takes a second key like any other kill.
	pendingReplace bool

	// wantProject is a project whose processes were just started, holding the
	// cursor until the server holds them; wantName is the entry the cursor
	// goes on to then — the first one started — landing on its row once
	// the scan has it.
	wantProject, wantName string

	// wantCursor is a shell just opened, waiting for the scan that will put it
	// in the tree. The cursor moves to it when it lands, so leaving the shell
	// leaves the cursor on the row that shell belongs to.
	wantCursor int

	// shown is the buffer in the pane under the tabline, and zero when
	// conn has the window to itself — for the everything view, a kill
	// preview, or the empty workspace. shownGone says the shell shown has
	// gone by the server's word, and the next list says what is under the
	// tabline now: the same pane with a new shell in it, after a rerun in
	// place, or nothing.
	shown     int
	shownGone bool

	// focus is what has focus, as far as the navigator knows: zero for the
	// navigator itself, else the pid of the shell that has it. was is where
	// focus was before that — the other end of ctrl-space ctrl-space. The
	// navigator moves focus everywhere it goes but by the mouse, and the
	// mouse can only move it between the navigator and the shell beside
	// it, which the navigator's own focus coming and going says.
	focus, was int

	// synced says the first list from this connection has been read: the
	// one that tells a navigator starting beside a shell already shown
	// which shell that is, so it can begin on that row rather than move
	// the shell aside.
	synced bool

	// dressed is the name each shell's pane last wore, so tmux is only
	// told what changed.
	dressed map[int]string

	// said is the mode and message the status line last read, for the
	// same reason.
	said statusText

	// askedExit is the plan shells found at their prompt whose exit the
	// server has been asked for, so the server is asked once per exit.
	askedExit map[int]bool

	// unread is the shells whose ending has settled and whose transcript
	// has not been read for what it says of the run: read once, off the
	// render path, by the next Update.
	unread map[int]bool

	// endings counts the endings learned of, to order them: the latest of
	// several shells for one entry is the one that speaks for it.
	endings int
}

func newModel() model {
	return model{
		query:     newLine(),
		collapsed: map[string]bool{},
		askedExit: map[int]bool{},
		unread:    map[int]bool{},
		dying:     map[int]dyingProc{},
		refused:   map[int]string{},
		terms:     map[int]*remoteTerm{},
		dressed:   map[int]string{},
		history:   map[int][]run{},
		histories: map[int]string{},
		deep:      map[int]agentFacts{},
		seen:      map[int]bool{},
		// Init sends the first scan, and Init cannot write here to say so.
		scanning: true,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(scanProjects, scanProcs, scanAgents, connectServer(), startDocker,
		tick(procPoll), agentTick(), checkUpdate(false, time.Now()))
}

// scanProjects loads the config and walks the projects directory off the
// render path, so a slow disk cannot delay the first paint.
func scanProjects() tea.Msg {
	cfg, err := loadConfig()
	if err != nil {
		return projectsMsg{err: fmt.Errorf("config: %w", err)}
	}
	projects, groups, err := discoverAll(cfg.roots(), cfg.skipSet())
	if err != nil {
		return projectsMsg{err: err}
	}

	// Each repository's index is asked for its sub-projects, together rather
	// than in turn: fifty repositories at twenty milliseconds each would
	// otherwise hold the first paint for a second.
	subs := make(map[string][]Project, len(projects))
	tasks := make(map[string][]task, len(projects))
	plans := make(map[string]plan, len(projects))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, p := range projects {
		wg.Go(func() {
			found := subProjects(p.Path)
			ts, pl := tasksOf(p.Path), readPlan(p.Path)
			mu.Lock()
			if len(found) > 0 {
				subs[p.Path] = found
			}
			tasks[p.Path], plans[p.Path] = ts, pl
			mu.Unlock()
		})
	}
	wg.Wait()
	return projectsMsg{projects: projects, groups: groups, subs: subs, roots: cfg.roots(), tasks: tasks, plans: plans}
}

// scanProcs reads the working directory of every visible process.
func scanProcs() tea.Msg {
	procs, err := runningProcs()
	if err != nil {
		return procsMsg{err: fmt.Errorf("processes: %w", err)}
	}
	return procsMsg{procs: procs}
}

// merge remakes the tree's input from the last process scan and the last
// word from docker: the containers filed under the compose that runs
// them, or under the place. Copies, since filing writes the parent.
func (m *model) merge() {
	host := append([]Proc{}, m.host...)
	cs := append([]Proc{}, m.containers...)
	m.procs = attachContainers(host, cs)
}

// scanNow asks for a process scan on behalf of something that just happened —
// a shell opened, the view narrowed. A scan already out began before the
// event, so its answer cannot carry it; another is owed as soon as it lands.
func (m *model) scanNow() tea.Cmd {
	if m.scanning {
		m.rescan = true
		return nil
	}
	m.scanning = true
	return scanProcs
}

// scanPoll is the tick's ask: freshness only, so a scan already out is answer
// enough and nothing is owed.
func (m *model) scanPoll() tea.Cmd {
	if m.scanning {
		return nil
	}
	return m.scanNow()
}

// Update handles one message and then tells the status line what the
// navigator now has to say, whatever the message was: nearly anything can
// change it — a key, a report, a scan — and it is written once, from here,
// only when it changed.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	next.dressStatus()
	if read := next.readEndings(); read != nil {
		cmd = tea.Batch(cmd, read)
	}
	return next, cmd
}

func (m model) update(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.InterruptMsg:
		// A signal is q: the client detaches and the shells stay held.
		m.server.leave()
		return m, nil

	case tea.BlurMsg:
		// The keys have gone to a buffer. What was pending here was about
		// the next key, and the next key is not coming: a kill left armed
		// would fire on the first letter typed back into the list. A
		// preview gives the window back.
		if m.pendingKill != nil {
			m.pendingKill = nil
			m.showAgain()
		}
		m.pendingReplace, m.pendingG = false, false
		// To the buffer under the tabline, when there is one: conn took
		// them to any other itself, and said so then.
		if m.shown != 0 {
			m.keysTo(m.shown)
		}

	case tea.FocusMsg:
		// The keys are here. The buffer under the tabline stays: a dead
		// one answers to these keys, and a live one is a click away.
		m.keysTo(0)

	case tea.WindowSizeMsg:
		m.width, m.windowRows = msg.Width, msg.Height
		m.keepRows()
		m.scrollToCursor()

	case projectsMsg:
		m.projects, m.groups, m.subs, m.err = msg.projects, msg.groups, msg.subs, msg.err
		if msg.tasks != nil {
			m.tasks, m.plans = msg.tasks, msg.plans
		}
		m.manifestDirs = nil
		if msg.roots != nil {
			m.roots = msg.roots
		}
		m.rebuild()
		return m, tea.Batch(m.restsCmd(), m.publish())

	case restsMsg:
		m.rests = msg.rests
		m.rebuild()
		return m, m.publish()

	case updateMsg:
		m.learnUpdate(msg)
		return m, nil

	case procsMsg:
		m.scanning = false
		var owed tea.Cmd
		if m.rescan {
			m.rescan = false
			owed = m.scanNow()
		}
		if msg.err != nil {
			// A failed scan says nothing about what is running, so the last
			// list that succeeded stands: blanking the tree on every hiccup
			// of a loaded machine would be flicker, not information. The
			// failure is reported rather than shown as an empty machine.
			m.status, m.statusErr = msg.err.Error(), true
		} else {
			m.host = msg.procs
			m.merge()
		}
		m.rebuild()
		return m, tea.Batch(owed, m.publish(), m.deepCmd())

	case dockerReadyMsg:
		m.docker = msg.feed
		return m, nextDocker(m.docker)

	case dockerMsg:
		// Docker's word, merged into the tree with the last process scan;
		// that it has stopped answering is said once, until it answers.
		m.containers = msg.containers
		if msg.stalled && !m.dockerStalled {
			m.status, m.statusErr = "docker is not answering; its containers are as last seen", true
		}
		m.dockerStalled = msg.stalled
		m.merge()
		m.rebuild()
		return m, tea.Batch(nextDocker(m.docker), m.publish())

	case reconnectMsg:
		return m, connectServer()

	case serverReadyMsg:
		if msg.err != nil {
			// Not reaching the server is not the end of it: nothing but a retry
			// will ever turn this window back into a useful one.
			m.serverErr = msg.err.Error()
			return m, m.retryConnect()
		}
		m.server, m.serverErr = msg.session, ""
		// A fresh connection knows nothing of the arrangement the last one
		// made; the server says what it holds, and the pane follows from
		// there.
		m.shown, m.synced = 0, false
		m.said = statusText{}
		// Ask what is already running: shells from a window that has since
		// been closed are still there, and this is where they come back.
		m.server.list()
		return m, nextEvent(m.server)

	case recordedMsg:
		// A run that could not be written is a pane a little poorer,
		// and worth a word; the exit itself stands regardless. One
		// written is marked so on its pane, for the next navigator.
		if msg.err != nil {
			m.status, m.statusErr = "could not record the run: "+msg.err.Error(), true
		} else {
			m.server.noteRecorded(msg.pid)
		}
		return m, nil

	case serverErrorMsg:
		// One ask failed; the server and its shells are fine. Say what it said
		// and carry on listening.
		m.status, m.statusErr = msg.err.Error(), true
		return m, nextEvent(m.server)

	case serverLostMsg:
		// The server hung this window up — the last shell closed and the
		// session went with it, or something ended the server outright. The
		// session keeps watching for a new one on its own; here the window
		// only stops showing shells that are no longer held.
		m.terms = map[int]*remoteTerm{}
		m.dressed = map[int]string{}
		m.rebuild()
		return m, nextEvent(m.server)

	case termOpenedMsg:
		if t, ok := m.terms[msg.pid]; ok {
			t.learn(msg.dir, msg.name, msg.run)
		} else {
			m.terms[msg.pid] = &remoteTerm{pid: msg.pid, dir: msg.dir, name: msg.name, run: msg.run}
		}
		// A shell asked for by name is one of several a project needed, and
		// none of them is more the one you meant than the others. Only a shell
		// opened on its own is shown, focused, with the cursor on it.
		if msg.name == "" {
			m.showPID(msg.pid)
		}
		return m, tea.Batch(nextEvent(m.server), m.scanNow())

	case sessionsMsg:
		// The server is talking, so whatever chase was on is over.
		m.backoff = 0
		// The server is the authority on what it holds, so the client takes
		// the list rather than merging into what it thought it knew.
		held := make(map[int]*remoteTerm, len(msg.sessions))
		beside, wanted := 0, 0
		for _, s := range msg.sessions {
			if s.Shown {
				beside = s.PID
			}
			if s.Wanted {
				wanted = s.PID
			}
			if was, ok := m.terms[s.PID]; ok {
				was.learn(s.Dir, s.Name, s.Run)
				m.learnExit(was, s.Exit, s.Ended, s.Summary, s.Recorded)
				held[s.PID] = was
				continue
			}
			t := &remoteTerm{pid: s.PID, dir: s.Dir, name: s.Name, run: s.Run}
			m.learnExit(t, s.Exit, s.Ended, s.Summary, s.Recorded)
			held[s.PID] = t
		}
		m.terms = held
		// A navigator starting under a buffer already shown — the last
		// navigator closed, or the server was found holding shells — keeps
		// it shown: the session restores as it was left, and the cursor
		// begins on its row. And the server is the authority on what is
		// under the tabline once the shell shown there has gone: the same
		// pane with a new shell in it, after a rerun in place, is the
		// buffer still, and the cursor follows it; nothing there is the
		// window back to conn.
		if !m.synced || m.shownGone {
			m.synced, m.shownGone = true, false
			m.shown = beside
			m.keepRows()
			if beside != 0 {
				m.wantCursor = beside
			}
		}
		m.rebuild()
		// A shell a chord opened to be shown is shown: it gets focus and
		// the cursor follows.
		if wanted != 0 {
			m.showPID(wanted)
		}
		// The endings the list carried are the finder's news too.
		return m, tea.Batch(nextEvent(m.server), m.scanNow(), m.publish())

	case termGoneMsg:
		delete(m.terms, msg.pid)
		delete(m.dressed, msg.pid)
		if m.shown == msg.pid {
			// The list that says so follows at once, and says what is
			// under the tabline now: the same pane with a new shell in
			// it, or nothing, which is the window back to conn.
			m.shownGone = true
		}
		if m.from == msg.pid {
			m.from = 0
		}
		m.rebuild()
		// Asking again is what notices a server that has just become
		// replaceable: the shell keeping an out-of-date one alive was this.
		m.server.list()
		return m, tea.Batch(nextEvent(m.server), m.scanNow())

	case agentTickMsg:
		return m, tea.Batch(scanAgents, agentTick())

	case agentsMsg:
		m.agents = msg.agents
		// An instance at work again has a turn nobody has seen yet; one
		// that finishes in the buffer with focus is being looked at.
		for pid, a := range m.agents {
			if a.working() {
				delete(m.seen, pid)
			} else if t := m.owningTerm(pid); t != nil && t.pid == m.focus && m.focus != 0 {
				m.seen[pid] = true
			}
		}
		// The marks changed without the tree changing; the windows show
		// the new ones.
		m.dressWindows()
		var cmds []tea.Cmd
		// An instance gone is a conversation at rest, and one come is a
		// conversation no longer at rest: the rests are listed again.
		if live := m.liveFingerprint(); live != m.restLive {
			m.restLive = live
			cmds = append(cmds, m.restsCmd())
		}
		// An instance that has started working sets the markers turning.
		if !m.spinning && m.spinNeeded() {
			m.spinning = true
			cmds = append(cmds, spin())
		}
		return m, tea.Batch(cmds...)

	case composeMsg:
		// Only a refusal is news: what was started was said when it was
		// asked for, and the rows say when it is up.
		if msg.err != nil {
			m.status, m.statusErr = "could not start "+strings.Join(msg.services, ", ")+" in "+msg.place.Name+": "+msg.err.Error(), true
		}
		m.docker.ask()
		return m, m.scanNow()

	case deepMsg:
		m.deep[msg.pid] = msg.facts

	case outcomeMsg:
		// What the run's transcript said, for a shell whose ending still
		// stands: one used by hand since has nothing to say of it. The
		// ending is complete now, and goes on the record.
		if t := m.terms[msg.pid]; t != nil && !t.live() {
			t.summary = msg.summary
			m.server.noteOutcome(msg.pid, msg.summary)
			return m, m.record(t)
		}

	case tickMsg:
		m.ticks++
		cmds := []tea.Cmd{m.scanPoll(), tick(procPoll)}
		if m.ticks%projectEvery == 0 {
			cmds = append(cmds, scanProjects)
		}
		if m.ticks%updateTicks == 0 {
			cmds = append(cmds, checkUpdate(false, time.Now()))
		}
		// Keep the shown buffer's heading current too.
		cmds = append(cmds, m.deepCmd())
		return m, tea.Batch(cmds...)

	case killedMsg:
		m.pendingKill = nil
		m.showAgain()
		signalled := 0
		for _, r := range msg.results {
			if errors.Is(r.err, errGone) {
				// Not there to signal; gone is what was asked for.
				// Nothing to mark: there is no row left to mark it on.
				signalled++
				continue
			}
			if r.err != nil {
				continue
			}
			signalled++
			m.dying[r.pid] = dyingProc{command: r.command}
			delete(m.refused, r.pid)
		}
		if signalled == 0 {
			m.status, m.statusErr = "could not kill "+msg.subject+": "+describeFailures(msg.results), true
			return m, nil
		}

		m.status, m.statusErr = ended(msg.results, msg.sig)+msg.subject, false
		if failed := len(msg.results) - signalled; failed > 0 {
			// Part of a subtree going unsignalled is worth saying: the rest
			// spins down and the survivors just sit there unexplained.
			m.status += " — " + strconv.Itoa(failed) + " could not be killed: " + describeFailures(msg.results)
			m.statusErr = true
		}
		if m.spinning {
			return m, nil
		}
		m.spinning = true
		return m, spin()

	case spinMsg:
		m.frame++
		m.ageDying()
		if !m.spinNeeded() {
			m.spinning = false
			return m, nil
		}
		cmds := []tea.Cmd{spin()}
		// Only a kill needs the process list chased; a turning marker is
		// about a session file that the ordinary refresh already re-reads.
		if len(m.dying) > 0 && m.frame%rescanFrames == 0 {
			cmds = append(cmds, m.scanPoll())
		}
		return m, tea.Batch(cmds...)

	case tea.PasteMsg:
		// Into the filter it is just more of the query.
		if m.typing {
			m.status = ""
			m.setFilter(m.filter + msg.Content)
			return m, nil
		}
		// At the navigator, pasted text lands nowhere — and saying so beats
		// a paste that silently vanishes and reads as broken.
		m.status, m.statusErr = "nothing here to paste into", false
		return m, nil

	case tea.KeyPressMsg:
		return m.keyPress(msg)
	}
	return m, nil
}

// keyPress is a key at conn: the one place every key is read, and the
// order they are read in is the order of the claims on them.
func (m model) keyPress(msg tea.KeyPressMsg) (model, tea.Cmd) {
	// A kill preview takes the next key, whatever it is — before even the
	// prefix, or the preview lies: routed later, a prefix excursion could
	// carry the armed kill along for minutes and hand it to an enter meant
	// to open something. X takes the tree, x the head alone, and any other
	// key keeps everything alive and gives the buffer back.
	if m.pendingKill != nil {
		req := m.pendingKill
		m.pendingKill = nil
		key := msg.String()
		if key == "x" && len(req.head) > 0 {
			req = req.headOnly()
		}
		if sig, ok := chooseSignal(key, req.sig); ok {
			req.sig = sig
			return m, m.runKill(req)
		}
		m.showAgain()
		m.status, m.statusErr = "kept "+req.subject, false
		return m, nil
	}

	// The filter takes every key while it is being typed, so a repository
	// called "scratch" can be typed without s opening a shell halfway through.
	if m.typing {
		return m, m.filterKey(msg)
	}

	// Ending the server ends the work it is holding, so it takes a
	// second key like any other kill.
	if m.pendingReplace {
		m.pendingReplace = false
		switch msg.String() {
		case "R", "y", "enter":
			// The session notices the server going and says so; clearing
			// here as well just spares the window a beat of stale rows.
			m.server.replace()
			m.terms = map[int]*remoteTerm{}
			m.status, m.statusErr = "ending the server and its buffers", false
			m.rebuild()
			return m, nil
		}
		m.status, m.statusErr = "left the server alone", false
		return m, nil
	}

	// gg is a pair, so the first g waits for the second; gf is the other
	// pair. Anything else cancels it and is swallowed, rather than being
	// acted on as though the g had not been typed.
	if m.pendingG {
		m.pendingG = false
		switch msg.String() {
		case "g":
			return m, m.jump(0)
		case "f":
			return m, m.openFileLine()
		}
		return m, nil
	}

	m.status = ""
	switch msg.String() {
	case "?":
		// The keys, in a popup over the whole window: tmux draws it,
		// and the next keystroke puts it away.
		m.server.help()
		return m, nil
	case "R":
		return m, m.askReplace()
	case "U":
		return m, m.updateConn()
	case "p", "ctrl+p":
		// The front door: everything openable, in a popup over the window.
		m.server.finder()
		return m, nil
	case "/":
		// / narrows the everything view, opening it first when it is not
		// up: looking for a name across the machine is the finder's, on p,
		// and / is the look within the view.
		if !m.viewingAll() {
			m.openAll()
		}
		return m, m.openFilter()
	case "enter":
		return m, m.openShell()
	case "r":
		// On a dead task buffer, r runs it again in place; elsewhere it
		// starts what the cursor's place says it needs.
		if t := m.terms[m.shown]; t != nil && !t.live() && t.name != "" {
			return m, m.rerun(t)
		}
		return m, m.run()
	case "s":
		return m, m.start("")
	case "a":
		// An agent conn owns, so it survives the window and can be
		// stepped back into, unlike the ones it can only watch. Which
		// kind is the window's call, then the config's; claude is the
		// default.
		return m, m.start(m.agentCommand())
	case ",":
		// The next kind for a to start — claude, ollama, claude — for
		// the whole server: the chords start the same kind, and the
		// finder names it.
		m.cycleKind()
		return m, nil
	case "e":
		return m, m.showEnvironment()
	case "x":
		return m, m.askKill(false)
	case "X":
		return m, m.askKill(true)
	case "g":
		m.pendingG = true
		return m, nil
	case "G":
		return m, m.jump(len(m.rows) - 1)
	case "down", "j":
		return m, m.move(1)
	case "up", "k":
		return m, m.move(-1)
	case "t", "b", "l":
		return m, m.runVerb(msg.String())
	case "J":
		return m, m.stepShell(1)
	case "K":
		return m, m.stepShell(-1)
	case "tab":
		// The next thing owed — an agent waiting on you, a run that
		// ended badly — and again around them in turn: the jump the
		// chord ctrl-space enter delivers from any buffer.
		return m, m.jumpWaiting()
	case "shift+tab":
		// Back to the previous buffer: the chord ctrl-space ctrl-space,
		// from any buffer.
		return m, m.back()
	case "space":
		m.toggleCollapse()
		return m, nil
	case "-":
		m.unfolded = !m.unfolded
		m.rebuild()
		return m, nil
	case ".":
		// Dot opens the everything view; there, it shows and hides the
		// hidden.
		if !m.viewingAll() {
			m.openAll()
			return m, nil
		}
		m.showAll = !m.showAll
		m.rebuild()
		if !m.showAll {
			// Narrowing is a question about right now, so ask again.
			return m, m.scanNow()
		}
		return m, nil
	case "esc":
		// Esc closes whatever is open — the filter, the everything view —
		// and it never closes conn. Leaving is q's word alone: one
		// reflexive esc too many, a beat after the filter it was meant
		// for has already gone, must not take the window with it.
		switch {
		case m.filter != "":
			m.setFilter("")
		case m.all:
			m.closeAll()
		}
		return m, nil
	case "q", "ctrl+c":
		// On a dead buffer, q closes it: the run is over, and its history
		// is on the record. Anywhere else q leaves the window, not the
		// buffers: the client detaches and conn keeps its place in the
		// home window for the next `conn`.
		if t := m.terms[m.shown]; t != nil && !t.live() && m.focus == 0 {
			m.closeBuffer(t)
			return m, nil
		}
		m.server.leave()
		return m, nil
	}
	return m, nil
}

// filterKey handles a keystroke while something is being looked up.
//
// Looking something up is a way of getting somewhere, so the keys that get
// you somewhere work while you are still typing: the list is narrowing under
// a cursor you can move, and enter, ctrl+r, ctrl+a or ctrl+x acts on
// whatever that cursor is on — a place, or a process that answered. Having
// to accept the filter first made finding a thing and doing something with
// it two separate acts, when it is one.
func (m *model) filterKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		// Enter is the one key that means both things. On a repository or a
		// sub-project it opens a shell there; on a process it steps into the
		// shell holding it. The filter is finished either way.
		m.typing = false
		if _, ok := m.selected(); ok {
			return m.openShell()
		}
		return nil

	// Moving through what is left, without leaving the typing.
	case "up", "ctrl+p":
		return m.move(-1)
	case "down", "ctrl+n":
		return m.move(1)

	case "esc":
		// Abandoning the search is not acting on anything, so it puts the
		// cursor back on the row it left.
		m.typing = false
		m.setFilter("")
		m.selectKey(m.filterFrom)
		return nil
	case "tab":
		// The jump reaches through the search: the waiting agent lives in
		// the whole list, and going to it is the end of looking.
		return m.jumpWaiting()
	case "shift+tab":
		// Going back is too: the chord reaches the list while a look
		// is still typed, and the key means what it means.
		return m.back()

	// The chords mean what their letters mean. Starting what a project needs
	// is the end of looking for it, so the search closes and the cursor goes
	// to the first thing started — the thing you would want to be watching
	// come up, and where the keys mean what they usually mean again.
	case "ctrl+r":
		// Starting what a project needs is the end of looking for it, so the
		// typing stops. The filter itself is held until the processes land,
		// the same way it is for a shell: dropping it now would take the
		// project out of the narrowed list until the scan caught up, and the
		// cursor with it.
		m.typing = false
		return m.run()
	case "ctrl+a":
		// The end of looking, the same as ctrl+r: left set, the typing
		// outlives the filter and quietly takes focus back the moment
		// the shell it opened is gone.
		m.typing = false
		return m.start(m.agentCommand())
	case "ctrl+x":
		// Killing what you found is also the end of looking for it. The
		// typing stops so the confirmation's key is a confirmation, and the
		// filter holds so the subject stays where the cursor has it.
		m.typing = false
		return m.askKill(false)
	}

	// Every other key edits the line: a letter is a letter — a project called
	// "scratch" has to be typeable without s doing something; the actions are
	// on the chords, which no name contains — and the editing keys are the
	// ones every line has.
	before := m.query.Value()
	m.query, _ = m.query.Update(msg)
	if v := m.query.Value(); v != before {
		m.status = "" // whatever was reported was about the last project
		m.setFilter(v)
	}
	return nil
}

// newLine is a line to type a query on, bare: no prompt, no placeholder,
// the status line draws it. It is focused for good, since only the keys
// meant for it reach it.
func newLine() textinput.Model {
	l := textinput.New()
	l.Prompt = ""
	l.Focus()
	return l
}

// lineText is a line as the status line shows it: the text with the
// cursor's block where the cursor is.
func lineText(l textinput.Model) string {
	r := []rune(l.Value())
	pos := min(max(l.Position(), 0), len(r))
	return string(r[:pos]) + "█" + string(r[pos:])
}

// selectKey puts the cursor back on a remembered subject, where it is still
// listed. A subject that has gone leaves the cursor where the rebuild put
// it, which held the position rather than jumping to the top.
func (m *model) selectKey(key string) {
	if key == "" {
		return
	}
	for i, r := range m.rows {
		if detailKey(r) == key {
			m.cursor = i
			m.scrollToCursor()
			return
		}
	}
}

// selectProject puts the cursor on a repository, wherever it has ended up in
// the list. It is how an action that closes the search leaves you looking at
// what you acted on rather than back at the top of everything.
func (m *model) selectProject(path string) {
	for i, r := range m.rows {
		if r.kind != rowProc && r.project.Path == path {
			m.cursor = i
			m.scrollToCursor()
			return
		}
	}
}

// putFilter sets the filter and the line it is typed on, without rebuilding:
// the line is the truth while it has focus, and follows the filter
// when something else set it — a paste, the filter clearing on its own.
func (m *model) putFilter(s string) {
	m.filter = s
	if m.query.Value() != s {
		m.query.SetValue(s)
	}
}

// setFilter narrows the list and starts again from the top, because the rows
// under the cursor are not the ones that were there a keystroke ago.
//
// Unless they are. A filter is trimmed and folded before anything is matched
// against it, so a space does not narrow anything — and typing one in the
// middle of "vim pro" sent the selection back to the top of a list that had
// not moved, which from the typist's side is the cursor jumping for no reason
// at all. When the rows cannot have changed, neither does the cursor.
func (m *model) setFilter(s string) {
	narrowed := !strings.EqualFold(strings.TrimSpace(m.filter), strings.TrimSpace(s))

	m.putFilter(s)
	// rebuild keeps the cursor on the subject it was on where that subject is
	// still listed, which is the whole of what is wanted when nothing changed.
	m.rebuild()
	if narrowed {
		m.cursor = m.firstAnswer()
	}
	m.scrollToCursor()
}

// firstAnswer is the row the narrowed cursor should land on: the first one
// that answers the filter by what it itself is — its name, its path, its
// command — rather than by standing above the answer. A repository listed
// for its child's sake is scaffolding, and the search should land on what was
// found.
func (m model) firstAnswer() int {
	f := strings.ToLower(strings.TrimSpace(m.filter))
	if f == "" {
		return 0
	}
	for i, r := range m.rows {
		if m.rowAnswers(r, f) {
			return i
		}
	}
	return 0
}

// jump puts the cursor on a row and brings it into view. Out of range means
// the nearest end, so the top of an empty list is not a special case.
func (m *model) jump(i int) tea.Cmd {
	if len(m.rows) == 0 {
		return nil
	}
	m.letGo()
	switch {
	case i < 0:
		i = 0
	case i >= len(m.rows):
		i = len(m.rows) - 1
	}
	m.cursor = i
	m.scrollToCursor()
	return nil
}

// jumpWaiting goes to the next row that needs you — an agent waiting on
// its user, a command that ended badly, a process gone wrong — in row order
// from the cursor, wrapping. Going to one conn holds means taking the
// client to its window, where the prompt or the transcript is; one it can
// only watch gets the cursor instead, which is as far as enter could take
// it either.
func (m *model) jumpWaiting() tea.Cmd {
	// From the filter, the jump is the end of looking: the rows while typing
	// are the query's answers — places alone until a query lands — and what
	// needs you lives in the whole list.
	if m.typing {
		m.typing = false
		m.setFilter("")
	}
	m.letGo()
	for step := 1; step <= len(m.rows); step++ {
		i := (m.cursor + step) % len(m.rows)
		r := m.rows[i]
		if !m.needsYou(r) {
			continue
		}
		if t := m.owningTerm(r.node.PID); t != nil {
			m.show(t)
			return nil
		}
		m.cursor = i
		m.scrollToCursor()
		return nil
	}
	m.status, m.statusErr = "nothing needs you", false
	return nil
}

// show puts a buffer in the pane under the tabline from anywhere: the
// cursor goes to its row, a filter that led here is finished, like enter's,
// the everything view gives way, and the buffer has focus — unless its run
// has ended. A dead buffer is readable beneath and answers to conn's keys —
// r reruns, gf opens the file, q closes — so the keys stay with conn, and
// enter steps into the shell when that is wanted.
func (m *model) show(t *remoteTerm) {
	m.typing = false
	m.setFilter("")
	m.all = false
	m.from = 0
	m.lookAt(t)
	for i, r := range m.rows {
		if r.kind == rowProc && m.owningTerm(r.node.PID) == t {
			m.cursor = i
			m.scrollToCursor()
			break
		}
	}
	m.shown = t.pid
	m.keepRows()
	if t.live() {
		m.keysTo(t.pid)
		m.server.show(t.pid)
		return
	}
	m.keysTo(0)
	m.server.showQuiet(t.pid)
}

// lookAt marks the finished turns of the agents in a buffer as seen: the
// buffer is being shown, and what it had to show has been looked at.
func (m *model) lookAt(t *remoteTerm) {
	for pid, a := range m.agents {
		if !a.working() && m.owningTerm(pid) == t {
			m.seen[pid] = true
		}
	}
}

// owed is the agent a row is running when it is owed a look: blocked on
// an ask, whatever has been seen, or done with a turn nobody has looked
// at yet. A finished turn already looked at is quiet.
func (m model) owed(r navRow) agent {
	a := m.awaiting(r)
	if a == nil {
		return nil
	}
	if _, blocked := a.blocked(); blocked || !m.seen[r.node.PID] {
		return a
	}
	return nil
}

// park gives the shown buffer a window of its own and conn the whole
// window: conn has focus, and something of its own to draw there.
func (m *model) park() {
	if m.shown == 0 {
		return
	}
	m.shown = 0
	m.keepRows()
	m.server.park()
	m.dressWindows()
}

// take takes the window for a view of conn's own — the everything view, a
// kill preview — remembering the buffer it took it from, for the view to
// give back.
func (m *model) take() {
	if m.shown != 0 {
		m.from = m.shown
		m.park()
	}
	m.keysTo(0)
	m.server.home()
}

// showAgain gives the window back to the buffer a view took it from, where
// that buffer is still held; with none, conn keeps the window.
func (m *model) showAgain() {
	t := m.terms[m.from]
	m.from = 0
	if t != nil && m.shown == 0 && m.pendingKill == nil {
		m.show(t)
	}
}

// openAll puts the everything view up: conn takes the window, and the list
// draws under the tabline.
func (m *model) openAll() {
	m.all = true
	m.take()
}

// closeAll takes the everything view down, and gives the window back to
// the buffer it took it from.
func (m *model) closeAll() {
	m.all = false
	m.showAgain()
}

// keysTo records that focus has gone to pid — zero for conn — keeping
// where it was, when that is somewhere else. Where it already is is not
// a move: conn's own blur after it sent focus somewhere says the same
// thing twice. And conn on the way from one buffer to another is not a
// place focus was: choosing a second buffer leaves the first as where
// focus was, so back from the second is the first, not the view it was
// chosen from. Back to the same buffer from conn is a round trip, and
// conn is where focus was.
func (m *model) keysTo(pid int) {
	switch {
	case pid == m.focus:
	case m.focus == 0 && m.was != 0 && m.was != pid:
		m.focus = pid
	default:
		m.was, m.focus = m.focus, pid
	}
}

// back returns focus to the previous pane: the buffer it was in before
// this one, or conn, whichever it was — the chord ctrl-space ctrl-space,
// from anywhere. A buffer that has gone since is no place to go; then,
// back from a buffer is the everything view, and back from conn is enter:
// the buffer under the cursor, when the row has one. From conn with none
// there is nowhere, which is said.
func (m *model) back() tea.Cmd {
	target := m.was
	if target != 0 && m.terms[target] == nil {
		target = 0
	}
	if target == 0 && m.focus == 0 {
		if t := m.cursorTerm(); t != nil {
			target = t.pid
		}
	}
	if target == 0 {
		if m.focus == 0 {
			m.status, m.statusErr = "no shell to go back to", false
			return nil
		}
		m.openAll()
		return nil
	}
	m.letGo()
	m.show(m.terms[target])
	return nil
}

// keepRows holds conn to the chrome's rows while a buffer is shown under
// it. The pane's size reaches conn late, as a message behind the resize:
// a frame drawn between a buffer joining and that message is drawn at the
// whole window's height into a pane two rows tall, and tmux writes the
// overflow wrapped into it. conn knows when a buffer is shown, so it does
// not wait to be told it is short: it shortens itself as it asks for the
// buffer, and a size taller than the chrome while one is shown is a size
// from before the join.
func (m *model) keepRows() {
	m.height = m.windowRows
	if m.shown != 0 {
		m.height = min(m.height, chromeRows)
	}
}

// showPID shows a buffer that may not have a row yet — one just opened,
// before the process scan has seen it. The keys go to it now; the cursor
// follows as soon as its row lands.
func (m *model) showPID(pid int) {
	m.all = false
	m.from = 0
	m.shown = pid
	m.keepRows()
	m.wantCursor = pid
	m.keysTo(pid)
	m.server.show(pid)
}

// stepShell shows the next or previous buffer in the tabline's order from
// the shown one — or, from a view, the one under the cursor — wrapping,
// and gives it focus: the chord ctrl-space j and k, from any buffer, and J
// and K at conn.
func (m *model) stepShell(delta int) tea.Cmd {
	order := m.heldOrder()
	if len(order) == 0 {
		m.status, m.statusErr = "no buffer is open", false
		return nil
	}
	m.letGo()
	from := m.shown
	if from == 0 {
		from = m.from
	}
	if from == 0 {
		if t := m.cursorTerm(); t != nil {
			from = t.pid
		}
	}
	at := -1
	for i, pid := range order {
		if pid == from {
			at = i
		}
	}
	next := order[(at+delta+len(order))%len(order)]
	m.show(m.terms[next])
	return nil
}

// isSelf reports whether a process is conn itself: this build, by its
// path, or a tmux on conn's socket — the client the launcher becomes, the
// server, the navigator. A `go run .` in this repository is not conn by
// its own command line, but the tmux client it turns into is, and the row
// folds the two together.
func isSelf(p Proc) bool {
	if strings.Contains(p.Argv, socketPath()) {
		return true
	}
	exe, _, _ := strings.Cut(p.Argv, " ")
	return exe != "" && exe == connExe()
}

// selfRun reports whether a row is conn itself: the process, or any in the
// run folded into it.
func (m model) selfRun(r navRow) bool {
	if r.kind != rowProc {
		return false
	}
	if isSelf(r.node.Proc) {
		return true
	}
	for _, n := range r.run {
		if isSelf(n.Proc) {
			return true
		}
	}
	return false
}

// heldOrder is every held shell in the order the navigator lists them. It
// is the order J and K step through, and it does not depend on what is
// folded or filtered — a shell is still there when its row is not — so it
// is read off the list as it would be drawn with nothing filtered, folded
// or collapsed: groups and repositories by name, a place's shells by the
// name their rows show. Sorting the shells any other way — by place and
// pid, say — is a list that reads downward and a J that steps upward. A
// shell the list has no row for, one the scan has not seen yet, follows,
// by age.
func (m model) heldOrder() []int {
	whole := m
	whole.typing, whole.filter, whole.showAll, whole.collapsed = false, "", true, nil
	seen := map[int]bool{}
	var pids []int
	for _, r := range whole.flatten() {
		if r.kind != rowProc {
			continue
		}
		if t := m.owningTerm(r.node.PID); t != nil && !seen[t.pid] {
			seen[t.pid] = true
			pids = append(pids, t.pid)
		}
	}
	var rest []int
	for pid := range m.terms {
		if !seen[pid] {
			rest = append(rest, pid)
		}
	}
	slices.Sort(rest)
	return append(pids, rest...)
}

// openFilter starts typing a filter. The list becomes every project straight
// away, before a single character is typed: half of looking one up is
// remembering which ones there are.
func (m *model) openFilter() tea.Cmd {
	m.filterFrom = ""
	if r, ok := m.selected(); ok {
		m.filterFrom = detailKey(r)
	}
	m.typing = true
	m.rebuild()
	m.cursor = 0
	m.scrollToCursor()
	return nil
}

// placeAt is the place a directory belongs to — the innermost repository or
// sub-project holding it, or failing those the group whose own level it is
// at — the same attribution rebuild gives a process working there.
func (m model) placeAt(dir string) (Project, bool) {
	var best Project
	found := false
	for _, p := range m.projects {
		if under(dir, p.Path) && len(p.Path) > len(best.Path) {
			best, found = p, true
		}
	}
	if !found {
		for _, g := range m.groups {
			if under(dir, g.Path) && len(g.Path) > len(best.Path) {
				best, found = g, true
			}
		}
		return best, found
	}
	for _, sp := range m.subs[best.Path] {
		if under(dir, sp.Path) && len(sp.Path) > len(best.Path) {
			best = sp
		}
	}
	return best, true
}

// move steps the cursor, wrapping at both ends so the list cycles.
// learnExit takes how and when a shell's command ended from a report,
// once: a recorded ending never changes, and one the shell has since been
// used past is not taken back from a list that still carries it. Each
// ending is numbered as it is learned, so the latest of several is known
// even from a pane that recorded no time.
func (m *model) learnExit(t *remoteTerm, exit, ended, summary string, recorded bool) {
	if exit == "" || t.exit != "" || t.dropped {
		return
	}
	m.endings++
	t.exit, t.ended = exit, m.endings
	if secs, err := strconv.ParseInt(ended, 10, 64); err == nil && secs > 0 {
		t.at = time.Unix(secs, 0)
	}
	// What the pane already knows of the ending — the transcript's word,
	// and that it is on the record — is taken with it: a navigator
	// starting beside a settled shell is not the first to see it end.
	t.summary, t.recorded = summary, recorded
}

// noticeEnded asks the server again about a plan's shell the scan finds at
// its prompt with no exit recorded yet: its command has just ended, and
// the pane carries how. tmux announces nothing when a pane's option is
// set, so the scan that sees the shell alone is what prompts the asking —
// once per exit, the command running again being the reset.
//
// It also notices a shell used again by hand after its command ended: an
// ending seen with the shell at its prompt, and then something running in
// the shell. The ending is history then — whatever was run by hand was the
// answer to it — and is dropped, here and on the pane, so the row is a
// shell again and not a failure that will never clear.
func (m *model) noticeEnded() {
	for pid, t := range m.terms {
		n := m.nodes[pid]
		if n == nil {
			continue
		}
		busy := len(n.Children) > 0 || !isShell(n.Command)
		if !t.live() {
			switch {
			case busy && t.settled:
				t.exit, t.at, t.summary, t.settled, t.dropped = "", time.Time{}, "", false, true
				delete(m.unread, pid)
				m.server.forgetExit(pid)
			case !busy && !t.settled:
				// Settled: the exit stands, and the transcript has
				// its last word on the run — unless the pane already
				// carries it, on the record, from a navigator before.
				t.settled = true
				if !t.recorded {
					m.unread[pid] = true
				}
			}
			continue
		}
		if t.name == "" {
			continue
		}
		if busy {
			delete(m.askedExit, pid)
			continue
		}
		if !m.askedExit[pid] {
			m.askedExit[pid] = true
			m.server.list()
		}
	}
}

// outcomeMsg is what a settled shell's transcript said of its run.
type outcomeMsg struct {
	pid     int
	summary string
}

// readEndings reads, once each, the transcripts of the shells whose
// endings have settled since the last time: what the run said of itself,
// by the shape of its last lines, for the row and the pane to carry.
func (m *model) readEndings() tea.Cmd {
	if len(m.unread) == 0 || m.server == nil {
		return nil
	}
	var cmds []tea.Cmd
	for pid := range m.unread {
		srv := m.server
		cmds = append(cmds, func() tea.Msg {
			return outcomeMsg{pid: pid, summary: summarize(srv.tail(pid, transcriptLines))}
		})
	}
	m.unread = map[int]bool{}
	return tea.Batch(cmds...)
}

// letGo ends the hold a run puts on the cursor. The hold is for the scans
// between the key and the processes landing, so the cursor is on the
// project while they start and on the first of them when it does; a move
// in that gap is the cursor being wanted somewhere else, and a hold that
// snapped it back on the next rebuild read as the keys not working.
func (m *model) letGo() {
	m.wantProject, m.wantName, m.wantCursor = "", "", 0
}

func (m *model) move(delta int) tea.Cmd {
	if len(m.rows) == 0 {
		return nil
	}
	m.letGo()
	m.cursor = (m.cursor + delta + len(m.rows)) % len(m.rows)
	m.scrollToCursor()
	return nil
}

// cursorTerm is the shell belonging to the row under the cursor: the one
// enter gives focus.
//
// A folded run is rarely a shell itself — the row is named for what the shell
// started — so the run is walked for the shell conn holds in it. That shell's
// pane is where the thing the row is named for is drawing.
func (m model) cursorTerm() *remoteTerm {
	r, ok := m.selected()
	if !ok || r.kind != rowProc {
		return nil
	}
	if t := m.terms[r.node.PID]; t != nil {
		return t
	}
	for _, n := range r.run {
		if t := m.terms[n.PID]; t != nil {
			return t
		}
	}
	return nil
}

// attachable reports whether enter takes you to the row. A repository opens
// a shell there, and a process is reached through the shell conn holds
// around it — which reaches the process only when the shell is running it:
// the row that stands for the shell, or one the shell started. What those
// started in turn — the tool a claude is running, the shells it runs them
// in — enter lands beside, in the same shell as their parent, so their rows
// are drawn dim: somebody else's process, not offered and then refused.
func (m model) attachable(r navRow) bool {
	if r.kind != rowProc {
		return true
	}
	for _, n := range r.run {
		if _, ok := m.terms[n.PID]; ok {
			return true
		}
	}
	if len(r.run) == 0 {
		return false
	}
	_, ok := m.terms[m.parent[r.run[0].PID]]
	return ok
}

// owningTerm is the shell conn holds that a process is running inside: itself,
// or the nearest ancestor that is one.
//
// A claude started with c is a child of the shell that ran it, so entering the
// claude row means entering that shell — which is where the claude is drawing.
// The walk is bounded because a process table that says a process is its own
// ancestor should not hang the navigator.
func (m model) owningTerm(pid int) *remoteTerm {
	for i := 0; pid > 1 && i <= len(m.procs); i++ {
		if t, ok := m.terms[pid]; ok {
			return t
		}
		pid = m.parent[pid]
	}
	return nil
}

// newShell starts a shell wherever the cursor is, whatever the cursor is on.
//
// This is the one way to put a process into the tree, so it cannot be reserved
// for the rows that happen to be enterable: standing on a process conn does not
// own is a perfectly good reason to want a shell where that process is working.
// Attaching to a foreign process is impossible; opening a shell beside it is
// not, and the two are different questions.
func (m *model) start(command string) tea.Cmd {
	r, ok := m.selected()
	if !ok {
		return nil
	}
	if m.server == nil {
		m.status, m.statusErr = "no server to hold it: "+m.serverErr, true
		return nil
	}
	dir := m.shellDir(r)
	if dir == globalPlace {
		m.status, m.statusErr = "global is not a place to open a shell in", false
		return nil
	}
	m.server.open(dir, command, "")
	return nil
}

// agentCommand is what a starts: the current kind's command, asked of the
// server through this navigator's connection. With no server there is no
// choice on record, and the config's kind is the answer — which start will
// then not hold anyway, and say why.
func (m model) agentCommand() string {
	if m.server == nil {
		return startAgent(nil)
	}
	return startAgent(m.server.run)
}

// cycleKind moves a on to the next kind of agent, and says which. The
// server holds the choice, not this navigator: a chord from any shell
// starts the same kind, and it holds until the server goes.
func (m *model) cycleKind() {
	if m.server == nil {
		m.status, m.statusErr = "no server to tell: "+m.serverErr, true
		return
	}
	k := nextKind(currentKind(m.server.run))
	if err := chooseKind(m.server.run, k); err != nil {
		m.status, m.statusErr = err.Error(), true
		return
	}
	m.status, m.statusErr = "a starts "+k.name, false
}

// holdsDir reports a directory inside some project, group or root the
// navigator lists.
func (m model) holdsDir(dir string) bool {
	for _, p := range m.projects {
		if under(dir, p.Path) {
			return true
		}
	}
	for _, g := range m.groups {
		if under(dir, g.Path) {
			return true
		}
	}
	return false
}

// shellDir is where a new shell on this row should start: the repository, or
// the directory the selected process is actually working in — which for a
// build or a test run is often further in than the repository root.
func (m model) shellDir(r navRow) string {
	if r.kind == rowProc && r.node.Dir != "" {
		return r.node.Dir
	}
	return r.project.Path
}

// openShell opens a shell on a repository row, or takes the client to the
// window of one already open on the row under the cursor. Enter on a
// repository always opens another, so a repository can hold as many shells
// as the work needs.
func (m *model) openShell() tea.Cmd {
	r, ok := m.selected()
	if !ok {
		return nil
	}
	if r.kind == rowRest {
		return m.continueRest(r.rest)
	}

	if r.kind == rowProc {
		// A container is a place of its own to step into: a shell inside
		// it, opened by compose in the place, held like any shell.
		if c := r.node.Container; c != nil {
			if !c.running() {
				m.status, m.statusErr = c.Service+" is not running", false
				return nil
			}
			if m.server == nil {
				m.status, m.statusErr = "no server to hold it: "+m.serverErr, true
				return nil
			}
			m.server.open(r.node.Dir, containerShell(r.node), "")
			return nil
		}
		t := m.owningTerm(r.node.PID)
		if t == nil {
			// The row is already drawn dim to say so; this is the reminder for
			// pressing enter on it anyway, not a failure.
			m.status, m.statusErr = "conn did not start "+procLabel(r.node), false
			return nil
		}
		m.show(t)
		return nil
	}
	return m.start("")
}

// runKill carries out a confirmed kill, splitting it by who owns the target.
//
// A shell conn holds at its prompt is hung up through the server rather
// than signalled: an interactive shell ignores SIGTERM, so signalling one
// leaves it sitting there and conn reporting that it would not go.
// Everything else is somebody else's process, and a signal is all conn has.
func (m *model) runKill(req *killRequest) tea.Cmd {
	hungUp, signalled := m.splitKill(req.nodes)
	// docker says when a container has stopped; the feed is asked to ask.
	for _, n := range signalled {
		if n.Container != nil {
			m.docker.ask()
			break
		}
	}
	return killTree(&killRequest{subject: req.subject, nodes: signalled, sig: req.sig}, hungUp)
}

// splitKill sorts a kill's targets into the shells the server hangs up and
// the processes to signal, which take in what runs under a held shell. A
// held shell running a command keeps its pane: what runs in it is
// signalled, parents first, and the shell is back at its prompt with the
// ending recorded — the buffer stays as the record of the ending, dead but
// readable, until q closes it. A held shell at its prompt with nothing
// running is a buffer of nothing but a prompt, and is hung up. A process
// is signalled once, whichever way it was reached.
func (m *model) splitKill(nodes []*ProcNode) (hungUp []killResult, signalled []*ProcNode) {
	seen := map[int]bool{}
	for _, n := range nodes {
		if _, mine := m.terms[n.PID]; !mine {
			if !seen[n.PID] {
				seen[n.PID] = true
				signalled = append(signalled, n)
			}
			continue
		}
		shell := m.nodes[n.PID]
		if shell == nil || len(shell.Children) == 0 {
			hungUp = append(hungUp, killResult{command: n.Command, pid: n.PID, hungUp: true})
			m.server.closeTerm(n.PID)
			continue
		}
		for _, under := range subtree(shell)[1:] {
			if _, held := m.terms[under.PID]; held || seen[under.PID] {
				continue
			}
			seen[under.PID] = true
			signalled = append(signalled, under)
		}
	}
	return hungUp, signalled
}

// closeBuffer closes a dead buffer: its run is over and on the record,
// and the shell at its prompt beneath the transcript is hung up. The
// tabline moves on to the next buffer.
func (m *model) closeBuffer(t *remoteTerm) {
	m.server.closeTerm(t.pid)
	delete(m.terms, t.pid)
	delete(m.dressed, t.pid)
	m.shown = 0
	m.keepRows()
	m.rebuild()
	if len(m.terms) > 0 {
		m.stepShell(1)
	}
	m.status, m.statusErr = "closed "+t.name+"; its runs stay on the record", false
}

// rerun runs a dead task buffer again in place: the same pane, the same
// name, the command it ran, its transcript starting over. The ending is
// forgotten first, so the run is live until it ends again.
func (m *model) rerun(t *remoteTerm) tea.Cmd {
	if m.server == nil {
		m.status, m.statusErr = "no server to run it in: "+m.serverErr, true
		return nil
	}
	t.exit, t.at, t.summary, t.settled, t.dropped, t.recorded = "", time.Time{}, "", false, false, false
	delete(m.unread, t.pid)
	m.server.respawn(t.pid, t.run)
	m.status, m.statusErr = "running "+t.name+" again: "+t.run, false
	return m.scanNow()
}

// openFileLine is gf on a buffer: the last file:line its transcript names
// — a failing test, a compiler's complaint — opened in the editor, in the
// buffer's own shell, which is at its prompt beneath the transcript.
func (m *model) openFileLine() tea.Cmd {
	t := m.terms[m.shown]
	if t == nil || m.server == nil {
		m.status, m.statusErr = "no buffer to open a file from", false
		return nil
	}
	loc := lastFileLine(m.server.tail(t.pid, transcriptLines))
	if loc == "" {
		m.status, m.statusErr = "the transcript names no file:line", false
		return nil
	}
	m.server.typeInto(t.pid, editorCommand(loc))
	m.status, m.statusErr = "opening "+loc, false
	m.keysTo(t.pid)
	m.server.show(t.pid)
	return nil
}

// entryOf is the named shell a row is, or is running in — a plan's
// entry, a task — or nil for a row that is neither.
func (m model) entryOf(r navRow) *remoteTerm {
	t := m.owningTerm(r.node.PID)
	if t == nil || t.name == "" {
		return nil
	}
	return t
}

// shellAround is the shell conn holds that a process is running inside, if it
// is not that shell itself.
func (m model) shellAround(n *ProcNode) *ProcNode {
	t := m.owningTerm(n.PID)
	if t == nil || t.pid == n.PID {
		return nil
	}
	return m.nodes[t.pid]
}

// retryConnect schedules another attempt to reach the server, waiting twice
// as long as the last one up to a cap. The wait is reset by a server that
// talks, so a normal loss is recovered in well under a second and only a
// server that keeps failing is given room.
func (m *model) retryConnect() tea.Cmd {
	switch {
	case m.backoff <= 0:
		m.backoff = reconnectWait
	default:
		m.backoff *= 2
		if m.backoff > reconnectMax {
			m.backoff = reconnectMax
		}
	}
	return reconnect(m.backoff)
}

// askReplace arms R: ending the server outright, and the shells with it.
// There is no upgrade dance to gate it on any more — a tmux server never
// goes stale under a new build — so it is the blunt instrument, kept for
// the day something wedges, and it always asks first.
func (m *model) askReplace() tea.Cmd {
	if len(m.terms) == 0 && m.serverErr == "" {
		m.status, m.statusErr = "nothing is held; there is nothing to replace", false
		return nil
	}
	m.pendingReplace = true
	return nil
}

// run starts what the project the cursor is in says it needs, and is not
// already running. It is a list to run rather than a promise to keep, so
// running it again starts only what has since stopped.
func (m *model) run() tea.Cmd {
	r, ok := m.selected()
	if !ok {
		return nil
	}
	return m.runPlace(r.project)
}

// runPlace starts what one place's plan says it needs and is not running.
func (m *model) runPlace(p Project) tea.Cmd {
	if m.server == nil {
		m.status, m.statusErr = "no server to hold them: "+m.serverErr, true
		return nil
	}

	plan := readPlan(p.Path)
	if len(plan.Entries) == 0 {
		m.status, m.statusErr = p.Name+" does not say what it needs", true
		return nil
	}

	// The plan's entries, and the place's services where it runs compose.
	missing, services := needs(p.Path, plan, m.namesIn(p.Path, plan), m.procs)
	if len(missing) == 0 && len(services) == 0 {
		m.status, m.statusErr = "everything "+p.Name+" needs is running", false
		return nil
	}

	for _, e := range missing {
		m.server.open(p.Path, e.Run, e.Name)
	}
	if len(missing) > 0 {
		// The cursor stays on the project while they start, then goes to the
		// first of them: what was started is what there is to watch come up,
		// and the first is the one the plan put first.
		m.wantCursor, m.wantProject, m.wantName = 0, p.Path, missing[0].Name
	}
	// Started rather than entered: this is several things at once, and none of
	// them is more the one you meant than the others.
	m.status, m.statusErr = "started "+describeStarted(missing, services), false
	cmds := []tea.Cmd{m.scanNow()}
	if len(services) > 0 {
		cmds = append(cmds, bringBack(p, services))
	}
	return tea.Batch(cmds...)
}

// describeStarted names what r started: the entries, then the services
// compose was asked for.
func describeStarted(entries []entry, services []string) string {
	names := make([]string, 0, len(entries)+len(services))
	for _, e := range entries {
		names = append(names, e.Name)
	}
	return strings.Join(append(names, services...), ", ")
}

// runVerb runs a task of the place the cursor is in, the way the place
// says it runs: the keys t, b and l, each the key of its verb.
func (m *model) runVerb(key string) tea.Cmd {
	r, ok := m.selected()
	if !ok {
		return nil
	}
	for _, v := range verbs {
		if v.key == key {
			return m.doVerb(r.project, v)
		}
	}
	return nil
}

// doVerb runs one place's task in a shell named for it, wrapped like a
// plan entry's so how it ends is recorded. A task is redone, not kept
// beside itself: the last run's shell, at its prompt, is closed for the
// new one. A run still going is left to finish.
func (m *model) doVerb(p Project, v *verb) tea.Cmd {
	if m.server == nil {
		m.status, m.statusErr = "no server to hold it: "+m.serverErr, true
		return nil
	}
	run, _, ok := v.command(p.Path)
	if !ok {
		m.status, m.statusErr = p.Name+" does not say "+v.unknown, true
		return nil
	}
	for _, t := range m.planned(p.Path) {
		if t.name != v.name {
			continue
		}
		if m.busy(t) {
			m.status, m.statusErr = "already "+v.doing+" "+p.Name, false
			return nil
		}
		m.server.closeTerm(t.pid)
	}
	m.server.open(p.Path, run, v.name)
	m.wantCursor, m.wantProject, m.wantName = 0, p.Path, v.name
	m.status, m.statusErr = v.doing+" "+p.Name+": "+run, false
	return m.scanNow()
}

// planned are the shells in a project that a plan started, which are the ones
// carrying the name the plan gave them.
func (m model) planned(path string) []*remoteTerm {
	var out []*remoteTerm
	for _, t := range m.terms {
		if t.name != "" && t.dir == path {
			out = append(out, t)
		}
	}
	slices.SortFunc(out, func(a, b *remoteTerm) int { return cmp.Compare(a.name, b.name) })
	return out
}

// namesIn is what a project already has running, by the names its plan
// uses: the shells started for its entries whose commands have not ended,
// and the entries whose command some process here is running, whoever
// started it. A shell whose command has ended is at its prompt with its
// transcript, not running the entry, and r starts the entry again beside
// it.
func (m model) namesIn(path string, plan plan) map[string]bool {
	running := map[string]bool{}
	for _, t := range m.planned(path) {
		if m.busy(t) {
			running[t.name] = true
		}
	}
	// And the entries whose command is running here by any other hand:
	// r does not start a second dev beside one already up.
	for _, e := range plan.Entries {
		if !running[e.Name] && m.entryRunning(path, e.Run) != nil {
			running[e.Name] = true
		}
	}
	return running
}

// busy reports a held shell still running the command it was started
// with: no ending learned, and not seen at its prompt by the scan. A shell
// at its prompt with no ending — the recording missed, or an older server's
// shell — is not running an entry, whatever it has not said.
func (m model) busy(t *remoteTerm) bool {
	if !t.live() {
		return false
	}
	n := m.nodes[t.pid]
	return n == nil || len(n.Children) > 0 || !isShell(n.Command)
}

// record writes a named shell's ending to the runs file, once: what it
// ran, what the plan called it, how it ended, what it said, when, and how
// long it ran, from when its shell began. A shell opened by hand is
// nobody's run.
func (m *model) record(t *remoteTerm) tea.Cmd {
	if t.name == "" || t.exit == "" || t.recorded {
		return nil
	}
	t.recorded = true
	r := run{Dir: t.dir, Name: t.name, Command: t.run, Exit: t.exit, Summary: t.summary, At: t.at}
	if r.At.IsZero() {
		r.At = time.Now()
	}
	if began := m.startedAt(t.pid); !began.IsZero() && r.At.After(began) {
		r.Took = r.At.Sub(began).Seconds()
	}
	pid := t.pid
	return func() tea.Msg {
		if err := recordRun(r); err != nil {
			return recordedMsg{pid: pid, err: err}
		}
		return recordedMsg{pid: pid}
	}
}

// recordedMsg says a run went on the record, or why it did not.
type recordedMsg struct {
	pid int
	err error
}

// entryStates is what a place's plan entries are doing, by name: up for
// one whose shell is running its command, the exit status and moment for
// one whose command ended, and nothing for one with no shell. An entry
// started again beside a shell left at its prompt is up; of several
// shells that ended, the latest ending speaks for the entry.
func (m model) entryStates(path string) map[string]entryState {
	states := map[string]entryState{}
	latest := map[string]int{}
	for _, t := range m.planned(path) {
		switch {
		case m.busy(t):
			states[t.name] = entryState{State: "up", Ports: m.portsUnder(t.pid)}
		case states[t.name].State != "up" && t.ended >= latest[t.name]:
			states[t.name], latest[t.name] = entryState{State: t.exit, At: t.at, Summary: t.summary}, t.ended
		}
	}
	// An entry is up when its command is running here, whoever started it:
	// the shell conn opened for it, or a shell opened by hand, or a
	// terminal outside conn. Whether dev is up is a fact about the
	// processes in the place, not about the labels on conn's shells.
	for _, e := range readPlan(path).Entries {
		if states[e.Name].State == "up" {
			continue
		}
		if n := m.entryRunning(path, e.Run); n != nil {
			states[e.Name] = entryState{State: "up", Ports: runPorts(subtree(n), n)}
		}
	}
	return states
}

// entryRunning finds a process in a place running an entry's command —
// by the name its row would show, or by its command line whole — or nil.
// A shell at its prompt is not running anything, whatever it was opened
// to run.
func (m model) entryRunning(path, command string) *ProcNode {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil
	}
	var found *ProcNode
	var walk func(n *ProcNode)
	walk = func(n *ProcNode) {
		if found != nil {
			return
		}
		if !isShell(n.Command) && (commandOf(n) == command || strings.TrimSpace(n.Argv) == command) {
			found = n
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, root := range m.byPlace[path] {
		walk(root)
	}
	return found
}

// portsUnder is every port listened on beneath a held shell: the entry's
// command and what it started, which is where a dev server's port is.
func (m model) portsUnder(pid int) []string {
	n := m.nodes[pid]
	if n == nil {
		return nil
	}
	return runPorts(subtree(n), n)
}

// ended is how the command a row's shell was started with ended, when the
// row is that shell at its prompt: the exit its pane recorded. Nothing
// while something runs in the shell — the command, or whatever was run
// by hand after it — and nothing for a shell started with no command.
func (m model) ended(r navRow) string {
	return m.ending(r).State
}

// ending is ended with the moment: how and when the row's shell's command
// ended, on the same terms.
func (m model) ending(r navRow) entryState {
	if r.kind != rowProc || len(r.run) != 1 || !isShell(r.node.Command) {
		return entryState{}
	}
	if t := m.terms[r.node.PID]; t != nil {
		return entryState{State: t.exit, At: t.at, Summary: t.summary}
	}
	return entryState{}
}

// unwell reports a process in a row's run that is stopped, in an
// uninterruptible wait, or a zombie: alive by the process table and no
// use to anyone.
func unwell(run []*ProcNode) bool {
	for _, n := range run {
		if n.State != "" && strings.ContainsRune("TUZ", rune(n.State[0])) {
			return true
		}
	}
	return false
}

// wrong reports a row that shows the cross: its command ended badly, or a
// process of its run is unwell. With an agent's prompt, it is the state
// that needs you.
func (m model) wrong(r navRow) bool {
	if r.kind != rowProc {
		return false
	}
	exit := m.ended(r)
	return unwell(r.run) || containerWrong(r.run) || (exit != "" && exit != "0")
}

// needsYou reports a row that tab goes to: an agent waiting on you, or a
// row gone wrong — the things that stop work until you look.
func (m model) needsYou(r navRow) bool {
	return m.owed(r) != nil || m.wrong(r)
}

// askKill previews a kill for whatever the cursor is on: what would die,
// with what it holds, before anything does. A plain kill takes the one
// selected process; a tree kill takes everything below it too; the
// preview shows the tree either way, since the consequences are the
// tree's, and X and x choose. On a place both widths are the same width
// — everything running in it. The preview has the window: a buffer shown
// gives it up, and has it back once the question is answered.
func (m *model) askKill(tree bool) tea.Cmd {
	r, ok := m.selected()
	if !ok {
		return nil
	}
	if r.kind == rowRest {
		// Nothing is running: there is nothing to stop.
		m.status, m.statusErr = "a conversation at rest is not running; enter continues it", false
		return nil
	}
	where := ""
	if p, ok := m.placeAt(r.project.Path); ok {
		where = "in " + p.Name
	}

	if r.kind != rowProc {
		// A place's kill covers everything beneath it: the sub-projects of a
		// repository, the repositories of a group. They are on screen under
		// the row, and an x that ignored them would be an x ignoring half of
		// what is shown.
		var roots []*ProcNode
		switch r.kind {
		case rowGroup:
			roots = m.groupTrees(r.project.Path)
		case rowProject:
			roots = m.repoTrees(r.project.Path)
		default:
			roots = m.byPlace[r.project.Path]
		}
		var nodes []*ProcNode
		for _, root := range roots {
			nodes = append(nodes, subtree(root)...)
		}
		if len(nodes) == 0 {
			m.status, m.statusErr = "nothing running in "+r.project.Name, true
			return nil
		}
		m.preview(&killRequest{
			subject: plural(len(nodes), "process", "processes") + " in " + r.project.Name,
			where:   "",
			nodes:   nodes,
			tree:    true,
		})
		return nil
	}

	// The head: the row's process, and the shell conn holds around it, for
	// a process running in one. The tree: the whole run the row stands for
	// and everything under it, the job included. An entry's shell — a
	// plan's, a task's — is its command and everything under it, whichever
	// was asked: the row is the entry, and the entry is the job.
	name := m.rowName(r)
	head := []*ProcNode{r.node}
	if shell := m.shellAround(r.node); shell != nil {
		head = append(head, shell)
	}
	all := m.withGroup(subtree(r.chain()))
	if t := m.entryOf(r); t != nil {
		if shell := m.nodes[t.pid]; shell != nil && t.live() && len(shell.Children) > 0 {
			all = m.withGroup(subtree(shell))
			head = nil
			name = t.name
		}
	}
	req := &killRequest{subject: name, where: where, nodes: all, head: head, tree: tree}
	if len(head) >= len(all) {
		req.head = nil
	}
	m.preview(m.escalated(r, req))
	return nil
}

// preview puts a kill's preview up: conn takes the window for it.
func (m *model) preview(req *killRequest) {
	m.pendingKill = req
	m.take()
}

// withGroup is a kill's targets and what shares a process group with them:
// a tree kill, or an entry's, is a kill of the job, and the job includes
// what a parent that exited left to init. They come after the tree, since
// nothing in the tree is supervising them.
func (m model) withGroup(nodes []*ProcNode) []*ProcNode {
	return append(nodes, groupMates(nodes, m.procs, m.nodes)...)
}

// escalated is a kill armed with SIGKILL when the row it is aimed at has
// already had a signal and not gone: asking a second time with the same
// signal would be asking the process again to do what it has declined to
// do. A kill aimed elsewhere is left as it was asked.
func (m model) escalated(r navRow, req *killRequest) *killRequest {
	if m.signalled(r) {
		req.sig = syscall.SIGKILL
	}
	return req
}

// ended names what was actually done, because a kill is not one thing: a shell
// conn holds is hung up and everything else is signalled, and a subtree can be
// both at once.
func ended(results []killResult, sig syscall.Signal) string {
	var hungUp, signalled int
	for _, r := range results {
		// A process already gone was not signalled and was not hung up;
		// nothing was done to it, so it names nothing.
		if r.err != nil {
			continue
		}
		if r.hungUp {
			hungUp++
			continue
		}
		signalled++
	}
	switch {
	case signalled == 0:
		return "closed "
	case hungUp == 0:
		return "sent " + signalName(sig) + " to "
	default:
		return "ended "
	}
}

// describeFailures says why a kill did not land, naming the reasons rather
// than the processes: a subtree fails for the same handful of reasons over and
// over, and "not permitted" said once is the useful report.
func describeFailures(results []killResult) string {
	var reasons []string
	seen := map[string]bool{}
	for _, r := range results {
		if r.err == nil || errors.Is(r.err, errGone) || seen[r.err.Error()] {
			continue
		}
		seen[r.err.Error()] = true
		reasons = append(reasons, r.err.Error())
	}
	slices.Sort(reasons)
	return strings.Join(reasons, ", ")
}

// toggleCollapse folds or unfolds the selected node. A row with nothing under
// it is left alone, so space never appears to do nothing at random.
func (m *model) toggleCollapse() {
	r, ok := m.selected()
	if !ok || m.childCount(r) == 0 {
		return
	}

	key := detailKey(r)
	if m.collapsed[key] {
		delete(m.collapsed, key)
	} else {
		m.collapsed[key] = true
	}

	// The rows below the cursor change, but the cursor keeps its subject.
	m.rows = m.flatten()
	m.scrollToCursor()
}

// childCount is how many processes a row hides when it is collapsed: every
// process in the repository, or every descendant of the process.
func (m model) childCount(r navRow) int {
	roots := m.byPlace[r.project.Path]
	switch r.kind {
	case rowGroup:
		roots = m.groupTrees(r.project.Path)
	case rowProject:
		roots = m.repoTrees(r.project.Path)
	case rowProc:
		return countTree(r.leaf()) - 1
	case rowRest:
		return 0
	}
	total := 0
	for _, n := range roots {
		total += countTree(n)
	}
	return total
}

// spinNeeded reports whether anything on screen is moving: a process on its
// way out, or an agent at work.
func (m model) spinNeeded() bool {
	if len(m.dying) > 0 {
		return true
	}
	for _, r := range m.rows {
		if a := m.agentFor(r); a != nil && a.working() {
			return true
		}
	}
	return false
}

// ageDying counts the frames each signalled process has lasted and gives up on
// the ones that are not going. Marking a process forever would both misreport
// it and keep rescanning on its behalf for the rest of the session.
func (m *model) ageDying() {
	var stuck []int
	for pid, d := range m.dying {
		d.frames++
		m.dying[pid] = d
		if d.frames > killLinger {
			stuck = append(stuck, pid)
		}
	}
	if len(stuck) == 0 {
		return
	}

	// Sorted, so what the footer says does not depend on map order.
	slices.Sort(stuck)
	names := make([]string, 0, len(stuck))
	for _, pid := range stuck {
		names = append(names, m.dying[pid].command+" "+strconv.Itoa(pid))
		m.refused[pid] = m.dying[pid].command
		delete(m.dying, pid)
	}
	m.status, m.statusErr = strings.Join(names, ", ")+" did not exit · x again kills outright", true
}

// pruneDying drops the processes that have gone. It reads the process list
// rather than the rows, because a dying process inside a folded subtree has no
// row and is not therefore gone.
func (m *model) pruneDying() {
	if len(m.dying) == 0 && len(m.refused) == 0 {
		return
	}
	live := make(map[int]bool, len(m.procs))
	for _, p := range m.procs {
		live[p.PID] = true
	}
	for pid := range m.dying {
		if !live[pid] {
			delete(m.dying, pid)
		}
	}
	for pid := range m.refused {
		if !live[pid] {
			delete(m.refused, pid)
		}
	}
}

// signalled reports whether a row's process has had a signal it has not
// acted on: on its way out still, or given up on. The row's whole run is
// asked, since the row is the run — an entry's row is its shell, and the
// signal went to the command under it.
func (m model) signalled(r navRow) bool {
	for _, n := range append([]*ProcNode{r.node}, r.run...) {
		if _, dying := m.dying[n.PID]; dying {
			return true
		}
		if _, refused := m.refused[n.PID]; refused {
			return true
		}
	}
	return false
}

// scrollToCursor moves the window the least amount that brings the cursor back
// into view, so scrolling follows the cursor instead of recentering on it.
func (m *model) scrollToCursor() {
	h := m.bodyHeight()
	if h == 0 {
		m.offset = 0
		return
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
	if max := len(m.rows) - h; m.offset > max {
		m.offset = max
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

// rebuild regroups processes by repository and reflattens the navigator,
// keeping the cursor on the same subject where that subject still exists.
func (m *model) rebuild() {
	was := ""
	wasRoot := 0
	if r, ok := m.selected(); ok {
		was = detailKey(r)
		// The root of the run too: the name a run is keyed by is borrowed —
		// a transient child can name it for a single scan — but the top of
		// the run is owned, and it is what the row still is when the name
		// has moved on.
		if r.kind == rowProc {
			wasRoot = r.chain().PID
		}
	}

	// What was just started has landed. The filter has done its job: the
	// project holds work now, so it stays in the list on its own merit and the
	// search that found it can go. Clearing here rather than when the key was
	// pressed is what stops the project blinking out and back while the scan
	// catches up.
	if m.wantCursor != 0 && m.running(m.wantCursor) {
		m.putFilter("")
	}
	// The server holding them is enough to know they exist; waiting for the
	// process scan as well would hold the search open for a poll longer, and
	// the server is the thing that was actually asked.
	if m.wantProject != "" && len(m.planned(m.wantProject)) > 0 {
		m.putFilter("")
	}

	m.groupProcs()
	m.rows = m.flatten()

	// Prefer the same subject; failing that, the row that grew from the
	// same root — a run renames itself when what it is running comes or
	// goes, and the cursor should ride the rename rather than strand on
	// whatever slid into the old row's place. Only with both gone — a
	// process that exited — hold the position in the list rather than
	// jumping to the top.
	found, root := -1, -1
	for i, r := range m.rows {
		if detailKey(r) == was {
			found = i
			break
		}
		if root < 0 && wasRoot != 0 && r.kind == rowProc && r.chain().PID == wasRoot {
			root = i
		}
	}
	if found < 0 {
		found = root
	}
	switch {
	case found >= 0:
		m.cursor = found
	case m.cursor >= len(m.rows):
		m.cursor = len(m.rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}

	// A project whose processes were just started keeps the cursor until
	// the server holds the first of them; then the cursor follows that
	// shell and lands on its row.
	if m.wantProject != "" {
		m.selectProject(m.wantProject)
		for _, t := range m.planned(m.wantProject) {
			// The one just started, not an earlier shell for the same
			// entry left at its prompt after its command ended.
			if t.name == m.wantName && t.live() {
				m.wantCursor = t.pid
				m.wantProject, m.wantName = "", ""
				break
			}
		}
	}
	m.noticeEnded()

	// A shell just opened takes the cursor as soon as it is in the tree, so
	// that leaving it leaves the cursor somewhere that makes sense. One the
	// scan has seen and the tree has no row for — opened somewhere no
	// project holds — is not coming, and the cursor stops waiting for it.
	if m.wantCursor != 0 {
		landed := false
		for i, r := range m.rows {
			// The shell may have folded into whatever it started, so the row
			// to land on is the one whose run begins with it.
			if r.kind == rowProc && r.holds(m.wantCursor) {
				m.cursor, landed = i, true
				break
			}
		}
		if landed || m.running(m.wantCursor) {
			m.wantCursor = 0
		}
	}

	m.pruneDying()
	m.readHistories()
	m.scrollToCursor()
	m.dressWindows()
}

// readHistories reads each named buffer's past runs for the heading's
// strip, once per ending: the runs file is read again for a buffer when
// its ending changed, and a buffer gone takes its strip with it.
func (m *model) readHistories() {
	for pid, t := range m.terms {
		if t.name == "" {
			continue
		}
		key := t.exit + "/" + strconv.Itoa(t.ended)
		if m.histories[pid] == key {
			continue
		}
		m.histories[pid] = key
		m.history[pid] = pastRuns(t.dir, t.name, t.run, runsShown)
	}
	for pid := range m.history {
		if _, held := m.terms[pid]; !held {
			delete(m.history, pid)
			delete(m.histories, pid)
			delete(m.deep, pid)
		}
	}
}

// dressWindows names each held shell's pane — its place, what is running
// there, and its mark — for the terminal's title while it has focus.
// Only what changed is said: saying the same thing again would be
// noise on the server.
func (m *model) dressWindows() {
	for pid, t := range m.terms {
		name, mark := m.shellLabel(pid, t)
		if mark != "" {
			name += " " + mark
		}
		if m.dressed[pid] != name {
			m.dressed[pid] = name
			m.server.dress(pid, name)
		}
	}
}

// dressStatus writes the navigator's part of the status line — the mode
// its keys are in and what it has to say — when either changed.
func (m *model) dressStatus() {
	t := m.statusLine()
	if t != m.said {
		m.said = t
		m.server.say(t)
	}
}

// statusLine is what conn has the status line read, in tmux's styling.
// One mode chip: a confirmation waiting on its second key, a query being
// typed or a filter standing, the everything view, the shown buffer's run
// failed, answers owed, an agent working — the first of those that holds,
// and nothing when none does. The message is the last report, or what a
// newer conn says, else the session's facts.
func (m model) statusLine() statusText {
	var t statusText
	t.edge = m.tabEdge()
	need := m.needCount()
	switch {
	case m.pendingReplace:
		t.mode = statusChip(tp.owed, "CONFIRM")
		t.msg = tmuxStyled(tp.owed, true, " end the server, and "+
			plural(len(m.terms), "buffer", "buffers")+"? · R confirms")
		return t

	case m.pendingKill != nil:
		t.mode = statusChip(tp.owed, "CONFIRM")
		t.msg = tmuxStyled(tp.gray, false, " "+m.pendingKill.facts())
		return t

	case m.typing:
		t.mode = statusChip(tp.ink, "/"+lineText(m.query))

	case m.filter != "":
		// A standing filter is the view's mode still, in its color.
		t.mode = statusChip(tp.teal, "/"+m.filter)

	case m.viewingAll():
		t.mode = statusChip(tp.teal, "ALL")

	case m.shownFailed():
		t.mode = statusChip(tp.owed, "FAILED")

	case need > 0:
		t.mode = statusChip(tp.owed, strconv.Itoa(need)+" OWED")

	case m.shownWorking():
		t.mode = statusChip(tp.amber, "WORKING")
	}

	// What was just reported stays beside the query being typed: acting
	// from the search is the point of it, and an action that says nothing
	// looks like one that did nothing.
	switch {
	case m.status != "":
		color := tp.ink
		if m.statusErr {
			color = tp.owed
		}
		t.msg = tmuxStyled(color, false, " "+m.status)
	case m.release != "":
		// A newer conn, said whenever nothing else is: it gives way to
		// any report and is back after, until it is taken.
		t.msg = tmuxStyled(tp.gray, false, " "+updateNotice(m.release))
	case !m.typing && m.filter == "":
		t.msg = tmuxStyled(tp.gray, false, " "+m.sessionFacts())
	}
	return t
}

// shownFailed reports the shown buffer's run ended badly, or its process
// has gone wrong.
func (m model) shownFailed() bool {
	r, ok := m.shownRow()
	return ok && m.wrong(r)
}

// shownWorking reports the shown buffer's agent mid-turn.
func (m model) shownWorking() bool {
	r, ok := m.shownRow()
	if !ok {
		return false
	}
	a := m.agentFor(r)
	return a != nil && a.working()
}

// shownRow is the row the shown buffer stands for, when the scan has it.
func (m model) shownRow() (navRow, bool) {
	n := m.nodes[m.shown]
	if m.shown == 0 || n == nil {
		return navRow{}, false
	}
	run := runFrom(n, len(m.procs))
	return navRow{kind: rowProc, run: run, node: nameOf(run)}, true
}

// sessionFacts is what the status line says when nothing else is said:
// the buffers open, and how much more is live behind them; in the
// everything view, what it lists.
func (m model) sessionFacts() string {
	listed := 0
	for _, roots := range m.byPlace {
		for _, n := range roots {
			listed += countTree(n)
		}
	}
	if m.viewingAll() {
		places := 0
		for _, r := range m.rows {
			if isPlace(r) {
				places++
			}
		}
		return dots(plural(listed, "process", "processes"), plural(places, "place", "places"))
	}
	behind := listed
	for pid := range m.terms {
		if n := m.nodes[pid]; n != nil {
			behind -= countTree(n)
		}
	}
	facts := plural(len(m.terms), "buffer", "buffers")
	if behind > 0 {
		facts += " " + glyphDot + " " + strconv.Itoa(behind) + " more processes live behind them"
	}
	if t := m.terms[m.shown]; t != nil {
		return dots(m.tabLabel(m.shown, t), facts)
	}
	return facts
}

// windowLabelWidth is as much of a shell's label as its title gets: a
// whole command line there would run the terminal's title bar off the
// end.
const windowLabelWidth = 24

// shellLabel is what a held shell's window is called and how it is marked:
// the place it works in and the name its row would show — the plan's name
// for it, unless what is running says more — and its agent's mark, if it
// is running one. It reads the process tree rather than the rows, because
// a row can be folded away or filtered out and the window is still there.
func (m model) shellLabel(pid int, t *remoteTerm) (string, string) {
	label := t.name
	mark := ""
	if n := m.nodes[pid]; n != nil {
		run := runFrom(n, len(m.procs))
		r := navRow{kind: rowProc, run: run, node: nameOf(run)}
		if label == "" {
			label = commandOf(r.node)
		}
		// A command that ended, well or badly, or a process gone wrong,
		// marks the window with the row's mark.
		switch {
		case m.wrong(r):
			mark = glyphFailed
		case m.ended(r) == "0" || containerDone(r):
			mark = glyphDone
		}
		if a := m.agentFor(r); a != nil {
			switch {
			case a.working():
				mark = glyphBusy
			case m.owed(r) != nil:
				if _, blocked := a.blocked(); blocked {
					mark = glyphAsk
				} else {
					mark = glyphOn
				}
			default:
				mark = glyphOff
			}
		}
	}
	if label == "" {
		label = "shell"
	}
	if p, ok := m.placeAt(t.dir); ok {
		label = p.Name + ": " + label
	}
	return truncateTail(label, windowLabelWidth), mark
}

// running reports whether a pid is in the process list as it now stands.
func (m model) running(pid int) bool {
	for _, p := range m.procs {
		if p.PID == pid {
			return true
		}
	}
	return false
}

// groupProcs files running processes under the repository they belong to.
//
// A process is attributed to the innermost repository containing it, so a
// process in a nested checkout is listed there and not under its parent repo.
func (m *model) groupProcs() {
	m.grouped = make(map[string][]Project, len(m.groups))
	for _, p := range m.projects {
		if p.Group != "" {
			m.grouped[p.Group] = append(m.grouped[p.Group], p)
		}
	}

	m.parent = make(map[int]int, len(m.procs))
	for _, pr := range m.procs {
		m.parent[pr.PID] = pr.PPID
	}

	// What the containers publish, for telling docker's proxy from a
	// service of the machine's own.
	published := map[string]bool{}
	for _, pr := range m.procs {
		if pr.Container != nil {
			for _, port := range pr.Ports {
				published[port] = true
			}
		}
	}

	// An agent is yours wherever it runs: one started outside every root
	// — in a scratch directory, in a checkout the config does not list —
	// is listed under global with what it runs, since it is the process
	// that most needs you and the place is a label, not a gate.
	agents := map[int]bool{}
	for _, pr := range m.procs {
		if pr.Dir != globalPlace && m.holdsDir(pr.Dir) {
			continue
		}
		if _, ok := agentKindOf(&ProcNode{Proc: pr}); ok {
			agents[pr.PID] = true
		}
	}

	owner := make(map[string][]Proc, len(m.projects))
	for _, pr := range m.procs {
		// A container of no project, or of a directory no root holds,
		// is global; one of a project elsewhere is named for both. So is
		// a service listening from no project's directory (service) —
		// unless it is docker's proxy, which the containers already say
		// — and an agent, with what it runs.
		if pr.Dir == globalPlace || !m.holdsDir(pr.Dir) {
			switch {
			case pr.Container != nil:
				if pr.Container.Project != "" {
					pr.Command = pr.Container.Project + "/" + pr.Container.Service
				}
			case m.underAgent(pr.PID, agents):
			case !service(pr) || proxied(pr, published):
				continue
			}
			owner[globalPlace] = append(owner[globalPlace], pr)
			continue
		}
		best := ""
		for _, p := range m.projects {
			if under(pr.Dir, p.Path) && len(p.Path) > len(best) {
				best = p.Path
			}
		}
		if best == "" {
			// Work at a group's own level — a shell opened on the group row —
			// is in none of its repositories, and belongs to the group.
			for _, g := range m.groups {
				if under(pr.Dir, g.Path) && len(g.Path) > len(best) {
					best = g.Path
				}
			}
			if best != "" {
				owner[best] = append(owner[best], pr)
			}
			continue
		}
		// Within the repository, the innermost sub-project containing the
		// process is its place; a process in none of them works at the root
		// — unless the directory it works in, or one between it and the
		// root, carries a manifest the index did not list: an ignored
		// directory, one made since the scan. The process makes the
		// sub-project, and the manifest names it.
		place := best
		for _, sp := range m.subs[best] {
			if under(pr.Dir, sp.Path) && len(sp.Path) > len(place) {
				place = sp.Path
			}
		}
		if place == best {
			if found := m.manifestDirUp(pr.Dir, best); found != "" {
				place = found
				m.addSub(best, found)
			}
		}
		owner[place] = append(owner[place], pr)
	}

	m.byPlace = make(map[string][]*ProcNode, len(owner))
	m.nodes = make(map[int]*ProcNode, len(m.procs))
	for path, procs := range owner {
		m.byPlace[path] = procForest(procs)
		for _, root := range m.byPlace[path] {
			indexNodes(root, m.nodes)
		}
	}
}

// underAgent reports a process that is one of the agents, or runs beneath
// one: what an agent started is listed with it.
func (m model) underAgent(pid int, agents map[int]bool) bool {
	for step := 0; step < 64 && pid > 1; step++ {
		if agents[pid] {
			return true
		}
		next, ok := m.parent[pid]
		if !ok || next == pid {
			return false
		}
		pid = next
	}
	return false
}

// manifestDirUp is the nearest directory from dir up to, and not
// including, root that carries a manifest — or "". The answer is kept by
// directory until the places are scanned again, since the processes are
// filed twice a second and the directories change slowly.
func (m *model) manifestDirUp(dir, root string) string {
	if dir == "" || dir == root || !under(dir, root) {
		return ""
	}
	if found, ok := m.manifestDirs[dir]; ok {
		return found
	}
	found := ""
	for d := dir; d != root && under(d, root); d = filepath.Dir(d) {
		if hasManifest(d) {
			found = d
			break
		}
	}
	if m.manifestDirs == nil {
		m.manifestDirs = map[string]string{}
	}
	m.manifestDirs[dir] = found
	return found
}

// addSub lists a sub-project found by a process working in it, beside the
// ones the repository's index listed, in their order.
func (m *model) addSub(repo, path string) {
	for _, sp := range m.subs[repo] {
		if sp.Path == path {
			return
		}
	}
	rel, err := filepath.Rel(repo, path)
	if err != nil {
		return
	}
	if m.subs == nil {
		m.subs = map[string][]Project{}
	}
	subs := append(m.subs[repo], Project{Name: filepath.ToSlash(rel), Path: path})
	slices.SortFunc(subs, func(a, b Project) int { return cmp.Compare(a.Name, b.Name) })
	m.subs[repo] = subs
}

// repoTrees is every process tree in a repository: the ones at its root and
// the ones inside each of its sub-projects.
func (m model) repoTrees(repo string) []*ProcNode {
	out := append([]*ProcNode{}, m.byPlace[repo]...)
	for _, s := range m.subs[repo] {
		out = append(out, m.byPlace[s.Path]...)
	}
	return out
}

// placeHasWork reports whether a sub-project holds a process tree or a shell
// the server is holding there — the shell counting before the scan has seen
// it, for the same reason a repository's does.
func (m model) placeHasWork(path string) bool {
	if len(m.byPlace[path]) > 0 {
		return true
	}
	for _, t := range m.terms {
		if t.dir == path {
			return true
		}
	}
	return false
}

// workIn reports whether anything is running in a repository, its
// sub-projects included.
func (m model) workIn(repo string) bool {
	if len(m.repoTrees(repo)) > 0 {
		return true
	}
	for _, t := range m.terms {
		if under(t.dir, repo) {
			return true
		}
	}
	return false
}

// groupTrees is every process tree in a group: at its own level, and inside
// each of its repositories.
func (m model) groupTrees(g string) []*ProcNode {
	out := append([]*ProcNode{}, m.byPlace[g]...)
	for _, p := range m.grouped[g] {
		out = append(out, m.repoTrees(p.Path)...)
	}
	return out
}

// workInGroup reports whether anything is running anywhere in a group. The
// terms check covers a shell just opened at the group's own level, before
// the scan has seen it.
func (m model) workInGroup(g Project) bool {
	if len(m.byPlace[g.Path]) > 0 {
		return true
	}
	for _, t := range m.terms {
		if under(t.dir, g.Path) {
			return true
		}
	}
	return false
}

// flatten turns the visible places — groups, repositories, sub-projects —
// and their process trees into the flat list of selectable rows the
// navigator draws and the cursor walks.
func (m model) flatten() []navRow {
	var rows []navRow
	for _, top := range m.topPlaces() {
		if top.kind == rowProject {
			rows = append(rows, m.flattenRepo(top.project, "")...)
			continue
		}

		// A group is a heading only for the work at its own level — a
		// shell opened on the group, the global place's processes; its
		// repositories are headings of their own, named for it.
		repos := m.visibleRepos(top.project)
		own := m.byPlace[top.project.Path]
		if m.typing {
			if f := strings.ToLower(strings.TrimSpace(m.filter)); f != "" {
				own = m.matchingProcs(own, f)
			} else {
				own = nil
			}
		}
		if len(own) > 0 || top.project.Path == globalPlace || (len(repos) == 0 && !m.typing) {
			rows = append(rows, top)
			if !m.collapsed[detailKey(top)] {
				for _, n := range m.treesByNeed(own) {
					rows = append(rows, m.flattenProc(top.project, n, "")...)
				}
			}
		}
		for _, p := range repos {
			rows = append(rows, m.flattenRepo(p, "")...)
		}
	}
	return rows
}

// qualified is a repository's heading: its name under its group's, when
// it has one.
func (m model) qualified(p Project) string {
	if p.Group == "" {
		return p.Name
	}
	for _, g := range m.groups {
		if g.Path == p.Group {
			return g.Name + "/" + p.Name
		}
	}
	return filepath.Base(p.Group) + "/" + p.Name
}

// topPlaces is the top of the navigator: the groups and the repositories
// standing alone, in one order — the places with something that needs
// you above the rest, each side alphabetical — each listed when its own
// rule says so; and the global place last, below every project, while
// anything is in it: the blank line above it divides your work from
// the machine's own.
func (m model) topPlaces() []navRow {
	var out []navRow
	for _, g := range m.groups {
		if m.groupVisible(g) {
			out = append(out, navRow{kind: rowGroup, project: g})
		}
	}
	for _, p := range m.projects {
		if p.Group == "" && m.repoVisible(p) {
			out = append(out, navRow{kind: rowProject, project: p})
		}
	}
	slices.SortStableFunc(out, func(a, b navRow) int { return m.byNeed(a.project, b.project) })
	if len(m.byPlace[globalPlace]) > 0 && m.groupVisible(globalGroup) {
		out = append(out, navRow{kind: rowGroup, project: globalGroup})
	}
	return out
}

// repoVisible is the repositories' listing rule. While a filter is at work,
// the ones that answer to it — by name or path, by a sub-project answering,
// or by a process running there whose command answers, so typing claude finds
// the repositories a claude is working in and not only the ones named for it.
// A repository whose sub-project answers is listed for the answer's sake, the
// way a directory holds the file you were looking for. Otherwise the ones
// with work in them, or all behind the dot. A shell the server is holding
// counts as work before the scan has seen it: it is running, and asking for
// it and then watching the project vanish for a poll would be a lie about
// what just happened.
func (m model) repoVisible(p Project) bool {
	if m.typing || m.filter != "" {
		return matchesFilter(p, m.filter) || len(m.matchingSubs(p)) > 0 ||
			m.procAnswers(p.Path, m.filter)
	}
	return m.showAll || m.workIn(p.Path)
}

// groupVisible is the same rule at the group's altitude: its own name or a
// process running at its folder answering the filter, or any of its
// repositories answering; its own directory holding work, or any of its
// repositories holding some.
func (m model) groupVisible(g Project) bool {
	if m.typing || m.filter != "" {
		return matchesFilter(g, m.filter) || len(m.visibleRepos(g)) > 0 ||
			m.procAnswers(g.Path, m.filter)
	}
	if m.showAll || m.workInGroup(g) {
		return true
	}
	for _, p := range m.grouped[g.Path] {
		if m.workIn(p.Path) {
			return true
		}
	}
	return false
}

// visibleRepos is the repositories listed beneath a group, by the
// repositories' own rule.
func (m model) visibleRepos(g Project) []Project {
	var out []Project
	for _, p := range m.grouped[g.Path] {
		if m.repoVisible(p) {
			out = append(out, p)
		}
	}
	slices.SortStableFunc(out, m.byNeed)
	return out
}

// byNeed orders two places: the one with something that needs you first,
// then by name. The list reads from the top, and a place whose agent is
// waiting or whose run failed is the place to read first; the rest keep
// their alphabetical slots, so a quiet list is the same list every time.
func (m model) byNeed(a, b Project) int {
	return cmp.Or(cmp.Compare(btoi(!m.placeNeeds(a)), btoi(!m.placeNeeds(b))), byName(a, b))
}

// placeNeeds reports a place with a row in it that needs you: among its
// own process trees, its sub-projects', or, for a group, its
// repositories'.
func (m model) placeNeeds(p Project) bool {
	for _, n := range m.byPlace[p.Path] {
		if m.needIn(n) {
			return true
		}
	}
	for _, sp := range m.subs[p.Path] {
		if m.placeNeeds(sp) {
			return true
		}
	}
	for _, rp := range m.grouped[p.Path] {
		if m.placeNeeds(rp) {
			return true
		}
	}
	return false
}

// needIn reports a process tree with a row in it that needs you: the run
// the root heads, or any run beneath it, folded or not — what is folded
// away still needs you.
func (m model) needIn(n *ProcNode) bool {
	return m.needUnder(n) > 0
}

// needUnder counts the rows in a process tree that need you.
func (m model) needUnder(n *ProcNode) int {
	run := []*ProcNode{n}
	if !m.unfolded {
		run = runFrom(n, len(m.procs))
	}
	r := navRow{kind: rowProc, run: run, node: nameOf(run)}
	count := btoi(m.needsYou(r))
	for _, c := range r.leaf().Children {
		count += m.needUnder(c)
	}
	return count
}

// needCount is how many rows need you, in the whole list, folded or
// filtered or not: the status line's number, which says there is
// something to tab to before the list says where.
func (m model) needCount() int {
	count := 0
	for _, roots := range m.byPlace {
		for _, n := range roots {
			count += m.needUnder(n)
		}
	}
	return count
}

// treesByNeed is a place's process trees with the ones that need you
// first, each side in its listed order: a name keeps its slot among its
// neighbors, and a tree that needs you steps ahead of them all.
func (m model) treesByNeed(roots []*ProcNode) []*ProcNode {
	out := slices.Clone(roots)
	slices.SortStableFunc(out, func(a, b *ProcNode) int {
		return cmp.Compare(btoi(!m.needIn(a)), btoi(!m.needIn(b)))
	})
	return out
}

// flattenRepo is one repository's section: its row, and beneath it the
// sub-projects and process trees, everything shifted right when the
// repository itself sits under a group.
func (m model) flattenRepo(p Project, indent string) []navRow {
	row := navRow{kind: rowProject, project: p, name: m.qualified(p), prefix: indent}
	rows := []navRow{row}
	subs := m.visibleSubs(p)
	// While a project is being looked up, an empty query lists places alone —
	// every process of every project would bury the names being scanned for.
	// But a query is a name, and a process that answers to it is as much a
	// thing being looked for as a project is: it is listed, pruned to the
	// branches that answer, so enter can step into it and ctrl+x can kill it.
	if m.typing {
		f := strings.ToLower(strings.TrimSpace(m.filter))
		var roots []*ProcNode
		if f != "" {
			roots = m.matchingProcs(m.byPlace[p.Path], f)
		}
		for _, n := range roots {
			rows = append(rows, m.flattenProc(p, n, indent)...)
		}
		for _, sp := range subs {
			srow := navRow{kind: rowSub, project: sp, name: p.Name + "/" + sp.Name, prefix: indent}
			rows = append(rows, srow)
			if f == "" {
				continue
			}
			for _, n := range m.matchingProcs(m.byPlace[sp.Path], f) {
				rows = append(rows, m.flattenProc(sp, n, indent)...)
			}
		}
		return rows
	}
	if m.collapsed[detailKey(row)] {
		return rows
	}
	// The repository's own processes, the trees that need you first; then
	// the newest conversation at rest, while the place has work: a process
	// that is not running, kept in view the way an exited container is
	// beside its siblings, for enter to pick back up. Each sub-project is
	// a heading of its own after, named for the repository and itself.
	for _, n := range m.treesByNeed(m.byPlace[p.Path]) {
		rows = append(rows, m.flattenProc(p, n, indent)...)
	}
	if c, ok := m.rests[p.Path]; ok && m.workIn(p.Path) {
		rows = append(rows, navRow{kind: rowRest, project: p, rest: c, prefix: indent})
	}
	slices.SortStableFunc(subs, m.byNeed)
	for _, sp := range subs {
		srow := navRow{kind: rowSub, project: sp, name: p.Name + "/" + sp.Name, prefix: indent}
		rows = append(rows, srow)
		if m.collapsed[detailKey(srow)] {
			continue
		}
		for _, n := range m.treesByNeed(m.byPlace[sp.Path]) {
			rows = append(rows, m.flattenProc(sp, n, indent)...)
		}
	}
	return rows
}

// visibleSubs is the sub-projects listed beneath a repository. While a
// filter is at work they are the ones that answer to it — an empty query
// lists projects alone, or every sub-project of every repository would bury
// the list being remembered. Otherwise they follow the repositories' own
// rule: the ones with work in them, or all of them behind the dot.
func (m model) visibleSubs(p Project) []Project {
	if m.typing || m.filter != "" {
		return m.matchingSubs(p)
	}
	all := m.subs[p.Path]
	if m.showAll {
		return all
	}
	var out []Project
	for _, sp := range all {
		if m.placeHasWork(sp.Path) {
			out = append(out, sp)
		}
	}
	return out
}

// matchingSubs is the sub-projects of a repository that answer to the
// filter, by their own path or qualified by their repository's name — which
// is how "mono api" and "services/api" both find the same place — or by a
// process running in them whose command answers.
func (m model) matchingSubs(p Project) []Project {
	f := strings.ToLower(strings.TrimSpace(m.filter))
	if f == "" {
		return nil
	}
	var out []Project
	for _, sp := range m.subs[p.Path] {
		if answers(f, sp.Name) || answers(f, p.Name+"/"+sp.Name) ||
			m.procAnswers(sp.Path, f) {
			out = append(out, sp)
		}
	}
	return out
}

func (m model) flattenProc(p Project, n *ProcNode, prefix string) []navRow {
	// Walk down while there is nothing to choose between (runFrom); unfolded,
	// every process has a row.
	run := []*ProcNode{n}
	if !m.unfolded {
		run = runFrom(n, len(m.procs))
	}

	row := navRow{kind: rowProc, project: p, run: run, node: nameOf(run), prefix: prefix}
	rows := []navRow{row}
	if m.collapsed[detailKey(row)] {
		return rows
	}

	for _, c := range row.leaf().Children {
		rows = append(rows, m.flattenProc(p, c, prefix+glyphIndent)...)
	}
	return rows
}

// nameOf picks the process a run is named for: the first in it that is not a
// shell.
//
// The deepest is the wrong answer. A shell that started a claude that started
// a caffeinate is a claude, not a caffeinate — the last process in a run is
// as often something the interesting one reached for as it is the point of
// the run. It also moves: naming a run after its deepest process renames the
// row every time the process that matters starts a tool and finishes with it.
//
// A run that is shells all the way down is named for the last of them, which
// is the shell you would be typing into.
func nameOf(run []*ProcNode) *ProcNode {
	for _, n := range run {
		if !isShell(n.Command) {
			return n
		}
	}
	return run[len(run)-1]
}

// shells are the commands that stand in front of what was actually run.
var shells = map[string]bool{
	"sh": true, "bash": true, "zsh": true, "fish": true,
	"dash": true, "ksh": true, "csh": true, "tcsh": true,
	"login": true,
}

func isShell(command string) bool { return shells[strings.TrimPrefix(command, "-")] }

// selected returns the row under the cursor.
func (m model) selected() (navRow, bool) {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return navRow{}, false
	}
	return m.rows[m.cursor], true
}

// holds reports whether a pid is anywhere in the run this row stands for.
func (r navRow) holds(pid int) bool {
	for _, n := range r.run {
		if n.PID == pid {
			return true
		}
	}
	return false
}

// agentFor returns the agent instance a row is running, if it is one. The
// process is checked as well as what the agent advertises, because what it
// advertises can outlive its process and a reused pid would otherwise be
// dressed up as an agent.
func (m model) agentFor(r navRow) agent {
	if r.kind != rowProc {
		return nil
	}
	a, ok := m.agents[r.node.PID]
	if !ok || !runs(a, r.node) {
		return nil
	}
	return a
}

// awaiting returns the agent a row is running when it is waiting on its user:
// done with a turn, by its own account, or blocked mid-turn on a specific
// ask. An instance idle since it was started has not finished a turn and is
// not owed an answer; a blocked one is owed its answer regardless. Both
// are read from the instance, not remembered by this window: the mark is
// the same from every window and after a restart.
func (m model) awaiting(r navRow) agent {
	a := m.agentFor(r)
	if a == nil || a.working() {
		return nil
	}
	if _, ok := a.blocked(); ok {
		return a
	}
	if t, ok := a.(turned); !ok || !t.finished() {
		return nil
	}
	return a
}

// deepMsg is what a held agent said of itself, read deeper than the scan
// does.
type deepMsg struct {
	pid   int
	facts agentFacts
}

// deepCmd reads every held agent for its heading's facts — its branch,
// its context — off the render path: the transcript is megabytes, and
// only its tail is read. The facts are kept by the buffer's shell, which
// is what the heading and the tabline are about.
func (m model) deepCmd() tea.Cmd {
	var cmds []tea.Cmd
	for pid, a := range m.agents {
		t := m.owningTerm(pid)
		if t == nil {
			continue
		}
		n := m.nodes[pid]
		if n == nil || !runs(a, n) {
			continue
		}
		shell := t.pid
		cmds = append(cmds, func() tea.Msg { return deepMsg{pid: shell, facts: a.describe()} })
	}
	return tea.Batch(cmds...)
}

// restsMsg carries the newest conversation at rest under each place with
// work.
type restsMsg struct{ rests map[string]conversation }

// restsCmd lists, off the render path, the newest conversation at rest
// under each place with work — the places listed, which are the ones a
// row at rest can sit under — vetted against the conversations running
// instances carry.
func (m model) restsCmd() tea.Cmd {
	dirs := map[string][]string{}
	for _, p := range m.projects {
		if m.workIn(p.Path) {
			dirs[p.Path] = m.convoDirs(p)
		}
	}
	live := m.liveConversations()
	return func() tea.Msg {
		rests := map[string]conversation{}
		for path, ds := range dirs {
			if c, ok := newestSuspended(ds, live); ok {
				rests[path] = c
			}
		}
		return restsMsg{rests: rests}
	}
}

// liveFingerprint is the live conversations in a word, for telling a
// change in them.
func (m model) liveFingerprint() string {
	ids := slices.Sorted(maps.Keys(m.liveConversations()))
	return strings.Join(ids, " ")
}

// continueRest picks a conversation at rest back up: a shell opens where
// it was had, running the command that continues it, and from there it is
// a live instance like any other.
func (m *model) continueRest(c conversation) tea.Cmd {
	if m.server == nil {
		m.status, m.statusErr = "no server to hold it: "+m.serverErr, true
		return nil
	}
	run := resumeCommand(c)
	if run == "" {
		m.status, m.statusErr = "no way to continue a "+c.Kind+" conversation", true
		return nil
	}
	m.server.open(c.Dir, run, "")
	return nil
}
