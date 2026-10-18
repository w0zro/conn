package main

import (
	"charm.land/lipgloss/v2"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/w0zro/conn/internal/theme"
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
// The context a conversation's last turn ran in is the last assistant
// turn's usage: fresh, written to the cache and read from it; none
// where no turn has been had. It is said in thousands.
func TestTailContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.jsonl")
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"hi"}}`,
		`{"type":"assistant","message":{"usage":{"input_tokens":10,"cache_creation_input_tokens":500,"cache_read_input_tokens":1000,"output_tokens":9}}}`,
		`{"type":"assistant","message":{"usage":{"input_tokens":32,"cache_creation_input_tokens":847,"cache_read_input_tokens":261032,"output_tokens":308}}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"x"}]}}`,
		`{"type":"last-prompt","lastPrompt":"show the context size"}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	title, context := tailRead(path)
	if title != "show the context size" || context != 261911 {
		t.Errorf("tailRead = %q, %d", title, context)
	}
	if err := os.WriteFile(path, []byte(lines[0]+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, context := tailRead(path); context != 0 {
		t.Errorf("context with no turn = %d", context)
	}
	for n, want := range map[int]string{261911: "262k", 999: "999", 1000: "1k", 52451: "52k"} {
		if got := tokens(n); got != want {
			t.Errorf("tokens(%d) = %s, want %s", n, got, want)
		}
	}
	m := model{screen: onProject, cur: project{Root: "/p"}, width: 60}
	line := m.drawRow(row{conv: &conversation{ID: "c", Title: "Fixing", Context: 261911}, pane: &pane{ID: "%1"}}, 0, false)
	if !strings.HasSuffix(strings.TrimSpace(line), "262k") {
		t.Errorf("row = %q, want the context at the right", line)
	}
}

// A conversation starts under the id conn chose, told what conn tells
// its agents, with a first prompt where there is one, each read from
// its file in the pane and the file let go, so that the command tmux
// carries stays short however long the note and the prompt are.
func TestNewCommand(t *testing.T) {
	got := newCommand("/s/note.id", "id", "/s/prompt.id")
	for _, want := range []string{"conn_note=$(cat '/s/note.id')\n", "rm -f '/s/note.id'\n", "conn_prompt=$(cat '/s/prompt.id')\n", "rm -f '/s/prompt.id'\n", "--append-system-prompt \"$conn_note\" --session-id 'id' \"$conn_prompt\""} {
		if !strings.Contains(got, want) {
			t.Errorf("newCommand lacks %q:\n%s", want, got)
		}
	}
	if got := newCommand("/s/note.id", "id", ""); strings.Contains(got, "conn_prompt") || strings.HasSuffix(got, " ''") {
		t.Errorf("with no prompt, newCommand = %q", got)
	}
	if got := resumeCommand("/s/note.id", "id"); !strings.Contains(got, "conn_note=$(cat '/s/note.id')\n") || !strings.HasSuffix(got, "--append-system-prompt \"$conn_note\" --resume 'id'") {
		t.Errorf("resumeCommand = %q", got)
	}
	if got := promptPath("/s/tmux.sock", "id"); got != "/s/prompt.id" {
		t.Errorf("promptPath = %q", got)
	}
	if got := notePath("/s/tmux.sock", "id"); got != "/s/note.id" {
		t.Errorf("notePath = %q", got)
	}
}

// A project with many names written down tells a conversation more
// than tmux carries in one command, and the conversation still starts:
// sc opens it, and the note is read in the pane and let go.
func TestConverseInAProjectToldAtLength(t *testing.T) {
	bin, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("no tmux")
	}
	scratch, err := os.MkdirTemp("/tmp", "cs-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(scratch) })
	scratch, _ = filepath.EvalSymlinks(scratch)
	t.Setenv("HOME", scratch)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(scratch, ".claude"))
	write(t, filepath.Join(scratch, "bin", "claude"), "#!/bin/sh\nexec sleep 1000\n")
	if err := os.Chmod(filepath.Join(scratch, "bin", "claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Join(scratch, "bin")+":"+os.Getenv("PATH"))
	web := filepath.Join(scratch, "web")
	if err := os.MkdirAll(web, 0o755); err != nil {
		t.Fatal(err)
	}
	srv := &tmux.Server{Tmux: bin, Socket: filepath.Join(scratch, "s")}
	t.Cleanup(func() { _ = srv.Down() })
	if _, err := srv.Run("-f", "/dev/null", "new-session", "-d", "-s", sessionName, "sleep 1000"); err != nil {
		t.Fatal(err)
	}
	// Written down in the project: a hundred names, each run with a
	// command of some length, as a project worked by agents comes to have.
	long := strings.Repeat("--flag=value ", 20)
	for i := 0; i < 100; i++ {
		if err := recordStart(startedPath(srv.Socket), started{Root: web, Name: fmt.Sprintf("suite-%d", i), Command: "go test " + long + "./...", Role: roleCheck, When: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	if note := told("conn", srv.Socket, project{Root: web}.belongs); len(note) < 16*1024 {
		t.Fatalf("the note is %d bytes; the test wants one tmux cannot carry", len(note))
	}

	k := newKnown()
	k.root[web] = web
	m := model{srv: srv, screen: onProject, cur: project{Name: "web", Root: web}, home: scratch, width: 120, height: 36, known: k}
	next, _ := m.Update(m.read()())
	m = next.(model)
	for _, r := range "sc" {
		next, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
		m = next.(model)
	}
	if m.in == "" {
		t.Fatalf("sc started no conversation: %q", m.note)
	}
	ps, _ := panes(srv)
	var agent *pane
	for i := range ps {
		if ps[i].ID == m.in {
			agent = &ps[i]
		}
	}
	if agent == nil || agent.Session == "" {
		t.Fatalf("the keys are in %s, which is no conversation", m.in)
	}
	// The pane reads the note and lets the file go.
	file := notePath(srv.Socket, agent.Session)
	gone := false
	for i := 0; i < 50 && !gone; i++ {
		if _, err := os.Stat(file); err != nil {
			gone = true
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !gone {
		t.Errorf("the note still waits at %s", file)
	}
}

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

// A row in a worktree says the branch it has out; a row in the
// repository's own checkout says nothing; in a folder of repositories,
// the repository comes first. A conversation at rest says the same.
func TestPlaced(t *testing.T) {
	base, _ := filepath.EvalSymlinks(t.TempDir())
	folder := filepath.Join(base, "shop")
	repo := filepath.Join(folder, "api")
	if err := os.MkdirAll(filepath.Join(repo, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "init", "-q", "-b", "main")
	git(t, repo, "commit", "-q", "--allow-empty", "-m", "start")
	tree := filepath.Join(base, "elsewhere", "api-fix")
	git(t, repo, "worktree", "add", "-q", tree)
	// A directory under the system's scratch is no project; the test
	// says which repository each is in, and git says the checkout.
	k := newKnown()
	k.root[filepath.Join(repo, "src")], k.root[tree], k.root[repo] = repo, repo, repo
	rows := []row{
		{pane: &pane{ID: "%1", Dir: filepath.Join(repo, "src")}},
		{pane: &pane{ID: "%2", Dir: tree}},
		{conv: &conversation{ID: "c", Dir: tree}},
	}
	placed(rows, project{Name: "api", Root: repo}, k)
	for i, want := range []string{"", "api-fix", "api-fix"} {
		if rows[i].branch != want || rows[i].repo != "" {
			t.Errorf("row %d placed as %q in %q, want branch %q in no repository", i, rows[i].branch, rows[i].repo, want)
		}
	}
	if rows[0].tree != repo || rows[1].tree != tree {
		t.Errorf("checkouts %q and %q, want %q and %q", rows[0].tree, rows[1].tree, repo, tree)
	}
	git(t, tree, "checkout", "-q", "-b", "fix-login")
	placed(rows, project{Name: "shop", Root: folder, Group: true}, k)
	if r := rows[1]; r.repo != "api" || r.branch != "fix-login" {
		t.Errorf("in a folder, the worktree's row is placed as %q · %q, want api · fix-login", r.repo, r.branch)
	}
	if got := rows[1].place(); len(got) != 2 || got[0].s != "api · " || got[1].s != "fix-login · " {
		t.Errorf("place = %+v", got)
	}
}

// A project's rows gather by worktree: the project's own checkouts'
// rows first as arranged, then each worktree's together, by name; a
// row in a folder stands under its repository's branch.
func TestByPlace(t *testing.T) {
	rows := []row{
		{pane: &pane{ID: "%1"}, branch: "fix-login"},
		{pane: &pane{ID: "%2"}},
		{pane: &pane{ID: "%3"}, branch: "api-docs", repo: "api"},
		{pane: &pane{ID: "%4"}},
		{pane: &pane{ID: "%5"}, branch: "fix-login"},
	}
	byPlace(rows)
	var got []string
	for _, r := range rows {
		got = append(got, r.pane.ID+" "+r.worktree())
	}
	want := []string{"%2 ", "%4 ", "%3 api · api-docs", "%1 fix-login", "%5 fix-login"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("byPlace = %q, want %q", got, want)
	}
}

// Drawn, a worktree's rows stand in under a head that names the branch
// alone, and say only their names.
func TestWorktreeLine(t *testing.T) {
	m := model{screen: onProject, cur: project{Name: "d", Root: "/p"}, home: "/home", width: 60, height: 30, rows: []row{
		{pane: &pane{ID: "%2", Name: "web", Dir: "/p"}, tree: "/p"},
		{pane: &pane{ID: "%1", Name: "web", Dir: "/w"}, branch: "fix-login", tree: "/w"},
	}}
	lines, _ := m.drawProject()
	plain := regexp.MustCompile("\x1b\\[[0-9;]*m")
	var got []string
	for _, l := range lines {
		if l = strings.TrimSpace(plain.ReplaceAllString(l, "")); l != "" {
			got = append(got, l)
		}
	}
	at := -1
	for i, l := range got {
		if l == "fix-login" {
			at = i
		}
	}
	if at < 0 || at+1 >= len(got) || !strings.HasSuffix(got[at+1], "web") || strings.Contains(got[at+1], "fix-login") {
		t.Errorf("drawn as %q, want web under a fix-login line", got)
	}
	// The worktree's row stands two columns in from the project's own.
	var cols []int
	for _, l := range lines {
		if l = plain.ReplaceAllString(l, ""); strings.Contains(l, "web") {
			cols = append(cols, utf8.RuneCountInString(l[:strings.Index(l, "web")]))
		}
	}
	if len(cols) != 2 || cols[1] != cols[0]+2 {
		t.Errorf("web stands at columns %v, want the worktree's two in from the project's", cols)
	}
	// The worktree stands a row off what is above it.
	for i, l := range lines {
		if strings.TrimSpace(plain.ReplaceAllString(l, "")) == "fix-login" && (i == 0 || strings.TrimSpace(lines[i-1]) != "") {
			t.Errorf("no blank line before the worktree's head: %q", lines[max(i-1, 0):i+1])
		}
	}
}

// A verb starts in the checkout the cursor stands in: a repository's
// stack runs there where the checkout is the repository's, and at the
// repository otherwise; with the cursor on no row, at the project.
func TestPlaceFor(t *testing.T) {
	k := newKnown()
	k.tree["/w/src"], k.root["/w"], k.root["/w/src"] = "/w", "/r", "/r"
	m := model{screen: onProject, cur: project{Name: "r", Root: "/r"}, known: k, rows: []row{{pane: &pane{ID: "%1", Dir: "/w/src"}}}}
	if got := m.at(); got != "/w" {
		t.Errorf("at = %q, want /w", got)
	}
	if got := m.placeFor("/r"); got != "/w" {
		t.Errorf("the repository's stack runs at %q, want its worktree /w", got)
	}
	if got := m.placeFor("/other"); got != "/other" {
		t.Errorf("another repository's stack runs at %q, want /other", got)
	}
	if got := m.here(); got != "/w" {
		t.Errorf("here = %q, want /w", got)
	}
	m.rows = nil
	if got := m.here(); got != "/r" || m.at() != "" {
		t.Errorf("on no row, here = %q, at = %q; want /r and nothing", got, m.at())
	}
}

func TestTemporaryIsNoProject(t *testing.T) {
	if got := rootOf(t.TempDir()); got != "" {
		t.Errorf("rootOf a temporary directory = %q", got)
	}
}

// The projects are the checkouts given, newest work first by the
// sightings, a checkout never worked in last; a sighting somewhere
// that is no checkout says nothing, home is no project, and a root
// given twice is one project.
func TestFindProjects(t *testing.T) {
	now := time.Now()
	roots := map[string]string{
		"/h/p/shop":     "/h/p/shop",
		"/h/p/shop/api": "/h/p/shop",
		"/h/p/notes":    "/h/p/notes",
		"/h/p/old":      "/h/p/old",
		"/h":            "/h",
		"/h/gone":       "",
	}
	seen := []sighting{
		{"/h/p/notes", now.Add(-3 * time.Hour)},
		{"/h/p/shop/api", now.Add(-time.Hour)},
		{"/h/p/shop", now.Add(-5 * time.Hour)},
		{"/h/p/old", now},
		{"/h", now},
		{"/h/gone", now},
	}
	repos := []string{"/h/p/shop", "/h/p/notes", "/h/p/new", "/h", "", "/h/p/shop"}
	got := findProjects(repos, seen, "/h", func(d string) string { return roots[d] })
	want := []project{
		{Name: "shop", Root: "/h/p/shop", Last: now.Add(-time.Hour)},
		{Name: "notes", Root: "/h/p/notes", Last: now.Add(-3 * time.Hour)},
		{Name: "new", Root: "/h/p/new"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("findProjects = %+v\nwant %+v", got, want)
	}
}

// The checkouts under home are the directories with a .git in them,
// to a depth, not looked into further; hidden directories, Library,
// node_modules and vendor are passed over, and so is a .git in home.
func TestRepositories(t *testing.T) {
	home := t.TempDir()
	for _, d := range []string{".git", "a/.git", "a/inner/.git", "b/c/.git", ".hidden/.git", "node_modules/x/.git", "Library/y/.git", "d/e/f/g/.git", "d/e/f/g/h/.git", "plain"} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got := repositories(home, 4)
	want := []string{filepath.Join(home, "a"), filepath.Join(home, "b/c"), filepath.Join(home, "d/e/f/g")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("repositories = %v\nwant %v", got, want)
	}
	if got := repositories(home, 1); !reflect.DeepEqual(got, want[:1]) {
		t.Errorf("repositories to a depth of one = %v", got)
	}
}

func TestParsePanes(t *testing.T) {
	out := "%1 @1 0 412 /dev/ttys004   abc-1     claude /Users/me/my project\n" +
		"%2 @2 1 0 /dev/ttys005 1       zsh /Users/me\n" +
		"%3 @3 0 77 /dev/ttys006    web check 0 1760000000 cat /p\n" +
		"%4 @1 0 78 /dev/ttys007  1      conn /Users/me\n"
	got := parsePanes(out)
	want := []pane{
		{ID: "%1", Window: "@1", PID: 412, TTY: "ttys004", Session: "abc-1", Command: "claude", Dir: "/Users/me/my project", Path: "/Users/me/my project"},
		{ID: "%2", Window: "@2", Dead: true, TTY: "ttys005", Panel: true, Command: "zsh", Dir: "/Users/me", Path: "/Users/me"},
		{ID: "%3", Window: "@3", PID: 77, TTY: "ttys006", Name: "web", Role: roleCheck, Exit: "0", Ended: time.Unix(1760000000, 0), Command: "cat", Dir: "/p", Path: "/p"},
		{ID: "%4", Window: "@1", PID: 78, TTY: "ttys007", Hold: true, Command: "conn", Dir: "/Users/me", Path: "/Users/me"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parsePanes = %+v\nwant %+v", got, want)
	}
	// A pane started for a project stands in it, wherever it runs.
	got = inProjects(got, byPane("%1 \n%2 \n%3 /Users/me/my project\n%4 \n"))
	if got[2].Dir != "/Users/me/my project" || got[2].Path != "/p" || got[0].Dir != "/Users/me/my project" {
		t.Errorf("inProjects = %+v", got)
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
	rows := arrange(convs, ps, live, nil, nil, func(d string) bool { return within(d, "/p") }, func(c conversation) string { return "t-" + c.ID }, roster{})
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
	// Conversations in start order, waiting where it stands; then the
	// pane opened by hand; then what runs elsewhere; then rest.
	want := []string{"marked@%1", "byhand@%2", "@%3", "away!", "rest"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("arrange = %v, want %v", got, want)
	}
	if rows[1].conv.Title != "t-byhand" || rows[1].state != "waiting" || rows[3].state != "working" {
		t.Errorf("rows = %+v", rows)
	}
	if rows[0].stand != placeConversations || rows[2].stand != placeClients || rows[3].stand != placeElsewhere || rows[4].stand != placeRest {
		t.Errorf("places = %d %d %d %d", rows[0].stand, rows[2].stand, rows[3].stand, rows[4].stand)
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
	m.box.SetValue("SHO")
	if got := m.shownProjects(); len(got) != 1 || got[0].Name != "shop" {
		t.Errorf("shop: %+v", got)
	}
	m.box.SetValue("w note")
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
		m.box.SetValue(typed)
		p, ok := m.typedPath()
		// The scratch directory is temporary, so it stands as itself.
		if !ok || p.Root != dir || p.Name != "new" {
			t.Errorf("typedPath(%q) = %+v, %v", typed, p, ok)
		}
	}
	m.box.SetValue("shop")
	if _, ok := m.typedPath(); ok {
		t.Error("a word was taken for a path")
	}
	m.box.SetValue("~/src/none")
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
	win, err := ensurePanel(srv, dir, "/nowhere/conn", panel, false)
	if err != nil {
		t.Fatal(err)
	}
	// The window named conn runs something else, so it is not conn's.
	if first, _ := srv.Display(sessionName+":0", "#{window_id}"); first == win {
		t.Fatalf("took the window named conn, %s", win)
	}
	again, err := ensurePanel(srv, dir, "/nowhere/conn", panel, false)
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
	got := machine(ps, nil, map[int][]string{22: {"3000", "3000"}}, parent, map[int][]string{22: {"5432"}})
	if len(got) != 2 {
		t.Fatalf("machine = %+v", got)
	}
	if got[0].Name != "web" || !reflect.DeepEqual(got[0].Ports, []string{"3000"}) || got[0].State != "running" || !reflect.DeepEqual(got[0].Peers, []string{"5432"}) {
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
	id, err := startNamed(srv, "check", "echo one; echo two; exit 3", dir, dir, roleCheck)
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
	if p == nil || p.Name != "check" || p.Exit != "3" || time.Since(p.Ended) > time.Minute {
		t.Fatalf("pane = %+v", p)
	}
	if how, err := endProc(srv, "sleep 1000", proc{pane: p, Name: "check"}); err != nil || how != "kill-pane "+id {
		t.Errorf("end = %q, %v", how, err)
	}
	// A server conn u started and that is still running is sent ctrl-c.
	id, _ = startNamed(srv, "server", "sleep 1000", dir, dir, roleStack)
	time.Sleep(300 * time.Millisecond)
	ps, _ = panes(srv)
	for i := range ps {
		if ps[i].ID == id {
			p = &ps[i]
		}
	}
	// Started again, it keeps its pane, under its name, with no end.
	pid, _ := srv.Display(id, "#{pane_pid}")
	if got, err := restartProc(srv, "conn", proc{pane: p, Name: "server"}); err != nil || got != id {
		t.Fatalf("restart = %q, %v", got, err)
	}
	time.Sleep(300 * time.Millisecond)
	ps, _ = panes(srv)
	for i := range ps {
		if ps[i].ID == id {
			p = &ps[i]
		}
	}
	if again, _ := srv.Display(id, "#{pane_pid}"); again == pid || again == "" || p.Name != "server" || p.Exit != "" {
		t.Errorf("the server was not started again in its pane: pid %s, then %s; pane %+v", pid, again, p)
	}
	if how, err := endProc(srv, "sleep 1000", proc{pane: p, Name: "server"}); err != nil || how != "send-keys C-c" {
		t.Errorf("end of a running server = %q, %v", how, err)
	}
	// Any other pane runs again what it was opened with, in place, and
	// x closes it.
	first, _ := srv.Display(sessionName+":0", "#{pane_id} #{pane_pid}")
	id, pid, _ = strings.Cut(first, " ")
	other := &pane{ID: id}
	if got, err := restartProc(srv, "conn", proc{pane: other, Name: "sleep"}); err != nil || got != id {
		t.Fatalf("restart = %q, %v", got, err)
	}
	if again, _ := srv.Display(id, "#{pane_pid}"); again == pid || again == "" {
		t.Errorf("the pane was not started again: pid %s, then %s", pid, again)
	}
	if how, err := endProc(srv, "sleep 1000", proc{pane: other, Name: "sleep"}); err != nil || how != "kill-pane "+id {
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
	if r, ok := next.(model).under(); !ok || r.pane.ID != "%2" {
		t.Errorf("cursor on %+v, want %%2", r)
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

// With conn's tab behind another, the terminal says what waits: the
// tab's title is "conn · n waiting", the count the status line has, and
// "conn" once nothing does; and the bell rings once as a conversation
// starts waiting, not again while it still waits, nor as one is
// answered.
func TestWaitingReachesTheTab(t *testing.T) {
	bin, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("no tmux")
	}
	script, err := exec.LookPath("script")
	if err != nil {
		t.Skip("no script to give a client a terminal")
	}
	dir, err := os.MkdirTemp("/tmp", "cs-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	path := write(t, filepath.Join(dir, "tmux.conf"), conf(theme.Dark))
	srv := &tmux.Server{Tmux: bin, Socket: filepath.Join(dir, "s")}
	defer func() { _ = srv.Down() }()
	if _, err := srv.Run("-f", path, "new-session", "-d", "-s", sessionName, "-x", "80", "-y", "24", "sleep 1000"); err != nil {
		t.Fatal(err)
	}
	// A terminal's client, on a terminal that takes a title, its output
	// what the terminal would be sent.
	attach := bin + " -S " + srv.Socket + " attach -t " + sessionName
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command(script, "-q", "/dev/null", "/bin/sh", "-c", attach)
	} else {
		cmd = exec.Command(script, "-qfec", attach, "/dev/null")
	}
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "TMUX=")
	in, keep, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = keep.Close() }()
	cmd.Stdin = in
	var out safeBuffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	for deadline := time.Now().Add(3 * time.Second); len(terminals(srv)) == 0; time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("no client attached: %q", out.String())
		}
	}
	time.Sleep(300 * time.Millisecond)

	// An operating system command ends in a BEL too, which is no bell:
	// a title is the one numbered 0 or 2.
	osc := regexp.MustCompile("\x1b\\]([0-9]*)(?:;([^\x07\x1b]*))?(?:\x07|\x1b\\\\)")
	title := func() string {
		got := ""
		for _, m := range osc.FindAllStringSubmatch(out.String(), -1) {
			if m[1] == "0" || m[1] == "2" {
				got = m[2]
			}
		}
		return got
	}
	bells := func() int {
		return strings.Count(osc.ReplaceAllString(out.String(), ""), "\a")
	}
	say := func(n int, wantTitle string, wantBells int) {
		t.Helper()
		sayWaiting(srv, n)
		deadline := time.Now().Add(2 * time.Second)
		for (title() != wantTitle || bells() < wantBells) && time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
		}
		// A bell too many would come at once.
		time.Sleep(300 * time.Millisecond)
		if got := title(); got != wantTitle {
			t.Errorf("with %d waiting the tab's title is %q, want %q", n, got, wantTitle)
		}
		if got := bells(); got != wantBells {
			t.Errorf("with %d waiting the bell has rung %d times, want %d", n, got, wantBells)
		}
	}
	say(1, "conn · 1 waiting", 1)
	say(1, "conn · 1 waiting", 1)
	say(2, "conn · 2 waiting", 2)
	say(1, "conn · 1 waiting", 2)
	say(0, "conn", 2)
	say(1, "conn · 1 waiting", 3)
}

// With conn's tab behind another and a conversation waiting, a run that
// fails anywhere reaches the tab as the waiting do: the title says
// "conn · 1 waiting · 1 failed", the status line says "1 failed" in the
// red of the failed row, and the bell rings once. Restarting it (rf), or
// dismissing it (xx), lowers the count without a bell, and at none the
// title is "conn · 1 waiting" again. What is read is the machine, as
// the panel reads it: tmux's panes, and Claude Code's instance files.
func TestAFailedRunReachesTheTab(t *testing.T) {
	bin, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("no tmux")
	}
	script, err := exec.LookPath("script")
	if err != nil {
		t.Skip("no script to give a client a terminal")
	}
	shots := os.Getenv("SHOTS")
	wear(theme.Dark)
	t.Cleanup(undress)
	g := theme.Dark

	scratch, err := os.MkdirTemp("/tmp", "cs-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(scratch) })
	t.Setenv("HOME", scratch)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(scratch, ".claude"))
	web := filepath.Join(scratch, "web")
	if err := os.MkdirAll(web, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, web, "init", "-q")
	git(t, web, "commit", "-q", "--allow-empty", "-m", "first")

	path := write(t, filepath.Join(scratch, "tmux.conf"), conf(g))
	srv := &tmux.Server{Tmux: bin, Socket: filepath.Join(scratch, "s")}
	t.Cleanup(func() { _ = srv.Down() })
	if _, err := srv.Run("-f", path, "new-session", "-d", "-s", sessionName, "-x", "120", "-y", "36", "-c", scratch, "sleep 1000"); err != nil {
		t.Fatal(err)
	}
	attach := bin + " -S " + srv.Socket + " attach -t " + sessionName
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command(script, "-q", "/dev/null", "/bin/sh", "-c", attach)
	} else {
		cmd = exec.Command(script, "-qfec", attach, "/dev/null")
	}
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "TMUX=")
	in, keepOpen, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = keepOpen.Close() })
	cmd.Stdin = in
	var out safeBuffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	for deadline := time.Now().Add(3 * time.Second); len(terminals(srv)) == 0; time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("no client attached: %q", out.String())
		}
	}
	time.Sleep(300 * time.Millisecond)

	osc := regexp.MustCompile("\x1b\\]([0-9]*)(?:;([^\x07\x1b]*))?(?:\x07|\x1b\\\\)")
	title := func() string {
		got := ""
		for _, m := range osc.FindAllStringSubmatch(out.String(), -1) {
			if m[1] == "0" || m[1] == "2" {
				got = m[2]
			}
		}
		return got
	}
	bells := func() int {
		return strings.Count(osc.ReplaceAllString(out.String(), ""), "\a")
	}

	// A conversation already waiting, in web.
	waiter := exec.Command("sleep", "1000")
	if err := waiter.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = waiter.Process.Kill(); _ = waiter.Wait() })
	write(t, filepath.Join(scratch, ".claude", "sessions", strconv.Itoa(waiter.Process.Pid)+".json"),
		`{"sessionId":"s1","status":"waiting","cwd":"`+web+`"}`)

	// The panel, open on web, as the panel reads the machine. The
	// scratch is temporary, which is no project; here it is one.
	k := newKnown()
	k.root[web] = web
	m := model{srv: srv, home: scratch, conn: "conn", known: k, g: g, loaded: true,
		screen: onProject, cur: project{Name: "web", Root: web}, width: panelWidth, height: 34}
	feed := func(msg tea.Msg) {
		if msg == nil {
			return
		}
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				if c != nil {
					next, _ := m.Update(c())
					m = next.(model)
				}
			}
			return
		}
		next, _ := m.Update(msg)
		m = next.(model)
	}
	press := func(keys string) {
		for _, r := range keys {
			next, c := m.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
			m = next.(model)
			if c != nil {
				feed(c())
			}
		}
	}

	var said strings.Builder
	shot := 0
	frame := func(name string) {
		if shots == "" {
			return
		}
		shot++
		fmt.Fprintf(&said, "%s: title %q, bells %d, status line %q\n", name, title(), bells(), regexp.MustCompile(`#\[[^\]]*\]`).ReplaceAllString(statusRight(t, srv), ""))
		if shot > 6 {
			return
		}
		ground := fmt.Sprintf("\x1b]10;%s\x1b\\\x1b]11;%s\x1b\\\x1b[2J\x1b[H", theme.Hex(g.Ink), g.Surface)
		// The panel at its width, the workspace beside it, and under
		// them the status line, which tmux draws.
		var screen strings.Builder
		for _, l := range strings.Split(m.View().Content, "\n") {
			screen.WriteString(l + "\x1b[0m" + strings.Repeat(" ", max(120-ansi.StringWidth(l), 0)) + "\r\n")
		}
		if err := os.WriteFile(filepath.Join(shots, name), []byte(ground+screen.String()+drawnStatus(t, srv)), 0o644); err != nil {
			t.Error(err)
		}
	}

	step := func(what, wantTitle string, wantBells int) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for {
			feed(m.read()())
			if title() == wantTitle && bells() >= wantBells || time.Now().After(deadline) {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		// A bell too many would come at once.
		time.Sleep(300 * time.Millisecond)
		feed(m.read()())
		time.Sleep(300 * time.Millisecond)
		if got := title(); got != wantTitle {
			t.Errorf("%s, the tab's title is %q, want %q", what, got, wantTitle)
		}
		if got := bells(); got != wantBells {
			t.Errorf("%s, the bell has rung %d times, want %d", what, got, wantBells)
		}
	}
	failedSaid := func(what string, n int) {
		t.Helper()
		status := statusRight(t, srv)
		word := fmt.Sprintf("%d failed", n)
		at := strings.Index(status, word)
		if n == 0 {
			if strings.Contains(status, "failed") {
				t.Errorf("%s, the status line still says what failed: %q", what, status)
			}
			return
		}
		if at < 0 {
			t.Errorf("%s, the status line does not say %q: %q", what, word, status)
			return
		}
		style := status[:at]
		if i := strings.LastIndex(style, "#["); i >= 0 {
			style = style[i:]
		}
		if !strings.Contains(strings.ToLower(style), strings.ToLower(g.Scheme[1])) {
			t.Errorf("%s, %q on the status line is not in the failed row's red %s: %q", what, word, g.Scheme[1], status)
		}
	}

	step("with a conversation waiting", "conn · 1 waiting", 1)
	failedSaid("with nothing failed", 0)
	frame("waiting-120x36.ans")

	// A check fails in web; started again, it runs.
	ran := filepath.Join(scratch, "ran")
	if _, err := startNamed(srv, "test", "if [ -e "+ran+" ]; then exec sleep 1000; fi; touch "+ran+"; exit 1", web, web, roleCheck); err != nil {
		t.Fatal(err)
	}
	step("with a check failed", "conn · 1 waiting · 1 failed", 2)
	failedSaid("with a check failed", 1)
	frame("failed-120x36.ans")
	step("with the check still failed", "conn · 1 waiting · 1 failed", 2)
	if len(m.failedRows()) != 1 {
		t.Fatalf("web's failed rows = %d, want the check", len(m.failedRows()))
	}

	// rf restarts it: the count falls, with no bell.
	press("rf")
	step("with the check restarted", "conn · 1 waiting", 2)
	failedSaid("with the check restarted", 0)
	frame("restarted-120x36.ans")

	// Another run fails, and rings again; xx on its row dismisses it.
	if _, err := startNamed(srv, "lint", "exit 2", web, web, roleCheck); err != nil {
		t.Fatal(err)
	}
	step("with the lint failed", "conn · 1 waiting · 1 failed", 3)
	failedSaid("with the lint failed", 1)
	frame("failed-again-120x36.ans")
	m.cursor = -1
	for i, it := range m.items() {
		if it.row >= 0 && strings.HasPrefix(m.rows[it.row].state, "failed ") {
			m.cursor = i
		}
	}
	if m.cursor < 0 {
		t.Fatal("the failed lint has no row in web")
	}
	press("xx")
	step("with the lint dismissed", "conn · 1 waiting", 3)
	failedSaid("with the lint dismissed", 0)
	frame("dismissed-120x36.ans")

	if shots != "" {
		if err := os.WriteFile(filepath.Join(shots, "tab-title-and-bell.txt"), []byte(said.String()), 0o644); err != nil {
			t.Error(err)
		}
	}
}

// statusRight is the right of conn's status line, as tmux expands it
// for the session: its words, with the styles they are drawn in.
func statusRight(t *testing.T, srv *tmux.Server) string {
	t.Helper()
	got, err := srv.Run("display", "-p", "-t", sessionName, "#{T:status-right}")
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimRight(got, "\n")
}

// drawnStatus draws conn's status line, 120 wide, as tmux would: what
// its left and right say in their #[fg=…,bold] styles, on the status
// line's own colours, and the key bar's row under it.
func drawnStatus(t *testing.T, srv *tmux.Server) string {
	t.Helper()
	g := theme.Dark
	left, err := srv.Run("display", "-p", "-t", sessionName, "#{T:status-left}")
	if err != nil {
		t.Fatal(err)
	}
	left, right := strings.TrimRight(left, "\n"), statusRight(t, srv)
	style := regexp.MustCompile(`#\[([^\]]*)\]`)
	rgb := func(hex string) string {
		r, _ := strconv.ParseUint(hex[1:3], 16, 8)
		gr, _ := strconv.ParseUint(hex[3:5], 16, 8)
		bl, _ := strconv.ParseUint(hex[5:7], 16, 8)
		return fmt.Sprintf("%d;%d;%d", r, gr, bl)
	}
	plain := "\x1b[0m\x1b[48;2;" + rgb(g.Border) + "m\x1b[38;2;" + rgb(g.Gray) + "m"
	draw := func(s string) string {
		var b strings.Builder
		last := 0
		for _, at := range style.FindAllStringSubmatchIndex(s, -1) {
			b.WriteString(s[last:at[0]])
			for _, attr := range strings.FieldsFunc(s[at[2]:at[3]], func(r rune) bool { return r == ',' || r == ' ' }) {
				switch {
				case attr == "default":
					b.WriteString(plain)
				case attr == "bold":
					b.WriteString("\x1b[1m")
				case strings.HasPrefix(attr, "fg=#") && len(attr) == 10:
					b.WriteString("\x1b[38;2;" + rgb(attr[3:]) + "m")
				}
			}
			last = at[1]
		}
		return b.String() + s[last:]
	}
	gap := 120 - utf8.RuneCountInString(style.ReplaceAllString(left, "")) - utf8.RuneCountInString(style.ReplaceAllString(right, ""))
	return plain + draw(left) + plain + strings.Repeat(" ", max(gap, 0)) + draw(right) + plain +
		"\x1b[0m\r\n\x1b[48;2;" + rgb(g.Surface) + "m" + strings.Repeat(" ", 120) + "\x1b[0m"
}

// safeBuffer is a buffer written by one goroutine and read by another.
type safeBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *safeBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
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
	rows := arrange(nil, ps, nil, map[string][]string{"%1": {"3000"}}, outside, func(d string) bool { return within(d, "/p") }, func(conversation) string { return "" }, roster{})
	if len(rows) != 2 || !reflect.DeepEqual(rows[0].ports, []string{"3000"}) || rows[1].outside == nil || rows[1].key() != "pid 77" {
		t.Fatalf("rows = %+v", rows)
	}
	if p, ok := rows[1].proc(); !ok || p.pane != nil || p.PID != 77 {
		t.Errorf("proc of an outside row = %+v, %v", p, ok)
	}
}

// Rows stand in their places by role: conversations, the stack, what
// talks to it, the checks, what is outside conn. In a place, what was
// written down stands as first written, whatever pane it runs in now,
// so a check run again keeps its row; a run that failed stays put.
func TestPlaces(t *testing.T) {
	in := func(d string) bool { return within(d, "/p") }
	starts := []started{
		{Root: "/p", Name: "web", Role: roleStack},
		{Root: "/p", Name: "test", Role: roleCheck},
		{Root: "/p", Name: "lint", Role: roleCheck},
		{Root: "/p", Name: "db", Role: roleClient},
		{Root: "/p", Name: "test", Command: "go test ./...", Role: roleCheck},
		{Root: "/q", Name: "first", Role: roleStack},
	}
	ros := rosterOf(starts, in)
	if ros.at["web"] != 0 || ros.at["test"] != 1 || ros.at["lint"] != 2 || ros.role["test"] != roleCheck || len(ros.at) != 3 {
		t.Errorf("roster = %+v", ros)
	}
	ps := []pane{
		{ID: "%9", Name: "test", Exit: "2", Dir: "/p"},
		{ID: "%2", Name: "lint", Exit: "0", Dir: "/p"},
		{ID: "%5", Name: "web", Dir: "/p"},
		{ID: "%6", Name: "watch", Dir: "/p"},
		{ID: "%7", Dir: "/p"},
		{ID: "%3", Session: "s1", Dir: "/p"},
	}
	outside := []proc{{Name: "postgres", PID: 77, Ports: []string{"5432"}, Dir: "/p"}}
	rows := arrange(nil, ps, nil, nil, outside, in, func(conversation) string { return "" }, ros)
	var got []string
	for _, r := range rows {
		switch {
		case r.outside != nil:
			got = append(got, r.outside.Name)
		case r.pane.Name != "":
			got = append(got, r.pane.Name)
		default:
			got = append(got, r.pane.ID)
		}
	}
	want := []string{"%3", "web", "watch", "%7", "test", "lint", "postgres"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("arrange = %v, want %v", got, want)
	}
	if rows[4].state != "failed 2" || rows[4].stand != placeChecks || rows[1].stand != placeStack || rows[3].stand != placeClients {
		t.Errorf("rows = %+v", rows)
	}
	// Run again in a new pane, a check is the same row.
	again := row{pane: &pane{ID: "%12", Name: "test"}, tree: "/p"}
	if again.key() != (row{pane: &pane{ID: "%9", Name: "test"}, tree: "/p"}).key() {
		t.Error("a check run again is not the row it was")
	}
	if again.key() == (row{pane: &pane{ID: "%12", Name: "test"}, tree: "/w"}).key() {
		t.Error("a check in another worktree is the same row")
	}
}

// The spinner turns in the margin beside a conversation working and a
// check running, in step with the clock, and not beside what rests or
// serves. Its tick starts once when a row turns, and stops itself
// when none does.
func TestSpinner(t *testing.T) {
	working := row{conv: &conversation{ID: "c", Title: "Fixing"}, pane: &pane{ID: "%1"}, state: "working"}
	idle := row{conv: &conversation{ID: "d", Title: "Resting"}, pane: &pane{ID: "%2"}, state: "idle"}
	check := row{pane: &pane{ID: "%3", Name: "test"}, stand: placeChecks}
	ended := row{pane: &pane{ID: "%4", Name: "lint", Exit: "0"}, stand: placeChecks, state: "done"}
	server := row{pane: &pane{ID: "%5", Name: "web"}, stand: placeStack}
	for _, c := range []struct {
		r    row
		want bool
	}{{working, true}, {idle, false}, {check, true}, {ended, false}, {server, false}} {
		if got := turns(c.r); got != c.want {
			t.Errorf("turns(%s) = %v, want %v", c.r.key(), got, c.want)
		}
	}
	m := model{screen: onProject, cur: project{Name: "d", Root: "/p"}, width: 60, height: 30, frame: 2}
	next, cmd := m.Update(read{root: "/p", rows: []row{idle, server}})
	if m = next.(model); m.spinning || cmd != nil {
		t.Error("the spinner started with nothing at work")
	}
	next, cmd = m.Update(read{root: "/p", rows: []row{working, check, idle}})
	if m = next.(model); !m.spinning || cmd == nil {
		t.Fatal("the spinner did not start when a row turned")
	}
	next, cmd = m.Update(read{root: "/p", rows: []row{working}})
	if m = next.(model); cmd != nil {
		t.Error("the spinner was started twice over")
	}
	if line := m.drawRow(working, 0, false); !strings.Contains(line, spinner[2]) {
		t.Errorf("a working row has no spinner turning: %q", line)
	}
	if line := m.drawRow(working, 0, true); !strings.Contains(line, cursorBar+spinner[2]) {
		t.Errorf("under the cursor, the spinner is not in the column after the bar: %q", line)
	}
	if line := m.drawRow(idle, 0, false); strings.ContainsAny(line, strings.Join(spinner, "")+steady) {
		t.Errorf("an idle row has a mark: %q", line)
	}
	// Another project's line turns while something in it is at work,
	// and keeps the tick going when nothing of the open project does.
	busy := project{Name: "e", Root: "/q", Running: 1, Turning: 1}
	still := project{Name: "f", Root: "/r", Running: 1}
	if line := m.projectLine(busy, false); !strings.Contains(line, spinner[2]) {
		t.Errorf("a project with something at work has no spinner turning: %q", line)
	}
	if line := m.projectLine(still, false); strings.ContainsAny(line, strings.Join(spinner, "")+steady) {
		t.Errorf("a project with nothing at work has a mark: %q", line)
	}
	m.projects = []project{m.cur, still}
	m.rows = []row{idle}
	if m.turning() {
		t.Error("the tick runs with nothing at work anywhere")
	}
	m.projects = []project{m.cur, busy}
	if !m.turning() {
		t.Error("the tick does not run for another project's line at work")
	}
	m.projects = nil
	m.rows = []row{working}
	// A process running persistently carries the steady cell, in the
	// same column; one that has ended, nothing.
	outside := row{outside: &proc{Name: "postgres", PID: 7}, stand: placeOutside, state: "outside conn"}
	for _, r := range []row{server, outside} {
		if !stands(r) {
			t.Errorf("%s does not stand", r.key())
		}
		if line := m.drawRow(r, 0, false); !strings.Contains(line, " "+steady+" ") {
			t.Errorf("a standing row has no steady mark: %q", line)
		}
	}
	for _, r := range []row{ended, check, working, idle, {pane: &pane{ID: "%6", Name: "web", Exit: "1"}, stand: placeStack, state: "failed 1"}} {
		if stands(r) {
			t.Errorf("%s stands", r.key())
		}
	}
	next, cmd = m.Update(spun{})
	if m = next.(model); !m.spinning || cmd == nil {
		t.Error("the tick did not go on while a row turns")
	}
	m.rows = []row{idle}
	next, _ = m.Update(spun{})
	if m = next.(model); m.spinning {
		t.Error("the tick went on with nothing at work")
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

// A process ended where it stands in the workspace leaves a hold in its
// place, and the window keeps its shape; one ended out of sight is
// closed.
func TestEndInPlace(t *testing.T) {
	srv, win, panel := scratchServer(t)
	ws := newWorkspace(srv, win, panel, "/x")
	ws.hold = "sleep 1000"
	// The workspace is told by the panel's mark, as attach sets it.
	if _, err := srv.Do(tmux.SetPane(panel, panelMark, "1")); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	id, err := startNamed(srv, "web", "sleep 1000", dir, dir, roleStack)
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.show(id, false); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	ps, _ := panes(srv)
	var p *pane
	for i := range ps {
		if ps[i].ID == id {
			p = &ps[i]
		}
	}
	if how, err := endProc(srv, ws.hold, proc{pane: p, Name: "web"}); err != nil || how != "send-keys C-c" {
		t.Errorf("end = %q, %v", how, err)
	}
	if h := inWorkspace(t, ws); h.ID != id || !h.Hold || h.Dead || h.Name != "" {
		t.Errorf("the ended process left %+v standing, want a hold in %s", h, id)
	}
	if w, _ := srv.Display(panel, "#{pane_width}"); w != strconv.Itoa(panelWidth) {
		t.Errorf("the panel is %s wide, want %d", w, panelWidth)
	}
	aside, _ := startNamed(srv, "worker", "sleep 1000", dir, dir, roleStack)
	time.Sleep(300 * time.Millisecond)
	ps, _ = panes(srv)
	for i := range ps {
		if ps[i].ID == aside {
			p = &ps[i]
		}
	}
	if how, err := endProc(srv, ws.hold, proc{pane: p, Name: "worker"}); err != nil || how != "send-keys C-c" {
		t.Errorf("end out of sight = %q, %v", how, err)
	}
	if _, gone := paneExit(srv, aside); !gone {
		t.Errorf("%s was not closed", aside)
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

// A panel started again over a process standing in the workspace, as a
// new build starts it, opens on that process: its project on the
// panel, the cursor to its row, the keys in it. Over a hold, it opens
// on the projects.
func TestOpensOnWhatStands(t *testing.T) {
	srv, win, panel := scratchServer(t)
	ws := newWorkspace(srv, win, panel, "/x")
	ws.hold = "sleep 1000"
	dir := t.TempDir()
	id, _ := openWindow(srv, "web", dir, "sleep 1000", "")
	if err := ws.show(id, false); err != nil {
		t.Fatal(err)
	}
	k := newKnown()
	k.root[dir] = dir
	m := model{srv: srv, ws: ws, known: k}
	next, cmd := m.Update(read{projects: []project{{Name: "d", Root: dir}}, shown: id, shownDir: dir, shownOK: true})
	m = next.(model)
	if m.screen != onProject || m.cur.Root != dir || m.follow != id {
		t.Errorf("opened on screen %d, project %q, following %q; want the project %q and %s", m.screen, m.cur.Root, m.follow, dir, id)
	}
	if b, ok := cmd().(tea.BatchMsg); ok && len(b) > 0 {
		b[0]()
	}
	if active, _ := srv.Display(id, "#{pane_active}"); active != "1" {
		t.Errorf("the keys are not in %s", id)
	}
	if err := ws.show("", false); err != nil {
		t.Fatal(err)
	}
	m = model{srv: srv, ws: ws, known: k}
	next, _ = m.Update(read{projects: []project{{Name: "d", Root: dir}}, shownOK: true})
	if m = next.(model); m.screen != onProjects || m.follow != "" {
		t.Errorf("over a hold, opened on screen %d following %q", m.screen, m.follow)
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

func TestWrapped(t *testing.T) {
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
	rows := arrange(nil, ps, nil, nil, nil, func(d string) bool { return within(d, "/p") }, func(conversation) string { return "" }, roster{})
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
		{Name: "conn", Root: "/p/w/conn", Last: now.Add(-time.Hour), Running: 2, Turning: 1},
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
	if f := got[0]; !f.Group || f.Running != 2 || f.Waiting != 1 || f.Turning != 1 || !f.Last.Equal(now.Add(-time.Hour)) {
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
	m.box.SetValue("essays")
	if got := names(); !reflect.DeepEqual(got, []string{"w", "essays"}) {
		t.Errorf("essays: %v", got)
	}
	m.box.SetValue("/p/w ")
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

// conn u with no name runs something once: a row under its command
// while its pane lives, a check where it waited, and written down
// nowhere, so what an agent runs to see something does not pile up as
// what the project runs.
func TestRunUUnderNoNameIsNotWrittenDown(t *testing.T) {
	srv, _, _ := scratchServer(t)
	t.Setenv("CONN_SOCKET", srv.Socket)
	dir := filepath.Dir(srv.Socket)
	var out strings.Builder
	if code := runU(&out, t.TempDir(), dir, []string{"-w", "--", "echo once; exit 3"}); code != 3 || !strings.Contains(out.String(), "once") || strings.Contains(out.String(), "exited") {
		t.Fatalf("conn u -w = %d: %q", code, out.String())
	}
	if got := readStarted(startedPath(srv.Socket)); len(got) != 0 {
		t.Errorf("written down: %+v", got)
	}
	ps, err := panes(srv)
	if err != nil {
		t.Fatal(err)
	}
	var run *pane
	for i := range ps {
		if ps[i].Run != "" {
			run = &ps[i]
		}
	}
	if run == nil || run.Name != "" || run.Run != "echo once; exit 3" || run.Role != roleCheck || run.Exit != "3" {
		t.Fatalf("pane = %+v", run)
	}
	in := func(string) bool { return true }
	r := row{pane: run, state: "failed 3"}
	if paneName(*run) != "echo once; exit 3" || placeOf(r, rosterOf(nil, in)) != placeChecks {
		t.Errorf("row = %q in place %d", paneName(*run), placeOf(r, rosterOf(nil, in)))
	}
	if state, _ := paneState(*run, nil); state != "failed 3" {
		t.Errorf("state = %q", state)
	}
	// Handed to an agent, it answers to its pid and runs again under
	// no name.
	m := model{srv: srv}
	h, ok := m.handing(r)
	if !ok || h.named || h.addr != strconv.Itoa(run.PID) {
		t.Fatalf("handing = %+v", h)
	}
	prompt := handPrompt(h)
	for _, want := range []string{"conn l " + strconv.Itoa(run.PID) + " reads", "conn u -w -- echo once; exit 3 runs it again"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt lacks %q:\n%s", want, prompt)
		}
	}
	// Up and not waited on, it is the stack's, under its command.
	out.Reset()
	if code := runU(&out, t.TempDir(), dir, []string{"--", "sleep 1000"}); code != 0 || !strings.HasPrefix(out.String(), "sleep 1000 is up in pane %") || !strings.Contains(out.String(), "; conn l ") || strings.Contains(out.String(), "conn l sleep") {
		t.Fatalf("conn u = %d: %q", code, out.String())
	}
	if got := readStarted(startedPath(srv.Socket)); len(got) != 0 {
		t.Errorf("written down: %+v", got)
	}
}

// conn u on a name that is up starts it again in its pane with the
// command given now: a name is one row across its runs, and an agent
// that runs a check again does not need a new name for it.
func TestRunUOnANameUpStartsItAgain(t *testing.T) {
	srv, _, _ := scratchServer(t)
	t.Setenv("CONN_SOCKET", srv.Socket)
	dir := filepath.Dir(srv.Socket)
	var out strings.Builder
	if code := runU(&out, t.TempDir(), dir, []string{"web", "--", "sleep 1000"}); code != 0 {
		t.Fatalf("conn u web = %d: %s", code, out.String())
	}
	before, _ := findByName(t, srv, "web")
	out.Reset()
	if code := runU(&out, t.TempDir(), dir, []string{"web", "-w", "--", "echo again"}); code != 0 {
		t.Fatalf("conn u web again = %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "again") {
		t.Errorf("the second run said %q; want its own output", out.String())
	}
	after, n := findByName(t, srv, "web")
	if n != 1 || after.ID != before.ID || after.Exit != "0" {
		t.Errorf("after the second run web is %d panes, %s exit %q; want the one pane %s, done", n, after.ID, after.Exit, before.ID)
	}
}

// findByName is the pane a name answers to, and how many do.
func findByName(t *testing.T, srv *tmux.Server, name string) (pane, int) {
	t.Helper()
	ps, err := panes(srv)
	if err != nil {
		t.Fatal(err)
	}
	var found pane
	n := 0
	for _, p := range ps {
		if p.Name == name {
			found, n = p, n+1
		}
	}
	return found, n
}

// conn u from a pane of conn's starts the command where the shell is,
// for the pane's project: an agent that goes into another checkout to
// run a check there is still working its own.
func TestRunUFromAPaneIsThePanesProject(t *testing.T) {
	srv, _, _ := scratchServer(t)
	t.Setenv("CONN_SOCKET", srv.Socket)
	own, other := filepath.Join(filepath.Dir(srv.Socket), "own"), filepath.Join(filepath.Dir(srv.Socket), "other")
	for _, d := range []string{own, other} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	out, err := srv.Run("new-window", "-d", "-t", sessionName+":", "-c", own, "-P", "-F", "#{pane_id}", "sleep 1000")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMUX", srv.Socket+",1,0")
	t.Setenv("TMUX_PANE", strings.TrimSpace(out))
	var said strings.Builder
	if code := runU(&said, t.TempDir(), other, []string{"site", "-w", "--", "pwd"}); code != 0 {
		t.Fatalf("conn u site -w = %d: %s", code, said.String())
	}
	if !strings.Contains(said.String(), realDir(other)) {
		t.Errorf("site ran in %q, not the shell's directory %s", said.String(), other)
	}
	got := readStarted(startedPath(srv.Socket))
	if len(got) != 1 || got[0].Root != realDir(own) {
		t.Errorf("written down as %+v, not the pane's project %s", got, own)
	}
	ps, err := panes(srv)
	if err != nil {
		t.Fatal(err)
	}
	var site pane
	for _, p := range ps {
		if p.Name == "site" {
			site = p
		}
	}
	if site.Dir != realDir(own) || site.Path != realDir(other) {
		t.Errorf("site stands in %q and runs in %q; want %s and %s", site.Dir, site.Path, own, other)
	}
	// Named from the pane, the check is its project's to read and end;
	// conn ls there lists it, and says where it ran.
	said.Reset()
	if code := runLs(&said, t.TempDir(), other, nil); code != 0 || !strings.Contains(said.String(), "site") || !strings.Contains(said.String(), realDir(other)) {
		t.Errorf("conn ls from the pane = %d:\n%s", code, said.String())
	}
}

// st runs the checks as they were last run here, with no question; ss,
// with no stack run here yet, asks.
func TestResolveRunsWhatWasRun(t *testing.T) {
	srv, _, _ := scratchServer(t)
	dir := filepath.Dir(srv.Socket)
	if err := recordStart(startedPath(srv.Socket), started{Root: dir, Name: "test", Command: "echo ok", Role: roleCheck}); err != nil {
		t.Fatal(err)
	}
	m := model{srv: srv, screen: onProject, cur: project{Name: "d", Root: dir}, rows: []row{}}
	next, _ := m.Update(tea.KeyPressMsg(tea.Key{Text: "s", Code: 's'}))
	next, cmd := next.(model).Update(tea.KeyPressMsg(tea.Key{Text: "t", Code: 't'}))
	m = next.(model)
	if m.question != "" || cmd == nil {
		t.Fatalf("t asked a question, or did nothing: question %q", m.question)
	}
	// The start comes first, then the read once the watch is over.
	batch, _ := cmd().(tea.BatchMsg)
	if len(batch) == 0 {
		t.Fatal("t started nothing")
	}
	if said, ok := batch[0]().(acted); !ok || string(said) != "test is up" {
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
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Text: "s", Code: 's'}))
	next, _ = next.(model).Update(tea.KeyPressMsg(tea.Key{Text: "s", Code: 's'}))
	if next.(model).question != roleStack {
		t.Error("ss did not ask what the stack is")
	}
}

// A run that failed reads failed with its status and stays where it
// stands; one that ended well reads done. With nothing written down,
// named panes stand in start order.
func TestArrangeFailedStays(t *testing.T) {
	ps := []pane{{ID: "%2", Name: "web", Dir: "/p"}, {ID: "%5", Name: "lint", Exit: "0", Dir: "/p"}, {ID: "%7", Name: "test", Exit: "1", Dir: "/p"}}
	rows := arrange(nil, ps, nil, nil, nil, func(d string) bool { return within(d, "/p") }, func(conversation) string { return "" }, roster{})
	var got []string
	for _, r := range rows {
		got = append(got, r.pane.Name+" "+or(r.state, "running"))
	}
	if want := []string{"web running", "lint done", "test failed 1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("rows = %v", got)
	}
}

// A failed check is handed with what it runs, how it ended, its last
// lines less conn's own closing line, and what runs it again; several
// that failed go to one agent together.
func TestHandPrompt(t *testing.T) {
	lines := []string{"--- FAIL: TestX", "FAIL", "", "[test exited 1]", ""}
	got := handPrompt(handing{name: "test", addr: "test", named: true, command: "go test ./...", dir: "~/p", state: "failed 1", lines: lines})
	for _, want := range []string{"In this project, test runs go test ./... at ~/p, and it failed 1.", "--- FAIL: TestX\nFAIL\n", "Work out what is wrong and fix it.", "conn l test reads", "conn u test -w -- go test ./... runs it again."} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "[test exited") {
		t.Errorf("prompt keeps conn's closing line:\n%s", got)
	}
	running := handPrompt(handing{name: "web", addr: "web", named: true, command: "npm run dev", dir: "~/p", state: "running"})
	if !strings.Contains(running, "and it is running.") || !strings.Contains(running, "Look into it.") || strings.Contains(running, "runs it again") {
		t.Errorf("prompt for a running process:\n%s", running)
	}
	all := handAllPrompt([]handing{{name: "test", addr: "test", named: true, command: "go test ./...", dir: "~/p", state: "failed 1", lines: lines}, {name: "lint", addr: "lint", named: true, command: "make lint", dir: "~/p", state: "failed 2", lines: []string{"lint: bad"}}})
	for _, want := range []string{"2 runs in this project failed.", "test runs go test ./...", "conn u lint -w -- make lint runs it again.", "lint: bad", "with each and fix it."} {
		if !strings.Contains(all, want) {
			t.Errorf("prompt for two lacks %q:\n%s", want, all)
		}
	}
	many := make([]string, 200)
	for i := range many {
		many[i] = strings.Repeat("x", 300)
	}
	if got := lastLines(many, 80, 16<<10); len(got) != 54 {
		t.Errorf("lastLines kept %d lines", len(got))
	}
}

// Handing a process to an agent starts the conversation and leaves it
// to it: the agent has its brief, so nothing is to be typed, and the
// keys stay on the panel where they were, the cursor on the row handed.
// The agent's row comes with the next read, the line says who was
// handed, and the brief waits for the agent in a file that it reads
// and lets go.
func TestHandingLeavesTheAgentToIt(t *testing.T) {
	bin, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("no tmux")
	}
	scratch, err := os.MkdirTemp("/tmp", "cs-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(scratch) })
	// git and tmux say where things are with /tmp's symlink resolved.
	scratch, _ = filepath.EvalSymlinks(scratch)
	t.Setenv("HOME", scratch)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(scratch, ".claude"))
	// claude, as conn starts it with the brief, waits.
	write(t, filepath.Join(scratch, "bin", "claude"), "#!/bin/sh\nexec sleep 1000\n")
	if err := os.Chmod(filepath.Join(scratch, "bin", "claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Join(scratch, "bin")+":"+os.Getenv("PATH"))
	web := filepath.Join(scratch, "web")
	if err := os.MkdirAll(web, 0o755); err != nil {
		t.Fatal(err)
	}

	srv := &tmux.Server{Tmux: bin, Socket: filepath.Join(scratch, "s")}
	t.Cleanup(func() { _ = srv.Down() })
	if _, err := srv.Run("-f", "/dev/null", "new-session", "-d", "-s", sessionName, "sleep 1000"); err != nil {
		t.Fatal(err)
	}
	// A process of the project's, running, to hand.
	held, err := openWindow(srv, "web", web, "sleep 1000", "")
	if err != nil {
		t.Fatal(err)
	}

	// The scratch is temporary, which is no project; here it is one.
	k := newKnown()
	k.root[web] = web
	m := model{srv: srv, screen: onProject, cur: project{Name: "web", Root: web}, home: scratch, width: 120, height: 36, known: k}
	read := func(m model) model {
		next, _ := m.Update(m.read()())
		return next.(model)
	}
	m = read(m)
	if len(m.rows) != 1 || m.rows[0].pane == nil || m.rows[0].pane.ID != held {
		t.Fatalf("the read found %d rows, want the held process", len(m.rows))
	}
	press := func(m model, s string) (model, tea.Cmd) {
		var cmd tea.Cmd
		for _, r := range s {
			var next tea.Model
			next, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
			m = next.(model)
		}
		return m, cmd
	}
	m, cmd := press(m, "aa")
	if m.in != "" || m.follow != "" {
		t.Errorf("after aa the keys are in %q, following %q; want them left on the panel", m.in, m.follow)
	}
	if m.cursor != 0 {
		t.Errorf("after aa the cursor is on %d, want the row handed", m.cursor)
	}
	if !strings.HasSuffix(m.note, " handed to an agent.") {
		t.Errorf("after aa the line says %q", m.note)
	}
	if cmd == nil {
		t.Fatal("aa asked for no read")
	}
	next, _ := m.Update(cmd())
	m = next.(model)
	var agent *pane
	for _, r := range m.rows {
		if r.pane != nil && r.pane.Session != "" {
			agent = r.pane
		}
	}
	if agent == nil {
		t.Fatalf("no row for the agent among %d", len(m.rows))
	}
	// The agent reads its brief and lets the file go.
	file := promptPath(srv.Socket, agent.Session)
	gone := false
	for i := 0; i < 50 && !gone; i++ {
		if _, err := os.Stat(file); err != nil {
			gone = true
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !gone {
		t.Errorf("the brief still waits at %s", file)
	}
	if m.in != "" {
		t.Errorf("after the read the keys are in %q; want them left on the panel", m.in)
	}
}

// A conversation is told the project's checks and stack as last run.
func TestAgentNote(t *testing.T) {
	note := agentNote("conn", []stacked{{name: "web", command: "npm run dev"}}, []stacked{{name: "test", command: "go test ./..."}})
	for _, want := range []string{"In this project, as last run through conn:\n  conn u test -w -- go test ./...\n  conn u web -- npm run dev", "conn u -w -- COMMAND", "A name is a declaration", "Everything else runs under no name", "Never make up a name for a run"} {
		if !strings.Contains(note, want) {
			t.Errorf("note lacks %q:\n%s", want, note)
		}
	}
	if bare := agentNote("/x/conn", nil, nil); strings.Contains(bare, "as last run") || !strings.Contains(bare, "conn here is /x/conn") {
		t.Errorf("bare note:\n%s", bare)
	}
}

// x on a project ends everything conn holds in it and nothing in
// another project.
func TestEndAll(t *testing.T) {
	srv, _, _ := scratchServer(t)
	dir := filepath.Dir(srv.Socket)
	other := t.TempDir()
	for _, p := range [][2]string{{"web", dir}, {"worker", dir}, {"theirs", other}} {
		if _, err := startNamed(srv, p[0], "sleep 1000", p[1], realDir(p[1]), roleStack); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(300 * time.Millisecond)
	// A directory under /tmp is scratch and no project; the test says
	// which project each is.
	k := newKnown()
	k.root[realDir(dir)], k.root[realDir(other)] = realDir(dir), realDir(other)
	m := model{srv: srv, known: k}
	got := m.endAll(project{Name: "d", Root: realDir(dir)})().(acted)
	if string(got) != "d: ended 2" {
		t.Errorf("endAll said %q", got)
	}
	ps, _ := panes(srv)
	var left []string
	for _, p := range ps {
		if p.Name != "" {
			left = append(left, p.Name)
		}
	}
	if !reflect.DeepEqual(left, []string{"theirs"}) {
		t.Errorf("left: %v", left)
	}
}

// A client is kept under a process's name without taking the name from
// the process: web is still the stack, and c on it is the browser.
func TestClientFor(t *testing.T) {
	starts := []started{
		{Root: "/p", Name: "web", Command: "npm run dev", Role: roleStack},
		{Root: "/p", Name: "web", Command: "http://localhost:5173", Role: roleClient},
		{Root: "/p", Name: "db", Command: "psql app", Role: roleClient},
		{Root: "/p", Name: "db", Command: "psql -d app", Role: roleClient},
	}
	in := func(root string) bool { return root == "/p" }
	if stack := established(starts, in, roleStack); len(stack) != 1 || stack[0].name != "web" {
		t.Errorf("stack = %+v", stack)
	}
	if c, ok := clientFor(starts, in, "db"); !ok || c.command != "psql -d app" {
		t.Errorf("db's client = %+v, %v", c, ok)
	}
	if c, ok := clientFor(starts, in, "web"); !ok || !isURL(c.command) {
		t.Errorf("web's client = %+v, %v", c, ok)
	}
	if _, ok := clientFor(starts, in, "worker"); ok {
		t.Error("worker has a client")
	}
}

func TestParseClient(t *testing.T) {
	if got, err := parseClient("```\npsql postgres://localhost/app\n```\n"); err != nil || got != "psql postgres://localhost/app" {
		t.Errorf("parseClient = %q, %v", got, err)
	}
	if _, err := parseClient("NONE\n"); err == nil {
		t.Error("NONE was taken for a client")
	}
	if _, err := parseClient("\n\n"); err == nil {
		t.Error("nothing was taken for a client")
	}
}

// A port that answers HTTP speaks it; one that answers something else,
// or nothing, does not.
func TestSpeaksHTTP(t *testing.T) {
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("hi")) }))
	defer web.Close()
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(web.URL, "http://"))
	if !speaksHTTP(port) {
		t.Error("a web server does not speak HTTP")
	}
	raw, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	go func() {
		for {
			c, err := raw.Accept()
			if err != nil {
				return
			}
			_, _ = c.Write([]byte("E\x00\x00\x00"))
			_ = c.Close()
		}
	}()
	_, port, _ = net.SplitHostPort(raw.Addr().String())
	if speaksHTTP(port) {
		t.Error("a raw listener speaks HTTP")
	}
	if speaksHTTP("1") {
		t.Error("a closed port speaks HTTP")
	}
}

// c on a conversation goes into it; on a named run whose client is
// kept, it opens the client with no question.
func TestTalk(t *testing.T) {
	srv, _, _ := scratchServer(t)
	dir := filepath.Dir(srv.Socket)
	m := model{srv: srv, screen: onProject, cur: project{Name: "d", Root: realDir(dir)}}
	conv := row{conv: &conversation{ID: "c1"}, pane: &pane{ID: "%9", Session: "c1"}}
	next, _ := m.talk(conv)
	if next.(model).asked != "%9" {
		t.Error("c on a conversation did not go to it")
	}
	if err := recordStart(startedPath(srv.Socket), started{Root: realDir(dir), Name: "db", Command: "sleep 1000", Role: roleClient}); err != nil {
		t.Fatal(err)
	}
	next, cmd := m.talk(row{pane: &pane{ID: "%3", Name: "db", Dir: realDir(dir)}})
	m = next.(model)
	if m.question != "" || m.follow == "" || cmd == nil {
		t.Fatalf("c on db asked, or opened nothing: question %q, follow %q", m.question, m.follow)
	}
	// The pane runs its shell for a moment before the client.
	found := false
	for deadline := time.Now().Add(2 * time.Second); !found && time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		ps, _ := panes(srv)
		for _, p := range ps {
			found = found || (p.ID == m.follow && p.Command == "sleep")
		}
	}
	if !found {
		t.Errorf("no pane %s running the client", m.follow)
	}
}

// A listener outside conn is the project's where a pane of the
// project's connects to it, and the connection is read as the peer's
// port on this machine.
func TestTalksTo(t *testing.T) {
	got := parseConnections("p40\nn127.0.0.1:52011->127.0.0.1:5432\nn10.0.0.2:52012->93.184.216.34:443\np41\nn[::1]:52013->[::1]:6379\nnstray\n")
	if !reflect.DeepEqual(got, map[int][]string{40: {"5432"}, 41: {"6379"}}) {
		t.Errorf("parseConnections = %v", got)
	}
	web := proc{pane: &pane{ID: "%1"}, Name: "web", Peers: []string{"5432"}}
	db := proc{Name: "postgres", Ports: []string{"5432"}, Dir: "/"}
	cache := proc{Name: "redis", Ports: []string{"6379"}, Dir: "/"}
	if !talksTo([]proc{web}, db) || talksTo([]proc{web}, cache) {
		t.Error("web talks to the database and not the cache")
	}
	// Only what conn holds is asked what it connects to.
	if talksTo([]proc{{Name: "python", Peers: []string{"5432"}}}, db) {
		t.Error("a process outside conn made the database the project's")
	}
}

// Every row carries the mark of its role before its name, in the
// name's own ink; the mark stands in the column after the margin, so
// the spinner and the cursor's bar keep theirs.
func TestRowsCarryTheMarkOfTheirRole(t *testing.T) {
	m := model{screen: onProject, cur: project{Root: "/p"}, width: 60}
	for _, c := range []struct {
		row  row
		mark string
	}{
		{row{conv: &conversation{ID: "c", Title: "Fixing"}, pane: &pane{ID: "%1"}, stand: placeConversations}, markConversation},
		{row{pane: &pane{ID: "%2", Name: "web"}, stand: placeStack}, markStack},
		{row{pane: &pane{ID: "%3", Command: "zsh"}, stand: placeClients}, markClient},
		{row{pane: &pane{ID: "%4", Name: "test"}, stand: placeChecks, state: "running"}, markCheck},
		{row{pane: &pane{ID: "%4", Name: "test", Exit: "0"}, stand: placeChecks, state: "done"}, markPass},
		{row{pane: &pane{ID: "%4", Name: "test", Exit: "1"}, stand: placeChecks, state: "failed 1"}, markFail},
		{row{outside: &proc{Name: "postgres"}, stand: placeOutside, state: "outside conn"}, markOutside},
		{row{conv: &conversation{ID: "e", Title: "Away"}, elsewhere: true, stand: placeElsewhere}, markConversation},
	} {
		line := []rune(plainText(m.drawRow(c.row, 0, false)))
		if len(line) <= margin+1 || string(line[margin]) != c.mark || line[margin+1] != ' ' {
			t.Errorf("%s row draws %q, want %q after the margin", c.mark, string(line), c.mark)
		}
	}
}

// A verdict is drawn in the theme's own colors: a pass in the running
// green, a fail in the terminal's red with its name, and both keep
// their ink under the cursor, on the chosen ground. The red goes to
// the name where the row says where it was started as well. The ground is one
// whose spinner has a hue of its own, so a pass is seen to be drawn
// running and not turning.
func TestAVerdictWearsTheGround(t *testing.T) {
	g := theme.Dark
	g.Turning = "#C8ABFF"
	wear(g)
	t.Cleanup(undress)
	sgr := func(hex string) string {
		c := theme.RGB(hex)
		return fmt.Sprintf("38;2;%d;%d;%d", c.R, c.G, c.B)
	}
	m := model{screen: onProject, cur: project{Root: "/p"}, home: "/home", width: 60}
	pass := row{pane: &pane{ID: "%4", Name: "test", Dir: "/p", Path: "/p", Exit: "0"}, stand: placeChecks, state: "done"}
	fail := row{pane: &pane{ID: "%5", Name: "build", Dir: "/p/sub", Path: "/p/sub", Exit: "1"}, stand: placeChecks, state: "failed 1"}
	for _, c := range []struct {
		row    row
		cursor bool
		want   string
	}{
		{pass, false, sgr(g.Running) + "m" + markPass},
		{fail, false, sgr(g.Scheme[1]) + "m" + markFail},
		{fail, false, sgr(g.Scheme[1]) + "mbuild"},
		{pass, true, sgr(g.Running) + ";48;2"},
		{fail, true, sgr(g.Scheme[1]) + ";48;2"},
	} {
		if line := m.drawRow(c.row, 0, c.cursor); !strings.Contains(line, c.want) {
			t.Errorf("cursor %v: %q\nlacks %q", c.cursor, line, c.want)
		}
	}
	// The row under the cursor is still painted edge to edge.
	if line := plainText(m.drawRow(fail, 0, true)); !strings.HasSuffix(strings.TrimRight(line, " "), "/p/sub") || ansi.StringWidth(line) != m.width {
		t.Errorf("under the cursor the row is %d wide, want %d, and reads %q", ansi.StringWidth(line), m.width, line)
	}
}

// undress puts the panel's styles back to plain, as the tests have
// them.
func undress() {
	var plain lipgloss.Style
	ink, title, projectTitle, subhead, gray, faint, accent, failing, keyInk = plain, plain, plain, plain, plain, plain, plain, plain, plain
	chosen, chosenBar, ruleStyle, turning, passed, chosenTurning = plain, plain, plain, plain, plain, plain
}

// plainText is a drawn line with its styles taken off.
func plainText(s string) string {
	return regexp.MustCompile("\x1b\\[[0-9;]*m").ReplaceAllString(s, "")
}

// A run that has ended says how long ago in the figure column, which
// says it ended well; a process of the stack that did not says its
// status first, the accent saying it failed, and a check's mark says
// its verdict. One from before conn wrote the time down says only how
// it did.
func TestARestingRunSaysWhen(t *testing.T) {
	m := model{screen: onProject, cur: project{Root: "/p"}, width: 60}
	for _, c := range []struct {
		stand int
		exit  string
		ago   time.Duration
		state string
		want  string
	}{
		{placeChecks, "0", 4 * time.Minute, "done", "4m"},
		{placeChecks, "2", 9 * time.Minute, "failed 2", "9m"},
		{placeChecks, "0", 0, "done", markPass + " test"},
		{placeChecks, "1", 0, "failed 1", markFail + " test"},
		{placeStack, "0", 4 * time.Minute, "done", "4m"},
		{placeStack, "2", 9 * time.Minute, "failed 2", "2 9m"},
		{placeStack, "0", 0, "done", "done"},
		{placeStack, "2", 0, "failed 2", "2"},
	} {
		p := pane{ID: "%1", Name: "test", Exit: c.exit}
		if c.ago > 0 {
			p.Ended = time.Now().Add(-c.ago)
		}
		line := strings.TrimSpace(m.drawRow(row{pane: &p, stand: c.stand, state: c.state}, 0, false))
		if !strings.HasSuffix(line, c.want) || (c.exit == "0" && c.ago > 0 && strings.ContainsAny(line, "dp")) {
			t.Errorf("a run that ended %s %s ago draws %q, want it to end in %q", c.exit, c.ago, line, c.want)
		}
	}
}

// conn theme claude writes the theme on the ground it is given, and
// offers to put Claude Code on it where its settings name one of its
// own themes; a custom theme of the user's is left, and said.
func TestDressClaude(t *testing.T) {
	home := t.TempDir()
	settings := filepath.Join(home, ".claude", "settings.json")
	write(t, settings, `{"theme": "dark"}`)
	var out strings.Builder
	no := func(string) bool { return false }
	if err := dressClaude(&out, home, no, theme.Light); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(home, ".claude", "themes", "conn.json"))
	if !strings.Contains(string(b), `"base": "light-ansi"`) || !strings.Contains(out.String(), "/theme") {
		t.Errorf("declined: file %q, said %q", b, out.String())
	}
	out.Reset()
	if err := dressClaude(&out, home, func(string) bool { return true }, theme.Dark); err != nil {
		t.Fatal(err)
	}
	if in, _ := theme.ThemeInUse(home); in != theme.ClaudeThemeRef || !strings.Contains(out.String(), theme.ClaudeThemeRef) {
		t.Errorf("accepted: on %q, said %q", in, out.String())
	}
	write(t, settings, `{"theme": "custom:mine"}`)
	out.Reset()
	if err := dressClaude(&out, home, func(string) bool { t.Error("asked over a custom theme"); return true }, theme.Dark); err != nil {
		t.Fatal(err)
	}
	if in, _ := theme.ThemeInUse(home); in != "custom:mine" || !strings.Contains(out.String(), "custom:mine") {
		t.Errorf("custom: on %q, said %q", in, out.String())
	}
}

// A server's ground is the terminal's the first time, unless a flag
// said; its theme what a flag or config.json names, or conn's; both
// held while it is up, and put on another where it stands when a flag
// says so, which is when conn's own pane is started again.
func TestChooseGround(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	socket := filepath.Join(t.TempDir(), "s")
	light, dark := false, true
	asked := 0
	detect := func() bool { asked++; return false }
	choose := func(up bool, want theme.Want) (theme.Ground, bool) {
		t.Helper()
		g, again, err := chooseGround(home, socket, up, want, detect)
		if err != nil {
			t.Fatal(err)
		}
		return g, again
	}
	if g, again := choose(false, theme.Want{}); g != theme.Light || again || asked != 1 {
		t.Errorf("a server rising on a light terminal: dark %v, again %v, asked %d", g.Dark(), again, asked)
	}
	if g, again := choose(true, theme.Want{}); g.Dark() || again || asked != 1 {
		t.Errorf("a server up keeps its ground: dark %v, again %v, asked %d", g.Dark(), again, asked)
	}
	if g, again := choose(true, theme.Want{Dark: &light}); g.Dark() || again {
		t.Errorf("--light on a light server: dark %v, again %v", g.Dark(), again)
	}
	if g, again := choose(true, theme.Want{Dark: &dark}); !g.Dark() || !again {
		t.Errorf("--dark on a light server: dark %v, again %v", g.Dark(), again)
	}
	if m, ok := theme.ReadMode(socket); !ok || m != (theme.Mode{Dark: true, Theme: "conn"}) {
		t.Errorf("the flag's ground was not written down: %+v", m)
	}
	theme.ForgetMode(socket)
	if g, again := choose(false, theme.Want{Dark: &light}); g.Dark() || again || asked != 1 {
		t.Errorf("--light on a server rising: dark %v, again %v, asked %d", g.Dark(), again, asked)
	}

	// A sheet of one ground, its accent its own, worn by name.
	b, err := os.ReadFile("internal/theme/sheets/conn.json")
	if err != nil {
		t.Fatal(err)
	}
	sheet := strings.Replace(string(b), `"accent": "#BD3A1D"`, `"accent": "#123456"`, 1)
	write(t, filepath.Join(home, ".config", "conn", "themes", "paper.json"), sheet)
	if g, again := choose(true, theme.Want{Theme: "paper"}); g.Accent != "#123456" || g.Dark() || !again {
		t.Errorf("--theme paper on a server up: accent %s, again %v", g.Accent, again)
	}
	if g := ground(home, socket); g.Accent != "#123456" {
		t.Errorf("the panel does not read the theme the server wears: %s", g.Accent)
	}
	if _, again := choose(true, theme.Want{Theme: "paper"}); !again {
		t.Error("--theme on the theme worn does not read its sheet again")
	}
	if _, _, err := chooseGround(home, socket, true, theme.Want{Theme: "nope"}, detect); err == nil {
		t.Error("--theme nope was worn")
	}
	if m, _ := theme.ReadMode(socket); m.Theme != "paper" {
		t.Errorf("a theme that is not there was written down: %+v", m)
	}

	// config.json names the theme a server rises in; a flag names
	// another over it.
	theme.ForgetMode(socket)
	write(t, filepath.Join(home, ".config", "conn", "config.json"), `{"theme": "paper"}`)
	if g, _ := choose(false, theme.Want{Dark: &light}); g.Accent != "#123456" {
		t.Errorf("config.json's theme was not worn: %s", g.Accent)
	}
	theme.ForgetMode(socket)
	if g, _ := choose(false, theme.Want{Dark: &light, Theme: "conn"}); g != theme.Light {
		t.Errorf("--theme conn over config.json's: %s", g.Accent)
	}

	// config.json names the ground a server rises on, and the terminal
	// is not asked; a flag names another over it, and a ground that is
	// neither is said.
	theme.ForgetMode(socket)
	write(t, filepath.Join(home, ".config", "conn", "config.json"), `{"ground": "dark"}`)
	if g, _ := choose(false, theme.Want{}); !g.Dark() || asked != 1 {
		t.Errorf("config.json's dark: dark %v, asked %d", g.Dark(), asked)
	}
	theme.ForgetMode(socket)
	if g, _ := choose(false, theme.Want{Dark: &light}); g.Dark() {
		t.Error("--light over config.json's dark was dark")
	}
	theme.ForgetMode(socket)
	write(t, filepath.Join(home, ".config", "conn", "config.json"), `{"ground": "dusk"}`)
	if _, _, err := chooseGround(home, socket, false, theme.Want{}, detect); err == nil || !strings.Contains(err.Error(), `not "dusk"`) {
		t.Errorf("a ground of dusk: %v", err)
	}

	// A sheet gone while the server wears it is conn's own.
	_ = theme.WriteMode(socket, theme.Mode{Theme: "gone"})
	if g, _, err := chooseGround(home, socket, true, theme.Want{}, detect); err != nil || g != theme.Light {
		t.Errorf("a server on a sheet since gone: %v", err)
	}
}

// The server's configuration carries the ground it is written on: the
// panes on it in its ink, the sixteen by name, the cursor in the
// accent; and tmux takes it as written.
func TestConfWearsTheGround(t *testing.T) {
	for _, g := range []theme.Ground{theme.Dark, theme.Light} {
		c := conf(g)
		for _, want := range []string{
			"window-style \"bg=" + theme.Hex(g.Ground) + ",fg=" + theme.Hex(g.Ink) + "\"",
			"cursor-colour \"" + g.Cursor + "\"",
			"pane-colours[9] \"" + g.Scheme[9] + "\"",
			"status-style \"bg=" + g.Border + ",fg=" + g.Gray + "\"",
		} {
			if !strings.Contains(c, want) {
				t.Errorf("conf lacks %s", want)
			}
		}
	}
	bin, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("no tmux")
	}
	dir := t.TempDir()
	path := write(t, filepath.Join(dir, "tmux.conf"), conf(theme.Light))
	srv := &tmux.Server{Tmux: bin, Socket: filepath.Join(dir, "s")}
	t.Cleanup(func() { _ = srv.Down() })
	if _, err := srv.Run("-f", path, "new-session", "-d", "-s", sessionName, "sleep 1000"); err != nil {
		t.Fatal(err)
	}
	for opt, want := range map[string]string{"pane-colours[1]": theme.Light.Scheme[1], "cursor-colour": theme.Light.Cursor, "mode-style": "bg=" + theme.Light.Chosen + ",fg=" + theme.Light.ChosenInk} {
		out, err := srv.Run("show-options", "-gv", opt)
		if err != nil || !strings.EqualFold(strings.TrimSpace(out), want) {
			t.Errorf("%s = %q, %v; want %q", opt, strings.TrimSpace(out), err, want)
		}
	}
}

// The key bar is written inside a conditional of tmux's own, where a
// comma is the conditional's: no key or word has one. And the two rows
// of the status line are in the configuration, with the bar's mark.
func TestKeyBar(t *testing.T) {
	for _, hints := range [][]keyHint{projectHints, projectsHints, searchHints, backHints, cancelHints, copyHints, awayHints} {
		for _, h := range hints {
			if strings.ContainsAny(h.key+h.does, ",#") {
				t.Errorf("hint %q %q holds a comma or a hash", h.key, h.does)
			}
		}
	}
	// The bar fits a window of ordinary width: tmux cuts what runs off
	// the right, and the keys in full are behind ? instead.
	styled := regexp.MustCompile(`#\[[^\]]*\]`)
	for _, hints := range [][]keyHint{projectHints, projectsHints} {
		if w := ansi.StringWidth(styled.ReplaceAllString(keyBar(hints, theme.Dark), "")); w > 120 {
			t.Errorf("the bar is %d columns wide: %v", w, hints)
		}
	}
	// Every key the bar leaves out is said in full behind ?.
	for _, c := range []struct{ bar, full []keyHint }{{projectHints, projectKeys}, {projectsHints, projectsKeys}} {
		for _, h := range c.bar {
			found := h.key == "?"
			for _, k := range c.full {
				found = found || k.key == h.key || strings.HasPrefix(k.key, h.key)
			}
			if !found {
				t.Errorf("%s is on the bar and not behind ?", h.key)
			}
		}
	}
	bar := keyBar([]keyHint{{"x", "stop"}}, theme.Dark)
	if !strings.Contains(bar, "x ") || !strings.Contains(bar, "stop") || !strings.Contains(bar, theme.Dark.Surface) {
		t.Errorf("keyBar = %q", bar)
	}
	// On the panel the bar is lit in the accent's block, keys and all.
	if lit := connBar([]keyHint{{"x", "stop"}}, theme.Dark); !strings.Contains(lit, "bg="+theme.Dark.Block) || strings.Contains(lit, theme.Dark.Surface) {
		t.Errorf("connBar = %q", lit)
	}
	c := conf(theme.Dark)
	for _, want := range []string{"set -g status 2\n", "status-format[1]", "#[fill=" + theme.Dark.Block + " bg=" + theme.Dark.Block + "]#{" + barMark + "}", "#{" + panelIDMark + "}"} {
		if !strings.Contains(c, want) {
			t.Errorf("conf lacks %s", want)
		}
	}
}

// ? shows the screen's keys in full over it, on either screen, and
// any key puts them away.
func TestKeys(t *testing.T) {
	press := func(m model, k string) model {
		key := tea.Key{Text: k, Code: []rune(k)[0]}
		if k == "esc" {
			key = tea.Key{Code: tea.KeyEscape}
		}
		next, _ := m.Update(tea.KeyPressMsg(key))
		return next.(model)
	}
	m := model{screen: onProject, cur: project{Name: "d", Root: "/p"}, rows: []row{}, width: 80, height: 30}
	if m = press(m, "?"); !m.helping {
		t.Fatal("? did not show the keys")
	}
	lines, f := m.draw()
	if s := strings.Join(lines, "\n"); !strings.Contains(s, "run the checks") || !strings.Contains(s, "conversations at rest") || !strings.Contains(s, "d  keys") {
		t.Errorf("the keys are not drawn in full: %q", s)
	}
	if !reflect.DeepEqual(f.hints, backHints) {
		t.Errorf("the bar under the keys = %v", f.hints)
	}
	if m = press(m, "esc"); m.helping {
		t.Error("esc did not put the keys away")
	}
	if m = press(m, "?"); !m.helping {
		t.Fatal("? did not show the keys again")
	}
	if m = press(m, "x"); m.helping || len(m.rows) != 0 {
		t.Error("a key did not put the keys away, or did more")
	}
	m.screen = onProjects
	if m = press(m, "?"); !m.helping {
		t.Fatal("? did not show the keys on the projects screen")
	}
	if lines, _ := m.draw(); !strings.Contains(strings.Join(lines, "\n"), "stops everything in it") {
		t.Errorf("the projects screen's keys are not drawn: %q", lines)
	}
}

// A project with nothing running is read all the same: the read says
// so, and what to do, in place of reading.
func TestReadEmpty(t *testing.T) {
	m := model{screen: onProject, cur: project{Name: "d", Root: "/p"}, width: 80, height: 30}
	if lines, _ := m.draw(); !strings.Contains(strings.Join(lines, "\n"), "reading…") {
		t.Errorf("before the read, the screen does not say reading: %q", lines)
	}
	next, _ := m.Update(read{root: "/p"})
	m = next.(model)
	lines, _ := m.draw()
	if s := strings.Join(lines, "\n"); !strings.Contains(s, "nothing running") || !strings.Contains(s, "starts a conversation") || strings.Contains(s, "at rest") {
		t.Errorf("after an empty read, the screen does not say so: %q", lines)
	}
	next, _ = m.Update(read{root: "/p", rest: []row{{conv: &conversation{ID: "a"}}}})
	m = next.(model)
	if lines, _ := m.draw(); !strings.Contains(strings.Join(lines, "\n"), "picks up a conversation, one at rest") {
		t.Errorf("after a read with a conversation at rest, the screen does not say so: %q", lines)
	}
}

// A verb waits for its object. r again restarts the row, c lists the
// conversations at rest with a find line, and any other key puts the
// verb away and says what it waited for.
func TestVerbWaits(t *testing.T) {
	press := func(m model, k string) model {
		key := tea.Key{Text: k, Code: []rune(k)[0]}
		if k == "esc" {
			key = tea.Key{Code: tea.KeyEscape}
		}
		next, _ := m.Update(tea.KeyPressMsg(key))
		return next.(model)
	}
	m := model{screen: onProject, cur: project{Name: "d", Root: "/p"}, rows: []row{{pane: &pane{ID: "%1", Name: "web"}}}}
	m.rest = []row{{conv: &conversation{ID: "a", Title: "Grammar work"}}, {conv: &conversation{ID: "b", Title: "Look rebuild"}}}
	if m = press(m, "r"); m.verb != "r" {
		t.Fatalf("r did not wait: verb %q", m.verb)
	}
	if m = press(m, "x"); m.verb != "" || m.note == "" || m.listing {
		t.Errorf("x after r: verb %q, note %q, listing %v", m.verb, m.note, m.listing)
	}
	if m = press(m, "x"); m.verb != "x" {
		t.Fatalf("x did not wait: verb %q", m.verb)
	}
	if m = press(m, "q"); m.verb != "" || !strings.Contains(m.note, "s, the stack") {
		t.Errorf("q after x: verb %q, note %q", m.verb, m.note)
	}
	if m = press(press(m, "x"), "t"); m.verb != "" || !strings.Contains(m.note, "the checks") {
		t.Errorf("xt with no checks: verb %q, note %q", m.verb, m.note)
	}
	if m = press(press(m, "a"), "f"); m.verb != "" || m.note != "Nothing failed here." {
		t.Errorf("af with nothing failed: verb %q, note %q", m.verb, m.note)
	}
	if m = press(press(m, "r"), "f"); m.verb != "" || !strings.Contains(m.note, "what failed") {
		t.Errorf("rf with nothing failed: verb %q, note %q", m.verb, m.note)
	}
	m = press(press(m, "r"), "c")
	if !m.listing || m.verb != "" {
		t.Fatalf("rc did not list: listing %v, verb %q", m.listing, m.verb)
	}
	if m = press(m, "l"); len(m.shownRest()) != 1 || m.shownRest()[0].conv.ID != "b" {
		t.Errorf("typing l finds %d", len(m.shownRest()))
	}
	if m = press(m, "esc"); m.listing || m.screen != onProject {
		t.Error("esc did not put the list away")
	}
}

// The panel is the open project's rows, then a line for each other
// project with something running, waiting or at work; the cursor on a
// row or a line says which. A folder's counts take in its
// repositories', which have lines of their own, so a folder has a line
// only for work in the folder itself, and the line counts that alone.
func TestItems(t *testing.T) {
	m := model{screen: onProject, projects: []project{{Root: "/a", Running: 1}, {Root: "/b"}, {Root: "/c", Waiting: 1}, {Root: "/d"}, {Root: "/f", Group: true, Running: 1}, {Root: "/f/x", Depth: 1, Running: 1}, {Root: "/g", Turning: 1}, {Root: "/h", Group: true, Running: 2, Turning: 1}, {Root: "/h/y", Depth: 1, Running: 1}}}
	m.cur = m.projects[3]
	m.rows = []row{{pane: &pane{ID: "%1"}}}
	var got []string
	for _, it := range m.items() {
		got = append(got, m.itemKey(it))
	}
	if want := []string{"%1", "project /a", "project /c", "project /f/x", "project /g", "project /h", "project /h/y"}; !reflect.DeepEqual(got, want) {
		t.Errorf("items = %v", got)
	}
	if h := m.items()[5].project; h.Running != 1 || h.Turning != 1 {
		t.Errorf("the folder's line counts %d running, %d at work, want 1 and 1: the work in itself", h.Running, h.Turning)
	}
	m.cursor = 0
	if r, ok := m.under(); !ok || r.pane.ID != "%1" {
		t.Error("the cursor is not on the row")
	}
	m.cursor = 2
	if p, ok := m.lineUnder(); !ok || p.Root != "/c" {
		t.Error("the cursor is not on /c's line")
	}
}

// Opening a project, from the projects or from its line under another,
// puts the cursor on its first row once the rows are read, not on the
// line it was on before they came, and not on a pane gone into before.
func TestOpenLandsOnFirstRow(t *testing.T) {
	ps := []project{{Name: "a", Root: "/a", Running: 1}, {Name: "b", Root: "/b", Running: 1}}
	rows := []row{{pane: &pane{ID: "%1", Name: "web"}}, {pane: &pane{ID: "%2"}}}
	m := model{screen: onProjects, projects: ps, cursor: 1}
	next, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if m = next.(model); m.screen != onProject || m.cur.Root != "/b" || m.cursor != 0 || m.follow != "" {
		t.Fatalf("enter opened screen %v, project %q, cursor %d, following %q", m.screen, m.cur.Root, m.cursor, m.follow)
	}
	next, _ = m.Update(read{root: "/b", projects: ps, rows: rows})
	if m = next.(model); m.cursor != 0 || m.itemKey(m.items()[0]) != rows[0].key() {
		t.Errorf("after the read, the cursor is at %d on %s, want the first row", m.cursor, m.itemKey(m.items()[m.cursor]))
	}
	// From b's rows, enter on a's line opens a on its first row too.
	m.cursor = len(m.items()) - 1
	if key := m.itemKey(m.items()[m.cursor]); key != "project /a" {
		t.Fatalf("the cursor is on %s, want a's line", key)
	}
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if m = next.(model); m.cur.Root != "/a" || m.cursor != 0 || m.follow != "" {
		t.Errorf("a opened with cursor %d, following %q", m.cursor, m.follow)
	}
	next, _ = m.Update(read{root: "/a", projects: ps, rows: rows})
	if m = next.(model); m.cursor != 0 || m.itemKey(m.items()[0]) != rows[0].key() {
		t.Errorf("after a's read, the cursor is at %d, want the first row", m.cursor)
	}
	// A read while the rows stand keeps the cursor on its row.
	m.cursor = 1
	next, _ = m.Update(read{root: "/a", projects: ps, rows: []row{rows[1], rows[0]}})
	if m = next.(model); m.itemKey(m.items()[m.cursor]) != "%2" {
		t.Errorf("the cursor left its row for %s", m.itemKey(m.items()[m.cursor]))
	}
}

// ctrl-space from a pane writes the pane down and brings the keys to
// the panel, which notes it; ctrl-space on the panel goes to the other
// process, the one worked in before the last, and swaps the two, so
// twice over from inside a process is the other process, and esc goes
// back into the one the keys were last in. Either puts the process's
// project on the panel. With no other process yet, it says so.
func TestCtrlSpaceToggles(t *testing.T) {
	srv, win, panel := scratchServer(t)
	ws := newWorkspace(srv, win, panel, "/x")
	ws.hold = "sleep 1000"
	a, err := openWindow(srv, "a", t.TempDir(), "sleep 1000", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := openWindow(srv, "b", t.TempDir(), "sleep 1000", "")
	if err != nil {
		t.Fatal(err)
	}
	bind := bindBack(win, panel)
	for _, want := range []string{"if-shell", fromMark, "send-keys -t " + panel + " " + backKey} {
		if !strings.Contains(strings.Join(bind, " "), want) {
			t.Errorf("the binding lacks %s: %v", want, bind)
		}
	}
	key := tea.KeyPressMsg(tea.Key{Code: '@', Mod: tea.ModCtrl})
	m := model{srv: srv, own: panel, ws: ws, known: newKnown(), screen: onProject, cur: project{Name: "p", Root: "/p"}, rows: []row{{pane: &pane{ID: a}}, {pane: &pane{ID: b}}}}
	next, _ := m.Update(key)
	if m = next.(model); m.note == "" {
		t.Error("with no other process, ctrl-space said nothing")
	}
	next, _ = m.goTo(m.rows[0])
	m = next.(model)
	next, _ = m.goTo(m.rows[1])
	if m = next.(model); m.in != b || m.other != a {
		t.Fatalf("after going into a then b, in %q other %q", m.in, m.other)
	}
	// ctrl-space from b: the key wrote b down; the panel hears it.
	if _, err := srv.Do(tmux.SetGlobal(fromMark, b)); err != nil {
		t.Fatal(err)
	}
	next, _ = m.Update(key)
	if m = next.(model); m.in != b || m.other != a || m.follow != b {
		t.Errorf("arriving from b: in %q other %q, following %q", m.in, m.other, m.follow)
	}
	if from, _ := srv.Global(fromMark); from != "" {
		t.Errorf("the pane written down was not cleared: %q", from)
	}
	// ctrl-space on the panel: to a, and b is now the other.
	next, cmd := m.Update(key)
	if m = next.(model); m.in != a || m.other != b || m.follow != a || cmd == nil {
		t.Fatalf("to the other: in %q other %q following %q", m.in, m.other, m.follow)
	}
	if batch, ok := cmd().(tea.BatchMsg); ok && len(batch) > 0 {
		batch[0]()
	}
	if p := inWorkspace(t, ws); p.ID != a {
		t.Errorf("the workspace shows %s, want %s", p.ID, a)
	}
	// Again: back to b.
	next, _ = m.Update(key)
	if m = next.(model); m.in != b || m.other != a {
		t.Errorf("twice over: in %q other %q", m.in, m.other)
	}
	// Across projects: b's directory is in another project, and going
	// to b put that project on the panel, as tab does; the other, a,
	// is in the first, and going back puts the first on the panel again.
	ps, _ := panes(srv)
	for _, p := range ps {
		switch p.ID {
		case a:
			m.known.root[p.Dir] = "/p"
		case b:
			m.known.root[p.Dir] = "/q"
			next, _ = m.openOn(p)
			m = next.(model)
		}
	}
	if m.cur.Root != "/q" || m.in != b || m.other != a {
		t.Fatalf("going to b: on %q, in %q other %q", m.cur.Root, m.in, m.other)
	}
	if _, err := srv.Do(tmux.SetGlobal(fromMark, b)); err != nil {
		t.Fatal(err)
	}
	next, _ = m.Update(key)
	m = next.(model)
	next, _ = m.Update(key)
	if m = next.(model); m.cur.Root != "/p" || m.in != a || m.other != b || m.follow != a {
		t.Errorf("across projects: on %q, in %q other %q following %q", m.cur.Root, m.in, m.other, m.follow)
	}
	// esc on the panel with another project on it is back into a, the
	// process the keys were last in, its project on the panel again.
	m.cur, m.rows, m.follow = project{Name: "q", Root: "/q"}, nil, ""
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if m = next.(model); m.cur.Root != "/p" || m.in != a || m.follow != a {
		t.Errorf("esc: on %q, in %q following %q", m.cur.Root, m.in, m.follow)
	}
	// The other gone, there is nowhere to go.
	if _, err := srv.Do(tmux.Cmd{"kill-pane", "-t", b}); err != nil {
		t.Fatal(err)
	}
	next, _ = m.Update(key)
	if m = next.(model); m.in != a || m.note == "" {
		t.Errorf("with the other gone: in %q, note %q", m.in, m.note)
	}
}

// A run the keys started that fails at once is asked about again, with
// the question standing over the project; one that fails later, or
// ends well, or is still going past the watch, is let go of.
func TestFailedAtOnceIsAskedAgain(t *testing.T) {
	start := time.Now()
	m := model{screen: onProject, cur: project{Name: "d", Root: "/p"}, rows: []row{}}
	m.watching = []watched{
		{project: "/p", stacked: stacked{name: "test", command: "make test"}, as: roleCheck, at: start},
		{project: "/p", stacked: stacked{name: "lint", command: "make lint"}, as: roleCheck, at: start},
		{project: "/p", stacked: stacked{name: "build", command: "make build"}, as: roleCheck, at: start},
	}
	soon, late := start.Add(time.Second), start.Add(10*time.Second)
	// The old test, ended before the start, is not the run.
	rows := []row{
		{pane: &pane{ID: "%0", Name: "test", Exit: "1", Ended: start.Add(-time.Minute)}},
		{pane: &pane{ID: "%1", Name: "test", Exit: "127", Ended: soon}},
		{pane: &pane{ID: "%2", Name: "lint"}},
		{pane: &pane{ID: "%3", Name: "build"}},
	}
	next, _ := m.Update(read{at: soon, root: "/p", rows: rows})
	m = next.(model)
	if m.question != "" || len(m.watching) != 2 {
		t.Fatalf("asked before the watch was over: question %q, watching %d", m.question, len(m.watching))
	}
	rows[2].pane = &pane{ID: "%2", Name: "lint", Exit: "1", Ended: late}
	next, cmd := m.Update(read{at: late, root: "/p", rows: rows})
	m = next.(model)
	if m.question != roleCheck || cmd == nil || len(m.again) != 1 || m.again[0].name != "test" || m.again[0].pane != "%1" || len(m.watching) != 0 {
		t.Fatalf("question %q, again %+v, watching %d", m.question, m.again, len(m.watching))
	}
	if lines, _ := m.draw(); !strings.Contains(strings.Join(lines, "\n"), "test failed at once; asking Claude again") {
		t.Errorf("the screen does not say why it asks: %q", lines)
	}
	// Claude says the command is right: the question goes, and says so.
	next, _ = m.Update(roleAnswer{root: "/p", as: roleCheck, kept: true})
	m = next.(model)
	if m.question != "" || m.again != nil || !strings.Contains(m.note, "the failure is the project's") {
		t.Errorf("kept: question %q, note %q", m.question, m.note)
	}
}

// The answer asked again gives a command for a name that failed, under
// that name; KEEP, NONE, or the same command again, keeps it.
func TestParseAgain(t *testing.T) {
	failed := []failure{{stacked: stacked{name: "test", command: "npm test"}}, {stacked: stacked{name: "lint", command: "npm run lint"}}}
	got, err := parseAgain("test: pnpm test\nweb: pnpm dev\nlint: npm run lint\n", failed)
	if err != nil || len(got) != 1 || got[0].name != "test" || got[0].command != "pnpm test" {
		t.Errorf("parseAgain = %+v, %v", got, err)
	}
	for _, out := range []string{"KEEP", "keep.", "NONE", "lint: npm run lint"} {
		if _, err := parseAgain(out, failed); !errors.Is(err, errKept) {
			t.Errorf("parseAgain(%q) = %v, want kept", out, err)
		}
	}
	p := againPrompt(t.TempDir(), roleCheck, failed[:1], map[string][]string{"": {"sh: pnpm: command not found", "[test exited 127]"}})
	if !strings.Contains(p, "test: npm test\nexited ") || !strings.Contains(p, "command not found") || strings.Contains(p, "[test exited") {
		t.Errorf("againPrompt = %q", p)
	}
}

// Every list is a ring: j past the last row is on the first, k
// before the first on the last, and the same on the projects, the
// conversations at rest, and the search's matches, by the keys that
// move there. g and G go to the top and the bottom.
func TestRing(t *testing.T) {
	for _, c := range []struct{ at, by, n, want int }{{0, -1, 3, 2}, {2, 1, 3, 0}, {1, 1, 3, 2}, {0, 1, 0, 0}, {0, -1, 1, 0}} {
		if got := ring(c.at, c.by, c.n); got != c.want {
			t.Errorf("ring(%d, %d, %d) = %d, want %d", c.at, c.by, c.n, got, c.want)
		}
	}
	press := func(m model, k tea.Key) model {
		next, _ := m.Update(tea.KeyPressMsg(k))
		return next.(model)
	}
	j, k, g, G := tea.Key{Text: "j", Code: 'j'}, tea.Key{Text: "k", Code: 'k'}, tea.Key{Text: "g", Code: 'g'}, tea.Key{Text: "G", Code: 'G', Mod: tea.ModShift}
	ctrlJ, ctrlK := tea.Key{Code: 'j', Mod: tea.ModCtrl}, tea.Key{Code: 'k', Mod: tea.ModCtrl}
	// The project: two rows and another project's line, three items.
	m := model{screen: onProject, cur: project{Name: "d", Root: "/p"}, projects: []project{{Root: "/p"}, {Root: "/q", Running: 1}}}
	m.rows = []row{{pane: &pane{ID: "%1"}}, {pane: &pane{ID: "%2"}}}
	if m = press(m, k); m.cursor != 2 {
		t.Errorf("k from the top is at %d, want the bottom", m.cursor)
	}
	if m = press(m, j); m.cursor != 0 {
		t.Errorf("j from the bottom is at %d, want the top", m.cursor)
	}
	if m = press(m, G); m.cursor != 2 {
		t.Errorf("G is at %d, want the bottom", m.cursor)
	}
	if m = press(m, g); m.cursor != 0 {
		t.Errorf("g is at %d, want the top", m.cursor)
	}
	// The projects.
	ps := model{screen: onProjects, projects: []project{{Root: "/a"}, {Root: "/b"}, {Root: "/c"}}}
	if ps = press(ps, ctrlK); ps.cursor != 2 {
		t.Errorf("ctrl-k from the top of the projects is at %d", ps.cursor)
	}
	if ps = press(ps, ctrlJ); ps.cursor != 0 {
		t.Errorf("ctrl-j from the bottom of the projects is at %d", ps.cursor)
	}
	// The conversations at rest.
	m.listing = true
	m.rest = []row{{conv: &conversation{ID: "a"}}, {conv: &conversation{ID: "b"}}}
	if m = press(m, ctrlK); m.pickAt != 1 {
		t.Errorf("ctrl-k from the top of the list is at %d", m.pickAt)
	}
	if m = press(m, ctrlJ); m.pickAt != 0 {
		t.Errorf("ctrl-j from the bottom of the list is at %d", m.pickAt)
	}
	m.listing = false
	// The settings.
	st := model{settings: true, srv: nil}
	n := len(st.settingRows())
	if n < 2 {
		t.Fatalf("the settings have %d rows", n)
	}
	if st = press(st, k); st.setAt != n-1 {
		t.Errorf("k from the top of the settings is at %d, want %d", st.setAt, n-1)
	}
	if st = press(st, j); st.setAt != 0 {
		t.Errorf("j from the bottom of the settings is at %d", st.setAt)
	}
	if st = press(st, G); st.setAt != n-1 {
		t.Errorf("G in the settings is at %d, want %d", st.setAt, n-1)
	}
	if st = press(st, g); st.setAt != 0 {
		t.Errorf("g in the settings is at %d", st.setAt)
	}
	// The search's matches.
	m.searching, m.found = true, []hit{{}, {}}
	if m = press(m, ctrlK); m.hitAt != 1 {
		t.Errorf("ctrl-k from the top of the matches is at %d", m.hitAt)
	}
	if m = press(m, ctrlJ); m.hitAt != 0 {
		t.Errorf("ctrl-j from the bottom of the matches is at %d", m.hitAt)
	}
}

// Moving through /'s lines shows each in its pane once the cursor has
// rested: the pane in the workspace, in copy mode on the line with the
// search in hand, the keys still on the panel. The pane shown before
// is let go of, and esc lets go of the last.
func TestSearchShowsTheLine(t *testing.T) {
	srv, win, panel := scratchServer(t)
	if _, err := srv.Run("set-option", "-g", "mode-keys", "vi"); err != nil {
		t.Fatal(err)
	}
	ws := newWorkspace(srv, win, panel, "/x")
	ws.hold = "sleep 1000"
	a, err := openWindow(srv, "a", t.TempDir(), "seq 1 300; sleep 1000", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := openWindow(srv, "b", t.TempDir(), "seq 1 300; sleep 1000", "")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	var said []written
	for _, id := range []string{a, b} {
		lines, _, err := srv.Scrollback(id)
		if err != nil {
			t.Fatal(err)
		}
		said = append(said, written{pane: pane{ID: id}, name: id, lines: lines})
	}
	m := model{srv: srv, own: panel, ws: ws, known: newKnown(), screen: onProject, cur: project{Name: "p", Root: "/p"}, searching: true, said: said}
	m.box.SetValue("150")
	m.found = hits(m.box.Value(), m.said)
	where := func(id string) string {
		out, _ := srv.Run("display-message", "-p", "-t", id, "#{pane_in_mode}|#{copy_cursor_line}|#{search_present}")
		return strings.TrimSpace(out)
	}
	move := func() {
		t.Helper()
		next, cmd := m.searchKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
		m = next.(model)
		if cmd == nil {
			t.Fatal("moving asked for nothing")
		}
		next, cmd = m.update(peek{m.peeks})
		m = next.(model)
		if msg := cmd(); msg != nil {
			t.Fatalf("showing the line: %v", msg)
		}
	}
	move()
	if got := inWorkspace(t, ws).ID; got != b || m.landed != b {
		t.Errorf("the second line is shown in %q, landed %q, want %q", got, m.landed, b)
	}
	if got := where(b); got != "1|150|1" {
		t.Errorf("b shows %q, want copy mode on 150 with the search in hand", got)
	}
	move()
	if got := inWorkspace(t, ws).ID; got != a {
		t.Errorf("around at the end, the workspace shows %q, want %q", got, a)
	}
	if got := where(a); got != "1|150|1" {
		t.Errorf("a shows %q", got)
	}
	if got := where(b); !strings.HasPrefix(got, "0|") {
		t.Errorf("b was not let go of: %q", got)
	}
	// A peek a later move has overtaken shows nothing.
	if _, cmd := m.update(peek{m.peeks - 1}); cmd != nil {
		t.Error("a stale peek asked for something")
	}
	next, cmd := m.searchKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if m = next.(model); m.searching || m.landed != "" || cmd == nil {
		t.Fatalf("esc: searching %v, landed %q", m.searching, m.landed)
	}
	cmd()
	if got := where(a); !strings.HasPrefix(got, "0|") {
		t.Errorf("esc did not let go of a: %q", got)
	}
}

// - goes up to the projects, as vim's - goes up to the directory; esc
// and backspace no longer do. On the projects, esc goes back down into
// the project - came up from, once what was typed to find is cleared;
// and nowhere when conn opened on the projects.
func TestDashGoesUpToTheProjects(t *testing.T) {
	m := model{screen: onProject, cur: project{Name: "d", Root: "/p"}, rows: []row{}}
	esc := tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape})
	for _, k := range []tea.Key{{Code: tea.KeyEscape}, {Code: tea.KeyBackspace}} {
		if next, _ := m.Update(tea.KeyPressMsg(k)); next.(model).screen != onProject {
			t.Errorf("%s left the project", k)
		}
	}
	next, _ := m.Update(tea.KeyPressMsg(tea.Key{Text: "-", Code: '-'}))
	if m = next.(model); m.screen != onProjects {
		t.Fatal("- did not go up to the projects")
	}
	m.box.SetValue("co")
	next, _ = m.Update(esc)
	if m = next.(model); m.screen != onProjects || m.box.Value() != "" {
		t.Errorf("esc with a find typed did not clear it and stay: screen %v, typed %q", m.screen, m.box.Value())
	}
	next, _ = m.Update(esc)
	if m = next.(model); m.screen != onProject || m.cur.Root != "/p" {
		t.Errorf("esc did not go back down into the project: screen %v, cur %q", m.screen, m.cur.Root)
	}
	fresh := model{screen: onProjects}
	if next, _ := fresh.Update(esc); next.(model).screen != onProjects {
		t.Error("esc left the projects with no project to go back to")
	}
}

// , shows the settings on either screen: a row a theme, a row a
// ground, on beside what the server is on. enter puts the server on
// the row's where it stands, the panel wears it, and config.json keeps
// it beside what else it says; esc puts the settings away.
// Every line typed into - the projects' find, !'s line, /'s search and
// rc's find - is edited as readline edits it: the cursor moves within
// it, a word is taken back, the rest cut; the caret is drawn where the
// cursor stands; and the line is cleared when it is left, however it is
// left.
func TestTheLinesEditAsReadlineAndClearWhenLeft(t *testing.T) {
	keys := func(m model, ks ...tea.Key) model {
		for _, k := range ks {
			next, _ := m.Update(tea.KeyPressMsg(k))
			m = next.(model)
		}
		return m
	}
	text := func(s string) []tea.Key {
		var ks []tea.Key
		for _, r := range s {
			ks = append(ks, tea.Key{Code: r, Text: string(r)})
		}
		return ks
	}
	ctrl := func(r rune) tea.Key { return tea.Key{Code: r, Mod: tea.ModCtrl} }
	alt := func(r rune) tea.Key { return tea.Key{Code: r, Mod: tea.ModAlt} }
	left, esc, enter := tea.Key{Code: tea.KeyLeft}, tea.Key{Code: tea.KeyEscape}, tea.Key{Code: tea.KeyEnter}
	// edits types into whichever line stands and checks each readline key.
	edits := func(m model, line string) model {
		t.Helper()
		m = keys(m, text("shp")...)
		m = keys(m, left)
		m = keys(m, text("o")...)
		if m.box.Value() != "shop" || m.box.Position() != 3 {
			t.Errorf("%s: left then a letter gave %q at %d", line, m.box.Value(), m.box.Position())
		}
		if m = keys(m, ctrl('a')); m.box.Position() != 0 {
			t.Errorf("%s: ctrl-a did not go to the start: at %d", line, m.box.Position())
		}
		if m = keys(m, ctrl('e')); m.box.Position() != 4 {
			t.Errorf("%s: ctrl-e did not go to the end: at %d", line, m.box.Position())
		}
		if m = keys(m, ctrl('w')); m.box.Value() != "" {
			t.Errorf("%s: ctrl-w left %q", line, m.box.Value())
		}
		m = keys(m, text("the shop")...)
		if m = keys(m, alt('b')); m.box.Position() != 4 {
			t.Errorf("%s: alt-b did not go back a word: at %d", line, m.box.Position())
		}
		m = keys(m, left, left, left)
		if before, after := m.boxText(); before != "t" || after != "he shop" {
			t.Errorf("%s: the caret is drawn between %q and %q", line, before, after)
		}
		return m
	}
	// The projects' find, cleared by opening a project.
	m := model{screen: onProjects, projects: []project{{Name: "shop", Root: "/p"}}, loaded: true}
	m = edits(m, "find")
	if m = keys(m, ctrl('e'), ctrl('w')); m.box.Value() != "the " {
		t.Errorf("find: ctrl-w took back %q", m.box.Value())
	}
	m = keys(m, ctrl('u'))
	m = keys(m, text("sho")...)
	if m = keys(m, enter); m.screen != onProject || m.box.Value() != "" {
		t.Errorf("opening a project left the find at %q on screen %v", m.box.Value(), m.screen)
	}
	// !'s line, cleared by esc and by enter.
	m = keys(m, text("!")...)
	m = edits(m, "!")
	if m = keys(m, ctrl('a'), alt('f'), ctrl('k')); m.box.Value() != "the" {
		t.Errorf("!: alt-f then ctrl-k left %q", m.box.Value())
	}
	if m = keys(m, esc); m.asking || m.box.Value() != "" {
		t.Errorf("esc left !'s line at %q", m.box.Value())
	}
	m = keys(m, text("!echo hi")...)
	if m = keys(m, enter); m.asking || m.box.Value() != "" {
		t.Errorf("enter left !'s line at %q", m.box.Value())
	}
	// /'s search, cleared by esc. The lines searched are read off the
	// loop, so the search is stood up by hand.
	m.searching = true
	m = edits(m, "/")
	if m = keys(m, esc); m.searching || m.box.Value() != "" {
		t.Errorf("esc left the search at %q", m.box.Value())
	}
	// rc's find, cleared by esc.
	m.listing = true
	m = edits(m, "rc")
	if m = keys(m, esc); m.listing || m.box.Value() != "" {
		t.Errorf("esc left rc's find at %q", m.box.Value())
	}
	// Back on the projects, the find is clear.
	m = keys(m, text("-")...)
	if m.screen != onProjects || m.box.Value() != "" {
		t.Errorf("- left the find at %q", m.box.Value())
	}
}

func TestSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	write(t, filepath.Join(home, ".config", "conn", "config.json"), `{"roots": ["~/projects"]}`)
	srv, _, panel := scratchServer(t)
	if err := theme.WriteMode(srv.Socket, theme.Mode{Dark: true, Theme: "conn"}); err != nil {
		t.Fatal(err)
	}
	press := func(m model, k string) (model, tea.Cmd) {
		key := tea.Key{Text: k, Code: []rune(k)[0]}
		switch k {
		case "esc":
			key = tea.Key{Code: tea.KeyEscape}
		case "enter":
			key = tea.Key{Code: tea.KeyEnter}
		}
		next, cmd := m.Update(tea.KeyPressMsg(key))
		return next.(model), cmd
	}
	m := model{srv: srv, home: home, own: panel, g: theme.Dark, screen: onProject, cur: project{Name: "d", Root: "/p"}, rows: []row{}, width: 80, height: 30}
	if m, _ = press(m, ","); !m.settings {
		t.Fatal(", did not show the settings")
	}
	lines, f := m.draw()
	s := strings.Join(lines, "\n")
	for _, want := range []string{"conn  settings", "theme", "datum", "skelly", "mode", "light"} {
		if !strings.Contains(s, want) {
			t.Errorf("the settings do not say %q:\n%s", want, s)
		}
	}
	if n := strings.Count(s, " on"); n != 2 {
		t.Errorf("%d rows are on, not the theme and the ground:\n%s", n, s)
	}
	if !reflect.DeepEqual(f.hints, settingsHints) {
		t.Errorf("the bar under the settings = %v", f.hints)
	}

	// skelly is the third theme.
	m, _ = press(m, "j")
	m, _ = press(m, "j")
	m, cmd := press(m, "enter")
	if cmd == nil {
		t.Fatal("enter on skelly did nothing")
	}
	next, _ := m.Update(cmd())
	m = next.(model)
	skelly, _ := theme.Load(themesDir(home), "skelly")
	if m.g != skelly.Dark || m.note != "" {
		t.Errorf("the panel does not wear skelly: %s, %q", m.g.Accent, m.note)
	}
	if md, _ := theme.ReadMode(srv.Socket); md != (theme.Mode{Dark: true, Theme: "skelly"}) {
		t.Errorf("the server's mode is %+v", md)
	}
	if c, _ := srv.Global("cursor-colour"); !strings.EqualFold(c, skelly.Dark.Cursor) {
		t.Errorf("the server's cursor is %s, not skelly's %s", c, skelly.Dark.Cursor)
	}
	b, _ := os.ReadFile(filepath.Join(home, ".config", "conn", "config.json"))
	var c map[string]any
	if err := json.Unmarshal(b, &c); err != nil || c["theme"] != "skelly" || c["ground"] != "dark" || c["roots"] == nil {
		t.Errorf("config.json after skelly: %s", b)
	}

	// light is the last row.
	m, _ = press(m, "G")
	m, cmd = press(m, "enter")
	next, _ = m.Update(cmd())
	if m = next.(model); m.g != skelly.Light {
		t.Errorf("light did not put the panel on skelly's light: %s", m.g.Accent)
	}
	if m, _ = press(m, "esc"); m.settings {
		t.Error("esc did not put the settings away")
	}

	// On the projects screen , opens them too, unless it is typed into
	// a find.
	m.screen = onProjects
	if m, _ = press(m, ","); !m.settings {
		t.Error(", did not show the settings on the projects screen")
	}
	m.settings = false
	m.box.SetValue("a")
	if m, _ = press(m, ","); m.settings || m.box.Value() != "a," {
		t.Errorf("a , typed into a find opened the settings: %q", m.box.Value())
	}
}

// Every screen's title is drawn in the accent's brighter cousin: a
// project's name, and conn on the projects screen and the settings.
func TestTheTitleIsTheShimmer(t *testing.T) {
	wear(theme.Dark)
	t.Cleanup(undress)
	c := theme.RGB(theme.Dark.Shimmer)
	shimmer := fmt.Sprintf("38;2;%d;%d;%dm", c.R, c.G, c.B)
	m := model{screen: onProject, cur: project{Name: "d", Root: "/p"}, rows: []row{}, width: 60, height: 20}
	for _, s := range []struct {
		name  string
		m     model
		title string
	}{
		{"a project", m, "d"},
		{"the projects", model{screen: onProjects, width: 60, height: 20}, "conn"},
		{"the settings", model{screen: onProject, settings: true, g: theme.Dark, width: 60, height: 20}, "conn"},
	} {
		if lines, _ := s.m.draw(); !strings.Contains(lines[1], shimmer+s.title) {
			t.Errorf("%s: the title is not in the shimmer: %q", s.name, lines[1])
		}
	}
}

// A click on the panel is the cursor to what it landed on, and in: a
// row's pane in the workspace, another project's line opened, and a
// project on the projects screen opened. A click on the head, a label
// or the air is nothing.
func TestClick(t *testing.T) {
	plain := regexp.MustCompile("\x1b\\[[0-9;]*m")
	lineOf := func(m model, word string) int {
		inner, _ := m.inner()
		lines, _ := inner.draw()
		for i, l := range lines {
			if strings.Contains(plain.ReplaceAllString(l, ""), word) {
				return i
			}
		}
		t.Fatalf("no line says %q", word)
		return -1
	}
	click := func(m model, y int) model {
		next, _ := m.Update(tea.MouseClickMsg{Y: y, Button: tea.MouseLeft})
		return next.(model)
	}
	m := model{screen: onProject, cur: project{Name: "d", Root: "/p"}, home: "/home", width: 60, height: 30,
		rows: []row{
			{pane: &pane{ID: "%1", Name: "web", Dir: "/p"}, tree: "/p"},
			{pane: &pane{ID: "%2", Name: "test", Dir: "/p"}, tree: "/p"},
		},
		projects: []project{{Name: "other", Root: "/q", Running: 1}},
	}
	if got := click(m, lineOf(m, "test")); got.cursor != 1 || got.in != "%2" {
		t.Errorf("a click on a row: cursor %d in %q, want 1 in %q", got.cursor, got.in, "%2")
	}
	if got := click(m, lineOf(m, "projects")); got.cursor != 0 || got.in != "" {
		t.Errorf("a click on the label: cursor %d in %q", got.cursor, got.in)
	}
	if got := click(m, 1); got.cursor != 0 || got.in != "" {
		t.Errorf("a click on the head: cursor %d in %q", got.cursor, got.in)
	}
	if got := click(m, lineOf(m, "other")); got.screen != onProject || got.cur.Root != "/q" {
		t.Errorf("a click on another project's line: on %q", got.cur.Root)
	}
	// A right click is not a click.
	if next, _ := m.Update(tea.MouseClickMsg{Y: lineOf(m, "test"), Button: tea.MouseRight}); next.(model).in != "" {
		t.Error("a right click went in")
	}
	// A verb left waiting is dropped by the click.
	m.verb = "x"
	if got := click(m, lineOf(m, "test")); got.verb != "" || got.in != "%2" {
		t.Errorf("a click with a verb waiting: verb %q in %q", got.verb, got.in)
	}
	ps := model{screen: onProjects, loaded: true, home: "/home", width: 60, height: 30,
		projects: []project{{Name: "a", Root: "/a"}, {Name: "b", Root: "/b"}}}
	if got := click(ps, lineOf(ps, "b")); got.screen != onProject || got.cur.Root != "/b" {
		t.Errorf("a click on the projects screen: screen %v on %q", got.screen, got.cur.Root)
	}
	if got := click(ps, 1); got.screen != onProjects {
		t.Error("a click on the projects screen's head opened something")
	}
}

// Typing :port on the projects screen asks conn what holds it: the
// project holding it, its holder said where the counts were, and enter
// opens the project on that process; a port held outside every project
// says so, and which process it is and where; a port nobody holds says
// that. What holds what comes with the read, as the machine's processes.
func TestFindAPort(t *testing.T) {
	shots := os.Getenv("SHOTS")
	if shots != "" {
		wear(theme.Dark)
		t.Cleanup(undress)
	}
	shoot := func(name string, m model) {
		if shots == "" {
			return
		}
		g := theme.Dark
		ground := fmt.Sprintf("\x1b]10;%s\x1b\\\x1b]11;%s\x1b\\\x1b[2J\x1b[H", theme.Hex(g.Ink), g.Surface)
		screen := strings.ReplaceAll(m.View().Content, "\n", "\x1b[0m\r\n")
		if err := os.WriteFile(filepath.Join(shots, name), []byte(ground+screen+"\x1b[0m"), 0o644); err != nil {
			t.Error(err)
		}
	}
	k := newKnown()
	k.root["/home/web"], k.root["/home/api"], k.root["/home/scratch"] = "/home/web", "/home/api", ""
	ps := []project{
		{Name: "api", Root: "/home/api", Running: 1},
		{Name: "web", Root: "/home/web", Running: 2, Waiting: 1},
		{Name: "notes", Root: "/home/notes"},
	}
	dev := &pane{ID: "%3", Name: "dev", Dir: "/home/web", PID: 4100, Command: "node"}
	db := &pane{ID: "%5", Name: "db", Dir: "/home/api", PID: 4200, Command: "postgres"}
	procs := []proc{
		{pane: dev, Name: "dev", PID: 4100, State: "running", Ports: []string{"3000"}, Command: "node", Dir: "/home/web"},
		{pane: db, Name: "db", PID: 4200, State: "running", Ports: []string{"5432"}, Command: "postgres", Dir: "/home/api"},
		{Name: "python3", PID: 5150, State: "outside conn", Ports: []string{"8000"}, Command: "python3 -m http.server 8000", Dir: "/home/scratch"},
	}
	fresh := func() model {
		m := model{screen: onProjects, home: "/home", width: 120, height: 36, known: k}
		next, _ := m.Update(read{projects: ps, procs: procs})
		return next.(model)
	}
	typing := func(m model, s string) model {
		for _, r := range s {
			next, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
			m = next.(model)
		}
		return m
	}
	screen := func(m model) []string {
		lines, _ := m.draw()
		for i := range lines {
			lines[i] = strings.TrimRight(plainText(lines[i]), " ")
		}
		return lines
	}
	saying := func(lines []string, s string) int {
		for i, l := range lines {
			if strings.Contains(l, s) {
				return i
			}
		}
		return -1
	}

	// :3000 narrows the list to web, its holder where its counts were.
	m := typing(fresh(), ":3000")
	shoot("port-held-120x36.ans", m)
	lines := screen(m)
	at := saying(lines, "dev :3000")
	if at < 0 || !strings.Contains(lines[at], "web") {
		t.Fatalf(":3000 does not show web held by dev :3000:\n%s", strings.Join(lines, "\n"))
	}
	if saying(lines, "api") >= 0 || saying(lines, "notes") >= 0 || strings.Contains(lines[at], "running") || strings.Contains(lines[at], "waiting") {
		t.Errorf(":3000 shows more than web holding it:\n%s", strings.Join(lines, "\n"))
	}

	// enter opens web with the cursor on dev, and dev in the workspace.
	next, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if m = next.(model); m.screen != onProject || m.cur.Root != "/home/web" {
		t.Fatalf("enter opened screen %v on %q, want web", m.screen, m.cur.Root)
	}
	rows := []row{
		{conv: &conversation{ID: "c1", Dir: "/home/web", Title: "fix the login form"}, pane: &pane{ID: "%2", Session: "c1", Dir: "/home/web"}, state: "waiting", stand: placeConversations},
		{pane: dev, state: "running", ports: []string{"3000"}, stand: placeStack},
		{pane: &pane{ID: "%4", Name: "worker", Dir: "/home/web"}, state: "running", stand: placeStack},
	}
	next, _ = m.Update(read{root: "/home/web", projects: ps, procs: procs, rows: rows})
	m = next.(model)
	shoot("port-opened-120x36.ans", m)
	if r, ok := m.under(); !ok || r.pane == nil || r.pane.ID != "%3" || m.wanted() != "%3" {
		t.Errorf("web opened with the cursor on %s and %q in the workspace, want dev's %%3", m.itemKey(m.items()[m.cursor]), m.wanted())
	}

	// :8000 is held outside every project: says so, and under it which
	// process and where it runs.
	m = typing(fresh(), ":8000")
	shoot("port-outside-120x36.ans", m)
	lines = screen(m)
	if at = saying(lines, "python3 holds :8000, outside any project"); at < 0 || at+1 >= len(lines) {
		t.Fatalf(":8000 does not say python3 holds it outside any project:\n%s", strings.Join(lines, "\n"))
	}
	under := lines[at+1]
	named := strings.Contains(under, "5150") || strings.Contains(under, "http.server")
	if !named || !strings.Contains(under, "~/scratch") {
		t.Errorf("under the outside holder, %q does not say which process it is and where it runs", under)
	}

	// :4000 is held by nothing.
	m = typing(fresh(), ":4000")
	shoot("port-free-120x36.ans", m)
	if lines = screen(m); saying(lines, "nothing holds :4000") < 0 {
		t.Errorf(":4000 does not say nothing holds it:\n%s", strings.Join(lines, "\n"))
	}
}

// Typing in rc's list finds a conversation at rest by what was said in
// it, not only by its title: each found that way shows, indented under
// its title, the line that said the words, the words in bold as in /'s
// search. The count says how many were found, the arrows move one
// conversation at a time, and enter picks the one under the cursor back
// up. The conversations are read off their transcripts, as conn reads
// them.
func TestPickUpByWhatWasSaid(t *testing.T) {
	bin, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("no tmux")
	}
	shots := os.Getenv("SHOTS")
	wear(theme.Dark)
	t.Cleanup(undress)
	shoot := func(name string, m model) {
		if shots == "" {
			return
		}
		g := theme.Dark
		ground := fmt.Sprintf("\x1b]10;%s\x1b\\\x1b]11;%s\x1b\\\x1b[2J\x1b[H", theme.Hex(g.Ink), g.Surface)
		screen := strings.ReplaceAll(m.View().Content, "\n", "\x1b[0m\r\n")
		if err := os.WriteFile(filepath.Join(shots, name), []byte(ground+screen+"\x1b[0m"), 0o644); err != nil {
			t.Error(err)
		}
	}

	scratch, err := os.MkdirTemp("/tmp", "cs-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(scratch) })
	// git and tmux say where things are with /tmp's symlink resolved.
	scratch, _ = filepath.EvalSymlinks(scratch)
	t.Setenv("HOME", scratch)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(scratch, ".claude"))
	// claude, as conn starts it to pick a conversation back up, waits.
	write(t, filepath.Join(scratch, "bin", "claude"), "#!/bin/sh\nexec sleep 1000\n")
	if err := os.Chmod(filepath.Join(scratch, "bin", "claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Join(scratch, "bin")+":"+os.Getenv("PATH"))
	web := filepath.Join(scratch, "web")
	if err := os.MkdirAll(web, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, web, "init", "-q")
	git(t, web, "commit", "-q", "--allow-empty", "-m", "first")

	// Four conversations at rest, newest first: one says the words, one
	// is titled with them, one says them in Claude's reply, and one has
	// nothing to do with them.
	encoded := regexp.MustCompile("[^A-Za-z0-9]").ReplaceAllString(web, "-")
	user := func(s string) string {
		b, _ := json.Marshal(s)
		return `{"type":"user","cwd":` + strconv.Quote(web) + `,"message":{"role":"user","content":` + string(b) + `}}`
	}
	reply := func(s string) string {
		b, _ := json.Marshal(s)
		return `{"type":"assistant","cwd":` + strconv.Quote(web) + `,"message":{"role":"assistant","content":[{"type":"text","text":` + string(b) + `}]}}`
	}
	titled := func(s string) string { return `{"type":"ai-title","aiTitle":` + strconv.Quote(s) + `}` }
	convs := []struct {
		id, title string
		lines     []string
	}{
		{"bbbbbbbb-0000-0000-0000-000000000002", "Tidy the session cookie", []string{
			user("after I sign in, the auth redirect lands on a blank page"),
			reply("The cookie is set on the wrong path."),
		}},
		{"aaaaaaaa-0000-0000-0000-000000000001", "Fix the auth redirect", []string{
			user("it loops"),
			reply("Looking."),
		}},
		{"dddddddd-0000-0000-0000-000000000004", "Grammar work", []string{
			user("make the manual read like the panel"),
			reply("Done."),
		}},
		{"cccccccc-0000-0000-0000-000000000003", "Move to the new router", []string{
			user("port the pages over"),
			reply("The old auth redirect in middleware.go has to go too."),
		}},
	}
	now := time.Now()
	for i, c := range convs {
		path := write(t, filepath.Join(scratch, ".claude", "projects", encoded, c.id+".jsonl"),
			strings.Join(append(c.lines, titled(c.title)), "\n")+"\n")
		when := now.Add(-time.Duration(i+1) * time.Hour)
		if err := os.Chtimes(path, when, when); err != nil {
			t.Fatal(err)
		}
	}

	srv := &tmux.Server{Tmux: bin, Socket: filepath.Join(scratch, "s")}
	t.Cleanup(func() { _ = srv.Down() })
	if _, err := srv.Run("-f", "/dev/null", "new-session", "-d", "-s", sessionName, "sleep 1000"); err != nil {
		t.Fatal(err)
	}

	// The scratch is temporary, which is no project; here it is one.
	k := newKnown()
	k.root[web] = web
	m := model{srv: srv, screen: onProject, cur: project{Name: "web", Root: web}, home: scratch, width: 120, height: 36, known: k}
	next, _ := m.Update(m.read()())
	m = next.(model)
	if len(m.rest) != 4 {
		t.Fatalf("the read found %d conversations at rest, want 4", len(m.rest))
	}

	// settle runs what a key asks done, as the program would, and hands
	// back what it answers; what takes longer than a moment is a tick.
	var settle func(m model, cmd tea.Cmd, depth int) model
	settle = func(m model, cmd tea.Cmd, depth int) model {
		if cmd == nil || depth > 4 {
			return m
		}
		got := make(chan tea.Msg, 1)
		go func() { got <- cmd() }()
		var msg tea.Msg
		select {
		case msg = <-got:
		case <-time.After(2 * time.Second):
			return m
		}
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				m = settle(m, c, depth+1)
			}
			return m
		}
		if msg == nil {
			return m
		}
		next, cmd := m.Update(msg)
		return settle(next.(model), cmd, depth+1)
	}
	press := func(m model, key tea.Key) model {
		next, cmd := m.Update(tea.KeyPressMsg(key))
		return settle(next.(model), cmd, 0)
	}
	typing := func(m model, s string) model {
		for _, r := range s {
			m = press(m, tea.Key{Code: r, Text: string(r)})
		}
		return m
	}
	m = typing(m, "rc")
	if !m.listing {
		t.Fatal("rc did not list the conversations at rest")
	}
	m = typing(m, "auth redirect")
	shoot("rc-found-120x36.ans", m)

	drawn, _ := m.draw()
	lines := make([]string, len(drawn))
	for i := range drawn {
		lines[i] = strings.TrimRight(plainText(drawn[i]), " ")
	}
	screen := strings.Join(lines, "\n")
	saying := func(s string) int {
		for i, l := range lines {
			if strings.Contains(l, s) {
				return i
			}
		}
		return -1
	}
	if !strings.Contains(screen, "3 conversations") {
		t.Errorf("the count does not say 3 conversations found:\n%s", screen)
	}
	if saying("Grammar work") >= 0 {
		t.Errorf("a conversation that never said the words is found:\n%s", screen)
	}
	for _, c := range []struct{ title, said string }{
		{"Tidy the session cookie", "the auth redirect lands on a blank page"},
		{"Move to the new router", "The old auth redirect in middleware.go"},
	} {
		at := saying(c.title)
		if at < 0 || at+1 >= len(lines) {
			t.Errorf("%q, which said the words, is not found:\n%s", c.title, screen)
			continue
		}
		under := lines[at+1]
		if !strings.Contains(under, c.said) {
			t.Errorf("under %q, %q is not the line that said the words:\n%s", c.title, under, screen)
			continue
		}
		indent := func(s string) int { return len(s) - len(strings.TrimLeft(s, " ")) }
		if indent(under) <= strings.Index(lines[at], c.title) {
			t.Errorf("the line said, %q, is not indented under its title %q", under, lines[at])
		}
		// Each word is in bold where the line says it.
		for _, w := range []string{"auth", "redirect"} {
			raw := drawn[at+1]
			i := strings.Index(raw, w)
			sgr := regexp.MustCompile("\x1b\\[([0-9;]*)m").FindAllStringSubmatch(raw[:max(i, 0)], -1)
			bold := len(sgr) > 0 && regexp.MustCompile(`(^|;)1(;|$)`).MatchString(sgr[len(sgr)-1][1])
			if i < 0 || !bold {
				t.Errorf("under %q, %q is not in bold: %q", c.title, w, raw)
			}
		}
	}
	if t.Failed() {
		t.FailNow()
	}

	// From the first found, two downs pass over the line said and the
	// conversation titled with the words, to the router conversation;
	// enter picks that one back up.
	m = press(m, tea.Key{Code: tea.KeyDown})
	m = press(m, tea.Key{Code: tea.KeyDown})
	shoot("rc-chosen-120x36.ans", m)
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = next.(model)
	if m.listing || m.in == "" {
		t.Fatalf("enter did not pick a conversation back up: listing %v, in %q, note %q", m.listing, m.in, m.note)
	}
	ps, _ := panes(srv)
	resumed := ""
	for _, p := range ps {
		if p.ID == m.in {
			resumed = p.Session
		}
	}
	if want := convs[3].id; resumed != want {
		t.Errorf("enter picked up %q, want the router conversation %q", resumed, want)
	}
}

// On the projects screen, a conversation waiting on you has a line of
// its own, indented under its project's: "❝" and its title, and at the
// right a faint tab for the one tab goes to next, so whether it can
// wait is told without opening the project. The cursor still moves
// between projects only, enter still opens one, and tab still goes
// straight into the conversation. Answered, the line goes. What is
// read is the machine, as the panel reads it: tmux's panes, Claude
// Code's instance files and transcripts.
func TestTheProjectsScreenNamesWhoWaits(t *testing.T) {
	bin, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("no tmux")
	}
	shots := os.Getenv("SHOTS")
	wear(theme.Dark)
	t.Cleanup(undress)
	g := theme.Dark

	scratch, err := os.MkdirTemp("/tmp", "cs-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(scratch) })
	// git and tmux say where things are with /tmp's symlink resolved.
	scratch, _ = filepath.EvalSymlinks(scratch)
	t.Setenv("HOME", scratch)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(scratch, ".claude"))
	repo := func(name string) string {
		dir := filepath.Join(scratch, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		git(t, dir, "init", "-q")
		git(t, dir, "commit", "-q", "--allow-empty", "-m", "first")
		return dir
	}
	web, api := repo("web"), repo("api")
	// A conversation in each, its title the last thing its transcript
	// says; web's is the one that will wait, api's has an hour gone.
	transcript := func(dir, id, title string, when time.Time) {
		encoded := regexp.MustCompile("[^A-Za-z0-9]").ReplaceAllString(dir, "-")
		path := write(t, filepath.Join(scratch, ".claude", "projects", encoded, id+".jsonl"),
			`{"type":"user","cwd":`+strconv.Quote(dir)+`,"message":{"role":"user","content":"hello"}}`+"\n"+
				`{"type":"ai-title","aiTitle":`+strconv.Quote(title)+`}`+"\n")
		if err := os.Chtimes(path, when, when); err != nil {
			t.Fatal(err)
		}
	}
	const waits = "Rate-limit the login endpoint"
	s1 := "aaaaaaaa-0000-0000-0000-000000000001"
	transcript(web, s1, waits, time.Now())
	transcript(api, "bbbbbbbb-0000-0000-0000-000000000002", "Index the orders table", time.Now().Add(-time.Hour))

	// The title says "·", which tmux writes only to a UTF-8 terminal.
	t.Setenv("LC_ALL", "C.UTF-8")
	path := write(t, filepath.Join(scratch, "tmux.conf"), conf(g))
	srv := &tmux.Server{Tmux: bin, Socket: filepath.Join(scratch, "s")}
	t.Cleanup(func() { _ = srv.Down() })
	if _, err := srv.Run("-f", path, "new-session", "-d", "-s", sessionName, "-x", "120", "-y", "36", "-c", scratch, "sleep 1000"); err != nil {
		t.Fatal(err)
	}
	// The conversation runs in a pane conn holds in web: a sleep, its
	// instance file saying what Claude Code would.
	id, err := openWindow(srv, "web", web, "sleep 1000", s1)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := srv.Run("display", "-p", "-t", id, "#{pane_pid}")
	if err != nil {
		t.Fatal(err)
	}
	instance := filepath.Join(scratch, ".claude", "sessions", strings.TrimSpace(pid)+".json")
	status := func(s string) {
		write(t, instance, `{"sessionId":"`+s1+`","status":"`+s+`","cwd":"`+web+`"}`)
	}

	// The scratch is temporary, which is no project; here both are.
	k := newKnown()
	k.root[web], k.root[api] = web, api
	fresh := func() model {
		m := model{srv: srv, screen: onProjects, home: scratch, conn: "conn", known: k, g: g, width: 120, height: 36}
		next, _ := m.Update(m.read()())
		return next.(model)
	}
	screen := func(m model) (raw, lines []string) {
		raw, _ = m.draw()
		lines = make([]string, len(raw))
		for i := range raw {
			lines[i] = strings.TrimRight(plainText(raw[i]), " ")
		}
		return raw, lines
	}
	saying := func(lines []string, s string) int {
		for i, l := range lines {
			if strings.Contains(l, s) {
				return i
			}
		}
		return -1
	}
	var said strings.Builder
	frame := func(name string, m model) {
		if shots == "" {
			return
		}
		title, _ := srv.Run("display", "-p", "-t", sessionName, "#{T:set-titles-string} #{window_bell_flag}")
		fmt.Fprintf(&said, "%s: %s\n", name, strings.TrimSpace(title))
		if err := os.WriteFile(filepath.Join(shots, "tab-title-and-bell.txt"), []byte(said.String()), 0o644); err != nil {
			t.Error(err)
		}
		ground := fmt.Sprintf("\x1b]10;%s\x1b\\\x1b]11;%s\x1b\\\x1b[2J\x1b[H", theme.Hex(g.Ink), g.Surface)
		content := strings.ReplaceAll(m.View().Content, "\n", "\x1b[0m\r\n")
		if err := os.WriteFile(filepath.Join(shots, name), []byte(ground+content+"\x1b[0m"), 0o644); err != nil {
			t.Error(err)
		}
	}
	key := func(m model, k tea.Key) model {
		next, _ := m.Update(tea.KeyPressMsg(k))
		return next.(model)
	}

	// At rest, no line names it.
	status("idle")
	m := fresh()
	frame("idle-120x36.ans", m)
	if _, lines := screen(m); saying(lines, waits) >= 0 {
		t.Errorf("with nothing waiting, the projects screen names %q:\n%s", waits, strings.Join(lines, "\n"))
	}

	// Waiting, web says so, and under it is the conversation's line.
	status("waiting")
	m = fresh()
	frame("waiting-120x36.ans", m)
	raw, lines := screen(m)
	all := strings.Join(lines, "\n")
	at := saying(lines, "1 waiting")
	if at < 0 || !strings.Contains(lines[at], "web") {
		t.Fatalf("web's line does not say 1 waiting:\n%s", all)
	}
	if at+1 >= len(lines) || !strings.Contains(lines[at+1], "❝ "+waits) {
		t.Fatalf("under web's line, no line names the waiting conversation, ❝ %s:\n%s", waits, all)
	}
	under := lines[at+1]
	indent := func(s string) int { return len(s) - len(strings.TrimLeft(s, " ")) }
	if indent(under) <= indent(lines[at]) {
		t.Errorf("the waiting conversation's line %q is not indented under web's %q", under, lines[at])
	}
	if !strings.HasSuffix(under, " tab") {
		t.Errorf("the waiting conversation's line %q has no tab at its right for the one tab goes to", under)
	}
	if mark := strings.SplitN(faint.Render("tab"), "tab", 2)[0]; mark == "" || !strings.Contains(raw[at+1], mark+"tab") {
		t.Errorf("the tab at the right of %q is not faint: %q", under, raw[at+1])
	}
	if t.Failed() {
		t.FailNow()
	}

	// The cursor moves between projects only: down from web is api.
	shown := m.shownProjects()
	if len(shown) < 2 || shown[m.cursor].Root != web {
		t.Fatalf("the projects screen opens with the cursor on %v of %d, want web", m.cursor, len(shown))
	}
	if down := key(m, tea.Key{Code: tea.KeyDown}); shown[down.cursor].Root != api {
		t.Errorf("down from web lands on %q, want api", shown[down.cursor].Root)
	}
	// enter opens web.
	if opened := key(m, tea.Key{Code: tea.KeyEnter}); opened.screen != onProject || opened.cur.Root != web {
		t.Errorf("enter on web opened screen %v on %q, want web", opened.screen, opened.cur.Root)
	}
	// tab goes straight into the conversation.
	if in := key(m, tea.Key{Code: tea.KeyTab}); in.screen != onProject || in.cur.Root != web || in.in != id {
		t.Errorf("tab went to screen %v on %q, into %q, want web's conversation %s", in.screen, in.cur.Root, in.in, id)
	}

	// Answered, the line goes, and web no longer says it waits.
	status("idle")
	m = fresh()
	frame("answered-120x36.ans", m)
	if _, lines := screen(m); saying(lines, waits) >= 0 || saying(lines, "waiting") >= 0 {
		t.Errorf("answered, the projects screen still says it waits:\n%s", strings.Join(lines, "\n"))
	}
}

// Under a conversation waiting on you, a gray line starting under its
// name says what it asks: "wants to run: " and the command, "wants to
// edit: " and the file from the project's root, or the first line of
// its question, cut with "…" two columns before the panel's edge. So
// on the project's panel, and under its project on the projects
// screen. Under the cursor the line is on the cursor's ground. As the
// ask changes, the line does; answered and back at work, the line goes
// and the rows below move up. What it asks is the tool its transcript
// last called with no result yet; the rest is the machine, as the
// panel reads it: tmux's panes and Claude Code's instance files.
func TestAWaitingConversationSaysWhatItAsks(t *testing.T) {
	bin, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("no tmux")
	}
	shots := os.Getenv("SHOTS")
	wear(theme.Dark)
	t.Cleanup(undress)
	g := theme.Dark

	scratch, err := os.MkdirTemp("/tmp", "cs-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(scratch) })
	scratch, _ = filepath.EvalSymlinks(scratch)
	t.Setenv("HOME", scratch)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(scratch, ".claude"))
	web := filepath.Join(scratch, "web")
	if err := os.MkdirAll(web, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, web, "init", "-q")
	git(t, web, "commit", "-q", "--allow-empty", "-m", "first")

	// The conversation's transcript: its title, and a turn had.
	const title = "Tidy the build"
	s1 := "aaaaaaaa-0000-0000-0000-000000000001"
	encoded := regexp.MustCompile("[^A-Za-z0-9]").ReplaceAllString(web, "-")
	transcript := write(t, filepath.Join(scratch, ".claude", "projects", encoded, s1+".jsonl"),
		`{"type":"user","cwd":`+strconv.Quote(web)+`,"message":{"role":"user","content":"clean up the build"}}`+"\n"+
			`{"type":"ai-title","aiTitle":`+strconv.Quote(title)+`}`+"\n"+
			`{"type":"assistant","cwd":`+strconv.Quote(web)+`,"message":{"role":"assistant","content":[{"type":"text","text":"Looking at the build."}]}}`+"\n")
	say := func(lines ...string) {
		t.Helper()
		f, err := os.OpenFile(transcript, os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()
		for _, l := range lines {
			if _, err := f.WriteString(l + "\n"); err != nil {
				t.Fatal(err)
			}
		}
	}
	tool := func(id, name, input string) string {
		return `{"type":"assistant","cwd":` + strconv.Quote(web) + `,"message":{"role":"assistant","content":[{"type":"tool_use","id":"` + id + `","name":"` + name + `","input":` + input + `}]}}`
	}
	result := func(id string) string {
		return `{"type":"user","cwd":` + strconv.Quote(web) + `,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"` + id + `","content":"ok"}]}}`
	}

	t.Setenv("LC_ALL", "C.UTF-8")
	path := write(t, filepath.Join(scratch, "tmux.conf"), conf(g))
	srv := &tmux.Server{Tmux: bin, Socket: filepath.Join(scratch, "s")}
	t.Cleanup(func() { _ = srv.Down() })
	if _, err := srv.Run("-f", path, "new-session", "-d", "-s", sessionName, "-x", "120", "-y", "36", "-c", scratch, "sleep 1000"); err != nil {
		t.Fatal(err)
	}
	// The conversation runs in a pane conn holds in web, and below it
	// in the panel a dev server runs, a row for the line to push down.
	id, err := openWindow(srv, "web", web, "sleep 1000", s1)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := srv.Run("display", "-p", "-t", id, "#{pane_pid}")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := startNamed(srv, "dev", "exec sleep 1000", web, web, roleStack); err != nil {
		t.Fatal(err)
	}
	instance := filepath.Join(scratch, ".claude", "sessions", strings.TrimSpace(pid)+".json")
	status := func(s string) {
		write(t, instance, `{"sessionId":"`+s1+`","status":"`+s+`","cwd":"`+web+`"}`)
	}

	k := newKnown()
	k.root[web] = web
	// The panel open on web, the cursor on the conversation; and the
	// projects screen, as each reads the machine now.
	panel := func() model {
		m := model{srv: srv, home: scratch, conn: "conn", known: k, g: g, loaded: true,
			screen: onProject, cur: project{Name: "web", Root: web}, width: panelWidth, height: 34}
		next, _ := m.Update(m.read()())
		m = next.(model)
		for i, it := range m.items() {
			if it.row >= 0 && m.rows[it.row].conv != nil && m.rows[it.row].conv.ID == s1 {
				m.cursor = i
			}
		}
		return m
	}
	projects := func() model {
		m := model{srv: srv, screen: onProjects, home: scratch, conn: "conn", known: k, g: g, width: 120, height: 36}
		next, _ := m.Update(m.read()())
		return next.(model)
	}
	screen := func(m model) (raw, lines []string) {
		raw, _ = m.draw()
		lines = make([]string, len(raw))
		for i := range raw {
			lines[i] = strings.TrimRight(plainText(raw[i]), " ")
		}
		return raw, lines
	}
	saying := func(lines []string, s string) int {
		for i, l := range lines {
			if strings.Contains(l, s) {
				return i
			}
		}
		return -1
	}
	prefix := func(st lipgloss.Style) string { return strings.SplitN(st.Render("x"), "x", 2)[0] }
	ground := prefix(lipgloss.NewStyle().Background(lipgloss.Color(g.Chosen)))
	ground = ground[strings.LastIndex(ground, "48;"):]

	var said strings.Builder
	shot := 0
	frame := func(name string, m model) {
		if shots == "" {
			return
		}
		got, _ := srv.Run("display", "-p", "-t", sessionName, "#{T:set-titles-string} #{window_bell_flag}")
		fmt.Fprintf(&said, "%s: %s\n", name, strings.TrimSpace(got))
		if err := os.WriteFile(filepath.Join(shots, "tab-title-and-bell.txt"), []byte(said.String()), 0o644); err != nil {
			t.Error(err)
		}
		if shot++; shot > 6 {
			return
		}
		var b strings.Builder
		fmt.Fprintf(&b, "\x1b]10;%s\x1b\\\x1b]11;%s\x1b\\\x1b[2J\x1b[H", theme.Hex(g.Ink), g.Surface)
		for _, l := range strings.Split(m.View().Content, "\n") {
			b.WriteString(l + "\x1b[0m" + strings.Repeat(" ", max(120-ansi.StringWidth(l), 0)) + "\r\n")
		}
		if m.screen == onProject {
			b.WriteString(drawnStatus(t, srv))
		}
		if err := os.WriteFile(filepath.Join(shots, name), []byte(strings.TrimSuffix(b.String(), "\r\n")+"\x1b[0m"), 0o644); err != nil {
			t.Error(err)
		}
	}
	indent := func(s string) int { return ansi.StringWidth(s) - ansi.StringWidth(strings.TrimLeft(s, " ")) }

	// asks checks the panel and the projects screen both say, under the
	// conversation, the line want, and returns the panel's lines.
	asks := func(what, want string) []string {
		t.Helper()
		m := panel()
		raw, lines := screen(m)
		all := strings.Join(lines, "\n")
		at := saying(lines, title)
		if at < 0 {
			t.Fatalf("%s, the panel has no row for the conversation:\n%s", what, all)
		}
		if at+1 >= len(lines) || !strings.HasPrefix(strings.TrimLeft(lines[at+1], " "), want) {
			t.Fatalf("%s, under the conversation's row the panel does not say %q:\n%s", what, want, all)
		}
		under := lines[at+1]
		if got, name := indent(under), strings.Index(lines[at], title); got != ansi.StringWidth(lines[at][:name]) {
			t.Errorf("%s, the line %q starts at column %d, not under the name in %q", what, under, got, lines[at])
		}
		if w := ansi.StringWidth(under); w > panelWidth-2 {
			t.Errorf("%s, the line %q runs %d columns, past two before the panel's edge at %d", what, under, w, panelWidth)
		}
		if !strings.Contains(raw[at+1], ground) {
			t.Errorf("%s, the line under the cursor's row is not on the cursor's ground %s: %q", what, g.Chosen, raw[at+1])
		}
		// Off the cursor, the line is gray.
		m.cursor = 0
		if m.items()[0].row >= 0 && m.rows[m.items()[0].row].conv != nil {
			m.cursor = len(m.items()) - 1
		}
		raw, lines = screen(m)
		if at := saying(lines, want); at < 0 || !strings.Contains(raw[at], prefix(gray)) {
			t.Errorf("%s, off the cursor the line saying %q is not gray: %q", what, want, raw[max(at, 0)])
		}

		// On the projects screen, under the conversation's line.
		_, plines := screen(projects())
		pall := strings.Join(plines, "\n")
		at = saying(plines, "❝ "+title)
		if at < 0 {
			t.Fatalf("%s, the projects screen names no conversation waiting in web:\n%s", what, pall)
		}
		if at+1 >= len(plines) || !strings.HasPrefix(strings.TrimLeft(plines[at+1], " "), want) {
			t.Errorf("%s, under the waiting conversation the projects screen does not say %q:\n%s", what, want, pall)
		} else if got, name := indent(plines[at+1]), strings.Index(plines[at], title); got != ansi.StringWidth(plines[at][:name]) {
			t.Errorf("%s, on the projects screen the line %q starts at column %d, not under the name in %q", what, plines[at+1], got, plines[at])
		}
		return lines
	}
	quiet := func(what string, m model, lines []string) {
		t.Helper()
		for _, w := range []string{"wants to", "Which", "rm -rf"} {
			if saying(lines, w) >= 0 {
				t.Errorf("%s, the %v screen still says %q:\n%s", what, m.screen, w, strings.Join(lines, "\n"))
			}
		}
	}

	// At rest, nothing is asked.
	status("idle")
	m := panel()
	frame("idle-120x36.ans", m)
	_, lines := screen(m)
	quiet("at rest", m, lines)
	restDev := saying(lines, "dev")

	// Stopped on a command, it wants to run it.
	say(tool("toolu_1", "Bash", `{"command":"rm -rf dist","description":"Remove the old build"}`))
	status("waiting")
	waiting := asks("stopped on a command", "wants to run: rm -rf dist")
	frame("waiting-120x36.ans", panel())
	frame("waiting-projects-120x36.ans", projects())
	if at := saying(waiting, title); saying(waiting, "dev") < at {
		t.Fatalf("the dev server's row is above the conversation's, nothing to push down:\n%s", strings.Join(waiting, "\n"))
	}
	waitDev := saying(waiting, "dev")

	// Run, and stopped on an edit: the line says the file, from web.
	say(result("toolu_1"), tool("toolu_2", "Edit", `{"file_path":`+strconv.Quote(filepath.Join(web, "app", "router.ts"))+`,"old_string":"a","new_string":"b"}`))
	lines = asks("stopped on an edit", "wants to edit: app/router.ts")
	if saying(lines, "rm -rf") >= 0 {
		t.Errorf("stopped on an edit, the panel still says the command:\n%s", strings.Join(lines, "\n"))
	}
	frame("edit-120x36.ans", panel())

	// Asked a question, the line is its first line, cut short on the
	// panel with "…" two columns before the edge.
	const question = "Which of the two login flows should the rate limit apply to first?"
	say(result("toolu_2"), tool("toolu_3", "AskUserQuestion", `{"questions":[{"header":"Rate limit","question":`+strconv.Quote(question+"\nThe old one is still used by the mobile app.")+`,"multiSelect":false,"options":[{"label":"Both","description":"Both flows"},{"label":"New","description":"The new flow only"}]}]}`))
	lines = asks("asked a question", "Which of the two")
	at := saying(lines, "Which of the two")
	if !strings.HasSuffix(lines[at], "…") || ansi.StringWidth(lines[at]) != panelWidth-2 {
		t.Errorf("the question's line on the panel %q (%d columns) is not cut with … two columns before the edge at %d", lines[at], ansi.StringWidth(lines[at]), panelWidth)
	}
	if !strings.HasPrefix(question, strings.TrimSuffix(strings.TrimLeft(lines[at], " "), "…")) {
		t.Errorf("the question's line on the panel %q is not the start of %q", lines[at], question)
	}
	if saying(lines, "mobile app") >= 0 {
		t.Errorf("the panel says more of the question than its first line:\n%s", strings.Join(lines, "\n"))
	}
	if _, plines := screen(projects()); saying(plines, question) < 0 {
		t.Errorf("the projects screen, with the room, does not say the question's first line whole:\n%s", strings.Join(plines, "\n"))
	}
	frame("question-120x36.ans", panel())

	// Answered, back at work: the line goes, and the rows below move up.
	say(result("toolu_3"))
	status("busy")
	m = panel()
	_, lines = screen(m)
	quiet("answered", m, lines)
	if got := saying(lines, "dev"); got != waitDev-1 || got != restDev {
		t.Errorf("answered, the dev server's row is at line %d, want %d, back up where it was at rest:\n%s", got, waitDev-1, strings.Join(lines, "\n"))
	}
	pm := projects()
	_, plines := screen(pm)
	quiet("answered", pm, plines)
	frame("answered-120x36.ans", m)
}

// With conn's tab behind another, a conversation at work that finishes
// its turn says so until it is looked at: the bell rings once, the
// tab's title reads "conn · 1 finished", its project's line on the
// projects screen says "1 finished" with a gray line under it naming
// the conversation, and its row says "finished" in plain ink with no
// spinner. tab reaches it after a conversation waiting on a question.
// The agent at work again, or the conversation gone into, quietly
// takes all of it away. What is read is the machine, as the panel
// reads it: tmux's panes, and Claude Code's instance files.
func TestAFinishedConversationSaysSo(t *testing.T) {
	bin, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("no tmux")
	}
	script, err := exec.LookPath("script")
	if err != nil {
		t.Skip("no script to give a client a terminal")
	}
	shots := os.Getenv("SHOTS")
	wear(theme.Dark)
	t.Cleanup(undress)
	g := theme.Dark

	scratch, err := os.MkdirTemp("/tmp", "cs-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(scratch) })
	scratch, _ = filepath.EvalSymlinks(scratch)
	t.Setenv("HOME", scratch)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(scratch, ".claude"))
	repo := func(name string) string {
		dir := filepath.Join(scratch, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		git(t, dir, "init", "-q")
		git(t, dir, "commit", "-q", "--allow-empty", "-m", "first")
		return dir
	}
	web, api := repo("web"), repo("api")
	transcript := func(dir, id, title string) {
		encoded := regexp.MustCompile("[^A-Za-z0-9]").ReplaceAllString(dir, "-")
		write(t, filepath.Join(scratch, ".claude", "projects", encoded, id+".jsonl"),
			`{"type":"user","cwd":`+strconv.Quote(dir)+`,"message":{"role":"user","content":"hello"}}`+"\n"+
				`{"type":"ai-title","aiTitle":`+strconv.Quote(title)+`}`+"\n")
	}
	const finishes, asks = "Rate-limit the login endpoint", "Index the orders table"
	s1, s2 := "aaaaaaaa-0000-0000-0000-000000000001", "bbbbbbbb-0000-0000-0000-000000000002"
	transcript(web, s1, finishes)
	transcript(api, s2, asks)

	// The title says "·", which tmux writes only to a UTF-8 terminal.
	t.Setenv("LC_ALL", "C.UTF-8")
	path := write(t, filepath.Join(scratch, "tmux.conf"), conf(g))
	srv := &tmux.Server{Tmux: bin, Socket: filepath.Join(scratch, "s")}
	t.Cleanup(func() { _ = srv.Down() })
	out, err := srv.Run("-f", path, "new-session", "-d", "-s", sessionName, "-x", "120", "-y", "36", "-c", scratch, "-P", "-F", "#{window_id} #{pane_id}", "sleep 1000")
	if err != nil {
		t.Fatal(err)
	}
	// conn's window: the panel, and beside it the workspace tab shows
	// a conversation in.
	win, own, _ := strings.Cut(strings.TrimSpace(out), " ")
	if _, err := srv.Do(tmux.SetPane(own, panelMark, "1")); err != nil {
		t.Fatal(err)
	}
	ws := newWorkspace(srv, win, own, "/x")
	ws.hold = "sleep 1000"

	// A terminal's client, on a terminal that takes a title, its output
	// what the terminal would be sent.
	attach := bin + " -S " + srv.Socket + " attach -t " + sessionName
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command(script, "-q", "/dev/null", "/bin/sh", "-c", attach)
	} else {
		cmd = exec.Command(script, "-qfec", attach, "/dev/null")
	}
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "TMUX=")
	in, keepOpen, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = keepOpen.Close() })
	cmd.Stdin = in
	var tty safeBuffer
	cmd.Stdout, cmd.Stderr = &tty, &tty
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	for deadline := time.Now().Add(3 * time.Second); len(terminals(srv)) == 0; time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("no client attached: %q", tty.String())
		}
	}
	time.Sleep(300 * time.Millisecond)
	osc := regexp.MustCompile("\x1b\\]([0-9]*)(?:;([^\x07\x1b]*))?(?:\x07|\x1b\\\\)")
	title := func() string {
		got := ""
		for _, m := range osc.FindAllStringSubmatch(tty.String(), -1) {
			if m[1] == "0" || m[1] == "2" {
				got = m[2]
			}
		}
		return got
	}
	bells := func() int {
		return strings.Count(osc.ReplaceAllString(tty.String(), ""), "\a")
	}

	// A conversation in each, in a pane conn holds: a sleep, its
	// instance file saying what Claude Code would. In web a dev server
	// runs too, a row for the cursor to rest on.
	conversation := func(name, dir, session string) (string, func(string)) {
		id, err := openWindow(srv, name, dir, "sleep 1000", session)
		if err != nil {
			t.Fatal(err)
		}
		pid, err := srv.Run("display", "-p", "-t", id, "#{pane_pid}")
		if err != nil {
			t.Fatal(err)
		}
		instance := filepath.Join(scratch, ".claude", "sessions", strings.TrimSpace(pid)+".json")
		return id, func(s string) {
			write(t, instance, `{"sessionId":"`+session+`","status":"`+s+`","cwd":"`+dir+`"}`)
		}
	}
	webPane, webStatus := conversation("web", web, s1)
	apiPane, apiStatus := conversation("api", api, s2)
	if _, err := startNamed(srv, "dev", "exec sleep 1000", web, web, roleStack); err != nil {
		t.Fatal(err)
	}

	// The projects screen, as the panel reads the machine. The scratch
	// is temporary, which is no project; here both are.
	k := newKnown()
	k.root[web], k.root[api] = web, api
	m := model{srv: srv, ws: ws, own: own, screen: onProjects, home: scratch, conn: "conn", known: k, g: g, loaded: true, width: panelWidth, height: 34}
	var update func(m model, msg tea.Msg) model
	update = func(m model, msg tea.Msg) model {
		if msg == nil {
			return m
		}
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				if c != nil {
					m = update(m, c())
				}
			}
			return m
		}
		next, _ := m.Update(msg)
		return next.(model)
	}
	press := func(k tea.Key) {
		next, c := m.Update(tea.KeyPressMsg(k))
		m = next.(model)
		if c != nil {
			m = update(m, c())
		}
	}
	// What the projects screen and web's panel draw, read now beside
	// what m has seen.
	projectsNow := func() model {
		p := m
		p.screen, p.cursor, p.width, p.height = onProjects, 0, 120, 36
		return update(p, p.read()())
	}
	panelNow := func() model {
		p := m
		if p.screen != onProject || p.cur.Root != web {
			p.screen, p.cur, p.rows, p.cursor = onProject, project{Name: "web", Root: web}, nil, 0
		}
		p = update(p, p.read()())
		// The cursor off the conversation's row, on the dev server's.
		for i, it := range p.items() {
			if it.row >= 0 && p.rows[it.row].conv == nil {
				p.cursor = i
			}
		}
		return p
	}
	screen := func(m model) (raw, lines []string) {
		raw, _ = m.draw()
		lines = make([]string, len(raw))
		for i := range raw {
			lines[i] = strings.TrimRight(plainText(raw[i]), " ")
		}
		return raw, lines
	}
	saying := func(lines []string, s string) int {
		for i, l := range lines {
			if strings.Contains(l, s) {
				return i
			}
		}
		return -1
	}
	indent := func(s string) int { return ansi.StringWidth(s) - ansi.StringWidth(strings.TrimLeft(s, " ")) }
	sgr := regexp.MustCompile("\x1b\\[[0-9;]*m")
	lastSGR := func(s string) string {
		all := sgr.FindAllString(s, -1)
		if len(all) == 0 {
			return ""
		}
		return all[len(all)-1]
	}

	var said strings.Builder
	shot := 0
	frame := func(name string, v model) {
		if shots == "" {
			return
		}
		fmt.Fprintf(&said, "%s: title %q, bells %d\n", name, title(), bells())
		if err := os.WriteFile(filepath.Join(shots, "tab-title-and-bell.txt"), []byte(said.String()), 0o644); err != nil {
			t.Error(err)
		}
		if shot++; shot > 6 {
			return
		}
		var b strings.Builder
		fmt.Fprintf(&b, "\x1b]10;%s\x1b\\\x1b]11;%s\x1b\\\x1b[2J\x1b[H", theme.Hex(g.Ink), g.Surface)
		for _, l := range strings.Split(v.View().Content, "\n") {
			b.WriteString(l + "\x1b[0m" + strings.Repeat(" ", max(120-ansi.StringWidth(l), 0)) + "\r\n")
		}
		if v.screen == onProject {
			b.WriteString(drawnStatus(t, srv))
		}
		if err := os.WriteFile(filepath.Join(shots, name), []byte(strings.TrimSuffix(b.String(), "\r\n")+"\x1b[0m"), 0o644); err != nil {
			t.Error(err)
		}
	}

	// step reads the machine until the tab says what is wanted, then
	// once more, and checks the title and the bells rung so far.
	step := func(what string, wantTitle func(string) bool, wantSaid string, wantBells int) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for {
			m = update(m, m.read()())
			if wantTitle(title()) && bells() >= wantBells || time.Now().After(deadline) {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		// A bell too many would come at once.
		time.Sleep(300 * time.Millisecond)
		m = update(m, m.read()())
		time.Sleep(300 * time.Millisecond)
		if got := title(); !wantTitle(got) {
			t.Errorf("%s, the tab's title is %q, want %s", what, got, wantSaid)
		}
		if got := bells(); got != wantBells {
			t.Errorf("%s, the bell has rung %d times, want %d", what, got, wantBells)
		}
	}
	is := func(want string) func(string) bool { return func(s string) bool { return s == want } }

	// finishedSaid checks the projects screen and web's panel say web's
	// conversation finished, or, with no, that neither says it.
	finishedSaid := func(what string, yes bool) {
		t.Helper()
		praw, plines := screen(projectsNow())
		pall := strings.Join(plines, "\n")
		at := -1
		for i, l := range plines {
			if strings.Contains(l, "web") && strings.Contains(l, tilde(web, scratch)) {
				at = i
			}
		}
		if at < 0 {
			t.Fatalf("%s, the projects screen has no line for web:\n%s", what, pall)
		}
		raw, lines := screen(panelNow())
		all := strings.Join(lines, "\n")
		row := saying(lines, finishes)
		if row < 0 {
			t.Fatalf("%s, web's panel has no row for the conversation:\n%s", what, all)
		}
		if !yes {
			if strings.Contains(plines[at], "finished") {
				t.Errorf("%s, web's line on the projects screen still says it finished: %q", what, plines[at])
			}
			if at+1 < len(plines) && strings.Contains(plines[at+1], finishes) {
				t.Errorf("%s, under web's line the projects screen still names the conversation: %q", what, plines[at+1])
			}
			if strings.Contains(lines[row], "finished") {
				t.Errorf("%s, the conversation's row still says it finished: %q", what, lines[row])
			}
			return
		}
		if !strings.Contains(plines[at], "1 finished") {
			t.Errorf("%s, web's line on the projects screen does not say 1 finished: %q\n%s", what, plines[at], pall)
		}
		if strings.Contains(plines[at], "waiting") {
			t.Errorf("%s, web's line on the projects screen says it waits: %q", what, plines[at])
		}
		if at+1 >= len(plines) || !strings.Contains(plines[at+1], finishes) {
			t.Errorf("%s, under web's line no line names the conversation that finished, %q:\n%s", what, finishes, pall)
		} else {
			if indent(plines[at+1]) <= indent(plines[at]) {
				t.Errorf("%s, the line %q naming the conversation is not indented under web's %q", what, plines[at+1], plines[at])
			}
			name := strings.Index(praw[at+1], finishes)
			if got := lastSGR(praw[at+1][:max(name, 0)]); got == "" || got != lastSGR(strings.SplitN(gray.Render("x"), "x", 2)[0]) {
				t.Errorf("%s, the line naming the conversation is not in the gray: %q", what, praw[at+1])
			}
		}
		if !strings.HasSuffix(lines[row], "finished") {
			t.Errorf("%s, the conversation's row does not say finished at its right: %q\n%s", what, lines[row], all)
		}
		if strings.Contains(lines[row], "waiting") {
			t.Errorf("%s, the conversation's row says it waits: %q", what, lines[row])
		}
		for _, f := range append(append([]string{}, spinner...), steady) {
			if strings.Contains(lines[row], f) {
				t.Errorf("%s, the conversation's row turns, %q: %q", what, f, lines[row])
			}
		}
		if word := strings.LastIndex(raw[row], "finished"); word >= 0 {
			if got, want := lastSGR(raw[row][:word]), lastSGR(strings.SplitN(ink.Render("x"), "x", 2)[0]); got != want {
				t.Errorf("%s, finished on the conversation's row is drawn in %q, want the plain ink %q: %q", what, got, want, raw[row])
			}
		}
	}

	// At work, the tab says nothing, and nothing finished.
	webStatus("busy")
	step("with the conversation at work", is("conn"), `"conn"`, 0)
	finishedSaid("with the conversation at work", false)
	frame("working-120x36.ans", projectsNow())

	// Its turn over, it finished: one bell, and said everywhere.
	webStatus("idle")
	step("with the conversation finished", is("conn · 1 finished"), `"conn · 1 finished"`, 1)
	finishedSaid("with the conversation finished", true)
	frame("finished-120x36.ans", projectsNow())
	frame("finished-panel-120x36.ans", panelNow())
	// Not looked at, it stays said, and rings no more.
	step("with the conversation still finished", is("conn · 1 finished"), `"conn · 1 finished"`, 1)
	finishedSaid("with the conversation still finished", true)
	if t.Failed() {
		t.FailNow()
	}

	// At work again, it is quietly taken away.
	webStatus("busy")
	step("with the conversation at work again", is("conn"), `"conn"`, 1)
	finishedSaid("with the conversation at work again", false)

	// api's conversation stops on a question; then web's finishes again.
	apiStatus("waiting")
	step("with api's conversation waiting", is("conn · 1 waiting"), `"conn · 1 waiting"`, 2)
	webStatus("idle")
	both := func(s string) bool {
		return strings.HasPrefix(s, "conn · ") && strings.Contains(s, "1 waiting") && strings.Contains(s, "1 finished")
	}
	step("with one waiting and one finished", both, `"conn · 1 waiting" and "1 finished"`, 3)
	finishedSaid("with one waiting and one finished", true)
	frame("waiting-and-finished-120x36.ans", projectsNow())

	// tab goes first to the question, then to the one that finished.
	press(tea.Key{Code: tea.KeyTab})
	if m.in != apiPane || inWorkspace(t, ws).ID != apiPane {
		t.Fatalf("the first tab went into %q, showing %q, want api's waiting conversation %s", m.in, inWorkspace(t, ws).ID, apiPane)
	}
	press(tea.Key{Code: tea.KeyTab})
	if m.in != webPane || inWorkspace(t, ws).ID != webPane {
		t.Fatalf("the second tab went into %q, showing %q, want web's finished conversation %s", m.in, inWorkspace(t, ws).ID, webPane)
	}

	// Gone into, it is quietly taken away; the question still waits.
	step("having gone into the finished conversation", is("conn · 1 waiting"), `"conn · 1 waiting"`, 3)
	finishedSaid("having gone into the finished conversation", false)
	frame("looked-at-120x36.ans", panelNow())
}
