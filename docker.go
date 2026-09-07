package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// A project on docker runs its services in containers, and a container is
// not a process the scan can see: on macOS it runs in docker's own machine,
// and on Linux it is root's, working in a directory of its own. docker
// knows, and compose writes on every container it starts the directory
// it was started for — the same fact lsof reports of a process — so a
// container is filed under a place by the rule everything else is. It is
// listed as a row named for its service, with the ports it publishes on
// the host, under the docker compose up that runs it when one runs in a
// shell and under the place itself when compose was left detached. Its
// row has no shell to enter; x asks docker to stop it.

// Container is what docker said of one container, kept on the Proc that
// stands for it in the tree.
type Container struct {
	ID      string // the short id, twelve hex digits
	Name    string // the container's name: compose-demo-web-1
	Service string // the compose service, or the name when compose did not start it
	Project string // the compose project, which its siblings share
	Image   string
	Status  string // Up 3 minutes (healthy); Exited (1) 3 minutes ago
	State   string // running, exited, paused, created, restarting, dead
	Exit    string // the status it exited with, when it has: "0", "1"…
	Health  string // healthy, unhealthy or starting, for a service with a health check; else ""
	Ago     string // how long ago the status changed, in the navigator's terms: now, 3m, 2h
}

// running reports the container's process alive: the state a row wants
// no mark for.
func (c *Container) running() bool { return c.State == "running" }

// dockerPath is where the docker client is, or "" where there is none: a
// machine without docker is asked nothing, every scan.
var dockerPath = func() string {
	p, err := exec.LookPath("docker")
	if err != nil {
		return ""
	}
	return p
}()

// containerLabels are the labels compose writes that say where a
// container belongs and what it is called.
const (
	labelWorkingDir = "com.docker.compose.project.working_dir"
	labelService    = "com.docker.compose.service"
	labelProject    = "com.docker.compose.project"
)

// containers lists the containers as rows for the tree: only the ones
// compose started for a directory, since a container without one belongs
// to no place, the way a process working outside every project belongs to
// none. Every container is asked for, the exited included: a service that
// died is the thing most worth a row, and attachContainers keeps the ones
// whose project is still going. A daemon that is not up, or no docker at
// all, is an empty list — nothing is running in a container, which is
// true.
func containers() []Proc {
	if dockerPath == "" {
		return nil
	}
	out, err := listing(scanTimeout, dockerPath, "ps", "-a", "--format", "{{json .}}")
	if err != nil {
		return nil
	}
	return parseContainers(out)
}

// dockerRow is the shape of one line of docker ps --format '{{json .}}':
// the fields conn reads, and the labels and ports as the strings docker
// prints them.
type dockerRow struct {
	ID     string `json:"ID"`
	Names  string `json:"Names"`
	Image  string `json:"Image"`
	Status string `json:"Status"`
	State  string `json:"State"`
	Labels string `json:"Labels"`
	Ports  string `json:"Ports"`
}

// parseContainers reads what docker ps said, one container a line.
func parseContainers(out []byte) []Proc {
	var procs []Proc
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var row dockerRow
		if err := json.Unmarshal(sc.Bytes(), &row); err != nil || row.ID == "" {
			continue
		}
		labels := parseLabels(row.Labels)
		dir := labels[labelWorkingDir]
		if dir == "" {
			continue
		}
		service := labels[labelService]
		if service == "" {
			service = row.Names
		}
		procs = append(procs, Proc{
			PID:     containerPID(row.ID),
			Command: service,
			Dir:     dir,
			Ports:   hostPorts(row.Ports),
			Container: &Container{
				ID: row.ID, Name: row.Names, Service: service, Project: labels[labelProject],
				Image: row.Image, Status: row.Status, State: row.State,
				Exit: exitOf(row.Status), Health: healthOf(row.Status), Ago: agoOf(row.Status),
			},
		})
	}
	return procs
}

// exitOf is the status a container exited with, read off docker's word
// for it — Exited (1) 3 minutes ago — or nothing for one that has not.
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

// agoOf is how long ago the status changed, as the navigator says ages:
// docker says Up 3 minutes, Exited (1) About an hour ago, Less than a
// second ago, and the row has room for 3m. The health in parentheses,
// when there is one, comes after the age and is not part of it.
func agoOf(status string) string {
	if i := strings.Index(status, " ("); i >= 0 && !strings.HasPrefix(status, "Exited") {
		status = status[:i]
	}
	fields := strings.Fields(strings.TrimSuffix(status, " ago"))
	n := len(fields)
	if n < 2 {
		return ""
	}
	unit := strings.TrimSuffix(fields[n-1], "s")
	count := 1
	if v, err := strconv.Atoi(fields[n-2]); err == nil {
		count = v
	}
	switch unit {
	case "second":
		return "now"
	case "minute":
		return strconv.Itoa(count) + "m"
	case "hour":
		return strconv.Itoa(count) + "h"
	case "day":
		return strconv.Itoa(count) + "d"
	case "week":
		return strconv.Itoa(count) + "w"
	case "month":
		return strconv.Itoa(count*4) + "w"
	case "year":
		return strconv.Itoa(count*52) + "w"
	}
	return ""
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
		labels[key] = value
		last = key
	}
	return labels
}

// hostPorts is the ports a container publishes on the host, as docker ps
// prints them — 0.0.0.0:8438->80/tcp, [::]:8438->80/tcp — each once,
// lowest first. A port with no host side is the container's own and is
// nowhere to go.
func hostPorts(s string) []string {
	var ports []string
	for piece := range strings.SplitSeq(s, ",") {
		host, _, ok := strings.Cut(strings.TrimSpace(piece), "->")
		if !ok {
			continue
		}
		if port, ok := portOf(host); ok && !slices.Contains(ports, port) {
			ports = append(ports, port)
		}
	}
	sortPorts(ports)
	return ports
}

// containerPID is the number a container goes by in the tree, where every
// node has one: below zero, where no process is, and the same for the
// container from one scan to the next, so the cursor holds its row. It is
// read off the id, whose first digits tell containers apart the way the
// whole id does.
func containerPID(id string) int {
	v, err := strconv.ParseInt(id[:min(6, len(id))], 16, 64)
	if err != nil {
		v = 0
	}
	return -int(v) - 2
}

// attachContainers files each container under the compose that runs it,
// when one is running in a shell: the process working in the container's
// directory that is compose, the deepest of them — compose is a docker
// running its plugin, and the plugin is the one that holds the containers.
// With no compose running there — up -d, and the shell gone — a container
// is a root of its own under the place, which is where its directory says
// it belongs.
func attachContainers(procs, cs []Proc) []Proc {
	// A container that is not running is listed while its project is:
	// a sibling running, or a compose up working in its directory. A
	// service that died beside the others is what needs noticing, and
	// tab goes to it; a project stopped whole is over, and its containers
	// are not a list of failures to read every day after.
	live := map[string]bool{}
	for _, c := range cs {
		if c.Container.running() {
			live[c.Container.Project] = true
		}
	}
	kept := cs[:0]
	for _, c := range cs {
		c.PPID = composeFor(procs, c.Dir, c.Container.Service)
		if c.Container.running() || live[c.Container.Project] || c.PPID != 0 {
			kept = append(kept, c)
		}
	}
	return append(procs, kept...)
}

// containerWrong reports a container in a row's run that needs a look: one
// that is not running and did not exit well, or one its health check calls
// unhealthy.
func containerWrong(run []*ProcNode) bool {
	for _, n := range run {
		c := n.Container
		if c == nil {
			continue
		}
		if !c.running() && c.Exit != "0" {
			return true
		}
		if c.Health == "unhealthy" {
			return true
		}
	}
	return false
}

// containerDone reports a row that is a container that exited well: a
// one-shot service that did its job.
func containerDone(r navRow) bool {
	c := r.node.Container
	return c != nil && !c.running() && c.Exit == "0"
}

// containerNote is what a container's row says after its name, where a
// process's says its ports: the ports, and what its health check says
// when it says anything but healthy; for one that is not running, how
// long ago it stopped, beside the mark.
func containerNote(n *ProcNode) string {
	c := n.Container
	if !c.running() {
		if c.Ago != "" {
			return " · " + c.Ago
		}
		return ""
	}
	// The health check's word stands where the ports would: a row is
	// narrow, and a service that is unhealthy is not one to go and open.
	// The ports are the pane's.
	if c.Health == "unhealthy" || c.Health == "starting" {
		return " · " + c.Health
	}
	if len(n.Ports) > 0 {
		return " · :" + strings.Join(n.Ports, " :")
	}
	return ""
}

// containerShell is the command that opens a shell inside a running
// container: compose's exec, in the place, for the service.
func containerShell(c *Container) string {
	return "docker compose exec " + c.Service + " sh"
}

// composeFor is the pid of the deepest process of the compose run working
// in dir that runs the service — a compose up naming it, else one naming
// no service, which runs them all — or zero.
func composeFor(procs []Proc, dir, service string) int {
	isCompose := map[int]bool{}
	for _, p := range procs {
		if p.Dir == dir && p.Container == nil && runsCompose(p) {
			isCompose[p.PID] = true
		}
	}
	bottom := func(pid int) int {
		for {
			next := 0
			for _, c := range procs {
				if c.PPID == pid && isCompose[c.PID] {
					next = c.PID
					break
				}
			}
			if next == 0 {
				return pid
			}
			pid = next
		}
	}
	all := 0
	for _, p := range procs {
		if !isCompose[p.PID] || isCompose[p.PPID] {
			continue // not the top of a compose run
		}
		switch named := composeNames(p); {
		case slices.Contains(named, service):
			return bottom(p.PID)
		case len(named) == 0 && all == 0:
			all = bottom(p.PID)
		}
	}
	return all
}

// composeNames is the services a compose up names after its up, the
// words there that are not flags; none is every service. The words before
// up are compose's own — a file, a project name — and not services.
func composeNames(p Proc) []string {
	fields := strings.Fields(p.Argv)
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

// runsCompose reports a process that is compose: docker running its
// compose command, or the compose plugin itself.
func runsCompose(p Proc) bool {
	fields := strings.Fields(p.Argv)
	if len(fields) < 2 {
		return false
	}
	name := strings.TrimPrefix(fieldBase(fields[0]), "docker-")
	return (name == "docker" || name == "compose") && slices.Contains(fields[1:], "compose")
}

// fieldBase is the last element of a path, or the word itself.
func fieldBase(s string) string {
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[i+1:]
	}
	return s
}

// dockerStop asks docker to stop a container: the container's own
// process gets SIGTERM, and docker follows with SIGKILL after its grace,
// which is the kill a process gets, done docker's way. A container not
// running is removed instead — its row closed, the way an ended entry's
// shell is closed — and compose makes it again when the service is next
// brought up.
func dockerStop(c *Container) error {
	if !c.running() {
		_, err := listing(scanTimeout, dockerPath, "rm", c.ID)
		return err
	}
	_, err := listing(scanTimeout, dockerPath, "stop", c.ID)
	return err
}

// containerLogs is the last of what a container has written, both
// streams, for the pane.
func containerLogs(id string, lines int) []string {
	if dockerPath == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
	defer cancel()
	out, _ := exec.CommandContext(ctx, dockerPath, "logs", "--tail", strconv.Itoa(lines), id).CombinedOutput()
	said := strings.TrimRight(string(out), "\n")
	if said == "" {
		return nil
	}
	return strings.Split(said, "\n")
}

// containerFields describes a container the way procFields describes a
// process: what it is, where it belongs, and what docker says of it,
// then the last of its logs.
func containerFields(n *ProcNode) []field {
	c := n.Container
	statusTone := toneGood
	if containerWrong([]*ProcNode{n}) {
		statusTone = toneBad
	} else if !c.running() {
		statusTone = toneQuiet
	}
	fs := []field{
		heading(procLabel(n)),
		note(n.Dir),
		gap(),
		field{label: "container", value: c.Name, tone: toneName},
		field{label: "image", value: c.Image, tone: toneQuiet},
		field{label: "status", value: c.Status, tone: statusTone},
	}
	if len(n.Ports) > 0 {
		fs = append(fs, field{label: "publishes", value: strings.Join(n.Ports, ", "), tone: toneAccent})
	}
	return append(fs, transcript(containerLogs(c.ID, transcriptLines))...)
}

// A service x stopped is a service r brings back, where it was: the
// services a place's compose declares are what compose says it needs, and
// r starts the ones that are down with docker compose up -d for them —
// which starts a stopped container and makes again one that was removed —
// and the compose up running in a shell there takes the service's logs up
// again, so the shape holds: the service under the entry, as it was. Unless
// an entry r is starting runs compose itself, which brings up everything at
// once. With no compose up in a shell, the service runs detached, and its
// logs are on its own row's pane.

// composeFiles are the names compose reads a project from, in the order
// compose looks for them.
var composeFiles = []string{"compose.yaml", "compose.yml", "docker-compose.yml", "docker-compose.yaml"}

// composeFile is the file compose would read a project in dir from, or
// nothing.
func composeFile(dir string) string {
	for _, f := range composeFiles {
		if exists(filepath.Join(dir, f)) {
			return f
		}
	}
	return ""
}

// composeServices is every service the place's compose declares, in the
// file's order, asked of compose itself: it is the one reader that knows
// what its file means. Nothing without docker, or for a file compose
// refuses. It is a process — tens of milliseconds — asked once, on r.
func composeServices(dir string) []string {
	if dockerPath == "" {
		return nil
	}
	out, err := listingIn(dir, scanTimeout, dockerPath, "compose", "config", "--services")
	if err != nil {
		return nil
	}
	return strings.Fields(string(out))
}

// servicesUp is the services with a container running in dir, by the
// scan's containers.
func servicesUp(dir string, procs []Proc) map[string]bool {
	up := map[string]bool{}
	for _, p := range procs {
		if p.Container != nil && p.Dir == dir {
			up[p.Container.Service] = true
		}
	}
	return up
}

// startsCompose reports a plan entry among those about to start that runs
// compose, and so brings every service up itself.
func startsCompose(entries []entry) bool {
	for _, e := range entries {
		if runsCompose(Proc{Argv: e.Run}) {
			return true
		}
	}
	return false
}

// needs is what a place needs and is not running: the plan's entries not
// running, and — where the place runs compose and no entry starting now
// runs it — the services that are down, for compose to bring back. running
// is the entries running by name; procs is the scan, for the containers.
// A service a plan entry is named for is the entry's to run.
func needs(dir string, plan plan, running map[string]bool, procs []Proc) (entries []entry, services []string) {
	entries = plan.missing(running)
	if composeFile(dir) == "" || startsCompose(entries) {
		return entries, nil
	}
	named := map[string]bool{}
	for _, e := range plan.Entries {
		named[e.Name] = true
	}
	up := servicesUp(dir, procs)
	for _, s := range composeServices(dir) {
		if !named[s] && !up[s] {
			services = append(services, s)
		}
	}
	return entries, services
}

// composeUp brings services back, detached; the compose up in a shell
// there, if any, takes their logs up again on its own.
func composeUp(dir string, services []string) error {
	args := append([]string{"compose", "up", "-d", "--"}, services...)
	_, err := listingIn(dir, scanTimeout, dockerPath, args...)
	return err
}

// bringBack is r's word to compose for a place's services, off the render
// path, and what came of it.
func bringBack(p Project, services []string) tea.Cmd {
	return func() tea.Msg {
		return composeMsg{place: p, services: services, err: composeUp(p.Path, services)}
	}
}

// composeMsg says how bringing a place's services back went.
type composeMsg struct {
	place    Project
	services []string
	err      error
}
