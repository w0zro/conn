package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// The manual, in a pane of conn's own. conn runs it as `conn manual`,
// the way it runs the readout and the hold, and for the same reason:
// the pane is conn's, so the keys in it are conn's to decide and the
// row it would otherwise take in the processes view is not taken — a
// conn is not listed among the processes conn is holding for you.
//
// It pages the text itself rather than handing the page to less. less
// is a good pager and it is not this one: esc is how the operator
// leaves everything else in conn, and a pager that took esc for the
// start of a command would be the one place in the station where the
// key meant something else.

// A manualModel is the manual being read: the lines, and where in them.
type manualModel struct {
	srv           *server
	path          string
	lines         []string
	top           int
	width, height int
	p             palette
}

func runManual(srv *server, path string, p palette) error {
	_, err := tea.NewProgram(manualModel{srv: srv, path: path, p: p}, programOptions()...).Run()
	return err
}

func (m manualModel) Init() tea.Cmd { return nil }

// page is how far a screenful moves, one line kept for the eye.
func (m manualModel) page() int { return max(m.height-1, 1) }

// last is the furthest the text can be scrolled: the end of it at the
// foot of the pane, never past.
func (m manualModel) last() int { return max(len(m.lines)-m.height, 0) }

func (m manualModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// The text is set to the pane. man wraps to a width, so a pane
		// that changes size is a page that has to be read again rather
		// than the same lines drawn narrower.
		m.width, m.height = msg.Width, msg.Height
		m.lines = manText(m.path, m.width)
		m.top = min(m.top, m.last())
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "q":
			// Out of the manual and back to the work it was covering.
			// The panel is told before this pane ends, so the workspace
			// is filled in the same breath: ending first and leaving the
			// panel to notice put a dead pane in the workspace for as
			// long as it took the next reading to come round.
			srv := m.srv
			return m, tea.Sequence(
				func() tea.Msg {
					if srv != nil {
						_ = srv.leaveHelp()
					}
					return nil
				},
				tea.Quit,
			)
		case "j", "down", "ctrl+n":
			m.top = min(m.top+1, m.last())
		case "k", "up", "ctrl+p":
			m.top = max(m.top-1, 0)
		case " ", "space", "pgdown", "ctrl+f":
			m.top = min(m.top+m.page(), m.last())
		case "b", "pgup", "ctrl+b":
			m.top = max(m.top-m.page(), 0)
		case "g", "home":
			m.top = 0
		case "G", "end":
			m.top = m.last()
		}
	}
	return m, nil
}

func (m manualModel) View() tea.View {
	c := canvas{p: m.p, width: max(m.width, 1)}
	for i := m.top; i < len(m.lines) && len(c.rows) < m.height; i++ {
		l := c.line()
		for _, run := range manLine(m.lines[i]) {
			color := m.p.ink
			if run.bold {
				color = m.p.ink + m.p.bold
			}
			l.add(color, run.text)
		}
		c.emit(l, 0, false)
	}
	for len(c.rows) < m.height {
		c.blank(0)
	}
	texts := make([]string, len(c.rows))
	for i, r := range c.rows {
		texts[i] = r.text
	}
	v := tea.NewView(strings.Join(texts, "\n"))
	v.AltScreen = true
	return v
}
