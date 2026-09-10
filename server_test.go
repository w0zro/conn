package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The server, against tmux itself. The test builds conn, brings a tmux
// server up on a scratch socket with conn in its home window, the way
// conn does for itself but detached, since a test has no terminal to
// attach, and drives the rail with send-keys, reading back what tmux
// says of its panes. It skips where tmux is not installed, and under
// -short. Every feature of the server layer that was proven by hand at
// a pty is proven here instead.

// A scratch server for one test.
type scratch struct {
	t   *testing.T
	srv *server
	dir string
}

func startScratch(t *testing.T) *scratch {
	t.Helper()
	if testing.Short() {
		t.Skip("a real tmux server is not started under -short")
	}
	tmux := lookPath("tmux")
	if tmux == "" {
		t.Skip("tmux is not installed")
	}
	// A unix socket's path is short by law; the test's own temp dir is
	// too deep for one on macOS.
	dir, err := os.MkdirTemp("/tmp", "conn-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	bin := filepath.Join(dir, "conn")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("building conn: %v\n%s", err, out)
	}
	s := &scratch{t: t, srv: &server{tmux: tmux, socket: filepath.Join(dir, "sock")}, dir: dir}
	t.Cleanup(func() { _, _ = s.srv.run("kill-server") })
	conf := filepath.Join(dir, "tmux.conf")
	if err := os.WriteFile(conf, []byte(tmuxConf("C-Space")), 0o600); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(filepath.Join(home, "repo", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(tmux, "-S", s.srv.socket, "-f", conf, "new-session", "-d", "-x", "160", "-y", "40",
		"-s", sessionName, "-n", homeWindow, "-c", home, "exec "+shellQuote(bin))
	cmd.Env = append(withoutTmux(os.Environ()),
		"CONN_SOCKET="+s.srv.socket, "XDG_STATE_HOME="+filepath.Join(dir, "state"),
		"HOME="+home, "TERM=xterm-256color", "COLORTERM=truecolor", "SHELL=/bin/sh")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("starting the server: %v\n%s", err, out)
	}
	return s
}

// keys sends keys to the rail.
func (s *scratch) keys(keys ...string) {
	s.t.Helper()
	if _, err := s.srv.run(append([]string{"send-keys", "-t", sessionName + ":" + homeWindow + ".0"}, keys...)...); err != nil {
		s.t.Fatal(err)
	}
}

// rail is what the rail shows.
func (s *scratch) rail() string {
	out, _ := s.srv.run("capture-pane", "-p", "-t", sessionName+":"+homeWindow+".0")
	return out
}

// panes is every pane as window.index:command:id.
func (s *scratch) panes() string {
	out, _ := s.srv.run("list-panes", "-a", "-F", "#{window_name}.#{pane_index}:#{pane_current_command}:#{pane_id}")
	return strings.Join(strings.Fields(out), " ")
}

// paneAt is window.index:command:id for one pane, or nothing.
func (s *scratch) paneAt(at string) string {
	for _, p := range strings.Fields(s.panes()) {
		if strings.HasPrefix(p, at+":") {
			return p
		}
	}
	return ""
}

// shellIn says whether the pane at a place runs a shell. /bin/sh is a
// bash on macOS and calls itself one.
func (s *scratch) shellIn(at string) bool {
	f := strings.Split(s.paneAt(at), ":")
	return len(f) == 3 && (f[1] == "sh" || f[1] == "bash" || f[1] == "zsh" || f[1] == "dash")
}

// parked says whether a pane is in a window of its own, out of home.
func (s *scratch) parked(id string) bool {
	for _, p := range strings.Fields(s.panes()) {
		if strings.HasSuffix(p, ":"+id) && !strings.HasPrefix(p, homeWindow+".") {
			return true
		}
	}
	return false
}

// display is a tmux format, of the rail.
func (s *scratch) display(format string) string {
	out, _ := s.srv.run("display-message", "-p", "-t", sessionName+":"+homeWindow+".0", format)
	return strings.TrimSpace(out)
}

// until waits for a condition, and fails with what was seen when it
// does not come.
func (s *scratch) until(what string, cond func() bool) {
	s.t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	s.t.Fatalf("waited for %s\nrail:\n%s\npanes: %s", what, s.rail(), s.panes())
}

// conn comes up in the home window: the console across it, then on a
// key the rail at its width with the hold beside it; s opens a shell
// into the slot and its row is on the rail; a second s opens another
// and the first parks in a window of its own; enter on the first brings
// it back; c zooms the console over the window and a key gives the slot
// its side again; the rail holds its width when the window is resized.
func TestTheServerHoldsTheRailAndTheSlot(t *testing.T) {
	s := startScratch(t)
	s.until("the console to finish", func() bool { return strings.Contains(s.rail(), prompt) })
	if !strings.Contains(s.rail(), "START-UP CHECKS") || s.display("#{pane_width}") != "160" {
		t.Errorf("the console should have the whole window:\n%s", s.rail())
	}

	s.keys("Space")
	s.until("the slot to open", func() bool {
		return s.display("#{pane_width}") == "48" && strings.Contains(s.panes(), "home.1:conn:")
	})
	if hold := s.display("#{window_panes}"); hold != "2" {
		t.Errorf("home has %s panes", hold)
	}

	s.keys("s")
	s.until("a shell in the slot", func() bool {
		return s.shellIn("home.1") && strings.Contains(s.rail(), "SHELL  ")
	})
	if strings.Contains(s.panes(), "conn:") && strings.Count(s.panes(), "conn:") > 1 {
		t.Errorf("the hold should be gone once a shell is in the slot: %s", s.panes())
	}
	first := s.display("#{pane_id}")
	slotFirst, _ := s.srv.run("display-message", "-p", "-t", sessionName+":"+homeWindow+".1", "#{pane_id}")
	slotFirst = strings.TrimSpace(slotFirst)
	if first == slotFirst {
		t.Fatalf("the rail and the slot are one pane: %s", s.panes())
	}

	s.keys("s")
	s.until("a second shell, with the first parked", func() bool {
		return s.shellIn("home.1") && s.parked(slotFirst)
	})

	// The cursor is on the newest shell, which is in the slot; j is the
	// first, and enter brings it back.
	s.keys("j")
	s.keys("Enter")
	s.until("the first shell back in the slot", func() bool {
		return strings.HasSuffix(s.paneAt("home.1"), ":"+slotFirst)
	})

	s.keys("c")
	s.until("the console zoomed over the window", func() bool {
		return s.display("#{window_zoomed_flag}") == "1" && strings.Contains(s.rail(), "START-UP CHECKS")
	})
	s.keys("Space")
	s.until("the slot to have its side again", func() bool {
		return s.display("#{window_zoomed_flag}") == "0" && s.display("#{pane_width}") == "48"
	})

	if _, err := s.srv.run("resize-window", "-x", "200", "-y", "40"); err != nil {
		t.Fatal(err)
	}
	s.until("the rail to hold its width", func() bool { return s.display("#{pane_width}") == "48" })
}

// conn down ends what the test brought up, and says so; a second
// conn down finds nothing.
func TestDownEndsTheScratchServer(t *testing.T) {
	s := startScratch(t)
	s.until("the console to finish", func() bool { return strings.Contains(s.rail(), prompt) })
	msg, ok := takeDown(s.srv, filepath.Join(s.dir, "home"))
	if !ok || !strings.Contains(msg, "Window home") || !strings.Contains(msg, "Server ") || !strings.Contains(msg, "ended") {
		t.Errorf("down: %v %q", ok, msg)
	}
	if msg, ok := takeDown(s.srv, s.dir); !ok || !strings.Contains(msg, "no server up") {
		t.Errorf("a second down: %v %q", ok, msg)
	}
}
