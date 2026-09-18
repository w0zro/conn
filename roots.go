package main

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
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
		b.rows = append(b.rows, tilde(dir, home))
	}
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
	// typed, not off the expansion: expandHome joins, and joining drops
	// a trailing separator, which turned "~/" — a directory to look
	// inside — into the name w0zro half-written under /Users.
	raw := strings.TrimSpace(typed)
	onSeparator := strings.HasSuffix(raw, string(filepath.Separator))
	path := expandHome(raw, home)
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
	measure, _, _ := columns(width)
	c := canvas{p: p, width: width}

	c.blank(0)
	l := c.line()
	l.add(p.orange+p.bold, "ROOTS")
	right := "NONE SET"
	if len(b.rows) > 0 {
		right = strconv.Itoa(len(b.rows)) + " UNDER IT"
	}
	l.to(measure - utf8.RuneCountInString(right))
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
	l.add(p.chip, " CONN HAS NO ROOTS · ENTER SAVES ONE ")
	c.emit(l, 0, true)
	return c.rows
}
