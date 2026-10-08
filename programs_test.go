package main

import (
	"testing"

	"github.com/w0zro/conn/internal/room"
	"github.com/w0zro/conn/internal/work"
	"github.com/w0zro/conn/internal/work/declared"
	"github.com/w0zro/conn/internal/work/docker"

	"github.com/w0zro/conn/internal/tmux"
)

// A row is a program conn knows by what it runs: postgres by its process
// name, by the brew formula whatever its version or tap, and by the
// docker image whatever its registry or tag. Anything else is nothing
// conn has a client for.
func TestARowIsAKnownProgramByCommandFormulaOrImage(t *testing.T) {
	for _, c := range []struct {
		what      string
		e         work.Entry
		container *docker.Container
		want      string
	}{
		{"a bare server", work.Entry{Command: "postgres -D /opt/homebrew/var/postgresql@14"}, nil, "psql"},
		{"a postmaster", work.Entry{Command: "/usr/lib/postgresql/16/bin/postmaster -D /var/lib/postgresql"}, nil, "psql"},
		{"a brew service", work.Entry{Command: "postgresql@14", Brew: "postgresql@14"}, nil, "psql"},
		{"a tapped formula", work.Entry{Command: "x", Brew: "homebrew/core/postgresql"}, nil, "psql"},
		{"a container", work.Entry{Container: "abc"}, &docker.Container{ID: "abc", Image: "postgres:16"}, "psql"},
		{"a registry's image", work.Entry{Container: "abc"}, &docker.Container{ID: "abc", Image: "docker.io/library/postgres@sha256:0123"}, "psql"},
		{"a node server", work.Entry{Command: "node server.js"}, nil, ""},
		{"another brew service", work.Entry{Command: "redis", Brew: "redis"}, nil, ""},
		{"another image", work.Entry{Container: "abc"}, &docker.Container{ID: "abc", Image: "redis:7"}, ""},
		{"a shell", work.Entry{Command: "zsh", Kind: work.KindShell}, nil, ""},
		{"a shell a postgres folded into", work.Entry{Command: "zsh", Kind: work.KindShell, Listener: "postgres -D data"}, nil, "psql"},
		{"a wrapper a node folded into", work.Entry{Command: "npm start", Listener: "node server.js"}, nil, ""},
	} {
		got := ""
		if p := programOf(c.e, c.container); p != nil {
			got = p.client
		}
		if got != c.want {
			t.Errorf("%s is known by %q, not %q", c.what, got, c.want)
		}
	}
}

// The client's line reaches the server where it listens: on this
// machine by the row's port, in a container as the user the image was
// given, or postgres unless it says.
func TestTheClientReachesTheServer(t *testing.T) {
	p := programOf(work.Entry{Command: "postgres"}, nil)
	if p == nil {
		t.Fatal("postgres is not known")
	}
	if got := p.args("5433"); got != "-h localhost -p 5433 postgres" {
		t.Errorf("the client is given %q", got)
	}
	if got := p.inContainer("app"); got != "psql -U app" {
		t.Errorf("in a container the client is %q", got)
	}
	if p.userEnv != "POSTGRES_USER" || p.user != "postgres" {
		t.Errorf("the container's user is read from %s, %s unless set", p.userEnv, p.user)
	}
}

// S is offered where a session can be opened: a server with a port, or
// a container. A brew service that is down listens nowhere, and a row
// that is no known program has no client to offer.
func TestSIsOfferedWhereAClientCanConnect(t *testing.T) {
	m := plainModel()
	m.view, m.inside = viewProcesses, true
	m.srv = &room.Server{Server: &tmux.Server{Tmux: "/nonexistent/tmux", Socket: "/tmp/none"}}
	m.containers = []docker.Container{{ID: "abc", Image: "postgres:16", State: "running", Dir: "/w/a"}}
	m.projects = []work.Project{{Path: "/w/a", Entries: []work.Entry{
		{PID: 300, Kind: work.KindRun, Command: "postgres -D data", Cwd: "/w/a", Status: work.StatusActive, Ports: []string{"5432"}},
		{PID: 301, Kind: work.KindRun, Command: "postgres -D data", Cwd: "/w/a", Status: work.StatusActive},
		{PID: -7, Kind: work.KindService, Command: "postgresql@14", Brew: "postgresql@14", Declared: declared.Mark("/w/a", "db"), Cwd: "/w/a", Status: work.StatusDown},
		{PID: -8, Kind: work.KindService, Command: "web", Container: "abc", Cwd: "/w/a", Status: work.StatusActive},
		{PID: 302, Kind: work.KindRun, Command: "node server.js", Cwd: "/w/a", Status: work.StatusActive, Ports: []string{"3000"}},
		{PID: 303, Kind: work.KindShell, Command: "zsh", Cwd: "/w/a", Status: work.StatusActive, Ports: []string{"5433"}, Listener: "postgres -D data"},
	}}}
	has := func(bar string) bool { return offered(bar, "S", "psql") }
	for _, c := range []struct {
		pid  int
		want bool
	}{{300, true}, {301, false}, {-7, false}, {-8, true}, {302, false}, {303, true}} {
		m.cursor = c.pid
		if got := has(m.bar()); got != c.want {
			t.Errorf("on %d the bar offers S: %v, want %v\n%s", c.pid, got, c.want, m.bar())
		}
		// The key itself, under the wrapper that tells the status line
		// of the bar changing from row to row.
		_, cmd := m.key("S")
		if (cmd != nil) != c.want {
			t.Errorf("on %d S asked for something: %v, want %v", c.pid, cmd != nil, c.want)
		}
		// With no tmux to open the session in, or no psql on the
		// path, the answer is a notice rather than a pane.
		if c.pid == 300 {
			if _, ok := answered(cmd).(noticeMsg); !ok {
				t.Errorf("on %d S answered %T, not a notice", c.pid, answered(cmd))
			}
		}
	}
	// Outside the server there is nowhere to open a session.
	m.inside, m.cursor = false, 300
	if _, cmd := m.key("S"); cmd != nil || has(m.bar()) {
		t.Error("outside the server S is offered, or does something")
	}
}
