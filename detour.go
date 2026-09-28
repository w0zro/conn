package main

import tea "charm.land/bubbletea/v2"

// A detour is a page of conn's own put in the workspace: the manual,
// which ? puts there, or the settings, which , does. Either is a
// question asked in the middle of something, and the answer to it is
// not a reason to move the operator, so the detour holds what leaving
// it puts back: the pane the keys were in, where the panel key brought
// them out of one to ask, and the row that was under the cursor.
//
// While one is up the panel says HELP or SETTINGS and no row is under
// the cursor: the page is not a process, so there is no row it belongs
// to, and a cursor left sitting on one would say the keys were about
// that row when they are about reading. The panel goes on reading the
// machine meanwhile — the list is what the station is for, and a page
// being read beside it is no reason to stop.
//
// Neither page is reachable. They are conn's furniture, and the keys
// step over furniture, so the panel key and the page's own way out are
// the ways out of it; a page that could be opened and not closed would
// be a trap rather than a help.
type detour struct {
	to     detourTo
	from   string // the pane the keys were in when it was asked for; blank from the panel
	cursor int    // the row that was under the cursor, 0 for none
}

// detourTo is which page of conn's own is in the workspace.
type detourTo int

const (
	noDetour detourTo = iota
	toManual
	toSettings
)

// helpWord is what the line says while the manual is up, and
// settingsWord while the settings are.
const (
	helpWord     = "HELP"
	settingsWord = "SETTINGS"
)

// word is what the line says while the page is up.
func (d detourTo) word() string {
	if d == toSettings {
		return settingsWord
	}
	return helpWord
}

// openDetour puts a page of conn's own in the workspace, and takes with
// it where the keys were before the panel key brought them here: in the
// pane the panel key brought them out of, if this was the key after it,
// and on the panel if the operator was working the view.
func (m model) openDetour(to detourTo, came string) (tea.Model, tea.Cmd) {
	if m.srv == nil || m.detour.to != noDetour {
		return m, nil // nowhere to put it, or a page of conn's own up already
	}
	mm, cmd := m.toProcesses()
	m = mm.(model)
	// Said here rather than when the page is up. Opening it is several
	// turns of talking to tmux, and the readout would be put in the
	// workspace by a reading landing in the middle of that — the readout
	// goes up wherever a row is under the cursor, and it is this that
	// takes the row out from under it.
	//
	// The row is kept rather than dropped. No row is under the cursor
	// while the page is up, but the operator has not unchosen it: they
	// asked a question of the station and are coming back to whatever
	// they were looking at.
	m.detour = detour{to: to, from: came, cursor: m.cursor}
	m.cursor = 0
	open := m.openHelp()
	if to == toSettings {
		open = m.openTheSettings()
	}
	return m, tea.Batch(cmd, open)
}

// leftDetour is conn putting things back as the page found them, which
// is what a page of conn's own asks for as it goes, and what a reading
// finding one dead answers with.
//
// So the keys go back where the panel key took them from. Pressed in
// the workspace, they go back into that pane — the work is put back in
// the workspace first, since the page displaced it to a window of its
// own and selecting it there is nothing happening at all. Pressed on
// the panel, they stay on the panel: the operator was working the view,
// and the workspace takes a hold, with the readout coming back to it on
// the next reading as it always does.
//
// A page found dead rather than leaving — killed from outside, or gone
// while nobody was tending the workspace — knows of no pane the keys
// came from, and falls back on the process the page was standing in
// front of.
func (m model) leftDetour(found bool) (tea.Model, tea.Cmd) {
	from := m.detour.from
	m = m.endDetour()
	if !m.inside || m.srv == nil {
		return m, nil
	}
	toPanel := tea.Batch(m.reviveBay(), m.serverCmd(func() error { return m.srv.focusPanel() }))
	if from != "" {
		if p, tty, ok := m.paneByID(from); ok && reachable(p) {
			return m, m.reach(p, tty)
		}
		// The pane the keys came from has gone while the page was up.
		// There is nothing to be put back into, and the panel is where
		// conn is worked from.
		return m, toPanel
	}
	// Nothing to go on: the page ended without saying. Back into the
	// work it was standing in front of, where there is any.
	if found {
		mm, cmd := m.backIn()
		m = mm.(model)
		if cmd != nil {
			return m, cmd
		}
	}
	return m, toPanel
}

// endDetour is the detour over, and the cursor back on the row it was
// asked from. follow lets the row go on the next reading if the process
// has ended meanwhile, which is what it does for a row nobody ever
// left; asked with no row under the cursor, it ends with none.
func (m model) endDetour() model {
	was := m.detour.cursor
	m.detour = detour{}
	if was == 0 {
		return m
	}
	m.cursor = was
	return m.published(false)
}
