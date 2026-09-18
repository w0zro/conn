package main

import (
	"strings"
	"testing"
)

// drawn is what a piece comes to on a line, in a palette: the row as
// emit frames it, which is what a view puts on the screen.
func drawn(p palette, width int, draw func(*line)) string {
	c := canvas{p: p, width: width}
	l := c.line()
	draw(l)
	c.emit(l, 0, false)
	return c.rows[0].text
}

// A piece keeps in the plain palette whatever it says in glyphs, and
// drops whatever it said in color. The dots stay: ● and ○ are not the
// same word. The stamp's half-cells go: they are a block of color
// rounded off, and with no color to round they are two stray
// characters where a word should start.
func TestAPieceKeepsItsGlyphsAndDropsItsColor(t *testing.T) {
	stamped := drawn(plain, 40, func(l *line) { l.stamp("WAITING 7M") })
	if strings.Contains(stamped, stampLeft) || strings.Contains(stamped, stampRight) {
		t.Errorf("the plain stamp is capped: %q", stamped)
	}
	if !strings.Contains(stamped, "WAITING 7M") {
		t.Errorf("the plain stamp does not say its word: %q", stamped)
	}
	if w := stampWidth("WAITING 7M", plain); w != 12 {
		t.Errorf("the plain stamp is %d cells, want 12", w)
	}

	lit := drawn(colored(), 40, func(l *line) { l.stamp("WAITING 7M") })
	if !strings.Contains(lit, stampLeft) || !strings.Contains(lit, stampRight) {
		t.Errorf("the stamp is not capped at half a cell: %q", lit)
	}
	if w := stampWidth("WAITING 7M", colored()); w != 14 {
		t.Errorf("the stamp is %d cells, want 14", w)
	}

	for _, d := range []string{dotWants, dotWorks, dotRests, dotOver} {
		if got := drawn(plain, 40, func(l *line) { l.dot("", d) }); !strings.Contains(got, d) {
			t.Errorf("the plain palette dropped the dot %q: %q", d, got)
		}
	}
}

// An eyebrow is a label, a rule running off it, and what it counts at
// the end: the label in capitals whatever case it was given, the count
// flush with the right edge, and the rule taking what is left between
// them. It is what a box was for, at the cost of one row and no
// columns.
func TestAnEyebrowRunsToItsCount(t *testing.T) {
	got := drawn(plain, 40, func(l *line) { l.eyebrow(0, "WAITING FOR YOU", 30, "2") })
	want := strings.Repeat(" ", margin) + "WAITING FOR YOU " + strings.Repeat("─", 12) + " 2"
	if got != want {
		t.Errorf("eyebrow:\n got %q\nwant %q", got, want)
	}
	// A label is drawn in the case it was given: a project is named by
	// its path, and the path keeps its own.
	got = drawn(plain, 40, func(l *line) { l.eyebrow(0, "w0zro/conn", 24, "") })
	want = strings.Repeat(" ", margin) + "w0zro/conn " + strings.Repeat("─", 13)
	if got != want {
		t.Errorf("a path in an eyebrow:\n got %q\nwant %q", got, want)
	}
	// With nothing to count, the rule runs to the edge itself.
	got = drawn(plain, 40, func(l *line) { l.eyebrow(0, "PROCEDURE", 20, "") })
	want = strings.Repeat(" ", margin) + "PROCEDURE " + strings.Repeat("─", 10)
	if got != want {
		t.Errorf("eyebrow with no count:\n got %q\nwant %q", got, want)
	}
}

// A line typed into is a field: the word before it, the ground it is
// typed on, what has been typed, and the caret after it. The field
// holds its width whether anything has been typed or not, so the row
// does not change shape as it is typed into.
func TestAFieldHoldsItsWidth(t *testing.T) {
	c := canvas{p: colored(), width: 40}
	empty, typed := c.line(), c.line()
	empty.field(0, 20, "FIND", "", "")
	typed.field(0, 20, "FIND", "pr", "o")
	if empty.cells != typed.cells {
		t.Errorf("a field typed into is %d cells and an empty one %d", typed.cells, empty.cells)
	}
	if got := drawn(plain, 40, func(l *line) { l.field(0, 20, "FIND", "pro", "") }); !strings.Contains(got, "FIND   pro"+caret) {
		t.Errorf("the field does not say what was typed: %q", got)
	}
	// The caret stands where the typing left it, not at the end.
	if got := drawn(plain, 40, func(l *line) { l.field(0, 20, "FIND", "pr", "o") }); !strings.Contains(got, "pr"+caret+"o") {
		t.Errorf("the caret is not where it was left: %q", got)
	}
}

// A key is the manual's row, put where the decision is made: the word
// with a cell of ground each side, and the width a caller places it by.
func TestAKeyIsTheWordAndItsGround(t *testing.T) {
	if got := drawn(plain, 40, func(l *line) { l.key("Enter") }); !strings.Contains(got, " Enter") {
		t.Errorf("key: %q", got)
	}
	if w := keyWidth("Enter"); w != 7 {
		t.Errorf("a key of five letters is %d cells, want 7", w)
	}
}

// A card is a block of the surface with half a cell of it above and
// below: the edges are drawn in the surface as ink, on whatever ground
// the card is not on, which is how a fill gets an edge in a grid of
// whole cells. The plain palette has no surface, so a card is its rows
// and nothing around them.
func TestACardHasAnEdgeAboveAndBelow(t *testing.T) {
	c := canvas{p: colored(), width: 40}
	c.card(cardAbove, 4, 20, 0)
	c.card(cardBelow, 4, 20, 0)
	if len(c.rows) != 2 {
		t.Fatalf("a card drew %d edges, want 2", len(c.rows))
	}
	if !strings.Contains(c.rows[0].text, strings.Repeat(cardAbove, 20)) {
		t.Errorf("the edge above: %q", c.rows[0].text)
	}
	if !strings.Contains(c.rows[1].text, strings.Repeat(cardBelow, 20)) {
		t.Errorf("the edge below: %q", c.rows[1].text)
	}
	if !strings.Contains(c.rows[0].text, colored().edge) {
		t.Error("the edge is not drawn in the surface")
	}
}

// The surface is a ground of its own: a block lifted onto it sits there
// edge to edge, and is not the ground a chosen row sits on. The plain
// palette has no ground to raise, and lifts nothing.
func TestTheSurfaceIsNotTheSelection(t *testing.T) {
	p := colored()
	if p.lifted().ground == p.chosen().ground {
		t.Error("a block set apart and a row chosen sit on the same ground")
	}
	if p.lifted().ground == p.ground {
		t.Error("a block lifted sits on the ground it was lifted off")
	}
	if plain.lifted() != plain {
		t.Error("the plain palette lifted something")
	}
}
