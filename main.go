package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
)

// conn comes up on its boot console, reads out the machine, runs its
// start-up checks, and continues to the watch. On a machine with tmux
// the first conn brings up a tmux server of its own, with conn as the
// rail of its home window and the slot beside it, and puts the terminal
// on it; a later conn attaches to what is there. conn down takes the
// server down with everything in it. Without tmux, conn shows the
// console and the watch and can reach nothing. Off a terminal it writes
// the console and is done.
// Everything else it was is in the history, and comes back piece by
// piece, in the form it is wanted in.
func main() {
	if holdEnv(os.Args) {
		home, _ := os.UserHomeDir()
		if err := runHold(findServer(home), colored()); err != nil {
			fmt.Fprintf(os.Stderr, "conn hold: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "console" {
		m := newModel(colored())
		m.consoleOnly = true
		if _, err := tea.NewProgram(m, programOptions()...).Run(); err != nil {
			fmt.Fprintf(os.Stderr, "conn console: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "down" {
		home, _ := os.UserHomeDir()
		msg, ok := takeDown(findServer(home), home)
		if !ok {
			fmt.Fprint(os.Stderr, msg)
			os.Exit(1)
		}
		fmt.Print(msg)
		return
	}
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
	m.self, _ = os.Executable()
	if _, err := tea.NewProgram(m, programOptions()...).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "conn: %v\n", err)
		os.Exit(1)
	}
}

// programOptions are the options both of conn's programs run with. The
// palette is truecolor, and a terminal that says COLORTERM=truecolor is
// taken at its word: under tmux the renderer would otherwise ask tmux
// about the terminal's terminfo, which says nothing of the RGB the
// server was told to use, and draw the palette in 256 colors.
func programOptions() []tea.ProgramOption {
	switch strings.ToLower(os.Getenv("COLORTERM")) {
	case "truecolor", "24bit":
		return []tea.ProgramOption{tea.WithColorProfile(colorprofile.TrueColor)}
	}
	return nil
}
