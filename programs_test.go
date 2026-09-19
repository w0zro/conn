package main

import (
	"strings"
	"testing"
)

// A row is a program conn knows by what it runs: postgres by its process
// name, by the brew formula whatever its version or tap, and by the
// docker image whatever its registry or tag. Anything else is nothing
// conn has a client for.
func TestARowIsAKnownProgramByCommandFormulaOrImage(t *testing.T) {
	for _, c := range []struct {
		what      string
		e         entry
		container *container
		want      string
	}{
		{"a bare server", entry{command: "postgres -D /opt/homebrew/var/postgresql@14"}, nil, "psql"},
		{"a postmaster", entry{command: "/usr/lib/postgresql/16/bin/postmaster -D /var/lib/postgresql"}, nil, "psql"},
		{"a brew service", entry{command: "postgresql@14", brew: "postgresql@14"}, nil, "psql"},
		{"a tapped formula", entry{command: "x", brew: "homebrew/core/postgresql"}, nil, "psql"},
		{"a container", entry{container: "abc"}, &container{id: "abc", image: "postgres:16"}, "psql"},
		{"a registry's image", entry{container: "abc"}, &container{id: "abc", image: "docker.io/library/postgres@sha256:0123"}, "psql"},
		{"a node server", entry{command: "node server.js"}, nil, ""},
		{"another brew service", entry{command: "redis", brew: "redis"}, nil, ""},
		{"another image", entry{container: "abc"}, &container{id: "abc", image: "redis:7"}, ""},
		{"a shell", entry{command: "zsh", kind: kindShell}, nil, ""},
		{"a shell a postgres folded into", entry{command: "zsh", kind: kindShell, listener: "postgres -D data"}, nil, "psql"},
		{"a wrapper a node folded into", entry{command: "npm start", listener: "node server.js"}, nil, ""},
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
	p := programOf(entry{command: "postgres"}, nil)
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
	m := newModel(plain)
	m.view, m.inside = viewProcesses, true
	m.srv = &server{tmux: "/nonexistent/tmux", socket: "/tmp/none"}
	m.containers = []container{{id: "abc", image: "postgres:16", state: "running", dir: "/w/a"}}
	m.projects = []project{{path: "/w/a", entries: []entry{
		{pid: 300, kind: kindRun, command: "postgres -D data", cwd: "/w/a", status: statusActive, ports: []string{"5432"}},
		{pid: 301, kind: kindRun, command: "postgres -D data", cwd: "/w/a", status: statusActive},
		{pid: -7, kind: kindService, command: "postgresql@14", brew: "postgresql@14", declared: markDeclared("/w/a", "db"), cwd: "/w/a", status: statusDown},
		{pid: -8, kind: kindService, command: "web", container: "abc", cwd: "/w/a", status: statusActive},
		{pid: 302, kind: kindRun, command: "node server.js", cwd: "/w/a", status: statusActive, ports: []string{"3000"}},
		{pid: 303, kind: kindShell, command: "zsh", cwd: "/w/a", status: statusActive, ports: []string{"5433"}, listener: "postgres -D data"},
	}}}
	has := func(bar string) bool { return strings.Contains(bar, "S #[nobold fg="+grayHex+"]psql") }
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
