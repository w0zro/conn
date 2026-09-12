package main

import (
	"os"
	"path/filepath"
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

	if got := askCursor(path); got != 0 {
		t.Errorf("with nothing published the cursor reads %d", got)
	}
	tellCursor(path, 49212)
	if got := askCursor(path); got != 49212 {
		t.Errorf("the cursor reads %d, not what was published", got)
	}
	tellCursor(path, 3)
	if got := askCursor(path); got != 3 {
		t.Errorf("the cursor reads %d after moving", got)
	}
	if err := os.WriteFile(path, []byte("not a pid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := askCursor(path); got != 0 {
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
	tellCursor(one, 11)
	tellCursor(two, 22)
	if askCursor(one) != 11 || askCursor(two) != 22 {
		t.Errorf("the two cursors are %d and %d", askCursor(one), askCursor(two))
	}
}

// The rail publishes wherever the cursor ends up, by whatever moved it
// — j and k, tab, a reading that carried it along — because every path
// goes out through the one place that tells it. Off the watch there is
// no cursor on a process, and the subject is not unchosen by going to
// the list to open something, so nothing is said rather than a nothing.
func TestTheRailPublishesItsCursor(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONN_SOCKET", filepath.Join(dir, "tmux.sock"))
	path := cursorPath("/nowhere")

	m := newModel(plain)
	m.view, m.inside, m.now = viewWatch, true, watchNow
	m.head.session.home = dir
	m.places = []place{{path: "/w", entries: []entry{
		{pid: 11, tty: "ttys001", status: statusIdle},
		{pid: 22, tty: "ttys002", status: statusWaiting, since: watchNow.Add(-time.Minute)},
		{pid: 33, tty: "ttys003", status: statusIdle},
	}}}
	m.cursor, m.cursorAt = 11, 0

	press := func(k string) {
		next, _ := m.Update(tea.KeyPressMsg(tea.Key{Text: k}))
		m = next.(model)
	}
	press("j")
	if m.cursor != 22 || askCursor(path) != 22 {
		t.Errorf("after j the cursor is %d and %d was published", m.cursor, askCursor(path))
	}
	press("k")
	if m.cursor != 11 || askCursor(path) != 11 {
		t.Errorf("after k the cursor is %d and %d was published", m.cursor, askCursor(path))
	}
	// tab moves it too, and says so by the same road.
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = next.(model)
	if m.cursor != 22 || askCursor(path) != 22 {
		t.Errorf("after tab the cursor is %d and %d was published", m.cursor, askCursor(path))
	}
	// A reading says it again whether or not it moved, so a file gone
	// missing comes back on the next beat rather than staying gone
	// until somebody presses a key.
	tellCursor(path, 0)
	next, _ = m.Update(watchMsg{gen: m.watchGen, places: []place{{path: "/w", entries: []entry{
		{pid: 22, tty: "ttys002", status: statusIdle},
	}}}})
	m = next.(model)
	if askCursor(path) != 22 {
		t.Errorf("a reading published %d, not where the cursor stands", askCursor(path))
	}

	// With no home there is nowhere to publish, and conn does not write
	// beside whatever directory it was started in.
	nowhere := m
	nowhere.head.session.home, nowhere.told = "", -1
	tellCursor(path, 55)
	next, _ = nowhere.Update(tea.KeyPressMsg(tea.Key{Text: "j"}))
	if got := askCursor(path); got != 55 {
		t.Errorf("a rail with no home published %d", got)
	}

	// On the list there is no process under the cursor; the look keeps
	// the subject it was given rather than being told a nothing.
	tellCursor(path, 99)
	m.view = viewProjects
	press("j")
	if got := askCursor(path); got != 99 {
		t.Errorf("the list published %d over the watch's cursor", got)
	}
}

// A reading of a subject the cursor has since left is no longer about
// anything: putting it up would be the page flicking back to a row
// nobody is looking at.
func TestTheLookDropsAReadingItHasMovedPast(t *testing.T) {
	m := lookModel{pid: 22, follow: true, p: plain}
	stale := lookReport{pid: 11, groups: []lookGroup{{title: "STALE"}}}
	fresh := lookReport{pid: 22, groups: []lookGroup{{title: "FRESH"}}}

	next, _ := m.Update(lookReadMsg{pid: 11, report: stale})
	if got := next.(lookModel).report; len(got.groups) != 0 {
		t.Errorf("a reading of pid 11 landed on a page about 22: %+v", got)
	}
	next, _ = m.Update(lookReadMsg{pid: 22, report: fresh})
	if got := next.(lookModel).report; len(got.groups) != 1 || got.groups[0].title != "FRESH" {
		t.Errorf("the reading of the subject did not land: %+v", got)
	}
}

// Following is what i opens: with no pid the page is about whatever the
// cursor is on, and a cursor that moves moves the page. A pid given by
// hand pins it, and the cursor does not move it.
func TestTheLookFollowsTheCursorUnlessPinned(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONN_SOCKET", filepath.Join(dir, "tmux.sock"))
	path := cursorPath("/nowhere")
	tellCursor(path, 77)

	m := lookModel{pid: 11, follow: true, cursor: path, p: plain, read: time.Now()}
	next, cmd := m.Update(lookTickMsg{})
	m = next.(lookModel)
	if m.pid != 77 {
		t.Errorf("the page is on pid %d, not where the cursor went", m.pid)
	}
	if cmd == nil {
		t.Error("a subject that moved was not read again")
	}

	pinned := lookModel{pid: 11, follow: false, cursor: path, p: plain, read: time.Now()}
	next, _ = pinned.Update(lookTickMsg{})
	if got := next.(lookModel).pid; got != 11 {
		t.Errorf("a pinned page moved to pid %d", got)
	}

	// A cursor that has not moved is not read again on every poll — only
	// on the beat — or the table and git would be read three times a
	// second for a page nobody is moving.
	steady := lookModel{pid: 77, follow: true, cursor: path, p: plain, read: time.Now()}
	before := steady.read
	next, _ = steady.Update(lookTickMsg{})
	if got := next.(lookModel); !got.read.Equal(before) {
		t.Error("a page whose cursor did not move read its subject again anyway")
	}
	// Once the beat has passed it reads regardless, so a page nobody is
	// moving still keeps up with its row.
	stale := lookModel{pid: 77, follow: true, cursor: path, p: plain, read: time.Now().Add(-2 * lookBeat)}
	next, _ = stale.Update(lookTickMsg{})
	if got := next.(lookModel); got.read.Equal(stale.read) {
		t.Error("a page past its beat did not read its subject again")
	}
}
