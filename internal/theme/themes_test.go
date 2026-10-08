package theme

import (
	"reflect"
	"regexp"
	"testing"
)

// Every theme is whole on both of its grounds: a ground and an ink,
// all sixteen, and every role a hex of its own. A role left empty
// would be drawn as no color at all, which nothing catches until it
// is on a screen.
func TestEveryThemeIsWholeOnBothGrounds(t *testing.T) {
	isHex := regexp.MustCompile(`^#[0-9A-F]{6}$`)
	for _, th := range All {
		for _, on := range []struct {
			name string
			g    Ground
		}{{"dark", th.Dark}, {"light", th.Light}} {
			g := on.g
			if g.Ground.A != 255 || g.Ink.A != 255 {
				t.Errorf("%s %s: the ground or the ink is unset", th.Name, on.name)
			}
			// A ground says which it is by its own luminance, and what
			// follows from that - the base Claude Code's theme sits on,
			// what nvim is told - is read off it; a dark ground that
			// read as light would dress every program for the wrong one.
			if g.dark() != (on.name == "dark") {
				t.Errorf("%s %s: the ground %s reads as %s", th.Name, on.name, Hex(g.Ground), map[bool]string{true: "dark", false: "light"}[g.dark()])
			}
			for i, c := range g.Scheme {
				if !isHex.MatchString(c) {
					t.Errorf("%s %s: slot %d is %q", th.Name, on.name, i, c)
				}
			}
			v := reflect.ValueOf(g)
			for i := 0; i < v.NumField(); i++ {
				if f := v.Field(i); f.Kind() == reflect.String && !isHex.MatchString(f.String()) {
					t.Errorf("%s %s: %s is %q, not a hex", th.Name, on.name, v.Type().Field(i).Name, f.String())
				}
			}
		}
	}
}

// The roles read on every ground of every theme. A role is what conn
// draws by meaning, so it has to hold up wherever a theme puts its
// slots: the inks and the accent at the ratio body text takes, the
// gray at the ratio large text takes, the faint quieter but a color
// still and not the ground, and the border quieter than the ink, since
// it is an edge.
func TestTheRolesReadOnEveryGround(t *testing.T) {
	const body, large = 4.5, 3 // WCAG AA
	for _, th := range All {
		for _, on := range []struct {
			name string
			g    Ground
		}{{"dark", th.Dark}, {"light", th.Light}} {
			g, bg := on.g, Hex(on.g.Ground)
			for _, r := range []struct {
				name, hex string
				least     float64
			}{
				{"ink", Hex(g.Ink), body}, {"parchment", g.Parchment, body},
				{"accent", g.Accent, body}, {"shimmer", g.Shimmer, body},
				{"gray", g.Gray, large}, {"faint", g.Faint, 2},
			} {
				if c := contrast(r.hex, bg); c < r.least {
					t.Errorf("%s %s: the %s (%s) is %.2f:1 on %s; %.1f:1 is what it takes",
						th.Name, on.name, r.name, r.hex, c, bg, r.least)
				}
			}
			// A block is read for the word in it, so the letters take
			// the ratio body text does against the fill they are on.
			if c := contrast(g.OnBlock, g.Block); c < body {
				t.Errorf("%s %s: the word in a block (%s on %s) is %.2f:1; %.1f:1 is what it takes",
					th.Name, on.name, g.OnBlock, g.Block, c, body)
			}
			// A chosen row is read like any other, on its own ground;
			// the cursor only has to be seen, as the faint does.
			if c := contrast(g.ChosenInk, g.Chosen); c < body {
				t.Errorf("%s %s: a chosen row (%s on %s) is %.2f:1; %.1f:1 is what it takes",
					th.Name, on.name, g.ChosenInk, g.Chosen, c, body)
			}
			if c := contrast(g.Cursor, bg); c < 2 {
				t.Errorf("%s %s: the cursor (%s) is %.2f:1 on %s, which is the ground again", th.Name, on.name, g.Cursor, c, bg)
			}
			if contrast(g.Border, bg) >= contrast(Hex(g.Ink), bg) {
				t.Errorf("%s %s: the border (%s) stands off the ground further than the ink does", th.Name, on.name, g.Border)
			}
		}
	}
}
