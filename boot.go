package main

import (
	"os"
	"path/filepath"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/w0zro/conn/internal/console"
	"github.com/w0zro/conn/internal/station"
	"github.com/w0zro/conn/internal/theme"
)

// The boot console is what the panel comes up on when it brings a
// server up: the wordmark, the station it is on, the start-up checks
// and their verdict, the whole window wide. It comes on a stage at a
// time — the header at once, from what is known before anything is
// read, then the readout once the station is read and its beat has
// passed, then the checks one by one, then the verdict — and a key
// skips to the end, or at the end goes on to the projects, where the
// workspace is made beside the panel. A panel started again over a
// server that is up comes up on no console: the server was seen come
// up once.
type booting struct {
	stamp string           // what the build stamped on the binary
	head  station.Station  // the build and the login, read before the loop
	st    *station.Station // the station, once read
	cfg   console.Config   // conn's own configuration file, as it read
	stage int              // the stage the console has come on to
	due   bool             // the readout's beat has passed and it waits on the station
	lit   bool             // the verdict's annunciator, this moment
}

// What comes to the console: the station read, a stage's beat, and the
// annunciator's turn.
type (
	stationRead station.Station
	stageDue    struct{}
	blink       struct{}
)

// The time before each stage after the header: a beat for the readout
// and the verdict, less for each check. The annunciator is lit a
// second and dark half of one.
const (
	beat      = 150 * time.Millisecond
	checkBeat = 80 * time.Millisecond
	litFor    = time.Second
	darkFor   = 500 * time.Millisecond
)

// newBooting is the console before its loop runs: the header's part of
// the station, read here, and how the configuration read.
func newBooting(home, stamp string) *booting {
	return &booting{
		stamp: stamp,
		head:  station.Station{Build: station.ReadBuild(stamp), Login: station.ReadLogin()},
		cfg:   configState(home),
		lit:   true,
	}
}

// start sets the console going: the station read off the loop, the
// readout's beat, and the annunciator.
func (b *booting) start() []tea.Cmd {
	stamp := b.stamp
	return []tea.Cmd{
		func() tea.Msg { return stationRead(station.Read(stamp)) },
		stageTick(beat),
		blinkTick(true),
	}
}

func stageTick(after time.Duration) tea.Cmd {
	return tea.Tick(after, func(time.Time) tea.Msg { return stageDue{} })
}

func blinkTick(lit bool) tea.Cmd {
	after := darkFor
	if lit {
		after = litFor
	}
	return tea.Tick(after, func(time.Time) tea.Msg { return blink{} })
}

// configState is conn's own configuration file as it read, for the
// console to say: where it is, whether there is one, what went wrong,
// and a theme it names that conn has no sheet for.
func configState(home string) console.Config {
	path := filepath.Join(configDir(home), "config.json")
	c := console.Config{Path: path}
	_, err := os.Stat(path)
	c.Present = err == nil
	cfg, err := readConfig(home)
	c.Err = err
	if err == nil && cfg.Theme != "" && !slices.Contains(theme.Names(themesDir(home)), cfg.Theme) {
		c.NoTheme = cfg.Theme
	}
	return c
}

// bootReport is the console's words as things stand: from the station
// once it is read, from the header's part of it before, with the
// annunciator as it is this moment.
func (m model) bootReport() console.Report {
	b := m.boot
	st := b.head
	if b.st != nil {
		st = *b.st
	}
	r := console.Compose(st, b.cfg, time.Now())
	r.Lit = b.lit
	return r
}

// bootMsg answers what comes to the console. The readout waits for the
// station to be read; every other stage comes on at its beat.
func (m model) bootMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	b := m.boot
	switch msg := msg.(type) {
	case stationRead:
		st := station.Station(msg)
		b.st = &st
		if !b.due {
			return m, nil
		}
		b.due = false
		return m.advance()
	case stageDue:
		if b.stage+1 == console.StageReadout && b.st == nil {
			b.due = true
			return m, nil
		}
		return m.advance()
	case blink:
		b.lit = !b.lit
		return m, blinkTick(b.lit)
	}
	return m, nil
}

// advance brings the next stage on and sets the one after it going.
func (m model) advance() (tea.Model, tea.Cmd) {
	b := m.boot
	last := console.LastStage(m.bootReport())
	if b.stage < last {
		b.stage++
	}
	if b.stage >= last {
		return m, nil
	}
	after := checkBeat
	if b.stage+1 == console.StageReadout || b.stage+1 == last {
		after = beat
	}
	return m, stageTick(after)
}

// consoleKey answers a key on the console. q and ctrl-c leave, as they
// do everywhere; any other key skips the sequence to the end, and at
// the end goes on: the console is put away, the workspace made beside
// the panel, and the projects, read while the console stood, come up.
func (m model) consoleKey(k string) (tea.Model, tea.Cmd) {
	if k == "q" || k == "ctrl+c" {
		m.note = m.leave()
		return m, nil
	}
	if last := console.LastStage(m.bootReport()); m.boot.stage < last {
		m.boot.stage, m.boot.lit = last, true
		return m, nil
	}
	m.boot = nil
	m.openWorkspace()
	return m, m.read()
}

// consoleHints is what the key bar says under the console.
var consoleHints = []keyHint{{"any key", "continue"}, {"q", "leave"}}

// drawConsole is the console as far as it has come on: the rows of the
// stages reached, and air where the rest will come, in the rows the
// foot leaves. The console is on the ground, not the surface the panel
// stands on: it is the whole window, not the panel beside the work.
func (m model) drawConsole() ([]string, foot) {
	rows := console.Screen(m.bootReport(), m.width, max(m.height-1, 1), console.Colored(m.g))
	lines := make([]string, len(rows))
	for i, r := range rows {
		if r.Stage <= m.boot.stage {
			lines[i] = r.Text
		}
	}
	return lines, foot{hints: consoleHints}
}
