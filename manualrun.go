package main

import (
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/w0zro/conn/internal/tmux"
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

// A manualModel is the manual being read: the page, and the viewport
// it is read through, which does the scrolling.
type manualModel struct {
	srv  *tmux.Server
	path string
	page viewport.Model
	p    palette
}

func newManual(srv *tmux.Server, path string, p palette) manualModel {
	page := viewport.New()
	page.KeyMap = manualKeys()
	return manualModel{srv: srv, path: path, page: page, p: p}
}

func runManual(srv *tmux.Server, path string, p palette) error {
	_, err := tea.NewProgram(newManual(srv, path, p), programOptions()...).Run()
	return err
}

// manualKeys are the viewport's own, with the keys less and emacs
// scroll by as well, and no scrolling sideways: man has set the text to
// the pane.
func manualKeys() viewport.KeyMap {
	k := viewport.DefaultKeyMap()
	k.Down.SetKeys("j", "down", "ctrl+n")
	k.Up.SetKeys("k", "up", "ctrl+p")
	k.PageDown.SetKeys("space", "f", "pgdown", "ctrl+f")
	k.PageUp.SetKeys("b", "pgup", "ctrl+b")
	k.Left.SetEnabled(false)
	k.Right.SetEnabled(false)
	return k
}

func (m manualModel) Init() tea.Cmd { return nil }

func (m manualModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// The text is set to the pane. man wraps to a width, so a pane
		// that changes size is a page that has to be read again rather
		// than the same lines drawn narrower.
		m.page.SetWidth(msg.Width)
		m.page.SetHeight(msg.Height)
		m.page.SetContentLines(m.rows(manText(m.path, msg.Width), msg.Width, msg.Height))
		return m, nil
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
						_ = srv.LeaveHelp()
					}
					return nil
				},
				tea.Quit,
			)
		case "g", "home":
			m.page.GotoTop()
			return m, nil
		case "G", "end":
			m.page.GotoBottom()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.page, cmd = m.page.Update(msg)
	return m, cmd
}

// rows is the page drawn, a row to a line of man's, bold where man set
// it bold, on the ground; and down to the foot of the pane, on the
// ground still, where the page is shorter than the pane.
func (m manualModel) rows(lines []string, width, height int) []string {
	c := canvas{p: m.p, width: max(width, 1)}
	for _, text := range lines {
		l := c.line()
		for _, run := range manLine(text) {
			color := m.p.ink
			if run.bold {
				color = m.p.ink + m.p.bold
			}
			l.add(color, run.text)
		}
		c.emit(l, 0, false)
	}
	for len(c.rows) < height {
		c.blank(0)
	}
	return textsOf(c.rows)
}

func (m manualModel) View() tea.View {
	v := tea.NewView(m.page.View())
	v.AltScreen = true
	return v
}
