package main

import (
	"strings"
	"testing"

	"github.com/w0zro/conn/internal/room"
	"github.com/w0zro/conn/internal/work"
	"github.com/w0zro/conn/internal/work/declared"
)

// The bar offers enter, x and u on a row exactly where the key does
// something there, on every kind of row, inside the server and off it.
// They were once decided apart, and came apart: x offered on a
// container already ended, enter offered off the server, and enter on
// a declaration started by hand bringing up a second one.
func TestTheBarOffersWhatTheKeysDo(t *testing.T) {
	web := declared.Mark("/w", "web")
	pg := declared.Mark("/w", "pg")
	rows := []work.Entry{
		{PID: 1, Kind: work.KindShell, Command: "zsh", TTY: "ttys001", Status: work.StatusIdle},
		{PID: 2, Kind: work.KindContact, Command: "claude", TTY: "ttys002", Status: work.StatusWaiting},
		{PID: 3, Kind: work.KindShell, Command: "zsh", Under: "go test ./...", TTY: "ttys003", Status: work.StatusWorking},
		{PID: -9, Kind: work.KindRun, Command: "web", Status: work.StatusDown, Declared: web},
		{PID: 5, Kind: work.KindRun, Command: "web", Status: work.StatusActive, Declared: web},
		{PID: -11, Kind: work.KindService, Command: "db", Status: work.StatusDown},
		{PID: -99, Kind: work.KindService, Command: "old", Status: work.StatusEnded, Fault: true, Container: "abc123"},
		{PID: -98, Kind: work.KindService, Command: "api", Status: work.StatusActive, Container: "def456"},
		{PID: 7, Kind: work.KindService, Command: "pg", Status: work.StatusActive, Declared: pg, Brew: "postgresql@14"},
		{PID: -7, Kind: work.KindService, Command: "pg", Status: work.StatusDown, Declared: pg, Brew: "postgresql@14"},
	}
	for _, inside := range []bool{true, false} {
		m := model{reading: reading{panes: map[string]room.Pane{"ttys001": {ID: "%1", TTY: "ttys001"}}}, view: viewProcesses, inside: inside, focused: true}
		m.projects = []work.Project{{Path: "/w", Entries: rows}}
		m.declared = map[string]declared.File{"/w": {List: []declared.Declaration{{Name: "web", Command: "npm run dev"}, {Name: "pg", Command: "brew services run postgresql@14"}}}}
		for _, e := range rows {
			m.cursor = e.PID
			bar := m.bar()
			offers := func(key string) bool { return strings.Contains(bar, "]"+key+" #[") }

			_, cmd := m.key("enter")
			if offers("enter") != (cmd != nil) {
				t.Errorf("inside %v, %s %s: the bar offers enter %v, enter does something %v", inside, e.Command, e.Status, offers("enter"), cmd != nil)
			}
			next, _ := m.key("x")
			armed := next.kill != nil
			if offers("x") != armed {
				t.Errorf("inside %v, %s %s: the bar offers x %v, x arms %v", inside, e.Command, e.Status, offers("x"), armed)
			}
			_, cmd = m.key("u")
			if offers("u") != (cmd != nil) {
				t.Errorf("inside %v, %s %s: the bar offers u %v, u does something %v", inside, e.Command, e.Status, offers("u"), cmd != nil)
			}
		}
	}
}

// A declaration started by hand is up: enter has no pane of conn's to
// go into, and does not bring a second one up beside it.
func TestEnterOnADeclarationStartedByHandBringsNothingUp(t *testing.T) {
	m := model{reading: reading{panes: map[string]room.Pane{}}, view: viewProcesses, inside: true}
	m.projects = []work.Project{{Path: "/w", Entries: []work.Entry{
		{PID: 5, Kind: work.KindRun, Command: "web", Status: work.StatusActive, Declared: declared.Mark("/w", "web")},
	}}}
	m.declared = map[string]declared.File{"/w": {List: []declared.Declaration{{Name: "web", Command: "npm run dev"}}}}
	m.cursor = 5
	if _, cmd := m.key("enter"); cmd != nil {
		t.Error("enter on a declaration started by hand brought something up")
	}
}
