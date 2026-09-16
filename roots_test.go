package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// tree makes a home with directories to complete against.
func tree(t *testing.T, names ...string) string {
	t.Helper()
	home := t.TempDir()
	for _, n := range names {
		if err := os.MkdirAll(filepath.Join(home, n), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

// What is typed is completed against the directories that are there.
// A path stopped on a separator is a directory to look inside; anything
// else is a name half-written, and only the names carrying on from it
// answer.
func TestATypedPathIsCompletedAgainstTheMachine(t *testing.T) {
	home := tree(t, "projects", "projects/conn", "projects/vim.pro", "prospect", "Music", ".hidden")
	for _, c := range []struct {
		typed string
		want  []string
	}{
		{"~/", []string{"Music", "projects", "prospect"}},
		{"~/pro", []string{"projects", "prospect"}},
		{"~/projects", []string{"projects"}},
		{"~/projects/", []string{"projects/conn", "projects/vim.pro"}},
		{"~/zzz", nil},
		{"", []string{"Music", "projects", "prospect"}},
	} {
		var want []string
		for _, w := range c.want {
			want = append(want, filepath.Join(home, w))
		}
		got := completeRoot(c.typed, home)
		if len(got) != len(want) {
			t.Errorf("%q completes to %q, want %q", c.typed, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%q completes to %q, want %q", c.typed, got, want)
				break
			}
		}
	}
}

// A directory the machine keeps for itself is not what a checkout is
// kept in, and a list of them is a list of somebody else's business.
// Asked for by name, it answers.
func TestAHiddenDirectoryAnswersOnlyWhenAskedFor(t *testing.T) {
	home := tree(t, ".config", "code")
	if got := completeRoot("~/", home); len(got) != 1 || filepath.Base(got[0]) != "code" {
		t.Errorf("the hidden directory was offered unasked: %q", got)
	}
	if got := completeRoot("~/.co", home); len(got) != 1 || filepath.Base(got[0]) != ".config" {
		t.Errorf("asked for by name it did not answer: %q", got)
	}
}

// Only directories: a root is one, and a file is not somewhere to walk.
func TestOnlyDirectoriesAnswer(t *testing.T) {
	home := tree(t, "projects")
	if err := os.WriteFile(filepath.Join(home, "project-notes.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := completeRoot("~/pro", home); len(got) != 1 || filepath.Base(got[0]) != "projects" {
		t.Errorf("a file was offered as a root: %q", got)
	}
}

// Saving keeps what conn does not understand. A config may hold
// settings from a conn older or newer than this one, and a save that
// dropped them would be conn deciding they did not matter.
func TestSavingARootKeepsTheRestOfTheFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	home := t.TempDir()
	path := configPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"agentRuns": {"ollama": "ollama launch"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := saveRoots(home, []string{"~/projects"}); err != nil {
		t.Fatal(err)
	}
	var back map[string]json.RawMessage
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("what conn wrote will not parse: %v\n%s", err, b)
	}
	if _, ok := back["agentRuns"]; !ok {
		t.Errorf("the save dropped what conn does not read:\n%s", b)
	}
	t.Setenv("CONN_ROOTS", "")
	got, err := projectRoots(home)
	if err != nil || len(got) != 1 || got[0] != filepath.Join(home, "projects") {
		t.Errorf("conn reads back %q (%v) from the file it wrote", got, err)
	}
}

// A file conn cannot parse is a file conn cannot keep, so it is not
// written over: overwriting is how somebody's config is lost.
func TestSavingWillNotWriteOverAFileItCannotRead(t *testing.T) {
	home := writeConfig(t, `{"roots": [`)
	if err := saveRoots(home, []string{"~/projects"}); err == nil {
		t.Fatal("conn wrote over a file it could not parse")
	}
	b, err := os.ReadFile(configPath(home))
	if err != nil || string(b) != `{"roots": [` {
		t.Errorf("the file was changed: %v %s", err, b)
	}
}

// The asking view says what the keys do, and fits the panel, which is
// where it is read.
func TestTheAskingViewSaysHowToAnswerIt(t *testing.T) {
	home := tree(t, "projects")
	b := composeRoots("~/pro", home)
	rows := drawRoots(b, 0, panelWidth, 12, plain)
	var text []string
	for _, r := range rows {
		text = append(text, r.text)
		if n := utf8.RuneCountInString(r.text); n > panelWidth {
			t.Errorf("a row is %d wide in a %d panel: %q", n, panelWidth, r.text)
		}
	}
	all := strings.Join(text, "\n")
	for _, want := range []string{"ROOTS", "ROOT  ~/pro", "~/projects", "ENTER SAVES ONE"} {
		if !strings.Contains(all, want) {
			t.Errorf("the view lacks %q:\n%s", want, all)
		}
	}
}

// Told nowhere to look, going on from the console arrives at the asking
// view rather than at an empty processes view — which is also what a
// machine with no checkouts looks like, and what a conn pointed at the
// wrong directory looks like.
func TestTheConsoleGoesToTheAskingViewWithNoRoots(t *testing.T) {
	m := model{head: station{build: testStation.build, login: testStation.login},
		now: testNow, p: plain, width: 120, height: 40, uid: 501, roots: testRoots}
	st := testStation
	m.st = &st
	m.stage = lastStage(m.report())
	next, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = next.(model)
	if m.view != viewRoots {
		t.Fatalf("a conn with no roots went to view %d, not the asking view", m.view)
	}
	// It opens on the home, which certainly exists, so the first thing
	// shown is a list rather than nothing.
	if m.rootTyped != "~/" {
		t.Errorf("the line opens on %q", m.rootTyped)
	}
}

// Enter writes the root and conn is working from it before the view is
// gone: the reading names projects by the roots just written, not by
// the ones conn came up with.
func TestAnsweringTheAskingViewPutsConnToWork(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CONN_ROOTS", "")
	home := tree(t, "work", "work/conn")
	m := model{head: station{login: login{home: home}}, p: plain, width: 120, height: 40}
	mm, _ := m.toRoots()
	m = mm.(model)
	for _, k := range []string{"w", "o", "r", "k"} {
		next, _ := m.rootsKey(k)
		m = next.(model)
	}
	if m.rootTyped != "~/work" {
		t.Fatalf("the line reads %q", m.rootTyped)
	}
	next, cmd := m.rootsKey("enter")
	m = next.(model)
	if m.rootErr != "" {
		t.Fatalf("saving said %q", m.rootErr)
	}
	if m.view != viewProcesses || cmd == nil {
		t.Errorf("answering left conn in view %d with cmd %v", m.view, cmd != nil)
	}
	// On disk, and conn reading it back.
	got, err := projectRoots(home)
	if err != nil || len(got) != 1 || got[0] != filepath.Join(home, "work") {
		t.Fatalf("the config holds %q (%v)", got, err)
	}
	// And in force, without waiting for a restart.
	if len(m.projRoots) != 1 || m.isProject == nil {
		t.Errorf("conn is not working from the root it just saved: %q", m.projRoots)
	}
	if m.roots(filepath.Join(home, "work", "conn")) == "" {
		t.Error("the reading does not name projects by the new root")
	}
}

// Tab fills the line in with the directory under the cursor and puts a
// separator after it, so the next keystroke is already looking inside.
// It is not an answer: the line is filled and the asking goes on.
func TestTabFillsInTheLineWithoutAnsweringIt(t *testing.T) {
	home := tree(t, "projects", "projects/conn", "prospect")
	m := model{head: station{login: login{home: home}}, p: plain}
	mm, _ := m.toRoots()
	m = mm.(model)
	for _, k := range []string{"p", "r", "o"} {
		next, _ := m.rootsKey(k)
		m = next.(model)
	}
	// ~/pro answers with two; the cursor is on the first.
	next, _ := m.rootsKey("tab")
	m = next.(model)
	if m.rootTyped != "~/projects/" {
		t.Fatalf("tab filled the line with %q", m.rootTyped)
	}
	if m.view != viewRoots {
		t.Error("tab answered the question instead of filling it in")
	}
	// And the line now looks inside what it named.
	if got := composeRoots(m.rootTyped, home); len(got.rows) != 1 || got.rows[0] != "~/projects/conn" {
		t.Errorf("after tab the line answers with %q", got.rows)
	}
	// The cursor walks what answers, and tab takes the one it is on.
	next, _ = m.rootsKey("ctrl+u")
	m = next.(model)
	for _, k := range []string{"~", "/", "p", "r", "o"} {
		next, _ := m.rootsKey(k)
		m = next.(model)
	}
	next, _ = m.rootsKey("down")
	m = next.(model)
	next, _ = m.rootsKey("tab")
	if got := next.(model).rootTyped; got != "~/prospect/" {
		t.Errorf("tab on the second row filled the line with %q", got)
	}
}
