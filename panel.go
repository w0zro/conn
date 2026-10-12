package main

import (
	"fmt"
	"os"
	"path/filepath"
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
	known *known

	width, height int
	screen        screen
	cursor        int
	note          string // what the last enter could not do

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
	elsewhere bool // running in a terminal conn does not hold
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
	m := model{srv: srv, home: home, own: own, known: newKnown()}
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

func (m model) Init() tea.Cmd { return tea.Batch(m.readProjects(), beat()) }

func beat() tea.Cmd { return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return tick{} }) }

// readProjects reads where work has happened: the directories Claude
// Code has worked in, and where the panes conn holds are, now.
func (m model) readProjects() tea.Cmd {
	srv, home, k := m.srv, m.home, m.known
	return func() tea.Msg {
		seen := claudeSightings(transcripts(), k.cwdOf)
		if ps, err := panes(srv); err == nil {
			for _, p := range ps {
				seen = append(seen, sighting{Dir: p.Dir, When: time.Now()})
			}
		}
		return projectsRead(findProjects(seen, home, k.rootOf))
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
// then what is running elsewhere, then the conversations at rest, newest
// first, up to conversationsShown.
func arrange(convs []conversation, ps []pane, live map[string]string, root string, title func(conversation) string) []row {
	var running, elsewhere, rest []row
	used := map[string]bool{}
	for i := range convs {
		c := convs[i]
		var at *pane
		tty, isLive := live[c.ID]
		for j := range ps {
			if ps[j].Session == c.ID || (isLive && tty != "" && ps[j].TTY == tty) {
				at = &ps[j]
				break
			}
		}
		switch {
		case at != nil:
			used[at.ID] = true
			c.Title = title(c)
			running = append(running, row{conv: &c, pane: at})
		case isLive:
			c.Title = title(c)
			elsewhere = append(elsewhere, row{conv: &c, elsewhere: true})
		case len(rest) < conversationsShown:
			c.Title = title(c)
			rest = append(rest, row{conv: &c})
		}
	}
	for j := range ps {
		if !used[ps[j].ID] && within(ps[j].Dir, root) {
			running = append(running, row{pane: &ps[j]})
		}
	}
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
			return m, tea.Batch(m.readProject(m.cur), beat())
		}
		return m, beat()
	case tea.KeyPressMsg:
		m.note = ""
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
		m.note = m.start(newCommand(id), id)
		return m, m.readProject(m.cur)
	case "s":
		m.note = m.start("", "")
		return m, m.readProject(m.cur)
	case "q", "ctrl+c":
		m.note = m.leave()
	}
	return m, nil
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
		if err := openWindow(m.srv, windowName(m.cur.Name), dir, resumeCommand(r.conv.ID), r.conv.ID); err != nil {
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
)

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
	h := max(m.height, len(lines)+2)
	for len(lines) < h-1 {
		lines = append(lines, "")
	}
	lines = append(lines[:h-1], faint.Render(fit(" "+foot, m.width)))
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
		age := ago(p.Last)
		left := " " + pad(p.Name, nameW) + "  " + tilde(p.Root, m.home)
		line := spread(left, age+" ", m.width)
		if i == m.cursor {
			line = chosen.Render(line)
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
				right = "running"
			case r.elsewhere:
				right = "in another terminal"
			default:
				right = ago(r.conv.When)
			}
		case r.pane != nil:
			left = r.pane.Command
			if r.pane.Session != "" {
				// Started by n, with nothing said yet: no transcript.
				left = "new conversation"
			}
			if r.pane.Dir != m.cur.Root {
				left += "  " + tilde(r.pane.Dir, m.home)
			}
			right = "running"
		}
		line := spread(" "+left, right+" ", m.width)
		switch {
		case i == m.cursor:
			line = chosen.Render(line)
		case r.pane != nil:
			line = running.Render(line)
		}
		lines = append(lines, line)
	}
	return lines, "enter goes to it · n new conversation · s shell · esc projects · q leaves conn running"
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
