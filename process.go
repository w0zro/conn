package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

// A process, as the kernel describes it: who runs it, what terminal it
// holds, where it is working, what it was started as and when. The
// platform files read the table; the watch is composed from it.
type process struct {
	pid, ppid, pgid int
	uid             int
	tty             string // ttys004, pts/3; blank without a terminal
	foreground      bool   // its group holds the terminal
	state           byte   // R running, S sleeping, T stopped, Z ended, I idle, D in disk wait
	started         time.Time
	command         string   // the program's name
	args            []string // what it was started as, when that could be read
	cwd             string
	// cpu is all the processor time this process has used, which says
	// nothing on its own: what it has used since the last reading is
	// how the watch tells work from waiting.
	cpu time.Duration
}

// The kinds of process the watch tells apart, by the program's name.
// Everything else is a run: a build, a test, a server, a script.
const (
	kindShell  = "SHELL"
	kindAgent  = "AGENT"
	kindEditor = "EDITOR"
	kindConn   = "CONN" // conn itself; not on the watch
	kindRun    = "RUN"
	kindHold   = "HOLD" // conn standing in an empty slot; not on the watch
)

var (
	shells  = []string{"zsh", "bash", "fish", "sh", "dash", "nu", "tcsh", "ksh"}
	agents  = []string{"claude", "codex", "gemini", "aider", "opencode", "goose", "amp", "copilot", "ollama"}
	editors = []string{"vim", "nvim", "vi", "hx", "helix", "emacs", "nano", "micro", "kak"}
)

// kindOf is the kind of a process, from the name of its program. A
// program that writes its own title puts its name first and what it is
// at after it — claude bg-spare is claude, at its spare work — so the
// name is the first word of what it was started as.
func kindOf(p process) string {
	name := strings.TrimPrefix(filepath.Base(p.command), "-")
	if len(p.args) > 0 {
		name = strings.TrimPrefix(filepath.Base(p.args[0]), "-")
	}
	name, _, _ = strings.Cut(name, " ")
	switch {
	case name == "conn" && len(p.args) > 1 && p.args[1] == "hold":
		return kindHold
	case name == "conn":
		return kindConn
	case slices.Contains(shells, name):
		return kindShell
	case slices.Contains(agents, name):
		return kindAgent
	case slices.Contains(editors, name):
		return kindEditor
	default:
		return kindRun
	}
}

// The words an entry stands under. Whether the kernel caught a process
// on a processor or asleep says nothing by itself - macOS calls nearly
// everything runnable - so alive alone is ACTIVE, and WORKING is kept
// for a process that did something between one reading and the next.
const (
	statusWorking = "WORKING" // doing something, right now
	statusWaiting = "WAITING" // an agent stopped on an ask it put to you
	statusActive  = "ACTIVE"  // alive, and not doing anything
	statusIdle    = "IDLE"    // a shell at its prompt, or an agent at rest
	statusStopped = "STOPPED" // suspended
	statusEnded   = "ENDED"   // finished, and not yet collected
)

// standing is what conn learned about a process past what the table
// says of it. Anything can be working, read off the processor time it
// spent. Only an agent says more, being the only thing here that knows
// its own mind: mid-turn, stopped on an ask it put to you, or stopped
// with its turn over and nothing pending.
type standing struct {
	working bool
	waiting bool   // stopped on something it asked of you
	idle    bool   // stopped with its turn over, asking nothing
	asking  string // what a waiting agent is stopped on, in its own words
	// When it came to stand this way, where it says so; zero where it
	// does not. Only an agent knows the moment it stopped, and only
	// waiting is worth the moment: how long a thing has been held up on
	// you is the order to answer it in.
	since time.Time
}

// An entry is a row of the watch: one process, standing for its own
// work, at its place in the tree the processes it is among actually
// are.
type entry struct {
	pid     int
	kind    string
	command string // what it was started as, the program by its base name
	tty     string
	started time.Time
	status  string
	fault   bool      // a status to be looked at: STOPPED, ENDED
	depth   int       // how deep under its place's own root; the root at 0
	since   time.Time // when it came to stand as it does, where that is known
	// What the watch has no column for and the look reads: where the
	// process itself is, whatever place its tree belongs to, and what an
	// agent says it is stopped on.
	cwd    string
	asking string
}

// A place is a directory work is happening in, and the entries at it.
type place struct {
	path    string // as read; the watch writes it from ~
	entries []entry
}

// watch composes the places from the process table: the processes of
// one user with a terminal, each standing for its own work, nested
// under whatever candidate process runs it — the tree they actually
// are, rather than one leaf apiece. rootOf turns a working directory
// into the place that holds it; a whole tree is one place's, the root's
// own directory, whatever a process under it has since cd'd to.
//
// conn is not on the watch, and neither is what it holds. It is the
// instrument, not the work — the one conn you are looking at, the conn
// behind it holding the terminal, and the hold standing in an empty
// slot alike — and it covers what runs under it, so the tmux client it
// holds is no more a row than conn is. An agent and an editor cover
// nothing: what they run is work, and reads as theirs. The rule goes
// by the program's name, so a conn on another socket, or an older conn
// installed beside this one, is off the watch too.
//
// A terminal is how the watch tells your work from the machine's, and
// it is most of it, but not all: a server you left running has no
// terminal and is work all the same. So a process of yours without
// one is adopted where it works inside a project you have terminal
// work in - the dev server under the agent that started it, and the
// one from last week that outlived its shell alike. The project is
// what makes that safe. A place that is merely a directory adopts
// nothing, since a shell sitting at home would otherwise take in
// every daemon on the machine with it. What conn is held in is not
// work either: the tmux server conn runs inside has no terminal and
// works in the repository like anything else there, and is no more a
// row than conn is.
func watch(procs []process, uid int, rootOf func(string) string, isProject func(string) bool, how map[int]standing) []place {
	byPid := map[int]process{}
	for _, p := range procs {
		byPid[p.pid] = p
	}
	// A candidate is a process of the user with a terminal.
	candidate := map[int]bool{}
	for _, p := range procs {
		candidate[p.pid] = p.uid == uid && p.tty != ""
	}
	// covered says whether conn stands anywhere above a process: the
	// tmux client conn holds is conn's own doing, not work of yours,
	// and goes off the watch with it rather than hanging from whatever
	// happens to be above conn.
	covered := func(p process) bool {
		seen := map[int]bool{}
		for pid := p.ppid; pid > 0 && !seen[pid]; {
			seen[pid] = true
			a, ok := byPid[pid]
			if !ok {
				return false
			}
			if candidate[a.pid] {
				switch kindOf(a) {
				case kindConn, kindHold:
					return true
				}
			}
			pid = a.ppid
		}
		return false
	}
	// treeParent is the nearest candidate ancestor a process hangs
	// from, climbing past whatever is not one itself. Nothing covered
	// gets this far, so no ancestor it can find is conn's.
	treeParent := func(p process) (int, bool) {
		seen := map[int]bool{}
		for pid := p.ppid; pid > 0 && !seen[pid]; {
			seen[pid] = true
			a, ok := byPid[pid]
			if !ok {
				return 0, false
			}
			if candidate[a.pid] {
				return a.pid, true
			}
			pid = a.ppid
		}
		return 0, false
	}
	// The places terminal work has established, kept to those that are
	// projects in their own right: work of yours in a project is what
	// lets the watch speak for anything else running there.
	worked := map[string]bool{}
	for _, p := range procs {
		if !candidate[p.pid] || covered(p) || p.cwd == "" {
			continue
		}
		switch kindOf(p) {
		case kindConn, kindHold:
			continue
		}
		if root := rootOf(p.cwd); isProject(root) {
			worked[root] = true
		}
	}
	// conn's own scaffolding: whatever conn runs beneath. Only what is
	// up for adoption is asked, so a process with a terminal still
	// stands for itself - the shell you started conn from is your
	// shell, and stays a row.
	scaffolding := map[int]bool{}
	for _, p := range procs {
		switch kindOf(p) {
		case kindConn, kindHold:
		default:
			continue
		}
		seen := map[int]bool{}
		for pid := p.ppid; pid > 0 && !seen[pid]; {
			seen[pid] = true
			scaffolding[pid] = true
			a, ok := byPid[pid]
			if !ok {
				break
			}
			pid = a.ppid
		}
	}
	// A process of the user with no terminal, working inside one of
	// those places, is adopted: from here it is a candidate like any
	// other, hanging from whatever runs it, or rooting a tree of its
	// own where nothing does.
	for _, p := range procs {
		if candidate[p.pid] || p.uid != uid || p.tty != "" || p.cwd == "" || scaffolding[p.pid] {
			continue
		}
		for root := range worked {
			if within(p.cwd, root) {
				candidate[p.pid] = true
				break
			}
		}
	}
	children := map[int][]int{}
	var roots []int
	for _, p := range procs {
		if !candidate[p.pid] || covered(p) {
			continue
		}
		switch kindOf(p) {
		case kindConn, kindHold:
			continue
		}
		if parent, ok := treeParent(p); ok {
			children[parent] = append(children[parent], p.pid)
		} else {
			roots = append(roots, p.pid)
		}
	}
	// The table is read a process at a time, not all at once, so what
	// comes back can be of two moments: the same pid listed twice, or
	// a pid reused in between leaving a parent that is its own
	// descendant. Neither costs more than a row — walked keeps the
	// first from being written twice, and newest keeps the second from
	// following itself down forever.
	walked := map[int]bool{}

	// Where a thing sits is where it started, and it sits there for as
	// long as it lives. A tree is placed by its own root's start, a row
	// among its siblings by its own, and a place by the first work that
	// began there — oldest first, so what is new goes on the end and
	// nothing above it moves.
	//
	// It was the newest start anywhere in a subtree, which brought fresh
	// work and its whole place to the top. That is a true thing to say
	// about a list and a hard one to read: an agent running a command a
	// second re-sorted the trees, their rows and the places under the
	// eye trying to follow them, and a row read twice was rarely in the
	// same spot. What is worth watching is found by looking, and looking
	// wants the list to hold still.
	//
	// The pid breaks a tie, so two things started in the same instant
	// come out the same way on every reading whatever order the table
	// was read in.
	startedAt := func(pid int) time.Time { return byPid[pid].started }
	byStart := func(pids []int) {
		sort.SliceStable(pids, func(i, j int) bool {
			a, b := startedAt(pids[i]), startedAt(pids[j])
			if a.Equal(b) {
				return pids[i] < pids[j]
			}
			return a.Before(b)
		})
	}
	byStart(roots)
	for pid := range children {
		byStart(children[pid])
	}

	places := map[string]*place{}
	var order []string
	placeAt := map[string]time.Time{}
	var walk func(pid, depth int, path string)
	walk = func(pid, depth int, path string) {
		if walked[pid] {
			return
		}
		walked[pid] = true
		p := byPid[pid]
		kind := kindOf(p)
		e := entry{pid: p.pid, kind: kind, command: commandLine(p), tty: p.tty, started: p.started, depth: depth,
			since: how[p.pid].since, cwd: p.cwd, asking: how[p.pid].asking}
		e.status, e.fault = statusOf(p, kind, len(children[pid]) > 0, how[p.pid])
		if places[path] == nil {
			places[path] = &place{path: path}
			order = append(order, path)
		}
		places[path].entries = append(places[path].entries, e)
		for _, c := range children[pid] {
			walk(c, depth+1, path)
		}
	}
	for _, rootPid := range roots {
		path := rootOf(byPid[rootPid].cwd)
		if t := startedAt(rootPid); placeAt[path].IsZero() || t.Before(placeAt[path]) {
			placeAt[path] = t
		}
		walk(rootPid, 0, path)
	}

	out := make([]place, 0, len(places))
	for _, path := range order {
		out = append(out, *places[path])
	}
	// A place sits where work there began, and the path breaks a tie the
	// way the pid does among rows.
	sort.SliceStable(out, func(i, j int) bool {
		a, b := placeAt[out[i].path], placeAt[out[j].path]
		if a.Equal(b) {
			return out[i].path < out[j].path
		}
		return a.Before(b)
	})
	return out
}

// waitingRound is the order to answer the waiting in: longest held up
// first. An agent that cannot say when it stopped goes last — it is
// waiting, which is what the word is for, but it cannot claim a turn
// ahead of one that can prove it waited longer. The order is the same
// on every reading, so a key stepping through it steps through the
// same ring; ties go by pid rather than by however the table came out.
func waitingRound(places []place) []entry {
	var out []entry
	for _, pl := range places {
		for _, e := range pl.entries {
			if e.status == statusWaiting {
				out = append(out, e)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		switch {
		case a.since.IsZero() != b.since.IsZero():
			return b.since.IsZero() // what cannot say goes last
		case !a.since.Equal(b.since):
			return a.since.Before(b.since)
		default:
			return a.pid < b.pid
		}
	})
	return out
}

// workingShare is what a process has to have kept busy of one
// processor, over the gap between two readings, to be working: a
// twentieth of it. A share and not a figure, since the gap is not
// fixed — a reading can come late, and a slow tick should not make
// everything look busier than it is.
const workingShare = 20

// cpuWorking is every process that spent processor time over the time
// there was to spend it in. What a process has used altogether says
// nothing on its own — a server up for a week has plenty and may be
// doing nothing at all — so what is asked is the difference since the
// last reading.
//
// A process born since that reading has no difference to ask for, and
// is asked against its own life instead. Waiting for a second reading
// would be waiting forever for the ones that matter most: a compiler
// is spawned, works, and is gone well inside the gap between two
// readings, and would read as merely alive for the one moment it was
// ever seen. Its whole life is inside the gap, which is what makes the
// two the same question.
//
// A process older than the last reading that the reading did not have
// is not asked at all. There is no span of conn's to ask about: its
// life is time conn was not watching, and the average over it says
// what the process has been doing all along rather than what it is
// doing now.
//
// The first reading has nothing behind it, so it says nothing is
// working. That is the case above, for every row at once: conn comes
// up on a machine already running, and a dev server that compiled for
// two seconds and has idled ever since would read as working for the
// first beat and correct itself on the second — a reading that lies,
// at the one moment the watch is being read hardest.
func cpuWorking(was map[int]time.Duration, wasAt time.Time, procs []process, nowAt time.Time) map[int]bool {
	busy := map[int]bool{}
	if wasAt.IsZero() {
		return busy
	}
	for _, p := range procs {
		var spent, over time.Duration
		switch before, ok := was[p.pid]; {
		case ok:
			spent, over = p.cpu-before, nowAt.Sub(wasAt)
		case p.started.After(wasAt):
			spent, over = p.cpu, nowAt.Sub(p.started)
		default:
			continue
		}
		if over > 0 && spent > 0 && spent*workingShare >= over {
			busy[p.pid] = true
		}
	}
	return busy
}

// cpuOf is the processor time each process has used, to be held until
// the next reading and asked against.
func cpuOf(procs []process) map[int]time.Duration {
	out := make(map[int]time.Duration, len(procs))
	for _, p := range procs {
		out[p.pid] = p.cpu
	}
	return out
}

// statusOf is the word for a process as it stands. A shell is only
// idle bare, at its prompt; running anything, even nested many levels
// down, it is active the way what it runs is. Working is narrower than
// active and is the one worth watching: the process was doing
// something between one reading and the next, which an agent says of
// itself and anything else is read off the processor time it used.
// Work is not claimed up the tree - a shell whose child is working is
// active, and the row doing the work is the one that says so.
//
// Waiting is the other end of the same question, and the only word
// here that asks something of you: an agent stopped on something it
// put to you and cannot go on without — a permission, a question, a
// dialog waiting to be answered. It is narrower than merely stopped.
// An agent whose turn is simply over is idle, the same word a shell at
// its prompt gets and for the same reason: at rest, nothing pending,
// yours when you want it. The difference is whether anything is held
// up, and only the one that is held up is worth a word that carries.
//
// Nothing but an agent is ever called waiting here - a server waiting
// on a socket is waiting on the socket - so the word is only ever
// about a person. It is no fault, nothing having gone wrong, so it is
// a word of its own rather than a chip.
func statusOf(p process, kind string, hasChildren bool, how standing) (string, bool) {
	switch {
	case p.state == 'T':
		return statusStopped, true
	case p.state == 'Z':
		return statusEnded, true
	case how.waiting:
		return statusWaiting, false
	case how.working:
		return statusWorking, false
	case how.idle:
		return statusIdle, false
	case kind == kindShell && !hasChildren:
		return statusIdle, false
	default:
		return statusActive, false
	}
}

// commandLine is what a process was started as: the program by its base
// name and its arguments, or the program's name alone when the arguments
// could not be read.
func commandLine(p process) string {
	if len(p.args) == 0 {
		return strings.TrimPrefix(filepath.Base(p.command), "-")
	}
	parts := append([]string{strings.TrimPrefix(filepath.Base(p.args[0]), "-")}, p.args[1:]...)
	return strings.Join(parts, " ")
}

// A project is what the watch sorts by: a git repository, or a folder
// under one of conn's roots that holds one. Everything else is in a
// project rather than being one — docs in conn, services/api in the
// monorepo, cmd in either — since a repository is the thing work is
// about and a directory below it is part of that work.
//
// It was a repository or any directory carrying a manifest, which made
// a place of every corner of a repository that happened to run through
// a package manager of its own: conn's own docs directory carried a
// plan and stood apart from conn on the watch, which is not two pieces
// of work and should not have been two blocks.

// within says whether a directory is at or under a root.
func within(dir, root string) bool {
	if root == "" {
		return false
	}
	return dir == root || strings.HasPrefix(dir, root+string(filepath.Separator))
}

// holdsRepo says whether a directory has a repository directly in it —
// the folder the checkouts are kept in. Directly, because a walk of
// everything below would make a project of every directory on the way
// down to a checkout, and the one that holds them is the one that
// stands for them.
func holdsRepo(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if isRepo(filepath.Join(dir, e.Name())) {
			return true
		}
	}
	return false
}

// projectDirs says whether a directory is a project: a repository, or a
// folder under one of conn's roots holding one. The roots are what keeps
// the second half of that in hand — any folder anywhere with a checkout
// somewhere below it would make a project of your home directory, and a
// shell sitting there would take in every daemon on the machine with it.
// A root is not itself a project, whatever is kept in it: it is where
// the checkouts live, which is the same line the projects list draws
// when it groups them. It remembers what it found, since the watch asks
// after the same directories on every reading.
func projectDirs(roots []string) func(string) bool {
	known := map[string]bool{}
	return func(dir string) bool {
		if dir == "" {
			return false
		}
		if is, ok := known[dir]; ok {
			return is
		}
		is := isRepo(dir)
		if !is {
			for _, root := range roots {
				if dir != root && within(dir, root) {
					is = holdsRepo(dir)
					break
				}
			}
		}
		known[dir] = is
		return is
	}
}

// placeRoots finds the place that holds a directory: the nearest project
// at or above it — the repository the work is in, or the folder the
// checkouts are kept in where the work is beside them rather than inside
// one. A directory with no project above it stands for itself.
//
// placeRoots remembers what it found, since the watch asks for the same
// directories on every read.
func placeRoots(isProject func(string) bool) func(string) string {
	known := map[string]string{}
	return func(dir string) string {
		if dir == "" {
			return ""
		}
		if root, ok := known[dir]; ok {
			return root
		}
		root := dir
		for d := dir; ; d = filepath.Dir(d) {
			if isProject(d) {
				root = d
				break
			}
			if filepath.Dir(d) == d {
				break
			}
		}
		known[dir] = root
		return root
	}
}

// age is how long since a time, in the two largest units that apply.
func age(since, now time.Time) string {
	if since.IsZero() {
		return ""
	}
	return spell(now.Sub(since))
}

// spell writes a span the way the watch's age column does, for a span
// that is not the distance from a moment to now: processor time spent,
// say, which has no moment to count from.
func spell(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	secs := int(d.Seconds()) % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dD %02dH", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dH %02dM", hours, mins)
	case mins > 0:
		return fmt.Sprintf("%dM %02dS", mins, secs)
	default:
		return fmt.Sprintf("%dS", secs)
	}
}
