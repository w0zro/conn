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
// the board. The board is what is running, by place, read again every
// two seconds while it is up; c brings the console back, and any key
// there returns to the board. The words of both are said again each
// second, from what was read and the clock as it stands. ctrl+c or q
// closes conn from either.

// The views.
const (
	viewConsole = iota
	viewBoard
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

// boardEvery is how often the board reads the process table.
const boardEvery = 2 * time.Second

type (
	stageMsg   struct{}          // the next stage is due
	clockMsg   struct{}          // the second has turned
	stationMsg struct{ station } // the station is read
	boardMsg   struct {          // the process table is read
		places []place
		err    string
		gen    int
	}
	boardTickMsg struct{ gen int } // the board is due to be read again
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
	boardErr string
	boardGen int // which stay on the board the ticks belong to
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

// boardReport is the board's words as things stand.
func (m model) boardReport() boardReport {
	r := m.report()
	return composeBoard(m.places, m.head.session.home, m.now, r.station, r.clock, m.boardErr)
}

func (m model) Init() tea.Cmd {
	return tea.Batch(readStationCmd, m.nextStage(), nextSecond(m.now))
}

func readStationCmd() tea.Msg {
	return stationMsg{readStation()}
}

// readBoard reads the process table and composes the board off it.
func (m model) readBoard() tea.Cmd {
	gen, self, uid, roots := m.boardGen, m.self, m.uid, m.roots
	return func() tea.Msg {
		procs, err := readProcesses(uid)
		if err != nil {
			return boardMsg{err: "THE PROCESS TABLE COULD NOT BE READ: " + err.Error(), gen: gen}
		}
		return boardMsg{places: board(procs, self, uid, roots), gen: gen}
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

func (m model) boardTick() tea.Cmd {
	gen := m.boardGen
	return tea.Tick(boardEvery, func(time.Time) tea.Msg { return boardTickMsg{gen} })
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
	case boardMsg:
		if msg.gen != m.boardGen {
			return m, nil
		}
		m.places, m.boardErr = msg.places, msg.err
		if m.view == viewBoard {
			return m, m.boardTick()
		}
	case boardTickMsg:
		if msg.gen != m.boardGen || m.view != viewBoard {
			return m, nil
		}
		return m, m.readBoard()
	case tea.KeyPressMsg:
		return m.key(msg.String())
	}
	return m, nil
}

// key answers a key: q and ctrl+c close conn from anywhere; on the
// console a key skips the sequence, then continues to the board; on the
// board c brings the console back.
func (m model) key(k string) (tea.Model, tea.Cmd) {
	switch {
	case k == "ctrl+c" || k == "q":
		return m, tea.Quit
	case m.view == viewConsole && m.stage < lastStage(m.report()):
		m.stage = lastStage(m.report())
		return m, nil
	case m.view == viewConsole:
		m.view = viewBoard
		m.boardGen++
		return m, m.readBoard()
	case k == "c":
		m.view = viewConsole
		return m, nil
	}
	return m, nil
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
	case viewBoard:
		rows = drawBoard(m.boardReport(), m.width, m.height, m.p)
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
