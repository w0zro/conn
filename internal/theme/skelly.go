package theme

// Skelly is skelly's, the app at SkellyLabs: the palette its stylesheet
// calls Glow. A green-tinted black and a bone ink, grays for everything
// that is only information, and one phosphor green kept for what needs
// you, which is what conn's accent already means: the WAITING stamp,
// the caution, the cursor. Black light, skelly's third hue, is the
// spinner beside a contact at work, and the magentas. Skelly draws a
// thing running in plain ink rather than a hue, and conn does the same
// here. Where skelly has a token the role takes it, and says which;
// skelly is a web page and has no sixteen, so the rest of the hues a
// program asks for by name are conn's own, drawn to sit in its
// temperature, with the phosphor as bright green and rose as red.
var Skelly = theme{
	Name: "skelly",
	Dark: Ground{
		Ground: RGB("#101211"), // --bg
		Ink:    RGB("#E6E9E3"), // --fg
		Scheme: [16]string{
			"#262A27", // --line
			"#F2737E", // --bad
			"#9FD3A8", // green
			"#E8C468", // yellow
			"#86AEC9", // blue
			"#C7ABFF", // --uv-text: black light
			"#7FCFC4", // cyan
			"#AEB3AB", // --fg-2
			"#5D635A", // --faint
			"#F59AA2", // --bad-text
			"#B9F27C", // --acc: the phosphor
			"#F2D58A", // bright yellow
			"#A6C6DD", // bright blue
			"#DCCBFF", // --uv-text-soft
			"#9FDFD6", // bright cyan
			"#E6E9E3", // --fg
		},
		Accent:    "#B9F27C", // --acc: what needs you
		Shimmer:   "#D9F8B8", // --acc-text-soft
		Border:    "#262A27", // --line
		Surface:   "#171A18", // --surface
		Running:   "#E6E9E3", // --run: running is ink, not a hue
		Turning:   "#C7ABFF", // --uv-text: black light
		Gray:      "#868C83", // --mute
		Faint:     "#5D635A", // --faint
		Parchment: "#AEB3AB", // --fg-2
		Block:     "#B9F27C", // --acc
		OnBlock:   "#101211", // --acc-on
		Chosen:    "#18240F", // --sel-bg: selected keeps its phosphor
		ChosenInk: "#D9F8B8", // --sel-fg
		Cursor:    "#B9F27C", // --sel-line: the ring around the box you type in

		MessageBg:       "#1F2320", // --surface-active
		MessageHoverBg:  "#262A27", // --line
		ToolBg:          "#171A18", // --surface
		DiffAddedBg:     "#1B2A14",
		DiffRemovedBg:   "#301A1D",
		DiffAddedDim:    "#151D11",
		DiffRemovedDim:  "#211517",
		DiffAddedWord:   "#2E4720",
		DiffRemovedWord: "#4F262C",
	},
	Light: Ground{
		Ground: RGB("#EDEFEA"), // --bg
		Ink:    RGB("#151814"), // --fg
		Scheme: [16]string{
			"#262A24", // --fg-approved: black is text, on paper
			"#B4233A", // --bad
			"#2E6B2A", // green
			"#7E5800", // yellow
			"#2F5F82", // blue
			"#5B35C7", // --uv-text: black light
			"#0E6B66", // cyan
			"#434840", // --fg-2
			"#7A8075", // --faint
			"#9E1F33", // --bad-text
			"#3F6B12", // --acc-text: the phosphor, as ink
			"#684800", // bright yellow
			"#244D6B", // bright blue
			"#4B2BA8", // --uv-text-soft
			"#0A5753", // bright cyan
			"#151814", // --fg
		},
		Accent:    "#3F6B12", // --acc-text: the phosphor as ink, which the ground would wash out
		Shimmer:   "#26440A", // --sel-fg: on paper, more ink
		Border:    "#D5D9D1", // --line
		Surface:   "#E3E6DF", // --bg-sidebar
		Running:   "#434840", // --run
		Turning:   "#5B35C7", // --uv-text
		Gray:      "#646A60", // --mute
		Faint:     "#7A8075", // --faint
		Parchment: "#434840", // --fg-2
		Block:     "#9BE05A", // --acc: the fill skelly's buttons and badges are
		OnBlock:   "#151814", // --acc-on: ink, since the fill is too pale for the ground
		Chosen:    "#E0F1CC", // --sel-bg
		ChosenInk: "#26440A", // --sel-fg
		Cursor:    "#5DB821", // --sel-line

		MessageBg:       "#DDE1D8", // --surface-active
		MessageHoverBg:  "#D5D9D1", // --line
		ToolBg:          "#E3E6DF", // --bg-sidebar
		DiffAddedBg:     "#DCEBC8",
		DiffRemovedBg:   "#F2DCDF",
		DiffAddedDim:    "#E5EDDB",
		DiffRemovedDim:  "#F0E5E6",
		DiffAddedWord:   "#B3D58C",
		DiffRemovedWord: "#E2AEB5",
	},
}
