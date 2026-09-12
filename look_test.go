package main

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// lookSubj is a waiting agent with everything the page has to say about
// one: a long command, a status it has stood in for a while, a
// directory of its own under its place, a pane conn holds, what it runs
// and what runs it, a conversation, and a place with a git standing.
func lookSubj() lookSubject {
	e := entry{
		pid: 49212, kind: kindAgent,
		command: "claude --resume d81d7536-e545-4881-8daa-f1d291a03be1",
		tty:     "ttys003", started: watchNow.Add(-92 * time.Minute),
		status: statusWaiting, since: watchNow.Add(-7 * time.Minute),
		cwd: "/Users/w0zro/projects/w0zro/conn/tools", asking: "input needed",
		depth: 1,
	}
	return lookSubject{
		entry: e,
		proc: process{pid: e.pid, state: 'S', foreground: true,
			started: e.started, cpu: 2*time.Minute + 14*time.Second},
		place: place{path: "/Users/w0zro/projects/w0zro/conn"},
		parent: entry{pid: 49200, kind: kindShell, command: "zsh",
			tty: "ttys003", status: statusActive},
		children: []entry{{pid: 49300, kind: kindRun, command: "caffeinate -i -t 300",
			tty: "ttys003", status: statusActive, depth: 2}},
		pane:   pane{id: "%2"},
		inside: true,
		sess: sessionFile{SessionID: "d81d7536-e545-4881-8daa-f1d291a03be1",
			Name: "conn-2d", Version: "2.1.267", Kind: "interactive"},
		convo: conversation{Branch: "main", Prompt: "i want the info to use the pane on the right"},
		git: gitStanding{repo: true, branch: "main", dirty: 3,
			commit: "263cf91", subject: "The look: what conn knows of a row",
			when: watchNow.Add(-3 * time.Hour), upstream: "origin/main", ahead: 142},
	}
}

// The look at both widths is the file of record.
func TestTheLookIsWhatItWas(t *testing.T) {
	b := composeLook(lookSubj(), "/Users/w0zro", watchNow)
	golden(t, "look-120x40.txt", texts(drawLook(b, 120, 40, plain)))
	golden(t, "look-narrow-60x40.txt", texts(drawLook(b, 60, 40, plain)))
}

// The page says the things the watch's columns have no room for, and
// says them whole.
func TestTheLookSaysWhatTheWatchCannot(t *testing.T) {
	text := texts(drawLook(composeLook(lookSubj(), "/Users/w0zro", watchNow), 120, 40, plain))

	// The command as it was written, not in conn's own upper case: it is
	// a thing somebody might retype.
	if !strings.Contains(text, "claude --resume d81d7536-e545-4881-8daa-f1d291a03be1") {
		t.Errorf("the whole command is not on the page:\n%s", text)
	}
	// The ask is the reason to open the page on a waiting row at all, so
	// it comes before what the row is, where a cut page cannot lose it.
	if !strings.Contains(text, "INPUT NEEDED") {
		t.Errorf("what the agent is stopped on is not on the page:\n%s", text)
	}
	if strings.Index(text, "WAITING ON YOU") > strings.Index(text, "WHAT") {
		t.Errorf("the ask is not the first thing on the page:\n%s", text)
	}
	// And it is said once. The group above carries how long, so the
	// status does not carry it too: on a dense page a thing said twice
	// reads as two things.
	if n := strings.Count(text, "7M 00S"); n != 1 {
		t.Errorf("how long it has waited is on the page %d times:\n%s", n, text)
	}
	for what, want := range map[string]string{
		"how long it has waited":  "FOR ....... 7M 00S",
		"its own directory":       "~/projects/w0zro/conn/tools",
		"what the table says":     "SLEEPING · HAS THE TERMINAL",
		"what it has spent":       "2M 14S SPENT",
		"which conversation":      "d81d7536-e545-4881-8daa-f1d291a03be1",
		"what it goes by":         "CONN-2D",
		"the last thing it asked": "i want the info to use the pane on the right",
		"the branch and the tree": "MAIN · 3 CHANGED",
		"the commit":              "263cf91",
		"what it is tracking":     "ORIGIN/MAIN · 142 AHEAD",
		"what runs it":            "SHELL zsh · 49200",
		"what it runs":            "RUN caffeinate -i -t 300 · ACTIVE",
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
func TestTheLookLeavesOutWhatThereIsNoneOf(t *testing.T) {
	s := lookSubject{
		entry: entry{pid: 88, kind: kindShell, command: "zsh", tty: "ttys009",
			started: watchNow.Add(-time.Hour), status: statusIdle,
			cwd: "/Users/w0zro/projects/w0zro/conn"},
		proc:   process{pid: 88, state: 'S'},
		place:  place{path: "/Users/w0zro/projects/w0zro/conn"},
		inside: true,
	}
	text := texts(drawLook(composeLook(s, "/Users/w0zro", watchNow), 120, 40, plain))

	for _, gone := range []string{"WAITING ON YOU", "ASKING", "AGENT", "PLACE\n", "TREE", "CWD", "CPU"} {
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
func TestTheLookSaysNothingOfPanesOutsideTheServer(t *testing.T) {
	s := lookSubj()
	s.inside, s.pane = false, pane{}
	text := texts(drawLook(composeLook(s, "/Users/w0zro", watchNow), 120, 40, plain))
	if strings.Contains(text, "PANE") {
		t.Errorf("outside the server the page still spoke of panes:\n%s", text)
	}
}

// A stopped process is not dated from an agent's own clock: the moment
// conn holds is when the agent last changed what it says of itself,
// which is not when anything stopped it.
func TestTheLookDoesNotDateAFaultFromTheAgentsClock(t *testing.T) {
	s := lookSubj()
	s.entry.status, s.entry.fault, s.entry.asking = statusStopped, true, ""
	text := texts(drawLook(composeLook(s, "/Users/w0zro", watchNow), 120, 40, plain))
	if strings.Contains(text, "STOPPED · FOR") {
		t.Errorf("a stopped row was dated from the agent's clock:\n%s", text)
	}
}

// A branch with nothing to track is not behind by nothing — there is
// nothing for it to be behind — so it says neither.
func TestTheLookSaysNothingOfTrackingWithNoUpstream(t *testing.T) {
	s := lookSubj()
	s.git.upstream, s.git.ahead, s.git.dirty = "", 0, 0
	text := texts(drawLook(composeLook(s, "/Users/w0zro", watchNow), 120, 40, plain))
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
	pl := place{path: "/w", entries: []entry{
		{pid: 1, kind: kindShell, command: "zsh", depth: 0},
		{pid: 2, kind: kindAgent, command: "claude", depth: 1},
		{pid: 3, kind: kindRun, command: "go test", depth: 2},
		{pid: 4, kind: kindRun, command: "compile", depth: 3}, // a grandchild
		{pid: 5, kind: kindRun, command: "caffeinate", depth: 2},
		{pid: 6, kind: kindShell, command: "zsh", depth: 0}, // another tree
	}}
	s, ok := subjectOf(2, []place{pl}, nil)
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
	s, _ = subjectOf(6, []place{pl}, nil)
	if s.parent.pid != 0 || len(s.children) != 0 {
		t.Errorf("a bare root stands under %d with %d children", s.parent.pid, len(s.children))
	}
}

// i asks the server to put the page in the slot; it does not take the
// watch's own pane, which is the whole point of it being over there.
func TestIPutsTheLookInTheSlot(t *testing.T) {
	m := newModel(plain)
	m.view, m.inside, m.now = viewWatch, true, watchNow
	m.srv = &server{tmux: "/nonexistent/tmux", socket: "/tmp/none"}
	m.places = []place{{path: "/w", entries: []entry{{pid: 49212, tty: "ttys003"}}}}
	m.cursor, m.cursorAt = 49212, 0

	next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "i"}))
	m = next.(model)
	if cmd == nil {
		t.Fatal("i asked the server for nothing")
	}
	if m.view != viewWatch {
		t.Errorf("i took the watch's own pane: view %d", m.view)
	}
	// The rail keeps drawing the watch while the page is in the slot.
	if !strings.Contains(m.View().Content, "STATUS") {
		t.Errorf("the watch is not still on the rail:\n%s", m.View().Content)
	}
	// A server that is not there is said on the bottom row rather than
	// swallowed.
	if n, ok := cmd().(noteMsg); !ok || !strings.Contains(n.note, "TMUX") {
		t.Errorf("a server that is not there should be said: %+v", n)
	}
}

// Nothing under the cursor, and nowhere to put a page, are each said
// rather than opening one about nothing.
func TestISaysWhyItCannotLook(t *testing.T) {
	m := newModel(plain)
	m.view, m.inside = viewWatch, true
	next, _ := m.Update(tea.KeyPressMsg(tea.Key{Text: "i"}))
	m = next.(model)
	if m.note != "NOTHING UNDER THE CURSOR" {
		t.Errorf("i on an empty watch says %q", m.note)
	}

	m = newModel(plain)
	m.view, m.inside = viewWatch, false
	m.places = []place{{path: "/w", entries: []entry{{pid: 7, tty: "ttys001"}}}}
	m.cursor = 7
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Text: "i"}))
	m = next.(model)
	if !strings.Contains(m.note, "OUTSIDE CONN'S TMUX SERVER") {
		t.Errorf("i outside the server says %q", m.note)
	}
}

// A row that ends while its page is up says so rather than going blank
// or holding the last thing it read.
func TestTheLookSaysWhenItsRowIsGone(t *testing.T) {
	text := texts(drawLook(lookReport{pid: 49212, gone: true}, 120, 40, plain))
	if !strings.Contains(text, "NO LONGER ON WATCH") {
		t.Errorf("a page whose row went says:\n%s", text)
	}
	if !strings.Contains(text, "PID 49212") {
		t.Errorf("it still says which row it was about:\n%s", text)
	}
}

// Every label fits inside the leader's field. One that does not leaves
// a single dot and starts its value a column past every other value on
// the page, which is the one thing a column of facts must not do.
func TestEveryLookLabelFitsTheLeader(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range []lookSubject{lookSubj(), {entry: entry{pid: 1, kind: kindShell}, inside: true}} {
		for _, g := range composeLook(s, "/Users/w0zro", watchNow).groups {
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
	text := texts(drawLook(composeLook(lookSubj(), "/Users/w0zro", watchNow), 120, 40, plain))
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
