package tmux

import (
	"reflect"
	"testing"
)

// The pure parts of the package: what conn says to tmux and reads back
// from it, held by tables, where the server tests hold the rest.

func TestAPathIsQuotedForTheShell(t *testing.T) {
	for in, want := range map[string]string{
		"/Users/w0zro/projects":        "'/Users/w0zro/projects'",
		"/Users/w0zro/SkellyLabs, Inc": "'/Users/w0zro/SkellyLabs, Inc'",
		"/tmp/it's":                    `'/tmp/it'\''s'`,
		"":                             "''",
	} {
		if got := ShellQuote(in); got != want {
			t.Errorf("ShellQuote(%q) = %s, want %s", in, got, want)
		}
	}
}

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
	if got != want {
		t.Errorf("parseOpened read %+v, want %+v", got, want)
	}
	// A short answer is not a crash: the fields it lacks are blank.
	if got := parseOpened("%4"); got.Pane.ID != "%4" || got.PID != 0 || got.Pane.TTY != "" {
		t.Errorf("a short answer read %+v", got)
	}
}

func TestThePanelKeyIsTheOperatorsToSet(t *testing.T) {
	t.Setenv("CONN_KEY", "")
	if got := PanelKey(); got != DefaultKey {
		t.Errorf("the key is %q by default, want %q", got, DefaultKey)
	}
	t.Setenv("CONN_KEY", "M-a")
	if got := PanelKey(); got != "M-a" {
		t.Errorf("CONN_KEY names %q, want M-a", got)
	}
}

func TestOnlyWorkStillRunningIsReachable(t *testing.T) {
	for name, c := range map[string]struct {
		p    Pane
		want bool
	}{
		"work":        {Pane{ID: "%4"}, true},
		"no pane":     {Pane{}, false},
		"a hold":      {Pane{ID: "%4", Hold: true}, false},
		"the readout": {Pane{ID: "%4", Readout: true}, false},
		"ended":       {Pane{ID: "%4", Dead: true}, false},
	} {
		if got := Reachable(c.p); got != c.want {
			t.Errorf("%s: Reachable = %v, want %v", name, got, c.want)
		}
	}
}
