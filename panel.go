package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	term "github.com/charmbracelet/x/term"
	"github.com/w0zro/conn/internal/tmux"
)

// The panel is conn's window: the projects work has happened in, and in
// one of them, what runs and the conversations that can be picked back
// up. Enter goes to a thing; ctrl-space comes back.

type screen int

const (
	onProjects screen = iota
	onProject
)

type model struct {
	srv   *tmux.Server
	home  string
	own   string // the panel's own pane
	conn  string // how an agent runs this build
	known *known

	width, height int
	screen        screen
	cursor        int
	note          string // what the last key could not do, or did
	// u's line: the command being typed, while it is.
	asking bool
	line   string

	// The projects screen.
	typed    string
	projects []project
	loaded   bool

	// A project.
	cur  project
	rows []row
}

// A row on a project's screen: a conversation, a pane, or both — a
// conversation running in a pane conn holds.
type row struct {
	conv      *conversation
	pane      *pane
	elsewhere bool   // running in a terminal conn does not hold
	state     string // working, waiting or idle, where Claude Code says
}

// known is what has been read off files that do not change under it:
// where each transcript was had, and the project each directory is in.
// A title is kept while its transcript has not moved.
type known struct {
	mu     sync.Mutex
	cwd    map[string]string
	root   map[string]string
	titles map[string]titled
}

type titled struct {
	when  time.Time
	title string
}

func newKnown() *known {
	return &known{cwd: map[string]string{}, root: map[string]string{}, titles: map[string]titled{}}
}

func (k *known) cwdOf(c conversation) string {
	k.mu.Lock()
	d, ok := k.cwd[c.path]
	k.mu.Unlock()
	if !ok {
		d = headCwd(c.path)
		k.mu.Lock()
		k.cwd[c.path] = d
		k.mu.Unlock()
	}
	return d
}

func (k *known) rootOf(dir string) string {
	k.mu.Lock()
	r, ok := k.root[dir]
	k.mu.Unlock()
	if !ok {
		r = rootOf(dir)
		k.mu.Lock()
		k.root[dir] = r
		k.mu.Unlock()
	}
	return r
}

func (k *known) titleOf(c conversation) string {
	k.mu.Lock()
	t, ok := k.titles[c.path]
	k.mu.Unlock()
	if !ok || !t.when.Equal(c.When) {
		t = titled{when: c.When, title: tailTitle(c.path)}
		k.mu.Lock()
		k.titles[c.path] = t
		k.mu.Unlock()
	}
	return t.title
}

func runPanel() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	srv, err := findServer(home)
	if err != nil {
		return err
	}
	own := os.Getenv("TMUX_PANE")
	// ctrl-space comes back here, wherever here is now.
	if own != "" {
		_, _ = srv.Do(tmux.Cmd{"bind", "-n", backKey, "select-window", "-t", own})
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	m := model{srv: srv, home: home, own: own, conn: connCommand(self), known: newKnown()}
	_, err = tea.NewProgram(m).Run()
	return err
}

// Messages.
type projectsRead []project
type projectRead struct {
	root string
	rows []row
}
type tick struct{}

// acted is what a key that runs off the loop did, or could not do.
type acted string

func (m model) Init() tea.Cmd { return tea.Batch(m.readProjects(), beat()) }

func beat() tea.Cmd { return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return tick{} }) }

// readProjects reads where work has happened: the directories Claude
// Code has worked in, and where the panes conn holds are, now. Each
// project is told what runs in it and how many conversations in it are
// waiting on you, wherever they run, and the status line is told the
// number waiting in all.
func (m model) readProjects() tea.Cmd {
	srv, home, own, k := m.srv, m.home, m.own, m.known
	return func() tea.Msg {
		seen := claudeSightings(transcripts(), k.cwdOf)
		ps, _ := panes(srv)
		live := liveClaude()
		running, waiting := map[string]int{}, map[string]int{}
		for _, p := range ps {
			if p.ID == own || p.Dead {
				continue
			}
			seen = append(seen, sighting{Dir: p.Dir, When: time.Now()})
			running[k.rootOf(p.Dir)]++
		}
		total := 0
		for _, r := range live {
			if r.state() == "waiting" {
				total++
				waiting[k.rootOf(r.Cwd)]++
			}
		}
		sayWaiting(srv, total)
		found := findProjects(seen, home, k.rootOf)
		for i := range found {
			found[i].Running, found[i].Waiting = running[found[i].Root], waiting[found[i].Root]
		}
		// What waits on you comes first; the rest stay newest first.
		sort.SliceStable(found, func(i, j int) bool { return found[i].Waiting > 0 && found[j].Waiting == 0 })
		return projectsRead(found)
	}
}

// conversationsShown is how many conversations at rest a project lists,
// newest first.
const conversationsShown = 50

// readProject reads a project's rows: its conversations, whether each is
// running and where, and the other panes conn holds in it.
func (m model) readProject(p project) tea.Cmd {
	srv, own, k := m.srv, m.own, m.known
	return func() tea.Msg {
		var convs []conversation
		for _, c := range transcripts() {
			dir := k.cwdOf(c)
			if dir == "" || k.rootOf(dir) != p.Root {
				continue
			}
			c.Dir = dir
			convs = append(convs, c)
		}
		ps, _ := panes(srv)
		var inProject []pane
		for _, q := range ps {
			if q.ID != own && !q.Dead {
				inProject = append(inProject, q)
			}
		}
		return projectRead{root: p.Root, rows: arrange(convs, inProject, liveClaude(), p.Root, k.titleOf)}
	}
}

// arrange lays out a project's rows: what is running in conn first,
// waiting before the rest, then what is running elsewhere, then the
// conversations at rest, newest first, up to conversationsShown.
func arrange(convs []conversation, ps []pane, live map[string]claudeRun, root string, title func(conversation) string) []row {
	var running, elsewhere, rest []row
	used := map[string]bool{}
	for i := range convs {
		c := convs[i]
		var at *pane
		run, isLive := live[c.ID]
		for j := range ps {
			if ps[j].Session == c.ID || (isLive && run.TTY != "" && ps[j].TTY == run.TTY) {
				at = &ps[j]
				break
			}
		}
		switch {
		case at != nil:
			used[at.ID] = true
			c.Title = title(c)
			running = append(running, row{conv: &c, pane: at, state: run.state()})
		case isLive:
			c.Title = title(c)
			elsewhere = append(elsewhere, row{conv: &c, elsewhere: true, state: run.state()})
		case len(rest) < conversationsShown:
			c.Title = title(c)
			rest = append(rest, row{conv: &c})
		}
	}
	for j := range ps {
		if !used[ps[j].ID] && within(ps[j].Dir, root) {
			state := live[ps[j].Session].state()
			if ps[j].Name != "" && ps[j].Exit != "" {
				state = "exited " + ps[j].Exit
			}
			running = append(running, row{pane: &ps[j], state: state})
		}
	}
	waitingFirst := func(rs []row) {
		sort.SliceStable(rs, func(i, j int) bool { return rs[i].state == "waiting" && rs[j].state != "waiting" })
	}
	waitingFirst(running)
	waitingFirst(elsewhere)
	return append(append(running, elsewhere...), rest...)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case projectsRead:
		m.projects, m.loaded = msg, true
		m.cursor = min(m.cursor, max(len(m.shownProjects())-1, 0))
	case projectRead:
		if m.screen == onProject && msg.root == m.cur.Root {
			m.rows = msg.rows
			m.cursor = min(m.cursor, max(len(m.rows)-1, 0))
		}
	case tick:
		// A resize can land before the program listens for one, and is
		// then never told; the terminal is asked on the beat as well.
		if w, h, err := term.GetSize(os.Stdout.Fd()); err == nil && (w != m.width || h != m.height) {
			m.width, m.height = w, h
		}
		if m.screen == onProject {
			return m, tea.Batch(m.readProject(m.cur), m.readProjects(), beat())
		}
		return m, tea.Batch(m.readProjects(), beat())
	case acted:
		m.note = string(msg)
		if m.screen == onProject {
			return m, m.readProject(m.cur)
		}
	case tea.KeyPressMsg:
		m.note = ""
		if m.asking {
			return m.lineKey(msg)
		}
		if m.screen == onProjects {
			return m.projectsKey(msg)
		}
		return m.projectKey(msg)
	}
	return m, nil
}

func (m model) projectsKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	shown := m.shownProjects()
	switch k.String() {
	case "enter":
		if m.cursor < len(shown) {
			return m.open(shown[m.cursor])
		}
		if p, ok := m.typedPath(); ok {
			return m.open(p)
		}
		return m, nil
	case "up", "ctrl+p", "ctrl+k":
		m.cursor = max(m.cursor-1, 0)
	case "down", "ctrl+n", "ctrl+j":
		m.cursor = min(m.cursor+1, max(len(shown)-1, 0))
	case "ctrl+c":
		m.note = m.leave()
	case "esc", "ctrl+u":
		m.typed, m.cursor = "", 0
	case "backspace":
		if r := []rune(m.typed); len(r) > 0 {
			m.typed, m.cursor = string(r[:len(r)-1]), 0
		}
	default:
		if t := k.Key().Text; t != "" {
			m.typed, m.cursor = m.typed+t, 0
		}
	}
	return m, nil
}

func (m model) projectKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc", "backspace":
		m.screen, m.rows, m.cursor = onProjects, nil, 0
		return m, m.readProjects()
	case "up", "k", "ctrl+p":
		m.cursor = max(m.cursor-1, 0)
	case "down", "j", "ctrl+n":
		m.cursor = min(m.cursor+1, max(len(m.rows)-1, 0))
	case "enter":
		if m.cursor < len(m.rows) {
			m.note = m.goTo(m.rows[m.cursor])
			return m, m.readProject(m.cur)
		}
	case "n":
		id := newID()
		m.note = m.start(newCommand(m.conn, id), id)
		return m, m.readProject(m.cur)
	case "s":
		m.note = m.start("", "")
		return m, m.readProject(m.cur)
	case "q", "ctrl+c":
		m.note = m.leave()
	case "x", "r":
		if m.cursor >= len(m.rows) {
			return m, nil
		}
		p, ok := m.rows[m.cursor].proc()
		if !ok {
			m.note = "That conversation is not running."
			return m, nil
		}
		if k.String() == "x" {
			return m, m.end(p)
		}
		return m, m.restart(p)
	case "u":
		m.asking, m.line = true, ""
	}
	return m, nil
}

// lineKey is a key while u's line is being typed.
func (m model) lineKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc", "ctrl+c":
		m.asking, m.line = false, ""
	case "enter":
		m.asking = false
		if strings.TrimSpace(m.line) == "" {
			return m, nil
		}
		return m, m.up(strings.TrimSpace(m.line))
	case "backspace":
		if r := []rune(m.line); len(r) > 0 {
			m.line = string(r[:len(r)-1])
		}
	case "ctrl+u":
		m.line = ""
	default:
		m.line += k.Key().Text
	}
	return m, nil
}

// proc is a row as the verbs take it: a pane, and what it is doing.
func (r row) proc() (proc, bool) {
	if r.pane == nil {
		return proc{}, false
	}
	session, name := r.pane.Session, paneName(*r.pane)
	if r.pane.Session != "" {
		name = "the new conversation"
	}
	if r.conv != nil {
		session, name = r.conv.ID, or(r.conv.Title, r.conv.ID)
	}
	return proc{pane: r.pane, Session: session, Name: name, PID: r.pane.PID, State: or(r.state, "running"), Dir: r.pane.Dir}, true
}

// end ends a row's process off the loop: ctrl-c can take seconds.
func (m model) end(p proc) tea.Cmd {
	srv := m.srv
	return func() tea.Msg {
		how, err := endProc(srv, p)
		if err != nil {
			return acted(err.Error())
		}
		return acted(p.Name + " ended: " + how)
	}
}

// restart starts a row's process again, off the loop.
func (m model) restart(p proc) tea.Cmd {
	srv, conn := m.srv, m.conn
	return func() tea.Msg {
		if _, err := restartProc(srv, conn, p); err != nil {
			return acted(err.Error())
		}
		return acted(p.Name + " restarted")
	}
}

// up starts a command at the project's root, out of sight, under the
// name of its program, as conn u does. One by that name that has
// ended is replaced; one still running is said.
func (m model) up(command string) tea.Cmd {
	srv, root, rows := m.srv, m.cur.Root, m.rows
	name := nameFor(command)
	return func() tea.Msg {
		replace := ""
		for _, r := range rows {
			if r.pane == nil || r.pane.Name != name {
				continue
			}
			if r.pane.Exit == "" {
				return acted(name + " is running; r restarts it")
			}
			replace = r.pane.ID
		}
		if _, err := startNamed(srv, name, command, root); err != nil {
			return acted(err.Error())
		}
		if replace != "" {
			_, _ = srv.Do(tmux.Kill(replace))
		}
		return acted(name + " is up; enter goes to it")
	}
}

// nameFor is the name a command typed at u runs under: its program's,
// past any VAR=value before it.
func nameFor(command string) string {
	for _, w := range strings.Fields(command) {
		if strings.Contains(w, "=") {
			continue
		}
		name := strings.Map(func(r rune) rune {
			if r == '.' || r == '_' || r == '-' || ('0' <= r && r <= '9') || ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z') {
				return r
			}
			return '-'
		}, filepath.Base(w))
		if name != "" && !isPID(name) {
			return name
		}
	}
	return "run"
}

// start runs a command in the project, or a shell where there is none,
// in a window of its own, marked with the conversation it is where it
// is one. It answers what it could not do.
func (m model) start(cmd, session string) string {
	if err := openWindow(m.srv, windowName(m.cur.Name), m.cur.Root, cmd, session); err != nil {
		return err.Error()
	}
	return ""
}

// leave lets the terminal go; what runs in conn keeps on.
func (m model) leave() string {
	if err := leave(m.srv); err != nil {
		return err.Error()
	}
	return ""
}

func (m model) open(p project) (tea.Model, tea.Cmd) {
	m.screen, m.cur, m.rows, m.cursor = onProject, p, nil, 0
	return m, m.readProject(p)
}

// goTo puts the client on a row's pane, picking its conversation back
// up in a window of its own where it is at rest. It answers what it
// could not do.
func (m model) goTo(r row) string {
	switch {
	case r.pane != nil:
		if err := goTo(m.srv, *r.pane); err != nil {
			return err.Error()
		}
	case r.elsewhere:
		return "That conversation is running in a terminal conn does not hold."
	case r.conv != nil:
		dir := r.conv.Dir
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			dir = m.cur.Root
		}
		if err := openWindow(m.srv, windowName(m.cur.Name), dir, resumeCommand(m.conn, r.conv.ID), r.conv.ID); err != nil {
			return err.Error()
		}
	}
	return ""
}

// shownProjects is the projects the typed words find: each word in the
// name or the path.
func (m model) shownProjects() []project {
	words := strings.Fields(strings.ToLower(m.typed))
	if len(words) == 0 {
		return m.projects
	}
	var out []project
	for _, p := range m.projects {
		hay := strings.ToLower(p.Name + " " + tilde(p.Root, m.home))
		all := true
		for _, w := range words {
			all = all && strings.Contains(hay, w)
		}
		if all {
			out = append(out, p)
		}
	}
	return out
}

// typedPath is the project at a typed path, where what is typed is a
// path to a directory.
func (m model) typedPath() (project, bool) {
	t := strings.TrimSpace(m.typed)
	if !strings.HasPrefix(t, "~") && !strings.Contains(t, "/") {
		return project{}, false
	}
	dir := expand(t, m.home)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return project{}, false
	}
	root := m.known.rootOf(dir)
	if root == "" {
		root = dir
	}
	return project{Name: filepath.Base(root), Root: root}, true
}

// Drawing.

var (
	bold    = lipgloss.NewStyle().Bold(true)
	faint   = lipgloss.NewStyle().Faint(true)
	chosen  = lipgloss.NewStyle().Reverse(true)
	running = lipgloss.NewStyle().Bold(true)
	// The one color conn uses: something is waiting on you.
	waiting = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(waitingColor))
)

// waitingColor is waiting's color, in the 256 palette, here and on the
// status line.
const waitingColor = "208"

func (m model) View() tea.View {
	var lines []string
	var foot string
	if m.screen == onProjects {
		lines, foot = m.drawProjects()
	} else {
		lines, foot = m.drawProject()
	}
	if m.note != "" {
		foot = m.note
	}
	if m.asking {
		foot = "run in " + m.cur.Name + ": " + m.line + "▏"
	}
	h := max(m.height, len(lines)+2)
	for len(lines) < h-1 {
		lines = append(lines, "")
	}
	foot = fit(" "+foot, m.width)
	if !m.asking {
		foot = faint.Render(foot)
	}
	lines = append(lines[:h-1], foot)
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	return v
}

func (m model) drawProjects() ([]string, string) {
	lines := []string{
		bold.Render(" conn") + faint.Render(" · projects"),
		" > " + m.typed + "▏",
		"",
	}
	shown := m.shownProjects()
	room := max(m.height-len(lines)-1, 1)
	first := max(m.cursor-room+1, 0)
	nameW := 0
	for _, p := range shown {
		nameW = max(nameW, ansi.StringWidth(p.Name))
	}
	nameW = min(nameW, 24)
	for i := first; i < len(shown) && i < first+room; i++ {
		p := shown[i]
		left := " " + pad(p.Name, nameW) + "  " + tilde(p.Root, m.home)
		line := spread(left, p.standing()+" ", m.width)
		switch {
		case i == m.cursor:
			line = chosen.Render(line)
		case p.Waiting > 0:
			line = waiting.Render(line)
		case p.Running > 0:
			line = running.Render(line)
		}
		lines = append(lines, line)
	}
	switch {
	case !m.loaded:
		lines = append(lines, faint.Render(" reading where work has happened…"))
	case len(shown) == 0 && m.typed != "":
		if p, ok := m.typedPath(); ok {
			lines = append(lines, " enter opens "+tilde(p.Root, m.home))
		} else {
			lines = append(lines, faint.Render(" nothing matches; a path to a directory opens it"))
		}
	case len(shown) == 0:
		lines = append(lines, faint.Render(" no projects yet; type a path to one"))
	}
	return lines, "type to find, or a path · enter opens · ctrl-c leaves conn running"
}

func (m model) drawProject() ([]string, string) {
	lines := []string{
		bold.Render(" "+m.cur.Name) + faint.Render("  "+tilde(m.cur.Root, m.home)),
		"",
	}
	if m.rows == nil {
		lines = append(lines, faint.Render(" reading…"))
	} else if len(m.rows) == 0 {
		lines = append(lines, faint.Render(" nothing running, and no conversations here yet; n starts one"))
	}
	room := max(m.height-len(lines)-1, 1)
	first := max(m.cursor-room+1, 0)
	for i := first; i < len(m.rows) && i < first+room; i++ {
		r := m.rows[i]
		var left, right string
		switch {
		case r.conv != nil:
			left = r.conv.Title
			if left == "" {
				left = r.conv.ID
			}
			switch {
			case r.pane != nil:
				right = or(r.state, "running")
			case r.elsewhere:
				right = or(r.state, "running") + " in another terminal"
			default:
				right = ago(r.conv.When)
			}
		case r.pane != nil:
			left = paneName(*r.pane)
			if r.pane.Session != "" {
				// Started by n, with nothing said yet: no transcript.
				left = "new conversation"
			}
			if r.pane.Dir != m.cur.Root {
				left += "  " + tilde(r.pane.Dir, m.home)
			}
			right = or(r.state, "running")
		}
		line := spread(" "+left, right+" ", m.width)
		switch {
		case i == m.cursor:
			line = chosen.Render(line)
		case r.state == "waiting":
			line = waiting.Render(line)
		case r.pane != nil:
			line = running.Render(line)
		}
		lines = append(lines, line)
	}
	return lines, "enter goes to it · x stops · r restarts · u runs a command · n new conversation · s shell · esc projects · q leaves"
}

// spread puts left and right on one line of width, left cut short where
// they do not both fit.
func spread(left, right string, width int) string {
	if width <= 0 {
		return left + "  " + right
	}
	rw := ansi.StringWidth(right)
	left = ansi.Truncate(left, max(width-rw-2, 0), "…")
	gap := width - ansi.StringWidth(left) - rw
	if gap < 1 {
		return fit(left, width)
	}
	return left + strings.Repeat(" ", gap) + right
}

func fit(s string, width int) string {
	if width <= 0 {
		return s
	}
	return ansi.Truncate(s, width, "…")
}

func pad(s string, w int) string {
	s = ansi.Truncate(s, w, "…")
	return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0))
}

// ago is how long ago a moment was, in the largest unit that says it.
func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case t.IsZero():
		return ""
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	case d < 60*24*time.Hour:
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	}
	return t.Format("Jan 2006")
}

// or is s, or else where s is blank.
func or(s, otherwise string) string {
	if s == "" {
		return otherwise
	}
	return s
}

// standing is what a project's row says at its right: what waits and
// what runs in it, or when work last happened in it.
func (p project) standing() string {
	var said []string
	if p.Waiting > 0 {
		said = append(said, fmt.Sprintf("%d waiting", p.Waiting))
	}
	if p.Running > 0 {
		said = append(said, fmt.Sprintf("%d running", p.Running))
	}
	if len(said) == 0 {
		return ago(p.Last)
	}
	return strings.Join(said, " · ")
}

// sayWaiting puts the number of conversations waiting on you on the
// status line, which shows it from every window, and takes it off at
// none.
func sayWaiting(srv *tmux.Server, n int) {
	want := ""
	if n > 0 {
		want = strconv.Itoa(n)
	}
	if was, _ := srv.Global(waitingMark); was == want {
		return
	}
	set := tmux.UnsetGlobal(waitingMark)
	if want != "" {
		set = tmux.SetGlobal(waitingMark, want)
	}
	_, _ = srv.Do(set, tmux.RefreshStatus())
}
