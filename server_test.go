package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// railW is the rail's width, as tmux reports a pane's: the tests ask
// tmux rather than assume it, so a change to railWidth does not also
// mean hunting down what was typed against it.
var railW = strconv.Itoa(railWidth)

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
		"CONN_ROOTS="+home,
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

// scratchPlace is what the rail calls the scratch root's repository:
// the one place on the watch that is this test's own, since the watch
// reads the whole machine's table and everything else it finds there is
// somewhere else entirely. The watch names a place by what is left of
// its path once the root is taken off, and the root here is the scratch
// home, so the repository under it is called by its own name.
const scratchPlace = "repo"

// lookPid is the pid the look in the slot says it is about, or "" when
// the slot is not a look or has not read yet.
var lookPidRe = regexp.MustCompile(`PID (\d+)`)

func (s *scratch) lookPidOf() string {
	if m := lookPidRe.FindStringSubmatch(s.slot()); m != nil {
		return m[1]
	}
	return ""
}

// slot is what the pane on the right shows.
func (s *scratch) slot() string {
	out, _ := s.srv.run("capture-pane", "-p", "-t", sessionName+":"+homeWindow+".1")
	return out
}

// placeRows is how many processes the rail says stand at that place,
// read off the place's own title line.
func (s *scratch) placeRows() int {
	// The rows under the scratch place's title, up to the blank line
	// that begins the next place: a place's title carries no count.
	lines := strings.Split(s.rail(), "\n")
	for i, line := range lines {
		if !strings.Contains(line, scratchPlace) {
			continue
		}
		n := 0
		for _, r := range lines[i+1:] {
			if strings.TrimSpace(r) == "" {
				break
			}
			n++
		}
		return n
	}
	return 0
}

// openShell opens a shell at the scratch root's own repository, from
// the list rather than with s at the cursor: the watch reads the whole
// machine's process table, so what the cursor lands on is whatever
// else is running here, while the list is only ever the roots this
// test laid down. The cursor follows the shell it opens, so a test
// that goes on to act on it acts on that one.
func (s *scratch) openShell() {
	s.t.Helper()
	s.keys("p")
	s.until("the list to find the scratch repository", func() bool {
		return strings.Contains(s.rail(), "PROJECTS") && strings.Contains(s.rail(), "repo")
	})
	s.keys("Enter")
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

// paneDead says whether the pane at a place is held dead on
// remain-on-exit, its process already gone.
func (s *scratch) paneDead(at string) bool {
	out, _ := s.srv.run("display-message", "-p", "-t", sessionName+":"+at, "#{pane_dead}")
	return strings.TrimSpace(out) == "1"
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

// bar is the status line as tmux expands it: the lamps, and the row conn
// has put there for whatever the slot holds.
func (s *scratch) bar() string {
	out, _ := s.srv.run("display-message", "-p", "-t", sessionName+":"+homeWindow+".0",
		"#{T:status-left}#{T:status-right}")
	return strings.TrimSpace(out)
}

// display is a tmux format, of the rail.
func (s *scratch) display(format string) string {
	out, _ := s.srv.run("display-message", "-p", "-t", sessionName+":"+homeWindow+".0", format)
	return strings.TrimSpace(out)
}

// active is a tmux format of the window's active pane, rather than of
// the rail — which is what display asks, and cannot answer where focus
// went.
func (s *scratch) active(format string) string {
	out, _ := s.srv.run("display-message", "-p", "-t", sessionName+":"+homeWindow, format)
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
	s.t.Fatalf("waited for %s\nrail:\n%s\nbar: %s\npanes: %s", what, s.rail(), s.bar(), s.panes())
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
		return s.display("#{pane_width}") == railW && strings.Contains(s.panes(), "home.1:conn:")
	})
	if hold := s.display("#{window_panes}"); hold != "2" {
		t.Errorf("home has %s panes", hold)
	}

	s.openShell()
	s.until("a shell in the slot", func() bool {
		return s.shellIn("home.1") && strings.Contains(s.rail(), scratchPlace)
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

	// The tree shows whatever else this machine is running too, so the
	// count to wait for is a rise from where it stood at the scratch's
	// own place, not a fixed number anywhere on the rail.
	before := s.placeRows()
	s.openShell()
	s.until("a second shell, with the first parked", func() bool {
		return s.shellIn("home.1") && s.parked(slotFirst)
	})
	s.until("the second shell's row", func() bool { return s.placeRows() > before })

	// The cursor is on the shell just opened, which is in the slot and
	// stands last, everything sitting where it started; k is the first
	// of the two, and enter brings it back.
	s.keys("k")
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
		return s.display("#{window_zoomed_flag}") == "0" && s.display("#{pane_width}") == railW
	})

	if _, err := s.srv.run("resize-window", "-x", "200", "-y", "40"); err != nil {
		t.Fatal(err)
	}
	s.until("the rail to hold its width", func() bool { return s.display("#{pane_width}") == railW })
}

// A shell that dies in the slot does not take the rail's width from
// it first: remain-on-exit holds the dead pane in the slot's own
// shape, so there is no moment the window is the rail alone, and conn
// swaps a hold into the dead pane once it reads that it is one.
func TestADeadSlotIsRevivedInPlaceNotResplit(t *testing.T) {
	s := startScratch(t)
	s.until("the console to finish", func() bool { return strings.Contains(s.rail(), prompt) })
	s.keys("Space")
	s.until("the slot to open", func() bool { return s.display("#{pane_width}") == railW })

	s.openShell()
	s.until("a shell in the slot", func() bool { return s.shellIn("home.1") })

	// The shell has the keys once s opens it, the same as select-pane
	// gave them there; exit goes to it directly, not through the rail.
	if _, err := s.srv.run("send-keys", "-t", sessionName+":"+homeWindow+".1", "exit", "Enter"); err != nil {
		t.Fatal(err)
	}
	s.until("the shell's pane to die, still in the slot", func() bool { return s.paneDead("home.1") })
	// remain-on-exit means this was never anything but true: the window
	// never had one pane to begin with, so there is nothing to catch mid
	// collapse.
	if n := s.display("#{window_panes}"); n != "2" {
		t.Errorf("home has %s panes with a dead shell in the slot", n)
	}
	if w := s.display("#{pane_width}"); w != railW {
		t.Errorf("the rail gave up its width to a dead shell: %s", w)
	}

	s.until("a hold to take the dead pane's place", func() bool {
		return strings.Contains(s.panes(), "home.1:conn:")
	})
	if n, w := s.display("#{window_panes}"), s.display("#{pane_width}"); n != "2" || w != railW {
		t.Errorf("the revival changed the window's shape: %s panes, %s wide", n, w)
	}
}

// x arms a kill on the shell in the slot, and x again confirms it:
// the shell is signalled, and the dead pane it leaves behind is
// revived the same way any other is — a hold in its place, the window
// never having given the rail's width up for it.
func TestXKillsTheEntryUnderTheCursor(t *testing.T) {
	s := startScratch(t)
	s.until("the console to finish", func() bool { return strings.Contains(s.rail(), prompt) })
	s.keys("Space")
	s.until("the slot to open", func() bool { return s.display("#{pane_width}") == railW })

	// The tree now shows whatever else this machine is running too, so
	// the row to wait for is a rise from where the count stood, and the
	// cursor a beat to follow the shell — awaited, but the watch reads
	// it on its own poll, not this test's.
	s.openShell()
	s.until("a shell in the slot", func() bool {
		return s.shellIn("home.1") && s.placeRows() == 1
	})

	s.keys("x")
	// The question conn asks is on the bar, beside CONFIRM, where the
	// window's width holds the whole of it.
	s.until("the kill armed", func() bool { return strings.Contains(s.bar(), "KILL") })
	s.keys("x")

	s.until("the shell's pane to die", func() bool { return s.paneDead("home.1") })
	s.until("a hold to take its place", func() bool {
		return strings.Contains(s.panes(), "home.1:conn:")
	})
	if w := s.display("#{pane_width}"); w != railW {
		t.Errorf("the rail gave up its width to the kill: %s", w)
	}
}

// x on what a shell runs ends the command, SIGTERM, and leaves the
// shell at its prompt — the same pane, still live — rather than taking
// it too; a kill of an entry is always the command alone.
func TestXEndsWhatAShellRunsAndKeepsTheShell(t *testing.T) {
	s := startScratch(t)
	s.until("the console to finish", func() bool { return strings.Contains(s.rail(), prompt) })
	s.keys("Space")
	s.until("the slot to open", func() bool { return s.display("#{pane_width}") == railW })

	s.openShell()
	s.until("a shell in the slot", func() bool { return s.shellIn("home.1") })

	if _, err := s.srv.run("send-keys", "-t", sessionName+":"+homeWindow+".1", "sleep 100", "Enter"); err != nil {
		t.Fatal(err)
	}
	// Two rows at the scratch's own place now: the shell, and the sleep
	// under it. Counting there rather than anywhere on the rail, which
	// is the whole machine's and has sleeps of its own on it.
	s.until("sleep running in the slot", func() bool {
		return s.placeRows() == 2 && strings.Contains(s.rail(), "sleep 100")
	})

	// The cursor stayed on the shell's own pid — it does not jump to a
	// child that only just appeared under it — and sleep is nested
	// right below it, the tree's next row down.
	s.keys("j")
	s.keys("x")
	s.until("the kill armed, naming sleep", func() bool { return strings.Contains(s.bar(), "END SLEEP 100") })
	s.keys("x")

	s.until("sleep to end and the shell to have the place to itself", func() bool {
		return s.placeRows() == 1
	})
	if s.paneDead("home.1") || !s.shellIn("home.1") {
		t.Errorf("the shell did not survive ending what it ran")
	}
}

// --light on a server already up puts it on the other ground where it
// stands: the sixteen and the ground the panes are drawn on change,
// the mode file says the new one, and the rail comes back painting
// from it — no conn down in between.
func TestTheGroundChangesUnderAServerAlreadyUp(t *testing.T) {
	s := startScratch(t)
	s.until("the console to finish", func() bool { return strings.Contains(s.rail(), prompt) })
	// The scratch server rose on dark, the ground of a terminal that
	// says nothing. tmux answers a color in its own case.
	if got := s.display("#{pane-colours[0]}"); !strings.EqualFold(got, darkScheme[0]) {
		t.Fatalf("the server did not rise on dark: slot 0 is %q", got)
	}

	// attach would take the terminal, which a test has none of; what is
	// under test is the ground, so the same steps run without a client.
	srv := &server{tmux: lookPath("tmux"), socket: s.srv.socket}
	conf := filepath.Join(filepath.Dir(srv.socket), "tmux.conf")
	applyMode(false)
	confText := tmuxConf("C-Space")
	applyMode(true) // the test binary goes back to the ground it had
	if err := os.WriteFile(conf, []byte(confText), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeMode(srv.socket, false); err != nil {
		t.Fatal(err)
	}
	if err := srv.reground(conf); err != nil {
		t.Fatal(err)
	}

	if got := s.display("#{pane-colours[0]}"); !strings.EqualFold(got, lightScheme[0]) {
		t.Errorf("slot 0 is %q after regrounding, not light's %q", got, lightScheme[0])
	}
	if got := s.display("#{window-style}"); !strings.EqualFold(got, "bg="+hex(lightGround)+",fg="+hex(lightInk)) {
		t.Errorf("the window style is %q, not on the light ground", got)
	}
	if dark, ok := readModeFile(srv.socket); !ok || dark {
		t.Errorf("the mode file was not put on light: dark %v, found %v", dark, ok)
	}
	// The rail came back, and came back conn: respawned it comes up on
	// the console, whose own identification carries the name.
	s.until("the rail to come back", func() bool {
		return strings.Contains(s.panes(), "home.0:conn:") && strings.Contains(s.rail(), "CONN 0.7.0")
	})
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

// A mode file beside the socket is what a real tmux server comes up on:
// light, when one says light, in the pane-colours a program in it would
// actually read back - the same wiring attach uses, proven against
// tmux itself rather than against tmuxConf's text. conn theme reads the
// same file, and conn down clears it.
func TestAServerComesUpOnItsModeFile(t *testing.T) {
	if testing.Short() {
		t.Skip("a real tmux server is not started under -short")
	}
	tmux := lookPath("tmux")
	if tmux == "" {
		t.Skip("tmux is not installed")
	}
	t.Cleanup(func() { applyMode(true) })

	dir, err := os.MkdirTemp("/tmp", "conn-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	srv := &server{tmux: tmux, socket: filepath.Join(dir, "sock")}
	t.Cleanup(func() { _, _ = srv.run("kill-server") })

	// Nothing has picked yet: a server not up comes up dark.
	if !serverMode(srv.socket) {
		t.Fatal("a socket with no mode file is not dark")
	}

	// A terminal that said light, on a first bring-up, leaves this
	// behind for attach to find; here it is put there by hand, the way
	// attach's own detectDark branch would.
	if err := writeMode(srv.socket, false); err != nil {
		t.Fatal(err)
	}

	// What attach does with a mode file already there: read it, and put
	// every color conn draws from on that ground, before tmuxConf is
	// asked for the server's look.
	dark, ok := readModeFile(srv.socket)
	if !ok || dark {
		t.Fatalf("readModeFile = (%v, %v), want (false, true)", dark, ok)
	}
	applyMode(dark)

	conf := filepath.Join(dir, "tmux.conf")
	if err := os.WriteFile(conf, []byte(tmuxConf(defaultPrefix)), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(tmux, "-S", srv.socket, "-f", conf, "new-session", "-d",
		"-s", sessionName, "-n", homeWindow, "sleep 30")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("starting the server: %v\n%s", out, err)
	}
	for _, c := range []struct{ option, want string }{
		{"pane-colours[0]", lightScheme[0]},
		{"pane-colours[9]", lightScheme[9]},
		{"cursor-colour", lightCursorHex},
	} {
		out, err := srv.run("show-options", "-g", c.option)
		if err != nil {
			t.Fatalf("%s: %v", c.option, err)
		}
		if got := strings.Fields(out); len(got) != 2 || !strings.EqualFold(got[1], c.want) {
			t.Errorf("%s: %q, want %s", c.option, out, c.want)
		}
	}

	// conn theme reads the file the same way, not an argument of its
	// own, so it never drifts from what the server actually came up on.
	claudeHome := t.TempDir()
	t.Setenv("CONN_SOCKET", srv.socket)
	if _, ok := dressProgram([]string{"claude"}, claudeHome, nil); !ok {
		t.Fatal("conn theme claude was not taken")
	}
	claudeJSON, err := os.ReadFile(filepath.Join(claudeHome, ".claude", "themes", "conn.json"))
	if err != nil || !strings.Contains(string(claudeJSON), `"base": "light-ansi"`) {
		t.Errorf("conn theme claude did not read the server's mode: %v\n%s", err, claudeJSON)
	}

	// conn down clears the file it wrote, so the next server to rise
	// asks the terminal fresh instead of remembering this one's ground.
	if err := srv.down(); err != nil {
		t.Fatal(err)
	}
	if _, ok := readModeFile(srv.socket); ok {
		t.Error("conn down left the mode file behind")
	}
	if !serverMode(srv.socket) {
		t.Error("after conn down, the socket is not dark again")
	}
}

// Tab is spelled "tab" by the time conn reads it, which is the thing a
// binding can only learn from a real terminal: bubbletea answers a key
// by its text where it has any, and a decoder that handed tab its own
// tab character would spell it "\t" and miss the case entirely. Nothing
// in a scratch server is waiting on anyone, so the note it writes is
// the proof the key arrived and landed where it was meant to.
func TestTabReachesTheWatchAsTab(t *testing.T) {
	s := startScratch(t)
	s.until("the console to finish", func() bool { return strings.Contains(s.rail(), prompt) })
	s.keys("Space")
	// The watch's first frame is not the settled window: conn is still
	// splitting the slot off behind it, and a key sent into that goes
	// nowhere. The slot being open is what says conn is listening.
	s.until("the slot to open on the watch", func() bool {
		return s.display("#{pane_width}") == railW && strings.Contains(s.rail(), "STATUS")
	})

	s.keys("Tab")
	s.until("the watch to answer tab", func() bool {
		return strings.Contains(s.rail(), "NO AGENT IS WAITING")
	})
}

// i opens the look in the slot, and the watch stays on the rail with
// the cursor still on the row the page is about — which is the whole
// point of the page being over there. The rail keeps its width, so the
// look arriving is a swap into the slot rather than a window laid out
// afresh, and focus stays where the keys are.
func TestILooksAtTheCursorsRowInTheSlot(t *testing.T) {
	s := startScratch(t)
	s.until("the console to finish", func() bool { return strings.Contains(s.rail(), prompt) })
	s.keys("Space")
	s.until("the slot to open on the watch", func() bool {
		return s.display("#{pane_width}") == railW && strings.Contains(s.rail(), "STATUS")
	})

	s.keys("i")
	s.until("the look to open in the slot", func() bool {
		return strings.Contains(s.slot(), "LOOK") && strings.Contains(s.slot(), "WHERE")
	})
	// The watch did not give up its pane, or its width, to say this.
	if r := s.rail(); !strings.Contains(r, "STATUS") || strings.Contains(r, "WHERE") {
		t.Errorf("the rail is not still the watch:\n%s", r)
	}
	if w := s.display("#{pane_width}"); w != railW {
		t.Errorf("the rail is %s wide, not %s", w, railW)
	}
	// And the keys are still the rail's: the look is a reading, not a
	// place to be put.
	if got := s.active("#{pane_index}"); got != "0" {
		t.Errorf("focus went to pane %s rather than staying on the rail", got)
	}

	// Reading down the list is j and k: the page follows the cursor,
	// so the one page serves the whole list and no key but j is
	// pressed. A page left on the row the cursor has walked away from
	// would be a page about nothing anybody is looking at.
	was := s.lookPidOf()
	if was == "" {
		t.Fatalf("the page says no pid:\n%s", s.slot())
	}
	s.keys("j")
	s.until("the page to follow the cursor down", func() bool {
		got := s.lookPidOf()
		return got != "" && got != was
	})
	moved := s.lookPidOf()
	s.keys("k")
	s.until("the page to follow it back", func() bool { return s.lookPidOf() == was })
	t.Logf("the page followed %s → %s → %s", was, moved, was)

	// Following costs nothing in panes: it is one page changing subject,
	// not a page per row.
	if w, n := s.display("#{session_windows}"), s.display("#{window_panes}"); w != "1" || n != "2" {
		t.Errorf("reading down the list left %s windows and %s panes in home", w, n)
	}

	// i again takes it down, and leaves a hold where an empty slot says
	// so. The key for the page is the key a reader reaches for to be rid
	// of it, and neither way costs a window.
	s.keys("i")
	s.until("the page to come down", func() bool {
		return !strings.Contains(s.slot(), "LOOK") && strings.Contains(s.slot(), holdWord)
	})
	s.keys("i")
	s.until("the page to come back", func() bool { return strings.Contains(s.slot(), "WHERE") })
	if w, n := s.display("#{session_windows}"), s.display("#{window_panes}"); w != "1" || n != "2" {
		t.Errorf("toggling left %s windows and %s panes in home", w, n)
	}

	// Reaching something real is rid of it, the way it is rid of a hold.
	s.openShell()
	s.until("a shell to take the slot from the look", func() bool { return s.shellIn("home.1") })
	if n := s.display("#{window_panes}"); n != "2" {
		t.Errorf("home has %s panes; the look was filed away rather than dropped", n)
	}
}
