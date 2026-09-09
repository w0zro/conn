package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// sized returns a model laid out for the given terminal dimensions. The
// startup scan is taken to have answered, which is the state every test that
// seeds procs directly is standing in.
func sized(w, h int) model {
	m, _ := newModel().Update(tea.WindowSizeMsg{Width: w, Height: h})
	s := m.(model)
	s.scanning = false
	return s
}

// withProcs builds a model from repos and one process per given directory.
func withProcs(w, h int, projects []Project, dirs []string) model {
	procs := make([]Proc, len(dirs))
	for i, d := range dirs {
		procs[i] = Proc{PID: 100 + i, PPID: 1, Command: "proc", Dir: d}
	}
	return withProcList(w, h, projects, procs)
}

// withProcList builds a model showing every repository. The narrowed view is
// the default in the app, so tests that want it call narrowed().
func withProcList(w, h int, projects []Project, procs []Proc) model {
	m := sized(w, h)
	m.showAll, m.all = true, true
	m.projects, m.procs = projects, procs
	m.rebuild()
	return m
}

// narrowed flips the model to the running-only view, as Update does.
func narrowed(m model) model {
	m.showAll = false
	m.rebuild()
	return m
}

// typed is a printable keystroke as the terminal would deliver it.
func typed(s string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
}

// press sends a key and returns the resulting model.
func press(m model, key string) model {
	var msg tea.KeyPressMsg
	switch key {
	case "up":
		msg = tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		msg = tea.KeyPressMsg{Code: tea.KeyDown}
	default:
		msg = typed(key)
	}
	next, _ := m.Update(msg)
	return next.(model)
}

// killed and killFailed are the outcome of signalling one process, shaped the
// way killTree reports it.
func killed(command string, pid int) killedMsg {
	return killedMsg{
		subject: command + " " + strconv.Itoa(pid),
		results: []killResult{{command: command, pid: pid}},
	}
}

func killFailed(command string, pid int, err error) killedMsg {
	msg := killed(command, pid)
	msg.results[0].err = err
	return msg
}

// targets lists what a pending kill would signal, in the order it would.
func targets(req *killRequest) []int {
	if req == nil {
		return nil
	}
	return pids(req.nodes)
}

// bodyRows returns every row of the view, plain.
func bodyRows(m model) []string {
	all := strings.Split(m.View().Content, "\n")
	out := make([]string, 0, len(all))
	for _, ln := range all {
		out = append(out, strings.TrimRight(stripANSI(ln), " "))
	}
	return out
}

// navColumn returns the non-blank rows of the everything view's list, as
// the view draws them: the rows and nothing over or under them — the
// tabline and the footer are the view's, not the list's.
func navColumn(m model) []string {
	var out []string
	for _, row := range m.navLines(m.bodyHeight()) {
		if row = strings.TrimRight(stripANSI(row), " "); strings.TrimSpace(row) != "" {
			out = append(out, row)
		}
	}
	return out
}

// tablineOf is the view's first row, plain: the tabs and the hint.
func tablineOf(m model) string {
	return strings.Join(strings.Fields(bodyRows(m)[0]), " ")
}

func wantRows(t *testing.T, got, want []string) {
	t.Helper()
	for i, w := range want {
		if i >= len(got) || !strings.HasPrefix(got[i], w) {
			t.Fatalf("row %d = %q, want prefix %q\nfull:\n%s", i, lineAt(got, i), w, strings.Join(got, "\n"))
		}
	}
}

func lineAt(ls []string, i int) string {
	if i < len(ls) {
		return ls[i]
	}
	return "<missing>"
}

// --- layout ---------------------------------------------------------------

func TestViewPutsTheTablineOverTheList(t *testing.T) {
	// The first row of the window is the tabline — the everything view's
	// tab, and the hint at its end — and the list follows under a blank.
	// conn's name is on the status line, not over the list.
	m := withProcList(80, 24, []Project{{Name: "alpha", Path: "/p/alpha"}}, nil)
	rows := bodyRows(m)
	if got := len(rows); got != 24 {
		t.Fatalf("view height = %d lines, want 24", got)
	}
	if !strings.HasPrefix(tablineOf(m), "everything") || !strings.HasSuffix(tablineOf(m), "running · all") {
		t.Errorf("tabline = %q, want the everything tab and its hint", tablineOf(m))
	}
	if rows[2] != "▸ alpha" {
		t.Errorf("third line = %q, want the list's first row under the tabline and a blank", rows[2])
	}
	if strings.Contains(stripANSI(m.View().Content), " conn\n") {
		t.Error("the view still carries conn's name")
	}
}

func TestTheFootTeachesTheKeysThatMatter(t *testing.T) {
	m := withProcList(80, 24, []Project{{Name: "alpha", Path: "/p/alpha"}}, nil)
	foot := strings.Join(bodyRows(m)[20:], " ")
	for _, want := range []string{"space folds a place", "/ narrows", "enter opens the buffer", "x previews a kill"} {
		if !strings.Contains(foot, want) {
			t.Errorf("foot = %q, want %q taught", foot, want)
		}
	}
}

func TestViewFitsShortTerminals(t *testing.T) {
	for _, h := range []int{0, 1, 2, 3} {
		got := len(strings.Split(sized(80, h).View().Content, "\n"))
		if got > 3 && got > h {
			t.Errorf("height %d: view = %d lines, overflows", h, got)
		}
	}
}

// --- navigator contents ---------------------------------------------------

func TestNavListsRepoNames(t *testing.T) {
	m := withProcList(80, 8, []Project{{Name: "alpha"}, {Name: "beta"}}, nil)
	wantRows(t, navColumn(m), []string{"▸ alpha", "  beta"})
}

func TestNavTruncatesLongNames(t *testing.T) {
	m := withProcList(80, 8, []Project{{Name: strings.Repeat("x", 100)}}, nil)
	if got := len([]rune(navColumn(m)[0])); got > m.width {
		t.Errorf("row is %d columns wide, want at most %d", got, m.width)
	}
}

func TestQualifiedNamesKeepTheirRepoName(t *testing.T) {
	m := withProcList(80, 8,
		[]Project{{Name: "w0zro/archive/checklists.org/checklists-api"}}, nil)

	row := navColumn(m)[0]
	if !strings.Contains(row, "checklists-api") {
		t.Errorf("row = %q, want the repo name to survive truncation", row)
	}
	if got := len([]rune(row)); got > m.width {
		t.Errorf("row is %d columns, want at most %d: %q", got, m.width, row)
	}
}

func TestNavShowsScanError(t *testing.T) {
	m := sized(80, 6)
	m.err = errors.New("boom")
	if !strings.Contains(strings.Join(navColumn(m), " "), "boom") {
		t.Errorf("scan errors should be visible:\n%s", strings.Join(navColumn(m), "\n"))
	}
}

func TestNavShowsEmptyProjectsDir(t *testing.T) {
	m := withProcList(80, 6, []Project{}, nil)
	if !strings.Contains(strings.Join(navColumn(m), " "), "no repositories") {
		t.Error("an empty projects dir should say so")
	}
}

// --- process trees --------------------------------------------------------

func TestARunThatNeverBranchesIsOneRow(t *testing.T) {
	// A shell that started a claude that started a go build is one thing
	// happening, and the row is named for the claude: the shell is what got
	// there and the go is what it reached for.
	m := withProcList(80, 12,
		[]Project{{Name: "conn", Path: "/p/conn"}},
		[]Proc{
			{PID: 10, PPID: 1, Command: "zsh", Dir: "/p/conn"},
			{PID: 20, PPID: 10, Command: "claude", Dir: "/p/conn"},
			{PID: 30, PPID: 20, Command: "go", Dir: "/p/conn/cmd"},
		},
	)
	wantRows(t, navColumn(m), []string{"▸ conn", "      claude"})

	if r, _ := m.rows[1], 0; r.chain().PID != 10 {
		t.Errorf("chain starts at %d, want the shell at the top of the run", r.chain().PID)
	}
}

func TestASameCommandForkingItselfIsStillOneRow(t *testing.T) {
	// nvim starts a second nvim; that is one editor, not two.
	m := withProcList(80, 12,
		[]Project{{Name: "conn", Path: "/p/conn"}},
		[]Proc{
			{PID: 10, PPID: 1, Command: "zsh", Dir: "/p/conn"},
			{PID: 20, PPID: 10, Command: "nvim", Dir: "/p/conn"},
			{PID: 21, PPID: 20, Command: "nvim", Dir: "/p/conn"},
		},
	)
	wantRows(t, navColumn(m), []string{"▸ conn", "      nvim"})
}

func TestARunStopsFoldingWhereItBranches(t *testing.T) {
	m := withProcList(80, 12,
		[]Project{{Name: "conn", Path: "/p/conn"}},
		[]Proc{
			{PID: 10, PPID: 1, Command: "zsh", Dir: "/p/conn"},
			{PID: 20, PPID: 10, Command: "nvim", Dir: "/p/conn"},
			{PID: 30, PPID: 10, Command: "zig", Dir: "/p/conn"},
		},
	)
	wantRows(t, navColumn(m), []string{"▸ conn", "      zsh", "        nvim", "        zig"})
}

func TestNavIndentsSiblingsUnderTheirParent(t *testing.T) {
	m := withProcList(80, 12,
		[]Project{{Name: "conn", Path: "/p/conn"}},
		[]Proc{
			{PID: 10, PPID: 1, Command: "zsh", Dir: "/p/conn"},
			{PID: 20, PPID: 10, Command: "vim", Dir: "/p/conn"},
			{PID: 30, PPID: 10, Command: "zig", Dir: "/p/conn"},
			{PID: 40, PPID: 20, Command: "fmt", Dir: "/p/conn"},
			{PID: 50, PPID: 20, Command: "lint", Dir: "/p/conn"},
		},
	)
	wantRows(t, navColumn(m), []string{
		"▸ conn", "      zsh", "        vim", "          fmt", "          lint", "        zig",
	})
}

func TestProcessesGoUnderTheInnermostRepo(t *testing.T) {
	m := withProcList(80, 12,
		[]Project{{Name: "outer", Path: "/p/outer"}, {Name: "inner", Path: "/p/outer/inner"}},
		[]Proc{{PID: 10, PPID: 1, Command: "zsh", Dir: "/p/outer/inner/src"}},
	)
	if n := len(m.byPlace["/p/outer"]); n != 0 {
		t.Errorf("outer repo got %d processes, want 0; the nested repo owns it", n)
	}
	if n := len(m.byPlace["/p/outer/inner"]); n != 1 {
		t.Errorf("inner repo got %d processes, want 1", n)
	}
}

// --- the "." toggle -------------------------------------------------------

func TestNavStartsNarrowedToRunningRepos(t *testing.T) {
	if newModel().showAll {
		t.Error("conn should open on the repositories with something running")
	}

	m := sized(80, 10)
	m.projects = []Project{{Name: "busy", Path: "/p/busy"}, {Name: "idle", Path: "/p/idle"}}
	m.procs = []Proc{{PID: 10, PPID: 1, Command: "zsh", Dir: "/p/busy"}}
	m.rebuild()

	col := strings.Join(navColumn(m), "\n")
	if !strings.Contains(col, "busy") {
		t.Errorf("a repo with a process should be listed at startup:\n%s", col)
	}
	if strings.Contains(col, "idle") {
		t.Errorf("an idle repo should not be listed at startup:\n%s", col)
	}
}

func TestNarrowedShowsOnlyReposWithProcesses(t *testing.T) {
	m := narrowed(withProcs(80, 10,
		[]Project{
			{Name: "busy", Path: "/p/busy"},
			{Name: "idle", Path: "/p/idle"},
			{Name: "nested", Path: "/p/nested"},
		},
		[]string{"/p/busy", "/p/nested/cmd/x", "/elsewhere"},
	))

	col := strings.Join(navColumn(m), "\n")
	if !strings.Contains(col, "busy") || !strings.Contains(col, "nested") {
		t.Errorf("repos with processes should be listed:\n%s", col)
	}
	if strings.Contains(col, "idle") {
		t.Errorf("a repo with nothing running should be hidden:\n%s", col)
	}
}

func TestShowAllIncludesIdleRepos(t *testing.T) {
	m := withProcs(80, 8,
		[]Project{{Name: "busy", Path: "/p/busy"}, {Name: "idle", Path: "/p/idle"}},
		[]string{"/p/busy"},
	)
	if !strings.Contains(strings.Join(navColumn(m), "\n"), "idle") {
		t.Error("showing all should include idle repos")
	}
}

func TestNarrowedWithNothingRunningExplainsItself(t *testing.T) {
	m := narrowed(withProcs(80, 8, []Project{{Name: "idle", Path: "/p/idle"}}, nil))

	col := strings.Join(navColumn(m), "\n")
	if !strings.Contains(col, "nothing running") {
		t.Errorf("an empty narrowed list should say why:\n%s", col)
	}
	if !strings.Contains(col, "show all") {
		t.Errorf("an empty narrowed list should say how to get back:\n%s", col)
	}
}

func TestAToggleRoundTrips(t *testing.T) {
	// Starts narrowed, as the app does.
	m := narrowed(withProcs(80, 8,
		[]Project{{Name: "busy", Path: "/p/busy"}, {Name: "idle", Path: "/p/idle"}},
		[]string{"/p/busy"},
	))

	m = press(m, ".")
	if !strings.Contains(strings.Join(navColumn(m), "\n"), "idle") {
		t.Error(". should bring every repo into the list")
	}
	m = press(m, ".")
	if strings.Contains(strings.Join(navColumn(m), "\n"), "idle") {
		t.Error(". should narrow back to running repos")
	}
}

func TestNarrowingRescansProcesses(t *testing.T) {
	wide := sized(80, 8)
	wide.showAll, wide.all = true, true

	_, cmd := wide.Update(typed("."))
	if cmd == nil {
		t.Error("narrowing should rescan processes so the list is current")
	}
}

func TestTheKeysListTheToggle(t *testing.T) {
	// Both sides at once: the modal describes the pair rather than tracking
	// which view the next press would show.
	if !strings.Contains(keysOf(), ". everything · running · all") {
		t.Error("the keys should mention the all/running toggle")
	}
}

func TestProcScanFailureKeepsRepoList(t *testing.T) {
	m := withProcs(80, 8, []Project{{Name: "alpha", Path: "/p/alpha"}}, []string{"/p/alpha"})
	next, _ := m.Update(procsMsg{err: errors.New("lsof exploded")})
	if !strings.Contains(strings.Join(navColumn(next.(model)), "\n"), "alpha") {
		t.Error("a failed process scan should not blank the repo list")
	}
}

// --- cursor ---------------------------------------------------------------

func threeRepos(h int) model {
	return withProcList(80, h, []Project{
		{Name: "a", Path: "/p/a"}, {Name: "b", Path: "/p/b"}, {Name: "c", Path: "/p/c"},
	}, nil)
}

func TestCursorStartsOnTheFirstRow(t *testing.T) {
	if c := threeRepos(10).cursor; c != 0 {
		t.Errorf("cursor = %d, want 0", c)
	}
}

func TestCursorMovesWithArrowsAndJK(t *testing.T) {
	for _, key := range []string{"down", "j"} {
		if c := press(threeRepos(10), key).cursor; c != 1 {
			t.Errorf("%q moved cursor to %d, want 1", key, c)
		}
	}
	for _, key := range []string{"up", "k"} {
		m := press(press(threeRepos(10), "down"), key)
		if m.cursor != 0 {
			t.Errorf("%q moved cursor to %d, want 0", key, m.cursor)
		}
	}
}

func TestCursorCyclesAtBothEnds(t *testing.T) {
	m := threeRepos(10)
	if c := press(m, "up").cursor; c != 2 {
		t.Errorf("up from the top went to %d, want 2 (wraps to the end)", c)
	}

	m = press(press(press(m, "down"), "down"), "down")
	if m.cursor != 0 {
		t.Errorf("down past the end went to %d, want 0 (wraps to the top)", m.cursor)
	}
}

func TestCursorWalksProcessesToo(t *testing.T) {
	m := withProcList(80, 12,
		[]Project{{Name: "conn", Path: "/p/conn"}},
		[]Proc{{PID: 10, PPID: 1, Command: "zsh", Dir: "/p/conn"}},
	)
	m = press(m, "down")

	r, ok := m.selected()
	if !ok || r.kind != rowProc || r.node.PID != 10 {
		t.Errorf("selected = %+v, want the process row", r)
	}
	wantRows(t, navColumn(m), []string{"  conn", "    ▸ zsh"})
}

func TestCursorOnEmptyListDoesNotPanic(t *testing.T) {
	m := withProcList(80, 8, nil, nil)
	press(press(m, "down"), "up") // must not panic
}

// --- scrolling ------------------------------------------------------------

func manyRepos(n, h int) model {
	ps := make([]Project, n)
	for i := range ps {
		ps[i] = Project{Name: string(rune('a' + i)), Path: "/p/" + string(rune('a'+i))}
	}
	return withProcList(80, h, ps, nil)
}

func TestScrollFollowsCursorPastTheBottom(t *testing.T) {
	m := manyRepos(10, 5) // 3 body rows under the tabline and its blank
	for range 3 {
		m = press(m, "down")
	}

	if m.offset != 1 {
		t.Errorf("offset = %d, want 1; the window should follow the cursor by one row", m.offset)
	}
	wantRows(t, navColumn(m), []string{"  b", "  c", "▸ d"})
}

func TestScrollKeepsCursorVisibleAfterWrap(t *testing.T) {
	m := press(manyRepos(10, 5), "up") // wraps to the last row

	col := navColumn(m)
	if !strings.HasPrefix(col[len(col)-1], "▸ j") {
		t.Errorf("after wrapping to the end the cursor should be on screen:\n%s", strings.Join(col, "\n"))
	}
}

func TestScrollStopsAtTheLastRow(t *testing.T) {
	m := manyRepos(10, 3)
	for range 9 {
		m = press(m, "down")
	}
	if want := len(m.rows) - m.bodyHeight(); m.offset != want {
		t.Errorf("offset = %d, want %d; the window should not scroll past the end", m.offset, want)
	}
}

// --- detail pane ----------------------------------------------------------

func TestCursorKeepsItsSubjectAcrossRescans(t *testing.T) {
	m := threeRepos(10)
	m = press(press(m, "down"), "down") // on "c"

	// A rescan that reorders the list should keep the cursor on "c".
	next, _ := m.Update(projectsMsg{projects: []Project{
		{Name: "new", Path: "/p/new"},
		{Name: "a", Path: "/p/a"},
		{Name: "b", Path: "/p/b"},
		{Name: "c", Path: "/p/c"},
	}})

	r, _ := next.(model).selected()
	if r.project.Path != "/p/c" {
		t.Errorf("cursor landed on %q after a rescan, want /p/c", r.project.Path)
	}
}

// --- helpers --------------------------------------------------------------

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != 0x1b {
			b.WriteByte(s[i])
			continue
		}

		// An OSC runs to a bell or a string terminator, not to the first
		// letter — its payload is words, and stopping at one would eat what
		// came after it.
		if i+1 < len(s) && s[i+1] == ']' {
			for i += 2; i < len(s); i++ {
				if s[i] == 0x07 {
					break
				}
				if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
					i++
					break
				}
			}
			continue
		}
		for i < len(s) && !isANSITerm(s[i]) {
			i++
		}
	}
	return b.String()
}

func isANSITerm(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// --- collapsing -----------------------------------------------------------

// nestedTree is one repo with zsh → (vim → fmt, zig).
func nestedTree(h int) model {
	return withProcList(80, h,
		[]Project{{Name: "conn", Path: "/p/conn"}},
		[]Proc{
			{PID: 10, PPID: 1, Command: "zsh", Dir: "/p/conn"},
			{PID: 20, PPID: 10, Command: "vim", Dir: "/p/conn"},
			{PID: 30, PPID: 10, Command: "zig", Dir: "/p/conn"},
			{PID: 40, PPID: 20, Command: "fmt", Dir: "/p/conn"},
			{PID: 50, PPID: 20, Command: "lint", Dir: "/p/conn"},
		},
	)
}

func TestSpaceCollapsesAProcessNode(t *testing.T) {
	m := nestedTree(12)
	wantRows(t, navColumn(m), []string{
		"▸ conn", "      zsh", "        vim", "          fmt", "          lint", "        zig",
	})

	// Move onto vim and fold it.
	m = press(press(press(m, "down"), "down"), " ")
	wantRows(t, navColumn(m), []string{
		"  conn", "      zsh", "      ▸ vim +2", "        zig",
	})
}

func TestSpaceCollapsesARepo(t *testing.T) {
	m := press(nestedTree(12), " ")

	col := navColumn(m)
	wantRows(t, col, []string{"▸ conn +5"})
	if len(col) != 1 {
		t.Errorf("a collapsed repo should hide its whole tree, got:\n%s", strings.Join(col, "\n"))
	}
}

func TestSpaceUnfoldsAgain(t *testing.T) {
	m := nestedTree(12)
	folded := press(m, " ")
	unfolded := press(folded, " ")

	if len(navColumn(unfolded)) != len(navColumn(m)) {
		t.Errorf("space should restore the tree:\n%s", strings.Join(navColumn(unfolded), "\n"))
	}
}

func TestCollapsedNodeReportsWhatItHides(t *testing.T) {
	// zsh hides vim, zig and fmt.
	m := press(press(nestedTree(12), "down"), " ")
	wantRows(t, navColumn(m), []string{"  conn", "    ▸ zsh +4"})
}

func TestSpaceOnALeafDoesNothing(t *testing.T) {
	m := nestedTree(12)
	for range 4 {
		m = press(m, "down") // onto "lint 50", a leaf
	}
	before := navColumn(m)

	m = press(m, " ")
	if got := navColumn(m); len(got) != len(before) {
		t.Errorf("space on a leaf changed the tree:\n%s", strings.Join(got, "\n"))
	}
	if strings.Contains(strings.Join(navColumn(m), ""), "+0") {
		t.Error("a leaf should not be marked as hiding anything")
	}
}

func TestSpaceOnARepoWithNoProcessesDoesNothing(t *testing.T) {
	m := withProcList(80, 8, []Project{{Name: "idle", Path: "/p/idle"}}, nil)
	m = press(m, " ")
	wantRows(t, navColumn(m), []string{"▸ idle"})
	if strings.Contains(navColumn(m)[0], "+") {
		t.Error("an idle repo should not be marked as hiding anything")
	}
}

func TestCursorSkipsFoldedChildren(t *testing.T) {
	// Fold zsh, then step down: the next row is the next repo, not a child.
	m := withProcList(80, 12,
		[]Project{{Name: "a", Path: "/p/a"}, {Name: "b", Path: "/p/b"}},
		[]Proc{
			{PID: 10, PPID: 1, Command: "zsh", Dir: "/p/a"},
			{PID: 20, PPID: 10, Command: "vim", Dir: "/p/a"},
		},
	)
	m = press(press(m, "down"), " ") // on zsh, folded
	m = press(m, "down")

	r, _ := m.selected()
	if r.kind != rowProject || r.project.Name != "b" {
		t.Errorf("cursor landed on %+v, want the next repo", r)
	}
}

func TestCollapseSurvivesARescan(t *testing.T) {
	m := press(nestedTree(12), " ") // repo folded
	next, _ := m.Update(procsMsg{procs: m.procs})

	if got := navColumn(next.(model)); len(got) != 1 {
		t.Errorf("a rescan should not unfold the tree:\n%s", strings.Join(got, "\n"))
	}
}

func TestTheKeysListTheFolds(t *testing.T) {
	// Both directions at once: the modal describes the pair rather than
	// tracking which way the next press would go.
	if !strings.Contains(keysOf(), "space · - fold · unfold all") {
		t.Error("the keys should mention folding")
	}
}

func TestCollapsedRowStaysInItsColumn(t *testing.T) {
	m := withProcList(80, 12,
		[]Project{{Name: strings.Repeat("x", 60), Path: "/p/x"}},
		[]Proc{{PID: 10, PPID: 1, Command: "zsh", Dir: "/p/x"}},
	)
	m = press(m, " ")

	if got := len([]rune(navColumn(m)[0])); got > m.width {
		t.Errorf("collapsed row is %d columns, want at most %d: %q", got, m.width, navColumn(m)[0])
	}
}

// --- killing --------------------------------------------------------------

// keysOf is the keys page flattened to one string, so a test can ask
// whether a key is listed.
func keysOf() string {
	return strings.Join(strings.Fields(stripANSI(strings.Join(keysPage(), "\n"))), " ")
}

// footer is what the navigator has the status line read — its mode and
// its message — flattened to one plain string so a test can ask whether
// something is in it.
func footer(m model) string {
	t := m.statusLine()
	return strings.Join(strings.Fields(stripTmux(t.mode)+" "+stripTmux(t.msg)), " ")
}

// stripTmux takes tmux's styling out of a status line's text.
func stripTmux(s string) string {
	for {
		i := strings.Index(s, "#[")
		if i < 0 {
			return strings.ReplaceAll(s, "##", "#")
		}
		j := strings.Index(s[i:], "]")
		if j < 0 {
			return s
		}
		s = s[:i] + s[i+j+1:]
	}
}

func TestXAsksBeforeKilling(t *testing.T) {
	m := press(nestedTree(12), "down") // onto zsh 10
	m = press(m, "x")

	// The preview shows the whole tree; x at it takes the head alone.
	if m.pendingKill == nil || len(m.pendingKill.head) != 1 || m.pendingKill.head[0].PID != 10 {
		t.Fatalf("pendingKill = %v, want the selected process as the head", m.pendingKill)
	}
	if f := footer(m); !strings.Contains(f, "CONFIRM") || !strings.Contains(f, "zsh") {
		t.Errorf("footer = %q, want it to ask before killing", f)
	}
	if preview := strings.Join(bodyRows(m), " "); !strings.Contains(preview, "x the head alone") {
		t.Errorf("preview = %q, want the head offered alone", preview)
	}
}

func TestXDoesNotKillOnItsOwn(t *testing.T) {
	m := press(nestedTree(12), "down")
	_, cmd := m.Update(typed("x"))
	if cmd != nil {
		t.Error("the first x should only arm the confirmation, not signal anything")
	}
}

func TestConfirmingRunsTheKill(t *testing.T) {
	m := press(press(nestedTree(12), "down"), "x")

	for _, key := range []string{"x", "y", "enter"} {
		var msg tea.KeyPressMsg
		if key == "enter" {
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		} else {
			msg = typed(key)
		}
		next, cmd := m.Update(msg)
		if cmd == nil {
			t.Errorf("%q should confirm the kill", key)
		}
		if next.(model).pendingKill != nil {
			t.Errorf("%q should clear the pending kill", key)
		}
	}
}

func TestAnyOtherKeyCancelsTheKill(t *testing.T) {
	armed := press(press(nestedTree(12), "down"), "x")

	for _, key := range []string{"s", "esc", "j", "a", " "} {
		var msg tea.KeyPressMsg
		switch key {
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		default:
			msg = typed(key)
		}
		next, cmd := armed.Update(msg)
		m := next.(model)

		if m.pendingKill != nil {
			t.Errorf("%q left the kill armed", key)
		}
		if cmd != nil {
			t.Errorf("%q signalled something instead of cancelling", key)
		}
		if !strings.Contains(footer(m), "kept") {
			t.Errorf("%q should say the kill was called off, footer = %q", key, footer(m))
		}
	}
}

func TestCancellingKeysDoNotAlsoActOnTheList(t *testing.T) {
	armed := press(press(nestedTree(12), "down"), "x")
	cursorWas := armed.cursor

	next, _ := armed.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if next.(model).cursor != cursorWas {
		t.Error("the key that cancels a kill should not also move the cursor")
	}
}

func TestQuitStillWorksWhileArmed(t *testing.T) {
	// Cancelling is the priority, but the user must not be trapped: the next
	// key after cancelling quits as usual.
	armed := press(press(nestedTree(12), "down"), "x")
	cancelled, asked := pipeServer(t, press(armed, "q"))
	press(cancelled, "q")
	if got := askedForKind(t, asked, kindLeave); got.Kind != kindLeave {
		t.Error("q should leave once the confirmation is cleared")
	}
}

func TestKillFailureIsReported(t *testing.T) {
	m := nestedTree(12)
	next, cmd := m.Update(killFailed("zsh", 10, errors.New("not permitted")))

	if f := footer(next.(model)); !strings.Contains(f, "not permitted") {
		t.Errorf("footer = %q, want the failure reported", f)
	}
	if cmd != nil {
		t.Error("a failed kill should not schedule a rescan")
	}
}

func TestKillSuccessStartsTheMarker(t *testing.T) {
	m := nestedTree(12)
	next, cmd := m.Update(killed("zsh", 10))
	got := next.(model)

	if f := footer(got); !strings.Contains(f, "SIGTERM") {
		t.Errorf("footer = %q, want it to report the signal", f)
	}
	if cmd == nil || !got.spinning {
		t.Error("a successful kill should start the frame chain that watches for the exit")
	}
	if _, dying := got.dying[10]; !dying {
		t.Error("a successful kill should mark the process")
	}
}

func TestStatusClearsOnTheNextKey(t *testing.T) {
	m := press(nestedTree(12), "r") // leaves a status about the missing server
	if !strings.Contains(footer(m), "no server") {
		t.Fatal("expected a status to clear")
	}

	m = press(m, "down")
	if strings.Contains(footer(m), "no server") {
		t.Errorf("status should clear once the cursor moves, footer = %q", footer(m))
	}
}

func TestTheKeysListTheKill(t *testing.T) {
	if !strings.Contains(keysOf(), "x · X preview a kill") {
		t.Error("the keys should mention the kill key")
	}
}

// --- automatic refresh ----------------------------------------------------

func TestTickRefreshesAndReschedulesItself(t *testing.T) {
	m := nestedTree(12)
	next, cmd := m.Update(tickMsg{})

	if cmd == nil {
		t.Fatal("a tick should rescan and schedule the next tick")
	}
	if next.(model).ticks != 1 {
		t.Errorf("ticks = %d, want 1", next.(model).ticks)
	}
}

func TestTickKeepsTicking(t *testing.T) {
	var m tea.Model = nestedTree(12)
	for i := range 3 {
		var cmd tea.Cmd
		m, cmd = m.Update(tickMsg{})
		if cmd == nil {
			t.Fatalf("tick %d did not schedule a successor", i)
		}
	}
	if got := m.(model).ticks; got != 3 {
		t.Errorf("ticks = %d, want 3", got)
	}
}

func TestProjectsAreRescannedLessOftenThanProcesses(t *testing.T) {
	m := nestedTree(12)
	m.ticks = projectEvery - 1

	next, _ := m.Update(tickMsg{})
	if next.(model).ticks%projectEvery != 0 {
		t.Fatalf("expected this tick to be a project-scanning one, ticks = %d", next.(model).ticks)
	}
}

func TestKilledProcessDisappearsOnTheNextScan(t *testing.T) {
	m := nestedTree(12)
	if len(m.rows) != 6 {
		t.Fatalf("rows = %d, want 6 to start", len(m.rows))
	}

	// The same scan without pid 40, as if it had exited.
	next, _ := m.Update(procsMsg{procs: []Proc{
		{PID: 10, PPID: 1, Command: "zsh", Dir: "/p/conn"},
		{PID: 20, PPID: 10, Command: "vim", Dir: "/p/conn"},
		{PID: 30, PPID: 10, Command: "zig", Dir: "/p/conn"},
	}})

	col := strings.Join(navColumn(next.(model)), "\n")
	if strings.Contains(col, "fmt") {
		t.Errorf("an exited process should leave the tree:\n%s", col)
	}
}

func TestCursorHoldsItsPlaceWhenTheSelectionExits(t *testing.T) {
	m := nestedTree(12)
	m = press(press(press(m, "down"), "down"), "down") // onto fmt 40, index 3

	if r, _ := m.selected(); r.node == nil || r.node.PID != 40 {
		t.Fatalf("setup: selected %+v, want pid 40", r)
	}

	next, _ := m.Update(procsMsg{procs: []Proc{
		{PID: 10, PPID: 1, Command: "zsh", Dir: "/p/conn"},
		{PID: 20, PPID: 10, Command: "vim", Dir: "/p/conn"},
		{PID: 30, PPID: 10, Command: "zig", Dir: "/p/conn"},
	}})

	if c := next.(model).cursor; c == 0 {
		t.Error("the cursor jumped to the top when its process exited; it should hold its place")
	}
}

func TestCursorClampsWhenTheListShrinksPastIt(t *testing.T) {
	m := nestedTree(12)
	for range 4 {
		m = press(m, "down") // last row
	}

	next, _ := m.Update(procsMsg{procs: nil}) // every process gone
	got := next.(model)

	if got.cursor >= len(got.rows) {
		t.Errorf("cursor %d is past the end of %d rows", got.cursor, len(got.rows))
	}
}

func TestRefreshPreservesCollapsedNodes(t *testing.T) {
	m := press(nestedTree(12), " ") // repo folded
	next, _ := m.Update(tickMsg{})
	next, _ = next.Update(procsMsg{procs: m.procs})

	if got := navColumn(next.(model)); len(got) != 1 {
		t.Errorf("an automatic refresh should not unfold the tree:\n%s", strings.Join(got, "\n"))
	}
}

func TestRefreshDoesNotClearAStatusMessage(t *testing.T) {
	m := nestedTree(12)
	next, _ := m.Update(killed("zsh", 10))
	next, _ = next.Update(tickMsg{})

	if !strings.Contains(footer(next.(model)), "SIGTERM") {
		t.Errorf("a background refresh should not wipe the last report, footer = %q", footer(next.(model)))
	}
}

func TestRefreshDoesNotDisturbAPendingKill(t *testing.T) {
	armed := press(press(nestedTree(12), "down"), "x")
	next, _ := armed.Update(tickMsg{})

	if next.(model).pendingKill == nil {
		t.Error("a background refresh should not cancel a pending confirmation")
	}
	if !strings.Contains(footer(next.(model)), "CONFIRM") {
		t.Error("the confirmation should stay on screen through a refresh")
	}
}

func TestASignalledProcessKeepsItsRowAndIsMarked(t *testing.T) {
	m := nestedTree(12)
	next, _ := m.Update(killed("fmt", 40))
	got := next.(model)

	row := navColumn(got)[3]
	if !strings.Contains(row, "fmt") {
		t.Fatalf("row = %q, want the process still listed until it is seen gone", row)
	}
	if !strings.Contains(row, spinFrames[got.frame%len(spinFrames)]) {
		t.Errorf("row = %q, want a marker on the signalled process", row)
	}
}

func TestOnlyTheSignalledProcessIsMarked(t *testing.T) {
	next, _ := nestedTree(12).Update(killed("fmt", 40))

	for i, row := range navColumn(next.(model)) {
		if strings.Contains(row, "fmt") {
			continue
		}
		for _, f := range spinFrames {
			if strings.Contains(row, f) {
				t.Errorf("row %d = %q carries a marker but was not signalled", i, row)
			}
		}
	}
}

func TestAFailedKillMarksNothing(t *testing.T) {
	next, _ := nestedTree(12).Update(killFailed("fmt", 40, errors.New("not permitted")))

	if got := next.(model); len(got.dying) != 0 {
		t.Errorf("dying = %v, want nothing marked when the signal did not land", got.dying)
	}
}

func TestTheMarkerAdvances(t *testing.T) {
	m := nestedTree(12)
	next, _ := m.Update(killed("fmt", 40))
	first := navColumn(next.(model))[3]

	next, _ = next.(model).Update(spinMsg{})
	if second := navColumn(next.(model))[3]; second == first {
		t.Errorf("the marker did not advance: %q twice", second)
	}
}

func TestTheMarkerGoesWhenTheProcessDoes(t *testing.T) {
	m := nestedTree(12)
	next, _ := m.Update(killed("fmt", 40))

	// The next scan without pid 40, as if it had acted on the signal.
	next, _ = next.(model).Update(procsMsg{procs: []Proc{
		{PID: 10, PPID: 1, Command: "zsh", Dir: "/p/conn"},
		{PID: 20, PPID: 10, Command: "vim", Dir: "/p/conn"},
		{PID: 30, PPID: 10, Command: "zig", Dir: "/p/conn"},
	}})
	got := next.(model)

	if col := strings.Join(navColumn(got), "\n"); strings.Contains(col, "fmt") {
		t.Errorf("an exited process should leave the tree:\n%s", col)
	}
	if len(got.dying) != 0 {
		t.Errorf("dying = %v, want it dropped once the process is gone", got.dying)
	}
}

func TestAProcessDyingInAFoldedSubtreeIsNotForgotten(t *testing.T) {
	// fmt 40 hangs under vim 20. Folding vim takes its row away, which is not
	// the same as the process having exited.
	m := press(press(nestedTree(12), "down"), "down") // onto vim 20
	next, _ := m.Update(killed("fmt", 40))
	folded := press(next.(model), " ")

	if _, dying := folded.dying[40]; !dying {
		t.Error("folding a subtree should not count its processes as gone")
	}
}

func TestTheFrameChainRescansButNotEveryFrame(t *testing.T) {
	// The chain always schedules the next frame; on a rescanning frame it also
	// asks for a scan, and the scan eventually finds the process gone. A batch
	// of two is that second command; a lone command is the next frame alone.
	m := nestedTree(12)
	next, _ := m.Update(killed("fmt", 40))
	got := next.(model)

	scans := 0
	for i := range 2 * rescanFrames {
		n, cmd := got.Update(spinMsg{})
		got = n.(model)
		if cmd == nil {
			t.Fatalf("frame %d ended the chain while a process was still dying", i)
		}
		if got.frame%rescanFrames != 0 {
			// Only the next frame, which is a real timer: running it would
			// make the test wait out the frame rate.
			continue
		}
		scans++
		batch, ok := cmd().(tea.BatchMsg)
		if !ok {
			t.Errorf("frame %d scheduled one command, want the next frame and a scan", got.frame)
			continue
		}
		if len(batch) != 2 {
			t.Errorf("frame %d batched %d commands, want the next frame and a scan", got.frame, len(batch))
		}
		// The scan answers well before the next rescanning frame; without the
		// answer, that frame would rightly decline to stack a second scan
		// behind a first still out.
		n, _ = got.Update(procsMsg{procs: got.procs})
		got = n.(model)
	}

	if scans != 2 {
		t.Errorf("scanned %d times in %d frames, want one every %d", scans, 2*rescanFrames, rescanFrames)
	}
}

func TestASecondKillJoinsTheRunningChain(t *testing.T) {
	m := nestedTree(12)
	next, cmd := m.Update(killed("fmt", 40))
	if cmd == nil {
		t.Fatal("the first kill should start the frame chain")
	}

	next, cmd = next.(model).Update(killed("zig", 30))
	if cmd != nil {
		t.Error("a second kill started its own chain, doubling the frame rate")
	}
	if got := next.(model); len(got.dying) != 2 {
		t.Errorf("dying = %v, want both signalled processes marked", got.dying)
	}
}

func TestTheChainStopsWhenNothingIsDying(t *testing.T) {
	m := nestedTree(12)
	m.spinning = true

	next, cmd := m.Update(spinMsg{})
	if cmd != nil {
		t.Error("the frame chain should stop once nothing is marked")
	}
	if next.(model).spinning {
		t.Error("spinning should be cleared so the next kill can start a chain")
	}
}

func TestAProcessThatIgnoresTheSignalIsGivenUpOn(t *testing.T) {
	m := nestedTree(12)
	next, _ := m.Update(killed("fmt", 40))
	got := next.(model)

	for i := 0; i <= killLinger; i++ {
		n, _ := got.Update(spinMsg{})
		got = n.(model)
	}

	if _, dying := got.dying[40]; dying {
		t.Error("a process that never acted on SIGTERM is still marked as dying")
	}
	if f := footer(got); !strings.Contains(f, "fmt 40 did not exit") {
		t.Errorf("footer = %q, want it to say the process did not go", f)
	}
	if col := strings.Join(navColumn(got), "\n"); !strings.Contains(col, "fmt") {
		t.Errorf("the process is still running and should still be listed:\n%s", col)
	}
}

// --- killing a whole tree ------------------------------------------------

func TestXKillsTheSubtreeParentsFirst(t *testing.T) {
	// zsh 10 holds vim 20 (holding fmt 40) and zig 30.
	m := press(press(nestedTree(12), "down"), "X") // onto zsh 10

	if got, want := targets(m.pendingKill), []int{10, 20, 40, 50, 30}; !slices.Equal(got, want) {
		t.Errorf("targets = %v, want the subtree parents first %v", got, want)
	}
	if f := footer(m); !strings.Contains(f, "5 processes") {
		t.Errorf("footer = %q, want it to say how much it is about to kill", f)
	}
	if preview := strings.Join(bodyRows(m), " "); !strings.Contains(preview, "X kills all 5") {
		t.Errorf("preview = %q, want the confirm line to count the tree", preview)
	}
}

func TestLowercaseXTakesOnlyTheOneProcess(t *testing.T) {
	m := press(press(nestedTree(12), "down"), "x") // onto zsh 10

	// The preview is the tree's; x at it is the head alone.
	if got := pids(m.pendingKill.head); !slices.Equal(got, []int{10}) {
		t.Errorf("head = %v, want only the selected process", got)
	}
	if got := targets(m.pendingKill.headOnly()); !slices.Equal(got, []int{10}) {
		t.Errorf("targets = %v, want only the selected process", got)
	}
}

func TestXOnALeafReadsAsAPlainKill(t *testing.T) {
	m := nestedTree(12)
	for range 3 {
		m = press(m, "down") // onto fmt 40, which has nothing below it
	}
	m = press(m, "X")

	if got := targets(m.pendingKill); !slices.Equal(got, []int{40}) {
		t.Errorf("targets = %v, want just the leaf", got)
	}
	if preview := strings.Join(bodyRows(m), " "); !strings.Contains(preview, "x kills fmt") || strings.Contains(preview, "head alone") {
		t.Errorf("preview = %q, want a plain kill with no head to offer", preview)
	}
}

func TestXOnARepoTakesEverythingInIt(t *testing.T) {
	m := press(nestedTree(12), "X") // cursor is on the repo row

	if got, want := targets(m.pendingKill), []int{10, 20, 40, 50, 30}; !slices.Equal(got, want) {
		t.Errorf("targets = %v, want every process in the repo %v", got, want)
	}
	if f := footer(m); !strings.Contains(f, "5 processes in conn") {
		t.Errorf("footer = %q, want it to say what it is about to clear out", f)
	}
}

func TestLowercaseXOnARepoTakesEverythingInItToo(t *testing.T) {
	// On a repository both widths are the same width. A narrow kill that
	// stopped only what the plan had started read as x ignoring half of what
	// was on screen.
	m := press(nestedTree(12), "x") // cursor is on the repo row

	if got, want := targets(m.pendingKill), []int{10, 20, 40, 50, 30}; !slices.Equal(got, want) {
		t.Errorf("targets = %v, want every process in the repo %v", got, want)
	}
}

func TestXOnAnIdleRepoSaysThereIsNothingToKill(t *testing.T) {
	for _, key := range []string{"x", "X"} {
		m := withProcList(80, 12, []Project{{Name: "conn", Path: "/p/conn"}}, nil)
		m = press(m, key)

		if m.pendingKill != nil {
			t.Errorf("%q on an idle repository should not arm a kill", key)
		}
		if f := footer(m); !strings.Contains(f, "nothing running in conn") {
			t.Errorf("%q footer = %q, want it to explain why nothing happened", key, f)
		}
	}
}

func TestXCollapsedStillKillsWhatIsFoldedAway(t *testing.T) {
	// Folding is a display state; it should not narrow what a kill covers.
	m := press(press(nestedTree(12), "down"), " ") // fold zsh 10
	m = press(m, "X")

	if got, want := targets(m.pendingKill), []int{10, 20, 40, 50, 30}; !slices.Equal(got, want) {
		t.Errorf("targets = %v, want the folded subtree too %v", got, want)
	}
}

func TestConfirmingATreeKillSignalsEveryProcess(t *testing.T) {
	m := press(press(nestedTree(12), "down"), "X")
	next, cmd := m.Update(typed("X"))
	if cmd == nil {
		t.Fatal("X should confirm a tree kill it armed")
	}
	if next.(model).pendingKill != nil {
		t.Error("confirming should clear the pending kill")
	}
}

func TestEveryProcessInATreeKillIsMarked(t *testing.T) {
	m := nestedTree(12)
	next, _ := m.Update(killedMsg{
		subject: "zsh 10 and 3 under it",
		results: []killResult{
			{command: "zsh", pid: 10}, {command: "vim", pid: 20},
			{command: "fmt", pid: 40}, {command: "zig", pid: 30},
		},
	})
	got := next.(model)

	for _, pid := range []int{10, 20, 30, 40} {
		if _, dying := got.dying[pid]; !dying {
			t.Errorf("pid %d was signalled but is not marked", pid)
		}
	}
	if f := footer(got); !strings.Contains(f, "sent SIGTERM to zsh 10 and 3 under it") {
		t.Errorf("footer = %q, want the whole kill reported once", f)
	}
}

func TestAPartlyRefusedTreeKillSaysSo(t *testing.T) {
	m := nestedTree(12)
	next, _ := m.Update(killedMsg{
		subject: "zsh 10 and 1 under it",
		results: []killResult{
			{command: "zsh", pid: 10},
			{command: "vim", pid: 20, err: errors.New("not permitted")},
		},
	})
	got := next.(model)

	f := footer(got)
	if !strings.Contains(f, "1 could not be killed") || !strings.Contains(f, "not permitted") {
		t.Errorf("footer = %q, want the survivors accounted for", f)
	}
	if _, dying := got.dying[20]; dying {
		t.Error("a process that refused the signal should not be marked as dying")
	}
	if _, dying := got.dying[10]; !dying {
		t.Error("the processes that did take the signal should still be marked")
	}
}

func TestAWhollyRefusedTreeKillReportsEachReasonOnce(t *testing.T) {
	m := nestedTree(12)
	next, _ := m.Update(killedMsg{
		subject: "3 processes in conn",
		results: []killResult{
			{command: "zsh", pid: 10, err: errors.New("not permitted")},
			{command: "vim", pid: 20, err: errors.New("not permitted")},
			{command: "zig", pid: 30, err: errors.New("already gone")},
		},
	})
	got := next.(model)

	if f := footer(got); !strings.Contains(f, "could not kill 3 processes in conn: already gone, not permitted") {
		t.Errorf("footer = %q, want each reason named once", f)
	}
	if got.spinning {
		t.Error("nothing was signalled, so nothing should be spinning")
	}
}

func TestTheKeysListTheTreeKill(t *testing.T) {
	if f := keysOf(); !strings.Contains(f, "preview a kill · of the tree") {
		t.Errorf("keys = %q, want the tree kill listed", f)
	}
}

// --- claude instances -----------------------------------------------------

// withClaude builds a repo holding one process, plus the sessions Claude Code
// would have advertised for it.
func withClaude(command string, sessions map[int]claudeSession) model {
	m := withProcList(96, 12,
		[]Project{{Name: "conn", Path: "/p/conn"}},
		[]Proc{{PID: 700, PPID: 1, Command: command, Dir: "/p/conn"}})
	m.agents = asAgents(sessions)
	return m
}

func TestABusyClaudeTurns(t *testing.T) {
	// The difference between an instance thinking and one waiting on you is
	// the thing worth crossing the room for, so it moves.
	m := withClaude("claude", map[int]claudeSession{
		700: {PID: 700, Name: "conn-1f", Status: busyStatus},
	})

	first := navColumn(m)[1]
	if !strings.Contains(first, "claude "+spinFrames[m.frame%len(spinFrames)]) {
		t.Fatalf("row = %q, want a turning marker on a working instance", first)
	}

	next, _ := m.Update(spinMsg{})
	if second := navColumn(next.(model))[1]; second == first {
		t.Errorf("the marker did not turn: %q twice", second)
	}
}

func TestAClaudeRowShowsTheModelItAdvertises(t *testing.T) {
	// The invocation names no model — a claude picks one up from the
	// transcript, not the command line — so the row shows what the instance
	// advertises, trimmed to what you would call it.
	m := withClaude("claude", map[int]claudeSession{
		700: {PID: 700, Name: "conn-1f", Status: busyStatus, Model: "claude-opus-4-8"},
	})
	if row := navColumn(m)[1]; !strings.Contains(row, "claude · opus-4-8") {
		t.Errorf("row = %q, want the advertised model beside the kind", row)
	}
}

func TestAWorkingInstanceSetsTheMarkersTurning(t *testing.T) {
	m := withClaude("claude", nil)
	if m.spinning {
		t.Fatal("setup: nothing should be turning yet")
	}

	next, cmd := m.Update(agentsMsg{agents: asAgents(map[int]claudeSession{
		700: {PID: 700, Status: busyStatus},
	})})
	if cmd == nil || !next.(model).spinning {
		t.Error("an instance that has started working should set the markers turning")
	}
}

func TestTheMarkersStopWhenNothingIsWorking(t *testing.T) {
	m := withClaude("claude", map[int]claudeSession{700: {PID: 700, Status: busyStatus}})
	m.spinning = true

	next, _ := m.Update(agentsMsg{agents: asAgents(map[int]claudeSession{
		700: {PID: 700, Status: "idle"},
	})})
	m = next.(model)

	next, cmd := m.Update(spinMsg{})
	if cmd != nil {
		t.Error("the frame chain should stop once nothing is working")
	}
	if next.(model).spinning {
		t.Error("spinning should be cleared so the next one can start it again")
	}
}

func TestATurningMarkerDoesNotChaseTheProcessList(t *testing.T) {
	// A kill needs the list re-read to notice the exit. A working instance is
	// a session file the ordinary refresh already picks up, and an lsof sweep
	// every few frames for the length of a Claude turn is not free.
	m := withClaude("claude", map[int]claudeSession{700: {PID: 700, Status: busyStatus}})
	m.spinning = true
	m.frame = rescanFrames - 1

	_, cmd := m.Update(spinMsg{})
	if cmd == nil {
		t.Fatal("the chain should keep running")
	}
	if _, batched := cmd().(tea.BatchMsg); batched {
		t.Error("a turning marker should schedule the next frame and nothing else")
	}
}

func TestAFinishedTurnLightsItsRow(t *testing.T) {
	// Done-and-waiting is the state that most wants to be seen: an idle
	// instance whose own record says it has answered since it started
	// gets the filled marker, and the row itself takes the attention
	// color rather than leaving a stopped spinner to whisper it. The
	// instance says so, not this window's memory of seeing it busy: a
	// navigator started after the turn marks it the same.
	m := withClaude("claude", map[int]claudeSession{
		700: {PID: 700, Name: "conn-1f", Status: "idle", Finished: true},
	})

	row := navColumn(m)[1]
	if !strings.Contains(row, "claude ●") {
		t.Errorf("row = %q, want a filled marker on a finished turn", row)
	}
	styled := false
	for raw := range strings.SplitSeq(m.View().Content, "\n") {
		if strings.Contains(raw, attnStyle.Render(" "+glyphOn)) {
			styled = true
		}
	}
	if !styled {
		t.Error("no row renders the mark in the attention style")
	}
}

func TestABlockedInstanceHoldsTheBrightDiamond(t *testing.T) {
	// Stopped mid-turn on a specific prompt is brighter than
	// done-and-waiting: that answer resumes work already in flight. It is
	// owed even from an instance this window never saw working — the prompt
	// exists either way — so no working turn precedes it here.
	m := withClaude("claude", map[int]claudeSession{
		700: {PID: 700, Name: "conn-1f", Status: waitingStatus, WaitingFor: "permission prompt"},
	})

	row := navColumn(m)[1]
	if !strings.Contains(row, "claude ◆") {
		t.Errorf("row = %q, want a diamond on a blocked instance", row)
	}
	styled := false
	for raw := range strings.SplitSeq(m.View().Content, "\n") {
		if strings.Contains(raw, blockedStyle.Render(" "+glyphAsk)) {
			styled = true
		}
	}
	if !styled {
		t.Error("no row renders the mark in the blocked style")
	}

	// The chord counts it among the waiting, unlike an instance merely idle
	// since launch.
	if got := press(m, "tab").status; got == "nothing needs you" {
		t.Error("the chord passed over a blocked instance")
	}
}

func TestAnInstanceIdleSinceLaunchStaysQuiet(t *testing.T) {
	// A fresh instance at its prompt is waiting, but it has finished nothing
	// and is owed nothing: hollow marker, no highlight, and the chord passes
	// it by.
	m := withClaude("claude", map[int]claudeSession{
		700: {PID: 700, Name: "conn-1f", Status: "idle"},
	})

	row := navColumn(m)[1]
	if !strings.Contains(row, "claude ○") {
		t.Errorf("row = %q, want a hollow marker on an instance idle since launch", row)
	}

	if got := press(m, "tab").status; got != "nothing needs you" {
		t.Errorf("status = %q, want the jump to find nothing owed", got)
	}
}

func TestARecycledPidDoesNotInheritAFinishedTurn(t *testing.T) {
	// A finished turn is the instance's own account, so whatever takes the
	// number next — idle, and by its own record having answered nothing —
	// does not light up on someone else's turn.
	m := withClaude("claude", nil)
	next, _ := m.Update(agentsMsg{agents: asAgents(map[int]claudeSession{
		700: {PID: 700, Status: busyStatus},
	})})
	next, _ = next.(model).Update(agentsMsg{agents: map[int]agent{}})
	next, _ = next.(model).Update(agentsMsg{agents: asAgents(map[int]claudeSession{
		700: {PID: 700, Status: "idle"},
	})})
	m = next.(model)

	if row := navColumn(m)[1]; !strings.Contains(row, "claude ○") {
		t.Errorf("row = %q, want the new holder of the pid quiet", row)
	}
}

func TestPrefixEnterCyclesTheWaitingAgents(t *testing.T) {
	// tab is the jump at the list: it goes to the next agent waiting on
	// its user, and pressing it again continues around them in turn.
	m := withProcList(96, 14,
		[]Project{{Name: "a", Path: "/p/a"}, {Name: "b", Path: "/p/b"}},
		[]Proc{
			{PID: 700, PPID: 1, Command: "claude", Dir: "/p/a"},
			{PID: 701, PPID: 1, Command: "claude", Dir: "/p/b"},
		})
	m.agents = asAgents(map[int]claudeSession{
		700: {PID: 700, Status: "idle", StatusFor: time.Minute, Finished: true},
		701: {PID: 701, Status: "idle", StatusFor: time.Hour, Finished: true},
	})

	m = press(m, "tab")
	if r, ok := m.selected(); !ok || r.kind != rowProc || r.node.PID != 700 {
		t.Errorf("cursor on %+v, want the first waiting agent, pid 700", r)
	}

	m = press(m, "tab")
	if r, ok := m.selected(); !ok || r.kind != rowProc || r.node.PID != 701 {
		t.Errorf("cursor on %+v, want the next waiting agent, pid 701", r)
	}

	m = press(m, "tab")
	if r, ok := m.selected(); !ok || r.kind != rowProc || r.node.PID != 700 {
		t.Errorf("cursor on %+v, want the cycle to wrap back to pid 700", r)
	}
}

// message is an ask of the server, reconstructed from the tmux commands the
// session runs, so the tests assert intent rather than plumbing.
type message struct {
	Kind string
	PID  int
	Dir  string
	Run  string
	Name string
}

const (
	kindOpen     = "open"
	kindShow     = "show"  // a shell moved beside the navigator
	kindPark     = "park"  // the shown shell moved back to a window of its own
	kindFocus    = "focus" // focus taken to a pane
	kindLeave    = "leave"
	kindClose    = "close"    // a held shell's pane killed
	kindDress    = "dress"    // a pane named for the title
	kindHelp     = "help"     // the keys popup asked for
	kindMode     = "mode"     // the status line's mode chip said
	kindMsg      = "msg"      // the status line's message said
	kindNeed     = "need"     // the status line's count of what needs you said
	kindSummary  = "summary"  // a run's word kept on its pane
	kindRecorded = "recorded" // an ending marked as on the record on its pane
	kindAgent    = "agent"    // the kind of agent a starts, told to the server
)

// pipeServer gives a model a session whose asks land on the returned
// channel. The session runs over a fake tmux: panes are seeded from the
// terms the test built, commands are answered plausibly, and the control
// side never starts, so no real server is touched.
func pipeServer(t *testing.T, m model) (model, chan message) {
	t.Helper()
	s, asked := recordingSession(m.terms)
	m.server = s
	return m, asked
}

func recordingSession(terms map[int]*remoteTerm) (*session, chan message) {
	s := newSession()
	s.closed = true // never attach a control client or probe for a server
	s.nav = "%0"    // the navigator's own pane, which the fake's home holds

	asked := make(chan message, 16)
	var mu sync.Mutex
	var opening *message
	var listing []string
	shown := 0 // the pid beside the navigator, if any
	nextPID := 900
	agent := "" // the kind a starts, as the server holds it; empty until told

	for pid, rt := range terms {
		id := "%" + strconv.Itoa(pid)
		s.panes[pid] = &pane{id: id, pid: pid, dir: rt.dir, name: rt.name}
		s.byPane[id] = pid
		listing = append(listing, fmt.Sprintf("%s\t%d\t%s\t%s\t%s\t\t\tshell", id, pid, rt.dir, rt.name, rt.dir))
	}

	// The fake's panes and windows are numbered by pid, so a target names
	// its pid whichever it is — after -t, or after -s for the pane a move
	// moves.
	after := func(args []string, flag string) int {
		for i, a := range args {
			if a == flag && i+1 < len(args) && (strings.HasPrefix(args[i+1], "%") || strings.HasPrefix(args[i+1], "@")) {
				pid, _ := strconv.Atoi(args[i+1][1:])
				return pid
			}
		}
		return 0
	}
	target := func(args []string) int { return after(args, "-t") }
	has := func(args []string, word string) bool {
		for _, a := range args {
			if a == word {
				return true
			}
		}
		return false
	}

	// One call can carry several commands, ; between them, and each is
	// answered on its own; the call returns the last one's answer.
	var one func(args []string) (string, error)
	s.run = func(args ...string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		out, err := "", error(nil)
		for len(args) > 0 {
			end := len(args)
			for i, a := range args {
				if a == ";" {
					end = i
					break
				}
			}
			out, err = one(args[:end])
			if end == len(args) {
				break
			}
			args = args[end+1:]
		}
		return out, err
	}
	one = func(args []string) (string, error) {
		switch args[0] {
		case "has-session":
			return "", nil
		case "new-window":
			// An open: the directory rides behind -c, the command — when
			// there is one — is the last argument, in the shell wrapper.
			dir, run := "", ""
			for i, a := range args {
				if a == "-c" && i+1 < len(args) {
					dir = args[i+1]
				}
			}
			if last := args[len(args)-1]; strings.Contains(last, "; exec ") {
				run, _, _ = strings.Cut(last, "; exec ")
				// The exit's recording rides between the command and
				// the shell, and is not the command.
				run = strings.TrimSuffix(run, recordExit())
			}
			nextPID++
			opening = &message{Kind: kindOpen, PID: nextPID, Dir: dir, Run: run}
			id := "%" + strconv.Itoa(nextPID)
			listing = append(listing, fmt.Sprintf("%s\t%d\t%s\t\t%s\t\t\tshell", id, nextPID, dir, dir))
			return fmt.Sprintf("%s %d", id, nextPID), nil
		case "set":
			switch {
			case has(args, "@conn_title"):
				asked <- message{Kind: kindDress, PID: target(args), Name: args[len(args)-1]}
			case has(args, "@conn_mode"):
				asked <- message{Kind: kindMode, Name: args[len(args)-1]}
			case has(args, "@conn_msg"):
				asked <- message{Kind: kindMsg, Name: args[len(args)-1]}
			case has(args, "@conn_summary"):
				asked <- message{Kind: kindSummary, PID: target(args), Name: args[len(args)-1]}
			case has(args, "@conn_recorded"):
				asked <- message{Kind: kindRecorded, PID: target(args)}
			case has(args, "@conn_need"):
				// Said with every mode and message; the fake passes on
				// only a count, or a test typing a query would fill
				// the channel with nothing.
				if need := args[len(args)-1]; need != "" {
					asked <- message{Kind: kindNeed, Name: need}
				}
			case has(args, agentOption):
				agent = args[len(args)-1]
				asked <- message{Kind: kindAgent, Name: agent}
			case has(args, "@conn_name") && opening != nil:
				// A pane's name lands right after its open, making the
				// recorded request whole.
				opening.Name = args[len(args)-1]
				asked <- *opening
				opening = nil
			}
			return "", nil
		case "list-panes":
			if has(args, "-t") {
				// The home window: the navigator, and the shell beside it.
				out := "%0\t1\t120\t29"
				if shown != 0 {
					out += fmt.Sprintf("\n%%%d\t\t120\t29", shown)
				}
				return out, nil
			}
			return strings.Join(listing, "\n"), nil
		case "join-pane", "swap-pane":
			// A shell moved beside the navigator.
			shown = after(args, "-s")
			asked <- message{Kind: kindShow, PID: shown}
			return "", nil
		case "break-pane":
			asked <- message{Kind: kindPark, PID: after(args, "-s")}
			shown = 0
			return "", nil
		case "select-pane":
			asked <- message{Kind: kindFocus, PID: target(args)}
			return "", nil
		case "select-window", "rename-window", "select-layout", "resize-window":
			return "", nil
		case "detach-client":
			asked <- message{Kind: kindLeave}
			return "", nil
		case "kill-pane":
			asked <- message{Kind: kindClose, PID: target(args)}
			return "", nil
		case "show":
			if has(args, agentOption) {
				return agent, nil
			}
			return "", nil
		case "refresh-client":
			return "", nil
		case "list-clients":
			return "1\tc0", nil
		case "display-message":
			return "120 30", nil
		case "display-popup":
			asked <- message{Kind: kindHelp, Run: args[len(args)-1]}
			return "", nil
		}
		return "", nil
	}
	return s, asked
}

// askedForKind waits for the server to be asked something of one kind,
// letting the asks before it — a pane's name, most often — go by.
func askedForKind(t *testing.T, asked chan message, kind string) message {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case got := <-asked:
			if got.Kind == kind {
				return got
			}
		case <-deadline:
			t.Fatalf("the server was never asked to %s", kind)
			return message{}
		}
	}
}

// askedForKinds waits until the server has been asked for one of each
// kind, in any order, and returns them by kind — for a key that asks two
// things at once, a close and an open, each on a goroutine of its own, so
// the fake records them in whichever order the scheduler runs them.
func askedForKinds(t *testing.T, asked chan message, kinds ...string) map[string]message {
	t.Helper()
	want := map[string]bool{}
	for _, k := range kinds {
		want[k] = true
	}
	got := map[string]message{}
	deadline := time.After(time.Second)
	for len(got) < len(want) {
		select {
		case m := <-asked:
			if want[m.Kind] {
				if _, seen := got[m.Kind]; !seen {
					got[m.Kind] = m
				}
			}
		case <-deadline:
			for k := range want {
				if _, seen := got[k]; !seen {
					t.Fatalf("the server was never asked to %s", k)
				}
			}
		}
	}
	return got
}

// askedFor waits for the next ask that is about the shells, or fails the
// test. The status line's
// asks — the mode, the message — ride along with any update and are not
// what a test asking "what did that key do" is about.
func askedFor(t *testing.T, asked chan message) message {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case got := <-asked:
			if got.Kind != kindMode && got.Kind != kindMsg && got.Kind != kindNeed {
				return got
			}
		case <-deadline:
			t.Fatal("the server was asked for nothing")
			return message{}
		}
	}
}

func TestPlaceAtPrefersTheInnermostPlace(t *testing.T) {
	m := model{
		projects: []Project{{Name: "mono", Path: "/p/mono"}},
		subs:     map[string][]Project{"/p/mono": {{Name: "api", Path: "/p/mono/api"}}},
		groups:   []Project{{Name: "p", Path: "/p"}},
	}

	if p, ok := m.placeAt("/p/mono/api/src"); !ok || p.Path != "/p/mono/api" {
		t.Errorf("placeAt = %+v %v, want the sub-project", p, ok)
	}
	if p, ok := m.placeAt("/p/mono/cmd"); !ok || p.Path != "/p/mono" {
		t.Errorf("placeAt = %+v %v, want the repository", p, ok)
	}
	if p, ok := m.placeAt("/p"); !ok || p.Path != "/p" {
		t.Errorf("placeAt = %+v %v, want work at the group's own level to belong to the group", p, ok)
	}
	if p, ok := m.placeAt("/elsewhere"); ok {
		t.Errorf("placeAt = %+v, want nothing outside every place", p)
	}
}

func TestPrefixEnterWithNothingWaitingSaysSo(t *testing.T) {
	m := withClaude("claude", map[int]claudeSession{
		700: {PID: 700, Status: busyStatus},
	})

	if m = press(m, "tab"); m.status != "nothing needs you" {
		t.Errorf("status = %q, want it to say nothing is waiting", m.status)
	}
}

func TestAReusedPIDIsNotDressedUpAsClaude(t *testing.T) {
	// A session file can outlive its process; the command name settles it.
	m := withClaude("vim", map[int]claudeSession{
		700: {PID: 700, Name: "conn-1f", Status: "busy"},
	})

	row := navColumn(m)[1]
	if strings.ContainsAny(row, "●○") {
		t.Errorf("row = %q, want no marker on a process that is not claude", row)
	}
}

func TestAClaudeWithNoSessionFileIsJustAProcess(t *testing.T) {
	// Claude's own helper is called claude too, and advertises no session.
	m := withClaude("claude", nil)

	row := navColumn(m)[1]
	if strings.ContainsAny(row, "●○") {
		t.Errorf("row = %q, want no marker without a session to report", row)
	}
}

func TestTheMarkerLeavesRoomForTheName(t *testing.T) {
	m := withClaude("claude", map[int]claudeSession{
		700: {PID: 700, Status: "busy"},
	})

	for i, row := range strings.Split(m.View().Content, "\n") {
		if w := lipgloss.Width(stripANSI(row)); w != m.width {
			t.Fatalf("row %d is %d columns wide, want %d: %q", i, w, m.width, stripANSI(row))
		}
	}
}

func TestTheListKeepsARowHoweverShortTheWindow(t *testing.T) {
	for _, h := range []int{3, 4, 6, 8, 12, 24} {
		m := withProcList(80, h, []Project{{Name: "alpha"}, {Name: "beta"}}, nil)
		if got := len(strings.Split(m.View().Content, "\n")); got != h {
			t.Errorf("height %d: view is %d lines", h, got)
		}
		if h >= 3 && len(navColumn(m)) == 0 {
			t.Errorf("height %d: no room was left for the list", h)
		}
	}
}

// --- the folded run, and unfolding it ------------------------------------

func TestDashShowsEveryProcess(t *testing.T) {
	m := withProcList(80, 12,
		[]Project{{Name: "conn", Path: "/p/conn"}},
		[]Proc{
			{PID: 10, PPID: 1, Command: "zsh", Dir: "/p/conn"},
			{PID: 20, PPID: 10, Command: "nvim", Dir: "/p/conn"},
			{PID: 21, PPID: 20, Command: "nvim", Dir: "/p/conn"},
		})
	wantRows(t, navColumn(m), []string{"▸ conn", "      nvim"})

	m = press(m, "-")
	wantRows(t, navColumn(m), []string{
		"▸ conn", "      zsh", "        nvim", "          nvim",
	})
}

func TestDashFoldsThemBackAgain(t *testing.T) {
	m := press(press(nestedTree(12), "-"), "-")
	if m.unfolded {
		t.Error("- should toggle rather than only unfold")
	}
}

func TestUnfoldingKeepsTheCursorOnItsProcess(t *testing.T) {
	m := withProcList(80, 12,
		[]Project{{Name: "conn", Path: "/p/conn"}},
		[]Proc{
			{PID: 10, PPID: 1, Command: "zsh", Dir: "/p/conn"},
			{PID: 20, PPID: 10, Command: "nvim", Dir: "/p/conn"},
			{PID: 21, PPID: 20, Command: "nvim", Dir: "/p/conn"},
		})
	m.cursor = 1 // the folded row, named for nvim 20

	m = press(m, "-")
	if r, _ := m.selected(); r.node.PID != 20 {
		t.Errorf("selected pid %d, want to still be on nvim 20 after unfolding", r.node.PID)
	}
}

// --- finding a project ---------------------------------------------------

// manyProjects is a set wide enough that finding one matters, with nothing
// running in any of them.
func manyProjects(w, h int) model {
	return withProcList(w, h, []Project{
		{Name: "conn", Path: "/p/w0zro/conn"},
		{Name: "hsg", Path: "/p/hsg/hsg"},
		{Name: "brand", Path: "/p/hsg/brand"},
		{Name: "tressle-api", Path: "/p/node/tressle-api"},
		{Name: "flocking-pixi", Path: "/p/flocking-pixi"},
	}, nil)
}

func TestSlashSearchesEveryProjectNotJustTheRunningOnes(t *testing.T) {
	// The point of the filter is to reach a project you are not working in,
	// so it looks past the narrowed view rather than within it.
	m := narrowed(manyProjects(90, 14))
	if len(m.rows) != 0 {
		t.Fatalf("setup: rows = %d, want the narrowed view to be empty", len(m.rows))
	}

	m = press(m, "/")
	m = typeFilter(m, "brand")

	wantRows(t, navColumn(m), []string{"▸ brand"})
}

// typeFilter sends each rune to the filter.
func typeFilter(m model, s string) model {
	for _, r := range s {
		next, _ := m.Update(typed(string(r)))
		m = next.(model)
	}
	return m
}

func TestFilterReachesProcessesByCommand(t *testing.T) {
	// Typing claude finds the repositories a claude is working in, not only
	// the ones named for it — at work the question is as often "where is that
	// running" as "where is that checked out".
	m := withProcList(90, 14, []Project{
		{Name: "conn", Path: "/p/conn"},
		{Name: "brand", Path: "/p/brand"},
	}, []Proc{{PID: 100, PPID: 1, Command: "claude", Dir: "/p/brand"}})

	m = typeFilter(press(narrowed(m), "/"), "claude")
	wantRows(t, navColumn(m), []string{"  brand", "    ▸ claude"})
}

func TestFilterReachesAChildProcessCommand(t *testing.T) {
	// The whole tree answers: a shell running an npm running a node is found
	// by any of their names, whichever the row happens to be named after.
	m := withProcList(90, 14, []Project{{Name: "brand", Path: "/p/brand"}},
		[]Proc{
			{PID: 100, PPID: 1, Command: "zsh", Dir: "/p/brand"},
			{PID: 101, PPID: 100, Command: "node", Dir: "/p/brand"},
		})

	m = typeFilter(press(narrowed(m), "/"), "node")
	wantRows(t, navColumn(m), []string{"  brand", "    ▸ node"})
}

func TestTypingListsTheProcessesThatAnswer(t *testing.T) {
	// A query is a name, and a process that answers to it is as much the
	// thing being looked for as a project is: it is listed under its place,
	// pruned to the branches that answer, so it can be acted on straight
	// from the search.
	m := withProcList(90, 14, []Project{{Name: "brand", Path: "/p/brand"}},
		[]Proc{
			{PID: 100, PPID: 1, Command: "zsh", Dir: "/p/brand"},
			{PID: 101, PPID: 100, Command: "node", Dir: "/p/brand"},
			{PID: 102, PPID: 1, Command: "vim", Dir: "/p/brand"},
		})

	m = typeFilter(press(narrowed(m), "/"), "node")
	rows := navColumn(m)
	wantRows(t, rows, []string{"  brand", "    ▸ node"})
	if len(rows) != 2 {
		t.Fatalf("rows = %q, want the vim pruned away", rows)
	}
}

func TestTypingAnEmptyQueryListsPlacesAlone(t *testing.T) {
	// Every process of every project would bury the names being scanned for;
	// the processes join the list only once a query gives them a reason to.
	m := withProcList(90, 14, []Project{{Name: "brand", Path: "/p/brand"}},
		[]Proc{{PID: 100, PPID: 1, Command: "vim", Dir: "/p/brand"}})

	m = press(narrowed(m), "/")
	for _, row := range navColumn(m) {
		if strings.Contains(row, "vim") {
			t.Fatalf("row %q lists a process before anything was typed", row)
		}
	}
}

func TestEnterOnAFoundProcessShowsItsWindow(t *testing.T) {
	m := withProcList(90, 14, []Project{{Name: "brand", Path: "/p/brand"}},
		[]Proc{{PID: 100, PPID: 1, Command: "zsh", Dir: "/p/brand"}})
	m.terms[100] = &remoteTerm{pid: 100}
	m, asked := pipeServer(t, m)

	// The cursor lands on the matching shell by itself; enter needs no move.
	m = typeFilter(press(narrowed(m), "/"), "zsh")
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)

	if m.typing {
		t.Error("stepping in is the end of looking")
	}
	if got := askedForKind(t, asked, kindShow); got.PID != 100 {
		t.Fatalf("asked %+v, want the client taken to pid 100's window", got)
	}
}

func TestCtrlXWhileTypingAsksToKillWhatWasFound(t *testing.T) {
	m := withProcList(90, 14, []Project{{Name: "brand", Path: "/p/brand"}},
		[]Proc{{PID: 100, PPID: 1, Command: "vim", Dir: "/p/brand"}})

	// The cursor lands on the matching process by itself.
	m = typeFilter(press(narrowed(m), "/"), "vim")
	next, _ := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	m = next.(model)

	if m.typing {
		t.Error("ctrl+x should end the typing, so the confirmation's key confirms")
	}
	if m.filter != "vim" {
		t.Errorf("filter = %q, want it held so the subject stays listed", m.filter)
	}
	if got := targets(m.pendingKill); len(got) != 1 || got[0] != 100 {
		t.Fatalf("pending kill on %v, want the vim, pid 100", got)
	}
}

func TestFilterMatchesThePathAsWellAsTheName(t *testing.T) {
	m := typeFilter(press(narrowed(manyProjects(90, 14)), "/"), "node")
	wantRows(t, navColumn(m), []string{"▸ tressle-api"})
}

func TestFilterIgnoresCase(t *testing.T) {
	m := typeFilter(press(narrowed(manyProjects(90, 14)), "/"), "CONN")
	wantRows(t, navColumn(m), []string{"▸ conn"})
}

func TestKeysGoToTheFilterWhileItIsBeingTyped(t *testing.T) {
	// A project called "conn" must be typeable without s opening a shell.
	m := press(narrowed(manyProjects(90, 14)), "/")
	m = typeFilter(m, "s")

	if m.filter != "s" {
		t.Errorf("filter = %q, want the keystroke to have gone into it", m.filter)
	}
	if len(m.terms) != 0 {
		t.Error("s while typing a filter should not open a shell")
	}
}

func TestBackspaceWidensTheFilter(t *testing.T) {
	m := typeFilter(press(narrowed(manyProjects(90, 14)), "/"), "brandx")
	if len(m.rows) != 0 {
		t.Fatalf("setup: rows = %d, want no match", len(m.rows))
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	wantRows(t, navColumn(next.(model)), []string{"▸ brand"})
}

func TestEnterKeepsTheFilterSoTheProjectStays(t *testing.T) {
	// Clearing it on accept would drop an idle project straight back out of
	// the narrowed list, before anything could be started in it.
	m := typeFilter(press(narrowed(manyProjects(90, 14)), "/"), "brand")
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)

	if m.typing {
		t.Error("enter should stop the typing")
	}
	if m.filter != "brand" {
		t.Errorf("filter = %q, want it still applied", m.filter)
	}
	wantRows(t, navColumn(m), []string{"▸ brand"})
}

func TestAMoveEndsTheHoldARunPutsOnTheCursor(t *testing.T) {
	// A run holds the cursor on the project until its processes land, so
	// the rows settling under it does not carry it off. A move in that gap
	// is the cursor being wanted elsewhere: the server re-listing its panes
	// — which a move onto a shell row makes it do — must not snap it back.
	root := t.TempDir()
	repo := filepath.Join(root, "conn")
	docs := filepath.Join(repo, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, ".conn"), []byte("web: python3 -m http.server\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := withProcList(90, 14, []Project{{Name: "conn", Path: repo}}, nil)
	m.subs = map[string][]Project{repo: {{Name: "docs", Path: docs}}}
	m = narrowed(m)
	m, _ = pipeServer(t, m)

	m = typeFilter(press(m, "/"), "docs")
	next, _ := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m = next.(model)
	if m.status != "started web" {
		t.Fatalf("status = %q, want the plan started", m.status)
	}
	// The shell lands; the process scan has not seen what it runs yet.
	next, _ = m.Update(termOpenedMsg{pid: 901, dir: docs, name: "web"})
	m = next.(model)
	next, _ = m.Update(sessionsMsg{sessions: []sessionInfo{{PID: 901, Dir: docs, Name: "web"}}})
	m = next.(model)
	if m.wantProject == "" && m.wantCursor == 0 {
		t.Fatal("setup: the hold should still be on before the process lands")
	}

	m = press(m, "k")
	moved := m.cursor
	next, _ = m.Update(sessionsMsg{sessions: []sessionInfo{{PID: 901, Dir: docs, Name: "web"}}})
	m = next.(model)
	if m.cursor != moved {
		t.Errorf("cursor = %d after the re-list, want it left at %d where k put it", m.cursor, moved)
	}
	if m.wantProject != "" || m.wantCursor != 0 {
		t.Error("the move should have ended the hold")
	}
	// Nor does the process landing carry the cursor off to it.
	next, _ = m.Update(procsMsg{procs: []Proc{
		{PID: 901, PPID: 1, Command: "zsh", Dir: docs},
		{PID: 902, PPID: 901, Command: "python3 -m http.server", Dir: docs},
	}})
	m = next.(model)
	if m.cursor != moved {
		t.Errorf("cursor = %d after the scan, want it left at %d", m.cursor, moved)
	}
}

// runsDocsPlan is a narrowed navigator over a repository conn with a
// sub-project docs carrying the given plan, wired to the recording server,
// with the filter typed to docs and the plan started from there.
func runsDocsPlan(t *testing.T, plan string) (m model, docs string) {
	t.Helper()
	root := t.TempDir()
	repo := filepath.Join(root, "conn")
	docs = filepath.Join(repo, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, ".conn"), []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	m = withProcList(90, 14, []Project{{Name: "conn", Path: repo}},
		[]Proc{{PID: 500, PPID: 1, Command: "go run .", Dir: repo}})
	m.subs = map[string][]Project{repo: {{Name: "docs", Path: docs}}}
	m = narrowed(m)
	m, _ = pipeServer(t, m)

	m = typeFilter(press(m, "/"), "doc")
	for i, r := range m.rows {
		if r.kind == rowSub {
			m.cursor = i
		}
	}
	if r, ok := m.selected(); !ok || r.kind != rowSub {
		t.Fatalf("setup: cursor on %+v, want docs", r)
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m = next.(model)
	if !strings.HasPrefix(m.status, "started ") {
		t.Fatalf("status = %q, want the plan started", m.status)
	}
	return m, docs
}

const webPlan = "web: python3 -m http.server\n"

func TestAShellListedBeforeItWasDressedIsStillThePlansShell(t *testing.T) {
	// tmux announces the new window before conn has set its directory and
	// name on it, and the list that announcement sets off reaches the
	// navigator first, knowing neither. The reports that follow — the
	// open itself, the list read after the options were set — do know,
	// and the shell must learn them: otherwise the navigator never sees
	// the plan's shell as the plan's, the filter that found the project
	// never lets go, and the hold on the project row never ends.
	m, docs := runsDocsPlan(t, webPlan)
	next, _ := m.Update(sessionsMsg{sessions: []sessionInfo{{PID: 901, Dir: docs}}})
	m = next.(model)
	next, _ = m.Update(termOpenedMsg{pid: 901, dir: docs, name: "web"})
	m = next.(model)
	if got := m.terms[901]; got == nil || got.name != "web" {
		t.Fatalf("terms[901] = %+v, want the name the open carried", got)
	}
	next, _ = m.Update(sessionsMsg{sessions: []sessionInfo{{PID: 901, Dir: docs, Name: "web"}}})
	m = next.(model)
	if m.filter != "" {
		t.Errorf("filter = %q, want it let go once the plan's shell is held", m.filter)
	}

	// The other order: the open first, then the early list. A blank in
	// the list does not take the name away.
	next, _ = m.Update(sessionsMsg{sessions: []sessionInfo{{PID: 901, Dir: docs}}})
	m = next.(model)
	if got := m.terms[901]; got == nil || got.name != "web" {
		t.Errorf("terms[901] = %+v after an early list, want the name kept", got)
	}
}

func TestAShellAtItsPromptWithNoExitRecordedIsNotRunning(t *testing.T) {
	// The recording can miss, and an older server's shells recorded
	// nothing: a plan shell the scan finds at its prompt with no ending is
	// not running its entry — r starts the entry again, and the key runs
	// a task again — rather than standing in the way for good.
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := withProcList(90, 14, []Project{{Name: "conn", Path: repo}},
		[]Proc{{PID: 700, PPID: 1, Command: "zsh", Dir: repo}, {PID: 701, PPID: 700, Command: "go", Dir: repo}})
	m.terms = map[int]*remoteTerm{700: {pid: 700, dir: repo, name: "build"}}
	m, asked := pipeServer(t, m)
	m.rebuild()
	if !m.namesIn(repo, readPlan(repo))["build"] {
		t.Fatal("setup: the build should count as running while go runs under its shell")
	}
	m = press(m, "b")
	if m.status != "already building conn" {
		t.Fatalf("status = %q, want the run left to finish", m.status)
	}

	// The go is gone and the shell is at its prompt, but nothing was
	// recorded: not running.
	next, _ := m.Update(procsMsg{procs: []Proc{{PID: 700, PPID: 1, Command: "zsh", Dir: repo}}})
	m = next.(model)
	if m.namesIn(repo, readPlan(repo))["build"] {
		t.Error("a shell at its prompt with no ending should not count as running")
	}
	if st := m.entryStates(repo)["build"]; st.State == "up" {
		t.Error("the checklist should not show it up either")
	}
	m = press(m, "b")
	got := askedForKinds(t, asked, kindClose, kindOpen)
	if got[kindClose].PID != 700 {
		t.Errorf("asked %+v, want the shell at its prompt closed for the new run", got[kindClose])
	}
	if got[kindOpen].Run != "go build ./..." {
		t.Errorf("asked %+v, want the build run again", got[kindOpen])
	}
}

func TestTRunsThePlacesTestsAndRedoesRatherThanStacks(t *testing.T) {
	// t runs the tests the way the place says, in a shell named test that
	// is wrapped like a plan entry's, and the cursor goes to it. A second
	// t while they run leaves them to finish; once they have ended, t
	// closes that shell and runs them again in a new one.
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := withProcList(90, 14, []Project{{Name: "conn", Path: repo}}, nil)
	m, asked := pipeServer(t, m)

	m = press(m, "t")
	got := askedForKind(t, asked, kindOpen)
	if got.Dir != repo || got.Run != "go test ./..." || got.Name != testName {
		t.Fatalf("asked %+v, want go test run at the place in a shell named test", got)
	}
	if m.status != "testing conn: go test ./..." || m.wantName != testName {
		t.Errorf("status %q, wantName %q; want the run said and the cursor headed for it", m.status, m.wantName)
	}

	// Running: left alone.
	next, _ := m.Update(sessionsMsg{sessions: []sessionInfo{{PID: 901, Dir: repo, Name: testName}}})
	m = next.(model)
	m = press(m, "t")
	if m.status != "already testing conn" {
		t.Errorf("status = %q, want the run left to finish", m.status)
	}
	select {
	case got := <-asked:
		if got.Kind == kindOpen || got.Kind == kindClose {
			t.Errorf("a second t while running asked %+v", got)
		}
	case <-time.After(50 * time.Millisecond):
	}

	// Ended: replaced.
	next, _ = m.Update(sessionsMsg{sessions: []sessionInfo{{PID: 901, Dir: repo, Name: testName, Exit: "1"}}})
	m = next.(model)
	m = press(m, "t")
	both := askedForKinds(t, asked, kindClose, kindOpen)
	if both[kindClose].PID != 901 {
		t.Errorf("asked %+v, want the ended test shell closed", both[kindClose])
	}
	if both[kindOpen].Run != "go test ./..." {
		t.Errorf("asked %+v, want the tests run again", both[kindOpen])
	}

	// A place that says nothing of its tests: said.
	bare := t.TempDir()
	m = withProcList(90, 14, []Project{{Name: "bare", Path: bare}}, nil)
	m, _ = pipeServer(t, m)
	if m = press(m, "t"); m.status != "bare does not say how its tests run" {
		t.Errorf("status = %q, want the lack said", m.status)
	}
}

func TestBAndLRunTheBuildAndTheLintTheSameWay(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := withProcList(90, 14, []Project{{Name: "conn", Path: repo}}, nil)
	m, asked := pipeServer(t, m)
	m = press(m, "b")
	if got := askedForKind(t, asked, kindOpen); got.Run != "go build ./..." || got.Name != "build" {
		t.Errorf("b asked %+v, want the build in a shell named build", got)
	}
	if m.status != "building conn: go build ./..." {
		t.Errorf("status = %q", m.status)
	}
	m = press(m, "l")
	if got := askedForKind(t, asked, kindOpen); got.Run != "go vet ./..." || got.Name != "lint" {
		t.Errorf("l asked %+v, want the lint in a shell named lint", got)
	}
}

func TestAFailedShellUsedAgainByHandDropsItsExit(t *testing.T) {
	// The shell at its prompt after its command ended badly shows the
	// cross. Running something in it by hand is acting on the failure:
	// the exit is dropped, here and on the pane, and a list still
	// carrying it does not bring it back. The command's own recording —
	// the tmux that sets the option, a child of the shell for a moment
	// before the prompt — is not a reuse.
	m := withProcList(90, 14, []Project{{Name: "tmp", Path: "/tmp"}},
		[]Proc{{PID: 700, PPID: 1, Command: "zsh", Dir: "/tmp"}, {PID: 701, PPID: 700, Command: "tmux", Dir: "/tmp"}})
	m.terms = map[int]*remoteTerm{700: {pid: 700, dir: "/tmp", name: "web"}}
	m, _ = pipeServer(t, m)
	var mu sync.Mutex
	drops := 0
	inner := m.server.run
	m.server.run = func(args ...string) (string, error) {
		if args[0] == "set" && slices.Contains(args, "-pu") {
			mu.Lock()
			drops++
			mu.Unlock()
		}
		return inner(args...)
	}
	dropped := func() int {
		time.Sleep(30 * time.Millisecond)
		mu.Lock()
		defer mu.Unlock()
		return drops
	}

	// The ending learned while the recording is still the shell's child.
	next, _ := m.Update(sessionsMsg{sessions: []sessionInfo{{PID: 700, Dir: "/tmp", Name: "web", Exit: "1"}}})
	m = next.(model)
	if m.terms[700].exit != "1" || dropped() != 0 {
		t.Fatalf("exit = %q, dropped %d; want the exit kept through its own recording", m.terms[700].exit, dropped())
	}
	// At its prompt: the row shows the cross.
	next, _ = m.Update(procsMsg{procs: []Proc{{PID: 700, PPID: 1, Command: "zsh", Dir: "/tmp"}}})
	m = next.(model)
	if row := renderRow(m, m.rows[1]); !strings.Contains(row, glyphFailed) {
		t.Fatalf("row = %q, want the cross", row)
	}
	// Used by hand: the exit goes, and the pane is told.
	next, _ = m.Update(procsMsg{procs: []Proc{{PID: 700, PPID: 1, Command: "zsh", Dir: "/tmp"}, {PID: 702, PPID: 700, Command: "ls", Dir: "/tmp"}}})
	m = next.(model)
	if m.terms[700].exit != "" || dropped() != 1 {
		t.Fatalf("exit = %q, dropped %d; want the exit dropped, once", m.terms[700].exit, dropped())
	}
	// A list read before the pane was told still carries it: not taken.
	next, _ = m.Update(sessionsMsg{sessions: []sessionInfo{{PID: 700, Dir: "/tmp", Name: "web", Exit: "1"}}})
	m = next.(model)
	next, _ = m.Update(procsMsg{procs: []Proc{{PID: 700, PPID: 1, Command: "zsh", Dir: "/tmp"}}})
	m = next.(model)
	if row := renderRow(m, m.rows[1]); strings.Contains(row, glyphFailed) || m.terms[700].exit != "" {
		t.Errorf("row = %q, exit %q; want a plain shell, its ending history", row, m.terms[700].exit)
	}
}

func TestASettledExitHasItsTranscriptReadForWhatTheRunSaid(t *testing.T) {
	// The shell at its prompt with its ending: the transcript is read
	// once, off the render path, and the row says what the run said of
	// itself — 3 failed — where a running row says its ports; the pane's
	// exited line carries it too. A shell used by hand after says nothing
	// of it any more.
	m := withProcList(90, 14, []Project{{Name: "tmp", Path: "/tmp"}},
		[]Proc{{PID: 700, PPID: 1, Command: "zsh", Dir: "/tmp"}})
	m.terms = map[int]*remoteTerm{700: {pid: 700, dir: "/tmp", name: "test"}}
	m, _ = pipeServer(t, m)
	var mu sync.Mutex
	reads := 0
	inner := m.server.run
	m.server.run = func(args ...string) (string, error) {
		if args[0] == "capture-pane" {
			mu.Lock()
			reads++
			mu.Unlock()
			return "--- FAIL: TestA (0.00s)\n--- FAIL: TestB (0.00s)\n--- FAIL: TestC (0.00s)\nFAIL\n\n", nil
		}
		return inner(args...)
	}
	// Learn the exit, then settle: the scan finds the shell at its
	// prompt, and the read is what Update batches after the update.
	m, _ = m.update(sessionsMsg{sessions: []sessionInfo{{PID: 700, Dir: "/tmp", Name: "test", Exit: "1"}}})
	var outcome *outcomeMsg
	for _, msg := range deliver(m.readEndings()) {
		if o, ok := msg.(outcomeMsg); ok {
			outcome = &o
		}
	}
	if outcome == nil {
		t.Fatal("settling should have had the transcript read")
	}
	if outcome.summary != "3 failed" {
		t.Errorf("summary = %q, want what the run said", outcome.summary)
	}
	m, _ = m.update(*outcome)
	if row := renderRow(m, m.rows[1]); !strings.Contains(stripANSI(row), "test · 3 failed") {
		t.Errorf("row = %q, want the run's word beside the name", stripANSI(row))
	}
	// And how long ago it ended, when the pane recorded that.
	m.terms[700].at = time.Now().Add(-3 * time.Minute)
	if row := renderRow(m, m.rows[1]); !strings.Contains(stripANSI(row), "test · 3 failed · 3m") {
		t.Errorf("row = %q, want the run's word and its age beside the name", stripANSI(row))
	}
	if st := m.ending(m.rows[1]); st.Summary != "3 failed" {
		t.Errorf("ending = %+v, want the summary carried", st)
	}
	// Read once: another scan reads nothing more.
	m, _ = m.update(procsMsg{procs: m.procs})
	if m.readEndings() != nil {
		t.Error("a settled ending should be read once")
	}
	mu.Lock()
	if reads != 1 {
		t.Errorf("the transcript was read %d times, want once", reads)
	}
	mu.Unlock()

	// Used by hand: the exit and its word go together.
	m, _ = m.update(procsMsg{procs: []Proc{{PID: 700, PPID: 1, Command: "zsh", Dir: "/tmp"}, {PID: 701, PPID: 700, Command: "ls", Dir: "/tmp"}}})
	if m.terms[700].summary != "" {
		t.Errorf("summary = %q after the shell was used by hand, want nothing", m.terms[700].summary)
	}
}

// deliver runs a command and everything it batches, collecting the messages.
func deliver(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range msg {
			out = append(out, deliver(c)...)
		}
		return out
	case nil:
		return nil
	default:
		return []tea.Msg{msg}
	}
}

func TestTheChecklistReportsTheLatestExit(t *testing.T) {
	// Two shells for one entry, both ended: the one that ended last
	// speaks for the entry, whichever the map hands over first.
	m := withProcList(90, 14, []Project{{Name: "tmp", Path: "/tmp"}}, nil)
	m, _ = pipeServer(t, m)
	next, _ := m.Update(sessionsMsg{sessions: []sessionInfo{
		{PID: 700, Dir: "/tmp", Name: "web", Exit: "1"}, {PID: 701, Dir: "/tmp", Name: "web"},
	}})
	m = next.(model)
	if got := m.entryStates("/tmp")["web"].State; got != "up" {
		t.Fatalf("web = %q, want up while one runs", got)
	}
	next, _ = m.Update(sessionsMsg{sessions: []sessionInfo{
		{PID: 700, Dir: "/tmp", Name: "web", Exit: "1"}, {PID: 701, Dir: "/tmp", Name: "web", Exit: "0"},
	}})
	m = next.(model)
	if got := m.entryStates("/tmp")["web"].State; got != "0" {
		t.Errorf("web = %q, want the later ending, 0", got)
	}
}

func TestAnEntryWhoseCommandEndedIsNotRunningAndRStartsItAgain(t *testing.T) {
	// The shell keeps the entry's name after its command ends, at its
	// prompt with the transcript; that is not the entry running. The
	// checklist shows it down, r starts it again beside the old shell,
	// and the cursor goes to the new one, not the old.
	m, docs := runsDocsPlan(t, webPlan)
	next, _ := m.Update(sessionsMsg{sessions: []sessionInfo{{PID: 901, Dir: docs, Name: "web"}}})
	m = next.(model)
	if running := m.namesIn(docs, readPlan(docs)); !running["web"] {
		t.Fatalf("running = %v, want web while its command runs", running)
	}

	next, _ = m.Update(sessionsMsg{sessions: []sessionInfo{{PID: 901, Dir: docs, Name: "web", Exit: "1"}}})
	m = next.(model)
	if running := m.namesIn(docs, readPlan(docs)); running["web"] {
		t.Fatalf("running = %v, want web down once its command ended", running)
	}
	m.letGo()
	m = press(m, "r")
	if m.status != "started web" {
		t.Fatalf("status = %q, want web started again", m.status)
	}
	next, _ = m.Update(sessionsMsg{sessions: []sessionInfo{
		{PID: 901, Dir: docs, Name: "web", Exit: "1"}, {PID: 902, Dir: docs, Name: "web"},
	}})
	m = next.(model)
	if m.wantCursor != 902 {
		t.Errorf("wantCursor = %d, want the shell just started, not the one at its prompt", m.wantCursor)
	}
}

func TestAPlanShellFoundAtItsPromptHasTheServerAskedOnce(t *testing.T) {
	// tmux announces nothing when the command ends and the pane's exit is
	// set, so the scan that finds the entry's shell alone — no command
	// under it — asks the server for the list, once; the command running
	// again clears that, so the next such scan asks again.
	m := withProcList(90, 14, []Project{{Name: "tmp", Path: "/tmp"}},
		[]Proc{{PID: 700, PPID: 1, Command: "zsh", Dir: "/tmp"}, {PID: 701, PPID: 700, Command: "npm", Dir: "/tmp"}})
	m.terms = map[int]*remoteTerm{700: {pid: 700, dir: "/tmp", name: "web"}}
	m, _ = pipeServer(t, m)
	var mu sync.Mutex
	lists := 0
	inner := m.server.run
	m.server.run = func(args ...string) (string, error) {
		if args[0] == "list-panes" && !slices.Contains(args, "-t") {
			mu.Lock()
			lists++
			mu.Unlock()
		}
		return inner(args...)
	}
	listed := func() int {
		time.Sleep(50 * time.Millisecond) // the list is read on its own goroutine
		mu.Lock()
		defer mu.Unlock()
		return lists
	}
	m.rebuild()
	if got := listed(); got != 0 {
		t.Fatalf("the server was listed %d times with the command running, want none", got)
	}
	// The command is gone: the shell alone, at its prompt.
	next, _ := m.Update(procsMsg{procs: []Proc{{PID: 700, PPID: 1, Command: "zsh", Dir: "/tmp"}}})
	m = next.(model)
	if got := listed(); got != 1 {
		t.Fatalf("the server was listed %d times, want once for the exit", got)
	}
	next, _ = m.Update(procsMsg{procs: m.procs})
	m = next.(model)
	if got := listed(); got != 1 {
		t.Errorf("the server was listed %d times after another scan, want still once", got)
	}
	// The command running again, then ending again: asked again.
	next, _ = m.Update(procsMsg{procs: []Proc{{PID: 700, PPID: 1, Command: "zsh", Dir: "/tmp"}, {PID: 702, PPID: 700, Command: "npm", Dir: "/tmp"}}})
	next, _ = next.(model).Update(procsMsg{procs: []Proc{{PID: 700, PPID: 1, Command: "zsh", Dir: "/tmp"}}})
	m = next.(model)
	if got := listed(); got != 2 {
		t.Errorf("the server was listed %d times, want twice for two endings", got)
	}
}

func TestRunLandsTheCursorOnTheFirstThingItStarted(t *testing.T) {
	// Running a plan from a filtered list: the project keeps the cursor
	// while its shell starts, and once the server holds the shell and the
	// scan sees what it runs, the cursor is on that row with the filter
	// gone — the plan was run, and here is what it started.
	m, docs := runsDocsPlan(t, webPlan)
	next, _ := m.Update(termOpenedMsg{pid: 901, dir: docs, name: "web"})
	m = next.(model)
	next, _ = m.Update(sessionsMsg{sessions: []sessionInfo{{PID: 901, Dir: docs, Name: "web"}}})
	m = next.(model)
	if r, ok := m.selected(); !ok || r.kind != rowSub {
		t.Fatalf("cursor on %+v before the scan, want still on docs", r)
	}
	if m.wantCursor != 901 || m.wantProject != "" {
		t.Fatalf("wantCursor %d, wantProject %q; want the hold handed to the shell", m.wantCursor, m.wantProject)
	}

	next, _ = m.Update(procsMsg{procs: []Proc{
		{PID: 500, PPID: 1, Command: "go run .", Dir: filepath.Dir(docs)},
		{PID: 901, PPID: 1, Command: "zsh", Dir: docs},
		{PID: 902, PPID: 901, Command: "python3 -m http.server", Dir: docs},
	}})
	m = next.(model)
	r, ok := m.selected()
	if !ok || r.kind != rowProc || !r.holds(901) {
		t.Fatalf("cursor on %+v, want the row of the shell the plan started", r)
	}
	if m.filter != "" {
		t.Errorf("filter = %q, want it gone", m.filter)
	}
	if m.wantCursor != 0 {
		t.Errorf("wantCursor = %d, want the landing to have ended the wait", m.wantCursor)
	}
}

func TestRunLandsOnThePlansFirstEntryWhicheverShellCameFirst(t *testing.T) {
	// Several entries start at once and the server may hold them in any
	// order; the cursor goes to the one the plan lists first.
	m, docs := runsDocsPlan(t, "web: python3 -m http.server\nwatch: go test ./...\n")
	next, _ := m.Update(sessionsMsg{sessions: []sessionInfo{
		{PID: 902, Dir: docs, Name: "watch"}, {PID: 901, Dir: docs, Name: "web"},
	}})
	m = next.(model)
	if m.wantCursor != 901 {
		t.Fatalf("wantCursor = %d, want web's shell, the plan's first entry", m.wantCursor)
	}
}

func TestOnceAcceptedTheOrdinaryKeysWorkAgain(t *testing.T) {
	m := typeFilter(press(narrowed(manyProjects(90, 14)), "/"), "brand")
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)

	// s now means open a shell rather than a letter of the filter.
	next, _ = m.Update(typed("s"))
	if got := next.(model); got.filter != "brand" {
		t.Errorf("filter = %q, want s to have been taken as a key", got.filter)
	}
}

func TestEscapeClearsTheFilterRatherThanQuitting(t *testing.T) {
	m := typeFilter(press(narrowed(manyProjects(90, 14)), "/"), "brand")
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd != nil {
		t.Error("esc with a filter applied should clear it, not quit")
	}
	if got := next.(model); got.filter != "" {
		t.Errorf("filter = %q, want it cleared", got.filter)
	}
}

func TestEscapeNeverQuits(t *testing.T) {
	// Esc closes what is open, and leaving is q's word alone: the esc that
	// closed the filter is often followed by a reflexive second one, and
	// that beat must not take the window with it.
	m, asked := pipeServer(t, manyProjects(90, 14))
	m = press(m, "esc")
	if len(asked) != 0 {
		t.Error("esc with nothing open should do nothing, not leave")
	}
	press(m, "q")
	if got := askedForKind(t, asked, kindLeave); got.Kind != kindLeave {
		t.Error("q should still leave")
	}
}

func TestEscapeWhileTypingAbandonsTheFilter(t *testing.T) {
	m := typeFilter(press(narrowed(manyProjects(90, 14)), "/"), "brand")
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(model)

	if m.typing || m.filter != "" {
		t.Errorf("typing=%v filter=%q, want the filter abandoned", m.typing, m.filter)
	}
}

func TestEscWhileTypingPutsTheCursorBack(t *testing.T) {
	// Abandoning the search is not acting on anything, so it puts the cursor
	// back on the row it left when / was pressed.
	m := manyProjects(90, 14)
	m = press(press(m, "down"), "down")
	r, ok := m.selected()
	if !ok {
		t.Fatal("setup: nothing selected")
	}
	was := detailKey(r)

	m = typeFilter(press(m, "/"), "conn")
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(model)

	if r, ok = m.selected(); !ok || detailKey(r) != was {
		t.Fatalf("cursor on %q, want back on %q", detailKey(r), was)
	}
}

func TestAFilterThatMatchesNothingSaysSo(t *testing.T) {
	m := typeFilter(press(narrowed(manyProjects(90, 14)), "/"), "zzz")
	wantRows(t, navColumn(m), []string{"  nothing answers zzz"})
}

func TestTheFooterShowsWhatIsBeingTyped(t *testing.T) {
	m := typeFilter(press(narrowed(manyProjects(160, 14)), "/"), "bra")
	if f := footer(m); !strings.Contains(f, "/bra") {
		t.Errorf("footer = %q, want it to show the filter being typed", f)
	}

	// Enter opens a shell in what is under the cursor, so the filter is over
	// and what it says next is about that rather than about the search.
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if next.(model).typing {
		t.Error("enter should finish the looking up")
	}
}

func TestTheEmptyListPointsAtTheFilter(t *testing.T) {
	m := narrowed(manyProjects(90, 14))
	col := strings.Join(navColumn(m), "\n")
	if !strings.Contains(col, "find a project") {
		t.Errorf("empty list = %q, want it to point at the way out", col)
	}
	if !strings.Contains(col, "?   the keys") {
		t.Errorf("empty list = %q, want the front door to teach the keys", col)
	}
}

func TestStartingSomethingClearsTheSearchThatFoundIt(t *testing.T) {
	// Once there is work in the project it stays listed on its own merit, so
	// the filter has nothing left to do.
	m := typeFilter(press(narrowed(manyProjects(90, 14)), "/"), "brand")
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)

	// A shell opened in it, and then the scan that finds it.
	m.wantCursor = 700
	next, _ = m.Update(procsMsg{procs: []Proc{
		{PID: 700, PPID: 1, Command: "zsh", Dir: "/p/hsg/brand"},
	}})
	m = next.(model)

	if m.filter != "" {
		t.Errorf("filter = %q, want it gone once the shell landed", m.filter)
	}
	// The cursor followed the shell it just started.
	wantRows(t, navColumn(m), []string{"  brand", "    ▸ zsh"})
}

func TestTheSearchHoldsUntilTheShellActuallyLands(t *testing.T) {
	// Clearing on the keystroke would drop the project out of the list until
	// the scan caught up, which is a flicker for no reason.
	m := typeFilter(press(narrowed(manyProjects(90, 14)), "/"), "brand")
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)
	m.wantCursor = 700 // asked for, not yet running

	next, _ = m.Update(procsMsg{procs: nil})
	m = next.(model)

	if m.filter != "brand" {
		t.Errorf("filter = %q, want it held until there is something to show", m.filter)
	}
	wantRows(t, navColumn(m), []string{"▸ brand"})
}

func TestEnteringSomethingClearsTheSearchAtOnce(t *testing.T) {
	// Nothing has to be waited for: the project already holds the shell.
	m := withProcList(90, 14,
		[]Project{{Name: "brand", Path: "/p/hsg/brand"}, {Name: "conn", Path: "/p/conn"}},
		[]Proc{{PID: 700, PPID: 1, Command: "zsh", Dir: "/p/hsg/brand"}})
	m.terms = map[int]*remoteTerm{700: {pid: 700, dir: "/p/hsg/brand"}}
	m, _ = pipeServer(t, m)
	m.showAll = false
	m.filter = "brand"
	m.rebuild()
	m.cursor = 1

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := next.(model); got.filter != "" {
		t.Errorf("filter = %q, want stepping in to have finished the search", got.filter)
	}
}

func TestKillingDoesNotClearTheSearch(t *testing.T) {
	// Clearing out several projects is one job; the list should not move
	// underneath it after each one.
	m := typeFilter(press(narrowed(manyProjects(90, 14)), "/"), "brand")
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)

	next, _ = m.Update(typed("X"))
	if got := next.(model); got.filter != "brand" {
		t.Errorf("filter = %q, want a kill to leave the search alone", got.filter)
	}
}

func TestSlashListsEveryProjectBeforeAnythingIsTyped(t *testing.T) {
	// Half of looking a project up is remembering which ones there are.
	m := narrowed(manyProjects(90, 14))
	if len(m.rows) != 0 {
		t.Fatalf("setup: rows = %d, want the narrowed view empty", len(m.rows))
	}

	m = press(m, "/")
	// Alphabetical, the order the scan delivers and topPlaces keeps.
	wantRows(t, navColumn(m), []string{
		"▸ brand", "  conn", "  flocking-pixi", "  hsg", "  tressle-api",
	})
}

func TestThePickerShowsProjectsWithoutTheirProcesses(t *testing.T) {
	// The names are what is being scanned; what is running would bury them.
	m := withProcList(90, 14,
		[]Project{{Name: "conn", Path: "/p/conn"}, {Name: "hsg", Path: "/p/hsg"}},
		[]Proc{
			{PID: 10, PPID: 1, Command: "zsh", Dir: "/p/conn"},
			{PID: 20, PPID: 10, Command: "nvim", Dir: "/p/conn"},
		})

	m = press(m, "/")
	wantRows(t, navColumn(m), []string{"▸ conn", "  hsg"})
	if len(m.rows) != 2 {
		t.Errorf("rows = %d, want only the two projects", len(m.rows))
	}
}

func TestThePickerDimsEverythingButTheCursor(t *testing.T) {
	m := press(narrowed(manyProjects(90, 14)), "/")

	for i, r := range m.rows {
		selected := i == m.cursor
		if selected && dimmed(m, r, true) {
			t.Error("the row under the cursor should stand out from the rest")
		}
		if !selected && !dimmed(m, r, false) {
			t.Errorf("row %d is lit; nothing is chosen yet", i)
		}
	}
}

func TestTypingNarrowsThePicker(t *testing.T) {
	m := press(narrowed(manyProjects(90, 14)), "/")
	if len(m.rows) != 5 {
		t.Fatalf("rows = %d, want every project to start", len(m.rows))
	}

	m = typeFilter(m, "h")
	wantRows(t, navColumn(m), []string{"▸ brand", "  hsg"}) // both are under /p/hsg
	m = typeFilter(m, "sg/h")
	wantRows(t, navColumn(m), []string{"▸ hsg"})
}

func TestLeavingThePickerBringsTheProcessesBack(t *testing.T) {
	m := withProcList(90, 14,
		[]Project{{Name: "conn", Path: "/p/conn"}},
		[]Proc{{PID: 10, PPID: 1, Command: "zsh", Dir: "/p/conn"}})

	m = press(m, "/")
	wantRows(t, navColumn(m), []string{"▸ conn"})

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	wantRows(t, navColumn(next.(model)), []string{"▸ conn", "      zsh"})
}

func TestARunIsNamedForTheProcessThatMatters(t *testing.T) {
	// claude keeps the machine awake while it works, so a caffeinate hangs
	// below it. Naming the run after its deepest process would call this a
	// caffeinate and hide the claude entirely.
	m := withProcList(80, 12,
		[]Project{{Name: "conn", Path: "/p/conn"}},
		[]Proc{
			{PID: 10, PPID: 1, Command: "zsh", Dir: "/p/conn"},
			{PID: 20, PPID: 10, Command: "claude", Dir: "/p/conn"},
			{PID: 30, PPID: 20, Command: "caffeinate", Dir: "/p/conn"},
		})
	wantRows(t, navColumn(m), []string{"▸ conn", "      claude"})
}

func TestATransientChildDoesNotRenameTheRow(t *testing.T) {
	// A claude reaching for a tool and finishing with it should not rename the
	// row it is on, twice, while you are looking at it.
	procs := []Proc{
		{PID: 10, PPID: 1, Command: "zsh", Dir: "/p/conn"},
		{PID: 20, PPID: 10, Command: "claude", Dir: "/p/conn"},
	}
	m := withProcList(80, 12, []Project{{Name: "conn", Path: "/p/conn"}}, procs)
	wantRows(t, navColumn(m), []string{"▸ conn", "      claude"})

	next, _ := m.Update(procsMsg{procs: append(procs,
		Proc{PID: 40, PPID: 20, Command: "rg", Dir: "/p/conn"})})
	wantRows(t, navColumn(next.(model)), []string{"▸ conn", "      claude"})
}

func TestARunOfNothingButShellsIsNamedForTheLast(t *testing.T) {
	// That is the one you would be typing into.
	m := withProcList(80, 12,
		[]Project{{Name: "conn", Path: "/p/conn"}},
		[]Proc{
			{PID: 10, PPID: 1, Command: "zsh", Dir: "/p/conn"},
			{PID: 20, PPID: 10, Command: "bash", Dir: "/p/conn"},
		})
	wantRows(t, navColumn(m), []string{"▸ conn", "      bash"})
}

func TestALoginShellIsStillAShell(t *testing.T) {
	if !isShell("-zsh") {
		t.Error("a login shell is written with a leading dash and is still a shell")
	}
	if isShell("claude") {
		t.Error("claude is not a shell")
	}
}

// --- the keys, on request ------------------------------------------------

func TestTheFootSaysTheSessionsFactsWhenQuiet(t *testing.T) {
	// The keys live behind ?, and the mode is the status line's to show;
	// the foot says the session's facts until something has to be said.
	m := manyProjects(90, 14)
	if got := footer(m); got != "ALL "+m.sessionFacts() {
		t.Errorf("footer = %q, want the facts alone", got)
	}
}

func TestQuestionMarkAsksForTheKeysPopup(t *testing.T) {
	// The navigator's pane is a column; the keys are a page, and the page
	// is tmux's popup over the whole window, from ? here as from the chord.
	m, asked := pipeServer(t, manyProjects(90, 14))
	before := footer(m)
	m = press(m, "?")
	if got := askedForKind(t, asked, kindHelp); got.Kind != kindHelp {
		t.Fatal("? did not ask for the popup")
	}
	if got := footer(m); got != before {
		t.Errorf("footer = %q, want the foot untouched", got)
	}
}

func TestTheKeysPageFitsItsPopup(t *testing.T) {
	// The popup is sized to the page; a row wider than the popup would wrap
	// inside it and push the rest off the bottom.
	var popup []string
	run := func(args ...string) (string, error) {
		switch args[0] {
		case "display-message":
			return "200 60", nil
		case "display-popup":
			popup = args
		}
		return "", nil
	}
	if err := showKeys(run, "/opt/conn", "c0"); err != nil {
		t.Fatal(err)
	}
	if len(popup) < 10 || popup[6] != "-w" || popup[8] != "-h" {
		t.Fatalf("popup = %q, want it sized", popup)
	}
	w, _ := strconv.Atoi(popup[7])
	for i, ln := range keysPage() {
		if got := lipgloss.Width(ln); got+2 > w {
			t.Errorf("page row %d is %d columns, wider than the popup's %d: %q", i, got, w, stripANSI(ln))
		}
	}
	for _, key := range []string{"shell", "of the tree", "the next thing owed", "leave"} {
		if !strings.Contains(keysOf(), key) {
			t.Errorf("the page does not list %q", key)
		}
	}
}

func TestThePopupIsCutToAShortClient(t *testing.T) {
	// tmux rejects a popup taller than the client rather than cutting it,
	// so the request is cut first: a short client gets what fits.
	var popup []string
	run := func(args ...string) (string, error) {
		switch args[0] {
		case "list-clients":
			return "10\tc0\n20\tc1", nil
		case "display-message":
			return "80 20", nil
		case "display-popup":
			popup = args
		}
		return "", nil
	}
	if err := showKeys(run, "/opt/conn", ""); err != nil {
		t.Fatal(err)
	}
	if len(popup) < 10 || popup[3] != "c1" || popup[9] != "20" {
		t.Errorf("popup = %q, want it on the client that spoke last, 20 rows tall", popup)
	}
}
func TestConnHoldsItsRowsWhileABufferIsShown(t *testing.T) {
	// The pane's size reaches conn late. A size the height of the whole
	// window arriving while a buffer is shown under the tabline is the
	// size from before the buffer joined: drawn at it, the frame overflows
	// the chrome and tmux wraps the overflow into the pane for a frame.
	m := withProcList(80, 24,
		[]Project{{Name: "tmp", Path: "/tmp"}},
		[]Proc{{PID: 700, PPID: 1, Command: "zsh", Dir: "/tmp"}})
	m.terms = map[int]*remoteTerm{700: {pid: 700}}
	m, _ = pipeServer(t, m)
	m = press(press(m, "down"), "enter") // into the buffer: shown under the tabline

	next, _ := m.Update(tea.WindowSizeMsg{Width: 254, Height: 62})
	m = next.(model)
	if m.height != chromeRows {
		t.Errorf("height = %d with a buffer shown, want the chrome, %d", m.height, chromeRows)
	}
	if m.width != 254 {
		t.Errorf("width = %d, want the window's: the tabline is as wide as it", m.width)
	}

	// The everything view: the buffer is parked, and conn has the window.
	m = press(m, ".")
	next, _ = m.Update(tea.WindowSizeMsg{Width: 254, Height: 62})
	m = next.(model)
	if m.height != 62 {
		t.Errorf("height = %d with nothing shown, want the window's 62", m.height)
	}

	// Entering the buffer shortens conn at once, before the join it
	// asked for can happen, so no frame is drawn tall into a pane about
	// to be two rows.
	m = press(m, "enter")
	if m.height != chromeRows {
		t.Errorf("height = %d on entering a buffer, want the chrome at once", m.height)
	}
}

func TestTheQueryIsALineWithReadlinesKeys(t *testing.T) {
	// The line the query is typed on is a text input, so it has every
	// editing key a line has and conn owns none of them: a word back, a
	// letter back, the cursor moved into the line and typing there.
	m := typeFilter(press(manyProjects(90, 14), "/"), "mono api")
	ctrl := func(m model, r rune) model {
		next, _ := m.Update(tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl})
		return next.(model)
	}
	if m = ctrl(m, 'w'); m.filter != "mono " {
		t.Errorf("after ctrl+w filter = %q, want the word taken back", m.filter)
	}
	if m = ctrl(m, 'h'); m.filter != "mono" {
		t.Errorf("after ctrl+h filter = %q, want a letter taken back", m.filter)
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	m = typeFilter(next.(model), "x")
	if m.filter != "monxo" {
		t.Errorf("after left and x filter = %q, want the letter typed where the cursor was", m.filter)
	}
	if line := lineText(m.query); line != "monx█o" {
		t.Errorf("the status line reads %q, want the cursor drawn where it is", line)
	}
	if m = ctrl(m, 'u'); m.filter != "o" {
		t.Errorf("after ctrl+u filter = %q, want what was before the cursor gone", m.filter)
	}
}

func TestARowSaysWhereItListens(t *testing.T) {
	// A dev server's row says what it is; the port beside it says where
	// it is. The port is the run's — the node under the npm holds it —
	// and a name too long for the row is the part that gives, not the
	// port.
	m := withProcList(30, 14,
		[]Project{{Name: "conn", Path: "/p/conn"}},
		[]Proc{
			{PID: 700, PPID: 1, Command: "npm", Argv: "npm run dev", Dir: "/p/conn"},
			{PID: 701, PPID: 700, Command: "node", Argv: "node vite", Dir: "/p/conn", Ports: []string{"5173", "24678"}},
			{PID: 702, PPID: 1, Command: "python3", Dir: "/p/conn",
				Argv:  "python3 -m some.very.long.module.name --with --many --flags --to --spare 8437",
				Ports: []string{"8437"}},
			{PID: 703, PPID: 1, Command: "go", Argv: "go test ./...", Dir: "/p/conn"},
		})
	rows := strings.Join(navColumn(m), "\n")
	if !strings.Contains(rows, "npm run … · :5173 :24678") {
		t.Errorf("rows = %q, want the run's ports beside the npm, the name giving way", rows)
	}
	if !strings.Contains(rows, "some.very.long.… · :8437") {
		t.Errorf("rows = %q, want the python cut and its port kept", rows)
	}
	if !strings.Contains(rows, "go test ./...\n") && !strings.HasSuffix(rows, "go test ./...") {
		t.Errorf("rows = %q, want nothing said of ports the go test has none of", rows)
	}
}

func TestARowShowsTheCrossWhenItsCommandEndedBadlyOrItsProcessIsUnwell(t *testing.T) {
	// The shell at its prompt after its command ended badly shows the
	// cross and reads red; after one that ended well it shows the check. A
	// process stopped or a zombie shows the cross too, shell or not. A
	// shell running something shows neither, whatever its pane recorded
	// of an earlier command.
	m := withProcList(90, 14,
		[]Project{{Name: "conn", Path: "/p/conn"}},
		[]Proc{
			{PID: 700, PPID: 1, Command: "zsh", Dir: "/p/conn"},
			{PID: 701, PPID: 1, Command: "zsh", Dir: "/p/conn"},
			{PID: 702, PPID: 1, Command: "node", Argv: "node worker.js", Dir: "/p/conn", State: "T"},
			{PID: 703, PPID: 1, Command: "zsh", Dir: "/p/conn"},
			{PID: 704, PPID: 703, Command: "npm", Argv: "npm test", Dir: "/p/conn"},
		})
	m.terms = map[int]*remoteTerm{
		700: {pid: 700, dir: "/p/conn", name: "web", exit: "1"},
		701: {pid: 701, dir: "/p/conn", name: "job", exit: "0"},
		703: {pid: 703, dir: "/p/conn", name: "test", exit: "1"},
	}
	m.rebuild()
	rows := map[string]string{}
	for _, r := range m.rows[1:] {
		rows[m.rowLabel(r)] = renderRow(m, r)
	}
	if row := rows["web"]; !strings.Contains(row, errStyle.Render(" "+glyphFailed)) {
		t.Errorf("web = %q, want the cross in the owed color", row)
	}
	if row := rows["job"]; !strings.Contains(row, glyphDone) || strings.Contains(row, glyphFailed) {
		t.Errorf("job = %q, want the check, ended well", row)
	}
	if row := rows["worker.js"]; !strings.Contains(row, glyphFailed) {
		t.Errorf("stopped worker = %q, want the cross", row)
	}
	if row := rows["npm test"]; strings.Contains(row, glyphFailed) || strings.Contains(row, glyphOff) {
		t.Errorf("a shell running something = %q, want no mark for an earlier ending", row)
	}
	// The window's name carries the cross too.
	if _, mark := m.shellLabel(700, m.terms[700]); mark != glyphFailed {
		t.Errorf("window mark = %q, want the cross", mark)
	}
	if _, mark := m.shellLabel(701, m.terms[701]); mark != glyphDone {
		t.Errorf("window mark = %q, want the check", mark)
	}
}

// renderRow draws one row as the navigator would, unselected.
func renderRow(m model, r navRow) string { return m.renderRow(r, false) }

func TestTabGoesToACommandThatEndedBadlyAsToAWaitingAgent(t *testing.T) {
	// What needs you is more than an agent's ask: a command that ended
	// badly, a process stopped or a zombie. Tab goes to them in turn — to
	// the row when conn only watches the process; into the shell when conn
	// holds it, where the transcript is — and says when nothing does.
	m := withProcList(90, 14,
		[]Project{{Name: "conn", Path: "/p/conn"}},
		[]Proc{
			{PID: 700, PPID: 1, Command: "zsh", Dir: "/p/conn"},
			{PID: 701, PPID: 1, Command: "node", Argv: "node worker.js", Dir: "/p/conn", State: "T"},
			{PID: 702, PPID: 1, Command: "go", Argv: "go test ./...", Dir: "/p/conn"},
		})
	m.terms = map[int]*remoteTerm{700: {pid: 700, dir: "/p/conn", name: "web", exit: "1"}}
	m, asked := pipeServer(t, m)

	// In the list's order: the stopped worker's row first, which conn only
	// watches, so the cursor goes there.
	m = press(m, "tab")
	if r, ok := m.selected(); !ok || r.kind != rowProc || r.node.PID != 701 {
		t.Fatalf("cursor on %+v, want the stopped worker", r)
	}
	// Then the shell whose command ended badly: its buffer is shown, dead
	// but readable, and the keys stay with conn — r reruns, q closes.
	m = press(m, "tab")
	if got := askedForKind(t, asked, kindShow); got.PID != 700 {
		t.Fatalf("tab showed %d, want the shell whose command ended badly", got.PID)
	}
	if m.shown != 700 || m.focus != 0 {
		t.Fatalf("shown = %d, focus = %d; want the dead buffer shown and the keys kept", m.shown, m.focus)
	}
	// Around again to the worker.
	m = press(m, "tab")
	if r, ok := m.selected(); !ok || r.kind != rowProc || r.node.PID != 701 {
		t.Errorf("cursor on %+v, want the stopped worker again", r)
	}

	// Nothing wrong and no agent waiting: said.
	m.terms[700].exit = "0"
	m.procs[1].State = "S"
	m.rebuild()
	if m = press(m, "tab"); m.status != "nothing needs you" {
		t.Errorf("status = %q, want the lack said", m.status)
	}
}

func TestAProcessThatIsConnSaysMe(t *testing.T) {
	// The launcher becomes a tmux client on conn's socket; under `go run .`
	// the row folds the go and the client together. Either way the row is
	// conn looking at itself, and says so.
	t.Setenv("CONN_SOCKET", "/tmp/conn-me-test.sock")
	m := withProcList(90, 14,
		[]Project{{Name: "conn", Path: "/p/conn"}},
		[]Proc{
			{PID: 700, PPID: 1, Command: "go", Argv: "go run .", Dir: "/p/conn"},
			{PID: 701, PPID: 700, Command: "tmux", Argv: "tmux -S /tmp/conn-me-test.sock attach -t conn", Dir: "/p/conn"},
			{PID: 702, PPID: 1, Command: "go", Argv: "go test ./...", Dir: "/p/conn"},
		})
	rows := navColumn(m)
	if len(rows) < 3 || strings.TrimSpace(rows[1]) != "(me)" {
		t.Errorf("rows = %q, want the go run row to read (me), and only that", rows)
	}
	if strings.Contains(rows[2], "(me)") {
		t.Errorf("rows = %q, want the go test row untagged", rows)
	}
}

func TestEscapeWithNothingOpenDoesNothing(t *testing.T) {
	m := manyProjects(90, 14)
	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape}); cmd != nil {
		t.Error("esc with nothing open should do nothing")
	}
}

func TestSomethingBeingSaidStillTakesTheFoot(t *testing.T) {
	// A confirmation or a report is about the next keystroke, so it is shown
	// whether or not the keys have been asked for.
	m := press(nestedTree(24), "r") // no server, so it explains itself
	if !strings.Contains(footer(m), "no server") {
		t.Errorf("footer = %q, want what was just said", footer(m))
	}
}

func TestTheSessionsAreReadOnAChainOfTheirOwn(t *testing.T) {
	// Reading them costs a few small files; the process scan costs an lsof
	// sweep of the machine. Tying them together made a session that had just
	// started working wait up to a process poll to say so.
	m := nestedTree(12)
	next, cmd := m.Update(agentTickMsg{})
	if cmd == nil {
		t.Fatal("the session chain should schedule its own next read")
	}
	if _, ok := cmd().(tea.BatchMsg); !ok {
		t.Error("a session tick should both read and schedule the next")
	}
	_ = next
}

func TestTheProcessTickNoLongerCarriesTheSessions(t *testing.T) {
	// Two chains reading them would double the rate for no reason.
	m := nestedTree(12)
	// On the repo-detail cadence, so the selected row's refresh rides along.
	m.ticks = repoDetailEvery - 1
	_, cmd := m.Update(tickMsg{})
	if cmd == nil {
		t.Fatal("a tick should still refresh")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatal("a tick should schedule more than one thing")
	}
	// The process scan and the next tick. If this becomes three, check
	// that the sessions have not been put back on this chain as well as
	// their own.
	if len(batch) != 2 {
		t.Errorf("tick batched %d commands, want 2", len(batch))
	}
}

// --- the ends of the list ------------------------------------------------

func TestGGoesToTheBottom(t *testing.T) {
	m := nestedTree(24)
	m = press(m, "G")

	if got, want := m.cursor, len(m.rows)-1; got != want {
		t.Errorf("cursor = %d, want the last row %d", got, want)
	}
}

func TestGGGoesToTheTop(t *testing.T) {
	m := press(nestedTree(24), "G")
	if m.cursor == 0 {
		t.Fatal("setup: expected to be somewhere other than the top")
	}

	m = press(m, "g")
	if m.cursor == 0 {
		t.Error("one g should wait for the second rather than moving")
	}
	m = press(m, "g")
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want the top", m.cursor)
	}
}

func TestOneGFollowedByAnythingElseDoesNothing(t *testing.T) {
	m := press(nestedTree(24), "G")
	at := m.cursor

	m = press(press(m, "g"), "j")
	if m.cursor != at {
		t.Errorf("cursor = %d, want %d: the key that cancels a g should not also move", m.cursor, at)
	}
	if m.pendingG {
		t.Error("the g should be over")
	}
}

func TestTheEndsScrollTheWindow(t *testing.T) {
	// A list longer than the window has to be scrolled to, not just pointed at.
	m := nestedTree(6)
	m = press(m, "G")

	if m.cursor < m.offset || m.cursor >= m.offset+m.bodyHeight() {
		t.Errorf("cursor %d is outside the window at %d..%d", m.cursor, m.offset, m.offset+m.bodyHeight())
	}
	m = press(press(m, "g"), "g")
	if m.offset != 0 {
		t.Errorf("offset = %d, want the window back at the top", m.offset)
	}
}

func TestTheEndsOfAnEmptyListAreHarmless(t *testing.T) {
	m := narrowed(withProcList(80, 12, []Project{{Name: "conn", Path: "/p/conn"}}, nil))
	if len(m.rows) != 0 {
		t.Fatalf("setup: rows = %d, want none", len(m.rows))
	}
	press(press(press(m, "G"), "g"), "g")
}

func TestTheEndsWorkWithinAFilter(t *testing.T) {
	// The list they move through is the one on screen.
	m := typeFilter(press(narrowed(manyProjects(90, 14)), "/"), "s")
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)
	if len(m.rows) < 2 {
		t.Fatalf("setup: rows = %d, want a few matches", len(m.rows))
	}

	m = press(m, "G")
	if m.cursor != len(m.rows)-1 {
		t.Errorf("cursor = %d, want the last match", m.cursor)
	}
}

func TestGIsALetterWhileAFilterIsBeingTyped(t *testing.T) {
	m := press(narrowed(manyProjects(90, 14)), "/")
	m = typeFilter(m, "g")

	if m.filter != "g" {
		t.Errorf("filter = %q, want the g typed into it", m.filter)
	}
	if m.pendingG {
		t.Error("a g in a filter is a letter, not the start of a motion")
	}
}

func TestTheKeysListTheEnds(t *testing.T) {
	f := keysOf()
	for _, key := range []string{"gg · G", "top · bottom"} {
		if !strings.Contains(f, key) {
			t.Errorf("keys = %q, want %q listed", f, key)
		}
	}
}

// --- acting on what the filter found -------------------------------------

func TestCtrlNAndCtrlPMoveWhileTyping(t *testing.T) {
	m := typeFilter(press(narrowed(manyProjects(90, 14)), "/"), "s")
	if len(m.rows) < 2 {
		t.Fatalf("setup: rows = %d, want a few matches", len(m.rows))
	}
	if m.cursor != 0 {
		t.Fatal("setup: expected to start at the top")
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl})
	m = next.(model)
	if m.cursor != 1 {
		t.Errorf("cursor = %d, want ctrl+n to move down", m.cursor)
	}
	if !m.typing {
		t.Error("moving should not end the looking up")
	}

	next, _ = m.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	if got := next.(model).cursor; got != 0 {
		t.Errorf("cursor = %d, want ctrl+p to move back up", got)
	}
}

func TestTypingStillNarrowsAfterMoving(t *testing.T) {
	m := typeFilter(press(narrowed(manyProjects(90, 14)), "/"), "c")
	next, _ := m.Update(tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl})
	m = typeFilter(next.(model), "onn")

	if m.filter != "conn" {
		t.Errorf("filter = %q, want the letters to have gone on narrowing it", m.filter)
	}
	wantRows(t, navColumn(m), []string{"▸ conn"})
}

func TestLettersAreStillLettersWhileTyping(t *testing.T) {
	// The actions are on chords because a project called "scratch" has to
	// be typeable without s, r and a doing things.
	m := typeFilter(press(narrowed(manyProjects(90, 14)), "/"), "sarx")
	if m.filter != "sarx" {
		t.Errorf("filter = %q, want every letter typed into it", m.filter)
	}
	if len(m.terms) != 0 {
		t.Error("no letter should have started anything")
	}
}

func TestTheFilterPromptIsTheFoot(t *testing.T) {
	m := typeFilter(press(narrowed(manyProjects(160, 24)), "/"), "b")
	if got := footer(m); got != "/b█" {
		t.Errorf("footer = %q, want the prompt alone", got)
	}
}

// --- the pid, and where a process is listening ---------------------------

func TestThePidIsOnlyShownWhenEveryProcessIs(t *testing.T) {
	// Folded, the list is about what is happening and the pid is a number
	// beside every row that never helps you read it.
	m := withProcList(80, 12,
		[]Project{{Name: "conn", Path: "/p/conn"}},
		[]Proc{
			{PID: 10, PPID: 1, Command: "zsh", Dir: "/p/conn"},
			{PID: 20, PPID: 10, Command: "nvim", Dir: "/p/conn"},
		})
	wantRows(t, navColumn(m), []string{"▸ conn", "      nvim"})

	m = press(m, "-")
	wantRows(t, navColumn(m), []string{"▸ conn", "      zsh 10", "        nvim 20"})
}

func TestTwoOfTheSameCommandAreStillToldApartUnfolded(t *testing.T) {
	// Which is the point of the pid: unfolded it is what tells them apart.
	m := withProcList(80, 12,
		[]Project{{Name: "conn", Path: "/p/conn"}},
		[]Proc{
			{PID: 10, PPID: 1, Command: "nvim", Dir: "/p/conn"},
			{PID: 11, PPID: 1, Command: "nvim", Dir: "/p/conn"},
		})
	m = press(m, "-")
	wantRows(t, navColumn(m), []string{"▸ conn", "      nvim 10", "      nvim 11"})
}

func TestPortsAreOrderedByNumber(t *testing.T) {
	got := []string{"8080", "80", "443"}
	sortPorts(got)
	want := []string{"80", "443", "8080"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ports = %v, want %v", got, want)
		}
	}
}

func TestASpaceInTheSearchDoesNotMoveTheCursor(t *testing.T) {
	// A filter is trimmed before it is matched against, so a space narrows
	// nothing. Sending the selection back to the top for one is the cursor
	// jumping in the middle of a name being typed.
	m := withProcList(90, 20, []Project{
		{Name: "alpha", Path: "/p/alpha"},
		{Name: "vim pro", Path: "/p/vim pro"},
		{Name: "vim proxy", Path: "/p/vim proxy"},
	}, nil)

	m = press(m, "/")
	for _, r := range "vim" {
		next, _ := m.Update(typed(string(r)))
		m = next.(model)
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(model)

	was, _ := m.selected()
	rows := len(m.rows)

	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	m = next.(model)

	if len(m.rows) != rows {
		t.Fatalf("the space changed the list from %d rows to %d", rows, len(m.rows))
	}
	if got, _ := m.selected(); got.project.Path != was.project.Path {
		t.Errorf("the space moved the cursor from %q to %q",
			was.project.Name, got.project.Name)
	}

	// A letter still does start again from the top: those rows really have
	// changed under the cursor.
	next, _ = m.Update(typed("p"))
	if m = next.(model); m.cursor != 0 {
		t.Errorf("cursor = %d after narrowing the list, want the top", m.cursor)
	}
}

func TestALostServerClearsTheShells(t *testing.T) {
	// The server hanging up is the ordinary end of holding nothing: the last
	// shell closed and the session went with it. The window only stops
	// showing what is no longer held; the session watches for a new server on
	// its own.
	m := sized(90, 14)
	m.terms = map[int]*remoteTerm{700: {pid: 700}}
	next, _ := m.Update(serverLostMsg{})
	got := next.(model)
	if len(got.terms) != 0 {
		t.Errorf("terms = %d; want the held shells cleared", len(got.terms))
	}
	if got.status != "" {
		t.Errorf("status = %q, want a clean loss to say nothing", got.status)
	}
}

func TestAFailedConnectIsRetried(t *testing.T) {
	// Not reaching the server leaves nothing to wait on but the retry itself.
	_, cmd := sized(90, 14).Update(serverReadyMsg{err: errors.New("no server")})
	if cmd == nil {
		t.Fatal("no command, want another attempt scheduled")
	}
}

func TestTheChaseBacksOffToACap(t *testing.T) {
	// The one failure retried from the model is tmux itself missing; the
	// retries slow down rather than hammering.
	m := sized(90, 14)
	for range 12 {
		next, _ := m.Update(serverReadyMsg{err: errors.New("tmux is not installed")})
		m = next.(model)
	}
	if m.backoff != reconnectMax {
		t.Errorf("backoff = %v after many failures, want capped at %v", m.backoff, reconnectMax)
	}
}

func TestAServerTalkingResetsTheChase(t *testing.T) {
	m := sized(90, 14)
	m.backoff = reconnectMax
	next, _ := m.Update(sessionsMsg{})
	if got := next.(model).backoff; got != 0 {
		t.Errorf("backoff = %v after the server spoke, want the chase dropped", got)
	}
}

func TestOnlyOneScanIsEverOut(t *testing.T) {
	// On a machine where lsof stalls, a poll that kept asking would pile a
	// stalled scan on top of every tick.
	m := sized(80, 8)
	if m.scanPoll() == nil {
		t.Fatal("an idle model should scan on the tick")
	}
	if m.scanPoll() != nil {
		t.Error("a second poll started a scan behind the first")
	}
	if m.rescan {
		t.Error("the poll owes nothing: a scan already out is answer enough")
	}
}

func TestAScanAskedForDuringOneIsOwed(t *testing.T) {
	// The scan already out began before the event that is asking, so its
	// answer cannot carry it.
	m := sized(80, 8)
	if m.scanNow() == nil {
		t.Fatal("an idle model should scan at once")
	}
	if m.scanNow() != nil {
		t.Error("a second ask started a scan behind the first")
	}
	if !m.rescan {
		t.Fatal("an event's ask during a scan should be owed")
	}

	next, _ := m.Update(procsMsg{})
	got := next.(model)
	if !got.scanning {
		t.Error("the owed scan should have gone out with the answer's arrival")
	}
	if got.rescan {
		t.Error("the debt should be settled by paying it")
	}
}

func TestAScanFailureKeepsTheLastList(t *testing.T) {
	// A failed scan says nothing about what is running; blanking the tree on
	// every hiccup of a loaded machine would be flicker, not information.
	m := withProcs(80, 8, []Project{{Name: "alpha", Path: "/p/alpha"}}, []string{"/p/alpha"})
	next, _ := m.Update(procsMsg{err: errors.New("lsof timed out")})
	got := next.(model)

	if len(got.procs) != 1 {
		t.Errorf("procs = %d, want the last good list kept", len(got.procs))
	}
	if !got.statusErr || got.status == "" {
		t.Error("a scan failure should be reported, not shown as an empty machine")
	}
}

func TestASlowListingIsCutOff(t *testing.T) {
	if _, err := listing(50*time.Millisecond, "sleep", "5"); err == nil {
		t.Error("a command that outlives the timeout should come back an error")
	}
}

// subbed is a model whose one repository holds two sub-projects, showing all.
func subbed(procs ...Proc) model {
	m := sized(90, 20)
	m.showAll = true
	m.projects = []Project{{Name: "mono", Path: "/p/mono"}}
	m.subs = map[string][]Project{"/p/mono": {
		{Name: "services/api", Path: "/p/mono/services/api"},
		{Name: "web", Path: "/p/mono/web"},
	}}
	m.procs = procs
	m.rebuild()
	return m
}

func TestProcessesFileUnderTheirSubProject(t *testing.T) {
	// A monorepo is a projects directory that happens to be one repository:
	// work inside a sub-project is listed there, not in a heap at the root.
	m := subbed(
		Proc{PID: 100, PPID: 1, Command: "node", Dir: "/p/mono/services/api"},
		Proc{PID: 101, PPID: 1, Command: "make", Dir: "/p/mono"},
	)
	wantRows(t, navColumn(m), []string{
		"▸ mono",
		"      make",
		"  mono/services/api",
		"      node",
		"  mono/web",
	})
}

func TestIdleSubProjectsAreBehindTheDot(t *testing.T) {
	// The repositories' own rule: what has work shows, the rest waits.
	m := narrowed(subbed(Proc{PID: 100, PPID: 1, Command: "node", Dir: "/p/mono/services/api"}))
	wantRows(t, navColumn(m), []string{
		"▸ mono",
		"  mono/services/api",
		"      node",
	})
	for _, row := range navColumn(m) {
		if strings.Contains(row, "web") {
			t.Errorf("row %q shows an idle sub-project in the narrowed view", row)
		}
	}
}

func TestTheFilterReachesAnIdleSubProject(t *testing.T) {
	// The cold start at work: nothing is running, and /api still lands you
	// somewhere you can press s or r.
	m := typeFilter(press(subbed(), "/"), "api")
	wantRows(t, navColumn(m), []string{
		"  mono",
		"▸ mono/services/api",
	})
	for _, row := range navColumn(m) {
		if strings.Contains(row, "web") {
			t.Errorf("row %q does not answer to the query", row)
		}
	}
}

func TestAnEmptyQueryListsProjectsAlone(t *testing.T) {
	// Every sub-project of every repository would bury the list of names
	// being remembered.
	m := press(subbed(), "/")
	for _, row := range navColumn(m) {
		if strings.Contains(row, "services") || strings.Contains(row, "web") {
			t.Errorf("row %q lists a sub-project before anything was typed", row)
		}
	}
}

func TestAShellOnASubProjectStartsThere(t *testing.T) {
	r := navRow{kind: rowSub, project: Project{Name: "services/api", Path: "/p/mono/services/api"}}
	if got := (model{}).shellDir(r); got != "/p/mono/services/api" {
		t.Errorf("shellDir = %q, want the sub-project's own directory", got)
	}
}

// cursorOn puts the cursor on the row for a path, failing if there is none.
func cursorOn(t *testing.T, m model, path string) model {
	t.Helper()
	for i, r := range m.rows {
		if r.kind != rowProc && r.project.Path == path {
			m.cursor = i
			return m
		}
	}
	t.Fatalf("no row for %q", path)
	return m
}

func TestXOnASubProjectTakesOnlyItsProcesses(t *testing.T) {
	m := subbed(
		Proc{PID: 100, PPID: 1, Command: "node", Dir: "/p/mono/services/api"},
		Proc{PID: 101, PPID: 1, Command: "make", Dir: "/p/mono"},
	)
	m = press(cursorOn(t, m, "/p/mono/services/api"), "x")

	if m.pendingKill == nil {
		t.Fatal("x on a sub-project should arm a kill")
	}
	if len(m.pendingKill.nodes) != 1 || m.pendingKill.nodes[0].PID != 100 {
		t.Errorf("nodes = %+v, want the sub-project's process alone", m.pendingKill.nodes)
	}
	if !strings.Contains(m.pendingKill.subject, "services/api") {
		t.Errorf("subject = %q, want it named for the sub-project", m.pendingKill.subject)
	}
}

func TestXOnTheRepositoryTakesSubProcessesToo(t *testing.T) {
	// The sub-projects are on screen beneath it; an x that ignored them
	// would be an x ignoring half of what is shown.
	m := subbed(
		Proc{PID: 100, PPID: 1, Command: "node", Dir: "/p/mono/services/api"},
		Proc{PID: 101, PPID: 1, Command: "make", Dir: "/p/mono"},
	)
	m = press(cursorOn(t, m, "/p/mono"), "x")

	if m.pendingKill == nil || len(m.pendingKill.nodes) != 2 {
		t.Fatalf("pendingKill = %+v, want both processes", m.pendingKill)
	}
}

func TestWorkInASubProjectKeepsItsRepositoryListed(t *testing.T) {
	m := narrowed(subbed(Proc{PID: 100, PPID: 1, Command: "node", Dir: "/p/mono/services/api"}))
	if len(m.rows) == 0 || m.rows[0].project.Name != "mono" {
		t.Fatalf("rows = %d, want the repository listed for its sub's work", len(m.rows))
	}
}

// groupedModel is a model with one group of two repositories and one
// repository standing alone, showing all.
func groupedModel(procs ...Proc) model {
	m := sized(90, 20)
	m.showAll = true
	m.projects = []Project{
		{Name: "api", Path: "/p/checklists.org/api", Group: "/p/checklists.org"},
		{Name: "web", Path: "/p/checklists.org/web", Group: "/p/checklists.org"},
		{Name: "conn", Path: "/p/conn"},
	}
	m.groups = []Project{{Name: "checklists.org", Path: "/p/checklists.org"}}
	m.procs = procs
	m.rebuild()
	return m
}

func TestAGroupHoldsItsRepositories(t *testing.T) {
	// A project is often several repositories in one folder, worked on at
	// that level; each repository is a heading named for the folder and
	// itself, and the folder is a heading only for work at its own level.
	m := groupedModel()
	wantRows(t, navColumn(m), []string{
		"▸ checklists.org/api",
		"  checklists.org/web",
		"  conn",
	})
}

func TestWorkInARepositoryLiftsItsGroupIntoView(t *testing.T) {
	m := narrowed(groupedModel(Proc{PID: 100, PPID: 1, Command: "node", Dir: "/p/checklists.org/api"}))
	wantRows(t, navColumn(m), []string{
		"▸ checklists.org/api",
		"      node",
	})
	for _, row := range navColumn(m) {
		if strings.Contains(row, "web") || strings.Contains(row, "conn") {
			t.Errorf("row %q has no work and should not be listed", row)
		}
	}
}

func TestAShellAtTheGroupLevelBelongsToTheGroup(t *testing.T) {
	// Working at that level means a shell opened there, in none of the
	// repositories; it belongs to the group row.
	m := narrowed(groupedModel(Proc{PID: 100, PPID: 1, Command: "zsh", Dir: "/p/checklists.org"}))
	wantRows(t, navColumn(m), []string{
		"▸ checklists.org",
		"      zsh",
	})
}

func TestTheFilterFindsTheGroupByName(t *testing.T) {
	m := typeFilter(press(groupedModel(), "/"), "check")
	rows := navColumn(m)
	if len(rows) == 0 || !strings.Contains(rows[0], "checklists.org") {
		t.Fatalf("rows = %v, want the group found by its name", rows)
	}
	for _, row := range rows {
		if strings.Contains(row, "conn") {
			t.Errorf("row %q does not answer to the query", row)
		}
	}
}

func TestXOnAGroupTakesEverythingInIt(t *testing.T) {
	m := groupedModel(
		Proc{PID: 100, PPID: 1, Command: "node", Dir: "/p/checklists.org/api"},
		Proc{PID: 101, PPID: 1, Command: "vite", Dir: "/p/checklists.org/web"},
		Proc{PID: 102, PPID: 1, Command: "zsh", Dir: "/p/checklists.org"},
	)
	m = press(cursorOn(t, m, "/p/checklists.org"), "x")

	if m.pendingKill == nil || len(m.pendingKill.nodes) != 3 {
		t.Fatalf("pendingKill = %+v, want all three processes in the group", m.pendingKill)
	}
	if !strings.Contains(m.pendingKill.subject, "checklists.org") {
		t.Errorf("subject = %q, want it named for the group", m.pendingKill.subject)
	}
}

func TestAShellOnAGroupRowStartsAtTheGroup(t *testing.T) {
	r := navRow{kind: rowGroup, project: Project{Name: "checklists.org", Path: "/p/checklists.org"}}
	if got := (model{}).shellDir(r); got != "/p/checklists.org" {
		t.Errorf("shellDir = %q, want the group's own directory", got)
	}
}

func TestLandingOnAShellLeavesItWhereItIsAndSaysWhatItIs(t *testing.T) {
	// With the navigator focused, the navigator draws the pane beside it:
	// landing on a held shell's row says what is known about the row and
	// asks nothing of the shell. A shell is only placed beside the
	// navigator to be entered.
	m := withProcList(90, 14,
		[]Project{{Name: "tmp", Path: "/tmp"}},
		[]Proc{{PID: 700, PPID: 1, Command: "zsh", Dir: "/tmp"}})
	m.terms = map[int]*remoteTerm{700: {pid: 700}}
	m, asked := pipeServer(t, m)

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown}) // onto the shell's row
	m = next.(model)
	select {
	case got := <-asked:
		if got.Kind == kindShow || got.Kind == kindFocus || got.Kind == kindPark {
			t.Errorf("a glance at the shell's row arranged the pane: %+v", got)
		}
	case <-time.After(50 * time.Millisecond):
	}
	if m.shown != 0 {
		t.Errorf("shown = %d, want nothing under the tabline", m.shown)
	}
}

func TestEnterOnAShownShellFocusesIt(t *testing.T) {
	m := withProcList(90, 14,
		[]Project{{Name: "tmp", Path: "/tmp"}},
		[]Proc{{PID: 700, PPID: 1, Command: "zsh", Dir: "/tmp"}})
	m.terms = map[int]*remoteTerm{700: {pid: 700}}
	m, asked := pipeServer(t, m)

	m = press(press(m, "down"), "enter")
	if got := askedForKind(t, asked, kindFocus); got.PID != 700 {
		t.Fatalf("asked %+v, want focus taken to shell 700", got)
	}
}

func TestJAndKStepThroughTheHeldShellsInOrder(t *testing.T) {
	// The chord ctrl-space j presses J here: the next held shell in the
	// navigator's order from the one shown, wrapping, focused.
	// The order is the places' order, whatever is folded or filtered.
	m := withProcList(90, 14,
		[]Project{{Name: "alpha", Path: "/p/alpha"}, {Name: "beta", Path: "/p/beta"}},
		[]Proc{
			{PID: 700, PPID: 1, Command: "zsh", Dir: "/p/beta"},
			{PID: 701, PPID: 1, Command: "zsh", Dir: "/p/alpha"},
		})
	m.terms = map[int]*remoteTerm{700: {pid: 700, dir: "/p/beta"}, 701: {pid: 701, dir: "/p/alpha"}}
	m, asked := pipeServer(t, m)

	if got := m.heldOrder(); len(got) != 2 || got[0] != 701 || got[1] != 700 {
		t.Fatalf("heldOrder = %v, want alpha's shell before beta's", got)
	}
	for i, want := range []int{701, 700, 701} {
		m = press(m, "J")
		if got := askedForKind(t, asked, kindFocus); got.PID != want {
			t.Errorf("J %d took focus to %d, want %d", i+1, got.PID, want)
		}
		if r, ok := m.selected(); !ok || !r.holds(want) {
			t.Errorf("J %d left the cursor on %+v, want the shell %d", i+1, r, want)
		}
	}
	m = press(m, "K")
	if got := askedForKind(t, asked, kindFocus); got.PID != 700 {
		t.Errorf("K took focus to %d, want back to 700", got.PID)
	}
}

func TestJStepsDownTheListWhateverTheShellsPids(t *testing.T) {
	// A place's rows are ordered by the name each shows, not by pid: three
	// shells running b, a and c, in pid order, are listed a, b, c. J from
	// the top of the list is the row below it, not the next pid.
	m := withProcList(90, 20,
		[]Project{{Name: "tmp", Path: "/tmp"}},
		[]Proc{
			{PID: 700, PPID: 1, Command: "zsh", Dir: "/tmp"},
			{PID: 710, PPID: 700, Command: "b", Dir: "/tmp"},
			{PID: 701, PPID: 1, Command: "zsh", Dir: "/tmp"},
			{PID: 711, PPID: 701, Command: "a", Dir: "/tmp"},
			{PID: 702, PPID: 1, Command: "zsh", Dir: "/tmp"},
			{PID: 712, PPID: 702, Command: "c", Dir: "/tmp"},
		})
	m.terms = map[int]*remoteTerm{
		700: {pid: 700, dir: "/tmp"}, 701: {pid: 701, dir: "/tmp"}, 702: {pid: 702, dir: "/tmp"},
	}
	m, asked := pipeServer(t, m)

	if got := m.heldOrder(); !slices.Equal(got, []int{701, 700, 702}) {
		t.Fatalf("heldOrder = %v, want the list's order a, b, c", got)
	}
	m.shown = 701
	for i, want := range []int{700, 702, 701} {
		m = press(m, "J")
		if got := askedForKind(t, asked, kindFocus); got.PID != want {
			t.Errorf("J %d took focus to %d, want %d", i+1, got.PID, want)
		}
	}
	m = press(m, "K")
	if got := askedForKind(t, asked, kindFocus); got.PID != 702 {
		t.Errorf("K took focus to %d, want back up to 702", got.PID)
	}
}

func TestJWithNoShellOpenSaysSo(t *testing.T) {
	m, _ := pipeServer(t, repoModel())
	m = press(m, "J")
	if f := footer(m); !strings.Contains(f, "no buffer is open") {
		t.Errorf("footer = %q, want it said that there is nothing to step to", f)
	}
}

func TestAShellAChordOpenedIsShownWhenItIsListed(t *testing.T) {
	// A chord opens a shell in a window named for wanting it shown; the
	// navigator sees the name in the listing and shows the shell the way
	// it shows one it opened: keys in it, cursor to follow.
	m := withProcList(90, 14, []Project{{Name: "tmp", Path: "/tmp"}}, nil)
	m, asked := pipeServer(t, m)
	m.server.panes[700] = &pane{id: "%700", pid: 700, dir: "/tmp"}
	m.server.byPane["%700"] = 700

	next, _ := m.Update(sessionsMsg{sessions: []sessionInfo{{PID: 700, Dir: "/tmp", Wanted: true}}})
	m = next.(model)
	if got := askedForKind(t, asked, kindFocus); got.PID != 700 {
		t.Fatalf("asked %+v, want focus taken to the wanted shell", got)
	}
	if m.wantCursor != 700 || m.shown != 700 {
		t.Errorf("wantCursor = %d, shown = %d, want both on 700", m.wantCursor, m.shown)
	}
}

func TestANavigatorStartingUnderAShownBufferKeepsIt(t *testing.T) {
	// The last navigator went with a buffer shown under it; the next one
	// keeps it shown — the session restores as it was left — and the
	// cursor begins on its row rather than wherever the list happens to
	// start.
	m := withProcList(90, 14,
		[]Project{{Name: "tmp", Path: "/tmp"}},
		[]Proc{{PID: 700, PPID: 1, Command: "zsh", Dir: "/tmp"}})
	m, asked := pipeServer(t, m)
	m.server.panes[700] = &pane{id: "%700", pid: 700, dir: "/tmp", shown: true}
	m.server.byPane["%700"] = 700

	next, _ := m.Update(sessionsMsg{sessions: []sessionInfo{{PID: 700, Dir: "/tmp", Shown: true}}})
	m = next.(model)
	if m.shown != 700 || m.height != chromeRows {
		t.Errorf("shown = %d at %d rows, want the buffer kept under the chrome", m.shown, m.height)
	}
	if r, ok := m.selected(); !ok || !r.holds(700) {
		t.Errorf("cursor on %+v, want the shell's row", r)
	}
	select {
	case got := <-asked:
		if got.Kind == kindShow || got.Kind == kindFocus {
			t.Errorf("the shell found shown was entered: %+v", got)
		}
	case <-time.After(50 * time.Millisecond):
	}
}

func TestThePickerDrawsInTheNavigatorsOwnPane(t *testing.T) {
	// The picker draws where the details do, in the navigator's own pane:
	// opening and closing it moves no shell.
	m := withProcList(90, 14,
		[]Project{{Name: "tmp", Path: "/tmp"}},
		[]Proc{{PID: 700, PPID: 1, Command: "zsh", Dir: "/tmp"}})
	m.terms = map[int]*remoteTerm{700: {pid: 700}}
	m, asked := pipeServer(t, m)
	m = press(press(press(m, "down"), "A"), "esc")
	select {
	case got := <-asked:
		if got.Kind == kindShow || got.Kind == kindPark || got.Kind == kindFocus {
			t.Errorf("the picker moved a shell: %+v", got)
		}
	case <-time.After(50 * time.Millisecond):
	}
}

func TestTheJumpToAWaitingAgentLeavesTheFilter(t *testing.T) {
	// The chord means "from anywhere". While typing, the rows are the
	// query's answers — places alone until a query lands — and a waiting
	// agent used to be invisible to it: the chord said nothing was owed
	// while a diamond stood in plain sight.
	m := withClaude("claude", map[int]claudeSession{
		700: {PID: 700, Name: "conn-1f", Status: waitingStatus, WaitingFor: "permission prompt"},
	})
	m = press(m, "/")

	m = press(m, "tab")
	if m.status == "nothing needs you" {
		t.Error("the chord searched only the filter's answers")
	}
	if m.typing {
		t.Error("the jump should be the end of looking")
	}
}

func TestTheCursorRidesOutATransientChild(t *testing.T) {
	// The scan can catch a brand-new shell mid-startup with a transient
	// child — an rc-init command — which names its run for one scan. The
	// cursor lands there (the run holds the wanted shell), and when the
	// child exits the run renames itself back to the shell. The cursor has
	// to ride the rename, not strand on whatever slides into the old row's
	// place.
	procs := []Proc{
		{PID: 200, PPID: 1, Command: "zsh", Dir: "/p/repo"},
		{PID: 201, PPID: 200, Command: "sleep", Argv: "sleep 400", Dir: "/p/repo"},
	}
	m := withProcList(96, 20, []Project{{Name: "repo", Path: "/p/repo"}}, procs)
	m.terms = map[int]*remoteTerm{200: {pid: 200, dir: "/p/repo"}, 901: {pid: 901, dir: "/p/repo"}}
	m.wantCursor = 901 // the shell just opened, as termOpenedMsg leaves it
	m.rebuild()

	// The scan catches the new shell with its rc-init child; the run is
	// named for the child, and the want lands on it.
	next, _ := m.Update(procsMsg{procs: append(procs,
		Proc{PID: 901, PPID: 1, Command: "zsh", Dir: "/p/repo"},
		Proc{PID: 902, PPID: 901, Command: "stat", Dir: "/p/repo"})})
	m = next.(model)
	if r, ok := m.selected(); !ok || !r.holds(901) {
		t.Fatalf("setup: cursor should have landed on the new shell's run")
	}

	// The child exits; the run is a bare shell again.
	next, _ = m.Update(procsMsg{procs: append(procs,
		Proc{PID: 901, PPID: 1, Command: "zsh", Dir: "/p/repo"})})
	m = next.(model)
	if r, ok := m.selected(); !ok || r.kind != rowProc || !r.holds(901) {
		t.Errorf("cursor stranded off the shell when its transient child exited")
	}
}

func TestAShellsWindowShowsItsPlaceAndItsAgentsMark(t *testing.T) {
	// tmux draws the status line, and it shows window names; the navigator
	// names each shell's window for the place and what is running there,
	// and gives it its row's mark, so the line reads as the list's leaves
	// from any shell.
	m := withProcList(96, 14,
		[]Project{{Name: "conn", Path: "/p/conn"}},
		[]Proc{
			{PID: 700, PPID: 1, Command: "zsh", Dir: "/p/conn"},
			{PID: 701, PPID: 700, Command: "claude", Dir: "/p/conn"},
		})
	m.terms = map[int]*remoteTerm{700: {pid: 700, dir: "/p/conn"}}
	m, asked := pipeServer(t, m)

	next, _ := m.Update(agentsMsg{agents: asAgents(map[int]claudeSession{
		701: {PID: 701, Status: waitingStatus, WaitingFor: "permission prompt"},
	})})
	m = next.(model)

	if got := askedForKind(t, asked, kindDress); got.PID != 700 || got.Name != "conn: claude ◆" {
		t.Errorf("dressed %+v, want pane 700 named for its place and claude, marked ◆", got)
	}

	// The same state again says nothing: what tmux has is what it would
	// be told.
	next, _ = m.Update(agentsMsg{agents: m.agents})
	m = next.(model)
	select {
	case again := <-asked:
		if again.Kind == kindDress {
			t.Errorf("dressed again with nothing changed: %+v", again)
		}
	case <-time.After(50 * time.Millisecond):
	}
}

func TestTheStatusLineIsToldTheModeOnceWhenItChanges(t *testing.T) {
	m := withProcList(90, 14,
		[]Project{{Name: "tmp", Path: "/tmp"}},
		[]Proc{{PID: 700, PPID: 1, Command: "zsh", Dir: "/tmp"}})
	m, asked := pipeServer(t, m)

	// Opening the filter puts the query, empty, in the mode's place. The
	// mode and the message are said together; both are taken.
	next, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = next.(model)
	if got := askedForKind(t, asked, kindMode); stripTmux(got.Name) != " /█ " {
		t.Fatalf("mode = %q, want the empty query being typed", stripTmux(got.Name))
	}
	askedForKind(t, asked, kindMsg)

	// A refresh that changes nothing says nothing.
	next, _ = m.Update(agentsMsg{agents: m.agents})
	m = next.(model)
	select {
	case again := <-asked:
		if again.Kind == kindMode || again.Kind == kindMsg || again.Kind == kindNeed {
			t.Errorf("the status line was told again with nothing changed: %+v", again)
		}
	case <-time.After(50 * time.Millisecond):
	}

	// Typing changes it; the letter is said.
	next, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = next.(model)
	if got := askedForKind(t, asked, kindMode); stripTmux(got.Name) != " /t█ " {
		t.Errorf("mode = %q, want the letter typed", stripTmux(got.Name))
	}
}

func TestFocusLeavingForAShellDisarmsTheKillAndKeepsTheCursorLit(t *testing.T) {
	m := withProcList(90, 14,
		[]Project{{Name: "tmp", Path: "/tmp"}},
		[]Proc{{PID: 700, PPID: 1, Command: "zsh", Dir: "/tmp"}})
	m.terms = map[int]*remoteTerm{700: {pid: 700, dir: "/tmp"}}
	m = press(m, "down")
	m = press(m, "x")
	if m.pendingKill == nil {
		t.Fatal("x should arm a kill")
	}
	row, _ := m.selected()
	bar := "48;2;42;38;32" // the chip's ground under the cursor's row
	if got := m.renderRow(row, true); !strings.Contains(got, bar) {
		t.Error("with focus here the cursor row should be on the bar")
	}

	// The keys go to a shell: the kill's second key is not coming, so the
	// kill is not waiting for it. The cursor row stays lit — it is the
	// shell shown beside the list, and dim would read as out of reach.
	next, _ := m.Update(tea.BlurMsg{})
	m = next.(model)
	if m.pendingKill != nil {
		t.Error("a kill left armed across a blur would fire on the first letter typed back")
	}
	if f := footer(m); strings.Contains(f, "kill") {
		t.Errorf("footer = %q, want the prompt gone with the kill", f)
	}
	if got := m.renderRow(row, true); !strings.Contains(got, bar) {
		t.Error("with focus in a shell the cursor row should stay on the bar")
	}
}

func TestAPlannedShellIsNamedByItsPlanWhateverItRuns(t *testing.T) {
	m := withProcList(96, 14,
		[]Project{{Name: "web", Path: "/p/web"}},
		[]Proc{{PID: 700, PPID: 1, Command: "zsh", Dir: "/p/web"}})
	m.terms = map[int]*remoteTerm{700: {pid: 700, dir: "/p/web", name: "dev"}}
	name, mark := m.shellLabel(700, m.terms[700])
	if name != "web: dev" || mark != "" {
		t.Errorf("label = %q %q, want the plan's name and no mark", name, mark)
	}

	m.procs = append(m.procs, Proc{PID: 701, PPID: 700, Command: "node", Argv: "npm run dev", Dir: "/p/web"})
	m.rebuild()
	if name, _ := m.shellLabel(700, m.terms[700]); name != "web: dev" {
		t.Errorf("label = %q, want the plan's name still, whatever runs in it", name)
	}
}

func TestJReachesAShellOutsideEveryPlace(t *testing.T) {
	// A shell opened somewhere no project holds has no row, but it is held
	// and shown like any other: J steps to it, and focus goes to it.
	m := withProcList(90, 14,
		[]Project{{Name: "conn", Path: "/p/conn"}},
		[]Proc{
			{PID: 700, PPID: 1, Command: "zsh", Dir: "/p/conn"},
			{PID: 701, PPID: 1, Command: "zsh", Dir: "/tmp"},
		})
	m.terms = map[int]*remoteTerm{700: {pid: 700, dir: "/p/conn"}, 701: {pid: 701, dir: "/tmp"}}
	m, asked := pipeServer(t, m)
	m = press(m, "down") // the cursor on 700's row, focus still here

	m = press(m, "J") // from the row under the cursor
	if got := askedForKind(t, asked, kindFocus); got.PID != 701 {
		t.Errorf("J took focus to %d, want the shell outside every place, 701", got.PID)
	}

	// The cursor is still on 700's row, having nowhere else to be. The
	// world changing under it — a scan, the server's list — must not read
	// that as the cursor asking for 700 back.
	next, _ := m.Update(procsMsg{procs: m.procs})
	m = next.(model)
	next, _ = m.Update(sessionsMsg{sessions: []sessionInfo{{PID: 700, Dir: "/p/conn"}, {PID: 701, Dir: "/tmp", Shown: true}}})
	m = next.(model)
	select {
	case got := <-asked:
		if got.Kind == kindShow || got.Kind == kindPark {
			t.Errorf("a refresh under a cursor that had not moved rearranged the pane: %+v", got)
		}
	case <-time.After(50 * time.Millisecond):
	}
	if m.shown != 701 {
		t.Errorf("shown = %d, want the shell J showed to stay shown", m.shown)
	}
}

// --- back -----------------------------------------------------------------

// twoShells is a navigator over two places with a held shell in each, wired
// to the recording server: alpha's shell is 701, beta's 700.
func twoShells(t *testing.T) (model, chan message) {
	t.Helper()
	m := withProcList(90, 14,
		[]Project{{Name: "alpha", Path: "/p/alpha"}, {Name: "beta", Path: "/p/beta"}},
		[]Proc{
			{PID: 700, PPID: 1, Command: "zsh", Dir: "/p/beta"},
			{PID: 701, PPID: 1, Command: "zsh", Dir: "/p/alpha"},
		})
	m.terms = map[int]*remoteTerm{700: {pid: 700, dir: "/p/beta"}, 701: {pid: 701, dir: "/p/alpha"}}
	return pipeServer(t, m)
}

func TestBackReturnsToTheShellBeforeThisOne(t *testing.T) {
	// The chord ctrl-space ctrl-space presses shift-tab here. From a shell
	// reached from another shell, back is that other shell, and back again
	// is this one: the last two, however the pane beside the list has been
	// rearranged since.
	m, asked := twoShells(t)
	m = press(m, "J") // to alpha's shell, 701
	askedForKind(t, asked, kindFocus)
	m = press(m, "J") // on to beta's, 700
	askedForKind(t, asked, kindFocus)
	if m.focus != 700 || m.was != 701 {
		t.Fatalf("focus %d, was %d; want focus in 700 from 701", m.focus, m.was)
	}

	m = press(m, "shift+tab")
	if got := askedForKind(t, asked, kindFocus); got.PID != 701 {
		t.Fatalf("back took focus to %d, want the shell before, 701", got.PID)
	}
	m = press(m, "shift+tab")
	if got := askedForKind(t, asked, kindFocus); got.PID != 700 {
		t.Fatalf("back again took focus to %d, want 700", got.PID)
	}
}

func TestBackFromAShellReachedFromTheListIsTheList(t *testing.T) {
	m, asked := twoShells(t)
	m = press(m, "J")
	askedForKind(t, asked, kindFocus)

	// The keys came from the list, so back is the list: its pane, which the
	// fake numbers zero.
	m = press(m, "shift+tab")
	if got := askedForKind(t, asked, kindFocus); got.PID != 0 {
		t.Fatalf("back took focus to %d, want the navigator", got.PID)
	}
	if m.focus != 0 || m.was != 701 {
		t.Errorf("focus %d, was %d; want focus at the list, from 701", m.focus, m.was)
	}
	// And from the list, back is the shell again.
	m = press(m, "shift+tab")
	if got := askedForKind(t, asked, kindFocus); got.PID != 701 {
		t.Fatalf("back took focus to %d, want the shell 701", got.PID)
	}
}

func TestBackBetweenTwoShellsChosenFromTheListSkipsTheList(t *testing.T) {
	// To one shell from the list, back to the list, and on to another
	// shell from there: the navigator was the way from the first shell to
	// the second, not a place focus stopped. Back from the second is
	// the first, and back again the second — however many times.
	m, asked := twoShells(t)
	m = press(m, "J") // to alpha's shell, 701
	askedForKind(t, asked, kindFocus)
	m = press(m, "shift+tab") // to the list
	askedForKind(t, asked, kindFocus)
	m = press(m, "J") // on to beta's, 700, by way of the list
	askedForKind(t, asked, kindFocus)
	if m.focus != 700 || m.was != 701 {
		t.Fatalf("focus %d, was %d; want focus in 700 from 701, the list passed through", m.focus, m.was)
	}

	m = press(m, "shift+tab")
	if got := askedForKind(t, asked, kindFocus); got.PID != 701 {
		t.Fatalf("back took focus to %d, want the shell before the list, 701", got.PID)
	}
	m = press(m, "shift+tab")
	if got := askedForKind(t, asked, kindFocus); got.PID != 700 {
		t.Fatalf("back again took focus to %d, want 700", got.PID)
	}
	// Whereas to the list and back to the same shell is a round trip: back
	// from there is still the list.
	m = press(m, "shift+tab") // to 701
	askedForKind(t, asked, kindFocus)
	m = press(m, "shift+tab") // to 700
	askedForKind(t, asked, kindFocus)
	m = press(m, "shift+tab") // to 701
	askedForKind(t, asked, kindFocus)
	if m.focus != 701 || m.was != 700 {
		t.Fatalf("focus %d, was %d; want focus in 701 from 700", m.focus, m.was)
	}
}

func TestTheMouseMovingFocusCountsAsAMove(t *testing.T) {
	// A click into conn's pane from the buffer under it focuses conn
	// without going through a key: a move back should know about. The
	// buffer stays shown: a dead one answers to conn's keys, and a live
	// one is a click away.
	m, asked := twoShells(t)
	m = press(press(m, "down"), "enter") // into alpha's shell, 701
	askedForKind(t, asked, kindFocus)
	if m.shown != 701 || m.focus != 701 {
		t.Fatalf("shown %d, focus %d; want alpha's shell entered", m.shown, m.focus)
	}
	next, _ := m.Update(tea.FocusMsg{})
	m = next.(model)
	if m.focus != 0 || m.was != 701 {
		t.Fatalf("focus %d, was %d after focus; want conn, from 701", m.focus, m.was)
	}
	notAsked(t, asked, kindPark)
	if m.shown != 701 {
		t.Errorf("shown = %d, want the buffer kept under the tabline", m.shown)
	}
	// The navigator blurring with nothing beside it — the mouse on another
	// window — is not a move into anything.
	next, _ = m.Update(tea.BlurMsg{})
	next, _ = next.(model).Update(tea.FocusMsg{})
	m = next.(model)
	if m.focus != 0 || m.was != 701 {
		t.Errorf("focus %d, was %d; want the list, from 701 still", m.focus, m.was)
	}
}

func TestBackWithNowhereToGoSaysSoOrTakesTheShellUnderTheCursor(t *testing.T) {
	m, asked := twoShells(t)
	// The keys have been nowhere and the cursor is on a place: nothing to do.
	m = press(m, "shift+tab")
	if m.status != "no shell to go back to" {
		t.Errorf("status = %q, want the lack said", m.status)
	}
	// On a shell's row, back with no history is enter.
	m = press(m, "down")
	m = press(m, "shift+tab")
	if got := askedForKind(t, asked, kindFocus); got.PID != 701 {
		t.Fatalf("back took focus to %d, want the shell under the cursor", got.PID)
	}
	// The shell focus came from has gone: back from the list falls to
	// what is shown; back from a shell falls to the list.
	m = press(m, "J") // 700, from 701
	askedForKind(t, asked, kindFocus)
	delete(m.terms, 701)
	m = press(m, "shift+tab")
	if got := askedForKind(t, asked, kindFocus); got.PID != 0 {
		t.Fatalf("back took focus to %d, want the navigator when the last shell is gone", got.PID)
	}
}

// composeTree is a plan's shell — zsh 10, held as app — running docker
// compose up (20), which runs its compose plugin (30).
func composeTree() model {
	m := withProcList(80, 12,
		[]Project{{Name: "conn", Path: "/p/conn"}},
		[]Proc{
			{PID: 10, PPID: 1, Command: "zsh", Dir: "/p/conn", Argv: "zsh"},
			{PID: 20, PPID: 10, Command: "docker", Dir: "/p/conn", Argv: "docker compose up"},
			{PID: 30, PPID: 20, Command: "docker-compose", Dir: "/p/conn", Argv: "docker-compose compose up"},
		},
	)
	m.terms[10] = &remoteTerm{pid: 10, dir: "/p/conn", name: "app"}
	m.rebuild()
	return m
}

// notAsked fails the test if the server is asked for kind within a moment.
func notAsked(t *testing.T, asked chan message, kind string) {
	t.Helper()
	deadline := time.After(50 * time.Millisecond)
	for {
		select {
		case got := <-asked:
			if got.Kind == kind {
				t.Fatalf("the server was asked to %s: %+v", kind, got)
			}
		case <-deadline:
			return
		}
	}
}

func TestKillingAShellRunningACommandSignalsTheCommandFirst(t *testing.T) {
	m := composeTree()
	m.terms[10].name = "" // a shell opened by hand, running compose
	m.rebuild()
	m, asked := pipeServer(t, m)
	m = press(m, "down") // onto the run, named for docker compose up
	m = press(m, "x")
	if f := footer(m); !strings.Contains(f, "CONFIRM") || !strings.Contains(f, "docker compose up") {
		t.Fatalf("footer = %q, want the command asked about", f)
	}

	hungUp, signalled := m.splitKill(m.pendingKill.nodes)
	if len(hungUp) != 0 {
		t.Errorf("hung up %+v, want the shell kept: the buffer is the record", hungUp)
	}
	if got := pids(signalled); !slices.Equal(got, []int{20, 30}) {
		t.Errorf("signalled %v, want what runs in the shell, parents first", got)
	}
	// The shell is never hung up: the command is signalled — hung up first,
	// docker compose up would leave its containers running — and the shell
	// stays at its prompt beneath the transcript, dead but readable.
	notAsked(t, asked, kindClose)
	next, _ := m.Update(procsMsg{procs: []Proc{{PID: 10, PPID: 1, Command: "zsh", Dir: "/p/conn"}}})
	m = next.(model)
	notAsked(t, asked, kindClose)
	if _, held := m.terms[10]; !held {
		t.Error("the shell should still be held")
	}
}

func TestATreeKillOfARunningShellSignalsEachProcessOnce(t *testing.T) {
	m, _ := pipeServer(t, composeTree())
	m = press(press(m, "down"), "X")

	_, signalled := m.splitKill(m.pendingKill.nodes)
	if got := pids(signalled); !slices.Equal(got, []int{20, 30}) {
		t.Errorf("signalled %v, want each process under the shell once", got)
	}
}

func TestKillingTheShellRowItselfSignalsWhatRunsInIt(t *testing.T) {
	m := composeTree()
	m.terms[10].name = ""
	m.rebuild()
	m, asked := pipeServer(t, m)
	m = press(press(m, "-"), "down") // unfolded, the shell has a row of its own
	m = press(m, "x")
	if f := footer(m); !strings.Contains(f, "zsh") {
		t.Fatalf("footer = %q, want the shell asked about", f)
	}

	hungUp, signalled := m.splitKill(m.pendingKill.nodes)
	if len(hungUp) != 0 {
		t.Errorf("hung up %+v, want the shell kept while something runs in it", hungUp)
	}
	if got := pids(signalled); !slices.Equal(got, []int{20, 30}) {
		t.Errorf("signalled %v, want what runs in the shell, parents first", got)
	}
	notAsked(t, asked, kindClose)
}

func TestAShellAtItsPromptIsHungUpAtOnce(t *testing.T) {
	m := withProcList(80, 12,
		[]Project{{Name: "conn", Path: "/p/conn"}},
		[]Proc{{PID: 10, PPID: 1, Command: "zsh", Dir: "/p/conn"}},
	)
	m.terms[10] = &remoteTerm{pid: 10, dir: "/p/conn"}
	m.rebuild()
	m, asked := pipeServer(t, m)
	m = press(press(m, "down"), "x")

	m.splitKill(m.pendingKill.nodes)
	if got := askedForKind(t, asked, kindClose); got.PID != 10 {
		t.Errorf("asked %+v, want the idle shell hung up now", got)
	}
}

func TestAShellWhoseCommandDoesNotExitStaysOpen(t *testing.T) {
	m, asked := pipeServer(t, composeTree())
	m = press(press(m, "down"), "x")
	m.splitKill(m.pendingKill.nodes)
	m.pendingKill = nil

	// The frames pass with the command still listed.
	for range killLinger + 1 {
		m.ageDying()
	}
	next, _ := m.Update(procsMsg{procs: m.procs})
	m = next.(model)
	notAsked(t, asked, kindClose)
	if _, held := m.terms[10]; !held {
		t.Error("the shell should still be held")
	}
}

func TestAShellClosedByHandIsNotWaitedOn(t *testing.T) {
	m, asked := pipeServer(t, composeTree())
	m = press(press(m, "down"), "x")
	m.splitKill(m.pendingKill.nodes)

	next, _ := m.Update(termGoneMsg{pid: 10})
	m = next.(model)
	next, _ = m.Update(procsMsg{procs: nil})
	m = next.(model)
	notAsked(t, asked, kindClose)
}

func TestXOnAnEntryIsNamedForItAndKeepsItsShell(t *testing.T) {
	m, asked := pipeServer(t, composeTree()) // app: zsh 10 running docker compose up
	m = press(press(m, "down"), "x")
	if f := footer(m); !strings.Contains(f, "app") {
		t.Fatalf("footer = %q, want the entry named", f)
	}
	if got := targets(m.pendingKill); !slices.Equal(got, []int{10, 20, 30}) {
		t.Errorf("targets = %v, want the shell and what runs in it", got)
	}
	hungUp, signalled := m.splitKill(m.pendingKill.nodes)
	if len(hungUp) != 0 || len(signalled) != 2 {
		t.Errorf("hung up %v, signalled %v; want the command signalled and the shell kept", hungUp, signalled)
	}
	// The shell stays: the buffer is the record of the ending, and r runs
	// the entry again in it.
	notAsked(t, asked, kindClose)

	// Unfolded, on the shell's own row, the same.
	m = press(press(press(m, "esc"), "-"), "x")
	if f := footer(m); !strings.Contains(f, "app") {
		t.Errorf("footer = %q, want the entry named on its shell's row too", f)
	}
}

func TestXOnAnEndedEntryClosesItsShellByName(t *testing.T) {
	m := withProcList(80, 12,
		[]Project{{Name: "conn", Path: "/p/conn"}},
		[]Proc{{PID: 10, PPID: 1, Command: "zsh", Dir: "/p/conn"}},
	)
	m.terms[10] = &remoteTerm{pid: 10, dir: "/p/conn", name: "app", exit: "1"}
	m.rebuild()
	m, asked := pipeServer(t, m)
	m = press(press(m, "down"), "x")
	if f := footer(m); !strings.Contains(f, "app") {
		t.Fatalf("footer = %q, want the ended entry's shell asked about by name", f)
	}
	m.splitKill(m.pendingKill.nodes)
	if got := askedForKind(t, asked, kindClose); got.PID != 10 {
		t.Errorf("asked %+v, want the shell hung up", got)
	}
}

func TestAgoSaysJustNowForAMomentAgo(t *testing.T) {
	if got := ago(time.Now()); got != "just now" {
		t.Errorf("ago = %q", got)
	}
	if got := ago(time.Now().Add(-3 * time.Minute)); got != "3m ago" {
		t.Errorf("ago = %q", got)
	}
}

// --- choosing the signal ---------------------------------------------------

func TestTheConfirmationOffersTheOtherSignals(t *testing.T) {
	// The plain prompt names the keys that send something other than
	// SIGTERM: a dev server that only tears down on ctrl-c wants SIGINT.
	m := press(press(nestedTree(24), "down"), "x")
	f := strings.Join(bodyRows(m), " ")
	for _, want := range []string{"9 kills outright", "i interrupts", "h hangs up"} {
		if !strings.Contains(f, want) {
			t.Errorf("preview = %q, want it to offer %q", f, want)
		}
	}
}

func TestAKeyAtTheConfirmationChoosesTheSignal(t *testing.T) {
	for key, want := range map[string]syscall.Signal{
		"x": syscall.SIGTERM, "y": syscall.SIGTERM,
		"9": syscall.SIGKILL, "i": syscall.SIGINT, "h": syscall.SIGHUP,
	} {
		armed := press(press(nestedTree(12), "down"), "x")
		req := armed.pendingKill
		next, cmd := armed.Update(typed(key))
		if cmd == nil {
			t.Errorf("%q should confirm the kill", key)
		}
		if next.(model).pendingKill != nil {
			t.Errorf("%q should clear the pending kill", key)
		}
		if got := req.signalOf(); got != want {
			t.Errorf("%q chose %s, want %s", key, signalName(got), signalName(want))
		}
	}
}

func TestTheReportNamesTheSignalSent(t *testing.T) {
	msg := killed("zsh", 10)
	msg.sig = syscall.SIGKILL
	next, _ := nestedTree(12).Update(msg)
	if f := footer(next.(model)); !strings.Contains(f, "sent SIGKILL to zsh 10") {
		t.Errorf("footer = %q, want the signal that went named", f)
	}
}

func TestXOnAProcessOnItsWayOutOffersSIGKILL(t *testing.T) {
	// The process was signalled a moment ago and is still listed. Asking
	// again with SIGTERM would ask it again to do what it has not done.
	m := press(nestedTree(12), "down") // onto zsh 10
	next, _ := m.Update(killed("zsh", 10))
	m = press(next.(model), "x")

	if m.pendingKill == nil || m.pendingKill.signalOf() != syscall.SIGKILL {
		t.Fatalf("pendingKill = %+v, want SIGKILL armed", m.pendingKill)
	}
	if f := footer(m); !strings.Contains(f, "outright") {
		t.Errorf("footer = %q, want the escalation said", f)
	}
	if preview := strings.Join(bodyRows(m), " "); !strings.Contains(preview, "outright") {
		t.Errorf("preview = %q, want the confirm line to say outright", preview)
	}
}

func TestXOnAProcessThatRefusedOffersSIGKILL(t *testing.T) {
	// The marker gave up on it; the process is still there, and refusing.
	m := nestedTree(12)
	for range 4 {
		m = press(m, "down") // onto lint 50
	}
	next, _ := m.Update(killed("lint", 50))
	m = next.(model)
	for range killLinger + 1 {
		m.ageDying()
	}
	if f := footer(m); !strings.Contains(f, "x again kills outright") {
		t.Errorf("footer = %q, want the way past a refusal said", f)
	}

	m = press(m, "x")
	if m.pendingKill == nil || m.pendingKill.signalOf() != syscall.SIGKILL {
		t.Fatalf("pendingKill = %+v, want SIGKILL armed for a process that refused", m.pendingKill)
	}

	// One that has since gone is nothing to escalate on.
	var left []Proc
	for _, p := range m.procs {
		if p.PID != 50 {
			left = append(left, p)
		}
	}
	m.pendingKill = nil
	next, _ = m.Update(procsMsg{procs: left})
	if _, still := next.(model).refused[50]; still {
		t.Error("a process a scan found gone is still held as refusing")
	}
}

func TestXOnAnEntryWhoseCommandRefusedOffersSIGKILL(t *testing.T) {
	// The entry's row is its shell; the signal went to the command under
	// it, and the row is what x is pressed on.
	m, _ := pipeServer(t, composeTree())
	m = press(m, "down")
	r, _ := m.selected()
	command := r.leaf().PID
	next, _ := m.Update(killed("docker", command))
	m = press(next.(model), "x")
	if m.pendingKill == nil || m.pendingKill.signalOf() != syscall.SIGKILL {
		t.Fatalf("pendingKill = %+v, want SIGKILL armed on the entry", m.pendingKill)
	}
}

func TestATreeKillCoversTheProcessGroup(t *testing.T) {
	// vim's group has a member init took in, working in another
	// directory: no tree under conn holds it, and the job does.
	m := withProcList(80, 12,
		[]Project{{Name: "conn", Path: "/p/conn"}},
		[]Proc{
			{PID: 10, PPID: 1, PGID: 10, Command: "zsh", Dir: "/p/conn"},
			{PID: 20, PPID: 10, PGID: 20, Command: "vim", Dir: "/p/conn"},
			{PID: 60, PPID: 1, PGID: 20, Command: "fmt", Dir: "/tmp"},
		},
	)
	m = press(press(m, "down"), "X") // onto the row zsh and vim fold into; kill the tree
	if got, want := targets(m.pendingKill), []int{10, 20, 60}; !slices.Equal(got, want) {
		t.Errorf("targets = %v, want %v: the tree, then the group's straggler", got, want)
	}
	if f := footer(m); !strings.Contains(f, "3 processes") {
		t.Errorf("footer = %q, want the straggler counted", f)
	}

	// A plain x previews the same tree, and takes the one process at it.
	m.pendingKill = nil
	m = press(m, "x")
	if got := pids(m.pendingKill.head); !slices.Equal(got, []int{20}) {
		t.Errorf("x's head = %v, want the row's process alone", got)
	}
}

func TestARowEndedByASignalSaysSo(t *testing.T) {
	// The run said nothing of itself; the exit says it was killed, and
	// the row says that where the summary would go.
	m := withProcList(90, 14, []Project{{Name: "tmp", Path: "/tmp"}},
		[]Proc{{PID: 700, PPID: 1, Command: "zsh", Dir: "/tmp"}})
	m.terms = map[int]*remoteTerm{700: {pid: 700, dir: "/tmp", name: "worker", exit: "137", at: time.Now().Add(-3 * time.Minute)}}
	m.rebuild()
	if row := stripANSI(renderRow(m, m.rows[1])); !strings.Contains(row, "worker · killed · 3m") {
		t.Errorf("row = %q, want the kill said beside the name", row)
	}
}

func TestASettledEndingGoesOnTheRecord(t *testing.T) {
	// The outcome is the ending complete — how, what it said, when — and
	// it is written once, with how long the shell had run, for the pane
	// to list after the shell is gone. A shell opened by hand records
	// nothing.
	stateDir(t)
	m := withProcList(90, 14, []Project{{Name: "tmp", Path: "/tmp"}},
		[]Proc{
			{PID: 700, PPID: 1, Command: "zsh", Dir: "/tmp", Started: time.Now().Add(-42 * time.Second).Format(startLayout)},
			{PID: 800, PPID: 1, Command: "zsh", Dir: "/tmp"},
		})
	at := time.Now().Add(-time.Second)
	m.terms = map[int]*remoteTerm{
		700: {pid: 700, dir: "/tmp", name: "test", exit: "1", at: at},
		800: {pid: 800, dir: "/tmp", exit: "1", at: at},
	}
	m.rebuild()
	m, cmd := m.update(outcomeMsg{pid: 700, summary: "3 failed"})
	for _, msg := range deliver(cmd) {
		if r, ok := msg.(recordedMsg); ok && r.err != nil {
			t.Fatal(r.err)
		}
	}
	got := pastRuns("/tmp", "test", "", 5)
	if len(got) != 1 || got[0].Exit != "1" || got[0].Summary != "3 failed" || !got[0].At.Equal(at) {
		t.Fatalf("runs = %+v, want the ending on the record", got)
	}
	if got[0].Took < 40 || got[0].Took > 45 {
		t.Errorf("took = %v, want the run's length from when its shell began", got[0].Took)
	}
	// Once: the same outcome again adds nothing.
	m, cmd = m.update(outcomeMsg{pid: 700, summary: "3 failed"})
	deliver(cmd)
	if got := pastRuns("/tmp", "test", "", 5); len(got) != 1 {
		t.Errorf("runs = %d, want the ending recorded once", len(got))
	}
	_, cmd = m.update(outcomeMsg{pid: 800, summary: ""})
	deliver(cmd)
	if got := pastRuns("/tmp", "", "", 5); len(got) != 0 {
		t.Errorf("a shell opened by hand recorded %+v, want nothing", got)
	}
}

func TestAPlaceWithSomethingThatNeedsYouRisesAboveTheQuietOnes(t *testing.T) {
	// The list reads from the top, so the top is where what needs you
	// goes: beta's failed run lifts beta above alpha, and when the failure
	// is cleared the places fall back into their alphabetical slots.
	m := withProcList(90, 14,
		[]Project{{Name: "alpha", Path: "/p/alpha"}, {Name: "beta", Path: "/p/beta"}},
		[]Proc{
			{PID: 700, PPID: 1, Command: "zsh", Dir: "/p/alpha"},
			{PID: 701, PPID: 1, Command: "zsh", Dir: "/p/beta"},
		})
	m.terms = map[int]*remoteTerm{
		700: {pid: 700, dir: "/p/alpha"},
		701: {pid: 701, dir: "/p/beta", name: "test", exit: "1"},
	}
	m.rebuild()
	if names := placeNames(m); len(names) != 2 || names[0] != "beta" {
		t.Errorf("places = %v, want beta, whose run failed, above alpha", names)
	}
	m.terms[701].exit = "0"
	m.rebuild()
	if names := placeNames(m); len(names) != 2 || names[0] != "alpha" {
		t.Errorf("places = %v, want the alphabetical order back once nothing needs you", names)
	}
}

func TestARowThatNeedsYouStandsFirstAmongItsSiblings(t *testing.T) {
	// Within a place the rows are by name, except that one needing you
	// steps ahead of the rest: the stopped worker, w by name, is listed
	// before the go test that is fine.
	m := withProcList(90, 14,
		[]Project{{Name: "conn", Path: "/p/conn"}},
		[]Proc{
			{PID: 700, PPID: 1, Command: "go", Argv: "go test ./...", Dir: "/p/conn"},
			{PID: 701, PPID: 1, Command: "node", Argv: "node worker.js", Dir: "/p/conn", State: "T"},
		})
	if r := m.rows[1]; r.kind != rowProc || r.node.PID != 701 {
		t.Errorf("first row under the place is %+v, want the stopped worker", r)
	}
	m.procs[1].State = "S"
	m.rebuild()
	if r := m.rows[1]; r.kind != rowProc || r.node.PID != 700 {
		t.Errorf("first row under the place is %+v, want go test, by name, once nothing needs you", r)
	}
}

// placeNames is the top-level places in the order the list shows them.
func placeNames(m model) []string {
	var names []string
	for _, r := range m.rows {
		if r.kind == rowProject && r.prefix == "" {
			names = append(names, r.project.Name)
		}
	}
	return names
}
func TestTheModeChipCountsWhatIsOwed(t *testing.T) {
	// The status line's mode chip says how many answers are owed, and
	// nothing when none is — the number is there before the list says
	// where, folded or not.
	m := withClaude("claude", map[int]claudeSession{
		700: {PID: 700, Name: "conn-1f", Status: waitingStatus, WaitingFor: "permission prompt"},
	})
	// With a buffer shown; the everything view's chip outranks the count.
	m.all, m.shown = false, 700
	m.keepRows()
	if got := stripTmux(m.statusLine().mode); !strings.Contains(got, "1 OWED") {
		t.Errorf("mode = %q, want the one blocked instance counted", got)
	}
	// The chip reaches the server's option, for the line to read.
	m, asked := pipeServer(t, m)
	m.dressStatus()
	if got := askedForKind(t, asked, kindMode); !strings.Contains(got.Name, "1 OWED") {
		t.Errorf("the server was told %q, want the count", got.Name)
	}
	m.collapsed[detailKey(m.rows[0])] = true
	m.rebuild()
	if got := stripTmux(m.statusLine().mode); !strings.Contains(got, "1 OWED") {
		t.Errorf("mode = %q after folding the place, want the count to stand", got)
	}
	m.agents = asAgents(map[int]claudeSession{700: {PID: 700, Status: "idle"}})
	if got := m.statusLine().mode; got != "" {
		t.Errorf("mode = %q, want nothing said when nothing is owed", got)
	}
}

func TestAWaitingInstanceSaysHowLongItHasWaited(t *testing.T) {
	// A blocked instance's row carries the age of its ask, the way a
	// failed run's carries how long ago it ended: three minutes waiting
	// reads 3m. An instance idle since it started, owed nothing, says
	// no age.
	m := withClaude("claude", map[int]claudeSession{
		700: {PID: 700, Name: "conn-1f", Status: waitingStatus, WaitingFor: "permission prompt", StatusFor: 3 * time.Minute},
	})
	if row := navColumn(m)[1]; !strings.Contains(row, "· 3m") {
		t.Errorf("row = %q, want the ask's age on it", row)
	}
	m.agents = asAgents(map[int]claudeSession{700: {PID: 700, Status: "idle", StatusFor: 3 * time.Minute}})
	if row := navColumn(m)[1]; strings.Contains(row, "3m") {
		t.Errorf("row = %q, want no age on an instance owed nothing", row)
	}
}

func TestAnEntryIsUpWhenItsCommandRunsWhoeverStartedIt(t *testing.T) {
	// Whether dev is up is a fact about the processes in the place, not
	// about the labels on conn's shells: an npm run dev started by hand in
	// a plain shell makes the entry up, with its ports, and r does not
	// start a second beside it.
	dir := t.TempDir()
	if err := writeFile(filepath.Join(dir, ".conn"), "dev: npm run dev\napi: go run .\n"); err != nil {
		t.Fatal(err)
	}
	m := withProcList(90, 14,
		[]Project{{Name: "app", Path: dir}},
		[]Proc{
			{PID: 700, PPID: 1, Command: "zsh", Dir: dir},
			{PID: 701, PPID: 700, Command: "node", Argv: "node /usr/local/bin/npm run dev", Dir: dir, Ports: []string{"5173"}},
		})
	m.terms = map[int]*remoteTerm{700: {pid: 700, dir: dir}}

	states := m.entryStates(dir)
	if st := states["dev"]; st.State != "up" || len(st.Ports) != 1 || st.Ports[0] != "5173" {
		t.Errorf("dev = %+v, want up on the process running its command, with its port", st)
	}
	if st := states["api"]; st.State != "" {
		t.Errorf("api = %+v, want nothing for an entry nothing runs", st)
	}
	if running := m.namesIn(dir, readPlan(dir)); !running["dev"] || running["api"] {
		t.Errorf("running = %v, want dev counted as running and api not", running)
	}
}

func TestAHeldShellKnowsWhatItWasStartedWith(t *testing.T) {
	// The command a shell was started with is recorded on its pane and
	// comes back with the listing: the run's identity, beside the label
	// the plan gave it.
	held, _ := parseListing("%3\t700\t/p/app\tdev\t/p/app\t\t\tshell\t\t\tnode\tnpm run dev\n")
	if len(held) != 1 || held[0].run != "npm run dev" || held[0].info().Run != "npm run dev" {
		t.Errorf("held = %+v, want the command read off the pane", held)
	}
	m := withProcList(90, 14, []Project{{Name: "app", Path: "/p/app"}}, nil)
	next, _ := m.Update(sessionsMsg{sessions: []sessionInfo{{PID: 700, Dir: "/p/app", Name: "dev", Run: "npm run dev"}}})
	m = next.(model)
	if t0 := m.terms[700]; t0 == nil || t0.run != "npm run dev" {
		t.Errorf("term = %+v, want the command kept on the shell", t0)
	}
}

func TestAnEndingAlreadyOnTheRecordIsNeitherReadNorRecordedAgain(t *testing.T) {
	// The transcript's word and the fact of the record live on the pane
	// beside the exit, so a navigator starting beside a settled shell
	// takes them with the listing: the row says 3 failed at once, the
	// transcript is not read again, and the runs file is not appended a
	// second time.
	stateDir(t)
	m := withProcList(90, 14, []Project{{Name: "tmp", Path: "/tmp"}},
		[]Proc{{PID: 700, PPID: 1, Command: "zsh", Dir: "/tmp"}})
	m, asked := pipeServer(t, m)
	m, _ = m.update(sessionsMsg{sessions: []sessionInfo{{PID: 700, Dir: "/tmp", Name: "test", Run: "go test ./...", Exit: "1", Summary: "3 failed", Recorded: true}}})
	term := m.terms[700]
	if term.summary != "3 failed" || !term.recorded {
		t.Fatalf("term = %+v, want the summary and the record taken from the pane", term)
	}
	if row := renderRow(m, m.rows[1]); !strings.Contains(stripANSI(row), "test · 3 failed") {
		t.Errorf("row = %q, want the run's word from the pane", stripANSI(row))
	}
	m, _ = m.update(procsMsg{procs: m.procs})
	if !term.settled || m.readEndings() != nil {
		t.Error("a settled ending on the record should be read again by nobody")
	}
	if m.record(term) != nil {
		t.Error("an ending on the record should not be recorded again")
	}
	if got := pastRuns("/tmp", "test", "go test ./...", 5); len(got) != 0 {
		t.Errorf("runs = %+v, want nothing appended", got)
	}
	select {
	case got := <-asked:
		if got.Kind == kindSummary || got.Kind == kindRecorded {
			t.Errorf("the pane was told again what it already carries: %+v", got)
		}
	case <-time.After(50 * time.Millisecond):
	}
}

func TestTheOutcomeAndTheRecordAreKeptOnThePane(t *testing.T) {
	// Once the transcript is read for what the run said, the pane is told;
	// once the ending is written, the pane is told that too — the state of
	// the ending lives with the shell that holds it, not in this window.
	stateDir(t)
	m := withProcList(90, 14, []Project{{Name: "tmp", Path: "/tmp"}},
		[]Proc{{PID: 700, PPID: 1, Command: "zsh", Dir: "/tmp"}})
	m.terms = map[int]*remoteTerm{700: {pid: 700, dir: "/tmp", name: "test", run: "go test ./..."}}
	m, asked := pipeServer(t, m)
	m, _ = m.update(sessionsMsg{sessions: []sessionInfo{{PID: 700, Dir: "/tmp", Name: "test", Exit: "1"}}})
	m, cmd := m.update(outcomeMsg{pid: 700, summary: "3 failed"})
	if got := askedForKind(t, asked, kindSummary); got.PID != 700 || got.Name != "3 failed" {
		t.Errorf("the pane was told %+v, want the summary on pane 700", got)
	}
	for _, msg := range deliver(cmd) {
		m, _ = m.update(msg)
	}
	if got := askedForKind(t, asked, kindRecorded); got.PID != 700 {
		t.Errorf("the pane was told %+v, want the record marked on pane 700", got)
	}
	if got := pastRuns("/tmp", "test", "go test ./...", 5); len(got) != 1 || got[0].Command != "go test ./..." {
		t.Errorf("runs = %+v, want the one run, with its command", got)
	}
}

func TestAnAgentOutsideEveryRootIsListedUnderGlobal(t *testing.T) {
	// An agent is yours wherever it runs: a claude started in a scratch
	// directory no root holds is listed under global, with the tool it is
	// running beneath it, where an editor there stays out — the place is
	// a label on the process, not a gate it must pass.
	m := withProcList(90, 14,
		[]Project{{Name: "conn", Path: "/p/conn"}},
		[]Proc{
			{PID: 700, PPID: 1, Command: "claude", Dir: "/scratch"},
			{PID: 701, PPID: 700, Command: "go", Argv: "go test ./...", Dir: "/scratch"},
			{PID: 702, PPID: 1, Command: "vim", Dir: "/scratch"},
		})
	var rows []navRow
	for _, r := range m.rows {
		if r.kind == rowProc {
			rows = append(rows, r)
		}
	}
	if len(rows) != 1 || rows[0].project.Name != "global" || rows[0].node.PID != 700 || !rows[0].holds(701) {
		t.Errorf("rows = %+v, want one run under global: the agent with its tool folded beneath, and not the editor", rows)
	}
}

func TestAProcessMakesTheSubProjectItWorksIn(t *testing.T) {
	// The index did not list services/api — ignored, or made since the
	// scan — but a process works there and the directory carries a
	// manifest: the process makes the sub-project, and the manifest names
	// it. A process in a directory with no manifest on the way up works
	// at the root.
	repo := t.TempDir()
	api := filepath.Join(repo, "services", "api")
	if err := os.MkdirAll(api, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(api, "package.json"), "{}"); err != nil {
		t.Fatal(err)
	}
	m := withProcList(90, 14,
		[]Project{{Name: "mono", Path: repo}},
		[]Proc{
			{PID: 700, PPID: 1, Command: "node", Dir: filepath.Join(api, "src")},
			{PID: 701, PPID: 1, Command: "make", Dir: filepath.Join(repo, "docs")},
		})
	wantRows(t, navColumn(m), []string{
		"▸ mono",
		"      make",
		"  mono/services/api",
		"      node",
	})
	if subs := m.subs[repo]; len(subs) != 1 || subs[0].Name != "services/api" {
		t.Errorf("subs = %+v, want the one the process made", subs)
	}
}

func TestTheNewestConversationAtRestIsARowUnderAPlaceWithWork(t *testing.T) {
	// An agent that exited left its conversation, and the conversation is
	// a process that is not running: the newest at rest under a place with
	// work is a dimmed row at the end of the place's family, aged, and
	// enter picks it back up where it was had. A place with no work shows
	// none — the row is kept the way an exited container is kept beside
	// its running siblings, not as a list of what once ran.
	m := withProcList(90, 14,
		[]Project{{Name: "conn", Path: "/p/conn"}},
		[]Proc{{PID: 700, PPID: 1, Command: "zsh", Dir: "/p/conn"}})
	m.terms = map[int]*remoteTerm{700: {pid: 700, dir: "/p/conn"}}
	m.rests = map[string]conversation{"/p/conn": {Kind: "claude", ID: "aaaa-1111", Dir: "/p/conn",
		When: time.Now().Add(-2 * time.Hour), Prompt: "fix the resize race"}}
	m.rebuild()
	last := m.rows[len(m.rows)-1]
	if last.kind != rowRest || last.rest.ID != "aaaa-1111" {
		t.Fatalf("last row = %+v, want the conversation at rest", last)
	}
	if row := stripANSI(renderRow(m, last)); !strings.Contains(row, "claude · suspended · 2h") {
		t.Errorf("row = %q, want the kind, that it is suspended, and its age", row)
	}

	m.cursor = len(m.rows) - 1
	m, asked := pipeServer(t, m)
	m = press(m, "enter")
	if got := askedForKind(t, asked, kindOpen); got.Dir != "/p/conn" || got.Run != "claude --resume aaaa-1111" {
		t.Errorf("asked %+v, want the conversation continued where it was had", got)
	}

	// No work in the place: no row at rest.
	m.procs, m.terms = nil, map[int]*remoteTerm{}
	m = narrowed(m)
	for _, r := range m.rows {
		if r.kind == rowRest {
			t.Errorf("row %+v shown under a place with no work", r)
		}
	}
}

func TestAnInstanceLeavingHasTheRestsListedAgain(t *testing.T) {
	// The live conversations changing — an instance gone, or one come —
	// is what makes a conversation at rest or not, and is when the rests
	// are listed again; the same instances again are not.
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(t.TempDir(), "absent"))
	m := withClaude("claude", nil)
	next, cmd := m.Update(agentsMsg{agents: asAgents(map[int]claudeSession{
		700: {PID: 700, SessionID: "aaaa-1111", Status: busyStatus},
	})})
	m = next.(model)
	if !asksForRests(cmd) {
		t.Error("a new live conversation should have the rests listed again")
	}
	next, cmd = m.Update(agentsMsg{agents: asAgents(map[int]claudeSession{
		700: {PID: 700, SessionID: "aaaa-1111", Status: "idle"},
	})})
	m = next.(model)
	if asksForRests(cmd) {
		t.Error("the same live conversation should not have the rests listed again")
	}
	next, cmd = m.Update(agentsMsg{agents: map[int]agent{}})
	if !asksForRests(cmd) {
		t.Error("an instance gone should have the rests listed again")
	}
	_ = next
}

// asksForRests reports a command among cmd's that lists the rests.
func asksForRests(cmd tea.Cmd) bool {
	for _, msg := range deliver(cmd) {
		if _, ok := msg.(restsMsg); ok {
			return true
		}
	}
	return false
}
