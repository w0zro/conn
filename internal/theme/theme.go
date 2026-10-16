// Package theme is what conn dresses a server in: the ground and the
// ink, the sixteen a program asks for by name, and the roles conn draws
// by meaning, each on a dark ground and a light one. A ground is a
// value, and everything conn draws takes the one it is handed: the
// server's configuration is written from it, and so is the panel.
// Nothing is package-wide, so nothing reads a color from a mode before.
package theme

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/color"
	"slices"
	"strings"
)

// A Ground is the theme on one of its two grounds.
type Ground struct {
	Ground, Ink color.RGBA // the pane's ground, and ordinary text on it
	Scheme      [16]string // what a program asks for by name; a slot, and never read for a role

	// The roles: what conn draws by meaning. Each is a hex of its own,
	// whether or not a slot happens to hold the same one.
	Accent  string // "you, here": the cursor, a block on the status line, what waits on you
	Shimmer string // the accent's brighter cousin

	// Claude Code's mark, which turns while Claude works, and what it
	// shimmers to: the accent and its shimmer, unless a theme gives a
	// mark at work a hue of its own.
	AtWork, AtWorkShimmer string

	Border    string // a pane's edge, a selection, the band behind the status line
	Surface   string // one step off the ground, short of the border: the panel's own ground
	Running   string // a process doing something
	Turning   string // the spinner beside what is at work: the running ink, unless a theme has a hue for it
	Gray      string // the second rank: a label, a comment
	Faint     string // the quietest text: a leader, a hint
	Parchment string // the second ink: a title, what conn says on the status line

	// The accent as a block — a stamp, the mode on the status line, the
	// match copy mode is on — and the letters knocked into it.
	Block, OnBlock string

	// The row under the cursor, and what copy mode has selected, and
	// the ink it is written in; and the cursor itself: the terminal's,
	// and the bar beside the row the panel's is on.
	Chosen, ChosenInk, Cursor string

	// The grounds no slot has a name for, which Claude Code draws: the
	// band behind what you said, at rest and under the pointer; the bar
	// behind a tool's output; and the washes a diff is laid on, one
	// step up for the words inside it, and dimmed.
	MessageBg, MessageHoverBg, ToolBg                                                        string
	DiffAddedBg, DiffRemovedBg, DiffAddedDim, DiffRemovedDim, DiffAddedWord, DiffRemovedWord string
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
	return Conn.On(dark)
}

// RGB is a color as a sheet writes it, #RRGGBB. Every color has been
// read once already, when its sheet was, so one that will not parse
// here is a color that came from nowhere, and stops conn rather than
// drawing black.
func RGB(h string) color.RGBA {
	c, err := parseHex(h)
	if err != nil {
		panic(err)
	}
	return c
}

// parseHex reads #RRGGBB, in either case, and nothing else.
func parseHex(h string) (color.RGBA, error) {
	b, err := hex.DecodeString(strings.TrimPrefix(h, "#"))
	if !strings.HasPrefix(h, "#") || err != nil || len(b) != 3 {
		return color.RGBA{}, fmt.Errorf("%q is not a color written #RRGGBB", h)
	}
	return color.RGBA{R: b[0], G: b[1], B: b[2], A: 255}, nil
}

// Hex is a color as a terminal wants it written.
func Hex(c color.RGBA) string {
	return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
}

// conn's own sheet. Dark is every terminal it ever knew; light is for the
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
// The washes and bars are in the ground's own temperature, none of
// them a fill. A wash this pale on light needs more room from the
// ground than the same wash does on dark to read as a color at all
// rather than a shade of the ground itself: lightness compresses
// toward white long before it compresses toward black. Each light one
// holds at least the contrast its dark counterpart has against its own
// ground.
//
// The border is scheme[0] on dark, the darkest thing there is and so
// the quietest edge. On light it cannot be: light's scheme[0] is black,
// which a program writing ANSI-0 means as text, so the border there is
// a color of its own, pale enough to be an edge on paper. The faint is
// scheme[8] on dark; light needed its own, since ANSI-8 there reads as
// a tint but nearly vanishes as text.
var (
	// Conn is conn's own theme, sheets/conn.json, and Dark and Light
	// are its two grounds.
	Conn        = mustParse("conn", connSheet())
	Dark, Light = Conn.Dark, Conn.Light
)

// connSheet is the sheet conn's own theme is read from.
func connSheet() []byte {
	b, _ := shippedSheet("conn")
	return b
}

// A role by the name a sheet gives it, and where the ground keeps it.
type role struct {
	name string
	at   *string
}

// roles is every role of a ground, in the order a sheet lists them.
// It is the one list of their names: a sheet is read by it, and Roles
// is written from it.
func (g *Ground) roles() []role {
	return []role{
		{"accent", &g.Accent}, {"shimmer", &g.Shimmer}, {"atWork", &g.AtWork}, {"atWorkShimmer", &g.AtWorkShimmer},
		{"border", &g.Border}, {"surface", &g.Surface}, {"running", &g.Running}, {"turning", &g.Turning}, {"gray", &g.Gray}, {"faint", &g.Faint}, {"parchment", &g.Parchment},
		{"block", &g.Block}, {"onBlock", &g.OnBlock}, {"chosen", &g.Chosen}, {"chosenInk", &g.ChosenInk}, {"cursor", &g.Cursor},
		{"messageBg", &g.MessageBg}, {"messageHoverBg", &g.MessageHoverBg}, {"toolBg", &g.ToolBg},
		{"diffAddedBg", &g.DiffAddedBg}, {"diffRemovedBg", &g.DiffRemovedBg}, {"diffAddedDim", &g.DiffAddedDim},
		{"diffRemovedDim", &g.DiffRemovedDim}, {"diffAddedWord", &g.DiffAddedWord}, {"diffRemovedWord", &g.DiffRemovedWord},
	}
}

// Roles is every role of a ground by name, for whatever has to hold
// each one to something: that it parses, that it reads on the ground.
func (g Ground) Roles() map[string]string {
	m := map[string]string{}
	for _, r := range g.roles() {
		m[r.name] = *r.at
	}
	return m
}

// A Theme is a name and a ground of each kind: what a terminal that is
// dark gets, and what a light one does.
type Theme struct {
	Name        string
	Dark, Light Ground
}

// On is the theme on one ground: dark, or light.
func (t Theme) On(dark bool) Ground {
	if dark {
		return t.Dark
	}
	return t.Light
}

// Parse reads a theme sheet: a JSON object with a "dark" ground, a
// "light" one, or both. A ground is its "ground" and "ink", the sixteen
// of its "scheme", and every role by name, each a color written
// #RRGGBB. A sheet with one ground wears it on either: conn dresses the
// whole server, so a dark theme on a light terminal is still whole.
// Anything missing, misspelt or not a color is said, by name.
func Parse(name string, b []byte) (Theme, error) {
	var sheet map[string]json.RawMessage
	if err := json.Unmarshal(b, &sheet); err != nil {
		return Theme{}, fmt.Errorf("theme %s: %w", name, err)
	}
	t := Theme{Name: name}
	var dark, light bool
	for k, v := range sheet {
		var err error
		switch k {
		case "dark":
			t.Dark, err = parseGround(v)
			dark = true
		case "light":
			t.Light, err = parseGround(v)
			light = true
		default:
			err = fmt.Errorf("%q is neither dark nor light", k)
		}
		if err != nil {
			return Theme{}, fmt.Errorf("theme %s: %s: %w", name, k, err)
		}
	}
	switch {
	case !dark && !light:
		return Theme{}, fmt.Errorf("theme %s has neither a dark ground nor a light one", name)
	case !dark:
		t.Dark = t.Light
	case !light:
		t.Light = t.Dark
	}
	return t, nil
}

// parseGround reads one ground of a sheet, and writes every color the
// way Hex does, whichever case the sheet wrote it in.
func parseGround(b []byte) (Ground, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		return Ground{}, err
	}
	var g Ground
	hexOf := func(k string) (string, error) {
		v, ok := fields[k]
		if !ok {
			return "", fmt.Errorf("no %s", k)
		}
		delete(fields, k)
		var h string
		if err := json.Unmarshal(v, &h); err != nil {
			return "", fmt.Errorf("%s: %w", k, err)
		}
		c, err := parseHex(h)
		if err != nil {
			return "", fmt.Errorf("%s: %w", k, err)
		}
		return Hex(c), nil
	}
	var err error
	if g.Ground, err = rgbOf(hexOf("ground")); err != nil {
		return Ground{}, err
	}
	if g.Ink, err = rgbOf(hexOf("ink")); err != nil {
		return Ground{}, err
	}
	var scheme []string
	if v, ok := fields["scheme"]; !ok {
		return Ground{}, fmt.Errorf("no scheme")
	} else if err := json.Unmarshal(v, &scheme); err != nil {
		return Ground{}, fmt.Errorf("scheme: %w", err)
	}
	delete(fields, "scheme")
	if len(scheme) != len(g.Scheme) {
		return Ground{}, fmt.Errorf("scheme has %d colors, not %d", len(scheme), len(g.Scheme))
	}
	for i, h := range scheme {
		c, err := parseHex(h)
		if err != nil {
			return Ground{}, fmt.Errorf("scheme %d: %w", i, err)
		}
		g.Scheme[i] = Hex(c)
	}
	for _, r := range g.roles() {
		if *r.at, err = hexOf(r.name); err != nil {
			return Ground{}, err
		}
	}
	if len(fields) > 0 {
		var extra []string
		for k := range fields {
			extra = append(extra, k)
		}
		slices.Sort(extra)
		return Ground{}, fmt.Errorf("%s is no role of conn's", strings.Join(extra, ", "))
	}
	return g, nil
}

// rgbOf is a color read by parseGround, as the ground keeps it.
func rgbOf(h string, err error) (color.RGBA, error) {
	if err != nil {
		return color.RGBA{}, err
	}
	return parseHex(h)
}

// mustParse reads a sheet conn ships with, which the tests have read.
func mustParse(name string, b []byte) Theme {
	t, err := Parse(name, b)
	if err != nil {
		panic(err)
	}
	return t
}
