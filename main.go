package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
)

// conn comes up on its start-up screen and holds it until ctrl+c. Off a
// terminal, it writes the screen and is done. Everything it was is in the
// history, and comes back piece by piece, in the form it is wanted in.
func main() {
	if !stdoutIsTerminal() {
		fmt.Println(joinRows(screen(stationReport(), minCols, minRows)))
		return
	}
	bright, alarm, normal = "\x1b[1m", "\x1b[7m", "\x1b[0m"
	if _, err := tea.NewProgram(newModel()).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "conn: %v\n", err)
		os.Exit(1)
	}
}

const greeting = "hello from the conn"
