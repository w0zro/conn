package main

import (
	"maps"
	"strings"
	"syscall"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// answered runs a command and passes back what it answered. A key
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
	m := model{head: station{build: testStation.build, login: login{user: "w0zro", host: "station"}}, now: testNow, p: plain, ticking: true,
		roots: rooting{real: []string{"/Users/w0zro/projects"}}} // told where the work is; see toRoots
	m.width, m.height = 120, 40
	view := func() string { return m.View().Content }
	has := func(s string) bool { return strings.Contains(view(), s) }
	if !has("STATION  W0ZRO@STATION") || !has("CONN 0.7.0 (devel)") || has("HOST ...") || has("SCREEN") {
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
	// Seven checks conn always makes, the config file's own, the two
	// roots on a line each now this terminal has the rows, and two
	// tools; the screen's own check and the verdict are the two stages
	// past them.
	if lastStage(m.report()) != stageChecks+13 {
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
	next, _ = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = next.(model)
	next, _ = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = next.(model)
	next, _ = m.Update(processesMsg{gen: m.processesGen})
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
	_, cmd := m.Update(processesMsg{bay: "ttys009", bayDead: true})
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
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Text: "a"}))
	m = next.(model)
	// Nothing is opened and nothing is waited for.
	if m.awaited != 0 {
		t.Errorf("off any project: awaited %d", m.awaited)
	}
}

// alt+A opens the sessions view over what claude left suspended at the
// project under the cursor, asking for that project's own directory
// alone. Plain A is not bound to it — a shift chord costs the same as
// an alt one, so there is no reason to answer to both.
func TestAltAOpensSessionsAtTheProject(t *testing.T) {
	m := newModel(plain)
	m.view = viewProcesses
	m.projects = []project{{path: "/w", entries: []entry{{pid: 11, tty: "ttys001"}}}}
	m.cursor = 11

	next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "alt+shift+a"}))
	m = next.(model)
	if cmd != nil || m.view != viewProcesses {
		t.Errorf("outside the server: cmd %v, view %d", cmd != nil, m.view)
	}

	m.inside = true
	next, cmd = m.Update(tea.KeyPressMsg(tea.Key{Text: "alt+shift+a"}))
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

	// In the processes view A is the sessions too: the capital of the
	// contact's key.
	m.view = viewProcesses
	next, cmd = m.Update(tea.KeyPressMsg(tea.Key{Text: "A"}))
	m = next.(model)
	if m.view != viewSessions || cmd == nil {
		t.Errorf("A did not open the sessions: view %d, cmd %v", m.view, cmd != nil)
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
	if !strings.Contains(m.kill.prompt, "kill -TERM 11 · claude?") {
		t.Errorf("the question: kill %+v", m.kill)
	}

	// A bare shell — nothing running in it to lose — is armed for SIGKILL
	// instead, since it is proven to ignore the gentler signals.
	m.projects = []project{{path: "/w", entries: []entry{{pid: 22, kind: kindShell, command: "zsh"}}}}
	m.cursor, m.kill = 22, nil
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Text: "x"}))
	m = next.(model)
	if m.kill == nil || m.kill.sig != syscall.SIGKILL || !strings.Contains(m.kill.prompt, "kill -KILL 22 · zsh?") {
		t.Errorf("arming a shell: kill %+v", m.kill)
	}

	// A shell whose rows are folded says what it runs, and x on it ends
	// that — the command, looking through the bash -c — and not the
	// shell.
	m.tree = []project{{path: "/w", entries: []entry{
		{pid: 30, kind: kindShell, command: "zsh", typed: "zsh"},
		{pid: 31, kind: kindShell, command: "bash -c go test ./...", typed: "bash -c go test ./...", depth: 1},
		{pid: 32, kind: kindRun, command: "go test ./...", typed: "go test ./...", depth: 2},
	}}}
	m.projects = fold(m.tree)
	m.cursor, m.kill = 30, nil
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Text: "x"}))
	m = next.(model)
	if m.kill == nil || m.kill.pid != 32 || m.kill.sig != syscall.SIGTERM || !strings.Contains(m.kill.prompt, "kill -TERM 32 · go?") {
		t.Errorf("arming a folded shell: kill %+v", m.kill)
	}

	m.projects, m.tree, m.kill = nil, nil, nil
	next, cmd = m.Update(tea.KeyPressMsg(tea.Key{Text: "x"}))
	m = next.(model)
	if cmd != nil || m.kill != nil {
		t.Errorf("off any entry: cmd %v, kill %v", cmd != nil, m.kill)
	}
}

// x on a declared process that is up arms ctrl-c in its pane, with the
// pane to close once the end is recorded, so that y takes the row to
// DOWN in one move; on one that has ended, the pane alone, since there
// is nothing left to stop.
func TestXOnADeclaredProcessCarriesItsPane(t *testing.T) {
	m := newModel(plain)
	m.view = viewProcesses
	mark := markDeclared("/w", "web")
	m.projects = []project{{path: "/w", entries: []entry{
		{pid: 40, kind: kindRun, command: "web · npm run dev", typed: "web · npm run dev", tty: "/dev/ttys009", declared: mark},
	}}}
	m.panes = map[string]pane{"/dev/ttys009": {id: "%7", tty: "/dev/ttys009", declared: mark}}
	m.cursor = 40

	next, _ := m.Update(tea.KeyPressMsg(tea.Key{Text: "x"}))
	m = next.(model)
	if m.kill == nil || !m.kill.interrupt || m.kill.pane != "%7" || m.kill.pid != 0 {
		t.Fatalf("arming an up declaration: kill %+v", m.kill)
	}
	if !strings.Contains(m.kill.prompt, "tmux send-keys -t %7 C-c · web?") {
		t.Errorf("the question: kill %+v", m.kill)
	}

	m.panes["/dev/ttys009"] = pane{id: "%7", tty: "/dev/ttys009", declared: mark, exit: "0"}
	m.kill = nil
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Text: "x"}))
	m = next.(model)
	if m.kill == nil || m.kill.interrupt || m.kill.pane != "%7" || !strings.Contains(m.kill.prompt, "kill-pane %7 · web?") {
		t.Errorf("arming an ended declaration: kill %+v", m.kill)
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
	next, _ := m.Update(tea.KeyPressMsg(tea.Key{Text: "j"}))
	m = next.(model)
	if m.kill != nil || m.cursor != 11 {
		t.Errorf("cancelled: kill %v, cursor %d", m.kill, m.cursor)
	}

	// x again is any other key, and withdraws it: the confirmation is
	// tmux's, and y is the yes.
	m.kill = &pendingKill{pid: 11, command: "claude", sig: syscall.SIGTERM}
	next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "x"}))
	m = next.(model)
	if m.kill != nil || cmd != nil {
		t.Errorf("x on the question: kill %v, cmd %v", m.kill, cmd != nil)
	}
	m.kill = &pendingKill{pid: 11, command: "claude", sig: syscall.SIGTERM}
	next, cmd = m.Update(tea.KeyPressMsg(tea.Key{Text: "y"}))
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
	if m.find.text != "conn" || m.view != viewProjects {
		t.Fatalf("typed: filter %q, view %d", m.find.text, m.view)
	}
	if rows := m.projectRows(); len(rows) != 2 || rows[1].name != "conn" {
		t.Errorf("conn leaves %d rows", len(rows))
	}
	// The cursor is held within them, and backspace widens them again.
	for range 5 {
		m, _ = key(m, "down")
	}
	if m.find.at != 1 {
		t.Errorf("the cursor ran to %d of 2 rows", m.find.at)
	}
	m, _ = key(m, "ctrl+p")
	if m.find.at != 0 {
		t.Errorf("ctrl+p left the cursor at %d", m.find.at)
	}
	m, _ = key(m, "backspace")
	if m.find.text != "con" || m.find.at != 0 {
		t.Errorf("after backspace: filter %q, cursor %d", m.find.text, m.find.at)
	}
	m, _ = key(m, "ctrl+u")
	if m.find.text != "" {
		t.Errorf("ctrl+u left %q", m.find.text)
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
	m.view, m.walked, m.find.at = viewProjects, testProjects, 3
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

// alt+a opens claude at the row instead, the way enter opens a shell
// there — plain a is a letter to type into the filter, and ctrl+a is
// the start of the line, so this is the list's key for it.
func TestAltAOpensAnAgentAtTheProject(t *testing.T) {
	m := newModel(plain)
	m.view, m.walked, m.find.at = viewProjects, testProjects, 3
	m.inside, m.srv = true, &server{tmux: "/nonexistent/tmux"}

	next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "alt+a"}))
	m = next.(model)
	if m.view != viewProcesses || cmd == nil {
		t.Fatalf("in the server: view %d, cmd %v", m.view, cmd != nil)
	}
	if msg, ok := cmd().(tea.BatchMsg); !ok || len(msg) != 2 {
		t.Errorf("alt+a did not both read the processes view and open the contact: %T", cmd())
	}
}

// alt+A opens the sessions view at the row's own directory, or, on a
// group, at every repository under it too — a transcript is filed by
// the exact directory it was had in, not the folder that names them.
// Plain A is not bound to it, unlike ctrl+shift+a which never could be
// — a shift chord costs the same as an alt one, so there is no reason
// to give up typing a capital letter into the filter for it.
func TestAltAOpensSessionsFromProjects(t *testing.T) {
	m := newModel(plain)
	m.view, m.walked, m.find.at = viewProjects, testProjects, 0 // arboreum.io, a group of two
	m.inside = true

	next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "alt+shift+a"}))
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
	m.view, m.find.text = viewProjects, ""
	next, cmd = m.Update(tea.KeyPressMsg(tea.Key{Text: "A"}))
	m = next.(model)
	if m.view != viewProjects || m.find.text != "A" || cmd != nil {
		t.Errorf("A did not type: view %d, filter %q, cmd %v", m.view, m.find.text, cmd != nil)
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
	if m.rfind.text != "topic" || len(m.sessionsRows()) != 1 {
		t.Fatalf("typed: filter %q, %d rows", m.rfind.text, len(m.sessionsRows()))
	}
	m, _ = key(m, "backspace")
	if m.rfind.text != "topi" {
		t.Errorf("after backspace: filter %q", m.rfind.text)
	}
	m, _ = key(m, "ctrl+u")
	if m.rfind.text != "" || len(m.sessionsRows()) != 2 {
		t.Errorf("ctrl+u left filter %q, %d rows", m.rfind.text, len(m.sessionsRows()))
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
	m.view, m.sessions, m.rfind.at = viewSessions, testSessions2, 1
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
		m.find.text, m.find.at = "already typed", 3
		next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Mod: tea.ModAlt, Code: 'p'}))
		got := next.(model)
		if got.view != viewProjects {
			t.Errorf("from view %d, alt+p left conn on view %d", view, got.view)
		}
		if got.find.text != "" || got.find.at != 0 || !got.scanning {
			t.Errorf("from view %d, alt+p did not open the list afresh: filter %q cursor %d scanning %v",
				view, got.find.text, got.find.at, got.scanning)
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

// A shell, a contact and the sessions at the project the panel is
// looking at, each reachable from anywhere in the server. They are the
// keys the prefix then s, then a and then A send, and each has to
// mean the one thing in every view the panel can be in, since in the
// list and in the sessions view a plain s or a is a letter being typed.
func TestTheChordsOpenAtWhateverThePanelIsLookingAt(t *testing.T) {
	panel := func(view int) model {
		m := newModel(plain)
		m.inside, m.view = true, view
		m.projects = []project{{path: "/w", entries: []entry{{pid: 11, tty: "ttys001"}}}}
		m.cursor = 11
		m.walked = []projectRow{{path: "/w/repo", name: "repo"}}
		m.sessionsProject, m.sessionsDirs = "/w/had", []string{"/w/had"}
		return m
	}
	press := func(m model, k string) (model, tea.Cmd) {
		next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: k}))
		return next.(model), cmd
	}

	// The processes view answers with the cursor's project, and stays
	// where it is: the shell it opens shows in the view it is already on.
	m, cmd := press(panel(viewProcesses), "alt+s")
	if m.view != viewProcesses || cmd == nil {
		t.Errorf("a shell from the processes view: view %d, cmd %v", m.view, cmd != nil)
	}
	if m, cmd = press(panel(viewProcesses), "alt+a"); m.view != viewProcesses || cmd == nil {
		t.Errorf("a contact from the processes view: view %d, cmd %v", m.view, cmd != nil)
	}

	// The list answers with the row under its cursor and comes back to
	// the processes view, which is where what it opened will show.
	if m, cmd = press(panel(viewProjects), "alt+s"); m.view != viewProcesses || cmd == nil {
		t.Errorf("a shell from the list: view %d, cmd %v", m.view, cmd != nil)
	}
	if m, cmd = press(panel(viewProjects), "alt+a"); m.view != viewProcesses || cmd == nil {
		t.Errorf("a contact from the list: view %d, cmd %v", m.view, cmd != nil)
	}

	// In a line typed into, ctrl is readline's: ctrl+s opened a shell
	// once, and opens nothing now, in the list or anywhere.
	for _, view := range []int{viewProcesses, viewProjects, viewSessions} {
		if m, cmd = press(panel(view), "ctrl+s"); cmd != nil && m.view == viewProcesses && view != viewProcesses {
			t.Errorf("ctrl+s from view %d opened a shell", view)
		}
	}

	// The sessions view answers with the project those sessions are
	// already about, so a shell can be opened beside what is being read.
	if m, cmd = press(panel(viewSessions), "alt+s"); m.view != viewProcesses || cmd == nil {
		t.Errorf("a shell from the sessions view: view %d, cmd %v", m.view, cmd != nil)
	}

	// alt+A goes to the sessions view rather than coming back, since it
	// is somewhere to be and not something to open.
	if m, _ = press(panel(viewProcesses), "alt+shift+a"); m.view != viewSessions || m.sessionsProject != "/w" {
		t.Errorf("sessions from the processes view: view %d, at %q", m.view, m.sessionsProject)
	}
	if m, _ = press(panel(viewProjects), "alt+shift+a"); m.view != viewSessions || m.sessionsProject != "/w/repo" {
		t.Errorf("sessions from the list: view %d, at %q", m.view, m.sessionsProject)
	}

	// The console is looking at the machine and not at a project, and
	// the keys have nothing to open. A plain s or a is still a letter
	// wherever a letter is being typed.
	if m, cmd = press(panel(viewConsole), "alt+s"); cmd != nil {
		t.Error("a shell was opened from the console")
	}
	if m, _ = press(panel(viewProjects), "a"); m.find.text != "a" {
		t.Errorf("a plain a stopped being a letter in the list: filter %q", m.find.text)
	}
	if m, _ = press(panel(viewProjects), "s"); m.find.text != "s" {
		t.Errorf("a plain s stopped being a letter in the list: filter %q", m.find.text)
	}
}

// Down and up the processes conn can actually put in front of you,
// which the prefix then j and then k send. A row conn only reports is
// stepped over: the keys travel with the cursor, and a row there is no
// pane for is nowhere to send them. A tree is one stop, not one a row,
// since the pane holds the whole of it.
func TestTheRingWalksOnlyWhatCanBeReached(t *testing.T) {
	m := newModel(plain)
	m.inside, m.view = true, viewProcesses
	m.srv = &server{tmux: "/nonexistent/tmux", socket: "/tmp/none"}
	m.projects = []project{
		{path: "/a", entries: []entry{
			{pid: 10, tty: "ttys001", kind: kindShell},   // reached
			{pid: 11, tty: "ttys002", kind: kindShell},   // only reported
			{pid: 12, tty: "ttys003", kind: kindContact}, // reached, and a tree
			{pid: 13, tty: "ttys003", kind: kindRun, depth: 1},
			{pid: 14, tty: "ttys004", kind: kindShell}, // a hold, which is conn's own
		}},
	}
	m.panes = map[string]pane{
		"ttys001": {id: "%1", tty: "ttys001"},
		"ttys003": {id: "%3", tty: "ttys003"},
		"ttys004": {id: "%4", tty: "ttys004", hold: true},
	}

	round, at := m.reachableRound()
	if len(round) != 2 || round[0].pid != 10 || round[1].pid != 12 || at[0] != 0 || at[1] != 2 {
		t.Fatalf("the ring is not the two panes with work in them: %+v at %v", round, at)
	}

	press := func(m model, k string) model {
		next, _ := m.Update(tea.KeyPressMsg(tea.Key{Text: k}))
		return next.(model)
	}
	// Down from the first reaches the tree's head, not the row under it.
	m.cursor = 10
	if got := press(m, "alt+j").cursor; got != 12 {
		t.Errorf("down from the first: cursor %d", got)
	}
	// And round again from the end, both ways.
	m.cursor = 12
	if got := press(m, "alt+j").cursor; got != 10 {
		t.Errorf("down from the last: cursor %d", got)
	}
	m.cursor = 10
	if got := press(m, "alt+k").cursor; got != 12 {
		t.Errorf("up from the first: cursor %d", got)
	}
	// From a row conn cannot reach, which is between the two.
	m.cursor = 11
	if got := press(m, "alt+j").cursor; got != 12 {
		t.Errorf("down from a row conn only reports: cursor %d", got)
	}
	if got := press(m, "alt+k").cursor; got != 10 {
		t.Errorf("up from a row conn only reports: cursor %d", got)
	}
	// From inside a tree, up is the head of the tree it is inside.
	m.cursor = 13
	if got := press(m, "alt+k").cursor; got != 12 {
		t.Errorf("up from inside a tree: cursor %d", got)
	}

	// The ring carries the keys with it: the pane goes into the bay,
	// which against no tmux is a reach that reaches nothing.
	m.cursor = 10
	if _, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "alt+j"})); cmd == nil {
		t.Error("the ring moved the cursor without putting the pane in the bay")
	}

	// With nothing conn holds there is nowhere to go, which is every row
	// outside conn's own server.
	m.panes = nil
	if got := press(m, "alt+j").cursor; got != 10 {
		t.Errorf("with no panes the cursor moved to %d", got)
	}
}

// A chord brings the keys to the panel out of whatever pane they were
// in. Cancelling puts them back: esc from the list and from the
// sessions view returns to the processes view, and to the pane the
// chord came from. Pressed on the panel there is nowhere to go back to.
func TestCancellingAChordGivesTheKeysBack(t *testing.T) {
	m := newModel(plain)
	m.inside, m.view, m.srv = true, viewProjects, &server{tmux: "/nonexistent/tmux", socket: "/tmp/none"}

	// Cancelling a visit a chord brought about asks for the pane back.
	m.from = "%7"
	next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "esc"}))
	m = next.(model)
	if m.view != viewProcesses || m.from != "" {
		t.Errorf("esc from the list: view %d, from %q", m.view, m.from)
	}
	if cmd == nil {
		t.Fatal("esc from a chord's list asked for nothing")
	}
	// Two commands: the reading, and the keys going back.
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Errorf("esc from a chord's list: %T", cmd())
	}

	// The sessions view cancels the same way.
	m.view, m.from = viewSessions, "%7"
	next, cmd = m.Update(tea.KeyPressMsg(tea.Key{Text: "esc"}))
	if m = next.(model); m.view != viewProcesses || cmd == nil {
		t.Errorf("esc from a chord's sessions view: view %d, cmd %v", m.view, cmd != nil)
	}

	// With nothing to go back to, esc only comes back to the view.
	m.view, m.from = viewProjects, ""
	next, cmd = m.Update(tea.KeyPressMsg(tea.Key{Text: "esc"}))
	m = next.(model)
	if _, batched := cmd().(tea.BatchMsg); batched {
		t.Error("esc with nowhere to go back to asked for the keys anyway")
	}

	// p is pressed on the panel, so it leaves nothing to go back to.
	m.view, m.from = viewProcesses, "%7"
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Text: "p"}))
	if m = next.(model); m.view != viewProjects || m.from != "" {
		t.Errorf("p on the panel: view %d, from %q", m.view, m.from)
	}
}

// gg and G are the ends of the list, where j and k are its steps. They
// count process rows across every project, the rows j and k walk, and
// not the titles above them; the first g of gg is nothing on its own,
// and any other key after it is simply that key. vim's M is not here:
// it means the middle of the screen, and the panel does not yet know
// what is on its screen, so a key of that name would teach a wrong
// meaning.
func TestTheMotionsReachTheEnds(t *testing.T) {
	m := newModel(plain)
	m.view = viewProcesses
	m.projects = []project{
		{path: "/w", entries: []entry{{pid: 11}, {pid: 22}}},
		{path: "/x", entries: []entry{{pid: 33}, {pid: 44}, {pid: 55}}},
	}
	m.cursor, m.cursorAt = 11, 0

	press := func(k string) {
		next, _ := m.Update(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
		m = next.(model)
	}
	for _, c := range []struct {
		keys []string
		want int
	}{
		{[]string{"G"}, 55},      // the last row, across the projects
		{[]string{"g", "g"}, 11}, // the first, and only on the second g
		{[]string{"G", "g"}, 55}, // a lone g moves nothing
		{[]string{"g", "j"}, 22}, // and the key after it is its own
		{[]string{"g", "g", "G"}, 55},
	} {
		m.cursor, m.cursorAt = 11, 0
		for _, k := range c.keys {
			press(k)
		}
		if m.cursor != c.want {
			t.Errorf("%v put the cursor on %d, want %d", c.keys, m.cursor, c.want)
		}
	}

	// With nothing running there is nowhere to go and nothing to answer.
	m.projects, m.cursor, m.cursorAt = nil, 0, 0
	press("G")
	if m.cursor != 0 {
		t.Errorf("with nothing running: cursor %d", m.cursor)
	}
}

// The list holds the processes running in each project, so enter on one
// of those rows goes into it — its pane in the bay and the keys in it,
// the way enter does on the row in the processes view — where enter on
// a project row starts a shell there. That is what makes p and the
// prefix and p the way to a process on a machine with more of them than
// there are rows to draw.
func TestEnterGoesIntoTheProcessUnderTheCursor(t *testing.T) {
	m := newModel(plain)
	m.view, m.walked, m.projects, m.panes = viewProjects, testProjects, testRunning, testPanes
	m.inside, m.srv = true, &server{tmux: "/nonexistent/tmux", socket: "/tmp/none"}
	rows := m.projectRows()

	at := func(pid int) int {
		for i, r := range rows {
			if r.pid == pid {
				return i
			}
		}
		t.Fatalf("no row for %d in %v", pid, rowNames(rows))
		return 0
	}
	// The contact waiting in conn: the view goes back to the processes
	// view, which is where a pane in the bay shows, and the pane is
	// reached rather than a shell being opened.
	m.find.at = at(11)
	next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	got := next.(model)
	if got.view != viewProcesses || cmd == nil {
		t.Fatalf("on a process row: view %d, cmd %v", got.view, cmd != nil)
	}
	// Against no tmux the reach reaches nothing, and nothing else was
	// started in its place.
	if _, ok := answered(cmd).(openedMsg); ok {
		t.Error("enter on a process row opened a shell instead of going into it")
	}

	// The shell conn holds no pane for is not a row at all, so there is
	// no way to press enter on one.
	for _, r := range rows {
		if r.pid == 44 {
			t.Error("the list holds a process conn cannot reach")
		}
	}

	// The heading for work off every project is still a directory, and
	// enter starts a shell there like any other row. Only work conn could
	// not place at all is a heading and not a place, and enter on it
	// opens nothing.
	m.find.at = at(55) - 1
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if got = next.(model); got.view != viewProcesses {
		t.Errorf("on ~/Downloads the panel stayed at view %d", got.view)
	}
	m.projects = []project{{entries: []entry{{pid: 66, kind: "SHELL", command: "zsh", tty: "ttys006"}}}}
	m.panes = map[string]pane{"ttys006": {id: "%6", tty: "ttys006"}}
	m.view, m.find.at = viewProjects, len(m.projectRows())-2 // the NO PROJECT heading
	next, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if got = next.(model); got.view != viewProjects || answered(cmd) != nil {
		t.Errorf("on NO PROJECT: view %d, %T", got.view, answered(cmd))
	}
}

// The list is read for as long as it is up, so its cursor cannot be an
// index alone: it holds the row it was on — a process by its pid, a
// project by its path — as processes come and go under it.
func TestTheListsCursorHoldsItsRowAcrossAReading(t *testing.T) {
	m := newModel(plain)
	m.view, m.walked, m.projects, m.panes = viewProjects, testProjects, testRunning, testPanes
	m.find.at = 0
	for i, r := range m.projectRows() {
		if r.pid == 22 { // the shell in conn, below the contact waiting there
			m.find.at = i
		}
	}
	was := m.find.at

	// The contact above it ends, and every row below moves up one.
	thinner := append([]project{{path: testRunning[0].path, entries: testRunning[0].entries[1:]}}, testRunning[1:]...)
	next, _ := m.Update(processesMsg{gen: m.processesGen, projects: thinner, panes: testPanes})
	m = next.(model)
	if m.find.at != was-1 {
		t.Fatalf("the cursor is on row %d, want %d", m.find.at, was-1)
	}
	if row, ok := m.atCursor(); !ok || row.pid != 22 {
		t.Errorf("the cursor stands on %+v, not the shell it was on", row)
	}

	// And the process it was on ending leaves it where that row was.
	next, _ = m.Update(processesMsg{gen: m.processesGen, projects: nil, panes: nil})
	m = next.(model)
	if row, ok := m.atCursor(); !ok || row.name != "conn" {
		t.Errorf("with the process gone the cursor stands on %+v", row)
	}
}

// esc in the processes view goes back into the last process the
// workspace held. Walking the rows is reading, not moving: the page
// takes the bay while the keys are on the panel and the work goes back
// to a window of its own, so what to come back to is remembered rather
// than read off the bay. The cursor is not consulted — enter is for the
// row you are looking at, esc for the process you came out of.
func TestEscGoesBackIntoTheLastProcess(t *testing.T) {
	m := newModel(plain)
	m.view, m.projects, m.panes = viewProcesses, testRunning, testPanes
	m.inside, m.srv = true, &server{tmux: "/nonexistent/tmux", socket: "/tmp/none"}
	// The status line has been said once already, so what a key asks for
	// here is the key's own asking and not the line's first telling.
	m.said, m.saidKeys, m.saidStation, m.saidUp, m.saidBar = true, m.keys(), m.station(), m.upWord(), m.bar()

	press := func(m model, k string) (model, tea.Cmd) {
		next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: k}))
		return next.(model), cmd
	}
	// Nothing has been worked in yet, so there is nowhere to go back to
	// and esc asks for nothing.
	if got, cmd := press(m, "esc"); cmd != nil || got.lastIn != "" {
		t.Errorf("esc with nothing worked in: lastIn %q, cmd %v", got.lastIn, cmd != nil)
	}

	// Going into the contact is what makes it the one to come back to.
	next, _ := m.Update(reachedMsg{"ttys001"})
	m = next.(model)
	if m.lastIn != "ttys001" {
		t.Fatalf("after reaching, lastIn is %q", m.lastIn)
	}

	// The keys come to the panel and the page takes the bay: the work is
	// out of the bay but it is still where the operator was.
	m.focused = true
	next, _ = m.Update(processesMsg{
		gen:      m.processesGen,
		projects: testRunning,
		panes:    withPane(testPanes, pane{id: "%9", tty: "ttys009", hold: true, readout: true}),
		bay:      "ttys009",
	})
	if m = next.(model); m.lastIn != "ttys001" {
		t.Errorf("the page took the bay and lastIn with it: %q", m.lastIn)
	}

	// And a hold standing in an empty bay is conn's own furniture too.
	next, _ = m.Update(processesMsg{
		gen:      m.processesGen,
		projects: testRunning,
		panes:    withPane(testPanes, pane{id: "%8", tty: "ttys008", hold: true}),
		bay:      "ttys008",
	})
	if m = next.(model); m.lastIn != "ttys001" {
		t.Errorf("a hold took lastIn: %q", m.lastIn)
	}

	// The cursor walks the list; the work stands where it was.
	was := m.cursor
	for range 3 {
		m, _ = press(m, "j")
	}
	if m.cursor == was {
		t.Fatal("j did not move the cursor")
	}
	got, cmd := press(m, "esc")
	if cmd == nil {
		t.Error("esc asked for nothing with a process to go back into")
	}
	if got.cursor != m.cursor {
		t.Errorf("esc moved the cursor to %d", got.cursor)
	}
	if got.lastIn != "ttys001" {
		t.Errorf("esc went back into %q", got.lastIn)
	}

	// Work that has since ended is nowhere to go: conn holds no pane for
	// it, and esc does nothing rather than reaching at a gone id.
	m.lastIn = "ttys004" // the process conn can only report
	if _, cmd := press(m, "esc"); cmd != nil {
		t.Error("esc reached for a process conn holds no pane for")
	}
	m.lastIn, m.panes = "ttys001", withPane(testPanes, pane{id: "%1", tty: "ttys001", dead: true})
	if _, cmd := press(m, "esc"); cmd != nil {
		t.Error("esc reached into a pane whose process has ended")
	}
}

// withPane is testPanes with one more pane in it, the map left alone.
func withPane(panes map[string]pane, p pane) map[string]pane {
	out := maps.Clone(panes)
	out[p.tty] = p
	return out
}

// The other process is the work before this work. The page sits in the
// workspace between every two things you go into — it takes it the
// moment the keys reach the panel — so a rule that read the other off
// the bay read the page, refused it as the furniture it is, and
// remembered nothing at all. The chord then did nothing for as long as
// the page existed, on every row and not only on a service's.
func TestTheOtherProcessIsTheWorkBeforeThisWork(t *testing.T) {
	m := newModel(plain)
	m.inside, m.view = true, viewProcesses
	m.srv = &server{tmux: "/nonexistent/tmux", socket: "/tmp/none"}
	m.panes = map[string]pane{
		"ttysa": {id: "%1", tty: "ttysa"},
		"ttysb": {id: "%2", tty: "ttysb"},
		"ttysp": {id: "%9", tty: "ttysp", hold: true, readout: true},
	}
	page := func(m model) model {
		// The keys come back to the panel and the page takes the
		// workspace, which is what happens between any two things.
		next, _ := m.Update(processesMsg{gen: m.processesGen, panes: m.panes, bay: "ttysp", bayReadout: true})
		return next.(model)
	}
	into := func(m model, tty string) model {
		next, _ := m.Update(reachedMsg{tty})
		return next.(model)
	}

	m = into(m, "ttysa")
	m = page(m)
	m = into(m, "ttysb")
	if m.lastBay != "ttysa" {
		t.Fatalf("the other is %q, not the work before this work", m.lastBay)
	}
	if m.lastIn != "ttysb" {
		t.Errorf("the work is %q", m.lastIn)
	}

	// And the chord goes there, rather than finding nothing to go to.
	next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "alt+o"}))
	if cmd == nil {
		t.Error("the other-process chord asked for nothing")
	}
	_ = next

	// Pressed again it is where it started: going into the other makes
	// the one just left the other in its turn.
	m = page(m)
	m = into(m, "ttysa")
	if m.lastBay != "ttysb" {
		t.Errorf("after going back, the other is %q", m.lastBay)
	}

	// Work whose pane has ended is nowhere to be sent, and is asked the
	// way every other road into a pane asks it.
	m.panes["ttysb"] = pane{id: "%2", tty: "ttysb", dead: true}
	if _, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "alt+o"})); cmd != nil {
		t.Error("the chord reached into a pane whose process has ended")
	}
}

// A declared process is brought up from its row: enter on a down row
// opens its pane and goes in, u brings up everything the project
// declares and does not have running, and from the list alt+u does the
// same and comes to the processes view. Outside the server nothing is
// opened, and a project with no file answers nothing.
func TestADeclaredProcessIsBroughtUpFromItsRow(t *testing.T) {
	app := "/Users/w0zro/projects/w0zro/app"
	down := entry{pid: declaredPID(app, "web"), kind: kindRun, command: "web · npm run dev", status: statusDown, declared: markDeclared(app, "web")}
	m := newModel(plain)
	m.view = viewProcesses
	m.projects = []project{{path: app, entries: []entry{down}}}
	m.declared = map[string]declared{app: {list: []declaration{{name: "web", command: "npm run dev"}}}}
	m.cursor = down.pid

	next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "enter"}))
	m = next.(model)
	if cmd != nil {
		t.Error("outside the server, enter on a down row opened something")
	}
	m.inside, m.srv = true, &server{tmux: "/nonexistent/tmux"}
	next, cmd = m.Update(tea.KeyPressMsg(tea.Key{Text: "enter"}))
	m = next.(model)
	if cmd == nil {
		t.Fatal("in the server, enter on a down row opened nothing")
	}
	if _, ok := answered(cmd).(openedMsg); ok {
		t.Error("a pane opened with no tmux to open it in")
	}

	next, cmd = m.Update(tea.KeyPressMsg(tea.Key{Text: "u"}))
	m = next.(model)
	if cmd == nil || m.view != viewProcesses {
		t.Fatalf("u: cmd %v, view %d", cmd != nil, m.view)
	}
	if msg, ok := answered(cmd).(raisedMsg); ok && len(msg.shells) != 0 {
		t.Errorf("panes opened with no tmux to open them in: %v", msg.shells)
	}

	// The panes come back parked: the cursor waits on the first of
	// them, and the table is read again.
	gen := m.processesGen
	next, cmd = m.Update(raisedMsg{shells: []shell{{pid: 500}, {pid: 501}}})
	m = next.(model)
	if m.awaited != 500 || m.processesGen != gen+1 || cmd == nil {
		t.Errorf("raised: awaited %d, gen %d from %d, cmd %v", m.awaited, m.processesGen, gen, cmd != nil)
	}

	// From the list, on the project's row.
	m.view, m.walked, m.find.at = viewProjects, []projectRow{{name: "w0zro/app", path: app}}, 0
	next, cmd = m.Update(tea.KeyPressMsg(tea.Key{Text: "alt+u"}))
	m = next.(model)
	if cmd == nil || m.view != viewProcesses {
		t.Errorf("alt+u from the list: cmd %v, view %d", cmd != nil, m.view)
	}

	// The raise reads the file itself, so a project the reading has not
	// read a file for — nothing running in it — is still asked.
	m.declared = nil
	next, cmd = m.Update(tea.KeyPressMsg(tea.Key{Text: "u"}))
	m = next.(model)
	if cmd == nil {
		t.Error("u with the file unread asked nothing")
	}
}

// x on a declared row: down, nothing to end; ended and holding its
// pane, the pane is closed, and the question says so; up, the command
// under the sh is what is asked to end, so the sh records the end.
func TestXOnADeclaredRow(t *testing.T) {
	app := "/Users/w0zro/projects/w0zro/app"
	mark := markDeclared(app, "web")
	m := newModel(plain)
	m.view, m.inside, m.srv = viewProcesses, true, &server{tmux: "/nonexistent/tmux"}
	m.projects = []project{{path: app, entries: []entry{
		{pid: declaredPID(app, "web"), kind: kindRun, status: statusDown, declared: mark},
		{pid: 300, kind: kindRun, command: "web · npm run dev", tty: "ttys003", status: statusEnded, declared: mark},
		{pid: 400, kind: kindRun, command: "web · npm run dev", tty: "ttys004", status: statusActive, declared: mark},
		{pid: 401, kind: kindRun, command: "npm run dev", tty: "ttys004", status: statusActive, depth: 1},
	}}}
	m.panes = map[string]pane{
		"ttys003": {id: "%3", tty: "ttys003", declared: mark, exit: "0"},
		"ttys004": {id: "%4", tty: "ttys004", declared: mark},
	}
	press := func(k string) tea.Cmd {
		next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: k}))
		m = next.(model)
		return cmd
	}
	m.cursor = declaredPID(app, "web")
	// The status line is told of the keys either way; what matters is
	// that no question is armed.
	press("x")
	if m.kill != nil {
		t.Errorf("x on a down row: kill %+v", m.kill)
	}
	m.cursor = 300
	press("x")
	if m.kill == nil || m.kill.pane != "%3" || !strings.Contains(m.kill.prompt, "kill-pane %3 · web?") {
		t.Fatalf("x on a held row: kill %+v", m.kill)
	}
	if cmd := press("y"); cmd == nil || m.kill != nil {
		t.Errorf("confirming the close: cmd %v, kill %+v", cmd != nil, m.kill)
	}
	m.cursor = 400
	press("x")
	if m.kill == nil || !m.kill.interrupt || m.kill.pane != "%4" || !strings.Contains(m.kill.prompt, "tmux send-keys -t %4 C-c · web?") {
		t.Errorf("x on an up row: kill %+v", m.kill)
	}
}

// z shows the whole tree and the fold of it again, at once from the
// reading held, and the status line says TREE while the tree is up.
// The reading comes folded at rest and whole once z has been pressed,
// and the tree is kept whole either way for the page.
func TestZShowsTheWholeTree(t *testing.T) {
	tree := []project{{path: "/w", entries: []entry{
		{pid: 1, kind: kindShell, typed: "zsh", status: statusActive},
		{pid: 2, kind: kindRun, typed: "go test ./...", status: statusActive, depth: 1},
	}}}
	m := newModel(plain)
	m.view = viewProcesses
	next, _ := m.Update(processesMsg{projects: fold(tree), tree: tree})
	m = next.(model)
	if rowsIn(m.projects) != 1 || len(m.tree[0].entries) != 2 || m.full {
		t.Fatalf("at rest: %d rows shown of %d, full %v", rowsIn(m.projects), len(m.tree[0].entries), m.full)
	}
	if strings.Contains(m.keys(), treeWord) {
		t.Error("the line says TREE at rest")
	}
	press := func(k string) {
		next, _ := m.Update(tea.KeyPressMsg(tea.Key{Text: k}))
		m = next.(model)
	}
	press("z")
	if !m.full || rowsIn(m.projects) != 2 || !strings.Contains(m.keys(), treeWord) {
		t.Errorf("after z: full %v, %d rows, line %q", m.full, rowsIn(m.projects), m.keys())
	}
	press("j")
	if m.cursor != 2 {
		t.Errorf("the cursor did not reach the row the tree showed: %d", m.cursor)
	}
	press("z")
	if m.full || rowsIn(m.projects) != 1 || m.cursor != 1 {
		t.Errorf("after z again: full %v, %d rows, cursor %d", m.full, rowsIn(m.projects), m.cursor)
	}
	// The reading folds at rest, and does not once the tree is asked for.
	m.full = true
	if cmd := m.readProcesses(); cmd == nil {
		t.Fatal("no reading")
	}
}

// A shell the server would not open left nothing on the screen: a key
// was pressed and nothing happened. What the server said is said
// under the rows, in its own words, until the next key.
func TestWhatTheServerWouldNotDoIsSaidUnderTheRows(t *testing.T) {
	m := newModel(plain)
	m.view, m.inside = viewProcesses, true
	next, _ := m.Update(noticeMsg{"the shell could not be opened: tmux swap-pane: can't find pane: %9"})
	m = next.(model)
	if text := texts(drawProcesses(m.processesReport(), 0, 48, 30, plain)); !strings.Contains(text, "SWAP-PANE: CAN'T FIND PANE") {
		t.Errorf("the notice is not under the rows:\n%s", text)
	}
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Text: "j"}))
	if got := next.(model); got.notice != "" {
		t.Errorf("a key did not take the notice down: %q", got.notice)
	}
}
