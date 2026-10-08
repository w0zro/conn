package main

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/w0zro/conn/internal/draw"
	"github.com/w0zro/conn/internal/room"
	"github.com/w0zro/conn/internal/work"
)

// The card is the manual's rows put where the decision is made, so
// every key on it is a key the manual documents. A key added to conn
// and put on the card without a line in the manual, or renamed in one
// and not the other, is the two records of the same station drifting
// apart; this is what holds them together.
func TestTheCardShowsOnlyKeysTheManualHas(t *testing.T) {
	section := manSection(string(manPage), "KEYS")
	if section == "" {
		t.Fatal("the manual has no keys section")
	}
	for _, g := range panelKeys("ctrl-space") {
		for _, h := range g.keys {
			for _, k := range strings.Fields(h.key) {
				if !boldWord(section, k) {
					t.Errorf("%s says %q, and the manual does not", g.title, k)
				}
			}
		}
	}
}

// And the other way: every key the manual gives the panel is on the
// card. A key the panel answers to and the card leaves out is a key the
// operator does not learn the station by, which is how o went missing
// from it. A click is not a key, and has no place on a card of keys.
func TestTheCardHasEveryKeyTheManualGivesThePanel(t *testing.T) {
	section := manSection(string(manPage), "KEYS")
	_, panel, ok := strings.Cut(section, ".SS On the panel, in the processes view\n")
	if !ok {
		t.Fatal("the manual has no subsection for the panel's keys")
	}
	panel, _, _ = strings.Cut(panel, "\n.SS ")
	onCard := map[string]bool{}
	for _, g := range panelKeys("ctrl-space") {
		for _, h := range g.keys {
			for _, k := range strings.Fields(h.key) {
				onCard[k] = true
			}
		}
	}
	lines := strings.Split(panel, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, ".TP") || i+1 == len(lines) {
			continue
		}
		term := strings.Fields(strings.NewReplacer(".BR ", "", ".B ", "", `"`, "", ",", "", `\-`, "-").Replace(lines[i+1]))
		if len(term) == 0 || term[0] == "left" {
			continue
		}
		if !onCard[term[0]] {
			t.Errorf("the manual gives the panel %q, and the card does not", term[0])
		}
	}
}

// manSection is the text of one .SH, for a test reading the page as the
// record it is.
func manSection(page, name string) string {
	_, rest, ok := strings.Cut(page, ".SH "+name+"\n")
	if !ok {
		return ""
	}
	if next := strings.Index(rest, "\n.SH "); next >= 0 {
		return rest[:next]
	}
	return rest
}

// boldWord says whether the page sets a word in bold anywhere: a key is
// .B or .BR, alone or among others, and roff writes ctrl-space with the
// hyphen escaped. On a .BR line the roman half is punctuation between
// the keys — .BR enter , — and is not a key; on a .B line everything
// is bold, which is how the comma is written as a key of its own.
func boldWord(page, word string) bool {
	word = strings.ReplaceAll(word, "-", `\-`)
	for _, line := range strings.Split(page, "\n") {
		roman := strings.HasPrefix(line, ".BR ")
		if !roman && !strings.HasPrefix(line, ".B ") {
			continue
		}
		for _, f := range strings.Fields(strings.NewReplacer(`", "`, " ", `"`, "").Replace(line)) {
			if roman && strings.Trim(f, ",.;:") == "" {
				continue
			}
			if f == word {
				return true
			}
		}
	}
	return false
}

// The card is drawn in the panel, which is forty-four columns wide and
// is not made wider for it: a row that does not fit is elided, and a
// key whose word is cut in half is a key the card did not say.
func TestTheCardFitsThePanel(t *testing.T) {
	rows := drawKeys(panelKeys("^space"), "processes", room.PanelWidth, 0, draw.Plain)
	if len(rows) < 20 {
		t.Fatalf("the card came to %d rows", len(rows))
	}
	for _, r := range rows {
		if strings.Contains(r.Text, "…") {
			t.Errorf("the panel cuts a row of the card: %q", r.Text)
		}
		if w := utf8.RuneCountInString(r.Text); w > room.PanelWidth {
			t.Errorf("a row runs %d columns past the panel: %q", w-room.PanelWidth, r.Text)
		}
	}
}

// A card too tall for the pane says how much of it is below rather than
// running off the foot of the window.
func TestTheCardSaysWhatIsBelowIt(t *testing.T) {
	rows := drawKeys(panelKeys("^space"), "processes", room.PanelWidth, 12, draw.Plain)
	if len(rows) != 12 {
		t.Fatalf("the card came to %d rows in a pane of 12", len(rows))
	}
	if !strings.Contains(rows[len(rows)-1].Text, "BELOW") {
		t.Errorf("the foot of a cut card says %q", rows[len(rows)-1].Text)
	}
}

// While the manual is in the workspace the panel holds the keys. The
// processes view is not worked while the manual is up — the keys are in
// the manual's own pane — so the half of the window beside it says what
// the keys are rather than showing a list going nowhere.
func TestThePanelHoldsTheKeysWhileTheManualIsUp(t *testing.T) {
	m := model{reading: reading{projects: []work.Project{{Path: "/w", Entries: []work.Entry{{PID: 11, TTY: "ttys001", Command: "vim"}}}}}, view: viewProcesses, inside: true, p: draw.Plain, width: room.PanelWidth, height: 40}
	if strings.Contains(m.View().Content, "THE ROW") {
		t.Fatal("the processes view is showing the keys with no manual up")
	}
	next, _ := m.Update(detourMsg{toManual})
	m = next.(model)
	content := m.View().Content
	for _, want := range []string{"KEYS", "THE ROW", "IN A PROCESS"} {
		if !strings.Contains(content, want) {
			t.Errorf("the panel does not say %q while the manual is up:\n%s", want, content)
		}
	}
	if strings.Contains(content, "vim") {
		t.Errorf("the panel is still drawing the processes view:\n%s", content)
	}
}
