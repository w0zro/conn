package main

import (
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// agentMark is the glyph beside an agent's row. A working one turns, one
// stopped mid-turn on a specific ask holds a bright diamond, one that has
// finished a turn and waits on its user holds a filled marker in the
// attention color, and one idle since it started — owed nothing — sits
// hollow and quiet. The diamond is the one worth crossing the room for:
// that answer resumes work already in flight.
func (m model) agentMark(r navRow, a agent) (string, lipgloss.Style) {
	if _, ok := a.blocked(); ok {
		return glyphAsk, blockedStyle
	}
	switch {
	case a.working():
		return spinFrames[m.frame%len(spinFrames)], busyStyle
	case m.awaiting(r) != nil:
		return glyphOn, attnStyle
	}
	return glyphOff, faintStyle
}

// View lays the window out as two full-height columns.
//
// The navigator draws in a pane of its own down the left of the home
// window, and the shell under its cursor is the tmux pane on its right: when
// a shell is shown, this view is exactly the navigator's column, as wide as
// its pane. With no shell to show the navigator has the whole window, and
// the right becomes its own pane — what is known about the row, or the
// picker. conn's name and its keys are on tmux's status line at the foot,
// not in a header of the column's own, so the column is the list from its
// first row and the right is the shell and nothing else. A terminal made
// to give up its first row to a header is a terminal drawing something
// other than what it was told it had room for.
func (m model) View() tea.View {
	v := tea.NewView(m.layout())
	v.AltScreen = true
	// Told when the keys leave for a shell and come back, so the cursor
	// can say whose the next letter is.
	v.ReportFocus = true
	return v
}

func (m model) layout() string {
	rows := m.height
	if rows <= 0 {
		rows = 1
	}

	left := m.leftColumn(rows)
	lines := padTo(left, rows)
	if m.showDetail() {
		right := m.paneLines(m.detailWidth(), rows)
		divider := ruleStyle.Render(glyphDivider)

		lines = make([]string, 0, rows)
		for i := 0; i < rows; i++ {
			// Every line ends reset, so nothing a row set can outlive it.
			lines = append(lines, pad(at(left, i), navWidth)+divider+at(right, i)+ansi.ResetStyle)
		}
	}
	return strings.Join(lines, "\n")
}

// leftColumn is conn's own column: the navigator, from the first row. Its
// name and what it has to say are said on tmux's status line, not here;
// the column is the list.
func (m model) leftColumn(rows int) []string {
	body := m.bodyHeight()
	nav := m.navLines(body)
	if len(nav) > body {
		nav = nav[:body]
	}
	return nav
}

// padTo lengthens lines to exactly n.
func padTo(lines []string, n int) []string {
	for len(lines) < n {
		lines = append(lines, "")
	}
	return lines[:n]
}

// navLines renders the visible window of the navigator: repositories, each
// followed by the processes running in them, nested the way they started one
// another.
func (m model) navLines(rows int) []string {
	if m.err != nil {
		return wrapText(m.err.Error(), navWidth-1, rows, errStyle)
	}
	if m.projects == nil {
		return nil // still scanning
	}
	switch {
	case len(m.projects) == 0:
		return []string{"  " + noteStyle.Render("no repositories")}
	case len(m.rows) == 0 && m.filter != "":
		return []string{"  " + noteStyle.Render("no project matches")}
	case len(m.rows) == 0:
		// The front door teaches the three doors out of it — including the
		// one that teaches everything else.
		return []string{
			"  " + noteStyle.Render("nothing running"),
			"",
			"  " + faintStyle.Render(".  show all"),
			"  " + faintStyle.Render("/  find a project"),
			"  " + faintStyle.Render("?  the keys"),
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
	return r.prefix == "" && (r.kind == rowGroup || r.kind == rowProject)
}

// renderRow draws one navigator row. The cursor is a marker in the gutter
// rather than a highlight, so it survives the tree rules beside it.
//
// A collapsed node carries the count of what it is hiding. That count is what
// distinguishes a folded node from a leaf, which the tree rules alone cannot
// show once the children are gone.
//
// A signalled process keeps its row and gains a red marker until a rescan finds
// it gone, so the list never claims an exit that has not been observed.
func (m model) renderRow(r navRow, selected bool) string {
	marker := " "
	if selected {
		marker = glyphSelected
	}
	style := m.rowStyle(r, selected)

	fold := ""
	if m.collapsed[detailKey(r)] {
		if n := m.childCount(r); n > 0 {
			fold = " +" + strconv.Itoa(n)
		}
	}

	// An agent's row says which of you the other is waiting on: a working
	// instance turns beside its name, and one that has finished a turn
	// lights the whole row, because done-and-waiting is the state that most
	// wants to be seen and the one a stopped spinner used to whisper.
	mark, markStyle := "", faintStyle
	if a := m.agentFor(r); a != nil {
		glyph, mstyle := m.agentMark(r, a)
		mark, markStyle = " "+glyph, mstyle
		if _, ok := a.blocked(); ok && !selected {
			style = blockedStyle
		} else if m.awaiting(r) != nil && !selected {
			style = attnStyle
		}
	}

	// A row whose command ended badly, or whose process is stopped or a
	// zombie, wears the cross in red — alive by the table and no use to
	// anyone, which is the state that most wants noticing after an agent's
	// ask. One whose command ended well wears the check in green: done,
	// and as worth seeing as a failure, since a run's ending either way is
	// what you were waiting on.
	if mark == "" && r.kind == rowProc {
		switch {
		case m.wrong(r):
			mark, markStyle = " "+glyphFailed, errStyle
			if !selected {
				style = errStyle
			}
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

	// A process that is conn itself is (me), and only that, in a color of
	// its own: the launcher become a tmux client would read as a go or a
	// tmux, and neither is what the row is. Ports it has none of, and a
	// mark it does not wear.
	if m.selfRun(r) && !selected {
		style = selfStyle
	}

	// Where it listens, beside the name the way an agent's model is: a
	// dev server's row says what it is, and this says where it is. It
	// stands outside the name rather than in it, so a name cut to fit
	// loses its tail and keeps its port — the port being the thing you
	// were about to go and look up.
	ports := ""
	if r.kind == rowProc && r.node.Container != nil {
		ports = containerNote(r.node)
	} else if r.kind == rowProc {
		if ps := runPorts(r.run, r.node); len(ps) > 0 {
			ports = " · :" + strings.Join(ps, " :")
		}
		// A shell at its prompt after its run says what the run said of
		// itself and how long ago it ended, where a running one says its
		// ports: 3 failed · 3m — the row reading as the transcript's last
		// word, and how stale it is.
		if e := m.ending(r); e.State != "" {
			ports = ""
			if e.Summary != "" {
				ports += " · " + e.Summary
			}
			if !e.At.IsZero() {
				ports += " · " + shortAge(e.At)
			}
		}
	}

	// A group or a repository sits on indent alone, naming a place the rows
	// beneath are inside. What hangs off a repository — its processes and its
	// sub-projects — is one family of siblings, a step further in.
	indent := r.prefix
	if r.kind == rowProc || r.kind == rowSub {
		indent += glyphIndent + " "
	}
	// A repository is cut from the left and a command from the right, because
	// what identifies each is at that end: the repo name after its parents,
	// and the program before its arguments.
	label := r.project.Name
	fromLeft := strings.Contains(label, "/")
	if r.kind == rowProc {
		label = m.rowLabel(r)
		fromLeft = false
	}

	// A column of gutter, the indent, and the marker's two columns come
	// before the name.
	room := navWidth - 3 - lipgloss.Width(indent) - lipgloss.Width(fold) -
		lipgloss.Width(spinner) - lipgloss.Width(mark) - lipgloss.Width(ports)

	// While a query is at work the matched letters are lit, so the narrowed
	// list always shows why it narrowed. The styled label is cut ansi-aware;
	// the plain path stays the plain cut.
	var seg string
	if q := strings.TrimSpace(m.filter); q != "" && (m.typing || m.filter != "") {
		seg = truncateStyled(highlight(label, matchSpans(q, label), style), room, fromLeft)
	} else if fromLeft {
		seg = style.Render(truncate(label, room))
	} else {
		seg = style.Render(truncateTail(label, room))
	}
	// The marker stands beside the name it marks, in the indent, rather
	// than at the edge of the column with the whole indent between them.
	return " " + indent + style.Render(marker) + " " + seg + style.Render(ports) +
		markStyle.Render(mark) + errStyle.Render(spinner) +
		faintStyle.Render(fold)
}

// rowLabel names a process row. A shell a project asked for by name is called
// that: "web" is what the project calls it and what you would say out loud,
// where "sleep 35228" is only true.
//
// The name belongs to the shell, so it stands for whatever is running in it —
// a run folded into one row is named for the shell that was asked for, not for
// the command that shell happens to be running now.
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
// adds: the pane's heading uses it too, so the pane is about what the row
// says it is about.
func (m model) rowName(r navRow) string {
	if m.selfRun(r) {
		return "(me)"
	}
	name := commandOf(r.node)

	// An agent reads as what it is, not how it was invoked: the kind, and
	// the model when the invocation names one. The resume id, the launcher,
	// the flags are the detail pane's to keep. This stands ahead of the
	// plan's name — an agent is named for its kind, not the entry that
	// happened to start it.
	if k, ok := agentKindOf(r.node); ok {
		name = agentLabel(k, m.agentNameOf(r.node), m.agentModelOf(r.node))
	} else if planned := m.plannedName(r); planned != "" {
		// A shell a project asked for is called what the project calls it,
		// whatever is running in it: web, not the http.server that is web
		// this time. The name comes from the service, the command is how it is
		// run today, and the row is about the service. The pane says the
		// command.
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
// names, else the one the live instance advertises. ollama names its model
// on the command line; claude keeps it in the transcript, which the scan
// reads for the row and folds into the instance.
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
// the row can be stepped into: a repository opens a shell, and a shell conn
// started can be returned to. Everything else is somebody else's process on
// somebody else's terminal, which conn cannot attach to, so it is drawn dim
// rather than offered and then refused.
func (m model) rowStyle(r navRow, selected bool) lipgloss.Style {
	// While a project is being looked up the list is a reference rather than
	// the working view. Every row is a candidate and none of them has been
	// chosen, so nothing is lit but the one under the cursor.
	if m.typing {
		if selected {
			return selStyle
		}
		return faintStyle
	}
	if !m.attachable(r) {
		if selected {
			return offSelStyle
		}
		return faintStyle
	}
	if selected {
		// Lit whether or not the keys are here: the row is the one the
		// keys went from, and dim reads as out of reach. Which pane has
		// the keys is the status line's to say.
		return selStyle
	}
	if r.kind != rowProc {
		return placeStyle
	}
	return itemStyle
}

// paneLeft is the pane's first column in the window: past the navigator and
// the divider.
func (m model) paneLeft() int { return navWidth + 1 }

// detailWidth is the room left for the detail pane beside the navigator.
func (m model) detailWidth() int { return m.width - m.paneLeft() }

// showDetail reports whether the navigator has room to carry a pane of its
// own beside the list. Beside an entered shell it has exactly its column
// and does not; with the window to itself it does, unless the window is
// narrow.
func (m model) showDetail() bool { return m.detailWidth() >= paneMin }

// paneLines renders the navigator's own pane beside the list: the picker
// while it is open, and otherwise what is known about the selected row —
// a held shell's included. The shell itself is only beside the navigator
// while the keys are in it, and then the navigator is its column alone.
func (m model) paneLines(width, rows int) []string {
	if m.resume != nil {
		return m.resumeLines(width, rows)
	}
	return m.detailLines(width, rows)
}

// resumeLines is the picker: a place's suspended conversations, newest
// first. Each row is when the conversation last moved, the branch it was on,
// and the last thing asked of it — the things a reader recognizes one by.
// The cursor's row is lit the way the navigator's is.
func (m model) resumeLines(width, rows int) []string {
	v := m.resume
	lines := []string{
		paneGutter + headingStyle.Render(v.place.Name),
		paneGutter + noteStyle.Render("suspended conversations"),
		"",
	}
	switch {
	case !v.loaded:
		return append(lines, paneGutter+noteStyle.Render("looking…"))
	case len(v.convos) == 0:
		return append(lines, paneGutter+noteStyle.Render("none to continue"))
	}
	list := v.matches()
	if len(list) == 0 {
		return append(lines, paneGutter+noteStyle.Render("nothing answers "+strings.TrimSpace(v.query)))
	}

	// The columns are sized to this listing: the age is short by construction,
	// and a branch keeps enough to be told apart without owning the row.
	agew, bw := 0, 0
	for _, c := range list {
		agew = max(agew, lipgloss.Width(shortAge(c.When)))
		bw = max(bw, lipgloss.Width(c.Branch))
	}
	bw = min(bw, 12)

	// The window slides the least amount that keeps the cursor on screen,
	// derived from the cursor alone so drawing moves nothing.
	sel := min(v.cursor, len(list)-1)
	detail := resumeDetail(list[sel], width, rows)
	body := max(rows-len(lines)-len(detail), 1)
	off := max(sel-body+1, 0)

	lead := 3 + agew + 2
	if bw > 0 {
		lead += bw + 2
	}
	for i := off; i < min(off+body, len(list)); i++ {
		c := list[i]
		marker, style := " ", itemStyle
		if i == sel {
			marker, style = glyphSelected, selStyle
		}
		row := " " + marker + " " + faintStyle.Render(pad(shortAge(c.When), agew)) + "  "
		if bw > 0 {
			row += faintStyle.Render(pad(truncate(c.Branch, bw), bw)) + "  "
		}
		// The prompt is the recognizer; a conversation that never got one is
		// named by what it said it was doing, or failing that by its id.
		text := c.Prompt
		if text == "" {
			text = c.Summary
		}
		if text == "" {
			text = c.ID
		}
		seg := style.Render(truncateTail(text, width-lead))
		if q := strings.TrimSpace(v.query); q != "" {
			seg = truncateStyled(highlight(text, matchSpans(q, text), style), width-lead, false)
		}
		lines = append(lines, row+seg)
	}
	for len(lines) < rows-len(detail) {
		lines = append(lines, "")
	}
	return append(lines, detail...)
}

// resumeDetail is the selected conversation whole, under the list: the full
// prompt a row could only truncate, what the session said it was doing, and
// where and on what branch it was had. A short pane keeps the list instead —
// the names are the scanning surface, and the depth can wait for room.
func resumeDetail(c conversation, width, rows int) []string {
	if rows < 14 {
		return nil
	}
	var fs []field
	add := func(label, value string, t tone) {
		if value != "" {
			fs = append(fs, field{label: label, value: value, tone: t})
		}
	}
	add("asked", c.Prompt, tonePlain)
	add("said", c.Summary, toneQuiet)
	add("branch", c.Branch, toneAccent)
	add("where", c.Dir, toneQuiet)
	if len(fs) == 0 {
		return nil
	}
	out := []string{ruleStyle.Render(strings.Repeat("─", width))}
	return append(out, renderBlock(fs, width)...)
}

// detailLines renders everything known about the selected row.
func (m model) detailLines(width, rows int) []string {
	r, ok := m.selected()
	if !ok {
		return []string{paneGutter + noteStyle.Render("nothing selected")}
	}

	fields, loaded := m.details[detailKey(r)]
	if !loaded {
		return []string{paneGutter + noteStyle.Render("loading…")}
	}

	// A transcript is read from its end: it is the last block, and when
	// the pane is too short for all of it, the lines that go are its
	// oldest, under its heading, so what the shell showed last is what
	// the pane shows.
	var lines []string
	tailAt := -1 // the first transcript line, when there is one
	for _, block := range blocks(fields) {
		drawn := renderBlock(block, width)
		if len(drawn) == 0 {
			continue
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		if isTranscript(block) {
			tailAt = len(lines) + 1
		}
		lines = append(lines, drawn...)
	}
	if over := len(lines) - rows; over > 0 {
		if tailAt >= 0 && tailAt+over <= len(lines) {
			lines = append(lines[:tailAt:tailAt], lines[tailAt+over:]...)
		} else {
			lines = lines[:rows]
		}
	}
	return lines
}

// isTranscript reports whether a block is a shell's transcript: a heading
// over lines of text.
func isTranscript(block []field) bool {
	return len(block) > 1 && block[0].kind == headingField && block[1].kind == textField
}

// blocks splits the fields at the breaks between groups. A group sets its own
// value column, so one long label does not indent a pane that has nothing else
// like it in it.
func blocks(fields []field) [][]field {
	var out [][]field
	var cur []field
	for _, f := range fields {
		if f.kind == gapField {
			out = append(out, cur)
			cur = nil
			continue
		}
		cur = append(cur, f)
	}
	return append(out, cur)
}

// renderBlock draws one group, preceded by the blank line that separates it
// from the last. A group with nothing in it draws nothing at all, so a pane
// that skipped a whole group does not leave a hole where it would have been.
func renderBlock(block []field, width int) []string {
	if len(block) == 0 {
		return nil
	}

	// The widest label in this group sets its value column, so values line up.
	labelW := 0
	for _, f := range block {
		if f.kind == pairField {
			labelW = max(labelW, lipgloss.Width(f.label))
		}
	}

	var lines []string
	for _, f := range block {
		switch f.kind {
		case headingField:
			lines = append(lines, paneGutter+titleStyle.Render(f.value))
		case noteField:
			for _, c := range wrapValue(f.value, width-len(paneGutter)-1) {
				lines = append(lines, paneGutter+noteStyle.Render(c))
			}
		case textField:
			// As the shell showed it, in the colors it drew, cut to the
			// pane: a transcript wrapped would be a different transcript.
			// Each line ends reset, so nothing the shell set outlives it.
			lines = append(lines, paneGutter+truncateStyled(f.value, width-len(paneGutter), false)+ansi.ResetStyle)
		default:
			lines = append(lines, wrapField(f, labelW, width)...)
		}
	}
	return lines
}

// wrapField draws one label and its value, wrapping a long value under the
// value column rather than letting it run off the pane.
func wrapField(f field, labelW, width int) []string {
	label := pad(labelStyle.Render(f.label), labelW)
	gutter := paneGutter
	valueW := max(width-labelW-2*len(gutter), 8)

	// The lead stands ahead of the value on its first line, in its own
	// tone, and the value wraps in the room it leaves.
	lead := ""
	if f.lead != "" {
		lead = toneStyles[f.leadTone].Render(f.lead)
		if f.value != "" {
			lead += "  "
		}
		valueW = max(valueW-lipgloss.Width(lead), 8)
	}

	chunks := wrapValue(f.value, valueW)
	if len(chunks) == 0 {
		chunks = []string{""}
	}

	style := toneStyles[f.tone]
	lines := make([]string, 0, len(chunks))
	for i, c := range chunks {
		if i == 0 {
			lines = append(lines, gutter+label+gutter+lead+style.Render(c))
			continue
		}
		lines = append(lines, gutter+strings.Repeat(" ", labelW)+gutter+style.Render(c))
	}
	return lines
}

// wrapValue breaks a value at spaces where it can, and mid-token when a single
// token is longer than the pane — paths and command lines usually are.
//
// Everything is measured in the columns a terminal will give it, the way the
// rest of this file measures. Counting bytes instead wraps a line of accented
// text a third of the way early, and cutting at a byte offset lands inside a
// character, leaving half of it on each of two lines where it draws as neither
// — which the ellipsis on a truncated prompt and the › between the processes
// of a run are both enough to trigger.
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
		// Split anything still too wide for the pane.
		for lipgloss.Width(lines[len(lines)-1]) > width {
			head, tail := cutColumns(lines[len(lines)-1], width)
			lines[len(lines)-1] = head
			if tail == "" {
				break // a single character wider than the whole pane
			}
			lines = append(lines, tail)
		}
	}
	return lines
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

// at returns the line at i, or blank past the end.
func at(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return ""
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

// wrapText breaks a message across at most rows navigator lines, measured in
// columns like everything else here.
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
