package main

import (
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// The claude on ttys007 says of itself that it has been working for
// seven minutes; nothing else has a moment, the way a first reading
// has none.
func testProcesses() processesReport {
	how := map[int]status{70100: {working: true, since: processesNow.Add(-7 * time.Minute)}}
	return composeProcesses(projectsFrom(testProcs, 501, testRoots, testIsProject, how), nil, "", testProjRoots, testIsProject, "/Users/w0zro", processesNow, "", false)
}

// The processes view at 120 by 40 is a file of record, as are the empty
// view and the one that could not be read.
func TestProcessesMatchesTheGolden(t *testing.T) {
	golden(t, "processes-120x40.txt", texts(drawProcesses(testProcesses(), 67040, 120, 40, plain)))
	golden(t, "processes-cursor-100x9.txt", texts(drawProcesses(testProcesses(), 80002, 100, 9, plain)))
	empty := composeProcesses(nil, nil, "", testProjRoots, testIsProject, "/Users/w0zro", processesNow, "", false)
	golden(t, "processes-empty-80x24.txt", texts(drawProcesses(empty, 0, 80, 24, plain)))
	failed := composeProcesses(nil, nil, "", testProjRoots, testIsProject, "/Users/w0zro", processesNow, "the process table could not be read: lsof: not found", false)
	golden(t, "processes-unread-80x24.txt", texts(drawProcesses(failed, 0, 80, 24, plain)))
	panel := composeProcesses(projectsFrom(testProcs, 501, testRoots, testIsProject, nil), map[string]pane{"ttys005": {id: "%0"}, "ttys007": {id: "%3"}}, "ttys007", testProjRoots, testIsProject, "/Users/w0zro", processesNow, "", false)
	panel.inside = true
	golden(t, "processes-panel-48x30.txt", texts(drawProcesses(panel, 70100, 48, 30, plain)))
	// A project's declarations: one up, in a pane marked as its own and
	// relabelled; one ended, holding its pane and dimmed; one down; and
	// a project whose file would not read, saying so under its rows.
	app := "/Users/w0zro/projects/w0zro/app"
	declared := map[string]declared{
		app: {list: []declaration{
			{name: "web", command: "npm run dev"},
			{name: "api", command: "go run ./cmd/api", dir: "api"},
			{name: "worker", command: "make run"},
		}},
		"/Users/w0zro/projects/w0zro/conn": {err: ".conn: line 2: want name [dir]: command"},
	}
	procs := append([]process{},
		process{pid: 900, ppid: 1, uid: 501, tty: "ttys020", state: 'S', command: "sh", args: []string{"sh", "-c", "npm run dev"}, started: processesNow.Add(-time.Hour), cwd: app},
		process{pid: 901, ppid: 900, uid: 501, tty: "ttys020", state: 'S', command: "node", args: []string{"npm", "run", "dev"}, started: processesNow.Add(-time.Hour), cwd: app},
		process{pid: 910, ppid: 1, uid: 501, tty: "ttys021", state: 'S', command: "cat", args: []string{"cat"}, started: processesNow.Add(-time.Hour), cwd: app + "/api"},
		process{pid: 67040, ppid: 1, uid: 501, tty: "ttys005", state: 'S', command: "zsh", args: []string{"-zsh"}, started: processesNow.Add(-90 * time.Second), cwd: "/Users/w0zro/projects/w0zro/conn"},
	)
	panes := map[string]pane{
		"ttys020": {id: "%20", tty: "ttys020", declared: markDeclared(app, "web")},
		"ttys021": {id: "%21", tty: "ttys021", declared: markDeclared(app, "api"), exit: "0"},
		"ttys005": {id: "%0", tty: "ttys005"},
	}
	isProject := func(dir string) bool { return dir == app || testIsProject(dir) }
	projects := attachDeclared(projectsFrom(procs, 501, rootFinder(isProject), isProject, nil), declared, panes)
	shown := composeProcesses(projects, panes, "ttys020", testProjRoots, testIsProject, "/Users/w0zro", processesNow, "", false)
	shown.inside = true
	golden(t, "processes-declared-48x30.txt", texts(drawProcesses(shown, declaredPID(app, "worker"), 48, 30, plain)))
	// The panel at rest: the same processes, folded. The shell over
	// claude keeps the contact and the shell says what else it runs;
	// the stopped vim stays for being a fault.
	quiet := composeProcesses(fold(projectsFrom(testProcs, 501, testRoots, testIsProject, nil)), map[string]pane{"ttys005": {id: "%0"}, "ttys007": {id: "%3"}}, "ttys007", testProjRoots, testIsProject, "/Users/w0zro", processesNow, "", false)
	quiet.inside = true
	golden(t, "processes-quiet-48x30.txt", texts(drawProcesses(quiet, 70100, 48, 30, plain)))
}

// A folder of checkouts is a project of its own, and the checkouts in
// it are projects under it: the folder is a block, and each checkout a
// block nested a level in under it by its own name, its rows a level in
// under that, the way the list groups them. The folder's heading is
// drawn where nothing runs in the folder itself, so the checkouts have
// something to sit under; a project whose path sorts between the folder
// and its checkouts follows them rather than splitting them.
func TestProjectsNestUnderTheFolderThatHoldsThem(t *testing.T) {
	rides := "/Users/w0zro/projects/w0zro/public-rides"
	isProject := func(dir string) bool {
		return dir == rides || dir == rides+"/public-rides.com" || dir == rides+"/public-rides.org" || dir == rides+"-notes" || testIsProject(dir)
	}
	procs := []process{
		{pid: 100, ppid: 1, uid: 501, tty: "ttys030", foreground: true, state: 'S', command: "claude", args: []string{"claude"}, started: processesNow.Add(-time.Hour), cwd: rides},
		{pid: 200, ppid: 1, uid: 501, tty: "ttys031", state: 'S', command: "zsh", args: []string{"-zsh"}, started: processesNow.Add(-time.Hour), cwd: rides + "/public-rides.com"},
		{pid: 201, ppid: 200, uid: 501, tty: "ttys031", foreground: true, state: 'S', command: "ruby", args: []string{"ruby", "ride"}, started: processesNow.Add(-time.Hour), cwd: rides + "/public-rides.com"},
		{pid: 300, ppid: 1, uid: 501, tty: "ttys032", foreground: true, state: 'S', command: "python3", args: []string{"python3", "data"}, started: processesNow.Add(-time.Hour), cwd: rides + "/public-rides.org"},
		{pid: 400, ppid: 1, uid: 501, tty: "ttys033", foreground: true, state: 'S', command: "vim", args: []string{"vim", "todo.md"}, started: processesNow.Add(-time.Hour), cwd: rides + "-notes"},
		{pid: 67040, ppid: 1, uid: 501, tty: "ttys005", state: 'S', command: "zsh", args: []string{"-zsh"}, started: processesNow.Add(-90 * time.Second), cwd: "/Users/w0zro/projects/w0zro/conn"},
	}
	panes := map[string]pane{"ttys030": {id: "%30"}, "ttys031": {id: "%31"}, "ttys032": {id: "%32"}, "ttys033": {id: "%33"}, "ttys005": {id: "%0"}}
	held := composeProcesses(projectsFrom(procs, 501, rootFinder(isProject), isProject, nil), panes, "", testProjRoots, isProject, "/Users/w0zro", processesNow, "", false)
	held.inside = true
	golden(t, "processes-nested-48x30.txt", texts(drawProcesses(held, 201, 48, 30, plain)))
	// The same with nothing running in the folder itself: its heading
	// stands, made for the blocks under it.
	empty := composeProcesses(projectsFrom(procs[1:], 501, rootFinder(isProject), isProject, nil), panes, "", testProjRoots, isProject, "/Users/w0zro", processesNow, "", false)
	names := []string{}
	for _, bp := range empty.projects {
		names = append(names, strings.Repeat("  ", bp.nest)+bp.path)
	}
	want := []string{"w0zro/conn", "w0zro/public-rides", "  public-rides.com", "  public-rides.org", "w0zro/public-rides-notes"}
	if !slices.Equal(names, want) {
		t.Errorf("the blocks are %q, not %q", names, want)
	}
	// And with one repository alone under the folder, and nothing in
	// the folder itself: no heading is made over one thing. The
	// repository stands at the margin by its whole name, the way it
	// did before folders were headings at all.
	alone := composeProcesses(projectsFrom(append(procs[1:3:3], procs[5]), 501, rootFinder(isProject), isProject, nil), panes, "", testProjRoots, isProject, "/Users/w0zro", processesNow, "", false)
	names = names[:0]
	for _, bp := range alone.projects {
		names = append(names, strings.Repeat("  ", bp.nest)+bp.path)
	}
	want = []string{"w0zro/conn", "w0zro/public-rides/public-rides.com"}
	if !slices.Equal(names, want) {
		t.Errorf("one block under a folder drew as %q, not %q", names, want)
	}
	// A folder something runs in is a block of its own, and holds even
	// one repository under it: the heading is not made, it is there.
	one := composeProcesses(projectsFrom(append(procs[0:3:3], procs[5]), 501, rootFinder(isProject), isProject, nil), panes, "", testProjRoots, isProject, "/Users/w0zro", processesNow, "", false)
	names = names[:0]
	for _, bp := range one.projects {
		names = append(names, strings.Repeat("  ", bp.nest)+bp.path)
	}
	want = []string{"w0zro/conn", "w0zro/public-rides", "  public-rides.com"}
	if !slices.Equal(names, want) {
		t.Errorf("a worked folder with one repository drew as %q, not %q", names, want)
	}
}

// The processes view's columns hold: the status flush right, a root's
// kind at the margin and what runs under it a level in per level, the
// path from ~, the time in status where a moment is known, no legend,
// no row past the width.
func TestProcessesLaysOut(t *testing.T) {
	rows := drawProcesses(testProcesses(), 70100, 120, 40, plain)
	text := texts(rows)
	measure, _, _ := columns(120)
	for _, s := range []string{
		// No name over it: that is the status line's now, at the bottom left
		// of the window. No rule and no column heads either: the view
		// begins at the top of the pane with the first project's name.
		// A project is named by what is left of its path once the root the
		// checkouts are kept under is taken off it; one outside every
		// root is written from ~, whole.
		"w0zro/conn", "SHELL   zsh", "TTYS005", "IDLE",
		"w0zro/vim.pro/conjurer", "7M      WORKING", "ACTIVE",
		"~", " STOPPED",
		// A root a level in under its project's name, and the tree under
		// it stepping in: the contact its shell runs, the shell the
		// contact runs, the go that one runs. The cursor's mark sits in
		// the margin regardless.
		"\n     SHELL   zsh",
		"\n         SHELL   bash -c go test ./...",
		"\n           RUN     go test ./...",
		"\n       EDITOR  vim notes.md",
		"▸      CONTACT claude --resume",
	} {
		if !strings.Contains(text, s) {
			t.Errorf("the view lacks %q:\n%s", s, text)
		}
	}
	// No rule and no column heads at the top; the first project's name is
	// the first thing there is, with a row of air above it and its rows
	// straight under it. Above, because a name is read and reading does
	// not start hard against the edge of a pane — the heads that used to
	// sit there were furniture, which can. None below: the indent says
	// what is under the name, and a blank row means a new thing begins.
	if got := strings.TrimSpace(rows[0].text); got != "" {
		t.Errorf("the first row is %q, not a row of air", got)
	}
	if got := strings.TrimSpace(rows[1].text); got != "~" {
		t.Errorf("the second row is %q, not the first project's name", got)
	}
	if got := rows[2].text; !strings.HasPrefix(got, "     SHELL") {
		t.Errorf("the name's first row is not a level in under it: %q", got)
	}
	if len(rows) != 40 || strings.TrimSpace(rows[39].text) != "" {
		t.Errorf("%d rows; the last is %q", len(rows), rows[len(rows)-1].text)
	}
	for _, r := range rows {
		if w := utf8.RuneCountInString(r.text); w > 120 {
			t.Errorf("row is %d wide: %q", w, r.text)
		}
		if strings.HasSuffix(r.text, "ACTIVE") && utf8.RuneCountInString(r.text) != margin+measure {
			t.Errorf("status is not flush with %d: %q", margin+measure, r.text)
		}
	}
	for i, r := range drawProcesses(testProcesses(), 70100, 120, 40, colored()) {
		if w := utf8.RuneCountInString(stripEscapes(r.text)); w != 120 {
			t.Errorf("colored row %d paints %d columns", i, w)
		}
	}
	if strings.Count(text, "▸") != 1 {
		t.Errorf("the cursor marks %d rows", strings.Count(text, "▸"))
	}
}

// A view taller than the terminal scrolls to keep the cursor in view
// and says how many rows are above and below. The name is the status
// line's, so nine rows of terminal are nine of the view.
func TestAProcessesViewThatWillNotFitScrolls(t *testing.T) {
	rows := drawProcesses(testProcesses(), 80001, 100, 9, plain)
	text := texts(rows)
	if len(rows) != 9 || !strings.Contains(text, "… 6 BELOW") || strings.Contains(text, "ABOVE") || !strings.Contains(text, "▸    SHELL") {
		t.Errorf("at 100x9 with the cursor on the first row:\n%s", text)
	}
	rows = drawProcesses(testProcesses(), 70301, 100, 9, plain)
	text = texts(rows)
	// The cursor's mark keeps the margin, and the row it marks still
	// steps in for the level it is at: the last row is the go three deep
	// under conjurer's shell.
	if len(rows) != 9 || !strings.Contains(text, "ABOVE") || strings.Contains(text, "BELOW") || !strings.Contains(text, "▸          RUN") {
		t.Errorf("at 100x9 with the cursor on the last row:\n%s", text)
	}
	if piped := drawProcesses(testProcesses(), 80001, 0, 0, plain); strings.Contains(texts(piped), "ABOVE") {
		t.Errorf("off a terminal:\n%s", texts(piped))
	}
}

// The cursor moves with j and k, stays within the rows, and follows its
// process across readings; when the process goes it holds its row.
func TestTheCursorFollowsItsProcess(t *testing.T) {
	m := model{p: plain, width: 120, height: 40, view: viewProcesses, uid: 501, roots: rooting{rootOf: testRoots}, now: processesNow}
	next, _ := m.Update(processesMsg{projects: projectsFrom(testProcs, 501, testRoots, testIsProject, nil)})
	m = next.(model)
	// The rows read by project and then oldest first: home's shell and
	// the vim it holds stopped, then conn's shell, then the conjurer's
	// tree — its shell, the claude it runs, the node that one started,
	// the bash it started after, and the bash's own go.
	if m.cursor != 80001 {
		t.Errorf("the cursor should start on the first row, not %d", m.cursor)
	}
	press := func(k string) {
		next, _ := m.Update(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
		m = next.(model)
	}
	for range 4 {
		press("j")
	}
	// Four rows down from home's shell is claude: the editor under it,
	// then conn's one shell, then conjurer's shell, then the contact.
	if m.cursor != 70100 || m.cursorAt != 4 {
		t.Errorf("after four j the cursor is on %d at %d", m.cursor, m.cursorAt)
	}
	for range 3 {
		press("k")
	}
	if m.cursor != 80002 {
		t.Errorf("after three k the cursor is on %d", m.cursor)
	}
	// A reading that still has the pid keeps the cursor on it, wherever
	// in the rows it has moved to.
	next, _ = m.Update(processesMsg{projects: projectsFrom(testProcs, 501, testRoots, testIsProject, nil)})
	m = next.(model)
	if m.cursor != 80002 || m.cursorAt != 1 {
		t.Errorf("the cursor left the pid it was on: %d at %d", m.cursor, m.cursorAt)
	}
	// Down to claude itself, and then claude gone: what it ran stands on
	// its own, and the cursor, with no pid of its own left to follow,
	// holds the row it was at — which the node it started now has.
	for range 3 {
		press("j")
	}
	if m.cursor != 70100 || m.cursorAt != 4 {
		t.Errorf("the cursor is on %d at %d, not on claude", m.cursor, m.cursorAt)
	}
	var without []process
	for _, p := range testProcs {
		if p.pid != 70100 {
			without = append(without, p)
		}
	}
	next, _ = m.Update(processesMsg{projects: projectsFrom(without, 501, testRoots, testIsProject, nil)})
	m = next.(model)
	if m.cursorAt != 4 || m.cursor != 70212 {
		t.Errorf("with its process gone the cursor is on %d at %d", m.cursor, m.cursorAt)
	}
	next, _ = m.Update(processesMsg{})
	m = next.(model)
	if m.cursor != 0 || strings.Contains(m.View().Content, "▸") {
		t.Errorf("an empty view has a cursor: %d", m.cursor)
	}
}

// A key at the end of the console goes to the processes view, which
// reads the table and reads it again on its tick; c brings the console
// back, and a stale tick is dropped.
func TestTheKeyContinuesToProcesses(t *testing.T) {
	m := model{head: station{build: testStation.build, login: testStation.login}, now: processesNow, p: plain, width: 120, height: 40, uid: 501,
		// A conn that has been told where the work is. One that has not
		// goes to the asking view instead of the processes view, which is
		// its own test.
		roots: rooting{rootOf: testRoots, isProject: testIsProject, real: []string{"/Users/w0zro/projects"}}}
	st := testStation
	m.st = &st
	m.stage = lastStage(m.report())
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = next.(model)
	if m.view != viewConsole || !m.entering || cmd == nil {
		t.Fatalf("a key at the end should read the processes view and hold the console for the answer")
	}
	// The console holds rather than putting an empty view up: the
	// processes view arrives with its rows in it, in one change of the
	// screen.
	if !strings.Contains(m.View().Content, "START-UP CHECKS") {
		t.Errorf("the console should still be up while the reading is on its way:\n%s", m.View().Content)
	}
	next, cmd = m.Update(processesMsg{projects: projectsFrom(testProcs, 501, testRoots, testIsProject, nil), gen: m.processesGen})
	m = next.(model)
	if m.view != viewProcesses || m.entering {
		t.Fatalf("the reading the console was waiting on did not put the processes view up")
	}
	if cmd == nil || !strings.Contains(m.View().Content, "claude --resume") {
		t.Errorf("the processes view should show what was read and set the tick going:\n%s", m.View().Content)
	}
	if _, cmd := m.Update(processesTickMsg{gen: m.processesGen - 1}); cmd != nil {
		t.Error("a stale tick should be dropped")
	}
	if _, cmd := m.Update(processesTickMsg{gen: m.processesGen}); cmd == nil {
		t.Error("the tick should read the processes view again")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = next.(model)
	if m.view != viewConsole || !strings.Contains(m.View().Content, "START-UP CHECKS") {
		t.Errorf("c should bring the console back:\n%s", m.View().Content)
	}
	if _, cmd := m.Update(processesTickMsg{gen: m.processesGen}); cmd != nil {
		t.Error("a tick off the processes view should be dropped")
	}
	// Coming back, the rows of the last stay are still held, so the
	// processes view goes up with them at once rather than holding for a
	// reading.
	next, cmd = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = next.(model)
	if m.view != viewProcesses || m.entering || cmd == nil || m.processesGen != 2 {
		t.Errorf("a key on the finished console should return to the processes view and read it afresh: gen %d", m.processesGen)
	}
	if !strings.Contains(m.View().Content, "claude --resume") {
		t.Errorf("the processes view came back empty rather than with the rows it had:\n%s", m.View().Content)
	}
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"}); cmd == nil {
		t.Error("q should close conn from the processes view")
	}
}

// In the server, the keys say what can be done, and a terminal the
// server does not hold is faint.
func TestTheProcessesViewInsideTheServer(t *testing.T) {
	w := composeProcesses(projectsFrom(testProcs, 501, testRoots, testIsProject, nil), map[string]pane{"ttys007": {id: "%3"}}, "ttys007", testProjRoots, testIsProject, "/Users/w0zro", processesNow, "", false)
	w.inside = true
	rows := drawProcesses(w, 67040, 120, 40, colored())
	text := texts(rows)
	p := colored()
	// ttys005 is in no pane the server holds here, and is the cursor's
	// row besides, so it reads at gray rather than faint; ttys007 is in
	// a pane, so it reads at the plain gray of a row conn can reach.
	if !strings.Contains(text, p.gray+"TTYS005") || !strings.Contains(text, p.gray+"TTYS007") {
		t.Errorf("the terminals are not colored by reach:\n%s", text)
	}
	// The bay's mark is on the kind of its head, which is the shell,
	// not the contact under it.
	if !strings.Contains(text, p.orange+p.bold+"SHELL") {
		t.Errorf("the bay's head is not marked:\n%s", text)
	}
	// In the panel there is no terminal column, and the rows close up.
	panelText := texts(drawProcesses(w, 67040, 48, 30, plain))
	if strings.Contains(panelText, "TTY") || !strings.Contains(panelText, "CONTACT claude --resu") {
		t.Errorf("the panel:\n%s", panelText)
	}
	for _, r := range drawProcesses(w, 67040, 48, 30, plain) {
		if w := utf8.RuneCountInString(r.text); w > 48 {
			t.Errorf("panel row is %d wide: %q", w, r.text)
		}
	}
}

// Enter reaches the cursor's process when its terminal is a pane of the
// server, n opens a shell at its project, and q detaches; each says why
// when it cannot. Outside the server q closes conn.
func TestKeysInsideTheServer(t *testing.T) {
	m := model{p: plain, width: 120, height: 40, view: viewProcesses, uid: 501, roots: rooting{rootOf: testRoots}, now: processesNow, srv: &server{tmux: "/nonexistent/tmux", socket: "/tmp/none"}, inside: true}
	next, _ := m.Update(processesMsg{projects: projectsFrom(testProcs, 501, testRoots, testIsProject, nil), panes: map[string]pane{"ttys007": {id: "%3", tty: "ttys007"}}})
	m = next.(model)
	press := func(k string, code rune) tea.Cmd {
		next, cmd := m.Update(tea.KeyPressMsg{Code: code, Text: k})
		m = next.(model)
		return cmd
	}
	// The cursor starts on home's shell, whose terminal the server does
	// not hold: enter asks the server for nothing.
	if cmd := press("enter", tea.KeyEnter); cmd != nil {
		t.Error("enter on a process outside the server asked the server for something")
	}
	// Down to the conjurer's tree, whose terminal is a pane of the
	// server: enter reaches it, and against no tmux reaches nothing.
	for range 4 {
		press("j", 'j')
	}
	if cmd := press("enter", tea.KeyEnter); cmd == nil {
		t.Error("enter on a process in the server should reach it")
	} else if _, ok := cmd().(reachedMsg); ok {
		t.Error("a pane was reached with no tmux to reach it with")
	}
	if cmd := press("s", 's'); cmd == nil {
		t.Error("s should open a shell at the project")
	}
	if cmd := press("q", 'q'); cmd == nil {
		t.Error("q should detach")
	} else if _, quit := cmd().(tea.QuitMsg); quit {
		t.Error("q inside the server should not close conn")
	}
	m.inside = false
	if cmd := press("enter", tea.KeyEnter); cmd != nil {
		t.Error("enter outside the server asked the server for something")
	}
	if cmd := press("q", 'q'); cmd == nil {
		t.Error("q outside the server should close conn")
	} else if _, quit := cmd().(tea.QuitMsg); !quit {
		t.Error("q outside the server should close conn")
	}
}

// The processes view says no keys. They are learned once; a legend on
// every row of every reading is a thing to read past forever.
func TestTheProcessesViewSaysNoKeys(t *testing.T) {
	w := composeProcesses(projectsFrom(testProcs, 501, testRoots, testIsProject, nil), map[string]pane{"ttys007": {id: "%3"}}, "ttys007", testProjRoots, testIsProject, "/Users/w0zro", processesNow, "", false)
	for _, inside := range []bool{false, true} {
		w.inside = inside
		for _, size := range [][2]int{{120, 40}, {48, 30}, {100, 9}, {0, 0}} {
			text := stripEscapes(texts(drawProcesses(w, 67040, size[0], size[1], plain)))
			for _, key := range []string{"MOVE", "REACHES", "OPENS", "DETACHES", "CLOSES", "CONSOLE"} {
				if strings.Contains(text, key) {
					t.Errorf("inside=%v at %dx%d the processes view still says %q:\n%s", inside, size[0], size[1], key, text)
				}
			}
		}
	}
	if rows := drawProcesses(w, 67040, 120, 40, plain); len(rows) != 40 {
		t.Errorf("the processes view fills %d of 40 rows", len(rows))
	}
}

// The cursor is a ground, not a mark: its row is drawn on the selection
// color from edge to edge, and no other row is. Where there is no color
// to raise — a pipe, a golden file — the row takes a mark instead, so
// the record still says which one it is.
func TestTheCursorIsAGround(t *testing.T) {
	p := colored()
	rows := drawProcesses(testProcesses(), 67040, 120, 40, p)
	on := 0
	for _, r := range rows {
		if strings.Contains(r.text, p.selection) {
			on++
			// conn's own project holds one row, the shell conn was not
			// started from, and it stands at the root of its tree, a
			// level in under the project's name.
			if !strings.HasPrefix(stripEscapes(r.text), "     SHELL   zsh") {
				t.Errorf("the raised row is not the cursor's: %q", stripEscapes(r.text))
			}
			// Raised from edge to edge: the row never falls back to the
			// ground partway along.
			if strings.Contains(r.text, p.ground) {
				t.Errorf("the raised row falls back to the ground: %q", r.text)
			}
		}
	}
	if on != 1 {
		t.Errorf("%d rows are raised; one should be", on)
	}
	if strings.Contains(texts(rows), "▸") {
		t.Error("the cursor is still a mark where it has a ground")
	}
	// In plain text there is no ground to raise, so the mark stays.
	plainRows := texts(drawProcesses(testProcesses(), 67040, 120, 40, plain))
	if !strings.Contains(plainRows, "▸    SHELL   zsh") {
		t.Errorf("the plain view lost its cursor:\n%s", plainRows)
	}
}

// Three tiers, by what conn can do with a row: what is in the bay is
// the orange, what conn holds a pane for is the ink, and what it can
// only report is a rank down — the whole row of it, not the command
// alone. Outside the server conn holds nothing, and dims nothing: the
// distinction would be every row.
func TestTheRowsReadByWhatConnCanDoWithThem(t *testing.T) {
	p := colored()
	held := composeProcesses(projectsFrom(testProcs, 501, testRoots, testIsProject, nil),
		map[string]pane{"ttys005": {id: "%0"}, "ttys007": {id: "%3"}}, "ttys007",
		testProjRoots, testIsProject, "/Users/w0zro", processesNow, "", false)
	held.inside = true
	// The cursor is on a row conn holds a pane for, away from the rows
	// under test, so none of them is giving up a rank of dimming to be
	// read.
	text := texts(drawProcesses(held, 67040, 120, 40, p))

	// The bay is a mark: the kind of the head of what is in it, and
	// nothing else. Not its command, not its terminal, not its status —
	// a row is a lot of orange, and the status column is a color of its
	// own already.
	if !strings.Contains(text, p.orange+p.bold+"SHELL") {
		t.Errorf("the bay's head is not marked:\n%s", text)
	}
	for _, notIn := range []string{p.orange + "zsh", p.orange + "TTYS007", p.orange + "ACTIVE"} {
		if strings.Contains(text, notIn) {
			t.Errorf("the orange ran past the kind: %q\n%s", notIn, text)
		}
	}
	// What hangs under the head is in the same pane and just as much in
	// the bay; it reads as the other true thing about it, which is that
	// conn holds a pane for it.
	if !strings.Contains(text, p.ink+"claude --resume") {
		t.Errorf("what hangs under the bay's head is not in the ink:\n%s", text)
	}
	// In nobody's pane: a rank down, and every column of it.
	for _, in := range []string{p.faint + "vim notes.md", p.faint + "TTYS009"} {
		if !strings.Contains(text, in) {
			t.Errorf("what conn cannot reach is not dimmed: %q missing\n%s", in, text)
		}
	}
	if strings.Contains(text, p.ink+"vim notes.md") {
		t.Error("what conn cannot reach is written in the ink")
	}
	// The row under the cursor gives a rank of the dimming back rather
	// than the reading: faint on the raised ground is barely there.
	onIt := texts(drawProcesses(held, 80002, 120, 40, p))
	if !strings.Contains(onIt, p.gray+p.bold+"vim notes.md") {
		t.Errorf("the dimmed row under the cursor is not read back up:\n%s", onIt)
	}
	// Outside the server, every command is the ink: conn can reach none
	// of them, so dimming would say nothing.
	out := texts(drawProcesses(testProcesses(), 67040, 120, 40, p))
	for _, in := range []string{p.ink + p.bold + "zsh", p.ink + "claude --resume", p.ink + "vim notes.md"} {
		if !strings.Contains(out, in) {
			t.Errorf("outside the server a command is not in the ink:\n%s", out)
		}
	}
}

// The panel is conn's width, not the terminal's. Inside the server, off
// the console, conn draws to panelWidth rather than to whatever the
// pane happens to be at the moment — it holds tmux to that width
// anyway, and drawing to it means the frame conn paints is already the
// shape the pane is about to be, so the split that opens the bay has
// nothing to reflow.
func TestThePanelDrawsToItsOwnWidth(t *testing.T) {
	m := model{p: plain, width: 140, height: 40, inside: true, view: viewProcesses}
	if got := m.cols(); got != panelWidth {
		t.Errorf("the panel drew to %d columns, not the panel's %d", got, panelWidth)
	}
	// The console is the whole window, and takes the width it is given.
	m.view = viewConsole
	if got := m.cols(); got != 140 {
		t.Errorf("the console drew to %d columns, not the window's 140", got)
	}
	// Outside the server there is no bay to leave room for.
	m.view, m.inside = viewProcesses, false
	if got := m.cols(); got != 140 {
		t.Errorf("outside the server the processes view drew to %d columns", got)
	}
	// A window narrower than the panel is still the whole of what there
	// is to draw in.
	m.inside, m.width = true, 30
	if got := m.cols(); got != 30 {
		t.Errorf("a 30-column window drew to %d", got)
	}
}

// A project is named by what is left of its path once the root the
// checkouts are kept under is taken off it. The root is the same for
// every project and says nothing that tells one from another, and it
// was said at the head of every block on a panel forty-four columns
// wide.
func TestAProjectIsNamedByWhatTellsItApart(t *testing.T) {
	roots := []string{"/Users/w0zro/projects", "/srv/work"}
	for _, c := range []struct{ path, want string }{
		{"/Users/w0zro/projects/w0zro/conn", "w0zro/conn"},
		{"/Users/w0zro/projects/w0zro/vim.pro/conjurer", "w0zro/vim.pro/conjurer"},
		{"/srv/work/api", "api"},
		// A root itself has nothing left of it after itself, and is
		// written from ~ like anywhere else with nothing to take off.
		{"/Users/w0zro/projects", "~/projects"},
		// Outside every root, where it is is the whole of what the line
		// has to say.
		{"/Users/w0zro", "~"},
		{"/private/tmp/scratch", "/private/tmp/scratch"},
	} {
		if got := projectName(c.path, roots, "/Users/w0zro"); got != c.want {
			t.Errorf("%s is called %q, want %q", c.path, got, c.want)
		}
	}
	// With no roots at all nothing is taken off anything.
	if got := projectName("/Users/w0zro/projects/w0zro/conn", nil, "/Users/w0zro"); got != "~/projects/w0zro/conn" {
		t.Errorf("with no roots the project is called %q", got)
	}
}

// The one word in the processes view that asks something of you blinks,
// which is the one thing on a screen that reaches the corner of an eye:
// reading down a list of rows that all say something, the row that
// wants you is the row that moves. On the dark half its cells are the
// ground and nothing around them moves — a word that jumped its
// neighbours about would be worse than one that never blinked.
func TestTheWaitingWordBlinks(t *testing.T) {
	held := []project{{path: "/w", entries: []entry{
		{pid: 11, kind: kindShell, command: "zsh", status: statusActive},
		{pid: 12, kind: kindContact, command: "claude", status: statusWaiting, depth: 1, since: processesNow.Add(-time.Minute)},
	}}}
	b := composeProcesses(held, nil, "", testProjRoots, testIsProject, "/Users/w0zro", processesNow, "", false)

	b.lit = true
	on := texts(drawProcesses(b, 0, 60, 12, plain))
	if !strings.Contains(on, statusWaiting) {
		t.Errorf("the lit half has no word:\n%s", on)
	}
	b.lit = false
	off := texts(drawProcesses(b, 0, 60, 12, plain))
	if strings.Contains(off, statusWaiting) {
		t.Errorf("the dark half still says it:\n%s", off)
	}
	// Only the word goes. Every row is the same shape on both halves, so
	// nothing around it moves.
	b.lit = true
	onRows := drawProcesses(b, 0, 60, 12, plain)
	b.lit = false
	offRows := drawProcesses(b, 0, 60, 12, plain)
	if len(onRows) != len(offRows) {
		t.Fatalf("the halves are %d rows and %d", len(onRows), len(offRows))
	}
	for i := range onRows {
		if lit, dark := onRows[i].text, offRows[i].text; lit != dark && !strings.Contains(lit, statusWaiting) {
			t.Errorf("row %d moved between the halves:\n%q\n%q", i, lit, dark)
		}
	}
	// What is merely active does not blink, and neither does a fault: a
	// process you suspended yourself is not asking anything of you.
	steady := composeProcesses([]project{{path: "/w", entries: []entry{
		{pid: 21, kind: kindEditor, command: "vim", status: statusStopped, fault: true},
	}}}, nil, "", testProjRoots, testIsProject, "/Users/w0zro", processesNow, "", false)
	steady.lit = false
	if !strings.Contains(texts(drawProcesses(steady, 0, 60, 12, plain)), statusStopped) {
		t.Error("a fault went dark with the blink")
	}
}

// The blink runs while something annunciates and stops when nothing
// does, so a view with nothing held up on it is not redrawn a second
// and a half at a time for nothing.
func TestTheBlinkRunsOnlyForWhatAnnunciates(t *testing.T) {
	m := newModel(plain)
	if !m.annunciating() {
		t.Error("the console does not annunciate")
	}
	m.view = viewProcesses
	m.projects = []project{{path: "/w", entries: []entry{{pid: 11, status: statusActive}}}}
	if m.annunciating() {
		t.Error("a view with nothing waiting annunciates")
	}
	m.projects[0].entries = append(m.projects[0].entries, entry{pid: 12, status: statusWaiting, since: processesNow})
	if !m.annunciating() {
		t.Error("a row waiting on you does not annunciate")
	}
	// Coming to it starts the tick; going off it stops the tick and
	// leaves the word lit, which is where anything not blinking rests.
	m.ticking, m.lit = false, false
	next, cmd := m.blinked()
	if !next.ticking || !next.lit || cmd == nil {
		t.Errorf("the blink did not start: ticking %v lit %v cmd %v", next.ticking, next.lit, cmd != nil)
	}
	if _, again := next.blinked(); again != nil {
		t.Error("the blink was started twice over")
	}
	next.projects = nil
	stopped, cmd := next.blinked()
	if stopped.ticking || !stopped.lit || cmd != nil {
		t.Errorf("the blink did not stop: ticking %v lit %v", stopped.ticking, stopped.lit)
	}
}

// The prefix twice over goes to the process that was in the bay before
// the one in it now, and takes the one in it now as the one to come
// back to — so pressed twice it is where it started. conn's own
// furniture is not somewhere you were working: a hold standing in an
// empty bay and the readout are not remembered, and going back never
// lands on one.
func TestTheOtherProcessIsTheOneYouWereLastIn(t *testing.T) {
	m := newModel(plain)
	m.view, m.inside, m.now = viewProcesses, true, processesNow
	m.srv = &server{tmux: "/nonexistent/tmux", socket: "/tmp/none"}
	m.panes = map[string]pane{
		"ttys001": {id: "%1", tty: "ttys001"},
		"ttys002": {id: "%2", tty: "ttys002"},
		"ttys009": {id: "%9", tty: "ttys009", hold: true},
	}
	// The status line already says what view this is, so the only thing
	// a key can ask the server for here is the pane swap under test.
	m.said, m.saidKeys, m.saidStation, m.saidUp, m.saidBar = true, m.keys(), m.station(), m.upWord(), m.bar()
	other := func(m model) (model, tea.Cmd) {
		next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Mod: tea.ModAlt, Code: 'o'}))
		return next.(model), cmd
	}

	// Nothing has been in the bay yet, so there is nowhere to go back
	// to, and the server is asked for nothing.
	m, cmd := other(m)
	if cmd != nil {
		t.Error("with nothing behind it, going back asked the server for something")
	}

	// A hold in the bay, then a process: the hold is not remembered.
	m.bay = "ttys009"
	next, _ := m.Update(reachedMsg{"ttys001"})
	m = next.(model)
	if m.lastBay != "" {
		t.Errorf("the hold was remembered as somewhere to go back to: %q", m.lastBay)
	}

	// A second process: the first is where going back leads.
	next, _ = m.Update(reachedMsg{"ttys002"})
	m = next.(model)
	if m.bay != "ttys002" || m.lastBay != "ttys001" {
		t.Errorf("bay %q, other %q", m.bay, m.lastBay)
	}
	m, cmd = other(m)
	if cmd == nil {
		t.Fatal("going back to the other process asked the server for nothing")
	}
	// Reaching answers with the terminal it put in the bay, and that
	// swaps which is which: pressed again it is back where it started.
	next, _ = m.Update(reachedMsg{"ttys001"})
	m = next.(model)
	if m.bay != "ttys001" || m.lastBay != "ttys002" {
		t.Errorf("after going back: bay %q, other %q", m.bay, m.lastBay)
	}

	// A process that has gone is not somewhere to go back to.
	gone := m
	gone.panes = map[string]pane{"ttys001": {id: "%1", tty: "ttys001"}}
	if _, cmd := other(gone); cmd != nil {
		t.Error("a pane that has gone was gone back to")
	}

	// Outside the server nothing can be reached at all.
	out := m
	out.inside = false
	if _, cmd := other(out); cmd != nil {
		t.Error("outside the server, going back asked the server for something")
	}
}

// The word that asks something of you is stamped the way the console
// stamps a fault and the status line stamps the keys: a block of the
// orange with the word knocked out of it. A block is not read but
// seen, and the thing that wants you should be seen before it is read.
// A fault beside it wears the same stamp and holds still; the blink is
// the difference between a thing to look at and a thing to answer.
func TestTheWaitingWordIsStampedLikeAFault(t *testing.T) {
	held := []project{{path: "/w", entries: []entry{
		{pid: 11, kind: kindContact, command: "claude", status: statusWaiting, since: processesNow.Add(-time.Minute)},
		{pid: 12, kind: kindEditor, command: "vim", status: statusStopped, fault: true},
	}}}
	b := composeProcesses(held, nil, "", testProjRoots, testIsProject, "/Users/w0zro", processesNow, "", false)
	p := colored()

	b.lit = true
	lit := texts(drawProcesses(b, 0, 80, 12, p))
	for what, want := range map[string]string{
		"the word that asks":  p.chip + " " + statusWaiting + " ",
		"the fault beside it": p.chip + " " + statusStopped + " ",
	} {
		if !strings.Contains(lit, want) {
			t.Errorf("%s is not stamped:\n%s", what, stripEscapes(lit))
		}
	}
	// The stamp is the console's own, not a second orange of its own.
	if !strings.Contains(screenChipOf(t), p.chip) {
		t.Error("the console and the processes view stamp in different colors")
	}

	// Dark, the asking word is gone and the fault is still there: one
	// is answered, the other is only looked at.
	b.lit = false
	dark := texts(drawProcesses(b, 0, 80, 12, p))
	if strings.Contains(dark, statusWaiting) {
		t.Errorf("the dark half still says it:\n%s", stripEscapes(dark))
	}
	if !strings.Contains(dark, p.chip+" "+statusStopped+" ") {
		t.Errorf("the fault blinked with it:\n%s", stripEscapes(dark))
	}
}

// screenChipOf is the console's own stamp, read off a console with a
// fault on it, so the two are compared rather than assumed.
func screenChipOf(t *testing.T) string {
	t.Helper()
	st := testStation
	st.volume.free = 6_800_000_000
	return texts(screen(compose(st, testNow), 120, 40, colored()))
}

// A nested block's title is a title: in the parchment and bold like one
// at the margin, with the indent saying what it is under. In the ink
// it read as one more row, and a folder of repositories was a wall.
func TestANestedTitleIsBoldLikeAnyTitle(t *testing.T) {
	rides := "/Users/w0zro/projects/w0zro/public-rides"
	isProject := func(dir string) bool { return dir == rides || dir == rides+"/public-rides.com" || testIsProject(dir) }
	procs := []process{
		{pid: 100, ppid: 1, uid: 501, tty: "ttys030", foreground: true, state: 'S', command: "claude", args: []string{"claude"}, started: processesNow.Add(-time.Hour), cwd: rides},
		{pid: 200, ppid: 1, uid: 501, tty: "ttys031", state: 'S', command: "zsh", args: []string{"-zsh"}, started: processesNow.Add(-time.Hour), cwd: rides + "/public-rides.com"},
	}
	held := composeProcesses(projectsFrom(procs, 501, rootFinder(isProject), isProject, nil), map[string]pane{"ttys030": {id: "%30"}, "ttys031": {id: "%31"}}, "", testProjRoots, isProject, "/Users/w0zro", processesNow, "", false)
	held.inside = true
	p := colored()
	for _, r := range drawProcesses(held, 100, 48, 30, p) {
		if strings.Contains(r.text, "public-rides.com") && !strings.Contains(r.text, p.parchment+p.bold+"public-rides.com") {
			t.Errorf("the nested title is not in the parchment and bold: %q", r.text)
		}
	}
}
