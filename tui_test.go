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
	next, ok := m.Update(blinkMsg{})
	m = next.(model)
	if m.lit {
		t.Error("the chip did not go dark on the turn")
	}
	if ok == nil {
		t.Fatal("the blink stopped at the first turn")
	}
	next, ok = m.Update(blinkMsg{})
	if m = next.(model); !m.lit || ok == nil {
		t.Error("the chip did not come back")
	}
}

// A shell conn opens is the cursor's as soon as it is read: the model
// waits for the process it started, and the reading that brings it puts
// the cursor there. A process that never appears is given up on, so it
// cannot claim the cursor later on a pid that came round again.
func TestTheCursorGoesToWhatConnOpened(t *testing.T) {
	here := []place{{path: "/w", entries: []entry{{pid: 11}, {pid: 22}}}}
	andTheShell := []place{{path: "/w", entries: []entry{{pid: 11}, {pid: 22}, {pid: 4242}}}}
	read := func(m model, places []place) model {
		next, _ := m.Update(watchMsg{places: places, gen: m.watchGen})
		return next.(model)
	}

	m := newModel(plain)
	m.view, m.cursor = viewWatch, 11
	m = read(m, here)

	next, cmd := m.Update(openedMsg{4242})
	m = next.(model)
	if m.wanted != 4242 || m.wantedLeft != waitForOpened {
		t.Errorf("the shell is not waited on: wanted %d for %d", m.wanted, m.wantedLeft)
	}
	if cmd == nil {
		t.Error("the watch was not read again at once")
	}

	// It is not there yet: the cursor stays where it was.
	m = read(m, here)
	if m.cursor != 11 || m.wanted != 4242 {
		t.Errorf("cursor %d, still waiting on %d", m.cursor, m.wanted)
	}
	// The reading that brings it moves the cursor, and the wait is over.
	m = read(m, andTheShell)
	if m.cursor != 4242 || m.wanted != 0 {
		t.Errorf("cursor %d, wanted %d: the shell should have it", m.cursor, m.wanted)
	}

	// A shell that never comes up is given up on, and the cursor is left
	// where the user had it.
	m.cursor, m.wanted, m.wantedLeft = 11, 9999, waitForOpened
	for range waitForOpened {
		m = read(m, here)
	}
	if m.wanted != 0 {
		t.Errorf("still waiting on %d after %d readings", m.wanted, waitForOpened)
	}
	m = read(m, append(here, place{path: "/x", entries: []entry{{pid: 9999}}}))
	if m.cursor == 9999 {
		t.Error("a pid that came round later took the cursor")
	}
}
