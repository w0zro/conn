package main

import (
	"fmt"
	"regexp"
	"strings"
)

// a hands a row to an agent: a new conversation in the project, given
// what the row is, how it is doing, and what it last wrote, so that a
// check that failed goes to an agent with its failure, and a server
// that is misbehaving with what it said. A conversation is an agent
// already, and has nothing to be handed.

// handLines is how many of a process's last lines go with it, and
// handBytes how much in all: a first prompt is a brief, not a log.
const (
	handLines = 80
	handBytes = 16 << 10
)

// A handing is a process as it is handed: what it is called, what conn
// l and conn x take for it, whether that is a name of its own, what it
// runs and where, how it is doing, and what it last wrote.
type handing struct {
	id                  string // its pane
	name, addr          string
	named               bool
	command, dir, state string
	lines               []string
}

// handPrompt is what the agent is told of one process: what it is, and
// what to do about it.
func handPrompt(h handing) string {
	var b strings.Builder
	h.describe(&b, handBytes)
	if strings.HasPrefix(h.state, "failed ") {
		b.WriteString("\nWork out what is wrong and fix it.")
	} else {
		b.WriteString("\nLook into it.")
	}
	h.tools(&b)
	return b.String()
}

// handAllPrompt is what the agent is told of several runs that failed:
// each, in the share of the size that falls to it, and to fix them.
func handAllPrompt(hs []handing) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d runs in this project failed.\n\n", len(hs))
	for _, h := range hs {
		h.describe(&b, handBytes/len(hs))
		h.tools(&b)
		b.WriteString("\n\n")
	}
	b.WriteString("Work out what is wrong with each and fix it.")
	return b.String()
}

// describe writes what a process is, how it is doing, and its last
// lines within a size.
func (h handing) describe(b *strings.Builder, size int) {
	fmt.Fprintf(b, "In this project, %s", h.name)
	if h.command != "" && h.command != h.name {
		fmt.Fprintf(b, " runs %s", h.command)
	}
	if h.dir != "" {
		fmt.Fprintf(b, " at %s", h.dir)
	}
	switch {
	case over(h.state):
		fmt.Fprintf(b, ", and it %s", h.state)
	case h.state != "":
		fmt.Fprintf(b, ", and it is %s", h.state)
	}
	b.WriteString(".")
	if lines := lastLines(h.lines, handLines, size); len(lines) > 0 {
		b.WriteString(" What it last wrote:\n\n")
		for _, l := range lines {
			b.WriteString(l)
			b.WriteString("\n")
		}
	}
}

// tools writes the conn lines that read the process and run it again.
func (h handing) tools(b *strings.Builder) {
	fmt.Fprintf(b, " conn l %s reads what it has said", or(h.addr, h.name))
	switch {
	case h.command == "" || !over(h.state):
	case h.named:
		fmt.Fprintf(b, ", and conn u %s -w -- %s runs it again", h.name, h.command)
	default:
		fmt.Fprintf(b, ", and conn u -w -- %s runs it again", h.command)
	}
	b.WriteString(".")
}

// lastLines is the last n lines, within a size, trailing blanks and the
// line conn's own pane writes at the end dropped.
func lastLines(lines []string, n, size int) []string {
	for len(lines) > 0 && (strings.TrimSpace(lines[len(lines)-1]) == "" || strings.HasPrefix(lines[len(lines)-1], "[") && strings.HasSuffix(lines[len(lines)-1], "]")) {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	total := 0
	for i := len(lines) - 1; i >= 0; i-- {
		total += len(lines[i]) + 1
		if total > size {
			return lines[i+1:]
		}
	}
	return lines
}

// reasonOf is the last thing a run said that is not only a verdict, as
// go test ends with FAIL and a shell with its status: why it failed.
func reasonOf(lines []string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		s := strings.TrimSpace(lines[i])
		switch {
		case s == "", strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]"):
		case verdict.MatchString(s):
		default:
			return s
		}
	}
	return ""
}

// verdict is a line that says only that a run failed, or how it ended.
var verdict = regexp.MustCompile(`^(--- )?FAIL\b.*$|^(exit status \d+|exited.*|make: \*\*\*.*)$`)
