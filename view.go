package main

import (
	"cmp"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// agentMark is the glyph beside an agent: a working one turns, one stopped
// mid-turn on a specific ask holds a bright diamond, one that has finished a
// turn nobody has looked at holds a filled marker in the attention color,
// and one idle — since it started, or since its result was seen — sits
// hollow and quiet. The diamond is the one worth crossing the room for:
// that answer resumes work already in flight; the filled mark is for what
// changed while you were elsewhere.
func (m model) agentMark(r navRow, a agent) (string, lipgloss.Style) {
	if _, ok := a.blocked(); ok {
		return glyphAsk, blockedStyle
	}
	switch {
	case a.working():
		return spinFrames[m.frame%len(spinFrames)], busyStyle
	case m.owed(r) != nil:
		return glyphOn, attnStyle
	}
	return glyphOff, hintStyle
}

// View lays the window out as the tabline over a body.
//
// conn draws in a pane of its own across the top of the home window, and
// the buffer with focus is the tmux pane beneath it: when a buffer is shown
// this view is exactly the chrome — the tabline, and the buffer's heading —
// as tall as its pane. With no buffer to show conn has the whole window, and
// the body is its own: the everything view, a kill preview, or the empty
// workspace. conn's name and its keys are on tmux's status line at the
// foot, not in a header of the column's own.
func (m model) View() tea.View {
	v := tea.NewView(m.layout())
	v.AltScreen = true
	// Told when focus leaves for a buffer and comes back, so the cursor
	// can say whose the next letter is.
	v.ReportFocus = true
	return v
}

func (m model) layout() string {
	rows := max(m.height, 1)
	lines := []string{m.edgeRow(), m.tabline()}
	if m.shown != 0 {
		lines = append(lines, "")
	} else {
		lines = append(lines, m.body(max(rows-2, 0))...)
	}
	lines = padTo(lines, rows)
	for i := range lines {
		// Every line is cut and painted to the width on the ground, and
		// ends reset, so nothing a row set can outlive it and nothing
		// wraps into the row below.
		lines[i] = groundStyle.Render(pad(truncateStyled(lines[i], m.width, false), m.width)) + ansi.ResetStyle
	}
	return strings.Join(lines, "\n")
}

// padTo lengthens lines to exactly n.
func padTo(lines []string, n int) []string {
	n = max(n, 0)
	for len(lines) < n {
		lines = append(lines, "")
	}
	return lines[:n]
}

// tab is one entry of the tabline: a buffer, or the everything view.
type tab struct {
	pid       int    // the held shell; zero for the everything view
	label     string // project/name
	qualifier string // what tells it from another tab of the same name, in gray: a branch, an ordinal
	mark      string // its state's glyph, or nothing
	style     lipgloss.Style
	focused   bool
}

// tabs is the working set: the everything view while it is up, then every
// held shell in the navigator's order. A tab is named place/name and marked
// with its state — an ask, a spinner, done-and-waiting, ended well or
// badly, a container — in the state's color, and nothing more: the facts
// live in the heading. Two buffers that would read the same are told
// apart by the shortest fact that differs — the branch, when both are
// known and differ — else by an ordinal, only while the collision lasts.
func (m model) tabs() []tab {
	var out []tab
	if m.viewingAll() {
		out = append(out, tab{label: "everything", focused: true})
	}
	for _, pid := range m.heldOrder() {
		t := m.terms[pid]
		if t == nil {
			continue
		}
		tb := tab{pid: pid, label: m.tabLabel(pid, t), focused: pid == m.shown || (m.shown == 0 && pid == m.from)}
		tb.mark, tb.style = m.tabMark(pid, t)
		out = append(out, tb)
	}
	byLabel := map[string][]int{}
	for i, tb := range out {
		if tb.pid != 0 {
			byLabel[tb.label] = append(byLabel[tb.label], i)
		}
	}
	for _, same := range byLabel {
		if len(same) < 2 {
			continue
		}
		branches := map[string]bool{}
		for _, i := range same {
			branches[m.deep[out[i].pid].Branch] = true
		}
		byBranch := len(branches) == len(same) && !branches[""]
		for n, i := range same {
			if byBranch {
				out[i].qualifier = m.deep[out[i].pid].Branch
			} else {
				out[i].qualifier = glyphDot + strconv.Itoa(n+1)
			}
		}
	}
	return out
}

// tabLabel is what a buffer's tab says: the place it works in and the name
// its row would show — the plan's name for a shell it started, what an
// agent is called, else what runs there — with no fact after it: a tab is
// a name and a mark, and the heading is where the facts are.
func (m model) tabLabel(pid int, t *remoteTerm) string {
	name := t.name
	if n := m.nodes[pid]; n != nil {
		run := runFrom(n, len(m.procs))
		r := navRow{kind: rowProc, run: run, node: nameOf(run)}
		if k, ok := agentKindOf(r.node); ok {
			name = agentLabel(k, m.agentNameOf(r.node), "")
		} else if name == "" {
			name = m.rowName(r)
		}
	}
	if name == "" {
		name = "shell"
	}
	if p, ok := m.placeAt(t.dir); ok {
		return p.Name + "/" + name
	}
	return name
}

// tabMark is a buffer's mark and its color: what its agent is doing, else
// how its run stands, else nothing.
func (m model) tabMark(pid int, t *remoteTerm) (string, lipgloss.Style) {
	n := m.nodes[pid]
	if n == nil {
		return "", faintStyle
	}
	run := runFrom(n, len(m.procs))
	r := navRow{kind: rowProc, run: run, node: nameOf(run)}
	if a := m.agentFor(r); a != nil {
		return m.agentMark(r, a)
	}
	if strings.HasPrefix(t.run, "docker") && strings.Contains(t.run, "logs") {
		return glyphContainer, tealStyle
	}
	switch {
	case m.signalled(r):
		return spinFrames[m.frame%len(spinFrames)], errStyle
	case m.wrong(r):
		return glyphFailed, errStyle
	case m.ended(r) == "0":
		return glyphDone, toneStyles[toneGood]
	}
	return "", faintStyle
}

// tabCell is one tab drawn: its text, how wide it is, and whether it is
// the focused one.
type tabCell struct {
	text       string
	width      int
	focused    bool
	everything bool // the everything view's tab, whose edge is the teal
}

// tabCells draws the tabs and says where the row starts: from the first
// tab, unless the focused one would fall off the end — then from as far
// along as keeps it in view. The focused tab is bold ink on the ground;
// the rest are gray on the bar; a mark sits after the name in its color,
// and a qualifier in gray between them.
func (m model) tabCells() (cells []tabCell, start int) {
	hint := m.tabHint()
	room := m.width - lipgloss.Width(hint) - 2
	focused := -1
	for i, t := range m.tabs() {
		bg, style := barStyle, hintStyle
		if t.focused {
			bg, style = groundStyle, itemStyle
			focused = i
		}
		text := " " + t.label
		cell := bg.Inherit(style).Bold(t.focused).Render(text)
		if t.qualifier != "" {
			cell += bg.Inherit(hintStyle).Render(" " + t.qualifier)
			text += " " + t.qualifier
		}
		if t.mark != "" {
			cell += bg.Render(" ") + bg.Inherit(t.style).Render(t.mark)
			text += " " + t.mark
		}
		cell += bg.Render(" ")
		cells = append(cells, tabCell{text: cell, width: lipgloss.Width(text) + 1, focused: t.focused, everything: t.pid == 0})
	}
	if focused >= 0 {
		total := 0
		for i := focused; i >= 0; i-- {
			total += cells[i].width
			if total > room {
				start = i + 1
				break
			}
		}
	}
	return cells, start
}

// tabline is the top row: the working set across the bar, and at the
// right end one hint at most.
func (m model) tabline() string {
	hint := m.tabHint()
	room := m.width - lipgloss.Width(hint) - 2
	cells, start := m.tabCells()
	var b strings.Builder
	used := 0
	for i := start; i < len(cells); i++ {
		if used+cells[i].width > room {
			break
		}
		b.WriteString(cells[i].text)
		used += cells[i].width
	}
	rest := max(m.width-used-lipgloss.Width(hint)-1, 0)
	return b.String() + barStyle.Render(strings.Repeat(" ", rest)) + barStyle.Inherit(faintStyle).Render(hint) + barStyle.Render(" ")
}

// edgeRow is the row over the tabline: the bar across, one dark with the
// terminal's margin over it, and under the focused tab's columns the
// tab's own ground with the edge along its top — the tab stands two rows
// tall, the focus signal its rim: orange for a buffer, teal for the
// everything view, which is where the identities are read.
func (m model) edgeRow() string {
	cells, start := m.tabCells()
	hint := m.tabHint()
	room := m.width - lipgloss.Width(hint) - 2
	used := 0
	for i := start; i < len(cells); i++ {
		if used+cells[i].width > room {
			break
		}
		if cells[i].focused {
			edge := orangeStyle
			if cells[i].everything {
				edge = tealStyle
			}
			return barStyle.Render(strings.Repeat(" ", used)) +
				groundStyle.Inherit(edge).Render(strings.Repeat(glyphEdge, cells[i].width)) +
				barStyle.Render(strings.Repeat(" ", max(m.width-used-cells[i].width, 0)))
		}
		used += cells[i].width
	}
	return barStyle.Render(strings.Repeat(" ", m.width))
}

// tabHint is the one hint the tabline's right end holds: the key that
// matters most now.
func (m model) tabHint() string {
	switch {
	case m.pendingKill != nil:
		return "esc keeps it"
	case m.shown != 0:
		if t := m.terms[m.shown]; t != nil && !t.live() && t.name != "" {
			return "r reruns"
		}
		return "⌃spc p opens"
	case m.viewingAll():
		return ". toggles running · all"
	}
	return "p opens"
}

// viewingAll reports the everything view on screen: put up, or standing
// in for a buffer while none is shown — with nothing to show, what is
// running is the thing to look at.
func (m model) viewingAll() bool {
	return m.all || (m.shown == 0 && m.pendingKill == nil)
}

// heading is the line under the tabline while a buffer is shown: what the
// buffer is, bold in parchment, and its facts in gray joined by middots —
// where it works, its pid, its ports, how its run ended and the runs before.
// tmux draws it on the border row under conn's pane, two columns in and
// two short of the right edge, so the line is cut to the width between.
func (m model) heading() string {
	t := m.terms[m.shown]
	if t == nil {
		return ""
	}
	name, facts := m.bufferFacts(m.shown, t)
	return headingStyle.Render(name) + " " + facts
}

// bufferFacts is a buffer's name and the facts beside it, styled: the
// ending in its color when the run has one, the past runs in theirs.
func (m model) bufferFacts(pid int, t *remoteTerm) (string, string) {
	name := "shell"
	var r navRow
	if n := m.nodes[pid]; n != nil {
		run := runFrom(n, len(m.procs))
		r = navRow{kind: rowProc, run: run, node: nameOf(run)}
		name = m.rowName(r)
	} else if t.name != "" {
		name = t.name
	}
	var facts []string
	if p, ok := m.placeAt(t.dir); ok {
		facts = append(facts, "in "+p.Name)
	}
	if t.run != "" && t.name != "" {
		facts = append(facts, t.run)
	}
	if r.node != nil {
		if a := m.agentFor(r); a != nil {
			if w, ok := a.(waited); ok && m.awaiting(r) != nil && w.since() > 0 {
				facts = append(facts, "waiting "+shortFor(w.since()))
			}
			if _, ok := a.blocked(); ok {
				if ask, _ := a.blocked(); ask != "" {
					facts = append(facts, "asks: "+ask)
				}
			}
		}
		// The branch, the pid and the ports, then what the buffer says of
		// itself read deeper: what it is and where before what it holds.
		if b := m.deep[pid].Branch; b != "" {
			facts = append(facts, b)
		}
		facts = append(facts, "pid "+strconv.Itoa(r.node.PID))
		if ps := runPorts(r.run, r.node); len(ps) > 0 {
			facts = append(facts, ":"+strings.Join(ps, " :"))
		}
		facts = append(facts, m.deep[pid].Facts...)
	}
	line := hintStyle.Render(dots(facts...))
	if e := m.ending(r); e.State != "" {
		style := toneStyles[toneGood]
		mark := glyphDone
		if e.State != "0" {
			style, mark = toneStyles[toneUrgent], glyphFailed
		}
		parts := []string{mark + " " + cmp.Or(exitWord(e.State), "exit "+e.State)}
		if e.Summary != "" && e.Summary != exitWord(e.State) {
			parts = append(parts, e.Summary)
		}
		if !e.At.IsZero() {
			parts = append(parts, ago(e.At))
		}
		line += hintStyle.Render(" "+glyphDot+" ") + style.Render(dots(parts...))
	}
	if strip := m.runsStrip(pid, t); strip != "" {
		line += hintStyle.Render(" "+glyphDot+" past runs ") + strip
	}
	return name, truncateStyled(line, m.width-4-lipgloss.Width(name+" "), false)
}

// runsStrip is the buffer's run history: the last runs of its command,
// newest first, each its mark and how long it took in the mark's color.
func (m model) runsStrip(pid int, t *remoteTerm) string {
	runs := m.history[pid]
	if len(runs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(runs))
	for _, r := range runs {
		style, mark := toneStyles[toneGood], glyphDone
		if r.Exit != "0" {
			style, mark = toneStyles[toneBad], glyphFailed
		}
		word := mark
		if r.Took > 0 {
			word += " " + shortTook(durationOf(r.Took))
		}
		parts = append(parts, style.Render(word))
	}
	return strings.Join(parts, " ")
}

// body is what conn draws under the tabline when it has the window: the
// kill preview while one waits — on the everything view itself when that
// is up, the processes to die marked where they stand, else a page of
// its own — else the everything view, put up, or standing in while no
// buffer is shown.
func (m model) body(rows int) []string {
	if m.pendingKill != nil && !m.all {
		return m.killPreview(rows)
	}
	if m.err != nil {
		return wrapText(m.err.Error(), m.width-len(gutter), rows, errStyle)
	}
	return m.everything(rows)
}

// withFooter puts up to two faint lines at the foot of a view, teaching the
// keys that matter now, when the view has the room.
func (m model) withFooter(lines []string, rows int, foot ...string) []string {
	if rows < len(lines)+len(foot)+2 {
		return padTo(lines, rows)
	}
	lines = padTo(lines, rows-len(foot)-1)
	for _, f := range foot {
		lines = append(lines, gutter+faintStyle.Render(truncateTail(f, m.width-len(gutter))))
	}
	return padTo(append(lines, ""), rows)
}

// everything is the old navigator as one view: every process grouped by
// place, docker merged in, a footer of the keys beneath.
func (m model) everything(rows int) []string {
	foot := []string{
		"space folds a place · / narrows by anything a row says · esc clears",
		"enter opens the buffer · x previews a kill · . toggles running · all",
	}
	if m.release != "" {
		foot[1] = "enter opens the buffer · x previews a kill · U installs conn " + strings.TrimPrefix(m.release, "v")
	}
	body := m.bodyHeight()
	lines := []string{""}
	if req := m.pendingKill; req != nil {
		// The processes to die are marked on their rows, the confirm
		// line stands under the list, and the keys' hints give way to
		// the signals'; the status line says what goes with them.
		foot = []string{"9 kills outright · i interrupts · h hangs up · esc changes your mind"}
		lines = append(lines, m.navLines(max(body-2, 1))...)
		lines = padTo(lines, max(body-1, 1))
		lines = append(lines, gutter+blockedStyle.Render(req.confirmWord())+" "+hintStyle.Render(glyphDot+" "+req.alternatives()))
		return m.withFooter(lines, rows, foot...)
	}
	lines = append(lines, m.navLines(body)...)
	return m.withFooter(lines, rows, foot...)
}

// bodyHeight is the number of rows the everything view's list has: the
// window under the edge row, the tabline and the blank after it, less the
// footer, which is what the cursor scrolls within.
func (m model) bodyHeight() int {
	rows := m.height - 3
	if rows >= 8 {
		rows -= 3
	}
	return max(rows, 1)
}

// navLines renders the visible window of the everything view: places, each
// followed by the processes running in them, nested the way they started one
// another.
func (m model) navLines(rows int) []string {
	if m.err != nil {
		return wrapText(m.err.Error(), m.width-len(gutter), rows, errStyle)
	}
	if m.projects == nil {
		return nil // still scanning
	}
	switch {
	case len(m.projects) == 0:
		return []string{gutter + noteStyle.Render("no repositories")}
	case len(m.rows) == 0 && m.filter != "":
		return []string{gutter + noteStyle.Render("nothing answers "+strings.TrimSpace(m.filter))}
	case len(m.rows) == 0:
		// The front door teaches the doors out of it — including the
		// one that teaches everything else.
		return []string{
			gutter + noteStyle.Render("nothing running"),
			"",
			gutter + faintStyle.Render("p   find a project · open a buffer"),
			gutter + faintStyle.Render(".   show all"),
			gutter + faintStyle.Render("?   the keys"),
		}
	}

	// Places read as paragraphs when the window has room: a blank row before
	// each place after the first. Only when the whole list fits with them —
	// a list that scrolls counts its rows, and a blank is not a row.
	gaps := 0
	if !m.typing {
		for i, r := range m.rows {
			if i > 0 && isPlace(r) {
				gaps++
			}
		}
	}
	spaced := m.offset == 0 && gaps > 0 && len(m.rows)+gaps <= rows

	end := min(m.offset+rows, len(m.rows))
	lines := make([]string, 0, rows)
	for i := m.offset; i < end; i++ {
		if spaced && i > 0 && isPlace(m.rows[i]) {
			lines = append(lines, "")
		}
		lines = append(lines, m.renderRow(m.rows[i], i == m.cursor))
	}
	return lines
}

// isPlace reports whether a row is a place at the top of the list: a group,
// or a repository under no group.
func isPlace(r navRow) bool {
	return r.prefix == "" && (r.kind == rowGroup || r.kind == rowProject || r.kind == rowSub)
}

// renderRow draws one row of the everything view. A place is a heading in
// parchment; a process sits under it, its cursor a marker in a gutter of
// its own, its state's mark beside its name in the state's color.
//
// A collapsed node carries the count of what it is hiding. That count is what
// distinguishes a folded node from a leaf, which the indent alone cannot
// show once the children are gone.
//
// A signalled process keeps its row and gains a turning marker until a rescan
// finds it gone, so the list never claims an exit that has not been observed.
func (m model) renderRow(r navRow, selected bool) string {
	marker := " "
	if selected {
		marker = glyphSelected
	}
	style := m.rowStyle(r, selected)
	// The cursor's row is a bar: everything on it keeps its color and
	// takes the chip's ground.
	bg := lipgloss.NewStyle()
	if selected {
		bg = chipStyle
	}
	on := func(st lipgloss.Style) lipgloss.Style { return bg.Inherit(st) }

	fold := ""
	if m.collapsed[detailKey(r)] {
		if n := m.childCount(r); n > 0 {
			fold = " +" + strconv.Itoa(n) + " folded"
		}
	}

	// An agent's row says which of you the other is waiting on: a working
	// instance turns beside its name, one stopped on an ask holds the
	// diamond, and one that has finished a turn holds the filled mark.
	mark, markStyle := "", faintStyle
	if a := m.agentFor(r); a != nil {
		glyph, mstyle := m.agentMark(r, a)
		mark, markStyle = " "+glyph, mstyle
	}

	// A row whose command ended badly, or whose process is stopped or a
	// zombie, shows the cross and says so; one whose command ended well
	// shows the check: a run's exit either way is what you were waiting on.
	if mark == "" && r.kind == rowProc {
		switch {
		case m.wrong(r):
			mark, markStyle = " "+glyphFailed, errStyle
		case m.ended(r) == "0" || containerDone(r):
			mark, markStyle = " "+glyphDone, toneStyles[toneGood]
		}
	}

	spinner := ""
	if r.kind == rowProc {
		if _, dying := m.dying[r.node.PID]; dying {
			spinner = " " + spinFrames[m.frame%len(spinFrames)]
		}
	}

	// A process a kill preview names shows the cross and says so, where
	// it stands on the list it was chosen from.
	doomed := r.kind == rowProc && m.pendingKill != nil && m.pendingKill.names(r.node.PID)
	if doomed {
		mark, markStyle = " "+glyphFailed, errStyle
	}

	// A process that is conn itself is (me), and only that, in gray.
	if m.selfRun(r) && !selected {
		style = selfStyle
	}
	// A conversation at rest is dim: not running, and owed nothing until
	// it is picked back up.
	if r.kind == rowRest && !selected {
		style = faintStyle
	}

	// The facts after the name, in gray: where it listens; at rest, for
	// how long; a container's port, health or age; a run's last word.
	facts := ""
	if r.kind == rowRest {
		facts = " " + glyphDot + " suspended " + glyphDot + " " + shortAge(r.rest.When)
	} else if r.kind == rowProc && r.node.Container != nil {
		facts = containerNote(r.node)
	} else if r.kind == rowProc {
		if ps := runPorts(r.run, r.node); len(ps) > 0 {
			facts = " " + glyphDot + " :" + strings.Join(ps, " :")
		}
		if e := m.ending(r); e.State != "" {
			facts = ""
			if said := cmp.Or(e.Summary, exitWord(e.State)); said != "" {
				facts += " " + glyphDot + " " + said
			}
			if !e.At.IsZero() {
				facts += " " + glyphDot + " " + shortAge(e.At)
			}
		}
		if a := m.awaiting(r); a != nil {
			if w, ok := a.(waited); ok && w.since() > 0 {
				facts += " " + glyphDot + " " + shortFor(w.since())
			}
		}
	}
	if doomed {
		facts = " " + glyphDot + " about to die" + facts
	}
	// A place's fold count and a wrong run's word read in the state's color.
	factStyle := hintStyle
	if r.kind == rowProc && (m.wrong(r) || doomed) && !selected {
		factStyle = errStyle
	}

	// A place is a heading at the margin, naming what the rows beneath are
	// inside; its rows sit one indent in, each child of a row a step
	// further.
	indent := r.prefix
	margin := ""
	if r.kind == rowProc || r.kind == rowRest {
		indent += glyphIndent + " "
		margin = " "
	}
	// A repository is cut from the left and a command from the right, because
	// what identifies each is at that end: the repo name after its parents,
	// and the program before its arguments.
	label := r.placeName()
	fromLeft := strings.Contains(label, "/")
	switch r.kind {
	case rowProc:
		label = m.rowLabel(r)
		fromLeft = false
		if r.node.Container != nil {
			label = glyphContainer + " " + label
			if !selected {
				style = tealStyle
			}
		}
	case rowRest:
		// Named for the kind that had it, as its live row would be.
		label = r.rest.Kind
		fromLeft = false
	}

	// The margin, the indent, and the marker's two columns come before
	// the name.
	room := m.width - 2 - lipgloss.Width(margin) - lipgloss.Width(indent) - lipgloss.Width(fold) -
		lipgloss.Width(spinner) - lipgloss.Width(mark) - lipgloss.Width(facts)

	// While a query is at work the matched letters are lit, so the narrowed
	// list always shows why it narrowed.
	var seg string
	if q := strings.TrimSpace(m.filter); q != "" && (m.typing || m.filter != "") {
		seg = truncateStyled(highlightOn(label, matchSpans(q, label), on(style), on(matchStyle)), room, fromLeft)
	} else if fromLeft {
		seg = on(style).Render(truncate(label, room))
	} else {
		seg = on(style).Render(truncateTail(label, room))
	}
	row := bg.Render(margin+indent) + on(selStyle).Render(marker) + bg.Render(" ") + seg + on(factStyle).Render(facts) +
		on(markStyle).Render(mark) + on(errStyle).Render(spinner) +
		on(hintStyle).Render(fold)
	if selected {
		return bg.Render(pad(row, m.width))
	}
	return row
}

// rowLabel names a process row. A shell a project asked for by name is called
// that: "web" is what the project calls it and what you would say out loud,
// where "sleep 35228" is only true.
//
// The pid is only shown while every process is on a line of its own. Folded,
// the list is about what is happening and the pid is a number beside every row
// that never helps you read it; unfolded, it is what tells two nvim apart and
// what you would type at another window.
func (m model) rowLabel(r navRow) string {
	name := m.rowName(r)
	if m.unfolded {
		return name + " " + nodeID(r.node)
	}
	return name
}

// rowName is what a row is called, without the number the unfolded list
// adds: the tab and the heading use it too, so they are about what the row
// says it is about.
func (m model) rowName(r navRow) string {
	if m.selfRun(r) {
		return "(me)"
	}
	name := commandOf(r.node)

	// An agent reads as what it is, not how it was invoked: the kind, and
	// the model when the invocation names one. This stands ahead of the
	// plan's name — an agent is named for its kind, not the entry that
	// happened to start it.
	if k, ok := agentKindOf(r.node); ok {
		name = agentLabel(k, m.agentNameOf(r.node), m.agentModelOf(r.node))
	} else if planned := m.plannedName(r); planned != "" {
		// A shell a project asked for is called what the project calls it,
		// whatever is running in it: web, not the http.server that is web
		// this time.
		name = planned
	}
	return name
}

// agentNameOf is what an agent's user called it, when the live instance
// says it was called anything.
func (m model) agentNameOf(n *ProcNode) string {
	if a, ok := m.agents[n.PID].(named); ok {
		return a.name()
	}
	return ""
}

// agentModelOf is the model an agent row shows: the one its invocation
// names, else the one the live instance advertises.
func (m model) agentModelOf(n *ProcNode) string {
	if model := agentModel(n.Argv); model != "" {
		return model
	}
	if a, ok := m.agents[n.PID].(modeled); ok {
		return a.model()
	}
	return ""
}

// plannedName is what a project called the shell a row stands for.
func (m model) plannedName(r navRow) string {
	for _, n := range r.run {
		if t, ok := m.terms[n.PID]; ok && t.name != "" {
			return t.name
		}
	}
	return ""
}

// commandOf is what a process was run with, cut down to what identifies it.
//
// The name alone is often not the answer: "npm run dev" reports itself as a
// node, and a row saying node tells you nothing you did not know. What was
// typed is what you would call it.
func commandOf(n *ProcNode) string {
	if short := shortArgv(n.Argv); short != "" {
		return short
	}
	return n.Command
}

// shortArgv trims a command line to the part that says what it is, or returns
// nothing when the arguments would say less than the name alone.
//
// A path is cut to its last element, because /opt/homebrew/bin/npm and npm are
// the same thing to read. The arguments are kept, since they are the whole
// difference between one npm run and another.
func shortArgv(argv string) string {
	fields := strings.Fields(argv)
	if len(fields) == 0 {
		return ""
	}

	// A command run through an interpreter names itself in its arguments, so
	// the interpreter is worth dropping when something follows it: the
	// script, or the module python was asked to run as one — http.server
	// is what "python3 -m http.server" is, and the -m says nothing on its
	// own.
	first := filepath.Base(fields[0])
	if len(fields) > 1 && interpreters[strings.TrimSuffix(first, ".exe")] {
		switch next := fields[1]; {
		case next == "-m" && len(fields) > 2:
			fields, first = fields[2:], fields[2]
		case !strings.HasPrefix(next, "-"):
			fields, first = fields[1:], filepath.Base(fields[1])
		}
	}

	// A shell given a script to run is the wrong thing to name a row after.
	// The script is somebody's idea of a command line, not a command: it is
	// long, it starts with whatever setup it needs, and by the time it has
	// been cut to fit, what is left is a fragment of a path. A bare shell says
	// less but is at least true.
	if shells[strings.TrimPrefix(first, "-")] {
		return ""
	}

	return strings.Join(append([]string{first}, fields[1:]...), " ")
}

// interpreters run something else, which is the thing worth naming.
var interpreters = map[string]bool{
	"node": true, "python": true, "python3": true, "ruby": true, "perl": true,
}

// rowStyle decides how brightly a row is drawn. Brightness in this list means
// the row can be stepped into: a place opens a shell, and a buffer conn holds
// can be returned to. Everything else is somebody else's process on somebody
// else's terminal, which conn cannot attach to, so it is drawn dim rather
// than offered and then refused: dim means look, don't step. The cursor
// changes none of it: selection is the bar under the row, never a color.
func (m model) rowStyle(r navRow, selected bool) lipgloss.Style {
	// While a project is being looked up the list is a reference rather than
	// the working view. Every row is a candidate and none of them has been
	// chosen, so nothing is lit but the one under the cursor.
	if m.typing && !selected {
		return faintStyle
	}
	if !m.attachable(r) {
		return faintStyle
	}
	if r.kind != rowProc {
		return placeStyle
	}
	return itemStyle
}

// killPreview is what x shows before anything dies: the buffer's heading
// saying so, the process tree with its pids and held ports, the
// consequences in prose, and the confirm line.
func (m model) killPreview(rows int) []string {
	req := m.pendingKill
	lines := []string{""}
	head := req.subject
	lines = append(lines, gutter+headingStyle.Render(head)+" "+hintStyle.Render(dots(req.where, "about to die")))
	lines = append(lines, "")

	// The tree, parents first, each a step further in than the one above.
	depth := map[int]int{}
	for _, n := range req.nodes {
		d := 0
		if pd, ok := depth[n.PPID]; ok {
			d = pd + 1
		}
		depth[n.PID] = d
		facts := []string{"pid " + nodeID(n)}
		if len(n.Ports) > 0 {
			facts = append(facts, "holds :"+strings.Join(n.Ports, " :"))
		}
		name := commandOf(n)
		if n.Container != nil {
			name = glyphContainer + " " + n.Container.Service
		}
		// The names in one column, the facts in the next: a name too long
		// for its column gives way, so a pid is always where the eye
		// expects it.
		column := min(m.width/2, 40)
		lead := gutter + strings.Repeat("  ", d) + errStyle.Render(glyphFailed) + " "
		name = truncateTail(name, max(column-lipgloss.Width(lead)-1, 4))
		row := pad(lead+itemStyle.Render(name), column) + hintStyle.Render(dots(facts...))
		lines = append(lines, truncateStyled(row, m.width, false))
	}
	// The consequences, when the window has the rows for them: the
	// confirm line is the one that must be read, and comes first when
	// they compete.
	var prose []string
	for _, l := range wrapValue(req.consequence(), m.width-len(gutter)) {
		prose = append(prose, gutter+hintStyle.Render(l))
	}
	confirm := gutter + blockedStyle.Render(req.confirmWord()) + " " + hintStyle.Render(glyphDot+" "+req.alternatives())
	if len(lines)+len(prose)+3 <= rows {
		lines = append(lines, "")
		lines = append(lines, prose...)
	}
	lines = append(lines, "", confirm)
	return m.withFooter(lines, rows,
		"after: the buffer stays as the record of the ending, until you close it",
		"9 kills outright · i interrupts · h hangs up · esc changes your mind")
}

// durationOf is seconds as a duration.
func durationOf(secs float64) time.Duration {
	return time.Duration(secs * float64(time.Second))
}

// pad right-fills a rendered line to width columns, measuring display width so
// styling does not count toward it.
func pad(s string, width int) string {
	if gap := width - lipgloss.Width(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}

// truncate shortens s to width columns, marking the cut with an ellipsis.
// Columns, not runes — a name in wide characters is as wide as the terminal
// will draw it, which is the measure everything else in this file uses.
//
// A qualified name like "w0zro/archive/conn" is cut from the left, because the
// repo name at the end is the part that identifies it; the parent directories
// are only there to break a tie. An unqualified name is cut from the right.
func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	total := lipgloss.Width(s)
	if total <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	if strings.Contains(s, "/") {
		// The widest tail that still fits beside the ellipsis.
		col := 0
		for i, r := range s {
			if total-col <= width-1 {
				return "…" + s[i:]
			}
			col += lipgloss.Width(string(r))
		}
		return "…"
	}
	head, _ := cutColumns(s, width-1)
	return head + "…"
}

// truncateTail cuts from the right, keeping the start. A command line says
// what it is first and how it was run afterwards, so the front is the part
// worth keeping — the opposite of a repository's name.
func truncateTail(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	head, _ := cutColumns(s, width-1)
	return head + "…"
}

// cutColumns splits s after the last character that still fits in width
// columns. A character too wide for the pane on its own is kept whole and
// overflows, because the alternative is to cut it into bytes that are not a
// character at all — and returning it uncut is what lets the caller stop
// rather than ask again for the same string.
func cutColumns(s string, width int) (head, tail string) {
	col := 0
	for i, r := range s {
		w := lipgloss.Width(string(r))
		if i > 0 && col+w > width {
			return s[:i], s[i:]
		}
		col += w
	}
	return s, ""
}

// wrapValue breaks a value at spaces where it can, and mid-token when a single
// token is longer than the width — paths and command lines usually are.
//
// Everything is measured in the columns a terminal will give it, the unit
// used throughout this file.
func wrapValue(s string, width int) []string {
	if width <= 0 {
		return nil
	}
	var lines []string
	for word := range strings.FieldsSeq(s) {
		switch {
		case len(lines) == 0:
			lines = append(lines, word)
		case lipgloss.Width(lines[len(lines)-1])+1+lipgloss.Width(word) <= width:
			lines[len(lines)-1] += " " + word
		default:
			lines = append(lines, word)
		}
		// Split anything still too wide.
		for lipgloss.Width(lines[len(lines)-1]) > width {
			head, tail := cutColumns(lines[len(lines)-1], width)
			lines[len(lines)-1] = head
			if tail == "" {
				break // a single character wider than the whole width
			}
			lines = append(lines, tail)
		}
	}
	return lines
}

// wrapText breaks a message across at most rows lines, measured in columns
// like everything else here.
func wrapText(s string, width, rows int, style lipgloss.Style) []string {
	if width <= 0 || rows <= 0 {
		return nil
	}
	var lines []string
	for rest := s; rest != "" && len(lines) < rows; {
		var head string
		head, rest = cutColumns(rest, width)
		lines = append(lines, " "+style.Render(head))
	}
	return lines
}
