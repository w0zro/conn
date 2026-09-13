package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// lookSubj is a waiting AI with everything the page has to say about
// one: a long command, a status it has stood in for a while, a
// directory of its own under its place, a pane conn holds, what it runs
// and what runs it, a conversation, and a place with a git standing.
func readoutSubj() readoutSubject {
	e := entry{
		pid: 49212, kind: kindContact,
		command: "claude --resume d81d7536-e545-4881-8daa-f1d291a03be1",
		tty:     "ttys003", started: processesNow.Add(-92 * time.Minute),
		status: statusWaiting, since: processesNow.Add(-7 * time.Minute),
		cwd: "/Users/w0zro/projects/w0zro/conn/tools", asking: "input needed",
		depth: 1,
	}
	return readoutSubject{
		entry: e,
		proc: process{pid: e.pid, state: 'S', foreground: true,
			started: e.started, cpu: 2*time.Minute + 14*time.Second},
		project: project{path: "/Users/w0zro/projects/w0zro/conn"},
		parent: entry{pid: 49200, kind: kindShell, command: "zsh",
			tty: "ttys003", status: statusActive},
		children: []entry{{pid: 49300, kind: kindRun, command: "caffeinate -i -t 300",
			tty: "ttys003", status: statusActive, depth: 2}},
		pane:   pane{id: "%2"},
		inside: true,
		sess: sessionFile{SessionID: "d81d7536-e545-4881-8daa-f1d291a03be1",
			Name: "conn-2d", Version: "2.1.267", Kind: "interactive"},
		carried: session{Branch: "main", Prompt: "i want the info to use the pane on the right",
			Ask: ask{Tool: "AskUserQuestion", Detail: "Is this session's lamp lit on the bar while this question waits?"}},
		git: gitStatus{repo: true, branch: "main", dirty: 3,
			commit: "263cf91", subject: "The look: what conn knows of a row",
			when: processesNow.Add(-3 * time.Hour), upstream: "origin/main", ahead: 142},
	}
}

// The look at both widths is the file of record.
func TestTheReadoutIsWhatItWas(t *testing.T) {
	b := composeReadout(readoutSubj(), "/Users/w0zro", processesNow)
	golden(t, "readout-120x40.txt", texts(drawReadout(b, 120, 40, plain)))
	golden(t, "readout-narrow-60x40.txt", texts(drawReadout(b, 60, 40, plain)))
}

// The page says the things the watch's columns have no room for, and
// says them whole.
func TestTheReadoutSaysWhatTheRowCannot(t *testing.T) {
	// Tall enough for the whole page: the file of record above shows
	// the cut at forty rows, and this reads what is said, not where the
	// pane ends.
	text := texts(drawReadout(composeReadout(readoutSubj(), "/Users/w0zro", processesNow), 120, 48, plain))

	// The command as it was written, not in conn's own upper case: it is
	// a thing somebody might retype.
	if !strings.Contains(text, "claude --resume d81d7536-e545-4881-8daa-f1d291a03be1") {
		t.Errorf("the whole command is not on the page:\n%s", text)
	}
	// The ask is the reason to open the page on a waiting row at all, so
	// it comes before what the row is, where a cut page cannot lose it.
	if !strings.Contains(text, "INPUT NEEDED") {
		t.Errorf("what the AI is stopped on is not on the page:\n%s", text)
	}
	if strings.Index(text, "WAITING") > strings.Index(text, "WHAT") {
		t.Errorf("the ask is not the first thing on the page:\n%s", text)
	}
	// And it is said once. The group above carries how long, so the
	// status does not carry it too: on a dense page a thing said twice
	// reads as two things.
	if n := strings.Count(text, "7M 00S"); n != 1 {
		t.Errorf("how long it has waited is on the page %d times:\n%s", n, text)
	}
	for what, want := range map[string]string{
		"how long it has waited":   "FOR ....... 7M 00S",
		"what it is asking, whole": "AskUserQuestion · Is this session's lamp lit on the bar while this question waits?",
		"its own directory":        "~/projects/w0zro/conn/tools",
		"what the table says":      "SLEEPING · HAS THE TERMINAL",
		"what it has spent":        "2M 14S SPENT",
		"which conversation":       "d81d7536-e545-4881-8daa-f1d291a03be1",
		"what it goes by":          "CONN-2D",
		"the last thing it asked":  "i want the info to use the pane on the right",
		"the branch and the tree":  "MAIN · 3 CHANGED",
		"the commit":               "263cf91",
		"what it is tracking":      "ORIGIN/MAIN · 142 AHEAD",
		"what runs it":             "SHELL zsh · 49200",
		"what it runs":             "RUN caffeinate -i -t 300 · ACTIVE",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("%s (%q) is not on the page:\n%s", what, want, text)
		}
	}
}

// A row with nothing more to say says nothing more: a shell has no ask
// and no conversation, so it gets neither group, and a heading over
// nothing is not drawn. A process sitting in its tree's own place is
// not told it is there twice.
func TestTheReadoutLeavesOutWhatThereIsNoneOf(t *testing.T) {
	s := readoutSubject{
		entry: entry{pid: 88, kind: kindShell, command: "zsh", tty: "ttys009",
			started: processesNow.Add(-time.Hour), status: statusIdle,
			cwd: "/Users/w0zro/projects/w0zro/conn"},
		proc:    process{pid: 88, state: 'S'},
		project: project{path: "/Users/w0zro/projects/w0zro/conn"},
		inside:  true,
	}
	text := texts(drawReadout(composeReadout(s, "/Users/w0zro", processesNow), 120, 40, plain))

	for _, gone := range []string{"WAITING", "ASKS", "SAID", "CONTACT", "PROJECT\n", "TREE", "CWD", "CPU"} {
		if strings.Contains(text, gone) {
			t.Errorf("%q is on a page that has nothing to put under it:\n%s", gone, text)
		}
	}
	// A terminal conn did not open says so, since inside the server that
	// is a fact about the row rather than about conn.
	if !strings.Contains(text, "CONN DID NOT OPEN IT") {
		t.Errorf("a terminal conn does not hold is not said:\n%s", text)
	}
	// A status with no moment behind it gets no clause rather than a
	// made-up one.
	if strings.Contains(text, "IDLE · FOR") {
		t.Errorf("a status with no moment was dated anyway:\n%s", text)
	}
}

// Outside the server conn holds no panes at all, so saying a row is in
// none of them says nothing about the row.
func TestTheReadoutSaysNothingOfPanesOutsideTheServer(t *testing.T) {
	s := readoutSubj()
	s.inside, s.pane = false, pane{}
	text := texts(drawReadout(composeReadout(s, "/Users/w0zro", processesNow), 120, 40, plain))
	if strings.Contains(text, "PANE") {
		t.Errorf("outside the server the page still spoke of panes:\n%s", text)
	}
}

// A stopped process is not dated from an AI's own clock: the moment
// conn holds is when the AI last changed what it says of itself,
// which is not when anything stopped it.
func TestTheReadoutDoesNotDateAFaultFromTheContactsClock(t *testing.T) {
	s := readoutSubj()
	s.entry.status, s.entry.fault, s.entry.asking = statusStopped, true, ""
	text := texts(drawReadout(composeReadout(s, "/Users/w0zro", processesNow), 120, 40, plain))
	if strings.Contains(text, "STOPPED · FOR") {
		t.Errorf("a stopped row was dated from the AI's clock:\n%s", text)
	}
}

// A branch with nothing to track is not behind by nothing — there is
// nothing for it to be behind — so it says neither.
func TestTheReadoutSaysNothingOfTrackingWithNoUpstream(t *testing.T) {
	s := readoutSubj()
	s.git.upstream, s.git.ahead, s.git.dirty = "", 0, 0
	text := texts(drawReadout(composeReadout(s, "/Users/w0zro", processesNow), 120, 40, plain))
	if strings.Contains(text, "TRACKING") {
		t.Errorf("a branch with no upstream was given one:\n%s", text)
	}
	if !strings.Contains(text, "MAIN · CLEAN") {
		t.Errorf("a clean tree does not say so:\n%s", text)
	}
}

// subjectOf reads the line of descent off the tree the watch wrote: the
// nearest row above at a shallower depth runs this one, and the rows
// below it one level deeper are what it runs. A grandchild is not a
// child, and the next row at the same depth is a sibling, not kin.
func TestSubjectOfReadsTheLineOfDescent(t *testing.T) {
	pl := project{path: "/w", entries: []entry{
		{pid: 1, kind: kindShell, command: "zsh", depth: 0},
		{pid: 2, kind: kindContact, command: "claude", depth: 1},
		{pid: 3, kind: kindRun, command: "go test", depth: 2},
		{pid: 4, kind: kindRun, command: "compile", depth: 3}, // a grandchild
		{pid: 5, kind: kindRun, command: "caffeinate", depth: 2},
		{pid: 6, kind: kindShell, command: "zsh", depth: 0}, // another tree
	}}
	s, ok := subjectOf(2, []project{pl}, nil)
	if !ok {
		t.Fatal("pid 2 was not found")
	}
	if s.parent.pid != 1 {
		t.Errorf("claude runs under pid %d, want 1", s.parent.pid)
	}
	var kids []int
	for _, k := range s.children {
		kids = append(kids, k.pid)
	}
	if len(kids) != 2 || kids[0] != 3 || kids[1] != 5 {
		t.Errorf("claude runs %v, want [3 5] — the grandchild is not a child", kids)
	}
	// A root has nothing above it, and the next tree is not its child.
	s, _ = subjectOf(6, []project{pl}, nil)
	if s.parent.pid != 0 || len(s.children) != 0 {
		t.Errorf("a bare root stands under %d with %d children", s.parent.pid, len(s.children))
	}
}

// i asks the server to put the page in the slot; it does not take the
// watch's own pane, which is the whole point of it being over there.
func TestIPutsTheReadoutInTheBay(t *testing.T) {
	m := newModel(plain)
	m.view, m.inside, m.now = viewProcesses, true, processesNow
	m.srv = &server{tmux: "/nonexistent/tmux", socket: "/tmp/none"}
	m.projects = []project{{path: "/w", entries: []entry{{pid: 49212, tty: "ttys003"}}}}
	m.cursor, m.cursorAt = 49212, 0

	next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "i"}))
	m = next.(model)
	if cmd == nil {
		t.Fatal("i asked the server for nothing")
	}
	if m.view != viewProcesses {
		t.Errorf("i took the watch's own pane: view %d", m.view)
	}
	// The rail keeps drawing the watch while the page is in the slot.
	if !strings.Contains(m.View().Content, "STATUS") {
		t.Errorf("the watch is not still on the rail:\n%s", m.View().Content)
	}
	// Against a server that is not there, the page does not come up.
	if _, ok := answered(cmd).(readoutMsg); ok {
		t.Error("the look came up with no tmux to put it up with")
	}
}

// With nothing under the cursor, or nowhere to put a page, i opens
// nothing.
func TestIOpensNothingAboutNothing(t *testing.T) {
	m := newModel(plain)
	m.view, m.inside = viewProcesses, true
	next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "i"}))
	m = next.(model)
	if cmd != nil || m.looking {
		t.Errorf("i on an empty watch: cmd %v, looking %v", cmd != nil, m.looking)
	}

	m = newModel(plain)
	m.view, m.inside = viewProcesses, false
	m.projects = []project{{path: "/w", entries: []entry{{pid: 7, tty: "ttys001"}}}}
	m.cursor = 7
	next, cmd = m.Update(tea.KeyPressMsg(tea.Key{Text: "i"}))
	m = next.(model)
	if cmd != nil || m.looking {
		t.Errorf("i outside the server: cmd %v, looking %v", cmd != nil, m.looking)
	}
}

// A row that ends while its page is up says so rather than going blank
// or holding the last thing it read.
func TestTheReadoutSaysWhenItsRowIsGone(t *testing.T) {
	text := texts(drawReadout(readoutReport{pid: 49212, gone: true}, 120, 40, plain))
	if !strings.Contains(text, "NO LONGER LISTED") {
		t.Errorf("a page whose row went says:\n%s", text)
	}
	if !strings.Contains(text, "PID 49212") {
		t.Errorf("it still says which row it was about:\n%s", text)
	}
}

// Every label fits inside the leader's field. One that does not leaves
// a single dot and starts its value a column past every other value on
// the page, which is the one thing a column of facts must not do.
func TestEveryReadoutLabelFitsTheLeader(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range []readoutSubject{readoutSubj(), {entry: entry{pid: 1, kind: kindShell}, inside: true}} {
		for _, g := range composeReadout(s, "/Users/w0zro", processesNow).groups {
			for _, f := range g.facts {
				seen[f.label] = true
				if len(f.label) > labelW {
					t.Errorf("the label %q is %d wide, past the leader's %d", f.label, len(f.label), labelW)
				}
			}
		}
	}
	if len(seen) < 10 {
		t.Errorf("only %d labels were seen; the check is not reaching the page", len(seen))
	}
	// And the values do line up, which is what the width is for.
	text := texts(drawReadout(composeReadout(readoutSubj(), "/Users/w0zro", processesNow), 120, 40, plain))
	col := -1
	for _, line := range strings.Split(text, "\n") {
		i := strings.Index(line, ". ")
		if !strings.Contains(line, " ... ") || i < 0 {
			continue
		}
		if col == -1 {
			col = i
		} else if i != col {
			t.Errorf("a value starts at column %d where the rest start at %d: %q", i, col, line)
		}
	}
}

// i is a toggle: the key for the page is the key a reader reaches for
// to be rid of it. conn knows which way it goes without asking tmux,
// since it put the page there itself, and a reading corrects it.
func TestIIsAToggle(t *testing.T) {
	m := newModel(plain)
	m.view, m.inside, m.now = viewProcesses, true, processesNow
	m.srv = &server{tmux: "/nonexistent/tmux", socket: "/tmp/none"}
	m.projects = []project{{path: "/w", entries: []entry{{pid: 49212, tty: "ttys003"}}}}
	m.cursor, m.cursorAt = 49212, 0

	// A tmux that answers for a home with a slot in it and does nothing
	// else, so the command i built can be run for the answer it gives.
	// Which way i went is read off that answer: both ways ask the
	// server for something, so a test that only checked that one did
	// would pass whichever way it went.
	t.Setenv("TMUX_PANE", "%0")
	stub := filepath.Join(t.TempDir(), "tmux")
	script := "#!/bin/sh\nshift 2\ncase \"$1\" in\n" +
		"list-panes) printf '%%0\\t/dev/ttys001\\t44\\t40\\t\\t\\t\\n%%9\\t/dev/ttys009\\t80\\t40\\t1\\t\\t1\\n' ;;\n" +
		"new-window|split-window) printf '%%9\\n' ;;\nesac\nexit 0\n"
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	m.srv = &server{tmux: stub, socket: "/tmp/none"}
	asked := func(cmd tea.Cmd) tea.Msg {
		t.Helper()
		if cmd == nil {
			t.Fatal("i asked the server for nothing")
		}
		return answered(cmd)
	}
	press := func() tea.Cmd {
		next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "i"}))
		m = next.(model)
		return cmd
	}
	// With no page up, i opens one, and conn knows it once tmux has
	// done it rather than guessing ahead of the answer.
	if got := asked(press()); got != (readoutMsg{on: true}) {
		t.Errorf("i with no page up answered %+v, not a page going up", got)
	}
	if m.looking {
		t.Error("conn called the page up before the server had put it there")
	}
	next, cmd := m.Update(readoutMsg{on: true})
	m = next.(model)
	if !m.looking || cmd == nil {
		t.Errorf("the page going up left looking %v and did not read again", m.looking)
	}

	// With one up, i takes it down.
	if got := asked(press()); got != (readoutMsg{on: false}) {
		t.Errorf("i with a page up answered %+v, not a page coming down", got)
	}
	next, _ = m.Update(readoutMsg{on: false})
	m = next.(model)
	if m.looking {
		t.Error("the page coming down left conn thinking it was still up")
	}

	// A reading is the truth, whatever conn thought.
	next, _ = m.Update(processesMsg{gen: m.processesGen, projects: m.projects, bayReadout: true})
	m = next.(model)
	if !m.looking {
		t.Error("a reading that found the page in the slot was not believed")
	}
	// And a real pane taking the slot is not the page.
	next, _ = m.Update(reachedMsg{"ttys003"})
	if next.(model).looking {
		t.Error("a process reaching the slot left conn thinking the page was there")
	}
}

// Closing wants no row under the cursor. The page is up whatever the
// cursor is on, and refusing to close it because the watch has emptied
// would leave it stuck there.
func TestIClosesThePageWithNothingUnderTheCursor(t *testing.T) {
	m := newModel(plain)
	m.view, m.inside, m.looking = viewProcesses, true, true
	next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "i"}))
	m = next.(model)
	if cmd == nil {
		t.Error("i on an empty watch with a page up did not close it")
	}
}

// Closing the page on a row conn holds goes to the row. The page is a
// reading of that row and the row is right there in a pane — read about
// it, then be in it — and an empty slot is a worse answer than the
// thing the page was about.
func TestIClosesOntoTheProcessItCanReach(t *testing.T) {
	m := newModel(plain)
	m.view, m.inside, m.looking, m.now = viewProcesses, true, true, processesNow
	m.projects = []project{{path: "/w", entries: []entry{
		{pid: 49212, tty: "ttys003"},
		{pid: 49213, tty: "ttys004"},
	}}}
	m.cursor, m.cursorAt = 49212, 0
	m.panes = map[string]pane{"ttys003": {id: "%7", tty: "ttys003"}}

	t.Setenv("TMUX_PANE", "%0")
	stub := filepath.Join(t.TempDir(), "tmux")
	script := "#!/bin/sh\nshift 2\ncase \"$1\" in\n" +
		"list-panes) printf '%%0\\t/dev/ttys001\\t44\\t40\\t\\t\\t\\n%%9\\t/dev/ttys009\\t80\\t40\\t1\\t\\t1\\n' ;;\n" +
		"new-window|split-window) printf '%%9\\n' ;;\nesac\nexit 0\n"
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	m.srv = &server{tmux: stub, socket: "/tmp/none"}

	press := func() tea.Msg {
		t.Helper()
		next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "i"}))
		m = next.(model)
		if cmd == nil {
			t.Fatal("i asked the server for nothing")
		}
		return answered(cmd)
	}
	if got := press(); got != (reachedMsg{"ttys003"}) {
		t.Errorf("i on a page about a row conn holds answered %+v, not the row", got)
	}

	// A row conn only reports has nothing to go to, and the page comes
	// down to the empty slot as before.
	m.looking = true
	m.cursor, m.cursorAt = 49213, 1
	if got := press(); got != (readoutMsg{on: false}) {
		t.Errorf("i on a page about a row conn cannot reach answered %+v", got)
	}
}
