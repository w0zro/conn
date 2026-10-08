// Package shell is the few words conn writes into a shell line: a word
// quoted so the shell takes it whole, and the line that holds a pane
// open once its work is done.
package shell

import "strings"

// Quote quotes a word for a shell line, so the shell takes it whole:
// a path with a space or a quote in it is one argument.
func Quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// HoldOpen keeps a pane standing after what it was opened for has
// finished. cat with nothing to read waits on the terminal for as long
// as the pane is there, which is exactly as long as wanted: the operator
// leaves by going somewhere else, and the pane goes when its work is
// replaced in the workspace.
//
// It is for a pane with something left to read in it — a log that ended,
// an error docker printed. A pane whose work is over and has left
// nothing behind should go, and a shell is that.
const HoldOpen = "exec cat"
