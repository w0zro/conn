package main

import (
	"strings"
	"testing"
)

// The bar offers enter, x and u on a row exactly where the key does
// something there, on every kind of row, inside the server and off it.
// They were once decided apart, and came apart: x offered on a
// container already ended, enter offered off the server, and enter on
// a declaration started by hand bringing up a second one.
func TestTheBarOffersWhatTheKeysDo(t *testing.T) {
	web := markDeclared("/w", "web")
	pg := markDeclared("/w", "pg")
	rows := []entry{
		{pid: 1, kind: kindShell, command: "zsh", tty: "ttys001", status: statusIdle},
		{pid: 2, kind: kindContact, command: "claude", tty: "ttys002", status: statusWaiting},
		{pid: 3, kind: kindShell, command: "zsh", under: "go test ./...", tty: "ttys003", status: statusWorking},
		{pid: -9, kind: kindRun, command: "web", status: statusDown, declared: web},
		{pid: 5, kind: kindRun, command: "web", status: statusActive, declared: web},
		{pid: -11, kind: kindService, command: "db", status: statusDown},
		{pid: -99, kind: kindService, command: "old", status: statusEnded, fault: true, container: "abc123"},
		{pid: -98, kind: kindService, command: "api", status: statusActive, container: "def456"},
		{pid: 7, kind: kindService, command: "pg", status: statusActive, declared: pg, brew: "postgresql@14"},
		{pid: -7, kind: kindService, command: "pg", status: statusDown, declared: pg, brew: "postgresql@14"},
	}
	for _, inside := range []bool{true, false} {
		m := model{view: viewProcesses, inside: inside, focused: true, panes: map[string]pane{"ttys001": {id: "%1", tty: "ttys001"}}}
		m.projects = []project{{path: "/w", entries: rows}}
		m.declared = map[string]declared{"/w": {list: []declaration{{name: "web", command: "npm run dev"}, {name: "pg", command: "brew services run postgresql@14"}}}}
		for _, e := range rows {
			m.cursor = e.pid
			bar := m.bar()
			offers := func(key string) bool { return strings.Contains(bar, "]"+key+" #[nobold") }

			_, cmd := m.key("enter")
			if offers("enter") != (cmd != nil) {
				t.Errorf("inside %v, %s %s: the bar offers enter %v, enter does something %v", inside, e.command, e.status, offers("enter"), cmd != nil)
			}
			next, _ := m.key("x")
			armed := next.(model).kill != nil
			if offers("x") != armed {
				t.Errorf("inside %v, %s %s: the bar offers x %v, x arms %v", inside, e.command, e.status, offers("x"), armed)
			}
			_, cmd = m.key("u")
			if offers("u") != (cmd != nil) {
				t.Errorf("inside %v, %s %s: the bar offers u %v, u does something %v", inside, e.command, e.status, offers("u"), cmd != nil)
			}
		}
	}
}

// A declaration started by hand is up: enter has no pane of conn's to
// go into, and does not bring a second one up beside it.
func TestEnterOnADeclarationStartedByHandBringsNothingUp(t *testing.T) {
	m := model{view: viewProcesses, inside: true, panes: map[string]pane{}}
	m.projects = []project{{path: "/w", entries: []entry{
		{pid: 5, kind: kindRun, command: "web", status: statusActive, declared: markDeclared("/w", "web")},
	}}}
	m.declared = map[string]declared{"/w": {list: []declaration{{name: "web", command: "npm run dev"}}}}
	m.cursor = 5
	if _, cmd := m.key("enter"); cmd != nil {
		t.Error("enter on a declaration started by hand brought something up")
	}
}
