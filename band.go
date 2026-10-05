package main

import (
	"strconv"
	"strings"

	"github.com/w0zro/conn/internal/work"

	"github.com/w0zro/conn/internal/tmux"

	"github.com/w0zro/conn/internal/theme"

	tea "charm.land/bubbletea/v2"
)

// Across the foot of the window is the status line, which is tmux's
// status line and an annunciator panel: dark until something conn's
// keys are doing lights it. It is written in tmux.go; conn lights its
// half through saying, here.

// saying puts what conn knows about its own keys on the status line,
// when it has changed since the last telling. Going through here is the
// point: a question is armed and answered from a handful of keys and
// the views are left and entered from as many more, and every one of
// them would otherwise have to remember to say so.
//
// The status line is tmux's line and conn reaches it by setting an
// option, which is a process — so it is written when what it says
// changes, which is on a keypress, and never on a beat.
func (m model) saying() (model, tea.Cmd) {
	if !m.inside || m.srv == nil {
		return m, nil
	}
	now := m.telling()
	if m.said != nil && *m.said == now {
		return m, nil
	}
	m.said = &now
	srv, ident := m.srv, designation(m.head.Login.Host, m.head.Build.Tag, m.g)
	if m.detour.to == toSettings {
		return m, func() tea.Msg { _ = srv.SayBand(now.keys, now.station, now.up, ident); return nil }
	}
	return m, func() tea.Msg { _ = srv.Say(now.keys, now.station, now.up, now.bar, ident); return nil }
}

// A band is the words conn puts on the status line: its word for the
// keys, the station's own word for while they are off the panel, the
// clock, and the key bar.
type band struct {
	keys, station, up, bar string
}

// telling is the band as things stand.
func (m model) telling() band {
	b := band{keys: m.keys(), station: m.station(), up: m.upWord(), bar: m.bar()}
	// The settings have the keys and say what the keys do there, the
	// bar being the keys that work on the row under the cursor and the
	// cursor being in that pane. The panel writes the rest of the line
	// and leaves that position to them; the bar it would have written
	// is forgotten rather than remembered, so the first telling after
	// the settings are done writes the panel's own again whatever it
	// says.
	if m.detour.to == toSettings {
		b.bar = ""
	}
	return b
}

// keys is what conn knows about its own keys, for the left of the
// status line. Two things can be there, and a question armed comes
// first: it takes the next key whatever it is, so while it stands the
// view under it cannot be worked and the view's word would be a lie.
// The question is on the status line rather than the panel because the
// line spans the window, where the panel's forty-four columns cut the
// question before the part that says how to answer it.
//
// Otherwise the word for the view the keys are in. The views are worked
// by different keys — a letter narrows the rows in projects and
// sessions and is a command in processes — so which one has the keys is
// a state the operator is in, the same kind of thing COPY says. tmux shows it only while the keys are on the panel, so a bay
// with the keys in it leaves the position dark.
func (m model) keys() string {
	if m.kill != nil {
		// The question itself is on the key bar, where the answer is.
		return tmux.StatusLineBlock("CONFIRM", m.g)
	}
	// Reading the manual, or keeping the settings, is a state the
	// operator is in, like a question armed, and it outranks the
	// wordmark: while either is up the panel is not being worked.
	if m.detour.to != noDetour && m.view == viewProcesses {
		return tmux.StatusLineBlock(m.detour.to.word(), m.g)
	}
	// The whole tree is a way of looking at the processes view rather
	// than a view of its own, and the band says so while it is on.
	if m.full && m.view == viewProcesses {
		return tmux.StatusLineBlock(treeWord, m.g)
	}
	// Otherwise the wordmark: the band is the station's, and the panel
	// says which view it is in by its own eyebrows. The console says
	// nothing, wearing a wordmark of its own six rows tall.
	if m.view == viewConsole {
		return ""
	}
	return m.wordmark()
}

// wordmark is conn's name as the band wears it, and after it how many
// lines the log has had since it was last opened: the one thing the
// band says on its own about what happened while the operator was not
// looking. It is on the band because the band is under every pane,
// and inside a process is where the operator was not looking from. It
// is a figure, and is written as the band writes its figures, in the
// gray after a dot the way the clock is, and not as a block: a block
// is a state the keys are in, and a count lit like one read as an
// alarm when it is a number to glance at. Nothing while the log has
// nothing new.
func (m model) wordmark() string {
	w := tmux.StatusLineWord(wordmarkLine, theme.Hex(m.g.Ink), true, m.g)
	if m.log.unseen > 0 {
		w += tmux.StatusLineWord("· LOG "+strconv.Itoa(m.log.unseen)+" ", m.g.Gray, false, m.g)
	}
	// The panel this one relieved, until the next key: the station came
	// back on another build, and nothing in the view would say so.
	if m.relieved {
		w += tmux.StatusLineWord("· RESTARTED ", m.g.Gray, false, m.g)
	}
	return w
}

// wordmarkLine is conn's name as the band wears it.
const wordmarkLine = " CONN "

// treeWord is what the line says while the processes view shows the
// whole tree.
const treeWord = "TREE"

// station is what the line says while the keys are not on the panel.
// Ordinarily nothing: the keys are in a process, and what that process
// is doing is its own business and is on its own screen. The manual
// and the settings are what conn puts the keys into that are conn's
// own, and each says so, so that a page filling the workspace is not
// mistaken for a program the operator opened and has to get out of by
// guessing.
func (m model) station() string {
	if m.detour.to != noDetour {
		return tmux.StatusLineBlock(m.detour.to.word(), m.g)
	}
	return m.wordmark()
}

// upWord is the right edge of the band: the time of day, local, as
// the console says it, and how long this conn has been up, as a
// mission clock reads. It is on the band because the band is under
// every view and every pane, the one place a clock is always in sight.
// To the minute: the band is written when its words change, and a
// second hand would write it every second.
func (m model) upWord() string {
	if m.now.IsZero() {
		return ""
	}
	word := m.now.Format("15:04")
	if !m.up.IsZero() {
		word += " · T+ " + strings.ToLower(uptime(m.up, m.now))
	}
	return tmux.StatusLineWord(word+" ", m.g.Gray, false, m.g)
}

// bar is the key bar across the foot of the window: the keys that work
// where the cursor is, and only those — a key the row under the cursor
// cannot take is not offered — or what has the keys instead: the
// manual, or a question armed, which is said here with its answers.
func (m model) bar() string {
	switch {
	case m.kill != nil:
		// The band says CONFIRM over it; the bar is the question and
		// its answers, and says neither twice.
		return tmux.StatusLineSay(m.kill.prompt, m.g) + "  " + keyBar([]keyHint{{"y", "Yes"}, {"any other key", "No"}}, m.g)
	case m.detour.to == toManual:
		return keyBar(helpHints, m.g)
	}
	// While a process has the keys, none of the panel's work: what
	// works is the panel key, which tmux takes before the process does,
	// and after it any key of the panel's. The bar says the key, and
	// the pairs that matter most from there.
	if m.inside && !m.focused && m.view != viewConsole {
		px := keyWord(tmux.PanelKey())
		hints := []keyHint{{px, "Panel"}}
		if len(work.WaitingRound(m.projects)) > 0 {
			hints = append(hints, keyHint{px + " tab", "Next waiting"})
		}
		return keyBar(append(hints, keyHint{px + " " + px, "Last process"}, keyHint{px + " ?", "Help"}), m.g)
	}
	var hints []keyHint
	switch m.view {
	case viewConsole:
		return keyBar(consoleHints, m.g)
	case viewProjects:
		rows := m.projectRows()
		if len(rows) > 1 {
			hints = append(hints, typedMoveHint)
		}
		if row, ok := m.atCursor(); ok && m.inside {
			if row.pid != 0 {
				hints = append(hints, keyHint{"enter", "Go in"})
			} else {
				hints = append(hints, keyHint{"enter", "Open a shell there"}, keyHint{"alt-a", "New contact"}, keyHint{"alt-A", "Sessions"})
			}
		}
		if m.inside {
			hints = append(hints, keyHint{"alt-r", "Recent"})
		}
		return keyBar(append(hints, keyHint{"esc", "Back"}), m.g)
	case viewSessions:
		if len(m.sessions.rows()) > 1 {
			hints = append(hints, typedMoveHint)
		}
		if len(m.sessions.rows()) > 0 && m.inside {
			// Here is the project the view is for; the recent view is
			// for every one, and resumes a session where it was had.
			word := "Resume it here"
			if m.sessions.recent {
				word = "Resume it"
			}
			hints = append(hints, keyHint{"enter", word})
		}
		// From one project's sessions, every project's; the recent
		// view is that already.
		if m.inside && !m.sessions.recent {
			hints = append(hints, keyHint{"alt-r", "Recent"})
		}
		return keyBar(append(hints, keyHint{"esc", "Back"}), m.g)
	case viewRoots:
		return keyBar(rootsHints, m.g)
	case viewLog:
		if len(m.log.read) > 1 {
			hints = append(hints, moveHint)
		}
		if e, ok := m.log.logAt(); ok && m.inside {
			if _, ok := m.logEntry(e); ok {
				hints = append(hints, keyHint{"enter", "Go in"})
			}
			hints = append(hints, keyHint{"/", "Find in output"})
		}
		return keyBar(append(hints, keyHint{"esc", "Back"}), m.g)
	case viewOutput:
		if len(m.out.matches()) > 1 {
			hints = append(hints, typedMoveHint)
		}
		if _, _, ok := m.out.outAt(); ok && m.inside {
			hints = append(hints, keyHint{"enter", "Go in"})
		}
		return keyBar(append(hints, keyHint{"esc", "Back"}), m.g)
	}
	if rowsIn(m.projects) > 1 {
		hints = append(hints, moveHint)
	}
	e, pl, ok := m.under()
	if ok {
		if word, cmd := m.enterOn(e); cmd != nil {
			hints = append(hints, keyHint{"enter", word})
		}
	}
	if len(work.WaitingRound(m.projects)) > 0 {
		hints = append(hints, keyHint{"tab", "Next waiting"})
	}
	// The port itself is the word: the bar has enter saying Open beside
	// it, and what tells the two apart is that this one names a port.
	if ok && serving(e) {
		hints = append(hints, keyHint{"o", "Open " + portsColumn(e.Ports[:1])})
	}
	if ok && m.endOn(e) != nil {
		hints = append(hints, keyHint{"x", "End it"})
	}
	// A shell, a contact and a bring-up are at the row's project, so
	// with no row under the cursor there is nowhere for them: what is
	// left is the station's own keys, which work wherever the cursor
	// is and are at the end of the bar for it.
	if m.inside && ok {
		hints = append(hints, keyHint{"s", "Shell"})
		if p := m.programUnder(e); p != nil {
			hints = append(hints, keyHint{"S", p.client})
		}
		hints = append(hints, keyHint{"a", "New contact"}, keyHint{"/", "Find in output"})
		if m.raiseOn(e) != nil {
			hints = append(hints, keyHint{"u", "Bring it up"})
		}
		if projectHasDown(m.projects, pl.Path) {
			hints = append(hints, keyHint{"U", "Bring up all"})
		}
	}
	hints = append(hints, keyHint{"p", "Projects"})
	if m.inside {
		hints = append(hints, keyHint{"r", "Recent"})
	}
	hints = append(hints, keyHint{"l", "Log"})
	return keyBar(append(hints, keyHint{",", "Settings"}, keyHint{"?", "Help"}), m.g)
}

// keyWord is the panel key as the bar writes it: ^space for C-Space,
// the caret being how a terminal has always written control, and
// short enough that the key said three times across the bar is still
// a bar of keys and not a sentence; alt-a for M-a. A named key is
// written as the manual writes it, in lower case; a letter is left
// as it came, since alt-A is not alt-a.
func keyWord(p string) string {
	p = strings.ReplaceAll(p, "C-", "^")
	p = strings.ReplaceAll(p, "M-", "alt-")
	if i := strings.LastIndexAny(p, "^-"); len(p)-i-1 > 1 {
		p = p[:i+1] + strings.ToLower(p[i+1:])
	}
	return p
}

// projectHasDown says whether a project has anything declared and not
// running, which is what U would bring up.
func projectHasDown(projects []work.Project, path string) bool {
	for _, pl := range projects {
		for _, e := range pl.Entries {
			if e.Status == work.StatusDown && pl.Path == path {
				return true
			}
		}
	}
	return false
}
