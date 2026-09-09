package main

import (
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The start-up screen is a system screen of the old kind, filling the
// terminal: ruled in double lines into a header, where the name is set
// large beside the station's identification; a body, where the machine
// is read out and the checks come up nominal; and a footer, where the
// checks are summed and conn calls hello.

// The palette is the site's: the desk the screen is painted on, paper
// for the text, faint and muted for the labels and the leaders, orange
// for the name and for a fault, and green for a check come up nominal.
// Off a terminal every sequence is empty and the screen is plain text.
type palette struct {
	ground, paper, faint, muted, orange, alarm, good, bold string
	normal, end                                            string // back to paper on the desk; the row's end
}

var (
	plain palette
	pal   = plain
)

func colored() palette {
	p := palette{
		ground: "\x1b[48;2;25;27;31m",
		paper:  "\x1b[38;2;241;235;222m",
		faint:  "\x1b[38;2;141;132;116m",
		muted:  "\x1b[38;2;111;102;86m",
		orange: "\x1b[38;2;189;58;29m",
		alarm:  "\x1b[48;2;189;58;29m\x1b[38;2;241;235;222m",
		good:   "\x1b[38;2;46;125;79m",
		bold:   "\x1b[1m",
		end:    "\x1b[0m",
	}
	p.normal = p.end + p.ground + p.paper
	return p
}

// The smallest terminal the screen is laid out for, the rows the footer
// takes, which come on last, and the columns the body is set to.
const (
	minCols    = 80
	minRows    = 24
	footerRows = 6
	leftCol    = 3  // where a line starts, from the left rule
	labelCol   = 16 // where a value starts, from the left rule
	statusCol  = 12 // the status column's width, against the right rule
)

// screen renders the start-up screen for a terminal of the given size:
// its rows, each the terminal's width, as many as its height.
func screen(r report, width, height int) []string {
	terminal := terminalCheck(r.term, width, height)
	width = max(width, minCols)
	height = max(height, minRows)
	inner := width - 2
	p := pal

	var rows []string
	rule := func(l, m, rt string) {
		rows = append(rows, p.normal+p.faint+l+strings.Repeat(m, inner)+rt+p.end)
	}
	// line frames text between the side rules at a column from the left
	// rule; cells is the text's width without its colors. Negative, the
	// column centers the text.
	line := func(text string, cells, col int) {
		if col < 0 {
			col = (inner - cells) / 2
		}
		right := max(inner-col-cells, 0)
		rows = append(rows, p.normal+p.faint+"║"+p.normal+strings.Repeat(" ", col)+text+p.normal+strings.Repeat(" ", right)+p.faint+"║"+p.end)
	}
	blank := func() { line("", 0, 0) }
	fit := func(s string, w int) string {
		if utf8.RuneCountInString(s) <= w {
			return s
		}
		if w <= 1 {
			return ""
		}
		return string([]rune(s)[:w-1]) + "…"
	}
	// paint is text in a color, back to paper after; width is the
	// text's own.
	paint := func(color, text string) string { return color + text + p.normal }

	// The header: the name, and beside it — under it, when the terminal
	// is too narrow for beside — who and what this is.
	rule("╔", "═", "╗")
	blank()
	name := letters(nameSet)
	nameW := utf8.RuneCountInString(name[0])
	ident := []string{
		"CONN " + r.version,
		r.build,
		"STATION  " + strings.ToUpper(r.station),
		r.clock,
	}
	identColor := []string{p.bold, p.faint, p.paper, p.paper}
	identCol := leftCol + nameW + 8
	identW := 0
	for _, s := range ident {
		identW = max(identW, utf8.RuneCountInString(s))
	}
	beside := identCol+identW <= inner-leftCol
	for i, l := range name {
		text, cells := paint(p.orange+p.bold, l), nameW
		if j := i - 1; beside && j >= 0 && j < len(ident) && ident[j] != "" {
			pad := strings.Repeat(" ", identCol-leftCol-nameW)
			text += pad + paint(identColor[j], ident[j])
			cells += len(pad) + utf8.RuneCountInString(ident[j])
		}
		line(text, cells, leftCol)
	}
	if !beside {
		blank()
		one := fit(strings.Join(ident, "  ·  "), inner-2*leftCol)
		line(one, utf8.RuneCountInString(one), leftCol)
	}
	blank()
	rule("╠", "═", "╣")

	// The body: the facts, then the checks. When the terminal is short the
	// facts are cut first, from the end, and the checks kept whole.
	var body []string
	entry := func(it item) {
		label := strings.ToUpper(it.label) + " "
		leader := strings.Repeat(".", max(labelCol-leftCol-utf8.RuneCountInString(label), 1)) + " "
		valueW := inner - leftCol - utf8.RuneCountInString(label+leader)
		if it.status != "" {
			valueW -= statusCol + 3
		}
		value := fit(strings.ToUpper(it.value), valueW)
		text := paint(p.faint, label) + paint(p.muted, leader) + value
		cells := utf8.RuneCountInString(label + leader + value)
		if it.status != "" {
			status := strings.ToUpper(it.status)
			gap := strings.Repeat(".", max(inner-leftCol-cells-statusCol-2, 1))
			color := p.faint
			switch {
			case it.fault:
				color = p.alarm + p.bold
				status = " " + status + " "
			case it.status == nominal:
				color = p.good + p.bold
			}
			text += " " + paint(p.muted, gap) + " " + paint(color, status)
			cells += 2 + len(gap) + utf8.RuneCountInString(status)
		}
		body = append(body, p.normal+p.faint+"║"+p.normal+strings.Repeat(" ", leftCol)+text+p.normal+strings.Repeat(" ", max(inner-leftCol-cells, 0))+p.faint+"║"+p.end)
	}
	section := func(title string, items []item) {
		body = append(body, p.normal+p.faint+"║"+p.normal+strings.Repeat(" ", inner)+p.faint+"║"+p.end)
		body = append(body, p.normal+p.faint+"║"+p.normal+strings.Repeat(" ", leftCol)+paint(p.bold, title)+strings.Repeat(" ", inner-leftCol-utf8.RuneCountInString(title))+p.faint+"║"+p.end)
		body = append(body, p.normal+p.faint+"║"+p.normal+strings.Repeat(" ", inner)+p.faint+"║"+p.end)
		for _, it := range items {
			entry(it)
		}
	}
	checks := append([]item{terminal}, r.checks...)
	room := max(height-len(rows)-footerRows, 0)
	section("START-UP CHECKS", checks)
	checkRows := body
	body = nil
	if facts := room - len(checkRows); facts >= 4 {
		section("SYSTEM", r.facts)
		if len(body) > facts {
			body = body[:facts]
		}
	}
	body = append(body, checkRows...)
	if len(body) > room {
		body = body[:room]
	}
	rows = append(rows, body...)
	for len(rows) < height-footerRows {
		blank()
	}

	// The footer: the sum of the checks, and the call.
	faults := 0
	for _, c := range checks {
		if c.fault {
			faults++
		}
	}
	sum, sumColor := "ALL SYSTEMS NOMINAL", p.good+p.bold
	switch {
	case faults == 1:
		sum, sumColor = " 1 SYSTEM NOT NOMINAL ", p.alarm+p.bold
	case faults > 1:
		sum, sumColor = " "+strconv.Itoa(faults)+" SYSTEMS NOT NOMINAL ", p.alarm+p.bold
	}
	rule("╠", "═", "╣")
	blank()
	line(paint(sumColor, sum), utf8.RuneCountInString(sum), -1)
	call := strings.ToUpper(greeting)
	line(paint(p.orange, "***")+"  "+paint(p.bold, call)+"  "+paint(p.orange, "***"), utf8.RuneCountInString(call)+10, -1)
	blank()
	rule("╚", "═", "╝")
	return rows
}

// terminalCheck is the terminal itself: what it calls itself, its size,
// and whether the screen fits it.
func terminalCheck(term string, width, height int) item {
	value := strings.TrimSpace(strings.ToUpper(term) + "  " + strconv.Itoa(width) + " X " + strconv.Itoa(height))
	if width < minCols || height < minRows {
		return item{label: "TERMINAL", value: value, status: "SMALL", fault: true}
	}
	return item{label: "TERMINAL", value: value, status: nominal}
}

// stdoutIsTerminal says whether what conn prints is going to a person's
// screen, or to a pipe or file.
func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
