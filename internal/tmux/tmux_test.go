package tmux

import (
	"reflect"
	"testing"
)

// list-panes, as tmux prints it for the format asked: the fields every
// pane is read with, then the user options asked for, in the order
// asked, blank where a pane has none.
func TestPanesAreParsed(t *testing.T) {
	opts := []string{"@a", "@b"}
	// The fields are the id, the terminal, the size, whether the
	// process has ended, whether the keys are in it, and where it stands
	// in its window.
	out := "%0 /dev/ttys004 48 40  1 0  \n" +
		"%5 /dev/ttys008 138 40 1  1 1 \n" +
		"%9 /dev/ttys010 138 40   2 9f1c2d3e4a5b x\n\n"
	want := map[string]Pane{
		"ttys004": {ID: "%0", TTY: "ttys004", Width: 48, Height: 40, Active: true, index: 0, Opts: map[string]string{"@a": "", "@b": ""}},
		"ttys008": {ID: "%5", TTY: "ttys008", Width: 138, Height: 40, Dead: true, index: 1, Opts: map[string]string{"@a": "1", "@b": ""}},
		"ttys010": {ID: "%9", TTY: "ttys010", Width: 138, Height: 40, index: 2, Opts: map[string]string{"@a": "9f1c2d3e4a5b", "@b": "x"}},
	}
	if got := parsePanes(out, opts); !reflect.DeepEqual(got, want) {
		t.Errorf("panes: %v", got)
	}
	// Asked for nothing past the fields, a pane carries no options.
	if got := parsePanes("%0 /dev/ttys004 48 40  1 0", nil); got["ttys004"].Opts != nil || got["ttys004"].ID != "%0" {
		t.Errorf("a pane read with no options: %+v", got)
	}
	if got := parsePanes("", opts); len(got) != 0 {
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
	for _, f := range []string{paneFormat(nil), paneFormat([]string{"@a", "@b"}), openFormat, windowFormat} {
		for i, r := range f {
			if r < 0x20 || r == 0x7f {
				t.Errorf("a format asks tmux for %q at %d: %q", r, i, f)
			}
		}
	}
}
