package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/w0zro/conn/internal/work/docker"
)

// conn l is what a process said: the lines its pane holds, where it
// runs in conn's server, or what docker kept of a container. The agents
// read the panes of a project's servers with tmux capture-pane, by a
// pane they had to find first; this reads them by the process.

// lLines is how many lines conn l writes when not told: the end of
// what a process said, which is what is read for.
const lLines = 100

// runL writes what the process an address names has said.
func runL(w io.Writer, home, dir string, args []string) int {
	fs := flag.NewFlagSet("l", flag.ContinueOnError)
	n := fs.Int("n", lLines, "the last n lines; 0 for all it holds")
	fs.SetOutput(io.Discard)
	words, err := parseAnywhere(fs, args)
	if err != nil || len(words) != 1 || *n < 0 {
		return usage("l", "conn l [-n lines] name|:port|pid")
	}
	h, err := readHere("l", home, dir)
	if err != nil {
		return fail("l", err)
	}
	f, err := h.find(words[0])
	if err != nil {
		return fail("l", err)
	}
	lines, err := h.said(f, *n)
	if err != nil {
		return fail("l", err)
	}
	for _, l := range lines {
		fmt.Fprintln(w, l)
	}
	return 0
}

// said is the last n lines a row has said, or all of them for 0: its
// pane's, or its container's log.
func (h here) said(f found, n int) ([]string, error) {
	if f.Container != "" {
		args := []string{"logs"}
		if n > 0 {
			args = append(args, "--tail", strconv.Itoa(n))
		}
		out, err := docker.Says(docker.Wait, append(args, f.Container)...)
		if err != nil {
			return nil, fmt.Errorf("docker would not say what %s logged: %v", nameOf(f.Entry), err)
		}
		return strings.Split(strings.TrimRight(string(out), "\n"), "\n"), nil
	}
	p, ok := h.paneOf(f.Entry)
	switch {
	case !ok && f.TTY == "":
		return nil, errors.New(nameOf(f.Entry) + " has no terminal; what it writes goes where whatever started it sent it")
	case !ok:
		return nil, errors.New(nameOf(f.Entry) + " runs in a terminal conn does not hold, and what it said is that terminal's")
	}
	lines, _, err := h.srv.Scrollback(p.ID)
	if err != nil {
		return nil, err
	}
	// The screen below the last line written is blank rows, not
	// something said.
	lines = trimBlank(lines)
	if n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, nil
}
