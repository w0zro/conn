package main

import (
	"encoding/json"
	"os"
	"time"
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

// cursorPath is where a server's panel publishes its cursor.
func cursorPath(home string) string { return socketPath(home) + ".cursor" }

// A subject is what the page is about: a process, by its pid; a
// project, by its path; or a suspended session, by its id. The
// processes view's cursor is always on a process; the list's cursor
// stands on projects as often as not, and the sessions list's on
// sessions, and each is as much a thing to read about as a row.
type subject struct {
	pid     int
	path    string // a project, where pid is 0
	session string // a suspended session, where both are empty
}

// none says whether there is a subject at all.
func (s subject) none() bool { return s.pid == 0 && s.path == "" && s.session == "" }

// A cursorNote is the note as it is written: the subject, and the
// reading it stands in.
type cursorNote struct {
	PID     int
	Path    string
	Session string
	Reading *reading
}

// A reading is the panel's reading of the machine, as it publishes it
// for the page: the rows as the panel shows them, the table's record
// behind each row, the server's panes and whether there is a server at
// all, the containers docker last said, which a container's row names
// by id, and the sessions the sessions list has in hand, which its
// rows name by id.
type reading struct {
	projects   []project
	records    map[int]record
	panes      map[string]pane
	inside     bool
	containers []container
	brews      []brewService // what brew said of its services; see brew.go
	sessions   []session
}

// sessionOf is the suspended session of an id among those the panel
// has in hand, where it is one.
func (r reading) sessionOf(id string) *session {
	for i := range r.sessions {
		if r.sessions[i].ID == id {
			return &r.sessions[i]
		}
	}
	return nil
}

// A record is the part of the table's own record behind a row that the
// page reads and the row does not carry: the kernel's state for it,
// whether its group holds the terminal, and the processor time it has
// spent altogether.
type record struct {
	pid        int
	state      byte
	foreground bool
	cpu        time.Duration
}

// recordOf is a process's record.
func recordOf(p process) record {
	return record{pid: p.pid, state: p.state, foreground: p.foreground, cpu: p.cpu}
}

// containerOf is the container a row stands for among those docker
// said, where it is one.
// brewOf is the brew service a row stands for among those brew
// reported, where it is one.
func (r reading) brewOf(e entry) *brewService {
	if e.brew == "" {
		return nil
	}
	return brewServiceNamed(r.brews, e.brew)
}

func (r reading) containerOf(e entry) *container {
	if e.container == "" {
		return nil
	}
	for i := range r.containers {
		if r.containers[i].id == e.container {
			return &r.containers[i]
		}
	}
	return nil
}

// tellCursor publishes the subject under the panel's cursor, with the
// reading it stands in. Nothing waits on this and nothing is broken by
// its failing: a readout that cannot read the cursor holds the subject
// it has, which is the same thing it does between one move and the
// next.
func tellCursor(path string, at subject, r *reading) {
	b, err := json.Marshal(cursorNote{PID: at.pid, Path: at.path, Session: at.session, Reading: r})
	if err != nil {
		return
	}
	_ = os.WriteFile(path, b, 0o600)
}

// readCursor is the note as the file holds it, or nothing where there
// is no file to read. The page compares one reading of it with the
// last to know whether anything changed, which is cheaper than working
// out what.
func readCursor(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

// parseCursor reads a note: the subject the panel's cursor is on, or
// none where there is none to read — no file yet, a panel that never
// published, a server that is not this one — and the reading the panel
// published beside it, where it did.
func parseCursor(note string) (subject, *reading) {
	var n cursorNote
	if json.Unmarshal([]byte(note), &n) != nil {
		return subject{}, nil
	}
	return subject{pid: n.PID, path: n.Path, session: n.Session}, n.Reading
}

// askCursor is the note read and parsed in one go.
func askCursor(path string) (subject, *reading) {
	return parseCursor(readCursor(path))
}

// The shapes conn's own types cross between its programs in: the same
// fields, exported so that encoding/json carries them, each kept
// beside the type it mirrors rather than a second definition to keep
// in step. containerWire is in docker.go.

type readingWire struct {
	Projects   []project
	Records    []record
	Panes      []pane
	Inside     bool
	Containers []container
	Brews      []brewService
	Sessions   []session
}

func (r reading) MarshalJSON() ([]byte, error) {
	w := readingWire{Projects: r.projects, Inside: r.inside, Containers: r.containers, Brews: r.brews, Sessions: r.sessions}
	for _, rec := range r.records {
		w.Records = append(w.Records, rec)
	}
	for _, p := range r.panes {
		w.Panes = append(w.Panes, p)
	}
	return json.Marshal(w)
}

func (r *reading) UnmarshalJSON(b []byte) error {
	var w readingWire
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	*r = reading{projects: w.Projects, inside: w.Inside, containers: w.Containers, brews: w.Brews, sessions: w.Sessions,
		records: map[int]record{}, panes: map[string]pane{}}
	for _, rec := range w.Records {
		r.records[rec.pid] = rec
	}
	for _, p := range w.Panes {
		r.panes[p.tty] = p
	}
	return nil
}

type projectWire struct {
	Path    string
	Entries []entry
	Note    string
}

func (p project) MarshalJSON() ([]byte, error) {
	return json.Marshal(projectWire{Path: p.path, Entries: p.entries, Note: p.note})
}

func (p *project) UnmarshalJSON(b []byte) error {
	var w projectWire
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	*p = project{path: w.Path, entries: w.Entries, note: w.Note}
	return nil
}

type entryWire struct {
	PID                       int
	Kind, Command, Typed, TTY string
	Started                   time.Time
	Status                    string
	Fault                     bool
	Depth                     int
	Since                     time.Time
	Cwd, Asking, Doing        string
	Container, Declared       string
	Sockets                   []socket
	Ports                     []string
	Brew                      string
	Shared                    int
}

func (e entry) MarshalJSON() ([]byte, error) {
	return json.Marshal(entryWire{
		PID: e.pid, Kind: e.kind, Command: e.command, Typed: e.typed, TTY: e.tty,
		Started: e.started, Status: e.status, Fault: e.fault, Depth: e.depth,
		Since: e.since, Cwd: e.cwd, Asking: e.asking, Doing: e.doing, Container: e.container,
		Declared: e.declared, Sockets: e.sockets, Ports: e.ports, Brew: e.brew, Shared: e.shared,
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
		declared: w.Declared, sockets: w.Sockets, ports: w.Ports, brew: w.Brew, shared: w.Shared,
	}
	return nil
}

type socketWire struct{ Proto, Addr, State string }

func (s socket) MarshalJSON() ([]byte, error) {
	return json.Marshal(socketWire{Proto: s.proto, Addr: s.addr, State: s.state})
}

func (s *socket) UnmarshalJSON(b []byte) error {
	var w socketWire
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	*s = socket{proto: w.Proto, addr: w.Addr, state: w.State}
	return nil
}

type brewWire struct {
	Name    string
	Running bool
	PID     int
	Exit    string
	Status  string
	Command string
	Log     string
}

func (s brewService) MarshalJSON() ([]byte, error) {
	return json.Marshal(brewWire{Name: s.name, Running: s.running, PID: s.pid, Exit: s.exit, Status: s.status, Command: s.command, Log: s.log})
}

func (s *brewService) UnmarshalJSON(b []byte) error {
	var w brewWire
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	*s = brewService{name: w.Name, running: w.Running, pid: w.PID, exit: w.Exit, status: w.Status, command: w.Command, log: w.Log}
	return nil
}

type recordWire struct {
	PID        int
	State      byte
	Foreground bool
	CPU        time.Duration
}

func (r record) MarshalJSON() ([]byte, error) {
	return json.Marshal(recordWire{PID: r.pid, State: r.state, Foreground: r.foreground, CPU: r.cpu})
}

func (r *record) UnmarshalJSON(b []byte) error {
	var w recordWire
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	*r = record{pid: w.PID, state: w.State, foreground: w.Foreground, cpu: w.CPU}
	return nil
}

type paneWire struct {
	ID, TTY                   string
	Width, Height             int
	Hold, Readout, Dead, Help bool
	Active                    bool
	Container, ShellIn        string
	Declared, Exit            string
}

func (p pane) MarshalJSON() ([]byte, error) {
	return json.Marshal(paneWire{ID: p.id, TTY: p.tty, Width: p.width, Height: p.height,
		Hold: p.hold, Readout: p.readout, Dead: p.dead, Help: p.help, Active: p.active,
		Container: p.container, ShellIn: p.shellIn, Declared: p.declared, Exit: p.exit})
}

func (p *pane) UnmarshalJSON(b []byte) error {
	var w paneWire
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	*p = pane{id: w.ID, tty: w.TTY, width: w.Width, height: w.Height,
		hold: w.Hold, readout: w.Readout, dead: w.Dead, help: w.Help, active: w.Active,
		container: w.Container, shellIn: w.ShellIn, declared: w.Declared, exit: w.Exit}
	return nil
}
