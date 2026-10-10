package theme

// Skelly is skelly's, from its brand at brand.skelly.ai: the palette it
// calls Glow, five colors and rules for the rest. A green-tinted black
// and a bone ink, and three glows in the dark. Phosphor is what needs
// you, which is what conn's accent already means: the WAITING stamp,
// the caution, the cursor, the selection. Neon is broke, the reds.
// Black light is the second glow, for a mark at work: the spinner
// beside a contact at work, Claude Code's mark while Claude works, and
// the magentas. A thing running has no hue, so it is drawn in ink.
//
// Every other skelly token is a rule on the five - a surface is the
// ground a step lighter, a line or a gray is the ink mixed into the
// ground, a glow's text, line and tint are the glow at a set lightness
// and chroma with its hue kept - and the values here are those rules
// as the brand page prints them, named by token. Where conn needs a
// step the brand has no token for, the same kind of rule makes it and
// the comment says so. The sixteen are the brand's own, as its
// palette.mjs --terminal writes them: the grays, the glows' text steps
// for red, green and magenta, and yellow, blue and cyan set by hue at
// the same lightness, the one place the brand has a color past the
// five. The test holds them to the copy under testdata/skelly;
// refreshing skelly is writing that file again.
var Skelly = theme{
	Name: "skelly",
	Dark: Ground{
		Ground: RGB("#101211"), // --bg: lights-out
		Ink:    RGB("#E6E9E3"), // --fg: bone
		Scheme: [16]string{
			"#202221", // black
			"#FF9B7D", // red
			"#C7F699", // green
			"#D8BD51", // yellow
			"#98BFFF", // blue
			"#C8ABFF", // magenta
			"#28D6DF", // cyan
			"#AEB1AD", // white
			"#878A86", // brightBlack
			"#FFC0AE", // brightRed
			"#ADE670", // brightGreen
			"#F2CF3B", // brightYellow
			"#B7D2FF", // brightBlue
			"#D7C6FF", // brightMagenta
			"#00EDF7", // brightCyan
			"#E6E9E3", // brightWhite
		},
		Accent:        "#B9F27C", // --acc: what needs you
		Shimmer:       "#C7F699", // --acc-text
		AtWork:        "#C8ABFF", // --uv-text: black light, a mark at work
		AtWorkShimmer: "#D7C6FF", // brightMagenta: black light a step further from the page
		Border:        "#272927", // --line
		Surface:       "#181A19", // --surface
		Running:       "#E6E9E3", // --fg: running has no hue
		Turning:       "#C8ABFF", // --uv-text
		Gray:          "#878A86", // --mute
		Faint:         "#626562", // --faint
		Parchment:     "#AEB1AD", // --fg-2
		Block:         "#B9F27C", // --acc
		OnBlock:       "#101211", // --acc-on
		Chosen:        "#11190A", // --acc-bg: the selection
		ChosenInk:     "#C7F699", // --acc-text
		Cursor:        "#B9F27C", // --acc

		MessageBg:       "#202221", // --surface-active
		MessageHoverBg:  "#272927", // --line
		ToolBg:          "#181A19", // --surface
		DiffAddedBg:     "#11190A", // --acc-bg
		DiffRemovedBg:   "#1C110E", // --bad-bg
		DiffAddedDim:    "#11160E", // --acc-bg half into --bg
		DiffRemovedDim:  "#161210", // --bad-bg half into --bg
		DiffAddedWord:   "#344B19", // --acc-line
		DiffRemovedWord: "#53271B", // --bad-line
	},
	Light: Ground{
		Ground: RGB("#EDEFEA"), // --bg: light
		Ink:    RGB("#151814"), // --fg: ink
		Scheme: [16]string{
			"#151814", // black
			"#9D2800", // red
			"#416B10", // green
			"#6F5C00", // yellow
			"#2A5AAB", // blue
			"#6934C5", // magenta
			"#006B70", // cyan
			"#626560", // white
			"#434642", // brightBlack
			"#802200", // brightRed
			"#2F5300", // brightGreen
			"#564700", // brightYellow
			"#03409C", // brightBlue
			"#4F2C93", // brightMagenta
			"#005256", // brightCyan
			"#7C7F7A", // brightWhite
		},
		Accent:        "#416B10", // --acc-text: phosphor as ink, which the ground would wash out
		Shimmer:       "#2F5300", // brightGreen: phosphor a step further from the page
		AtWork:        "#6934C5", // --uv-text
		AtWorkShimmer: "#4F2C93", // brightMagenta
		Border:        "#D2D4CF", // --line
		Surface:       "#E3E5E0", // --bg-sidebar
		Running:       "#151814", // --fg: running has no hue
		Turning:       "#6934C5", // --uv-text
		Gray:          "#626560", // --mute
		Faint:         "#7C7F7A", // --faint
		Parchment:     "#434642", // --fg-2
		Block:         "#9BE05A", // --acc: the fill of a button or a badge
		OnBlock:       "#151814", // --acc-on: ink, since the fill is too pale for the ground
		Chosen:        "#E5F5D9", // --acc-bg: the selection
		ChosenInk:     "#416B10", // --acc-text
		Cursor:        "#416B10", // --acc-text: --acc is too pale for a cursor on the ground

		MessageBg:       "#DEE0DB", // --surface-active
		MessageHoverBg:  "#D2D4CF", // --line
		ToolBg:          "#E3E5E0", // --bg-sidebar
		DiffAddedBg:     "#E5F5D9", // --acc-bg
		DiffRemovedBg:   "#FBEAE5", // --bad-bg
		DiffAddedDim:    "#E9F2E2", // --acc-bg half into --bg
		DiffRemovedDim:  "#F4EDE8", // --bad-bg half into --bg
		DiffAddedWord:   "#B5DC98", // --acc-line
		DiffRemovedWord: "#F7BEAE", // --bad-line
	},
}
