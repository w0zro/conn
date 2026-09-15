package main

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// What docker ps --format '{{json .}}' says of a compose project: two
// services publishing a port, one of them with a health check, a worker
// that exited badly, and a container docker run started, which compose
// wrote no directory on.
const dockerPS = `{"ID":"9f1c2d3e4a5b","Names":"compose-demo-web-1","Image":"nginx:alpine","State":"running","Status":"Up 3 minutes (healthy)","Ports":"0.0.0.0:8438->80/tcp, [::]:8438->80/tcp","Labels":"com.docker.compose.project=compose-demo,com.docker.compose.service=web,com.docker.compose.project.working_dir=/Users/w0zro/projects/compose-demo,maintainer=NGINX Docker Maintainers <docker-maint@nginx.com>"}
{"ID":"1a2b3c4d5e6f","Names":"compose-demo-cache-1","Image":"redis:alpine","State":"running","Status":"Up 3 minutes","Ports":"0.0.0.0:6390->6379/tcp","Labels":"com.docker.compose.project=compose-demo,com.docker.compose.service=cache,com.docker.compose.project.working_dir=/Users/w0zro/projects/compose-demo"}
{"ID":"abcdef012345","Names":"compose-demo-worker-1","Image":"alpine","State":"exited","Status":"Exited (3) 8 seconds ago","Ports":"","Labels":"com.docker.compose.project=compose-demo,com.docker.compose.service=worker,com.docker.compose.project.working_dir=/Users/w0zro/projects/compose-demo"}
{"ID":"ffee11223344","Names":"stray","Image":"postgres","State":"running","Status":"Up 2 hours","Ports":"0.0.0.0:5432->5432/tcp","Labels":""}
`

var dockerNow = time.Date(2026, 9, 15, 20, 0, 0, 0, time.UTC)

func containersFor(t *testing.T) []container {
	t.Helper()
	cs := parseContainers([]byte(dockerPS), dockerNow)
	if len(cs) != 4 {
		t.Fatalf("parsed %d containers, want 4", len(cs))
	}
	return cs
}

// docker ps is read a container to a line: the service and project off
// the labels compose wrote, the host side of the ports it publishes, and
// the status broken into the pieces the row needs — what it exited with,
// what its health check says, and how long ago that became true.
func TestDockerPsIsReadIntoContainers(t *testing.T) {
	cs := containersFor(t)

	web := cs[0]
	if web.service != "web" || web.project != "compose-demo" {
		t.Errorf("web is service %q of project %q", web.service, web.project)
	}
	if web.dir != "/Users/w0zro/projects/compose-demo" {
		t.Errorf("web's directory is %q", web.dir)
	}
	// Both families publish the same host port, and it is one port.
	if got := strings.Join(web.ports, ","); got != "8438" {
		t.Errorf("web publishes %q", got)
	}
	if web.health != "healthy" || web.exit != "" || !web.running() {
		t.Errorf("web: health %q exit %q state %q", web.health, web.exit, web.state)
	}
	// The maintainer's label holds a comma of its own; it must not eat
	// the labels that matter, which are read before it.
	if web.image != "nginx:alpine" {
		t.Errorf("web's image is %q", web.image)
	}
	if got := web.activity(); got != "web · :8438" {
		t.Errorf("web's row says %q", got)
	}

	// docker says an age; conn says a moment, so the column is written
	// the way every other row's is.
	if got := dockerNow.Sub(web.since); got != 3*time.Minute {
		t.Errorf("web has stood for %v, not three minutes", got)
	}

	worker := cs[2]
	if worker.exit != "3" || worker.running() {
		t.Errorf("worker: exit %q state %q", worker.exit, worker.state)
	}
	if got := dockerNow.Sub(worker.since); got != 8*time.Second {
		t.Errorf("worker exited %v ago, not eight seconds", got)
	}
	// A worker with no published port is named alone.
	if got := worker.activity(); got != "worker" {
		t.Errorf("worker's row says %q", got)
	}

	// A container compose did not start has no directory and no service
	// of its own, so its name stands for it.
	if stray := cs[3]; stray.dir != "" || stray.service != "stray" {
		t.Errorf("the stray: dir %q service %q", stray.dir, stray.service)
	}
}

// The status column says a container in the words it already uses, and a
// health check failing is a thing to look at: the service is up and
// answering wrongly, which is the case nobody notices unaided. An exit of
// zero is an ending and no fault; any other code is the fault it is.
func TestAContainersStatusIsSaidInTheColumnsOwnWords(t *testing.T) {
	for _, c := range []struct {
		container
		status string
		fault  bool
	}{
		{container{state: "running"}, statusActive, false},
		{container{state: "running", health: "healthy"}, statusActive, false},
		{container{state: "running", health: "starting"}, "STARTING", false},
		{container{state: "running", health: "unhealthy"}, "UNHEALTHY", true},
		{container{state: "restarting"}, "RESTARTING", true},
		{container{state: "paused"}, statusStopped, true},
		{container{state: "exited", exit: "0"}, statusEnded, false},
		{container{state: "exited", exit: "3"}, "EXIT 3", true},
	} {
		status, fault := containerStatus(c.container)
		if status != c.status || fault != c.fault {
			t.Errorf("%+v: %q fault=%v, want %q fault=%v", c.container, status, fault, c.status, c.fault)
		}
	}
}

// A container goes by a number below zero, where no process is, read off
// its id so the cursor holds its row from one reading to the next.
func TestAContainerHoldsItsRowByItsID(t *testing.T) {
	a, b := containerPID("9f1c2d3e4a5b"), containerPID("1a2b3c4d5e6f")
	if a >= 0 || b >= 0 {
		t.Errorf("containers took pids %d and %d, which a process could hold", a, b)
	}
	if a == b {
		t.Error("two containers share a row")
	}
	if containerPID("9f1c2d3e4a5b") != a {
		t.Error("a container's number moved between readings")
	}
}

// docker's ages, read back to durations. The health in parentheses comes
// after the age and is not part of it; an exit's age is the words before
// "ago".
func TestDockerAgesAreRead(t *testing.T) {
	for _, c := range []struct {
		status string
		want   time.Duration
		ok     bool
	}{
		{"Up 3 minutes (healthy)", 3 * time.Minute, true},
		{"Up About an hour", time.Hour, true},
		{"Up Less than a second", time.Second, true},
		{"Exited (1) 3 minutes ago", 3 * time.Minute, true},
		{"Exited (0) 2 hours ago", 2 * time.Hour, true},
		{"Created", 0, false},
	} {
		got, ok := ageOf(c.status)
		if got != c.want || ok != c.ok {
			t.Errorf("%q: %v %v, want %v %v", c.status, got, ok, c.want, c.ok)
		}
	}
}

// dockerRoots stands in for the root finder: the compose demo is its own
// project, and so is conn.
func dockerRoots(dir string) string {
	for _, root := range []string{"/Users/w0zro/projects/compose-demo", "/Users/w0zro/projects/w0zro/conn"} {
		if dir == root || strings.HasPrefix(dir, root+"/") {
			return root
		}
	}
	return dir
}

// Where a compose is running in a shell, its containers stand under it:
// the row that says docker compose up had nothing beneath it, and the
// services it is running are what it is doing.
func TestContainersStandUnderTheComposeThatRunsThem(t *testing.T) {
	projects := []project{{
		path: "/Users/w0zro/projects/compose-demo",
		entries: []entry{
			{pid: 100, kind: kindShell, command: "zsh", tty: "ttys003", cwd: "/Users/w0zro/projects/compose-demo"},
			{pid: 101, kind: kindRun, command: "docker compose up", tty: "ttys003", depth: 1,
				cwd: "/Users/w0zro/projects/compose-demo"},
		},
	}}
	out := attachContainers(projects, containersFor(t), dockerRoots, nil)
	if len(out) != 1 {
		t.Fatalf("%d projects, want 1: the stray belongs to none and is not filed", len(out))
	}
	rows := out[0].entries
	var got []string
	for _, e := range rows {
		got = append(got, strings.Repeat("  ", e.depth)+e.kind+" "+e.command)
	}
	want := []string{
		"SHELL zsh",
		"  RUN docker compose up",
		"    SERVICE cache · :6390",
		"    SERVICE web · :8438",
		"    SERVICE worker",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the rows read:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// The dead worker is listed beside its siblings, and says so.
	for _, e := range rows {
		if strings.HasPrefix(e.command, "worker") && (e.status != "EXIT 3" || !e.fault) {
			t.Errorf("the worker reads %q fault=%v", e.status, e.fault)
		}
	}
}

// Left detached there is no compose to stand under, and the project may
// have no processes at all — which is the case the process table cannot
// show, and the one that had conn saying nothing was running.
func TestDetachedContainersRootTheirOwnProject(t *testing.T) {
	// Work in another project, so the reading is not empty; nothing at
	// all in the compose demo.
	projects := []project{{
		path: "/Users/w0zro/projects/w0zro/conn",
		entries: []entry{{pid: 1, kind: kindShell, command: "zsh", tty: "ttys001",
			cwd: "/Users/w0zro/projects/w0zro/conn"}},
	}}
	out := attachContainers(projects, containersFor(t), dockerRoots, nil)
	if len(out) != 2 {
		t.Fatalf("%d projects, want 2: the compose demo is a project docker alone is working in", len(out))
	}
	if out[0].path != "/Users/w0zro/projects/w0zro/conn" {
		t.Errorf("the worked project lost its place to %q", out[0].path)
	}
	demo := out[1]
	if demo.path != "/Users/w0zro/projects/compose-demo" {
		t.Fatalf("the second project is %q", demo.path)
	}
	var got []string
	for _, e := range demo.entries {
		if e.depth != 0 {
			t.Errorf("%s stands at depth %d with no compose above it", e.command, e.depth)
		}
		got = append(got, e.command)
	}
	want := "cache · :6390, web · :8438, worker"
	if strings.Join(got, ", ") != want {
		t.Errorf("the rows read %q, want %q", strings.Join(got, ", "), want)
	}
	// A container conn holds no pane for is a row it can only report,
	// which is what the terminal being blank says.
	for _, e := range demo.entries {
		if e.tty != "" {
			t.Errorf("%s claims terminal %q", e.command, e.tty)
		}
	}
}

// A project stopped whole is over. Its containers are not a list of
// yesterday's failures to read again every day, so they go; a service
// that died while its siblings run is exactly what wants noticing, and
// stays.
func TestAProjectStoppedWholeIsNotListed(t *testing.T) {
	cs := containersFor(t)
	for i := range cs {
		if cs[i].project == "compose-demo" {
			cs[i].state, cs[i].exit = "exited", "0"
		}
	}
	out := attachContainers(nil, cs, dockerRoots, nil)
	if len(out) != 0 {
		t.Errorf("a project with nothing running left %d projects: %+v", len(out), out)
	}
}

// A container's row is reached like any other once conn has opened a
// pane for it: the pane stands in for the terminal a container has not
// got, and from there enter, the ring and esc all work unchanged.
func TestAContainerTakesThePaneConnOpenedForIt(t *testing.T) {
	cs := containersFor(t)
	paneOf := map[string]string{cs[0].id: "ttys009"}
	out := attachContainers(nil, cs, dockerRoots, paneOf)
	if len(out) != 1 {
		t.Fatalf("%d projects, want 1", len(out))
	}
	var web, worker entry
	for _, e := range out[0].entries {
		switch {
		case strings.HasPrefix(e.command, "web"):
			web = e
		case strings.HasPrefix(e.command, "worker"):
			worker = e
		}
	}
	if web.tty != "ttys009" {
		t.Errorf("web stands on terminal %q, not the pane conn opened", web.tty)
	}
	if web.container != cs[0].id {
		t.Errorf("web's row carries container %q", web.container)
	}
	// One conn has opened nothing for has no terminal, which is what
	// says the row can only be reported.
	if worker.tty != "" {
		t.Errorf("the worker claims terminal %q with no pane opened", worker.tty)
	}
	if worker.container == "" {
		t.Error("the worker's row carries no container for the keys to act on")
	}
}

// enter on a container opens its output; enter again, once there is a
// pane, goes into it the way enter goes into anything. s opens a shell
// inside the container rather than at the directory it was started for.
func TestEnterAndSActOnTheContainer(t *testing.T) {
	m := newModel(plain)
	m.view, m.inside = viewProcesses, true
	m.srv = &server{tmux: "/nonexistent/tmux", socket: "/tmp/none"}
	m.said, m.saidKeys = true, m.keys()
	m.projects = []project{{path: "/p", entries: []entry{
		{pid: -99, kind: kindService, command: "web · :8438", container: "abc123", cwd: "/p", status: statusActive},
	}}}
	m.cursor = -99

	press := func(k string) tea.Cmd {
		_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: k}))
		return cmd
	}
	if _, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})); cmd == nil {
		t.Error("enter on a container asked for nothing")
	}
	if press("s") == nil {
		t.Error("s on a container asked for nothing")
	}

	// With no container and no pane there is nothing to ask for, so
	// neither key invents one.
	m.projects[0].entries[0].container = ""
	if _, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})); cmd != nil {
		t.Error("enter opened something for a row that is neither reachable nor a container")
	}
}

// The page for a container is composed from what docker said, not from
// the process table, which has no record of it. It is named by its id,
// since the number conn files it under is conn's own bookkeeping.
func TestTheServicePageIsComposedFromDocker(t *testing.T) {
	now := time.Date(2026, 9, 15, 22, 0, 0, 0, time.UTC)
	c := container{
		id: "94e3da190ba7", name: "compose-demo-web-1", service: "web", project: "compose-demo",
		image: "nginx:alpine", state: "running", status: "Up 3 minutes (healthy)", health: "healthy",
		dir: "/Users/w0zro/projects/compose-demo", ports: []string{"8438"}, since: now.Add(-3 * time.Minute),
	}
	b := composeReadout(readoutSubject{container: &c, inside: true}, "/Users/w0zro", now)
	if b.name != c.id {
		t.Errorf("the page is headed %q, not the container's id", b.name)
	}
	text := texts(drawReadout(b, 60, 24, plain))
	for _, want := range []string{
		"94e3da190ba7", // the header, in place of a pid conn invented
		"IMAGE ..... NGINX:ALPINE",
		"UP 3 MINUTES (HEALTHY)", // docker's own sentence, which says it best
		"HEALTH .... HEALTHY",
		"COMPOSE ... COMPOSE-DEMO",
		"NAME ...... COMPOSE-DEMO-WEB-1",
		"PORTS ..... LOCALHOST:8438",
		"ENTER OPENS ITS LOG",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the page lacks %q:\n%s", want, text)
		}
	}
	// Nothing from the process page, which asks the table things it
	// cannot answer for a container.
	for _, unwanted := range []string{"PID ", "STATE ", "CPU ", "COMMAND "} {
		if strings.Contains(text, unwanted) {
			t.Errorf("the page still says %q:\n%s", unwanted, text)
		}
	}

	// A service that went wrong says so past docker's sentence, since
	// that buries it: Exited (3) reads as a fact, not as a fault.
	c.state, c.exit, c.health, c.status = "exited", "3", "", "Exited (3) 8 seconds ago"
	text = texts(drawReadout(composeReadout(readoutSubject{container: &c, inside: true}, "/Users/w0zro", now), 60, 24, plain))
	if !strings.Contains(text, "WRONG ..... EXIT 3") {
		t.Errorf("a dead service does not say what went wrong:\n%s", text)
	}
}

// x on a container asks docker to stop it, and says stop rather than
// kill: there is no process here to signal. One already stopped is left
// alone, the question being about nothing.
func TestXStopsAContainer(t *testing.T) {
	m := newModel(plain)
	m.view, m.inside = viewProcesses, true
	m.srv = &server{tmux: "/nonexistent/tmux", socket: "/tmp/none"}
	m.projects = []project{{path: "/p", entries: []entry{
		{pid: -99, kind: kindService, command: "web · :8438", container: "abc123", cwd: "/p", status: statusActive},
		{pid: -98, kind: kindService, command: "worker", container: "def456", cwd: "/p", status: statusEnded},
	}}}
	m.containers = []container{{id: "abc123", service: "web"}, {id: "def456", service: "worker"}}

	m.cursor = -99
	next, _ := m.Update(tea.KeyPressMsg(tea.Key{Text: "x"}))
	m = next.(model)
	if m.kill == nil || m.kill.container != "abc123" {
		t.Fatalf("x armed %+v", m.kill)
	}
	if !strings.Contains(m.kill.prompt, "STOP WEB") {
		t.Errorf("the question reads %q", m.kill.prompt)
	}
	if strings.Contains(m.kill.prompt, "KILL") || strings.Contains(m.kill.prompt, "-99") {
		t.Errorf("the question talks of killing or of a pid: %q", m.kill.prompt)
	}
	// By the service, not by the row's label, which carries the ports.
	if strings.Contains(m.kill.prompt, ":8438") {
		t.Errorf("the question asks about an address: %q", m.kill.prompt)
	}
	// Confirming asks docker rather than signalling anything.
	if _, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "x"})); cmd == nil {
		t.Error("confirming the stop asked for nothing")
	}

	// A service already gone is not asked about.
	m.kill, m.cursor = nil, -98
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Text: "x"}))
	if got := next.(model); got.kill != nil {
		t.Errorf("x armed a question on a service already stopped: %+v", got.kill)
	}
}
