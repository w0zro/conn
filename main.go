package main

import (
	"fmt"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
)

// conn comes up on its boot console, reads out the machine, runs its
// start-up checks, and continues to the watch. On a machine with tmux
// the first conn brings up a tmux server of its own, with conn in its
// watch window, and puts the terminal on it; a later conn attaches to
// what is there. Without tmux, conn shows the console and the watch and
// can reach nothing. Off a terminal it writes the console and is done.
// Everything else it was is in the history, and comes back piece by
// piece, in the form it is wanted in.
func main() {
	if !stdoutIsTerminal() {
		for _, r := range screen(compose(readStation(), time.Now()), minCols, 0, plain) {
			fmt.Println(r.text)
		}
		return
	}
	home, _ := os.UserHomeDir()
	srv := findServer(home)
	inside := srv != nil && insideConn(os.Getenv("TMUX"), srv.socket)
	if srv != nil && !inside {
		self, err := os.Executable()
		if err == nil {
			var code int
			code, err = srv.attach(self, home)
			if err == nil {
				os.Exit(code)
			}
		}
		// The server could not be brought up: conn goes on without it, and
		// says so.
		fmt.Fprintf(os.Stderr, "conn: the tmux server could not be brought up: %v\n", err)
		srv = nil
	}
	m := newModel(colored())
	m.srv, m.inside = srv, inside
	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "conn: %v\n", err)
		os.Exit(1)
	}
}
