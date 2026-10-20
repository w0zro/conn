package main

import (
	"strings"
	"testing"

	"github.com/w0zro/conn/internal/console"
	"github.com/w0zro/conn/internal/station"
	"github.com/w0zro/conn/internal/theme"
)

// A panel that brought the server up comes up on the console, the rows
// of the stages reached and air for the rest, in the rows the foot
// leaves. A key skips to the end; a key at the end puts the console
// away, and the panel is on its projects.
func TestConsoleComesOnAndIsPutAway(t *testing.T) {
	m := model{width: 120, height: 40, g: theme.On(true)}
	m.boot = &booting{head: station.Station{Build: station.Build{Tag: "0.25.0"}, Login: station.Login{User: "w0zro", Host: "station", Home: "/Users/w0zro"}}, cfg: console.Config{Path: "/Users/w0zro/.config/conn/config.json"}, lit: true}
	lines, f := m.drawConsole()
	if len(lines) != 39 {
		t.Errorf("the console takes %d rows of a 40-row panel, not the 39 the foot leaves", len(lines))
	}
	text := strings.Join(lines, "\n")
	if !strings.Contains(text, "CONN 0.25.0") || !strings.Contains(text, "W0ZRO@STATION") {
		t.Errorf("the header is not on at the first stage:\n%s", text)
	}
	if strings.Contains(text, "START-UP CHECKS") || strings.Contains(text, console.AllNominal) {
		t.Errorf("the checks or the verdict are on before their stage:\n%s", text)
	}
	if len(f.hints) == 0 || f.hints[0].key != "any key" {
		t.Errorf("the bar says %+v", f.hints)
	}

	next, _ := m.consoleKey("j")
	m = next.(model)
	if m.boot == nil || m.boot.stage != console.LastStage(m.bootReport()) {
		t.Fatalf("a key did not skip to the verdict: %+v", m.boot)
	}
	lines, _ = m.drawConsole()
	if text := strings.Join(lines, "\n"); !strings.Contains(text, "START-UP CHECKS") || !strings.Contains(text, "NOT NOMINAL") {
		// Nothing of the machine was read, so the checks are unknown,
		// and unknown is not nominal.
		t.Errorf("the whole console is not on after the skip:\n%s", text)
	}

	next, _ = m.consoleKey("j")
	m = next.(model)
	if m.boot != nil || m.screen != onProjects {
		t.Errorf("a key at the end did not put the console away: boot %v, screen %v", m.boot, m.screen)
	}
}

// The stages come on in order: the readout waits for the station to be
// read, and comes on at once when its beat has already passed.
func TestConsoleReadoutWaitsForTheStation(t *testing.T) {
	m := model{width: 120, height: 40, g: theme.On(true), boot: &booting{lit: true}}
	next, cmd := m.bootMsg(stageDue{})
	m = next.(model)
	if m.boot.stage != console.StageHeader || !m.boot.due || cmd != nil {
		t.Fatalf("the readout came on before the station was read: stage %d, due %v", m.boot.stage, m.boot.due)
	}
	next, cmd = m.bootMsg(stationRead(station.Station{}))
	m = next.(model)
	if m.boot.stage != console.StageReadout || m.boot.due || cmd == nil {
		t.Errorf("the station read did not bring the readout on: stage %d, due %v", m.boot.stage, m.boot.due)
	}
	next, _ = m.bootMsg(stageDue{})
	if next.(model).boot.stage != console.StageChecks {
		t.Errorf("the beat after the readout is not the first check: %d", next.(model).boot.stage)
	}
}
