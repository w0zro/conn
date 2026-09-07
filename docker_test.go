package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const dockerPS = `{"ID":"40de2f820e61","Names":"compose-demo-web-1","Image":"nginx:alpine","State":"running","Status":"Up 3 minutes","Labels":"com.docker.compose.project.working_dir=/p/demo,com.docker.compose.project=compose-demo,com.docker.compose.service=web,maintainer=NGINX Docker Maintainers <docker-maint@nginx.com>","Ports":"0.0.0.0:8438->80/tcp, [::]:8438->80/tcp"}
{"ID":"ed2d5cf6ab38","Names":"compose-demo-cache-1","Image":"redis:alpine","State":"running","Status":"Up 3 minutes","Labels":"com.docker.compose.project.working_dir=/p/demo,com.docker.compose.service=cache","Ports":"0.0.0.0:6390->6379/tcp, 6379/tcp"}
{"ID":"9a1b2c3d4e5f","Names":"skelly-postgres","Image":"postgres:16","State":"running","Status":"Up 2 hours","Labels":"","Ports":"5432/tcp"}
`

// dockerPSExited is a service of the same project that died.
const dockerPSExited = `{"ID":"c0ffee000001","Names":"compose-demo-worker-1","Image":"alpine","State":"exited","Status":"Exited (3) 3 minutes ago","Labels":"com.docker.compose.project.working_dir=/p/demo,com.docker.compose.project=compose-demo,com.docker.compose.service=worker","Ports":""}
`

func TestParseContainersReadsServiceDirectoryAndPorts(t *testing.T) {
	cs := parseContainers([]byte(dockerPS))
	if len(cs) != 3 || cs[2].Dir != dockerPlace {
		t.Fatalf("containers = %d, want the two compose started and one of docker's own place", len(cs))
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
		{"docker-compose up", false}, // v1 said no "compose"; the row stands alone
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

func TestALoneContainerKeepsARowOfItsOwn(t *testing.T) {
	// One service left running: the run is still compose, and the
	// service is under it, not folded into it as though it had gone.
	one := strings.SplitN(dockerPS, "\n", 2)[0] + "\n"
	procs := []Proc{
		{PID: 10, PPID: 1, Command: "zsh", Argv: "zsh", Dir: "/p/demo"},
		{PID: 20, PPID: 10, Command: "docker", Argv: "docker compose up", Dir: "/p/demo"},
		{PID: 30, PPID: 20, Command: "docker-compose", Argv: "docker-compose compose up", Dir: "/p/demo"},
	}
	procs = attachContainers(procs, parseContainers([]byte(one)))
	m := withProcList(80, 12, []Project{{Name: "demo", Path: "/p/demo"}}, procs)
	m.terms[10] = &remoteTerm{pid: 10, dir: "/p/demo", name: "app"}
	m.rebuild()
	wantRows(t, navColumn(m), []string{" ▸ demo", "      app", "        web · :8438"})
}

func TestNeedsIsThePlansEntriesThenTheServicesDown(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services:\n  web:\n    image: nginx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if composeFile(dir) != "compose.yaml" {
		t.Fatal("a compose.yaml is a compose project")
	}
	// The scan's containers say cache is up, and were started here.
	cs := parseContainers([]byte(dockerPS))
	for i := range cs {
		cs[i].Dir = dir
	}
	procs := attachContainers(nil, cs[1:]) // cache alone

	// An entry starting now that runs compose brings the services up itself.
	plan := plan{Entries: []entry{{Name: "app", Run: "docker compose up"}}}
	entries, services := needs(dir, plan, nil, procs)
	if len(entries) != 1 || entries[0].Name != "app" || len(services) != 0 {
		t.Errorf("needs = %v, %v; want the compose entry alone", entries, services)
	}
	// The compose entry running: what is left is the services down —
	// asked of compose, which these tests may not have.
	entries, services = needs(dir, plan, map[string]bool{"app": true}, procs)
	if len(entries) != 0 {
		t.Errorf("entries = %v, want none", entries)
	}
	if dockerPath == "" {
		if len(services) != 0 {
			t.Errorf("services = %v, want nothing without docker to ask", services)
		}
		return
	}
	if !slices.Equal(services, []string{"web"}) {
		t.Errorf("services = %v, want the one down", services)
	}
}

func TestAComposeFileIsAPlanOfOneEntry(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := readPlan(dir)
	if p.Source != "docker-compose.yml" || len(p.Entries) != 1 || p.Entries[0].Name != composeEntry || p.Entries[0].Run != "docker compose up" {
		t.Errorf("plan = %+v, want compose up, from the compose file", p)
	}
	// A plan of the project's own comes first.
	if err := os.WriteFile(filepath.Join(dir, ".conn"), []byte("app: docker compose up\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if p := readPlan(dir); p.Source != planFile {
		t.Errorf("plan = %+v, want the project's own", p)
	}
}

func TestRSaysWhatItStartedAndOnlyARefusalAfter(t *testing.T) {
	m := withProcs(80, 12, []Project{{Name: "demo", Path: t.TempDir()}}, nil)
	m, _ = pipeServer(t, m)
	if got := describeStarted([]entry{{Name: "app"}}, []string{"web"}); got != "app, web" {
		t.Errorf("described %q", got)
	}
	next, _ := m.Update(composeMsg{place: m.projects[0], services: []string{"web"}})
	if got := next.(model).status; got != "" {
		t.Errorf("status = %q, want nothing said of a start that went", got)
	}
	next, _ = m.Update(composeMsg{place: m.projects[0], services: []string{"web"}, err: errors.New("no daemon")})
	if got := next.(model).status; got != "could not start web in demo: no daemon" {
		t.Errorf("status = %q", got)
	}
}

func TestAnEntryThatRunsComposeBringsTheServicesUpItself(t *testing.T) {
	if !startsCompose([]entry{{Name: "app", Run: "docker compose up"}}) {
		t.Error("docker compose up brings the services up itself")
	}
	if startsCompose([]entry{{Name: "agent", Run: "claude"}}) {
		t.Error("an entry that is not compose leaves the services to r")
	}
}

func TestAContainerFilesUnderTheComposeThatNamesItsService(t *testing.T) {
	procs := []Proc{
		{PID: 20, PPID: 1, Command: "docker", Argv: "docker compose up", Dir: "/p/demo"},
		{PID: 30, PPID: 20, Command: "docker-compose", Argv: "docker-compose compose up", Dir: "/p/demo"},
		{PID: 40, PPID: 1, Command: "docker", Argv: "docker compose up web", Dir: "/p/demo"},
		{PID: 50, PPID: 40, Command: "docker-compose", Argv: "docker-compose compose up web", Dir: "/p/demo"},
	}
	if got := composeFor(procs, "/p/demo", "web"); got != 50 {
		t.Errorf("web under %d, want the compose up that names it", got)
	}
	if got := composeFor(procs, "/p/demo", "cache"); got != 30 {
		t.Errorf("cache under %d, want the compose up that names no service", got)
	}
	if got := composeNames(Proc{Argv: "docker compose -f x.yaml up -d web cache"}); !slices.Equal(got, []string{"web", "cache"}) {
		t.Errorf("names = %v", got)
	}
}

func TestAPlaceWithNeitherPlanNorComposeSaysSo(t *testing.T) {
	m := withProcs(80, 12, []Project{{Name: "demo", Path: t.TempDir()}}, nil)
	m, _ = pipeServer(t, m)
	m = press(m, "r")
	if m.status != "demo does not say what it needs" {
		t.Errorf("status = %q", m.status)
	}
}

func TestDockersWordsForStateAreRead(t *testing.T) {
	for _, tc := range []struct{ status, exit, health, ago string }{
		{"Up 3 minutes", "", "", "3m"},
		{"Up 3 minutes (healthy)", "", "healthy", "3m"},
		{"Up About an hour (unhealthy)", "", "unhealthy", "1h"},
		{"Up 2 seconds (health: starting)", "", "starting", "now"},
		{"Exited (3) 3 minutes ago", "3", "", "3m"},
		{"Exited (0) 11 months ago", "0", "", "44w"},
		{"Exited (137) Less than a second ago", "137", "", "now"},
		{"Created", "", "", ""},
	} {
		if got := exitOf(tc.status); got != tc.exit {
			t.Errorf("exitOf(%q) = %q, want %q", tc.status, got, tc.exit)
		}
		if got := healthOf(tc.status); got != tc.health {
			t.Errorf("healthOf(%q) = %q, want %q", tc.status, got, tc.health)
		}
		if got := agoOf(tc.status); got != tc.ago {
			t.Errorf("agoOf(%q) = %q, want %q", tc.status, got, tc.ago)
		}
	}
}

func TestAnExitedContainerIsListedWhileItsProjectRuns(t *testing.T) {
	// Beside running siblings: listed, as the thing that needs a look.
	cs := attachContainers(nil, parseContainers([]byte(dockerPS+dockerPSExited)))
	if len(cs) != 4 || cs[3].Container.Service != "worker" || cs[3].Container.Exit != "3" {
		t.Errorf("containers = %v, want the dead worker beside the two running", cs)
	}
	// Alone, its project over: not a failure to read every day after.
	if cs := attachContainers(nil, parseContainers([]byte(dockerPSExited))); len(cs) != 0 {
		t.Errorf("containers = %v, want an exited container of a stopped project left out", cs)
	}
	// Alone, but compose up still running in its directory: listed under it.
	compose := []Proc{{PID: 20, PPID: 1, Command: "docker", Argv: "docker compose up", Dir: "/p/demo"}}
	if cs := attachContainers(compose, parseContainers([]byte(dockerPSExited))); len(cs) != 2 || cs[1].PPID != 20 {
		t.Errorf("containers = %v, want the exited container under the compose still up", cs)
	}
}

// troubledModel is composeModel with a worker that died and a web whose
// health check is failing.
func troubledModel() model {
	rows := strings.Replace(dockerPS, `"Status":"Up 3 minutes","Labels":"com.docker.compose.project.working_dir=/p/demo,com.docker.compose.project=compose-demo,com.docker.compose.service=web`,
		`"Status":"Up 3 minutes (unhealthy)","Labels":"com.docker.compose.project.working_dir=/p/demo,com.docker.compose.project=compose-demo,com.docker.compose.service=web`, 1)
	procs := []Proc{
		{PID: 10, PPID: 1, Command: "zsh", Argv: "zsh", Dir: "/p/demo"},
		{PID: 20, PPID: 10, Command: "docker", Argv: "docker compose up", Dir: "/p/demo"},
		{PID: 30, PPID: 20, Command: "docker-compose", Argv: "docker-compose compose up", Dir: "/p/demo"},
	}
	procs = attachContainers(procs, parseContainers([]byte(rows+dockerPSExited)))
	m := withProcList(80, 12, []Project{{Name: "demo", Path: "/p/demo"}}, procs)
	m.terms[10] = &remoteTerm{pid: 10, dir: "/p/demo", name: "app"}
	m.rebuild()
	return m
}

func TestADeadServiceShowsTheCrossAndTabGoesToIt(t *testing.T) {
	m := troubledModel()
	wantRows(t, navColumn(m), []string{" ▸ demo", "      app", "        cache · :6390", "        web · unhealthy ✗", "        worker · 3m ✗"})
	var web, worker navRow
	for _, r := range m.rows {
		if r.kind == rowProc && r.node.Command == "web" {
			web = r
		}
		if r.kind == rowProc && r.node.Command == "worker" {
			worker = r
		}
	}
	if !m.needsYou(worker) || !m.needsYou(web) {
		t.Error("a service that died, and one its check calls unhealthy, need you")
	}
	m = press(m, "tab")
	if r, _ := m.selected(); r.node == nil || r.node.Container == nil || r.node.Command != "web" {
		t.Errorf("tab landed on %+v, want the first service in trouble", r.node)
	}
}

func TestEnterOnAContainerOpensAShellInsideIt(t *testing.T) {
	m, asked := pipeServer(t, troubledModel())
	for range 2 {
		m = press(m, "down") // onto cache
	}
	m = press(m, "enter")
	got := askedForKind(t, asked, kindOpen)
	if got.Run != "docker compose exec cache sh" || got.Dir != "/p/demo" {
		t.Errorf("asked %+v, want a shell inside cache, opened by compose in the place", got)
	}

	for range 2 {
		m = press(m, "down") // onto worker, which died
	}
	m = press(m, "enter")
	if m.status != "worker is not running" {
		t.Errorf("status = %q, want a dead service refused", m.status)
	}
}

func TestDockerNotAnsweringIsSaidOnce(t *testing.T) {
	docker.Lock()
	docker.stalled, docker.last = true, nil
	docker.Unlock()
	defer func() { docker.Lock(); docker.stalled = false; docker.Unlock() }()

	note, stalled := dockerNote(false)
	if note == "" || !stalled {
		t.Errorf("note = %q, stalled %v; want it said the first time", note, stalled)
	}
	if note, _ := dockerNote(true); note != "" {
		t.Errorf("note = %q, want nothing said again while it stays so", note)
	}

	m := withProcs(80, 12, []Project{{Name: "demo", Path: "/p/demo"}}, nil)
	next, _ := m.Update(procsMsg{docker: "docker is not answering; its containers are as last seen", stalled: true})
	m = next.(model)
	if !strings.Contains(m.status, "not answering") || !m.dockerStalled {
		t.Errorf("status = %q, stalled %v", m.status, m.dockerStalled)
	}
}

func TestAContainerOfNoProjectIsDockers(t *testing.T) {
	cs := parseContainers([]byte(dockerPS))
	if len(cs) != 3 || cs[2].Dir != dockerPlace || cs[2].Command != "skelly-postgres" {
		t.Fatalf("containers = %v, want the docker run's, named for itself, in docker's place", cs)
	}
	// Those of a compose project from a directory no root holds, and the
	// docker run's: all under docker, the compose ones named for their
	// project too where it is known.
	elsewhere := strings.ReplaceAll(dockerPS, "/p/demo", "/elsewhere")
	procs := attachContainers(nil, parseContainers([]byte(elsewhere)))
	m := withProcList(80, 12, []Project{{Name: "conn", Path: "/p/conn"}}, procs)
	rows := navColumn(m)
	if len(rows) != 5 || strings.TrimSpace(rows[1]) != "docker" || !strings.Contains(rows[3], "compose-demo/") || !strings.Contains(rows[4], "skelly-postgres") {
		t.Fatalf("rows = %q, want docker among the places with the three under it", rows)
	}

	// enter opens a shell inside it by docker's own exec; s has nowhere
	// to open one; n makes nothing there; x on the place stops them all.
	m, asked := pipeServer(t, m)
	for range 4 {
		m = press(m, "down") // onto skelly-postgres
	}
	m = press(m, "enter")
	if got := askedForKind(t, asked, kindOpen); got.Run != "docker exec -it skelly-postgres sh" {
		t.Errorf("asked %+v, want docker's exec for a container of no place", got)
	}
	m = press(m, "s")
	if !strings.Contains(m.status, "not a place to open a shell in") {
		t.Errorf("status = %q, want s refused on a container of no directory", m.status)
	}
	for range 3 {
		m = press(m, "up") // onto docker
	}
	m = press(m, "s")
	if !strings.Contains(m.status, "not a place to open a shell in") {
		t.Errorf("status = %q, want s refused on docker's row", m.status)
	}
	if got := m.newProjectDir(); got == dockerPlace {
		t.Error("n should not make a project in docker's place")
	}
	m = press(m, "x")
	if got := len(targets(m.pendingKill)); got != 3 {
		t.Errorf("x on docker asks for %d, want every container in it", got)
	}

	// The place is gone with its last container.
	m = withProcList(80, 12, []Project{{Name: "conn", Path: "/p/conn"}}, nil)
	if rows := navColumn(m); len(rows) != 1 {
		t.Errorf("rows = %q, want no docker row with nothing in it", rows)
	}
}

func TestAnExitedContainerOfNoProjectIsNotListed(t *testing.T) {
	exited := strings.Replace(dockerPS, `"Image":"postgres:16","State":"running","Status":"Up 2 hours"`, `"Image":"postgres:16","State":"exited","Status":"Exited (0) 5 months ago"`, 1)
	for _, c := range attachContainers(nil, parseContainers([]byte(exited))) {
		if c.Dir == dockerPlace {
			t.Errorf("listed %v, want a stopped container of no project left out", c)
		}
	}
}
