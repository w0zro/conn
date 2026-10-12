package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

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
	out := "%1 @1 0 412 /dev/ttys004  abc-1   claude /Users/me/my project\n" +
		"%2 @2 1 0 /dev/ttys005 1    zsh /Users/me\n" +
		"%3 @3 0 77 /dev/ttys006   web 0 cat /p\n"
	got := parsePanes(out)
	want := []pane{
		{ID: "%1", Window: "@1", PID: 412, TTY: "ttys004", Session: "abc-1", Command: "claude", Dir: "/Users/me/my project"},
		{ID: "%2", Window: "@2", Dead: true, TTY: "ttys005", Panel: true, Command: "zsh", Dir: "/Users/me"},
		{ID: "%3", Window: "@3", PID: 77, TTY: "ttys006", Name: "web", Exit: "0", Command: "cat", Dir: "/p"},
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
	rows := arrange(convs, ps, live, "/p", func(c conversation) string { return "t-" + c.ID })
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
	if got[1].State != "exited 1" {
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
		{Name: "web", PID: 1, State: "exited 0", Dir: "/p"},
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
	next, _ := m.Update(projectRead{root: "/p", rows: []row{{pane: b, state: "waiting"}, {pane: a}}})
	if got := next.(model); got.rows[got.cursor].pane.ID != "%2" {
		t.Errorf("cursor on %s, want %%2", got.rows[got.cursor].pane.ID)
	}
	m = model{projects: []project{{Root: "/a"}, {Root: "/b"}}, cursor: 1, loaded: true}
	next, _ = m.Update(projectsRead{{Root: "/b", Waiting: 1}, {Root: "/a"}})
	if got := next.(model); got.shownProjects()[got.cursor].Root != "/b" {
		t.Errorf("cursor on %s, want /b", got.shownProjects()[got.cursor].Root)
	}
}

// The projects list is read on the beat inside a project too; it does
// not move the project's cursor.
func TestProjectsReadLeavesTheProjectsCursor(t *testing.T) {
	a, b := &pane{ID: "%1"}, &pane{ID: "%2"}
	m := model{screen: onProject, cur: project{Root: "/p"}, rows: []row{{pane: a}, {pane: b}}, cursor: 1}
	next, _ := m.Update(projectsRead{{Root: "/x"}, {Root: "/y"}, {Root: "/z"}})
	if got := next.(model).cursor; got != 1 {
		t.Errorf("cursor = %d, want 1", got)
	}
}
