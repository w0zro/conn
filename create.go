package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// A new project is a directory with a repository in it, made where the
// others are. n asks for a name, makes the directory under the place the
// cursor is in — inside a group, beside a repository — runs git init in
// it, and opens a shell there: the thing you would do next, and what lists
// the project while nothing else is running in it.

// openCreate starts typing the name of a new project. Where it will go is
// settled now, from the row the cursor is on, and said beside the name as
// it is typed.
func (m *model) openCreate() tea.Cmd {
	dir := m.newProjectDir()
	if dir == "" {
		m.status, m.statusErr = "no projects directory to make it in", true
		return nil
	}
	m.creating, m.newIn = true, dir
	m.newName = newLine()
	m.resume = nil // one thing at a time; the name is it now
	return nil
}

// newProjectDir is where a project made now goes: into the group the
// cursor is on; beside the repository it is on or in — in its group, or
// the directory it was found in; and, with nothing under the cursor, the
// first of the roots. Nothing when there is not even a root.
func (m model) newProjectDir() string {
	if r, ok := m.selected(); ok {
		if r.kind == rowGroup {
			return r.project.Path
		}
		if repo, ok := m.repoHolding(r.project.Path); ok {
			if repo.Group != "" {
				return repo.Group
			}
			return filepath.Dir(repo.Path)
		}
	}
	if len(m.roots) > 0 {
		return m.roots[0]
	}
	return ""
}

// repoHolding is the repository a place is, or is inside: the deepest one
// whose path holds it, a sub-project being inside its repository.
func (m model) repoHolding(path string) (Project, bool) {
	var found Project
	ok := false
	for _, p := range m.projects {
		if under(path, p.Path) && (!ok || len(p.Path) > len(found.Path)) {
			found, ok = p, true
		}
	}
	return found, ok
}

// createKey handles a keystroke while the name is being typed: enter makes
// the project, esc thinks better of it, and everything else is the line's.
// A name that will not do is said, and the line stays for a better one.
func (m *model) createKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		name := strings.TrimSpace(m.newName.Value())
		if err := checkName(name); err != nil {
			m.status, m.statusErr = err.Error(), true
			return nil
		}
		m.creating = false
		m.status, m.statusErr = "making "+name, false
		return createProject(m.newIn, name)
	case "esc":
		m.creating = false
		m.status = ""
		return nil
	}
	m.newName, _ = m.newName.Update(msg)
	return nil
}

// checkName is what a project can be called: one directory's name. A path
// would make it somewhere other than where the line said it would go.
func checkName(name string) error {
	switch {
	case name == "":
		return errors.New("a name is needed")
	case name == "." || name == ".." || strings.ContainsAny(name, `/\`):
		return errors.New("a name, not a path: " + name)
	}
	return nil
}

// createdMsg says a project was made, or why not.
type createdMsg struct {
	dir  string // the repository's path
	name string
	err  error
}

// createProject makes the directory and the repository in it, off the
// render path: git init is quick, but not on a network mount. A directory
// already there is not made over — it may well be a project, and this is
// no way to find out — and a git that fails leaves nothing behind.
func createProject(in, name string) tea.Cmd {
	return func() tea.Msg {
		dir := filepath.Join(in, name)
		if _, err := os.Lstat(dir); err == nil {
			return createdMsg{dir: dir, name: name, err: errors.New(dir + " is already there")}
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return createdMsg{dir: dir, name: name, err: err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
		defer cancel()
		if out, err := exec.CommandContext(ctx, "git", "init", "-q", dir).CombinedOutput(); err != nil {
			_ = os.Remove(dir)
			said := strings.TrimSpace(string(out))
			if said == "" {
				said = err.Error()
			}
			return createdMsg{dir: dir, name: name, err: errors.New("git init: " + said)}
		}
		return createdMsg{dir: dir, name: name}
	}
}

// created takes the answer: the cursor is owed the new project once the
// scan finds it, and a shell opens there meanwhile, which is what lists
// it before then and where the work in it starts.
func (m *model) created(msg createdMsg) tea.Cmd {
	if msg.err != nil {
		m.status, m.statusErr = msg.err.Error(), true
		return nil
	}
	m.status, m.statusErr = "made "+msg.name, false
	m.landOn = msg.dir
	m.server.open(msg.dir, "", "")
	return scanProjects
}

// landIfFound puts the cursor on the project it is owed, once a scan has
// listed it. Until one has, the debt stands.
func (m *model) landIfFound() {
	if m.landOn == "" {
		return
	}
	for i, r := range m.rows {
		if r.kind == rowProject && r.project.Path == m.landOn {
			m.cursor = i
			m.scrollToCursor()
			m.landOn = ""
			return
		}
	}
}
