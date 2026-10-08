package stationlog

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/w0zro/conn/internal/work"
)

// The log is the panel over time. The panel says what each process is
// doing now and how long it has stood so, and nothing of what happened
// while the operator was in a pane: a server that went down and came
// back, a run that ended with a code and was run again, a contact that
// waited and was answered from another window. The log is that record:
// one line a change, as the panel saw it between one reading and the
// next, under the project it happened in, at the moment it was seen.
//
// A line says what the panel would have said: the row's own word,
// WAITING or EXIT 1 or DOWN, and GONE for a row that left the table.
// Not every change is a line: see news and left for which are.
// Nothing is read from inside a pane for it. The words are the panel's
// vocabulary and no other, so a line in the log is a row of the panel
// read later.
//
// It is a file, under the state directory, a line a record with its
// fields parted by tabs, so that tail, grep and awk read it without
// conn: the station keeps a log the way a station does, and the log is
// the machine's to read as much as conn's.

// An Event is one line of the log.
type Event struct {
	At      time.Time
	Project string // the project's path, as the table has it
	Label   string // what the panel called the row
	PID     int
	Word    string // the row's status as the panel says it, or GONE
	// What the line has to add, read later: how long the state that
	// ended had stood, or what a wait is on, in the contact's words.
	// Blank where there is nothing to add.
	Note string
}

// Gone is the word for a row that left the table: a shell closed, a
// process ended and collected, a container taken down. The panel has
// no word for it, there being no row to say one, so the log has one.
const Gone = "GONE"

// Changes is what moved between two readings, as the log records it:
// a row whose word changed where the change is news, and a row that is
// gone where its going is. label is what the panel calls a row. The
// events stand in the panel's order, the rows that left after them.
//
// A row that is new is not a line. What appears on the panel the
// operator put there — a shell opened, a contact started, a process
// brought up — and a log of one's own doing is read past; the log is
// what happened while the operator was not looking, which is what the
// panel came to say of a row that was already there.
//
// A row is the same row across the two readings by its pid and when it
// started, so a pid come round again is a new row and not the old one
// changing its mind.
func Changes(was, now []work.Project, label func(work.Entry) string, at time.Time) []Event {
	type key struct {
		pid     int
		started time.Time
	}
	type stood struct {
		project string
		work.Entry
	}
	before := map[key]stood{}
	for _, pl := range was {
		for _, e := range pl.Entries {
			before[key{e.PID, e.Started}] = stood{pl.Path, e}
		}
	}
	var events []Event
	var gone []key
	for _, pl := range now {
		for _, e := range pl.Entries {
			k := key{e.PID, e.Started}
			prev, ok := before[k]
			delete(before, k)
			if !ok || !news(prev.Entry, e) {
				continue
			}
			events = append(events, Event{At: at, Project: pl.Path, Label: label(e), PID: e.PID, Word: e.Status, Note: note(prev.Entry, e, at)})
		}
	}
	for k := range before {
		if left(before[k].Entry) {
			gone = append(gone, k)
		}
	}
	// The rows that left, in a settled order: the map's own is none.
	sort.Slice(gone, func(i, j int) bool { return gone[i].pid < gone[j].pid })
	for _, k := range gone {
		prev := before[k]
		events = append(events, Event{At: at, Project: prev.project, Label: label(prev.Entry), PID: prev.PID, Word: Gone, Note: ran(prev.Entry, at)})
	}
	return events
}

// note is what a line adds to its word, for a reader who was not
// there. A wait begun says what it is on, in the contact's own words.
// A state over says how long it stood: a contact's turn over says how
// long the turn took, or how long it waited where the turn ended on a
// wait; a row that ended or went down says how long it ran; a row up
// again says how long it was down. A row whose moment conn never saw
// says nothing of the length.
func note(prev, e work.Entry, at time.Time) string {
	switch {
	case e.Status == work.StatusWaiting:
		return firstLine(e.Asking)
	case e.Kind == work.KindContact && e.Status == work.StatusIdle && prev.Status == work.StatusWaiting:
		return stood("waited", prev.Since, at)
	case e.Kind == work.KindContact && e.Status == work.StatusIdle:
		return stood("took", prev.Since, at)
	case prev.Status == work.StatusDown:
		return stood("down", prev.Since, at)
	case e.Status == work.StatusDown, e.Fault, e.Status == work.StatusClosed:
		return ran(prev, at)
	}
	return ""
}

// ran is how long a row had been running when it ended, counted from
// its start, which the table always knows.
func ran(e work.Entry, at time.Time) string {
	return stood("ran", e.Started, at)
}

// stood is a word and a span since a moment: waited 9 min, ran 3h
// 02m; and nothing where the moment is not known.
func stood(word string, since, at time.Time) string {
	if since.IsZero() || at.Before(since) {
		return ""
	}
	d := at.Sub(since)
	if d < time.Minute {
		return word + " " + strconv.Itoa(int(d.Seconds())) + " s"
	}
	return word + " " + work.Minutes(d)
}

// firstLine is a text's first line, trimmed: a question is read as its
// opening, and a line of the log is one line.
func firstLine(s string) string {
	if i := strings.IndexAny(s, "\n\r"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// news says whether a row's word changing is worth a line: the row has
// come to want something of the operator, or to say something is over.
// A wait begun, a fault, a listener gone, a declared process down, and
// a declared process up again from down, since what was down and is
// not is a thing that happened. A contact's turn over is news too —
// idle after working or waiting is the answer the operator was waiting
// on — but its turn begun is not, the operator having begun it. A
// shell between commands, a server between requests and a build
// between files change their word every reading, and a log of that is
// a log nobody reads.
func news(prev, e work.Entry) bool {
	switch {
	case prev.Status == e.Status:
		return false
	case e.Status == work.StatusWaiting, e.Status == work.StatusDown, e.Status == work.StatusClosed, e.Fault:
		return true
	case prev.Status == work.StatusDown:
		return true
	case e.Kind == work.KindContact && e.Status == work.StatusIdle:
		return true
	}
	return false
}

// left says whether a row's going is worth a line. A shell closed and
// an editor quit are the operator's own doing, ten times a day; a
// contact gone, a run that was there and is not, a service taken down
// are what the operator would ask about.
func left(e work.Entry) bool {
	return e.Kind != work.KindShell && e.Kind != work.KindEditor
}

// logStamp is how a line writes its moment: local time, to the
// second, in the form a reader and a sort both take.
const logStamp = "2006-01-02 15:04:05"

// logCap is the file's size past which the older half is let go, so
// a station up for a season keeps a log and not an archive. A line is
// under a hundred bytes; a megabyte is a long while of changes.
const logCap = 1 << 20

// Append writes events to the log, a line each, making the file and
// its directory where there is none. A field is parted by a tab, so a
// tab or a newline in a label is written as a space.
func Append(path string, events []Event) error {
	if len(events) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	var b strings.Builder
	for _, e := range events {
		b.WriteString(e.Line())
		b.WriteByte('\n')
	}
	_, err = f.WriteString(b.String())
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return trimLog(path)
}

// Line is an event as the file holds it.
func (e Event) Line() string {
	flat := func(s string) string {
		return strings.Map(func(r rune) rune {
			if r == '\t' || r == '\n' || r == '\r' {
				return ' '
			}
			return r
		}, s)
	}
	fields := []string{e.At.Format(logStamp), flat(e.Project), flat(e.Label), strconv.Itoa(e.PID), flat(e.Word)}
	if e.Note != "" {
		fields = append(fields, flat(e.Note))
	}
	return strings.Join(fields, "\t")
}

// parseLogLine reads a line of the file back, and says whether it was
// one: a line of another shape is not the log's, and is passed over.
// The note is the sixth field, and a line without one has five.
func parseLogLine(line string) (Event, bool) {
	f := strings.Split(line, "\t")
	if len(f) < 5 || len(f) > 6 {
		return Event{}, false
	}
	at, err := time.ParseInLocation(logStamp, f[0], time.Local)
	if err != nil {
		return Event{}, false
	}
	pid, err := strconv.Atoi(f[3])
	if err != nil {
		return Event{}, false
	}
	e := Event{At: at, Project: f[1], Label: f[2], PID: pid, Word: f[4]}
	if len(f) == 6 {
		e.Note = f[5]
	}
	return e, true
}

// Read is the log's last lines, oldest first, up to max of them;
// every line where max is 0. No file is an empty log and no error.
func Read(path string, max int) ([]Event, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var events []Event
	s := bufio.NewScanner(f)
	for s.Scan() {
		if e, ok := parseLogLine(s.Text()); ok {
			events = append(events, e)
		}
	}
	err = s.Err()
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if max > 0 && len(events) > max {
		events = events[len(events)-max:]
	}
	return events, err
}

// trimLog lets the older half of the file go once it is past the cap,
// at a line, so what is kept is whole lines.
func trimLog(path string) error {
	info, err := os.Stat(path)
	if err != nil || info.Size() <= logCap {
		return err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	cut := len(b) / 2
	if i := strings.IndexByte(string(b[cut:]), '\n'); i >= 0 {
		cut += i + 1
	}
	return os.WriteFile(path, b[cut:], 0o600)
}
