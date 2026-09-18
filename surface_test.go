package main

import (
	"strconv"
	"strings"
	"testing"
)

// luminance is a hex color's relative brightness, enough to order
// grounds by.
func luminance(h string) float64 {
	v, _ := strconv.ParseUint(strings.TrimPrefix(h, "#"), 16, 32)
	r, g, b := float64(v>>16&0xff), float64(v>>8&0xff), float64(v&0xff)
	return 0.2126*r + 0.7152*g + 0.0722*b
}

// The surface is a step off the ground, short of the border, on both
// grounds of every theme: on a dark ground it is lighter than the
// ground and darker than the border, and on a light ground the other
// way about. A panel on it stands off the bay without taking the
// selection.
func TestTheSurfaceIsBetweenTheGroundAndTheBorder(t *testing.T) {
	for _, th := range themes {
		for name, g := range map[string]ground{"dark": th.dark, "light": th.light} {
			ground, surface, border := luminance(hex(g.ground)), luminance(g.surface), luminance(g.border)
			if name == "dark" && !(ground < surface && surface < border) {
				t.Errorf("%s on dark: ground %.0f, surface %.0f, border %.0f", th.name, ground, surface, border)
			}
			if name == "light" && !(ground > surface && surface > border) {
				t.Errorf("%s on light: ground %.0f, surface %.0f, border %.0f", th.name, ground, surface, border)
			}
		}
	}
	// The palette on the surface paints its rows on it, and returns to
	// it after every piece.
	p := colored().onSurface()
	if p.ground != ansiHex(48, surfaceHex) || !strings.HasPrefix(p.normal, p.end+p.ground) {
		t.Errorf("the surface palette grounds on %q", p.ground)
	}
	if got := plain.onSurface(); !got.plain || got.ground != "" {
		t.Error("the plain palette took a ground")
	}
}
