package main

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
)

// Where the panel's cursor is, for the readout to follow. The two are
// separate programs in separate panes — the panel cannot draw in the
// bay and the readout cannot see the panel's model — so the cursor has
// to travel between them somehow.
//
// It travels as a few bytes in a file beside the socket. The panel
// writes the pid under its cursor whenever that changes; the readout
// reads it on a poll and changes subject when it moves. A file rather
// than a tmux option because the readout has to read it often to keep
// up with j held down, and every tmux reading is a process: a write
// here is a handful of microseconds and a read is one open, where
// asking tmux three times a second would be three execs a second
// forever.
//
// It is named after the socket rather than put in the state directory
// by name, so two servers on one machine each have their own and
// neither moves the other's cursor.

// cursorPath is where a server's panel publishes its cursor.
func cursorPath(home string) string { return socketPath(home) + ".cursor" }

// tellCursor publishes the pid under the panel's cursor. Nothing waits
// on this and nothing is broken by its failing: a readout that cannot
// read the cursor holds the subject it has, which is the same thing it
// does between one move and the next.
func tellCursor(path string, pid int, c *container) {
	line := strconv.Itoa(pid)
	// A container is said here in full, because the page has no other
	// way to learn it. Every other row the readout can look up for
	// itself — the process table holds it — but a container is docker's
	// to know, and only the panel is talking to docker. Publishing what
	// the panel already has keeps that one conversation in one process,
	// where a readout asking docker on its own beat would be a second.
	if c != nil {
		if b, err := json.Marshal(c); err == nil {
			line += "\n" + string(b)
		}
	}
	_ = os.WriteFile(path, []byte(line), 0o600)
}

// askCursor is the pid the panel's cursor is on, or 0 where there is
// none to read — no file yet, a panel that never published, a server
// that is not this one.
func askCursor(path string) (int, *container) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, nil
	}
	head, rest, _ := strings.Cut(string(b), "\n")
	pid, err := strconv.Atoi(strings.TrimSpace(head))
	if err != nil {
		return 0, nil
	}
	if rest = strings.TrimSpace(rest); rest != "" {
		var c container
		if json.Unmarshal([]byte(rest), &c) == nil && c.id != "" {
			return pid, &c
		}
	}
	return pid, nil
}
