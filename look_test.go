package main

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// lookAgent is a waiting agent with everything the page has to say
// about one: a long command, a status it has stood in for a while, a
// directory of its own under its place, a pane conn holds, and an ask.
func lookAgent() (entry, place) {
	return entry{
		pid: 49212, kind: kindAgent,
		command: "claude --resume d81d7536-e545-4881-8daa-f1d291a03be1",
		tty:     "ttys003", started: watchNow.Add(-92 * time.Minute),
		status: statusWaiting, since: watchNow.Add(-7 * time.Minute),
		cwd: "/Users/w0zro/projects/w0zro/conn/tools", asking: "input needed",
	}, place{
		path: "/Users/w0zro/projects/w0zro/conn",
	}
}

// The look at both widths is the file of record.
func TestTheLookIsWhatItWas(t *testing.T) {
	e, pl := lookAgent()
	b := composeLook(e, pl, pane{id: "%2"}, true, "/Users/w0zro", watchNow)
	golden(t, "look-120x22.txt", texts(drawLook(b, 120, 22, plain)))
	golden(t, "look-rail-48x26.txt", texts(drawLook(b, 48, 26, plain)))
}

// The page says the things the watch's columns have no room for, and
// says them whole: the command rather than its head, and as it was
// written rather than in conn's own upper case, since it is a thing
// somebody might retype.
func TestTheLookSaysWhatTheWatchCannot(t *testing.T) {
	e, pl := lookAgent()
	text := texts(drawLook(composeLook(e, pl, pane{id: "%2"}, true, "/Users/w0zro", watchNow), 120, 22, plain))

	if !strings.Contains(text, "claude --resume d81d7536-e545-4881-8daa-f1d291a03be1") {
		t.Errorf("the whole command is not on the page:\n%s", text)
	}
	// The ask is the reason to open the page on a waiting row at all, so
	// it comes before what the row is, where a cut page cannot lose it.
	if !strings.Contains(text, "ASKING") || !strings.Contains(text, "INPUT NEEDED") {
		t.Errorf("what the agent is stopped on is not on the page:\n%s", text)
	}
	if strings.Index(text, "WAITING ON YOU") > strings.Index(text, "WHAT") {
		t.Errorf("the ask is not the first thing on the page:\n%s", text)
	}
	// How long it has stood that way, which is what orders the answering.
	if !strings.Contains(text, "WAITING · FOR 7M 00S") {
		t.Errorf("how long it has waited is not on the page:\n%s", text)
	}
	// Its own directory, which differs from its tree's place.
	if !strings.Contains(text, "~/projects/w0zro/conn/tools") {
		t.Errorf("the process's own directory is not on the page:\n%s", text)
	}
}

// A row with nothing more to say says nothing more: a shell has no ask,
// so it gets no agent group, and a heading over nothing is not drawn.
// A process sitting in its tree's own place is not told it is there
// twice.
func TestTheLookLeavesOutWhatThereIsNoneOf(t *testing.T) {
	e := entry{pid: 88, kind: kindShell, command: "zsh", tty: "ttys009",
		started: watchNow.Add(-time.Hour), status: statusIdle,
		cwd: "/Users/w0zro/projects/w0zro/conn"}
	pl := place{path: "/Users/w0zro/projects/w0zro/conn"}
	text := texts(drawLook(composeLook(e, pl, pane{}, true, "/Users/w0zro", watchNow), 120, 22, plain))

	if strings.Contains(text, "WAITING ON YOU") || strings.Contains(text, "ASKING") {
		t.Errorf("a shell got a group for an ask it never made:\n%s", text)
	}
	if strings.Contains(text, "CWD") {
		t.Errorf("a process in its own place was told so twice:\n%s", text)
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
	e, pl := lookAgent()
	text := texts(drawLook(composeLook(e, pl, pane{}, false, "/Users/w0zro", watchNow), 120, 22, plain))
	if strings.Contains(text, "PANE") {
		t.Errorf("outside the server the page still spoke of panes:\n%s", text)
	}
}

// A stopped process is not dated from an agent's own clock: the moment
// conn holds is when the agent last changed what it says of itself,
// which is not when anything stopped it.
func TestTheLookDoesNotDateAFaultFromTheAgentsClock(t *testing.T) {
	e, pl := lookAgent()
	e.status, e.fault = statusStopped, true
	text := texts(drawLook(composeLook(e, pl, pane{id: "%2"}, true, "/Users/w0zro", watchNow), 120, 22, plain))
	if strings.Contains(text, "· FOR") {
		t.Errorf("a stopped row was dated from the agent's clock:\n%s", text)
	}
}

// i opens the page on the row under the cursor, and i or esc comes back
// to the watch. The page is read off the last reading rather than off
// the row as it was when i was pressed, so a status that changes while
// it is up is on it.
func TestIOpensTheLookAndComesBack(t *testing.T) {
	m := newModel(plain)
	m.view, m.inside, m.now = viewWatch, true, watchNow
	e, pl := lookAgent()
	pl.entries = []entry{e}
	m.places = []place{pl}
	m.panes = map[string]pane{"ttys003": {id: "%2", tty: "ttys003"}}
	m.cursor, m.cursorAt = e.pid, 0

	press := func(k string) tea.Cmd {
		next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: k}))
		m = next.(model)
		return cmd
	}
	press("i")
	if m.view != viewLook || m.looking != e.pid {
		t.Fatalf("i left the view %d looking at %d", m.view, m.looking)
	}
	if !strings.Contains(m.View().Content, "INPUT NEEDED") {
		t.Errorf("the page is not on the cursor's row:\n%s", m.View().Content)
	}

	// The reading moves on and the page moves with it.
	e.asking, e.status = "", statusWorking
	pl.entries = []entry{e}
	next, _ := m.Update(watchMsg{places: []place{pl}, gen: m.watchGen})
	m = next.(model)
	if strings.Contains(m.View().Content, "INPUT NEEDED") {
		t.Errorf("the page held an ask that had been answered:\n%s", m.View().Content)
	}

	if cmd := press("i"); cmd == nil || m.view != viewWatch || m.looking != 0 {
		t.Errorf("i did not come back to the watch: view %d, looking %d", m.view, m.looking)
	}
	press("i")
	if cmd := press("esc"); cmd == nil || m.view != viewWatch {
		t.Errorf("esc did not come back to the watch: view %d", m.view)
	}
}

// A row that ends while its page is up says so rather than going blank
// or holding the last thing it read.
func TestTheLookSaysWhenItsRowIsGone(t *testing.T) {
	m := newModel(plain)
	m.view, m.looking, m.now = viewLook, 49212, watchNow
	if text := m.View().Content; !strings.Contains(text, "NO LONGER ON WATCH") {
		t.Errorf("a page whose row went says:\n%s", text)
	}
}

// Nothing under the cursor is said rather than opening a page about
// nothing.
func TestIOnNothingSaysSo(t *testing.T) {
	m := newModel(plain)
	m.view, m.inside = viewWatch, true
	next, _ := m.Update(tea.KeyPressMsg(tea.Key{Text: "i"}))
	m = next.(model)
	if m.view != viewWatch || m.note != "NOTHING UNDER THE CURSOR" {
		t.Errorf("i on an empty watch: view %d, note %q", m.view, m.note)
	}
}
