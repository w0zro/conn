package main

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// brew services info --all --json as brew writes it: a service never
// started, one running under a pid, and one whose last run ended badly.
const brewInfo = `[
  {"name": "herdr", "running": false, "loaded": false, "pid": null, "exit_code": null, "status": "none",
   "command": "/opt/homebrew/opt/herdr/bin/herdr server", "log_path": "/opt/homebrew/var/log/herdr.log"},
  {"name": "postgresql@14", "running": true, "loaded": true, "pid": 24422, "exit_code": 0, "status": "started",
   "command": "/opt/homebrew/opt/postgresql@14/bin/postgres -D /opt/homebrew/var/postgresql@14",
   "log_path": "/opt/homebrew/var/log/postgresql@14.log"},
  {"name": "redis", "running": false, "loaded": true, "pid": null, "exit_code": 78, "status": "error",
   "command": "/opt/homebrew/opt/redis/bin/redis-server /opt/homebrew/etc/redis.conf", "log_path": "/opt/homebrew/var/log/redis.log"}
]`

// A declaration that starts a service with brew names the formula; any
// other line, and a brew line that is not a start, does not.
func TestABrewDeclarationNamesItsFormula(t *testing.T) {
	for command, want := range map[string]string{
		"brew services start postgresql@14": "postgresql@14",
		"brew services run redis":           "redis",
		"brew services start --all":         "",
		"brew services stop redis":          "",
		"brew install redis":                "",
		"npm run dev":                       "",
	} {
		got, ok := brewArgs(command)
		if got != want || ok != (want != "") {
			t.Errorf("%q names %q, want %q", command, got, want)
		}
	}
}

// Brew's answer is read for what each service is: running under what
// pid, or not, and with what exit where its last run ended badly. A
// nought exit is no exit.
func TestBrewServicesAreReadFromBrew(t *testing.T) {
	services, err := parseBrewServices([]byte(brewInfo))
	if err != nil || len(services) != 3 {
		t.Fatalf("read %d services, %v", len(services), err)
	}
	pg := brewServiceNamed(services, "postgresql@14")
	if pg == nil || !pg.running || pg.pid != 24422 || pg.exit != "" || pg.log != "/opt/homebrew/var/log/postgresql@14.log" {
		t.Errorf("postgres read as %+v", pg)
	}
	if word, fault := brewStatus(*pg); word != statusActive || fault {
		t.Errorf("a running service reads %s", word)
	}
	if word, fault := brewStatus(*brewServiceNamed(services, "herdr")); word != statusDown || fault {
		t.Errorf("a service never started reads %s", word)
	}
	if word, fault := brewStatus(*brewServiceNamed(services, "redis")); word != "EXIT 78" || !fault {
		t.Errorf("a service that ended badly reads %s, fault %v", word, fault)
	}
	if brewServiceNamed(services, "nothing") != nil {
		t.Error("a formula brew did not report was found")
	}
}

// A declared brew service is a row of its project as brew reports it:
// down before brew has said anything of it, and active with the pid
// and the ports of the process running it once it has, labelled by its
// declared name and marked as the declaration. A pane opened on its
// log is its terminal. A project with no block shows nothing of it,
// and the ordinary declared rows do not take it for a pane to raise.
func TestABrewServiceIsARowAsBrewReportsIt(t *testing.T) {
	services, _ := parseBrewServices([]byte(brewInfo))
	decl := map[string]declared{
		"/w/a": {list: []declaration{{name: "db", command: "brew services start postgresql@14"}, {name: "web", command: "npm run dev"}}},
		"/w/q": {list: []declaration{{name: "db", command: "brew services start postgresql@14"}}},
	}
	projects := []project{{path: "/w/a", entries: []entry{{pid: 1, kind: kindShell, command: "zsh", tty: "ttys001", status: statusIdle}}}}
	sockets := map[int][]socket{24422: {{"TCP", "127.0.0.1:5432", "LISTEN"}, {"TCP", "[::1]:5432", "LISTEN"}, {"unix", "/tmp/.s.PGSQL.5432", ""}}}

	out := attachDeclared(projects, decl, nil)
	if n := len(out[0].entries); n != 2 {
		t.Fatalf("the declarations made %d rows; the brew one is not a pane's", n-1)
	}
	if e := out[0].entries[1]; !strings.HasPrefix(e.command, "web") || e.status != statusDown {
		t.Errorf("the ordinary declaration is %+v", e)
	}

	out = attachBrew(out, decl, nil, nil, nil)
	if n := len(out); n != 1 {
		t.Fatalf("a project with no block grew one: %d projects", n)
	}
	if n := len(out[0].entries); n != 3 {
		t.Fatalf("the brew service is not a row: %d rows", n)
	}
	down := out[0].entries[2]
	if down.kind != kindService || down.brew != "postgresql@14" || down.command != "postgresql@14" || down.status != statusDown || down.pid >= 0 || declaredNameOf(down) != "db" {
		t.Errorf("before brew has spoken the row is %+v", down)
	}

	out = attachBrew(attachDeclared(projects, decl, nil), decl, services, sockets, map[string]string{brewMark("postgresql@14"): "ttys009"})
	up := out[0].entries[2]
	if up.status != statusActive || up.fault || up.pid != 24422 || strings.Join(up.ports, " ") != "5432" || len(up.sockets) != 3 || up.tty != "ttys009" || up.cwd != "/w/a" {
		t.Errorf("as brew reports it the row is %+v", up)
	}
	// Up by brew's word, for u to leave alone; the other declaration
	// is down and would be raised.
	upBy, _ := upAndHeld(out, nil, "/w/a")
	if !upBy[markDeclared("/w/a", "db")] || upBy[markDeclared("/w/a", "web")] {
		t.Errorf("u sees as up: %v", upBy)
	}
}

// A service two projects declare is one service: the panel files it
// once, under the first, saying * for the project, and puts it back
// under that project when the reading is by project again.
func TestAServiceTwoProjectsDeclareIsFiledOnce(t *testing.T) {
	services, _ := parseBrewServices([]byte(brewInfo))
	decl := map[string]declared{
		"/w/a": {list: []declaration{{name: "db", command: "brew services start postgresql@14"}}},
		"/w/q": {list: []declaration{{name: "pg", command: "brew services start postgresql@14"}, {name: "cache", command: "brew services start redis"}}},
	}
	projects := []project{
		{path: "/w/a", entries: []entry{{pid: 1, kind: kindShell, command: "zsh", tty: "ttys001", status: statusIdle}}},
		{path: "/w/q", entries: []entry{{pid: 2, kind: kindShell, command: "zsh", tty: "ttys002", status: statusIdle}}},
	}
	sockets := map[int][]socket{24422: {{"TCP", "127.0.0.1:5432", "LISTEN"}}}
	out := attachBrew(projects, decl, services, sockets, nil)
	filed := byState(out)
	var rows []string
	for _, pl := range filed {
		for _, e := range pl.entries {
			if e.brew != "" {
				rows = append(rows, groupTitle(pl.path)+" "+e.brew+" "+filedFrom(e, nil, "/Users/w0zro")+" x"+string(rune('0'+e.shared)))
			}
		}
	}
	// A service that ended badly is a fault among the idle rows, with
	// its stamp, as a container that exited is.
	if want := "SERVING postgresql@14 * x2, IDLE redis /w/q x1"; strings.Join(rows, ", ") != want {
		t.Errorf("filed as %q, want %q", strings.Join(rows, ", "), want)
	}
	back := unfiled(filed)
	var under []string
	for _, pl := range back {
		for _, e := range pl.entries {
			if e.brew != "" {
				under = append(under, pl.path+" "+e.brew)
			}
		}
	}
	if len(back) != 2 || strings.Join(under, ", ") != "/w/a postgresql@14, /w/q redis" {
		t.Errorf("unfiled: %v in %d projects", under, len(back))
	}
}

// Enter on a brew service up opens its log, and down asks brew to
// start it; x asks brew to stop one that is up, by its declared name,
// and leaves one down alone. The bar says as much.
func TestTheKeysAskBrewAboutItsService(t *testing.T) {
	m := newModel(plain)
	m.view, m.inside = viewProcesses, true
	m.srv = &server{tmux: "/nonexistent/tmux", socket: "/tmp/none"}
	m.brews, _ = parseBrewServices([]byte(brewInfo))
	m.projects = []project{{path: "/w/a", entries: []entry{
		{pid: 24422, kind: kindService, command: "postgresql@14", brew: "postgresql@14", declared: markDeclared("/w/a", "db"), cwd: "/w/a", status: statusActive, ports: []string{"5432"}},
		{pid: -7, kind: kindService, command: "herdr", brew: "herdr", declared: markDeclared("/w/a", "herd"), cwd: "/w/a", status: statusDown},
	}}}
	m.said, m.saidKeys, m.saidStation, m.saidUp, m.saidBar = true, m.keys(), m.station(), m.upWord(), m.bar()
	has := func(bar, key, does string) bool {
		return strings.Contains(bar, key+" #[nobold fg="+grayHex+"]"+strings.ToLower(does))
	}

	m.cursor = 24422
	if bar := m.bar(); !has(bar, "enter", "Its log") || !has(bar, "x", "End it") {
		t.Errorf("on a service up the bar says %q", bar)
	}
	if _, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})); cmd == nil {
		t.Error("enter on a service up opened nothing")
	}
	next, _ := m.Update(tea.KeyPressMsg(tea.Key{Text: "x"}))
	m = next.(model)
	if m.kill == nil || m.kill.brew != "postgresql@14" || m.kill.prompt != "brew services stop postgresql@14 · db?" {
		t.Fatalf("x armed %+v", m.kill)
	}
	if _, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "y"})); cmd == nil {
		t.Error("confirming the stop asked brew for nothing")
	}

	m.kill, m.cursor = nil, -7
	if bar := m.bar(); !has(bar, "enter", "Bring it up") || has(bar, "x", "End it") {
		t.Errorf("on a service down the bar says %q", bar)
	}
	if _, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})); cmd == nil {
		t.Error("enter on a service down asked brew for nothing")
	}
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Text: "x"}))
	if got := next.(model); got.kill != nil {
		t.Errorf("x armed a question on a service down: %+v", got.kill)
	}
}

// A brew service's page is what brew says of it, where it belongs and
// how many projects declare it, and what it has open; before brew has
// reported it, the page says so.
func TestTheBrewServicePageIsComposedFromBrew(t *testing.T) {
	services, _ := parseBrewServices([]byte(brewInfo))
	e := entry{pid: 24422, kind: kindService, command: "postgresql@14", brew: "postgresql@14", cwd: "/Users/w0zro/projects/w0zro/conn",
		status: statusActive, ports: []string{"5432"}, shared: 2,
		sockets: []socket{{"TCP", "127.0.0.1:5432", "LISTEN"}, {"unix", "/tmp/.s.PGSQL.5432", ""}}}
	text := texts(drawReadout(composeReadout(readoutSubject{entry: e, brew: brewServiceNamed(services, "postgresql@14"), inside: true}, "/Users/w0zro", processesNow), 100, 40, plain))
	for _, want := range []string{
		"READOUT", "postgresql@14",
		"Kind ...... Service · Homebrew",
		"Formula ... postgresql@14",
		"Status .... Started",
		"Process ... 24422",
		"Command ... /opt/homebrew/opt/postgresql@14/bin/postgres -D /opt/homebrew/var/postgresql@14",
		"Log ....... /opt/homebrew/var/log/postgresql@14.log",
		"Project ... ~/projects/w0zro/conn",
		"Shared .... 2 projects",
		"Ports ..... localhost:5432",
		"Pane ...... None · Enter opens its log",
		"Listens ... TCP 127.0.0.1:5432",
		"Unix ...... /tmp/.s.PGSQL.5432",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the page lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Wrong") || strings.Contains(text, "PID ") {
		t.Errorf("the page says what is not so:\n%s", text)
	}
	e.status, e.fault, e.pid, e.ports, e.sockets = "EXIT 78", true, -7, nil, nil
	text = texts(drawReadout(composeReadout(readoutSubject{entry: entry{pid: -7, kind: kindService, command: "redis", brew: "redis", cwd: "/w", status: "EXIT 78", fault: true}, brew: brewServiceNamed(services, "redis"), inside: true}, "/Users/w0zro", processesNow), 100, 40, plain))
	if !strings.Contains(text, "Wrong ..... Exit 78") || !strings.Contains(text, "Status .... Error") {
		t.Errorf("a service that ended badly does not say so:\n%s", text)
	}
	text = texts(drawReadout(composeReadout(readoutSubject{entry: entry{pid: -8, kind: kindService, command: "vault", brew: "vault", cwd: "/w", status: statusDown}, inside: true}, "/Users/w0zro", processesNow), 100, 40, plain))
	if !strings.Contains(text, "Status .... Not reported by brew") {
		t.Errorf("a service brew has not reported does not say so:\n%s", text)
	}
}

// What a row has open, and what brew said, travel with the reading to
// the page.
func TestSocketsAndBrewTravelWithTheReading(t *testing.T) {
	services, _ := parseBrewServices([]byte(brewInfo))
	r := reading{projects: []project{{path: "/w", entries: []entry{
		{pid: 24422, kind: kindService, command: "postgresql@14", brew: "postgresql@14", shared: 2, ports: []string{"5432"},
			sockets: []socket{{"TCP", "127.0.0.1:5432", "LISTEN"}}},
	}}}, brews: services, records: map[int]record{}, panes: map[string]pane{}}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var back reading
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	e := back.projects[0].entries[0]
	if e.brew != "postgresql@14" || e.shared != 2 || strings.Join(e.ports, "") != "5432" || len(e.sockets) != 1 || e.sockets[0].addr != "127.0.0.1:5432" {
		t.Errorf("the row came back as %+v", e)
	}
	if svc := back.brewOf(e); svc == nil || svc.pid != 24422 || svc.log == "" {
		t.Errorf("brew's word came back as %+v", svc)
	}
}

// An asking of brew sends nothing anywhere: brew's analytics, its update
// check and its hints are off in the environment conn gives it, and the
// beat between askings is slow enough that booting brew's ruby costs
// the machine little.
func TestBrewIsAskedQuietly(t *testing.T) {
	for _, want := range []string{"HOMEBREW_NO_ANALYTICS=1", "HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_ENV_HINTS=1"} {
		if !slices.Contains(brewEnv, want) {
			t.Errorf("brew is asked without %s", want)
		}
	}
	if brewBeat < 10*time.Second {
		t.Errorf("brew is asked every %s", brewBeat)
	}
}
