package main

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/w0zro/conn/internal/room"
	"github.com/w0zro/conn/internal/work"
	"github.com/w0zro/conn/internal/work/brew"
	"github.com/w0zro/conn/internal/work/declared"
)

// raise brings one declaration up: the answer to enter on a down row,
// in the bay with the keys in it, and to u, parked, the keys left on
// the panel. replace is the pane holding the last run of it, where one
// stands.
func (m model) raise(path string, d declared.Declaration, replace string, enter bool) tea.Cmd {
	srv := m.srv
	return func() tea.Msg {
		sh, err := srv.RaiseDeclared(d.At(path), d.Command, d.Name, declared.Mark(path, d.Name), replace, enter)
		if err != nil {
			return nil
		}
		if !enter {
			return raisedMsg{shells: []room.Shell{sh}}
		}
		return openedMsg{shell: sh}
	}
}

// raiseAll brings up everything a project declares that is not up, in
// the order of the file, parked: the keys stay on the panel and the
// reading lists the rows as they come. The file is read fresh, off the
// loop, so what is raised is what it says now. up is passed over, and
// held is replaced, by mark.
func (m model) raiseAll(path string, up map[string]bool, held map[string]string) tea.Cmd {
	srv := m.srv
	return func() tea.Msg {
		list, err := declared.Read(path)
		if err != nil {
			return nil
		}
		var shells []room.Shell
		for _, d := range list {
			mark := declared.Mark(path, d.Name)
			if up[mark] {
				continue
			}
			// A brew service is started by brew, not in a pane.
			if formula, ok := declared.BrewArgs(d.Command); ok {
				_, _ = brew.Says(brew.Wait, "services", "start", formula)
				continue
			}
			sh, err := srv.RaiseDeclared(d.At(path), d.Command, d.Name, mark, held[mark], false)
			if err != nil {
				continue
			}
			shells = append(shells, sh)
		}
		return raisedMsg{shells: shells}
	}
}

// interruptDeclared sends ctrl-c to a declared process's pane and,
// once the pane has recorded the end, takes the pane down, so the row
// goes straight to DOWN rather than standing ENDED for a second x: an
// end the operator asked for has nothing in it to read. A command that
// dies of the ctrl-c takes the shell with it and the pane closes on its
// own, which is the same end. The wait is bounded by closeWait; a
// process that does not answer keeps its pane, and the row goes on
// saying it is up.
func (m model) interruptDeclared(pane, command string) tea.Cmd {
	srv := m.srv
	return func() tea.Msg {
		if err := srv.Interrupt(pane); err == nil {
			deadline := time.Now().Add(closeWait)
			for time.Now().Before(deadline) {
				exit, err := srv.PaneExit(pane)
				if err != nil {
					break // the pane is gone, and so is the process
				}
				if exit != "" {
					_ = srv.ClosePane(pane)
					break
				}
				time.Sleep(closePoll)
			}
		}
		return killedMsg{command: command}
	}
}

// closeHeld takes down the pane a declared process ended in.
func (m model) closeHeld(id, name string) tea.Cmd {
	srv := m.srv
	return func() tea.Msg {
		_ = srv.ClosePane(id)
		return killedMsg{command: name}
	}
}

// declarationOf is the declaration a row stands for, from the file as
// last read.
func (m model) declarationOf(e work.Entry) (path string, d declared.Declaration, ok bool) {
	path, name, ok := declared.Unmark(e.Declared)
	if !ok {
		return "", declared.Declaration{}, false
	}
	for _, d := range m.declared[path].List {
		if d.Name == name {
			return path, d, true
		}
	}
	return "", declared.Declaration{}, false
}

// endDeclared is x on a declared process's row. Down, there is nothing
// to end. Ended and holding its pane, the pane is what goes, and the
// question says close. Up, it is sent ctrl-c in its pane, the way a
// hand stops what it ran in the foreground, and the pane goes once the
// end is recorded, so the row is DOWN in the one move rather than
// ENDED for a second x.
func (m model) endDeclared(e work.Entry) *pendingKill {
	if e.TTY == "" {
		return nil
	}
	_, name, _ := declared.Unmark(e.Declared)
	id := m.panes[e.TTY].ID
	if m.panes[e.TTY].Exit != "" {
		return &pendingKill{prompt: closePrompt(id, name), end: m.closeHeld(id, name)}
	}
	return &pendingKill{prompt: interruptPrompt(id, name), end: m.interruptDeclared(id, name)}
}

// declaredActs are the keys on a process a project's .conn declares.
type declaredActs struct{}

// enter on a declaration down brings it up and goes in. One started by
// hand is up, and has no pane of conn's to go into.
func (declaredActs) enter(m model, e work.Entry) (string, tea.Cmd) {
	if e.Status != work.StatusDown {
		return "", nil
	}
	if path, d, ok := m.declarationOf(e); ok {
		return "Bring it up, go in", m.raise(path, d, "", true)
	}
	return "", nil
}

// end stops a declaration in a pane conn opened for it in that pane.
// One started by hand is a process like any other, wherever it runs,
// and is signalled as one.
func (declaredActs) end(m model, e work.Entry) *pendingKill {
	if e.Status == work.StatusDown || m.panes[e.TTY].Declared == e.Declared {
		return m.endDeclared(e)
	}
	return processActs{}.end(m, e)
}

// raise opens the declaration anew in place of the pane holding its
// last run, where one stands.
func (declaredActs) raise(m model, e work.Entry) tea.Cmd {
	path, d, ok := m.declarationOf(e)
	if !ok {
		return nil
	}
	return m.raise(path, d, m.panes[e.TTY].ID, false)
}

// down is a declaration not up: down, or ended and holding its pane.
func (declaredActs) down(e work.Entry, panes map[string]room.Pane) bool {
	if e.TTY == "" {
		return e.Status == work.StatusDown
	}
	return panes[e.TTY].Exit != ""
}
