package work

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
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
}

// LogGone is the word for a row that left the table: a shell closed, a
// process ended and collected, a container taken down. The panel has
// no word for it, there being no row to say one, so the log has one.
const LogGone = "GONE"

// Changes is what moved between two readings, as the log records it:
// a row that is new, a row that is gone, and a row whose word changed
// where the change is news. label is what the panel calls a row. The
// events stand in the panel's order, the rows that left after them.
//
// A row is the same row across the two readings by its pid and when it
// started, so a pid come round again is a new row and not the old one
// changing its mind.
func Changes(was, now []Project, label func(Entry) string, at time.Time) []Event {
	type key struct {
		pid     int
		started time.Time
	}
	type stood struct {
		project string
		Entry
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
			if ok && !news(prev.Entry, e) {
				continue
			}
			events = append(events, Event{At: at, Project: pl.Path, Label: label(e), PID: e.PID, Word: e.Status})
		}
	}
	for k := range before {
		gone = append(gone, k)
	}
	// The rows that left, in a settled order: the map's own is none.
	sort.Slice(gone, func(i, j int) bool { return gone[i].pid < gone[j].pid })
	for _, k := range gone {
		prev := before[k]
		events = append(events, Event{At: at, Project: prev.project, Label: label(prev.Entry), PID: prev.PID, Word: LogGone})
	}
	return events
}

// quiet is a word that comes and goes with the work itself: a shell
// between commands, a server between requests, a build between files.
func quiet(status string) bool {
	switch status {
	case StatusWorking, StatusActive, StatusIdle:
		return true
	}
	return false
}

// news says whether a row's word changing is worth a line. A change
// between two quiet words is not: a shell runs a command every minute
// and a server answers a request every second, and a log of that is a
// log nobody reads. A contact is the exception, its quiet being its
// turn: working is a turn begun and idle is a turn over, which is the
// thing the operator was waiting on. Anything else that changes — a
// wait begun or answered, a fault, a declared process down or up, a
// listener gone — is the panel's word changing, and is news.
func news(prev, e Entry) bool {
	if prev.Status == e.Status {
		return false
	}
	if quiet(prev.Status) && quiet(e.Status) && e.Kind != KindContact {
		return false
	}
	return true
}

// Faulty says whether a word is a fault's: a thing to look at, which
// the panel stamps. The panel knows a fault by its row; the log knows
// it by the word alone, which is all a line carries.
func Faulty(word string) bool {
	switch {
	case word == StatusStopped, word == StatusEnded, word == StatusClosed:
		return true
	case strings.HasPrefix(word, exitWord):
		return true
	}
	return false
}

// logStamp is how a line writes its moment: local time, to the
// second, in the form a reader and a sort both take.
const logStamp = "2006-01-02 15:04:05"

// logCap is the file's size past which the older half is let go, so
// a station up for a season keeps a log and not an archive. A line is
// under a hundred bytes; a megabyte is a long while of changes.
const logCap = 1 << 20

// AppendLog writes events to the log, a line each, making the file and
// its directory where there is none. A field is parted by a tab, so a
// tab or a newline in a label is written as a space.
func AppendLog(path string, events []Event) error {
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
	return strings.Join([]string{e.At.Format(logStamp), flat(e.Project), flat(e.Label), strconv.Itoa(e.PID), flat(e.Word)}, "\t")
}

// ParseLogLine reads a line of the file back, and says whether it was
// one: a line of another shape is not the log's, and is passed over.
func ParseLogLine(line string) (Event, bool) {
	f := strings.Split(line, "\t")
	if len(f) != 5 {
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
	return Event{At: at, Project: f[1], Label: f[2], PID: pid, Word: f[4]}, true
}

// ReadLog is the log's last lines, oldest first, up to max of them;
// every line where max is 0. No file is an empty log and no error.
func ReadLog(path string, max int) ([]Event, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var events []Event
	s := bufio.NewScanner(f)
	for s.Scan() {
		if e, ok := ParseLogLine(s.Text()); ok {
			events = append(events, e)
		}
	}
	if max > 0 && len(events) > max {
		events = events[len(events)-max:]
	}
	return events, s.Err()
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
