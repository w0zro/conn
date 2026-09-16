package main

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"
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
// The row goes with the pid. The panel and the page each read the
// process table, and the table is the same, but what the panel says of
// a row is not all read off the table: whether it is working is read
// off the processor time between two of the panel's own readings, what
// a contact is doing is read off its transcript, and how long a row
// has stood as it does is dated from the panel's own eye. A page that
// worked those out again for itself worked them out from a different
// pair of readings, and said ACTIVE of a row the panel beside it said
// was WORKING. So the panel says the row as it shows it, and the page
// says the same row: the one thing the operator is looking at is said
// the same way on both sides of the border.
//
// It is named after the socket rather than put in the state directory
// by name, so two servers on one machine each have their own and
// neither moves the other's cursor.

// cursorPath is where a server's panel publishes its cursor.
func cursorPath(home string) string { return socketPath(home) + ".cursor" }

// A cursorNote is what the panel says beside the pid: the row as it
// shows it, and the container where the row is one. A container is
// said in full because the page has no other way to learn it — every
// other row the readout can look up for itself, but a container is
// docker's to know, and only the panel is talking to docker.
type cursorNote struct {
	Row       *entry     `json:"row,omitempty"`
	Container *container `json:"container,omitempty"`
}

// tellCursor publishes the pid under the panel's cursor, with the row
// it stands on. Nothing waits on this and nothing is broken by its
// failing: a readout that cannot read the cursor holds the subject it
// has, which is the same thing it does between one move and the next.
func tellCursor(path string, pid int, row *entry, c *container) {
	line := strconv.Itoa(pid)
	if row != nil || c != nil {
		if b, err := json.Marshal(cursorNote{Row: row, Container: c}); err == nil {
			line += "\n" + string(b)
		}
	}
	_ = os.WriteFile(path, []byte(line), 0o600)
}

// readCursor is the note as the file holds it, or nothing where there
// is no file to read. The page compares one reading with the last to
// know whether anything changed, which is cheaper than working out
// what.
func readCursor(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

// parseCursor reads a note: the pid the panel's cursor is on, or 0
// where there is none to read — no file yet, a panel that never
// published, a server that is not this one — and the row and the
// container the panel said beside it.
func parseCursor(note string) (int, *entry, *container) {
	head, rest, _ := strings.Cut(note, "\n")
	pid, err := strconv.Atoi(strings.TrimSpace(head))
	if err != nil {
		return 0, nil, nil
	}
	var n cursorNote
	if rest = strings.TrimSpace(rest); rest != "" && json.Unmarshal([]byte(rest), &n) != nil {
		return pid, nil, nil
	}
	if n.Container != nil && n.Container.id == "" {
		n.Container = nil
	}
	return pid, n.Row, n.Container
}

// askCursor is the note read and parsed in one go.
func askCursor(path string) (int, *entry, *container) {
	return parseCursor(readCursor(path))
}

// entryWire is a row as it crosses between conn's programs, the way
// containerWire is a container: the same fields, exported so that
// encoding/json carries them, kept beside the type it mirrors.
type entryWire struct {
	PID                       int
	Kind, Command, Typed, TTY string
	Started                   time.Time
	Status                    string
	Fault                     bool
	Depth                     int
	Since                     time.Time
	Cwd, Asking, Doing        string
	Container                 string
}

func (e entry) MarshalJSON() ([]byte, error) {
	return json.Marshal(entryWire{
		PID: e.pid, Kind: e.kind, Command: e.command, Typed: e.typed, TTY: e.tty,
		Started: e.started, Status: e.status, Fault: e.fault, Depth: e.depth,
		Since: e.since, Cwd: e.cwd, Asking: e.asking, Doing: e.doing, Container: e.container,
	})
}

func (e *entry) UnmarshalJSON(b []byte) error {
	var w entryWire
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	*e = entry{
		pid: w.PID, kind: w.Kind, command: w.Command, typed: w.Typed, tty: w.TTY,
		started: w.Started, status: w.Status, fault: w.Fault, depth: w.Depth,
		since: w.Since, cwd: w.Cwd, asking: w.Asking, doing: w.Doing, container: w.Container,
	}
	return nil
}
