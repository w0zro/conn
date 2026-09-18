package main

import (
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// The key bar, in a pane of conn's own across the foot of home, one
// row tall and the window's width. It draws what the panel has put in
// the server's options — the keys that work where the cursor is, and
// the station's designation — on the band's ground, and draws it again
// when the panel says so: the panel signals a tmux channel each time it
// writes the options, and the bar waits on it, so nothing is read on a
// beat. A key pressed on the bar puts the keys back on the panel.

type barModel struct {
	srv         *server
	left, right string // the keys, and the designation, in conn's escapes
	width       int
	p           palette
}

// barMsg is the options as read after the panel's signal.
type barMsg struct{ left, right string }

func runBar(srv *server, p palette) error {
	_, err := tea.NewProgram(barModel{srv: srv, p: p}, programOptions()...).Run()
	return err
}

// Init reads the bar once; each reading, once answered, waits for the
// panel to change it, so there is one wait on the channel at a time.
func (b barModel) Init() tea.Cmd {
	return b.read(false)
}

// read is the options as they stand, after the panel's signal where
// wait is set: tmux remembers a signal nobody was waiting on, so a
// change that lands between one wait and the next is not lost.
func (b barModel) read(wait bool) tea.Cmd {
	srv := b.srv
	return func() tea.Msg {
		if srv == nil {
			return nil
		}
		if wait {
			_ = exec.Command(srv.tmux, "-S", srv.socket, "wait-for", barChannel).Run()
		}
		left, _ := srv.run("show-options", "-gqv", "@conn_bar_text")
		right, _ := srv.run("show-options", "-gqv", "@conn_ident")
		return barMsg{unescaped(strings.TrimRight(left, "\n")), unescaped(strings.TrimRight(right, "\n"))}
	}
}

// unescaped is an option's value as it was set: tmux writes a control
// character in it back as three octal digits after a backslash, and a
// backslash as two.
func unescaped(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) && isOctal(s[i+1]) && isOctal(s[i+2]) && isOctal(s[i+3]) {
			b.WriteByte((s[i+1]-'0')<<6 | (s[i+2]-'0')<<3 | (s[i+3] - '0'))
			i += 3
			continue
		}
		if s[i] == '\\' && i+1 < len(s) && s[i+1] == '\\' {
			b.WriteByte('\\')
			i++
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func isOctal(c byte) bool { return c >= '0' && c <= '7' }

func (b barModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		b.width = msg.Width
	case barMsg:
		b.left, b.right = msg.left, msg.right
		return b, b.read(true)
	case tea.KeyPressMsg:
		if b.srv != nil {
			srv := b.srv
			return b, func() tea.Msg { _ = srv.focusPanel(); return nil }
		}
	}
	return b, nil
}

// View is the one row: the keys a cell in from the left, the mark a
// cell in from the right, the band between.
func (b barModel) View() tea.View {
	width := max(b.width, 1)
	left, right := b.left, b.right
	if visible(left)+visible(right)+3 > width {
		right = ""
	}
	gap := max(width-1-visible(left)-visible(right)-1, 0)
	row := b.p.end + b.p.selection + b.p.ink + " " + left + strings.Repeat(" ", gap) + right + " " + b.p.end
	v := tea.NewView(row)
	v.AltScreen = true
	return v
}
