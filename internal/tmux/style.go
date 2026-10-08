package tmux

import "strings"

// A Style is how tmux is asked to draw a run of text on the status
// line: an ink and a ground, as hexes, and whether it is bold.
type Style struct {
	FG, BG string
	Bold   bool
}

// Styled is text in a style, in tmux's markup. The attributes are
// parted by spaces and not by commas: a comma inside a style is a comma
// to any conditional around it, and tmux would read the style as the
// conditional's branches. A hash is tmux's own character on the line
// and is doubled to be shown.
func Styled(st Style, text string) string {
	weight := "nobold"
	if st.Bold {
		weight = "bold"
	}
	return "#[bg=" + st.BG + " fg=" + st.FG + " " + weight + "]" + strings.ReplaceAll(text, "#", "##")
}
