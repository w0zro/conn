package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// o is offered where a row serves: it is alive and has a port. A
// contact is never a thing to go to, whatever it has open, and a row
// that is not running serves nothing.
func TestOIsOfferedWhereARowServes(t *testing.T) {
	m := newModel(plain)
	m.view, m.inside = viewProcesses, true
	m.srv = &server{tmux: "/nonexistent/tmux", socket: "/tmp/none"}
	m.projects = []project{{path: "/w/a", entries: []entry{
		{pid: 300, kind: kindRun, command: "node vite", cwd: "/w/a", status: statusActive, ports: []string{"5173", "24678"}},
		{pid: 301, kind: kindRun, command: "node build.js", cwd: "/w/a", status: statusActive},
		{pid: 302, kind: kindContact, command: "claude", cwd: "/w/a", status: statusWaiting, ports: []string{"7000"}},
		{pid: 303, kind: kindRun, command: "npm run dev", cwd: "/w/a", status: statusDown, declared: markDeclared("/w/a", "dev"), ports: []string{"5174"}},
	}}}
	// With nothing on the path there is no browser to open, so the key
	// answers a notice and no browser is started by the test.
	t.Setenv("PATH", "")
	for _, c := range []struct {
		pid  int
		want bool
	}{{300, true}, {301, false}, {302, false}, {303, false}} {
		m.cursor = c.pid
		if got := strings.Contains(m.bar(), "o #[nobold fg="+grayHex+"]open :5173"); got != c.want {
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
