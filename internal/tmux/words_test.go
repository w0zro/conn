package tmux

import (
	"reflect"
	"testing"
)

// The pure parts of the package: what conn says to tmux and reads back
// from it, held by tables, where the server tests hold the rest.

func TestWindowsAreReadNameThenPath(t *testing.T) {
	out := "home /Users/w0zro\nshell /Users/w0zro/projects/SkellyLabs, Inc\nbare\n\n"
	want := []Window{{"home", "/Users/w0zro"}, {"shell", "/Users/w0zro/projects/SkellyLabs, Inc"}}
	if got := ParseWindows(out); !reflect.DeepEqual(got, want) {
		t.Errorf("ParseWindows read %+v, want %+v: a path with a space arrives whole, a line with no path is passed over", got, want)
	}
}

func TestAnOpenedPaneIsReadBack(t *testing.T) {
	got := parseOpened("%4 8123 /dev/ttys005\n")
	want := Shell{Pane: Pane{ID: "%4", TTY: "ttys005"}, PID: 8123}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseOpened read %+v, want %+v", got, want)
	}
	// A short answer is not a crash: the fields it lacks are blank.
	if got := parseOpened("%4"); got.Pane.ID != "%4" || got.PID != 0 || got.Pane.TTY != "" {
		t.Errorf("a short answer read %+v", got)
	}
}
