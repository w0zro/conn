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

	if got, _ := askCursor(path); got.pid != 0 {
		t.Errorf("with nothing published the cursor reads %d", got.pid)
	}
	tellCursor(path, subject{pid: 49212}, nil)
	if got, _ := askCursor(path); got.pid != 49212 {
		t.Errorf("the cursor reads %d, not what was published", got.pid)
	}
	tellCursor(path, subject{pid: 3}, nil)
	if got, _ := askCursor(path); got.pid != 3 {
		t.Errorf("the cursor reads %d after moving", got.pid)
	}
	if err := os.WriteFile(path, []byte("not a pid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, _ := askCursor(path); got.pid != 0 {
		t.Errorf("rubbish reads as pid %d rather than as no cursor", got.pid)
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
	tellCursor(one, subject{pid: 11}, nil)
	tellCursor(two, subject{pid: 22}, nil)
	one11, _ := askCursor(one)
	two22, _ := askCursor(two)
	if one11.pid != 11 || two22.pid != 22 {
		t.Errorf("the two cursors are %d and %d", one11.pid, two22.pid)
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
	published := func() int { at, _ := askCursor(path); return at.pid }
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
	tellCursor(path, subject{pid: 0}, nil)
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
	nowhere.head.login.home, nowhere.told = "", subject{}
	tellCursor(path, subject{pid: 55}, nil)
	nowhere.Update(tea.KeyPressMsg(tea.Key{Text: "j"}))
	if got, _ := askCursor(path); got.pid != 55 {
		t.Errorf("a panel with no home published %d", got.pid)
	}

	// On a list with no row under the cursor there is no subject; the
	// readout keeps the one it was given rather than being told a
	// nothing.
	tellCursor(path, subject{pid: 99}, nil)
	m.view = viewProjects
	press("j")
	if got, _ := askCursor(path); got.pid != 99 {
		t.Errorf("the list published %d over the processes view's cursor", got.pid)
	}
}

// A reading of a subject the cursor has since left is no longer about
// anything: putting it up would be the page flicking back to a row
// nobody is looking at.
func TestTheReadoutDropsAReadingItHasMovedPast(t *testing.T) {
	m := readoutModel{at: subject{pid: 22}, follow: true, p: plain}
	stale := readoutReport{pid: 11, groups: []readoutGroup{{title: "STALE"}}}
	fresh := readoutReport{pid: 22, groups: []readoutGroup{{title: "FRESH"}}}

	next, _ := m.Update(readoutReadMsg{at: subject{pid: 11}, report: stale, ok: true})
	if got := next.(readoutModel).report; len(got.groups) != 0 {
		t.Errorf("a reading of pid 11 landed on a page about 22: %+v", got)
	}
	next, _ = m.Update(readoutReadMsg{at: subject{pid: 22}, report: fresh, ok: true})
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
	tellCursor(path, subject{pid: 77}, nil)

	m := readoutModel{at: subject{pid: 11}, follow: true, cursor: path, p: plain, read: time.Now()}
	next, cmd := m.Update(readoutTickMsg{})
	m = next.(readoutModel)
	if m.at.pid != 77 {
		t.Errorf("the page is on pid %d, not where the cursor went", m.at.pid)
	}
	if cmd == nil {
		t.Error("a subject that moved was not read again")
	}

	pinned := readoutModel{at: subject{pid: 11}, follow: false, cursor: path, p: plain, read: time.Now()}
	next, _ = pinned.Update(readoutTickMsg{})
	if got := next.(readoutModel).at.pid; got != 11 {
		t.Errorf("a pinned page moved to pid %d", got)
	}

	// A cursor that has not moved is not read again on every poll — only
	// on the beat — or the table and git would be read three times a
	// second for a page nobody is moving.
	steady := readoutModel{at: subject{pid: 77}, follow: true, cursor: path, p: plain, read: time.Now()}
	before := steady.read
	next, _ = steady.Update(readoutTickMsg{})
	if got := next.(readoutModel); !got.read.Equal(before) {
		t.Error("a page whose cursor did not move read its subject again anyway")
	}
	// Once the beat has passed it reads regardless, so a page nobody is
	// moving still keeps up with its row.
	stale := readoutModel{at: subject{pid: 77}, follow: true, cursor: path, p: plain, read: time.Now().Add(-2 * readoutBeat)}
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
		reading: reading{projects: []project{{path: "/w", entries: []entry{
			{pid: 11, kind: kindShell, command: "zsh", tty: "ttys001"},
			{pid: 22, kind: kindRun, command: "go test ./...", tty: "ttys002"},
		}}}},
		git: map[string]gitStatus{"/w": {repo: true, branch: "main"}},
	}
	m := readoutModel{at: subject{pid: 11}, follow: true, cursor: path, p: plain, table: held,
		report: readoutReport{pid: 11}, read: time.Now()}

	tellCursor(path, subject{pid: 22}, nil)
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
	tellCursor(path, subject{pid: 33}, nil)
	next, _ = m.Update(readoutTickMsg{})
	after := next.(readoutModel)
	if after.at.pid != 33 {
		t.Errorf("the page did not follow the cursor to pid %d", after.at.pid)
	}
	if after.report.gone || after.report.pid != 22 {
		t.Errorf("a row the table has not got put up %+v rather than holding the page", after.report)
	}
}

// What an asking was told is about a project or a session, not about
// the row it was asked for, so an asking the cursor has moved past is
// dropped while what it was told is kept: the row the cursor went to
// is in the same project as often as not, and git is a process.
func TestTheReadoutKeepsWhatADroppedAskingWasTold(t *testing.T) {
	m := readoutModel{at: subject{pid: 22}, follow: true, p: plain}
	table := readoutTable{git: map[string]gitStatus{"/w": {repo: true, branch: "main"}}}

	next, _ := m.Update(readoutReadMsg{at: subject{pid: 11}, report: readoutReport{pid: 11}, table: table, ok: true})
	if got := next.(readoutModel).table.git["/w"]; !got.repo {
		t.Errorf("what a dropped asking was told was dropped with it: %+v", got)
	}
}

// The reading goes with the pid. Whether a row is working is read off
// the processor time between two readings, and what a contact is doing
// off its transcript, and a page that worked those out again for itself
// worked them out from a different pair of readings — and said ACTIVE
// of a row the panel beside it said was WORKING. So the panel publishes
// its reading whole, and the page says that row: the same word on both
// sides of the border, and the page asks the machine nothing.
func TestThePageSaysTheRowAsThePanelSaysIt(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONN_SOCKET", filepath.Join(dir, "tmux.sock"))
	path := cursorPath("/nowhere")

	// The panel's reading: a shell running a contact that is working,
	// on a tool, for a while; the record behind each; the panes it
	// holds; and a container docker said, on its own row.
	shown := entry{pid: 22, kind: kindContact, command: "claude", typed: "claude", tty: "ttys002",
		status: statusWorking, doing: "edit tui.go", since: time.Now().Add(-40 * time.Second), cwd: "/w", depth: 1}
	r := reading{
		projects: []project{{path: "/w", entries: []entry{
			{pid: 11, kind: kindShell, command: "zsh", tty: "ttys001", status: statusActive},
			shown,
			{pid: -99, kind: kindService, command: "web", ports: []string{"8438"}, tty: "", status: statusActive, cwd: "/w", container: "abc123def456"},
		}}},
		records:    map[int]record{11: {pid: 11, state: 'S', foreground: false}, 22: {pid: 22, state: 'S', foreground: true, cpu: 90 * time.Second}},
		panes:      map[string]pane{"ttys002": {id: "%3", tty: "ttys002"}},
		inside:     true,
		containers: []container{{id: "abc123def456", service: "web", image: "nginx", state: "running", dir: "/w"}},
	}
	tellCursor(path, subject{pid: 22}, &r)
	at, got := askCursor(path)
	if at.pid != 22 || got == nil || len(got.projects) != 1 || got.records[22].cpu != 90*time.Second ||
		got.panes["ttys002"].id != "%3" || !got.inside || len(got.containers) != 1 {
		t.Fatalf("the reading did not travel whole: %+v, %+v", at, got)
	}
	if row := got.projects[0].entries[1]; row.status != statusWorking || row.doing != "edit tui.go" {
		t.Fatalf("the row did not travel: %+v", row)
	}

	// The page, with nothing of its own yet, says the row as the panel
	// says it, with what stands around it and what the record adds: the
	// contact's sheet, since the cursor is on the contact.
	m := readoutModel{at: subject{pid: 11}, follow: true, cursor: path, p: plain, report: readoutReport{pid: 11}, read: time.Now()}
	next, _ := m.Update(readoutTickMsg{})
	m = next.(readoutModel)
	text := texts(drawReadout(m.report, 120, 40, plain))
	for _, want := range []string{"Working · edit tui.go · for", "Under ........ Shell zsh · 11", "It has the terminal and is sleeping.", "Processor .... 1m 30s", "Pane ......... %3"} {
		if !strings.Contains(text, want) {
			t.Errorf("the page does not say %q:\n%s", want, text)
		}
	}

	// A row that stays put and changes its word is said again: the note
	// changed, so the page reads it, and nothing is asked after for a
	// cursor that did not move.
	r.projects[0].entries[1].status, r.projects[0].entries[1].doing = statusIdle, ""
	tellCursor(path, subject{pid: 22}, &r)
	before := m.read
	next, _ = m.Update(readoutTickMsg{})
	m = next.(readoutModel)
	if text := texts(drawReadout(m.report, 120, 40, plain)); !strings.Contains(text, "Idle") {
		t.Errorf("the page did not follow the row's word:\n%s", text)
	}
	if !m.read.Equal(before) {
		t.Error("a row that changed its word without moving asked after itself again")
	}

	// A container's row is composed from what docker said, which travels
	// with the reading; the page looks it up by the row.
	tellCursor(path, subject{pid: -99}, &r)
	next, _ = m.Update(readoutTickMsg{})
	m = next.(readoutModel)
	if text := texts(drawReadout(m.report, 120, 40, plain)); !strings.Contains(text, "nginx") || !strings.Contains(text, "abc123def456") {
		t.Errorf("the service's page was not composed from what the panel said of it:\n%s", text)
	}

	// A row the panel's reading has not got is a row that has gone: the
	// panel's cursor is always in the panel's own reading, so only a
	// pinned pid can be missing from it.
	pinned := readoutModel{at: subject{pid: 22}, follow: false, cursor: path, p: plain, report: readoutReport{pid: 22}, read: time.Now()}
	r.projects[0].entries = r.projects[0].entries[:1]
	tellCursor(path, subject{pid: 11}, &r)
	next, _ = pinned.Update(readoutTickMsg{})
	if got := next.(readoutModel).report; !got.gone || got.pid != 22 {
		t.Errorf("a pinned pid gone from the reading put up %+v", got)
	}
}

// The list's cursor stands on projects as often as on processes, and
// the page follows it either way: a process row is the process, and a
// project row is the project, by its path. A heading that is no place
// — the work off every project — is no subject, and the page keeps the
// one it had.
func TestTheListPublishesTheRowItsCursorIsOn(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONN_SOCKET", filepath.Join(dir, "tmux.sock"))
	path := cursorPath("/nowhere")

	m := newModel(plain)
	m.view, m.inside, m.now = viewProjects, true, processesNow
	m.head.login.home = dir
	m.walked = []projectRow{{name: "w0zro/conn", path: "/Users/w0zro/projects/w0zro/conn"}}
	m.projects = []project{
		{path: "/Users/w0zro/projects/w0zro/conn", entries: []entry{{pid: 11, tty: "ttys001", kind: kindShell, command: "zsh", status: statusIdle}}},
		{path: "", entries: []entry{{pid: 22, tty: "ttys002", kind: kindShell, command: "zsh", status: statusIdle}}},
	}
	m.panes = map[string]pane{"ttys001": {id: "%1", tty: "ttys001"}, "ttys002": {id: "%2", tty: "ttys002"}}
	rows := m.projectRows()
	if len(rows) != 4 {
		t.Fatalf("the list has %d rows: %+v", len(rows), rows)
	}
	press := func(k string) {
		next, _ := m.Update(tea.KeyPressMsg(tea.Key{Text: k}))
		m = next.(model)
	}
	published := func() subject { at, _ := askCursor(path); return at }

	// The first row is the project; the page is about the place.
	press("ctrl+p") // nothing to move to, and a key to publish on
	if got := published(); got.path != "/Users/w0zro/projects/w0zro/conn" || got.pid != 0 {
		t.Errorf("on the project row the list published %+v", got)
	}
	// Down one is the shell in it; the page is about the process.
	press("ctrl+n")
	if got := published(); got.pid != 11 {
		t.Errorf("on the process row the list published %+v", got)
	}
	// Down again is the heading for work off every project, which is
	// no place: the page keeps the process.
	press("ctrl+n")
	if got := published(); got.pid != 11 {
		t.Errorf("on the heading the list published %+v", got)
	}
	// And the shell under it is a process like any other.
	press("ctrl+n")
	if got := published(); got.pid != 22 {
		t.Errorf("on the last row the list published %+v", got)
	}

	// The page comes up in the list the way it does in the processes
	// view: the keys on the panel and a row under the cursor is enough,
	// and the walk landing is one of the moments it is asked for.
	m.looking, m.focused = false, true
	m.srv = &server{tmux: "/nonexistent/tmux", socket: filepath.Join(dir, "tmux.sock")}
	next, cmd := m.Update(projectsMsg{projects: m.walked})
	if got := next.(model); !got.looking || cmd == nil {
		t.Error("the walk landing in the list did not put the page in the workspace")
	}
}

// A project's page is where it is, what git says of it, and what conn
// has running there, each row at its own depth. A project with no git
// and nothing running says where it is and no more.
func TestAProjectHasAPageOfItsOwn(t *testing.T) {
	t.Setenv("CONN_SOCKET", filepath.Join(t.TempDir(), "tmux.sock"))
	held := readoutTable{
		reading: reading{projects: []project{{path: "/Users/w0zro/projects/w0zro/conn", entries: []entry{
			{pid: 11, kind: kindShell, command: "zsh", typed: "zsh", status: statusActive},
			{pid: 22, kind: kindRun, command: "go test ./...", typed: "go test ./...", status: statusWorking, depth: 1},
		}}}},
		git: map[string]gitStatus{"/Users/w0zro/projects/w0zro/conn": {repo: true, branch: "main", dirty: 2, commit: "abc1234", subject: "A thing", when: processesNow.Add(-time.Hour)}},
	}
	page, ok := readoutPage(subject{path: "/Users/w0zro/projects/w0zro/conn"}, held)
	if !ok {
		t.Fatal("a project was not there to be worded")
	}
	text := texts(drawReadout(page, 120, 40, plain))
	for _, want := range []string{"READOUT", "w0zro/conn", "PROJECT", "main · 2 changed", "RUNNING", "Shell zsh · 11 · Active", "  Run go test ./... · 22 · Working"} {
		if !strings.Contains(text, want) {
			t.Errorf("the project's page does not say %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "PID ") {
		t.Errorf("a project's page names a pid:\n%s", text)
	}
	bare, _ := readoutPage(subject{path: "/elsewhere"}, readoutTable{})
	if text := texts(drawReadout(bare, 120, 40, plain)); strings.Contains(text, "RUNNING") || strings.Contains(text, "BRANCH") {
		t.Errorf("a project with nothing to say said it anyway:\n%s", text)
	}
}

// The sessions list's cursor stands on suspended sessions, and the
// page follows it there too: the row is the session, by its id, and
// the page is what a reader would pick it up by, with the command
// that picks it up. The sessions landing is one of the moments the
// page is asked for.
func TestTheSessionsListPublishesTheSessionItsCursorIsOn(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONN_SOCKET", filepath.Join(dir, "tmux.sock"))
	path := cursorPath("/nowhere")

	m := newModel(plain)
	m.view, m.inside, m.now = viewSessions, true, processesNow
	m.head.login.home = dir
	m.sessionsProject, m.sessionsDirs = "/Users/w0zro/projects/w0zro/conn", []string{"/Users/w0zro/projects/w0zro/conn"}
	m.sessions = []session{
		{ID: "d81d7536-e545-4881-8daa-f1d291a03be1", Dir: "/Users/w0zro/projects/w0zro/conn", When: processesNow.Add(-2 * time.Hour),
			Branch: "main", Prompt: "make the page follow the list", Model: "claude-opus-5", Carried: 571_592},
		{ID: "0c1d2e3f-0000-4000-8000-000000000000", Dir: "/Users/w0zro/projects/w0zro/conn", When: processesNow.Add(-26 * time.Hour), Branch: "topic"},
	}
	press := func(k string) {
		next, _ := m.Update(tea.KeyPressMsg(tea.Key{Text: k}))
		m = next.(model)
	}
	published := func() subject { at, _ := askCursor(path); return at }
	press("ctrl+p")
	if got := published(); got.session != "d81d7536-e545-4881-8daa-f1d291a03be1" {
		t.Errorf("on the first session the list published %+v", got)
	}
	press("ctrl+n")
	if got := published(); got.session != "0c1d2e3f-0000-4000-8000-000000000000" {
		t.Errorf("on the second session the list published %+v", got)
	}
	// The sessions travel with the reading, so the page can word one.
	_, r := askCursor(path)
	if r == nil || len(r.sessions) != 2 {
		t.Fatalf("the sessions did not travel: %+v", r)
	}

	// The page, from what the panel said.
	held := readoutTable{reading: *r, git: map[string]gitStatus{"/Users/w0zro/projects/w0zro/conn": {repo: true, branch: "main", commit: "abc1234", subject: "A thing", when: processesNow.Add(-time.Hour)}}}
	page, ok := readoutPage(subject{session: "d81d7536-e545-4881-8daa-f1d291a03be1"}, held)
	if !ok {
		t.Fatal("a session the panel published was not there to be worded")
	}
	text := texts(drawReadout(page, 120, 40, plain))
	for _, want := range []string{"d81d7536-e545-4881-8daa-f1d291a03be1", "Session", "Claude Code · Anthropic", " ago · ", "main", "make the page follow the list",
		"claude-opus-5", "571K carried", "claude --resume d81d7536-e545-4881-8daa-f1d291a03be1", "w0zro/conn", "Branch"} {
		if !strings.Contains(text, want) {
			t.Errorf("the session's page does not say %q:\n%s", want, text)
		}
	}
	// A session the panel has not published is not yet said.
	if _, ok := readoutPage(subject{session: "nobody"}, held); ok {
		t.Error("a session the panel never published was worded anyway")
	}

	// The sessions landing puts the page up.
	m.looking, m.focused = false, true
	m.srv = &server{tmux: "/nonexistent/tmux", socket: filepath.Join(dir, "tmux.sock")}
	next, cmd := m.Update(sessionsMsg{dirs: m.sessionsDirs, sessions: m.sessions})
	if got := next.(model); !got.looking || cmd == nil {
		t.Error("the sessions landing did not put the page in the workspace")
	}
}
