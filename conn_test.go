package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/w0zro/conn/internal/tmux"
)

func write(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// A first prompt with an image in it is a line of megabytes; the
// directory after it is still found.
func TestHeadCwdPastALongLine(t *testing.T) {
	long := `{"type":"user","message":{"content":"` + strings.Repeat("A", 3<<20) + `"},"cwd":"/Users/me/projects/shop"}` + "\n"
	path := write(t, filepath.Join(t.TempDir(), "x.jsonl"), `{"type":"mode"}`+"\n"+long)
	if got := headCwd(path); got != "/Users/me/projects/shop" {
		t.Errorf("headCwd = %q", got)
	}
}

func TestHeadCwdUnquotes(t *testing.T) {
	path := write(t, filepath.Join(t.TempDir(), "x.jsonl"), `{"cwd":"/Users/me/a \"b\"\\c"}`+"\n")
	if got := headCwd(path); got != `/Users/me/a "b"\c` {
		t.Errorf("headCwd = %q", got)
	}
	none := write(t, filepath.Join(t.TempDir(), "y.jsonl"), `{"type":"mode"}`+"\n")
	if got := headCwd(none); got != "" {
		t.Errorf("headCwd of a transcript with no cwd = %q", got)
	}
}

func TestTailTitle(t *testing.T) {
	cases := []struct{ name, lines, want string }{
		{"a title", `{"type":"ai-title","aiTitle":"Fix  the\nbuild"}`, "Fix the build"},
		{"a rename beats a title written after it",
			`{"type":"custom-title","customTitle":"mine"}` + "\n" + `{"type":"ai-title","aiTitle":"theirs"}`, "mine"},
		{"a title written after an older rename",
			`{"type":"custom-title","customTitle":"mine"}` + "\n" + `{"type":"user"}` + "\n" + `{"type":"ai-title","aiTitle":"theirs"}`, "theirs"},
		{"no title: the last prompt", `{"type":"last-prompt","lastPrompt":"make it go"}`, "make it go"},
	}
	for _, c := range cases {
		path := write(t, filepath.Join(t.TempDir(), "x.jsonl"), c.lines+"\n")
		if got := tailTitle(path); got != c.want {
			t.Errorf("%s: tailTitle = %q, want %q", c.name, got, c.want)
		}
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// A worktree, wherever it is on disk, is its repository's project; a
// directory in no repository is its own.
func TestRepoRoot(t *testing.T) {
	base, _ := filepath.EvalSymlinks(t.TempDir())
	repo := filepath.Join(base, "shop")
	if err := os.MkdirAll(filepath.Join(repo, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "init", "-q")
	git(t, repo, "commit", "-q", "--allow-empty", "-m", "start")
	tree := filepath.Join(base, "elsewhere", "shop-fix")
	git(t, repo, "worktree", "add", "-q", tree)
	plain := filepath.Join(base, "notes")
	if err := os.Mkdir(plain, 0o700); err != nil {
		t.Fatal(err)
	}
	for dir, want := range map[string]string{
		repo:                       repo,
		filepath.Join(repo, "src"): repo,
		tree:                       repo,
		plain:                      plain,
	} {
		if got := repoRoot(dir); got != want {
			t.Errorf("repoRoot(%s) = %s, want %s", dir, got, want)
		}
	}
}

func TestTemporaryIsNoProject(t *testing.T) {
	if got := rootOf(t.TempDir()); got != "" {
		t.Errorf("rootOf a temporary directory = %q", got)
	}
}

func TestFindProjects(t *testing.T) {
	now := time.Now()
	roots := map[string]string{
		"/h/p/shop":     "/h/p/shop",
		"/h/p/shop/api": "/h/p/shop",
		"/h/p/notes":    "/h/p/notes",
		"/h":            "/h",
		"/h/gone":       "",
	}
	seen := []sighting{
		{"/h/p/notes", now.Add(-3 * time.Hour)},
		{"/h/p/shop/api", now.Add(-time.Hour)},
		{"/h/p/shop", now.Add(-5 * time.Hour)},
		{"/h", now},
		{"/h/gone", now},
	}
	got := findProjects(seen, "/h", func(d string) string { return roots[d] })
	want := []project{
		{Name: "shop", Root: "/h/p/shop", Last: now.Add(-time.Hour)},
		{Name: "notes", Root: "/h/p/notes", Last: now.Add(-3 * time.Hour)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("findProjects = %+v\nwant %+v", got, want)
	}
}

func TestParsePanes(t *testing.T) {
	out := "%1 @1 0 412 /dev/ttys004   abc-1   claude /Users/me/my project\n" +
		"%2 @2 1 0 /dev/ttys005 1     zsh /Users/me\n" +
		"%3 @3 0 77 /dev/ttys006    web 0 cat /p\n" +
		"%4 @1 0 78 /dev/ttys007  1    conn /Users/me\n"
	got := parsePanes(out)
	want := []pane{
		{ID: "%1", Window: "@1", PID: 412, TTY: "ttys004", Session: "abc-1", Command: "claude", Dir: "/Users/me/my project"},
		{ID: "%2", Window: "@2", Dead: true, TTY: "ttys005", Panel: true, Command: "zsh", Dir: "/Users/me"},
		{ID: "%3", Window: "@3", PID: 77, TTY: "ttys006", Name: "web", Exit: "0", Command: "cat", Dir: "/p"},
		{ID: "%4", Window: "@1", PID: 78, TTY: "ttys007", Hold: true, Command: "conn", Dir: "/Users/me"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parsePanes = %+v\nwant %+v", got, want)
	}
}

// What runs in conn comes first, then what runs elsewhere, then the
// conversations at rest. A conversation is found in a pane by the mark
// conn put on it or by the terminal Claude Code runs on.
func TestArrange(t *testing.T) {
	now := time.Now()
	convs := []conversation{
		{ID: "rest", When: now},
		{ID: "marked", When: now.Add(-time.Minute)},
		{ID: "away", When: now.Add(-2 * time.Minute)},
		{ID: "byhand", When: now.Add(-3 * time.Minute)},
	}
	ps := []pane{
		{ID: "%1", Session: "marked", Dir: "/p"},
		{ID: "%2", TTY: "ttys9", Dir: "/p"},
		{ID: "%3", Command: "npm", Dir: "/p/web"},
		{ID: "%4", Command: "zsh", Dir: "/other"},
	}
	live := map[string]claudeRun{
		"away":   {TTY: "ttys1", Status: "busy"},
		"byhand": {TTY: "ttys9", Status: "waiting"},
	}
	rows := arrange(convs, ps, live, nil, nil, func(d string) bool { return within(d, "/p") }, func(c conversation) string { return "t-" + c.ID })
	var got []string
	for _, r := range rows {
		s := ""
		if r.conv != nil {
			s = r.conv.ID
		}
		if r.pane != nil {
			s += "@" + r.pane.ID
		}
		if r.elsewhere {
			s += "!"
		}
		got = append(got, s)
	}
	// byhand is waiting, so it leads what runs in conn.
	want := []string{"byhand@%2", "marked@%1", "@%3", "away!", "rest"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("arrange = %v, want %v", got, want)
	}
	if rows[0].conv.Title != "t-byhand" || rows[0].state != "waiting" || rows[3].state != "working" {
		t.Errorf("rows = %+v", rows)
	}
}

func TestParsePS(t *testing.T) {
	got := parsePS("  412 ttys003  Sat Oct 10 03:53:54 2026\n  9 ??       Mon Oct  5 10:00:00 2026\n")
	if got[412].tty != "ttys003" || got[9].tty != "" {
		t.Errorf("parsePS = %+v", got)
	}
	if got[412].started.Hour() != 3 || got[9].started.Day() != 5 {
		t.Errorf("parsePS times = %v, %v", got[412].started, got[9].started)
	}
}

func TestBeganAt(t *testing.T) {
	started := time.Date(2026, 10, 10, 3, 53, 54, 0, time.UTC).In(time.Local)
	if !beganAt("Sat Oct 10 03:53:54 2026", started) {
		t.Error("the process the file names was not taken for it")
	}
	if beganAt("Sat Oct 10 03:53:54 2026", started.Add(time.Hour)) {
		t.Error("a later process with the same pid was taken for it")
	}
}

func TestShownProjects(t *testing.T) {
	m := model{home: "/h", projects: []project{
		{Name: "shop", Root: "/h/p/shop"},
		{Name: "notes", Root: "/h/w/notes"},
	}}
	m.typed = "SHO"
	if got := m.shownProjects(); len(got) != 1 || got[0].Name != "shop" {
		t.Errorf("shop: %+v", got)
	}
	m.typed = "w note"
	if got := m.shownProjects(); len(got) != 1 || got[0].Name != "notes" {
		t.Errorf("w note: %+v", got)
	}
}

func TestTypedPath(t *testing.T) {
	home, _ := filepath.EvalSymlinks(t.TempDir())
	dir := filepath.Join(home, "src", "new")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	m := model{home: home, known: newKnown()}
	for _, typed := range []string{"~/src/new", dir, "src/new"} {
		m.typed = typed
		p, ok := m.typedPath()
		// The scratch directory is temporary, so it stands as itself.
		if !ok || p.Root != dir || p.Name != "new" {
			t.Errorf("typedPath(%q) = %+v, %v", typed, p, ok)
		}
	}
	m.typed = "shop"
	if _, ok := m.typedPath(); ok {
		t.Error("a word was taken for a path")
	}
	m.typed = "~/src/none"
	if _, ok := m.typedPath(); ok {
		t.Error("a missing directory was opened")
	}
}

// conn's window is found by its mark, made where there is none, and
// started again where it runs another build.
func TestEnsurePanel(t *testing.T) {
	bin, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("no tmux")
	}
	dir, err := os.MkdirTemp("/tmp", "cs-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	srv := &tmux.Server{Tmux: bin, Socket: filepath.Join(dir, "s")}
	defer func() { _ = srv.Down() }()
	if _, err := srv.Run("-f", "/dev/null", "new-session", "-d", "-s", sessionName, "-n", "conn", "sleep 1000"); err != nil {
		t.Fatal(err)
	}
	panel := "exec sleep 999"
	win, err := ensurePanel(srv, dir, "/nowhere/conn", panel)
	if err != nil {
		t.Fatal(err)
	}
	// The window named conn runs something else, so it is not conn's.
	if first, _ := srv.Display(sessionName+":0", "#{window_id}"); first == win {
		t.Fatalf("took the window named conn, %s", win)
	}
	again, err := ensurePanel(srv, dir, "/nowhere/conn", panel)
	if err != nil || again != win {
		t.Errorf("second ensurePanel = %s, %v; want %s", again, err, win)
	}
	found, err := findPanel(srv, "")
	if err != nil || found.window != win {
		t.Errorf("findPanel = %+v, %v", found, err)
	}
}

func TestNewID(t *testing.T) {
	id := newID()
	if len(id) != 36 || id[14] != '4' || !strings.ContainsRune("89ab", rune(id[19])) || !isSessionID(id) {
		t.Errorf("newID = %q", id)
	}
	if newID() == id {
		t.Error("two ids alike")
	}
}

func TestState(t *testing.T) {
	for status, want := range map[string]string{"busy": "working", "shell": "working", "waiting": "waiting", "idle": "idle", "pondering": ""} {
		if got := (claudeRun{Status: status}).state(); got != want {
			t.Errorf("state of %q = %q, want %q", status, got, want)
		}
	}
}

func TestStanding(t *testing.T) {
	if got := (project{Running: 2, Waiting: 1}).standing(); got != "1 waiting · 2 running" {
		t.Errorf("standing = %q", got)
	}
	if got := (project{Last: time.Now().Add(-3 * time.Hour)}).standing(); got != "3h" {
		t.Errorf("standing at rest = %q", got)
	}
}

func TestParseListening(t *testing.T) {
	out := "p412\nf5\nn*:3000\nf6\nn[::1]:3000\np77\nf9\nn127.0.0.1:5432\n"
	got := parseListening(out)
	if !reflect.DeepEqual(got, map[int][]string{412: {"3000", "3000"}, 77: {"5432"}}) {
		t.Errorf("parseListening = %v", got)
	}
}

// A listener's port is its pane's, however far down the pane's tree it
// runs; conn's own pane is no process of the project's.
func TestMachine(t *testing.T) {
	ps := []pane{
		{ID: "%0", PID: 10, Panel: true, Command: "conn", Dir: "/h"},
		{ID: "%1", PID: 20, Name: "web", Command: "zsh", Dir: "/p"},
		{ID: "%2", PID: 30, Name: "test", Exit: "1", Command: "cat", Dir: "/p"},
	}
	parent := map[int]int{21: 20, 22: 21}
	got := machine(ps, nil, map[int][]string{22: {"3000", "3000"}}, parent)
	if len(got) != 2 {
		t.Fatalf("machine = %+v", got)
	}
	if got[0].Name != "web" || !reflect.DeepEqual(got[0].Ports, []string{"3000"}) || got[0].State != "running" {
		t.Errorf("web = %+v", got[0])
	}
	if got[1].State != "failed 1" {
		t.Errorf("test = %+v", got[1])
	}
}

func TestShellLine(t *testing.T) {
	if got := shellLine([]string{"npm test | tail -5"}); got != "npm test | tail -5" {
		t.Errorf("one word: %q", got)
	}
	if got := shellLine([]string{"go", "test", "-run", "Test A"}); got != "go test -run 'Test A'" {
		t.Errorf("words: %q", got)
	}
}

func TestFind(t *testing.T) {
	h := here{root: "/p", tree: "/p/wt", procs: []proc{
		{Name: "web", PID: 1, State: "done", Dir: "/p"},
		{Name: "web", PID: 2, State: "running", Ports: []string{"3000"}, Dir: "/p"},
		{Name: "api", PID: 3, State: "running", Dir: "/p"},
		{Name: "api", PID: 4, State: "running", Dir: "/p/wt"},
		{Name: "db", PID: 5, State: "running", Dir: "/elsewhere"},
	}}
	for addr, want := range map[string]int{"web": 2, ":3000": 2, "api": 4, "5": 5} {
		if p, err := h.find(addr); err != nil || p.PID != want {
			t.Errorf("find(%s) = %d, %v; want %d", addr, p.PID, err, want)
		}
	}
	if _, err := h.find("db"); err == nil {
		t.Error("a name reached another project")
	}
	if p, _ := h.find("5"); h.inProject(p) {
		t.Error("another project's process was the caller's to change")
	}
}

// conn u starts a command under a name, the pane says how it ended,
// conn l reads it, and conn x closes it.
func TestStartWaitEnd(t *testing.T) {
	bin, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("no tmux")
	}
	dir, err := os.MkdirTemp("/tmp", "cs-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	srv := &tmux.Server{Tmux: bin, Socket: filepath.Join(dir, "s")}
	defer func() { _ = srv.Down() }()
	if _, err := srv.Run("-f", "/dev/null", "new-session", "-d", "-s", sessionName, "sleep 1000"); err != nil {
		t.Fatal(err)
	}
	id, err := startNamed(srv, "check", "echo one; echo two; exit 3", dir)
	if err != nil {
		t.Fatal(err)
	}
	exit, err := waitExit(srv, id)
	if err != nil || exit != "3" {
		t.Fatalf("waitExit = %q, %v", exit, err)
	}
	lines, err := said(srv, id)
	if err != nil || len(lines) < 3 || lines[0] != "one" || lines[1] != "two" || lines[len(lines)-1] != "[check exited 3]" {
		t.Errorf("said = %q, %v", lines, err)
	}
	ps, _ := panes(srv)
	var p *pane
	for i := range ps {
		if ps[i].ID == id {
			p = &ps[i]
		}
	}
	if p == nil || p.Name != "check" || p.Exit != "3" {
		t.Fatalf("pane = %+v", p)
	}
	if how, err := endProc(srv, proc{pane: p, Name: "check"}); err != nil || how != "kill-pane "+id {
		t.Errorf("end = %q, %v", how, err)
	}
	// A server conn u started and that is still running is sent ctrl-c.
	id, _ = startNamed(srv, "server", "sleep 1000", dir)
	time.Sleep(300 * time.Millisecond)
	ps, _ = panes(srv)
	for i := range ps {
		if ps[i].ID == id {
			p = &ps[i]
		}
	}
	if how, err := endProc(srv, proc{pane: p, Name: "server"}); err != nil || how != "send-keys C-c" {
		t.Errorf("end of a running server = %q, %v", how, err)
	}
	// Any other pane runs again what it was opened with, in place, and
	// x closes it.
	first, _ := srv.Display(sessionName+":0", "#{pane_id} #{pane_pid}")
	id, pid, _ := strings.Cut(first, " ")
	other := &pane{ID: id}
	if got, err := restartProc(srv, "conn", proc{pane: other, Name: "sleep"}); err != nil || got != id {
		t.Fatalf("restart = %q, %v", got, err)
	}
	if again, _ := srv.Display(id, "#{pane_pid}"); again == pid || again == "" {
		t.Errorf("the pane was not started again: pid %s, then %s", pid, again)
	}
	if how, err := endProc(srv, proc{pane: other, Name: "sleep"}); err != nil || how != "kill-pane "+id {
		t.Errorf("end of a pane = %q, %v", how, err)
	}
	if _, err := restartProc(srv, "conn", proc{Name: "python", PID: 1}); err == nil {
		t.Error("a process outside conn was restarted")
	}
}

func TestNameFor(t *testing.T) {
	for command, want := range map[string]string{
		"npm run dev":                 "npm",
		"PORT=4000 ./bin/server -v":   "server",
		"python3 -m http.server 80":   "python3",
		"/usr/local/bin/redis-server": "redis-server",
		"FOO=1":                       "run",
	} {
		if got := nameFor(command); got != want {
			t.Errorf("nameFor(%q) = %q, want %q", command, got, want)
		}
	}
}

// A conversation at rest has no process for x or r; a pane does.
func TestRowProc(t *testing.T) {
	if _, ok := (row{conv: &conversation{ID: "a"}}).proc(); ok {
		t.Error("a conversation at rest was taken for a process")
	}
	p, ok := (row{pane: &pane{ID: "%1", PID: 9, Name: "web", Dir: "/p"}}).proc()
	if !ok || p.Name != "web" || p.State != "running" || p.PID != 9 {
		t.Errorf("proc = %+v, %v", p, ok)
	}
}

// The cursor stays on its row when the rows are sorted again under it.
func TestCursorKeepsItsRow(t *testing.T) {
	a, b := &pane{ID: "%1"}, &pane{ID: "%2"}
	m := model{screen: onProject, cur: project{Root: "/p"}, rows: []row{{pane: a}, {pane: b}}, cursor: 1}
	next, _ := m.Update(read{root: "/p", rows: []row{{pane: b, state: "waiting"}, {pane: a}}})
	if got := next.(model); got.rows[got.cursor].pane.ID != "%2" {
		t.Errorf("cursor on %s, want %%2", got.rows[got.cursor].pane.ID)
	}
	m = model{projects: []project{{Root: "/a"}, {Root: "/b"}}, cursor: 1, loaded: true}
	next, _ = m.Update(read{projects: []project{{Root: "/b", Waiting: 1}, {Root: "/a"}}})
	if got := next.(model); got.shownProjects()[got.cursor].Root != "/b" {
		t.Errorf("cursor on %s, want /b", got.shownProjects()[got.cursor].Root)
	}
}

// The projects list is read on the beat inside a project too; it does
// not move the project's cursor.
func TestProjectsReadLeavesTheProjectsCursor(t *testing.T) {
	a, b := &pane{ID: "%1"}, &pane{ID: "%2"}
	m := model{screen: onProject, cur: project{Root: "/p"}, rows: []row{{pane: a}, {pane: b}}, cursor: 1}
	next, _ := m.Update(read{projects: []project{{Root: "/x"}, {Root: "/y"}, {Root: "/z"}}})
	if got := next.(model).cursor; got != 1 {
		t.Errorf("cursor = %d, want 1", got)
	}
}

func TestTells(t *testing.T) {
	for line, want := range map[string]bool{
		"%window-add @3":   true,
		"%window-close @3": true,
		"%subscription-changed conn $0 @1 1 %1 : zsh /p 0  ": true,
		"%begin 1700000000 12 1":                             false,
		"%end 1700000000 12 1":                               false,
		"%output %1 hello":                                   false,
		"%exit":                                              false,
		"plain reply":                                        false,
	} {
		if got := tells(line); got != want {
			t.Errorf("tells(%q) = %v", line, got)
		}
	}
}

// tmux tells a control-mode client of a window opened and of a change
// in what a pane runs, and watch says changed for each.
func TestWatchTmux(t *testing.T) {
	bin, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("no tmux")
	}
	dir, err := os.MkdirTemp("/tmp", "cs-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	srv := &tmux.Server{Tmux: bin, Socket: filepath.Join(dir, "s")}
	defer func() { _ = srv.Down() }()
	if _, err := srv.Run("-f", "/dev/null", "new-session", "-d", "-s", sessionName, "sleep 1000"); err != nil {
		t.Fatal(err)
	}
	// Claude Code's own files are elsewhere, and still: what is heard is
	// tmux.
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	got := make(chan tea.Msg, 16)
	go watch(ctx, func(m tea.Msg) { got <- m }, srv)
	wait := func(what string) {
		t.Helper()
		select {
		case <-got:
		case <-time.After(3 * time.Second):
			t.Fatalf("no word of %s", what)
		}
	}
	wait("the subscription's first values")
	if _, err := srv.Run("new-window", "-d", "-t", sessionName+":", "sleep 1000"); err != nil {
		t.Fatal(err)
	}
	wait("a window opened")
	id, _ := srv.Run("new-window", "-d", "-P", "-F", "#{pane_id}", "-t", sessionName+":", "sleep 1000")
	wait("another window opened")
	// What a new window brings with it — a rename, the subscription's
	// first value for its pane — is let pass, so that what follows is
	// the mark's own word: tmux says a subscription's changes at most
	// once a second.
	for quiet := time.After(1500 * time.Millisecond); ; {
		select {
		case <-got:
			continue
		case <-quiet:
		}
		break
	}
	if _, err := srv.Do(tmux.SetPane(strings.TrimSpace(id), exitMark, "0")); err != nil {
		t.Fatal(err)
	}
	wait("a mark set")
}

func TestParseTerminals(t *testing.T) {
	out := "100 0 /dev/ttys001\n300 1 client-77\n200 0 /dev/ttys004\n"
	if got := parseTerminals(out); !reflect.DeepEqual(got, []string{"/dev/ttys004", "/dev/ttys001"}) {
		t.Errorf("parseTerminals = %v", got)
	}
}

// Claude Code's instance files and transcripts are watched, a
// transcript folder made after the watch began among them.
func TestWatchFiles(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	for _, d := range []string{"sessions", "projects"} {
		if err := os.Mkdir(filepath.Join(cfg, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	got := make(chan struct{}, 16)
	go watchFiles(ctx, func() { got <- struct{}{} })
	time.Sleep(200 * time.Millisecond)
	wait := func(what string) {
		t.Helper()
		select {
		case <-got:
		case <-time.After(3 * time.Second):
			t.Fatalf("no word of %s", what)
		}
		// What else the same change brought.
		for drained := time.After(300 * time.Millisecond); ; {
			select {
			case <-got:
				continue
			case <-drained:
			}
			break
		}
	}
	write(t, filepath.Join(cfg, "sessions", "12.json"), `{"status":"busy"}`)
	wait("an instance file")
	dir := filepath.Join(cfg, "projects", "-p")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	wait("a transcript folder")
	write(t, filepath.Join(dir, "a.jsonl"), "{}\n")
	wait("a transcript in the new folder")
}

// A pane's ports are on its row; a server outside conn in the project
// is a row of its own, which x can end and o can open.
func TestArrangePorts(t *testing.T) {
	ps := []pane{{ID: "%1", Name: "web", Dir: "/p"}}
	outside := []proc{{Name: "python3", PID: 77, Ports: []string{"8000"}, Dir: "/p"}}
	rows := arrange(nil, ps, nil, map[string][]string{"%1": {"3000"}}, outside, func(d string) bool { return within(d, "/p") }, func(conversation) string { return "" })
	if len(rows) != 2 || !reflect.DeepEqual(rows[0].ports, []string{"3000"}) || rows[1].outside == nil || rows[1].key() != "pid 77" {
		t.Fatalf("rows = %+v", rows)
	}
	if p, ok := rows[1].proc(); !ok || p.pane != nil || p.PID != 77 {
		t.Errorf("proc of an outside row = %+v, %v", p, ok)
	}
}

func TestParseStack(t *testing.T) {
	got, err := parseStack("```\nweb: npm run dev\n\nworker: node worker.js\nweb: again\nnot a line\n```\n")
	if err != nil || !reflect.DeepEqual(got, []stacked{{name: "web", command: "npm run dev"}, {name: "worker", command: "node worker.js"}}) {
		t.Errorf("parseStack = %v, %v", got, err)
	}
	got, err = parseStack("dev: npm run dev\ncheck:watch: npm run check:watch\nweb:npm start\n")
	want := []stacked{{name: "dev", command: "npm run dev"}, {name: "check-watch", command: "npm run check:watch"}, {name: "web", command: "npm start"}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("parseStack of colons = %v, %v", got, err)
	}
	if _, err := parseStack("NONE\n"); err == nil {
		t.Error("NONE was taken for a stack")
	}
	if _, err := parseStack("I could not tell.\n"); err == nil {
		t.Error("an answer naming nothing was taken for a stack")
	}
}

// While u's question stands, only enter and esc do anything, and an
// answer that comes after esc is not shown.
func TestStackKeys(t *testing.T) {
	press := func(m model, k string) model {
		key := tea.Key{Text: k, Code: []rune(k)[0]}
		if k == "esc" {
			key = tea.Key{Code: tea.KeyEscape}
		}
		next, _ := m.Update(tea.KeyPressMsg(key))
		return next.(model)
	}
	m := model{screen: onProject, cur: project{Name: "d", Root: "/p"}, rows: []row{{pane: &pane{ID: "%1"}}, {pane: &pane{ID: "%2"}}}}
	m.question = roleStack
	next, _ := m.Update(roleAnswer{root: "/p", as: roleStack, list: []stacked{{name: "web", command: "npm run dev"}}})
	m = next.(model)
	if m.offer == nil {
		t.Fatal("the answer was not offered")
	}
	if m = press(m, "j"); m.cursor != 0 || m.question == "" {
		t.Errorf("j moved the cursor or put the question away: cursor %d, question %q", m.cursor, m.question)
	}
	if m = press(m, "esc"); m.question != "" || m.offer != nil {
		t.Error("esc did not put the question away")
	}
	next, _ = m.Update(roleAnswer{root: "/p", as: roleStack, list: []stacked{{name: "web", command: "npm run dev"}}})
	if next.(model).offer != nil {
		t.Error("an answer after esc was offered")
	}
}

// scratchServer is a tmux server of its own for a test, with conn's
// session and a first window standing for conn's: a pane for the panel.
func scratchServer(t *testing.T) (*tmux.Server, string, string) {
	t.Helper()
	bin, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("no tmux")
	}
	dir, err := os.MkdirTemp("/tmp", "cs-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	srv := &tmux.Server{Tmux: bin, Socket: filepath.Join(dir, "s")}
	t.Cleanup(func() { _ = srv.Down() })
	out, err := srv.Run("-f", "/dev/null", "new-session", "-d", "-s", sessionName, "-x", "160", "-y", "40", "-P", "-F", "#{window_id} #{pane_id}", "sleep 1000")
	if err != nil {
		t.Fatal(err)
	}
	win, panel, _ := strings.Cut(strings.TrimSpace(out), " ")
	return srv, win, panel
}

// inWorkspace is the pane beside the panel in conn's window.
func inWorkspace(t *testing.T, ws workspace) pane {
	t.Helper()
	p, ok, err := ws.shown()
	if err != nil || !ok {
		t.Fatalf("no workspace: %v", err)
	}
	return p
}

// The workspace is made beside the panel with a hold in it; showing a
// pane swaps it in and the hold goes; showing nothing puts a hold back
// and the pane back in a window of its own, still running; a pane that
// ends there is replaced by a hold and the panel keeps its place.
func TestWorkspace(t *testing.T) {
	srv, win, panel := scratchServer(t)
	ws := newWorkspace(srv, win, panel, "/x")
	ws.hold = "sleep 1000"
	if p := inWorkspace(t, func() workspace { _, _ = ws.ensure(); return ws }()); !p.Hold {
		t.Fatalf("the workspace was made without a hold: %+v", p)
	}
	id, err := openWindow(srv, "web", "/tmp", "sleep 1000", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.show(id, false); err != nil {
		t.Fatal(err)
	}
	if p := inWorkspace(t, ws); p.ID != id {
		t.Errorf("the workspace shows %s, want %s", p.ID, id)
	}
	if err := ws.show("", false); err != nil {
		t.Fatal(err)
	}
	if p := inWorkspace(t, ws); !p.Hold {
		t.Errorf("showing nothing left %+v", p)
	}
	ps, _ := panes(srv)
	for _, p := range ps {
		if p.ID == id && (p.Window == win || p.Dead) {
			t.Errorf("the pane did not go back to a window of its own, running: %+v", p)
		}
	}
	// A pane that ends in the workspace.
	short, _ := openWindow(srv, "short", "/tmp", "sleep 0.3", "")
	_ = ws.show(short, false)
	time.Sleep(700 * time.Millisecond)
	if p := inWorkspace(t, ws); !p.Dead {
		t.Fatalf("the ended pane did not stand: %+v", p)
	}
	_ = ws.show("", false)
	if p := inWorkspace(t, ws); !p.Hold || p.Dead {
		t.Errorf("an ended pane was not replaced by a hold: %+v", p)
	}
}

// tab shows the conversations waiting on you in turn, round to the
// first, and says so when none waits.
func TestNextWaiting(t *testing.T) {
	srv, win, panel := scratchServer(t)
	ws := newWorkspace(srv, win, panel, "/x")
	ws.hold = "sleep 1000"
	var ids []string
	for range 2 {
		id, _ := openWindow(srv, "c", "/tmp", "sleep 1000", "")
		ids = append(ids, id)
	}
	m := model{srv: srv, ws: ws, known: newKnown()}
	next, _ := m.nextWaiting()
	if m = next.(model); m.note == "" {
		t.Error("nothing said when nothing waits")
	}
	m.waiting = []pane{{ID: ids[0], Dir: "/tmp"}, {ID: ids[1], Dir: "/tmp"}}
	for _, want := range []string{ids[0], ids[1], ids[0]} {
		next, cmd := m.nextWaiting()
		m = next.(model)
		if c := cmd; c != nil {
			// The batch holds the show and a read; only the show is run.
			if b, ok := c().(tea.BatchMsg); ok && len(b) > 0 {
				b[0]()
			}
		}
		if p := inWorkspace(t, ws); p.ID != want {
			t.Errorf("tab showed %s, want %s", p.ID, want)
		}
	}
}

// Lines that say the text, newest first in each pane, with the saying
// each is counted back from the pane's end, as tmux's search counts.
func TestHits(t *testing.T) {
	panes := []written{
		{name: "web", lines: []string{"GET / 200", "error: boom", "GET /x 500 error error", ""}},
		{name: "test", lines: []string{"ERROR in test"}},
	}
	got := hits("error", panes)
	want := []hit{
		{said: 0, line: "GET /x 500 error error", at: 11, k: 1},
		{said: 0, line: "error: boom", at: 0, k: 2},
		{said: 1, line: "ERROR in test", at: 0, k: 0},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("hits = %+v\nwant %+v", got, want)
	}
	if got := hits("ERROR", panes); len(got) != 1 || got[0].said != 1 {
		t.Errorf("a capital minds case: %+v", got)
	}
	if hits("", panes) != nil {
		t.Error("nothing typed found something")
	}
}

func TestAround(t *testing.T) {
	if got, at := around("short line", 6, 40); got != "short line" || at != 6 {
		t.Errorf("around a short line = %q, %d", got, at)
	}
	line := strings.Repeat("x", 100) + "needle" + strings.Repeat("y", 50)
	got, at := around(line, 100, 40)
	if !strings.HasPrefix(got, "…") || !strings.HasPrefix(got[at:], "needle") {
		t.Errorf("around a long line = %q, %d", got, at)
	}
}

// Rows that run stay in the order their panes were made, whichever
// window tmux lists each in.
func TestArrangeKeepsStartOrder(t *testing.T) {
	ps := []pane{{ID: "%7", Name: "tests", Dir: "/p"}, {ID: "%3", Name: "web", Dir: "/p"}}
	rows := arrange(nil, ps, nil, nil, nil, func(d string) bool { return within(d, "/p") }, func(conversation) string { return "" })
	if rows[0].pane.ID != "%3" || rows[1].pane.ID != "%7" {
		t.Errorf("rows = %s, %s", rows[0].pane.ID, rows[1].pane.ID)
	}
}

// A folder of repositories is one with two or more checkouts directly
// in it, other than home and a folder directly in home.
func TestIsFolder(t *testing.T) {
	home, _ := filepath.EvalSymlinks(t.TempDir())
	for _, d := range []string{"projects/w/a/.git", "projects/w/b/.git", "projects/w/c", "projects/one/x/.git", "projects/z/.git"} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	projects := filepath.Join(home, "projects")
	if !isFolder(filepath.Join(projects, "w"), home) {
		t.Error("two checkouts made no folder")
	}
	if isFolder(filepath.Join(projects, "one"), home) {
		t.Error("one checkout made a folder")
	}
	if isFolder(projects, home) {
		t.Error("a folder directly in home was taken for a folder of repositories")
	}
	if isFolder(home, home) {
		t.Error("home was taken for a folder")
	}
}

// Repositories in a folder are listed beneath it, the folder with what
// they hold summed, and work in the folder itself is the folder's.
func TestGroup(t *testing.T) {
	now := time.Now()
	ps := []project{
		{Name: "hsg", Root: "/p/hsg", Last: now.Add(-3 * time.Hour)},
		{Name: "conn", Root: "/p/w/conn", Last: now.Add(-time.Hour), Running: 2},
		{Name: "essays", Root: "/p/w/essays", Last: now.Add(-5 * time.Hour), Waiting: 1},
		{Name: "w", Root: "/p/w", Last: now.Add(-9 * time.Hour)},
	}
	got := group(ps, func(d string) bool { return d == "/p/w" })
	var names []string
	for _, p := range got {
		names = append(names, listName(p))
	}
	if want := []string{"w/", "  essays", "  conn", "hsg"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("group = %v, want %v", names, want)
	}
	if f := got[0]; !f.Group || f.Running != 2 || f.Waiting != 1 || !f.Last.Equal(now.Add(-time.Hour)) {
		t.Errorf("folder = %+v", f)
	}
	if !got[0].belongs("/p/w/conn") || got[0].belongs("/p/hsg") || got[2].belongs("/p/w/essays") {
		t.Error("belongs took in the wrong work")
	}
}

// A repository found brings its folder above it; a folder found brings
// what it holds.
func TestShownProjectsKeepFolders(t *testing.T) {
	m := model{home: "/h", projects: []project{
		{Name: "w", Root: "/p/w", Group: true},
		{Name: "conn", Root: "/p/w/conn", Depth: 1},
		{Name: "essays", Root: "/p/w/essays", Depth: 1},
		{Name: "hsg", Root: "/p/hsg"},
	}}
	names := func() []string {
		var out []string
		for _, p := range m.shownProjects() {
			out = append(out, p.Name)
		}
		return out
	}
	m.typed = "essays"
	if got := names(); !reflect.DeepEqual(got, []string{"w", "essays"}) {
		t.Errorf("essays: %v", got)
	}
	m.typed = "/p/w "
	if got := names(); !reflect.DeepEqual(got, []string{"w", "conn", "essays"}) {
		t.Errorf("/p/w: %v", got)
	}
}

// A start written down is read back; what a role means in a project is
// each name's last command there, in the order the names first ran,
// and a name run since as the other role has left the first.
func TestStarted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "started.jsonl")
	if got := readStarted(path); got != nil {
		t.Errorf("readStarted of no file = %v", got)
	}
	for _, s := range []started{
		{Root: "/p", Name: "web", Command: "npm run dev", Role: roleStack},
		{Root: "/p", Name: "test", Command: "npm test", Role: roleCheck},
		{Root: "/q", Name: "web", Command: "other", Role: roleStack},
		{Root: "/p", Name: "web", Command: "npm run dev -- --host", Role: roleStack},
		{Root: "/p", Name: "lint", Command: "npm run lint", Role: roleCheck},
		{Root: "/p", Name: "watch", Command: "npm run watch", Role: roleCheck},
		{Root: "/p", Name: "watch", Command: "npm run watch", Role: roleStack},
	} {
		if err := recordStart(path, s); err != nil {
			t.Fatal(err)
		}
	}
	write(t, path, strings.Join(append(func() []string {
		b, _ := os.ReadFile(path)
		return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	}(), "not json", `{"root":"/p","name":"","command":"x"}`), "\n")+"\n")
	in := func(root string) bool { return root == "/p" }
	stack := established(readStarted(path), in, roleStack)
	if want := []stacked{{name: "web", command: "npm run dev -- --host", root: "/p"}, {name: "watch", command: "npm run watch", root: "/p"}}; !reflect.DeepEqual(stack, want) {
		t.Errorf("stack = %+v", stack)
	}
	checks := established(readStarted(path), in, roleCheck)
	if want := []stacked{{name: "test", command: "npm test", root: "/p"}, {name: "lint", command: "npm run lint", root: "/p"}}; !reflect.DeepEqual(checks, want) {
		t.Errorf("checks = %+v", checks)
	}
}

// conn u writes down what it started as the stack, and what it waited
// on as a check.
func TestRunUWritesItDown(t *testing.T) {
	srv, _, _ := scratchServer(t)
	t.Setenv("CONN_SOCKET", srv.Socket)
	dir := filepath.Dir(srv.Socket)
	var out strings.Builder
	if code := runU(&out, t.TempDir(), dir, []string{"web", "--", "sleep 1000"}); code != 0 {
		t.Fatalf("conn u web = %d: %s", code, out.String())
	}
	if code := runU(&out, t.TempDir(), dir, []string{"test", "-w", "--", "echo ok"}); code != 0 {
		t.Fatalf("conn u test -w = %d: %s", code, out.String())
	}
	got := readStarted(startedPath(srv.Socket))
	if len(got) != 2 || got[0].Name != "web" || got[0].Role != roleStack || got[0].Command != "sleep 1000" || got[0].Root != realDir(dir) || got[1].Name != "test" || got[1].Role != roleCheck || got[1].Command != "echo ok" {
		t.Errorf("started = %+v", got)
	}
}

// t runs the checks as they were last run here, with no question; u,
// with no stack run here yet, asks.
func TestResolveRunsWhatWasRun(t *testing.T) {
	srv, _, _ := scratchServer(t)
	dir := filepath.Dir(srv.Socket)
	if err := recordStart(startedPath(srv.Socket), started{Root: dir, Name: "test", Command: "echo ok", Role: roleCheck}); err != nil {
		t.Fatal(err)
	}
	m := model{srv: srv, screen: onProject, cur: project{Name: "d", Root: dir}, rows: []row{}}
	next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "t", Code: 't'}))
	m = next.(model)
	if m.question != "" || cmd == nil {
		t.Fatalf("t asked a question, or did nothing: question %q", m.question)
	}
	if said, ok := cmd().(acted); !ok || string(said) != "test is up" {
		t.Errorf("t said %q", said)
	}
	ps, _ := panes(srv)
	found := false
	for _, p := range ps {
		found = found || p.Name == "test"
	}
	if !found {
		t.Error("no pane named test")
	}
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Text: "u", Code: 'u'}))
	if next.(model).question != roleStack {
		t.Error("u did not ask what the stack is")
	}
}

// A run that failed comes after what waits and before the rest, and
// reads failed with its status; one that ended well reads done.
func TestArrangeFailedFirst(t *testing.T) {
	ps := []pane{{ID: "%2", Name: "web", Dir: "/p"}, {ID: "%5", Name: "lint", Exit: "0", Dir: "/p"}, {ID: "%7", Name: "test", Exit: "1", Dir: "/p"}}
	rows := arrange(nil, ps, nil, nil, nil, func(d string) bool { return within(d, "/p") }, func(conversation) string { return "" })
	var got []string
	for _, r := range rows {
		got = append(got, r.pane.Name+" "+or(r.state, "running"))
	}
	if want := []string{"test failed 1", "web running", "lint done"}; !reflect.DeepEqual(got, want) {
		t.Errorf("rows = %v", got)
	}
}

// A failed check is handed with what it runs, how it ended, its last
// lines less conn's own closing line, and what runs it again.
func TestHandPrompt(t *testing.T) {
	lines := []string{"--- FAIL: TestX", "FAIL", "", "[test exited 1]", ""}
	got := handPrompt("test", "go test ./...", "~/p", "failed 1", lines)
	for _, want := range []string{"In this project, test runs go test ./... at ~/p, and it failed 1.", "--- FAIL: TestX\nFAIL\n", "Work out what is wrong and fix it.", "conn l test reads", "conn u test -w -- go test ./... runs it again."} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "[test exited") {
		t.Errorf("prompt keeps conn's closing line:\n%s", got)
	}
	running := handPrompt("web", "npm run dev", "~/p", "running", nil)
	if !strings.Contains(running, "and it is running.") || !strings.Contains(running, "Look into it.") || strings.Contains(running, "runs it again") {
		t.Errorf("prompt for a running process:\n%s", running)
	}
	many := make([]string, 200)
	for i := range many {
		many[i] = strings.Repeat("x", 300)
	}
	if got := lastLines(many, 80, 16<<10); len(got) != 54 {
		t.Errorf("lastLines kept %d lines", len(got))
	}
}

// A conversation is told the project's checks and stack as last run.
func TestAgentNote(t *testing.T) {
	note := agentNote("conn", []stacked{{name: "web", command: "npm run dev"}}, []stacked{{name: "test", command: "go test ./..."}})
	for _, want := range []string{"In this project, as last run through conn:\n  conn u test -w -- go test ./...\n  conn u web -- npm run dev"} {
		if !strings.Contains(note, want) {
			t.Errorf("note lacks %q:\n%s", want, note)
		}
	}
	if bare := agentNote("/x/conn", nil, nil); strings.Contains(bare, "as last run") || !strings.Contains(bare, "conn here is /x/conn") {
		t.Errorf("bare note:\n%s", bare)
	}
}
