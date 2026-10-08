// Package wire is how the panel tells the pages it opens where its
// cursor is, and what it read of the machine: a note in a file beside
// the server's socket, written when the cursor moves or a reading
// lands, and read by a page on a poll.
package wire

import (
	"encoding/json"
	"os"
	"time"

	"github.com/w0zro/conn/internal/room"
	"github.com/w0zro/conn/internal/work"
	"github.com/w0zro/conn/internal/work/brew"
	"github.com/w0zro/conn/internal/work/claude"
	"github.com/w0zro/conn/internal/work/docker"
)

// Where the panel's cursor is, for the readout to follow. The two are
// separate programs in separate panes — the panel cannot draw in the
// bay and the readout cannot see the panel's model — so the cursor has
// to travel between them somehow.
//
// It travels as a file beside the socket. The panel writes the pid
// under its cursor whenever that changes; the readout reads it on a
// poll and changes subject when it moves. A file rather than a tmux
// option because the readout has to read it often to keep up with j
// held down, and every tmux reading is a process: a write here is a
// handful of microseconds and a read is one open, where asking tmux
// three times a second would be three execs a second forever.
//
// The reading goes with the pid. The page said what the panel said of
// a row by reading the machine again for itself, and the two readings
// disagreed: whether a row is working is read off the processor time
// between two of the panel's own readings, what a contact is doing off
// its transcript, how long a row has stood as it does off the panel's
// own eye, and a page that worked those out again worked them out from
// a different pair of readings. Reading the machine is a process
// besides — lsof and ps on macOS, and tmux for the panes — and the
// page was a second instrument running the panel's every beat. So the
// panel publishes its reading whole: the rows, the table's record
// behind each, the server's panes, and what docker said. The page
// reads that and asks the machine nothing; what it asks for itself is
// what is about one row only — what a contact says of its session, and
// what git says of the project.
//
// It is named after the socket rather than put in the state directory
// by name, so two servers on one machine each have their own and
// neither moves the other's cursor.

// Path is where a server's panel publishes its note.
func Path(home string) string { return room.SocketPath(home) + ".cursor" }

// A Note is what the panel publishes: what its cursor is on - a
// process by its pid, a project by its path, or a suspended session by
// its id - and the reading it stands in.
type Note struct {
	PID     int
	Path    string
	Session string
	Reading *Reading
}

// A Reading is the panel's reading of the machine, as it publishes it
// for the page: the rows as the panel shows them, the table's record
// behind each row, the server's panes and whether there is a server at
// all, the containers docker last said, which a container's row names
// by id, what brew said of its services, and the sessions the sessions
// list has in hand, which its rows name by id.
type Reading struct {
	Projects   []work.Project
	Records    map[int]Record
	Panes      map[string]room.Pane
	Inside     bool
	Containers []docker.Container
	Brews      []brew.Service
	Sessions   []claude.Session
}

// SessionOf is the suspended session of an id among those the panel
// has in hand, where it is one.
func (r Reading) SessionOf(id string) *claude.Session {
	for i := range r.Sessions {
		if r.Sessions[i].ID == id {
			return &r.Sessions[i]
		}
	}
	return nil
}

// BrewOf is the brew service a row stands for among those brew
// reported, where it is one.
func (r Reading) BrewOf(e work.Entry) *brew.Service {
	if e.Brew == "" {
		return nil
	}
	return brew.Named(r.Brews, e.Brew)
}

// ContainerOf is the container a row stands for among those docker
// said, where it is one.
func (r Reading) ContainerOf(e work.Entry) *docker.Container {
	if e.Container == "" {
		return nil
	}
	for i := range r.Containers {
		if r.Containers[i].ID == e.Container {
			return &r.Containers[i]
		}
	}
	return nil
}

// A Record is the part of the table's own record behind a row that the
// page reads and the row does not carry: the kernel's state for it,
// whether its group holds the terminal, and the processor time it has
// spent altogether.
type Record struct {
	PID        int
	State      byte
	Foreground bool
	CPU        time.Duration
}

// RecordOf is a process's record.
func RecordOf(p work.Process) Record {
	return Record{PID: p.PID, State: p.State, Foreground: p.Foreground, CPU: p.CPU}
}

// Publish writes a note. Nothing waits on this and nothing is broken by
// its failing: a page that cannot read the note holds the subject it
// has, which is the same thing it does between one move and the next.
func Publish(path string, n Note) {
	b, err := json.Marshal(n)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, b, 0o600)
}

// Read is the note as the file holds it, or nothing where there is no
// file to read. A page compares one reading of it with the last to know
// whether anything changed, which is cheaper than working out what.
func Read(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

// Parse reads a note, and says whether there was one to read: no file
// yet, a panel that never published and a server that is not this one
// are none.
func Parse(raw string) (Note, bool) {
	var n Note
	if json.Unmarshal([]byte(raw), &n) != nil {
		return Note{}, false
	}
	return n, true
}
