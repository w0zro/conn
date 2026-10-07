package draw

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/w0zro/conn/internal/theme"

	"github.com/charmbracelet/x/ansi"
)

// A value that will not fit is cut at its end; a path gives up its
// middle and keeps its name.
func TestPathsShortenFromTheMiddle(t *testing.T) {
	exe := "/var/folders/51/kkgwpd9j2r53trb71lnmfp6r0000gn/T/go-build3688959642/b001/exe/conn"
	for _, c := range []struct {
		in   string
		w    int
		path bool
		want string
	}{
		{exe + " · 5.5 MB", 90, true, exe + " · 5.5 MB"},
		{exe + " · 5.5 MB", 60, true, "/var/…/T/go-build3688959642/b001/exe/conn · 5.5 MB"},
		{exe + " · 5.5 MB", 30, true, "/var/…/b001/exe/conn · 5.5 MB"},
		{exe + " · 5.5 MB", 22, true, "/var/…/conn · 5.5 MB"},
		{exe, 8, true, "…xe/conn"},
		{"~/projects/w0zro/conn/conn", 20, true, "~/…/w0zro/conn/conn"},
		{"~/projects/w0zro/conn", 40, true, "~/projects/w0zro/conn"},
		{"APPLE M3 PRO · 11 CORES (5P + 6E)", 12, false, "APPLE M3 PR…"},
		{"/NOT/A/PATH/BY/ITS/FLAG", 12, false, "/NOT/A/PATH…"},
		{"/a/b", 1, true, ""},
	} {
		if got := Fit(c.in, c.w, c.path); got != c.want || utf8.RuneCountInString(got) > c.w {
			t.Errorf("fit(%q, %d, %v) = %q, want %q", c.in, c.w, c.path, got, c.want)
		}
	}
}

// A value is held to its width in the cells it takes on the screen, not
// the characters it is written in: a project named in Japanese, or a
// session title with an emoji in it, takes two cells a character, and a
// row cut by its characters ran past the panel's edge.
func TestValuesAreHeldToCellsNotCharacters(t *testing.T) {
	for _, s := range []string{
		"~/projects/プロジェクト/設計書",
		"fix the 🙂 in the greeting and the 漢字 in the title",
		"plain ascii that is long enough to be cut somewhere",
	} {
		for w := 2; w <= 24; w++ {
			for _, path := range []bool{false, true} {
				if got := Fit(s, w, path); ansi.StringWidth(got) > w {
					t.Errorf("fit(%q, %d, %v) = %q, %d cells", s, w, path, got, ansi.StringWidth(got))
				}
			}
			for _, l := range WrapValue(s, w) {
				if ansi.StringWidth(l) > w {
					t.Errorf("wrapValue(%q, %d) gave %q, %d cells", s, w, l, ansi.StringWidth(l))
				}
			}
		}
	}
	// A character wider than the line goes on a line of its own, rather
	// than holding the wrap in place for ever.
	if got := WrapValue("漢字", 1); len(got) != 2 {
		t.Errorf("wrapValue of wide characters in one cell = %q", got)
	}
}

// The line as drawn keeps the caret on screen. What fits is drawn
// whole; a filter longer than the room shows its start until the caret
// goes past it, and a path shows its end until the caret goes before
// it; a cut end is marked.
func TestTheDrawnLineKeepsTheCaretOnScreen(t *testing.T) {
	for _, c := range []struct {
		text          string
		caret, room   int
		path          bool
		before, after string
	}{
		{"conn", 4, 10, false, "conn", ""},
		{"conn", 1, 10, false, "c", "onn"},
		{"abcdefghij", 10, 5, false, "…ghij", ""},
		{"abcdefghij", 0, 5, false, "", "abcd…"},
		{"abcdefghij", 6, 5, false, "…cde…", ""},
		{"/a/b/c/d/e", 10, 5, true, "…/d/e", ""},
		{"/a/b/c/d/e", 0, 5, true, "", "/a/b…"},
		{"ab", 1, 1, false, "", ""},
	} {
		before, after := TypedRuns(c.text, c.caret, c.room, c.path)
		if before != c.before || after != c.after {
			t.Errorf("typedRuns(%q, %d, %d, %v) = %q, %q; want %q, %q", c.text, c.caret, c.room, c.path, before, after, c.before, c.after)
		}
	}
}

// Drawn narrow, a row's port is the last thing to go: in eight cells
// it stands alone, without the dot that joined it to the command,
// and only a width the port itself does not fit gives the cells
// to the command.
func TestTheActivityGivesTheCommandUpBeforeThePorts(t *testing.T) {
	l := (&Canvas{P: Plain, Width: 20}).Line()
	l.Activity(Plain.Ink, Plain.Gray, "node vite", []string{"5173"}, 8)
	if l.b.String() != ":5173" {
		t.Errorf("in eight cells the row says %q", l.b.String())
	}
	l = (&Canvas{P: Plain, Width: 20}).Line()
	l.Activity(Plain.Ink, Plain.Gray, "node vite", []string{"5173"}, 12)
	if l.b.String() != "nod… · :5173" {
		t.Errorf("in twelve cells the row says %q", l.b.String())
	}
	l = (&Canvas{P: Plain, Width: 20}).Line()
	l.Activity(Plain.Ink, Plain.Gray, "node vite", []string{"5173"}, 4)
	if l.b.String() != "nod…" {
		t.Errorf("in four cells the row says %q", l.b.String())
	}
}

// The palette on the surface paints its rows on it, and returns to
// it after every piece.
func TestThePaletteOnTheSurfaceGroundsOnIt(t *testing.T) {
	p := Colored(theme.Conn.Dark).OnSurface()
	if p.Ground != groundIn(theme.Conn.Dark.Surface) || !strings.HasPrefix(p.Normal, p.End+p.Ground) {
		t.Errorf("the surface palette grounds on %q", p.Ground)
	}
	if got := Plain.OnSurface(); !got.Plain || got.Ground != "" {
		t.Error("the plain palette took a ground")
	}
}
