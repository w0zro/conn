package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/w0zro/conn/internal/room"
	"github.com/w0zro/conn/internal/work"
	"github.com/w0zro/conn/internal/work/declared"

	"github.com/w0zro/conn/internal/tmux"
)

// o is offered where a row serves: it is alive and has a port. A
// contact is never a thing to go to, whatever it has open, and a row
// that is not running serves nothing.
func TestOIsOfferedWhereARowServes(t *testing.T) {
	m := plainModel()
	m.view, m.inside = viewProcesses, true
	m.srv = room.New(&tmux.Server{Tmux: "/nonexistent/tmux", Socket: "/tmp/none"})
	m.projects = []work.Project{{Path: "/w/a", Entries: []work.Entry{
		{PID: 300, Kind: work.KindRun, Command: "node vite", Cwd: "/w/a", Status: work.StatusActive, Ports: []string{"5173", "24678"}},
		{PID: 301, Kind: work.KindRun, Command: "node build.js", Cwd: "/w/a", Status: work.StatusActive},
		{PID: 302, Kind: work.KindContact, Command: "claude", Cwd: "/w/a", Status: work.StatusWaiting, Ports: []string{"7000"}},
		{PID: 303, Kind: work.KindRun, Command: "npm run dev", Cwd: "/w/a", Status: work.StatusDown, Declared: declared.Mark("/w/a", "dev"), Ports: []string{"5174"}},
	}}}
	// With nothing on the path there is no browser to open, so the key
	// answers a notice and no browser is started by the test.
	t.Setenv("PATH", "")
	for _, c := range []struct {
		pid  int
		want bool
	}{{300, true}, {301, false}, {302, false}, {303, false}} {
		m.cursor = c.pid
		if got := offered(m.bar(), "o", "Open :5173"); got != c.want {
			t.Errorf("on %d the bar offers o: %v, want %v\n%s", c.pid, got, c.want, m.bar())
		}
		_, cmd := m.key("o")
		if (cmd != nil) != c.want {
			t.Errorf("on %d o asked for something: %v, want %v", c.pid, cmd != nil, c.want)
		}
		if c.want {
			notice, ok := answered(cmd).(noticeMsg)
			if !ok || !strings.Contains(notice.text, "was not found on the path") {
				t.Errorf("on %d o answered %#v", c.pid, answered(cmd))
			}
		}
	}
}

// The port a row says is the port the browser is sent to, on this
// machine, under the scheme that port answers to — proven against a
// server of each kind rather than against conn's idea of them.
func TestThePortIsWhereTheBrowserGoes(t *testing.T) {
	nothing := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	plainly := httptest.NewServer(nothing)
	defer plainly.Close()
	secured := httptest.NewTLSServer(nothing)
	defer secured.Close()
	for _, c := range []struct{ what, at, want string }{
		{"an http server", plainly.URL, "http"},
		{"an https server", secured.URL, "https"},
		{"a port nothing is on", "http://localhost:" + freePort(t), "http"},
	} {
		port := portOf(t, c.at)
		if got, want := localURL(port), c.want+"://localhost:"+port; got != want {
			t.Errorf("with %s on the port the browser is sent to %q, want %q", c.what, got, want)
		}
	}
}

// portOf is the port a test server came up on.
func portOf(t *testing.T, at string) string {
	t.Helper()
	u, err := url.Parse(at)
	if err != nil {
		t.Fatalf("parsing %q: %v", at, err)
	}
	return u.Port()
}

// freePort is a port of this machine nothing is listening on: one taken
// and given back, which is as close to a free port as the system will
// say.
func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("taking a port: %v", err)
	}
	port := portOf(t, "http://"+l.Addr().String())
	if err := l.Close(); err != nil {
		t.Fatalf("giving the port back: %v", err)
	}
	return port
}
