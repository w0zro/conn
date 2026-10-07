package main

import (
	"strings"

	"github.com/w0zro/conn/internal/tmux"

	tea "charm.land/bubbletea/v2"
)

// hold is what stands in the bay when no process does: the ground, and
// a placard saying the position is vacant. A key in it passes focus
// back to the panel. conn runs it as `conn hold`, in a pane of its own
// server.
//
// It said how to fill the bay before, a sentence of instructions
// shown on every empty bay forever, and cut off at the width of most
// windows. Instructions live in the manual; a vacant position gets a
// placard.

const holdWord = "VACANT"

type holdModel struct {
	srv           *tmux.Server
	width, height int
	p             Palette
}

func runHold(srv *tmux.Server, p Palette) error {
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
			return h, func() tea.Msg { _ = h.srv.FocusPanel(); return nil }
		}
	}
	return h, nil
}

func (h holdModel) View() tea.View {
	c := Canvas{P: h.p, Width: max(h.width, 1)}
	for len(c.Rows) < h.height/2 {
		c.Blank(0)
	}
	l := c.Line()
	l.Add(h.p.Faint, holdWord)
	c.Emit(l, 0, true)
	for len(c.Rows) < h.height {
		c.Blank(0)
	}
	v := tea.NewView(strings.Join(Texts(c.Rows), "\n"))
	v.AltScreen = true
	return v
}
