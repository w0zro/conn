package work

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/w0zro/conn/internal/station"

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
	BrewWait = 5 * time.Second // the longest brew is given to answer
	// How often brew is asked, while anything is declared. An asking
	// boots brew's ruby, half a second of a core, and a service does
	// not change on its own between one and the next; a start or a
	// stop from the panel asks again at once.
	BrewBeat = 15 * time.Second
)

var BrewPath = station.LookPath("brew")

// A BrewService is one service as brew reports it.
type BrewService struct {
	Name    string // the formula
	Running bool
	PID     int
	Exit    string // the code its last run ended with, or "" for none or nought
	Status  string // brew's own word: started, stopped, none, error, scheduled
	Command string // what launchd runs for it
	Log     string // where it writes
}

// BrewArgs reads a declared command that starts a service with brew:
// brew services start FORMULA, or run. Anything else is not one.
func BrewArgs(command string) (formula string, ok bool) {
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

// BrewMark is the mark on the pane conn opens to watch a service's log,
// which is what makes that pane the row's terminal.
func BrewMark(formula string) string { return "brew:" + formula }

// BrewEnv is what conn adds to brew's environment when it asks. brew
// records every command it is given with a curl to its analytics,
// forked and left to itself, and would send one for every asking conn
// makes; it checks itself for updates too, and prints hints. An asking
// is conn's own and not the operator's use of brew, and sends nothing
// anywhere.
var BrewEnv = []string{"HOMEBREW_NO_ANALYTICS=1", "HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_ENV_HINTS=1"}

// BrewSays asks brew, given a wait.
func BrewSays(wait time.Duration, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	cmd := exec.CommandContext(ctx, BrewPath, args...)
	cmd.Env = append(os.Environ(), BrewEnv...)
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
func readBrewServices() ([]BrewService, error) {
	if BrewPath == "" {
		return nil, nil
	}
	out, err := BrewSays(BrewWait, "services", "info", "--all", "--json")
	if err != nil {
		return nil, err
	}
	return ParseBrewServices(out)
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

// ParseBrewServices reads brew's answer.
func ParseBrewServices(out []byte) ([]BrewService, error) {
	var rows []brewRow
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, err
	}
	services := make([]BrewService, 0, len(rows))
	for _, r := range rows {
		s := BrewService{Name: r.Name, Running: r.Running, Status: r.Status, Command: r.Command, Log: r.LogPath}
		if r.PID != nil {
			s.PID = *r.PID
		}
		if r.ExitCode != nil && *r.ExitCode != 0 {
			s.Exit = strconv.Itoa(*r.ExitCode)
		}
		services = append(services, s)
	}
	return services, nil
}

// BrewServiceNamed is the service of a formula among those brew
// reported, where it is one.
func BrewServiceNamed(services []BrewService, formula string) *BrewService {
	for i := range services {
		if services[i].Name == formula {
			return &services[i]
		}
	}
	return nil
}

// BrewStatus is the word a service's row wears, and whether it is a
// fault: ACTIVE running, EXIT n where its last run ended badly, and
// DOWN otherwise, which u brings up.
func BrewStatus(s BrewService) (string, bool) {
	switch {
	case s.Running:
		return StatusActive, false
	case s.Exit != "":
		return exitWord + s.Exit, true
	case s.Status == "error":
		return "ERROR", true
	}
	return StatusDown, false
}

// BrewDeclared says whether any project declares a brew service, which
// is when brew is worth asking.
func BrewDeclared(declared map[string]Declared) bool {
	for _, d := range declared {
		for _, decl := range d.List {
			if _, ok := BrewArgs(decl.Command); ok {
				return true
			}
		}
	}
	return false
}

// AttachBrew puts each declared brew service among its project's rows,
// at the foot of the block the way a declaration down stands, as brew
// reports it: with the pid and the sockets of the process running it,
// or down. The declared name is the row's label, as for any
// declaration, and the mark says which. A project with no block —
// nothing running in it — shows nothing of its file, as for the rest
// of it. A service two projects declare is a row under each, each
// counting how many; the panel files them as one, see byState.
func AttachBrew(projects []Project, declared map[string]Declared, services []BrewService, sockets map[int][]Socket, paneOf map[string]string) []Project {
	if len(declared) == 0 {
		return projects
	}
	out := make([]Project, len(projects))
	copy(out, projects)
	paths := make([]string, 0, len(declared))
	for path := range declared {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	shared := map[string]int{}
	var rows []struct {
		block int
		e     Entry
	}
	for _, path := range paths {
		i := BlockOf(out, path)
		if i < 0 || declared[path].Err != "" {
			continue
		}
		for _, decl := range declared[path].List {
			formula, ok := BrewArgs(decl.Command)
			if !ok {
				continue
			}
			shared[formula]++
			rows = append(rows, struct {
				block int
				e     Entry
			}{i, brewEntry(path, decl, formula, BrewServiceNamed(services, formula), sockets, paneOf)})
		}
	}
	for _, r := range rows {
		r.e.Shared = shared[r.e.Brew]
		out[r.block].Entries = append(out[r.block].Entries, r.e)
	}
	return out
}

// brewEntry is a declared brew service as a row.
func brewEntry(path string, decl Declaration, formula string, svc *BrewService, sockets map[int][]Socket, paneOf map[string]string) Entry {
	e := Entry{
		PID: DeclaredPID(path, decl.Name), Kind: KindService,
		Command: formula, Typed: formula, Status: StatusDown,
		Cwd: decl.At(path), Declared: MarkDeclared(path, decl.Name), Brew: formula,
		TTY: paneOf[BrewMark(formula)],
	}
	if svc == nil {
		return e
	}
	e.Status, e.Fault = BrewStatus(*svc)
	if svc.Running && svc.PID > 0 {
		e.PID = svc.PID
		e.Sockets = sockets[svc.PID]
		e.Ports = ListeningPorts(e.Sockets)
	}
	return e
}

// BrewMsg carries what brew said of its services.
type BrewMsg struct {
	Services []BrewService
	Err      error
}

// BrewTickMsg is the beat on which brew is asked, while anything is
// declared.
type BrewTickMsg struct{}

// NextBrew is the next beat.
func NextBrew() tea.Cmd {
	return tea.Tick(BrewBeat, func(time.Time) tea.Msg { return BrewTickMsg{} })
}

// ReadBrew asks brew, off the loop.
func ReadBrew() tea.Msg {
	services, err := readBrewServices()
	return BrewMsg{Services: services, Err: err}
}
