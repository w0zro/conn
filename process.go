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
	kindConn   = "CONN"
	kindRun    = "RUN"
	kindHold   = "HOLD" // conn standing in an empty slot; not on the watch
)

var (
	shells  = []string{"zsh", "bash", "fish", "sh", "dash", "nu", "tcsh", "ksh"}
	agents  = []string{"claude", "codex", "gemini", "aider", "opencode", "goose", "amp", "copilot", "ollama"}
	editors = []string{"vim", "nvim", "vi", "hx", "helix", "emacs", "nano", "micro", "kak"}
)

// kindOf is the kind of a process, from the name of its program.
func kindOf(p process) string {
	name := strings.TrimPrefix(filepath.Base(p.command), "-")
	if len(p.args) > 0 {
		name = strings.TrimPrefix(filepath.Base(p.args[0]), "-")
	}
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
	statusHere    = "HERE"    // this conn
	statusActive  = "ACTIVE"  // alive, at its work
	statusIdle    = "IDLE"    // a shell at its prompt
	statusStopped = "STOPPED" // suspended
	statusEnded   = "ENDED"   // finished, and not yet collected
)

// An entry is a row of the watch: one process that stands for the work
// it is doing.
type entry struct {
	pid     int
	kind    string
	command string // what it was started as, the program by its base name
	tty     string
	started time.Time
	status  string
	fault   bool // a status to be looked at: STOPPED, ENDED
}

// A place is a directory work is happening in, and the entries at it.
type place struct {
	path    string // as read; the watch writes it from ~
	entries []entry
}

// watch composes the places from the process table: the processes of one
// user with a terminal, each standing for its work. A shell shows only
// when it is idle, with nothing of its own on the watch; an agent, an
// editor and conn show and cover what they run; anything else shows when
// it is the leaf of its tree. rootOf turns a working directory into the
// place that holds it.
func watch(procs []process, self, uid int, rootOf func(string) string) []place {
	byPid := map[int]process{}
	for _, p := range procs {
		byPid[p.pid] = p
	}
	// A candidate is a process of the user with a terminal.
	candidate := map[int]bool{}
	for _, p := range procs {
		candidate[p.pid] = p.uid == uid && p.tty != ""
	}
	// covered says whether an ancestor stands for a process: an agent, an
	// editor or a conn above it, on the watch, covers what it runs.
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
				case kindAgent, kindEditor, kindConn:
					return true
				}
			}
			pid = a.ppid
		}
		return false
	}
	// A leaf has no candidate child.
	hasChild := map[int]bool{}
	for _, p := range procs {
		if candidate[p.pid] {
			hasChild[p.ppid] = true
		}
	}
	places := map[string]*place{}
	for _, p := range procs {
		if !candidate[p.pid] || covered(p) {
			continue
		}
		kind := kindOf(p)
		if kind == kindHold {
			continue
		}
		if kind == kindShell && hasChild[p.pid] {
			continue
		}
		if kind == kindRun && hasChild[p.pid] {
			continue
		}
		e := entry{pid: p.pid, kind: kind, command: commandLine(p), tty: p.tty, started: p.started}
		e.status, e.fault = statusOf(p, kind, p.pid == self)
		root := rootOf(p.cwd)
		if places[root] == nil {
			places[root] = &place{path: root}
		}
		places[root].entries = append(places[root].entries, e)
	}
	out := make([]place, 0, len(places))
	for _, pl := range places {
		sort.SliceStable(pl.entries, func(i, j int) bool { return pl.entries[i].started.After(pl.entries[j].started) })
		out = append(out, *pl)
	}
	// The newest work first; a place with nothing dated, last.
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].entries[0].started.After(out[j].entries[0].started)
	})
	return out
}

// statusOf is the word for a process as it stands.
func statusOf(p process, kind string, self bool) (string, bool) {
	switch {
	case self:
		return statusHere, false
	case p.state == 'T':
		return statusStopped, true
	case p.state == 'Z':
		return statusEnded, true
	case kind == kindShell:
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

// placeRoots finds the place that holds a directory: the nearest ancestor
// with a .git in it, else the directory itself. It remembers what it
// found, since the watch asks for the same directories on every read.
func placeRoots() func(string) string {
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
			if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
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
