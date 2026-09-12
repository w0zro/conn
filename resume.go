package main

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// The picker: a place's suspended conversations, filtered the way the
// project list is. A row is what a reader would recognize a
// conversation by — its branch and the last thing it was asked — with
// how long since it last moved against the right. Enter continues the
// one under the cursor, in a shell like any other; esc abandons the
// look without opening anything.

// resumeReport is the picker's words as things stand. A directory the
// picker looked under and found nothing in — no conversations had, or
// none yet read — is not an error; there is none for the picker to say.
type resumeReport struct {
	place   string // the place it was opened on, tilde'd
	home    string
	now     time.Time
	loading bool
	rows    []conversation
	total   int
	filter  string
}

// composeResume words the picker: the filter's rows out of the whole
// place found.
func composeResume(convos []conversation, place, filter, home string, now time.Time, loading bool) resumeReport {
	return resumeReport{
		place: tilde(place, home), home: home, now: now, loading: loading,
		rows: matchingConvos(convos, filter), total: len(convos), filter: filter,
	}
}

// matchingConvos is the conversations a filter leaves: one answers by
// its branch, the last thing it was asked, or the directory it was had
// in, the same three a reader would recognize it by.
func matchingConvos(cs []conversation, filter string) []conversation {
	f := strings.ToLower(strings.TrimSpace(filter))
	if f == "" {
		return cs
	}
	var out []conversation
	for _, c := range cs {
		if strings.Contains(strings.ToLower(c.Prompt), f) ||
			strings.Contains(strings.ToLower(c.Branch), f) ||
			strings.Contains(strings.ToLower(c.Dir), f) {
			out = append(out, c)
		}
	}
	return out
}

// branchW is the picker's one column of its own: the branch, from the
// margin. The age takes the measure's own ageW, flush with the right,
// the way it does on the watch; the prompt takes what is left between
// them.
const branchW = 14

// drawResume renders the picker for a terminal of the given size, with
// the cursor on the given row.
func drawResume(b resumeReport, cursor, width, height int, p palette) []row {
	width = max(width, railMinCols)
	measure, _, _ := columns(width)
	c := canvas{p: p, width: width}

	// The header: the name of the view, and against the right the count
	// — of everything found at the place, or of what the filter left out
	// of it.
	c.blank(0)
	l := c.line()
	l.add(p.orange+p.bold, "RESUME")
	right := strconv.Itoa(b.total) + " SUSPENDED"
	switch {
	case b.loading && b.total == 0:
		right = "" // nothing has been found yet, and none is not a count
	case b.filter != "":
		right = strconv.Itoa(len(b.rows)) + " OF " + strconv.Itoa(b.total)
	}
	l.to(measure - utf8.RuneCountInString(right))
	l.add(p.gray, right)
	c.emit(l, 0, false)
	c.rule(0, measure)

	// The place it is for, the way a place titles its block on the watch.
	l = c.line()
	l.add(p.parchment+p.bold, fit(b.place, measure, true))
	c.emit(l, 0, false)

	// The line typed into: the word, and the filter with the caret after
	// it, so it is plain that the keys go here.
	l = c.line()
	l.add(p.gray, "FIND")
	l.to(findW)
	l.add(p.ink+p.bold, fit(b.filter, measure-findW-1, false))
	l.add(p.orange+p.bold, caret)
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
		l := d.line()
		l.add(color, s)
		d.emit(l, 0, true)
		body = d.rows
	}
	switch {
	case b.loading && len(b.rows) == 0:
		say(p.gray, "LOOKING")
	case len(b.rows) == 0 && b.filter != "":
		say(p.gray, "NOTHING ANSWERS TO "+strings.ToUpper(b.filter))
	case len(b.rows) == 0:
		say(p.gray, "NOTHING SUSPENDED HERE")
	default:
		d.blank(0)
		ageCol := measure - ageW
		promptW := ageCol - 1 - branchW
		for i, cv := range b.rows {
			l := d.line()
			if i == cursor {
				l.p = p.chosen()
				if p.plain {
					l.mark = "▸"
				}
				cursorRow = len(d.rows)
			}
			l.add(p.gray, fit(cv.Branch, branchW-1, false))
			l.to(branchW)
			// A conversation with nothing read of it is named by where it
			// was had; one with a prompt is named by that instead, since it
			// is the more of the two a reader would recognize it by.
			prompt, path := cv.Prompt, false
			if prompt == "" {
				prompt, path = tilde(cv.Dir, b.home), true
			}
			l.add(p.ink, fit(prompt, promptW, path))
			l.to(ageCol)
			l.add(p.gray, age(cv.When, b.now))
			d.emit(l, 0, false)
		}
		body = d.rows
	}
	c.rows = append(c.rows, scrolled(body, cursorRow, room-len(c.rows), width, p)...)

	// The ground fills what the rows do not, as on the list.
	if height > 0 {
		for len(c.rows) < height {
			c.blank(0)
		}
	}
	return c.rows
}
