package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// enter presses enter.
func enter(m model) (model, tea.Cmd) {
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	return next.(model), cmd
}

func TestNAsksForANameAndSaysWhereTheProjectGoes(t *testing.T) {
	// The name is typed on the status line, in a mode of its own, with
	// the directory the project will be made in beside it: the one thing
	// worth knowing before enter.
	m := withProcList(90, 14, []Project{{Name: "brand", Path: "/p/brand"}}, nil)
	m = press(m, "n")
	if !m.creating || m.newIn != "/p" {
		t.Fatalf("creating = %v in %q, want the name asked for, beside brand", m.creating, m.newIn)
	}
	m = typeFilter(m, "site")
	st := m.statusLine()
	if !strings.Contains(st.mode, "NEW site") || !strings.Contains(st.msg, "in /p") {
		t.Errorf("status = %+v, want the name being typed and where it goes", st)
	}
	// The letters are the name's, not the list's: s did not open a shell.
	if len(m.terms) != 0 {
		t.Errorf("terms = %v, want none opened by the s in site", m.terms)
	}
}

func TestEscThinksBetterOfTheNewProject(t *testing.T) {
	m := withProcList(90, 14, []Project{{Name: "brand", Path: "/p/brand"}}, nil)
	m = typeFilter(press(m, "n"), "site")
	m = press(m, "esc")
	if m.creating {
		t.Error("still creating after esc")
	}
	if st := m.statusLine(); st.mode != "" {
		t.Errorf("mode = %q, want the line gone", st.mode)
	}
}

func TestANewProjectGoesWhereTheCursorIs(t *testing.T) {
	// Into a group; beside a repository, in its group or the directory it
	// was found in; beside the repository a sub-project or a process is
	// in; and, with nothing under the cursor, the first root.
	m := withProcList(90, 14, []Project{
		{Name: "api", Path: "/p/mono/api", Group: "/p/mono"},
		{Name: "brand", Path: "/p/brand"},
	}, []Proc{{PID: 700, PPID: 1, Command: "zsh", Dir: "/p/brand/site"}})
	m.groups = []Project{{Name: "mono", Path: "/p/mono"}}
	m.grouped = map[string][]Project{"/p/mono": {m.projects[0]}}
	m.subs = map[string][]Project{"/p/brand": {{Name: "site", Path: "/p/brand/site"}}}
	m.roots = []string{"/p"}
	m.rebuild()

	want := map[string]string{
		"/p/mono":       "/p/mono", // the group
		"/p/mono/api":   "/p/mono", // a repository in it
		"/p/brand":      "/p",      // a repository standing alone
		"/p/brand/site": "/p",      // its sub-project
	}
	for i, r := range m.rows {
		m.cursor = i
		if r.kind == rowProc {
			if got := m.newProjectDir(); got != "/p" {
				t.Errorf("from the process in brand/site: %q, want beside brand", got)
			}
			continue
		}
		if got := m.newProjectDir(); got != want[r.project.Path] {
			t.Errorf("from %s: %q, want %q", r.project.Path, got, want[r.project.Path])
		}
	}

	m.rows, m.cursor = nil, 0
	if got := m.newProjectDir(); got != "/p" {
		t.Errorf("with nothing under the cursor: %q, want the first root", got)
	}
	m.roots = nil
	if got := m.newProjectDir(); got != "" {
		t.Errorf("with no root: %q, want nothing to make it in", got)
	}
}

func TestTheLineIsPrefilledWithTheCursorsFolder(t *testing.T) {
	// From a repository in w0zro, the line reads w0zro/ and the name goes
	// after it; backspaced away, the project goes at the root, which no
	// row leads to once every repository is in a folder.
	m := withProcList(90, 14, []Project{{Name: "conn", Path: "/p/w0zro/conn", Group: "/p/w0zro"}}, nil)
	m.groups = []Project{{Name: "w0zro", Path: "/p/w0zro"}}
	m.grouped = map[string][]Project{"/p/w0zro": {m.projects[0]}}
	m.roots = []string{"/p"}
	m.rebuild()
	m.cursor = len(m.rows) - 1 // the repository

	m = press(m, "n")
	if m.newIn != "/p" || m.newName.Value() != "w0zro/" {
		t.Fatalf("line = %q in %q, want w0zro/ under the root", m.newName.Value(), m.newIn)
	}
	if got, err := newProjectPath(m.newIn, m.newName.Value()+"site"); err != nil || got != "/p/w0zro/site" {
		t.Errorf("path = %q (%v), want the name after the prefix", got, err)
	}
	if got, err := newProjectPath(m.newIn, "site"); err != nil || got != "/p/site" {
		t.Errorf("path = %q (%v), want the root with the prefix gone", got, err)
	}
	if got, err := newProjectPath(m.newIn, "other/site"); err != nil || got != "/p/other/site" {
		t.Errorf("path = %q (%v), want a folder typed in the prefix's place", got, err)
	}
	if got, err := newProjectPath(m.newIn, "~/elsewhere/site"); err != nil || !strings.HasSuffix(got, "/elsewhere/site") || strings.HasPrefix(got, "/p") {
		t.Errorf("path = %q (%v), want a path from ~ taken as it is", got, err)
	}
}

func TestALineThatClimbsOutIsRefused(t *testing.T) {
	// The line said the project goes under the root; a line that climbs
	// out, or names the root itself, would put it somewhere else. The
	// line stays for a better one.
	m := withProcList(90, 14, []Project{{Name: "brand", Path: "/p/brand"}}, nil)
	for _, bad := range []string{"", ".", "..", "../x", "w0zro/../../x"} {
		m = press(m, "n")
		m.newName.SetValue(bad)
		var cmd tea.Cmd
		m, cmd = enter(m)
		if cmd != nil || !m.creating || !m.statusErr {
			t.Errorf("%q: cmd = %v, creating = %v, err = %v; want refused with the line kept", bad, cmd != nil, m.creating, m.statusErr)
		}
		m = press(m, "esc")
	}
}

func TestAFolderOnTheWayIsMade(t *testing.T) {
	// A folder typed before the name that is not there yet is made: a new
	// group, with its first repository in it.
	root := t.TempDir()
	msg := createProject(filepath.Join(root, "new", "site"))().(createdMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	if _, err := os.Stat(filepath.Join(root, "new", "site", ".git")); err != nil {
		t.Errorf("no repository at new/site: %v", err)
	}
}

func TestANewProjectIsMadeAndAShellOpensThere(t *testing.T) {
	// enter makes the directory and the repository in it, opens a shell
	// there, and the cursor lands on the project once the scan lists it.
	root := t.TempDir()
	repo := filepath.Join(root, "brand")
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Skipf("git init: %v\n%s", err, out)
	}
	m := withProcList(90, 14, []Project{{Name: "brand", Path: repo}}, nil)
	m.roots = []string{root}
	m, asked := pipeServer(t, m)
	m = typeFilter(press(m, "n"), "site")
	m, cmd := enter(m)
	if m.creating || cmd == nil {
		t.Fatal("enter did not start making the project")
	}
	msg, ok := cmd().(createdMsg)
	if !ok || msg.err != nil {
		t.Fatalf("made = %+v, want the project made", msg)
	}
	site := filepath.Join(root, "site")
	if _, err := os.Stat(filepath.Join(site, ".git")); err != nil {
		t.Fatalf("no repository in %s: %v", site, err)
	}

	next, cmd := m.Update(msg)
	m = next.(model)
	if m.status != "made site" || m.statusErr {
		t.Errorf("status = %q (err %v), want made site", m.status, m.statusErr)
	}
	if got := askedForKind(t, asked, kindOpen); got.Dir != site || got.Run != "" {
		t.Errorf("opened %+v, want a shell in %s", got, site)
	}
	if _, rescan := cmd().(projectsMsg); !rescan {
		t.Error("the project scan was not asked for")
	}

	// The scan lists it; the cursor is on it.
	m.showAll = true
	next, _ = m.Update(projectsMsg{projects: []Project{
		{Name: "brand", Path: repo}, {Name: "site", Path: site},
	}, roots: []string{root}})
	m = next.(model)
	if r, ok := m.selected(); !ok || r.project.Path != site {
		t.Errorf("cursor on %+v, want the new project", r)
	}
	if m.landOn != "" {
		t.Error("the landing is still owed after it was made")
	}
}

func TestAProjectAlreadyThereIsNotMadeOver(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "site"), 0o755); err != nil {
		t.Fatal(err)
	}
	msg := createProject(filepath.Join(root, "site"))().(createdMsg)
	if msg.err == nil || !strings.Contains(msg.err.Error(), "already there") {
		t.Errorf("err = %v, want the directory left alone", msg.err)
	}
	if _, err := os.Stat(filepath.Join(root, "site", ".git")); err == nil {
		t.Error("a repository was made in a directory that was already there")
	}
}
