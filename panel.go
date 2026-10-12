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
	srv  *tmux.Server
	home string
	own  string // the panel's own pane
	ws   workspace
	// The pane the workspace is to show: what the workspace was last
	// asked for, a count of the asks so that only the last is carried
	// out, and a pane just opened, which the cursor goes to once its row
	// is read.
	asked  string
	asks   int
	follow string
	// When follow was set: a read that listed the panes after it and
	// has no row for the pane means the pane is gone.
	followAt time.Time
	conn     string // how an agent runs this build
	known    *known

	width, height int
	screen        screen
	cursor        int
	note          string // what the last key could not do, or did
	// !'s line: the command being typed, while it is.
	asking bool
	line   string
	// u's or t's question: the role asked about, blank for none, and
	// what Claude Code answered, or why it did not. While it stands it
	// is the project's screen.
	question role
	offer    []stacked
	offerErr string
	// The panes holding a conversation waiting on you, and the one tab
	// last went to, so the next tab goes on to another.
	waiting     []pane
	lastWaiting string
	// /'s search: whether it stands, what is typed, the panes' lines as
	// last read, and the lines that say what is typed.
	searching bool
	query     string
	said      []written
	found     []hit
	hitAt     int

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
	repo      string   // in a folder of repositories, the one it is in
}

// dir is where a row's work is.
func (r row) dir() string {
	switch {
	case r.conv != nil:
		return r.conv.Dir
	case r.pane != nil:
		return r.pane.Dir
	case r.outside != nil:
		return r.outside.Dir
	}
	return ""
}

// known is what has been read off files that do not change under it:
// where each transcript was had, and the project each directory is in.
// A title is kept while its transcript has not moved.
type known struct {
	mu     sync.Mutex
	cwd    map[string]string
	root   map[string]string
	titles map[string]titled
	folder map[string]bool
}

type titled struct {
	when  time.Time
	title string
}

func newKnown() *known {
	return &known{cwd: map[string]string{}, root: map[string]string{}, titles: map[string]titled{}, folder: map[string]bool{}}
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

// isFolder is isFolder, asked once a directory.
func (k *known) isFolder(dir, home string) bool {
	k.mu.Lock()
	f, ok := k.folder[dir]
	k.mu.Unlock()
	if !ok {
		f = isFolder(dir, home)
		k.mu.Lock()
		k.folder[dir] = f
		k.mu.Unlock()
	}
	return f
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
	self, err := os.Executable()
	if err != nil {
		return err
	}
	m := model{srv: srv, home: home, conn: connCommand(self), known: newKnown()}
	// The panel's window is conn's: the panel, and the workspace beside
	// it. ctrl-space comes back here, wherever here is now.
	if own := os.Getenv("TMUX_PANE"); own != "" {
		if win, err := srv.Display(own, "#{window_id}"); err == nil && win != "" {
			m.own, m.ws = own, newWorkspace(srv, win, own, self)
			_, _ = srv.Do(bindBack(win, own))
			swaps.Lock()
			_, _ = m.ws.ensure()
			swaps.Unlock()
		}
	}
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
	at       time.Time // when the panes were listed
	projects []project
	root     string
	rows     []row
	waiting  []pane // the panes conn holds a conversation waiting on you in
	// What the workspace shows, blank for a hold, and whether it stands:
	// a pane closed there leaves the panel alone in the window, and one
	// ended there stands dead.
	shown   string
	shownOK bool
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
	srv, home, own, k, window := m.srv, m.home, m.own, m.known, m.ws.window
	var cur *project
	if m.screen == onProject {
		c := m.cur
		cur = &c
	}
	return func() tea.Msg {
		all := transcripts()
		at := time.Now()
		ps, _ := panes(srv)
		var held []pane
		r := read{at: at}
		for _, p := range ps {
			if p.Window == window && p.ID != own {
				r.shownOK = !p.Dead
				if !p.Hold {
					r.shown = p.ID
				}
			}
			if p.Hold {
				continue
			}
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
		r.projects = group(found, func(dir string) bool { return k.isFolder(dir, home) })
		for _, p := range held {
			if state, _ := paneState(p, live); state == "waiting" {
				r.waiting = append(r.waiting, p)
			}
		}

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
				case p.Dir != "" && cur.belongs(k.rootOf(p.Dir)):
					outside = append(outside, p)
				}
			}
			var convs []conversation
			for _, c := range all {
				dir := k.cwdOf(c)
				if dir == "" || !cur.belongs(k.rootOf(dir)) {
					continue
				}
				c.Dir = dir
				convs = append(convs, c)
			}
			in := func(dir string) bool { return cur.belongs(k.rootOf(dir)) }
			r.root, r.rows = cur.Root, arrange(convs, held, live, ports, outside, in, k.titleOf)
			// In a folder of repositories, a row says whose it is.
			if cur.Group {
				for i := range r.rows {
					if root := k.rootOf(r.rows[i].dir()); root != cur.Root {
						r.rows[i].repo = filepath.Base(root)
					}
				}
			}
		}
		return r
	}
}

// arrange lays out a project's rows: what is running in conn first,
// waiting before the rest, then what is running elsewhere, then the
// conversations at rest, newest first, up to conversationsShown.
func arrange(convs []conversation, ps []pane, live map[string]claudeRun, ports map[string][]string, outside []proc, in func(dir string) bool, title func(conversation) string) []row {
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
		if !used[ps[j].ID] && in(ps[j].Dir) {
			state := live[ps[j].Session].state()
			if ps[j].Name != "" && ps[j].Exit != "" {
				state = ended(ps[j].Exit)
			}
			running = append(running, row{pane: &ps[j], state: state})
		}
	}
	// What runs stays in the order it was started, a pane's id being
	// tmux's count of the panes it has made: the order tmux lists panes
	// in is the windows', and showing a pane moves it into conn's.
	sort.SliceStable(running, func(i, j int) bool { return paneNumber(running[i]) < paneNumber(running[j]) })
	// What wants you comes first: a conversation waiting, then a run
	// that failed.
	wantsYou := func(r row) int {
		switch {
		case r.state == "waiting":
			return 0
		case strings.HasPrefix(r.state, "failed "):
			return 1
		}
		return 2
	}
	waitingFirst := func(rs []row) {
		sort.SliceStable(rs, func(i, j int) bool { return wantsYou(rs[i]) < wantsYou(rs[j]) })
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

// preview is the time to show what the cursor is on, once it has
// rested there: passing over rows does not swap each into the
// workspace.
type preview struct{ ask int }

const previewWait = 80 * time.Millisecond

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	return next.(model).reconcile(cmd)
}

// reconcile asks the workspace for what the panel now wants shown,
// once the wish has rested.
func (m model) reconcile(cmd tea.Cmd) (tea.Model, tea.Cmd) {
	if m.ws.srv == nil {
		return m, cmd
	}
	if want := m.wanted(); want != m.asked {
		m.asked = want
		m.asks++
		ask := m.asks
		cmd = tea.Batch(cmd, tea.Tick(previewWait, func(time.Time) tea.Msg { return preview{ask} }))
	}
	return m, cmd
}

// wanted is the pane the workspace is to show: one just opened; in a
// project, the pane of the row under the cursor, or of the line found
// under it while a search stands; and nothing on the projects screen.
func (m model) wanted() string {
	switch {
	case m.follow != "":
		return m.follow
	case m.screen != onProject:
		return ""
	case m.searching:
		if m.hitAt < len(m.found) {
			return m.said[m.found[m.hitAt].said].pane.ID
		}
		return ""
	case m.cursor < len(m.rows) && m.rows[m.cursor].pane != nil:
		return m.rows[m.cursor].pane.ID
	}
	return ""
}

// showing shows a pane in the workspace, off the loop.
func (m model) showing(id string, focus bool) tea.Cmd {
	ws := m.ws
	return func() tea.Msg {
		if err := ws.show(id, focus); err != nil {
			return acted(err.Error())
		}
		return nil
	}
}

func (m model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case preview:
		if msg.ask == m.asks {
			return m, m.showing(m.asked, false)
		}
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case read:
		m.waiting = msg.waiting
		m.takeProjects(msg.projects)
		if m.screen == onProject && msg.root == m.cur.Root {
			on := ""
			if m.cursor < len(m.rows) {
				on = m.rows[m.cursor].key()
			}
			m.rows = msg.rows
			m.cursor = keep(m.cursor, on, m.rows, row.key)
			// A pane just opened takes the cursor once its row is read,
			// and is let go of where a read asked for after it was opened
			// has no row for it: it ended before it was read.
			found := false
			for i, r := range m.rows {
				if m.follow != "" && r.pane != nil && r.pane.ID == m.follow {
					m.cursor, m.follow, found = i, "", true
				}
			}
			if m.follow != "" && !found && msg.at.After(m.followAt) {
				m.follow = ""
			}
		}
		// The workspace is put right where it is not what the panel
		// asked: gone, ended, or showing another pane.
		if m.ws.srv != nil && (!msg.shownOK || msg.shown != m.wanted()) {
			return m, m.showing(m.wanted(), false)
		}
	case changed:
		if m.searching {
			return m, tea.Batch(m.read(), readSaid(m.srv, m.cur.Root, m.rows))
		}
		return m, m.read()
	case saidRead:
		if m.searching && msg.root == m.cur.Root {
			m.said = msg.panes
			m.found = hits(m.query, m.said)
			m.hitAt = min(m.hitAt, max(len(m.found)-1, 0))
		}
	case roleAnswer:
		if m.question == msg.as && m.screen == onProject && msg.root == m.cur.Root {
			m.offer, m.offerErr = msg.list, msg.err
		}
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
		if m.question != "" {
			return m.offerKey(msg)
		}
		if m.searching {
			return m.searchKey(msg)
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
	case "tab":
		return m.nextWaiting()
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
	case "tab":
		return m.nextWaiting()
	case "esc", "backspace":
		m.screen, m.rows, m.cursor = onProjects, nil, 0
		return m, m.read()
	case "up", "k", "ctrl+p":
		m.cursor, m.follow = max(m.cursor-1, 0), ""
	case "down", "j", "ctrl+n":
		m.cursor, m.follow = min(m.cursor+1, max(len(m.rows)-1, 0)), ""
	case "enter":
		if m.cursor < len(m.rows) {
			return m.goTo(m.rows[m.cursor])
		}
	case "n":
		id := newID()
		return m.start(m.cur.Root, newCommand(m.conn, id), id)
	case "s":
		return m.start(m.cur.Root, "", "")
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
	case "!":
		m.asking, m.line = true, ""
	case "/":
		m.searching, m.query, m.said, m.found, m.hitAt = true, "", nil, nil, 0
		return m, readSaid(m.srv, m.cur.Root, m.rows)
	case "u":
		return m.resolve(roleStack)
	case "t":
		return m.resolve(roleCheck)
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

// nextWaiting shows a conversation waiting on you in the workspace, the
// keys in it, the one after the last tab went to, round to the first,
// with its project on the panel and the cursor on its row. A
// conversation waiting in a terminal conn does not hold is not one conn
// can show.
func (m model) nextWaiting() (tea.Model, tea.Cmd) {
	if len(m.waiting) == 0 {
		m.note = "Nothing is waiting on you."
		return m, nil
	}
	next := m.waiting[0]
	for i, p := range m.waiting {
		if p.ID == m.lastWaiting {
			next = m.waiting[(i+1)%len(m.waiting)]
		}
	}
	m.lastWaiting = next.ID
	if root := m.known.rootOf(next.Dir); root != "" && (m.screen != onProject || root != m.cur.Root) {
		cur := project{Name: filepath.Base(root), Root: root}
		for _, p := range m.projects {
			if p.Root == root {
				cur = p
			}
		}
		m.screen, m.cur, m.rows, m.cursor = onProject, cur, nil, 0
		m.searching, m.question = false, ""
	}
	m.follow, m.asked, m.followAt = next.ID, next.ID, time.Now()
	return m, tea.Batch(m.showing(next.ID, true), m.read())
}

// searchKey is a key while /'s search stands: what is typed searches,
// the arrows move between the lines found, enter goes to one, and esc
// puts the search away. The search stands while a line is gone to, for
// the next.
func (m model) searchKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc", "ctrl+c":
		m.searching, m.said, m.found = false, nil, nil
		return m, nil
	case "enter":
		if m.hitAt < len(m.found) {
			h := m.found[m.hitAt]
			return m, land(m.ws, m.srv, m.said[h.said].pane, h.k, m.query)
		}
		return m, nil
	case "up", "ctrl+p", "ctrl+k":
		m.hitAt = max(m.hitAt-1, 0)
		return m, nil
	case "down", "ctrl+n", "ctrl+j":
		m.hitAt = min(m.hitAt+1, max(len(m.found)-1, 0))
		return m, nil
	case "backspace":
		if r := []rune(m.query); len(r) > 0 {
			m.query = string(r[:len(r)-1])
		}
	case "ctrl+u":
		m.query = ""
	default:
		t := k.Key().Text
		if t == "" {
			return m, nil
		}
		m.query += t
	}
	m.found, m.hitAt = hits(m.query, m.said), 0
	return m, nil
}

// offerKey is a key while u's or t's question stands: enter starts
// what Claude Code named, and writes it down as what the role means
// here; esc puts the question away; nothing else does anything.
func (m model) offerKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "enter":
		if m.offer == nil {
			return m, nil
		}
		list, as := m.offer, m.question
		m.question, m.offer = "", nil
		m.note = "starting…"
		return m, m.startRoles(list, as, true)
	case "esc", "q", "ctrl+c":
		m.question, m.offer, m.offerErr = "", nil, ""
	}
	return m, nil
}

// resolve runs what a role means in the project — the stack brought
// up, the checks run — as it was last run here by whoever ran it, and
// asks Claude Code where nobody has yet.
func (m model) resolve(as role) (tea.Model, tea.Cmd) {
	var starts []started
	if m.srv != nil {
		starts = readStarted(startedPath(m.srv.Socket))
	}
	cur := m.cur
	if list := established(starts, cur.belongs, as); len(list) > 0 {
		m.note = "starting…"
		return m, m.startRoles(list, as, false)
	}
	m.question, m.offer, m.offerErr = as, nil, ""
	return m, askRole(m.cur, as)
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

// up starts a command typed at ! at the project's root, out of sight,
// under the name of its program, as conn u does.
func (m model) up(command string) tea.Cmd {
	srv, root, rows := m.srv, m.cur.Root, m.rows
	return func() tea.Msg {
		name := nameFor(command)
		said, err := startIn(srv, rows, name, command, root)
		if err != nil {
			return acted(err.Error())
		}
		return acted(said)
	}
}

// startRoles starts each process of a role, off the loop, each at its
// own root, or the project's where it has none, and writes each down
// as what the role means there where asked to.
func (m model) startRoles(list []stacked, as role, record bool) tea.Cmd {
	srv, root, rows := m.srv, m.cur.Root, m.rows
	return func() tea.Msg {
		var said []string
		for _, s := range list {
			at := or(s.root, root)
			r, err := startIn(srv, rows, s.name, s.command, at)
			if err != nil {
				return acted(s.name + ": " + err.Error())
			}
			if record {
				_ = recordStart(startedPath(srv.Socket), started{Root: at, Name: s.name, Command: s.command, Role: as})
			}
			said = append(said, r)
		}
		return acted(strings.Join(said, "; "))
	}
}

// startIn starts a command under a name at a project's root. One by
// that name that has ended is replaced; one still running is left, and
// said.
func startIn(srv *tmux.Server, rows []row, name, command, root string) (string, error) {
	replace := ""
	for _, r := range rows {
		if r.pane == nil || r.pane.Name != name {
			continue
		}
		if r.pane.Exit == "" {
			return name + " is running already", nil
		}
		replace = r.pane.ID
	}
	if _, err := startNamed(srv, name, command, root); err != nil {
		return "", err
	}
	if replace != "" {
		_, _ = srv.Do(tmux.Kill(replace))
	}
	return name + " is up", nil
}

// nameFor is the name a command typed at ! runs under: its program's,
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
// is one, and shows it in the workspace with the keys in it.
func (m model) start(dir, cmd, session string) (tea.Model, tea.Cmd) {
	id, err := openWindow(m.srv, windowName(m.cur.Name), dir, cmd, session)
	if err != nil {
		m.note = err.Error()
		return m, nil
	}
	m.follow, m.asked, m.followAt = id, id, time.Now()
	return m, tea.Batch(m.showing(id, true), m.read())
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

// goTo puts a row's pane in the workspace with the keys in it, picking
// its conversation back up in a window of its own where it is at rest.
func (m model) goTo(r row) (tea.Model, tea.Cmd) {
	switch {
	case r.pane != nil:
		m.asked = r.pane.ID
		return m, m.showing(r.pane.ID, true)
	case r.elsewhere:
		m.note = "That conversation is running in a terminal conn does not hold."
	case r.conv != nil:
		dir := r.conv.Dir
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			dir = m.cur.Root
		}
		return m.start(dir, resumeCommand(m.conn, r.conv.ID), r.conv.ID)
	}
	return m, nil
}

// shownProjects is the projects the typed words find: each word in the
// name or the path.
func (m model) shownProjects() []project {
	words := strings.Fields(strings.ToLower(m.typed))
	if len(words) == 0 {
		return m.projects
	}
	matches := func(p project) bool {
		hay := strings.ToLower(p.Name + " " + tilde(p.Root, m.home))
		for _, w := range words {
			if !strings.Contains(hay, w) {
				return false
			}
		}
		return true
	}
	// A folder found brings what it holds; a repository found brings
	// the folder it is in, so it is never shown out of its place.
	var out []project
	var folder *project
	folderIn, folderFound := false, false
	for _, p := range m.projects {
		switch {
		case p.Group:
			f := p
			folder, folderIn, folderFound = &f, false, matches(p)
			if folderFound {
				out, folderIn = append(out, p), true
			}
		case p.Depth > 0 && folder != nil:
			if folderFound || matches(p) {
				if !folderIn {
					out, folderIn = append(out, *folder), true
				}
				out = append(out, p)
			}
		default:
			folder = nil
			if matches(p) {
				out = append(out, p)
			}
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
	return project{Name: filepath.Base(root), Root: root, Group: m.known.isFolder(root, m.home)}, true
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
	// The foot is wrapped to the panel's width, and the screen drawn
	// again in the rows the foot leaves it.
	_, foot := m.draw()
	feet := m.feet(foot)
	inner := m
	inner.height = max(m.height-len(feet)+1, 3)
	lines, _ := inner.draw()
	h := max(m.height, len(lines)+len(feet))
	rows := h - len(feet)
	for len(lines) < rows {
		lines = append(lines, "")
	}
	lines = append(lines[:rows], feet...)
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	return v
}

// draw is the screen's lines and its foot: what the last key did, the
// line being typed, or the keys.
func (m model) draw() ([]string, string) {
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
	return lines, foot
}

// feet is the foot as lines the panel's width: the keys, a · between
// each, broken between keys; anything else broken between words. The
// keys are in gray; what a key did, and a line being typed, are not.
func (m model) feet(foot string) []string {
	if foot == "" {
		return []string{""}
	}
	width := max(m.width-1, 10)
	sep, parts := " · ", strings.Split(foot, " · ")
	if len(parts) == 1 {
		sep, parts = " ", strings.Fields(foot)
		if m.asking {
			sep, parts = "", []string{foot}
		}
	}
	var out []string
	line := ""
	for _, p := range parts {
		switch {
		case line == "":
			line = p
		case ansi.StringWidth(line+sep+p) <= width:
			line += sep + p
		default:
			out = append(out, line)
			line = p
		}
	}
	out = append(out, line)
	for i, l := range out {
		l = fit(" "+l, m.width)
		if !m.asking && m.question == "" && m.note == "" {
			l = faint.Render(l)
		}
		out[i] = l
	}
	return out
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
		nameW = max(nameW, ansi.StringWidth(listName(p)))
	}
	nameW = min(nameW, 24)
	for i := first; i < len(shown) && i < first+room; i++ {
		p := shown[i]
		left := " " + pad(listName(p), nameW) + "  " + tilde(p.Root, m.home)
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
	return lines, "type to find, or a path · enter opens · tab goes to what waits · ctrl-c leaves conn running"
}

func (m model) drawProject() ([]string, string) {
	lines := []string{
		bold.Render(" "+listName(project{Name: m.cur.Name, Group: m.cur.Group})) + faint.Render("  "+tilde(m.cur.Root, m.home)),
		"",
	}
	if m.question != "" {
		return m.drawOffer(lines)
	}
	if m.searching {
		return m.drawSearch(lines)
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
		if r.repo != "" {
			left = r.repo + " · " + left
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
	return lines, "enter goes to it · tab goes to what waits · / searches output · o opens its port · x stops · r restarts · u starts the project · t runs the checks · ! runs a command · n new conversation · s shell · esc projects · q leaves"
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

// drawOffer is the project's screen while u's or t's question stands:
// the question, then Claude Code's answer and the two keys that take
// it.
func (m model) drawOffer(lines []string) ([]string, string) {
	what, verb := "runs", "start them"
	if m.question == roleCheck {
		what, verb = "checks", "run them"
	}
	switch {
	case m.offerErr != "":
		lines = append(lines, " Claude would not say what "+what+" "+m.cur.Name+":", " "+m.offerErr)
		return lines, "esc back"
	case m.offer == nil:
		lines = append(lines, " Nobody has run it here yet; asking Claude what "+what+" "+m.cur.Name+"…")
		return lines, "esc cancel"
	}
	ask := " Start these in " + m.cur.Name + "?"
	if m.question == roleCheck {
		ask = " Run these checks in " + m.cur.Name + "?"
	}
	lines = append(lines, bold.Render(ask), "")
	w := 0
	for _, s := range m.offer {
		w = max(w, len(s.name))
	}
	for _, s := range m.offer {
		lines = append(lines, fit("   "+pad(s.name, w)+"  "+s.command, m.width))
	}
	lines = append(lines, "", " "+bold.Render("enter")+"  "+verb+"    "+bold.Render("esc")+"  cancel")
	return lines, ""
}

// drawSearch is the project's screen while /'s search stands: the line
// typed, how many lines say it, and the lines, each under the name of
// the row its pane is, the match in bold.
func (m model) drawSearch(lines []string) ([]string, string) {
	count := ""
	switch {
	case m.said == nil:
		count = "reading…"
	case m.query != "" && len(m.found) == 1:
		count = "1 line"
	case m.query != "":
		count = fmt.Sprintf("%d lines", len(m.found))
	}
	lines = append(lines, spread(" / "+m.query+"▏", faint.Render(count)+" ", m.width), "")
	nameW := 0
	for _, h := range m.found {
		nameW = max(nameW, ansi.StringWidth(m.said[h.said].name))
	}
	nameW = min(nameW, 20)
	room := max(m.height-len(lines)-1, 1)
	first := max(m.hitAt-room+1, 0)
	for i := first; i < len(m.found) && i < first+room; i++ {
		h := m.found[i]
		lead := " " + pad(m.said[h.said].name, nameW) + "  "
		text, at := around(h.line, h.at, max(m.width-ansi.StringWidth(lead)-1, 8))
		at = min(at, len(text))
		end := min(at+len(m.query), len(text))
		line := lead + text[:at] + bold.Render(text[at:end]) + text[end:]
		if i == m.hitAt {
			line = chosen.Render(lead + text)
		}
		lines = append(lines, line)
	}
	return lines, "type to search · ↑↓ · enter goes to it · esc back"
}

// paneNumber is a row's pane's place in the order tmux made its panes,
// and a row with no pane comes after every one that has one.
func paneNumber(r row) int {
	if r.pane == nil {
		return int(^uint(0) >> 1)
	}
	n, err := strconv.Atoi(strings.TrimPrefix(r.pane.ID, "%"))
	if err != nil {
		return int(^uint(0) >> 1)
	}
	return n
}

// listName is a project's name as the list writes it: a folder of
// repositories with a slash after it, and a repository in one indented
// beneath it.
func listName(p project) string {
	switch {
	case p.Group:
		return p.Name + "/"
	case p.Depth > 0:
		return strings.Repeat("  ", p.Depth) + p.Name
	}
	return p.Name
}
