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
	out := "%1 @1 0 /dev/ttys004 abc-1 claude /Users/me/my project\n%2 @2 1 /dev/ttys005  zsh /Users/me\n"
	got := parsePanes(out)
	want := []pane{
		{ID: "%1", Window: "@1", TTY: "ttys004", Session: "abc-1", Command: "claude", Dir: "/Users/me/my project"},
		{ID: "%2", Window: "@2", Dead: true, TTY: "ttys005", Command: "zsh", Dir: "/Users/me"},
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
	live := map[string]string{"away": "ttys1", "byhand": "ttys9"}
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
	want := []string{"marked@%1", "byhand@%2", "@%3", "away!", "rest"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("arrange = %v, want %v", got, want)
	}
	if rows[0].conv.Title != "t-marked" {
		t.Errorf("title = %q", rows[0].conv.Title)
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
