package main

import "strings"

// The panel is filed by project: a block per project under an eyebrow
// of its own, holding the rows it has, folded; and at the end of the
// rule what the project wants of you, which is the whole of the triage
// in one line a project instead of one a process.
//
// It was filed by state before — waiting, working, serving, idle, not
// running, each its own block — so that the one row which had stopped
// for the operator came to the top whatever project it was in. That
// spent the one axis the eye searches on, where a thing is, saying a
// second time what the row already said four other ways: a row at work
// turns a spinner, a row waiting blinks its stamp, a row not running is
// struck through, a row serving says its port. Every state carries a
// mark that finds the eye wherever the row sits, and a blink finds it
// better than a position does, since a position has to be known before
// it can be looked at.
//
// And it was paid for twice. The groups had to stand empty to keep the
// panel one shape as rows came and went, and every row had to carry the
// project it was read in, since one zsh says nothing beside another.
// Filed by project the position is the project — which nothing else on
// the row can say — the right of the row is free for the row's own
// word, and the blocks are the projects, which come and go when work
// does rather than every reading.

// The states a row stands in, worst first. A block says the first of
// these it holds and nothing where it holds none: a wait is answered, a
// fault is looked at, a down row is brought up, and what is working or
// serving or quiet asks nothing. A word on every block would be a word
// read past on every block.
const (
	standWaiting = iota
	standFault
	standDown
	standWorking
	standServing
	standOver
	standRests
)

// stateOf is how a row stands, for the mark it takes and for what its
// block says of itself. What is alive with nothing to report — a shell
// at its prompt, an editor, a process up and not doing anything, a
// contact at rest — rests. A stopped row is a fault and is alive: fg
// brings it back. One that ended, cleanly or with a code, is over,
// and a code is a fault besides.
func stateOf(status string, fault bool) int {
	switch {
	case status == statusWaiting:
		return standWaiting
	case fault && (status == statusEnded || strings.HasPrefix(status, exitWord)):
		return standFault
	case status == statusDown:
		return standDown
	case status == statusEnded || strings.HasPrefix(status, exitWord):
		return standOver
	case fault:
		return standFault
	case status == statusWorking:
		return standWorking
	}
	return standRests
}

// over says whether a row is not running: declared and never came up,
// or ended, cleanly or with a code. Its command is struck through.
func over(status string) bool {
	switch status {
	case statusDown, statusEnded:
		return true
	}
	return strings.HasPrefix(status, exitWord)
}

// serving says whether a row is a thing to reach: it is alive and has
// a port, one it listens on or one its container publishes. A server
// is not idle, it is at its work, and a port is what tells a
// node that serves from a node that builds, both of which the process
// table calls the same. A contact is filed by what it asks of you and
// never by what it has open, and what is not running serves nothing.
func serving(e entry) bool {
	if over(e.status) {
		return false
	}
	return e.kind != kindContact && len(e.ports) > 0
}

// portsWord is how a row says its ports after its command: web · :8438,
// or every port it has, lowest first. Nothing for a row with none.
func portsWord(ports []string) string {
	if len(ports) == 0 {
		return ""
	}
	return " · " + portsColumn(ports)
}

// portsColumn is the ports written together: :8438, or every port
// lowest first. Nothing for none.
func portsColumn(ports []string) string {
	if len(ports) == 0 {
		return ""
	}
	return ":" + strings.Join(ports, " :")
}
