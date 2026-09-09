package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// hold is what stands in the slot when no process does: the ground, and
// a line saying how to put one there. A key in it hands focus back to
// the rail. conn runs it as `conn hold`, in a pane of its own server.

const holdHint = "ENTER ON THE WATCH REACHES A PROCESS · S OPENS A SHELL"

type holdModel struct {
	srv           *server
	width, height int
	p             palette
}

func runHold(srv *server, p palette) error {
	_, err := tea.NewProgram(holdModel{srv: srv, p: p}, programOptions()...).Run()
	return err
}

func (h holdModel) Init() tea.Cmd { return nil }

func (h holdModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.width, h.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		if h.srv != nil {
			return h, func() tea.Msg { _ = h.srv.focusRail(); return nil }
		}
	}
	return h, nil
}

func (h holdModel) View() tea.View {
	c := canvas{p: h.p, width: max(h.width, 1)}
	for len(c.rows) < h.height/2 {
		c.blank(0)
	}
	l := c.line()
	l.add(h.p.faint, fit(holdHint, max(h.width-2, 1), false))
	c.emit(l, 0, true)
	for len(c.rows) < h.height {
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

// holdEnv says whether this conn was started as the hold.
func holdEnv(args []string) bool {
	return len(args) > 1 && args[1] == "hold"
}
