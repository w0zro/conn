package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/w0zro/conn/internal/draw"
	"github.com/w0zro/conn/internal/work"

	"github.com/w0zro/conn/internal/tmux"

	tea "charm.land/bubbletea/v2"
)

// A declared brew service is a row of its project as brew reports it:
// down before brew has said anything of it, and active with the pid
// and the ports of the process running it once it has, labelled by its
// declared name and marked as the declaration. A pane opened on its
// log is its terminal. A project with no block of its own is given
// one for what it declares, and the ordinary declared rows do not take
// it for a pane to raise.
func TestABrewServiceIsARowAsBrewReportsIt(t *testing.T) {
	services, _ := work.ParseBrewServices([]byte(brewInfo))
	decl := map[string]work.Declared{
		"/w/a": {List: []work.Declaration{{Name: "db", Command: "brew services start postgresql@14"}, {Name: "web", Command: "npm run dev"}}},
		"/w/q": {List: []work.Declaration{{Name: "db", Command: "brew services start postgresql@14"}}},
	}
	projects := []work.Project{{Path: "/w/a", Entries: []work.Entry{{PID: 1, Kind: work.KindShell, Command: "zsh", TTY: "ttys001", Status: work.StatusIdle}}}}
	sockets := map[int][]work.Socket{24422: {{Proto: "TCP", Addr: "127.0.0.1:5432", State: "LISTEN"}, {Proto: "TCP", Addr: "[::1]:5432", State: "LISTEN"}, {Proto: "unix", Addr: "/tmp/.s.PGSQL.5432", State: ""}}}

	out := work.AttachDeclared(projects, decl, nil)
	if n := len(out[0].Entries); n != 2 {
		t.Fatalf("the declarations made %d rows; the brew one is not a pane's", n-1)
	}
	if e := out[0].Entries[1]; !strings.HasPrefix(e.Command, "web") || e.Status != work.StatusDown {
		t.Errorf("the ordinary declaration is %+v", e)
	}

	out = work.AttachBrew(out, decl, nil, nil, nil)
	if n := len(out); n != 2 {
		t.Fatalf("the project that only declares has no block: %d projects", n)
	}
	if n := len(out[0].Entries); n != 3 {
		t.Fatalf("the brew service is not a row: %d rows", n)
	}
	down := out[0].Entries[2]
	if down.Kind != work.KindService || down.Brew != "postgresql@14" || down.Command != "postgresql@14" || down.Status != work.StatusDown || down.PID >= 0 || declaredNameOf(down) != "db" {
		t.Errorf("before brew has spoken the row is %+v", down)
	}

	out = work.AttachBrew(work.AttachDeclared(projects, decl, nil), decl, services, sockets, map[string]string{work.BrewMark("postgresql@14"): "ttys009"})
	up := out[0].Entries[2]
	if up.Status != work.StatusActive || up.Fault || up.PID != 24422 || strings.Join(up.Ports, " ") != "5432" || len(up.Sockets) != 3 || up.TTY != "ttys009" || up.Cwd != "/w/a" {
		t.Errorf("as brew reports it the row is %+v", up)
	}
	// Up by brew's word, for u to leave alone; the other declaration
	// is down and would be raised.
	upBy, _ := work.UpAndHeld(out, nil, "/w/a")
	if !upBy[work.MarkDeclared("/w/a", "db")] || upBy[work.MarkDeclared("/w/a", "web")] {
		t.Errorf("u sees as up: %v", upBy)
	}
}

// Enter on a brew service up opens its log, and down asks brew to
// start it; x asks brew to stop one that is up, by its declared name,
// and leaves one down alone. The bar says as much.
func TestTheKeysAskBrewAboutItsService(t *testing.T) {
	m := plainModel()
	m.view, m.inside = viewProcesses, true
	m.srv = &tmux.Server{Tmux: "/nonexistent/tmux", Socket: "/tmp/none"}
	m.brews, _ = work.ParseBrewServices([]byte(brewInfo))
	m.projects = []work.Project{{Path: "/w/a", Entries: []work.Entry{
		{PID: 24422, Kind: work.KindService, Command: "postgresql@14", Brew: "postgresql@14", Declared: work.MarkDeclared("/w/a", "db"), Cwd: "/w/a", Status: work.StatusActive, Ports: []string{"5432"}},
		{PID: -7, Kind: work.KindService, Command: "herdr", Brew: "herdr", Declared: work.MarkDeclared("/w/a", "herd"), Cwd: "/w/a", Status: work.StatusDown},
	}}}
	said := m.telling()
	m.said = &said
	has := func(bar, key, does string) bool {
		return offered(bar, key, does)
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
	if m.kill == nil || m.kill.end == nil || m.kill.prompt != "brew services stop postgresql@14 · db?" {
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
	services, _ := work.ParseBrewServices([]byte(brewInfo))
	e := work.Entry{PID: 24422, Kind: work.KindService, Command: "postgresql@14", Brew: "postgresql@14", Cwd: "/Users/w0zro/projects/w0zro/conn",
		Status: work.StatusActive, Ports: []string{"5432"}, Shared: 2,
		Sockets: []work.Socket{{Proto: "TCP", Addr: "127.0.0.1:5432", State: "LISTEN"}, {Proto: "unix", Addr: "/tmp/.s.PGSQL.5432", State: ""}}}
	text := texts(drawReadout(composeReadout(readoutSubject{entry: e, brew: work.BrewServiceNamed(services, "postgresql@14"), inside: true}, "/Users/w0zro", processesNow), 100, 40, draw.Plain))
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
	e.Status, e.Fault, e.PID, e.Ports, e.Sockets = "EXIT 78", true, -7, nil, nil
	text = texts(drawReadout(composeReadout(readoutSubject{entry: work.Entry{PID: -7, Kind: work.KindService, Command: "redis", Brew: "redis", Cwd: "/w", Status: "EXIT 78", Fault: true}, brew: work.BrewServiceNamed(services, "redis"), inside: true}, "/Users/w0zro", processesNow), 100, 40, draw.Plain))
	if !strings.Contains(text, "Wrong ..... Exit 78") || !strings.Contains(text, "Status .... Error") {
		t.Errorf("a service that ended badly does not say so:\n%s", text)
	}
	text = texts(drawReadout(composeReadout(readoutSubject{entry: work.Entry{PID: -8, Kind: work.KindService, Command: "vault", Brew: "vault", Cwd: "/w", Status: work.StatusDown}, inside: true}, "/Users/w0zro", processesNow), 100, 40, draw.Plain))
	if !strings.Contains(text, "Status .... Not reported by brew") {
		t.Errorf("a service brew has not reported does not say so:\n%s", text)
	}
}

// What a row has open, and what brew said, travel with the reading to
// the page.
func TestSocketsAndBrewTravelWithTheReading(t *testing.T) {
	services, _ := work.ParseBrewServices([]byte(brewInfo))
	r := reading{projects: []work.Project{{Path: "/w", Entries: []work.Entry{
		{PID: 24422, Kind: work.KindService, Command: "postgresql@14", Brew: "postgresql@14", Shared: 2, Ports: []string{"5432"},
			Sockets: []work.Socket{{Proto: "TCP", Addr: "127.0.0.1:5432", State: "LISTEN"}}},
	}}}, brews: services, records: map[int]record{}, panes: map[string]tmux.Pane{}}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var back reading
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	e := back.projects[0].Entries[0]
	if e.Brew != "postgresql@14" || e.Shared != 2 || strings.Join(e.Ports, "") != "5432" || len(e.Sockets) != 1 || e.Sockets[0].Addr != "127.0.0.1:5432" {
		t.Errorf("the row came back as %+v", e)
	}
	if svc := back.brewOf(e); svc == nil || svc.PID != 24422 || svc.Log == "" {
		t.Errorf("brew's word came back as %+v", svc)
	}
}
