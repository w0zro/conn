package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
)

// dressProgram writes a theme for a program conn holds but cannot dress
// through its server, since the program writes its own hex rather than
// asking for a color by name: Claude Code and nvim. It answers
// what to say and whether it went well; ask says yes to a question, and
// is nil where there is nobody to ask.
func dressProgram(args []string, home string, ask func(string) bool) (string, bool) {
	if len(args) != 1 {
		return "conn theme: say which program: conn theme claude, conn theme vim\n", false
	}
	// The theme drawn matches whichever ground the server on this
	// machine is running, or would come up on if none is up yet - not
	// an argument of its own, so it never drifts from what conn itself
	// is dressed in.
	applyMode(serverMode(socketPath(home)))
	switch args[0] {
	case "claude":
		return dressClaude(home, ask)
	case "vim", "nvim":
		return dressVim(home)
	}
	return fmt.Sprintf("conn theme: conn has no theme for %s; it has one for claude and one for vim\n", args[0]), false
}

// dressClaude writes the theme, and offers to put Claude Code on it
// when nothing of the user's own is in the way: a settings file on one
// of the themes Claude Code comes with, or on none. A custom theme is
// somebody's own doing, and conn says what it is and leaves it.
func dressClaude(home string, ask func(string) bool) (string, bool) {
	path, err := writeClaudeTheme(home)
	if err != nil {
		return fmt.Sprintf("conn theme: %v\n", err), false
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Wrote conn's theme for Claude Code to %s\n", tilde(path, home))
	in, ok := themeInUse(home)
	switch {
	case !ok:
		fmt.Fprintf(&b, "Claude Code has no settings file yet; it will read the theme once %q is its theme.\n", claudeThemeRef)
	case in == claudeThemeRef:
		// Already on it, and the themes directory is watched: a session
		// that is up has the new colors already.
	case !builtinThemes[in]:
		fmt.Fprintf(&b, "Claude Code is on %s, which is not conn's to change. Pick Conn with /theme when you want it.\n", in)
	case ask != nil && ask("Put Claude Code on it now?"):
		if err := useClaudeTheme(home); err != nil {
			fmt.Fprintf(&b, "conn theme: %v\n", err)
			return b.String(), false
		}
		fmt.Fprintf(&b, "Claude Code is on %s. A session that is up picks it up as the directory is watched.\n", claudeThemeRef)
	default:
		fmt.Fprintf(&b, "Pick Conn with /theme, or set %q as the theme in ~/.claude/settings.json.\n", claudeThemeRef)
	}
	return b.String(), true
}

// asks is how conn puts a yes-or-no question, when there is somebody at
// the terminal to answer it and nowhere for the answer to be piped from.
func asks() func(string) bool {
	if !stdoutIsTerminal() || !stdinIsTerminal() {
		return nil
	}
	return func(q string) bool {
		fmt.Printf("%s [y/N] ", q)
		var answer string
		if _, err := fmt.Scanln(&answer); err != nil {
			return false // nothing typed, or no line to read: that is a no
		}
		answer = strings.ToLower(strings.TrimSpace(answer))
		return answer == "y" || answer == "yes"
	}
}

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
	args, override, err := parseModeFlags(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "conn:", err)
		os.Exit(2)
	}
	if len(args) > 0 {
		os.Exit(runCommand(args[0], args[1:]))
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
			code, err = srv.attach(self, home, override)
			if err == nil {
				os.Exit(code)
			}
		}
		// The server could not be brought up: conn goes on without it, and
		// says so.
		fmt.Fprintf(os.Stderr, "conn: the tmux server could not be brought up: %v\n", err)
		srv = nil
	}
	// A pane of conn's own server draws on the ground the server already
	// chose; anything else - no tmux, or the server could not come up -
	// has nobody to ask but the terminal itself, or --light/--dark.
	if inside {
		applyMode(serverMode(srv.socket))
	} else {
		applyMode(askDark(override))
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

// A command conn answers to by name, after conn itself: what it is
// called, a line on it, and what it does, answering the exit status.
type command struct {
	name string
	use  string
	run  func(args []string) int
}

// The commands. hold and look are conn's own, run in a pane of its
// server, and are not offered.
var commands = []command{
	{"down", "take the server down, with everything in it", func([]string) int {
		home, _ := os.UserHomeDir()
		return say(takeDown(findServer(home), home))
	}},
	{"theme", "write conn's theme for a program that draws its own: claude, vim", func(args []string) int {
		home, _ := os.UserHomeDir()
		return say(dressProgram(args, home, asks()))
	}},
	{"look", "", func(args []string) int {
		home, _ := os.UserHomeDir()
		pid := 0
		if len(args) > 0 {
			pid, _ = strconv.Atoi(args[0])
		}
		applyMode(serverMode(socketPath(home)))
		if err := runLook(findServer(home), pid, home, colored()); err != nil {
			fmt.Fprintf(os.Stderr, "conn look: %v\n", err)
			return 1
		}
		return 0
	}},
	{"hold", "", func([]string) int {
		home, _ := os.UserHomeDir()
		applyMode(serverMode(socketPath(home)))
		if err := runHold(findServer(home), colored()); err != nil {
			fmt.Fprintf(os.Stderr, "conn hold: %v\n", err)
			return 1
		}
		return 0
	}},
}

// runCommand runs the command of a name; a name conn does not know is
// said, with the names it does.
func runCommand(name string, args []string) int {
	for _, c := range commands {
		if c.name == name {
			return c.run(args)
		}
	}
	fmt.Fprint(os.Stderr, usage(name))
	return 2
}

// usage is what conn says of a name it does not know.
func usage(name string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "conn: no such command: %s\n\nconn alone comes up on the console and the watch. The commands:\n", name)
	for _, c := range commands {
		if c.use != "" {
			fmt.Fprintf(&b, "  conn %-6s  %s\n", c.name, c.use)
		}
	}
	return b.String()
}

// say prints a message where it goes, out or err, and answers the exit
// status: 0 when it went well.
func say(msg string, ok bool) int {
	if !ok {
		fmt.Fprint(os.Stderr, msg)
		return 1
	}
	fmt.Print(msg)
	return 0
}
