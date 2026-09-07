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
// others are. n asks where, on a line prefilled with the place the cursor
// is in, relative to the root it was found under — w0zro/ for a cursor on
// a repository in that folder — and the name goes on the end of it; the
// prefix backspaced away is the root itself, and another folder typed in
// its place is that folder, made if it has to be. enter makes the
// directory, runs git init in it, and opens a shell there: the thing you
// would do next, and what lists the project while nothing else is
// running in it.

// openCreate starts typing where the new project goes. The line is
// prefilled with the folder the cursor is in, relative to its root, so
// the name goes beside the neighbors it has and the folder is there to be
// changed.
func (m *model) openCreate() tea.Cmd {
	dir := m.newProjectDir()
	if dir == "" {
		m.status, m.statusErr = "no projects directory to make it in", true
		return nil
	}
	root, prefix := m.rootOf(dir)
	m.creating, m.newIn = true, root
	m.newName = newLine()
	m.newName.SetValue(prefix)
	m.resume = nil // one thing at a time; the name is it now
	return nil
}

// rootOf is the root a directory is under, and the directory's path
// inside it, with a slash on the end to type a name after — nothing for
// the root itself. A directory under no root is its own root.
func (m model) rootOf(dir string) (root, prefix string) {
	for _, r := range m.roots {
		// Symlinks resolved, to match how the scan resolved the places
		// under it, so a root reached through a symlink still holds them.
		if real, err := filepath.EvalSymlinks(r); err == nil {
			r = real
		}
		if under(dir, r) {
			rel, err := filepath.Rel(r, dir)
			if err != nil || rel == "." {
				return r, ""
			}
			return r, filepath.ToSlash(rel) + "/"
		}
	}
	return dir, ""
}

// newProjectDir is the folder a project made now goes in: the group the
// cursor is on; the folder the repository it is on or in was found in;
// and, with nothing under the cursor, the first of the roots. Nothing
// when there is not even a root.
func (m model) newProjectDir() string {
	if r, ok := m.selected(); ok {
		if r.kind == rowGroup && r.project.Path != dockerPlace {
			return r.project.Path
		}
		if repo, ok := m.repoHolding(r.project.Path); ok {
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

// createKey handles a keystroke while the line is being typed: enter makes
// the project, esc cancels, and every other key edits the line.
// A line that will not do is said, and stays for a better one.
func (m *model) createKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		dir, err := newProjectPath(m.newIn, m.newName.Value())
		if err != nil {
			m.status, m.statusErr = err.Error(), true
			return nil
		}
		m.creating = false
		m.status, m.statusErr = "making "+filepath.Base(dir), false
		return createProject(dir)
	case "esc":
		m.creating = false
		m.status = ""
		return nil
	}
	m.newName, _ = m.newName.Update(msg)
	return nil
}

// newProjectPath is where the line says the project goes: a name, or a
// path with the name on the end, under the root; an absolute path, or one
// from ~, is where it says. A path that climbs out of the root would put
// the project somewhere the line did not say.
func newProjectPath(root, typed string) (string, error) {
	typed = strings.TrimSpace(typed)
	if typed == "" {
		return "", errors.New("a name is needed")
	}
	dir := expandPath(typed)
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(root, dir)
	}
	dir = filepath.Clean(dir)
	if dir == filepath.Clean(root) || strings.Contains(typed, "..") {
		return "", errors.New("a name, or a path to one: " + typed)
	}
	return dir, nil
}

// createdMsg says a project was made, or why not.
type createdMsg struct {
	dir  string // the repository's path
	name string
	err  error
}

// createProject makes the directory — and the folders on the way to it,
// a new group being one — and the repository in it, off the render path:
// git init is quick, but not on a network mount. A directory already
// there is not made over — it may well be a project, and this is no way
// to find out — and a git that fails leaves nothing behind.
func createProject(dir string) tea.Cmd {
	return func() tea.Msg {
		name := filepath.Base(dir)
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
// scan finds it, and a shell opens there meanwhile — it lists the project
// before then, and is where the work in it starts.
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
