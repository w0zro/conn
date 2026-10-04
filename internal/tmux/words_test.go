package tmux

import (
	"reflect"
	"strings"
	"testing"

	"github.com/w0zro/conn/internal/theme"
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

func TestTheStatusLineWordsAreTmuxFormats(t *testing.T) {
	g := theme.Conn.Dark
	ground := theme.Hex(g.Ground)
	// A block is lit in the accent with the ground knocked out of it,
	// and nothing where there is no word; a word stands on the band's
	// own ground, in a colour and a weight; what conn says in words is
	// on the surface in the parchment. A hash is tmux's own character
	// on the line and is doubled wherever conn's text carries one.
	for name, c := range map[string]struct{ got, want string }{
		"block":     {StatusLineBlock("COPY", g), "#[bg=" + g.Accent + " fg=" + ground + " bold] COPY "},
		"no block":  {StatusLineBlock("", g), ""},
		"word":      {StatusLineWord(" CONN ", theme.Hex(g.Ink), true, g), "#[bg=" + g.Border + " fg=" + theme.Hex(g.Ink) + " bold] CONN "},
		"figure":    {StatusLineWord("14:32 ", g.Gray, false, g), "#[bg=" + g.Border + " fg=" + g.Gray + " nobold]14:32 "},
		"hash":      {StatusLineWord("#3", g.Gray, false, g), "#[bg=" + g.Border + " fg=" + g.Gray + " nobold]##3"},
		"said":      {StatusLineSay("kill -TERM 123", g), "#[bg=" + g.Surface + " fg=" + g.Parchment + " nobold] kill -TERM 123"},
		"said hash": {StatusLineSay("#1", g), "#[bg=" + g.Surface + " fg=" + g.Parchment + " nobold] ##1"},
	} {
		if c.got != c.want {
			t.Errorf("%s:\n%s\nwant\n%s", name, c.got, c.want)
		}
	}
}

func TestTheKeyBarIsAKeyAndAWordEach(t *testing.T) {
	g := theme.Conn.Dark
	ink := theme.Hex(g.Ink)
	got := KeyBar([]Hint{{"j k", "Move"}, {"enter", "Go In"}}, g)
	want := " #[bg=" + g.Surface + " fg=" + ink + " bold]j k #[nobold fg=" + g.Gray + "]move" +
		"   #[bg=" + g.Surface + " fg=" + ink + " bold]enter #[nobold fg=" + g.Gray + "]go in"
	if got != want {
		t.Errorf("the bar reads\n%s\nwant\n%s", got, want)
	}
	// The copy-mode bar is written inside a conditional of the format's
	// own, where a comma is the conditional's: no hint carries one, and
	// no style is written with one.
	bar := KeyBar(CopyHints, g)
	if strings.Contains(bar, ",") {
		t.Errorf("the copy-mode bar carries a comma: %s", bar)
	}
	for _, h := range CopyHints {
		if h.Key == "" || h.Does == "" {
			t.Errorf("a copy hint is missing a half: %+v", h)
		}
	}
}
