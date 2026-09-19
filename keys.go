package main

import (
	"strings"
	"unicode/utf8"
)

// The keys, on the screen. ? used to hand the workspace to the manual
// and leave the panel standing on a view that could not be worked
// while it was up — the page nobody opens for a key they do not know
// is there, beside a list of processes going nowhere. The panel is
// where the keys are pressed, so while the manual is up it holds the
// keys themselves: every one the panel answers to, by what it acts on,
// with the prose beside it for the ones that want a sentence.
//
// It is the key bar's rows again, without the bar's one rule: the bar
// says the keys that work on the row under the cursor and nothing
// else, and the card says them all, because a key not offered is still
// a key the operator is learning the station by.

// A keyGroup is a heading and the keys under it.
type keyGroup struct {
	title string
	keys  []keyHint
}

// panelKeys is every key conn answers to, by what it acts on: the
// cursor, the row it is on, the station, and the keys that work from
// inside a process, which are the panel key and one after it. px is
// the panel key as the bar writes it, since it is the operator's to
// set and a card naming ctrl-space on a station keyed to something
// else would be a card that lies.
func panelKeys(px string) []keyGroup {
	return []keyGroup{{
		"MOVING", []keyHint{
			{"j k", "a row"},
			{"gg G", "first row, last row"},
			{"tab", "next waiting"},
			{"z", "the whole tree"},
		},
	}, {
		"THE ROW", []keyHint{
			{"enter", "go in"},
			{"esc", "back where you were"},
			{"x", "end it"},
			{"s", "a shell there"},
			{"S", "its own client"},
			{"a", "a new contact"},
			{"A", "its sessions"},
			{"u U", "bring up, bring up all"},
		},
	}, {
		"THE STATION", []keyHint{
			{"p", "the projects"},
			{"c", "the console"},
			{"?", "these keys"},
			{"q", "detach"},
		},
	}, {
		"IN A PROCESS", []keyHint{
			{px, "the panel"},
			{px + " " + px, "the last process"},
			{px + " tab", "next waiting"},
			{px + " ?", "these keys"},
		},
	}}
}

// keysColumn is where a group's words start: past the widest run of
// keys in it, and a gap. The keys are a column of their own so the eye
// runs down the presses and the words line up beside them, and the
// column is the group's rather than the card's — the keys pressed
// inside a process are the panel key and one after it, and a card set
// to those would push every single-letter key's word half the panel
// out and take the words' room with it.

// keysWidth is the cells a hint's keys take: each key its own, since a
// hint is either two keys that do the same thing — j k — or two
// pressed in turn, and both read as two.
func keysWidth(keys string) int {
	w := 0
	for i, k := range strings.Fields(keys) {
		if i > 0 {
			w++
		}
		w += keyWidth(k)
	}
	return w
}

// drawKeys renders the card: the view the keys belong to against the
// right, and under a rule the groups, a heading and its keys each.
func drawKeys(groups []keyGroup, view string, width, height int, p palette) []row {
	width = max(width, panelMinCols)
	measure := measureAt(width)
	c := canvas{p: p, width: width}

	c.blank(0)
	l := c.line()
	l.add(p.orange+p.bold, "KEYS")
	right := strings.ToUpper(view)
	l.to(measure - utf8.RuneCountInString(right))
	l.add(p.gray, right)
	c.emit(l, 0, false)
	c.rule(0, measure)

	d := canvas{p: p, width: width}
	for _, g := range groups {
		col := 0
		for _, h := range g.keys {
			col = max(col, keysWidth(h.key)+3)
		}
		d.blank(0)
		l := d.line()
		l.eyebrow(0, g.title, measure, "")
		d.emit(l, 0, false)
		d.blank(0)
		for _, h := range g.keys {
			l := d.line()
			l.to(1)
			for i, k := range strings.Fields(h.key) {
				if i > 0 {
					l.add("", " ")
				}
				l.key(k)
			}
			l.to(col)
			l.add(p.ink, fit(h.does, measure-l.cells, false))
			d.emit(l, 0, false)
		}
	}

	room := height
	if height == 0 {
		room = 1 << 30
	}
	c.rows = append(c.rows, scrolled(d.rows, -1, room-len(c.rows), width, p)...)
	if height > 0 {
		for len(c.rows) < height {
			c.blank(0)
		}
	}
	return c.rows
}
