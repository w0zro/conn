package main

import "github.com/w0zro/conn/internal/tmux"

// The bay as the panel knows it: the terminal in it, as last read or as
// conn last put there; whether that is the readout; and the work it has
// held, which is where the keys go back into.
//
// Work and the bay are not the same thing. While the keys are on the
// panel the readout is in the bay, and the work has been put back in a
// window of its own; the work is what the bay held before the readout
// borrowed it, which is the process the operator was last in. The
// readout and a hold standing in an empty bay are conn's own furniture
// rather than somewhere the operator was working, and going back to one
// would be going back to nothing, so neither is ever work.
type bay struct {
	tty     string // the terminal in the bay
	readout bool   // it is the readout, so the readout is not asked for twice
	work    string // the last terminal the bay held that was work: where esc goes back into
	other   string // the work before that: where the panel key pressed on the panel goes
	// The terminal the output view has the bay showing, which is being
	// looked at and not gone into: furniture for as long as it is this,
	// and not work a reading should take for where the operator was.
	preview string
}

// slotted is a terminal taken into the bay by conn: work, and the work
// it replaces remembered, so there is an other to go back to.
//
// The other is read off the work rather than off the bay. The bay is
// not where the last thing you were in has been since you left it: the
// readout takes the workspace the moment the keys reach the panel, so
// by the time anything is opened or reached the bay is the readout, and
// a rule that refused to remember furniture — rightly — never
// remembered anything at all. The key did nothing for the whole of the
// readout's life.
//
// work is only ever work, being set here and, on a reading, only for a
// bay that can be reached. So one holds what you are in and the other
// what you were in before it, and the readout cannot get between them.
func (b *bay) slotted(tty string) {
	if b.work != "" && b.work != tty {
		b.other = b.work
	}
	b.tty, b.work, b.readout = tty, tty, false
}

// read is the bay as a reading found it. Work found there is what esc
// goes back into, so a conn that came up to a bay it did not fill
// itself still knows where the operator was; furniture found there
// leaves standing whatever the bay held before it.
//
// Whether it is the readout is conn's own to know — it sets it when it
// puts the readout there or takes it away — and the reading corrects
// it; asking tmux on every reading would be a process for something
// conn already knows.
func (b *bay) read(tty string, p tmux.Pane, readout bool) {
	b.tty, b.readout = tty, readout
	if tmux.Reachable(p) && tty != b.preview {
		b.work = tty
	}
}
