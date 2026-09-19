package main

import (
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The settings view: conn's configuration, worked where conn is worked.
// The file is small and a text editor opens it in a second, which is
// the argument for never building this; the argument against is that
// everything else conn knows is on the screen and the one thing conn
// runs on was a path an operator had to be told and a spelling they had
// to get right. conn already asks for a root on the first start and
// writes the answer down — this is that, for a station already at work.
//
// It is a list, like the rest of conn: the settings are the rows, the
// cursor moves among them, and enter answers the row under it. A root
// is typed into the asking view, which is the one place conn completes
// a path and knows what is a directory on this machine.
//
// What it writes is the file, and only the keys it came to change:
// saveSetting carries the rest through. What is in force is another
// matter — CONN_ROOTS stands in front of the file and the view says so
// rather than showing rows that are not what conn is walking.

// What a row of the view is.
type settingKind int

const (
	rootSetting    settingKind = iota // one configured root
	addRootSetting                    // the row that asks for another
)

// A settingRow is one row of the view: what it says, what is wrong or
// notable about it against the right, and which setting it stands for.
type settingRow struct {
	kind settingKind
	text string // the root as the file writes it
	note string // MISSING, NOT A DIR
	at   int    // which root, for the row that edits or removes one
}

// A settingsReport is the view's words as things stand.
type settingsReport struct {
	path    string // the file, from ~
	present bool   // there is a file there at all
	err     string // it will not parse, or would not save
	forced  bool   // CONN_ROOTS stands in front of the roots in it
	roots   int    // how many the file names
	rows    []settingRow
}

// addRootRow is what the row that takes another root says.
const addRootRow = "+ A ROOT"

// composeSettings is the file as the view shows it: the roots it names,
// as it names them, with what each one turned out to be on this
// machine.
//
// The roots are the file's own, not the ones in force: this is the view
// that edits the file, and a view that showed CONN_ROOTS's directories
// would be offering to edit rows that are not in the file at all.
func composeSettings(home string) settingsReport {
	b := settingsReport{path: tilde(configPath(home), home)}
	if _, err := os.Stat(configPath(home)); err == nil {
		b.present = true
	}
	c, err := readConfig(home)
	if err != nil {
		// A file that will not parse gets no rows. conn cannot tell
		// what is in it, so it cannot keep what it does not understand,
		// and a view offering to add a root would be offering a write
		// that will be refused; what there is to do is open the file.
		b.err = err.Error()
		return b
	}
	if _, source, _ := resolveRoots(home); source == rootsEnv {
		b.forced = true
	}
	for i, r := range c.Roots {
		row := settingRow{kind: rootSetting, text: r, at: i}
		if s := rootStateOf(expandHome(strings.TrimSpace(r), home)); s.problem != "" {
			row.note = strings.ToUpper(s.problem)
		}
		b.rows = append(b.rows, row)
		b.roots++
	}
	b.rows = append(b.rows, settingRow{kind: addRootSetting, text: addRootRow})
	return b
}

// drawSettings renders the view for a pane of the given size, with the
// cursor on the given row.
func drawSettings(b settingsReport, cursor, width, height int, p palette) []row {
	width = max(width, panelMinCols)
	measure := measureAt(width)
	c := canvas{p: p, width: width}

	// The header: the view, and the file it is of, since a setting that
	// is not doing what was meant is answered by opening that file and
	// the operator should not have to be told twice where it is.
	c.blank(0)
	l := c.line()
	l.add(p.orange+p.bold, "SETTINGS")
	right := b.path
	if !b.present {
		right += " · NEW"
	}
	if w := measure - utf8.RuneCountInString("SETTINGS") - 2; utf8.RuneCountInString(right) > w {
		right = fit(right, w, true)
	}
	l.to(measure - utf8.RuneCountInString(right))
	l.add(p.gray, right)
	c.emit(l, 0, false)
	c.rule(0, measure)

	room := height
	if height == 0 {
		room = 1 << 30
	}
	d := canvas{p: p, width: width}
	cursorRow := -1
	say := func(color, s string) {
		l := d.line()
		l.add(color, fit(s, measure, false))
		d.emit(l, 0, false)
	}
	if b.err != "" {
		d.blank(0)
		l := d.line()
		l.add(p.chip, " "+strings.ToUpper(fit(b.err, measure-2, false))+" ")
		d.emit(l, 0, false)
		if len(b.rows) == 0 {
			d.blank(0)
			say(p.gray, "CONN WILL NOT WRITE OVER A FILE IT CANNOT READ")
		}
	}
	kind := settingKind(-1)
	for i, r := range b.rows {
		if r.kind != kind && !(kind == rootSetting && r.kind == addRootSetting) {
			// A heading for the setting, with what the file says of it
			// against the right: how many roots it names.
			d.blank(0)
			l := d.line()
			l.eyebrow(0, "ROOTS", measure, strconv.Itoa(b.roots))
			d.emit(l, 0, false)
			d.blank(0)
			if b.forced {
				// The file is not what conn is walking. Said here rather
				// than left to the console, because this is the view
				// where somebody edits a root and waits for the list to
				// change.
				say(p.gray, "CONN_ROOTS STANDS IN FRONT OF THESE")
				d.blank(0)
			}
		}
		kind = r.kind
		l := d.line()
		if i == cursor {
			l.p = p.chosen()
			if p.plain {
				l.mark = "▸"
			}
			cursorRow = len(d.rows)
		}
		text, note := r.text, r.note
		room := measure
		if note != "" {
			room -= utf8.RuneCountInString(note) + 2
		}
		color := l.p.ink
		if r.kind == addRootSetting {
			color = l.p.gray
		}
		l.add(color, fit(text, room, r.kind == rootSetting))
		if note != "" {
			l.to(measure - utf8.RuneCountInString(note))
			l.add(l.p.gray, note)
		}
		d.emit(l, 0, false)
	}

	c.rows = append(c.rows, scrolled(d.rows, cursorRow, room-len(c.rows), width, p)...)
	if height > 0 {
		for len(c.rows) < height {
			c.blank(0)
		}
	}
	return c.rows
}
