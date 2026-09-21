package main

import (
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
// machine and over http.
func TestThePortIsWhereTheBrowserGoes(t *testing.T) {
	if got := localURL("5173"); got != "http://localhost:5173" {
		t.Errorf("the browser is sent to %q", got)
	}
}
