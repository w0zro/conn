package main

import "strings"

// The name is set in a face drawn for it: four letters on a grid twelve
// pixels tall, strokes three wide, corners turned. Each text row holds two
// rows of pixels, drawn with the half blocks, so the letters come out near
// square in a terminal's tall cells and stay one file of plain text.

const name = "CONN"

// glyphs are the letters, a row of pixels per string, # for ink.
var glyphs = map[rune][]string{
	'C': {
		"..######..",
		".########.",
		"###....###",
		"###.......",
		"###.......",
		"###.......",
		"###.......",
		"###.......",
		"###.......",
		"###....###",
		".########.",
		"..######..",
	},
	'O': {
		"..######..",
		".########.",
		"###....###",
		"###....###",
		"###....###",
		"###....###",
		"###....###",
		"###....###",
		"###....###",
		"###....###",
		".########.",
		"..######..",
	},
	'N': {
		"#####.....###",
		"######....###",
		"######....###",
		"###.###...###",
		"###.###...###",
		"###..###..###",
		"###..###..###",
		"###...###.###",
		"###...###.###",
		"###....######",
		"###....######",
		"###.....#####",
	},
}

// letterGap is the pixels between letters.
const letterGap = 2

// letters sets a word in the face and returns its text rows.
func letters(word string) []string {
	var rows []string
	for _, r := range word {
		g := glyphs[r]
		if rows == nil {
			rows = make([]string, len(g))
		}
		for i := range g {
			if rows[i] != "" {
				rows[i] += strings.Repeat(".", letterGap)
			}
			rows[i] += g[i]
		}
	}
	out := make([]string, 0, len(rows)/2)
	for i := 0; i+1 < len(rows); i += 2 {
		out = append(out, halfBlocks(rows[i], rows[i+1]))
	}
	return out
}

// halfBlocks draws two rows of pixels as one row of text: a full block
// where both are inked, an upper or lower half where one is, and a space
// where neither.
func halfBlocks(top, bottom string) string {
	var b strings.Builder
	for i := range top {
		switch {
		case top[i] == '#' && bottom[i] == '#':
			b.WriteRune('█')
		case top[i] == '#':
			b.WriteRune('▀')
		case bottom[i] == '#':
			b.WriteRune('▄')
		default:
			b.WriteByte(' ')
		}
	}
	return b.String()
}
