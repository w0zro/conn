package main

import (
	"fmt"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
)

// conn comes up on its boot console, reads out the machine, runs its
// start-up checks, and waits on a key. Off a terminal it writes the
// console and is done. Everything it was is in the history, and comes
// back piece by piece, in the form it is wanted in.
func main() {
	if !stdoutIsTerminal() {
		for _, r := range screen(compose(readStation(), time.Now()), minCols, 0, plain) {
			fmt.Println(r.text)
		}
		return
	}
	if _, err := tea.NewProgram(newModel(colored())).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "conn: %v\n", err)
		os.Exit(1)
	}
}
