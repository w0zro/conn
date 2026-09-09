package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// The finder is the front door: p at conn, the prefix's p from any buffer, and one
// list holds everything openable — the buffers held, a plan's entries,
// the containers, the conversations at rest, and every place, for a
// shell at its root. Enter
// attaches to what is running, starts what is defined, and, on a name that
// matches nothing, makes the repository and opens its shell: opening what
// does not exist creates it, like a new file.
//
// The finder is a popup, drawn by this build run as `conn page finder` over
// whichever client asked, and it reads what the navigator knows rather than
// scanning the machine for itself: the navigator writes its listing beside
// the socket whenever the listing changes (publish), and the popup opens
// on it at once. Acting is done through the server the way the chords act
// — a window named for wanting, a window made and wanted — and the
// navigator answers by showing what was asked for.

// finderEntry is one openable thing, as the navigator wrote it down.
type finderEntry struct {
	Kind  string    `json:"kind"`            // buffer, entry, container, resume, place, process
	Label string    `json:"label"`           // place/name, as the row reads
	Dir   string    `json:"dir"`             // where it runs, or would
	Pane  string    `json:"pane,omitempty"`  // a held buffer's pane
	Name  string    `json:"name,omitempty"`  // an entry's name
	Run   string    `json:"run,omitempty"`   // what starting it runs
	Held  bool      `json:"held,omitempty"`  // a process conn can step into
	Facts []segment `json:"facts,omitempty"` // what the row says after its name
}

// segment is a piece of a row's facts in its tone.
type segment struct {
	Text string `json:"text"`
	Tone tone   `json:"tone"`
}

// finderSnapshot is the listing as written: the entries, and the root a
// new project goes under. The kind of agent a starts is the server's to
// say, and the popup asks it.
type finderSnapshot struct {
	Entries []finderEntry `json:"entries"`
	Root    string        `json:"root,omitempty"`
}

// finderPath is where the listing is kept: beside the socket.
func finderPath() string {
	return filepath.Join(filepath.Dir(socketPath()), "finder.json")
}

// publish writes the finder's listing when it changed, off the render
// path. What it lists is what the navigator lists, in the navigator's
// order, and what the places define that is not running.
func (m *model) publish() tea.Cmd {
	snap := m.finderSnapshot()
	b, err := json.Marshal(snap)
	if err != nil || string(b) == m.snapshot {
		return nil
	}
	m.snapshot = string(b)
	return func() tea.Msg {
		path := finderPath()
		_ = os.MkdirAll(filepath.Dir(path), 0o700)
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, b, 0o600); err == nil {
			_ = os.Rename(tmp, path)
		}
		return nil
	}
}

// finderSnapshot is the listing: every run in every place as the everything
// view would list it with nothing folded or filtered — the held ones
// first, since they are the ones to step into — then each place's tasks
// and entries that are not running, its conversation at rest, and the
// place itself.
func (m model) finderSnapshot() finderSnapshot {
	whole := m
	whole.typing, whole.filter, whole.showAll, whole.collapsed, whole.unfolded = false, "", true, nil, false
	var held, others, defined []finderEntry
	for _, r := range whole.flatten() {
		switch r.kind {
		case rowProc:
			e := m.procEntry(r)
			if e.Held {
				held = append(held, e)
			} else {
				others = append(others, e)
			}
		case rowRest:
			c := r.rest
			defined = append(defined, finderEntry{Kind: "resume", Label: r.project.Name + " " + glyphDot + " resume",
				Dir: c.Dir, Run: resumeCommand(c),
				Facts: []segment{{`"` + cmp.Or(c.Prompt, c.Summary, c.ID) + `"`, toneQuiet},
					{shortAge(c.When), toneQuiet}, {c.Branch, toneQuiet}}})
		case rowProject, rowSub, rowGroup:
			if r.project.Path == globalPlace {
				continue
			}
			defined = append(defined, m.definedEntries(r.project)...)
			facts := []segment{{"shell", toneQuiet}, {"opens at the root", toneQuiet}}
			if r.kind == rowSub {
				facts = []segment{{"shell", toneQuiet}, {"opens in " + r.project.Name, toneQuiet}}
			}
			defined = append(defined, finderEntry{Kind: "place", Label: r.project.Name + "/terminal", Dir: r.project.Path, Facts: facts})
		}
	}
	snap := finderSnapshot{Entries: append(append(held, defined...), others...)}
	if len(m.roots) > 0 {
		snap.Root = m.roots[0]
	}
	return snap
}

// procEntry is a process row as the finder lists it: its place and name,
// its state's mark and last word, and whether conn holds it.
func (m model) procEntry(r navRow) finderEntry {
	place := r.project.Name
	e := finderEntry{Kind: "process", Label: place + "/" + m.rowName(r), Dir: r.node.Dir}
	if t := m.owningTerm(r.node.PID); t != nil {
		e.Kind, e.Held, e.Dir = "buffer", true, t.dir
		if m.server != nil {
			if p := m.server.pane(t.pid); p != nil {
				e.Pane = p.id
			}
		}
	}
	if c := r.node.Container; c != nil {
		e.Kind, e.Held, e.Run, e.Name = "container", false, containerLogs(r.node), c.Service
		if c.running() {
			e.Facts = append(e.Facts, segment{glyphContainer + " container", toneAccent})
			if len(r.node.Ports) > 0 {
				e.Facts = append(e.Facts, segment{":" + strings.Join(r.node.Ports, " :"), toneAccent})
			}
			if c.Ago != "" {
				e.Facts = append(e.Facts, segment{"up " + c.Ago, toneAccent})
			}
		} else {
			e.Facts = append(e.Facts, segment{glyphContainer + " " + cmp.Or(c.Status, "stopped"), toneQuiet})
		}
		return e
	}
	if a := m.agentFor(r); a != nil {
		mark, _ := m.agentMark(r, a)
		switch {
		case a.working():
			e.Facts = append(e.Facts, segment{mark + " working", toneAttn})
		case m.owed(r) != nil:
			t := toneGood
			word := "done, review when ready"
			if ask, ok := a.blocked(); ok {
				t, word = toneUrgent, "asks: "+ask
			}
			e.Facts = append(e.Facts, segment{mark + " running", t}, segment{word, t})
		default:
			e.Facts = append(e.Facts, segment{mark + " idle", toneQuiet})
		}
		return e
	}
	switch {
	case m.wrong(r):
		ending := m.ending(r)
		word := cmp.Or(ending.Summary, exitWord(ending.State), "stopped")
		e.Facts = append(e.Facts, segment{glyphFailed + " " + word, toneBad})
		if !ending.At.IsZero() {
			e.Facts = append(e.Facts, segment{shortAge(ending.At), toneBad})
		}
	case m.ended(r) == "0":
		ending := m.ending(r)
		e.Facts = append(e.Facts, segment{glyphDone + " " + cmp.Or(ending.Summary, "done"), toneGood})
		if !ending.At.IsZero() {
			e.Facts = append(e.Facts, segment{shortAge(ending.At), toneGood})
		}
	default:
		// Up and quiet: the hollow mark, in gray — the filled one is for
		// a result you have not looked at.
		word := "running"
		if !e.Held {
			word = "not conn's — look, don't step"
		}
		e.Facts = append(e.Facts, segment{glyphOff + " " + word, toneQuiet})
		if ps := runPorts(r.run, r.node); len(ps) > 0 {
			e.Facts = append(e.Facts, segment{":" + strings.Join(ps, " :"), toneAccent})
		}
	}
	if t := m.owningTerm(r.node.PID); t != nil && t.name != "" {
		if runs := m.history[t.pid]; len(runs) > 0 {
			e.Facts = append(e.Facts, runSegments(runs)...)
		}
	}
	return e
}

// definedEntries is what a place defines that is not running: its plan's
// entries.
func (m model) definedEntries(p Project) []finderEntry {
	var out []finderEntry
	states := m.entryStates(p.Path)
	// A name with a buffer already — running, or dead but readable — is
	// listed as that buffer: enter shows it, and r there runs it again.
	held := map[string]bool{}
	for _, t := range m.planned(p.Path) {
		held[t.name] = true
	}
	for _, en := range m.plans[p.Path].Entries {
		if held[en.Name] || states[en.Name].State == "up" {
			continue
		}
		out = append(out, finderEntry{Kind: "entry", Label: p.Name + "/" + en.Name, Dir: p.Path, Name: en.Name, Run: en.Run,
			Facts: []segment{{"plan", toneQuiet}, {en.Run, toneQuiet}}})
	}
	return out
}

// runSegments is a run history as segments: each run's mark and how long
// it took, in the mark's color, newest first.
func runSegments(runs []run) []segment {
	var out []segment
	for _, r := range runs {
		mark, t := glyphDone, toneGood
		if r.Exit != "0" {
			mark, t = glyphFailed, toneBad
		}
		if r.Took > 0 {
			mark += " " + shortTook(durationOf(r.Took))
		}
		out = append(out, segment{mark, t})
	}
	return out
}

// The popup.

// finderShare is the popup's share of the client's width.
const finderShare = 85

// showFinder shows the finder in a popup over the client, sized to its
// listing — most of the width, and as tall as the rows it will list, the
// query line and the foot — so it opens with no room to spare; "" is the
// client that spoke last.
func showFinder(run runner, exe, client string) error {
	rows := 12
	if snap, err := readFinder(); err == nil {
		rows = len(snap.Entries)
	}
	return popupShare(run, client, finderShare, rows+8, shellQuote(exe)+" page finder")
}

// popupShare runs a command in a popup over the client that is a share of
// the client's width and as many rows as asked, cut to the client.
func popupShare(run runner, client string, share, rows int, command string) error {
	if client == "" {
		var err error
		if client, err = latestClient(run); err != nil {
			return err
		}
	}
	width, height := 96, rows
	if out, err := run("display-message", "-p", "-c", client, "#{client_width} #{client_height}"); err == nil {
		if f := strings.Fields(out); len(f) == 2 {
			if cw, err := strconv.Atoi(f[0]); err == nil && cw > 0 {
				width = max(min(cw*share/100, cw), 20)
			}
			if ch, err := strconv.Atoi(f[1]); err == nil && ch > 0 {
				height = max(min(rows, ch-2), 8)
			}
		}
	}
	_, err := run("display-popup", "-E", "-c", client, "-T", "",
		"-w", strconv.Itoa(width), "-h", strconv.Itoa(height), command)
	return err
}

// finderModel is the popup: the query, the listing, what answers, and the
// cursor over it.
type finderModel struct {
	query   textinput.Model
	snap    finderSnapshot
	agent   string // the kind a starts, as the server holds it
	loaded  bool
	err     error
	cursor  int
	width   int
	height  int
	said    string // what the last action said, when it could not act
	saidErr bool
}

// finderReadMsg is the listing, read, and the kind of agent a starts.
type finderReadMsg struct {
	snap  finderSnapshot
	agent string
	err   error
}

func newFinderModel() finderModel {
	return finderModel{query: newLine(), width: 96, height: 24}
}

func (m finderModel) Init() tea.Cmd {
	return func() tea.Msg {
		snap, err := readFinder()
		return finderReadMsg{snap: snap, agent: currentKind(tmuxCommand).name, err: err}
	}
}

// readFinder reads the navigator's listing. Without one — the navigator
// not running, or nothing written yet — the listing is read the way conn
// ls reads it, which takes a moment.
func readFinder() (finderSnapshot, error) {
	b, err := os.ReadFile(finderPath())
	if err == nil {
		var snap finderSnapshot
		if err := json.Unmarshal(b, &snap); err == nil {
			return snap, nil
		}
	}
	m, err := readList()
	if err != nil {
		return finderSnapshot{}, err
	}
	m.plans = map[string]plan{}
	m.history, m.histories = map[int][]run{}, map[int]string{}
	for _, p := range m.projects {
		m.plans[p.Path] = readPlan(p.Path)
	}
	m.readHistories()
	return m.finderSnapshot(), nil
}

func (m finderModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case finderReadMsg:
		m.loaded, m.err, m.snap, m.agent = true, msg.err, msg.snap, msg.agent
	case tea.PasteMsg:
		m.query.SetValue(m.query.Value() + msg.Content)
		m.cursor = 0
	case tea.KeyPressMsg:
		return m.key(msg)
	}
	return m, nil
}

// key is a keystroke: enter acts on the row under the cursor — or on the
// query, when nothing answers it — up and down move, ctrl-a starts an
// agent where the row is, comma turns the kind, esc and ctrl-c leave, and
// every other key edits the query.
func (m finderModel) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		return m, tea.Quit
	case "enter":
		return m.act()
	case "up", "ctrl+p", "ctrl+k":
		m.move(-1)
		return m, nil
	case "down", "ctrl+n", "ctrl+j":
		m.move(1)
		return m, nil
	case "ctrl+a":
		return m.agentHere()
	case ",":
		k := nextKind(currentKind(tmuxCommand))
		if err := chooseKind(tmuxCommand, k); err != nil {
			m.said, m.saidErr = err.Error(), true
			return m, nil
		}
		m.agent = k.name
		m.said, m.saidErr = "a starts "+k.name, false
		return m, nil
	}
	before := m.query.Value()
	m.query, _ = m.query.Update(msg)
	if m.query.Value() != before {
		m.cursor, m.said = 0, ""
	}
	return m, nil
}

func (m *finderModel) move(by int) {
	n := len(m.matches())
	if n == 0 {
		return
	}
	m.cursor = (min(m.cursor, n-1) + by + n) % n
}

// matches is the entries that answer the query, by their label and what
// their facts say. An empty query is the whole listing.
func (m finderModel) matches() []finderEntry {
	q := strings.TrimSpace(m.query.Value())
	if q == "" {
		return m.snap.Entries
	}
	var out []finderEntry
	for _, e := range m.snap.Entries {
		if answers(q, e.Label) || answers(q, e.Label+" "+plainFacts(e)) {
			out = append(out, e)
		}
	}
	return out
}

// plainFacts is an entry's facts in one line, for matching.
func plainFacts(e finderEntry) string {
	parts := make([]string, 0, len(e.Facts))
	for _, s := range e.Facts {
		parts = append(parts, s.Text)
	}
	return strings.Join(parts, " ")
}

// act is enter: what the row under the cursor is opened as, or, with no
// row answering the query, the project the query names, made.
func (m finderModel) act() (tea.Model, tea.Cmd) {
	list := m.matches()
	if len(list) == 0 {
		return m.create()
	}
	e := list[min(m.cursor, len(list)-1)]
	if err := openEntry(tmuxCommand, e); err != nil {
		m.said, m.saidErr = err.Error(), true
		return m, nil
	}
	return m, tea.Quit
}

// create makes the project the query names, under the root, and opens its
// shell: the finder's answer to a name that matches nothing.
func (m finderModel) create() (tea.Model, tea.Cmd) {
	typed := strings.TrimSpace(m.query.Value())
	if typed == "" {
		return m, nil
	}
	if m.snap.Root == "" {
		m.said, m.saidErr = "no projects directory to make it in", true
		return m, nil
	}
	dir, err := newProjectPath(m.snap.Root, typed)
	if err == nil {
		err = createProject(dir)
	}
	if err == nil {
		err = runShellAt(dir, "")
	}
	if err != nil {
		m.said, m.saidErr = err.Error(), true
		return m, nil
	}
	return m, tea.Quit
}

// agentHere starts an agent where the row under the cursor is.
func (m finderModel) agentHere() (tea.Model, tea.Cmd) {
	list := m.matches()
	if len(list) == 0 {
		m.said, m.saidErr = "no place to start it in", false
		return m, nil
	}
	e := list[min(m.cursor, len(list)-1)]
	if e.Dir == "" || e.Dir == globalPlace {
		m.said, m.saidErr = "no place to start it in", false
		return m, nil
	}
	if err := runShellAt(e.Dir, startAgent(tmuxCommand)); err != nil {
		m.said, m.saidErr = err.Error(), true
		return m, nil
	}
	return m, tea.Quit
}

// openEntry opens one entry through the server: a held buffer is asked to
// be shown, a plan's entry starts in a buffer that is wanted — in the
// pane of its last run, when that run is over — a container's logs open
// as a buffer, a conversation at rest is picked back up, a place opens a
// shell at its root. A process conn does not hold is nowhere to go.
func openEntry(run runner, e finderEntry) error {
	switch e.Kind {
	case "buffer":
		return showHeld(run, e.Pane)
	case "entry":
		return startNamed(run, e.Dir, e.Name, e.Run)
	case "container", "resume":
		if e.Run == "" {
			return errors.New("no way to open " + e.Label)
		}
		_, err := createWindow(run, e.Dir, e.Run, e.Name, true)
		return err
	case "place":
		return runShellAt(e.Dir, "")
	}
	return errors.New(e.Label + " is not conn's to step into")
}

// showHeld asks the navigator to show a held buffer: its window is named
// for wanting, which the navigator answers by showing it. A buffer already
// under the tabline is in the home window, whose name is not for
// renaming: focus goes to it instead.
func showHeld(run runner, pane string) error {
	if pane == "" {
		return errors.New("the buffer has no pane to show")
	}
	out, err := run("display-message", "-p", "-t", pane, "#{@conn_home}\t#{window_id}")
	if err != nil {
		return err
	}
	home, win, _ := strings.Cut(strings.TrimSpace(out), "\t")
	if home == "1" {
		_, err = run("select-window", "-t", win, ";", "select-pane", "-t", pane)
		return err
	}
	_, err = run("rename-window", "-t", pane, wantName)
	return err
}

// startNamed starts a place's entry by name, wanted: in the pane
// of its last run when that run is over — the buffer, run again in place
// — and in a new window otherwise. A run still going is shown rather than
// started again beside itself.
func startNamed(run runner, dir, name, command string) error {
	out, err := run("list-panes", "-a", "-F", listFormat)
	if err != nil && !errors.Is(err, errNoServer) {
		return err
	}
	held, _ := parseListing(out)
	procs, perr := runningProcs()
	busy := busyHeld(held, procs, perr)
	for _, p := range held {
		if p.name != name || p.dir != dir {
			continue
		}
		if busy[p.pid] {
			return showHeld(run, p.id)
		}
		_, err := run("set", "-pu", "-t", p.id, "@conn_exit", ";", "set", "-pu", "-t", p.id, "@conn_ended", ";",
			"set", "-pu", "-t", p.id, "@conn_summary", ";", "set", "-pu", "-t", p.id, "@conn_recorded", ";",
			"set", "-p", "-t", p.id, "@conn_run", command, ";",
			"respawn-pane", "-k", "-t", p.id, command+recordExit()+`; exec "$SHELL"`)
		if err != nil {
			return err
		}
		return showHeld(run, p.id)
	}
	_, err = createWindow(run, dir, command, name, true)
	return err
}

func (m finderModel) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

// render draws the page: the query line with its prompt and block cursor
// over a rule, the rows that answer with the cursor's on a bar and the
// matched letters lit, a blank, and at the foot what enter does with no
// match and how agents start. tmux draws the border around it.
func (m finderModel) render() string {
	inside := m.width
	wash := lipgloss.NewStyle().Background(lipgloss.Color(colorWash))
	line := func(s string) string { return wash.Render(pad(truncateStyled(s, inside, false), inside)) }
	q := m.query.Value()
	query := gutter + wash.Inherit(orangeStyle).Bold(true).Render(glyphJoin) + " " + wash.Inherit(itemStyle).Render(q) + cursorStyle.Render(" ")
	box := []string{line(query), wash.Inherit(ruleStyle).Render(strings.Repeat("─", inside)), line("")}
	switch {
	case !m.loaded:
		box = append(box, line(gutter+noteStyle.Render("looking…")))
	case m.err != nil:
		box = append(box, line(gutter+errStyle.Render(m.err.Error())))
	default:
		box = append(box, m.rows(inside)...)
	}
	box = append(box, line(""))
	for _, f := range m.foot() {
		box = append(box, line(gutter+faintStyle.Render(f)))
	}
	for len(box) < m.height {
		box = append(box, line(""))
	}
	return strings.Join(box[:min(len(box), m.height)], "\n")
}

// rows is the listing that answers, the cursor's row on the chip's bar
// with the orange marker, the labels in one column and the facts in the
// next, as many as the window has rows for.
func (m finderModel) rows(inside int) []string {
	wash := lipgloss.NewStyle().Background(lipgloss.Color(colorWash))
	list := m.matches()
	if len(list) == 0 {
		q := strings.TrimSpace(m.query.Value())
		if q == "" {
			return []string{wash.Render(pad(gutter+noteStyle.Render("nothing to open yet"), inside))}
		}
		return []string{wash.Render(pad(gutter+noteStyle.Render("nothing answers "+q+" — enter makes it"), inside))}
	}
	labelW := 0
	for _, e := range list {
		labelW = max(labelW, lipgloss.Width(e.Label))
	}
	labelW = min(labelW, max(inside/3, 16))
	size := max(1, m.height-7)
	cursor := min(m.cursor, len(list)-1)
	top := max(0, min(cursor-size+1, len(list)-size))
	q := strings.TrimSpace(m.query.Value())
	var out []string
	for i := top; i < len(list) && i < top+size; i++ {
		e := list[i]
		selected := i == cursor
		bg := wash
		marker := " "
		if selected {
			bg = chipStyle
			marker = glyphSelected
		}
		labelStyle := bg.Inherit(itemStyle)
		if selected {
			labelStyle = labelStyle.Bold(true)
		}
		if e.Kind == "process" && !e.Held {
			labelStyle = bg.Inherit(faintStyle)
		}
		label := labelStyle.Render(truncateTail(e.Label, labelW))
		if q != "" {
			label = truncateStyled(highlightOn(e.Label, matchSpans(q, e.Label), labelStyle, bg.Inherit(matchStyle)), labelW, false)
		}
		row := bg.Render(gutter) + bg.Inherit(selStyle).Render(marker) + bg.Render(" ") + label +
			bg.Render(strings.Repeat(" ", max(labelW-lipgloss.Width(e.Label), 0)+4))
		room := inside - lipgloss.Width(row)
		parts := make([]string, 0, len(e.Facts))
		for _, s := range e.Facts {
			if s.Text != "" {
				parts = append(parts, bg.Inherit(toneStyles[s.Tone]).Render(s.Text))
			}
		}
		facts := strings.Join(parts, bg.Inherit(hintStyle).Render(" "+glyphDot+" "))
		row += truncateStyled(facts, room, false)
		out = append(out, bg.Render(pad(row, inside)))
	}
	return out
}

// foot is the two faint lines at the popup's foot: what enter does with no
// match, and how agents start.
func (m finderModel) foot() []string {
	if m.said != "" {
		style := hintStyle
		if m.saidErr {
			style = errStyle
		}
		return []string{style.Render(m.said), ""}
	}
	name := strings.TrimSpace(m.query.Value())
	if name == "" {
		name = "w0zro/parser"
	}
	return []string{
		"no match? enter on \"" + name + "\" makes the repo and opens its",
		"shell " + glyphDot + " agents: ^a a " + cmp.Or(m.agent, defaultKind().name) + " here " + glyphDot + " , the next kind",
	}
}

// runFinder is `conn page finder`: the finder, in the popup.
func runFinder() {
	if _, err := tea.NewProgram(newFinderModel()).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "conn: %v\n", err)
		os.Exit(1)
	}
}
