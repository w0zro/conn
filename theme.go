package main

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// How conn looks, in one place: the palette names every color by its job, the
// glyphs are the marks drawn beside and between the words, and the styles the
// rest of the code reads are derived from them. An appearance change is a
// change here; the render code asks for roles, not colors.
//
// The ground is warm graphite — a hangar at night, kin to the manual's paper
// by temperature rather than tint. Most of the screen is ink and gray; hue
// is spent on state and on focus, so a glance finds what changed. One
// orange serves three jobs on purpose — the brand, the focus edge, the
// answer owed — because all three mean "here". conn draws these colors
// itself, in truecolor, and hands tmux the same values for the status
// line, the borders and the popups, so the window is one palette by
// construction whatever the terminal's own is.

// The palette, by the job each color does here.
const (
	colorGround = "#15130F" // the screen itself
	colorBar    = "#100E0B" // the tabline's ground, a step under the screen
	colorWash   = "#1D1A15" // overlays, the status line: one step off the ground
	colorChip   = "#2A2620" // the selection bar, chips, borders: two steps off
	colorBorder = "#3A342A" // an overlay's edge

	colorInk       = "#E6DFD0" // content at full weight
	colorGray      = "#8B8272" // labels, facts, asides
	colorFaint     = "#5C564A" // hints, dead output, idle: text a reader may skip
	colorParchment = "#BFB39A" // headings, place names

	colorOrange = "#E85D2F" // the brand, the focus edge, the cursor, the CONN chip
	colorOwed   = "#FF7847" // an ask, a failure, a stop, a death in progress
	colorAmber  = "#E3A94F" // working: an agent mid-turn
	colorGreen  = "#93C98B" // done and waiting on you; ended well
	colorTeal   = "#7FC7BD" // identity: branches, ports, containers, env names
)

// The styles are package-wide because everything drawing reads them.
var (
	hintStyle, ruleStyle, itemStyle, selStyle     lipgloss.Style
	faintStyle, labelStyle, errStyle, busyStyle   lipgloss.Style
	attnStyle, blockedStyle, headingStyle         lipgloss.Style
	titleStyle                                    lipgloss.Style
	offSelStyle, noteStyle, matchStyle, selfStyle lipgloss.Style
	placeStyle, tealStyle, orangeStyle            lipgloss.Style

	// The grounds: the tabline's bar, the screen, and the selection bar.
	barStyle, groundStyle, chipStyle lipgloss.Style
	// The block cursor: reversed ink.
	cursorStyle lipgloss.Style
)

func init() { applyStyles() }

// applyStyles builds every style from the palette.
func applyStyles() {
	fg := func(c string) lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color(c)) }

	itemStyle = fg(colorInk)
	// Structure is bold: headings and place names in parchment, the
	// cursor's row in the orange that means here.
	headingStyle = fg(colorParchment).Bold(true)
	titleStyle = headingStyle
	placeStyle = headingStyle
	selStyle = fg(colorOrange).Bold(true)
	orangeStyle = fg(colorOrange)
	// offSelStyle marks the cursor's row when that row is one conn cannot
	// step into: bold enough to find, dim enough to still read as unavailable.
	offSelStyle = fg(colorFaint).Bold(true)

	// Gray carries everything true but secondary; faint is for what a
	// reader may skip. conn's own asides are italic, set apart from content.
	faintStyle = fg(colorFaint)
	labelStyle = fg(colorGray)
	hintStyle = fg(colorGray)
	noteStyle = fg(colorGray).Italic(true)
	// A process that is conn itself reads in gray: (me), and nothing more.
	selfStyle = fg(colorGray)

	// The rules that separate, never speak.
	ruleStyle = fg(colorChip)

	// The states. busyStyle turns beside work in progress; attnStyle marks an
	// agent that is done and waiting on its user; blockedStyle one stopped
	// mid-turn on a specific ask, holding up work already in flight; and
	// errStyle a failure, a stop, a death.
	busyStyle = fg(colorAmber)
	attnStyle = fg(colorGreen).Bold(true)
	blockedStyle = fg(colorOwed).Bold(true)
	errStyle = fg(colorOwed)
	tealStyle = fg(colorTeal)

	// matchStyle lights the letters a query matched, inside whatever style
	// the row otherwise has: a narrowed list always shows why it narrowed.
	matchStyle = fg(colorOrange).Bold(true)

	barStyle = lipgloss.NewStyle().Background(lipgloss.Color(colorBar))
	groundStyle = lipgloss.NewStyle().Background(lipgloss.Color(colorGround))
	chipStyle = lipgloss.NewStyle().Background(lipgloss.Color(colorChip))
	cursorStyle = lipgloss.NewStyle().Background(lipgloss.Color(colorInk)).Foreground(lipgloss.Color(colorGround))

	toneStyles = map[tone]lipgloss.Style{
		tonePlain:  itemStyle,
		toneGood:   fg(colorGreen),
		toneAttn:   fg(colorAmber),
		toneUrgent: fg(colorOwed).Bold(true),
		toneBad:    fg(colorOwed),
		toneAccent: fg(colorTeal),
		toneQuiet:  faintStyle,
		toneName:   itemStyle,
		toneCount:  fg(colorTeal),
		toneSelf:   fg(colorGray),
	}
}

// tone is how a value reads. Most facts are plain; the few that carry a
// state carry it in the same colors as the marks, and the ones that are
// true but secondary recede.
type tone int

const (
	tonePlain  tone = iota // content, read at full weight
	toneGood               // alive and well: running, working, passed
	toneAttn               // worth a glance: a dirty tree, a suspicious value
	toneUrgent             // holding up work: blocked on an ask, a failure named
	toneBad                // wrong: a zombie, a failure
	toneAccent             // identity worth picking out: a branch, a port, a name
	toneQuiet              // true but secondary: ids, urls, hints
	toneName               // what a thing is called: a model, a command, a state
	toneCount              // a measure: tokens, a share of the machine, a count
	toneSelf               // your own words
)

// toneStyles is the color each tone reads in.
var toneStyles map[tone]lipgloss.Style

// tmuxPalette is what tmux draws with: the status line, the borders, the
// popups, and the ground under every pane. The same values conn draws with.
type tmuxPalette struct {
	ground, bar, wash, chip, border string
	ink, gray, faint, parchment     string
	orange, owed, amber, green      string
	teal                            string
}

var hangar = tmuxPalette{
	ground: colorGround, bar: colorBar, wash: colorWash, chip: colorChip, border: colorBorder,
	ink: colorInk, gray: colorGray, faint: colorFaint, parchment: colorParchment,
	orange: colorOrange, owed: colorOwed, amber: colorAmber, green: colorGreen,
	teal: colorTeal,
}

// tp is the palette tmux draws with.
var tp = hangar

// paintShells says whether the shells' ground is the hangar's too: their
// default background and ink, so the window is one ground from the tabline
// to the status line. The config's "terminal" theme leaves the shells to
// the terminal's own colors, and conn's own panes keep the hangar.
var paintShells = true

// applyTheme reads the config's theme: "terminal" keeps the shells' ground
// the terminal's; anything else is the hangar, which is the one palette
// conn has.
func applyTheme(theme string) {
	paintShells = theme != "terminal"
}

// brandChip is conn's name at the head of the status line: CONN, bold in
// the ground's color on the orange, the mode's chip butted against it. It
// is the one inverted ground on screen — it says whose line this is, and
// it stays while everything after it changes.
func brandChip() string {
	return "#[fg=" + tp.ground + ",bg=" + tp.orange + ",bold] CONN "
}

// statusChip is a mode on the status line: the word, bold in its color on
// the chip's ground, and after it the rest of the line washed one step off
// the ground — one tone for every mode, the chip alone carrying the color.
// The word's # are doubled: tmux expands them otherwise.
func statusChip(color, word string) string {
	return "#[fg=" + color + ",bg=" + tp.chip + ",bold] " +
		strings.ReplaceAll(word, "#", "##") +
		" #[fg=" + tp.gray + ",bg=" + tp.wash + ",fill=" + tp.wash + "]"
}

// tmuxStyled wraps text for tmux's status line: its color and weight in
// tmux's own style syntax, reset after. tmux expands a # in a format,
// so the ones in the text are doubled.
func tmuxStyled(fg string, bold bool, text string) string {
	style := "#[fg=" + fg
	if bold {
		style += ",bold"
	}
	return style + "]" + strings.ReplaceAll(text, "#", "##") + "#[default]"
}

// The glyphs: one mark per meaning, everywhere it appears — a tab, a row, a
// heading, an ending. Filled is lit; hollow is quiet.
const (
	glyphSelected  = "▸"  // the cursor, in a gutter of its own
	glyphIndent    = "  " // a child sits on indent alone: the tree's rules were the loudest thing on screen
	glyphOn        = "●"  // done and waiting on you
	glyphOff       = "○"  // the same thing, quiet: idle since it started
	glyphAsk       = "◆"  // an ask — an answer owed
	glyphFailed    = "✗"  // ended badly; stopped; a zombie
	glyphDone      = "✓"  // ended well
	glyphContainer = "⬢"  // a container, merged in beside the processes
	glyphBusy      = "⠹"  // the spinner, standing still: for a title that is not redrawn per frame
	glyphJoin      = "›"  // the prompt line, and the joins of a run
	glyphNote      = "←"  // leads an annotation
	glyphDot       = "·"  // joins the facts of a line
)

// gutter is the room every line of a buffer or a view keeps off the left
// edge: the buffer is a page, and a page gets a margin.
const gutter = "  "

// paneGutter is the same margin, by the name the older render code uses.
const paneGutter = gutter

// spinFrames is the turning marker beside work in progress: a process being
// killed, an agent mid-turn.
var spinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// dots joins facts on a line with middots: pid 4402 · :3000 · 2h. Empty
// facts are left out.
func dots(facts ...string) string {
	var out []string
	for _, f := range facts {
		if f != "" {
			out = append(out, f)
		}
	}
	return strings.Join(out, " "+glyphDot+" ")
}
