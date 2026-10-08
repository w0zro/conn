// Package work reads the work on this machine and files it by project:
// the process table and what each process has open, the containers
// docker holds up and the services brew does, what each contact is
// doing and the sessions it left, and what each project's .conn says
// should be running. It reads; what the panel makes of it is main's.
package work

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// A Process, as the kernel describes it: who runs it, what terminal it
// holds, where it is working, what it was started as and when. The
// platform files read the table; the processes view is composed from
// it.
type Process struct {
	PID, PPID, PGID int
	UID             int
	TTY             string // ttys004, pts/3; blank without a terminal
	Foreground      bool   // its group holds the terminal
	State           byte   // R running, S sleeping, T stopped, Z ended, I idle, D in disk wait
	Started         time.Time
	Command         string   // the program's name
	Args            []string // what it was started as, when that could be read
	Cwd             string
	// cpu is all the processor time this process has used, which says
	// nothing on its own: what it has used since the last reading is
	// how the processes view tells work from waiting.
	CPU time.Duration
	// What it has open to the world: the ports it listens on, the
	// connections it holds, the unix sockets it has by path; see
	// sockets.go.
	Sockets []Socket
}

// The kinds of process the processes view tells apart, by the program's
// name. Everything else is a run: a build, a test, a server, a script.
const (
	KindShell   = "SHELL"
	KindContact = "CONTACT"
	KindEditor  = "EDITOR"
	KindConn    = "CONN" // conn itself; not in the processes view
	KindRun     = "RUN"
	kindHold    = "HOLD" // conn standing in an empty bay; not in the processes view
)

// KindService is what a service's row is called: a container, or a
// service brew holds. It is not a RUN: a run is a program on this
// machine with a terminal above it somewhere, and a service is a thing
// docker or brew is holding up on your behalf.
const KindService = "SERVICE"

// A name is a contact's when the name means an agent and means little
// else. The word carries more here than the other kinds do — it says
// there is a mind at the other end, that the row can stop and wait on
// you, and that tab is for it — so a name that is as likely to be
// something ordinary has not earned it, and the thing it names is a
// run like any other program conn does not recognise.
//
// goose was here and is gone: it is a database migration tool as much
// as it is an agent, and goose up in a repository is the commoner of
// the two. ollama was here and is gone: it names a model runner whose
// processes are a server and a download, and conn would have called a
// daemon a contact.
var (
	shells  = []string{"zsh", "bash", "fish", "sh", "dash", "nu", "tcsh", "ksh"}
	editors = []string{"vim", "nvim", "vi", "hx", "helix", "emacs", "nano", "micro", "kak"}
)

// Who a contact is with: the agent the program is, and whose it is.
// The station sees a process called claude and nothing else; that it
// is Claude Code, and Anthropic's, is a fact about the program conn
// matched by name, the same kind of thing as knowing zsh is a shell.
// A maker conn is not sure of is left off rather than guessed at: the
// agent's own name is the part that answers the question.
type agent struct{ Name, Maker string }

var Contacts = map[string]agent{
	"claude":   {"Claude Code", "Anthropic"},
	"codex":    {"Codex", "OpenAI"},
	"gemini":   {"Gemini CLI", "Google"},
	"copilot":  {"Copilot CLI", "GitHub"},
	"amp":      {"Amp", "Sourcegraph"},
	"opencode": {"OpenCode", "SST"},
	"aider":    {"Aider", ""},
}

// isContact says whether a program's name is an agent's.
func isContact(name string) bool {
	_, ok := Contacts[name]
	return ok
}

// KindOf is the kind of a process, from the name of its program. A
// program that writes its own title puts its name first and what it is
// at after it — claude bg-spare is claude, at its spare work — so the
// name is the first word of what it was started as.
func KindOf(p Process) string {
	name := strings.TrimPrefix(filepath.Base(p.Command), "-")
	if len(p.Args) > 0 {
		name = strings.TrimPrefix(filepath.Base(p.Args[0]), "-")
	}
	name, _, _ = strings.Cut(name, " ")
	switch {
	case name == "conn" && len(p.Args) > 1 && p.Args[1] == "hold":
		return kindHold
	case name == "conn":
		return KindConn
	case slices.Contains(shells, name):
		return KindShell
	case isContact(name):
		return KindContact
	case slices.Contains(editors, name):
		return KindEditor
	default:
		return KindRun
	}
}

// The words an entry stands under. Whether the kernel caught a process
// on a processor or asleep says nothing by itself - macOS calls nearly
// everything runnable - so alive alone is ACTIVE, and WORKING is kept
// for a process that did something between one reading and the next.
const (
	StatusWorking = "WORKING" // doing something, right now
	StatusWaiting = "WAITING" // a contact stopped on an ask it put to you
	StatusActive  = "ACTIVE"  // alive, and not doing anything
	StatusIdle    = "IDLE"    // a shell at its prompt, or a contact at rest
	StatusStopped = "STOPPED" // suspended
	StatusEnded   = "ENDED"   // finished, and not yet collected
	StatusDown    = "DOWN"    // declared in the project's .conn, and not running
	StatusClosed  = "CLOSED"  // it was listening, and the listener has gone while it lives
)

// exitWord is what a status that ended with a code begins with.
const exitWord = "EXIT "

// Said is a status as a row says it. The vocabulary is the machine's and
// stays in capitals wherever conn reasons about it — the manual's table
// of every word a row can say is that list — and what the operator
// reads is a word: Working, Waiting, Stopped. Capitals are for labels,
// and a status is not a label but a fact about a thing.
func Said(status string) string {
	if status == "" {
		return ""
	}
	return strings.ToUpper(status[:1]) + strings.ToLower(status[1:])
}

// Status is what conn learned about a process past what the table
// says of it. Anything can be working, read off the processor time it
// spent. Only a contact says more, being the only thing here that knows
// its own mind: mid-turn, stopped on an ask it put to you, or stopped
// with its turn over and nothing pending.
type Status struct {
	Working bool
	Waiting bool   // stopped on something it asked of you
	Idle    bool   // stopped with its turn over, asking nothing
	Asking  string // what a waiting contact is stopped on, in its own words
	// When it came to stand this way, where it says so; zero where it
	// does not. Only a contact knows the moment it stopped, and only
	// waiting is worth the moment: how long a thing has been held up on
	// you is the order to answer it in.
	Since time.Time
}

// An Entry is a row of the processes view: one process, standing for
// its own work, at its project in the tree the processes it is among
// actually are.
type Entry struct {
	PID     int
	Kind    string
	Command string // what it was started as, the program by its base name
	Typed   string // the same less what conn itself added, which is what was typed
	TTY     string
	Started time.Time
	Status  string
	Fault   bool      // a status to be looked at: STOPPED, ENDED
	Depth   int       // how deep under its project's own root; the root at 0
	Since   time.Time // when it came to stand as it does, where that is known
	// What the processes view has no column for and the readout reads:
	// where the process itself is, whatever project its tree belongs to,
	// and what a contact says it is stopped on.
	Cwd    string
	Asking string
	// What it has open to the world, for the page; see sockets.go. The
	// TCP ports it listens on are the row's own fact, said after its
	// command and filing it under SERVING; a service carries the ports
	// it publishes on the host here.
	Sockets []Socket
	Ports   []string
	// What a working contact is doing, read off its transcript: the
	// tool it has in flight, as a verb and an object.
	Doing string
	// What a contact's session is about, read off its transcript: the
	// title Claude Code gave it from its first prompt, or the name it
	// was renamed to. It is the row's label while the contact is not
	// working, since claude says nothing and the pid says less.
	Title string
	// The tokens a contact's latest turn carried, read off its
	// transcript with its title; see carried.
	Carried int
	// The container this row is, where it is one: the id docker knows it
	// by, which the keys act on. A process row carries nothing here.
	Container string
	// The declaration this row is, or stands for, as its pane is marked;
	// see declared.go. A process row carries nothing here.
	Declared string
	// The Homebrew service this row is, by its formula, where it is one
	// a project declares; see brew.go. And how many projects declare
	// it, where more than one does: the panel files them as one row,
	// and says * for the project.
	Brew   string
	Shared int
	// What runs under a shell whose rows are folded, for its activity
	// column; see fold.go. underShell says it is a shell itself, and a
	// command found later under it is taken instead. underKind is that
	// command's own kind, which the row wears on the panel: a shell
	// standing for the vim it runs is an editor there, and one standing
	// for a build is work. See panelKind.
	Under      string
	UnderShell bool
	UnderKind  string
	// The command of the one listener folded into this row, whose
	// ports and sockets the row carries; see fold.go. A program conn
	// knows by name is known here too, so the row takes the client.
	Listener string
}

// A Project is a directory work is happening in, and the entries at it.
type Project struct {
	Path    string // as read; the processes view writes it from ~
	Entries []Entry
	Note    string // what is wrong with the project's .conn, where something is
}

// ProjectsFrom composes the projects from the process table: the
// processes of one user with a terminal, each standing for its own
// work, nested under whatever candidate process runs it — the tree they
// actually are, rather than one leaf apiece. rootOf turns a working
// directory into the project that holds it; a whole tree is one
// project's, the root's own directory, whatever a process under it has
// since cd'd to.
//
// conn is not in the processes view, and neither is what it holds. It
// is the instrument, not the work — the one conn you are looking at,
// the conn behind it holding the terminal, and the hold standing in an
// empty bay alike — and it covers what runs under it, so the tmux
// client it holds is no more a row than conn is. A contact and an
// editor cover nothing: what they run is work, and reads as theirs. The
// rule goes by the program's name, so a conn on another socket, or an
// older conn installed beside this one, is off the processes view too.
//
// A terminal is how the processes view tells your work from the
// machine's, and it is most of it, but not all: a server you left
// running has no terminal and is work all the same. So a process of
// yours without one is adopted where it works inside a project you have
// terminal work in - the dev server under the contact that started it,
// and the one from last week that outlived its shell alike. The project
// is what makes that safe. A project that is merely a directory adopts
// nothing, since a shell sitting at home would otherwise take in every
// daemon on the machine with it. What conn is held in is not work
// either: the tmux server conn runs inside has no terminal and works in
// the repository like anything else there, and is no more a row than
// conn is.
func ProjectsFrom(procs []Process, uid int, rootOf func(string) string, isProject func(string) bool, how map[int]Status) []Project {
	byPid := map[int]Process{}
	for _, p := range procs {
		byPid[p.PID] = p
	}
	// holding is what conn is standing on: the shell the operator typed
	// conn into, and whatever stands between that shell and conn — a go
	// run in development, a wrapper script, a login shell under it.
	//
	// conn takes the terminal over, so that shell is blocked behind it
	// for as long as conn runs, in whatever directory it happened to be
	// in, which for most people is home. It was a row: a shell doing
	// nothing, at a project nobody is working in, which cannot be gone
	// to because going to it is what conn already is. It is part of the
	// instrument the same as the client conn holds, and the only
	// difference between the two is which side of conn they stand on.
	//
	// The walk climbs from each conn and stops the moment the terminal
	// changes, which is where the lineage leaves the terminal conn was
	// launched from. A job the operator suspended in that shell before
	// starting conn is not on the way up from conn and stays a row,
	// which is right: it is work, and it is waiting for them.
	holding := map[int]bool{}
	for _, p := range procs {
		if p.UID != uid || p.TTY == "" {
			continue
		}
		if k := KindOf(p); k != KindConn && k != kindHold {
			continue
		}
		seen := map[int]bool{}
		for pid := p.PPID; pid > 0 && !seen[pid]; {
			seen[pid] = true
			a, ok := byPid[pid]
			if !ok || a.TTY != p.TTY {
				break
			}
			holding[a.PID] = true
			pid = a.PPID
		}
	}
	// A candidate is a process of the user with a terminal, that conn is
	// not itself standing on.
	candidate := map[int]bool{}
	for _, p := range procs {
		candidate[p.PID] = p.UID == uid && p.TTY != "" && !holding[p.PID]
	}
	// covered says whether conn stands anywhere above a process: the tmux
	// client conn holds is conn's own doing, not work of yours, and goes
	// off the processes view with it rather than hanging from whatever
	// happens to be above conn.
	covered := func(p Process) bool {
		seen := map[int]bool{}
		for pid := p.PPID; pid > 0 && !seen[pid]; {
			seen[pid] = true
			a, ok := byPid[pid]
			if !ok {
				return false
			}
			if candidate[a.PID] {
				switch KindOf(a) {
				case KindConn, kindHold:
					return true
				}
			}
			pid = a.PPID
		}
		return false
	}
	// treeParent is the nearest candidate ancestor a process hangs
	// from, climbing past whatever is not one itself. Nothing covered
	// gets this far, so no ancestor it can find is conn's.
	treeParent := func(p Process) (int, bool) {
		seen := map[int]bool{}
		for pid := p.PPID; pid > 0 && !seen[pid]; {
			seen[pid] = true
			a, ok := byPid[pid]
			if !ok {
				return 0, false
			}
			if candidate[a.PID] {
				return a.PID, true
			}
			pid = a.PPID
		}
		return 0, false
	}
	// The projects terminal work has established, kept to those that are
	// projects in their own right: work of yours in a project is what
	// lets the processes view speak for anything else running there.
	worked := map[string]bool{}
	for _, p := range procs {
		if !candidate[p.PID] || covered(p) || p.Cwd == "" {
			continue
		}
		switch KindOf(p) {
		case KindConn, kindHold:
			continue
		}
		if root := rootOf(p.Cwd); isProject(root) {
			worked[root] = true
		}
	}
	// conn's own scaffolding: whatever conn runs beneath. Only what is
	// up for adoption is asked, so a process with a terminal still
	// stands for itself - the shell you started conn from is your
	// shell, and stays a row.
	scaffolding := map[int]bool{}
	for _, p := range procs {
		switch KindOf(p) {
		case KindConn, kindHold:
		default:
			continue
		}
		seen := map[int]bool{}
		for pid := p.PPID; pid > 0 && !seen[pid]; {
			seen[pid] = true
			scaffolding[pid] = true
			a, ok := byPid[pid]
			if !ok {
				break
			}
			pid = a.PPID
		}
	}
	// A process of the user with no terminal, working inside one of
	// those projects, is adopted: from here it is a candidate like any
	// other, hanging from whatever runs it, or rooting a tree of its
	// own where nothing does.
	for _, p := range procs {
		if candidate[p.PID] || p.UID != uid || p.TTY != "" || p.Cwd == "" || scaffolding[p.PID] {
			continue
		}
		for root := range worked {
			if Within(p.Cwd, root) {
				candidate[p.PID] = true
				break
			}
		}
	}
	children := map[int][]int{}
	var roots []int
	for _, p := range procs {
		if !candidate[p.PID] || covered(p) {
			continue
		}
		switch KindOf(p) {
		case KindConn, kindHold:
			continue
		}
		if parent, ok := treeParent(p); ok {
			children[parent] = append(children[parent], p.PID)
		} else {
			roots = append(roots, p.PID)
		}
	}
	// The table is read a process at a time, not all at once, so what
	// comes back can be of two moments: the same pid listed twice, or
	// a pid reused in between leaving a parent that is its own
	// descendant. Neither costs more than a row — walked keeps the
	// first from being written twice, and newest keeps the second from
	// following itself down forever.
	walked := map[int]bool{}

	// Within a project, where a thing sits is where it started, and it
	// sits there for as long as it lives. A tree is placed by its own
	// root's start and a row among its siblings by its own — oldest
	// first, so what is new goes on the end and nothing above it moves.
	//
	// It was the newest start anywhere in a subtree, which brought fresh
	// work and its whole project to the top. That is a true thing to say
	// about a list and a hard one to read: a contact running a command a
	// second re-sorted the trees, their rows and the projects under the
	// eye trying to follow them, and a row read twice was rarely in the
	// same spot. What is worth watching is found by looking, and looking
	// wants the list to hold still.
	//
	// The pid breaks a tie, so two things started in the same instant
	// come out the same way on every reading whatever order the table
	// was read in.
	startedAt := func(pid int) time.Time { return byPid[pid].Started }
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

	projects := map[string]*Project{}
	var order []string
	var walk func(pid, depth int, path string)
	walk = func(pid, depth int, path string) {
		if walked[pid] {
			return
		}
		walked[pid] = true
		p := byPid[pid]
		kind := KindOf(p)
		e := Entry{PID: p.PID, Kind: kind, Command: commandLine(p), Typed: typedLine(p), TTY: p.TTY, Started: p.Started, Depth: depth,
			Since: how[p.PID].Since, Cwd: p.Cwd, Asking: how[p.PID].Asking, Sockets: p.Sockets, Ports: ListeningPorts(p.Sockets)}
		e.Status, e.Fault = statusOf(p, kind, len(children[pid]) > 0, how[p.PID])
		if projects[path] == nil {
			projects[path] = &Project{Path: path}
			order = append(order, path)
		}
		projects[path].Entries = append(projects[path].Entries, e)
		for _, c := range children[pid] {
			walk(c, depth+1, path)
		}
	}
	for _, rootPid := range roots {
		walk(rootPid, 0, rootOf(byPid[rootPid].Cwd))
	}

	out := make([]Project, 0, len(projects))
	for _, path := range order {
		out = append(out, *projects[path])
	}
	// The projects are in the order of their paths, which is the order
	// the list has them in, so a project is in the same place on every
	// reading and every day whatever began there first. They sat where
	// work there began before, which put the same project at the top one
	// day and at the bottom the next, and the view was learnt again each
	// morning.
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// WaitingRound is the order to answer the waiting in: longest held up
// first. A contact that cannot say when it stopped goes last — it is
// waiting, which is what the word is for, but it cannot claim a turn
// ahead of one that can prove it waited longer. The order is the same
// on every reading, so a key stepping through it steps through the
// same ring; ties go by pid rather than by however the table came out.
func WaitingRound(projects []Project) []Entry {
	var out []Entry
	for _, pl := range projects {
		for _, e := range pl.Entries {
			if e.Status == StatusWaiting {
				out = append(out, e)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		switch {
		case a.Since.IsZero() != b.Since.IsZero():
			return b.Since.IsZero() // what cannot say goes last
		case !a.Since.Equal(b.Since):
			return a.Since.Before(b.Since)
		default:
			return a.PID < b.PID
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

// CpuWorking is every process that spent processor time over the time
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
// at the one moment the processes view is being read hardest.
func CpuWorking(was map[int]time.Duration, wasAt time.Time, procs []Process, nowAt time.Time) map[int]bool {
	busy := map[int]bool{}
	if wasAt.IsZero() {
		return busy
	}
	for _, p := range procs {
		var spent, over time.Duration
		switch before, ok := was[p.PID]; {
		case ok:
			spent, over = p.CPU-before, nowAt.Sub(wasAt)
		case p.Started.After(wasAt):
			spent, over = p.CPU, nowAt.Sub(p.Started)
		default:
			continue
		}
		if over > 0 && spent > 0 && spent*workingShare >= over {
			busy[p.PID] = true
		}
	}
	return busy
}

// CpuOf is the processor time each process has used, to be held until
// the next reading and asked against.
func CpuOf(procs []Process) map[int]time.Duration {
	out := make(map[int]time.Duration, len(procs))
	for _, p := range procs {
		out[p.PID] = p.CPU
	}
	return out
}

// statusOf is the word for a process as it stands. A shell is only
// idle bare, at its prompt; running anything, even nested many levels
// down, it is active the way what it runs is. Working is narrower than
// active and is the one worth watching: the process was doing
// something between one reading and the next, which a contact says of
// itself and anything else is read off the processor time it used.
// Work is not claimed up the tree - a shell whose child is working is
// active, and the row doing the work is the one that says so.
//
// Waiting is the other end of the same question, and the only word
// here that asks something of you: a contact stopped on something it
// put to you and cannot go on without — a permission, a question, a
// dialog waiting to be answered. It is narrower than merely stopped.
// A contact whose turn is simply over is idle, the same word a shell at
// its prompt gets and for the same reason: at rest, nothing pending,
// yours when you want it. The difference is whether anything is held
// up, and only the one that is held up is worth a word that carries.
//
// Nothing but a contact is ever called waiting here - a server waiting
// on a socket is waiting on the socket - so the word is only ever
// about a person. It is no fault, nothing having gone wrong, so it is
// a word of its own rather than a chip.
func statusOf(p Process, kind string, hasChildren bool, how Status) (string, bool) {
	switch {
	case p.State == 'T':
		return StatusStopped, true
	case p.State == 'Z':
		return StatusEnded, true
	case how.Waiting:
		return StatusWaiting, false
	case how.Working:
		return StatusWorking, false
	case how.Idle:
		return StatusIdle, false
	case kind == KindShell && !hasChildren:
		return StatusIdle, false
	default:
		return StatusActive, false
	}
}

// commandLine is what a process was started as: the program by its base
// name and its arguments, or the program's name alone when the arguments
// could not be read.
func commandLine(p Process) string {
	return strings.Join(commandWords(p, false), " ")
}

// typedLine is the command as the operator typed it: the same words
// less the argument conn adds when it starts a contact. A contact conn
// raised read on the watch as claude --app…, which was conn showing the
// operator the noise conn itself had made. The readout keeps the whole
// line, being where the whole of anything goes.
func typedLine(p Process) string {
	return strings.Join(commandWords(p, true), " ")
}

// ownFlags are the arguments conn adds to a contact's command line,
// each taking a value of its own: the note, and the session picked
// back up. Both are conn's doing, and neither says anything a row
// should: a resumed session is named by its title, not its id.
var ownFlags = []string{"--append-system-prompt", "--resume"}

// ownFlag says whether an argument is one of conn's own, and whether
// its value is joined to it with an equals sign.
func ownFlag(a string) (own, joined bool) {
	for _, f := range ownFlags {
		if a == f {
			return true, false
		}
		if strings.HasPrefix(a, f+"=") {
			return true, true
		}
	}
	return false, false
}

// commandWords is the command as words, one an argument. An argument
// is one line however it was written: a python -c handed a script
// with newlines in it is one process, and its row is one row, where
// the newline written out took the rows under it down with it.
func commandWords(p Process, lessOwn bool) []string {
	if len(p.Args) == 0 {
		return []string{strings.TrimPrefix(filepath.Base(p.Command), "-")}
	}
	words := []string{strings.TrimPrefix(filepath.Base(p.Args[0]), "-")}
	for i := 1; i < len(p.Args); i++ {
		a := p.Args[i]
		if lessOwn {
			if own, joined := ownFlag(a); own {
				if !joined {
					i++
				}
				continue
			}
		}
		words = append(words, oneLine(a))
	}
	return words
}

// oneLine is a string with its whitespace, newlines among it, run
// together to single spaces.
func oneLine(s string) string {
	if !strings.ContainsAny(s, "\n\r\t\v\f") {
		return s
	}
	return strings.Join(strings.Fields(s), " ")
}

// AsTyped is the command as typed, or as written where nothing was
// read of what was typed.
func (e Entry) AsTyped() string {
	if e.Typed != "" {
		return e.Typed
	}
	return e.Command
}

// Program is a command line's first word: what a process is called by
// where the whole line is too much, as on the kill's question.
func Program(command string) string {
	if f := strings.Fields(command); len(f) > 0 {
		return f[0]
	}
	return command
}

// A project is what the processes view sorts by: a git repository, or a
// folder under one of conn's roots that holds one. Everything else is
// in a project rather than being one — docs in conn, services/api in
// the monorepo, cmd in either — since a repository is the thing work is
// about and a directory below it is part of that work.
//
// It was a repository or any directory carrying a manifest, which made
// a project of every corner of a repository that happened to run
// through a package manager of its own: conn's own docs directory stood
// apart from conn in the processes view, which is not two pieces of
// work and should not have been two blocks.

// Within says whether a directory is at or under a root.
func Within(dir, root string) bool {
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
		if IsRepo(filepath.Join(dir, e.Name())) {
			return true
		}
	}
	return false
}

// ProjectDirs says whether a directory is a project: a repository, or a
// folder under one of conn's roots holding one. The roots are what
// keeps the second half of that honest — any folder anywhere with a
// checkout somewhere below it would make a project of your home
// directory, and a shell sitting there would take in every daemon on
// the machine with it. A root is not itself a project, whatever is kept
// in it: it is where the checkouts live, which is the same line the
// projects list draws when it groups them. It remembers what it found,
// since the processes view asks after the same directories on every
// reading.
func ProjectDirs(roots []string) func(string) bool {
	known := map[string]bool{}
	return func(dir string) bool {
		if dir == "" {
			return false
		}
		if is, ok := known[dir]; ok {
			return is
		}
		is := IsRepo(dir)
		if !is {
			for _, root := range roots {
				if dir != root && Within(dir, root) {
					is = holdsRepo(dir)
					break
				}
			}
		}
		known[dir] = is
		return is
	}
}

// RootFinder finds the project that holds a directory: the nearest
// project at or above it — the repository the work is in, or the folder
// the checkouts are kept in where the work is beside them rather than
// inside one. A directory with no project above it stands for itself.
//
// rootFinder remembers what it found, since the processes view asks for
// the same directories on every read.
func RootFinder(isProject func(string) bool) func(string) string {
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

// Age is how long since a time, in the two largest units that apply.
func Age(since, now time.Time) string {
	if since.IsZero() {
		return ""
	}
	return Spell(now.Sub(since))
}

// Minutes is how long a wait has stood, as the panel says it beside
// the row: in minutes under an hour, since a wait is answered in
// minutes, and above that as the hours and minutes are spelled.
func Minutes(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Hour {
		return strconv.Itoa(int(d.Minutes())) + " min"
	}
	return strings.ToLower(Spell(d))
}

// SinceWord is how long a row has stood as it does, for the processes
// view's column: the one largest unit that applies, and nothing where
// the moment is not known. Two units were four columns of precision the
// column is not read for; the number is glanced at, against the status
// beside it.
func SinceWord(since, now time.Time) string {
	if since.IsZero() {
		return ""
	}
	return Brief(now.Sub(since))
}

// Brief writes a span in its one largest unit.
func Brief(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d >= 24*time.Hour:
		return fmt.Sprintf("%dD", int(d.Hours())/24)
	case d >= time.Hour:
		return fmt.Sprintf("%dH", int(d.Hours()))
	case d >= time.Minute:
		return fmt.Sprintf("%dM", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dS", int(d.Seconds()))
	}
}

// Stood is a row as conn last saw it stand: its status, when it took
// it, and when its process began, so a pid come round again is not
// taken for the process that had it.
type Stood struct {
	status  string
	at      time.Time
	started time.Time
}

// SinceSeen fills in when each row came to stand as it does, where the
// row does not say so itself, and answers what to hold for the next
// reading. A contact says its own moment and keeps it. Anything else is
// dated by conn's own eye: a row whose status differs from the last
// reading changed between the two, and is dated now; one born since the
// last reading has stood as it does since it began; one that stands as
// it did keeps the moment it had. What conn was not watching it has no
// moment for, and says nothing: the first reading dates nothing, and a
// shell idle since before conn came up stays undated until it changes.
func SinceSeen(projects []Project, was map[int]Stood, wasAt, now time.Time) map[int]Stood {
	next := map[int]Stood{}
	for i := range projects {
		for j := range projects[i].Entries {
			e := &projects[i].Entries[j]
			if e.Since.IsZero() {
				prev, ok := was[e.PID]
				switch {
				case ok && prev.started.Equal(e.Started) && prev.status == e.Status:
					e.Since = prev.at
				case ok && prev.started.Equal(e.Started):
					e.Since = now
				case !wasAt.IsZero() && e.Started.After(wasAt):
					e.Since = e.Started
				}
			}
			next[e.PID] = Stood{status: e.Status, at: e.Since, started: e.Started}
		}
	}
	return next
}

// Spell writes a span the way the processes view's age column does, for
// a span that is not the distance from a moment to now: processor time
// spent, say, which has no moment to count from.
func Spell(d time.Duration) string {
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

// WithoutConnsOwn is the table less what is conn's own doing and not
// the operator's: the panes watching a service, whose tail is the
// service's row and not a row beside it; and whatever else is on the
// panel's own terminal. conn is the one thing that runs there, and a
// process that carries that terminal is one conn asked for — brew,
// whose ruby forks a curl for its analytics and exits without waiting
// for it. covered cannot find conn above a child conn has let go of,
// but the terminal it carries is enough to know it by. conn itself
// stays: the shell it was launched from is held by walking up from it.
func WithoutConnsOwn(procs []Process, watching map[string]bool, panelTTY string) []Process {
	if len(watching) == 0 && panelTTY == "" {
		return procs
	}
	kept := procs[:0]
	for _, p := range procs {
		if watching[p.TTY] {
			continue
		}
		if p.TTY == panelTTY && KindOf(p) != KindConn {
			continue
		}
		kept = append(kept, p)
	}
	return kept
}

// IsRepo says whether a directory is the top of a git repository. .git
// is a directory in a clone and a file in a worktree or a submodule.
func IsRepo(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// Over says whether a row is not running: declared and never came up,
// or ended, cleanly or with a code. Its command is struck through.
func Over(status string) bool {
	switch status {
	case StatusDown, StatusEnded:
		return true
	}
	return strings.HasPrefix(status, exitWord)
}

// Faulty says whether a word is a fault's: a thing to look at, which
// the panel stamps. The panel knows a fault by its row; the log knows
// it by the word alone, which is all a line carries.
func Faulty(word string) bool {
	switch {
	case word == StatusStopped, word == StatusEnded, word == StatusClosed:
		return true
	case strings.HasPrefix(word, exitWord):
		return true
	case SaidWords[word]:
		return true
	}
	return false
}
