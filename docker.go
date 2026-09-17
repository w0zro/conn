package main

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// A project on docker runs its services in containers, and a container is
// not a process the table can show: on macOS it runs in docker's own
// machine, and on Linux it runs as root, working in a directory of its
// own. So a project left on docker compose up -d was not a thin row in
// the processes view — it was no row at all, and the project itself was
// absent while three services ran in it. In the foreground it was one
// row saying docker compose and nothing under it, still reading ACTIVE
// with a service dead behind it.
//
// docker knows, and compose writes on every container it starts the
// directory it was started for — the same fact lsof reports of a process
// — so a container is filed under a project by the rule everything else
// is. It is listed as a row named for its service with the ports it
// publishes on the host, under the compose that runs it where one runs
// in a shell, and as a root under the project where compose was left
// detached.
//
// A container has no terminal, so conn holds no pane for it and the row
// is one it can only report: dimmed, and stepped over by the keys that
// carry you into things. That is the truth about it — there is nowhere
// to send the keys — and it is the same answer the processes view
// already gives for work it cannot reach.

// The labels compose writes that say where a container belongs and what
// it is called.
const (
	labelWorkingDir = "com.docker.compose.project.working_dir"
	labelService    = "com.docker.compose.service"
	labelProject    = "com.docker.compose.project"
)

// dockerWait bounds the reading's question to docker. The daemon answers
// in tens of milliseconds when it is up, and not at all for a while when
// it is starting or wedged; a reading that waited on it would hold the
// process list up behind it, so the wait is short and what docker last
// said stands meanwhile.
const dockerWait = 3 * time.Second

// dockerStopWait is longer, because stopping is not asking: docker gives
// the container ten seconds to go of its own accord before it insists,
// and a wait that gave up first would leave conn saying nothing happened
// while it was happening.
const dockerStopWait = 20 * time.Second

// kindService is what a container's row is called. It is not a RUN: a run
// is a program on this machine with a terminal above it somewhere, and a
// service is a thing docker is holding up on your behalf.
const kindService = "SERVICE"

// A container as docker described it.
type container struct {
	id      string // the short id, twelve hex digits
	name    string // compose-demo-web-1
	service string // the compose service, or the name where compose did not start it
	project string // the compose project, which its siblings share
	image   string
	state   string // running, exited, paused, created, restarting, dead
	status  string // Up 3 minutes (healthy); Exited (1) 3 minutes ago
	exit    string // the code it exited with, where it has: "0", "1"…
	health  string // healthy, unhealthy, starting; empty with no health check
	dir     string // the compose working directory: where it belongs
	ports   []string
	since   time.Time // when it came to stand as it does, as docker's age gives it
}

// running reports the container's process alive: the state a row wants no
// mark for.
func (c container) running() bool { return c.state == "running" }

// dockerPath is where the docker client is, or nothing where there is
// none: a machine without docker is asked nothing, ever.
var dockerPath = lookPath("docker")

// docker is what docker last said, which stands while it does not answer.
var docker struct {
	sync.Mutex
	last []container
}

// readContainers is what docker is holding up, as rows for the processes
// view. The exited are asked for too, because a service that died beside
// the others is the thing most worth a row; attachContainers keeps the
// ones whose project is still going and drops the rest of what the
// machine has ever kept.
//
// A daemon that is down, or no docker at all, is an empty list: nothing
// is running in a container. A daemon that does not answer in time is a
// different thing, and what it last said stands — the second answer
// says which, so the view can admit the rows are as last seen.
func readContainers() ([]container, bool) {
	if dockerPath == "" {
		return nil, false
	}
	out, err := dockerSays(dockerWait, "ps", "-a", "--format", "{{json .}}")
	docker.Lock()
	defer docker.Unlock()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			// Refused, in its own time: the daemon is down, or the
			// client could not reach it. Nothing is running in a
			// container that conn can see, and that is an answer rather
			// than a silence.
			docker.last = nil
			return nil, false
		}
		// It did not answer in time. What it last said stands, and the
		// view says so rather than quietly showing yesterday's rows.
		return docker.last, true
	}
	docker.last = parseContainers(out, time.Now())
	return docker.last, false
}

// dockerSays runs the docker client and answers what it printed. An exit
// of its own and a silence are told apart by the caller, and are not the
// same news: one says the daemon is down, the other says only that it did
// not answer yet.
func dockerSays(wait time.Duration, args ...string) ([]byte, error) {
	return dockerSaysIn("", wait, args...)
}

// dockerSaysIn is dockerSays run in a directory, for what compose reads
// relative to where it is asked: its files, and the project they name.
func dockerSaysIn(dir string, wait time.Duration, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	cmd := exec.CommandContext(ctx, dockerPath, args...)
	cmd.Dir = dir
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

// dockerRow is one line of docker ps --format '{{json .}}': the fields
// conn reads, with the labels and the ports as the strings docker prints
// them.
type dockerRow struct {
	ID     string `json:"ID"`
	Names  string `json:"Names"`
	Image  string `json:"Image"`
	Status string `json:"Status"`
	State  string `json:"State"`
	Labels string `json:"Labels"`
	Ports  string `json:"Ports"`
}

// parseContainers reads what docker ps said, one container to a line.
func parseContainers(out []byte, now time.Time) []container {
	var cs []container
	for line := range strings.SplitSeq(string(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var row dockerRow
		if err := json.Unmarshal([]byte(line), &row); err != nil || row.ID == "" {
			continue
		}
		labels := parseLabels(row.Labels)
		service := labels[labelService]
		if service == "" {
			service = row.Names
		}
		c := container{
			id: row.ID, name: row.Names, service: service, project: labels[labelProject],
			image: row.Image, state: row.State, status: row.Status,
			exit: exitOf(row.Status), health: healthOf(row.Status),
			dir: labels[labelWorkingDir], ports: hostPorts(row.Ports),
		}
		// docker says an age and conn says a time: the column is written
		// from a moment, the way every other row's is, so what docker
		// gives as three minutes is read back to when that was.
		if age, ok := ageOf(row.Status); ok {
			c.since = now.Add(-age)
		}
		cs = append(cs, c)
	}
	return cs
}

// exitOf is the code a container exited with, read off docker's word for
// it — Exited (1) 3 minutes ago — or nothing for one that has not.
func exitOf(status string) string {
	_, rest, ok := strings.Cut(status, "Exited (")
	if !ok {
		return ""
	}
	code, _, ok := strings.Cut(rest, ")")
	if !ok {
		return ""
	}
	return code
}

// healthOf is what a health check says, read off the status — Up 3
// minutes (healthy), (unhealthy), (health: starting) — or nothing for a
// service with no check.
func healthOf(status string) string {
	_, rest, ok := strings.Cut(status, "(")
	if !ok {
		return ""
	}
	word, _, _ := strings.Cut(rest, ")")
	switch word {
	case "healthy", "unhealthy":
		return word
	case "health: starting":
		return "starting"
	}
	return ""
}

// ageOf is how long ago the status last changed, off docker's words for
// it: Up 3 minutes, Exited (1) About an hour ago, Up Less than a second.
// The health in parentheses comes after the age and is not part of it.
func ageOf(status string) (time.Duration, bool) {
	if i := strings.Index(status, " ("); i >= 0 && !strings.HasPrefix(status, "Exited") {
		status = status[:i]
	}
	fields := strings.Fields(strings.TrimSuffix(status, " ago"))
	if len(fields) < 2 {
		return 0, false
	}
	unit := strings.TrimSuffix(fields[len(fields)-1], "s")
	count := 1
	if v, err := strconv.Atoi(fields[len(fields)-2]); err == nil {
		count = v
	}
	var each time.Duration
	switch unit {
	case "second":
		each = time.Second
	case "minute":
		each = time.Minute
	case "hour":
		each = time.Hour
	case "day":
		each = 24 * time.Hour
	case "week":
		each = 7 * 24 * time.Hour
	case "month":
		each = 30 * 24 * time.Hour
	case "year":
		each = 365 * 24 * time.Hour
	default:
		return 0, false
	}
	return time.Duration(count) * each, true
}

// parseLabels reads labels as docker ps prints them: key=value, comma
// separated. A value can hold a comma of its own — a maintainer's address
// — so a piece with no = in it is the tail of the value before it.
func parseLabels(s string) map[string]string {
	labels := map[string]string{}
	last := ""
	for piece := range strings.SplitSeq(s, ",") {
		key, value, ok := strings.Cut(piece, "=")
		if !ok {
			if last != "" {
				labels[last] += "," + piece
			}
			continue
		}
		labels[key], last = value, key
	}
	return labels
}

// hostPorts is the ports a container publishes on the host, as docker ps
// prints them — 0.0.0.0:8438->80/tcp, [::]:8438->80/tcp — each once and
// lowest first. A port with no host side stays inside the container and
// is nowhere to go.
func hostPorts(s string) []string {
	var ports []int
	for piece := range strings.SplitSeq(s, ",") {
		host, _, ok := strings.Cut(strings.TrimSpace(piece), "->")
		if !ok {
			continue
		}
		// 0.0.0.0:8438, [::]:8438: the port is what follows the last colon.
		i := strings.LastIndex(host, ":")
		if i < 0 {
			continue
		}
		port, err := strconv.Atoi(host[i+1:])
		if err != nil || slices.Contains(ports, port) {
			continue
		}
		ports = append(ports, port)
	}
	sort.Ints(ports)
	out := make([]string, 0, len(ports))
	for _, p := range ports {
		out = append(out, strconv.Itoa(p))
	}
	return out
}

// containerPID is the number a container goes by among rows, where every
// row has one: below zero, where no process is, and the same for the
// container from one reading to the next so the cursor holds its row. It
// is read off the id, whose first digits are enough to tell containers
// apart.
func containerPID(id string) int {
	v, err := strconv.ParseInt(id[:min(6, len(id))], 16, 64)
	if err != nil {
		v = 0
	}
	return -int(v) - 2
}

// containerStatus is what the status column says of a container, in the
// words the column already uses, and whether it is a thing to look at.
//
// A health check failing is a fault: the service is up and answering
// wrongly, which is the case nobody notices without being told. An exit
// of zero is ENDED and no fault — a worker that finished is not a
// problem — and any other code is the fault it plainly is.
func containerStatus(c container) (string, bool) {
	switch {
	case c.health == "unhealthy":
		return "UNHEALTHY", true
	case c.state == "restarting":
		return "RESTARTING", true
	case c.state == "paused":
		return statusStopped, true
	case c.running() && c.health == "starting":
		return "STARTING", false
	case c.running():
		return statusActive, false
	case c.exit != "" && c.exit != "0":
		return "EXIT " + c.exit, true
	}
	return statusEnded, false
}

// activity is what a container's row says it is: the service, and the
// ports it publishes on the host, which is where you would go to reach
// it. web · :8438
func (c container) activity() string {
	if len(c.ports) == 0 {
		return c.service
	}
	return c.service + " · :" + strings.Join(c.ports, " :")
}

// attachContainers files each container under the project its directory
// says it belongs to, and under the compose that runs it where one is
// running in a shell there — the deepest such process, compose being a
// docker running its plugin and the plugin being the one that holds the
// containers. With no compose running there — up -d, and the shell long
// gone — a container is a root of its own under the project.
//
// A container compose did not start carries no directory and so belongs
// to no project. It is left out rather than filed somewhere invented: a
// docker run from anywhere is not work at a place, and a row under a
// project it has nothing to do with would be a lie about where it is.
// paneOf says which terminal conn has opened for a container, by its id:
// a container has none of its own, so the pane conn opened to watch it
// stands in for one. With that the row is reached, left and walked to
// like every other — the whole of what having a terminal means here.
func attachContainers(projects []project, cs []container, rootOf func(string) string, paneOf, shellIn map[string]string) []project {
	// A stopped container is listed while its project is: a sibling still
	// running, or a compose working in its directory. A service that died
	// beside the others is exactly what wants noticing. A project stopped
	// whole is over, and its containers are not a list of yesterday's
	// failures to read again every day.
	live := map[string]bool{}
	for _, c := range cs {
		if c.running() && c.project != "" {
			live[c.project] = true
		}
	}
	at := map[string][]container{}
	var paths []string
	for _, c := range cs {
		if c.dir == "" {
			continue
		}
		path := rootOf(c.dir)
		if !c.running() && !live[c.project] && !composeRuns(projects, path, c.dir, c.service) {
			continue
		}
		if _, ok := at[path]; !ok {
			paths = append(paths, path)
		}
		at[path] = append(at[path], c)
	}
	// The order among a project's own containers is the order compose
	// writes them in its file, which docker does not keep; the service
	// name is the one order that reads the same on every reading.
	for path := range at {
		sort.SliceStable(at[path], func(i, j int) bool { return at[path][i].service < at[path][j].service })
	}
	out := make([]project, 0, len(projects)+len(paths))
	filled := map[string]bool{}
	for _, pl := range projects {
		if rows := at[pl.path]; len(rows) > 0 {
			filled[pl.path] = true
			pl.entries = placeContainers(pl, rows, paneOf, shellIn)
		}
		out = append(out, pl)
	}
	// A project nothing but docker is working in is a project all the
	// same, and it goes on the end. The projects above it are in the
	// order work began in them, which a container has no answer for —
	// docker says how long ago a status changed, not when the project
	// was taken up — so these are by name, which holds still.
	sort.Strings(paths)
	for _, path := range paths {
		if filled[path] {
			continue
		}
		pl := project{path: path}
		pl.entries = placeContainers(pl, at[path], paneOf, shellIn)
		out = append(out, pl)
	}
	return out
}

// composeRuns reports a compose working in the container's directory
// somewhere in the projects: the reason a stopped container is still
// worth a row when none of its siblings is running.
func composeRuns(projects []project, path, dir, service string) bool {
	for _, pl := range projects {
		if pl.path == path && composeAt(pl, path, dir, service) >= 0 {
			return true
		}
	}
	return false
}

// placeContainers puts a project's containers among its rows: each under
// the compose that runs it where there is one, and at the foot of the
// project where there is not.
func placeContainers(pl project, cs []container, paneOf, shellIn map[string]string) []entry {
	entries := slices.Clone(pl.entries)
	for _, c := range cs {
		e := entry{
			pid: containerPID(c.id), kind: kindService,
			command: c.activity(), typed: c.activity(),
			started: c.since, since: c.since, cwd: c.dir,
			container: c.id, tty: paneOf[c.id],
		}
		e.status, e.fault = containerStatus(c)
		// The shells conn opened inside this container come out of the
		// project's own rows, to go back under the service below.
		var shells []entry
		entries, shells = liftShells(entries, shellIn, c.id)
		if i := composeAt(project{path: pl.path, entries: entries}, pl.path, c.dir, c.service); i >= 0 {
			e.depth = entries[i].depth + 1
			// After the compose's own subtree, so the containers of one
			// compose stand together under it rather than between its
			// rows.
			j := i + 1
			for j < len(entries) && entries[j].depth > entries[i].depth {
				j++
			}
			entries = slices.Insert(entries, j, append([]entry{e}, nest(shells, c, e.depth)...)...)
			continue
		}
		entries = append(entries, e)
		entries = append(entries, nest(shells, c, e.depth)...)
	}
	return entries
}

// liftShells takes the rows running in a shell conn opened inside this
// container out of the list, and answers what is left and what was
// taken. A pane's whole tree shares its terminal, so asking by terminal
// takes the shell and anything under it in one go.
func liftShells(entries []entry, shellIn map[string]string, id string) (kept, shells []entry) {
	kept = make([]entry, 0, len(entries))
	for _, row := range entries {
		if row.tty != "" && shellIn[row.tty] == id {
			shells = append(shells, row)
			continue
		}
		kept = append(kept, row)
	}
	return kept, shells
}

// nest is the lifted rows as they stand under their service: the shell
// itself named for the container it is inside, since docker exec is
// what conn typed and not what the operator asked for, and whatever the
// shell is running kept at its own remove below it.
//
// The rows carry no container of their own. A shell inside a service is
// the operator's work, not another handle on the service, and a key
// that stops a container should not be armed from a row that is only
// standing in one.
func nest(shells []entry, c container, depth int) []entry {
	if len(shells) == 0 {
		return nil
	}
	root := shells[0].depth
	for _, row := range shells {
		root = min(root, row.depth)
	}
	out := make([]entry, 0, len(shells))
	for _, row := range shells {
		row.depth = row.depth - root + depth + 1
		if row.depth == depth+1 {
			row.kind, row.typed = kindShell, "sh in "+c.service
			row.command = row.typed
		}
		out = append(out, row)
	}
	return out
}

// composeAt is where in a project's rows the compose that runs a service
// stands, or below zero where none does: the deepest compose working in
// the container's own directory that either names the service or names
// none, which is the one that runs them all.
func composeAt(pl project, path, dir, service string) int {
	if pl.path != path {
		return -1
	}
	best, all := -1, -1
	for i, e := range pl.entries {
		if e.cwd != dir || !runsCompose(e) {
			continue
		}
		switch named := composeNames(e); {
		case slices.Contains(named, service):
			if best < 0 || e.depth > pl.entries[best].depth {
				best = i
			}
		case len(named) == 0:
			if all < 0 || e.depth > pl.entries[all].depth {
				all = i
			}
		}
	}
	if best >= 0 {
		return best
	}
	return all
}

// runsCompose reports a row that is compose: docker running its compose
// command, or the compose plugin itself.
func runsCompose(e entry) bool {
	fields := strings.Fields(e.asTyped())
	if len(fields) < 2 {
		return false
	}
	name := strings.TrimPrefix(fields[0], "docker-")
	return (name == "docker" || name == "compose") && slices.Contains(fields[1:], "compose")
}

// composeNames is the services a compose up names after its up, the words
// there that are not flags; none is every service. The words before up
// belong to compose — a file, a project name — and are not services.
func composeNames(e entry) []string {
	fields := strings.Fields(e.asTyped())
	i := slices.Index(fields, "up")
	if i < 0 {
		return nil
	}
	var names []string
	for _, f := range fields[i+1:] {
		if !strings.HasPrefix(f, "-") {
			names = append(names, f)
		}
	}
	return names
}

// containerWire is a container as it crosses between conn's programs.
// The panel talks to docker and the readout draws the page, and they are
// separate processes, so what the panel knows has to be written down for
// the other to read. conn's own fields are unexported and encoding/json
// carries only exported ones, so this is that shape — kept beside the
// type it mirrors, rather than a second definition of a container to
// keep in step with this one.
type containerWire struct {
	ID, Name, Service, Project string
	Image, State, Status, Exit string
	Health, Dir                string
	Ports                      []string
	Since                      time.Time
}

func (c container) MarshalJSON() ([]byte, error) {
	return json.Marshal(containerWire{
		ID: c.id, Name: c.name, Service: c.service, Project: c.project,
		Image: c.image, State: c.state, Status: c.status, Exit: c.exit,
		Health: c.health, Dir: c.dir, Ports: c.ports, Since: c.since,
	})
}

func (c *container) UnmarshalJSON(b []byte) error {
	var w containerWire
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	*c = container{
		id: w.ID, name: w.Name, service: w.Service, project: w.Project,
		image: w.Image, state: w.State, status: w.Status, exit: w.Exit,
		health: w.Health, dir: w.Dir, ports: w.Ports, since: w.Since,
	}
	return nil
}
