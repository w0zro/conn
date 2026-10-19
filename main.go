// conn: one operator, many processes.
//
// conn opens on the projects work has happened in, read from what the
// machine records. In a project it lists what runs and the
// conversations Claude Code has had there; enter goes to one, picking it
// back up where it is at rest.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/w0zro/conn/internal/theme"
	"github.com/w0zro/conn/internal/tmux"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "conn:", err)
		os.Exit(1)
	}
}

// commands are the verbs for whoever has no panel, by name.
var commands = map[string]func(w io.Writer, home, dir string, args []string) int{
	"ls":   runLs,
	"l":    runL,
	"u":    runU,
	"x":    runX,
	"r":    runR,
	"down": runDown,
}

func run(args []string) error {
	args, want, err := theme.ParseFlags(args)
	if err != nil {
		return err
	}
	if len(args) > 0 {
		switch args[0] {
		case "panel":
			return runPanel()
		case "hold":
			return runHold()
		case "theme":
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			return runTheme(os.Stdout, home, want, args[1:])
		}
		if cmd, ok := commands[args[0]]; ok {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			os.Exit(cmd(os.Stdout, home, dir, args[1:]))
		}
		return fmt.Errorf("conn knows ls, l, u, x, r, down and theme; with nothing after it, it opens on your projects")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	srv, err := findServer(home)
	if err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	// From inside conn, conn is ctrl-space away; this is the same. A
	// flag given there puts the server on the ground it names where it
	// stands.
	if insideConn(srv.Socket) {
		if want.Any() {
			return reground(srv, self, home, want)
		}
		own, err := findPanel(srv, "")
		if err != nil || own.pane == "" {
			return fmt.Errorf("conn's own window is gone; leave with ctrl-b d and run conn again")
		}
		_, err = srv.Do(tmux.Cmd{"select-window", "-t", own.window}, tmux.SelectPane(own.pane))
		return err
	}
	code, err := attach(srv, self, home, want)
	if errors.Is(err, errEarlier) {
		if !ask("An earlier conn's server is up. End it, and everything running in it?") {
			return fmt.Errorf("left the earlier conn running")
		}
		if err := endServer(srv); err != nil {
			return err
		}
		code, err = attach(srv, self, home, want)
	}
	if err != nil {
		return err
	}
	if code != 0 {
		os.Exit(code)
	}
	return nil
}

// ask puts a yes-or-no question on the terminal; no is the answer to
// anything but yes.
func ask(q string) bool {
	fmt.Fprintf(os.Stderr, "%s [y/N] ", q)
	a, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	a = strings.ToLower(strings.TrimSpace(a))
	return a == "y" || a == "yes"
}

// runTheme writes conn's theme for a program that draws its own hex
// rather than asking for a color by name, which is the one thing in a
// pane conn's server cannot dress: Claude Code. The ground is what a
// flag says, what the server is on, or the terminal's own.
func runTheme(w io.Writer, home string, want theme.Want, args []string) error {
	if len(args) != 1 || args[0] != "claude" {
		return fmt.Errorf("conn theme claude writes conn's theme for Claude Code")
	}
	srv, err := findServer(home)
	if err != nil {
		return err
	}
	g, err := askGround(srv, home, want)
	if err != nil {
		return err
	}
	return dressClaude(w, home, ask, g)
}

// dressClaude writes the theme, and offers to put Claude Code on it
// when nothing of the user's own is in the way: a settings file on one
// of the themes Claude Code comes with, or on none. A custom theme is
// somebody's own doing, and conn says what it is and leaves it.
func dressClaude(w io.Writer, home string, ask func(string) bool, g theme.Ground) error {
	path, err := theme.WriteClaudeTheme(home, g)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "Wrote conn's theme for Claude Code to %s\n", tilde(path, home))
	in, ok := theme.ThemeInUse(home)
	switch {
	case !ok:
		_, _ = fmt.Fprintf(w, "Claude Code has no settings file yet; it will read the theme once %q is its theme.\n", theme.ClaudeThemeRef)
	case in == theme.ClaudeThemeRef:
		// Already on it, and the themes directory is watched: a session
		// that is up has the new colors already.
	case !theme.BuiltinThemes[in]:
		_, _ = fmt.Fprintf(w, "Claude Code is on %s, which is not conn's to change. Pick Conn with /theme when you want it.\n", in)
	case ask("Put Claude Code on it now?"):
		if err := theme.UseClaudeTheme(home); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(w, "Claude Code is on %s. A session that is up picks it up as the directory is watched.\n", theme.ClaudeThemeRef)
	default:
		_, _ = fmt.Fprintf(w, "Pick Conn with /theme, or set %q as the theme in ~/.claude/settings.json.\n", theme.ClaudeThemeRef)
	}
	return nil
}
