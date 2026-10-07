package main

import (
	"os"
	"strconv"
	"strings"

	"github.com/w0zro/conn/internal/draw"
	"github.com/w0zro/conn/internal/theme"

	"github.com/w0zro/conn/internal/config"

	"github.com/charmbracelet/x/ansi"
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
// a path and knows what is a directory on this machine; a theme and a
// ground are picked off the rows, conn having a fixed few of each, and
// either picked is worn at once rather than waiting for the next start.
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
	themeSetting                      // one of the themes conn has
	groundSetting                     // a ground, or the terminal's own
)

// A settingRow is one row of the view: what it says, what is wrong or
// notable about it against the right, and which setting it stands for.
type settingRow struct {
	kind settingKind
	text string // the root as the file writes it, or the setting's name
	note string // MISSING, NOT A DIR, IN USE, IN THE FILE
	at   int    // which root, for the row that edits or removes one
	// value is what the file would say for this row, where that is not
	// what the row says: the ground's own word, and nothing at all for
	// the row that gives the choice back to the terminal.
	value string
}

// A settingsReport is the view's words as things stand.
type settingsReport struct {
	path    string // the file, from ~
	present bool   // there is a file there at all
	err     string // it will not parse, or would not save
	forced  bool   // CONN_ROOTS stands in front of the roots in it
	roots   int    // how many the file names
	// What the file names that conn does not have, as written: a theme
	// by no name of conn's, a ground that is neither. Neither is a
	// reason to refuse the file — conn uses its own and goes on — and
	// both are said, since a setting written and not taken is the
	// quiet mistake this file has.
	unknownTheme, unknownGround string
	rows                        []settingRow
}

// The notes a theme's row wears: the one conn is wearing now, and the
// one the file names where a flag on the way in put conn in another.
const (
	noteInUse  = "IN USE"
	noteInFile = "IN THE FILE"
)

// grounds are the ground's rows, in the order they are offered: the two
// conn has, and the one that is not a ground at all — the terminal's
// answer, which is what conn does with no ground named.
var grounds = []struct{ text, value string }{
	{"DARK", config.DarkGround},
	{"LIGHT", config.LightGround},
	{askTheTerminal, ""},
}

// addRootRow is what the row that takes another root says, and
// askTheTerminal the row that leaves the ground to the terminal.
const (
	addRootRow     = "+ A ROOT"
	askTheTerminal = "ASK THE TERMINAL"
)

// composeSettings is the file as the view shows it: the roots it names,
// as it names them, with what each one turned out to be on this
// machine, and the themes conn has with the one being worn marked.
//
// The roots are the file's own, not the ones in force: this is the view
// that edits the file, and a view that showed CONN_ROOTS's directories
// would be offering to edit rows that are not in the file at all.
func composeSettings(home, inUse string, dark bool) settingsReport {
	b := settingsReport{path: config.Tilde(config.Path(home), home)}
	if _, err := os.Stat(config.Path(home)); err == nil {
		b.present = true
	}
	c, err := config.Read(home)
	if err != nil {
		// A file that will not parse gets no rows. conn cannot tell
		// what is in it, so it cannot keep what it does not understand,
		// and a view offering to add a root would be offering a write
		// that will be refused; what there is to do is open the file.
		b.err = err.Error()
		return b
	}
	if _, source, _ := config.ResolveRoots(home); source == config.RootsEnv {
		b.forced = true
	}
	for i, r := range c.Roots {
		row := settingRow{kind: rootSetting, text: r, at: i}
		if s := config.RootStateOf(config.ExpandHome(strings.TrimSpace(r), home)); s.Problem != "" {
			row.note = strings.ToUpper(s.Problem)
		}
		b.rows = append(b.rows, row)
		b.roots++
	}
	b.rows = append(b.rows, settingRow{kind: addRootSetting, text: addRootRow})
	for _, t := range theme.All {
		row := settingRow{kind: themeSetting, text: t.Name}
		switch t.Name {
		case inUse:
			row.note = noteInUse
		case c.Theme:
			// The file names one and conn is wearing another: a flag
			// said otherwise on the way in. Both are said, the
			// difference being the thing somebody came here to see.
			row.note = noteInFile
		}
		b.rows = append(b.rows, row)
	}
	if _, ok := theme.Named(c.Theme); c.Theme != "" && !ok {
		b.unknownTheme = c.Theme
	}
	// The grounds, and under them the row that gives the choice back to
	// the terminal. It is a row like the others because it is a choice
	// like the others: what conn does about the ground is one question
	// with three answers, and two of them in a list with the third
	// somewhere else would be conn hiding its own default.
	named, isNamed := config.GroundNamed(c.Ground)
	for _, g := range grounds {
		row := settingRow{kind: groundSetting, text: g.text, value: g.value}
		switch {
		case g.value != "" && dark == (g.value == config.DarkGround):
			row.note = noteInUse
		case g.value != "" && isNamed && named == (g.value == config.DarkGround):
			row.note = noteInFile
		case g.value == "" && !isNamed:
			// The file names no ground conn knows, so the terminal is
			// asked, which is what this row says.
			row.note = noteInFile
		}
		b.rows = append(b.rows, row)
	}
	if !isNamed && c.Ground != "" {
		b.unknownGround = c.Ground
	}
	return b
}

// settingsHints is what the key bar says: the keys that work on the row
// under the cursor, and only those. The settings are worked in a pane
// of conn's own, so they write the bar themselves — the panel writes
// the rest of the line and leaves that position alone while they
// stand; see saying in settingsrun.go.
func settingsHints(rows []settingRow, at int) []keyHint {
	var hints []keyHint
	if len(rows) > 1 {
		hints = append(hints, moveHint)
	}
	if at < len(rows) {
		switch rows[at].kind {
		case rootSetting:
			hints = append(hints, keyHint{"enter", "Change it"}, keyHint{"x", "Take it out"})
		case addRootSetting:
			hints = append(hints, keyHint{"enter", "Add one"})
		case themeSetting:
			if rows[at].note != noteInUse {
				hints = append(hints, keyHint{"enter", "Wear it"})
			}
		case groundSetting:
			switch r := rows[at]; {
			case r.value == "" && r.note != noteInFile:
				// Nothing changes now: the terminal is asked when a
				// server rises, and one is up.
				hints = append(hints, keyHint{"enter", "For the next start"})
			case r.value != "" && r.note != noteInUse:
				hints = append(hints, keyHint{"enter", "Wear it"})
			}
		}
	}
	return append(hints, keyHint{"esc", "Back"})
}

// drawSettings renders the view for a pane of the given size, with the
// cursor on the given row.
func drawSettings(b settingsReport, cursor, width, height int, p draw.Palette) []draw.Row {
	// The header: the view, and the file it is of, since a setting that
	// is not doing what was meant is answered by opening that file and
	// the operator should not have to be told twice where it is.
	measure := draw.MeasureAt(max(width, draw.PanelMinCols))
	right := b.path
	if !b.present {
		right += " · NEW"
	}
	if w := measure - ansi.StringWidth("SETTINGS") - 2; ansi.StringWidth(right) > w {
		right = draw.Fit(right, w, true)
	}
	c, _ := draw.Head("SETTINGS", right, width, p)

	d := draw.Canvas{P: p, Width: c.Width}
	cursorRow := -1
	say := func(color, s string) {
		l := d.Line()
		l.Add(color, draw.Fit(s, measure, false))
		d.Emit(l, 0, false)
	}
	if b.err != "" {
		d.Blank(0)
		l := d.Line()
		l.Add(p.Chip, " "+strings.ToUpper(draw.Fit(b.err, measure-2, false))+" ")
		d.Emit(l, 0, false)
		if len(b.rows) == 0 {
			d.Blank(0)
			say(p.Gray, "CONN WILL NOT WRITE OVER A FILE IT CANNOT READ")
		}
	}
	kind := settingKind(-1)
	for i, r := range b.rows {
		if r.kind != kind && (kind != rootSetting || r.kind != addRootSetting) {
			// A heading for each setting, with what the file says of it
			// against the right: how many roots it names, and nothing
			// for the themes and the grounds, which are as many as conn
			// has either way.
			d.Blank(0)
			l := d.Line()
			switch r.kind {
			case themeSetting:
				l.Eyebrow(0, "THEME", measure, "")
			case groundSetting:
				l.Eyebrow(0, "GROUND", measure, "")
			default:
				l.Eyebrow(0, "ROOTS", measure, strconv.Itoa(b.roots))
			}
			d.Emit(l, 0, false)
			d.Blank(0)
			// What the file says of this setting that conn could not
			// use, under the heading it belongs to. The console says so
			// too, in a word; this is the view the operator came to to
			// put it right.
			switch {
			case r.kind == rootSetting || r.kind == addRootSetting:
				if b.forced {
					// The file is not what conn is walking. Said here
					// rather than left to the console, because this is
					// the view where somebody edits a root and waits
					// for the list to change.
					say(p.Gray, "CONN_ROOTS STANDS IN FRONT OF THESE")
					d.Blank(0)
				}
			case r.kind == themeSetting && b.unknownTheme != "":
				say(p.Gray, "CONN HAS NO THEME NAMED "+strings.ToUpper(b.unknownTheme))
				d.Blank(0)
			case r.kind == groundSetting && b.unknownGround != "":
				say(p.Gray, strings.ToUpper(b.unknownGround)+" IS NEITHER GROUND")
				d.Blank(0)
			}
		}
		kind = r.kind
		l := d.Line()
		if i == cursor {
			l.P = p.Chosen()
			if p.Plain {
				l.Mark = "▸"
			}
			cursorRow = len(d.Rows)
		}
		text, note := r.text, r.note
		if r.kind == themeSetting {
			text = strings.ToUpper(text)
		}
		room := measure
		if note != "" {
			room -= ansi.StringWidth(note) + 2
		}
		color := l.P.Ink
		if r.kind == addRootSetting {
			color = l.P.Gray
		}
		l.Add(color, draw.Fit(text, room, r.kind == rootSetting))
		// The note against the right, with the console's own leader
		// carrying the eye to it. On the panel the two were a glance
		// apart; the settings are worked in the workspace now, and a
		// name at one edge with IN USE at the other and eighty columns
		// of nothing between is not a row. It is the mark every fact
		// the console and the readout state already wears.
		if note != "" {
			at := measure - ansi.StringWidth(note)
			l.Add("", " ")
			l.Add(l.P.Faint, strings.Repeat(".", max(at-l.Cells-1, 1)))
			l.To(at)
			l.Add(l.P.Gray, note)
		}
		d.Emit(l, 0, false)
	}

	return c.Foot(d.Rows, cursorRow, height)
}
