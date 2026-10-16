package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	term "github.com/charmbracelet/x/term"
	"github.com/w0zro/conn/internal/theme"
	"github.com/w0zro/conn/internal/tmux"
)

// The panel is conn's window: the projects work has happened in, and in
// one of them, what runs and the conversations that can be picked back
// up. Enter goes to a thing; ctrl-space comes back. Started again over
// a process standing in the workspace, as a new build starts it, the
// panel opens on that process, the keys in it, where it was left.

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
	g        theme.Ground // the ground the server is on
	bar      string       // the keys as last written on the bar
	// The spinner: whether its tick is running, and the frame it is on.
	spinning bool
	frame    int

	width, height int
	screen        screen
	cursor        int
	note          string // what the last key could not do, or did
	// !'s line: the command being typed, while it is.
	asking bool
	// ?: the screen's keys, over it, until a key puts them away.
	helping bool
	line    string
	// u's, t's or c's question: the role asked about, blank for none,
	// and what Claude Code answered, or why it did not. While it stands
	// it is the project's screen.
	question role
	offer    []stacked
	offerErr string
	// The runs the keys started that are watched for failing at once,
	// those that have while others are still watched, and those the
	// question standing asks about again.
	watching []watched
	failed   []failure
	again    []failure
	// The panes holding a conversation waiting on you, and the one tab
	// last went to, so the next tab goes on to another.
	waiting     []pane
	lastWaiting string
	// /'s search: whether it stands, what is typed, the panes' lines as
	// read when it opened, and the lines that say what is typed. The
	// pane the line under the cursor is shown in, in copy mode on the
	// line, and a count of the moves so that only the last is shown.
	searching bool
	query     string
	said      []written
	found     []hit
	hitAt     int
	landed    string
	peeks     int

	// The projects screen.
	typed    string
	projects []project
	procs    []proc
	loaded   bool

	// A project: what runs in it, and its conversations at rest, which
	// rc lists.
	cur  project
	rows []row
	rest []row
	// A verb pressed and waiting for its object, blank for none.
	verb string
	// The pane the keys were last in, and the one before it:
	// ctrl-space on the panel goes to the other.
	in, other string
	// rc's list of the conversations at rest: whether it stands, what
	// is typed to find one, and the cursor in it.
	listing bool
	pick    string
	pickAt  int
	// ,'s settings: whether they stand, and the cursor in them.
	settings bool
	setAt    int
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
	branch    string   // in a worktree, the branch it has out
	tree      string   // the checkout it runs in
	stand     int      // where it stands on the screen, by its role
}

// The places on a project's screen, by role, in order: conversations,
// the stack, what talks to it, the checks, what the project talks to
// outside conn, and conversations in terminals conn does not hold.
// A row stands in its place whatever its state; wanting you is a
// color, not a move.
const (
	placeConversations = iota
	placeStack
	placeClients
	placeChecks
	placeOutside
	placeElsewhere
	placeRest
)

// The mark a row carries for its role, after the margin and before its
// name: what a row is, said once, in a cell, so the name is only the
// name. A conversation is an opening quote: someone speaking at the
// other end. A client is the prompt it shows
// you. The stack is a ring, the commonest row and the lightest mark;
// a check is the same ring, lighter still, a run that comes and goes;
// a listener outside conn is the ring unfilled, held by nobody.
//
// A check's mark is its verdict once it has one: the dot while it has
// not ended, a check in the running green where it passed, a cross in
// the accent where it failed.
const (
	markConversation = "❝"
	markStack        = "○"
	markClient       = "❯"
	markCheck        = "◦"
	markPass         = "✓"
	markFail         = "✗"
	markOutside      = "◌"
)

// markOf is the mark for where a row stands, and what it says of
// itself: a check's verdict, where it has one.
func markOf(r row) string {
	switch r.stand {
	case placeStack:
		return markStack
	case placeClients:
		return markClient
	case placeChecks:
		switch {
		case r.pane == nil || r.pane.Exit == "":
			return markCheck
		case r.pane.Exit == "0":
			return markPass
		}
		return markFail
	case placeOutside:
		return markOutside
	}
	return markConversation
}

// A roster is what a project has written down it runs, by name: each
// name's role, and its place in the order the names were first written
// down. A client is written under the name of what it talks to, and
// says nothing of that name's role.
type roster struct {
	role map[string]role
	at   map[string]int
}

// isCheck says whether a name is written down as a check of a project:
// the role it was last written under, as the roster takes it.
func isCheck(starts []started, root, name string) bool {
	if name == "" {
		return false
	}
	check := false
	for _, s := range starts {
		if s.Root == root && s.Name == name && s.Role != roleClient {
			check = s.Role == roleCheck
		}
	}
	return check
}

func rosterOf(starts []started, in func(string) bool) roster {
	ros := roster{role: map[string]role{}, at: map[string]int{}}
	for _, s := range starts {
		if !in(s.Root) || s.Role == roleClient {
			continue
		}
		if _, seen := ros.at[s.Name]; !seen {
			ros.at[s.Name] = len(ros.at)
		}
		ros.role[s.Name] = s.Role
	}
	return ros
}

// placeOf is where a row stands, by what it is.
func placeOf(r row, ros roster) int {
	switch {
	case r.outside != nil:
		return placeOutside
	case r.elsewhere:
		return placeElsewhere
	case r.conv != nil || r.pane == nil || r.pane.Session != "":
		return placeConversations
	case r.pane.Name == "":
		// A shell, or a client opened by c: it talks to the stack.
		return placeClients
	case ros.role[r.pane.Name] == roleCheck:
		return placeChecks
	}
	return placeStack
}

// place is what a row says of where it runs, before its name: the
// repository in a folder, the branch in a worktree.
func (r row) place() []piece {
	var out []piece
	for _, p := range []string{r.repo, r.branch} {
		if p != "" {
			out = append(out, piece{p + " · ", gray})
		}
	}
	return out
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
// where each transcript was had, the project each directory is in, the
// checkout it is in, and where a checkout keeps its HEAD. A title is
// kept while its transcript has not moved.
type known struct {
	mu     sync.Mutex
	cwd    map[string]string
	root   map[string]string
	tree   map[string]string
	head   map[string]string
	titles map[string]titled
	folder map[string]bool
	// The checkouts under home, and when they were last looked for.
	repos   []string
	scanned time.Time
}

// scanEvery is how long the checkouts found under home are kept
// before they are looked for again: a clone shows up within it.
const scanEvery = 30 * time.Second

// repositories is the checkouts under home, looked for at most every
// scanEvery, each as the root git names it, so a checkout found by
// walking is the same project as the panes in it: a worktree stands
// for its repository, and a path through a symbolic link for the real
// one. The roots are asked once each.
func (k *known) repositories(home string) []string {
	k.mu.Lock()
	repos, fresh := k.repos, time.Since(k.scanned) < scanEvery
	k.mu.Unlock()
	if fresh {
		return repos
	}
	repos = repositories(home, repoDepth)
	for i, r := range repos {
		repos[i] = k.rootOf(r)
	}
	k.mu.Lock()
	k.repos, k.scanned = repos, time.Now()
	k.mu.Unlock()
	return repos
}

type titled struct {
	when    time.Time
	title   string
	context int
}

func newKnown() *known {
	return &known{cwd: map[string]string{}, root: map[string]string{}, tree: map[string]string{}, head: map[string]string{}, titles: map[string]titled{}, folder: map[string]bool{}}
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

// treeOf is the checkout a directory is in: a worktree's own top, the
// repository's, or the directory where it is in none.
func (k *known) treeOf(dir string) string {
	k.mu.Lock()
	t, ok := k.tree[dir]
	k.mu.Unlock()
	if !ok {
		t = checkout(dir)
		k.mu.Lock()
		k.tree[dir] = t
		k.mu.Unlock()
	}
	return t
}

// branchOf is the branch a checkout has out, read off its HEAD each
// time, since a checkout changes branches under conn; or the
// checkout's own name, detached.
func (k *known) branchOf(tree string) string {
	k.mu.Lock()
	h, ok := k.head[tree]
	k.mu.Unlock()
	if !ok {
		h = headPath(tree)
		k.mu.Lock()
		k.head[tree] = h
		k.mu.Unlock()
	}
	return branchAt(h, tree)
}

func (k *known) titleOf(c conversation) string {
	return k.tailOf(c).title
}

// contextOf is the context a conversation's last turn ran in.
func (k *known) contextOf(c conversation) int {
	return k.tailOf(c).context
}

// tailOf is what the transcript's tail says, read once while the
// transcript has not moved.
func (k *known) tailOf(c conversation) titled {
	k.mu.Lock()
	t, ok := k.titles[c.path]
	k.mu.Unlock()
	if !ok || !t.when.Equal(c.When) {
		title, context := tailRead(c.path)
		t = titled{when: c.When, title: title, context: context}
		k.mu.Lock()
		k.titles[c.path] = t
		k.mu.Unlock()
	}
	return t
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
	g := ground(home, srv.Socket)
	wear(g)
	m := model{srv: srv, home: home, conn: connCommand(self), known: newKnown(), g: g}
	// The panel's window is conn's: the panel, and the workspace beside
	// it. ctrl-space comes back here, wherever here is now.
	if own := os.Getenv("TMUX_PANE"); own != "" {
		if win, err := srv.Display(own, "#{window_id}"); err == nil && win != "" {
			m.own, m.ws = own, newWorkspace(srv, win, own, self)
			// The panel stands on the surface, a step off the ground the
			// workspace is on, so the two halves of the window read as two.
			_ = srv.Paint(own, g.Surface)
			_, _ = srv.Do(bindBack(win, own), tmux.SetGlobal(panelIDMark, own))
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
	procs    []proc // what listens on the machine, for the projects screen's port find
	root     string
	rows     []row
	rest     []row  // the project's conversations at rest
	waiting  []pane // the panes conn holds a conversation waiting on you in
	// What the workspace shows, blank for a hold, and whether it stands:
	// a pane closed there leaves the panel alone in the window, and one
	// ended there stands dead. The directory is where it runs, which
	// says the project it is in.
	shown    string
	shownDir string
	shownOK  bool
}

// acted is what a key that runs off the loop did, or could not do.
type acted string

// spun is the spinner's tick.
type spun struct{}

// The spinner's frames: the cell full but for one dot, the gap going
// round, a full turn in eight, and a turn a second. A single dot going
// round was a trace too faint to be seen turning beside a row of text;
// the full cell has the weight of the dot beside it, and the gap is
// what moves. It turns in the margin, where the rows at work make a
// column of their own and the text they are about keeps its line. Its
// frame is the clock's, so every row turns in step.
var spinner = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

const spinEvery = 125 * time.Millisecond

func spinTick() tea.Cmd {
	return tea.Tick(spinEvery, func(time.Time) tea.Msg { return spun{} })
}

// turns says whether a row is at work on something that will come to
// rest: a conversation working, a check running. A server running is
// not at work in that sense, and sits still.
func turns(r row) bool {
	switch {
	case r.state == "working":
		return true
	case r.stand == placeChecks && r.pane != nil && r.conv == nil:
		return r.pane.Exit == ""
	}
	return false
}

// steady is the mark beside a process running persistently, the
// stack's and what the project talks to outside conn: the spinner's
// cell full, with no gap going round. Running, and at rest.
const steady = "⣿"

// stands says whether a row runs persistently: a process of the stack
// still running, or a listener outside conn.
func stands(r row) bool {
	switch {
	case r.outside != nil:
		return true
	case r.stand == placeStack && r.pane != nil && r.conv == nil && r.pane.Name != "":
		return r.pane.Exit == ""
	}
	return false
}

// turning says whether anything on the screen turns: a row of the open
// project, or another project's line with something at work in it.
func (m model) turning() bool {
	if m.screen != onProject {
		return false
	}
	for _, it := range m.items() {
		if it.row < 0 && it.project.Turning > 0 || it.row >= 0 && turns(m.rows[it.row]) {
			return true
		}
	}
	return false
}

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
					r.shown, r.shownDir = p.ID, p.Dir
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
		starts := readStarted(startedPath(srv.Socket))
		running, waiting, turning := map[string]int{}, map[string]int{}, map[string]int{}
		for _, p := range held {
			seen = append(seen, sighting{Dir: p.Dir, When: time.Now()})
			root := k.rootOf(p.Dir)
			running[root]++
			if p.Exit == "" && isCheck(starts, root, p.Name) {
				turning[root]++
			}
		}
		total := 0
		for _, r := range live {
			switch r.state() {
			case "waiting":
				total++
				waiting[k.rootOf(r.Cwd)]++
			case "working":
				turning[k.rootOf(r.Cwd)]++
			}
		}
		sayWaiting(srv, total)
		// What conn holds a pane in is a project while it does, a
		// checkout or not.
		repos := k.repositories(home)
		for _, p := range held {
			repos = append(repos, k.rootOf(p.Dir))
		}
		found := findProjects(repos, seen, home, k.rootOf)
		for i := range found {
			found[i].Running, found[i].Waiting, found[i].Turning = running[found[i].Root], waiting[found[i].Root], turning[found[i].Root]
		}
		r.projects = group(found, func(dir string) bool { return k.isFolder(dir, home) })
		for _, p := range held {
			if state, _ := paneState(p, live); state == "waiting" {
				r.waiting = append(r.waiting, p)
			}
		}

		procs := machine(ps, live, listening(), parents(), connections())
		r.procs = procs
		if cur != nil {
			// What listens, and where: a port under one of conn's panes is
			// that pane's; a server outside conn whose directory is in the
			// project, or that the project's panes connect to, is a row of
			// its own.
			ports := map[string][]string{}
			var mine, outside []proc
			for _, p := range procs {
				if p.pane != nil {
					ports[p.pane.ID] = p.Ports
					if cur.belongs(k.rootOf(p.Dir)) {
						mine = append(mine, p)
					}
				}
			}
			for _, p := range procs {
				if p.pane == nil && ((p.Dir != "" && cur.belongs(k.rootOf(p.Dir))) || talksTo(mine, p)) {
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
			ros := rosterOf(starts, cur.belongs)
			// The panel is what runs; a conversation at rest is a row of
			// rc's list instead.
			r.root = cur.Root
			for _, row := range arrange(convs, held, live, ports, outside, in, k.titleOf, ros) {
				if row.conv != nil {
					row.conv.Context = k.contextOf(*row.conv)
				}
				if row.conv != nil && row.pane == nil && !row.elsewhere {
					r.rest = append(r.rest, row)
				} else {
					r.rows = append(r.rows, row)
				}
			}
			placed(r.rows, *cur, k)
			placed(r.rest, *cur, k)
			byPlace(r.rows)
		}
		return r
	}
}

// byPlace gathers a project's rows by worktree: the project's own
// checkouts' rows first, as arranged, then each worktree's rows
// together, worktrees by name, each as arranged.
func byPlace(rows []row) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i].worktree(), rows[j].worktree()
		if (a == "") != (b == "") {
			return a == ""
		}
		return a < b
	})
}

// worktree is the line a row stands under: the branch its worktree has
// out, after its repository in a folder; blank in the project's own
// checkout.
func (r row) worktree() string {
	if r.branch == "" {
		return ""
	}
	if r.repo != "" {
		return r.repo + " · " + r.branch
	}
	return r.branch
}

// placed says on each row where it runs, where that is not the
// project's own checkout: in a folder of repositories, the repository;
// in a worktree, the branch it has out.
func placed(rows []row, cur project, k *known) {
	for i := range rows {
		dir := rows[i].dir()
		if dir == "" {
			continue
		}
		root := k.rootOf(dir)
		if cur.Group && root != cur.Root {
			rows[i].repo = filepath.Base(root)
		}
		rows[i].tree = k.treeOf(dir)
		if root != "" && rows[i].tree != root {
			rows[i].branch = k.branchOf(rows[i].tree)
		}
	}
}

// arrange lays out a project's rows in their places: conversations,
// the stack, clients, checks, what is outside conn, what runs
// elsewhere, then the conversations at rest, newest first, up to
// conversationsShown. In a place, what the project has written down
// stands in the order it was first written, and the rest in the order
// it was started.
func arrange(convs []conversation, ps []pane, live map[string]claudeRun, ports map[string][]string, outside []proc, in func(dir string) bool, title func(conversation) string, ros roster) []row {
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
	for i := range running {
		if running[i].pane != nil {
			running[i].ports = ports[running[i].pane.ID]
		}
	}
	for i := range outside {
		running = append(running, row{outside: &outside[i], state: "outside conn", ports: outside[i].Ports})
	}
	for i := range running {
		running[i].stand = placeOf(running[i], ros)
	}
	// In a place, what was written down stands as it was first written,
	// before what was not; the rest in the order it was started, a
	// pane's id being tmux's count of the panes it has made.
	written := func(r row) (int, bool) {
		if r.pane == nil || r.pane.Name == "" {
			return 0, false
		}
		at, ok := ros.at[r.pane.Name]
		return at, ok
	}
	sort.SliceStable(running, func(i, j int) bool {
		a, b := running[i], running[j]
		if a.stand != b.stand {
			return a.stand < b.stand
		}
		ai, aok := written(a)
		bi, bok := written(b)
		switch {
		case aok != bok:
			return aok
		case aok && ai != bi:
			return ai < bi
		}
		return paneNumber(a) < paneNumber(b)
	})
	for i := range elsewhere {
		elsewhere[i].stand = placeElsewhere
	}
	for i := range rest {
		rest[i].stand = placeRest
	}
	return append(append(running, elsewhere...), rest...)
}

// preview is the time to show what the cursor is on, once it has
// rested there: passing over rows does not swap each into the
// workspace.
type preview struct{ ask int }

const previewWait = 80 * time.Millisecond

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	next, cmd = next.(model).reconcile(cmd)
	// The spinner ticks while a row turns, started once and kept going
	// by its own tick, and left to stop when nothing turns.
	if n := next.(model); n.turning() && !n.spinning {
		n.spinning = true
		next, cmd = n, tea.Batch(cmd, spinTick())
	}
	return next.(model).telling(cmd)
}

// telling writes the keys that work here on the key bar, when they
// have changed since the last telling: the bar is tmux's line and
// conn reaches it by setting an option, which is a process.
func (m model) telling(cmd tea.Cmd) (tea.Model, tea.Cmd) {
	if m.srv == nil || m.own == "" {
		return m, cmd
	}
	_, f := m.draw()
	bar := connBar(f.hints, m.g)
	if bar == m.bar {
		return m, cmd
	}
	m.bar = bar
	srv := m.srv
	return m, tea.Batch(cmd, func() tea.Msg {
		_, _ = srv.Do(tmux.SetGlobal(barMark, bar), tmux.RefreshStatus())
		return nil
	})
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
	case m.listing:
		return ""
	case m.searching:
		return m.landed
	}
	if r, ok := m.under(); ok && r.pane != nil {
		return r.pane.ID
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
	case rethemed:
		return m.takeTheme(msg), nil
	case read:
		m.waiting = msg.waiting
		m.procs = msg.procs
		// The first read, with a process standing in the workspace: the
		// panel has been started again over it, and opens on it.
		if !m.loaded && msg.shown != "" && msg.shownOK && m.known.rootOf(msg.shownDir) != "" {
			m.takeProjects(msg.projects)
			return m.openOn(pane{ID: msg.shown, Dir: msg.shownDir})
		}
		// The cursor stays on its item, by key, as the projects and the
		// rows are read again under it.
		// A project just opened has no rows yet, and its cursor is on
		// the first row to come: nothing is kept.
		on := ""
		if items := m.items(); m.screen == onProject && m.rows != nil && m.cursor < len(items) {
			on = m.itemKey(items[m.cursor])
		}
		m.takeProjects(msg.projects)
		if m.screen == onProject && msg.root == m.cur.Root {
			// A project with nothing running is read all the same: nil
			// rows would say the read has not come back.
			m.rows = msg.rows
			if m.rows == nil {
				m.rows = []row{}
			}
		}
		if m.screen == onProject {
			m.cursor = keep(m.cursor, on, m.items(), m.itemKey)
		}
		if m.screen == onProject && msg.root == m.cur.Root {
			picked := ""
			if shown := m.shownRest(); m.pickAt < len(shown) {
				picked = shown[m.pickAt].key()
			}
			m.rest = msg.rest
			m.pickAt = keep(m.pickAt, picked, m.shownRest(), row.key)
			// A pane just opened takes the cursor once its row is read,
			// and is let go of where a read asked for after it was opened
			// has no row for it: it ended before it was read.
			found := false
			for i, r := range m.rows {
				if m.follow != "" && r.pane != nil && r.pane.ID == m.follow {
					m.cursor, m.follow, found = m.itemAt(i), "", true
				}
			}
			if m.follow != "" && !found && msg.at.After(m.followAt) {
				m.follow = ""
			}
		}
		// A run the keys started that failed at once is asked about.
		var quick tea.Cmd
		m, quick = m.watch(msg.at)
		// The workspace is put right where it is not what the panel
		// asked: gone, ended, or showing another pane.
		if m.ws.srv != nil && (!msg.shownOK || msg.shown != m.wanted()) {
			return m, tea.Batch(quick, m.showing(m.wanted(), false))
		}
		return m, quick
	case changed:
		return m, m.read()
	case saidRead:
		if m.searching && msg.root == m.cur.Root {
			m.said = msg.panes
			m.found = hits(m.query, m.said)
			m.hitAt = min(m.hitAt, max(len(m.found)-1, 0))
			return m.peeking()
		}
	case peek:
		if msg.n == m.peeks && m.searching {
			return m.peek()
		}
	case roleAnswer:
		if m.question == msg.as && m.screen == onProject && msg.root == m.cur.Root {
			if msg.kept {
				m.note = names(m.again) + " failed at once; Claude says the command is right and the failure is the project's"
				m.question, m.again = "", nil
				return m, nil
			}
			m.offer, m.offerErr = msg.list, msg.err
		}
	case sized:
		if w, h, err := term.GetSize(os.Stdout.Fd()); err == nil && (w != m.width || h != m.height) {
			m.width, m.height = w, h
		}
	case acted:
		m.note = string(msg)
	case spun:
		if !m.turning() {
			m.spinning = false
			return m, nil
		}
		m.frame = int(time.Now().UnixMilli()/spinEvery.Milliseconds()) % len(spinner)
		return m, spinTick()
	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			m.note = ""
			return m.click(msg.Y)
		}
	case tea.KeyPressMsg:
		m.note = ""
		if s := msg.String(); s == "ctrl+space" || s == "ctrl+@" {
			return m.arrived()
		}
		if m.helping {
			m.helping = false
			return m, nil
		}
		if m.settings {
			return m.settingsKey(msg)
		}
		if m.asking {
			return m.lineKey(msg)
		}
		if m.question != "" {
			return m.offerKey(msg)
		}
		if m.listing {
			return m.listKey(msg)
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
			// A port found opens its project with the cursor on the holder.
			if port, ok := m.typedPort(); ok {
				for _, h := range m.holders(port) {
					if h.pane != nil && shown[m.cursor].belongs(m.known.rootOf(h.Dir)) {
						next, cmd := m.open(shown[m.cursor])
						nm := next.(model)
						nm.follow, nm.asked, nm.followAt = h.pane.ID, h.pane.ID, time.Now()
						return nm, cmd
					}
				}
			}
			return m.open(shown[m.cursor])
		}
		if p, ok := m.typedPath(); ok {
			return m.open(p)
		}
		return m, nil
	case "x":
		if m.cursor < len(shown) && m.typed == "" {
			return m, m.endAll(shown[m.cursor])
		}
	case "up", "ctrl+p", "ctrl+k":
		m.cursor = ring(m.cursor, -1, len(shown))
	case "down", "ctrl+n", "ctrl+j":
		m.cursor = ring(m.cursor, 1, len(shown))
	case "ctrl+c":
		m.note = m.leave()
	case "?":
		m.helping = true
	case ",":
		if m.typed == "" {
			m.settings, m.setAt = true, 0
			return m, nil
		}
		m.typed, m.cursor = m.typed+",", 0
	case "esc":
		// esc takes back what was typed to find; with nothing typed, it
		// goes back down into the project - came up from.
		if m.typed == "" && m.cur.Root != "" {
			return m.open(m.cur)
		}
		m.typed, m.cursor = "", 0
	case "ctrl+u":
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
	if m.verb != "" {
		return m.objectKey(k)
	}
	switch k.String() {
	case "r", "x", "a", "s":
		// A verb waits for its object: the verb again is the row.
		m.verb = k.String()
		return m, nil
	case "tab":
		return m.nextWaiting()
	case "esc":
		// Back into the process the keys were last in, its project on
		// the panel.
		if p, ok := m.standing(m.in); ok {
			return m.openOn(p)
		}
		m.note = "No process to go back to yet."
	case "-":
		// Up to the projects, as vim's - goes up to the directory.
		m.screen, m.rows, m.cursor = onProjects, nil, 0
		return m, m.read()
	case "up", "k", "ctrl+p":
		m.cursor, m.follow = ring(m.cursor, -1, len(m.items())), ""
	case "down", "j", "ctrl+n":
		m.cursor, m.follow = ring(m.cursor, 1, len(m.items())), ""
	case "g":
		m.cursor, m.follow = 0, ""
	case "G":
		m.cursor, m.follow = max(len(m.items())-1, 0), ""
	case "enter":
		return m.goIn()
	case "q", "ctrl+c":
		m.note = m.leave()
	case "!":
		m.asking, m.line = true, ""
	case "/":
		m.searching, m.query, m.said, m.found, m.hitAt = true, "", nil, nil, 0
		return m, readSaid(m.srv, m.cur.Root, m.rows)
	case "c":
		if r, ok := m.under(); ok {
			return m.talk(r)
		}
	case "?":
		m.helping = true
	case ",":
		m.settings, m.setAt = true, 0
	}
	return m, nil
}

// lineKey is a key while !'s line is being typed: enter runs it, and
// with nothing typed opens a shell.
func (m model) lineKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc", "ctrl+c":
		m.asking, m.line = false, ""
	case "enter":
		m.asking = false
		if strings.TrimSpace(m.line) == "" {
			// Nothing typed is a shell.
			return m.start(m.here(), "", "")
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

// rowName is the name a row's process answers to, which a client is
// kept under.
func rowName(r row) string {
	switch {
	case r.pane != nil:
		return paneName(*r.pane)
	case r.outside != nil:
		return r.outside.Name
	}
	return ""
}

// talk opens a session with a row's process: a conversation or a shell
// is gone into; otherwise the client kept under its name, or the
// browser where a port of its answers HTTP, and failing both, Claude
// Code is asked what talks to it.
func (m model) talk(r row) (tea.Model, tea.Cmd) {
	switch {
	case r.conv != nil || (r.pane != nil && (r.pane.Session != "" || r.pane.Name == "")):
		return m.goTo(r)
	case r.pane == nil && r.outside == nil:
		return m, nil
	}
	name := rowName(r)
	var starts []started
	if m.srv != nil {
		starts = readStarted(startedPath(m.srv.Socket))
	}
	if client, ok := clientFor(starts, m.cur.belongs, name); ok {
		return m.useClient(client, false)
	}
	for _, port := range r.ports {
		if speaksHTTP(port) {
			return m.useClient(stacked{name: name, command: "http://localhost:" + port, root: m.cur.Root}, true)
		}
	}
	command, dir := "", r.dir()
	switch {
	case r.pane != nil && r.pane.Name != "":
		command, _ = m.srv.Display(r.pane.ID, "#{"+runMark+"}")
	case r.outside != nil:
		command = r.outside.Command
	}
	m.question, m.offer, m.offerErr = roleClient, nil, ""
	return m, askClient(m.cur, name, command, dir, r.ports)
}

// useClient opens a client: a URL in the browser, a command in a
// window of its own at its root with the keys in it; and keeps it
// under its name where asked to.
func (m model) useClient(c stacked, record bool) (tea.Model, tea.Cmd) {
	if record && m.srv != nil {
		_ = recordStart(startedPath(m.srv.Socket), started{Root: or(c.root, m.cur.Root), Name: c.name, Command: c.command, Role: roleClient})
	}
	if isURL(c.command) {
		url := c.command
		return m, func() tea.Msg {
			if err := openURL(url); err != nil {
				return acted(err.Error())
			}
			return acted("opened " + url)
		}
	}
	return m.start(or(c.root, m.cur.Root), c.command, "")
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
	return m.openOn(next)
}

// openOn shows a pane in the workspace, the keys in it, with its
// project on the panel and the cursor on its row.
func (m model) openOn(p pane) (tea.Model, tea.Cmd) {
	m = m.onProjectOf(p).into(p.ID)
	m.follow, m.asked, m.followAt = p.ID, p.ID, time.Now()
	return m, tea.Batch(m.showing(p.ID, true), m.read())
}

// onProjectOf puts a pane's project on the panel where another is.
func (m model) onProjectOf(p pane) model {
	if root := m.known.rootOf(p.Dir); root != "" && (m.screen != onProject || root != m.cur.Root) {
		cur := project{Name: filepath.Base(root), Root: root}
		for _, q := range m.projects {
			if q.Root == root {
				cur = q
			}
		}
		m.screen, m.cur, m.rows, m.cursor = onProject, cur, nil, 0
		m.searching, m.question = false, ""
	}
	return m
}

// searchKey is a key while /'s search stands: what is typed searches,
// the arrows move between the lines found, enter goes to one, and esc
// puts the search away. The search stands while a line is gone to, for
// the next.
func (m model) searchKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc", "ctrl+c":
		m.searching, m.said, m.found = false, nil, nil
		return m.unland("")
	case "enter":
		if m.hitAt < len(m.found) {
			h := m.found[m.hitAt]
			p := m.said[h.said].pane
			m, let := m.unland(p.ID)
			m = m.into(p.ID)
			m.landed, m.asked = p.ID, p.ID
			return m, tea.Batch(let, land(m.ws, m.srv, p, h.k, m.query))
		}
		return m, nil
	case "up", "ctrl+p", "ctrl+k":
		m.hitAt = ring(m.hitAt, -1, len(m.found))
		return m.peeking()
	case "down", "ctrl+n", "ctrl+j":
		m.hitAt = ring(m.hitAt, 1, len(m.found))
		return m.peeking()
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
	return m.peeking()
}

// A peek is the line under the search's cursor, shown once the cursor
// has rested on it: n is which move it was.
type peek struct{ n int }

// peeking waits for the cursor to rest before the line under it is
// shown, so the arrows held down move through the lines and do not
// show each in turn.
func (m model) peeking() (tea.Model, tea.Cmd) {
	m.peeks++
	n := m.peeks
	return m, tea.Tick(previewWait, func(time.Time) tea.Msg { return peek{n} })
}

// peek shows the line under the search's cursor in its pane, in the
// workspace, in copy mode on the line with every saying of what is
// typed lit: the keys stay on the panel. The pane shown before is let
// go of; with no line found, a hold stands in the workspace.
func (m model) peek() (tea.Model, tea.Cmd) {
	if m.srv == nil || m.ws.srv == nil {
		return m, nil
	}
	if m.hitAt >= len(m.found) {
		return m.unland("")
	}
	h := m.found[m.hitAt]
	p := m.said[h.said].pane
	was := m.landed
	m.landed, m.asked = p.ID, p.ID
	ws, srv, k, text := m.ws, m.srv, h.k, m.query
	return m, func() tea.Msg {
		if was != "" && was != p.ID {
			srv.Unmode(was)
		}
		if err := ws.show(p.ID, false); err != nil {
			return acted(err.Error())
		}
		if err := srv.Land(p.ID, k, text); err != nil {
			return acted(err.Error())
		}
		return nil
	}
}

// unland takes the pane the search last showed out of copy mode, where
// it is not the pane next shown: left in it, it would stand frozen on
// that screen for whoever next goes in.
func (m model) unland(next string) (model, tea.Cmd) {
	was := m.landed
	m.landed = ""
	if was == "" || was == next || m.srv == nil {
		return m, nil
	}
	srv := m.srv
	return m, func() tea.Msg {
		srv.Unmode(was)
		return nil
	}
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
		if as == roleClient {
			return m.useClient(list[0], true)
		}
		m.again = nil
		return m.run(list, as, true)
	case "esc", "q", "ctrl+c":
		m.question, m.offer, m.offerErr, m.again = "", nil, "", nil
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
		return m.run(list, as, false)
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
	srv, hold := m.srv, m.ws.hold
	return func() tea.Msg {
		how, err := endProc(srv, hold, p)
		if err != nil {
			return acted(err.Error())
		}
		return acted(p.Name + " ended: " + how)
	}
}

// endAll ends everything in a project, off the loop: every process conn
// holds in it, conversations included, which keep their transcripts,
// and every server in it outside conn. The project's own project is
// read for each, not the panel's record of it, which can be older.
func (m model) endAll(p project) tea.Cmd {
	srv, hold := m.srv, m.ws.hold
	root := rootOf
	if m.known != nil {
		root = m.known.rootOf
	}
	return func() tea.Msg {
		ps, err := panes(srv)
		if err != nil {
			return acted(err.Error())
		}
		n := 0
		var failed []string
		// What the project talks to, and does not hold, is not ended
		// with it: a database other projects use too.
		for _, pr := range machine(ps, liveClaude(), listening(), parents(), nil) {
			if pr.Dir == "" || !p.belongs(root(pr.Dir)) {
				continue
			}
			if _, err := endProc(srv, hold, pr); err != nil {
				failed = append(failed, err.Error())
				continue
			}
			n++
		}
		said := fmt.Sprintf("%s: ended %d", p.Name, n)
		if len(failed) > 0 {
			said += "; " + strings.Join(failed, "; ")
		}
		return acted(said)
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

// at is the checkout the cursor stands in: the row under it's, or
// blank on no row.
func (m model) at() string {
	if r, ok := m.under(); ok {
		if dir := r.dir(); dir != "" {
			return m.known.treeOf(dir)
		}
	}
	return ""
}

// here is where a verb starts something: the checkout the cursor
// stands in, or the project's root.
func (m model) here() string {
	return or(m.at(), m.cur.Root)
}

// placeFor is where a repository's process runs: the checkout the
// cursor stands in where that is a checkout of the repository, and
// otherwise the repository itself.
func (m model) placeFor(repo string) string {
	if tree := m.at(); tree != "" && m.known.rootOf(tree) == repo {
		return tree
	}
	return repo
}

// up starts a command typed at ! where the cursor stands, out of
// sight, under the name of its program, as conn u does.
func (m model) up(command string) tea.Cmd {
	srv, root, rows := m.srv, m.here(), m.rows
	return func() tea.Msg {
		name := nameFor(command)
		said, err := startIn(srv, rows, name, command, root)
		if err != nil {
			return acted(err.Error())
		}
		return acted(said)
	}
}

// startRoles starts each process of a role, off the loop, each in its
// repository's checkout where the cursor stands, or at its repository,
// and writes each down under the repository as what the role means
// there where asked to.
func (m model) startRoles(list []stacked, as role, record bool) tea.Cmd {
	srv, rows := m.srv, m.rows
	type place struct{ repo, at string }
	places := make([]place, len(list))
	for i, s := range list {
		repo := or(s.root, m.cur.Root)
		places[i] = place{repo, m.placeFor(repo)}
	}
	return func() tea.Msg {
		var said []string
		for i, s := range list {
			r, err := startIn(srv, rows, s.name, s.command, places[i].at)
			if err != nil {
				return acted(s.name + ": " + err.Error())
			}
			if record {
				_ = recordStart(startedPath(srv.Socket), started{Root: places[i].repo, Name: s.name, Command: s.command, Role: as})
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
	m = m.into(id)
	m.follow, m.asked, m.followAt = id, id, time.Now()
	return m, tea.Batch(m.showing(id, true), m.read())
}

// arrived is ctrl-space heard on the panel. From a pane, the pane is
// written down by the key as it fires and read here once: the keys
// were in it. Pressed on the panel, where the keys already were, it
// is the other process: the one worked in before the last. So from
// inside a process, twice over is the other process. Arriving from a
// pane puts its project on the panel, the cursor on its row.
func (m model) arrived() (tea.Model, tea.Cmd) {
	if m.srv == nil {
		return m, nil
	}
	from, _ := m.srv.Global(fromMark)
	if from == "" {
		return m.toOther()
	}
	_, _ = m.srv.Do(tmux.UnsetGlobal(fromMark))
	m = m.into(from)
	p, ok := m.standing(from)
	if !ok {
		return m, nil
	}
	// Its project is the one on the panel, the cursor on its row.
	m = m.onProjectOf(p)
	m.follow, m.followAt = p.ID, time.Now()
	return m, m.read()
}

// into notes that the keys have gone into a pane: it is the one they
// are in, and the one they were in is the other.
func (m model) into(id string) model {
	if id != "" && id != m.in {
		m.other, m.in = m.in, id
	}
	return m
}

// standing is a pane the keys can go into, by id: a pane of a
// process, not the panel's own and not a hold, and not ended; and
// whether there is one.
func (m model) standing(id string) (pane, bool) {
	if id == "" || id == m.own || m.srv == nil {
		return pane{}, false
	}
	ps, _ := panes(m.srv)
	for _, p := range ps {
		if p.ID == id && !p.Hold && !p.Dead && !p.Panel {
			return p, true
		}
	}
	return pane{}, false
}

// toOther puts the keys in the process worked in before the one they
// were last in, with its project on the panel where that is another,
// and takes that one as the other, so twice over is where it started.
func (m model) toOther() (tea.Model, tea.Cmd) {
	p, ok := m.standing(m.other)
	if !ok {
		m.note = "No other process to go to yet."
		return m, nil
	}
	return m.openOn(p)
}

// converse starts a conversation where the cursor stands, with a first
// prompt where there is one.
func (m model) converse(prompt string) (tea.Model, tea.Cmd) {
	id := newID()
	return m.start(m.here(), newCommand(m.told(), id, prompt), id)
}

// leave lets the terminal go; what runs in conn keeps on.
func (m model) leave() string {
	if err := leave(m.srv); err != nil {
		return err.Error()
	}
	return ""
}

func (m model) open(p project) (tea.Model, tea.Cmd) {
	m.screen, m.cur, m.rows, m.cursor, m.follow = onProject, p, nil, 0, ""
	return m, m.read()
}

// goIn goes into what the cursor is on: a row, or another project.
func (m model) goIn() (tea.Model, tea.Cmd) {
	if r, ok := m.under(); ok {
		return m.goTo(r)
	}
	if p, ok := m.lineUnder(); ok && !m.isOpen(p) {
		return m.open(p)
	}
	return m, nil
}

// click is a left click on the panel: the cursor goes to the row or the
// project's line under it, and in, as enter does. A click anywhere else
// is nothing. The screen is measured as drawn, in the rows the foot
// leaves it.
func (m model) click(y int) (tea.Model, tea.Cmd) {
	if m.helping || m.settings || m.asking || m.question != "" || m.listing || m.searching {
		return m, nil
	}
	inner, _ := m.inner()
	if m.screen == onProjects {
		if i, ok := inner.projectOn(y); ok {
			m.cursor = i
			return m.open(m.shownProjects()[i])
		}
		return m, nil
	}
	if i, ok := inner.itemOn(y); ok {
		m.cursor, m.follow, m.verb = i, "", ""
		return m.goIn()
	}
	return m, nil
}

// goTo puts a row's pane in the workspace with the keys in it, picking
// its conversation back up in a window of its own where it is at rest.
func (m model) goTo(r row) (tea.Model, tea.Cmd) {
	switch {
	case r.pane != nil:
		m = m.into(r.pane.ID)
		m.asked = r.pane.ID
		return m, m.showing(r.pane.ID, true)
	case r.elsewhere:
		m.note = "That conversation is running in a terminal conn does not hold."
	case r.conv != nil:
		dir := r.conv.Dir
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			dir = m.cur.Root
		}
		return m.start(dir, resumeCommand(m.told(), r.conv.ID), r.conv.ID)
	}
	return m, nil
}

// told is what a conversation started in the project is told.
func (m model) told() string {
	socket := ""
	if m.srv != nil {
		socket = m.srv.Socket
	}
	return told(m.conn, socket, m.cur.belongs)
}

// hand starts a conversation in the project given a row's process:
// what it is, how it is doing, and what it last wrote.
func (m model) hand(r row) (tea.Model, tea.Cmd) {
	if r.conv != nil || (r.pane != nil && r.pane.Session != "") {
		m.note = "That is a conversation already; enter goes to it."
		return m, nil
	}
	h, ok := m.handing(r)
	if !ok {
		return m, nil
	}
	return m.converse(handPrompt(h))
}

// handAll starts one conversation in the project given every run that
// failed.
func (m model) handAll(rows []row) (tea.Model, tea.Cmd) {
	var hs []handing
	for _, r := range rows {
		if h, ok := m.handing(r); ok {
			hs = append(hs, h)
		}
	}
	switch len(hs) {
	case 0:
		m.note = "Nothing failed here."
		return m, nil
	case 1:
		return m.converse(handPrompt(hs[0]))
	}
	return m.converse(handAllPrompt(hs))
}

// handing is a row's process as it is handed to an agent; a
// conversation is none.
func (m model) handing(r row) (handing, bool) {
	var h handing
	switch {
	case r.conv != nil || (r.pane != nil && r.pane.Session != ""):
		return h, false
	case r.pane != nil:
		h.name, h.dir, h.state = paneName(*r.pane), r.pane.Dir, or(r.state, "running")
		if r.pane.Name != "" {
			h.command, _ = m.srv.Display(r.pane.ID, "#{"+runMark+"}")
		}
		h.lines, _ = said(m.srv, r.pane.ID)
	case r.outside != nil:
		h.name, h.command, h.dir, h.state = r.outside.Name, r.outside.Command, r.outside.Dir, "running outside conn, pid "+strconv.Itoa(r.outside.PID)
		if len(r.ports) > 0 {
			h.state += ", on :" + strings.Join(r.ports, " :")
		}
	default:
		return h, false
	}
	h.dir = tilde(h.dir, m.home)
	return h, true
}

// shownProjects is the projects the typed words find: each word in the
// name or the path.
func (m model) shownProjects() []project {
	if port, ok := m.typedPort(); ok {
		return m.holding(port)
	}
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

// typedPort is the port typed to find what holds it: a colon and
// digits.
func (m model) typedPort() (string, bool) {
	t := strings.TrimSpace(m.typed)
	port, ok := strings.CutPrefix(t, ":")
	if !ok || port == "" {
		return "", false
	}
	for _, r := range port {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	return port, true
}

// holders is what listens on a port.
func (m model) holders(port string) []proc {
	var out []proc
	for _, p := range m.procs {
		if slices.Contains(p.Ports, port) {
			out = append(out, p)
		}
	}
	return out
}

// holding is the projects something holds a port in, each with the
// folder of repositories it is in, as the finder lays them out.
func (m model) holding(port string) []project {
	held := map[string]bool{}
	for _, h := range m.holders(port) {
		if root := m.known.rootOf(h.Dir); root != "" {
			held[root] = true
		}
	}
	var out []project
	var folder *project
	folderIn := false
	for _, p := range m.projects {
		switch {
		case p.Group:
			f := p
			folder, folderIn = &f, false
		case p.Depth > 0 && folder != nil:
			if held[p.Root] {
				if !folderIn {
					out, folderIn = append(out, *folder), true
				}
				out = append(out, p)
			}
		default:
			folder = nil
			if held[p.Root] {
				out = append(out, p)
			}
		}
	}
	return out
}

// holderSays is what holds a port in a project, as the line says it:
// each holder's name and the port.
func (m model) holderSays(p project, port string) string {
	var names []string
	for _, h := range m.holders(port) {
		if p.belongs(m.known.rootOf(h.Dir)) {
			names = append(names, h.Name+" :"+port)
		}
	}
	return strings.Join(names, ", ")
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

// Drawing. The panel is drawn in the theme's roles on the surface its
// pane is painted on: a margin of three columns on the left, where the
// cursor's bar stands, the rows between it and a column short of the
// right edge, and the row under the cursor on the chosen ground from
// edge to edge. A title is in the parchment, a path or a figure in the
// gray, what runs in the ink, what rests in the gray, what waits on
// you in the accent; a key is the ink in bold and what it does the
// gray after it.

const (
	margin    = 3
	cursorBar = "▌"
	caret     = "▏"
)

// The styles, in the ground the server is on once the panel knows it;
// plain until then, and in the tests.
var (
	ink, title, projectTitle, subhead, gray, faint, accent, failing, keyInk, chosen, chosenBar, ruleStyle, turning, passed, chosenTurning lipgloss.Style
)

// wear dresses the panel's styles in a ground. The pane's ground is
// the surface, painted by the panel on its pane; the styles say only
// the ink each piece is written in, and the chosen ground.
func wear(g theme.Ground) {
	fg := func(h string) lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color(h)) }
	ink = fg(theme.Hex(g.Ink))
	title = fg(g.Parchment).Bold(true)
	// A screen's title is the accent's brighter cousin: the panel's
	// name for where you are.
	projectTitle = fg(g.Shimmer).Bold(true)
	subhead = fg(g.Parchment)
	gray = fg(g.Gray)
	faint = fg(g.Faint)
	accent = fg(g.Accent).Bold(true)
	// A failure is a verdict, and a verdict is slot-shaped: the
	// terminal's red, as Claude Code's error is.
	failing = fg(g.Scheme[1]).Bold(true)
	keyInk = fg(theme.Hex(g.Ink)).Bold(true)
	chosen = lipgloss.NewStyle().Background(lipgloss.Color(g.Chosen)).Foreground(lipgloss.Color(g.ChosenInk))
	chosenBar = lipgloss.NewStyle().Background(lipgloss.Color(g.Chosen)).Foreground(lipgloss.Color(g.ChosenMark)).Bold(true)
	turning = fg(g.Turning)
	passed = fg(g.Running)
	chosenTurning = lipgloss.NewStyle().Background(lipgloss.Color(g.Chosen)).Foreground(lipgloss.Color(g.Turning))
	ruleStyle = fg(g.Border)
}

// A piece is text in a style.
type piece struct {
	s  string
	st lipgloss.Style
}

// keeps says whether a piece keeps its own ink under the cursor rather
// than taking the chosen ink: a verdict does, as the spinner does.
func (p piece) keeps() bool {
	return p.s == markPass+" " || p.s == markFail+" "
}

// measure is the columns a row has between the margin and the right
// edge, less one for the border beside the panel.
func (m model) measure() int { return max(m.width-margin-2, 10) }

// at is text at the margin.
func at(s string) string { return strings.Repeat(" ", margin) + s }

// row lays a row out: the margin, the left pieces, the gap, and the
// right piece against the measure. The left gives way to the right,
// its last piece cut short with an ellipsis. The row under the cursor
// is on the chosen ground from edge to edge in the chosen ink, its bar
// in the margin, and nothing else colored on it.
func (m model) row(left []piece, right piece, cursor bool) string {
	return m.rowTurning("", left, right, cursor)
}

// rowTurning lays a row out with a mark in the running ink in the
// margin, the column after the cursor's bar: the spinner's frame where
// a row turns, the steady cell where it stands; and is row where turn
// is blank.
func (m model) rowTurning(turn string, left []piece, right piece, cursor bool) string {
	measure := m.measure()
	rw := ansi.StringWidth(right.s)
	if rw > measure {
		right.s, rw = ansi.Truncate(right.s, measure, "…"), measure
	}
	room := measure
	if rw > 0 {
		room = max(measure-rw-2, 0)
	}
	var fitted []piece
	used := 0
	for _, p := range left {
		w := ansi.StringWidth(p.s)
		if used+w > room {
			if cut := ansi.Truncate(p.s, max(room-used, 0), "…"); cut != "" {
				fitted = append(fitted, piece{cut, p.st})
				used += ansi.StringWidth(cut)
			}
			break
		}
		fitted = append(fitted, p)
		used += w
	}
	gap := strings.Repeat(" ", max(measure-used-rw, 1))
	if cursor {
		// The row is written in the chosen ink on the chosen ground,
		// edge to edge; a piece that keeps its ink is written in it on
		// the same ground.
		var b strings.Builder
		plain := strings.Repeat(" ", margin-2)
		width := ansi.StringWidth(plain)
		for _, p := range fitted {
			if p.keeps() {
				b.WriteString(chosen.Render(plain))
				b.WriteString(p.st.Background(chosen.GetBackground()).Render(p.s))
				plain = ""
			} else {
				plain += p.s
			}
			width += ansi.StringWidth(p.s)
		}
		plain += gap + right.s
		width += ansi.StringWidth(gap + right.s)
		plain += strings.Repeat(" ", max(m.width-2-width, 0))
		lead := chosen.Render(" ")
		if turn != "" {
			lead = chosenTurning.Render(turn)
		}
		return chosenBar.Render(cursorBar) + lead + b.String() + chosen.Render(plain)
	}
	var b strings.Builder
	if turn != "" {
		b.WriteString(" " + turning.Render(turn) + strings.Repeat(" ", margin-2))
	} else {
		b.WriteString(strings.Repeat(" ", margin))
	}
	for _, p := range fitted {
		b.WriteString(p.st.Render(p.s))
	}
	b.WriteString(gap)
	b.WriteString(right.st.Render(right.s))
	return b.String()
}

// A keyHint is a key and what it does here.
type keyHint struct{ key, does string }

// The keys each screen says on the bar: the verbs, a word each, so
// the bar fits a window of any width; ? shows them all, in full, and
// the keys the bar leaves out.
var (
	projectHints  = []keyHint{{"enter", "go"}, {"/", "search"}, {"s", "start"}, {"x", "stop"}, {"r", "restart"}, {"a", "agent"}, {"c", "talk"}, {"!", "run"}, {"?", "keys"}}
	projectsHints = []keyHint{{"type", "find"}, {"enter", "open"}, {"x", "stop"}, {"esc", "back"}, {"ctrl-c", "leave"}, {"?", "keys"}}
	projectKeys   = []keyHint{{"enter", "goes to it"}, {"j k", "move, around at the ends"}, {"g G", "top, bottom"}, {"tab", "what waits"}, {"/", "search output"}, {"ss", "start the stack"}, {"st", "run the checks"}, {"sc", "new conversation"}, {"!", "run a command, or a shell"}, {"aa", "hand to an agent"}, {"af", "hand what failed to one"}, {"xx", "stop"}, {"xs", "stop the stack"}, {"xt", "stop the checks"}, {"xp", "stop everything"}, {"rr", "restart"}, {"rs", "restart the stack"}, {"rt", "run the checks again"}, {"rf", "restart what failed"}, {"rc", "conversations at rest"}, {"c", "talk to it"}, {"esc", "back into the process you were in"}, {"-", "projects"}, {",", "settings"}, {"q", "leave"}}
	projectsKeys  = []keyHint{{"type", "to find, or a path to open"}, {"enter", "opens"}, {"tab", "what waits"}, {"x", "stops everything in it"}, {"esc", "back to the project, or clears the find"}, {",", "settings"}, {"ctrl-c", "leaves conn running"}}
	searchHints   = []keyHint{{"type", "to search"}, {"↑ ↓", "move"}, {"enter", "goes to it"}, {"esc", "back"}}
	backHints     = []keyHint{{"esc", "back"}}
	settingsHints = []keyHint{{"j k", "move"}, {"g G", "top, bottom"}, {"enter", "set"}, {"esc", "back"}}
	// The objects each verb waits for.
	objectHints = map[string][]keyHint{
		"r": {{"r", "the row"}, {"s", "the stack"}, {"t", "the checks"}, {"f", "what failed"}, {"c", "a conversation at rest"}, {"esc", "cancel"}},
		"a": {{"a", "the row"}, {"f", "what failed"}, {"esc", "cancel"}},
		"s": {{"s", "the stack"}, {"t", "the checks"}, {"c", "a conversation"}, {"esc", "cancel"}},
		"x": {{"x", "the row"}, {"s", "the stack"}, {"t", "the checks"}, {"p", "the project"}, {"esc", "cancel"}},
	}
	listHints   = []keyHint{{"type", "to find"}, {"↑ ↓", "move"}, {"enter", "picks it back up"}, {"esc", "back"}}
	cancelHints = []keyHint{{"esc", "cancel"}}
)

// A foot is what stands under a screen: the keys, or a line of conn's
// own saying — what the last key did, or the line being typed.
type foot struct {
	hints []keyHint
	say   string
}

func (m model) View() tea.View {
	// The foot is wrapped to the panel's width, and the screen drawn
	// again in the rows the foot leaves it.
	inner, feet := m.inner()
	lines, _ := inner.draw()
	h := max(m.height, len(lines)+len(feet))
	rows := h - len(feet)
	for len(lines) < rows {
		lines = append(lines, "")
	}
	lines = append(lines[:rows], feet...)
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	// The mouse reaches the panel: tmux hands a click on a pane to the
	// program in it once the program asks for the mouse.
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// inner is the panel in the rows the foot leaves it, and the foot.
func (m model) inner() (model, []string) {
	_, f := m.draw()
	feet := m.feet(f)
	inner := m
	inner.height = max(m.height-len(feet)+1, 3)
	return inner, feet
}

// draw is the screen's lines and its foot.
func (m model) draw() ([]string, foot) {
	var lines []string
	var f foot
	switch {
	case m.helping:
		lines, f = m.drawKeys()
	case m.settings:
		lines, f = m.drawSettings()
	case m.screen == onProjects:
		lines, f = m.drawProjects()
	default:
		lines, f = m.drawProject()
	}
	if m.note != "" {
		f.say = m.note
	}
	if m.asking {
		f.say = "run in " + m.cur.Name + ": " + m.line + caret
	}
	return lines, f
}

// feet is the foot as lines the panel's width: a saying of conn's own,
// in the ink, broken between words, under a row of air; and nothing
// otherwise, the keys being on the key bar across the window's foot.
func (m model) feet(f foot) []string {
	if f.say == "" {
		return []string{""}
	}
	width := m.measure()
	var parts []string
	var widths []int
	for _, w := range strings.Fields(f.say) {
		parts, widths = append(parts, ink.Render(w)), append(widths, ansi.StringWidth(w))
	}
	sep, sepW := " ", 1
	out := []string{""}
	line, used := "", 0
	for i, p := range parts {
		switch {
		case line == "":
			line, used = p, widths[i]
		case used+sepW+widths[i] <= width:
			line, used = line+sep+p, used+sepW+widths[i]
		default:
			out = append(out, at(line))
			line, used = p, widths[i]
		}
	}
	if line != "" {
		out = append(out, at(line))
	}
	return out
}

// head is a screen's first rows: the title in the accent's brighter
// cousin, what goes with it in the gray, and a row of air.
func head(name, with string) []string {
	return []string{"", at(projectTitle.Render(name) + "  " + gray.Render(with)), ""}
}

// drawKeys is the screen's keys in full, one to a line, over it.
func (m model) drawKeys() ([]string, foot) {
	name, keys := "conn", projectsKeys
	if m.screen == onProject {
		name, keys = listName(project{Name: m.cur.Name, Group: m.cur.Group}), projectKeys
	}
	lines := head(name, "keys")
	w := 0
	for _, k := range keys {
		w = max(w, ansi.StringWidth(k.key))
	}
	for _, k := range keys {
		lines = append(lines, at(ink.Render(pad(k.key, w))+"  "+gray.Render(k.does)))
	}
	return lines, foot{hints: backHints}
}

func (m model) drawProjects() ([]string, foot) {
	lines := head("conn", "projects")
	lines = append(lines, at(gray.Render("find")+"  "+ink.Render(m.typed)+accent.Render(caret)), "")
	shown := m.shownProjects()
	first, room := m.projectsWindow(len(lines))
	nameW := 0
	for _, p := range shown {
		nameW = max(nameW, ansi.StringWidth(listName(p)))
	}
	nameW = min(nameW, 24)
	for i := first; i < len(shown) && i < first+room; i++ {
		p := shown[i]
		name := gray
		switch {
		case p.Waiting > 0:
			name = accent
		case p.Running > 0:
			name = ink
		}
		left := []piece{{pad(listName(p), nameW), name}, {"  " + tilde(p.Root, m.home), gray}}
		right := p.standing()
		if port, ok := m.typedPort(); ok {
			right = m.holderSays(p, port)
		}
		lines = append(lines, m.row(left, piece{right, gray}, i == m.cursor))
	}
	if port, ok := m.typedPort(); ok && m.loaded {
		lines = append(lines, m.drawOutside(port, len(shown) > 0)...)
		return lines, foot{hints: projectsHints}
	}
	switch {
	case !m.loaded:
		lines = append(lines, at(faint.Render("reading where work has happened…")))
	case len(shown) == 0 && m.typed != "":
		if p, ok := m.typedPath(); ok {
			lines = append(lines, at(ink.Render("enter opens "+tilde(p.Root, m.home))))
		} else {
			lines = append(lines, at(faint.Render("nothing matches; a path to a directory opens it")))
		}
	case len(shown) == 0:
		lines = append(lines, at(faint.Render("no projects yet; type a path to one")))
	}
	return lines, foot{hints: projectsHints}
}

// drawOutside is what holds a port outside every project, one plain
// line and under it the process and where it runs; and where nothing
// holds it at all, that.
func (m model) drawOutside(port string, inProject bool) []string {
	var out []string
	for _, h := range m.holders(port) {
		if m.known.rootOf(h.Dir) != "" {
			continue
		}
		out = append(out, at(ink.Render(h.Name+" holds :"+port+", outside any project")))
		what := strings.TrimSpace(strconv.Itoa(h.PID) + " " + h.Command)
		if h.Dir != "" {
			what += "  " + tilde(h.Dir, m.home)
		}
		out = append(out, at(gray.Render(what)))
	}
	if len(out) == 0 && !inProject {
		out = append(out, at(faint.Render("nothing holds :"+port)))
	}
	return out
}

// projectsWindow is the window the projects screen shows of its
// projects under a head of so many lines: the first project shown, and
// how many, the cursor's in view.
func (m model) projectsWindow(head int) (first, room int) {
	room = max(m.height-head-1, 1)
	return max(m.cursor-room+1, 0), room
}

// projectOn is the project whose line a click landed on, by the line's
// row on the screen.
func (m model) projectOn(y int) (int, bool) {
	head := len(head("", "")) + 2 // the find line, and a row of air
	first, room := m.projectsWindow(head)
	i := y - head + first
	if y < head || i >= first+room || i >= len(m.shownProjects()) {
		return 0, false
	}
	return i, true
}

func (m model) drawProject() ([]string, foot) {
	lines := head(listName(project{Name: m.cur.Name, Group: m.cur.Group}), tilde(m.cur.Root, m.home))
	if m.question != "" {
		return m.drawOffer(lines)
	}
	if m.searching {
		return m.drawSearch(lines)
	}
	if m.listing {
		return m.drawList(lines)
	}
	// The open project's rows, one under the next with no air between
	// the places, since the figure says what a row is; then the other
	// projects' lines; and the window of it all that keeps the cursor's
	// line in view.
	switch {
	case m.rows == nil:
		lines = append(lines, at(faint.Render("reading…")))
	case len(m.rows) == 0:
		// Nothing running says so, then what starts something, a key a
		// row as the bar has them, and air before the other projects.
		key := ink.Bold(true)
		lines = append(lines, at(gray.Render("nothing running")), "",
			at(key.Render("sc")+gray.Render("  starts a conversation")))
		if n := len(m.rest); n > 0 {
			many := "one"
			if n > 1 {
				many = strconv.Itoa(n)
			}
			lines = append(lines, at(key.Render("rc")+gray.Render("  picks up a conversation, "+many+" at rest")))
		}
		lines = append(lines, "")
	}
	l := m.lay(len(lines))
	hints := projectHints
	if m.verb != "" {
		hints = objectHints[m.verb]
	}
	return append(lines, l.body[l.first:min(l.first+l.room, len(l.body))]...), foot{hints: hints}
}

// A laid is the open project's body as drawn under a head: its lines,
// the item each line is, or -1 for a label or air, and the window of
// it in view, the cursor's line inside.
type laid struct {
	body  []string
	owner []int
	first int
	room  int
}

// lay lays the open project's items out under a head of so many lines.
func (m model) lay(head int) laid {
	var l laid
	add := func(line string, owner int) {
		l.body, l.owner = append(l.body, line), append(l.owner, owner)
	}
	cursorAt := 0
	items := m.items()
	for i, it := range items {
		switch {
		case it.row < 0:
			// The other projects stand under a label of their own, with
			// a rule running off it, so they read as projects and not as
			// rows of this one.
			if i == 0 || items[i-1].row >= 0 {
				if i > 0 {
					add("", -1)
				}
				add(m.label("projects"), -1)
			}
			if i == m.cursor {
				cursorAt = len(l.body)
			}
			add(m.projectLine(it.project, i == m.cursor), i)
		default:
			// A worktree's rows stand in under a head of their own: the
			// branch alone, in the parchment the project's head is in
			// and without its weight, a lesser head of the same kind.
			// A label with a rule is a section of the panel, which a
			// worktree is not.
			r := m.rows[it.row]
			depth := 0
			if r.worktree() != "" {
				depth = 1
				// A worktree stands a row off what is above it.
				if it.row == 0 || m.rows[it.row-1].worktree() != r.worktree() {
					add("", -1)
					add(at(subhead.Render(r.worktree())), -1)
				}
			}
			if i == m.cursor {
				cursorAt = len(l.body)
			}
			add(m.drawRow(r, depth, i == m.cursor), i)
		}
	}
	l.room = max(m.height-head-1, 1)
	l.first = max(cursorAt-l.room+1, 0)
	return l
}

// itemOn is the item a click landed on, by its row on the screen of
// the open project, where it landed on one.
func (m model) itemOn(y int) (int, bool) {
	head := len(head("", ""))
	l := m.lay(head)
	i := y - head + l.first
	if y < head || i >= l.first+l.room || i >= len(l.body) || l.owner[i] < 0 {
		return 0, false
	}
	return l.owner[i], true
}

// drawRow is one row of a project: the mark, the name, and the figure.
// The mark says the role; the name is in the ink where it runs and the
// gray where it rests, and the mark in the same. The
// figure at the right is the one thing worth knowing of the row without
// going in: its ports, the context a conversation runs in, how and how
// long ago a run ended, or what wants you.
func (m model) drawRow(r row, depth int, cursor bool) string {
	var left []piece
	var right piece
	// A row under a worktree's head stands in from it.
	if depth > 0 {
		left = append(left, piece{strings.Repeat("  ", depth), ink})
	}
	// The mark says the role, in the name's own ink.
	left = append(left, piece{markOf(r) + " ", gray})
	// Under its worktree's line, a row says only its name.
	if r.branch == "" {
		left = append(left, r.place()...)
	}
	switch {
	case r.conv != nil:
		name := r.conv.Title
		if name == "" {
			name = r.conv.ID
		}
		switch {
		case r.state == "waiting":
			left, right = append(left, piece{name, accent}), piece{"waiting", accent}
		case r.pane != nil:
			left, right = append(left, piece{name, ink}), piece{stated(r.state), gray}
		case r.elsewhere:
			left, right = append(left, piece{name, ink}), piece{strings.TrimSpace(stated(r.state) + " in another terminal"), gray}
		default:
			left, right = append(left, piece{name, gray}), piece{ago(r.conv.When), faint}
		}
		// A conversation running says the context its last turn ran
		// in, before its state.
		if r.conv.Context > 0 && (r.pane != nil || r.elsewhere) {
			right.s = strings.TrimSpace(tokens(r.conv.Context) + "  " + right.s)
		}
	case r.pane != nil:
		name := paneName(*r.pane)
		if r.pane.Session != "" {
			// Started by n, with nothing said yet: no transcript.
			name = "new conversation"
		}
		at := len(left)
		left = append(left, piece{name, ink})
		// Started below the checkout the row says, it says where.
		if r.pane.Dir != or(r.tree, m.cur.Root) {
			left = append(left, piece{"  " + tilde(r.pane.Dir, m.home), gray})
		}
		right = piece{stated(r.state), gray}
		// A run that has ended says how long ago, which says it ended
		// well; one that did not says its status first, in red.
		// One from before conn wrote the time down says only how it
		// did.
		if r.pane.Name != "" && r.pane.Exit != "" {
			// The red says a run failed; the figure keeps the
			// status, which says how, and the age. A check's mark says
			// its verdict, so its figure is the age alone.
			how, when := strings.TrimPrefix(ended(r.pane.Exit), "failed "), ago(r.pane.Ended)
			if r.stand == placeChecks || (how == "done" && when != "") {
				how = ""
			}
			right.s = strings.TrimSpace(how + " " + when)
		}
		// Wanting you is the accent, where the row stands, and a run
		// that failed is the terminal's red.
		switch {
		case r.state == "waiting":
			left[at].st, right.st = accent, accent
		case strings.HasPrefix(r.state, "failed "):
			left[at].st, right.st = failing, failing
		}
	case r.outside != nil:
		// The mark says it is outside conn; the figure is its ports.
		left = append(left, piece{r.outside.Name, ink})
	}
	if len(r.ports) > 0 {
		right.s = strings.TrimSpace(":" + strings.Join(r.ports, " :") + "  " + right.s)
	}
	// The mark takes the name's ink; a verdict takes its own, the
	// running green for a pass, and keeps it under the cursor.
	for i := range left {
		if left[i].s != markOf(r)+" " {
			continue
		}
		left[i].st = left[len(left)-1].st
		for j := range left {
			if j > i && left[j].s != "" && left[j].s[0] != ' ' {
				left[i].st = left[j].st
				break
			}
		}
		if left[i].s == markPass+" " {
			left[i].st = passed
		}
	}
	turn := ""
	switch {
	case turns(r):
		turn = spinner[m.frame%len(spinner)]
	case stands(r):
		turn = steady
	}
	return m.rowTurning(turn, left, right, cursor)
}

// stated is what a row's state says at the right. The ordinary states
// say nothing: the margin says running and working, and idle is the
// absence of anything to say; a run that ended says how it did and when
// instead. What is left is what wants you, or is unusual: waiting,
// failed, outside conn.
func stated(state string) string {
	switch state {
	case "", "idle", "done", "running", "working":
		return ""
	}
	return state
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
// none. It is in the terminal's tab title too, and rings the bell as
// one more conversation starts waiting.
func sayWaiting(srv *tmux.Server, n int) {
	want := ""
	if n > 0 {
		want = strconv.Itoa(n)
	}
	was, _ := srv.Global(waitingMark)
	if was == want {
		return
	}
	before, _ := strconv.Atoi(was)
	set := tmux.UnsetGlobal(waitingMark)
	if want != "" {
		set = tmux.SetGlobal(waitingMark, want)
	}
	cmds := []tmux.Cmd{set}
	for _, c := range terminals(srv) {
		cmds = append(cmds, tmux.Cmd{"refresh-client", "-S", "-t", c})
	}
	_, _ = srv.Do(cmds...)
	// One bell for each question: as the count rises, never as it falls
	// or holds.
	if n > before {
		for _, c := range terminals(srv) {
			if f, err := os.OpenFile(c, os.O_WRONLY, 0); err == nil {
				_, _ = f.WriteString("\a")
				_ = f.Close()
			}
		}
	}
}

// key is what a row is, through a sort: its conversation, or its pane.
func (r row) key() string {
	if r.conv != nil {
		return r.conv.ID
	}
	// A named process is itself across its runs: run again, it is the
	// same row in a new pane, in its checkout.
	if r.pane != nil && r.pane.Name != "" {
		return "named " + r.tree + "\x00" + r.pane.Name
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

// drawOffer is the project's screen while u's, t's or c's question
// stands: the question, then Claude Code's answer and the two keys
// that take it.
func (m model) drawOffer(lines []string) ([]string, foot) {
	what, verb, ask := "runs", "start them", "Start these in "+m.cur.Name+"?"
	switch m.question {
	case roleCheck:
		what, verb, ask = "checks", "run them", "Run these checks in "+m.cur.Name+"?"
	case roleClient:
		what, verb, ask = "talks to it in", "open it", "Talk to it with this?"
	}
	if len(m.again) > 0 {
		ask = "Start these instead?"
		if m.question == roleCheck {
			ask = "Run these instead?"
		}
	}
	switch {
	case len(m.again) > 0 && m.offer == nil && m.offerErr == "":
		lines = append(lines, at(gray.Render(names(m.again)+" failed at once; asking Claude again…")))
		return lines, foot{hints: cancelHints}
	case m.offerErr != "":
		lines = append(lines, at(ink.Render("Claude would not say what "+what+" "+m.cur.Name+":")), at(gray.Render(m.offerErr)))
		return lines, foot{hints: backHints}
	case m.offer == nil:
		lines = append(lines, at(gray.Render("Nobody has done it here yet; asking Claude what "+what+" "+m.cur.Name+"…")))
		return lines, foot{hints: cancelHints}
	}
	lines = append(lines, at(title.Render(ask)), "")
	w := 0
	for _, s := range m.offer {
		w = max(w, len(s.name))
	}
	for _, s := range m.offer {
		lines = append(lines, m.row([]piece{{pad(s.name, w), keyInk}, {"  " + s.command, gray}}, piece{}, false))
	}
	return lines, foot{hints: []keyHint{{"enter", verb}, {"esc", "cancel"}}}
}

// drawSearch is the project's screen while /'s search stands: the line
// typed, how many lines say it, and the lines, each under the name of
// the row its pane is, the match in bold.
func (m model) drawSearch(lines []string) ([]string, foot) {
	count := ""
	switch {
	case m.said == nil:
		count = "reading…"
	case m.query != "" && len(m.found) == 1:
		count = "1 line"
	case m.query != "":
		count = fmt.Sprintf("%d lines", len(m.found))
	}
	lines = append(lines, m.row([]piece{{"/ ", gray}, {m.query, ink}, {caret, accent}}, piece{count, gray}, false), "")
	nameW := 0
	for _, h := range m.found {
		nameW = max(nameW, ansi.StringWidth(m.said[h.said].name))
	}
	nameW = min(nameW, 20)
	room := max(m.height-len(lines)-1, 1)
	first := max(m.hitAt-room+1, 0)
	for i := first; i < len(m.found) && i < first+room; i++ {
		h := m.found[i]
		name := pad(m.said[h.said].name, nameW) + "  "
		text, a := around(h.line, h.at, max(m.measure()-ansi.StringWidth(name), 8))
		a = min(a, len(text))
		end := min(a+len(m.query), len(text))
		left := []piece{{name, gray}, {text[:a], ink}, {text[a:end], keyInk}, {text[end:], ink}}
		lines = append(lines, m.row(left, piece{}, i == m.hitAt))
	}
	return lines, foot{hints: searchHints}
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

// objectKey is the key after a verb: the object the verb takes. The
// verb again is the row under the cursor, or for x the project whose
// line it is; s is the stack and t the checks, as they stand on the
// panel; f is what failed; p is the open project; c after r lists the conversations at
// rest to pick one back up. Anything else puts the verb away.
func (m model) objectKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	verb := m.verb
	m.verb = ""
	switch verb + k.String() {
	case "rr":
		if r, ok := m.under(); ok {
			if p, ok := r.proc(); ok {
				return m, m.restart(p)
			}
		}
	case "rc":
		m.listing, m.pick, m.pickAt = true, "", 0
	case "rs", "rt":
		// With none standing, the role is run as u and t run it.
		place, as, what := placeStack, roleStack, "the stack"
		if k.String() == "t" {
			place, as, what = placeChecks, roleCheck, "the checks"
		}
		if rows := m.inPlace(place); len(rows) > 0 {
			return m.restartEach(rows, what)
		}
		return m.resolve(as)
	case "rf":
		return m.restartEach(m.failedRows(), "what failed")
	case "xx":
		if r, ok := m.under(); ok {
			if p, ok := r.proc(); ok {
				return m, m.end(p)
			}
			return m, nil
		}
		if p, ok := m.lineUnder(); ok {
			return m, m.endAll(p)
		}
	case "aa":
		if r, ok := m.under(); ok {
			return m.hand(r)
		}
	case "af":
		return m.handAll(m.failedRows())
	case "ss":
		return m.resolve(roleStack)
	case "st":
		return m.resolve(roleCheck)
	case "sc":
		return m.converse("")
	case "xs":
		return m.endEach(m.inPlace(placeStack), "the stack")
	case "xt":
		return m.endEach(m.inPlace(placeChecks), "the checks")
	case "xp":
		return m, m.endAll(m.cur)
	case verb + "esc", verb + "ctrl+c":
	default:
		var said []string
		for _, h := range objectHints[verb] {
			if h.key != "esc" {
				said = append(said, h.key+", "+h.does)
			}
		}
		m.note = verb + " waits for " + strings.Join(said, "; ")
	}
	return m, nil
}

// inPlace is the open project's rows that stand in a place.
func (m model) inPlace(place int) []row {
	var out []row
	for _, r := range m.rows {
		if r.stand == place {
			out = append(out, r)
		}
	}
	return out
}

// endEach ends each row's process; see each.
func (m model) endEach(rows []row, what string) (tea.Model, tea.Cmd) {
	srv, hold := m.srv, m.ws.hold
	return m.each(rows, what, "ended", func(p proc) error {
		_, err := endProc(srv, hold, p)
		return err
	})
}

// restartEach starts each row's process again; see each.
func (m model) restartEach(rows []row, what string) (tea.Model, tea.Cmd) {
	srv, conn := m.srv, m.conn
	return m.each(rows, what, "restarted", func(p proc) error {
		_, err := restartProc(srv, conn, p)
		return err
	})
}

// each does a verb to each row's process, off the loop and one after
// another, and says how many it did; with none, it says there is
// nothing.
func (m model) each(rows []row, what, did string, act func(proc) error) (tea.Model, tea.Cmd) {
	var ps []proc
	for _, r := range rows {
		if p, ok := r.proc(); ok {
			ps = append(ps, p)
		}
	}
	if len(ps) == 0 {
		m.note = "Nothing of " + what + " stands here."
		return m, nil
	}
	return m, func() tea.Msg {
		var failed []string
		for _, p := range ps {
			if err := act(p); err != nil {
				failed = append(failed, err.Error())
			}
		}
		said := fmt.Sprintf("%s: %s %d", what, did, len(ps)-len(failed))
		if len(failed) > 0 {
			said += "; " + strings.Join(failed, "; ")
		}
		return acted(said)
	}
}

// failedRows is the open project's rows whose run failed.
func (m model) failedRows() []row {
	var out []row
	for _, r := range m.rows {
		if strings.HasPrefix(r.state, "failed ") {
			out = append(out, r)
		}
	}
	return out
}

// ring is a cursor moved by one over a list of n: past the last it
// is on the first, and before the first on the last, so a list is
// gone around from either end.
func ring(at, by, n int) int {
	if n <= 0 {
		return 0
	}
	return ((at+by)%n + n) % n
}

// counted is a word for n of a thing: the word alone for one, the
// figure before it and an s after it for more.
func counted(n int, word string) string {
	if n == 1 {
		return "one " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}

// shownRest is the conversations at rest the typed words find: each
// word in the title.
func (m model) shownRest() []row {
	words := strings.Fields(strings.ToLower(m.pick))
	if len(words) == 0 {
		return m.rest
	}
	var out []row
	for _, r := range m.rest {
		hay := strings.ToLower(or(r.conv.Title, r.conv.ID))
		found := true
		for _, w := range words {
			found = found && strings.Contains(hay, w)
		}
		if found {
			out = append(out, r)
		}
	}
	return out
}

// listKey is a key while rc's list stands: what is typed finds, the
// arrows move, enter picks a conversation back up in a window of its
// own, and esc puts the list away.
func (m model) listKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	shown := m.shownRest()
	switch k.String() {
	case "esc", "ctrl+c":
		m.listing = false
	case "enter":
		if m.pickAt < len(shown) {
			m.listing = false
			return m.goTo(shown[m.pickAt])
		}
	case "up", "ctrl+p", "ctrl+k":
		m.pickAt = ring(m.pickAt, -1, len(shown))
	case "down", "ctrl+n", "ctrl+j":
		m.pickAt = ring(m.pickAt, 1, len(shown))
	case "backspace":
		if r := []rune(m.pick); len(r) > 0 {
			m.pick, m.pickAt = string(r[:len(r)-1]), 0
		}
	case "ctrl+u":
		m.pick, m.pickAt = "", 0
	default:
		if t := k.Key().Text; t != "" {
			m.pick, m.pickAt = m.pick+t, 0
		}
	}
	return m, nil
}

// drawList is the project's screen while rc's list stands: the find
// line, then the conversations at rest the typed words find, newest
// first, each with how long ago it was.
func (m model) drawList(lines []string) ([]string, foot) {
	shown := m.shownRest()
	count := counted(len(shown), "conversation") + " at rest"
	lines = append(lines, m.row([]piece{{"find", gray}, {"  " + m.pick, ink}, {caret, accent}}, piece{count, gray}, false), "")
	room := max(m.height-len(lines)-1, 1)
	first := max(m.pickAt-room+1, 0)
	for i := first; i < len(shown) && i < first+room; i++ {
		r := shown[i]
		left := append(r.place(), piece{or(r.conv.Title, r.conv.ID), ink})
		lines = append(lines, m.row(left, piece{ago(r.conv.When), gray}, i == m.pickAt))
	}
	if len(shown) == 0 {
		lines = append(lines, at(faint.Render("nothing here")))
	}
	return lines, foot{hints: listHints}
}

// The panel is the open project on top — named at the head, its rows
// under — and under that a line for each other project with something
// running in it. The cursor runs over rows and project lines alike, so
// another project is j or k away and enter opens it, which puts it on
// top. The verbs that take a project take the open one; x on a
// project's line stops everything in it.

// An item on the panel: a row of the open project, or another
// project's line.
type item struct {
	project project
	row     int // the row's index, or -1 for the project's line
}

// items is the panel's lines in order: the open project's rows, and
// under them a line for each other project with something running or
// waiting in it, in the finder's order. A folder of repositories has
// no line of its own, what runs in it being its repositories', and a
// repository of the open folder none either, its rows being on top.
func (m model) items() []item {
	var out []item
	for j := range m.rows {
		out = append(out, item{project: m.cur, row: j})
	}
	for _, p := range m.projects {
		if p.Group || m.isOpen(p) || (m.cur.Group && within(p.Root, m.cur.Root)) || (p.Running == 0 && p.Waiting == 0 && p.Turning == 0) {
			continue
		}
		out = append(out, item{project: p, row: -1})
	}
	return out
}

// isOpen says whether a project is the open one.
func (m model) isOpen(p project) bool {
	return p.Root == m.cur.Root && p.Group == m.cur.Group
}

// itemKey is what an item is, through a read: the project's root, or
// its row's key.
func (m model) itemKey(it item) string {
	if it.row < 0 {
		return "project " + it.project.Root
	}
	if it.row < len(m.rows) {
		return m.rows[it.row].key()
	}
	return ""
}

// itemAt is the item a row of the open project is.
func (m model) itemAt(row int) int {
	for i, it := range m.items() {
		if it.row == row {
			return i
		}
	}
	return 0
}

// under is the row under the cursor, where the cursor is on one.
func (m model) under() (row, bool) {
	items := m.items()
	if m.cursor < len(items) && items[m.cursor].row >= 0 && items[m.cursor].row < len(m.rows) {
		return m.rows[items[m.cursor].row], true
	}
	return row{}, false
}

// lineUnder is the project whose line is under the cursor, where it is
// on one.
func (m model) lineUnder() (project, bool) {
	items := m.items()
	if m.cursor < len(items) && items[m.cursor].row < 0 {
		return items[m.cursor].project, true
	}
	return project{}, false
}

// label is a block's name: a small label in the gray, and a rule
// running off it to the right edge.
func (m model) label(s string) string {
	rule := strings.Repeat("─", max(m.measure()-ansi.StringWidth(s)-1, 0))
	return at(gray.Render(s) + " " + ruleStyle.Render(rule))
}

// projectLine is another project's line: its name, and what it says of
// itself at the right; in the parchment, as the head names a project,
// and in the accent where something waits. The folder it is in is not
// the project and is not said.
func (m model) projectLine(p project, cursor bool) string {
	name, style := p.Name, title
	if p.Waiting > 0 {
		style = accent
	}
	// The spinner turns in the margin while something in the project
	// is at work, as it does beside the row.
	turn := ""
	if p.Turning > 0 {
		turn = spinner[m.frame%len(spinner)]
	}
	return m.rowTurning(turn, []piece{{name, style}}, piece{p.standing(), gray}, cursor)
}
