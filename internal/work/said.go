package work

import (
	"regexp"
	"strings"
)

// What a process says when something has gone wrong, in its stack's
// own words. A row's word on the panel comes from the machine: an exit
// code, a stopped process, a listener gone. A process that is still
// running and has printed a panic has told the terminal and nobody
// else, and the panel went on saying ACTIVE. The stacks say it the
// same way every time — Go prints panic:, go test prints FAIL, Python
// prints a Traceback, node an uncaught Error, and every server says a
// port is already in use in the one word — so conn knows the words the
// way it knows which program is a contact, by name, in a table here,
// and a held pane saying one is a line in the log with the line it
// said. Nothing is configured: the words are the stack's, and a stack
// conn does not know says nothing until it is added here.

// A saying is a stack's word for something gone wrong, and how a line
// says it.
type saying struct {
	word  string         // as the log writes it
	lines *regexp.Regexp // a line that says it
}

var sayings = []saying{
	// Go: a runtime panic, a fatal runtime error, and go test's verdict
	// on a test and on a package.
	{"PANIC", regexp.MustCompile(`^panic: `)},
	{"FATAL", regexp.MustCompile(`^fatal error: `)},
	{"FAIL", regexp.MustCompile(`^(--- FAIL: |FAIL\t|FAIL$)`)},
	// Rust: a thread's panic.
	{"PANIC", regexp.MustCompile(`^thread '[^']*' panicked`)},
	// Python: an exception that reached the top.
	{"TRACEBACK", regexp.MustCompile(`^Traceback \(most recent call last\):`)},
	// A server that could not take its port, in the word every stack
	// uses for it; before the uncaught error, since node says both on
	// the one line and the port is the more to say.
	{"EADDRINUSE", regexp.MustCompile(`EADDRINUSE|address already in use`)},
	// node: an uncaught error, which prints its class and message on a
	// line of its own; git's and rustc's error: in the lower case are
	// ordinary failures of a command and not this.
	{"ERROR", regexp.MustCompile(`^([A-Z][A-Za-z]*)?Error: `)},
}

// SaidWords is every word a pane can be logged as saying, for the log
// to stamp them as it stamps a fault.
var SaidWords = func() map[string]bool {
	m := map[string]bool{}
	for _, s := range sayings {
		m[s.word] = true
	}
	return m
}()

// SaidWord is the word a line says, where it says one.
func SaidWord(line string) (string, bool) {
	for _, s := range sayings {
		if s.lines.MatchString(line) {
			return s.word, true
		}
	}
	return "", false
}

// NewLines is what a pane has said since its tail was last read: the
// lines of the tail now that follow the tail then. The tails are the
// last so many lines of a pane, so the earlier one is found in the
// later by its last few lines, and what follows them is new; a pane
// that has said more than a tail's worth since is new from the top.
// The last line of a tail is the one still being written, and is not
// counted as said until a line stands after it.
func NewLines(was, now []string) []string {
	if len(now) == 0 {
		return nil
	}
	// The line being written, and the blank rows of the screen under
	// it, are not said yet.
	end := len(now)
	for end > 0 && strings.TrimSpace(now[end-1]) == "" {
		end--
	}
	if end > 0 {
		end--
	}
	now = now[:end]
	if len(was) == 0 {
		return nil
	}
	wasEnd := len(was)
	for wasEnd > 0 && strings.TrimSpace(was[wasEnd-1]) == "" {
		wasEnd--
	}
	if wasEnd > 0 {
		wasEnd--
	}
	was = was[:wasEnd]
	anchor := was
	if len(anchor) > 8 {
		anchor = anchor[len(anchor)-8:]
	}
	if len(anchor) == 0 {
		return now
	}
	for i := len(now) - len(anchor); i >= 0; i-- {
		if equalLines(now[i:i+len(anchor)], anchor) {
			return now[i+len(anchor):]
		}
	}
	return now
}

func equalLines(a, b []string) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
