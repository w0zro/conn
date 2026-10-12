package main

import (
	"fmt"
	"strings"
)

// a hands a row to an agent: a new conversation in the project, given
// what the row is, how it is doing, and what it last wrote, so that a
// check that failed goes to an agent with its failure, and a server
// that is misbehaving with what it said. A conversation is an agent
// already, and has nothing to be handed.

// handLines is how many of a process's last lines go with it, and
// handBytes how much in all: a first prompt goes on a shell line.
const (
	handLines = 80
	handBytes = 16 << 10
)

// handPrompt is what the agent is told: the process by the name it
// answers to, what it runs and where, how it is doing, its last lines,
// and what to do about it.
func handPrompt(name, command, dir, state string, lines []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "In this project, %s", name)
	if command != "" && command != name {
		fmt.Fprintf(&b, " runs %s", command)
	}
	if dir != "" {
		fmt.Fprintf(&b, " at %s", dir)
	}
	switch {
	case over(state):
		fmt.Fprintf(&b, ", and it %s", state)
	case state != "":
		fmt.Fprintf(&b, ", and it is %s", state)
	}
	b.WriteString(".")
	if lines = lastLines(lines, handLines, handBytes); len(lines) > 0 {
		b.WriteString(" What it last wrote:\n\n")
		for _, l := range lines {
			b.WriteString(l)
			b.WriteString("\n")
		}
	}
	if strings.HasPrefix(state, "failed ") {
		b.WriteString("\nWork out what is wrong and fix it.")
	} else {
		b.WriteString("\nLook into it.")
	}
	fmt.Fprintf(&b, " conn l %s reads what it has said", name)
	if command != "" && over(state) {
		fmt.Fprintf(&b, ", and conn u %s -w -- %s runs it again", name, command)
	}
	b.WriteString(".")
	return b.String()
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
