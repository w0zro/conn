package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// conn settings — the settings, in a pane of conn's own, the way the
// manual and the readout are. conn runs it as `conn settings` and
// swaps it into the workspace; the panel says SETTINGS and goes on
// reading the machine beside it.
//
// It is in the workspace because that is where conn puts what is being
// worked on. The panel is the list of what is running, forty columns
// of it, and a form that replaced the list for as long as a root was
// being typed took away the one thing the operator keeps the station
// up for. A configuration is worked on and then left, which is what
// the workspace is for.
//
// What it writes is the file, and only the keys it came to change. The
// panel is not told what was written: it reads the roots off the file
// on its own beat and re-roots itself when they have changed, which is
// the same thing it does for a file edited by hand. The mode is the
// exception — it is not in that file but on the server, and the panel
// is told so it wears it now rather than on its next start; see
// wearMode.

// A settingsModel is the settings being worked: the cursor among the
// rows, what went wrong writing, and the root being typed where one
// is.
type settingsModel struct {
	srv           *server
	home          string
	self          string // the pane this conn runs in, for a reground
	width, height int
	p             palette
	at            int    // the row the cursor is on
	err           string // what went wrong writing the file
	// The root being typed. asking is whether the line is up at all;
	// askingAt is the root it will be written over, or -1 for one being
	// added, and rootErr what went wrong saving what was typed.
	asking   bool
	line     typed
	askingAt int
	rootErr  string
	// firstG is a g that has been pressed and is nothing on its own,
	// the way it is on the panel: gg is the first row and a g thought
	// better of costs nothing.
	firstG bool
}

func runSettings(srv *server, home string, p palette) error {
	m := settingsModel{srv: srv, home: home, self: ownPane(), p: p}
	_, err := tea.NewProgram(m, programOptions()...).Run()
	return err
}

func (m settingsModel) Init() tea.Cmd { return m.saying() }

// report is the file as the view shows it, with what went wrong
// writing laid over the top: a save that failed is about the file the
// view is showing.
func (m settingsModel) report() settingsReport {
	b := composeSettings(m.home, current.theme, current.dark)
	if m.err != "" {
		b.err = m.err
	}
	return b
}

// rootsReport is the asking line as it stands: what has been typed,
// the directories on this machine that answer it, and where the caret
// is. editing is always on — the first start asks on the panel, where
// there is no workspace to ask in yet, so every root typed here is one
// being changed or added to a conn already at work.
func (m settingsModel) rootsReport() rootsReport {
	b := composeRootsAt(m.line.text, m.home)
	b.caret, b.editing, b.err = m.line.cur, true, m.rootErr
	return b
}

func (m settingsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		next, cmd := m.key(msg.String())
		return next, tea.Batch(cmd, next.saying())
	}
	return m, nil
}

// key answers a key. The rows are settings, so the keys are the list's:
// j and k move, enter answers the row under the cursor, x takes a root
// out, and esc leaves. Every change is made by a key pressed on
// purpose and written as it is made.
func (m settingsModel) key(k string) (settingsModel, tea.Cmd) {
	if m.asking {
		return m.rootKey(k)
	}
	rows := m.report().rows
	// The second g of gg, which is the only key the first one waits
	// for. Every other key clears it and goes on to do what it does.
	half := m.firstG
	m.firstG = false
	if half && k == "g" {
		m.at = 0
		return m, nil
	}
	switch k {
	case "ctrl+c", "esc":
		return m, m.leaving()
	case "j", "down":
		m.at = ring(m.at+1, len(rows))
	case "k", "up":
		m.at = ring(m.at-1, len(rows))
	case "g":
		m.firstG = true
	case "G":
		m.at = clamp(len(rows)-1, len(rows))
	case "enter":
		if m.at >= len(rows) {
			return m, nil
		}
		switch r := rows[m.at]; r.kind {
		case rootSetting:
			// The root as the file writes it, to be typed over: the
			// operator is fixing a path more often than replacing one.
			return m.askRoot(r.at, r.text)
		case addRootSetting:
			return m.askRoot(-1, "~/")
		case themeSetting:
			return m.useTheme(r.text)
		case groundSetting:
			return m.useGround(r.value)
		}
	case "x":
		if m.at < len(rows) && rows[m.at].kind == rootSetting {
			return m.dropRoot(rows[m.at].at)
		}
	}
	return m, nil
}

// askRoot puts the line up, typing one root: the one at an index, to be
// written over, or another, at -1. Completing a path against the
// machine is what the line is for, and is the reason a root is typed
// here rather than into the file.
func (m settingsModel) askRoot(at int, text string) (settingsModel, tea.Cmd) {
	m.asking, m.askingAt, m.rootErr = true, at, ""
	m.line = typed{}
	m.line.set(text)
	return m, nil
}

// rootKey answers a key on the line. The line is typed into like the
// panel's own; see typed. What is the line's own: tab fills it in with
// the directory under the cursor, enter takes it, and esc goes back to
// the rows with nothing written. A line that changes takes what went
// wrong saving off with it, since the error was about what was typed
// and that is not what is typed now.
func (m settingsModel) rootKey(k string) (settingsModel, tea.Cmd) {
	b := composeRoots(m.line.text, m.home)
	switch {
	case m.line.edit(k, len(b.rows)):
		if m.line.text != b.typed {
			m.rootErr = ""
		}
	case k == "ctrl+c":
		return m, m.leaving()
	case k == "tab":
		// Filling the line in is not answering: what is typed becomes
		// the directory under the cursor, with a separator after it, so
		// the next keystroke is already looking inside it.
		if m.line.at < len(b.rows) {
			m.line.set(b.rows[m.line.at] + "/")
		}
	case k == "esc":
		m.asking, m.rootErr = false, ""
	case k == "enter":
		return m.takeRoot(b)
	}
	return m, nil
}

// takeRoot writes the root the operator settled on. The root is what
// the cursor is on where the line has not been typed past it, and what
// was typed otherwise: somebody who typed a whole path and pressed
// enter meant that path, not the first thing that happened to be
// listed under it.
func (m settingsModel) takeRoot(b rootsReport) (settingsModel, tea.Cmd) {
	root := strings.TrimSpace(m.line.text)
	if typedIsADir(root, m.home) {
		// what was typed names a directory of its own: take it
	} else if m.line.at < len(b.rows) {
		root = b.rows[m.line.at]
	}
	if root == "" {
		return m, nil
	}
	// The roots the file names, with this one put in: written over the
	// root the rows sent here, and added to the rest otherwise. The
	// file is read again rather than kept from the last reading, since
	// it is the file this writes and the operator may have edited it by
	// hand meanwhile.
	c, err := readConfig(m.home)
	if err != nil {
		m.rootErr = err.Error()
		return m, nil
	}
	roots := append([]string{}, c.Roots...)
	if m.askingAt >= 0 && m.askingAt < len(roots) {
		roots[m.askingAt] = tilde(expandHome(root, m.home), m.home)
	} else {
		roots = append(roots, tilde(expandHome(root, m.home), m.home))
	}
	m.asking, m.rootErr = false, ""
	return m.wroteRoots(roots)
}

// dropRoot takes one root out of the file. No question is asked: a root
// is a line the operator typed and can type again, the view shows what
// is left at once, and conn is not going to make a habit of asking
// twice about work that is not a process.
func (m settingsModel) dropRoot(at int) (settingsModel, tea.Cmd) {
	c, err := readConfig(m.home)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	if at < 0 || at >= len(c.Roots) {
		return m, nil
	}
	return m.wroteRoots(append(append([]string{}, c.Roots[:at]...), c.Roots[at+1:]...))
}

// wroteRoots writes the roots. Nothing is said to the panel: it reads
// the file on every reading and walks what it now names, so the list
// is on the new roots a beat later without either conn telling the
// other anything.
func (m settingsModel) wroteRoots(roots []string) (settingsModel, tea.Cmd) {
	if err := saveRoots(m.home, roots); err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.err = ""
	m.at = clamp(m.at, len(m.report().rows))
	return m, nil
}

// useTheme puts conn in a theme and writes it down. The other axis is
// left alone: the ground conn is on was settled when the server rose,
// and picking a theme is not a reason to go back over it.
func (m settingsModel) useTheme(name string) (settingsModel, tea.Cmd) {
	if _, ok := themeNamed(name); !ok {
		return m, nil
	}
	if err := saveTheme(m.home, name); err != nil {
		m.err = err.Error()
		return m, nil
	}
	return m.wearing(mode{theme: name, dark: current.dark})
}

// useGround puts conn on a ground and writes it down, or, for a ground
// of nothing, takes the key out and leaves the choice to the terminal
// from the next start.
//
// The terminal is not asked again here. OSC 11 is a question put to the
// terminal and answered by it, and a conn in a pane of conn's own
// server would be asking tmux, which answers with the ground conn
// itself set: the question is only worth asking when a server rises,
// and that is where it is asked. So the row that gives the choice back
// changes nothing now and says so; what is on stays on until conn down.
func (m settingsModel) useGround(name string) (settingsModel, tea.Cmd) {
	dark, named := groundNamed(name)
	if name != "" && !named {
		return m, nil
	}
	if err := saveGround(m.home, name); err != nil {
		m.err = err.Error()
		return m, nil
	}
	if !named {
		m.err = ""
		return m, nil
	}
	return m.wearing(mode{theme: current.theme, dark: dark})
}

// wearing puts conn in a mode, now rather than on the next start: this
// pane takes it here, the mode file is what a pane of conn's own reads
// when it comes up, the configuration is what the next server reads,
// and the server standing is sourced again so every pane takes the new
// sixteen where it stands. What conn writes for other programs — Claude
// Code's theme, nvim's colorscheme — is written again where it is
// already there, since those are conn's colors too and a station half
// in one theme is worse than either.
//
// This pane is the one the server does not start again: it has the
// mode on already, and being started again would put the cursor back
// at the top of the page the operator is working. The panel is not
// started again either, a fresh conn there being the console; it is
// told, and wears the mode where it stands.
func (m settingsModel) wearing(want mode) (settingsModel, tea.Cmd) {
	m.err = ""
	// Every color conn draws from is package-wide, so it is put on
	// here, on the loop, and what the server is told is worked out here
	// too and handed over ready: a command reading the palette off
	// another goroutine would be reading it while the next key writes
	// it.
	applyMode(want)
	m.p = colored()
	refreshClaudeTheme(m.home)
	refreshVimColorscheme(m.home)
	// Outside the server there is nothing to dress but this conn, and
	// applyMode has done it.
	if m.srv == nil || m.self == "" {
		return m, nil
	}
	srv, conf, bg, self := m.srv, tmuxConf(panelKey()), surfaceHex, m.self
	return m, func() tea.Msg {
		_ = srv.rewear(conf, bg, self, want)
		_ = srv.wearMode()
		return nil
	}
}

// leaving is the settings done with: the panel is told, so it fills the
// workspace in the same breath, and then this conn ends. Ending first
// and leaving the panel to notice put a dead pane in the workspace for
// as long as it took the next reading to come round.
func (m settingsModel) leaving() tea.Cmd {
	srv := m.srv
	return tea.Sequence(
		func() tea.Msg {
			if srv != nil {
				_ = srv.leaveSettings()
			}
			return nil
		},
		tea.Quit,
	)
}

// saying puts the keys that work here on the key bar. The bar says the
// keys that work where the cursor is and only those, which is a thing
// only the pane with the cursor in it knows — so while the settings
// have the keys the settings write the bar, and the panel writes the
// rest of the line and leaves that position alone.
func (m settingsModel) saying() tea.Cmd {
	if m.srv == nil || m.self == "" {
		return nil // not in a pane of the server: there is no line to write
	}
	bar := keyBar(rootsBarHints)
	if !m.asking {
		bar = keyBar(settingsHints(m.report().rows, m.at))
	}
	srv := m.srv
	return func() tea.Msg { _ = srv.sayBar(bar); return nil }
}

func (m settingsModel) View() tea.View {
	var rows []row
	if m.asking {
		rows = drawRoots(m.rootsReport(), m.line.at, max(m.width, 1), m.height, m.p)
	} else {
		rows = drawSettings(m.report(), m.at, max(m.width, 1), m.height, m.p)
	}
	texts := make([]string, len(rows))
	for i, r := range rows {
		texts[i] = r.text
	}
	v := tea.NewView(strings.Join(texts, "\n"))
	v.AltScreen = true
	return v
}
