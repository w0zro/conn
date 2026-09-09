package main

import (
	"image/color"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// The ground and the ink, as the terminal is asked to take them for its
// own while conn is up, so its padding is the ground too.
var (
	groundColor = color.RGBA{R: 21, G: 19, B: 15, A: 255}
	inkColor    = color.RGBA{R: 230, G: 223, B: 208, A: 255}
)

// The program holds two views. The console comes on first: the header
// at once, from what is known before anything is read; the station is
// read meanwhile, and the readout comes on when it is in hand and its
// beat has passed, then the checks one by one, then the verdict, in
// under a second. A key skips to the end; a key at the end continues to
// the watch. The watch is what is running, by place, read again every
// two seconds while it is up; j and k move the cursor, which follows
// its process across readings; c brings the console back, and any key
// there returns to the watch. The words of both are said again each
// second, from what was read and the clock as it stands. ctrl+c or q
// closes conn from either.

// The views.
const (
	viewConsole = iota
	viewWatch
)

// The time before each stage after the header: a beat for the readout
// and the verdict, less for each check.
func (m model) stageDelay(stage int) time.Duration {
	switch stage {
	case stageReadout, lastStage(m.report()):
		return 150 * time.Millisecond
	default:
		return 80 * time.Millisecond
	}
}

// watchEvery is how often the watch reads the process table.
const watchEvery = 2 * time.Second

type (
	stageMsg   struct{}          // the next stage is due
	clockMsg   struct{}          // the second has turned
	stationMsg struct{ station } // the station is read
	watchMsg   struct {          // the process table is read
		places []place
		err    string
		gen    int
	}
	watchTickMsg struct{ gen int } // the watch is due to be read again
)

type model struct {
	head          station  // what the header needs: the build and who is at the station
	st            *station // the station, once read
	now           time.Time
	stage         int  // the stage the console has come on to
	due           bool // the readout's beat has passed and it waits on the station
	width, height int
	p             palette

	view     int
	places   []place
	cursor   int // the pid the cursor is on
	cursorAt int // where in the rows it was, for when the pid goes
	watchErr string
	watchGen int // which stay on the watch the ticks belong to
	self     int // this process
	uid      int
	roots    func(string) string
}

func newModel(p palette) model {
	return model{
		head:  station{build: readBuild(), session: readSession()},
		now:   time.Now(),
		p:     p,
		self:  os.Getpid(),
		uid:   os.Getuid(),
		roots: placeRoots(),
	}
}

// report is the console's words as things stand: from the station once
// it is read, from the header's part of it before.
func (m model) report() report {
	if m.st != nil {
		return compose(*m.st, m.now)
	}
	return compose(m.head, m.now)
}

// watchReport is the watch's words as things stand.
func (m model) watchReport() watchReport {
	r := m.report()
	return composeWatch(m.places, m.head.session.home, m.now, r.station, r.clock, m.watchErr)
}

func (m model) Init() tea.Cmd {
	return tea.Batch(readStationCmd, m.nextStage(), nextSecond(m.now))
}

func readStationCmd() tea.Msg {
	return stationMsg{readStation()}
}

// readWatch reads the process table and composes the watch off it.
func (m model) readWatch() tea.Cmd {
	gen, self, uid, roots := m.watchGen, m.self, m.uid, m.roots
	return func() tea.Msg {
		procs, err := readProcesses(uid)
		if err != nil {
			return watchMsg{err: "THE PROCESS TABLE COULD NOT BE READ: " + err.Error(), gen: gen}
		}
		return watchMsg{places: watch(procs, self, uid, roots), gen: gen}
	}
}

func (m model) nextStage() tea.Cmd {
	return tea.Tick(m.stageDelay(m.stage+1), func(time.Time) tea.Msg { return stageMsg{} })
}

// nextSecond ticks on the turn of the second, not a second after the
// last tick, so no second is skipped.
func nextSecond(now time.Time) tea.Cmd {
	return tea.Tick(time.Until(now.Truncate(time.Second).Add(time.Second)), func(time.Time) tea.Msg { return clockMsg{} })
}

func (m model) watchTick() tea.Cmd {
	gen := m.watchGen
	return tea.Tick(watchEvery, func(time.Time) tea.Msg { return watchTickMsg{gen} })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case stationMsg:
		st := msg.station
		m.st = &st
		if m.due {
			m.due = false
			return m.advance()
		}
	case stageMsg:
		if m.stage+1 == stageReadout && m.st == nil {
			m.due = true
			return m, nil
		}
		return m.advance()
	case clockMsg:
		m.now = time.Now()
		return m, nextSecond(m.now)
	case watchMsg:
		if msg.gen != m.watchGen {
			return m, nil
		}
		m.places, m.watchErr = msg.places, msg.err
		m.cursor, m.cursorAt = follow(m.places, m.cursor, m.cursorAt)
		if m.view == viewWatch {
			return m, m.watchTick()
		}
	case watchTickMsg:
		if msg.gen != m.watchGen || m.view != viewWatch {
			return m, nil
		}
		return m, m.readWatch()
	case tea.KeyPressMsg:
		return m.key(msg.String())
	}
	return m, nil
}

// key answers a key: q and ctrl+c close conn from anywhere; on the
// console a key skips the sequence, then continues to the watch; on the
// watch c brings the console back.
func (m model) key(k string) (tea.Model, tea.Cmd) {
	switch {
	case k == "ctrl+c" || k == "q":
		return m, tea.Quit
	case m.view == viewConsole && m.stage < lastStage(m.report()):
		m.stage = lastStage(m.report())
		return m, nil
	case m.view == viewConsole:
		m.view = viewWatch
		m.watchGen++
		return m, m.readWatch()
	case k == "c":
		m.view = viewConsole
		return m, nil
	case k == "j" || k == "down":
		m.cursor, m.cursorAt = follow(m.places, 0, m.cursorAt+1)
	case k == "k" || k == "up":
		m.cursor, m.cursorAt = follow(m.places, 0, max(m.cursorAt-1, 0))
	}
	return m, nil
}

// follow finds the cursor after the rows change: the row of its pid,
// where that is still on watch, else the row where it was, held within
// the rows there are. It answers the pid and the row.
func follow(places []place, pid, at int) (int, int) {
	var pids []int
	for _, pl := range places {
		for _, e := range pl.entries {
			pids = append(pids, e.pid)
		}
	}
	if len(pids) == 0 {
		return 0, 0
	}
	for i, p := range pids {
		if pid != 0 && p == pid {
			return p, i
		}
	}
	at = min(max(at, 0), len(pids)-1)
	return pids[at], at
}

// advance brings the next stage on and sets the one after it going.
func (m model) advance() (tea.Model, tea.Cmd) {
	last := lastStage(m.report())
	if m.stage < last {
		m.stage++
	}
	if m.stage < last {
		return m, m.nextStage()
	}
	return m, nil
}

// View is the view that is up. The console shows as far as it has come
// on: rows of a later stage are the ground until their turn.
func (m model) View() tea.View {
	var rows []row
	switch m.view {
	case viewWatch:
		rows = drawWatch(m.watchReport(), m.cursor, m.width, m.height, m.p)
	default:
		rows = screen(m.report(), m.width, m.height, m.p)
	}
	ground := rows[0].text // the first row is blank, on the ground, at the rows' width
	texts := make([]string, 0, len(rows))
	for i, r := range rows {
		if i >= m.height && m.height > 0 {
			break
		}
		if m.view == viewConsole && r.stage > m.stage {
			texts = append(texts, ground)
		} else {
			texts = append(texts, r.text)
		}
	}
	v := tea.NewView(strings.Join(texts, "\n"))
	v.AltScreen = true
	v.BackgroundColor = groundColor
	v.ForegroundColor = inkColor
	v.WindowTitle = "conn"
	return v
}
