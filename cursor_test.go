package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// The cursor travels as a few bytes beside the socket, and a reader
// that finds nothing there — no file, a file of rubbish — reads no
// cursor rather than a wrong one.
func TestTheCursorTravelsAsAPid(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONN_SOCKET", filepath.Join(dir, "tmux.sock"))
	path := cursorPath("/nowhere")

	if got, _, _ := askCursor(path); got != 0 {
		t.Errorf("with nothing published the cursor reads %d", got)
	}
	tellCursor(path, 49212, nil, nil)
	if got, _, _ := askCursor(path); got != 49212 {
		t.Errorf("the cursor reads %d, not what was published", got)
	}
	tellCursor(path, 3, nil, nil)
	if got, _, _ := askCursor(path); got != 3 {
		t.Errorf("the cursor reads %d after moving", got)
	}
	if err := os.WriteFile(path, []byte("not a pid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := askCursor(path); got != 0 {
		t.Errorf("rubbish reads as pid %d rather than as no cursor", got)
	}
}

// It is named after the socket, so two servers on one machine do not
// move each other's cursor.
func TestEachServerHasItsOwnCursor(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONN_SOCKET", filepath.Join(dir, "one.sock"))
	one := cursorPath("/nowhere")
	t.Setenv("CONN_SOCKET", filepath.Join(dir, "two.sock"))
	two := cursorPath("/nowhere")
	if one == two {
		t.Fatalf("both servers publish to %s", one)
	}
	tellCursor(one, 11, nil, nil)
	tellCursor(two, 22, nil, nil)
	one11, _, _ := askCursor(one)
	two22, _, _ := askCursor(two)
	if one11 != 11 || two22 != 22 {
		t.Errorf("the two cursors are %d and %d", one11, two22)
	}
}

// The panel publishes wherever the cursor ends up, by whatever moved it
// — j and k, tab, a reading that carried it along — because every path
// goes out through the one place that tells it. Off the processes
// view there is no cursor on a process, and the subject is not unchosen
// by going to the list to open something, so nothing is said rather
// than a nothing.
func TestThePanelPublishesItsCursor(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONN_SOCKET", filepath.Join(dir, "tmux.sock"))
	path := cursorPath("/nowhere")

	m := newModel(plain)
	m.view, m.inside, m.now = viewProcesses, true, processesNow
	m.head.login.home = dir
	m.projects = []project{{path: "/w", entries: []entry{
		{pid: 11, tty: "ttys001", status: statusIdle},
		{pid: 22, tty: "ttys002", status: statusWaiting, since: processesNow.Add(-time.Minute)},
		{pid: 33, tty: "ttys003", status: statusIdle},
	}}}
	m.cursor, m.cursorAt = 11, 0

	press := func(k string) {
		next, _ := m.Update(tea.KeyPressMsg(tea.Key{Text: k}))
		m = next.(model)
	}
	published := func() int { pid, _, _ := askCursor(path); return pid }
	press("j")
	if m.cursor != 22 || published() != 22 {
		t.Errorf("after j the cursor is %d and %d was published", m.cursor, published())
	}
	press("k")
	if m.cursor != 11 || published() != 11 {
		t.Errorf("after k the cursor is %d and %d was published", m.cursor, published())
	}
	// tab moves it too, and says so by the same road.
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = next.(model)
	if m.cursor != 22 || published() != 22 {
		t.Errorf("after tab the cursor is %d and %d was published", m.cursor, published())
	}
	// A reading says it again whether or not it moved, so a file gone
	// missing comes back on the next beat rather than staying gone
	// until somebody presses a key.
	tellCursor(path, 0, nil, nil)
	next, _ = m.Update(processesMsg{gen: m.processesGen, projects: []project{{path: "/w", entries: []entry{
		{pid: 22, tty: "ttys002", status: statusIdle},
	}}}})
	m = next.(model)
	if published() != 22 {
		t.Errorf("a reading published %d, not where the cursor stands", published())
	}

	// With no home there is nowhere to publish, and conn does not write
	// beside whatever directory it was started in.
	nowhere := m
	nowhere.head.login.home, nowhere.told = "", -1
	tellCursor(path, 55, nil, nil)
	nowhere.Update(tea.KeyPressMsg(tea.Key{Text: "j"}))
	if got, _, _ := askCursor(path); got != 55 {
		t.Errorf("a panel with no home published %d", got)
	}

	// On the list there is no process under the cursor; the readout keeps
	// the subject it was given rather than being told a nothing.
	tellCursor(path, 99, nil, nil)
	m.view = viewProjects
	press("j")
	if got, _, _ := askCursor(path); got != 99 {
		t.Errorf("the list published %d over the processes view's cursor", got)
	}
}

// A reading of a subject the cursor has since left is no longer about
// anything: putting it up would be the page flicking back to a row
// nobody is looking at.
func TestTheReadoutDropsAReadingItHasMovedPast(t *testing.T) {
	m := readoutModel{pid: 22, follow: true, p: plain}
	stale := readoutReport{pid: 11, groups: []readoutGroup{{title: "STALE"}}}
	fresh := readoutReport{pid: 22, groups: []readoutGroup{{title: "FRESH"}}}

	next, _ := m.Update(readoutReadMsg{pid: 11, report: stale})
	if got := next.(readoutModel).report; len(got.groups) != 0 {
		t.Errorf("a reading of pid 11 landed on a page about 22: %+v", got)
	}
	next, _ = m.Update(readoutReadMsg{pid: 22, report: fresh})
	if got := next.(readoutModel).report; len(got.groups) != 1 || got.groups[0].title != "FRESH" {
		t.Errorf("the reading of the subject did not land: %+v", got)
	}
}

// Following is how the panel opens it: with no pid the page is about whatever the
// cursor is on, and a cursor that moves moves the page. A pid given by
// hand pins it, and the cursor does not move it.
func TestTheReadoutFollowsTheCursorUnlessPinned(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONN_SOCKET", filepath.Join(dir, "tmux.sock"))
	path := cursorPath("/nowhere")
	tellCursor(path, 77, nil, nil)

	m := readoutModel{pid: 11, follow: true, cursor: path, p: plain, read: time.Now()}
	next, cmd := m.Update(readoutTickMsg{})
	m = next.(readoutModel)
	if m.pid != 77 {
		t.Errorf("the page is on pid %d, not where the cursor went", m.pid)
	}
	if cmd == nil {
		t.Error("a subject that moved was not read again")
	}

	pinned := readoutModel{pid: 11, follow: false, cursor: path, p: plain, read: time.Now()}
	next, _ = pinned.Update(readoutTickMsg{})
	if got := next.(readoutModel).pid; got != 11 {
		t.Errorf("a pinned page moved to pid %d", got)
	}

	// A cursor that has not moved is not read again on every poll — only
	// on the beat — or the table and git would be read three times a
	// second for a page nobody is moving.
	steady := readoutModel{pid: 77, follow: true, cursor: path, p: plain, read: time.Now()}
	before := steady.read
	next, _ = steady.Update(readoutTickMsg{})
	if got := next.(readoutModel); !got.read.Equal(before) {
		t.Error("a page whose cursor did not move read its subject again anyway")
	}
	// Once the beat has passed it reads regardless, so a page nobody is
	// moving still keeps up with its row.
	stale := readoutModel{pid: 77, follow: true, cursor: path, p: plain, read: time.Now().Add(-2 * readoutBeat)}
	next, _ = stale.Update(readoutTickMsg{})
	if got := next.(readoutModel); got.read.Equal(stale.read) {
		t.Error("a page past its beat did not read its subject again")
	}
}

// The machine takes a tenth of a second to read and the cursor moves
// faster than that, so the page answers the new row out of the table it
// already holds — every row is in it — and the reading that follows
// only says it again, newer.
func TestTheReadoutAnswersFromTheTableAlreadyRead(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONN_SOCKET", filepath.Join(dir, "tmux.sock"))
	path := cursorPath("/nowhere")

	held := readoutTable{
		projects: []project{{path: "/w", entries: []entry{
			{pid: 11, kind: kindShell, command: "zsh", tty: "ttys001"},
			{pid: 22, kind: kindRun, command: "go test ./...", tty: "ttys002"},
		}}},
		git: map[string]gitStatus{"/w": {repo: true, branch: "main"}},
	}
	m := readoutModel{pid: 11, follow: true, cursor: path, p: plain, table: held,
		report: readoutReport{pid: 11}, read: time.Now()}

	tellCursor(path, 22, nil, nil)
	next, _ := m.Update(readoutTickMsg{})
	m = next.(readoutModel)
	if m.report.pid != 22 {
		t.Errorf("the page is still about pid %d after the cursor moved", m.report.pid)
	}
	if text := texts(drawReadout(m.report, 120, 40, plain)); !strings.Contains(text, "go test ./...") {
		t.Errorf("the row the cursor landed on was not said out of the table already read:\n%s", text)
	}

	// A row the table has never seen is a row that started since it was
	// read, not a row that has gone: the page waits for the reading on
	// its way rather than putting up a gravestone.
	tellCursor(path, 33, nil, nil)
	next, _ = m.Update(readoutTickMsg{})
	after := next.(readoutModel)
	if after.pid != 33 {
		t.Errorf("the page did not follow the cursor to pid %d", after.pid)
	}
	if after.report.gone || after.report.pid != 22 {
		t.Errorf("a row the table has not got put up %+v rather than holding the page", after.report)
	}
}

// The table a reading was made from is about the machine, not about the
// row it was read for, so a reading the cursor has moved past is dropped
// while the table it came with is kept: the row the cursor went to is in
// it too.
func TestTheReadoutKeepsTheTableOfAReadingItDrops(t *testing.T) {
	m := readoutModel{pid: 22, follow: true, p: plain}
	table := readoutTable{projects: []project{{path: "/w", entries: []entry{{pid: 11, kind: kindShell}}}}}

	next, _ := m.Update(readoutReadMsg{pid: 11, report: readoutReport{pid: 11}, table: table})
	if got := next.(readoutModel).table.projects; len(got) != 1 {
		t.Errorf("the table of a dropped reading was dropped with it: %+v", got)
	}
}

// The row goes with the pid. Whether a row is working is read off the
// processor time between two readings, and what a contact is doing off
// its transcript, and a page that worked those out again for itself
// worked them out from a different pair of readings — and said ACTIVE
// of a row the panel beside it said was WORKING. So the panel says the
// row as it shows it, and the page says that row: the same word on
// both sides of the border.
func TestThePageSaysTheRowAsThePanelSaysIt(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONN_SOCKET", filepath.Join(dir, "tmux.sock"))
	path := cursorPath("/nowhere")

	// The panel's reading of the row: working, on a tool, for a while.
	shown := entry{pid: 22, kind: kindContact, command: "claude", typed: "claude", tty: "ttys002",
		status: statusWorking, doing: "edit tui.go", since: time.Now().Add(-40 * time.Second), cwd: "/w"}
	tellCursor(path, 22, &shown, nil)
	pid, row, _ := askCursor(path)
	if pid != 22 || row == nil || row.status != statusWorking || row.doing != "edit tui.go" {
		t.Fatalf("the row did not travel: pid %d, row %+v", pid, row)
	}

	// The page's own table has the same row, read as merely alive.
	held := readoutTable{projects: []project{{path: "/w", entries: []entry{
		{pid: 11, kind: kindShell, command: "zsh", tty: "ttys001", status: statusActive, depth: 0},
		{pid: 22, kind: kindContact, command: "claude", typed: "claude", tty: "ttys002", status: statusActive, depth: 1, cwd: "/w"},
	}}}}
	m := readoutModel{pid: 11, follow: true, cursor: path, p: plain, table: held,
		report: readoutReport{pid: 11}, read: time.Now()}
	next, _ := m.Update(readoutTickMsg{})
	m = next.(readoutModel)
	text := texts(drawReadout(m.report, 120, 40, plain))
	if !strings.Contains(text, "WORKING · FOR") {
		t.Errorf("the page says the row its own way, not the panel's:\n%s", text)
	}
	// What stands around it is still the table's: the shell that runs it.
	if !strings.Contains(text, "SHELL zsh · 11") {
		t.Errorf("the page lost the tree around the row:\n%s", text)
	}

	// A row that stays put and changes its word is said again: the note
	// changed, so the page reads it, and nothing is read off the machine
	// for a cursor that did not move.
	shown.status, shown.doing = statusIdle, ""
	tellCursor(path, 22, &shown, nil)
	before := m.read
	next, _ = m.Update(readoutTickMsg{})
	m = next.(readoutModel)
	if text := texts(drawReadout(m.report, 120, 40, plain)); !strings.Contains(text, "IDLE") {
		t.Errorf("the page did not follow the row's word:\n%s", text)
	}
	if !m.read.Equal(before) {
		t.Error("a row that changed its word without moving read the machine again")
	}
}
