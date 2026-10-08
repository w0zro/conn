package theme

// Datum is datum, the colorscheme at datum.w0zro.com: one palette
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
var Datum = theme{
	Name: "datum",
	Dark: Ground{
		Ground: RGB("#0F1318"), // bg0
		Ink:    RGB("#DBE0E8"), // fg0
		Scheme: [16]string{
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
		Accent:    "#FA94CD", // purple: the cursor
		Shimmer:   "#F5B9D9", // call: purple's quiet sibling
		Border:    "#2B2F35", // bg2: the selection
		Surface:   "#181C21", // bg1: the panel's ground
		Running:   "#54DCAA", // green: datum has the one
		Gray:      "#8F98A3", // fg1
		Faint:     "#757D87", // fg1 a fifth of the way to bg0: datum's promptBorder
		Parchment: "#DBE0E8", // fg0: datum has two inks, and the bold carries a title
		Block:     "#FA94CD", // purple, with bg0 knocked out of it
		OnBlock:   "#0F1318",
		Chosen:    "#2B2F35", // bg2: the selection
		ChosenInk: "#DBE0E8",
		Cursor:    "#FA94CD",

		MessageBg:       "#181C21", // bg1: a turn of yours, at rest
		MessageHoverBg:  "#2B2F35", // bg2: under the pointer
		ToolBg:          "#181C21", // bg1: the step off the ground
		DiffAddedBg:     "#1E3F38",
		DiffRemovedBg:   "#443029",
		DiffAddedDim:    "#162727",
		DiffRemovedDim:  "#272020",
		DiffAddedWord:   "#2E6D5A",
		DiffRemovedWord: "#7B4F3A",
	},
	Light: Ground{
		Ground: RGB("#F1F6FD"), // bg0
		Ink:    RGB("#292E35"), // fg0
		Scheme: [16]string{
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
		Accent:    "#973070",
		Shimmer:   "#633750",
		Border:    "#CED3D9",
		Surface:   "#E7ECF2",
		Running:   "#007553",
		Gray:      "#616A76",
		Faint:     "#7E8691",
		Parchment: "#292E35",
		Block:     "#973070",
		OnBlock:   "#F1F6FD",
		Chosen:    "#CED3D9",
		ChosenInk: "#292E35",
		Cursor:    "#973070",

		MessageBg:       "#E7ECF2",
		MessageHoverBg:  "#CED3D9",
		ToolBg:          "#E7ECF2",
		DiffAddedBg:     "#AED2CD",
		DiffRemovedBg:   "#DBC4B6",
		DiffAddedDim:    "#D4E7E9",
		DiffRemovedDim:  "#E8E1DF",
		DiffAddedWord:   "#7DB8AB",
		DiffRemovedWord: "#CBA184",
	},
}
