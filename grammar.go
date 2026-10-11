package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/w0zro/conn/internal/room"
	"github.com/w0zro/conn/internal/shell"
	"github.com/w0zro/conn/internal/work"
	"github.com/w0zro/conn/internal/work/claude"
)

// The processes view's keys are a grammar, in vim's spirit: a verb
// waits for an object, as vim's operators do. The verb pressed again is
// the row under the cursor, as dd is the line; p is the row's project;
// j, k and G are a row by a motion from the cursor. . does the last
// verb and object again where the cursor is now. n verbs and m objects
// are n×m acts from n+m keys, and the same letters mean the same thing
// on every row: what each does on a container, a brew service or a
// process is the row's to say, not a key of its own.
//
// enter and / are commands, acting at once, as vim's x and / do. The
// station's views moved behind g, as vim keeps its own: gp the
// projects, gr the recent sessions, gl the log, gc the console.

// verbs are the keys that wait for an object, and what the bar calls
// each while it waits.
var verbs = map[string]string{
	"u": "Bring up",
	"x": "End",
	"r": "Restart",
	"c": "Talk to",
	"l": "Output of",
	"i": "Page of",
	"s": "Shell at",
	"a": "Contact on",
}

// An act is a verb and its object, kept for . to do again.
type act struct {
	verb, object string
}

// verbKey answers the key after a verb: its object, or anything else,
// which lets the verb go and does nothing, as esc does in vim.
func (m model) verbKey(verb, k, came string) (model, tea.Cmd) {
	switch k {
	case verb, "p", "j", "k", "G":
		m.last = act{verb, k}
		return m.doAct(act{verb, k}, came)
	}
	return m, nil
}

// doAct does a verb on its object: the project, or the row the object
// names, which the cursor goes to first so the eye is on what was done.
func (m model) doAct(a act, came string) (model, tea.Cmd) {
	switch a.object {
	case "j":
		m = m.onRow(0, ring(m.cursorAt+1, rowsIn(m.projects)))
	case "k":
		m = m.onRow(0, ring(m.cursorAt-1, rowsIn(m.projects)))
	case "G":
		m = m.onRow(0, rowsIn(m.projects)-1)
	}
	e, pl, ok := m.under()
	if !ok {
		return m, nil
	}
	if a.object == "p" {
		return m.onProject(a.verb, pl, came)
	}
	return m.onEntry(a.verb, e, pl)
}

// onEntry is a verb on one row.
func (m model) onEntry(verb string, e work.Entry, pl work.Project) (model, tea.Cmd) {
	switch verb {
	case "u":
		return m, m.raiseOn(e)
	case "x":
		m.kill = m.endOn(e)
		return m, nil
	case "r":
		return m, m.restartRows([]work.Entry{e}, pl.Path)
	case "c":
		if !m.inside {
			return m, nil
		}
		if p := m.programUnder(e); p != nil {
			return m, m.openClient(e, p, pl.Path)
		}
		if serving(e) {
			return m, m.openServing(e)
		}
		m.notice = "conn knows no client for " + work.Program(e.AsTyped())
		return m, nil
	case "l":
		if p, ok := m.panes[e.TTY]; ok && room.Reachable(p) && m.inside {
			m.peek, m.bay.readout = true, false
		}
		return m, m.outputOf(e)
	case "i":
		if m.inside {
			return m, m.openReadout()
		}
	case "s":
		if !m.inside {
			return m, nil
		}
		if e.Container != "" {
			return m, m.shellInContainer(e)
		}
		dir := e.Cwd
		if dir == "" || !work.Within(dir, pl.Path) {
			dir = pl.Path
		}
		return m, m.openShell(dir)
	case "a":
		if m.inside && pl.Path != "" {
			return m, m.contactOn(e, pl.Path)
		}
	}
	return m, nil
}

// onProject is a verb on the row's project.
func (m model) onProject(verb string, pl work.Project, came string) (model, tea.Cmd) {
	switch verb {
	case "u":
		return m.raiseAt()
	case "x":
		m.kill = m.endProject(pl)
		return m, nil
	case "r":
		var rows []work.Entry
		for _, e := range projectWork(pl) {
			if m.restartable(e) {
				rows = append(rows, e)
			}
		}
		return m, m.restartRows(rows, pl.Path)
	case "l":
		if pl.Path != "" {
			return m.openOutput(pl.Path, came)
		}
	case "i":
		if m.inside {
			return m, m.openReadout()
		}
	case "s":
		if m.inside && pl.Path != "" {
			return m, m.openShell(pl.Path)
		}
	case "a":
		if m.inside && pl.Path != "" {
			return m, m.startContact(pl.Path)
		}
	}
	return m, nil
}

// projectWork is the rows of a project that are its work and not the
// operator's company: runs, services and containers, each once — a
// row under one already taken goes with it. A contact, a shell and an
// editor are where the operator works, and xp ending the project's
// work leaves them be.
func projectWork(pl work.Project) []work.Entry {
	var out []work.Entry
	taken := -1
	for _, e := range pl.Entries {
		if taken >= 0 && e.Depth > taken {
			continue
		}
		taken = -1
		switch e.Kind {
		case work.KindContact, work.KindShell, work.KindEditor:
			continue
		}
		if e.Status == work.StatusDown || work.Over(e.Status) && e.Declared == "" {
			continue
		}
		out = append(out, e)
		taken = e.Depth
	}
	return out
}

// endProject arms one question for ending a project's work: what each
// row's own x would do, all of it on y.
func (m model) endProject(pl work.Project) *pendingKill {
	var ends []tea.Cmd
	for _, e := range projectWork(pl) {
		if k := m.endOn(e); k != nil {
			ends = append(ends, k.end)
		}
	}
	if len(ends) == 0 {
		return nil
	}
	return &pendingKill{prompt: question(fmt.Sprintf("end %d of its processes", len(ends)), projectName(pl.Path, m.roots.real, m.head.Login.Home)), end: tea.Batch(ends...)}
}

// restartable says whether r can start a row again: a container, a
// brew service, or a pane conn opened for it by name.
func (m model) restartable(e work.Entry) bool {
	if e.Container != "" || e.Brew != "" {
		return true
	}
	p, ok := m.panes[e.TTY]
	return ok && e.Declared != "" && p.Declared == e.Declared
}

// restartRows starts rows again, off the loop, as conn r does, and
// says what would not be.
func (m model) restartRows(rows []work.Entry, project string) tea.Cmd {
	if !m.inside || len(rows) == 0 {
		return nil
	}
	h := here{read: read{reading: m.reading}, srv: m.srv, home: m.head.Login.Home}
	return func() tea.Msg {
		var failed []string
		for _, e := range rows {
			if _, err := h.restart(found{Entry: e, project: project}); err != nil {
				failed = append(failed, err.Error())
			}
		}
		if len(failed) > 0 {
			return noticeMsg{strings.Join(failed, "; ")}
		}
		return killedMsg{command: "restart"}
	}
}

// outputOf puts what a row said in the workspace and leaves the keys on
// the panel: its pane, where conn holds one, until the next key, the
// page taking the workspace back after it; a container's log, or a
// brew service's, in a pane opened to follow it.
func (m model) outputOf(e work.Entry) tea.Cmd {
	if !m.inside {
		return nil
	}
	if p, ok := m.panes[e.TTY]; ok && room.Reachable(p) {
		srv := m.srv
		return func() tea.Msg {
			if err := srv.Preview(p); err != nil {
				return noticeMsg{"its output could not be shown: " + err.Error()}
			}
			return nil
		}
	}
	switch {
	case e.Container != "":
		return m.watchContainer(e)
	case e.Brew != "":
		return m.watchBrew(e)
	}
	name := work.Program(e.AsTyped())
	return func() tea.Msg {
		return noticeMsg{name + " runs in a terminal conn does not hold"}
	}
}

// contactOn opens a contact at a project handed one row: what it is,
// how it stands, and the last of what it said, so the first turn is
// about the process and not about finding it. The prompt is Claude
// Code's first, given on its command line.
func (m model) contactOn(e work.Entry, dir string) tea.Cmd {
	srv := m.srv
	pane, held := m.panes[e.TTY]
	return func() tea.Msg {
		var said []string
		if held {
			said, _ = srv.Tail(pane.ID, contactLines)
		}
		cmd := claude.Command(room.ContactNote(srv.Socket)) + " " + shell.Quote(handedPrompt(e, trimBlank(said)))
		sh, err := srv.OpenCmd(dir, cmd)
		if err != nil {
			return noticeMsg{"the contact could not be opened: " + err.Error()}
		}
		return openedMsg{shell: sh}
	}
}

// contactLines is how much of what a row said a contact is handed.
const contactLines = 60

// handedPrompt is what a contact is told of the row it is handed.
func handedPrompt(e work.Entry, said []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Look at this process, which conn handed you: %s", e.AsTyped())
	if e.PID > 0 {
		fmt.Fprintf(&b, ", pid %d", e.PID)
	}
	fmt.Fprintf(&b, ", %s", strings.ToLower(string(e.Status)))
	if len(e.Ports) > 0 {
		fmt.Fprintf(&b, ", on :%s", strings.Join(e.Ports, ", :"))
	}
	if e.Cwd != "" {
		fmt.Fprintf(&b, ", working in %s", e.Cwd)
	}
	b.WriteString(".")
	if len(said) > 0 {
		b.WriteString(" The last of what it said:\n\n")
		b.WriteString(strings.Join(said, "\n"))
	}
	b.WriteString("\n\nSay what you make of it, and ask before you change anything.")
	return b.String()
}
