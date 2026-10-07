package main

import (
	"strings"
	"testing"
	"time"

	"github.com/w0zro/conn/internal/work"

	"github.com/w0zro/conn/internal/tmux"

	tea "charm.land/bubbletea/v2"
)

// Where a compose is running in a shell, its containers stand under it:
// the row that says docker compose up had nothing beneath it, and the
// services it is running are what it is doing.
func TestContainersStandUnderTheComposeThatRunsThem(t *testing.T) {
	projects := []work.Project{{
		Path: "/Users/w0zro/projects/compose-demo",
		Entries: []work.Entry{
			{PID: 100, Kind: work.KindShell, Command: "zsh", TTY: "ttys003", Cwd: "/Users/w0zro/projects/compose-demo"},
			{PID: 101, Kind: work.KindRun, Command: "docker compose up", TTY: "ttys003", Depth: 1,
				Cwd: "/Users/w0zro/projects/compose-demo"},
		},
	}}
	out := work.AttachContainers(projects, containersFor(t), dockerRoots, nil, nil)
	if len(out) != 1 {
		t.Fatalf("%d projects, want 1: the stray belongs to none and is not filed", len(out))
	}
	rows := out[0].Entries
	var got []string
	for _, e := range rows {
		got = append(got, strings.Repeat("  ", e.Depth)+e.Kind+" "+e.Command+PortsWord(e.Ports))
	}
	// A service carries the ports it publishes on the host, which is
	// where you would go to reach it; a worker with none is named alone.
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
		if strings.HasPrefix(e.Command, "worker") && (e.Status != "EXIT 3" || !e.Fault) {
			t.Errorf("the worker reads %q fault=%v", e.Status, e.Fault)
		}
	}
}

// Left detached there is no compose to stand under, and the project may
// have no processes at all — which is the case the process table cannot
// show, and the one that had conn saying nothing was running.
func TestDetachedContainersRootTheirOwnProject(t *testing.T) {
	// Work in another project, so the reading is not empty; nothing at
	// all in the compose demo.
	projects := []work.Project{{
		Path: "/Users/w0zro/projects/w0zro/conn",
		Entries: []work.Entry{{PID: 1, Kind: work.KindShell, Command: "zsh", TTY: "ttys001",
			Cwd: "/Users/w0zro/projects/w0zro/conn"}},
	}}
	out := work.AttachContainers(projects, containersFor(t), dockerRoots, nil, nil)
	if len(out) != 2 {
		t.Fatalf("%d projects, want 2: the compose demo is a project docker alone is working in", len(out))
	}
	if out[0].Path != "/Users/w0zro/projects/w0zro/conn" {
		t.Errorf("the worked project lost its place to %q", out[0].Path)
	}
	demo := out[1]
	if demo.Path != "/Users/w0zro/projects/compose-demo" {
		t.Fatalf("the second project is %q", demo.Path)
	}
	var got []string
	for _, e := range demo.Entries {
		if e.Depth != 0 {
			t.Errorf("%s stands at depth %d with no compose above it", e.Command, e.Depth)
		}
		got = append(got, e.Command+PortsWord(e.Ports))
	}
	want := "cache · :6390, web · :8438, worker"
	if strings.Join(got, ", ") != want {
		t.Errorf("the rows read %q, want %q", strings.Join(got, ", "), want)
	}
	// A container conn holds no pane for is a row it can only report,
	// which is what the terminal being blank says.
	for _, e := range demo.Entries {
		if e.TTY != "" {
			t.Errorf("%s claims terminal %q", e.Command, e.TTY)
		}
	}
}

// enter on a container opens its output; enter again, once there is a
// pane, goes into it the way enter goes into anything. s opens a shell
// inside the container rather than at the directory it was started for.
func TestEnterAndSActOnTheContainer(t *testing.T) {
	m := plainModel()
	m.view, m.inside = viewProcesses, true
	m.srv = &tmux.Server{Tmux: "/nonexistent/tmux", Socket: "/tmp/none"}
	said := m.telling()
	m.said = &said
	m.projects = []work.Project{{Path: "/p", Entries: []work.Entry{
		{PID: -99, Kind: work.KindService, Command: "web", Ports: []string{"8438"}, Container: "abc123", Cwd: "/p", Status: work.StatusActive},
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
	m.projects[0].Entries[0].Container = ""
	m.said.bar = m.bar() // the bar stops offering enter, which is a change of its own
	if _, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})); cmd != nil {
		t.Error("enter opened something for a row that is neither reachable nor a container")
	}
}

// The page for a container is composed from what docker said, not from
// the process table, which has no record of it. It is named by its id,
// since the number conn files it under is conn's own bookkeeping.
func TestTheServicePageIsComposedFromDocker(t *testing.T) {
	now := time.Date(2026, 9, 15, 22, 0, 0, 0, time.UTC)
	c := work.Container{
		ID: "94e3da190ba7", Name: "compose-demo-web-1", Service: "web", Project: "compose-demo",
		Image: "nginx:alpine", State: "running", Status: "Up 3 minutes (healthy)", Health: "healthy",
		Dir: "/Users/w0zro/projects/compose-demo", Ports: []string{"8438"}, Since: now.Add(-3 * time.Minute),
	}
	b := composeReadout(readoutSubject{container: &c, inside: true}, "/Users/w0zro", now)
	if b.name != c.ID {
		t.Errorf("the page is headed %q, not the container's id", b.name)
	}
	text := texts(drawReadout(b, 60, 24, Plain))
	for _, want := range []string{
		"94e3da190ba7", // the header, in place of a pid conn invented
		"Image ..... nginx:alpine",
		"Up 3 minutes (healthy)", // docker's own sentence, which says it best
		"Health .... Healthy",
		"Compose ... compose-demo",
		"Name ...... compose-demo-web-1",
		"Ports ..... localhost:8438",
		"Enter opens its log",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the page lacks %q:\n%s", want, text)
		}
	}
	// Nothing from the process page, which asks the table things it
	// cannot answer for a container.
	for _, unwanted := range []string{"PID ", "State ", "CPU ", "Command "} {
		if strings.Contains(text, unwanted) {
			t.Errorf("the page still says %q:\n%s", unwanted, text)
		}
	}

	// A service that went wrong says so past docker's sentence, since
	// that buries it: Exited (3) reads as a fact, not as a fault.
	c.State, c.Exit, c.Health, c.Status = "exited", "3", "", "Exited (3) 8 seconds ago"
	text = texts(drawReadout(composeReadout(readoutSubject{container: &c, inside: true}, "/Users/w0zro", now), 60, 24, Plain))
	if !strings.Contains(text, "Wrong ..... Exit 3") {
		t.Errorf("a dead service does not say what went wrong:\n%s", text)
	}
}

// x on a container asks docker to stop it, and says stop rather than
// kill: there is no process here to signal. One already stopped is left
// alone, the question being about nothing.
func TestXStopsAContainer(t *testing.T) {
	m := plainModel()
	m.view, m.inside = viewProcesses, true
	m.srv = &tmux.Server{Tmux: "/nonexistent/tmux", Socket: "/tmp/none"}
	m.projects = []work.Project{{Path: "/p", Entries: []work.Entry{
		{PID: -99, Kind: work.KindService, Command: "web", Ports: []string{"8438"}, Container: "abc123", Cwd: "/p", Status: work.StatusActive},
		{PID: -98, Kind: work.KindService, Command: "worker", Container: "def456", Cwd: "/p", Status: work.StatusEnded},
	}}}
	m.containers = []work.Container{{ID: "abc123", Service: "web"}, {ID: "def456", Service: "worker"}}

	m.cursor = -99
	next, _ := m.Update(tea.KeyPressMsg(tea.Key{Text: "x"}))
	m = next.(model)
	if m.kill == nil || m.kill.end == nil {
		t.Fatalf("x armed %+v", m.kill)
	}
	if !strings.Contains(m.kill.prompt, "docker stop abc123 · web?") {
		t.Errorf("the question reads %q", m.kill.prompt)
	}
	if strings.Contains(m.kill.prompt, "kill") || strings.Contains(m.kill.prompt, "-99") {
		t.Errorf("the question talks of killing or of a pid: %q", m.kill.prompt)
	}
	// By the service, not by the row as drawn, which carries the ports.
	if strings.Contains(m.kill.prompt, ":8438") {
		t.Errorf("the question asks about an address: %q", m.kill.prompt)
	}
	// Confirming asks docker rather than signalling anything.
	if _, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "y"})); cmd == nil {
		t.Error("confirming the stop asked for nothing")
	}

	// A service already gone is not asked about.
	m.kill, m.cursor = nil, -98
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Text: "x"}))
	if got := next.(model); got.kill != nil {
		t.Errorf("x armed a question on a service already stopped: %+v", got.kill)
	}
}
