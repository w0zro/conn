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

// settingsAt is the settings, in the pane they are worked in, with the
// cursor on a row.
func settingsAt(t *testing.T, home string, at int) settingsModel {
	t.Helper()
	return settingsModel{p: plain, width: bayWidth, height: 40, home: home, at: at}
}

// bayWidth is a workspace to draw the settings in: what is left of a
// wide window beside the panel.
const bayWidth = 120

// key is a key pressed in the settings, for a test that presses
// several.
func (m settingsModel) press(t *testing.T, k tea.KeyPressMsg) settingsModel {
	t.Helper()
	next, _ := m.Update(k)
	return next.(settingsModel)
}

// typing is a key of a letter, as a terminal sends one.
func typing(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

// The settings show the file, and the file as it is written: the roots
// it names, in its own words, with what each one turned out to be on
// this machine against the right.
func TestTheSettingsShowTheFile(t *testing.T) {
	home := configured(t, `{"roots":["~/projects","~/gone"]}`, "projects")
	b := composeSettings(home, "conn", true)
	if !b.present || b.err != "" {
		t.Fatalf("the file read as present %v, err %q", b.present, b.err)
	}
	if b.roots != 2 || len(b.rows) != 3+len(themes)+len(grounds) {
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
	b := composeSettings(home, "conn", true)
	if !b.forced {
		t.Fatal("the view does not know CONN_ROOTS is in force")
	}
	if len(b.rows) != 2+len(themes)+len(grounds) || b.rows[0].text != "~/projects" {
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
	m = m.press(t, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.asking || m.askingAt != -1 {
		t.Fatalf("enter on the add row: asking %v, at %d", m.asking, m.askingAt)
	}
	m.line.set(filepath.Join(home, "work"))
	m = m.press(t, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.asking {
		t.Error("a root saved left the line up")
	}
	c := wrote(t, home)
	if len(c.Roots) != 2 || c.Roots[0] != "~/projects" || c.Roots[1] != "~/work" {
		t.Errorf("the file names %q", c.Roots)
	}
	// And what conn had nothing to do with is still in it.
	if c.Theme != "datum" {
		t.Errorf("the theme in the file came out %q", c.Theme)
	}
	// conn walks it on its next reading rather than on the next start;
	// the settings tell the panel nothing, and the panel reads the file
	// every time it reads the table. See
	// TestAReadingTakesTheRootsAsTheFileNowNamesThem.
	if rows := m.report().rows; rows[1].text != "~/work" {
		t.Errorf("the rows do not show what was written: %+v", rows)
	}
}

// Enter on a root is that root, to be typed over: what is written goes
// back where it came from, and the other roots are left alone.
func TestARootIsTypedOverWhereItStands(t *testing.T) {
	home := configured(t, `{"roots":["~/gone","~/projects"]}`, "projects", "work")
	m := settingsAt(t, home, 0)
	m = m.press(t, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.line.text != "~/gone" || m.askingAt != 0 {
		t.Fatalf("enter on a root gave %q at %d", m.line.text, m.askingAt)
	}
	m.line.set(filepath.Join(home, "work"))
	m = m.press(t, tea.KeyPressMsg{Code: tea.KeyEnter})
	if c := wrote(t, home); len(c.Roots) != 2 || c.Roots[0] != "~/work" || c.Roots[1] != "~/projects" {
		t.Errorf("the file names %q", c.Roots)
	}
}

// esc leaves the typing without writing anything, and goes back to the
// rows. On the first start, which asks on the panel, there is nothing
// to go back to and esc does nothing: conn cannot show the processes
// view until it has been told where to look.
func TestEscLeavesARootAsItWas(t *testing.T) {
	home := configured(t, `{"roots":["~/projects"]}`, "projects")
	m := settingsAt(t, home, 0)
	m = m.press(t, tea.KeyPressMsg{Code: tea.KeyEnter})
	m.line.set("~/somewhere-else")
	m = m.press(t, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.asking {
		t.Fatal("esc from a root being typed left the line up")
	}
	if c := wrote(t, home); len(c.Roots) != 1 || c.Roots[0] != "~/projects" {
		t.Errorf("esc wrote something: %q", c.Roots)
	}
	first := model{p: plain, width: panelWidth, height: 40}
	first.head.login.home = home
	mm, _ := first.toRoots()
	first = mm.(model)
	next, _ := first.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if next.(model).view != viewRoots {
		t.Error("esc found a way out of the first start, which has none")
	}
}

// esc on the rows is the settings done with: the panel is told, so the
// workspace is filled in the same breath, and then this conn ends.
func TestEscLeavesTheSettings(t *testing.T) {
	home := configured(t, `{"roots":["~/projects"]}`, "projects")
	m := settingsAt(t, home, 0)
	if _, cmd := m.key("esc"); cmd == nil {
		t.Error("esc on the rows did nothing")
	}
	// From the line it is the rows it goes back to, not out.
	m = m.press(t, tea.KeyPressMsg{Code: tea.KeyEnter})
	next, cmd := m.key("esc")
	if cmd != nil || next.asking {
		t.Error("esc on the line left the settings rather than the line")
	}
}

// x takes a root out of the file, and the rest of the file stands.
func TestXTakesARootOut(t *testing.T) {
	home := configured(t, `{"roots":["~/projects","~/work"]}`, "projects", "work")
	m := settingsAt(t, home, 0)
	m = m.press(t, typing('x'))
	c := wrote(t, home)
	if len(c.Roots) != 1 || c.Roots[0] != "~/work" {
		t.Fatalf("the file names %q", c.Roots)
	}
	// The add row is all that is left once the last one goes.
	m = m.press(t, typing('x'))
	if c := wrote(t, home); len(c.Roots) != 0 {
		t.Errorf("the file still names %q", c.Roots)
	}
	if rows := m.report().rows; len(rows) != 1+len(themes)+len(grounds) || rows[0].kind != addRootSetting {
		t.Errorf("what is left is %+v", rows)
	}
}

// A file conn cannot read is not written over: conn cannot tell what is
// in it, so it cannot keep it, and the view says so rather than
// quietly making a new file out of half an answer.
func TestAFileThatWillNotParseIsNotWrittenOver(t *testing.T) {
	home := configured(t, `{"roots": [`, "projects")
	b := composeSettings(home, "conn", true)
	if b.err == "" {
		t.Fatal("the view says nothing about a file that will not parse")
	}
	m := settingsAt(t, home, 0)
	m = m.press(t, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.asking {
		t.Fatal("enter put a line up with nothing to edit")
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "conn", "config.json")); err != nil {
		t.Fatal(err)
	}
	b = composeSettings(home, "conn", true)
	if b.err == "" {
		t.Error("the file was written over")
	}
}

// The comma puts the settings in the workspace, and the panel says
// SETTINGS while they stand: the panel stays the panel, reading the
// machine beside them, and holds no row under the cursor, the keys
// being in the other pane.
func TestTheCommaOpensTheSettings(t *testing.T) {
	m := model{view: viewProcesses, cursor: 4321, inside: true, srv: &server{}}
	next, cmd := m.key(",")
	m = next.(model)
	if !m.setting || cmd == nil {
		t.Fatalf("the comma left setting %v, cmd %v", m.setting, cmd != nil)
	}
	if m.view != viewProcesses {
		t.Errorf("the comma took the panel off the processes view, to %d", m.view)
	}
	if m.cursor != 0 || m.settingCursor != 4321 {
		t.Errorf("the row under the cursor is %d, kept as %d", m.cursor, m.settingCursor)
	}
	if got := m.keys(); !strings.Contains(got, settingsWord) || strings.Contains(got, wordmarkLine) {
		t.Errorf("the band says %q", got)
	}
	if got := m.station(); !strings.Contains(got, settingsWord) {
		t.Errorf("with the keys in the settings the band says %q", got)
	}
	// And they are one at a time: a comma pressed again while they
	// stand is not a second pane of settings.
	if _, cmd := m.key(","); cmd != nil {
		t.Error("the comma opened the settings over the settings")
	}
	// Leaving puts the row back and the panel's own word with it.
	next, cmd = m.leftSettings(false)
	m = next.(model)
	if m.setting || m.cursor != 4321 {
		t.Errorf("leaving left setting %v, cursor %d", m.setting, m.cursor)
	}
	if cmd == nil {
		t.Error("the workspace was left holding the settings")
	}
	if got := m.keys(); !strings.Contains(got, wordmarkLine) {
		t.Errorf("with the settings gone the band says %q", got)
	}
}

// The settings say as they go, the way the manual does, and the panel
// answers the key wherever it is and whatever view it is in.
func TestTheSettingsSayWhenTheyAreDone(t *testing.T) {
	m := model{view: viewProjects, inside: true, srv: &server{}, setting: true, settingFrom: "%4",
		panes: map[string]pane{"ttys011": {id: "%4", tty: "ttys011"}}}
	next, cmd := m.key("alt+,") // what leaveSettingsKey arrives as
	if got := next.(model); got.setting || got.settingFrom != "" {
		t.Errorf("the settings are still up: setting %v, from %q", got.setting, got.settingFrom)
	}
	if cmd == nil {
		t.Error("nothing was done to put them away")
	}
	// The panel key is the other way out, and it does not put the keys
	// back where they came from: the key says where to go.
	next, _ = m.arrived("")
	if got := next.(model); got.setting || got.settingFrom != "" {
		t.Errorf("the panel key left setting %v, from %q", got.setting, got.settingFrom)
	}
}

// A reading that finds the settings in the workspace leaves the cursor
// let go, the way it does for the manual: there is no row the keys are
// about while they stand, and follow would hand one back on the beat.
func TestTheReadingLeavesTheCursorAloneWhileSetting(t *testing.T) {
	projects := []project{{path: "/w", entries: []entry{{pid: 11, tty: "ttys001"}, {pid: 22, tty: "ttys002"}}}}
	m := model{view: viewProcesses, inside: true, cursor: 0, projects: projects}
	up := processesMsg{projects: projects, gen: m.processesGen, baySetting: true}
	next, _ := m.Update(up)
	if got := next.(model); got.cursor != 0 || !got.setting {
		t.Errorf("the reading put the cursor back on %d (setting %v)", got.cursor, got.setting)
	}
	next, _ = m.Update(processesMsg{projects: projects, gen: m.processesGen})
	if got := next.(model); got.cursor == 0 || got.setting {
		t.Errorf("with no settings up the reading left the cursor at %d (setting %v)", got.cursor, got.setting)
	}
}

// The bar says the keys that work on the row under the cursor, and the
// settings write it themselves: the cursor is in their pane, and the
// panel cannot know what it is on. The panel leaves that position
// alone while they stand.
func TestTheSettingsSayWhatTheirKeysDo(t *testing.T) {
	home := configured(t, `{"roots":["~/projects"]}`, "projects")
	m := settingsAt(t, home, 0)
	rows := m.report().rows
	if got := keyBar(settingsHints(rows, 0)); !strings.Contains(got, "take it out") {
		t.Errorf("on a root the bar says %q", got)
	}
	if got := keyBar(settingsHints(rows, 1)); !strings.Contains(got, "add one") {
		t.Errorf("on the add row the bar says %q", got)
	}
	// The theme conn is wearing takes no enter, so the bar offers none.
	at := rowFor(t, m, themeSetting, "")
	for i, r := range rows {
		if r.kind == themeSetting && r.note == noteInUse {
			at = i
		}
	}
	if got := keyBar(settingsHints(rows, at)); strings.Contains(got, "wear it") {
		t.Errorf("the theme already worn offers %q", got)
	}

	// The panel writes the band and leaves the bar to them.
	p := model{view: viewProcesses, inside: true, srv: &server{}, setting: true}
	p, _ = p.saying()
	if p.saidBar != "" {
		t.Errorf("the panel wrote the bar while the settings had the keys: %q", p.saidBar)
	}
	p.setting = false
	p, _ = p.saying()
	if p.saidBar == "" {
		t.Error("with the settings gone the panel does not write the bar again")
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

// A theme picked in the settings is written down and worn at once: the
// file names it for the next start, and conn is drawing in it before
// the key is answered. Outside the server there is nothing to dress but
// conn itself.
func TestAThemePickedIsWrittenAndWorn(t *testing.T) {
	holdMode(t)
	applyMode(mode{theme: defaultTheme, dark: true})
	home := configured(t, `{"roots":["~/projects"]}`, "projects")
	m := settingsAt(t, home, 0)
	rows := m.report().rows
	at := -1
	for i, r := range rows {
		if r.kind == themeSetting && r.text != current.theme {
			at = i
			break
		}
	}
	if at < 0 {
		t.Skip("conn has one theme")
	}
	want := rows[at].text
	m.at = at
	m = m.press(t, tea.KeyPressMsg{Code: tea.KeyEnter})
	if current.theme != want {
		t.Errorf("conn is wearing %q, not %q", current.theme, want)
	}
	if current.dark != true {
		t.Error("picking a theme changed the ground under it")
	}
	if c := wrote(t, home); c.Theme != want {
		t.Errorf("the file names the theme %q", c.Theme)
	}
	// The roots the file already named are still in it: the theme is
	// one key, and conn writes the key it came to change.
	if c := wrote(t, home); len(c.Roots) != 1 {
		t.Errorf("writing the theme took the roots with it: %q", c.Roots)
	}
	// And the view says so, the row wearing the note rather than the
	// one the file used to name.
	for _, r := range m.report().rows {
		if r.kind == themeSetting && r.text == want && r.note != noteInUse {
			t.Errorf("the theme worn is noted %q", r.note)
		}
	}
}

// The panel is told the mode changed and wears it where it stands. A
// fresh conn in that pane would be the console, and the operator
// picked a theme rather than asking to start again.
func TestThePanelWearsTheModeTheSettingsWrote(t *testing.T) {
	holdMode(t)
	applyMode(mode{theme: defaultTheme, dark: true})
	home := t.TempDir()
	socket := filepath.Join(home, "conn.sock")
	if err := writeMode(socket, mode{theme: "datum", dark: false}); err != nil {
		t.Fatal(err)
	}
	m := model{view: viewProcesses, inside: true, srv: &server{socket: socket}}
	m.head.login.home = home
	next, _ := m.key("alt+w") // what wearModeKey arrives as
	if current.theme != "datum" || current.dark {
		t.Errorf("the panel is wearing %+v", current)
	}
	if next.(model).p.ink == "" {
		t.Error("the panel did not take the new palette")
	}
}

// gg and G are the ends of the list here as they are in the processes
// view: the settings are rows and the keys that move among rows are the
// same keys everywhere.
func TestTheSettingsMoveToBothEnds(t *testing.T) {
	home := configured(t, `{"roots":["~/projects","~/work"]}`, "projects", "work")
	m := settingsAt(t, home, 0)
	last := len(m.report().rows) - 1
	m = m.press(t, typing('G'))
	if m.at != last {
		t.Errorf("G went to row %d of %d", m.at, last)
	}
	m = m.press(t, typing('g')).press(t, typing('g'))
	if m.at != 0 {
		t.Errorf("gg went to row %d", m.at)
	}
	// j from the last row comes round to the first, as it does in the
	// processes view.
	m.at = last
	if got := m.press(t, typing('j')).at; got != 0 {
		t.Errorf("j past the last row went to %d", got)
	}
}

// A ground picked in the settings is written down and worn at once,
// like a theme, and the theme is left where it was: the two are
// separate axes and picking one is not an answer about the other.
func TestAGroundPickedIsWrittenAndWorn(t *testing.T) {
	holdMode(t)
	applyMode(mode{theme: "datum", dark: true})
	home := configured(t, `{"roots":["~/projects"],"theme":"datum"}`, "projects")
	m := settingsAt(t, home, 0)
	m.at = rowFor(t, m, groundSetting, lightGround)
	m = m.press(t, tea.KeyPressMsg{Code: tea.KeyEnter})
	if current.dark {
		t.Error("conn is still on dark")
	}
	if current.theme != "datum" {
		t.Errorf("picking a ground put conn in the theme %q", current.theme)
	}
	c := wrote(t, home)
	if c.Ground != lightGround || c.Theme != "datum" || len(c.Roots) != 1 {
		t.Errorf("the file came out %+v", c)
	}
	// And a fresh server would come up on it without asking the
	// terminal anything.
	if askMode(override{}, home).dark {
		t.Error("a fresh server would not come up on the ground in the file")
	}
	for _, r := range m.report().rows {
		if r.kind == groundSetting && r.value == lightGround && r.note != noteInUse {
			t.Errorf("the ground worn is noted %q", r.note)
		}
	}
}

// The row that asks the terminal takes the key out of the file and
// changes nothing where conn stands: OSC 11 is a question for a server
// rising, and a conn in a pane of its own server would be asking tmux,
// which answers with the ground conn itself set.
func TestAskingTheTerminalTakesTheGroundOutOfTheFile(t *testing.T) {
	holdMode(t)
	applyMode(mode{theme: defaultTheme, dark: false})
	home := configured(t, `{"roots":["~/projects"],"ground":"light"}`, "projects")
	m := settingsAt(t, home, 0)
	m.at = rowFor(t, m, groundSetting, "")
	m = m.press(t, tea.KeyPressMsg{Code: tea.KeyEnter})
	if current.dark {
		t.Error("the ground changed under a server already up")
	}
	if c := wrote(t, home); c.Ground != "" || len(c.Roots) != 1 {
		t.Errorf("the file came out %+v", c)
	}
	// The key is gone rather than written empty: conn reads the file,
	// and a key that is there says somebody decided.
	b, err := os.ReadFile(configPath(home))
	if err != nil || strings.Contains(string(b), "ground") {
		t.Errorf("the file still names a ground: %s (%v)", b, err)
	}
	for _, r := range m.report().rows {
		if r.kind == groundSetting && r.value == "" && r.note != noteInFile {
			t.Errorf("the row that asks the terminal is noted %q", r.note)
		}
	}
}

// A ground the file names that is neither is said under the heading it
// belongs to, and a theme conn does not have the same way: the console
// says so in a word, and this is the view somebody came to to put it
// right.
func TestTheSettingsSayWhatTheFileNamesAndConnHasNot(t *testing.T) {
	home := configured(t, `{"roots":["~/projects"],"theme":"solarized","ground":"grey"}`, "projects")
	b := composeSettings(home, "conn", true)
	if b.unknownTheme != "solarized" || b.unknownGround != "grey" {
		t.Fatalf("the view read them as %q and %q", b.unknownTheme, b.unknownGround)
	}
	var text strings.Builder
	for _, r := range drawSettings(b, 0, panelWidth, 0, plain) {
		text.WriteString(r.text + "\n")
	}
	for _, want := range []string{"CONN HAS NO THEME NAMED SOLARIZED", "GREY IS NEITHER GROUND"} {
		if !strings.Contains(text.String(), want) {
			t.Errorf("the view does not say %q:\n%s", want, text.String())
		}
	}
}

// The settings are drawn in the workspace and the roots are typed
// there too: the line is the same asking view the first start uses,
// with a way back to the rows that the first start has not got.
func TestTheSettingsDrawTheRootsLineInThePane(t *testing.T) {
	home := configured(t, `{"roots":["~/projects"]}`, "projects")
	m := settingsAt(t, home, 0)
	if got := m.View().Content; !strings.Contains(got, "SETTINGS") {
		t.Errorf("the pane does not say what it is:\n%s", got)
	}
	m = m.press(t, tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := m.View().Content; !strings.Contains(got, "~/projects") {
		t.Errorf("the line is not the root being typed over:\n%s", got)
	}
	if !m.rootsReport().editing {
		t.Error("the line does not know it is changing a root rather than asking for the first")
	}
}

// rowFor is the row of a setting with a given value, for a test that
// moves the cursor to it.
func rowFor(t *testing.T, m settingsModel, kind settingKind, value string) int {
	t.Helper()
	for i, r := range m.report().rows {
		if r.kind == kind && r.value == value {
			return i
		}
	}
	t.Fatalf("no row of kind %d with value %q", kind, value)
	return 0
}
