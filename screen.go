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
// checks are summed and conn calls hello. It is monochrome, in whatever
// the terminal's phosphor is; the name, the call and a fault stand out.

// bright, alarm and normal are the attributes the screen uses, and are
// empty off a terminal.
var bright, alarm, normal string

// The smallest terminal the screen is laid out for, and the rows the
// footer takes, which come on last.
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
	if width < minCols {
		width = minCols
	}
	if height < minRows {
		height = minRows
	}
	inner := width - 2

	var rows []string
	rule := func(l, m, rt string) {
		rows = append(rows, l+strings.Repeat(m, inner)+rt)
	}
	// line frames text between the side rules at a column from the left
	// rule; cells is the text's width without its attributes. Negative,
	// the column centers the text.
	line := func(text string, cells, col int) {
		if col < 0 {
			col = (inner - cells) / 2
		}
		right := inner - col - cells
		if right < 0 {
			right = 0
		}
		rows = append(rows, "║"+strings.Repeat(" ", col)+text+strings.Repeat(" ", right)+"║")
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
	identCol := leftCol + nameW + 8
	identW := 0
	for _, s := range ident {
		identW = max(identW, utf8.RuneCountInString(s))
	}
	beside := identCol+identW <= inner-leftCol
	for i, l := range name {
		text, cells := bright+l+normal, nameW
		if j := i - 1; beside && j >= 0 && j < len(ident) && ident[j] != "" {
			pad := strings.Repeat(" ", identCol-leftCol-nameW)
			text += pad + ident[j]
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
	// facts go first, then what is left is cut above the footer.
	var body []string
	entry := func(it item) {
		label := strings.ToUpper(it.label) + " "
		leader := strings.Repeat(".", max(labelCol-leftCol-utf8.RuneCountInString(label), 1))
		valueW := inner - labelCol - 1
		text := label + leader + " "
		if it.status != "" {
			valueW -= statusCol + 2
		}
		value := fit(strings.ToUpper(it.value), valueW)
		text += value
		cells := utf8.RuneCountInString(text)
		if it.status != "" {
			gap := inner - leftCol - cells - statusCol
			text += " " + strings.Repeat(".", max(gap-2, 1)) + " "
			status := strings.ToUpper(it.status)
			if it.fault {
				status = alarm + status + normal
			}
			text += status
			cells = inner - leftCol - statusCol + utf8.RuneCountInString(strings.ToUpper(it.status))
		}
		body = append(body, "║"+strings.Repeat(" ", leftCol)+text+strings.Repeat(" ", max(inner-leftCol-cells, 0))+"║")
	}
	section := func(title string, items []item) {
		body = append(body, "║"+strings.Repeat(" ", inner)+"║")
		body = append(body, "║"+strings.Repeat(" ", leftCol)+title+strings.Repeat(" ", inner-leftCol-utf8.RuneCountInString(title))+"║")
		body = append(body, "║"+strings.Repeat(" ", inner)+"║")
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
	sum := "ALL SYSTEMS NOMINAL"
	if faults == 1 {
		sum = alarm + "1 SYSTEM NOT NOMINAL" + normal
	} else if faults > 1 {
		sum = alarm + strconv.Itoa(faults) + " SYSTEMS NOT NOMINAL" + normal
	}
	rule("╠", "═", "╣")
	blank()
	line(sum, utf8.RuneCountInString(strings.ReplaceAll(strings.ReplaceAll(sum, alarm, ""), normal, "")), -1)
	call := "***  " + strings.ToUpper(greeting) + "  ***"
	line(bright+call+normal, utf8.RuneCountInString(call), -1)
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
