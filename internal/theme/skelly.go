package theme

// Skelly is skelly's, from its brand at brand.skelly.ai: the palette it
// calls Glow, five colors and rules for the rest. A green-tinted black
// and a bone ink, and three glows in the dark. Phosphor is what needs
// you, which is what conn's accent already means: the WAITING stamp,
// the caution, the cursor, the selection. Neon is broke, the reds.
// Black light is the lamp: the spinner beside a contact at work, and
// the magentas. A thing running has no hue, so it is drawn in ink.
//
// Every other skelly token is a rule on the five - a surface is the
// ground a step lighter, a line or a gray is the ink mixed into the
// ground, a glow's text, line and tint are the glow at a set lightness
// and chroma with its hue kept - and the values here are those rules
// as the brand page prints them, named by token. Where conn needs a
// step the brand has no token for, the same kind of rule makes it and
// the comment says so. skelly is a web page and has no sixteen, so the
// green, yellow, blue and cyan a program asks for by name are conn's
// own, drawn to sit in its temperature.
var Skelly = theme{
	Name: "skelly",
	Dark: Ground{
		Ground: RGB("#101211"), // --bg: lights-out
		Ink:    RGB("#E6E9E3"), // --fg: bone
		Scheme: [16]string{
			"#272927", // --line
			"#FF7E5B", // --bad: neon
			"#9FD3A8", // green
			"#E8C468", // yellow
			"#86AEC9", // blue
			"#C8ABFF", // --uv-text: black light
			"#7FCFC4", // cyan
			"#AEB1AD", // --fg-2
			"#626562", // --faint
			"#FF9B7D", // --bad-text
			"#B9F27C", // --acc: phosphor
			"#F2D58A", // bright yellow
			"#A6C6DD", // bright blue
			"#DDCFFF", // black light at l 0.88, c 0.08: a step past --uv-text
			"#9FDFD6", // bright cyan
			"#E6E9E3", // --fg
		},
		Accent:    "#B9F27C", // --acc: what needs you
		Shimmer:   "#C7F699", // --acc-text
		Border:    "#272927", // --line
		Surface:   "#181A19", // --surface
		Running:   "#E6E9E3", // --fg: running has no hue
		Turning:   "#C8ABFF", // --uv-text
		Gray:      "#878A86", // --mute
		Faint:     "#626562", // --faint
		Parchment: "#AEB1AD", // --fg-2
		Block:     "#B9F27C", // --acc
		OnBlock:   "#101211", // --acc-on
		Chosen:    "#11190A", // --acc-bg: the selection
		ChosenInk: "#C7F699", // --acc-text
		Cursor:    "#B9F27C", // --acc

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
			"#151814", // --fg: black is text, on paper
			"#B5380A", // --bad: neon
			"#2E6B2A", // green
			"#7E5800", // yellow
			"#2F5F82", // blue
			"#6934C5", // --uv-text: black light
			"#0E6B66", // cyan
			"#434642", // --fg-2
			"#7C7F7A", // --faint
			"#9D2800", // --bad-text
			"#416B10", // --acc-text: phosphor, as ink
			"#684800", // bright yellow
			"#244D6B", // bright blue
			"#521FA1", // black light at l 0.40, c 0.19: on paper, more ink
			"#0A5753", // bright cyan
			"#151814", // --fg
		},
		Accent:    "#416B10", // --acc-text: phosphor as ink, which the ground would wash out
		Shimmer:   "#305302", // phosphor at l 0.40, c 0.11: on paper, more ink
		Border:    "#D2D4CF", // --line
		Surface:   "#E3E5E0", // --bg-sidebar
		Running:   "#151814", // --fg: running has no hue
		Turning:   "#6934C5", // --uv-text
		Gray:      "#626560", // --mute
		Faint:     "#7C7F7A", // --faint
		Parchment: "#434642", // --fg-2
		Block:     "#9BE05A", // --acc: the fill of a button or a badge
		OnBlock:   "#151814", // --acc-on: ink, since the fill is too pale for the ground
		Chosen:    "#E5F5D9", // --acc-bg: the selection
		ChosenInk: "#416B10", // --acc-text
		Cursor:    "#416B10", // --acc-text: --acc is too pale for a cursor on the ground

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
