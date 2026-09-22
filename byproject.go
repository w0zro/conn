package main

import (
	"cmp"
	"slices"
	"strings"
)

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

// The order a project's rows stand in: what can answer you first, then
// what you type at, then the work. A contact is the row you deal with
// and the one the eye should land on; a shell is the way in to a pane;
// everything else is what is going on in there. Within a rank the rows
// keep the order they were read in, which is the order they started in,
// so a row stays where it is for as long as it lives and what is new
// goes on the end of its own rank.
//
// The panel is a list of rows here and not a tree of them, and draws no
// indent. Ordering by kind and keeping the tree are not both possible:
// a contact almost always runs under the shell of its pane, so a tree
// that held its shape would leave every contact a row down under a
// shell rather than at the top. What runs what is on z, which draws the
// tree whole, and on the page beside the panel, which says a row's
// Under and its Runs.
//
// A row keeps the depth it was read at all the same, undrawn: it is how
// headOf finds the process a pane was opened on, which is what the
// bay's bar goes on and where the cursor belongs once the pane is
// reached.
func byKind(rows []entry) []entry {
	slices.SortStableFunc(rows, func(a, b entry) int {
		return cmp.Compare(rank(panelKind(a)), rank(panelKind(b)))
	})
	return rows
}

// panelKind is the kind a row wears on the panel, which is not always
// the kind of its program. A shell whose rows folded into it stands for
// what it runs — its row says vim notes.md, or go test ./... — so it
// wears what it runs: the editor's mark over the one, work's over the
// other. A shell at its prompt runs nothing and is the prompt. A shell
// that found only another shell under it is a shell still.
//
// The tree on z keeps the program's own kind in its kind column, where
// the shell and what it runs are two rows and neither stands for the
// other.
func panelKind(e entry) string {
	if e.kind == kindShell && e.under != "" && e.underKind != "" {
		return e.underKind
	}
	return e.kind
}

// rank is where a kind stands in that order. A kind conn does not tell
// apart is a run, and ranks with the work.
func rank(kind string) int {
	switch kind {
	case kindContact:
		return 0
	case kindShell:
		return 1
	}
	return 2
}

// The states a row's word stands it in, worst first. A block says the
// first of these it holds and nothing where it holds none: a wait is
// answered, a fault is looked at, a down row is brought up, and what is
// working or over or quiet asks nothing. A word on every block would be
// a word read past on every block.
//
// A port is not here. Serving is not a word a row says but a thing it
// has, and a row has it or not whatever its word is.
const (
	standWaiting = iota
	standFault
	standDown
	standOver
	standWorking
	standRests
)

// stateOf is how a row stands, for the mark it takes and for what its
// block says of itself. What is alive with nothing to report — a shell
// at its prompt, an editor, a process up and not doing anything, a
// contact at rest — rests. A stopped row is a fault and is alive: fg
// brings it back. One that ended with a code is a fault before it is
// over, since the code is the thing to look at.
func stateOf(status string, fault bool) int {
	switch {
	case status == statusWaiting:
		return standWaiting
	case fault:
		return standFault
	case status == statusDown:
		return standDown
	case over(status):
		return standOver
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

// portsWord is how a row of the tree says its ports after its command:
// web · :8438, or every port it has, lowest first. Nothing for a row
// with none. The panel stands them at its right instead, as a column,
// and asks for portsColumn.
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
