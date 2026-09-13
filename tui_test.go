package main

import (
	"strings"
	"syscall"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// answered runs a command and processes back what it answered. A key
// that changes a mode answers with a batch — the status line's telling
// beside whatever the key itself asked for — and the status line's half
// answers nothing, so flattening the batch leaves the one message that
// matters.
func answered(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return msg
	}
	for _, c := range batch {
		if c == nil {
			continue
		}
		if m := c(); m != nil {
			return m
		}
	}
	return nil
}

// The console comes on in stages: the header at once, the readout when
// the station has been read and its beat has passed, the screen's check
// alone, then the rest; a key skips to the end; a key at the end
// continues to the processes view; the clock turns on the second.
func TestProgramComesOnInStages(t *testing.T) {
	// ticking as newModel leaves it: conn comes up on the console, which
	// annunciates, and Init sets the blink going.
	m := model{head: station{build: testStation.build, login: login{user: "w0zro", host: "station"}}, now: testNow, p: plain, ticking: true}
	m.width, m.height = 120, 40
	view := func() string { return m.View().Content }
	has := func(s string) bool { return strings.Contains(view(), s) }
	if !has("STATION  W0ZRO@STATION") || !has("CONN 0.7.0 (devel)") || has("HOST ...") || has("SCREEN") || has(prompt) {
		t.Errorf("the header alone should be up at the start:\n%s", view())
	}
	if got := strings.Count(view(), "\n") + 1; got != 40 {
		t.Errorf("view is %d rows, not the terminal's 40", got)
	}

	// The readout's beat passes before the station is read: it waits.
	next, cmd := m.Update(stageMsg{})
	m = next.(model)
	if cmd != nil || !m.due || has("HOST ...") {
		t.Errorf("the readout came on before the station was read:\n%s", view())
	}
	next, cmd = m.Update(stationMsg{testStation})
	m = next.(model)
	if cmd == nil || m.due || !has("SYSTEM .... MACOS 26.6.2 (25G83)") || has("SCREEN") {
		t.Errorf("the readout should come on with the station:\n%s", view())
	}
	next, _ = m.Update(stageMsg{})
	m = next.(model)
	if !has("SCREEN") || has("STATE ...") {
		t.Errorf("the screen check should be up third, alone:\n%s", view())
	}
	for m.stage < lastStage(m.report()) {
		next, _ = m.Update(stageMsg{})
		m = next.(model)
	}
	if !has("CLOCK") || !has("ALL SYSTEMS NOMINAL") {
		t.Errorf("the console did not finish:\n%s", view())
	}
	if lines := strings.Split(view(), "\n"); !strings.Contains(lines[len(lines)-1], prompt) {
		t.Errorf("the prompt is not on the bottom row:\n%s", view())
	}
	if lastStage(m.report()) != stageChecks+10 {
		t.Errorf("last stage is %d", lastStage(m.report()))
	}
	if next, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"}); cmd == nil || next.(model).view != viewConsole || !next.(model).entering {
		t.Errorf("a key at the end should read the processes view and hold the console for the answer")
	}
	before := m.report().clock
	next, cmd = m.Update(clockMsg{})
	m = next.(model)
	if m.report().clock == before || cmd == nil {
		t.Errorf("the clock did not turn: %q", m.report().clock)
	}
}

// The station arriving first, then the beat, comes on the same way; and
// a key during the sequence skips to the end.
func TestStationBeforeTheBeatAndAKeySkips(t *testing.T) {
	m := model{head: station{build: testStation.build}, now: testNow, p: plain, width: 120, height: 40, ticking: true}
	next, cmd := m.Update(stationMsg{testStation})
	m = next.(model)
	if cmd != nil || m.stage != stageHeader {
		t.Errorf("the station alone should not bring the readout on")
	}
	next, cmd = m.Update(stageMsg{})
	m = next.(model)
	if cmd == nil || m.stage != stageReadout {
		t.Errorf("the beat after the station should bring the readout on: stage %d", m.stage)
	}
	next, cmd = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = next.(model)
	if cmd != nil || m.stage != lastStage(m.report()) {
		t.Errorf("a key should skip to the end: stage %d", m.stage)
	}
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"}); cmd == nil {
		t.Error("q should close the console")
	}
}

// The clock ticks on the turn of the second.
func TestTheClockTicksOnTheSecond(t *testing.T) {
	if d := m0().stageDelay(stageReadout); d != 150*time.Millisecond {
		t.Errorf("the readout's beat is %v", d)
	}
	start := time.Now()
	nextSecond(start)()
	if late := time.Since(start.Truncate(time.Second).Add(time.Second)); late < 0 || late > 50*time.Millisecond {
		t.Errorf("the tick came %v from the turn of the second", late)
	}
}

func m0() model {
	return model{head: station{build: testStation.build}, now: testNow, p: plain}
}

// The dark half is half the lit half, and each turn schedules the
// other: the blink goes on by itself for as long as conn is up.
func TestTheBlinkHasTwoHalves(t *testing.T) {
	if blinkDark*2 != blinkLit {
		t.Errorf("lit %v, dark %v: the dark half should be half of the lit", blinkLit, blinkDark)
	}
	m := newModel(plain)
	if !m.lit {
		t.Error("the chip starts dark")
	}
	next, ok := m.Update(blinkMsg{gen: m.blinkGen})
	m = next.(model)
	if m.lit {
		t.Error("the chip did not go dark on the turn")
	}
	if ok == nil {
		t.Fatal("the blink stopped at the first turn")
	}
	next, ok = m.Update(blinkMsg{gen: m.blinkGen})
	if m = next.(model); !m.lit || ok == nil {
		t.Error("the chip did not come back")
	}
	// Off the console, with nothing in the processes view waiting, the
	// blink stops and rests lit; coming back to the console starts it
	// again; a turn from an earlier run is dropped. The first key skips
	// the console's stages to the end, the second leaves for the processes
	// view, and the processes view is not up until its reading is.
	next, ok = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = next.(model)
	next, ok = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = next.(model)
	next, ok = m.Update(processesMsg{gen: m.processesGen})
	if m = next.(model); m.view != viewProcesses || m.ticking {
		t.Errorf("on a view with nothing waiting the blink still ticks: view %d", m.view)
	}
	next, ok = m.Update(blinkMsg{gen: m.blinkGen})
	if m = next.(model); !m.lit || ok != nil {
		t.Error("the blink went on with nothing to annunciate")
	}
	next, ok = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if m = next.(model); m.view != viewConsole || ok == nil || !m.ticking {
		t.Error("c did not start the blink again")
	}
	next, ok = m.Update(blinkMsg{gen: m.blinkGen - 1})
	if m = next.(model); !m.lit || ok != nil {
		t.Error("a turn from an earlier stay was not dropped")
	}
}

// A reading that finds home without its bay has the bay opened; a
// reading with the bay only ticks.
func TestAHomeWithoutItsBayGetsOne(t *testing.T) {
	m := model{p: plain, width: 48, height: 40, view: viewProcesses, inside: true, srv: &server{tmux: "/nonexistent/tmux"}}
	_, cmd := m.Update(processesMsg{noBay: true})
	if cmd == nil {
		t.Fatal("no command for a home without its bay")
	}
	if _, cmd := m.Update(processesMsg{bay: "ttys009"}); cmd == nil {
		t.Error("a home with its bay should still tick")
	}
}

// A bay whose pane died on remain-on-exit is revived, not resplit: the
// panel never has to give up its width and take it back for it. With
// nothing to reach, a hold takes the bay; with a process to reach, that
// process does.
func TestABayWhosePaneDiedIsRevived(t *testing.T) {
	m := model{p: plain, width: 48, height: 40, view: viewProcesses, inside: true, srv: &server{tmux: "/nonexistent/tmux"}}
	next, cmd := m.Update(processesMsg{bay: "ttys009", bayDead: true})
	m = next.(model)
	if cmd == nil {
		t.Fatal("no command for a bay whose pane died")
	}
	if msg, ok := cmd().(tea.BatchMsg); !ok || len(msg) != 2 {
		t.Errorf("a dead bay did not both tick and revive: %T", cmd())
	}
}

// When what was in the bay ends, the bay takes the next process conn
// holds, from the cursor down and round again from the top; a hold, the
// readout and a dead pane are passed over, and with nothing to reach
// there is nothing.
func TestTheBayTakesTheNextProcessWhenItsOwnEnds(t *testing.T) {
	m := model{p: plain, view: viewProcesses, inside: true}
	m.projects = []project{{path: "/w", entries: []entry{
		{pid: 1, tty: "ttys001"}, {pid: 2, tty: "ttys002"}, {pid: 3, tty: "ttys003"}, {pid: 4, tty: "ttys004"},
	}}}
	m.panes = map[string]pane{
		"ttys001": {id: "%1", tty: "ttys001"},
		"ttys002": {id: "%2", tty: "ttys002", dead: true},
		"ttys003": {id: "%3", tty: "ttys003", hold: true},
	}
	m.cursor = 2
	if e, ok := m.nextReachable(); !ok || e.pid != 1 {
		t.Errorf("from the dead row, round again to the first: %+v %v", e, ok)
	}
	m.cursor = 1
	if e, ok := m.nextReachable(); !ok || e.pid != 1 {
		t.Errorf("the cursor's own row when it can be reached: %+v %v", e, ok)
	}
	m.panes["ttys004"] = pane{id: "%4", tty: "ttys004"}
	m.cursor = 2
	if e, ok := m.nextReachable(); !ok || e.pid != 4 {
		t.Errorf("the next down before round again: %+v %v", e, ok)
	}
	m.panes = nil
	if _, ok := m.nextReachable(); ok {
		t.Error("with nothing held, something was reached")
	}
}

// A shell conn opens is the cursor's once the process table has it. The
// bay is marked at once; until the reading brings the shell the
// processes view reads soon rather than at its pace, and the cursor
// stays where it was; a shell that never comes is given up on when the
// wait is out.
func TestTheCursorGoesToTheShellOnceItIsRead(t *testing.T) {
	here := []project{{path: "/w", entries: []entry{{pid: 11}, {pid: 22}}}}
	read := func(m model, projects []project) model {
		next, _ := m.Update(processesMsg{projects: projects, gen: m.processesGen})
		return next.(model)
	}
	m := newModel(plain)
	m.view, m.cursor, m.now = viewProcesses, 11, time.Now()
	m = read(m, here)

	next, cmd := m.Update(openedMsg{shell: shell{pane: pane{id: "%9", tty: "ttys009"}, pid: 4242}})
	m = next.(model)
	if cmd == nil || m.awaited != 4242 || m.bay != "ttys009" {
		t.Errorf("after opening: cmd %v, awaited %d, bay %q", cmd != nil, m.awaited, m.bay)
	}
	// A reading without it yet leaves the cursor, and the next read is soon.
	m = read(m, here)
	if m.cursor != 11 || m.awaited != 4242 {
		t.Errorf("before the shell is read: cursor %d, awaited %d", m.cursor, m.awaited)
	}
	if next, _ := m.Update(processesTickMsg{gen: m.processesGen}); next == nil {
		t.Error("the tick should read")
	}
	// The reading that brings the shell puts the cursor on it.
	withIt := []project{{path: "/w", entries: []entry{{pid: 4242, kind: kindShell}, {pid: 11}, {pid: 22}}}}
	m = read(m, withIt)
	if m.cursor != 4242 || m.awaited != 0 {
		t.Errorf("with the shell read: cursor %d, awaited %d", m.cursor, m.awaited)
	}
	// A shell that never comes up is given up on once the wait is out.
	m.awaited, m.until, m.cursor = 9999, time.Now().Add(-time.Second), 11
	m = read(m, here)
	if m.awaited != 0 || m.cursor != 11 {
		t.Errorf("after the wait: awaited %d, cursor %d", m.awaited, m.cursor)
	}
}

// Reaching a process puts it in the bay, and conn knows that without
// reading the server back: the row says it is the one shown at once,
// and the row that was shown stops saying so.
func TestTheReachedRowIsTheBayAtOnce(t *testing.T) {
	m := newModel(plain)
	m.view, m.bay = viewProcesses, "ttys001"
	m.projects = []project{{path: "/w", entries: []entry{{pid: 11, tty: "ttys001"}, {pid: 22, tty: "ttys002"}}}}
	m.panes = map[string]pane{"ttys001": {id: "%1", tty: "ttys001"}, "ttys002": {id: "%2", tty: "ttys002"}}

	gen := m.processesGen
	next, cmd := m.Update(reachedMsg{"ttys002"})
	m = next.(model)
	if m.bay != "ttys002" {
		t.Errorf("the bay is %q, not the reached terminal", m.bay)
	}
	if cmd == nil || m.processesGen == gen {
		t.Error("the processes view was not read again after reaching")
	}
	// The processes view says so: the reached row is shown, the one it
	// replaced is not.
	w := m.processesReport()
	for _, pl := range w.projects {
		for _, r := range pl.rows {
			if r.tty == "ttys002" && !r.shown {
				t.Error("the reached row does not read as the one in the bay")
			}
			if r.tty == "ttys001" && r.shown {
				t.Error("the row that left the bay still reads as shown")
			}
		}
	}
}

// A pane is the whole tree in it, so reaching one from a row down
// inside it reaches the head, and the cursor goes to the head too. The
// row that asked is not the row that answered, and a cursor left on the
// sub-process would pick out the one thing in the pane that is not what
// is in the bay.
func TestReachingFromInsideATreePutsTheCursorOnItsHead(t *testing.T) {
	m := newModel(plain)
	m.view, m.inside = viewProcesses, true
	m.projects = []project{{path: "/w", entries: []entry{
		{pid: 9, tty: "ttys001"},
		{pid: 11, tty: "ttys002"},           // the head: what the pane was opened on
		{pid: 12, tty: "ttys002", depth: 1}, // the contact it runs
		{pid: 13, tty: "ttys002", depth: 2}, // and what the contact runs
	}}}
	m.panes = map[string]pane{"ttys001": {id: "%1", tty: "ttys001"}, "ttys002": {id: "%2", tty: "ttys002"}}
	m.cursor, m.cursorAt = 13, 3 // down inside the tree

	next, _ := m.Update(reachedMsg{"ttys002"})
	m = next.(model)
	if m.cursor != 11 || m.cursorAt != 1 {
		t.Errorf("the cursor is on pid %d at row %d, not the head of the pane it reached", m.cursor, m.cursorAt)
	}
	// And the cursor and the mark are the same row, which is the point.
	for _, pl := range m.processesReport().projects {
		for _, r := range pl.rows {
			if r.shown != (r.pid == m.cursor) {
				t.Errorf("pid %d: shown %v, cursor on %d", r.pid, r.shown, m.cursor)
			}
		}
	}
}

// tab goes to what is waiting on you: the first press to the one that
// has waited longest, each after it to the next, and round again from
// the end. Off the ring it starts at the front, so a cursor anywhere
// else is one key from the thing that has waited longest.
func TestTabWalksTheWaitingLongestFirst(t *testing.T) {
	at := func(s int) time.Time { return time.Now().Add(time.Duration(-s) * time.Second) }
	m := newModel(plain)
	m.view = viewProcesses
	m.projects = []project{{path: "/w", entries: []entry{
		{pid: 11, status: statusIdle},
		{pid: 22, status: statusWaiting, since: at(60)},
		{pid: 33, status: statusWorking},
		{pid: 44, status: statusWaiting, since: at(600)},
	}}}
	m.cursor, m.cursorAt = 11, 0

	tab := func() {
		next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		m = next.(model)
	}
	for i, want := range []int{44, 22, 44, 22} { // longest first, then round
		tab()
		if m.cursor != want {
			t.Fatalf("tab %d put the cursor on %d, want %d", i+1, m.cursor, want)
		}
	}
	// And the row it lands on is the row it means, not just the pid.
	if e, _, ok := m.under(); !ok || e.status != statusWaiting {
		t.Errorf("tab landed on %+v, which is not waiting", e)
	}

	// With nothing waiting the cursor stays.
	m.projects = []project{{path: "/w", entries: []entry{{pid: 11, status: statusIdle}}}}
	m.cursor, m.cursorAt = 11, 0
	tab()
	if m.cursor != 11 {
		t.Errorf("with nothing waiting: cursor %d", m.cursor)
	}
}

// In the server, tab puts the waiting contact's pane in the bay and the
// keys in it, so one press has the operator answering; a process conn
// holds no pane for is gone to on the panel and the keys stay. The
// prefix then tab sends alt+tab, which does the same from any view,
// putting the processes view up on the way.
func TestTabReachesTheWaitingContact(t *testing.T) {
	m := newModel(plain)
	m.view, m.inside, m.srv = viewProcesses, true, &server{tmux: "/nonexistent/tmux", socket: "/tmp/none"}
	m.projects = []project{{path: "/w", entries: []entry{
		{pid: 11, status: statusIdle, tty: "ttys001"},
		{pid: 22, status: statusWaiting, tty: "ttys002", since: time.Now().Add(-time.Minute)},
	}}}
	m.panes = map[string]pane{"ttys002": {id: "%2", tty: "ttys002"}}
	m.cursor = 11

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = next.(model)
	if m.cursor != 22 || cmd == nil {
		t.Fatalf("tab: cursor %d, cmd %v", m.cursor, cmd != nil)
	}
	// The command is the reach, which against no tmux reaches nothing.
	if _, ok := answered(cmd).(reachedMsg); ok {
		t.Error("tab reached a pane with no tmux to reach it with")
	}

	// Not held: the cursor goes, the keys do not.
	m.panes, m.cursor = nil, 11
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = next.(model)
	if m.cursor != 22 || answered(cmd) != nil {
		t.Errorf("unheld: cursor %d, cmd %v", m.cursor, answered(cmd))
	}

	// From the console, by the chord: the processes view comes up and the
	// process is reached.
	m.panes = map[string]pane{"ttys002": {id: "%2", tty: "ttys002"}}
	m.view, m.cursor = viewConsole, 11
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModAlt})
	m = next.(model)
	if m.view != viewProcesses || m.cursor != 22 || cmd == nil {
		t.Errorf("from the console: view %d, cursor %d, cmd %v", m.view, m.cursor, cmd != nil)
	}
}

// a opens claude at the project under the cursor, the way s opens a
// shell there; outside the server nothing can be opened, and off any
// project there is nothing to open it at.
func TestAOpensAContactAtTheProject(t *testing.T) {
	m := newModel(plain)
	m.view = viewProcesses
	m.projects = []project{{path: "/w", entries: []entry{{pid: 11, tty: "ttys001"}}}}
	m.cursor = 11

	next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "a"}))
	m = next.(model)
	if cmd != nil {
		t.Error("outside the server, a opened something")
	}

	m.inside, m.srv = true, &server{tmux: "/nonexistent/tmux"}
	next, cmd = m.Update(tea.KeyPressMsg(tea.Key{Text: "a"}))
	m = next.(model)
	if cmd == nil {
		t.Fatal("in the server, a opened nothing")
	}
	if _, ok := answered(cmd).(openedMsg); ok {
		t.Error("a shell opened with no tmux to open it in")
	}

	m.projects = nil
	next, cmd = m.Update(tea.KeyPressMsg(tea.Key{Text: "a"}))
	m = next.(model)
	// Nothing is opened and nothing is waited for.
	if m.awaited != 0 {
		t.Errorf("off any project: awaited %d", m.awaited)
	}
}

// alt+a opens the sessions view over what claude left suspended at the
// project under the cursor, asking for that project's own directory
// alone. Plain A is not bound to it — a shift chord costs the same as
// an alt one, so there is no reason to answer to both.
func TestAltAOpensSessionsAtTheProject(t *testing.T) {
	m := newModel(plain)
	m.view = viewProcesses
	m.projects = []project{{path: "/w", entries: []entry{{pid: 11, tty: "ttys001"}}}}
	m.cursor = 11

	next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "alt+a"}))
	m = next.(model)
	if cmd != nil || m.view != viewProcesses {
		t.Errorf("outside the server: cmd %v, view %d", cmd != nil, m.view)
	}

	m.inside = true
	next, cmd = m.Update(tea.KeyPressMsg(tea.Key{Text: "alt+a"}))
	m = next.(model)
	if m.view != viewSessions || !m.sessionsLoading || cmd == nil {
		t.Fatalf("in the server: view %d, loading %v, cmd %v", m.view, m.sessionsLoading, cmd != nil)
	}
	if got := m.sessionsDirs; len(got) != 1 || got[0] != "/w" {
		t.Errorf("convosDirs = %v", got)
	}
	if msg, ok := cmd().(sessionsMsg); !ok || len(msg.dirs) != 1 || msg.dirs[0] != "/w" {
		t.Errorf("scanConvos did not ask for the project under the cursor: %v", cmd())
	}

	// A is unbound in the processes view: nothing happens, the view holds.
	m.view = viewProcesses
	next, cmd = m.Update(tea.KeyPressMsg(tea.Key{Text: "A"}))
	m = next.(model)
	if m.view != viewProcesses || cmd != nil {
		t.Errorf("A did something: view %d, cmd %v", m.view, cmd != nil)
	}
}

// x arms a kill on the entry under the cursor rather than sending one;
// off any entry there is nothing to arm, and it says so.
func TestXArmsAKillOnTheEntryUnderTheCursor(t *testing.T) {
	m := newModel(plain)
	m.view = viewProcesses
	m.projects = []project{{path: "/w", entries: []entry{{pid: 11, kind: kindContact, command: "claude"}}}}
	m.cursor = 11

	next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "x"}))
	m = next.(model)
	if cmd != nil || m.kill == nil || m.kill.pid != 11 || m.kill.command != "claude" || m.kill.sig != syscall.SIGTERM {
		t.Fatalf("arming: cmd %v, kill %+v", cmd != nil, m.kill)
	}
	// The question travels with the kill, to the status line.
	if !strings.Contains(m.kill.prompt, "END CLAUDE 11 ·") {
		t.Errorf("the question: kill %+v", m.kill)
	}

	// A bare shell — nothing running in it to lose — is armed for SIGKILL
	// instead, since it is proven to ignore the gentler signals.
	m.projects = []project{{path: "/w", entries: []entry{{pid: 22, kind: kindShell, command: "zsh"}}}}
	m.cursor, m.kill = 22, nil
	next, cmd = m.Update(tea.KeyPressMsg(tea.Key{Text: "x"}))
	m = next.(model)
	if m.kill == nil || m.kill.sig != syscall.SIGKILL || !strings.Contains(m.kill.prompt, "KILL ZSH 22 ·") {
		t.Errorf("arming a shell: kill %+v", m.kill)
	}

	m.projects, m.kill = nil, nil
	next, cmd = m.Update(tea.KeyPressMsg(tea.Key{Text: "x"}))
	m = next.(model)
	if cmd != nil || m.kill != nil {
		t.Errorf("off any entry: cmd %v, kill %v", cmd != nil, m.kill)
	}
}

// x, y or enter answers an armed kill by sending it; anything else
// cancels, and takes the key that cancelled it rather than also acting
// on it — j does not also move the cursor.
func TestAnArmedKillIsConfirmedOrCancelled(t *testing.T) {
	m := newModel(plain)
	m.view = viewProcesses
	m.projects = []project{{path: "/w", entries: []entry{{pid: 11, kind: kindContact, command: "claude"}}}}
	m.cursor = 11

	m.kill = &pendingKill{pid: 11, command: "claude", sig: syscall.SIGTERM}
	next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "j"}))
	m = next.(model)
	if m.kill != nil || m.cursor != 11 {
		t.Errorf("cancelled: kill %v, cursor %d", m.kill, m.cursor)
	}

	m.kill = &pendingKill{pid: 11, command: "claude", sig: syscall.SIGTERM}
	next, cmd = m.Update(tea.KeyPressMsg(tea.Key{Text: "x"}))
	m = next.(model)
	if m.kill != nil || cmd == nil {
		t.Fatalf("confirmed: kill %v, cmd %v", m.kill, cmd != nil)
	}
	msg, ok := cmd().(killedMsg)
	if !ok || msg.pid != 11 || msg.command != "claude" || msg.sig != syscall.SIGTERM {
		t.Errorf("killEntry did not ask to signal the armed entry: %v", cmd())
	}
}

// After a kill the table is read again after a beat, so the row is not
// read a moment too soon.
func TestAKilledMsgRereads(t *testing.T) {
	m := newModel(plain)
	m.view = viewProcesses
	_, cmd := m.Update(killedMsg{command: "claude", pid: 11, sig: syscall.SIGTERM})
	if cmd == nil {
		t.Error("nothing is read again after a kill")
	}
}

// p leaves the processes view for the list and walks the roots; what is
// typed narrows the rows and puts the cursor back at the top; the
// arrows and ctrl+n and ctrl+p move it, held within the rows there are;
// esc comes back to the processes view, and the processes view reads
// again.
func TestTheListIsALineTypedInto(t *testing.T) {
	m := newModel(plain)
	m.view, m.width, m.height = viewProcesses, 48, 30
	key := func(m model, k string) (model, tea.Cmd) {
		next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: k, Code: rune(k[0])}))
		return next.(model), cmd
	}
	m, cmd := key(m, "p")
	if m.view != viewProjects || !m.scanning || cmd == nil {
		t.Fatalf("after p: view %d, scanning %v, cmd %v", m.view, m.scanning, cmd != nil)
	}
	next, _ := m.Update(projectsMsg{projects: testProjects})
	m = next.(model)
	if m.scanning || len(m.projectRows()) != len(testProjects) {
		t.Errorf("with the roots walked: scanning %v, %d rows", m.scanning, len(m.projectRows()))
	}
	// The letters the processes view is worked by are characters here.
	for _, k := range []string{"c", "o", "n", "n"} {
		m, _ = key(m, k)
	}
	if m.filter != "conn" || m.view != viewProjects {
		t.Fatalf("typed: filter %q, view %d", m.filter, m.view)
	}
	if rows := m.projectRows(); len(rows) != 2 || rows[1].name != "conn" {
		t.Errorf("conn leaves %d rows", len(rows))
	}
	// The cursor is held within them, and backspace widens them again.
	for range 5 {
		m, _ = key(m, "down")
	}
	if m.pcursor != 1 {
		t.Errorf("the cursor ran to %d of 2 rows", m.pcursor)
	}
	m, _ = key(m, "ctrl+p")
	if m.pcursor != 0 {
		t.Errorf("ctrl+p left the cursor at %d", m.pcursor)
	}
	m, _ = key(m, "backspace")
	if m.filter != "con" || m.pcursor != 0 {
		t.Errorf("after backspace: filter %q, cursor %d", m.filter, m.pcursor)
	}
	m, _ = key(m, "ctrl+u")
	if m.filter != "" {
		t.Errorf("ctrl+u left %q", m.filter)
	}
	m, cmd = key(m, "esc")
	if m.view != viewProcesses || cmd == nil {
		t.Errorf("after esc: view %d, cmd %v", m.view, cmd != nil)
	}
}

// Enter on a row opens a shell at that project and comes back to the
// processes view, where the shell will show. Outside the server nothing
// can be opened, and the list holds.
func TestEnterOpensAShellAtTheProject(t *testing.T) {
	m := newModel(plain)
	m.view, m.walked, m.pcursor = viewProjects, testProjects, 3
	next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = next.(model)
	if m.view != viewProjects || cmd != nil {
		t.Errorf("outside the server: view %d, cmd %v", m.view, cmd != nil)
	}
	m.inside, m.srv = true, &server{tmux: "/nonexistent/tmux"}
	next, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = next.(model)
	if m.view != viewProcesses || cmd == nil {
		t.Fatalf("in the server: view %d, cmd %v", m.view, cmd != nil)
	}
	// The shell is opened at the row the cursor was on: the tmux that
	// cannot be run says so as a note, which is where the path shows.
	if msg, ok := cmd().(tea.BatchMsg); !ok || len(msg) != 2 {
		t.Errorf("enter did not both read the processes view and open the shell: %T", cmd())
	}
}

// ctrl+a opens claude at the row instead, the way enter opens a shell
// there — plain a is a letter to type into the filter, so this is the
// list's key for it, the way ctrl+u is its key for clearing the filter.
func TestCtrlAOpensAnAgentAtTheProject(t *testing.T) {
	m := newModel(plain)
	m.view, m.walked, m.pcursor = viewProjects, testProjects, 3
	m.inside, m.srv = true, &server{tmux: "/nonexistent/tmux"}

	next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "ctrl+a"}))
	m = next.(model)
	if m.view != viewProcesses || cmd == nil {
		t.Fatalf("in the server: view %d, cmd %v", m.view, cmd != nil)
	}
	if msg, ok := cmd().(tea.BatchMsg); !ok || len(msg) != 2 {
		t.Errorf("ctrl+a did not both read the processes view and open the contact: %T", cmd())
	}
}

// alt+a opens the sessions view at the row's own directory, or, on a
// group, at every repository under it too — a transcript is filed by
// the exact directory it was had in, not the folder that names them.
// Plain A is not bound to it, unlike ctrl+shift+a which never could be
// — a shift chord costs the same as an alt one, so there is no reason
// to give up typing a capital letter into the filter for it.
func TestAltAOpensSessionsFromProjects(t *testing.T) {
	m := newModel(plain)
	m.view, m.walked, m.pcursor = viewProjects, testProjects, 0 // arboreum.io, a group of two
	m.inside = true

	next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "alt+a"}))
	m = next.(model)
	if m.view != viewSessions || cmd == nil {
		t.Fatalf("view %d, cmd %v", m.view, cmd != nil)
	}
	want := []string{
		"/Users/w0zro/projects/arboreum.io",
		"/Users/w0zro/projects/arboreum.io/content",
		"/Users/w0zro/projects/arboreum.io/welcome",
	}
	if !equal(m.sessionsDirs, want) {
		t.Errorf("convosDirs = %v, want %v", m.sessionsDirs, want)
	}

	// A is a letter to type here, the same as a is: the sessions view does
	// not take it from the filter.
	m.view, m.filter = viewProjects, ""
	next, cmd = m.Update(tea.KeyPressMsg(tea.Key{Text: "A"}))
	m = next.(model)
	if m.view != viewProjects || m.filter != "A" || cmd != nil {
		t.Errorf("A did not type: view %d, filter %q, cmd %v", m.view, m.filter, cmd != nil)
	}
}

// The sessions view is a line typed into, the same as the list: what is
// typed narrows the rows and puts the cursor back at the top, backspace
// and ctrl+u widen it again, and esc leaves without continuing
// anything.
func TestSessionsIsALineTypedInto(t *testing.T) {
	m := newModel(plain)
	m.view, m.sessions = viewSessions, testSessions2
	key := func(m model, k string) (model, tea.Cmd) {
		next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: k}))
		return next.(model), cmd
	}
	for _, k := range []string{"t", "o", "p", "i", "c"} {
		m, _ = key(m, k)
	}
	if m.rfilter != "topic" || len(m.sessionsRows()) != 1 {
		t.Fatalf("typed: filter %q, %d rows", m.rfilter, len(m.sessionsRows()))
	}
	m, _ = key(m, "backspace")
	if m.rfilter != "topi" {
		t.Errorf("after backspace: filter %q", m.rfilter)
	}
	m, _ = key(m, "ctrl+u")
	if m.rfilter != "" || len(m.sessionsRows()) != 2 {
		t.Errorf("ctrl+u left filter %q, %d rows", m.rfilter, len(m.sessionsRows()))
	}
	m, cmd := key(m, "esc")
	if m.view != viewProcesses || cmd == nil {
		t.Errorf("after esc: view %d, cmd %v", m.view, cmd != nil)
	}
}

// Enter continues the session under the cursor in a shell running
// claude --resume, and comes back to the processes view, where the
// shell will show; outside the server nothing can be opened, and the
// sessions view holds.
func TestEnterResumesTheSessionUnderTheCursor(t *testing.T) {
	m := newModel(plain)
	m.view, m.sessions, m.rcursor = viewSessions, testSessions2, 1
	next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = next.(model)
	if m.view != viewSessions || cmd != nil {
		t.Errorf("outside the server: view %d, cmd %v", m.view, cmd != nil)
	}
	m.inside, m.srv = true, &server{tmux: "/nonexistent/tmux"}
	next, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = next.(model)
	if m.view != viewProcesses || cmd == nil {
		t.Fatalf("in the server: view %d, cmd %v", m.view, cmd != nil)
	}
	if msg, ok := cmd().(tea.BatchMsg); !ok || len(msg) != 2 {
		t.Errorf("enter did not both read the processes view and open the session: %T", cmd())
	}
}

// A sessions view's listing that lands after it moved on to another
// project — or closed — is dropped: only the one that asked for these
// dirs wants them.
func TestAStaleSessionsAnswerIsDropped(t *testing.T) {
	m := newModel(plain)
	m.view, m.sessionsDirs, m.sessionsLoading = viewSessions, []string{"/a"}, true

	next, _ := m.Update(sessionsMsg{dirs: []string{"/b"}, sessions: testSessions2})
	m = next.(model)
	if !m.sessionsLoading || len(m.sessions) != 0 {
		t.Errorf("a stale answer landed: loading %v, %d convos", m.sessionsLoading, len(m.sessions))
	}

	next, _ = m.Update(sessionsMsg{dirs: []string{"/a"}, sessions: testSessions2})
	m = next.(model)
	if m.sessionsLoading || len(m.sessions) != len(testSessions2) {
		t.Errorf("the matching answer did not land: loading %v, %d convos", m.sessionsLoading, len(m.sessions))
	}
}

// Projects has a key of its own that reaches it from every view, which
// is what the prefix chord sends. p cannot serve: it opens projects
// from the processes view, where it is a key, but in projects and in
// sessions it is a letter being typed into the line, and on the console
// it is one of the any-keys that continue to the processes view.
func TestAltPOpensTheListFromAnywhere(t *testing.T) {
	base := newModel(plain)
	base.now, base.width, base.height = processesNow, 120, 40
	for _, view := range []int{viewProcesses, viewConsole, viewProjects, viewSessions} {
		m := base
		m.view = view
		m.filter, m.pcursor = "already typed", 3
		next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Mod: tea.ModAlt, Code: 'p'}))
		got := next.(model)
		if got.view != viewProjects {
			t.Errorf("from view %d, alt+p left conn on view %d", view, got.view)
		}
		if got.filter != "" || got.pcursor != 0 || !got.scanning {
			t.Errorf("from view %d, alt+p did not open the list afresh: filter %q cursor %d scanning %v",
				view, got.filter, got.pcursor, got.scanning)
		}
		if cmd == nil {
			t.Errorf("from view %d, alt+p did not walk the roots", view)
		}
	}

	// The console's wait on a reading is called off, or that reading would
	// land a moment later and put the processes view up over the list.
	m := base
	m.view, m.entering = viewConsole, true
	next, _ := m.Update(tea.KeyPressMsg(tea.Key{Mod: tea.ModAlt, Code: 'p'}))
	m = next.(model)
	if m.entering {
		t.Fatal("alt+p on the console left it waiting to go to the processes view")
	}
	next, _ = m.Update(processesMsg{gen: m.processesGen})
	if got := next.(model).view; got != viewProjects {
		t.Errorf("the reading the console had asked for put view %d up over the list", got)
	}

	// The kill question still takes the next key, whatever it is.
	armed := base
	armed.view = viewProcesses
	armed.kill = &pendingKill{pid: 49212, command: "zsh", sig: syscall.SIGTERM}
	next, _ = armed.Update(tea.KeyPressMsg(tea.Key{Mod: tea.ModAlt, Code: 'p'}))
	if got := next.(model); got.view != viewProcesses || got.kill != nil {
		t.Errorf("alt+p fired under an armed kill: view %d", got.view)
	}
}
