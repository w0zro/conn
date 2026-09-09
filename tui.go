package main

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// The program holds the screen. It comes on a row at a time, the way a
// screen was painted down a serial line, the clock keeps time once it is
// up, and it stays until ctrl+c or q.

// paintPace is the time between rows as the screen comes on.
const paintPace = 30 * time.Millisecond

// paintMsg says the next row is due; clockMsg says the second has turned.
type (
	paintMsg struct{}
	clockMsg struct{}
)

type model struct {
	report        report
	shown         int // rows on screen so far
	width, height int
}

func newModel() model {
	return model{report: stationReport()}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(nextRow(), nextSecond())
}

func nextRow() tea.Cmd {
	return tea.Tick(paintPace, func(time.Time) tea.Msg { return paintMsg{} })
}

func nextSecond() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return clockMsg{} })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case paintMsg:
		if m.shown < screenRows {
			m.shown++
			return m, nextRow()
		}
	case clockMsg:
		m.report.clock = zulu(time.Now())
		return m, nextSecond()
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) View() tea.View {
	v := tea.NewView(place(screen(m.report), m.shown, m.width, m.height))
	v.AltScreen = true
	return v
}
