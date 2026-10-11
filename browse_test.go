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

// cc offers the port where a row serves: it is alive and has a port. A
// contact is never a thing to go to, whatever it has open, and a row
// that is not running serves nothing. A running row with no port is
// still asked about; a port that does not speak the web is too.
func TestCcOpensThePortWhereARowServes(t *testing.T) {
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
	}{{300, true}, {301, true}, {302, false}, {303, false}} {
		m.cursor = c.pid
		if got := offered(m.bar(), "cc", "Open :5173"); got != (c.pid == 300) {
			t.Errorf("on %d the bar offers cc: %v, want %v\n%s", c.pid, got, c.want, m.bar())
		}
		_, cmd := press(m, "c", "c")
		if (cmd != nil) != c.want {
			t.Errorf("on %d o asked for something: %v, want %v", c.pid, cmd != nil, c.want)
		}
		if c.want {
			notice, ok := answered(cmd).(noticeMsg)
			if !ok || !strings.Contains(notice.text, "not on the path") {
				t.Errorf("on %d o answered %#v", c.pid, answered(cmd))
			}
		}
	}
}

// The port a row says is the port the browser is sent to, on this
// machine, under the scheme that port answers to — proven against a
// server of each kind rather than against conn's idea of them. A port
// nothing is on is no page.
func TestThePortIsWhereTheBrowserGoes(t *testing.T) {
	nothing := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	plainly := httptest.NewServer(nothing)
	defer plainly.Close()
	secured := httptest.NewTLSServer(nothing)
	defer secured.Close()
	for _, c := range []struct {
		what, at, want string
		web            bool
	}{
		{"an http server", plainly.URL, "http", true},
		{"an https server", secured.URL, "https", true},
		{"a port nothing is on", "http://localhost:" + freePort(t), "", false},
	} {
		if got, web := speaksWeb(portOf(t, c.at)); got != c.want || web != c.web {
			t.Errorf("with %s on the port: %q %v, want %q %v", c.what, got, web, c.want, c.web)
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
