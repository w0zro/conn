package main

// datumTheme is datum, the colorscheme at datum.w0zro.com: one palette
// for the whole terminal, derived rather than picked. Its hues are the
// Okabe-Ito colorblind-safe set placed in OKLCH, held to WCAG and APCA
// contrast and to every kind of color-vision deficiency, and every
// port of it is generated from one table so that none can drift. conn
// wears it the way its terminal ports do, and nothing here is conn's
// own choice. The sixteen are datum's ANSI table as its Ghostty port
// writes them; the ground and the ink are bg0 and fg0; the accent is
// purple, datum's cursor in every port; the border is bg2, its
// selection; the gray is fg1, its comment; the washes are what its
// Claude Code port lays a diff on. Where datum has no name for a role
// conn draws, the nearest thing it does have stands in, and each says
// which. The test holds every value here to the ports themselves, kept
// under testdata/datum.
//
// datum's slots 7 and 15 are the whites, as ANSI means them: fg0 and a
// white past it on dark, bg2 and bg1 on light. That is why conn reads
// its inks by name and never off those slots.
var datumTheme = theme{
	name: "datum",
	dark: ground{
		ground: rgb("#0F1318"), // bg0
		ink:    rgb("#DBE0E8"), // fg0
		scheme: [16]string{
			"#2B2F35", // bg2
			"#FE9864", // red
			"#54DCAA", // green
			"#E8DF69", // yellow
			"#69B9F7", // blue
			"#FA94CD", // purple
			"#6AE5EC", // cyan
			"#DBE0E8", // fg0
			"#8F98A3", // fg1
			"#F8BD5F", // orange
			"#54DCAA", // green again: datum has the one
			"#FCDCAD", // var, the quiet orange
			"#B2DAFB", // op, the quiet blue
			"#F5B9D9", // call, the quiet purple
			"#A5F5F9", // param, the quiet cyan
			"#EEF2F7", // a white past fg0
		},
		accent:    "#FA94CD", // purple: the cursor
		shimmer:   "#F5B9D9", // call: purple's quiet sibling
		border:    "#2B2F35", // bg2: the selection
		surface:   "#181C21", // bg1: the panel's ground
		running:   "#54DCAA", // green: datum has the one
		gray:      "#8F98A3", // fg1
		faint:     "#757D87", // fg1 a fifth of the way to bg0: datum's promptBorder
		parchment: "#DBE0E8", // fg0: datum has two inks, and the bold carries a title

		messageBg:       "#181C21", // bg1: a turn of yours, at rest
		messageHoverBg:  "#2B2F35", // bg2: under the pointer
		toolBg:          "#181C21", // bg1: the step off the ground
		diffAddedBg:     "#1E3F38",
		diffRemovedBg:   "#443029",
		diffAddedDim:    "#162727",
		diffRemovedDim:  "#272020",
		diffAddedWord:   "#2E6D5A",
		diffRemovedWord: "#7B4F3A",
	},
	light: ground{
		ground: rgb("#F1F6FD"), // bg0
		ink:    rgb("#292E35"), // fg0
		scheme: [16]string{
			"#292E35", // fg0: black is text, on paper
			"#A24500", // red
			"#007553", // green
			"#656023", // yellow
			"#0176B8", // blue
			"#973070", // purple
			"#0D7A7F", // cyan
			"#CED3D9", // bg2
			"#616A76", // fg1
			"#976700", // orange
			"#007553", // green again
			"#4D3919", // var
			"#2F516C", // op
			"#633750", // call
			"#154B4E", // param
			"#E7ECF2", // bg1
		},
		accent:    "#973070",
		shimmer:   "#633750",
		border:    "#CED3D9",
		surface:   "#E7ECF2",
		running:   "#007553",
		gray:      "#616A76",
		faint:     "#7E8691",
		parchment: "#292E35",

		messageBg:       "#E7ECF2",
		messageHoverBg:  "#CED3D9",
		toolBg:          "#E7ECF2",
		diffAddedBg:     "#AED2CD",
		diffRemovedBg:   "#DBC4B6",
		diffAddedDim:    "#D4E7E9",
		diffRemovedDim:  "#E8E1DF",
		diffAddedWord:   "#7DB8AB",
		diffRemovedWord: "#CBA184",
	},
}
