package main

import (
	"slices"
	"strings"
	"testing"
)

const dockerPS = `{"ID":"40de2f820e61","Names":"compose-demo-web-1","Image":"nginx:alpine","Status":"Up 3 minutes","Labels":"com.docker.compose.project.working_dir=/p/demo,com.docker.compose.project=compose-demo,com.docker.compose.service=web,maintainer=NGINX Docker Maintainers <docker-maint@nginx.com>","Ports":"0.0.0.0:8438->80/tcp, [::]:8438->80/tcp"}
{"ID":"ed2d5cf6ab38","Names":"compose-demo-cache-1","Image":"redis:alpine","Status":"Up 3 minutes","Labels":"com.docker.compose.project.working_dir=/p/demo,com.docker.compose.service=cache","Ports":"0.0.0.0:6390->6379/tcp, 6379/tcp"}
{"ID":"9a1b2c3d4e5f","Names":"skelly-postgres","Image":"postgres:16","Status":"Up 2 hours","Labels":"","Ports":"5432/tcp"}
`

func TestParseContainersReadsServiceDirectoryAndPorts(t *testing.T) {
	cs := parseContainers([]byte(dockerPS))
	if len(cs) != 2 {
		t.Fatalf("containers = %d, want the two compose started; one without a directory belongs nowhere", len(cs))
	}
	web, cache := cs[0], cs[1]
	if web.Command != "web" || web.Dir != "/p/demo" || web.Container.Image != "nginx:alpine" {
		t.Errorf("web = %+v, want its service, directory and image", web)
	}
	if got := web.Ports; !slices.Equal(got, []string{"8438"}) {
		t.Errorf("web ports = %v, want the host port once, both address families", got)
	}
	if got := cache.Ports; !slices.Equal(got, []string{"6390"}) {
		t.Errorf("cache ports = %v, want the published port and not the container's own", got)
	}
	if web.PID >= 0 || cache.PID >= 0 || web.PID == cache.PID {
		t.Errorf("pids %d and %d, want numbers of conn's own below zero, distinct", web.PID, cache.PID)
	}
	if web.PID != containerPID("40de2f820e61") {
		t.Error("a container's number should be the same from one scan to the next")
	}
}

func TestParseLabelsKeepsACommaInsideAValue(t *testing.T) {
	labels := parseLabels("a=1,maintainer=Someone, Inc <x@y>,com.docker.compose.service=web")
	if labels["maintainer"] != "Someone, Inc <x@y>" || labels["com.docker.compose.service"] != "web" {
		t.Errorf("labels = %v", labels)
	}
}

func TestContainersFileUnderTheComposeThatRunsThem(t *testing.T) {
	procs := []Proc{
		{PID: 10, PPID: 1, Command: "zsh", Argv: "zsh", Dir: "/p/demo"},
		{PID: 20, PPID: 10, Command: "docker", Argv: "docker compose up", Dir: "/p/demo"},
		{PID: 30, PPID: 20, Command: "docker-compose", Argv: "/x/cli-plugins/docker-compose compose up", Dir: "/p/demo"},
		{PID: 40, PPID: 1, Command: "docker", Argv: "docker compose up", Dir: "/p/other"},
	}
	cs := attachContainers(procs, parseContainers([]byte(dockerPS)))
	byName := map[string]Proc{}
	for _, p := range cs {
		byName[p.Command] = p
	}
	if got := byName["web"].PPID; got != 30 {
		t.Errorf("web under %d, want the compose plugin, the deepest of the run in its directory", got)
	}
	if got := byName["cache"].PPID; got != 30 {
		t.Errorf("cache under %d, want the same compose", got)
	}

	// Compose left detached: the container is a root under its place.
	detached := attachContainers(nil, parseContainers([]byte(dockerPS)))
	if detached[0].PPID != 0 {
		t.Errorf("with no compose running, ppid = %d, want a root", detached[0].PPID)
	}
}

func TestRunsComposeSpotsTheCommandAndThePlugin(t *testing.T) {
	for _, tc := range []struct {
		argv string
		want bool
	}{
		{"docker compose up", true},
		{"/usr/local/bin/docker compose -f x.yaml up -d", true},
		{"/Users/x/.docker/cli-plugins/docker-compose compose up", true},
		{"docker-compose up", false}, // v1 said no "compose"; the row is its own
		{"docker ps", false},
		{"zsh", false},
	} {
		if got := runsCompose(Proc{Argv: tc.argv}); got != tc.want {
			t.Errorf("runsCompose(%q) = %v, want %v", tc.argv, got, tc.want)
		}
	}
}

// composeModel is a place with compose up running in a held shell and two
// containers under it.
func composeModel() model {
	procs := []Proc{
		{PID: 10, PPID: 1, Command: "zsh", Argv: "zsh", Dir: "/p/demo"},
		{PID: 20, PPID: 10, Command: "docker", Argv: "docker compose up", Dir: "/p/demo"},
		{PID: 30, PPID: 20, Command: "docker-compose", Argv: "docker-compose compose up", Dir: "/p/demo"},
	}
	procs = attachContainers(procs, parseContainers([]byte(dockerPS)))
	m := withProcList(80, 12, []Project{{Name: "demo", Path: "/p/demo"}}, procs)
	m.terms[10] = &remoteTerm{pid: 10, dir: "/p/demo", name: "app"}
	m.rebuild()
	return m
}

func TestAContainerRowIsNamedForItsServiceWithItsPorts(t *testing.T) {
	m := composeModel()
	wantRows(t, navColumn(m), []string{" ▸ demo", "      app", "        cache · :6390", "        web · :8438"})

	m = press(m, "-") // unfolded, the rows show what they go by
	col := strings.Join(navColumn(m), "\n")
	// The column cuts a long name from the right, so the id shows as far as
	// it fits.
	if !strings.Contains(col, "web 40de2") || !strings.Contains(col, "cache ed2") {
		t.Errorf("unfolded rows should show a container's id:\n%s", col)
	}
}

func TestAContainerIsDimAndKilledByDocker(t *testing.T) {
	m := composeModel()
	for range 2 {
		m = press(m, "down") // onto cache
	}
	r, _ := m.selected()
	if r.node.Container == nil || r.node.Command != "cache" {
		t.Fatalf("selected %+v, want the cache container", r.node)
	}
	if m.attachable(r) {
		t.Error("a container has no shell to enter")
	}
	m = press(m, "x")
	if f := footer(m); !strings.Contains(f, "kill cache ed2d5cf6ab38?") {
		t.Errorf("footer = %q, want the container named by its service and id", f)
	}
}

func TestAServiceCalledClaudeIsNotAnAgent(t *testing.T) {
	n := &ProcNode{Proc: Proc{PID: -5, Command: "claude", Container: &Container{ID: "abc", Service: "claude"}}}
	if _, ok := agentKindOf(n); ok {
		t.Error("a container named claude is a service, not an instance conn can read")
	}
}

func TestContainersListsWhatDockerRuns(t *testing.T) {
	if dockerPath == "" {
		t.Skip("no docker")
	}
	cs := containers()
	for _, c := range cs {
		if c.PID >= 0 || c.Dir == "" || c.Container == nil {
			t.Errorf("container %+v, want a number below zero, a directory and what docker said", c)
		}
	}
	if len(cs) > 0 {
		n := &ProcNode{Proc: cs[0]}
		if fs := containerFields(n); len(fs) < 5 {
			t.Errorf("fields = %+v, want the container described", fs)
		}
	}
}
