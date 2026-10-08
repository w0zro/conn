package main

import (
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/w0zro/conn/internal/draw"
	"github.com/w0zro/conn/internal/room"
	"github.com/w0zro/conn/internal/work"
	"github.com/w0zro/conn/internal/work/stationlog"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// The log view: the panel over time, which l puts up. The panel is all
// present tense — what each row is doing now, and how long it has
// stood so — and says nothing of what happened while the operator was
// in a pane. The log is that: a line for each change worth one, newest
// first, under the day it happened on. A line is the moment, the
// project, what the panel called the row, the row's word as the panel
// said it, and under it what the line adds, how long the state stood
// or what a wait is on, so a line reads as a row of the panel read
// later. The band counts the lines written since the view was last
// opened, so an operator coming out of a pane is told that something
// happened before they go looking for what.
//
// The lines are written as the readings land, whatever view is up and
// wherever the keys are, and read back from the file when the view
// opens; see stationlog.Changes for what counts as a change. Enter goes to
// the row's process where it is still on the panel; esc, or l again,
// goes back.

// logKeep is how many lines the view reads back from the file: a
// screen scrolls through a few hundred and the file holds the rest.
const logKeep = 500

// logMsg is the file read, for the view that asked.
type logMsg struct{ events []stationlog.Event }

// The log as the panel holds it: the lines read, oldest first as the
// file has them, the cursor among them counted from the newest, and
// the count the band says.
type logList struct {
	read   []stationlog.Event
	loaded bool
	at     int // the cursor, counted from the newest line
	fresh  int // lines written since the view was opened, or came since it was: drawn in the ink
	unseen int // lines written since the view was last opened, for the band
}

// logPath is the log's file: beside the server's socket, which is in
// the state directory unless a socket is named, so a server on a
// socket of its own — a test's, a scratch one — keeps a log of its own
// and never writes into the station's.
func logPath(home string) string {
	return filepath.Join(filepath.Dir(room.SocketPath(home)), "log")
}

// logged takes the changes a reading found: onto the view where it is
// open, and onto the band's count where it is not, and to the file
// either way. Nothing is written beside whatever directory conn was
// started in, so a conn with no home keeps no log.
func (m model) logged(events []stationlog.Event) (model, tea.Cmd) {
	if len(events) == 0 {
		return m, nil
	}
	if m.view == viewLog {
		m.log.fresh += len(events)
	} else {
		m.log.unseen += len(events)
	}
	if m.log.loaded {
		m.log.read = append(m.log.read, events...)
		if n := len(m.log.read) - logKeep; n > 0 {
			m.log.read = m.log.read[n:]
		}
		// The cursor stays on its line as the newer ones come in above
		// it, the way the panel keeps the cursor on its row.
		if m.log.at > 0 {
			m.log.at = min(m.log.at+len(events), len(m.log.read)-1)
		}
	}
	home := m.head.Login.Home
	if home == "" {
		return m, nil
	}
	path := logPath(home)
	return m, func() tea.Msg { _ = stationlog.Append(path, events); return nil }
}

// logging is a reading read against the last for what changed, and
// the changes logged. The rows are the panel's as filed, whatever the
// view is showing of them: the log is the panel over time, and the
// tree is a way of looking at the panel. The first reading has nothing
// to be read against and logs nothing: what was already there when
// conn came up is the panel's to show, and the log begins with what
// changed while conn watched. A reading that failed is not a table of
// nothing, and is read against nothing.
func (m model) logging(msg processesMsg) (model, tea.Cmd) {
	if msg.err != "" {
		return m, nil
	}
	rows := msg.projects
	if m.full {
		rows = fold(msg.tree)
	}
	var cmd tea.Cmd
	if m.seenAny {
		m, cmd = m.logged(stationlog.Changes(m.seen, rows, logLabel, time.Now()))
	}
	m.seen, m.seenAny = rows, true
	return m, cmd
}

// logLabel is what a line calls a row: its declared name, a contact's
// title, and otherwise what it was started as. A working contact's
// row says what it is doing, which is the one thing about it that
// changes; a line is read later, and names the row by what stays.
func logLabel(e work.Entry) string {
	if name := declaredNameOf(e); name != "" {
		return name
	}
	if e.Kind == work.KindContact && e.Title != "" {
		return e.Title
	}
	return e.AsTyped()
}

// logReport is the log view's words as things stand.
func (m model) logReport() logReport {
	alive := func(e stationlog.Event) bool { _, ok := m.logEntry(e); return ok }
	return composeLog(m.log.read, m.log.fresh, alive, m.roots.real, m.head.Login.Home, m.now)
}

// openLog puts the log view up, and reads the file for it. came is the
// pane the keys were in, for esc to go back to. Opening it is seeing
// what the band was counting, so the count is cleared, and the lines
// it counted are drawn as new until the view is left.
func (m model) openLog(came string) (model, tea.Cmd) {
	m.from = came
	m.view = viewLog
	m.log.fresh, m.log.unseen, m.log.at = m.log.unseen, 0, 0
	m.processesGen++
	cmds := []tea.Cmd{m.readProcesses()}
	if !m.log.loaded {
		path := logPath(m.head.Login.Home)
		cmds = append(cmds, func() tea.Msg {
			events, _ := stationlog.Read(path, logKeep)
			return logMsg{events}
		})
	}
	return m, tea.Batch(cmds...)
}

// landedLog takes the file as read. A line written since the read was
// asked for is already on the list, and is kept over the file's copy
// of it.
func (m model) landedLog(msg logMsg) model {
	if m.log.loaded {
		return m
	}
	m.log.read, m.log.loaded = append(msg.events, m.log.read...), true
	if n := len(m.log.read) - logKeep; n > 0 {
		m.log.read = m.log.read[n:]
	}
	m.log.at = clamp(m.log.at, len(m.log.read))
	return m
}

// logAt is the line under the cursor, newest first, where there is one.
func (l logList) logAt() (stationlog.Event, bool) {
	i := len(l.read) - 1 - l.at
	if i < 0 || i >= len(l.read) {
		return stationlog.Event{}, false
	}
	return l.read[i], true
}

// logEntry is the row a line is about, where the row is still on the
// panel: the same pid, begun before the line was written.
func (m model) logEntry(e stationlog.Event) (work.Entry, bool) {
	for _, pl := range m.projects {
		for _, r := range pl.Entries {
			if r.PID == e.PID && !r.Started.After(e.At) {
				return r, true
			}
		}
	}
	return work.Entry{}, false
}

// logKey answers a key on the log view: j and k move among the lines,
// gg and G to the newest and the oldest, enter goes to the line's
// process where it is still there, / searches the output of the line's
// project, and esc or l again goes back.
func (m model) logKey(k string) (model, tea.Cmd) {
	switch k {
	case "ctrl+c", "q":
		return m.leave()
	case "esc", "l":
		return m.backFrom()
	case "j", "down":
		m.log.at = ring(m.log.at+1, len(m.log.read))
	case "k", "up":
		m.log.at = ring(m.log.at-1, len(m.log.read))
	case "g":
		// The half of gg; see key.
		m.firstG = true
	case "G":
		m.log.at = max(len(m.log.read)-1, 0)
	case "/":
		// The output of the line's project, searched: a line says a run
		// ended with a code, and the search is what the pane said.
		if e, ok := m.log.logAt(); ok {
			return m.openOutput(e.Project, m.from)
		}
	case "enter":
		if e, ok := m.log.logAt(); ok {
			if r, ok := m.logEntry(e); ok {
				m.from = ""
				return m.goTo(r)
			}
		}
	}
	return m, nil
}

// A logRow is a line as the view draws it.
type logRow struct {
	at      time.Time
	project string // the project as the panel names its block
	label   string
	word    string
	note    string // what the line adds, on a row of its own under it
	alive   bool   // its row is still on the panel
	fresh   bool   // written since the view was last opened
}

// logReport is the log view's words as things stand.
type logReport struct {
	rows  []logRow // newest first
	total int
	fresh int
	now   time.Time
}

// composeLog words the log view: the lines newest first, each naming
// its project the way the panel names a block, and whether the row it
// is about is still there to go to.
func composeLog(events []stationlog.Event, fresh int, alive func(stationlog.Event) bool, roots []string, home string, now time.Time) logReport {
	b := logReport{total: len(events), fresh: min(fresh, len(events)), now: now}
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		b.rows = append(b.rows, logRow{
			at: e.At, project: projectName(e.Project, roots, home), label: e.Label, word: e.Word, note: e.Note,
			alive: alive != nil && alive(e), fresh: len(events)-1-i < fresh,
		})
	}
	return b
}

// logDay is the eyebrow a line stands under: TODAY, YESTERDAY, and
// the date before that, in the form the cover of the manual writes
// one.
func logDay(at, now time.Time) string {
	y1, m1, d1 := at.Date()
	y2, m2, d2 := now.Date()
	switch {
	case y1 == y2 && m1 == m2 && d1 == d2:
		return "TODAY"
	case at.AddDate(0, 0, 1).Format("20060102") == now.Format("20060102"):
		return "YESTERDAY"
	}
	return strings.ToUpper(at.Format("2 Jan"))
}

// The log view's columns: the time at the margin, the project after
// it, and the word against the right, the label taking what is left.
const (
	logTimeW    = 5
	logProjectW = 12
)

// drawLog renders the log view for a terminal of the given size, with
// the cursor on the given line from the newest.
func drawLog(b logReport, cursor, width, height int, p draw.Palette) []draw.Row {
	// The header: the name of the view, and against the right how many
	// lines are new since it was last opened, or how many there are.
	right := strconv.Itoa(b.total) + " LINES"
	if b.fresh > 0 {
		right = strconv.Itoa(b.fresh) + " NEW"
	}
	c, measure := draw.Head("LOG", right, width, p)

	var body []draw.Row
	cursorRow := -1
	d := draw.Canvas{P: p, Width: c.Width}
	if len(b.rows) == 0 {
		d.Blank(0)
		l := d.Line()
		l.Add(p.Gray, "NOTHING YET")
		d.Emit(l, 0, true)
		body = d.Rows
	}
	// The widest word, so the words stand in one column: a stamp's two
	// cells counted, and never under the panel's floor.
	wordW := panelStatusW
	for _, r := range b.rows {
		w := ansi.StringWidth(r.word)
		if r.word == work.StatusWaiting || work.Faulty(r.word) {
			w = draw.StampWidth(r.word, p)
		}
		wordW = max(wordW, w)
	}
	day := ""
	for i, r := range b.rows {
		if on := logDay(r.at, b.now); on != day {
			day = on
			d.Blank(0)
			l := d.Line()
			l.Eyebrow(0, day, measure, "")
			d.Emit(l, 0, false)
		}
		l := d.Line()
		if i == cursor {
			l.P = p.Chosen()
			l.Mark = draw.CursorBar
			if p.Plain {
				l.Mark = "▸"
			}
			cursorRow = len(d.Rows)
		}
		ink := p.Faint
		if r.fresh {
			ink = p.Ink
		}
		l.Add(p.Gray, r.at.Format("15:04"))
		l.To(logTimeW + 1)
		l.Add(p.Gray, draw.Fit(r.project, logProjectW-1, true))
		l.To(logTimeW + 1 + logProjectW)
		labelW := measure - l.Cells - wordW - 1
		if r.alive {
			l.Add(ink+p.Bold, draw.Fit(r.label, labelW, false))
		} else {
			l.Add(ink, draw.Fit(r.label, labelW, false))
		}
		switch {
		case r.word == work.StatusWaiting || work.Faulty(r.word):
			l.To(measure - draw.StampWidth(r.word, p))
			l.Stamp(r.word)
		default:
			l.To(measure - ansi.StringWidth(r.word))
			l.Add(p.Gray, r.word)
		}
		d.Emit(l, 0, false)
		// What the line adds, under it in the gray, from the label's
		// column: how long the state stood, or what the wait is on.
		if r.note != "" {
			l := d.Line()
			if i == cursor {
				l.P = p.Chosen()
			}
			l.To(logTimeW + 1 + logProjectW)
			l.Add(p.Gray, draw.Fit(r.note, measure-l.Cells, false))
			d.Emit(l, 0, false)
		}
	}
	if len(b.rows) > 0 {
		body = d.Rows
	}
	return c.Foot(body, cursorRow, height)
}
