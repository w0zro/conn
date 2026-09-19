package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// A service Homebrew holds up is a thing to reach the way a container
// is: a database, a model server, a queue, started once with brew
// services and left running under launchd, with no terminal above it
// and a directory of its own under /opt/homebrew that belongs to no
// project. The process table has it — it runs as the operator — and
// files it nowhere. Nothing about the process says which project it is
// working for; the project's .conn does. A declaration whose command
// is brew services start, or run, names the formula, and the row it
// makes is the service as brew reports it rather than a pane running
// the command: ACTIVE with its pid and the ports it listens on, DOWN
// when it is not started, and EXIT n when its last run ended badly.
// Enter opens its log; x asks brew to stop it; u, or Enter on it down,
// asks brew to start it.
//
// The one thing brew is asked is brew services info --all --json,
// which says of every service it knows whether it is running, under
// what pid, with what exit, and where it writes its log. It answers in
// under half a second and is asked only while some project declares a
// service, on a slow beat, off the loop, so a conn on a machine with no
// brew, or with brew and nothing declared, never asks.

const (
	brewWait = 5 * time.Second // the longest brew is given to answer
	// How often brew is asked, while anything is declared. An asking
	// boots brew's ruby, half a second of a core, and a service does
	// not change on its own between one and the next; a start or a
	// stop from the panel asks again at once.
	brewBeat = 15 * time.Second
)

var brewPath = lookPath("brew")

// A brewService is one service as brew reports it.
type brewService struct {
	name    string // the formula
	running bool
	pid     int
	exit    string // the code its last run ended with, or "" for none or nought
	status  string // brew's own word: started, stopped, none, error, scheduled
	command string // what launchd runs for it
	log     string // where it writes
}

// brewArgs reads a declared command that starts a service with brew:
// brew services start FORMULA, or run. Anything else is not one.
func brewArgs(command string) (formula string, ok bool) {
	f := strings.Fields(command)
	if len(f) < 4 || f[0] != "brew" || f[1] != "services" || f[2] != "start" && f[2] != "run" {
		return "", false
	}
	for _, a := range f[3:] {
		if !strings.HasPrefix(a, "-") {
			return a, true
		}
	}
	return "", false
}

// brewMark is the mark on the pane conn opens to watch a service's log,
// which is what makes that pane the row's terminal.
func brewMark(formula string) string { return "brew:" + formula }

// brewEnv is what conn adds to brew's environment when it asks. brew
// records every command it is given with a curl to its analytics,
// forked and left to itself, and would send one for every asking conn
// makes; it checks itself for updates too, and prints hints. An asking
// is conn's own and not the operator's use of brew, and sends nothing
// anywhere.
var brewEnv = []string{"HOMEBREW_NO_ANALYTICS=1", "HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_ENV_HINTS=1"}

// brewSays asks brew, given a wait.
func brewSays(wait time.Duration, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	cmd := exec.CommandContext(ctx, brewPath, args...)
	cmd.Env = append(os.Environ(), brewEnv...)
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

// readBrewServices is every service brew knows, as it stands. Without
// brew there is nothing to ask.
func readBrewServices() ([]brewService, error) {
	if brewPath == "" {
		return nil, nil
	}
	out, err := brewSays(brewWait, "services", "info", "--all", "--json")
	if err != nil {
		return nil, err
	}
	return parseBrewServices(out)
}

// brewRow is one entry of brew services info --all --json.
type brewRow struct {
	Name     string `json:"name"`
	Running  bool   `json:"running"`
	PID      *int   `json:"pid"`
	ExitCode *int   `json:"exit_code"`
	Status   string `json:"status"`
	Command  string `json:"command"`
	LogPath  string `json:"log_path"`
}

// parseBrewServices reads brew's answer.
func parseBrewServices(out []byte) ([]brewService, error) {
	var rows []brewRow
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, err
	}
	services := make([]brewService, 0, len(rows))
	for _, r := range rows {
		s := brewService{name: r.Name, running: r.Running, status: r.Status, command: r.Command, log: r.LogPath}
		if r.PID != nil {
			s.pid = *r.PID
		}
		if r.ExitCode != nil && *r.ExitCode != 0 {
			s.exit = strconv.Itoa(*r.ExitCode)
		}
		services = append(services, s)
	}
	return services, nil
}

// brewServiceNamed is the service of a formula among those brew
// reported, where it is one.
func brewServiceNamed(services []brewService, formula string) *brewService {
	for i := range services {
		if services[i].name == formula {
			return &services[i]
		}
	}
	return nil
}

// brewStatus is the word a service's row wears, and whether it is a
// fault: ACTIVE running, EXIT n where its last run ended badly, and
// DOWN otherwise, which u brings up.
func brewStatus(s brewService) (string, bool) {
	switch {
	case s.running:
		return statusActive, false
	case s.exit != "":
		return "EXIT " + s.exit, true
	case s.status == "error":
		return "ERROR", true
	}
	return statusDown, false
}

// brewDeclared says whether any project declares a brew service, which
// is when brew is worth asking.
func brewDeclared(declared map[string]declared) bool {
	for _, d := range declared {
		for _, decl := range d.list {
			if _, ok := brewArgs(decl.command); ok {
				return true
			}
		}
	}
	return false
}

// attachBrew puts each declared brew service among its project's rows,
// at the foot of the block the way a declaration down stands, as brew
// reports it: with the pid and the sockets of the process running it,
// or down. The declared name is the row's label, as for any
// declaration, and the mark says which. A project with no block —
// nothing running in it — shows nothing of its file, as for the rest
// of it. A service two projects declare is a row under each, each
// counting how many; the panel files them as one, see byState.
func attachBrew(projects []project, declared map[string]declared, services []brewService, sockets map[int][]socket, paneOf map[string]string) []project {
	if len(declared) == 0 {
		return projects
	}
	out := make([]project, len(projects))
	copy(out, projects)
	paths := make([]string, 0, len(declared))
	for path := range declared {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	shared := map[string]int{}
	var rows []struct {
		block int
		e     entry
	}
	for _, path := range paths {
		i := blockOf(out, path)
		if i < 0 || declared[path].err != "" {
			continue
		}
		for _, decl := range declared[path].list {
			formula, ok := brewArgs(decl.command)
			if !ok {
				continue
			}
			shared[formula]++
			rows = append(rows, struct {
				block int
				e     entry
			}{i, brewEntry(path, decl, formula, brewServiceNamed(services, formula), sockets, paneOf)})
		}
	}
	for _, r := range rows {
		r.e.shared = shared[r.e.brew]
		out[r.block].entries = append(out[r.block].entries, r.e)
	}
	return out
}

// brewEntry is a declared brew service as a row.
func brewEntry(path string, decl declaration, formula string, svc *brewService, sockets map[int][]socket, paneOf map[string]string) entry {
	e := entry{
		pid: declaredPID(path, decl.name), kind: kindService,
		command: formula, typed: formula, status: statusDown,
		cwd: decl.at(path), declared: markDeclared(path, decl.name), brew: formula,
		tty: paneOf[brewMark(formula)],
	}
	if svc == nil {
		return e
	}
	e.status, e.fault = brewStatus(*svc)
	if svc.running && svc.pid > 0 {
		e.pid = svc.pid
		e.sockets = sockets[svc.pid]
		e.ports = listeningPorts(e.sockets)
	}
	return e
}

// brewMsg carries what brew said of its services.
type brewMsg struct {
	services []brewService
	err      error
}

// brewTickMsg is the beat on which brew is asked, while anything is
// declared.
type brewTickMsg struct{}

// nextBrew is the next beat.
func nextBrew() tea.Cmd {
	return tea.Tick(brewBeat, func(time.Time) tea.Msg { return brewTickMsg{} })
}

// readBrew asks brew, off the loop.
func readBrew() tea.Msg {
	services, err := readBrewServices()
	return brewMsg{services: services, err: err}
}
