package main

import (
	"fmt"
	"image/color"
)

// A theme is everything conn dresses a server in, by name: the ground
// and the ink, the sixteen a program asks for by name, and the roles
// conn draws by meaning, each on a dark ground and a light one. conn is
// the theme conn has always worn, and the one it wears unless told
// another; see mode.go for how a ground is chosen.

// A ground is a theme on one of its two grounds.
type ground struct {
	ground, ink color.RGBA // the pane's ground, and ordinary text on it
	scheme      [16]string // what a program asks for by name; a slot, and never read for a role

	// The roles: what conn draws by meaning. Each is a hex of its own,
	// whether or not a slot happens to hold the same one.
	accent    string // "you, here": the cursor, a chip, a title, the caret, a block on the status line
	shimmer   string // the accent's brighter cousin, for Claude Code to shimmer with
	border    string // a pane's edge, a selection, the band behind the status line
	surface   string // one step off the ground, short of the border: the panel's own ground
	running   string // a process doing something, said by the dot at the head of its row
	gray      string // the second rank: a label, a comment
	faint     string // the quietest text: a leader, a hint, a line number
	parchment string // the second ink: a title, punctuation, what conn says on the status line

	// The grounds no slot has a name for: the band behind what you said
	// to Claude Code, at rest and under the pointer; the bar behind a
	// tool's output, which is also the step off the ground a colorscheme
	// lifts a float onto; and the washes a diff is laid on, one step up
	// for the words inside it, and dimmed.
	messageBg, messageHoverBg, toolBg                                                        string
	diffAddedBg, diffRemovedBg, diffAddedDim, diffRemovedDim, diffAddedWord, diffRemovedWord string
}

// A theme is a name and its two grounds.
type theme struct {
	name        string
	dark, light ground
}

// themes is every theme conn has, in the order they are offered.
var themes = []theme{connTheme, datumTheme}

// defaultTheme is the one conn wears unless told another.
const defaultTheme = "conn"

// themeNamed is the theme by that name, and whether conn has one.
func themeNamed(name string) (theme, bool) {
	for _, t := range themes {
		if t.name == name {
			return t, true
		}
	}
	return theme{}, false
}

// on is the theme on one ground: dark, or light.
func (t theme) on(dark bool) ground {
	if dark {
		return t.dark
	}
	return t.light
}

// rgb is a color as a table writes it, #RRGGBB, as the terminal is
// asked to take it. A table is read at start and by the tests, so a
// hex that will not parse stops conn there rather than drawing black.
func rgb(h string) color.RGBA {
	var c color.RGBA
	if _, err := fmt.Sscanf(h, "#%02X%02X%02X", &c.R, &c.G, &c.B); err != nil {
		panic(fmt.Sprintf("not a color: %q", h))
	}
	c.A = 255
	return c
}

// connTheme is conn's own. Dark is every terminal it ever knew; light
// is for the terminal that says its own ground is light when conn asks.
//
// Light is not dark with the lightness flipped. On paper, emphasis is
// more ink, not more light, so the light scheme's bright slots go
// darker than its normal ones - the opposite of the dark scheme, where
// bright is lighter.
//
// The two neutral slots are the exception, and keep what every program
// means by them: 0 is black, which on paper is ordinary text and the
// strongest ink there is, and 8 is the gray a program dims with. They
// are not a border and a quieter border; a program writing ANSI-0
// expects to be read.
//
// Most of the sixteen are the console's own tokens: the two oranges
// for the reds, the parchment and the ink for the whites, the faint
// for bright black, the border for black. The blue and the magenta are
// conn's own, added so that the slots a shell theme leans on -
// structure, type, what can be run - stay apart from one another
// instead of collapsing into the orange and the teal. Normal, then
// bright.
//
// The border, and the grounds that go with it - a pane's edge, a
// selection, the band behind what you said - is scheme[0] on dark,
// which is the darkest thing there is and so the quietest edge. On
// light it cannot be: light's scheme[0] is black, and black is what a
// program writing ANSI-0 means by ordinary text; Claude Code writes the
// unchanged lines of a diff in it. A border pale enough to be an edge
// on paper is #D8D0BD, which is 1.27:1 against the light ground and
// unreadable as text, so the two part company here rather than in the
// sixteen.
//
// The faint is the quietest tier conn draws text in - a hint, a leader,
// Claude Code's subtle and promptBorder. Dark matches scheme[8], the
// ANSI-8 slot faint has always drawn from; light needed a color of its
// own, since ANSI-8 there (#9A9080) reads fine as a background tint but
// nearly vanishes as foreground text on the light ground. scheme[8]
// itself is untouched either way: a pane still gets exactly the ANSI-8
// it always did.
//
// The parchment is the second ink, scheme[7] on both grounds, and the
// shimmer is scheme[1], the red the orange lifts to; each is read by
// name all the same, since a slot is what a program asks for and a
// role is what conn means, and a theme whose slot 7 is white has a
// second ink still.
//
// The washes and bars are in the ground's own temperature, none of
// them a fill. A wash this pale on light needs more room from the
// ground than the same wash does on dark to read as a color at all
// rather than a shade of the ground itself - lightness compresses
// toward white long before it compresses toward black. Each light one
// was chosen by holding it to at least the contrast its dark
// counterpart already has against its own ground (WCAG ratio; e.g.
// dark's diffAddedWord is 1.65:1 against its ground, light's #C2D4B0
// was only 1.30:1 against light's - #A4C187 is what 1.65:1 costs on
// the same hue).
var connTheme = theme{
	name: "conn",
	dark: ground{
		ground: color.RGBA{R: 21, G: 19, B: 15, A: 255},
		ink:    color.RGBA{R: 230, G: 223, B: 208, A: 255},
		scheme: [16]string{
			"#2A2620", // black
			"#FF7847", // red
			"#93C98B", // green
			"#E3A94F", // yellow
			"#7FA7C9", // blue
			"#C98BA8", // magenta
			"#7FC7BD", // cyan
			"#BFB39A", // white
			"#5C564A", // bright black
			"#E85D2F", // bright red
			"#A8DBA0", // bright green
			"#F2C06E", // bright yellow
			"#9BBEDB", // bright blue
			"#DBA6C0", // bright magenta
			"#9AD9D0", // bright cyan
			"#E6DFD0", // bright white
		},
		accent:    "#E85D2F",
		shimmer:   "#FF7847",
		border:    "#2A2620",
		surface:   "#1D1A15",
		running:   "#93C98B",
		gray:      "#8B8272",
		faint:     "#5C564A",
		parchment: "#BFB39A",

		messageBg:       "#2A2620",
		messageHoverBg:  "#33302A",
		toolBg:          "#1D1A15",
		diffAddedBg:     "#1E2A1C",
		diffRemovedBg:   "#331F17",
		diffAddedDim:    "#191F17",
		diffRemovedDim:  "#231A14",
		diffAddedWord:   "#2C4028",
		diffRemovedWord: "#4A2A1D",
	},
	light: ground{
		ground: color.RGBA{R: 0xEF, G: 0xE9, B: 0xDB, A: 255},
		ink:    color.RGBA{R: 0x1A, G: 0x16, B: 0x11, A: 255},
		scheme: [16]string{
			"#2B2620", // black
			"#A63214", // red
			"#23703F", // green
			"#8A5F00", // yellow
			"#3E5F7A", // blue
			"#7A4258", // magenta
			"#0D6B70", // cyan
			"#4A4335", // white
			"#9A9080", // bright black
			"#BD3A1D", // bright red
			"#1C5A33", // bright green
			"#75500A", // bright yellow
			"#32506A", // bright blue
			"#68384B", // bright magenta
			"#0A585D", // bright cyan
			"#1A1611", // bright white
		},
		accent:    "#BD3A1D",
		shimmer:   "#A63214",
		border:    "#D8D0BD",
		surface:   "#E6DFCF",
		running:   "#23703F",
		gray:      "#6F6656",
		faint:     "#867C6A",
		parchment: "#4A4335",

		messageBg:       "#D8D0BD",
		messageHoverBg:  "#CFC6B0",
		toolBg:          "#E6DFCF",
		diffAddedBg:     "#CAD7BB",
		diffRemovedBg:   "#E8D3C4",
		diffAddedDim:    "#DFE1CD",
		diffRemovedDim:  "#EBDED0",
		diffAddedWord:   "#A4C187",
		diffRemovedWord: "#E0BDA4",
	},
}

// What conn draws from, as wear last left it: every reader in conn
// takes its color from here, by name, and none of them cares which
// theme or ground it came off. conn's dark until applyMode says
// otherwise, which is what every terminal was before conn learned to
// ask.
var (
	groundColor, inkColor = connTheme.dark.ground, connTheme.dark.ink
	scheme                = connTheme.dark.scheme

	cursorHex    = connTheme.dark.accent
	shimmerHex   = connTheme.dark.shimmer
	borderHex    = connTheme.dark.border
	surfaceHex   = connTheme.dark.surface
	runningHex   = connTheme.dark.running
	grayHex      = connTheme.dark.gray
	faintHex     = connTheme.dark.faint
	parchmentHex = connTheme.dark.parchment

	messageBg, messageHoverBg, toolBg = connTheme.dark.messageBg, connTheme.dark.messageHoverBg, connTheme.dark.toolBg
	diffAddedBg, diffRemovedBg        = connTheme.dark.diffAddedBg, connTheme.dark.diffRemovedBg
	diffAddedDim, diffRemovedDim      = connTheme.dark.diffAddedDim, connTheme.dark.diffRemovedDim
	diffAddedWord, diffRemovedWord    = connTheme.dark.diffAddedWord, connTheme.dark.diffRemovedWord
)

// wear puts every color conn draws from onto one ground of one theme.
func wear(g ground) {
	groundColor, inkColor = g.ground, g.ink
	scheme = g.scheme
	cursorHex, shimmerHex, borderHex, surfaceHex, runningHex = g.accent, g.shimmer, g.border, g.surface, g.running
	grayHex, faintHex, parchmentHex = g.gray, g.faint, g.parchment
	messageBg, messageHoverBg, toolBg = g.messageBg, g.messageHoverBg, g.toolBg
	diffAddedBg, diffRemovedBg = g.diffAddedBg, g.diffRemovedBg
	diffAddedDim, diffRemovedDim = g.diffAddedDim, g.diffRemovedDim
	diffAddedWord, diffRemovedWord = g.diffAddedWord, g.diffRemovedWord
}
