package main

import "strings"

// The name is set the way the old logon screens set theirs: each letter
// drawn large out of copies of itself, seven rows tall, two characters to
// a stroke, on a grid of plain text.

const nameSet = "CONN"

// glyphs are the letters, a row per string, # where the letter goes.
var glyphs = map[rune][]string{
	'C': {
		"  ######  ",
		" ##    ## ",
		"##        ",
		"##        ",
		"##        ",
		" ##    ## ",
		"  ######  ",
	},
	'O': {
		"  ######  ",
		" ##    ## ",
		"##      ##",
		"##      ##",
		"##      ##",
		" ##    ## ",
		"  ######  ",
	},
	'N': {
		"##      ##",
		"###     ##",
		"## ##   ##",
		"##  ##  ##",
		"##   ## ##",
		"##     ###",
		"##      ##",
	},
}

// letterGap is the columns between letters.
const letterGap = 3

// letters sets a word in the face and returns its rows.
func letters(word string) []string {
	var rows []string
	for _, r := range word {
		g := glyphs[r]
		if rows == nil {
			rows = make([]string, len(g))
		}
		for i := range g {
			if rows[i] != "" {
				rows[i] += strings.Repeat(" ", letterGap)
			}
			rows[i] += strings.ReplaceAll(g[i], "#", string(r))
		}
	}
	return rows
}
