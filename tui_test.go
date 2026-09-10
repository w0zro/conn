package main

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// The console comes on in stages: the header at once, the readout when
// the station is in hand and its beat has passed, the screen's check
// alone, then the rest; a key skips to the end; a key at the end
// continues to the watch; the clock turns on the second.
func TestProgramComesOnInStages(t *testing.T) {
	m := model{head: station{build: testStation.build, session: session{user: "w0zro", host: "station"}}, now: testNow, p: plain}
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
	if cmd == nil || m.due || !has("HOST ...... STATION") || has("SCREEN") {
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
	if lastStage(m.report()) != stageChecks+8 {
		t.Errorf("last stage is %d", lastStage(m.report()))
	}
	if next, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"}); cmd == nil || next.(model).view != viewWatch {
		t.Errorf("a key at the end should continue to the watch")
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
	m := model{head: station{build: testStation.build}, now: testNow, p: plain, width: 120, height: 40}
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
	// Off the console the blink stops, lit; coming back to the console
	// starts it again; a turn from an earlier stay is dropped.
	m.view = viewWatch
	next, ok = m.Update(blinkMsg{gen: m.blinkGen})
	if m = next.(model); !m.lit || ok != nil {
		t.Error("the blink went on off the console")
	}
	next, ok = m.key("c")
	if m = next.(model); m.view != viewConsole || ok == nil {
		t.Error("c did not start the blink again")
	}
	next, ok = m.Update(blinkMsg{gen: m.blinkGen - 1})
	if m = next.(model); !m.lit || ok != nil {
		t.Error("a turn from an earlier stay was not dropped")
	}
}

// A reading that finds home without its slot has the slot opened; a
// reading with the slot only ticks.
func TestAHomeWithoutItsSlotGetsOne(t *testing.T) {
	m := model{p: plain, width: 48, height: 40, view: viewWatch, inside: true, srv: &server{tmux: "/nonexistent/tmux"}}
	_, cmd := m.Update(watchMsg{noSlot: true})
	if cmd == nil {
		t.Fatal("no command for a home without its slot")
	}
	if _, cmd := m.Update(watchMsg{slot: "ttys009"}); cmd == nil {
		t.Error("a home with its slot should still tick")
	}
}

// A shell conn opens is the cursor's once the process table has it. The
// slot is marked at once; until the reading brings the shell the watch
// reads soon rather than at its pace, and the cursor stays where it was;
// a shell that never comes is given up on when the wait is out.
func TestTheCursorGoesToTheShellOnceItIsRead(t *testing.T) {
	here := []place{{path: "/w", entries: []entry{{pid: 11}, {pid: 22}}}}
	read := func(m model, places []place) model {
		next, _ := m.Update(watchMsg{places: places, gen: m.watchGen})
		return next.(model)
	}
	m := newModel(plain)
	m.view, m.cursor, m.now = viewWatch, 11, time.Now()
	m = read(m, here)

	next, cmd := m.Update(openedMsg{shell: shell{pane: pane{id: "%9", tty: "ttys009"}, pid: 4242}})
	m = next.(model)
	if cmd == nil || m.awaited != 4242 || m.slot != "ttys009" {
		t.Errorf("after opening: cmd %v, awaited %d, slot %q", cmd != nil, m.awaited, m.slot)
	}
	// A reading without it yet leaves the cursor, and the next read is soon.
	m = read(m, here)
	if m.cursor != 11 || m.awaited != 4242 {
		t.Errorf("before the shell is read: cursor %d, awaited %d", m.cursor, m.awaited)
	}
	if next, _ := m.Update(watchTickMsg{gen: m.watchGen}); next == nil {
		t.Error("the tick should read")
	}
	// The reading that brings the shell puts the cursor on it.
	withIt := []place{{path: "/w", entries: []entry{{pid: 4242, kind: kindShell}, {pid: 11}, {pid: 22}}}}
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

// Reaching a process puts it in the slot, and conn knows that without
// reading the server back: the row says it is the one shown at once,
// and the row that was shown stops saying so.
func TestTheReachedRowIsTheSlotAtOnce(t *testing.T) {
	m := newModel(plain)
	m.view, m.slot = viewWatch, "ttys001"
	m.places = []place{{path: "/w", entries: []entry{{pid: 11, tty: "ttys001"}, {pid: 22, tty: "ttys002"}}}}
	m.panes = map[string]pane{"ttys001": {id: "%1", tty: "ttys001"}, "ttys002": {id: "%2", tty: "ttys002"}}

	gen := m.watchGen
	next, cmd := m.Update(reachedMsg{"ttys002"})
	m = next.(model)
	if m.slot != "ttys002" {
		t.Errorf("the slot is %q, not the reached terminal", m.slot)
	}
	if cmd == nil || m.watchGen == gen {
		t.Error("the watch was not read again after reaching")
	}
	// The watch says so: the reached row is shown, the one it replaced
	// is not.
	w := m.watchReport()
	for _, pl := range w.places {
		for _, r := range pl.rows {
			if r.tty == "ttys002" && !r.shown {
				t.Error("the reached row does not read as the one in the slot")
			}
			if r.tty == "ttys001" && r.shown {
				t.Error("the row that left the slot still reads as shown")
			}
		}
	}
}
