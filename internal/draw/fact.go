package draw

// A Fact is a line of a readout, the console's or a process's: what is
// reported and what was found. A path is shown as it is, where every
// other value is set in capitals.
type Fact struct {
	Label, Value string
	Path         bool
	// The value is the world's text rather than conn's own vocabulary —
	// a command line, something typed — and is kept as it was written.
	// Paths are kept too, but they are kept and elided head-first,
	// which is a path's own business and not this.
	Verbatim bool
}

// FactCol is the column a fact's value stands at, from its label's.
const FactCol = 12
