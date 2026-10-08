package main

import (
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/w0zro/conn/internal/console"
	"github.com/w0zro/conn/internal/draw"
	"github.com/w0zro/conn/internal/room"
	"github.com/w0zro/conn/internal/work"
	"github.com/w0zro/conn/internal/work/claude"

	"github.com/w0zro/conn/internal/station"

	"github.com/w0zro/conn/internal/tmux"

	"github.com/w0zro/conn/internal/theme"

	tea "charm.land/bubbletea/v2"
)

// The claude on ttys007 says of itself that it has been working for
// seven minutes; nothing else has a moment, the way a first reading
// has none.
func testProcesses() processesReport {
	how := map[int]work.Status{70100: {Working: true, Since: processesNow.Add(-7 * time.Minute)}}
	return composeProcesses(work.ProjectsFrom(testProcs, 501, testRoots, testIsProject, how), nil, "", testProjRoots, testIsProject, "/Users/w0zro", processesNow, "", false, false)
}

// The processes view at 120 by 40 is a file of record, as are the empty
// view and the one that could not be read.
func TestProcessesMatchesTheGolden(t *testing.T) {
	golden(t, "processes-120x40.txt", texts(drawProcesses(testProcesses(), 67040, 120, 40, draw.Plain)))
	golden(t, "processes-cursor-100x9.txt", texts(drawProcesses(testProcesses(), 80002, 100, 9, draw.Plain)))
	empty := composeProcesses(nil, nil, "", testProjRoots, testIsProject, "/Users/w0zro", processesNow, "", false, false)
	golden(t, "processes-empty-80x24.txt", texts(drawProcesses(empty, 0, 80, 24, draw.Plain)))
	failed := composeProcesses(nil, nil, "", testProjRoots, testIsProject, "/Users/w0zro", processesNow, "the process table could not be read: lsof: not found", false, false)
	golden(t, "processes-unread-80x24.txt", texts(drawProcesses(failed, 0, 80, 24, draw.Plain)))
	panel := composeProcesses(work.ProjectsFrom(testProcs, 501, testRoots, testIsProject, nil), map[string]room.Pane{"ttys005": {ID: "%0"}, "ttys007": {ID: "%3"}}, "ttys007", testProjRoots, testIsProject, "/Users/w0zro", processesNow, "", false, false)
	panel.inside = true
	golden(t, "processes-panel-48x30.txt", texts(drawProcesses(panel, 70100, 48, 30, draw.Plain)))
	// A project's declarations: one up, in a pane marked as its own and
	// relabelled; one ended, holding its pane and dimmed; one down; and
	// a project whose file would not read, saying so under its rows.
	app := "/Users/w0zro/projects/w0zro/app"
	declared := map[string]work.Declared{
		app: {List: []work.Declaration{
			{Name: "web", Command: "npm run dev"},
			{Name: "api", Command: "go run ./cmd/api", Dir: "api"},
			{Name: "worker", Command: "make run"},
		}},
		"/Users/w0zro/projects/w0zro/conn": {Err: ".conn: line 2: want name [dir]: command"},
	}
	procs := append([]work.Process{},
		work.Process{PID: 900, PPID: 1, UID: 501, TTY: "ttys020", State: 'S', Command: "sh", Args: []string{"sh", "-c", "npm run dev"}, Started: processesNow.Add(-time.Hour), Cwd: app},
		work.Process{PID: 901, PPID: 900, UID: 501, TTY: "ttys020", State: 'S', Command: "node", Args: []string{"npm", "run", "dev"}, Started: processesNow.Add(-time.Hour), Cwd: app},
		work.Process{PID: 910, PPID: 1, UID: 501, TTY: "ttys021", State: 'S', Command: "cat", Args: []string{"cat"}, Started: processesNow.Add(-time.Hour), Cwd: app + "/api"},
		work.Process{PID: 67040, PPID: 1, UID: 501, TTY: "ttys005", State: 'S', Command: "zsh", Args: []string{"-zsh"}, Started: processesNow.Add(-90 * time.Second), Cwd: "/Users/w0zro/projects/w0zro/conn"},
	)
	panes := map[string]room.Pane{
		"ttys020": {ID: "%20", TTY: "ttys020", Declared: work.MarkDeclared(app, "web")},
		"ttys021": {ID: "%21", TTY: "ttys021", Declared: work.MarkDeclared(app, "api"), Exit: "0"},
		"ttys005": {ID: "%0", TTY: "ttys005"},
	}
	isProject := func(dir string) bool { return dir == app || testIsProject(dir) }
	projects := work.AttachDeclared(work.ProjectsFrom(procs, 501, work.RootFinder(isProject), isProject, nil), declared, declaredPanes(panes))
	shown := composeProcesses(projects, panes, "ttys020", testProjRoots, testIsProject, "/Users/w0zro", processesNow, "", false, false)
	shown.inside = true
	golden(t, "processes-declared-48x30.txt", texts(drawProcesses(shown, work.DeclaredPID(app, "worker"), 48, 30, draw.Plain)))
	// The panel at rest: the same processes, folded and filed as the
	// panel files them. The shell over claude keeps the contact and the
	// shell says what else it runs; the stopped vim stays for being a
	// fault. It is drawn as the panel and not as the tree: folded rows
	// stand in the panel's order, by kind, and the tree's indent over
	// them would say a contact runs under the shell standing below it.
	quiet := composeProcesses(fold(work.ProjectsFrom(testProcs, 501, testRoots, testIsProject, nil)), map[string]room.Pane{"ttys005": {ID: "%0"}, "ttys007": {ID: "%3"}}, "ttys007", testProjRoots, testIsProject, "/Users/w0zro", processesNow, "", false, true)
	quiet.inside = true
	golden(t, "processes-quiet-48x30.txt", texts(drawProcesses(quiet, 70100, 48, 30, draw.Plain)))
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
	procs := []work.Process{
		{PID: 100, PPID: 1, UID: 501, TTY: "ttys030", Foreground: true, State: 'S', Command: "claude", Args: []string{"claude"}, Started: processesNow.Add(-time.Hour), Cwd: rides},
		{PID: 200, PPID: 1, UID: 501, TTY: "ttys031", State: 'S', Command: "zsh", Args: []string{"-zsh"}, Started: processesNow.Add(-time.Hour), Cwd: rides + "/public-rides.com"},
		{PID: 201, PPID: 200, UID: 501, TTY: "ttys031", Foreground: true, State: 'S', Command: "ruby", Args: []string{"ruby", "ride"}, Started: processesNow.Add(-time.Hour), Cwd: rides + "/public-rides.com"},
		{PID: 300, PPID: 1, UID: 501, TTY: "ttys032", Foreground: true, State: 'S', Command: "python3", Args: []string{"python3", "data"}, Started: processesNow.Add(-time.Hour), Cwd: rides + "/public-rides.org"},
		{PID: 400, PPID: 1, UID: 501, TTY: "ttys033", Foreground: true, State: 'S', Command: "vim", Args: []string{"vim", "todo.md"}, Started: processesNow.Add(-time.Hour), Cwd: rides + "-notes"},
		{PID: 67040, PPID: 1, UID: 501, TTY: "ttys005", State: 'S', Command: "zsh", Args: []string{"-zsh"}, Started: processesNow.Add(-90 * time.Second), Cwd: "/Users/w0zro/projects/w0zro/conn"},
	}
	panes := map[string]room.Pane{"ttys030": {ID: "%30"}, "ttys031": {ID: "%31"}, "ttys032": {ID: "%32"}, "ttys033": {ID: "%33"}, "ttys005": {ID: "%0"}}
	held := composeProcesses(work.ProjectsFrom(procs, 501, work.RootFinder(isProject), isProject, nil), panes, "", testProjRoots, isProject, "/Users/w0zro", processesNow, "", false, false)
	held.inside = true
	golden(t, "processes-nested-48x30.txt", texts(drawProcesses(held, 201, 48, 30, draw.Plain)))
	// The same with nothing running in the folder itself: its heading
	// stands, made for the blocks under it.
	empty := composeProcesses(work.ProjectsFrom(procs[1:], 501, work.RootFinder(isProject), isProject, nil), panes, "", testProjRoots, isProject, "/Users/w0zro", processesNow, "", false, false)
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
	alone := composeProcesses(work.ProjectsFrom(append(procs[1:3:3], procs[5]), 501, work.RootFinder(isProject), isProject, nil), panes, "", testProjRoots, isProject, "/Users/w0zro", processesNow, "", false, false)
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
	one := composeProcesses(work.ProjectsFrom(append(procs[0:3:3], procs[5]), 501, work.RootFinder(isProject), isProject, nil), panes, "", testProjRoots, isProject, "/Users/w0zro", processesNow, "", false, false)
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
	rows := drawProcesses(testProcesses(), 70100, 120, 40, draw.Plain)
	text := texts(rows)
	measure := draw.MeasureOf(120)
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
		"▸      CONTACT claude",
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
	if got := strings.TrimSpace(rows[0].Text); got != "" {
		t.Errorf("the first row is %q, not a row of air", got)
	}
	if got := strings.TrimSpace(rows[1].Text); got != "~" {
		t.Errorf("the second row is %q, not the first project's name", got)
	}
	if got := rows[2].Text; !strings.HasPrefix(got, "     SHELL") {
		t.Errorf("the name's first row is not a level in under it: %q", got)
	}
	if len(rows) != 40 || strings.TrimSpace(rows[39].Text) != "" {
		t.Errorf("%d rows; the last is %q", len(rows), rows[len(rows)-1].Text)
	}
	for _, r := range rows {
		if w := utf8.RuneCountInString(r.Text); w > 120 {
			t.Errorf("row is %d wide: %q", w, r.Text)
		}
		if strings.HasSuffix(r.Text, "ACTIVE") && utf8.RuneCountInString(r.Text) != draw.Margin+measure {
			t.Errorf("status is not flush with %d: %q", draw.Margin+measure, r.Text)
		}
	}
	for i, r := range drawProcesses(testProcesses(), 70100, 120, 40, draw.Colored(theme.Conn.Dark)) {
		if w := utf8.RuneCountInString(stripEscapes(r.Text)); w != 120 {
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
	rows := drawProcesses(testProcesses(), 80001, 100, 9, draw.Plain)
	text := texts(rows)
	if len(rows) != 9 || !strings.Contains(text, "… 6 BELOW") || strings.Contains(text, "ABOVE") || !strings.Contains(text, "▸    SHELL") {
		t.Errorf("at 100x9 with the cursor on the first row:\n%s", text)
	}
	rows = drawProcesses(testProcesses(), 70301, 100, 9, draw.Plain)
	text = texts(rows)
	// The cursor's mark keeps the margin, and the row it marks still
	// steps in for the level it is at: the last row is the go three deep
	// under conjurer's shell.
	if len(rows) != 9 || !strings.Contains(text, "ABOVE") || strings.Contains(text, "BELOW") || !strings.Contains(text, "▸          RUN") {
		t.Errorf("at 100x9 with the cursor on the last row:\n%s", text)
	}
	if piped := drawProcesses(testProcesses(), 80001, 0, 0, draw.Plain); strings.Contains(texts(piped), "ABOVE") {
		t.Errorf("off a terminal:\n%s", texts(piped))
	}
}

// The cursor moves with j and k, stays within the rows, and follows its
// process across readings; when the process goes it holds its row.
// The rows are a ring to j and k: k on the first row is the last, and
// j on the last is the first, so cycling through the processes never
// stops at an end.
func TestJAndKGoRoundTheRows(t *testing.T) {
	m := model{p: draw.Plain, width: 120, height: 40, view: viewProcesses, uid: 501, roots: rooting{rootOf: testRoots}, now: processesNow}
	next, _ := m.Update(processesMsg{projects: work.ProjectsFrom(testProcs, 501, testRoots, testIsProject, nil)})
	m = next.(model)
	press := func(k string) {
		next, _ := m.Update(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
		m = next.(model)
	}
	last := rowsIn(m.projects) - 1
	if last < 1 {
		t.Fatalf("the fixture has %d rows, too few to go round", last+1)
	}
	press("k")
	if m.cursorAt != last {
		t.Errorf("k on the first row went to %d, not the last row %d", m.cursorAt, last)
	}
	press("j")
	if m.cursorAt != 0 {
		t.Errorf("j on the last row went to %d, not the first", m.cursorAt)
	}
	if got := ring(0, 0); got != 0 {
		t.Errorf("with no rows the ring is %d", got)
	}
}

func TestTheCursorFollowsItsProcess(t *testing.T) {
	m := model{p: draw.Plain, width: 120, height: 40, view: viewProcesses, uid: 501, roots: rooting{rootOf: testRoots}, now: processesNow}
	next, _ := m.Update(processesMsg{projects: work.ProjectsFrom(testProcs, 501, testRoots, testIsProject, nil)})
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
	next, _ = m.Update(processesMsg{projects: work.ProjectsFrom(testProcs, 501, testRoots, testIsProject, nil)})
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
	var without []work.Process
	for _, p := range testProcs {
		if p.PID != 70100 {
			without = append(without, p)
		}
	}
	next, _ = m.Update(processesMsg{projects: work.ProjectsFrom(without, 501, testRoots, testIsProject, nil)})
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
	m := model{head: station.Station{Build: testStation.Build, Login: testStation.Login}, now: processesNow, p: draw.Plain, width: 120, height: 40, uid: 501,
		// A conn that has been told where the work is. One that has not
		// goes to the asking view instead of the processes view, which is
		// its own test.
		roots: rooting{rootOf: testRoots, isProject: testIsProject, real: []string{"/Users/w0zro/projects"}}}
	st := testStation
	m.console.st = &st
	m.console.stage = console.LastStage(m.report())
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
	next, cmd = m.Update(processesMsg{projects: work.ProjectsFrom(testProcs, 501, testRoots, testIsProject, nil), gen: m.processesGen})
	m = next.(model)
	if m.view != viewProcesses || m.entering {
		t.Fatalf("the reading the console was waiting on did not put the processes view up")
	}
	if cmd == nil || !strings.Contains(m.View().Content, "claude") {
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
	if !strings.Contains(m.View().Content, "claude") {
		t.Errorf("the processes view came back empty rather than with the rows it had:\n%s", m.View().Content)
	}
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"}); cmd == nil {
		t.Error("q should close conn from the processes view")
	}
}

// In the server, the keys say what can be done, and a terminal the
// server does not hold is faint.
func TestTheProcessesViewInsideTheServer(t *testing.T) {
	w := composeProcesses(work.ProjectsFrom(testProcs, 501, testRoots, testIsProject, nil), map[string]room.Pane{"ttys007": {ID: "%3"}}, "ttys007", testProjRoots, testIsProject, "/Users/w0zro", processesNow, "", false, false)
	w.inside = true
	rows := drawProcesses(w, 67040, 120, 40, draw.Colored(theme.Conn.Dark))
	text := texts(rows)
	p := draw.Colored(theme.Conn.Dark)
	// ttys005 is in no pane the server holds here, and is the cursor's
	// row besides, so it reads at gray rather than faint; ttys007 is in
	// a pane, so it reads at the plain gray of a row conn can reach.
	if !strings.Contains(text, p.Gray+"TTYS005") || !strings.Contains(text, p.Gray+"TTYS007") {
		t.Errorf("the terminals are not colored by reach:\n%s", text)
	}
	// The bay's mark is on the kind of its head, which is the shell,
	// not the contact under it.
	if !strings.Contains(text, p.Orange+p.Bold+"SHELL") {
		t.Errorf("the bay's head is not marked:\n%s", text)
	}
	// In the panel there is no terminal column, and the rows close up.
	panelText := texts(drawProcesses(w, 67040, 48, 30, draw.Plain))
	if strings.Contains(panelText, "TTY") || !strings.Contains(panelText, "CONTACT claude ") {
		t.Errorf("the panel:\n%s", panelText)
	}
	for _, r := range drawProcesses(w, 67040, 48, 30, draw.Plain) {
		if w := utf8.RuneCountInString(r.Text); w > 48 {
			t.Errorf("panel row is %d wide: %q", w, r.Text)
		}
	}
}

// Enter reaches the cursor's process when its terminal is a pane of the
// server, n opens a shell at its project, and q detaches; each says why
// when it cannot. Outside the server q closes conn.
func TestKeysInsideTheServer(t *testing.T) {
	m := model{p: draw.Plain, width: 120, height: 40, view: viewProcesses, uid: 501, roots: rooting{rootOf: testRoots}, now: processesNow, srv: &room.Server{Server: &tmux.Server{Tmux: "/nonexistent/tmux", Socket: "/tmp/none"}}, inside: true}
	next, _ := m.Update(processesMsg{projects: work.ProjectsFrom(testProcs, 501, testRoots, testIsProject, nil), panes: map[string]room.Pane{"ttys007": {ID: "%3", TTY: "ttys007"}}})
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
	w := composeProcesses(work.ProjectsFrom(testProcs, 501, testRoots, testIsProject, nil), map[string]room.Pane{"ttys007": {ID: "%3"}}, "ttys007", testProjRoots, testIsProject, "/Users/w0zro", processesNow, "", false, false)
	for _, inside := range []bool{false, true} {
		w.inside = inside
		for _, size := range [][2]int{{120, 40}, {48, 30}, {100, 9}, {0, 0}} {
			text := stripEscapes(texts(drawProcesses(w, 67040, size[0], size[1], draw.Plain)))
			for _, key := range []string{"MOVE", "REACHES", "OPENS", "DETACHES", "CLOSES", "CONSOLE"} {
				if strings.Contains(text, key) {
					t.Errorf("inside=%v at %dx%d the processes view still says %q:\n%s", inside, size[0], size[1], key, text)
				}
			}
		}
	}
	if rows := drawProcesses(w, 67040, 120, 40, draw.Plain); len(rows) != 40 {
		t.Errorf("the processes view fills %d of 40 rows", len(rows))
	}
}

// The cursor is a ground, not a mark: its row is drawn on the selection
// color from edge to edge, and no other row is. Where there is no color
// to raise — a pipe, a golden file — the row takes a mark instead, so
// the record still says which one it is.
func TestTheCursorIsAGround(t *testing.T) {
	p := draw.Colored(theme.Conn.Dark)
	rows := drawProcesses(testProcesses(), 67040, 120, 40, p)
	on := 0
	for _, r := range rows {
		if strings.Contains(r.Text, p.Selection) {
			on++
			// conn's own project holds one row, the shell conn was not
			// started from, and it stands at the root of its tree, a
			// level in under the project's name.
			if !strings.HasPrefix(stripEscapes(r.Text), "     SHELL   zsh") {
				t.Errorf("the raised row is not the cursor's: %q", stripEscapes(r.Text))
			}
			// Raised from edge to edge: the row never falls back to the
			// ground partway along.
			if strings.Contains(r.Text, p.Ground) {
				t.Errorf("the raised row falls back to the ground: %q", r.Text)
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
	plainRows := texts(drawProcesses(testProcesses(), 67040, 120, 40, draw.Plain))
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
	p := draw.Colored(theme.Conn.Dark)
	held := composeProcesses(work.ProjectsFrom(testProcs, 501, testRoots, testIsProject, nil),
		map[string]room.Pane{"ttys005": {ID: "%0"}, "ttys007": {ID: "%3"}}, "ttys007",
		testProjRoots, testIsProject, "/Users/w0zro", processesNow, "", false, false)
	held.inside = true
	// The cursor is on a row conn holds a pane for, away from the rows
	// under test, so none of them is giving up a rank of dimming to be
	// read.
	text := texts(drawProcesses(held, 67040, 120, 40, p))

	// The bay is a mark: the kind of the head of what is in it, and
	// nothing else. Not its command, not its terminal, not its status —
	// a row is a lot of orange, and the status column is a color of its
	// own already.
	if !strings.Contains(text, p.Orange+p.Bold+"SHELL") {
		t.Errorf("the bay's head is not marked:\n%s", text)
	}
	for _, notIn := range []string{p.Orange + "zsh", p.Orange + "TTYS007", p.Orange + "ACTIVE"} {
		if strings.Contains(text, notIn) {
			t.Errorf("the orange ran past the kind: %q\n%s", notIn, text)
		}
	}
	// What hangs under the head is in the same pane and just as much in
	// the bay; it reads as the other true thing about it, which is that
	// conn holds a pane for it.
	if !strings.Contains(text, p.Ink+"claude") {
		t.Errorf("what hangs under the bay's head is not in the ink:\n%s", text)
	}
	// In nobody's pane: a rank down, and every column of it.
	for _, in := range []string{p.Faint + "vim notes.md", p.Faint + "TTYS009"} {
		if !strings.Contains(text, in) {
			t.Errorf("what conn cannot reach is not dimmed: %q missing\n%s", in, text)
		}
	}
	if strings.Contains(text, p.Ink+"vim notes.md") {
		t.Error("what conn cannot reach is written in the ink")
	}
	// The row under the cursor gives a rank of the dimming back rather
	// than the reading: faint on the raised ground is barely there.
	onIt := texts(drawProcesses(held, 80002, 120, 40, p))
	if !strings.Contains(onIt, p.Gray+p.Bold+"vim notes.md") {
		t.Errorf("the dimmed row under the cursor is not read back up:\n%s", onIt)
	}
	// Outside the server, every command is the ink: conn can reach none
	// of them, so dimming would say nothing.
	out := texts(drawProcesses(testProcesses(), 67040, 120, 40, p))
	for _, in := range []string{p.Ink + p.Bold + "zsh", p.Ink + "claude", p.Ink + "vim notes.md"} {
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
	m := model{p: draw.Plain, width: 140, height: 40, inside: true, view: viewProcesses}
	if got := m.cols(); got != room.PanelWidth {
		t.Errorf("the panel drew to %d columns, not the panel's %d", got, room.PanelWidth)
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
	held := []work.Project{{Path: "/w", Entries: []work.Entry{
		{PID: 11, Kind: work.KindShell, Command: "zsh", Status: work.StatusActive},
		{PID: 12, Kind: work.KindContact, Command: "claude", Status: work.StatusWaiting, Depth: 1, Since: processesNow.Add(-time.Minute)},
	}}}
	b := composeProcesses(held, nil, "", testProjRoots, testIsProject, "/Users/w0zro", processesNow, "", false, false)

	b.lit = true
	on := texts(drawProcesses(b, 0, 60, 12, draw.Plain))
	if !strings.Contains(on, work.StatusWaiting) {
		t.Errorf("the lit half has no word:\n%s", on)
	}
	b.lit = false
	off := texts(drawProcesses(b, 0, 60, 12, draw.Plain))
	if strings.Contains(off, work.StatusWaiting) {
		t.Errorf("the dark half still says it:\n%s", off)
	}
	// Only the word goes. Every row is the same shape on both halves, so
	// nothing around it moves.
	b.lit = true
	onRows := drawProcesses(b, 0, 60, 12, draw.Plain)
	b.lit = false
	offRows := drawProcesses(b, 0, 60, 12, draw.Plain)
	if len(onRows) != len(offRows) {
		t.Fatalf("the halves are %d rows and %d", len(onRows), len(offRows))
	}
	for i := range onRows {
		if lit, dark := onRows[i].Text, offRows[i].Text; lit != dark && !strings.Contains(lit, work.StatusWaiting) {
			t.Errorf("row %d moved between the halves:\n%q\n%q", i, lit, dark)
		}
	}
	// What is merely active does not blink, and neither does a fault: a
	// process you suspended yourself is not asking anything of you.
	steady := composeProcesses([]work.Project{{Path: "/w", Entries: []work.Entry{
		{PID: 21, Kind: work.KindEditor, Command: "vim", Status: work.StatusStopped, Fault: true},
	}}}, nil, "", testProjRoots, testIsProject, "/Users/w0zro", processesNow, "", false, false)
	steady.lit = false
	if !strings.Contains(texts(drawProcesses(steady, 0, 60, 12, draw.Plain)), work.StatusStopped) {
		t.Error("a fault went dark with the blink")
	}
}

// A contact carrying past HeavyContext is stamped with the figure where
// a fault's word goes, and the stamp blinks. Waiting outranks it, and
// the figure is back when the wait is over; a contact at the line and
// not past it is not stamped, and the panel says its figure unstamped.
func TestAHeavyContactIsStampedAndBlinks(t *testing.T) {
	contact := work.Entry{PID: 12, Kind: work.KindContact, Command: "claude", Status: work.StatusIdle, Carried: 612_000}
	tree := func(e work.Entry, lit bool) string {
		b := composeProcesses([]work.Project{{Path: "/w", Entries: []work.Entry{e}}}, nil, "", testProjRoots, testIsProject, "/Users/w0zro", processesNow, "", false, false)
		b.lit = lit
		return texts(drawProcesses(b, 0, 60, 12, draw.Plain))
	}
	if on := tree(contact, true); !strings.Contains(on, "612K") || strings.Contains(on, work.StatusIdle) {
		t.Errorf("the lit half is not the figure in the status's place:\n%s", on)
	}
	if off := tree(contact, false); strings.Contains(off, "612K") {
		t.Errorf("the dark half still says the figure:\n%s", off)
	}
	waiting := contact
	waiting.Status, waiting.Since = work.StatusWaiting, processesNow.Add(-time.Minute)
	if on := tree(waiting, true); !strings.Contains(on, work.StatusWaiting) || strings.Contains(on, "612K") {
		t.Errorf("a heavy contact waiting does not say WAITING alone:\n%s", on)
	}
	light := contact
	light.Carried = claude.HeavyContext
	if on := tree(light, true); !strings.Contains(on, work.StatusIdle) {
		t.Errorf("a contact at the line, not past it, was stamped:\n%s", on)
	}
	// The panel's own drawing says the same, at the row's right.
	filed := func(e work.Entry, lit bool) string {
		b := composeProcesses([]work.Project{{Path: "/w", Entries: []work.Entry{e}}}, nil, "", testProjRoots, testIsProject, "/Users/w0zro", processesNow, "", false, true)
		b.lit = lit
		return texts(drawFiled(b, 0, 44, 6, draw.Plain))
	}
	if on, off := filed(contact, true), filed(contact, false); !strings.Contains(on, "612K") || strings.Contains(off, "612K") {
		t.Errorf("the panel does not blink the figure:\n%s\n%s", on, off)
	}
	if on := filed(waiting, true); strings.Contains(on, "612K") {
		t.Errorf("the panel says the figure over a wait:\n%s", on)
	}
	// Under the line the panel says the figure at the row's right all
	// the same, in the gray and holding still.
	light.Carried = 212_000
	if on, off := filed(light, true), filed(light, false); !strings.Contains(on, "212K") || !strings.Contains(off, "212K") {
		t.Errorf("the panel does not hold the figure of a contact under the line:\n%s\n%s", on, off)
	}
	m := plainModel()
	m.view = viewProcesses
	m.projects = []work.Project{{Path: "/w", Entries: []work.Entry{contact}}}
	if !m.annunciating() {
		t.Error("a heavy contact does not set the blink going")
	}
}

// The spinner's tick runs while a row is working on the panel by state
// and stops when none is, or when the tree is up, or off the view: the
// panel is redrawn eight times a second for something seen to move and
// not otherwise. Its frame is the clock's at the tick's grain, so the
// frames go round in order at the tick's pace.
func TestTheSpinnerTurnsOnlyForWhatWorks(t *testing.T) {
	m := plainModel()
	if m.working() {
		t.Error("the console has a spinner turning")
	}
	m.view = viewProcesses
	m.projects = []work.Project{{Path: "/w", Entries: []work.Entry{{PID: 11, Status: work.StatusIdle}}}}
	if m.working() {
		t.Error("an idle row has a spinner turning")
	}
	m.projects = []work.Project{{Path: "/w", Entries: []work.Entry{{PID: 11, Status: work.StatusIdle}, {PID: 12, Status: work.StatusWorking}}}}
	if !m.working() {
		t.Error("a working row has no spinner turning")
	}
	m.full = true
	if m.working() {
		t.Error("the tree has a spinner turning")
	}
	m.full = false
	next, cmd := m.turned()
	if !next.spin.on || cmd == nil {
		t.Errorf("the spinner did not start: turning %v cmd %v", next.spin.on, cmd != nil)
	}
	if _, again := next.turned(); again != nil {
		t.Error("the spinner was started twice over")
	}
	next.projects = nil
	stopped, cmd := next.turned()
	if stopped.spin.on || cmd != nil {
		t.Errorf("the spinner did not stop: turning %v", stopped.spin.on)
	}
	// The frames, a turn a second: the eighth of a second after a frame
	// is the next one, and the second after it is the same one again.
	m.now = time.Unix(100, 0)
	first := m.processesReport().spin
	m.now = m.now.Add(spinEvery)
	if second := m.processesReport().spin; second != (first+1)%len(draw.Spinner) {
		t.Errorf("a tick on, the frame is %d after %d", second, first)
	}
	m.now = time.Unix(101, 0)
	if again := m.processesReport().spin; again != first {
		t.Errorf("a second on, the frame is %d, not %d again", again, first)
	}
	if spinEvery*time.Duration(len(draw.Spinner)) != time.Second {
		t.Errorf("a turn is %v, not a second", spinEvery*time.Duration(len(draw.Spinner)))
	}
}

// The blink runs while something annunciates and stops when nothing
// does, so a view with nothing held up on it is not redrawn a second
// and a half at a time for nothing.
func TestTheBlinkRunsOnlyForWhatAnnunciates(t *testing.T) {
	m := plainModel()
	if !m.annunciating() {
		t.Error("the console does not annunciate")
	}
	m.view = viewProcesses
	m.projects = []work.Project{{Path: "/w", Entries: []work.Entry{{PID: 11, Status: work.StatusActive}}}}
	if m.annunciating() {
		t.Error("a view with nothing waiting annunciates")
	}
	m.projects[0].Entries = append(m.projects[0].Entries, work.Entry{PID: 12, Status: work.StatusWaiting, Since: processesNow})
	if !m.annunciating() {
		t.Error("a row waiting on you does not annunciate")
	}
	// Coming to it starts the tick; going off it stops the tick and
	// leaves the word lit, which is where anything not blinking rests.
	m.blink.on, m.lit = false, false
	next, cmd := m.blinked()
	if !next.blink.on || !next.lit || cmd == nil {
		t.Errorf("the blink did not start: ticking %v lit %v cmd %v", next.blink.on, next.lit, cmd != nil)
	}
	if _, again := next.blinked(); again != nil {
		t.Error("the blink was started twice over")
	}
	next.projects = nil
	stopped, cmd := next.blinked()
	if stopped.blink.on || !stopped.lit || cmd != nil {
		t.Errorf("the blink did not stop: ticking %v lit %v", stopped.blink.on, stopped.lit)
	}
}

// The panel key, pressed on the panel, goes to the process that was in
// the bay before the one in it now, and takes the one in it now as the
// one to come back to — so pressed twice it is where it started. conn's own
// furniture is not somewhere you were working: a hold standing in an
// empty bay and the readout are not remembered, and going back never
// lands on one.
func TestTheOtherProcessIsTheOneYouWereLastIn(t *testing.T) {
	m := plainModel()
	m.view, m.inside, m.now = viewProcesses, true, processesNow
	m.srv = &room.Server{Server: &tmux.Server{Tmux: "/nonexistent/tmux", Socket: "/tmp/none"}}
	m.panes = map[string]room.Pane{
		"ttys001": {ID: "%1", TTY: "ttys001"},
		"ttys002": {ID: "%2", TTY: "ttys002"},
		"ttys009": {ID: "%9", TTY: "ttys009", Hold: true},
	}
	// The status line already says what view this is, so the only thing
	// a key can ask the server for here is the pane swap under test.
	said := m.telling()
	m.said = &said
	other := func(m model) (model, tea.Cmd) {
		next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Mod: tea.ModAlt, Code: '-'}))
		return next.(model), cmd
	}

	// Nothing has been in the bay yet, so there is nowhere to go back
	// to, and the server is asked for nothing.
	m, cmd := other(m)
	if cmd != nil {
		t.Error("with nothing behind it, going back asked the server for something")
	}

	// A hold in the bay, then a process: the hold is not remembered.
	m.bay.tty = "ttys009"
	next, _ := m.Update(reachedMsg{"ttys001"})
	m = next.(model)
	if m.bay.other != "" {
		t.Errorf("the hold was remembered as somewhere to go back to: %q", m.bay.other)
	}

	// A second process: the first is where going back leads.
	next, _ = m.Update(reachedMsg{"ttys002"})
	m = next.(model)
	if m.bay.tty != "ttys002" || m.bay.other != "ttys001" {
		t.Errorf("bay %q, other %q", m.bay.tty, m.bay.other)
	}
	m, cmd = other(m)
	if cmd == nil {
		t.Fatal("going back to the other process asked the server for nothing")
	}
	// Reaching answers with the terminal it put in the bay, and that
	// swaps which is which: pressed again it is back where it started.
	next, _ = m.Update(reachedMsg{"ttys001"})
	m = next.(model)
	if m.bay.tty != "ttys001" || m.bay.other != "ttys002" {
		t.Errorf("after going back: bay %q, other %q", m.bay.tty, m.bay.other)
	}

	// A process that has gone is not somewhere to go back to.
	gone := m
	gone.panes = map[string]room.Pane{"ttys001": {ID: "%1", TTY: "ttys001"}}
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
	held := []work.Project{{Path: "/w", Entries: []work.Entry{
		{PID: 11, Kind: work.KindContact, Command: "claude", Status: work.StatusWaiting, Since: processesNow.Add(-time.Minute)},
		{PID: 12, Kind: work.KindEditor, Command: "vim", Status: work.StatusStopped, Fault: true},
	}}}
	b := composeProcesses(held, nil, "", testProjRoots, testIsProject, "/Users/w0zro", processesNow, "", false, false)
	p := draw.Colored(theme.Conn.Dark)

	b.lit = true
	lit := texts(drawProcesses(b, 0, 80, 12, p))
	for what, want := range map[string]string{
		"the word that asks":  p.Chip + " " + work.StatusWaiting + " ",
		"the fault beside it": p.Chip + " " + work.StatusStopped + " ",
	} {
		if !strings.Contains(lit, want) {
			t.Errorf("%s is not stamped:\n%s", what, stripEscapes(lit))
		}
	}
	// The stamp is the console's own, not a second orange of its own.
	if !strings.Contains(screenChipOf(t), p.Chip) {
		t.Error("the console and the processes view stamp in different colors")
	}

	// Dark, the asking word is gone and the fault is still there: one
	// is answered, the other is only looked at.
	b.lit = false
	dark := texts(drawProcesses(b, 0, 80, 12, p))
	if strings.Contains(dark, work.StatusWaiting) {
		t.Errorf("the dark half still says it:\n%s", stripEscapes(dark))
	}
	if !strings.Contains(dark, p.Chip+" "+work.StatusStopped+" ") {
		t.Errorf("the fault blinked with it:\n%s", stripEscapes(dark))
	}
}

// screenChipOf is the console's own stamp, read off a console with a
// fault on it, so the two are compared rather than assumed.
func screenChipOf(t *testing.T) string {
	t.Helper()
	st := testStation
	st.Volume.Free = 6_800_000_000
	return texts(console.Screen(console.Compose(st, testNow), 120, 40, draw.Colored(theme.Conn.Dark)))
}

// A nested block's title is a title: in the parchment and bold like one
// at the margin, with the indent saying what it is under. In the ink
// it read as one more row, and a folder of repositories was a wall.
func TestANestedTitleIsBoldLikeAnyTitle(t *testing.T) {
	rides := "/Users/w0zro/projects/w0zro/public-rides"
	isProject := func(dir string) bool { return dir == rides || dir == rides+"/public-rides.com" || testIsProject(dir) }
	procs := []work.Process{
		{PID: 100, PPID: 1, UID: 501, TTY: "ttys030", Foreground: true, State: 'S', Command: "claude", Args: []string{"claude"}, Started: processesNow.Add(-time.Hour), Cwd: rides},
		{PID: 200, PPID: 1, UID: 501, TTY: "ttys031", State: 'S', Command: "zsh", Args: []string{"-zsh"}, Started: processesNow.Add(-time.Hour), Cwd: rides + "/public-rides.com"},
	}
	held := composeProcesses(work.ProjectsFrom(procs, 501, work.RootFinder(isProject), isProject, nil), map[string]room.Pane{"ttys030": {ID: "%30"}, "ttys031": {ID: "%31"}}, "", testProjRoots, isProject, "/Users/w0zro", processesNow, "", false, false)
	held.inside = true
	p := draw.Colored(theme.Conn.Dark)
	for _, r := range drawProcesses(held, 100, 48, 30, p) {
		if strings.Contains(r.Text, "public-rides.com") && !strings.Contains(r.Text, p.Parchment+p.Bold+"public-rides.com") {
			t.Errorf("the nested title is not in the parchment and bold: %q", r.Text)
		}
	}
}

// What is conn's own doing is not a row: a pane watching a service, and
// anything on the panel's own terminal that is not conn itself — a
// curl brew forked for its analytics and let go of, which carries the
// panel's terminal and nothing else to know it by. conn stays, since
// the shell it was launched from is held by walking up from it, and a
// process on any other terminal is untouched.
func TestConnsOwnDoingIsNoRow(t *testing.T) {
	procs := []work.Process{
		{PID: 10, PPID: 1, TTY: "ttys001", Args: []string{"conn"}},
		{PID: 11, PPID: 1, TTY: "ttys001", Args: []string{"curl", "https://analytics.brew.sh"}},
		{PID: 12, PPID: 1, TTY: "ttys002", Args: []string{"tail", "-f", "log"}},
		{PID: 13, PPID: 1, TTY: "ttys003", Args: []string{"zsh"}},
	}
	got := work.WithoutConnsOwn(procs, map[string]bool{"ttys002": true}, "ttys001")
	var pids []int
	for _, p := range got {
		pids = append(pids, p.PID)
	}
	if !slices.Equal(pids, []int{10, 13}) {
		t.Errorf("the table kept %v, not conn and the shell", pids)
	}
	if n := len(work.WithoutConnsOwn(procs, nil, "")); n != 4 {
		t.Errorf("with nothing to take out, %d of 4 were kept", n)
	}
}

// A project nothing is up in is not listed. Its rows are what its .conn
// declares and nothing of this machine, and a panel of them is a list
// of what could be started. Something up in it — a process, or a
// service brew holds for it — lists it, with what is down under it.
func TestAProjectNothingIsUpInIsNotListed(t *testing.T) {
	down := work.Entry{PID: -17000000, Kind: work.KindRun, Command: "npm run dev", Status: work.StatusDown}
	projects := []work.Project{
		{Path: "/r/app", Entries: []work.Entry{{PID: 1, Kind: work.KindShell, Command: "zsh", Status: work.StatusIdle}, down}},
		{Path: "/r/lib", Entries: []work.Entry{down}},
		{Path: "/r/db", Entries: []work.Entry{down, {PID: 2, Kind: work.KindService, Command: "postgresql@14", Status: work.StatusActive, Brew: "postgresql@14"}}},
		{Path: "/r/zed", Note: ".conn: line 1: want name [dir]: command"},
		{Path: "/r/gone"},
	}
	var got []string
	for _, pl := range worked(projects) {
		got = append(got, pl.Path)
	}
	want := []string{"/r/app", "/r/db", "/r/zed"}
	if !slices.Equal(got, want) {
		t.Errorf("the projects listed are %v, want %v", got, want)
	}
	// What is down under a project something is up in stays: a
	// declaration is worth reading beside work already happening.
	if rows := worked(projects)[0].Entries; len(rows) != 2 || rows[1].Status != work.StatusDown {
		t.Errorf("the down row was dropped with its project: %+v", rows)
	}
}
