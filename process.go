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

// The words an entry stands under. A live process is ACTIVE whether the
// kernel caught it on a processor or asleep: the instant says nothing,
// and macOS calls nearly everything runnable.
const (
	statusActive  = "ACTIVE"  // alive, at its work
	statusIdle    = "IDLE"    // a shell at its prompt
	statusStopped = "STOPPED" // suspended
	statusEnded   = "ENDED"   // finished, and not yet collected
)

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
	fault   bool // a status to be looked at: STOPPED, ENDED
	depth   int  // how deep under its place's own root; the root at 0
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
func watch(procs []process, uid int, rootOf func(string) string) []place {
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

	// newest is the latest a pid or anything under it started: what
	// orders a tree among its siblings, and a place among the others —
	// fresh work under a shell open for hours still counts as fresh.
	memo := map[int]time.Time{}
	var newest func(pid int, seen map[int]bool) time.Time
	newest = func(pid int, seen map[int]bool) time.Time {
		if t, ok := memo[pid]; ok {
			return t
		}
		if seen[pid] {
			return byPid[pid].started
		}
		seen[pid] = true
		t := byPid[pid].started
		for _, c := range children[pid] {
			if ct := newest(c, seen); ct.After(t) {
				t = ct
			}
		}
		memo[pid] = t
		return t
	}
	newestOf := func(pid int) time.Time { return newest(pid, map[int]bool{}) }
	sortNewest := func(pids []int) {
		sort.SliceStable(pids, func(i, j int) bool { return newestOf(pids[i]).After(newestOf(pids[j])) })
	}
	sortNewest(roots)
	for pid := range children {
		sortNewest(children[pid])
	}

	places := map[string]*place{}
	var order []string
	placeNewest := map[string]time.Time{}
	var walk func(pid, depth int, path string)
	walk = func(pid, depth int, path string) {
		if walked[pid] {
			return
		}
		walked[pid] = true
		p := byPid[pid]
		kind := kindOf(p)
		e := entry{pid: p.pid, kind: kind, command: commandLine(p), tty: p.tty, started: p.started, depth: depth}
		e.status, e.fault = statusOf(p, kind, len(children[pid]) > 0)
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
		if t := newestOf(rootPid); t.After(placeNewest[path]) {
			placeNewest[path] = t
		}
		walk(rootPid, 0, path)
	}

	out := make([]place, 0, len(places))
	for _, path := range order {
		out = append(out, *places[path])
	}
	// The newest work first, anywhere in a place's trees.
	sort.SliceStable(out, func(i, j int) bool {
		return placeNewest[out[i].path].After(placeNewest[out[j].path])
	})
	return out
}

// statusOf is the word for a process as it stands. A shell is only
// idle bare, at its prompt; running anything, even nested many levels
// down, it is active the way what it runs is.
func statusOf(p process, kind string, hasChildren bool) (string, bool) {
	switch {
	case p.state == 'T':
		return statusStopped, true
	case p.state == 'Z':
		return statusEnded, true
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

// manifests are the files that mark a directory as a project of its own:
// a plan of conn's, then the file a package manager runs the project by,
// since most projects run through one and the manifest is where that is
// said.
var manifests = []string{
	".conn", "Procfile",
	"package.json", "deno.json", "composer.json",
	"go.mod", "Cargo.toml", "Gemfile", "mix.exs",
	"pyproject.toml", "pom.xml", "build.gradle", "build.gradle.kts",
}

// hasManifest says whether a directory carries one of them.
func hasManifest(dir string) bool {
	for _, f := range manifests {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			return true
		}
	}
	return false
}

// placeRoots finds the place that holds a directory: the nearest ancestor
// with a .git in it, and within that repository the nearest directory
// from where the work happens up to it — not counting the repository
// itself, whose own row already stands for its manifest — that carries a
// manifest. A monorepo's apps and services are places of their own, and
// the manifest is what says so. Outside every repository the directory
// stands for itself.
//
// The process makes the sub-project, and it is made from where the
// process is, not from an index: a manifest git ignores, or one written
// a minute ago, marks its directory the same as one a scan would have
// listed. placeRoots remembers what it found, since the watch asks for
// the same directories on every read.
func placeRoots() func(string) string {
	known := map[string]string{}
	return func(dir string) string {
		if dir == "" {
			return ""
		}
		if root, ok := known[dir]; ok {
			return root
		}
		root, sub, repo := dir, "", false
		for d := dir; ; d = filepath.Dir(d) {
			if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
				root, repo = d, true
				break
			}
			if sub == "" && hasManifest(d) {
				sub = d
			}
			if filepath.Dir(d) == d {
				break
			}
		}
		if repo && sub != "" {
			root = sub
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
	d := now.Sub(since)
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
