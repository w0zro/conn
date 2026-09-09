package main

import (
	"image/color"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// The ground and the ink, as the terminal is asked to take them for its
// own while the console is up, so its padding is the ground too.
var (
	groundColor = color.RGBA{R: 21, G: 19, B: 15, A: 255}
	inkColor    = color.RGBA{R: 230, G: 223, B: 208, A: 255}
)

// The program holds the console. It comes on in stages — the header at
// once, the system block, the checks one by one, the verdict — in under
// a second; a key skips to the end of the sequence, and the clock keeps
// time once it is up. It waits there on a key: for now, with nothing
// past the console, the key closes it, as ctrl+c or q does at any time.

// The time before each stage after the header: a beat for the readout
// and the verdict, less for each check.
func (m model) stageDelay(stage int) time.Duration {
	switch stage {
	case stageReadout, lastStage(m.report):
		return 150 * time.Millisecond
	default:
		return 80 * time.Millisecond
	}
}

// stageMsg says the next stage is due; clockMsg says the second has turned.
type (
	stageMsg struct{}
	clockMsg struct{}
)

type model struct {
	report        report
	stage         int // the stage the console has come on to
	width, height int
}

func newModel() model {
	return model{report: stationReport()}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.nextStage(), nextSecond())
}

func (m model) nextStage() tea.Cmd {
	return tea.Tick(m.stageDelay(m.stage+1), func(time.Time) tea.Msg { return stageMsg{} })
}

func nextSecond() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return clockMsg{} })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case stageMsg:
		last := lastStage(m.report)
		if m.stage < last {
			m.stage++
		}
		if m.stage < last {
			return m, m.nextStage()
		}
	case clockMsg:
		m.report.clock = zulu(time.Now())
		return m, nextSecond()
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		default:
			if m.stage >= lastStage(m.report) {
				return m, tea.Quit
			}
			m.stage = lastStage(m.report)
		}
	}
	return m, nil
}

// View is the console as far as it has come on: rows of a later stage
// are the ground until their turn.
func (m model) View() tea.View {
	rows := screen(m.report, m.width, m.height)
	ground := screen(report{}, m.width, 1)[0].text
	texts := make([]string, 0, len(rows))
	for i, r := range rows {
		if i >= m.height && m.height > 0 {
			break
		}
		if r.stage > m.stage {
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
