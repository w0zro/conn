package main

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/w0zro/conn/internal/config"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// conn walks where it was told and nowhere else, so a conn that has
// been told nothing cannot do anything at all: the processes view is
// every project work is happening in, and with no roots there are no
// projects to happen in. Rather than showing that as an empty list —
// which is also what a machine with no checkouts looks like, and what a
// conn pointed at the wrong directory looks like — conn asks.
//
// It asks in a view shaped like the list, because that is the shape the
// operator already knows for typing a little and picking from what
// comes back. The line is typed into, the directories that answer it
// are the rows, and enter takes one.

// rootsW is the width of the word before the line typed into, matching
// the list's own FIND.
const rootsW = findW

// A rootsReport is the asking view's words as things stand.
type rootsReport struct {
	typed string   // the path as it has been typed
	caret int      // where in it the caret is, in runes
	rows  []string // the directories that answer it, from ~
	err   string   // what went wrong saving, where something did
	// editing is whether this is a root being changed from the
	// settings rather than the first root conn has ever been given. The
	// chip at the foot says what pressing enter will do, and on the
	// first start it also says why the view is up at all.
	editing bool
}

// composeRoots is the view's words: what has been typed, and the
// directories on this machine that could be what was meant.
func composeRoots(typed, home string) rootsReport {
	b := composeRootsAt(typed, home)
	b.caret = utf8.RuneCountInString(typed)
	return b
}

// composeRootsAt is composeRoots with the caret left at the start, for
// the view to put where it is.
func composeRootsAt(typed, home string) rootsReport {
	b := rootsReport{typed: typed}
	for _, dir := range completeRoot(typed, home) {
		b.rows = append(b.rows, config.Tilde(dir, home))
	}
	return b
}

// A rootLine is a root being typed: the line, with the cursor among the
// directories answering it, and what went wrong saving it, where
// something did. The asking view on the panel is one, and so is the
// line the settings type a root into.
type rootLine struct {
	line typed
	err  string
}

// askingFor is a line put up saying text, with the caret at its end.
func askingFor(text string) rootLine {
	var r rootLine
	r.line.set(text)
	return r
}

// edit answers the keys every root line has, and says whether the key
// was one of them: the line's own (see typed), and tab, which fills the
// line in with the directory under the cursor and a separator after it,
// so the next keystroke is already looking inside it. Filling the line
// in is not answering. A line that changes takes what went wrong saving
// off with it, since the error was about what was typed and that is not
// what is typed now.
func (r *rootLine) edit(k, home string) bool {
	b := composeRoots(r.line.text, home)
	switch {
	case r.line.edit(k, len(b.rows)):
		if r.line.text != b.typed {
			r.err = ""
		}
	case k == "tab":
		if r.line.at < len(b.rows) {
			r.line.set(b.rows[r.line.at] + "/")
		}
	default:
		return false
	}
	return true
}

// chosen is the root the operator settled on, as typed or listed, and
// blank where there is none. It is what the cursor is on where the line
// has not been typed past it, and what was typed otherwise: somebody
// who typed a whole path and pressed enter meant that path, not the
// first thing that happened to be listed under it.
func (r rootLine) chosen(home string) string {
	root := strings.TrimSpace(r.line.text)
	if typedIsADir(root, home) {
		return root
	}
	if rows := composeRoots(r.line.text, home).rows; r.line.at < len(rows) {
		return rows[r.line.at]
	}
	return root
}

// report is the line's words as things stand.
func (r rootLine) report(home string) rootsReport {
	b := composeRootsAt(r.line.text, home)
	b.caret, b.err = r.line.cur, r.err
	return b
}

// completeRoot is the directories a typed path could become: the ones
// under the deepest directory it names, whose names carry on from where
// the typing stopped. A path typed up to a separator is a directory to
// look inside; anything else is a name half-written.
//
// Only directories, because a root is one. Hidden ones are left out
// unless they were asked for by name, since a checkout is not usually
// kept in one and a list of them is a list of the machine's own
// business.
func completeRoot(typed, home string) []string {
	// Whether the typing stopped on a separator is read off what was
	// typed, not off the expansion: config.ExpandHome joins, and joining
	// drops a trailing separator, which turned "~/" — a directory to
	// look inside — into the name w0zro half-written under /Users.
	raw := strings.TrimSpace(typed)
	onSeparator := strings.HasSuffix(raw, string(filepath.Separator))
	path := config.ExpandHome(raw, home)
	var dir, prefix string
	switch {
	case raw == "":
		dir = home
	case onSeparator:
		dir = path
	default:
		dir, prefix = filepath.Split(path)
		if dir == "" {
			dir = home // a bare name is read against the home, not the cwd
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() {
			continue
		}
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(prefix, ".") {
			continue
		}
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		out = append(out, filepath.Join(dir, name))
	}
	sort.Strings(out)
	return out
}

// drawRoots draws the asking view, in the list's own shape: the header
// with what conn has, the line being typed into, the directories that
// answer it, and the chip saying what pressing enter will do.
func drawRoots(b rootsReport, cursor, width, height int, p palette) []row {
	width = max(width, panelMinCols)
	measure := measureAt(width)
	c := canvas{p: p, width: width}

	c.blank(0)
	l := c.line()
	l.add(p.orange+p.bold, "ROOTS")
	right := "NONE SET"
	if len(b.rows) > 0 {
		right = strconv.Itoa(len(b.rows)) + " UNDER IT"
	}
	l.to(measure - ansi.StringWidth(right))
	l.add(p.gray, right)
	c.emit(l, 0, false)
	c.rule(0, measure)

	l = c.line()
	before, after := typedRuns(b.typed, b.caret, measure-rootsW-2, true)
	l.field(0, measure-rootsW, "ROOT", before, after)
	c.emit(l, 0, false)

	room := height
	if height == 0 {
		room = 1 << 30
	}
	var body []row
	cursorRow := -1
	d := canvas{p: p, width: width}
	say := func(color, s string) {
		d.blank(0)
		ln := d.line()
		ln.add(color, s)
		d.emit(ln, 0, true)
		body = d.rows
	}
	switch {
	case b.err != "":
		say(p.chip, " "+strings.ToUpper(b.err)+" ")
	case len(b.rows) == 0:
		say(p.gray, "NOTHING UNDER "+strings.ToUpper(b.typed))
	default:
		d.blank(0)
		for i, dir := range b.rows {
			ln := d.line()
			if i == cursor {
				ln.p = p.chosen()
				if p.plain {
					ln.mark = "▸"
				}
				cursorRow = len(d.rows)
			}
			ln.add(ln.p.ink, fit(dir, measure, true))
			d.emit(ln, 0, false)
		}
		body = d.rows
	}

	// One row is kept back for the chip at the foot, which says what
	// the keys do here: this view exists to be answered, and an operator
	// who cannot see how to answer it is stuck in the one place conn
	// offers no way out of.
	c.rows = append(c.rows, scrolled(body, cursorRow, room-len(c.rows)-1, width, p)...)
	if height > 0 {
		for len(c.rows) < height-1 {
			c.blank(0)
		}
	}
	l = c.line()
	word := " CONN HAS NO ROOTS · ENTER SAVES ONE "
	if b.editing {
		word = " ENTER SAVES IT · ESC GOES BACK "
	}
	l.add(p.chip, word)
	c.emit(l, 0, true)
	return c.rows
}

// typedIsADir says whether what was typed already names a directory, so
// that a path typed in full is taken as it stands.
func typedIsADir(typed, home string) bool {
	if strings.TrimSpace(typed) == "" {
		return false
	}
	info, err := os.Stat(config.ExpandHome(strings.TrimSpace(typed), home))
	return err == nil && info.IsDir()
}

// toRoots is the asking view, which conn goes to instead of the
// processes view when it has no roots. It comes up on the home, which
// is where checkouts usually are and is a directory that certainly
// exists, so the first thing shown is a list rather than nothing.
func (m model) toRoots() (model, tea.Cmd) {
	m.view, m.asking = viewRoots, askingFor("~/")
	return m, nil
}

// rootsKey is the asking view's keys. The line is typed into like
// every root line; see rootLine. What is the view's own: enter takes the
// root — the config is written and conn is working from it before the
// view is gone.
func (m model) rootsKey(k string) (model, tea.Cmd) {
	switch {
	case m.asking.edit(k, m.head.Login.Home):
	case k == "ctrl+c":
		return m.leave()
	case k == "esc":
		// Nothing. The first start has nowhere to go back to: conn
		// cannot show the processes view until this is answered, and a
		// key that did nothing would be conn pretending there was a
		// way past it.
	case k == "enter":
		return m.takeRoot()
	}
	return m, nil
}

// takeRoot writes the root the operator settled on and puts conn to
// work on it.
func (m model) takeRoot() (model, tea.Cmd) {
	home := m.head.Login.Home
	root := m.asking.chosen(home)
	if root == "" {
		return m, nil
	}
	full := config.ExpandHome(root, home)
	// The roots the file names, with this one added. The file is read
	// again rather than taken off the model, since it is the file this
	// writes and the operator may have edited it by hand since conn
	// last read it.
	c, err := config.Read(home)
	if err != nil {
		m.asking.err = err.Error()
		return m, nil
	}
	roots := append(append([]string{}, c.Roots...), config.Tilde(full, home))
	if err := config.SaveRoots(home, roots); err != nil {
		m.asking.err = err.Error()
		return m, nil
	}
	// conn works from it now, not on the next start: the roots the
	// reading names projects by are the ones just written, and the walk
	// and the table are asked again against them.
	m = m.rooted(rootOn(config.CleanRoots(roots, home)))
	m.view, m.processesGen = viewProcesses, m.processesGen+1
	cmds := []tea.Cmd{m.readProcesses(), m.scanProjects()}
	if m.inside {
		cmds = append(cmds, m.serverCmd(func() error { return m.srv.Narrow() }))
	}
	return m, tea.Batch(cmds...)
}
