package main

import (
	"flag"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/w0zro/conn/internal/config"
	"github.com/w0zro/conn/internal/room"
	"github.com/w0zro/conn/internal/work"
	"github.com/w0zro/conn/internal/work/brew"
	"github.com/w0zro/conn/internal/work/docker"
)

// The CLI is the grammar's verbs for whoever has no panel: an agent, a
// script, a person at a shell. Each command reads the machine once, as
// the panel reads it, from the project the working directory is in,
// and addresses a process in it the way an agent knows one — by name,
// by the port an error named, or by the pid conn ls gave.

// A here is the machine read once, from a directory: the read, the
// project the directory is in, and the server, where one is up.
type here struct {
	read
	home string
	// root is the project the directory is in: the repository, or the
	// folder of them where the directory is beside them rather than in
	// one. group is the folder of repositories above root, where root
	// is one of them, and "" otherwise.
	root, group string
	srv         *room.Server
}

// readHere reads the machine from a directory. A directory in no
// project is an error, said as cmd says it.
func readHere(cmd, home, dir string) (here, error) {
	configured, err := config.Roots(home)
	if err != nil {
		return here{}, err
	}
	in := sources{uid: os.Getuid(), home: home, configured: configured, containers: docker.Read(), self: os.Getpid()}
	h := here{home: home}
	if srv := room.Find(home); srv != nil {
		if _, err := srv.Panes(); err == nil {
			in.srv, h.srv = srv, srv
		}
	}
	r, err := readMachine(in)
	if err != nil {
		return here{}, fmt.Errorf("the process table could not be read: %v", err)
	}
	// brew is asked only where a project declares a service of it, as
	// the panel asks it: a question to brew is a second or so.
	if brew.Declared(r.declared) {
		if services, err := brew.Read(); err == nil {
			in.brews, in.declared = services, r.declared
			if again, err := readMachine(in); err == nil {
				r = again
			}
		}
	}
	h.read = r
	h.root = r.rooting.rootOf(realDir(dir))
	if !r.rooting.isProject(h.root) {
		return here{}, fmt.Errorf("%s is in no project under the roots in config.json", config.Tilde(dir, home))
	}
	if above := r.rooting.rootOf(parentOf(h.root)); above != h.root && r.rooting.isProject(above) {
		h.group = above
	}
	return h, nil
}

// parentOf is the directory above, or the directory itself at the top.
func parentOf(dir string) string {
	if i := strings.LastIndexByte(dir, '/'); i > 0 {
		return dir[:i]
	}
	return dir
}

// projects is the projects under a directory, as the read has them.
func (h here) projects(under string) []work.Project {
	var out []work.Project
	for _, pl := range h.read.projects {
		if work.Within(pl.Path, under) {
			out = append(out, pl)
		}
	}
	return out
}

// A found is a row an address named, and the project it is in.
type found struct {
	work.Entry
	project string
}

// find is the row an address names. A name is looked for in the
// project the directory is in, then in the folder of repositories
// above it: an agent's word reaches its own work first. A pid and a
// port name one process on the machine, and are looked for everywhere.
// No row, or more than one, is an error that says which.
func (h here) find(addr string) (found, error) {
	var match func(work.Entry) bool
	var places [][]work.Project
	switch {
	case strings.HasPrefix(addr, ":"):
		port := addr[1:]
		match = func(e work.Entry) bool { return slices.Contains(portsOf(e), port) }
		places = [][]work.Project{h.read.projects}
	case isPid(addr):
		pid, _ := strconv.Atoi(addr)
		match = func(e work.Entry) bool { return e.PID == pid }
		places = [][]work.Project{h.read.projects}
	default:
		match = func(e work.Entry) bool { return nameOf(e) == addr }
		places = [][]work.Project{h.projects(h.root)}
		if h.group != "" {
			places = append(places, h.projects(h.group))
		}
	}
	for _, place := range places {
		var hits []found
		for _, pl := range place {
			for _, e := range pl.Entries {
				if match(e) {
					hits = append(hits, found{e, pl.Path})
				}
			}
		}
		switch len(hits) {
		case 0:
			continue
		case 1:
			return hits[0], nil
		}
		var pids []string
		for _, f := range hits {
			pids = append(pids, pidWord(f.Entry))
		}
		return found{}, fmt.Errorf("%s names %d processes: %s; say which by pid", addr, len(hits), strings.Join(pids, ", "))
	}
	return found{}, noSuch(addr)
}

// A noSuch is an address no row answers to, as against one that
// answers for more than one.
type noSuch string

func (a noSuch) Error() string {
	return "no process here answers to " + string(a) + "; conn ls lists them"
}

// isPid says whether an address is a pid: digits and nothing else.
func isPid(addr string) bool {
	if addr == "" {
		return false
	}
	for _, r := range addr {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// paneOf is the pane of conn's server a row runs in, by its terminal.
func (h here) paneOf(e work.Entry) (room.Pane, bool) {
	if h.srv == nil || e.TTY == "" {
		return room.Pane{}, false
	}
	p, ok := h.panes[e.TTY]
	return p, ok
}

// fail says what went wrong, as a command of conn's says it, and
// answers the exit status.
func fail(cmd string, err error) int {
	fmt.Fprintf(os.Stderr, "conn %s: %v\n", cmd, err)
	return 1
}

// usage says how a command is used, and answers the status a usage
// error exits with.
func usage(cmd, use string) int {
	fmt.Fprintf(os.Stderr, "conn %s: usage: %s\n", cmd, use)
	return 2
}

// parseAnywhere parses a command's flags wherever they stand among its
// words, before the address or after it, as an agent writes them
// either way, and answers the words that are not flags.
func parseAnywhere(fs *flag.FlagSet, args []string) ([]string, error) {
	var words []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return words, nil
		}
		words = append(words, fs.Arg(0))
		args = fs.Args()[1:]
	}
}
