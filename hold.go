package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// hold is what stands in the bay when no process does: the ground, and
// a placard saying the position is vacant. A key in it processes focus
// back to the panel. conn runs it as `conn hold`, in a pane of its own
// server.
//
// It said how to fill the bay before, a sentence of instructions
// shown on every empty bay forever, and cut off at the width of most
// windows. Instructions live in the manual; a vacant position gets a
// placard.

const holdWord = "VACANT"

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
			return h, func() tea.Msg { _ = h.srv.focusPanel(); return nil }
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
	l.add(h.p.faint, holdWord)
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
