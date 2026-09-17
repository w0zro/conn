package main

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
	for _, th := range themes {
		for _, on := range []struct {
			name string
			g    ground
		}{{"dark", th.dark}, {"light", th.light}} {
			g := on.g
			if g.ground.A != 255 || g.ink.A != 255 {
				t.Errorf("%s %s: the ground or the ink is unset", th.name, on.name)
			}
			for i, c := range g.scheme {
				if !isHex.MatchString(c) {
					t.Errorf("%s %s: slot %d is %q", th.name, on.name, i, c)
				}
			}
			v := reflect.ValueOf(g)
			for i := 0; i < v.NumField(); i++ {
				if f := v.Field(i); f.Kind() == reflect.String && !isHex.MatchString(f.String()) {
					t.Errorf("%s %s: %s is %q, not a hex", th.name, on.name, v.Type().Field(i).Name, f.String())
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
	for _, th := range themes {
		for _, on := range []struct {
			name string
			g    ground
		}{{"dark", th.dark}, {"light", th.light}} {
			g, bg := on.g, hex(on.g.ground)
			for _, r := range []struct {
				name, hex string
				least     float64
			}{
				{"ink", hex(g.ink), body}, {"parchment", g.parchment, body},
				{"accent", g.accent, body}, {"shimmer", g.shimmer, body},
				{"gray", g.gray, large}, {"faint", g.faint, 2},
			} {
				if c := contrast(r.hex, bg); c < r.least {
					t.Errorf("%s %s: the %s (%s) is %.2f:1 on %s; %.1f:1 is what it takes",
						th.name, on.name, r.name, r.hex, c, bg, r.least)
				}
			}
			if contrast(g.border, bg) >= contrast(hex(g.ink), bg) {
				t.Errorf("%s %s: the border (%s) stands off the ground further than the ink does", th.name, on.name, g.border)
			}
		}
	}
}
