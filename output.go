package main

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/w0zro/conn/internal/work"

	"github.com/w0zro/conn/internal/tmux"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// The output view: what a project's processes have written, searched.
// / on a row opens it over the row's project, and from inside a
// process the panel key then / opens it over the project of the pane
// the keys were in. It is a line typed into, like the list: what is
// typed is looked for in the scrollback of every pane conn holds for
// the project, and the lines that say it are the rows, under the row
// each pane is, newest first within it, since the last thing a process
// said is usually the thing being looked for.
//
// The bay follows the cursor, the way the page does: the match's pane
// is put in the bay in tmux's own copy mode, the line in the middle
// with its context either side, every saying of the text marked and
// the one under the cursor in the accent, and the keys stay on the
// panel. Enter puts the keys in, and from there n and N go on to the
// next saying and q leaves, as in any pane. Esc goes back where the
// keys were, and the page takes the bay again. The output is the
// panes' own and is read with capture-pane; nothing of it is kept, and
// the view keeps its text while it stays over one project, so the panel
// key then / from a pane just landed in is the list where it was left.
//
// Rows conn only reports have no pane, and nothing of theirs is here:
// what a process wrote to a terminal conn does not hold is not conn's
// to read. The search is tmux's search: the text as typed, and case
// ignored while it is typed in the lower case, which is how tmux and
// vim both take a search, so the saying the view counts is the saying
// tmux lands on.

// An outPane is one pane of the project, as read for the view: the row
// it is on the panel, and what it holds.
type outPane struct {
	tty, id string
	pid     int
	label   string   // what the panel calls the pane's row
	lines   []string // its scrollback, oldest first; see tmux.Scrollback
	history int      // how many of them are history, the rest the screen
}

// outMsg is a project's panes read, for the view that asked.
type outMsg struct {
	project string
	panes   []outPane
}

// The output view as the panel holds it: the project it is over, its
// panes as last read, and the line typed into with the cursor among
// the matches it leaves.
type outList struct {
	project string
	panes   []outPane
	loading bool
	find    typed
	// The match the bay was last asked to show, so moving to the same
	// one again asks nothing, and the pane it was shown in, which is
	// taken out of copy mode when the view is left without going in.
	shown    outShown
	previews int // which asking of the bay the next tick belongs to
}

// An outShown is a match as the bay shows it: the pane, and the saying
// of the text in it counted back from the end.
type outShown struct {
	tty  string
	k    int
	text string
}

// An outMatch is a line that says the text: which pane, which of its
// lines, and the line.
type outMatch struct {
	pane, line int
	text       string
	at         int // where in the line the text begins, in bytes
	// Which saying of the text the line's first one is, counted back
	// from the pane's end: what the pane is told to land on, which
	// holds whatever width it is wrapped to; see tmux.Land.
	k int
}

// linesSaying is the lines of the panes that say the text, the panes in
// their order and each pane's lines newest first. An empty line of
// text matches nothing: the view is for finding, and every line is
// not a finding.
func linesSaying(text string, panes []outPane) []outMatch {
	if text == "" {
		return nil
	}
	lower := strings.ToLower(text) == text
	var out []outMatch
	for i, p := range panes {
		k := 0
		for j := len(p.lines) - 1; j >= 0; j-- {
			at := indexFold(p.lines[j], text, lower)
			if at < 0 {
				continue
			}
			// A line that says the text more than once is one row, on
			// its first saying, which is the last one a search back from
			// the end reaches.
			n := sayings(p.lines[j], text, lower)
			out = append(out, outMatch{pane: i, line: j, text: p.lines[j], at: at, k: k + n - 1})
			k += n
		}
	}
	return out
}

// sayings is how many times a line says the text, counted as tmux's
// search counts them: every place it begins, overlapping or not.
func sayings(line, text string, fold bool) int {
	if fold {
		line = strings.ToLower(line)
	}
	n := 0
	for i := 0; i+len(text) <= len(line); i++ {
		if line[i:i+len(text)] == text {
			n++
		}
	}
	return n
}

// indexFold is where text begins in line, or -1: as typed, or with
// case ignored where the text has no capital in it, which is how tmux
// takes a search and how the match it lands on will have been found.
func indexFold(line, text string, fold bool) int {
	if !fold {
		return strings.Index(line, text)
	}
	return strings.Index(strings.ToLower(line), text)
}

// written is how many lines a pane has said: its scrollback less the
// blank rows at the foot of the screen, which are the screen's and
// not the process's.
func written(lines []string) int {
	n := len(lines)
	for n > 0 && strings.TrimSpace(lines[n-1]) == "" {
		n--
	}
	return n
}

// heldPanes is the panes conn holds for a project's rows, each once
// and in the panel's order: a pane holds a tree of processes, and the
// pane is read once for all of them.
func heldPanes(projects []work.Project, panes map[string]tmux.Pane, project string) []outPane {
	var out []outPane
	seen := map[string]bool{}
	for _, pl := range projects {
		if pl.Path != project {
			continue
		}
		for _, e := range pl.Entries {
			p, ok := panes[e.TTY]
			if !ok || p.ID == "" || seen[e.TTY] {
				continue
			}
			seen[e.TTY] = true
			out = append(out, outPane{tty: e.TTY, id: p.ID, pid: e.PID, label: rowLabel(e)})
		}
	}
	return out
}

// openOutput puts the output view up over a project, and reads its
// panes. came is the pane the keys were in, for esc to go back to.
func (m model) openOutput(project, came string) (model, tea.Cmd) {
	if !m.inside {
		return m, nil
	}
	m.from = came
	m.view = viewOutput
	// Opened again over the project it was last over, the view is
	// where it was left, text and cursor: a pane just landed in is a
	// list to come back to. Another project is a fresh line.
	if m.out.project != project {
		m.out = outList{project: project}
		m.out.find.clear()
	}
	m.out.loading, m.out.shown = true, outShown{}
	m.processesGen++
	return m, tea.Batch(m.readProcesses(), m.captureOutput())
}

// captureOutput reads the scrollback of the project's panes, off the
// loop, and passes it back for the view. A pane that could not be
// read is passed back with nothing in it rather than left out, so the
// rows it stands for keep their place.
func (m model) captureOutput() tea.Cmd {
	srv, project := m.srv, m.out.project
	panes := heldPanes(m.projects, m.panes, project)
	if srv == nil || len(panes) == 0 {
		return func() tea.Msg { return outMsg{project: project, panes: panes} }
	}
	return func() tea.Msg {
		for i := range panes {
			panes[i].lines, panes[i].history, _ = srv.Scrollback(panes[i].id)
		}
		return outMsg{project: project, panes: panes}
	}
}

// landedOutput takes the panes as read, where they are the view's: one
// opened on another project since has moved past the answer.
func (m model) landedOutput(msg outMsg) (model, tea.Cmd) {
	if m.view != viewOutput || msg.project != m.out.project {
		return m, nil
	}
	m.out.panes, m.out.loading = msg.panes, false
	m.out.find.at = clamp(m.out.find.at, len(m.out.matches()))
	return m.previewing()
}

// previewDelay is how long the view waits after the cursor moves
// before the bay is asked to follow: a word typed is five moves, and
// the bay follows the word rather than each letter of it.
const previewDelay = 120 * time.Millisecond

// previewing has the bay follow the cursor, after a moment. It is asked
// on every move of the cursor and every reading, and asks the bay for
// the match under the cursor once the moves have stopped.
func (m model) previewing() (model, tea.Cmd) {
	if !m.inside || m.view != viewOutput {
		return m, nil
	}
	m.out.previews++
	gen := m.out.previews
	return m, tea.Tick(previewDelay, func(time.Time) tea.Msg { return previewTickMsg{gen} })
}

// preview is the bay asked to show the match under the cursor, where
// it is not showing it already: the pane into the bay, keys staying on
// the panel, and the pane in copy mode on the saying. A pane it was
// showing before is taken out of copy mode on its way out.
func (m model) preview() (model, tea.Cmd) {
	hit, pane, ok := m.out.outAt()
	if !ok {
		return m, nil
	}
	want := outShown{tty: pane.tty, k: hit.k, text: m.out.find.text}
	target, held := m.panes[pane.tty]
	if want == m.out.shown || !held {
		return m, nil
	}
	was := m.out.shown
	m.out.shown = want
	m.bay.preview = pane.tty
	srv, before := m.srv, m.panes[was.tty].ID
	return m, func() tea.Msg {
		if before != "" && before != target.ID {
			srv.Unmode(before)
		}
		if srv.Preview(target) != nil {
			return nil
		}
		_ = srv.Land(target.ID, want.k, want.text)
		return nil
	}
}

// leaveOutput is the view left without going in: the pane the bay was
// showing is taken out of copy mode, since the keys never went into
// it, and the bay is the page's to take again.
func (m model) leaveOutput() (model, tea.Cmd) {
	var cmd tea.Cmd
	if id := m.panes[m.out.shown.tty].ID; id != "" && m.srv != nil {
		srv := m.srv
		cmd = func() tea.Msg { srv.Unmode(id); return nil }
	}
	m.out.shown, m.bay.preview = outShown{}, ""
	return m, cmd
}

// matches is the lines the typed text leaves, which the cursor is an
// index into.
func (l outList) matches() []outMatch {
	return linesSaying(l.find.text, l.panes)
}

// outAt is the match under the cursor and its pane, where there is one.
func (l outList) outAt() (outMatch, outPane, bool) {
	rows := l.matches()
	if l.find.at >= len(rows) {
		return outMatch{}, outPane{}, false
	}
	hit := rows[l.find.at]
	return hit, l.panes[hit.pane], true
}

// outputKey answers a key on the output view, which is a line typed
// into; see typed. What is the view's own: enter goes into the match's
// pane in copy mode with the cursor on it, esc goes back, and ctrl+c
// is what it is everywhere.
func (m model) outputKey(k string) (model, tea.Cmd) {
	switch {
	case m.out.find.edit(k, len(m.out.matches())):
		return m.previewing()
	case k == "ctrl+c":
		return m.leave()
	case k == "esc":
		// The pane the bay was showing is let go of on the way out,
		// which toProcesses does for every way out of the view.
		return m.backFrom()
	case k == "enter":
		if hit, pane, ok := m.out.outAt(); m.inside && ok {
			target, ok := m.panes[pane.tty]
			if !ok {
				return m, nil
			}
			m.from = ""
			m.out.shown, m.bay.preview = outShown{}, ""
			m = m.onRow(pane.pid, m.cursorAt)
			var cmd tea.Cmd
			m, cmd = m.toProcesses()
			return m, tea.Batch(cmd, m.landIn(target, pane.tty, hit.k, m.out.find.text))
		}
	}
	return m, nil
}

// landIn puts a pane in the bay with the keys in it, as reach does, in
// copy mode on the saying; see tmux.Land. The bay may be showing it
// already, in which case the landing is the same one again and only
// the keys move.
func (m model) landIn(target tmux.Pane, tty string, k int, text string) tea.Cmd {
	srv := m.srv
	return func() tea.Msg {
		if srv.Show(target) != nil {
			return nil
		}
		_ = srv.Land(target.ID, k, text)
		return reachedMsg{tty}
	}
}

// outputReport is the output view's words as things stand.
type outputReport struct {
	project string // as the panel names its block
	filter  string
	caret   int
	loading bool
	lines   int // how many lines the panes have said, in all
	panes   []outPaneReport
	rows    []outMatch
}

// An outPaneReport is a pane as the view heads its block: the row's
// label and how many lines it has said, or how many say the text.
type outPaneReport struct {
	label string
	lines int
	hits  int
}

// composeOutput words the output view.
func composeOutput(l outList, project string) outputReport {
	b := outputReport{project: project, filter: l.find.text, caret: l.find.cur, loading: l.loading, rows: l.matches()}
	for _, p := range l.panes {
		n := written(p.lines)
		b.lines += n
		b.panes = append(b.panes, outPaneReport{label: p.label, lines: n})
	}
	for _, r := range b.rows {
		b.panes[r.pane].hits++
	}
	return b
}

// outputReport is the output view's words as things stand.
func (m model) outputReport() outputReport {
	return composeOutput(m.out, projectName(m.out.project, m.roots.real, m.head.Login.Home))
}

// around is a line fitted to the room with the match in view: the
// whole line where it fits, and otherwise the part around the match,
// a cut start marked. It answers where the match now begins.
func around(text string, at, room int) (string, int) {
	text = strings.TrimRight(text, " ")
	at = min(max(at, 0), len(text))
	r := []rune(text)
	if len(r) <= room {
		return text, at
	}
	lead := utf8.RuneCountInString(text[:at])
	if lead+8 <= room {
		return Fit(text, room, false), at
	}
	// The start is cut so the match sits a few cells in, after the mark.
	kept := string(r[lead-8:])
	return Fit("…"+kept, room, false), len("…") + len(string(r[lead-8:lead]))
}

// plural is a figure and its noun: 1 LINE, 2 LINES.
func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

// drawOutput renders the output view for a terminal of the given size,
// with the cursor on the given match.
func drawOutput(b outputReport, cursor, width, height int, p Palette) []Row {
	// The header: the name of the view, and against the right how many
	// lines say the text, or how many lines there are to say it.
	right := plural(b.lines, "LINE", "LINES")
	switch {
	case b.loading && b.lines == 0:
		right = ""
	case b.filter != "":
		right = plural(len(b.rows), "MATCH", "MATCHES")
	}
	c, measure := Head("OUTPUT", right, width, p)

	// The project it is over, the way a project titles its block.
	l := c.Line()
	l.Add(p.Parchment+p.Bold, Fit(b.project, measure, true))
	c.Emit(l, 0, false)

	// The line typed into.
	l = c.Line()
	before, after := TypedRuns(b.filter, b.caret, measure-findW-2, false)
	l.Field(0, measure-findW, "FIND", before, after)
	c.Emit(l, 0, false)

	var body []Row
	cursorRow := -1
	d := Canvas{P: p, Width: c.Width}
	say := func(color, s string) {
		d.Blank(0)
		l := d.Line()
		l.Add(color, s)
		d.Emit(l, 0, true)
		body = d.Rows
	}
	switch {
	case b.loading && len(b.panes) == 0:
		say(p.Gray, "READING")
	case len(b.panes) == 0:
		say(p.Gray, "NOTHING HELD HERE")
	case b.filter == "":
		// Nothing typed yet: the panes that will be searched, each with
		// what it has said, so the scope is plain before the text is.
		for _, pane := range b.panes {
			d.Blank(0)
			l := d.Line()
			count := plural(pane.lines, "LINE", "LINES")
			l.Eyebrow(0, Fit(pane.label, measure-ansi.StringWidth(count)-2, false), measure, count)
			d.Emit(l, 0, false)
		}
		body = d.Rows
	case len(b.rows) == 0:
		say(p.Gray, "NOTHING SAYS "+strings.ToUpper(b.filter))
	default:
		last := -1
		for i, r := range b.rows {
			if r.pane != last {
				last = r.pane
				pane := b.panes[r.pane]
				d.Blank(0)
				l := d.Line()
				count := plural(pane.hits, "MATCH", "MATCHES")
				l.Eyebrow(0, Fit(pane.label, measure-ansi.StringWidth(count)-2, false), measure, count)
				d.Emit(l, 0, false)
			}
			l := d.Line()
			if i == cursor {
				l.P = p.Chosen()
				l.Mark = CursorBar
				if p.Plain {
					l.Mark = "▸"
				}
				cursorRow = len(d.Rows)
			}
			lead := len(r.text) - len(strings.TrimLeft(r.text, " "))
			text, at := around(r.text[lead:], r.at-lead, measure)
			end := min(at+len(b.filter), len(text))
			for end < len(text) && !utf8.RuneStart(text[end]) {
				end--
			}
			l.Add(p.Gray, text[:at])
			l.Add(p.Ink+p.Bold, text[at:end])
			l.Add(p.Gray, text[end:])
			d.Emit(l, 0, false)
		}
		body = d.Rows
	}
	return c.Foot(body, cursorRow, height)
}
