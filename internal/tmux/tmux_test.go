package tmux

import (
	"reflect"
	"testing"
)

// list-panes, as tmux prints it for the format asked.
func TestPanesAreParsed(t *testing.T) {
	// The last three fields are tmux's word for where the keys are in
	// the window — the panel has them here, and nothing else does —
	// where each pane stands in it, and conn's own mark for the
	// settings.
	out := "%0 /dev/ttys004 48 40         1 0 \n" +
		"%1 /dev/ttys007 138 40 1        0 1 \n" +
		"%5 /dev/ttys008 138 40  1       0 2 \n" +
		// A readout carries the hold's own mark as well as its own: it is
		// furniture like a hold, and everything that acts on holds acts on
		// it. Only the panel has to tell the two apart. The manual and the
		// settings are furniture the same way; this pane wears every mark
		// at once, so the parse is read for all of them together.
		"%7 /dev/ttys009 138 40 1  1   1   0 3 1\n" +
		// A pane conn opened for a container says which: it is the
		// terminal that container has not got, and its row is reached
		// through it.
		"%9 /dev/ttys010 138 40    9f1c2d3e4a5b     0 4 \n" +
		// A pane holding a shell inside a container says which container,
		// on the other mark: it is work of the operator's own, not the
		// service being read.
		"%11 /dev/ttys011 138 40     9f1c2d3e4a5b    0 5 \n" +
		// A pane conn opened for a declared process says which, and
		// how the process ended once it has.
		"%13 /dev/ttys012 138 40       web@%2FUsers%2Fw0zro%2Fapp 1 0 6 \n\n"
	want := map[string]Pane{
		"ttys004": {ID: "%0", TTY: "ttys004", Width: 48, Height: 40, Active: true, index: 0},
		"ttys007": {ID: "%1", TTY: "ttys007", Width: 138, Height: 40, Hold: true, index: 1},
		"ttys008": {ID: "%5", TTY: "ttys008", Width: 138, Height: 40, Dead: true, index: 2},
		"ttys009": {ID: "%7", TTY: "ttys009", Width: 138, Height: 40, Hold: true, Readout: true, Help: true, Settings: true, index: 3},
		"ttys010": {ID: "%9", TTY: "ttys010", Width: 138, Height: 40, Container: "9f1c2d3e4a5b", index: 4},
		"ttys011": {ID: "%11", TTY: "ttys011", Width: 138, Height: 40, ShellIn: "9f1c2d3e4a5b", index: 5},
		"ttys012": {ID: "%13", TTY: "ttys012", Width: 138, Height: 40, Declared: "web@%2FUsers%2Fw0zro%2Fapp", Exit: "1", index: 6},
	}
	if got := parsePanes(out); !reflect.DeepEqual(got, want) {
		t.Errorf("panes: %v", got)
	}
	if got := parsePanes(""); len(got) != 0 {
		t.Errorf("no panes parsed as %v", got)
	}
}

// The client's environment carries no tmux of its own.
func TestTheClientEnvironmentDropsTmux(t *testing.T) {
	got := WithoutTmux([]string{"HOME=/h", "TMUX=/tmp/x,1,0", "TERM=xterm", "TMUX_PANE=%3"})
	if !reflect.DeepEqual(got, []string{"HOME=/h", "TERM=xterm"}) {
		t.Errorf("environment: %q", got)
	}
}

// Every format conn hands tmux is printable ASCII. tmux sanitizes what
// it prints to something that is not a terminal, and what counts as
// printable is the locale's: in the C locale it turns a tab in the
// answer into an underscore, and conn read no panes at all. A space
// tells the fields apart in every locale there is.
func TestNoFormatAsksTmuxForAControlCharacter(t *testing.T) {
	for _, f := range []string{paneFormat, openFormat, windowFormat} {
		for i, r := range f {
			if r < 0x20 || r == 0x7f {
				t.Errorf("a format asks tmux for %q at %d: %q", r, i, f)
			}
		}
	}
}
