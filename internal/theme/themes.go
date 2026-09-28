// Package theme is what conn dresses a server in. A theme is that, by
// name: the ground and the ink, the sixteen a program asks for by name,
// and the roles conn draws by meaning, each on a dark ground and a
// light one. conn is the theme conn has always worn, and the one it
// wears unless told another; see mode.go for how a ground is chosen.
// The same colors are written out for the programs conn dresses: a
// theme for Claude Code, and a colorscheme for nvim.
//
// A ground is a value, and everything conn draws takes the one it is
// handed: the palette is built off it, the tmux configuration and the
// status line are written from it, and so are the theme for Claude Code
// and the colorscheme for nvim. Nothing is package-wide, so a conn that
// changes mode hands the new ground to what draws and nothing reads a
// color from the mode before.
package theme

import (
	"fmt"
	"image/color"

	"github.com/w0zro/conn/internal/config"
)

// A Ground is a theme on one of its two grounds.
type Ground struct {
	Ground, Ink color.RGBA // the pane's ground, and ordinary text on it
	Scheme      [16]string // what a program asks for by name; a slot, and never read for a role

	// The roles: what conn draws by meaning. Each is a hex of its own,
	// whether or not a slot happens to hold the same one.
	Accent    string // "you, here": the cursor, a chip, a title, the caret, a block on the status line
	Shimmer   string // the accent's brighter cousin, for Claude Code to shimmer with
	Border    string // a pane's edge, a selection, the band behind the status line
	Surface   string // one step off the ground, short of the border: the panel's own ground
	Running   string // a process doing something, said by the dot at the head of its row
	Gray      string // the second rank: a label, a comment
	Faint     string // the quietest text: a leader, a hint, a line number
	Parchment string // the second ink: a title, punctuation, what conn says on the status line

	// The grounds no slot has a name for: the band behind what you said
	// to Claude Code, at rest and under the pointer; the bar behind a
	// tool's output, which is also the step off the ground a colorscheme
	// lifts a float onto; and the washes a diff is laid on, one step up
	// for the words inside it, and dimmed.
	MessageBg, MessageHoverBg, ToolBg                                                        string
	DiffAddedBg, DiffRemovedBg, DiffAddedDim, DiffRemovedDim, DiffAddedWord, DiffRemovedWord string
}

// A theme is a name and its two grounds.
type theme struct {
	Name        string
	Dark, Light Ground
}

// All is every theme conn has, in the order they are offered.
var All = []theme{Conn, Datum}

// Default is the one conn wears unless told another.
const Default = "conn"

// Named is the theme by that name, and whether conn has one.
func Named(name string) (theme, bool) {
	for _, t := range All {
		if t.Name == name {
			return t, true
		}
	}
	return theme{}, false
}

// Known says whether conn has a theme by that name.
func Known(name string) bool {
	_, ok := Named(name)
	return ok
}

// on is the theme on one ground: dark, or light.
func (t theme) on(dark bool) Ground {
	if dark {
		return t.Dark
	}
	return t.Light
}

// RGB is a color as a table writes it, #RRGGBB, as the terminal is
// asked to take it. A table is read at start and by the tests, so a
// hex that will not parse stops conn there rather than drawing black.
func RGB(h string) color.RGBA {
	var c color.RGBA
	if _, err := fmt.Sscanf(h, "#%02X%02X%02X", &c.R, &c.G, &c.B); err != nil {
		panic(fmt.Sprintf("not a color: %q", h))
	}
	c.A = 255
	return c
}

// Conn is conn's own. Dark is every terminal it ever knew; light
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
var Conn = theme{
	Name: "conn",
	Dark: Ground{
		Ground: color.RGBA{R: 21, G: 19, B: 15, A: 255},
		Ink:    color.RGBA{R: 230, G: 223, B: 208, A: 255},
		Scheme: [16]string{
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
		Accent:    "#E85D2F",
		Shimmer:   "#FF7847",
		Border:    "#2A2620",
		Surface:   "#1D1A15",
		Running:   "#93C98B",
		Gray:      "#8B8272",
		Faint:     "#5C564A",
		Parchment: "#BFB39A",

		MessageBg:       "#2A2620",
		MessageHoverBg:  "#33302A",
		ToolBg:          "#1D1A15",
		DiffAddedBg:     "#1E2A1C",
		DiffRemovedBg:   "#331F17",
		DiffAddedDim:    "#191F17",
		DiffRemovedDim:  "#231A14",
		DiffAddedWord:   "#2C4028",
		DiffRemovedWord: "#4A2A1D",
	},
	Light: Ground{
		Ground: color.RGBA{R: 0xEF, G: 0xE9, B: 0xDB, A: 255},
		Ink:    color.RGBA{R: 0x1A, G: 0x16, B: 0x11, A: 255},
		Scheme: [16]string{
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
		Accent:    "#BD3A1D",
		Shimmer:   "#A63214",
		Border:    "#D8D0BD",
		Surface:   "#E6DFCF",
		Running:   "#23703F",
		Gray:      "#6F6656",
		Faint:     "#867C6A",
		Parchment: "#4A4335",

		MessageBg:       "#D8D0BD",
		MessageHoverBg:  "#CFC6B0",
		ToolBg:          "#E6DFCF",
		DiffAddedBg:     "#CAD7BB",
		DiffRemovedBg:   "#E8D3C4",
		DiffAddedDim:    "#DFE1CD",
		DiffRemovedDim:  "#EBDED0",
		DiffAddedWord:   "#A4C187",
		DiffRemovedWord: "#E0BDA4",
	},
}

// dark is whether this is a theme's dark ground, read off the ground
// itself the way a terminal's is asked: a theme's dark ground is dark
// by construction, and reading it keeps a ground one thing rather than
// a table and a flag about the table. What follows from it - the base
// Claude Code's theme sits on, what nvim is told its background is -
// is read here too.
func (g Ground) dark() bool {
	return isDark(g.Ground)
}

// configTheme is the theme the file names, when conn has one by that
// name, and conn's own otherwise: a file that cannot be read is the
// console's to report, not a reason to come up in nothing.
func configTheme(home string) string {
	c, _ := config.Read(home)
	if _, ok := Named(c.Theme); ok {
		return c.Theme
	}
	return Default
}

// Hex is a color as a terminal wants it written.
func Hex(c color.RGBA) string {
	return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
}
