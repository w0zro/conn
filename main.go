package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
)

// conn comes up on its boot console and holds it until ctrl+c. Off a
// terminal, it writes the console and is done. Everything it was is in
// the history, and comes back piece by piece, in the form it is wanted in.
func main() {
	if !stdoutIsTerminal() {
		for _, r := range screen(stationReport(), minCols, 0) {
			fmt.Println(r.text)
		}
		return
	}
	pal = colored()
	if _, err := tea.NewProgram(newModel()).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "conn: %v\n", err)
		os.Exit(1)
	}
}
