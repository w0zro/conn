package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/w0zro/conn/internal/console"
	"github.com/w0zro/conn/internal/draw"
	"github.com/w0zro/conn/internal/work"

	"github.com/w0zro/conn/internal/station"

	"github.com/w0zro/conn/internal/tmux"

	"github.com/w0zro/conn/internal/theme"

	"github.com/w0zro/conn/internal/config"
)

// panelW is the panel's width, as tmux reports a pane's: the tests ask
// tmux rather than assume it, so a change to panelWidth does not also
// mean hunting down what was typed against it.
var panelW = strconv.Itoa(tmux.PanelWidth)

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
	srv *tmux.Server
	dir string
}

func startScratch(t *testing.T) *scratch {
	t.Helper()
	if testing.Short() {
		t.Skip("a real tmux server is not started under -short")
	}
	tmuxBin := station.LookPath("tmux")
	if tmuxBin == "" {
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
	s := &scratch{t: t, srv: &tmux.Server{Tmux: tmuxBin, Socket: filepath.Join(dir, "sock")}, dir: dir}
	t.Cleanup(func() { _, _ = s.srv.Run("kill-server") })
	conf := filepath.Join(dir, "tmux.conf")
	if err := os.WriteFile(conf, []byte(serverConfOn("C-Space", theme.Conn.Dark)), 0o600); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(filepath.Join(home, "repo", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	// The scratch conn keeps its own configuration, in the test's own
	// directory, and the test reads the same one. Without this it wrote
	// into whatever config directory the test binary was run with — the
	// suite's own, shared by every test in the process — so a conn told
	// to wear a theme in one test came up wearing it in the next.
	config := filepath.Join(dir, "config")
	t.Setenv("XDG_CONFIG_HOME", config)
	cmd := exec.Command(tmuxBin, "-S", s.srv.Socket, "-f", conf, "new-session", "-d", "-x", "160", "-y", "40",
		"-s", tmux.SessionName, "-n", tmux.HomeWindow, "-c", home, "exec "+tmux.ShellQuote(bin))
	cmd.Env = append(tmux.WithoutTmux(os.Environ()),
		"CONN_SOCKET="+s.srv.Socket, "XDG_STATE_HOME="+filepath.Join(dir, "state"),
		"XDG_CONFIG_HOME="+config, "CONN_ROOTS="+home,
		"HOME="+home, "TERM=xterm-256color", "COLORTERM=truecolor", "SHELL=/bin/sh")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("starting the server: %v\n%s", err, out)
	}
	return s
}

// keys sends keys to the panel.
func (s *scratch) keys(keys ...string) {
	s.t.Helper()
	if _, err := s.srv.Run(append([]string{"send-keys", "-t", tmux.SessionName + ":" + tmux.HomeWindow + ".0"}, keys...)...); err != nil {
		s.t.Fatal(err)
	}
}

// panel is what the panel pane shows.
func (s *scratch) panel() string {
	out, _ := s.srv.Run("capture-pane", "-p", "-t", tmux.SessionName+":"+tmux.HomeWindow+".0")
	return out
}

// pageUp is whether the workspace holds the readout: the page of
// groups under its header, or a contact's sheet, which has no header
// and is told by its specifications. Which one comes up depends on the
// row the cursor lands on, and the machine running the test may well
// have a contact in its table.
func (s *scratch) pageUp() bool {
	bay := s.bay()
	return strings.Contains(bay, "READOUT") || strings.Contains(bay, "SPECIFICATIONS")
}

// finished is whether the console has come to its verdict, the last
// thing it shows: every check's row says NOMINAL, so the word alone
// does not tell the end from the middle.
func (s *scratch) finished() bool {
	panel := s.panel()
	return strings.Contains(panel, console.AllNominal) || strings.Contains(panel, "NOT NOMINAL")
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
var readoutPidRe = regexp.MustCompile(`(?:PID|Process \.+) (\d+)`)

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
		if _, err := s.srv.Run("new-window", "-d",
			"-c", filepath.Join(s.dir, "home", "repo"), "sleep 120"); err != nil {
			s.t.Fatal(err)
		}
	}
}

// bayPane is the id of whatever pane is in the bay.
func (s *scratch) bayPane() string {
	out, _ := s.srv.Run("display-message", "-p", "-t", tmux.SessionName+":"+tmux.HomeWindow+".1", "#{pane_id}")
	return strings.TrimSpace(out)
}

// bayKeys sends keys to the pane on the right, which is where they are
// while a page of conn's own stands there.
func (s *scratch) bayKeys(keys ...string) {
	s.t.Helper()
	if _, err := s.srv.Run(append([]string{"send-keys", "-t", tmux.SessionName + ":" + tmux.HomeWindow + ".1"}, keys...)...); err != nil {
		s.t.Fatal(err)
	}
}

// bay is what the pane on the right shows.
func (s *scratch) bay() string {
	out, _ := s.srv.Run("capture-pane", "-p", "-t", tmux.SessionName+":"+tmux.HomeWindow+".1")
	return out
}

// projectRows is how many processes the panel says stand at that
// project, read off the project's own title line.
func (s *scratch) projectRows() int {
	return len(s.projectRowLines())
}

// projectRowLines is the scratch project's rows as the panel draws
// them: filed by project, a row is the mark of its kind and its
// command, so the scratch project's rows are the ones under its
// eyebrow, down to the next eyebrow or the end.
//
// Only in the processes view. The list names the same project and puts
// the same processes under it, in a layout of its own, so a count taken
// there answers a different question in the same units. openShell
// presses enter and returns without waiting for the view to come back,
// so the list is on the panel often enough for a careless count to be
// the list's; that is how a baseline taken here once came to be
// measured against the wrong view.
func (s *scratch) projectRowLines() []string {
	if !s.inProcesses() {
		return nil
	}
	var out []string
	under := false
	for _, line := range strings.Split(s.panel(), "\n") {
		switch {
		case strings.HasPrefix(strings.TrimSpace(line), scratchProject+" ─"):
			under = true
		case isEyebrow(line):
			under = false
		case under && isRow(line):
			out = append(out, line)
		}
	}
	return out
}

// isEyebrow says whether a panel line is a block's head: a label with a
// rule running off it to the right edge. It is where one project's rows
// end and the next project's begin.
func isEyebrow(line string) bool {
	return !isRow(line) && strings.Contains(line, " ─")
}

// isRow says whether a panel line is a process row: past the margin —
// the cursor's mark, the bay's bar, a spinner turning — it begins with
// the mark of a kind.
func isRow(line string) bool {
	f := rowFields(line)
	if len(f) == 0 {
		return false
	}
	return isMark(f[0])
}

// rowFields is a panel line's words with the margin's marks dropped:
// the cursor's mark and the bay's bar, which a spinner's frame stands
// beside without a space between them.
func rowFields(line string) []string {
	var out []string
	for _, f := range strings.Fields(line) {
		f = strings.TrimLeft(f, "▸"+draw.CursorBar+strings.Join(draw.Spinner, ""))
		if f == "" {
			continue
		}
		out = append(out, f)
	}
	return out
}

// rowSays says whether the scratch project has a row of that name
// saying that word at its right: DOWN or ENDED for what is not
// running, and nothing at all for a row that is up, which has nothing
// to report and so says nothing.
func (s *scratch) rowSays(name, word string) bool {
	for _, line := range s.projectRowLines() {
		if rowCommand(line) != name {
			continue
		}
		t := strings.TrimRight(line, " ")
		if word == "" {
			if !strings.HasSuffix(t, " "+work.StatusDown) && !strings.HasSuffix(t, " "+work.StatusEnded) {
				return true
			}
			continue
		}
		if strings.HasSuffix(t, " "+word) {
			return true
		}
	}
	return false
}

// cursorAmongShells is which of the scratch project's shell rows the
// cursor is on, counting from nought, or below nought when it is on
// none of them. The cursor's row is the one on the raised ground from
// edge to edge, which the capture keeps as the selection's color.
func (s *scratch) cursorAmongShells() int {
	out, _ := s.srv.Run("capture-pane", "-e", "-p", "-t", tmux.SessionName+":"+tmux.HomeWindow+".0")
	raised := "48;2;" + rgbOf(theme.Conn.Dark.Border)
	// Only the scratch project's shells are counted, as shellRows
	// counts them: the machine may have a shell of its own standing
	// under another project, above these, and it is not one of the
	// shells the test opened.
	n, under := 0, false
	for _, line := range strings.Split(out, "\n") {
		plain := stripEscapes(line)
		switch {
		case strings.HasPrefix(strings.TrimSpace(plain), scratchProject+" ─"):
			under = true
			continue
		case isEyebrow(plain):
			under = false
			continue
		}
		if !under || !isShell(rowCommand(plain)) {
			continue
		}
		if strings.Contains(line[:min(len(line), 60)], raised) {
			return n
		}
		n++
	}
	return -1
}

// rgbOf is a hex color as an escape's R;G;B.
func rgbOf(hex string) string {
	var r, g, b int
	_, _ = fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &b)
	return fmt.Sprintf("%d;%d;%d", r, g, b)
}

// shellRows is how many of the scratch project's rows are shells, which
// is what a test that opened shells is asking about. It steps over a
// tree's inner rows and over whatever else the machine is running.
func (s *scratch) shellRows() int {
	n := 0
	for _, r := range s.projectRowLines() {
		if isShell(rowCommand(r)) {
			n++
		}
	}
	return n
}

// rowCommand is the first word of a panel row past the margin's marks
// and the mark of its kind.
func rowCommand(r string) string {
	for _, f := range rowFields(r) {
		if isMark(f) {
			continue
		}
		return f
	}
	return ""
}

// isShell says whether a command is one of the shells a scratch test
// might open.
func isShell(cmd string) bool {
	return cmd == "sh" || cmd == "bash" || cmd == "zsh" || cmd == "dash"
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
	out, _ := s.srv.Run("list-panes", "-a", "-F", "#{window_name}.#{pane_index}:#{pane_current_command}:#{pane_id}")
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
	out, _ := s.srv.Run("display-message", "-p", "-t", tmux.SessionName+":"+at, "#{pane_dead}")
	return strings.TrimSpace(out) == "1"
}

// parked says whether a pane is in a window of its own, out of home.
func (s *scratch) parked(id string) bool {
	for _, p := range strings.Fields(s.panes()) {
		if strings.HasSuffix(p, ":"+id) && !strings.HasPrefix(p, tmux.HomeWindow+".") {
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
// The band wears the wordmark in every panel view, so it is the panel
// itself that says: a process row's dot is the processes view's alone,
// and so is the word it says with no row at all, which is what a
// runner with nothing running under its roots shows.
func (s *scratch) inProcesses() bool {
	panel := s.panel()
	if strings.Contains(panel, "NO PROCESSES") {
		return true
	}
	for _, line := range strings.Split(panel, "\n") {
		if isRow(line) {
			return true
		}
	}
	return false
}

// statusLine is the status line as tmux expands it: what conn has put
// there for wherever its keys are.
// It is the band, and the key bar's row after it.
func (s *scratch) statusLine() string {
	out, _ := s.srv.Run("display-message", "-p", "-t", tmux.SessionName+":"+tmux.HomeWindow+".0",
		"#{T:status-left}#{T:status-right} #{T:@conn_bar}#{T:@conn_ident}")
	return strings.TrimSpace(stripStyles(out))
}

// stripStyles is a status line format without its #[...] styles.
func stripStyles(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case in && r == ']':
			in = false
		case in:
		case r == '#':
			in = true
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// display is a tmux format, of the panel.
func (s *scratch) display(format string) string {
	out, _ := s.srv.Run("display-message", "-p", "-t", tmux.SessionName+":"+tmux.HomeWindow+".0", format)
	return strings.TrimSpace(out)
}

// active is a tmux format of the window's active pane, rather than of
// the panel — which is what display asks, and cannot answer where focus
// went.
func (s *scratch) active(format string) string {
	out, _ := s.srv.Run("display-message", "-p", "-t", tmux.SessionName+":"+tmux.HomeWindow, format)
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
	at, _ := parseCursor(readCursor(s.srv.Socket + ".cursor"))
	s.t.Fatalf("waited for %s\nrail:\n%s\nbar: %s\npanes: %s\ncursor note: %+v\nbay:\n%s", what, s.panel(), s.statusLine(), s.panes(), at, strings.TrimRight(s.bay(), "\n "))
}

// conn comes up in the home window: the console across it, then on a
// key the panel at its width with the hold beside it; s opens a shell
// into the bay and its row is on the panel; a second s opens another
// and the first parks in a window of its own; enter on the first brings
// it back; c zooms the console over the window and a key gives the bay
// its side again; the panel holds its width when the window is resized.
func TestTheServerHoldsThePanelAndTheBay(t *testing.T) {
	s := startScratch(t)
	s.until("the console to finish", func() bool { return s.finished() })
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
	bayFirst, _ := s.srv.Run("display-message", "-p", "-t", tmux.SessionName+":"+tmux.HomeWindow+".1", "#{pane_id}")
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

	// The cursor goes to the shell just opened once a reading has it,
	// which is the second of the project's two shells; the machine may
	// be running anything else around them, so what is waited for is
	// the cursor on that row and not a count. k is the first of the
	// two, and enter brings it back.
	s.until("the cursor on the second shell", func() bool { return s.cursorAmongShells() == 1 })
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

	if _, err := s.srv.Run("resize-window", "-x", "200", "-y", "40"); err != nil {
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
	s.until("the console to finish", func() bool { return s.finished() })
	s.keys("Space")
	s.until("the bay to open", func() bool { return s.display("#{pane_width}") == panelW })

	s.openShell()
	s.until("a shell in the bay", func() bool { return s.shellIn("home.1") })
	first, _ := s.srv.Run("display-message", "-p", "-t", tmux.SessionName+":"+tmux.HomeWindow+".1", "#{pane_id}")
	first = strings.TrimSpace(first)

	// A second shell takes the bay and parks the first in a window of
	// its own, which is where work waits when it is not in front of you.
	s.openShell()
	s.until("the first shell parked in a window of its own", func() bool { return s.parked(first) })

	if _, err := s.srv.Run("send-keys", "-t", first, "exit", "Enter"); err != nil {
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

// s opens a shell at the project the panel is looking at and puts it
// in the bay. From inside a process it is the panel key then s, and
// the panel key cannot be driven here: send-keys writes to the pane
// and never reaches tmux's key table, and a scratch server has no
// client attached to press a key at. What the key is bound to do is
// held by the configuration; this is what happens when s lands on the
// panel.
func TestTheShellKeyOpensIntoTheBay(t *testing.T) {
	s := startScratch(t)
	s.until("the console to finish", func() bool { return s.finished() })
	s.keys("Space")
	s.until("the bay to open", func() bool { return s.display("#{pane_width}") == panelW })

	s.openShell()
	s.until("a shell in the bay", func() bool { return s.shellIn("home.1") })
	first := s.bayPane()
	// The key opens at the project the cursor stands on, and the cursor
	// does not reach the shell until the reading that has it does.
	s.until("the shell's row on the panel", func() bool { return s.projectRows() >= 1 })

	s.keys("s")
	s.until("a second shell in the bay, with the first parked", func() bool {
		return s.shellIn("home.1") && s.bayPane() != first && s.parked(first)
	})
	if n, w := s.display("#{window_panes}"), s.display("#{pane_width}"); n != "2" || w != panelW {
		t.Errorf("the key changed the window's shape: %s panes, %s wide", n, w)
	}
}

// The panel key, landing on the panel with the keys already there,
// goes to the other process: the one in the bay before the one in it
// now. With two shells open and the second in the bay, it brings the
// first back.
func TestThePanelKeyOnThePanelGoesToTheOtherProcess(t *testing.T) {
	s := startScratch(t)
	s.until("the console to finish", func() bool { return s.finished() })
	s.keys("Space")
	s.until("the bay to open", func() bool { return s.display("#{pane_width}") == panelW })

	s.openShell()
	s.until("a shell in the bay", func() bool { return s.shellIn("home.1") })
	first := s.bayPane()

	s.openShell()
	s.until("a second shell, with the first parked", func() bool {
		return s.shellIn("home.1") && s.bayPane() != first && s.parked(first)
	})
	s.until("both shells' rows on the panel", func() bool { return s.projectRows() >= 2 })

	s.keys("M--")
	s.until("the other held shell to come back into the bay", func() bool { return s.bayPane() == first })
	if n, w := s.display("#{window_panes}"), s.display("#{pane_width}"); n != "2" || w != panelW {
		t.Errorf("the key changed the window's shape: %s panes, %s wide", n, w)
	}
}

func TestADeadBayIsRevivedInPlaceNotResplit(t *testing.T) {
	s := startScratch(t)
	s.until("the console to finish", func() bool { return s.finished() })
	s.keys("Space")
	s.until("the bay to open", func() bool { return s.display("#{pane_width}") == panelW })

	s.openShell()
	s.until("a shell in the bay", func() bool { return s.shellIn("home.1") })

	// The shell has the keys once s opens it, the same as select-pane
	// gave them there; exit goes to it directly, not through the panel.
	if _, err := s.srv.Run("send-keys", "-t", tmux.SessionName+":"+tmux.HomeWindow+".1", "exit", "Enter"); err != nil {
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
	s.until("the console to finish", func() bool { return s.finished() })
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
	s.until("the kill armed", func() bool { return strings.Contains(s.statusLine(), "kill -KILL") })
	s.keys("y")

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
	s.until("the console to finish", func() bool { return s.finished() })
	s.keys("Space")
	s.until("the bay to open", func() bool { return s.display("#{pane_width}") == panelW })

	s.openShell()
	s.until("a shell in the bay", func() bool { return s.shellIn("home.1") })

	if _, err := s.srv.Run("send-keys", "-t", tmux.SessionName+":"+tmux.HomeWindow+".1", "sleep 100", "Enter"); err != nil {
		t.Fatal(err)
	}
	// One row at the scratch's own project still: the shell, saying
	// sleep 100 for what it runs, the sleep folded into it. Counting
	// there rather than anywhere on the panel, which is the whole
	// machine's and has sleeps of its own on it.
	s.until("sleep running in the bay, on the shell's row", func() bool {
		return s.projectRows() == 1 && strings.Contains(s.panel(), "sleep 100")
	})

	// The cursor stayed on the shell's own pid, and x on the shell's
	// row is x on what it runs: the question names sleep.
	s.keys("x")
	s.until("the kill armed, naming sleep", func() bool {
		return strings.Contains(s.statusLine(), "kill -TERM") && strings.Contains(s.statusLine(), " sleep?")
	})
	s.keys("y")

	s.until("sleep to end and the shell to be bare again", func() bool {
		return s.projectRows() == 1 && !strings.Contains(s.panel(), "sleep 100")
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
	s := startScratch(t)
	s.until("the console to finish", func() bool { return s.finished() })
	// The scratch server rose on dark, the ground of a terminal that
	// says nothing. tmux answers a color in its own case.
	if got := s.display("#{pane-colours[0]}"); !strings.EqualFold(got, theme.Conn.Dark.Scheme[0]) {
		t.Fatalf("the server did not rise on dark: slot 0 is %q", got)
	}

	// attach would take the terminal, which a test has none of; what is
	// under test is the ground, so the same steps run without a client.
	srv := &tmux.Server{Tmux: station.LookPath("tmux"), Socket: s.srv.Socket}
	conf := filepath.Join(filepath.Dir(srv.Socket), "tmux.conf")
	light := connOn(false).Wear()
	if err := os.WriteFile(conf, []byte(serverConfOn("C-Space", light)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := theme.WriteMode(srv.Socket, connOn(false)); err != nil {
		t.Fatal(err)
	}
	// reground paints the panel's pane on the surface of the ground
	// asked for.
	if err := srv.Reground(conf, light.Surface, "", filepath.Join(s.dir, "conn"), true); err != nil {
		t.Fatal(err)
	}

	if got := s.display("#{pane-colours[0]}"); !strings.EqualFold(got, theme.Conn.Light.Scheme[0]) {
		t.Errorf("slot 0 is %q after regrounding, not light's %q", got, theme.Conn.Light.Scheme[0])
	}
	// The panel's pane is painted on the surface of the new ground; the
	// window's style, which the bay is on, is the ground itself.
	if got := s.display("#{window-style}"); !strings.EqualFold(got, "bg="+theme.Conn.Light.Surface) {
		t.Errorf("the panel's pane is %q, not on the light surface", got)
	}
	if got, _ := s.srv.Run("show-options", "-gv", "window-style"); !strings.EqualFold(strings.TrimSpace(got), "bg="+theme.Hex(theme.Conn.Light.Ground)+",fg="+theme.Hex(theme.Conn.Light.Ink)) {
		t.Errorf("the window style is %q, not on the light ground", strings.TrimSpace(got))
	}
	if m, ok := theme.ReadModeFile(srv.Socket); !ok || m != connOn(false) {
		t.Errorf("the mode file was not put on light: %+v, found %v", m, ok)
	}
	// The panel came back, and came back conn: respawned, it comes up
	// on the console and runs it through to the end. The version it
	// says is not the thing to look for — it moves with every release,
	// and a shallow checkout with no tags has none to say — so what is
	// waited for is the page finishing.
	s.until("the panel to come back", func() bool {
		return strings.Contains(s.panes(), "home.0:conn:") && s.finished()
	})
}

// --theme on a server already up puts it in the other theme where it
// stands, the same road --light takes: the sixteen and the cursor
// change, the mode file names the theme, and the panel comes back
// painting from it.
func TestTheThemeChangesUnderAServerAlreadyUp(t *testing.T) {
	s := startScratch(t)
	s.until("the console to finish", func() bool { return s.finished() })
	if got := s.display("#{pane-colours[0]}"); !strings.EqualFold(got, theme.Conn.Dark.Scheme[0]) {
		t.Fatalf("the server did not rise in conn: slot 0 is %q", got)
	}

	srv := &tmux.Server{Tmux: station.LookPath("tmux"), Socket: s.srv.Socket}
	conf := filepath.Join(filepath.Dir(srv.Socket), "tmux.conf")
	datum := theme.Mode{Theme: "datum", Dark: true}
	if err := os.WriteFile(conf, []byte(serverConfOn("C-Space", datum.Wear())), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := theme.WriteMode(srv.Socket, datum); err != nil {
		t.Fatal(err)
	}
	// reground paints the panel's pane on the surface of the ground
	// asked for.
	if err := srv.Reground(conf, datum.Wear().Surface, "", filepath.Join(s.dir, "conn"), true); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct{ option, want string }{
		{"pane-colours[0]", theme.Datum.Dark.Scheme[0]},
		{"pane-colours[5]", theme.Datum.Dark.Scheme[5]},
		{"cursor-colour", theme.Datum.Dark.Accent},
	} {
		if got := s.display("#{" + c.option + "}"); !strings.EqualFold(got, c.want) {
			t.Errorf("%s is %q after regrounding, not datum's %q", c.option, got, c.want)
		}
	}
	if got := s.display("#{window-style}"); !strings.EqualFold(got, "bg="+theme.Datum.Dark.Surface) {
		t.Errorf("the panel's pane is %q, not on datum's surface", got)
	}
	if got, _ := s.srv.Run("show-options", "-gv", "window-style"); !strings.EqualFold(strings.TrimSpace(got), "bg="+theme.Hex(theme.Datum.Dark.Ground)+",fg="+theme.Hex(theme.Datum.Dark.Ink)) {
		t.Errorf("the window style is %q, not on datum's ground", strings.TrimSpace(got))
	}
	if m, ok := theme.ReadModeFile(srv.Socket); !ok || m != datum {
		t.Errorf("the mode file was not put in datum: %+v, found %v", m, ok)
	}
	s.until("the panel to come back", func() bool {
		return strings.Contains(s.panes(), "home.0:conn:") && s.finished()
	})
}

// ? fills the workspace with the manual and the panel with the keys:
// the half of the window where the keys are pressed says what they are
// while the half beside it says what the station is. esc puts both
// back.
func TestTheKeysStandOnThePanelWhileTheManualIsUp(t *testing.T) {
	s := startScratch(t)
	s.until("the console to finish", func() bool { return s.finished() })
	s.keys("Enter")
	s.until("the processes view", func() bool { return s.inProcesses() })

	s.keys("?")
	s.until("the manual in the workspace", func() bool {
		return strings.Contains(s.bay(), "CONN(1)")
	})
	s.until("the keys on the panel", func() bool {
		panel := s.panel()
		return strings.Contains(panel, "KEYS") && strings.Contains(panel, "IN A PROCESS")
	})
	if got := s.panel(); strings.Contains(got, scratchProject+" ─") {
		t.Errorf("the panel is still drawing the processes view:\n%s", got)
	}
	if !strings.Contains(s.statusLine(), helpWord) {
		t.Errorf("the band does not say %s: %q", helpWord, s.statusLine())
	}

	// The manual has the keys, so esc goes to the manual's own pane —
	// send-keys writes to a pane and not through tmux's key table — and
	// both halves of the window come back.
	if _, err := s.srv.Run("send-keys", "-t", tmux.SessionName+":"+tmux.HomeWindow+".1", "Escape"); err != nil {
		t.Fatal(err)
	}
	s.until("the processes view again", func() bool {
		return s.inProcesses() && !strings.Contains(s.panel(), "IN A PROCESS")
	})
}

// A theme or a ground picked in the settings dresses the server where
// it stands, and the panel keeps the keys. --theme on the way in
// respawns the panel, which a conn asking for a theme from its own
// settings cannot do: it would be killing the view the key was pressed
// in. So the sixteen change, the mode file says the new theme and
// ground, the file names both, and the pane the settings are in is the
// same pane it was.
func TestAThemePickedInTheSettingsDressesTheServer(t *testing.T) {
	s := startScratch(t)
	s.until("the console to finish", func() bool { return s.finished() })
	if got := s.display("#{pane-colours[0]}"); !strings.EqualFold(got, theme.Conn.Dark.Scheme[0]) {
		t.Fatalf("the server did not rise in conn: slot 0 is %q", got)
	}
	s.keys("Enter")
	s.until("the processes view", func() bool { return s.inProcesses() })
	was := s.paneAt(tmux.HomeWindow + ".0")

	// The settings go to the workspace and take the keys with them; the
	// panel stays the panel, and the band says where the keys have
	// gone.
	s.keys(",")
	s.until("the settings", func() bool { return strings.Contains(s.bay(), "SETTINGS") })
	if !s.inProcesses() {
		t.Errorf("the panel left the processes view for the settings:\n%s", s.panel())
	}
	if got := s.statusLine(); !strings.Contains(got, "SETTINGS") {
		t.Errorf("the band says %q", got)
	}
	// The scratch server is told its root by the environment, which
	// stands in front of the file, and the view says so where somebody
	// would otherwise edit a root and wait for a list that will not
	// change.
	if !strings.Contains(s.bay(), "CONN_ROOTS") {
		t.Errorf("the settings do not say what is in force:\n%s", s.bay())
	}
	// Down the rows to a theme that is not the one conn is wearing, and
	// take it.
	s.bayKeys("j", "j")
	s.bayKeys("Enter")
	s.until("the server to be dressed in datum", func() bool {
		return strings.EqualFold(s.display("#{pane-colours[0]}"), theme.Datum.Dark.Scheme[0])
	})
	if got := s.display("#{window-style}"); !strings.EqualFold(got, "bg="+theme.Datum.Dark.Surface) {
		t.Errorf("the panel's pane is %q, not on datum's surface", got)
	}
	if m, ok := theme.ReadModeFile(s.srv.Socket); !ok || m.Theme != "datum" {
		t.Errorf("the mode file says %+v, found %v", m, ok)
	}

	// And the ground under it, which is the other axis: the theme
	// stands while the ground changes.
	s.bayKeys("j", "j")
	s.bayKeys("Enter")
	s.until("the server to be on the light ground", func() bool {
		return strings.EqualFold(s.display("#{window-style}"), "bg="+theme.Datum.Light.Surface)
	})
	if m, ok := theme.ReadModeFile(s.srv.Socket); !ok || m.Dark || m.Theme != "datum" {
		t.Errorf("the mode file says %+v, found %v", m, ok)
	}
	if c, err := config.Read(filepath.Join(s.dir, "home")); err != nil || c.Ground != config.LightGround {
		t.Errorf("the file names the ground %q: %v", c.Ground, err)
	}
	c, err := config.Read(filepath.Join(s.dir, "home"))
	if err != nil || c.Theme != "datum" {
		t.Errorf("the file names the theme %q: %v", c.Theme, err)
	}
	// The panel was not restarted — a fresh conn there is the console —
	// and neither was the pane the keys are in, which would have put
	// the cursor back at the top of the page being worked.
	if got := s.paneAt(tmux.HomeWindow + ".0"); got != was {
		t.Errorf("the panel was %s and is now %s", was, got)
	}
	if !strings.Contains(s.bay(), "SETTINGS") {
		t.Errorf("the settings went away with the theme:\n%s", s.bay())
	}
	if got := s.active("#{pane_index}"); got != "1" {
		t.Errorf("the keys are in pane %s, not in the settings", got)
	}

	// esc puts the workspace back and the keys with it.
	s.bayKeys("Escape")
	s.until("the workspace to be filled again", func() bool {
		bay := strings.TrimSpace(s.bay())
		return !strings.Contains(s.statusLine(), "SETTINGS") &&
			!strings.Contains(bay, "SETTINGS") && !strings.Contains(bay, "dead")
	})
	if got := s.active("#{pane_index}"); got != "0" {
		t.Errorf("the keys stayed in pane %s", got)
	}
}

// conn down ends what the test brought up, and says so; a second
// conn down finds nothing.
func TestDownEndsTheScratchServer(t *testing.T) {
	s := startScratch(t)
	s.until("the console to finish", func() bool { return s.finished() })
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
// tmux itself rather than against tmux.Conf's text. conn theme reads the
// same file, and conn down clears it.
func TestAServerComesUpOnItsModeFile(t *testing.T) {
	if testing.Short() {
		t.Skip("a real tmux server is not started under -short")
	}
	tmuxBin := station.LookPath("tmux")
	if tmuxBin == "" {
		t.Skip("tmux is not installed")
	}
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // no configuration of the machine's own

	dir, err := os.MkdirTemp("/tmp", "conn-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	srv := &tmux.Server{Tmux: tmuxBin, Socket: filepath.Join(dir, "sock")}
	t.Cleanup(func() { _, _ = srv.Run("kill-server") })

	// Nothing has picked yet: a server not up comes up dark.
	if m := theme.ServerMode(srv.Socket, home); m != connOn(true) {
		t.Fatalf("a socket with no mode file is %+v, not conn's dark", m)
	}

	// A terminal that said light, on a first bring-up, leaves this
	// behind for attach to find; here it is put there by hand, the way
	// attach's own detectDark branch would.
	if err := theme.WriteMode(srv.Socket, connOn(false)); err != nil {
		t.Fatal(err)
	}

	// What attach does with a mode file already there: read it, and
	// write the server's own drawing from the ground it wears.
	m, ok := theme.ReadModeFile(srv.Socket)
	if !ok || m != connOn(false) {
		t.Fatalf("theme.ReadModeFile = (%+v, %v), want (conn light, true)", m, ok)
	}

	conf := filepath.Join(dir, "tmux.conf")
	if err := os.WriteFile(conf, []byte(serverConfOn(tmux.DefaultKey, m.Wear())), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(tmuxBin, "-S", srv.Socket, "-f", conf, "new-session", "-d",
		"-s", tmux.SessionName, "-n", tmux.HomeWindow, "sleep 30")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("starting the server: %v\n%s", out, err)
	}
	for _, c := range []struct{ option, want string }{
		{"pane-colours[0]", theme.Conn.Light.Scheme[0]},
		{"pane-colours[9]", theme.Conn.Light.Scheme[9]},
		{"cursor-colour", theme.Conn.Light.Accent},
	} {
		out, err := srv.Run("show-options", "-g", c.option)
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
	t.Setenv("CONN_SOCKET", srv.Socket)
	if _, ok := dressProgram([]string{"claude"}, claudeHome, nil); !ok {
		t.Fatal("conn theme claude was not taken")
	}
	claudeJSON, err := os.ReadFile(filepath.Join(claudeHome, ".claude", "themes", "conn.json"))
	if err != nil || !strings.Contains(string(claudeJSON), `"base": "light-ansi"`) {
		t.Errorf("conn theme claude did not read the server's mode: %v\n%s", err, claudeJSON)
	}

	// conn down clears the file it wrote, so the next server to rise
	// asks the terminal fresh instead of remembering this one's ground.
	if said, ok := takeDown(srv, home); !ok {
		t.Fatal(said)
	}
	if _, ok := theme.ReadModeFile(srv.Socket); ok {
		t.Error("conn down left the mode file behind")
	}
	if m := theme.ServerMode(srv.Socket, home); m != connOn(true) {
		t.Errorf("after conn down, the socket is %+v, not conn's dark again", m)
	}
}

// The panel opens the readout in the bay, and the processes view stays on the
// panel with the cursor still on the row the page is about — which is
// the whole point of the page being over there. The panel keeps its
// width, so the readout arriving is a swap into the bay rather than a
// window laid out afresh, and focus stays where the keys are.
func TestThePageFollowsTheCursorDownTheList(t *testing.T) {
	s := startScratch(t)
	s.until("the console to finish", func() bool { return s.finished() })
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
	// while the keys are on the panel in this view. It is up once it
	// is about a row: the page comes up before the panel has told it
	// where the cursor is, and says nothing until then.
	s.until("the readout to take the workspace", func() bool {
		return s.pageUp() && s.readoutPidOf() != "" && s.readoutPidOf() != "0"
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
	t.Logf("the page is first on %s, with the panel:\n%s", was, s.panel())
	s.keys("j")
	s.until("the page to follow the cursor down", func() bool {
		got := s.readoutPidOf()
		return got != "" && got != was
	})
	moved := s.readoutPidOf()
	t.Logf("after j the page is on %s, with the panel:\n%s", moved, s.panel())
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
	if !s.pageUp() {
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
	s.until("the console to finish", func() bool { return s.finished() })
	s.keys("Space")
	s.until("the workspace to open", func() bool { return s.display("#{pane_width}") == panelW })

	// A row of the test's own to be about, and the page comes up for it
	// without anybody asking.
	s.sleepers(1)
	s.until("the sleeper's row on the panel", func() bool { return s.projectRows() >= 1 })
	s.until("the page to take the workspace", func() bool { return s.pageUp() })

	// Going into a process puts the process there instead.
	s.openShell()
	s.until("a shell in the workspace", func() bool {
		return s.shellIn("home.1") && !s.pageUp()
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

// Cancelling a list begun with the panel key puts the operator back
// where the keys came from, and putting them back means reaching that
// pane, not selecting it.
// By the time the cancel comes the pane is not in the workspace: the
// page takes the workspace while the keys are on the panel, and what it
// displaced went to a window of its own. Selecting a pane there does
// select it — in a window the client is not looking at, which is
// nothing happening at all.
//
// The page cannot be the one to park it here, for the reason the rule
// above gives: that turn is a focus event and this server has no client
// to send one. A second shell parks the first just as well, and being
// parked is the whole of what the cancel has to deal with. The panel
// key cannot be driven either; what the binding writes down before it
// sends its key is written here in its place, and its key sent.
func TestCancellingTheListGoesBackIntoTheProcess(t *testing.T) {
	s := startScratch(t)
	s.until("the console to finish", func() bool { return s.finished() })
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

	// The panel key, as the binding fires it out of the first shell's
	// pane, and p after it.
	if _, err := s.srv.Run("set-option", "-g", "@conn_from", first); err != nil {
		t.Fatal(err)
	}
	s.keys("M--")
	s.keys("p")
	s.until("the list", func() bool { return strings.Contains(s.panel(), "PROJECTS") })

	// esc brings the view back and the shell with it: out of its own
	// window, into the workspace, with the keys in it.
	s.keys("Escape")
	s.until("the shell back in the workspace", func() bool { return s.bayPane() == first })
	if s.parked(first) {
		t.Error("the shell stayed in a window of its own")
	}
	if got := s.active("#{pane_id}"); got != first {
		t.Errorf("the keys are in %s, not the shell the keys came from", got)
	}
	if n, w := s.display("#{window_panes}"), s.display("#{pane_width}"); n != "2" || w != panelW {
		t.Errorf("the cancel changed the window's shape: %s panes, %s wide", n, w)
	}

	// p on the panel, with no arrival before it, leaves nothing to go
	// back to: the cancel is the view alone, and nothing is put anywhere.
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
// the bay to be read; x closes that pane, whereupon the row is down
// again; and x on the one still running ends it and closes its pane in
// one move.
func TestUBringsUpWhatTheProjectDeclares(t *testing.T) {
	s := startScratch(t)
	repo := filepath.Join(s.dir, "home", "repo")
	// sleeper answers ctrl-c slowly, the way a docker compose up does
	// while it stops its services: six seconds, past the five a first
	// cut of the close gave before leaving the pane standing.
	if err := os.WriteFile(filepath.Join(repo, work.DeclaredName), []byte("sleeper: perl -e '$SIG{INT} = sub { sleep 6; exit 0 }; sleep 120'\nquick: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.until("the console to finish", func() bool { return s.finished() })
	s.keys("Space")
	s.until("the bay to open", func() bool { return s.display("#{pane_width}") == panelW })
	// The down rows show beside work in the project, and nothing is
	// working in it yet: a shell there is the work.
	s.openShell()
	s.until("a shell in the bay", func() bool { return s.shellIn("home.1") })

	// A declared row goes by its name on the panel, which is where these
	// rows are read.
	s.until("the two down rows", func() bool { return s.rowSays("sleeper", work.StatusDown) && s.rowSays("quick", work.StatusDown) })

	// marked is the id of the pane carrying a declaration's mark, and
	// what it recorded of its end.
	marked := func(name string) (id, exit string) {
		out, _ := s.srv.Run("list-panes", "-a", "-F", "#{pane_id} #{@conn_declared} #{@conn_exit}")
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
	s.keys("U")
	s.until("both panes opened and marked, quick's end recorded", func() bool {
		sleeper, _ := marked("sleeper")
		quick, exit := marked("quick")
		return sleeper != "" && quick != "" && exit == "0"
	})
	s.until("sleeper ACTIVE and quick ENDED on the panel", func() bool {
		return s.rowSays("sleeper", "") && s.rowSays("quick", work.StatusEnded)
	})
	// The panes were parked: the bay still holds the shell it held.
	if !s.shellIn("home.1") {
		t.Error("U changed what the bay holds")
	}

	// quick ended at once and is not running, which is the last group,
	// and the scratch project sorts last within it: G is quick's row,
	// whatever else the machine has down. Enter there is into its pane.
	quick, _ := marked("quick")
	s.keys("G")
	s.keys("Enter")
	s.until("quick's pane in the bay", func() bool { return s.bayPane() == quick })

	s.keys("x")
	s.until("the close armed, naming quick", func() bool {
		return strings.Contains(s.statusLine(), "kill-pane") && strings.Contains(s.statusLine(), " quick?")
	})
	s.keys("y")
	s.until("the pane gone and quick down again", func() bool {
		id, _ := marked("quick")
		return id == "" && s.rowSays("quick", work.StatusDown) && s.rowSays("sleeper", "")
	})

	// sleeper is still running. x on its head row asks for ctrl-c in
	// its pane, and y does that and takes the pane down with it once
	// the end is recorded, six seconds on: the row is DOWN in the one
	// move, with no ENDED to close.
	// G is quick's down row. sleeper's head is above it, past the
	// perl where the fold has left that on the panel: each row up is
	// asked, and the question that names sleeper is the one answered;
	// the rest are withdrawn.
	s.keys("G")
	armed := func() bool { return strings.Contains(s.statusLine(), "any other key") }
	for i := 0; i < 24; i++ {
		s.keys("k")
		s.keys("x")
		asked := false
		for deadline := time.Now().Add(time.Second); time.Now().Before(deadline) && !asked; {
			asked = armed()
			time.Sleep(50 * time.Millisecond)
		}
		if !asked {
			continue
		}
		if strings.Contains(s.statusLine(), "send-keys") && strings.Contains(s.statusLine(), " sleeper?") {
			break
		}
		s.keys("n")
		s.until("the question withdrawn", func() bool { return !armed() })
	}
	if !strings.Contains(s.statusLine(), " sleeper?") {
		t.Fatalf("sleeper's row not found above quick's: %s", s.statusLine())
	}
	s.keys("y")
	// The answer takes six seconds and the helper's patience is eight;
	// the first stretch is waited out here so the wait after it is not
	// a near thing on a slow runner.
	time.Sleep(3 * time.Second)
	s.until("sleeper's pane gone and its row down", func() bool {
		id, _ := marked("sleeper")
		return id == "" && s.rowSays("sleeper", work.StatusDown) && s.rowSays("quick", work.StatusDown)
	})
}

// A reground starts the panel and the page in the workspace again from
// the conn that asked, not from the command each pane first rose on,
// and the page as the page it is: its command is the one its marks
// say, the readout's for the readout and a hold's for a hold. Which of
// the two stands depends on whether the cursor has a row to read, and
// the panel may put the readout up at any reading, so the page is read
// with its marks in one go rather than against what stood before. The scratch's
// binary is moved out from under the server, the way Go's cache trim
// takes a go run's, and the reground is handed the binary where it now
// is: both panes come up from there, alive.
func TestARegroundStartsThePanesFromTheConnAsking(t *testing.T) {
	s := startScratch(t)
	s.until("the console to finish", func() bool { return s.finished() })
	s.keys("Space")
	s.until("the bay to open", func() bool {
		return s.display("#{pane_width}") == panelW && strings.Contains(s.panes(), "home.1:conn:")
	})
	moved := filepath.Join(s.dir, "conn-rebuilt")
	if err := os.Rename(filepath.Join(s.dir, "conn"), moved); err != nil {
		t.Fatal(err)
	}
	if err := s.srv.Reground(filepath.Join(s.dir, "tmux.conf"), theme.Conn.Dark.Surface, "", moved, true); err != nil {
		t.Fatal(err)
	}
	s.until("the panel up again from the moved binary", func() bool {
		return s.display("#{pane_dead}") == "0" && strings.Contains(s.display("#{pane_start_command}"), moved) && s.finished()
	})
	// tmux answers the command in quotes of its own.
	out, _ := s.srv.Run("display-message", "-p", "-t", tmux.SessionName+":"+tmux.HomeWindow+".1", "#{@conn_readout}|#{pane_dead} #{pane_start_command}")
	readout, page, _ := strings.Cut(strings.TrimSpace(out), "|")
	want := " hold"
	if readout == "1" {
		want = " readout"
	}
	if page = strings.Trim(page, `"`); !strings.HasPrefix(page, "0 ") || !strings.Contains(page, moved) || !strings.HasSuffix(page, want) {
		t.Errorf("the page did not come up again from the moved binary as%s: %q", want, page)
	}
}

// A conn of another build reaching the server relieves it: the panel
// comes up again from the new binary, straight onto the processes view
// with no console between, and the work goes on — the shell in the
// workspace in the same process, with the keys still in it, and the
// process in a window of its own untouched. The panel of another build
// is had by saying the server is on one; building a second conn to
// differ would test the linker.
func TestAConnOfAnotherBuildRelievesTheServer(t *testing.T) {
	s := startScratch(t)
	s.until("the console to finish", func() bool { return s.finished() })
	s.keys("Space")
	s.until("the bay to open", func() bool {
		return s.display("#{pane_width}") == panelW && strings.Contains(s.panes(), "home.1:conn:")
	})
	s.until("the panel to name its build", func() bool { return s.srv.Build() != "" })
	s.openShell()
	s.until("a shell in the bay with the keys", func() bool { return s.shellIn("home.1") && s.active("#{pane_index}") == "1" })
	shell := s.bayPane()
	repo := filepath.Join(s.dir, "home", "repo")
	work, err := s.srv.Run("new-window", "-d", "-P", "-F", "#{pane_pid}", "-c", repo, "exec sleep 600")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.srv.SayBuild("another"); err != nil {
		t.Fatal(err)
	}
	bin, conf := filepath.Join(s.dir, "conn"), filepath.Join(s.dir, "tmux.conf")
	relieved, err := s.srv.Relieve(conf, bin)
	if err != nil || !relieved {
		t.Fatalf("Relieve = %v, %v; want the station relieved", relieved, err)
	}
	print, _ := tmux.Fingerprint(bin)
	s.until("the panel up again on this build, on the processes view", func() bool {
		return s.srv.Build() == print && strings.Contains(s.panel(), scratchProject)
	})
	if s.finished() {
		t.Error("the relieved panel came up on the console")
	}
	if s.srv.Resuming() {
		t.Error("the panel left the word to resume standing for the next one")
	}
	if got := s.display("#{window_zoomed_flag}"); got != "0" {
		t.Error("the relieved panel took the whole window, as the console does")
	}
	pids, _ := s.srv.Run("list-panes", "-a", "-F", "#{pane_pid}")
	if !strings.Contains(" "+strings.Join(strings.Fields(pids), " ")+" ", " "+strings.TrimSpace(work)+" ") {
		t.Errorf("the work was not left running: pane pid %s gone from %q", strings.TrimSpace(work), pids)
	}
	if s.bayPane() != shell || s.active("#{pane_index}") != "1" {
		t.Errorf("the shell lost the workspace or the keys: bay %s, keys in pane %s", s.bayPane(), s.active("#{pane_index}"))
	}
	if relieved, err := s.srv.Relieve(conf, bin); err != nil || relieved {
		t.Errorf("Relieve on a station already on the build = %v, %v; want nothing done", relieved, err)
	}
}

// A conn attaching to a server whose panel has died starts the panel
// again from its own binary. The home window stays once the bay has
// opened, with the dead pane in it, and the attach would otherwise
// land on a panel saying its file was not found.
func TestAnAttachStartsADeadPanelAgain(t *testing.T) {
	s := startScratch(t)
	s.until("the console to finish", func() bool { return s.finished() })
	s.keys("Space")
	s.until("the bay to open", func() bool {
		return s.display("#{pane_width}") == panelW && strings.Contains(s.panes(), "home.1:conn:")
	})
	gone := filepath.Join(s.dir, "gone")
	if _, err := s.srv.Run("respawn-pane", "-k", "-t", tmux.SessionName+":"+tmux.HomeWindow+".0", "exec "+tmux.ShellQuote(gone)); err != nil {
		t.Fatal(err)
	}
	s.until("the panel to have died", func() bool { return s.display("#{pane_dead}") == "1" })
	if err := s.srv.RestoreHome(filepath.Join(s.dir, "home"), filepath.Join(s.dir, "conn")); err != nil {
		t.Fatal(err)
	}
	s.until("the panel up again", func() bool {
		return s.display("#{pane_dead}") == "0" && strings.Contains(s.panes(), "home.0:conn:") && s.finished()
	})
	if got := s.display("#{window_panes}"); got != "2" {
		t.Errorf("home has %s panes after the panel came back, not the panel and the bay", got)
	}
}
