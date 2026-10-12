package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	outside   *proc    // a server in the project that conn does not hold
	elsewhere bool     // running in a terminal conn does not hold
	state     string   // working, waiting or idle, where Claude Code says
	ports     []string // what it listens on
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
	p := tea.NewProgram(m)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go watch(ctx, p.Send, srv)
	_, err = p.Run()
	return err
}

// Messages.

// A read is the machine as read once: the projects, and the rows of the
// project on the screen, where one is.
type read struct {
	projects []project
	root     string
	rows     []row
}

// acted is what a key that runs off the loop did, or could not do.
type acted string

// sized is the terminal's size asked once, a moment after the start: a
// resize can land before the program listens for one, and is then never
// told.
type sized struct{}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.read(), tea.Tick(time.Second, func(time.Time) tea.Msg { return sized{} }))
}

// conversationsShown is how many conversations at rest a project lists,
// newest first.
const conversationsShown = 50

// read reads the machine once, off the loop: where work has happened,
// what runs in each project and how many of its conversations wait on
// you, wherever they run; and, in a project, its rows. The status line
// is told the number waiting in all.
func (m model) read() tea.Cmd {
	srv, home, own, k := m.srv, m.home, m.own, m.known
	var cur *project
	if m.screen == onProject {
		c := m.cur
		cur = &c
	}
	return func() tea.Msg {
		all := transcripts()
		ps, _ := panes(srv)
		var held []pane
		for _, p := range ps {
			if p.ID != own && !p.Dead {
				held = append(held, p)
			}
		}
		live := liveClaude()

		seen := claudeSightings(all, k.cwdOf)
		running, waiting := map[string]int{}, map[string]int{}
		for _, p := range held {
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
		r := read{projects: found}

		if cur != nil {
			// What listens, and where: a port under one of conn's panes is
			// that pane's, and a server outside conn whose directory is in
			// the project is a row of its own.
			ports := map[string][]string{}
			var outside []proc
			for _, p := range machine(ps, live, listening(), parents()) {
				switch {
				case p.pane != nil:
					ports[p.pane.ID] = p.Ports
				case p.Dir != "" && k.rootOf(p.Dir) == cur.Root:
					outside = append(outside, p)
				}
			}
			var convs []conversation
			for _, c := range all {
				dir := k.cwdOf(c)
				if dir == "" || k.rootOf(dir) != cur.Root {
					continue
				}
				c.Dir = dir
				convs = append(convs, c)
			}
			r.root, r.rows = cur.Root, arrange(convs, held, live, ports, outside, cur.Root, k.titleOf)
		}
		return r
	}
}

// arrange lays out a project's rows: what is running in conn first,
// waiting before the rest, then what is running elsewhere, then the
// conversations at rest, newest first, up to conversationsShown.
func arrange(convs []conversation, ps []pane, live map[string]claudeRun, ports map[string][]string, outside []proc, root string, title func(conversation) string) []row {
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
	for i := range running {
		if running[i].pane != nil {
			running[i].ports = ports[running[i].pane.ID]
		}
	}
	for i := range outside {
		running = append(running, row{outside: &outside[i], state: "outside conn", ports: outside[i].Ports})
	}
	waitingFirst(running)
	waitingFirst(elsewhere)
	return append(append(running, elsewhere...), rest...)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case read:
		m.takeProjects(msg.projects)
		if m.screen == onProject && msg.root == m.cur.Root {
			on := ""
			if m.cursor < len(m.rows) {
				on = m.rows[m.cursor].key()
			}
			m.rows = msg.rows
			m.cursor = keep(m.cursor, on, m.rows, row.key)
		}
	case changed:
		return m, m.read()
	case sized:
		if w, h, err := term.GetSize(os.Stdout.Fd()); err == nil && (w != m.width || h != m.height) {
			m.width, m.height = w, h
		}
	case acted:
		m.note = string(msg)
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

// takeProjects takes the projects as read. On the projects screen the
// cursor stays on its project as the list is sorted again under it, not
// on the place it was at; in a project, the cursor is the project's,
// and the list leaves it alone.
func (m *model) takeProjects(ps []project) {
	if m.screen != onProjects {
		m.projects, m.loaded = ps, true
		return
	}
	on := ""
	if shown := m.shownProjects(); m.cursor < len(shown) {
		on = shown[m.cursor].Root
	}
	m.projects, m.loaded = ps, true
	m.cursor = keep(m.cursor, on, m.shownProjects(), func(p project) string { return p.Root })
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
		return m, m.read()
	case "up", "k", "ctrl+p":
		m.cursor = max(m.cursor-1, 0)
	case "down", "j", "ctrl+n":
		m.cursor = min(m.cursor+1, max(len(m.rows)-1, 0))
	case "enter":
		if m.cursor < len(m.rows) {
			m.note = m.goTo(m.rows[m.cursor])
			return m, m.read()
		}
	case "n":
		id := newID()
		m.note = m.start(newCommand(m.conn, id), id)
		return m, m.read()
	case "s":
		m.note = m.start("", "")
		return m, m.read()
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
	case "o":
		if m.cursor < len(m.rows) {
			return m, openPort(m.rows[m.cursor])
		}
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

// openPort opens the port a row listens on in the browser, the lowest
// where it listens on several.
func openPort(r row) tea.Cmd {
	return func() tea.Msg {
		if len(r.ports) == 0 {
			return acted("It listens on no port.")
		}
		url := "http://localhost:" + r.ports[0]
		opener := "open"
		if runtime.GOOS != "darwin" {
			opener = "xdg-open"
		}
		if err := exec.Command(opener, url).Run(); err != nil {
			return acted(err.Error())
		}
		return acted("opened " + url)
	}
}

// proc is a row as the verbs take it: a pane, and what it is doing.
func (r row) proc() (proc, bool) {
	if r.outside != nil {
		return *r.outside, true
	}
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
	return m, m.read()
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
	if !m.asking && m.note == "" {
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
		case r.outside != nil:
			left = r.outside.Name
			if r.outside.Dir != m.cur.Root {
				left += "  " + tilde(r.outside.Dir, m.home)
			}
			right = r.state
		}
		if len(r.ports) > 0 {
			right = ":" + strings.Join(r.ports, " :") + "  " + right
		}
		line := spread(" "+left, right+" ", m.width)
		switch {
		case i == m.cursor:
			line = chosen.Render(line)
		case r.state == "waiting":
			line = waiting.Render(line)
		case r.pane != nil || r.outside != nil:
			line = running.Render(line)
		}
		lines = append(lines, line)
	}
	return lines, "enter goes to it · o opens its port · x stops · r restarts · u runs a command · n new conversation · s shell · esc projects · q leaves"
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
	cmds := []tmux.Cmd{set}
	for _, c := range terminals(srv) {
		cmds = append(cmds, tmux.Cmd{"refresh-client", "-S", "-t", c})
	}
	_, _ = srv.Do(cmds...)
}

// key is what a row is, through a sort: its conversation, or its pane.
func (r row) key() string {
	if r.conv != nil {
		return r.conv.ID
	}
	if r.pane != nil {
		return r.pane.ID
	}
	if r.outside != nil {
		return "pid " + strconv.Itoa(r.outside.PID)
	}
	return ""
}

// keep is where the cursor goes when a list is read again: onto the
// item it was on, by key, wherever that has moved to, and otherwise
// where it was, within the list.
func keep[T any](cursor int, on string, items []T, key func(T) string) int {
	if on != "" {
		for i, it := range items {
			if key(it) == on {
				return i
			}
		}
	}
	return min(cursor, max(len(items)-1, 0))
}
