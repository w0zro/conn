// Package theme is what conn dresses a server in: the ground and the
// ink, the sixteen a program asks for by name, and the roles conn draws
// by meaning, each on a dark ground and a light one. A ground is a
// value, and everything conn draws takes the one it is handed: the
// server's configuration is written from it, and so is the panel.
// Nothing is package-wide, so nothing reads a color from a mode before.
package theme

import (
	"fmt"
	"image/color"
)

// A Ground is the theme on one of its two grounds.
type Ground struct {
	Ground, Ink color.RGBA // the pane's ground, and ordinary text on it
	Scheme      [16]string // what a program asks for by name; a slot, and never read for a role

	// The roles: what conn draws by meaning. Each is a hex of its own,
	// whether or not a slot happens to hold the same one.
	Accent    string // "you, here": the cursor, a block on the status line, what waits on you
	Shimmer   string // the accent's brighter cousin
	Border    string // a pane's edge, a selection, the band behind the status line
	Surface   string // one step off the ground, short of the border: the panel's own ground
	Running   string // a process doing something
	Gray      string // the second rank: a label, a comment
	Faint     string // the quietest text: a leader, a hint
	Parchment string // the second ink: a title, what conn says on the status line

	// The accent as a block — a stamp, the mode on the status line, the
	// match copy mode is on — and the letters knocked into it.
	Block, OnBlock string

	// The row under the cursor, and what copy mode has selected, and
	// the ink it is written in; and the cursor itself.
	Chosen, ChosenInk, Cursor string
}

// Dark says whether this is the dark ground, read off the ground
// itself the way a terminal's is asked.
func (g Ground) Dark() bool {
	return IsDark(g.Ground)
}

// IsDark reads a ground as dark or light by the relative luminance a
// screen reader uses to say if text passes on it: below half is dark.
func IsDark(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	luminance := 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)
	return luminance/0xffff < 0.5
}

// On is conn's theme on one ground: dark, or light.
func On(dark bool) Ground {
	if dark {
		return Dark
	}
	return Light
}

// RGB is a color as the table writes it, #RRGGBB. The table is read at
// start and by the tests, so a hex that will not parse stops conn there
// rather than drawing black.
func RGB(h string) color.RGBA {
	var c color.RGBA
	if _, err := fmt.Sscanf(h, "#%02X%02X%02X", &c.R, &c.G, &c.B); err != nil {
		panic(fmt.Sprintf("not a color: %q", h))
	}
	c.A = 255
	return c
}

// Hex is a color as a terminal wants it written.
func Hex(c color.RGBA) string {
	return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
}

// conn's own. Dark is every terminal it ever knew; light is for the
// terminal that says its own ground is light when conn asks.
//
// Light is not dark with the lightness flipped. On paper, emphasis is
// more ink, not more light, so the light scheme's bright slots go
// darker than its normal ones — the opposite of the dark scheme, where
// bright is lighter. The two neutral slots are the exception, and keep
// what every program means by them: 0 is black, which on paper is
// ordinary text, and 8 is the gray a program dims with.
//
// Most of the sixteen are conn's own tokens: the two oranges for the
// reds, the parchment and the ink for the whites, the faint for bright
// black, the border for black. The blue and the magenta are added so
// that the slots a shell theme leans on — structure, type, what can be
// run — stay apart instead of collapsing into the orange and the teal.
//
// The border is scheme[0] on dark, the darkest thing there is and so
// the quietest edge. On light it cannot be: light's scheme[0] is black,
// which a program writing ANSI-0 means as text, so the border there is
// a color of its own, pale enough to be an edge on paper. The faint is
// scheme[8] on dark; light needed its own, since ANSI-8 there reads as
// a tint but nearly vanishes as text.
var (
	Dark = Ground{
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
		Block:     "#E85D2F",
		OnBlock:   "#15130F",
		Chosen:    "#2A2620",
		ChosenInk: "#E6DFD0",
		Cursor:    "#E85D2F",
	}
	Light = Ground{
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
		Block:     "#BD3A1D",
		OnBlock:   "#EFE9DB",
		Chosen:    "#D8D0BD",
		ChosenInk: "#1A1611",
		Cursor:    "#BD3A1D",
	}
)

// Roles is every role of a ground by name, for whatever has to hold
// each one to something: that it parses, that it reads on the ground.
func (g Ground) Roles() map[string]string {
	return map[string]string{
		"accent": g.Accent, "shimmer": g.Shimmer, "border": g.Border, "surface": g.Surface,
		"running": g.Running, "gray": g.Gray, "faint": g.Faint, "parchment": g.Parchment,
		"block": g.Block, "onBlock": g.OnBlock, "chosen": g.Chosen, "chosenInk": g.ChosenInk, "cursor": g.Cursor,
	}
}
