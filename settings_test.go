package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// configured is a home with a config file in it, and conn pointed at
// both:
// the tests here are about what conn writes, so each one writes its own
// file and reads it back.
func configured(t *testing.T, config string, dirs ...string) string {
	t.Helper()
	home := tree(t, dirs...)
	dir := filepath.Join(home, ".config")
	if err := os.MkdirAll(filepath.Join(dir, "conn"), 0o755); err != nil {
		t.Fatal(err)
	}
	if config != "" {
		if err := os.WriteFile(filepath.Join(dir, "conn", "config.json"), []byte(config), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CONN_ROOTS", "")
	return home
}

// wrote is the config file as it now stands.
func wrote(t *testing.T, home string) config {
	t.Helper()
	c, err := readConfig(home)
	if err != nil {
		t.Fatalf("the file conn wrote: %v", err)
	}
	return c
}

// settingsAt is a model on the settings view, with the cursor on a row.
func settingsAt(t *testing.T, home string, at int) model {
	t.Helper()
	m := model{p: plain, width: panelWidth, height: 40, view: viewSettings, settingAt: at}
	m.head.login.home = home
	return m
}

// The settings show the file, and the file as it is written: the roots
// it names, in its own words, with what each one turned out to be on
// this machine against the right.
func TestTheSettingsShowTheFile(t *testing.T) {
	home := configured(t, `{"roots":["~/projects","~/gone"]}`, "projects")
	b := composeSettings(home)
	if !b.present || b.err != "" {
		t.Fatalf("the file read as present %v, err %q", b.present, b.err)
	}
	if b.roots != 2 || len(b.rows) != 3 {
		t.Fatalf("%d roots in %d rows", b.roots, len(b.rows))
	}
	if b.rows[0].text != "~/projects" || b.rows[0].note != "" {
		t.Errorf("the first root is %q, noted %q", b.rows[0].text, b.rows[0].note)
	}
	if b.rows[1].note != "MISSING" {
		t.Errorf("a root that is not there is noted %q", b.rows[1].note)
	}
	if b.rows[2].kind != addRootSetting {
		t.Error("there is no row to add a root with")
	}
}

// CONN_ROOTS stands in front of the file, and the view says so. The
// rows stay the file's: this is the view that edits the file, and rows
// that were the environment's would be rows nothing here can change.
func TestTheSettingsSayWhenTheEnvironmentStandsInFront(t *testing.T) {
	home := configured(t, `{"roots":["~/projects"]}`, "projects", "elsewhere")
	t.Setenv("CONN_ROOTS", filepath.Join(home, "elsewhere"))
	b := composeSettings(home)
	if !b.forced {
		t.Fatal("the view does not know CONN_ROOTS is in force")
	}
	if len(b.rows) != 2 || b.rows[0].text != "~/projects" {
		t.Fatalf("the rows are not the file's: %+v", b.rows)
	}
	var text strings.Builder
	for _, r := range drawSettings(b, 0, panelWidth, 0, plain) {
		text.WriteString(r.text + "\n")
	}
	if !strings.Contains(text.String(), "CONN_ROOTS") {
		t.Errorf("the view does not say what is in force:\n%s", text.String())
	}
}

// A root added from the settings is added to the ones already there.
// The first start writes the one root it asked for, and a settings view
// that wrote the same way would answer "add another" by throwing the
// first one away.
func TestARootAddedFromTheSettingsKeepsTheRest(t *testing.T) {
	home := configured(t, `{"roots":["~/projects"],"theme":"datum"}`, "projects", "work")
	m := settingsAt(t, home, 1) // the row that adds one
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)
	if m.view != viewRoots || m.askingAt != -1 || m.askingBack != viewSettings {
		t.Fatalf("enter on the add row went to view %d, at %d, back to %d", m.view, m.askingAt, m.askingBack)
	}
	m.asking.set(filepath.Join(home, "work"))
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)
	if m.view != viewSettings {
		t.Fatalf("a root saved from the settings left the view at %d", m.view)
	}
	c := wrote(t, home)
	if len(c.Roots) != 2 || c.Roots[0] != "~/projects" || c.Roots[1] != "~/work" {
		t.Errorf("the file names %q", c.Roots)
	}
	// And what conn had nothing to do with is still in it.
	if c.Theme != "datum" {
		t.Errorf("the theme in the file came out %q", c.Theme)
	}
	// conn walks it now, not on the next start.
	if len(m.roots.real) != 2 {
		t.Errorf("conn is walking %q", m.roots.real)
	}
}

// Enter on a root is that root, to be typed over: what is written goes
// back where it came from, and the other roots are left alone.
func TestARootIsTypedOverWhereItStands(t *testing.T) {
	home := configured(t, `{"roots":["~/gone","~/projects"]}`, "projects", "work")
	m := settingsAt(t, home, 0)
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)
	if m.asking.text != "~/gone" || m.askingAt != 0 {
		t.Fatalf("enter on a root gave %q at %d", m.asking.text, m.askingAt)
	}
	m.asking.set(filepath.Join(home, "work"))
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)
	if c := wrote(t, home); len(c.Roots) != 2 || c.Roots[0] != "~/work" || c.Roots[1] != "~/projects" {
		t.Errorf("the file names %q", c.Roots)
	}
}

// esc leaves the typing without writing anything, and goes back to the
// settings. On the first start there is nothing to go back to and esc
// does nothing, since conn cannot show the processes view until it has
// been told where to look.
func TestEscLeavesARootAsItWas(t *testing.T) {
	home := configured(t, `{"roots":["~/projects"]}`, "projects")
	m := settingsAt(t, home, 0)
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)
	m.asking.set("~/somewhere-else")
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(model)
	if m.view != viewSettings {
		t.Fatalf("esc from a root being typed left the view at %d", m.view)
	}
	if c := wrote(t, home); len(c.Roots) != 1 || c.Roots[0] != "~/projects" {
		t.Errorf("esc wrote something: %q", c.Roots)
	}
	first := model{p: plain, width: panelWidth, height: 40}
	first.head.login.home = home
	mm, _ := first.toRoots()
	first = mm.(model)
	next, _ = first.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if next.(model).view != viewRoots {
		t.Error("esc found a way out of the first start, which has none")
	}
}

// x takes a root out of the file, and the rest of the file stands.
func TestXTakesARootOut(t *testing.T) {
	home := configured(t, `{"roots":["~/projects","~/work"]}`, "projects", "work")
	m := settingsAt(t, home, 0)
	next, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = next.(model)
	c := wrote(t, home)
	if len(c.Roots) != 1 || c.Roots[0] != "~/work" {
		t.Fatalf("the file names %q", c.Roots)
	}
	if len(m.roots.real) != 1 || !strings.HasSuffix(m.roots.real[0], "work") {
		t.Errorf("conn is walking %q", m.roots.real)
	}
	// The add row is all that is left once the last one goes.
	next, _ = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = next.(model)
	if c := wrote(t, home); len(c.Roots) != 0 {
		t.Errorf("the file still names %q", c.Roots)
	}
	if rows := m.settingsReport().rows; len(rows) != 1 || rows[0].kind != addRootSetting {
		t.Errorf("what is left is %+v", rows)
	}
}

// A file conn cannot read is not written over: conn cannot tell what is
// in it, so it cannot keep it, and the view says so rather than
// quietly making a new file out of half an answer.
func TestAFileThatWillNotParseIsNotWrittenOver(t *testing.T) {
	home := configured(t, `{"roots": [`, "projects")
	b := composeSettings(home)
	if b.err == "" {
		t.Fatal("the view says nothing about a file that will not parse")
	}
	m := settingsAt(t, home, 0)
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := next.(model).view; got != viewSettings {
		t.Fatalf("enter went to view %d with nothing to edit", got)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "conn", "config.json")); err != nil {
		t.Fatal(err)
	}
	b = composeSettings(home)
	if b.err == "" {
		t.Error("the file was written over")
	}
}

// The comma opens the settings from the processes view, and esc comes
// back to it.
func TestTheCommaOpensTheSettings(t *testing.T) {
	home := configured(t, `{"roots":["~/projects"]}`, "projects")
	m := model{p: plain, width: panelWidth, height: 40, view: viewProcesses}
	m.head.login.home = home
	next, _ := m.Update(tea.KeyPressMsg{Code: ',', Text: ","})
	m = next.(model)
	if m.view != viewSettings {
		t.Fatalf("the comma went to view %d", m.view)
	}
	if !strings.Contains(m.View().Content, "SETTINGS") {
		t.Errorf("the settings view does not say what it is:\n%s", m.View().Content)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if got := next.(model).view; got != viewProcesses {
		t.Errorf("esc from the settings went to view %d", got)
	}
}

// What conn writes is what conn reads: the file is a config file, and a
// root written by the view comes back off disk as the same root.
func TestWhatTheViewWritesIsWhatConnReads(t *testing.T) {
	home := configured(t, "", "projects")
	if err := saveRoots(home, []string{"~/projects"}); err != nil {
		t.Fatal(err)
	}
	if err := saveTheme(home, "datum"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(configPath(home))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		t.Fatalf("conn wrote a file it cannot read: %v", err)
	}
	if c := wrote(t, home); len(c.Roots) != 1 || c.Theme != "datum" {
		t.Errorf("the file came back as %+v", c)
	}
}
