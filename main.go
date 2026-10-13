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
	"ls": runLs,
	"l":  runL,
	"u":  runU,
	"x":  runX,
	"r":  runR,
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
		return fmt.Errorf("conn knows ls, l, u, x and r; with nothing after it, it opens on your projects")
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
		if want != nil {
			return reground(srv, self, want)
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
