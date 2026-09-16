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

// panelW is the panel's width, as tmux reports a pane's: the tests ask
// tmux rather than assume it, so a change to panelWidth does not also
// mean hunting down what was typed against it.
var panelW = strconv.Itoa(panelWidth)

// The server, against tmux itself. The test builds conn, brings a tmux
// server up on a scratch socket with conn in its home window, the way
// conn does for itself but detached, since a test has no terminal to
// attach, and drives the panel with send-keys, reading back what tmux
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

// keys sends keys to the panel.
func (s *scratch) keys(keys ...string) {
	s.t.Helper()
	if _, err := s.srv.run(append([]string{"send-keys", "-t", sessionName + ":" + homeWindow + ".0"}, keys...)...); err != nil {
		s.t.Fatal(err)
	}
}

// panel is what the panel pane shows.
func (s *scratch) panel() string {
	out, _ := s.srv.run("capture-pane", "-p", "-t", sessionName+":"+homeWindow+".0")
	return out
}

// scratchProject is what the panel calls the scratch root's repository:
// the one project in the processes view that is this test's own, since
// the processes view reads the whole machine's table and everything
// else it finds there is somewhere else entirely. The processes view
// names a project by what is left of its path once the root is taken
// off, and the root here is the scratch home, so the repository under
// it is called by its own name.
const scratchProject = "repo"

// readoutPid is the pid the readout in the bay says it is about, or ""
// when the bay is not a readout or has not read yet.
var readoutPidRe = regexp.MustCompile(`PID (\d+)`)

func (s *scratch) readoutPidOf() string {
	if m := readoutPidRe.FindStringSubmatch(s.bay()); m != nil {
		return m[1]
	}
	return ""
}

// sleepers puts processes of the test's own in the scratch project,
// each in a window of its own so each has a terminal and reads as a
// row. The machine a test runs on is not a fixture: this one has one
// repository and whatever conn is holding, and a test that needs rows
// to read has to bring them.
func (s *scratch) sleepers(n int) {
	s.t.Helper()
	for i := 0; i < n; i++ {
		if _, err := s.srv.run("new-window", "-d",
			"-c", filepath.Join(s.dir, "home", "repo"), "sleep 120"); err != nil {
			s.t.Fatal(err)
		}
	}
}

// bayPane is the id of whatever pane is in the bay.
func (s *scratch) bayPane() string {
	out, _ := s.srv.run("display-message", "-p", "-t", sessionName+":"+homeWindow+".1", "#{pane_id}")
	return strings.TrimSpace(out)
}

// bay is what the pane on the right shows.
func (s *scratch) bay() string {
	out, _ := s.srv.run("capture-pane", "-p", "-t", sessionName+":"+homeWindow+".1")
	return out
}

// projectRows is how many processes the panel says stand at that
// project, read off the project's own title line.
func (s *scratch) projectRows() int {
	return len(s.projectRowLines())
}

// projectRowLines is the scratch project's rows as the panel draws
// them, up to the blank that begins the next project.
//
// Only in the processes view. The list names the same project and puts
// the same processes under it, in a layout of its own — dense, with no
// blank between a project and its rows and none between one project and
// the next — so a count taken there answers a different question in the
// same units. openShell presses enter and returns without waiting for
// the view to come back, so the list is on the panel often enough for a
// careless count to be the list's; that is how a baseline taken here
// once came to be measured against the wrong view.
func (s *scratch) projectRowLines() []string {
	if !s.inProcesses() {
		return nil
	}
	lines := strings.Split(s.panel(), "\n")
	for i, line := range lines {
		// The title line, and not a row that merely says the word: a
		// project is named alone on its line.
		if strings.TrimSpace(line) != scratchProject {
			continue
		}
		// The title has a row of air under it, and then its rows.
		rows := lines[i+1:]
		if len(rows) > 0 && strings.TrimSpace(rows[0]) == "" {
			rows = rows[1:]
		}
		var out []string
		for _, r := range rows {
			if strings.TrimSpace(r) == "" {
				break
			}
			out = append(out, r)
		}
		return out
	}
	return nil
}

// shellRows is how many of the scratch project's rows are shells, which
// is what a test that opened shells is asking about. It steps over a
// tree's inner rows and over whatever else the machine is running.
func (s *scratch) shellRows() int {
	n := 0
	for _, r := range s.projectRowLines() {
		if strings.HasPrefix(strings.TrimSpace(r), "SHELL") {
			n++
		}
	}
	return n
}

// openShell opens a shell at the scratch root's own repository, from
// the list rather than with s at the cursor: the processes view reads
// the whole machine's process table, so what the cursor lands on is
// whatever else is running here, while the list is only ever the roots
// this test laid down. The cursor follows the shell it opens, so a test
// that goes on to act on it acts on that one.
func (s *scratch) openShell() {
	s.t.Helper()
	s.keys("p")
	s.until("the list to find the scratch repository", func() bool {
		return strings.Contains(s.panel(), "PROJECTS") && strings.Contains(s.panel(), "repo")
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

// shellIn says whether the pane at a project runs a shell. /bin/sh is a
// bash on macOS and calls itself one.
func (s *scratch) shellIn(at string) bool {
	f := strings.Split(s.paneAt(at), ":")
	return len(f) == 3 && (f[1] == "sh" || f[1] == "bash" || f[1] == "zsh" || f[1] == "dash")
}

// paneDead says whether the pane at a project is held dead on
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

// inProcesses says whether the keys are on the panel in the processes
// view. It asks the status line, which is conn's own word for the view
// its keys are in — not the panel's layout. A test keyed to a word the
// header happened to hold breaks the day the header goes, which is how
// four of these came to fail at once.
func (s *scratch) inProcesses() bool {
	return strings.Contains(s.statusLine(), "PROCS")
}

// statusLine is the status line as tmux expands it: what conn has put
// there for wherever its keys are.
func (s *scratch) statusLine() string {
	out, _ := s.srv.run("display-message", "-p", "-t", sessionName+":"+homeWindow+".0",
		"#{T:status-left}#{T:status-right}")
	return strings.TrimSpace(out)
}

// display is a tmux format, of the panel.
func (s *scratch) display(format string) string {
	out, _ := s.srv.run("display-message", "-p", "-t", sessionName+":"+homeWindow+".0", format)
	return strings.TrimSpace(out)
}

// active is a tmux format of the window's active pane, rather than of
// the panel — which is what display asks, and cannot answer where focus
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
	s.t.Fatalf("waited for %s\nrail:\n%s\nbar: %s\npanes: %s", what, s.panel(), s.statusLine(), s.panes())
}

// conn comes up in the home window: the console across it, then on a
// key the panel at its width with the hold beside it; s opens a shell
// into the bay and its row is on the panel; a second s opens another
// and the first parks in a window of its own; enter on the first brings
// it back; c zooms the console over the window and a key gives the bay
// its side again; the panel holds its width when the window is resized.
func TestTheServerHoldsThePanelAndTheBay(t *testing.T) {
	s := startScratch(t)
	s.until("the console to finish", func() bool { return strings.Contains(s.panel(), prompt) })
	if !strings.Contains(s.panel(), "START-UP CHECKS") || s.display("#{pane_width}") != "160" {
		t.Errorf("the console should have the whole window:\n%s", s.panel())
	}

	s.keys("Space")
	s.until("the bay to open", func() bool {
		return s.display("#{pane_width}") == panelW && strings.Contains(s.panes(), "home.1:conn:")
	})
	if hold := s.display("#{window_panes}"); hold != "2" {
		t.Errorf("home has %s panes", hold)
	}

	s.openShell()
	s.until("a shell in the bay", func() bool {
		return s.shellIn("home.1") && s.projectRows() >= 1
	})
	if strings.Contains(s.panes(), "conn:") && strings.Count(s.panes(), "conn:") > 1 {
		t.Errorf("the hold should be gone once a shell is in the bay: %s", s.panes())
	}
	first := s.display("#{pane_id}")
	bayFirst, _ := s.srv.run("display-message", "-p", "-t", sessionName+":"+homeWindow+".1", "#{pane_id}")
	bayFirst = strings.TrimSpace(bayFirst)
	if first == bayFirst {
		t.Fatalf("the panel and the bay are one pane: %s", s.panes())
	}

	// Both shells stand at the scratch's own project, so what to wait for
	// is the two of them by name. A rise in the row count was the older
	// way to ask, and it asked against a number taken a moment earlier —
	// which is only the same number if the panel is in the same view and
	// the project is holding nothing else transient. Counting the shells
	// is the thing the test is actually about, and it needs no baseline.
	s.openShell()
	s.until("a second shell, with the first parked", func() bool {
		return s.shellIn("home.1") && s.parked(bayFirst)
	})
	s.until("the second shell's row", func() bool { return s.shellRows() >= 2 })

	// The cursor is on the shell just opened, which is in the bay and
	// stands last, everything sitting where it started; k is the first
	// of the two, and enter brings it back.
	s.keys("k")
	s.keys("Enter")
	s.until("the first shell back in the bay", func() bool {
		return strings.HasSuffix(s.paneAt("home.1"), ":"+bayFirst)
	})

	s.keys("c")
	s.until("the console zoomed over the window", func() bool {
		return s.display("#{window_zoomed_flag}") == "1" && strings.Contains(s.panel(), "START-UP CHECKS")
	})
	s.keys("Space")
	s.until("the bay to have its side again", func() bool {
		return s.display("#{window_zoomed_flag}") == "0" && s.display("#{pane_width}") == panelW
	})

	if _, err := s.srv.run("resize-window", "-x", "200", "-y", "40"); err != nil {
		t.Fatal(err)
	}
	s.until("the panel to hold its width", func() bool { return s.display("#{pane_width}") == panelW })
}

// A shell that dies in the bay does not take the panel's width from
// it first: remain-on-exit holds the dead pane in the bay's own
// shape, so there is no moment the window is the panel alone, and conn
// swaps a hold into the dead pane once it reads that it is one.
// A window of work whose work has ended is done, and goes with it.
// remain-on-exit belongs to the bay, so that a pane dying there does
// not collapse the layout under conn; set for the whole server it also
// held every parked window standing after its shell exited, dead,
// where nothing in conn ever showed it and nothing short of conn down
// ever cleared it.
func TestAParkedWindowGoesWhenItsWorkEnds(t *testing.T) {
	s := startScratch(t)
	s.until("the console to finish", func() bool { return strings.Contains(s.panel(), prompt) })
	s.keys("Space")
	s.until("the bay to open", func() bool { return s.display("#{pane_width}") == panelW })

	s.openShell()
	s.until("a shell in the bay", func() bool { return s.shellIn("home.1") })
	first, _ := s.srv.run("display-message", "-p", "-t", sessionName+":"+homeWindow+".1", "#{pane_id}")
	first = strings.TrimSpace(first)

	// A second shell takes the bay and parks the first in a window of
	// its own, which is where work waits when it is not in front of you.
	s.openShell()
	s.until("the first shell parked in a window of its own", func() bool { return s.parked(first) })

	if _, err := s.srv.run("send-keys", "-t", first, "exit", "Enter"); err != nil {
		t.Fatal(err)
	}
	s.until("the parked window to go with its shell", func() bool {
		return !strings.Contains(s.panes(), ":"+first)
	})

	// And the bay is still the bay. What ended was somewhere else, and
	// home neither collapsed nor gave up the panel's width.
	if n, w := s.display("#{window_panes}"), s.display("#{pane_width}"); n != "2" || w != panelW {
		t.Errorf("home changed shape when a parked window went: %s panes, %s wide", n, w)
	}
}

// The key the prefix then s sends, which opens a shell at the project
// the panel is looking at and puts it in the bay. The prefix half
// cannot be driven here: send-keys writes to the pane and never
// reaches tmux's key table, and a scratch server has no client
// attached to press a prefix at. What the chord is bound to send is
// held by the configuration; this is what happens when it lands.
func TestTheShellKeyOpensIntoTheBay(t *testing.T) {
	s := startScratch(t)
	s.until("the console to finish", func() bool { return strings.Contains(s.panel(), prompt) })
	s.keys("Space")
	s.until("the bay to open", func() bool { return s.display("#{pane_width}") == panelW })

	s.openShell()
	s.until("a shell in the bay", func() bool { return s.shellIn("home.1") })
	first := s.bayPane()
	// The key opens at the project the cursor stands on, and the cursor
	// does not reach the shell until the reading that has it does.
	s.until("the shell's row on the panel", func() bool { return s.projectRows() >= 1 })

	s.keys("M-s")
	s.until("a second shell in the bay, with the first parked", func() bool {
		return s.shellIn("home.1") && s.bayPane() != first && s.parked(first)
	})
	if n, w := s.display("#{window_panes}"), s.display("#{pane_width}"); n != "2" || w != panelW {
		t.Errorf("the key changed the window's shape: %s panes, %s wide", n, w)
	}
}

// The key the prefix then j sends, which walks to the next process
// conn holds and puts it in the bay. With two shells open and the
// second in the bay, it brings the first back: the ring is the panes
// there are, and from the last of them it comes round to the first.
func TestTheRingKeyWalksToTheOtherHeldProcess(t *testing.T) {
	s := startScratch(t)
	s.until("the console to finish", func() bool { return strings.Contains(s.panel(), prompt) })
	s.keys("Space")
	s.until("the bay to open", func() bool { return s.display("#{pane_width}") == panelW })

	s.openShell()
	s.until("a shell in the bay", func() bool { return s.shellIn("home.1") })
	first := s.bayPane()

	s.openShell()
	s.until("a second shell, with the first parked", func() bool {
		return s.shellIn("home.1") && s.bayPane() != first && s.parked(first)
	})
	// The ring steps from where the cursor stands, and the cursor does
	// not reach the second shell until the reading that has it does.
	s.until("both shells' rows on the panel", func() bool { return s.projectRows() >= 2 })

	s.keys("M-j")
	s.until("the other held shell to come round into the bay", func() bool { return s.bayPane() == first })
	if n, w := s.display("#{window_panes}"), s.display("#{pane_width}"); n != "2" || w != panelW {
		t.Errorf("the ring changed the window's shape: %s panes, %s wide", n, w)
	}
}

func TestADeadBayIsRevivedInPlaceNotResplit(t *testing.T) {
	s := startScratch(t)
	s.until("the console to finish", func() bool { return strings.Contains(s.panel(), prompt) })
	s.keys("Space")
	s.until("the bay to open", func() bool { return s.display("#{pane_width}") == panelW })

	s.openShell()
	s.until("a shell in the bay", func() bool { return s.shellIn("home.1") })

	// The shell has the keys once s opens it, the same as select-pane
	// gave them there; exit goes to it directly, not through the panel.
	if _, err := s.srv.run("send-keys", "-t", sessionName+":"+homeWindow+".1", "exit", "Enter"); err != nil {
		t.Fatal(err)
	}
	s.until("the shell's pane to die, still in the bay", func() bool { return s.paneDead("home.1") })
	// remain-on-exit means this was never anything but true: the window
	// never had one pane to begin with, so there is nothing to catch mid
	// collapse.
	if n := s.display("#{window_panes}"); n != "2" {
		t.Errorf("home has %s panes with a dead shell in the bay", n)
	}
	if w := s.display("#{pane_width}"); w != panelW {
		t.Errorf("the panel gave up its width to a dead shell: %s", w)
	}

	s.until("a hold to take the dead pane's place", func() bool {
		return strings.Contains(s.panes(), "home.1:conn:")
	})
	if n, w := s.display("#{window_panes}"), s.display("#{pane_width}"); n != "2" || w != panelW {
		t.Errorf("the revival changed the window's shape: %s panes, %s wide", n, w)
	}
}

// x arms a kill on the shell in the bay, and x again confirms it:
// the shell is signalled, and the dead pane it leaves behind is
// revived the same way any other is — a hold in its project, the window
// never having given the panel's width up for it.
func TestXKillsTheEntryUnderTheCursor(t *testing.T) {
	s := startScratch(t)
	s.until("the console to finish", func() bool { return strings.Contains(s.panel(), prompt) })
	s.keys("Space")
	s.until("the bay to open", func() bool { return s.display("#{pane_width}") == panelW })

	// The tree now shows whatever else this machine is running too, so the
	// row to wait for is a rise from where the count stood, and the cursor
	// a beat to follow the shell — awaited, but the processes view reads
	// it on its own poll, not this test's.
	s.openShell()
	s.until("a shell in the bay", func() bool {
		return s.shellIn("home.1") && s.projectRows() == 1
	})

	s.keys("x")
	// The question conn asks is on the status line, beside CONFIRM, where
	// the window's width holds the whole of it.
	s.until("the kill armed", func() bool { return strings.Contains(s.statusLine(), "KILL") })
	s.keys("x")

	s.until("the shell's pane to die", func() bool { return s.paneDead("home.1") })
	s.until("a hold to take its project", func() bool {
		return strings.Contains(s.panes(), "home.1:conn:")
	})
	if w := s.display("#{pane_width}"); w != panelW {
		t.Errorf("the panel gave up its width to the kill: %s", w)
	}
}

// x on what a shell runs ends the command, SIGTERM, and leaves the
// shell at its prompt — the same pane, still live — rather than taking
// it too; a kill of an entry is always the command alone.
func TestXEndsWhatAShellRunsAndKeepsTheShell(t *testing.T) {
	s := startScratch(t)
	s.until("the console to finish", func() bool { return strings.Contains(s.panel(), prompt) })
	s.keys("Space")
	s.until("the bay to open", func() bool { return s.display("#{pane_width}") == panelW })

	s.openShell()
	s.until("a shell in the bay", func() bool { return s.shellIn("home.1") })

	if _, err := s.srv.run("send-keys", "-t", sessionName+":"+homeWindow+".1", "sleep 100", "Enter"); err != nil {
		t.Fatal(err)
	}
	// Two rows at the scratch's own project now: the shell, and the sleep
	// under it. Counting there rather than anywhere on the panel, which is
	// the whole machine's and has sleeps of its own on it.
	s.until("sleep running in the bay", func() bool {
		return s.projectRows() == 2 && strings.Contains(s.panel(), "sleep 100")
	})

	// The cursor stayed on the shell's own pid — it does not jump to a
	// child that only just appeared under it — and sleep is nested
	// right below it, the tree's next row down.
	s.keys("j")
	s.keys("x")
	s.until("the kill armed, naming sleep", func() bool { return strings.Contains(s.statusLine(), "END SLEEP ") })
	s.keys("x")

	s.until("sleep to end and the shell to have the project to itself", func() bool {
		return s.projectRows() == 1
	})
	if s.paneDead("home.1") || !s.shellIn("home.1") {
		t.Errorf("the shell did not survive ending what it ran")
	}
}

// --light on a server already up puts it on the other ground where it
// stands: the sixteen and the ground the panes are drawn on change,
// the mode file says the new one, and the panel comes back painting
// from it — no conn down in between.
func TestTheGroundChangesUnderAServerAlreadyUp(t *testing.T) {
	holdMode(t)
	s := startScratch(t)
	s.until("the console to finish", func() bool { return strings.Contains(s.panel(), prompt) })
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
	applyMode(true) // the rest of this test reads the dark table
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
	// The panel came back, and came back conn: respawned, it comes up
	// on the console and runs it through to the end. The version it
	// says is not the thing to look for — it moves with every release,
	// and a shallow checkout with no tags has none to say — so what is
	// waited for is the page finishing.
	s.until("the panel to come back", func() bool {
		return strings.Contains(s.panes(), "home.0:conn:") && strings.Contains(s.panel(), prompt)
	})
}

// conn down ends what the test brought up, and says so; a second
// conn down finds nothing.
func TestDownEndsTheScratchServer(t *testing.T) {
	s := startScratch(t)
	s.until("the console to finish", func() bool { return strings.Contains(s.panel(), prompt) })
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
	holdMode(t)

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
	// asked for the server's own drawing.
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

// The panel opens the readout in the bay, and the processes view stays on the
// panel with the cursor still on the row the page is about — which is
// the whole point of the page being over there. The panel keeps its
// width, so the readout arriving is a swap into the bay rather than a
// window laid out afresh, and focus stays where the keys are.
func TestThePageFollowsTheCursorDownTheList(t *testing.T) {
	s := startScratch(t)
	s.until("the console to finish", func() bool { return strings.Contains(s.panel(), prompt) })
	s.keys("Space")
	s.until("the bay to open in the processes view", func() bool {
		return s.display("#{pane_width}") == panelW && s.inProcesses()
	})

	// Rows of the test's own to read and to walk between. The page needs
	// something under the cursor and j needs somewhere to go, and what
	// the machine running the test happens to have is not that: a
	// container has nothing at all in it but conn.
	s.sleepers(2)
	s.until("the sleepers' rows on the panel", func() bool { return s.projectRows() >= 2 })

	// Nothing is pressed for the page: it is what the workspace holds
	// while the keys are on the panel in this view.
	s.until("the readout to take the workspace", func() bool {
		return strings.Contains(s.bay(), "READOUT") && strings.Contains(s.bay(), "WHERE")
	})
	// The processes view did not give up its pane, or its width, to say
	// this.
	if r := s.panel(); !s.inProcesses() || strings.Contains(r, "WHERE") {
		t.Errorf("the panel is not still the processes view:\n%s", r)
	}
	if w := s.display("#{pane_width}"); w != panelW {
		t.Errorf("the panel is %s wide, not %s", w, panelW)
	}
	// And the keys are still the panel's: the readout is a reading, not a
	// project to be put.
	if got := s.active("#{pane_index}"); got != "0" {
		t.Errorf("focus went to pane %s rather than staying on the panel", got)
	}

	// The baseline for the counts below, taken here and not earlier.
	// What they assert is that reading down the list costs no window, so
	// the number to hold against is the one the view settles at with the
	// page already up. Taken before that, it can catch the window the
	// page is made in — showReadout opens one, swaps the page into the
	// workspace and kills what it displaced — and a baseline one too
	// high fails against a steady state that was never wrong.
	windows := s.display("#{session_windows}")

	// Reading down the list is j and k: the page follows the cursor,
	// so the one page serves the whole list and no key but j is
	// pressed. A page left on the row the cursor has walked away from
	// would be a page about nothing anybody is looking at.
	was := s.readoutPidOf()
	if was == "" {
		t.Fatalf("the page says no pid:\n%s", s.bay())
	}
	s.keys("j")
	s.until("the page to follow the cursor down", func() bool {
		got := s.readoutPidOf()
		return got != "" && got != was
	})
	moved := s.readoutPidOf()
	s.keys("k")
	s.until("the page to follow it back", func() bool { return s.readoutPidOf() == was })
	t.Logf("the page followed %s → %s → %s", was, moved, was)

	// Following costs nothing in panes: it is one page changing subject,
	// not a page per row.
	if w, n := s.display("#{session_windows}"), s.display("#{window_panes}"); w != windows || n != "2" {
		t.Errorf("reading down the list left %s windows and %s panes in home, not %s and 2", w, n, windows)
	}

	// No key takes the page away, because none puts it there: i is a
	// letter like any other now, and the page is simply what the
	// workspace holds in this view.
	s.keys("i")
	s.until("the sleepers' rows to be walked again", func() bool { return s.readoutPidOf() != "" })
	if !strings.Contains(s.bay(), "READOUT") {
		t.Errorf("a key took the page away:\n%s", s.bay())
	}

	// Going into a process is what takes it: the workspace holds that
	// instead, and it costs no window of its own.
	s.openShell()
	s.until("a shell to take the bay from the readout", func() bool { return s.shellIn("home.1") })
	if n := s.display("#{window_panes}"); n != "2" {
		t.Errorf("home has %s panes; the readout was filed away rather than dropped", n)
	}
}

// The workspace holds the page while the keys are on the panel. Going
// into a process puts that process there and the keys with it; bringing
// the keys back to the processes view brings the page back, about the
// row the cursor is on, which after reaching something is that
// something. Nothing is pressed for it.
func TestThePageIsWhatTheWorkspaceHoldsInTheProcessesView(t *testing.T) {
	s := startScratch(t)
	s.until("the console to finish", func() bool { return strings.Contains(s.panel(), prompt) })
	s.keys("Space")
	s.until("the workspace to open", func() bool { return s.display("#{pane_width}") == panelW })

	// A row of the test's own to be about, and the page comes up for it
	// without anybody asking.
	s.sleepers(1)
	s.until("the sleeper's row on the panel", func() bool { return s.projectRows() >= 1 })
	s.until("the page to take the workspace", func() bool { return strings.Contains(s.bay(), "READOUT") })

	// Going into a process puts the process there instead.
	s.openShell()
	s.until("a shell in the workspace", func() bool {
		return s.shellIn("home.1") && !strings.Contains(s.bay(), "READOUT")
	})

	// The keys coming back bring the page back. That half cannot be
	// driven here: the keys arriving is a focus event, a tmux server
	// with no client attached sends none, and a test has no terminal to
	// attach one from. What the rule does with the keys is held at the
	// model, where the message can be handed over directly.
	if w, n := s.display("#{pane_width}"), s.display("#{window_panes}"); w != panelW || n != "2" {
		t.Errorf("home changed shape: %s wide, %s panes", w, n)
	}
}

// Cancelling the list puts the operator back where the chord came from,
// and putting them back means reaching that pane, not selecting it.
// By the time the cancel comes the pane is not in the workspace: the
// page takes the workspace while the keys are on the panel, and what it
// displaced went to a window of its own. Selecting a pane there does
// select it — in a window the client is not looking at, which is
// nothing happening at all.
//
// The page cannot be the one to park it here, for the reason the rule
// above gives: that turn is a focus event and this server has no client
// to send one. A second shell parks the first just as well, and being
// parked is the whole of what the cancel has to deal with. The prefix
// half cannot be driven either; what the binding writes down before it
// sends its key is written here in its place.
func TestCancellingTheListGoesBackIntoTheProcess(t *testing.T) {
	s := startScratch(t)
	s.until("the console to finish", func() bool { return strings.Contains(s.panel(), prompt) })
	s.keys("Space")
	s.until("the bay to open", func() bool { return s.display("#{pane_width}") == panelW })

	s.openShell()
	s.until("a shell in the bay", func() bool { return s.shellIn("home.1") })
	first := s.bayPane()
	s.until("the shell's row on the panel", func() bool { return s.projectRows() >= 1 })

	s.openShell()
	s.until("a second shell, with the first parked", func() bool {
		return s.shellIn("home.1") && s.bayPane() != first && s.parked(first)
	})

	// The chord, as the binding fires it out of the first shell's pane.
	if _, err := s.srv.run("set-option", "-g", "@conn_from", first); err != nil {
		t.Fatal(err)
	}
	s.keys("M-p")
	s.until("the list", func() bool { return strings.Contains(s.panel(), "PROJECTS") })

	// esc brings the view back and the shell with it: out of its own
	// window, into the workspace, with the keys in it.
	s.keys("Escape")
	s.until("the shell back in the workspace", func() bool { return s.bayPane() == first })
	if s.parked(first) {
		t.Error("the shell stayed in a window of its own")
	}
	if got := s.active("#{pane_id}"); got != first {
		t.Errorf("the keys are in %s, not the shell the chord came from", got)
	}
	if n, w := s.display("#{window_panes}"), s.display("#{pane_width}"); n != "2" || w != panelW {
		t.Errorf("the cancel changed the window's shape: %s panes, %s wide", n, w)
	}

	// p on the panel is not a chord and leaves nothing to go back to:
	// the cancel is the view alone, and nothing is put anywhere.
	bay := s.bayPane()
	s.keys("p")
	s.until("the list from the panel", func() bool { return strings.Contains(s.panel(), "PROJECTS") })
	s.keys("Escape")
	s.until("the processes view", func() bool { return s.inProcesses() })
	if got := s.bayPane(); got != bay {
		t.Errorf("esc from a list opened on the panel moved the workspace to %s", got)
	}
}

// A project's .conn, against the server: its declarations are down
// rows on the panel from the first reading; u brings them up, parked
// and marked, and the one that ended at once reads ENDED where the one
// still running reads ACTIVE; enter on the ended one puts its pane in
// the bay to be read; and x closes that pane, whereupon the row is down
// again.
func TestUBringsUpWhatTheProjectDeclares(t *testing.T) {
	s := startScratch(t)
	repo := filepath.Join(s.dir, "home", "repo")
	if err := os.WriteFile(filepath.Join(repo, declaredName), []byte("sleeper: sleep 120\nquick: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.until("the console to finish", func() bool { return strings.Contains(s.panel(), prompt) })
	s.keys("Space")
	s.until("the bay to open", func() bool { return s.display("#{pane_width}") == panelW })

	rowSays := func(name, word string) bool {
		for _, l := range s.projectRowLines() {
			if strings.Contains(l, name+" · ") && strings.HasSuffix(strings.TrimRight(l, " "), word) {
				return true
			}
		}
		return false
	}
	s.until("the two down rows", func() bool { return rowSays("sleeper", "DOWN") && rowSays("quick", "DOWN") })

	// marked is the id of the pane carrying a declaration's mark, and
	// what it recorded of its end.
	marked := func(name string) (id, exit string) {
		out, _ := s.srv.run("list-panes", "-a", "-F", "#{pane_id} #{@conn_declared} #{@conn_exit}")
		for _, l := range strings.Split(out, "\n") {
			f := strings.Split(l, " ")
			if len(f) == 3 && strings.HasPrefix(f[1], name+"@") {
				return f[0], f[2]
			}
		}
		return "", ""
	}
	// The panel is the whole machine's, and the cursor came up on its
	// first row, which is some other project's. The scratch project is
	// under /tmp, which sorts after everything under /Users, so its
	// rows are the last: G is a row of it.
	s.keys("G")
	s.keys("u")
	s.until("both panes opened and marked, quick's end recorded", func() bool {
		sleeper, _ := marked("sleeper")
		quick, exit := marked("quick")
		return sleeper != "" && quick != "" && exit == "0"
	})
	s.until("sleeper ACTIVE and quick ENDED on the panel", func() bool {
		return rowSays("sleeper", "ACTIVE") && rowSays("quick", "ENDED")
	})
	// The panes were parked: the bay still holds conn's own furniture.
	if s.shellIn("home.1") {
		t.Error("u put a pane in the bay")
	}

	// The cursor waits on the first pane raised, sleeper's head; under
	// it the sleep, and then quick's row. Enter there is into its pane.
	quick, _ := marked("quick")
	s.keys("j")
	s.keys("j")
	s.keys("Enter")
	s.until("quick's pane in the bay", func() bool { return s.bayPane() == quick })

	s.keys("x")
	s.until("the close armed, naming quick", func() bool { return strings.Contains(s.statusLine(), "CLOSE QUICK ") })
	s.keys("x")
	s.until("the pane gone and quick down again", func() bool {
		id, _ := marked("quick")
		return id == "" && rowSays("quick", "DOWN") && rowSays("sleeper", "ACTIVE")
	})
}
