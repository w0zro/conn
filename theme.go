package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Everything in conn's server asks the terminal for a color by name and
// gets conn's: tmux draws the sixteen from the scheme. Claude Code asks
// for none of them — it writes its own hex, and tmux passes truecolor
// through untouched — so it is the one program in a pane that conn's
// palette cannot reach. conn theme claude prints a theme for it, drawn
// from the same table the server's palette comes from, so the two
// cannot drift apart.
//
// conn prints it and stops there. Where the file goes and whether it is
// the theme in use is Claude Code's business and the user's, not
// conn's: conn dresses its own server, not the programs it holds.

// The gray of the console's second rank, as a hex; the ground, the ink
// and the border have it already.
const grayHex = "#8B8272"

// claudeTheme is what conn would have Claude Code paint with: every
// token it knows, against conn's scheme. The accents are the sixteen.
// The grounds are the console's, a step apart from each other rather
// than a color of their own — a band is a lift off the ground, not a
// fill. Orange stays what conn spends it on, the thing that wants you:
// an error, and conn's own mark. A mode is on all day, so the modes
// take the cool pair, the cyan and the magenta.
func claudeTheme() map[string]string {
	var (
		black, red, green, yellow = scheme[0], scheme[1], scheme[2], scheme[3]
		blue, magenta, cyan       = scheme[4], scheme[5], scheme[6]
		faint, orange             = scheme[8], scheme[9]
		brGreen, brYellow         = scheme[10], scheme[11]
		brBlue, brMagenta, brCyan = scheme[12], scheme[13], scheme[14]
		parchment, ink            = scheme[7], scheme[15]
	)
	ground := hex(groundColor)
	// A band is the ground lifted, not a color; a word on it is the ink.
	lift := mix(ground, ink, 0.06)
	// A shimmer is the same color on its way to the parchment.
	shimmer := func(c string) string { return mix(c, parchment, 0.45) }
	// A diff is its color laid thinly on the ground: the block, the word
	// inside it, and the block when it is not the one being read.
	tint := func(c string, t float64) string { return mix(ground, c, t) }

	return map[string]string{
		// conn's own mark, and who is speaking.
		"claude":            orange,
		"claudeShimmer":     shimmer(orange),
		"clawd_body":        orange,
		"clawd_background":  ground,
		"briefLabelClaude":  orange,
		"briefLabelYou":     blue,
		"professionalBlue":  blue,
		"ide":               blue,
		"permission":        blue,
		"permissionShimmer": shimmer(blue),

		"claudeBlue_FOR_SYSTEM_SPINNER":        blue,
		"claudeBlueShimmer_FOR_SYSTEM_SPINNER": shimmer(blue),

		// The modes: on all day, so never the orange.
		"planMode":          cyan,
		"autoAccept":        brCyan,
		"autoAcceptShimmer": shimmer(brCyan),
		"skill":             magenta,
		"merged":            magenta,
		"effortUltra":       magenta,
		"fastMode":          brYellow,
		"fastModeShimmer":   shimmer(brYellow),

		// The page: the ground, the ink, and the two ranks below it.
		"background":          ground,
		"text":                ink,
		"inverseText":         ground,
		"inactive":            grayHex,
		"inactiveShimmer":     shimmer(grayHex),
		"subtle":              black,
		"promptBorder":        faint,
		"promptBorderShimmer": grayHex,
		"bashBorder":          magenta,
		"suggestion":          brBlue,
		"remember":            brBlue,
		"selectionBg":         black,
		"rate_limit_fill":     blue,
		"rate_limit_empty":    black,
		"chromeYellow":        yellow,
		"success":             green,
		"error":               red,
		"warning":             yellow,
		"warningShimmer":      shimmer(yellow),

		// The bands. Each is the ground lifted, tinted where the thing on
		// it has a color of its own.
		"userMessageBackground":      lift,
		"userMessageBackgroundHover": black,
		"composerSidebarBackground":  lift,
		"bashMessageBackgroundColor": tint(magenta, 0.12),
		"memoryBackgroundColor":      tint(cyan, 0.12),

		// A diff, laid on the ground.
		"diffAdded":         tint(green, 0.22),
		"diffAddedWord":     tint(green, 0.40),
		"diffAddedDimmed":   tint(green, 0.12),
		"diffRemoved":       tint(red, 0.22),
		"diffRemovedWord":   tint(red, 0.40),
		"diffRemovedDimmed": tint(red, 0.12),

		// An agent is told apart by color, so these are the sixteen
		// themselves, and no two the same.
		"red_FOR_SUBAGENTS_ONLY":    red,
		"orange_FOR_SUBAGENTS_ONLY": orange,
		"yellow_FOR_SUBAGENTS_ONLY": yellow,
		"green_FOR_SUBAGENTS_ONLY":  green,
		"cyan_FOR_SUBAGENTS_ONLY":   cyan,
		"blue_FOR_SUBAGENTS_ONLY":   blue,
		"purple_FOR_SUBAGENTS_ONLY": magenta,
		"pink_FOR_SUBAGENTS_ONLY":   brMagenta,

		"rainbow_red":            red,
		"rainbow_red_shimmer":    shimmer(red),
		"rainbow_orange":         orange,
		"rainbow_orange_shimmer": shimmer(orange),
		"rainbow_yellow":         yellow,
		"rainbow_yellow_shimmer": shimmer(yellow),
		"rainbow_green":          green,
		"rainbow_green_shimmer":  shimmer(brGreen),
		"rainbow_blue":           blue,
		"rainbow_blue_shimmer":   shimmer(brBlue),
		"rainbow_indigo":         brBlue,
		"rainbow_indigo_shimmer": shimmer(cyan),
		"rainbow_violet":         magenta,
		"rainbow_violet_shimmer": shimmer(brMagenta),
	}
}

// claudeThemeJSON is the theme as Claude Code reads it: a name, the base
// it sits on, and the tokens, each color written the way its own themes
// write one.
func claudeThemeJSON() (string, error) {
	over := map[string]string{}
	for k, c := range claudeTheme() {
		over[k] = rgbString(c)
	}
	b, err := json.MarshalIndent(struct {
		Name      string            `json:"name"`
		Base      string            `json:"base"`
		Overrides map[string]string `json:"overrides"`
	}{Name: "conn", Base: "dark", Overrides: over}, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b) + "\n", nil
}

// rgbString is a color as a Claude Code theme writes one.
func rgbString(c string) string {
	r, g, b := parseHex(c)
	return fmt.Sprintf("rgb(%d,%d,%d)", r, g, b)
}

// mix is a step from one color toward another: 0 is the first, 1 the
// second.
func mix(a, b string, t float64) string {
	ar, ag, ab := parseHex(a)
	br, bg, bb := parseHex(b)
	at := func(x, y int) int { return int(float64(x) + (float64(y)-float64(x))*t + 0.5) }
	return fmt.Sprintf("#%02X%02X%02X", at(ar, br), at(ag, bg), at(ab, bb))
}

// parseHex reads #RRGGBB.
func parseHex(s string) (int, int, int) {
	v, _ := strconv.ParseUint(strings.TrimPrefix(s, "#"), 16, 32)
	return int(v>>16) & 0xFF, int(v>>8) & 0xFF, int(v) & 0xFF
}
