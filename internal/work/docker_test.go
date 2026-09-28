package work

import (
	"strings"
	"testing"
	"time"
)

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

// A container's row is reached like any other once conn has opened a
// pane for it: the pane stands in for the terminal a container has not
// got, and from there enter, the ring and esc all work unchanged.
func TestAContainerTakesThePaneConnOpenedForIt(t *testing.T) {
	cs := containersFor(t)
	paneOf := map[string]string{cs[0].ID: "ttys009"}
	out := AttachContainers(nil, cs, dockerRoots, paneOf, nil)
	if len(out) != 1 {
		t.Fatalf("%d projects, want 1", len(out))
	}
	var web, worker Entry
	for _, e := range out[0].Entries {
		switch {
		case strings.HasPrefix(e.Command, "web"):
			web = e
		case strings.HasPrefix(e.Command, "worker"):
			worker = e
		}
	}
	if web.TTY != "ttys009" {
		t.Errorf("web stands on terminal %q, not the pane conn opened", web.TTY)
	}
	if web.Container != cs[0].ID {
		t.Errorf("web's row carries container %q", web.Container)
	}
	// One conn has opened nothing for has no terminal, which is what
	// says the row can only be reported.
	if worker.TTY != "" {
		t.Errorf("the worker claims terminal %q with no pane opened", worker.TTY)
	}
	if worker.Container == "" {
		t.Error("the worker's row carries no container for the keys to act on")
	}
}

// The status column says a container in the words it already uses, and a
// health check failing is a thing to look at: the service is up and
// answering wrongly, which is the case nobody notices unaided. An exit of
// zero is an ending and no fault; any other code is the fault it is.
func TestAContainersStatusIsSaidInTheColumnsOwnWords(t *testing.T) {
	for _, c := range []struct {
		Container
		status string
		fault  bool
	}{
		{Container{State: "running"}, StatusActive, false},
		{Container{State: "running", Health: "healthy"}, StatusActive, false},
		{Container{State: "running", Health: "starting"}, "STARTING", false},
		{Container{State: "running", Health: "unhealthy"}, "UNHEALTHY", true},
		{Container{State: "restarting"}, "RESTARTING", true},
		{Container{State: "paused"}, StatusStopped, true},
		{Container{State: "exited", Exit: "0"}, StatusEnded, false},
		{Container{State: "exited", Exit: "3"}, "EXIT 3", true},
	} {
		status, fault := ContainerStatus(c.Container)
		if status != c.status || fault != c.fault {
			t.Errorf("%+v: %q fault=%v, want %q fault=%v", c.Container, status, fault, c.status, c.fault)
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
		if cs[i].Project == "compose-demo" {
			cs[i].State, cs[i].Exit = "exited", "0"
		}
	}
	out := AttachContainers(nil, cs, dockerRoots, nil, nil)
	if len(out) != 0 {
		t.Errorf("a project with nothing running left %d projects: %+v", len(out), out)
	}
}

// A shell conn opened inside a container is the operator's own work: it
// is listed, it stands under the service it is inside, and it does not
// take the slot the service's terminal is in. Both panes carry the same
// container id, on different marks, and the one that stands for the
// service is the one reading it — otherwise the row points at whichever
// of the two was ranged over last, and enter stops going back to the
// log once a shell has been opened.
func TestAShellInAContainerStandsUnderItAndNotInItsSlot(t *testing.T) {
	cs := containersFor(t)
	id := cs[0].ID
	pl := Project{Path: dockerRoots(cs[0].Dir), Entries: []Entry{
		{PID: 900, Kind: KindRun, Command: "docker exec -it " + id + " sh", Typed: "docker exec",
			TTY: "ttys012", Cwd: cs[0].Dir},
	}}
	out := AttachContainers([]Project{pl}, cs, dockerRoots,
		map[string]string{id: "ttys009"}, map[string]string{"ttys012": id})
	if len(out) != 1 {
		t.Fatalf("%d projects, want 1", len(out))
	}
	var service, shell Entry
	var at int
	for i, e := range out[0].Entries {
		switch {
		case e.Container == id:
			service, at = e, i
		case e.PID == 900:
			shell = e
		}
	}
	if service.TTY != "ttys009" {
		t.Errorf("the service's terminal is %q, not the pane reading it", service.TTY)
	}
	if shell.PID == 0 {
		t.Fatalf("the shell is not listed: %+v", out[0].Entries)
	}
	if shell.Kind != KindShell || shell.Typed != "sh in "+cs[0].Service {
		t.Errorf("the shell reads %s %q", shell.Kind, shell.Typed)
	}
	if shell.Depth != service.Depth+1 {
		t.Errorf("the shell is at depth %d, the service at %d", shell.Depth, service.Depth)
	}
	if at+1 >= len(out[0].Entries) || out[0].Entries[at+1].PID != 900 {
		t.Errorf("the shell does not stand under its service:\n%+v", out[0].Entries)
	}
	// The shell is not a second handle on the container: a key that
	// stops a service must not be armed from a row that is only in one.
	if shell.Container != "" {
		t.Errorf("the shell carries container %q", shell.Container)
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

// docker ps is read a container to a line: the service and project off
// the labels compose wrote, the host side of the ports it publishes, and
// the status broken into the pieces the row needs — what it exited with,
// what its health check says, and how long ago that became true.
func TestDockerPsIsReadIntoContainers(t *testing.T) {
	cs := containersFor(t)

	web := cs[0]
	if web.Service != "web" || web.Project != "compose-demo" {
		t.Errorf("web is service %q of project %q", web.Service, web.Project)
	}
	if web.Dir != "/Users/w0zro/projects/compose-demo" {
		t.Errorf("web's directory is %q", web.Dir)
	}
	// Both families publish the same host port, and it is one port.
	if got := strings.Join(web.Ports, ","); got != "8438" {
		t.Errorf("web publishes %q", got)
	}
	if web.Health != "healthy" || web.Exit != "" || !web.running() {
		t.Errorf("web: health %q exit %q state %q", web.Health, web.Exit, web.State)
	}
	// The maintainer's label holds a comma of its own; it must not eat
	// the labels that matter, which are read before it.
	if web.Image != "nginx:alpine" {
		t.Errorf("web's image is %q", web.Image)
	}

	// docker says an age; conn says a moment, so the column is written
	// the way every other row's is.
	if got := dockerNow.Sub(web.Since); got != 3*time.Minute {
		t.Errorf("web has stood for %v, not three minutes", got)
	}

	worker := cs[2]
	if worker.Exit != "3" || worker.running() {
		t.Errorf("worker: exit %q state %q", worker.Exit, worker.State)
	}
	if got := dockerNow.Sub(worker.Since); got != 8*time.Second {
		t.Errorf("worker exited %v ago, not eight seconds", got)
	}

	// A container compose did not start has no directory and no service
	// of its own, so its name stands for it.
	if stray := cs[3]; stray.Dir != "" || stray.Service != "stray" {
		t.Errorf("the stray: dir %q service %q", stray.Dir, stray.Service)
	}
}

func containersFor(t *testing.T) []Container {
	t.Helper()
	cs := ParseContainers([]byte(dockerPS), dockerNow)
	if len(cs) != 4 {
		t.Fatalf("parsed %d containers, want 4", len(cs))
	}
	return cs
}

var dockerNow = time.Date(2026, 9, 15, 20, 0, 0, 0, time.UTC)

// What docker ps --format '{{json .}}' says of a compose project: two
// services publishing a port, one of them with a health check, a worker
// that exited badly, and a container docker run started, which compose
// wrote no directory on.
const dockerPS = `{"ID":"9f1c2d3e4a5b","Names":"compose-demo-web-1","Image":"nginx:alpine","State":"running","Status":"Up 3 minutes (healthy)","Ports":"0.0.0.0:8438->80/tcp, [::]:8438->80/tcp","Labels":"com.docker.compose.project=compose-demo,com.docker.compose.service=web,com.docker.compose.project.working_dir=/Users/w0zro/projects/compose-demo,maintainer=NGINX Docker Maintainers <docker-maint@nginx.com>"}
{"ID":"1a2b3c4d5e6f","Names":"compose-demo-cache-1","Image":"redis:alpine","State":"running","Status":"Up 3 minutes","Ports":"0.0.0.0:6390->6379/tcp","Labels":"com.docker.compose.project=compose-demo,com.docker.compose.service=cache,com.docker.compose.project.working_dir=/Users/w0zro/projects/compose-demo"}
{"ID":"abcdef012345","Names":"compose-demo-worker-1","Image":"alpine","State":"exited","Status":"Exited (3) 8 seconds ago","Ports":"","Labels":"com.docker.compose.project=compose-demo,com.docker.compose.service=worker,com.docker.compose.project.working_dir=/Users/w0zro/projects/compose-demo"}
{"ID":"ffee11223344","Names":"stray","Image":"postgres","State":"running","Status":"Up 2 hours","Ports":"0.0.0.0:5432->5432/tcp","Labels":""}
`

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
