package draw

import (
	"strings"
	"testing"

	"github.com/w0zro/conn/internal/theme"
)

// drawn is what a piece comes to on a line, in a palette: the row as
// Emit frames it, which is what a view puts on the screen.
func drawn(p Palette, width int, draw func(*Line)) string {
	c := Canvas{P: p, Width: width}
	l := c.Line()
	draw(l)
	c.Emit(l, 0, false)
	return c.Rows[0].Text
}

// A piece keeps in the plain palette whatever it says in glyphs, and
// drops whatever it said in color. The dots stay: ● and ○ are not the
// same word. The stamp's half-cells go: they are a block of color
// rounded off, and with no color to round they are two stray
// characters where a word should start.
func TestAPieceKeepsItsGlyphsAndDropsItsColor(t *testing.T) {
	stamped := drawn(Plain, 40, func(l *Line) { l.Stamp("WAITING 7M") })
	if strings.Contains(stamped, stampLeft) || strings.Contains(stamped, stampRight) {
		t.Errorf("the plain stamp is capped: %q", stamped)
	}
	if !strings.Contains(stamped, "WAITING 7M") {
		t.Errorf("the plain stamp does not say its word: %q", stamped)
	}
	if w := StampWidth("WAITING 7M", Plain); w != 12 {
		t.Errorf("the plain stamp is %d cells, want 12", w)
	}

	lit := drawn(Colored(theme.Conn.Dark), 40, func(l *Line) { l.Stamp("WAITING 7M") })
	if !strings.Contains(lit, stampLeft) || !strings.Contains(lit, stampRight) {
		t.Errorf("the stamp is not capped at half a cell: %q", lit)
	}
	if w := StampWidth("WAITING 7M", Colored(theme.Conn.Dark)); w != 14 {
		t.Errorf("the stamp is %d cells, want 14", w)
	}

	for _, d := range marks {
		if got := drawn(Plain, 40, func(l *Line) { l.Dot("", d) }); !strings.Contains(got, d) {
			t.Errorf("the plain palette dropped the mark %q: %q", d, got)
		}
	}
}

// An eyebrow is a label, a rule running off it, and what it counts at
// the end: the label in capitals whatever case it was given, the count
// flush with the right edge, and the rule taking what is left between
// them. It is what a box was for, at the cost of one row and no
// columns.
func TestAnEyebrowRunsToItsCount(t *testing.T) {
	got := drawn(Plain, 40, func(l *Line) { l.Eyebrow(0, "WAITING FOR YOU", 30, "2") })
	want := strings.Repeat(" ", Margin) + "WAITING FOR YOU " + strings.Repeat("─", 12) + " 2"
	if got != want {
		t.Errorf("eyebrow:\n got %q\nwant %q", got, want)
	}
	// A label is drawn in the case it was given: a project is named by
	// its path, and the path keeps its own.
	got = drawn(Plain, 40, func(l *Line) { l.Eyebrow(0, "w0zro/conn", 24, "") })
	want = strings.Repeat(" ", Margin) + "w0zro/conn " + strings.Repeat("─", 13)
	if got != want {
		t.Errorf("a path in an eyebrow:\n got %q\nwant %q", got, want)
	}
	// With nothing to count, the rule runs to the edge itself.
	got = drawn(Plain, 40, func(l *Line) { l.Eyebrow(0, "PROCEDURE", 20, "") })
	want = strings.Repeat(" ", Margin) + "PROCEDURE " + strings.Repeat("─", 10)
	if got != want {
		t.Errorf("eyebrow with no count:\n got %q\nwant %q", got, want)
	}
}

// A line typed into is a field: the word before it, the ground it is
// typed on, what has been typed, and the caret after it. The field
// holds its width whether anything has been typed or not, so the row
// does not change shape as it is typed into.
func TestAFieldHoldsItsWidth(t *testing.T) {
	c := Canvas{P: Colored(theme.Conn.Dark), Width: 40}
	empty, typed := c.Line(), c.Line()
	empty.Field(0, 20, "FIND", "", "")
	typed.Field(0, 20, "FIND", "pr", "o")
	if empty.Cells != typed.Cells {
		t.Errorf("a field typed into is %d cells and an empty one %d", typed.Cells, empty.Cells)
	}
	// The field is cut down to the ground, not raised to the ground a
	// chosen row sits on: a box to type into is told from the cursor.
	g := theme.Conn.Dark
	if got := drawn(Colored(g).OnSurface(), 40, func(l *Line) { l.Field(0, 20, "FIND", "pr", "o") }); !strings.Contains(got, groundIn(theme.Hex(g.Ground))+inkIn(theme.Hex(g.Ink))) || strings.Contains(got, groundIn(g.Border)) {
		t.Errorf("the field is not on the ground under the surface: %q", got)
	}
	if got := drawn(Plain, 40, func(l *Line) { l.Field(0, 20, "FIND", "pro", "") }); !strings.Contains(got, "FIND   pro"+caret) {
		t.Errorf("the field does not say what was typed: %q", got)
	}
	// The caret stands where the typing left it, not at the end.
	if got := drawn(Plain, 40, func(l *Line) { l.Field(0, 20, "FIND", "pr", "o") }); !strings.Contains(got, "pr"+caret+"o") {
		t.Errorf("the caret is not where it was left: %q", got)
	}
}

// A key is the manual's row, put where the decision is made: the word
// with a cell of ground each side, and the width a caller places it by.
func TestAKeyIsTheWordAndItsGround(t *testing.T) {
	if got := drawn(Plain, 40, func(l *Line) { l.Key("Enter") }); !strings.Contains(got, " Enter") {
		t.Errorf("key: %q", got)
	}
	if w := KeyWidth("Enter"); w != 7 {
		t.Errorf("a key of five letters is %d cells, want 7", w)
	}
}

// A card is a block of the surface with half a cell of it above and
// below: the edges are drawn in the surface as ink, on whatever ground
// the card is not on, which is how a fill gets an edge in a grid of
// whole cells. The plain palette has no surface, so a card is its rows
// and nothing around them.
func TestACardHasAnEdgeAboveAndBelow(t *testing.T) {
	c := Canvas{P: Colored(theme.Conn.Dark), Width: 40}
	c.Card(CardAbove, 4, 20, 0)
	c.Card(CardBelow, 4, 20, 0)
	if len(c.Rows) != 2 {
		t.Fatalf("a card drew %d edges, want 2", len(c.Rows))
	}
	if !strings.Contains(c.Rows[0].Text, strings.Repeat(CardAbove, 20)) {
		t.Errorf("the edge above: %q", c.Rows[0].Text)
	}
	if !strings.Contains(c.Rows[1].Text, strings.Repeat(CardBelow, 20)) {
		t.Errorf("the edge below: %q", c.Rows[1].Text)
	}
	if !strings.Contains(c.Rows[0].Text, Colored(theme.Conn.Dark).edge) {
		t.Error("the edge is not drawn in the surface")
	}
}

// The surface is a ground of its own: a block lifted onto it sits there
// edge to edge, and is not the ground a chosen row sits on. The plain
// palette has no ground to raise, and lifts nothing.
func TestTheSurfaceIsNotTheSelection(t *testing.T) {
	p := Colored(theme.Conn.Dark)
	if p.Lifted().Ground == p.Chosen().Ground {
		t.Error("a block set apart and a row chosen sit on the same ground")
	}
	if p.Lifted().Ground == p.Ground {
		t.Error("a block lifted sits on the ground it was lifted off")
	}
	if Plain.Lifted() != Plain {
		t.Error("the plain palette lifted something")
	}
}

// The marks a row can wear.
var marks = []string{MarkContact, MarkShell, MarkEditor, MarkService, MarkRun}
