package main

import (
	"os"
	"strings"
	"testing"
)

// The manual conn shows is the one it was built with. A conn run out of
// a build directory has no installed page, and an installed conn may
// have an older page beside a newer binary; the one in the binary is
// the only one certain to describe the conn showing it.
func TestConnCarriesItsOwnManual(t *testing.T) {
	onDisk, err := os.ReadFile("man/conn.1")
	if err != nil {
		t.Skip(err)
	}
	if string(manPage) != string(onDisk) {
		t.Error("the manual in the binary is not the page in the tree")
	}
	if !strings.Contains(string(manPage), ".SH KEYS") {
		t.Error("the manual carries no keys section")
	}
}

// conn writes the page beside its own state and points man at it.
func TestTheManualIsWrittenWhereManCanReadIt(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	home := t.TempDir()
	path, err := writeManPage(home)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != string(manPage) {
		t.Fatalf("the page conn wrote: %v", err)
	}
	if cmd := manCommand(path); !strings.HasPrefix(cmd, "man ") || !strings.Contains(cmd, path) {
		t.Errorf("conn runs %q", cmd)
	}
}

// While the manual is in the workspace the panel says HELP, and no row
// is under the cursor: the manual is not a process, so there is no row
// these keys are about.
func TestTheManualPutsThePanelInHelp(t *testing.T) {
	m := model{view: viewProcesses, cursor: 4321, inside: true}
	if got := m.keys(); !strings.Contains(got, "PROCS") {
		t.Fatalf("a panel with no manual up says %q", got)
	}
	next, _ := m.Update(helpMsg{on: true})
	m = next.(model)
	if !m.helping {
		t.Error("conn does not know the manual is up")
	}
	if m.cursor != 0 {
		t.Errorf("a row is still under the cursor: %d", m.cursor)
	}
	if got := m.keys(); !strings.Contains(got, "HELP") || strings.Contains(got, "PROCS") {
		t.Errorf("the panel says %q", got)
	}
	// The word is the view's again once the manual is out of the
	// workspace, which the reading is what says.
	next, _ = m.Update(processesMsg{gen: m.processesGen})
	if got := next.(model).keys(); !strings.Contains(got, "PROCS") {
		t.Errorf("with the manual gone the panel says %q", got)
	}
}

// Off the processes view the word is the view's own: the manual may be
// standing in the workspace while the operator is working the list, and
// there the keys are the list's.
func TestHelpIsThePanelsWordOnlyInTheProcessesView(t *testing.T) {
	m := model{view: viewProjects, helping: true, inside: true}
	if got := m.keys(); !strings.Contains(got, "PROJECTS") {
		t.Errorf("the list says %q while the manual is up", got)
	}
}

// The chord that opens the manual puts it away again. The manual is
// conn's own furniture and the keys step over furniture, so nothing
// else can reach it: a manual that opened and would not close would be
// a trap rather than a help.
func TestTheChordPutsTheManualAwayAgain(t *testing.T) {
	m := model{view: viewProcesses, inside: true, srv: &server{}, cursor: 77, helping: true}
	next, cmd := m.key("alt+?")
	m = next.(model)
	if m.helping {
		t.Error("the manual is still up after the chord that closes it")
	}
	if cmd == nil {
		t.Error("nothing was done to put it away")
	}
	if got := m.keys(); !strings.Contains(got, "PROCS") {
		t.Errorf("the panel still says %q", got)
	}
}

// While the manual is up the reading does not hand a row back. follow
// keeps hold of the row the operator was on as the rows change under
// it, and with the manual up there is no such row — without this the
// cursor came back on the next beat, a couple of seconds later.
func TestTheReadingLeavesTheCursorAloneWhileHelping(t *testing.T) {
	projects := []project{{path: "/w", entries: []entry{{pid: 11, tty: "ttys001"}, {pid: 22, tty: "ttys002"}}}}
	m := model{view: viewProcesses, inside: true, cursor: 0, projects: projects}
	// A reading that finds the manual in the workspace: conn is helping,
	// and the cursor it was told to let go of stays let go.
	up := processesMsg{projects: projects, gen: m.processesGen, bayHelp: true}
	next, _ := m.Update(up)
	if got := next.(model); got.cursor != 0 || !got.helping {
		t.Errorf("the reading put the cursor back on %d (helping %v)", got.cursor, got.helping)
	}
	// And with the manual gone it follows as it always did.
	next, _ = m.Update(processesMsg{projects: projects, gen: m.processesGen})
	if got := next.(model); got.cursor == 0 || got.helping {
		t.Errorf("with no manual up the reading left the cursor at %d (helping %v)", got.cursor, got.helping)
	}
}
