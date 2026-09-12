package main

import (
	"os"
	"strconv"
	"strings"
)

// Where the rail's cursor is, for the look to follow. The two are
// separate programs in separate panes — the rail cannot draw in the
// slot and the look cannot see the rail's model — so the cursor has to
// travel between them somehow.
//
// It travels as a few bytes in a file beside the socket. The rail
// writes the pid under its cursor whenever that changes; the look reads
// it on a poll and changes subject when it moves. A file rather than a
// tmux option because the look has to read it often to keep up with
// j held down, and every tmux reading is a process: a write here is a
// handful of microseconds and a read is one open, where asking tmux
// three times a second would be three execs a second forever.
//
// It is named after the socket rather than put in the state directory
// by name, so two servers on one machine each have their own and
// neither moves the other's cursor.

// cursorPath is where a server's rail publishes its cursor.
func cursorPath(home string) string { return socketPath(home) + ".cursor" }

// tellCursor publishes the pid under the rail's cursor. Nothing waits
// on this and nothing is broken by its failing: a look that cannot read
// the cursor holds the subject it has, which is the same thing it does
// between one move and the next.
func tellCursor(path string, pid int) {
	_ = os.WriteFile(path, []byte(strconv.Itoa(pid)), 0o600)
}

// askCursor is the pid the rail's cursor is on, or 0 where there is
// none to read — no file yet, a rail that never published, a server
// that is not this one.
func askCursor(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return 0
	}
	return pid
}
