package main

import (
	"fmt"
	"strings"

	"github.com/w0zro/conn/internal/tmux"

	"github.com/w0zro/conn/internal/theme"
)

// The keys, where the decision is made. Every key conn has was
// invisible until ?, which is the one thing a manual cannot
// mend: nobody opens the manual for a key they do not know is there.
// So the foot of the window carries the keys that work where the
// cursor is, a key and a word each, the way a footer says what a
// screen can do; see bar in tui.go, which chooses them. It is the
// manual's rows put where the eye already is.

// A keyHint is a key and a word for what it does here.
type keyHint struct{ key, does string }

// The keys the bar says where the view has no cursor to ask; the
// rest are chosen by what the cursor's row can take, in bar.
var (
	moveHint = keyHint{"j k", "Move"}
	// On a line typed into, j and k are letters, and the cursor moves
	// on the arrows and readline's own pair; the bar says the arrows.
	typedMoveHint = keyHint{"up down", "Move"}
	rootsHints    = []keyHint{typedMoveHint, {"enter", "Saves it"}}
	consoleHints  = []keyHint{{"any key", "Continue"}}
	// A root typed in the settings, where esc is a way back to the
	// rows. The first start has none: conn cannot show anything until
	// the line is answered, and a key that did nothing would be conn
	// pretending there was a way past it.
	rootsBarHints = append(append([]keyHint{}, rootsHints...), keyHint{"esc", "Back"})
	// The manual has the keys while it is up, so the bar says the
	// manual's: the panel beside it is the card of conn's own, which is
	// read rather than worked.
	helpHints = []keyHint{moveHint, {"space b", "Page"}, {"g G", "Top, end"}, {"esc", "Back"}}
)

// keyBar is the hints as the bar writes them; the writing is the
// status line's own, in tmux.KeyBar, where the bar for copy mode is
// written too.
func keyBar(hints []keyHint, g theme.Ground) string {
	hs := make([]tmux.Hint, len(hints))
	for i, h := range hints {
		hs[i] = tmux.Hint{Key: h.key, Does: h.does}
	}
	return tmux.KeyBar(hs, g)
}

// designation is the station's mark at the right of the key bar: the
// host in capitals, and the conn that is running.
func designation(host, version string, g theme.Ground) string {
	return fmt.Sprintf("#[bg=%s fg=%s nobold]%s ", g.Surface, g.Gray,
		strings.ReplaceAll(join(" · ", strings.ToUpper(host), strings.TrimSpace("conn "+version)), "#", "##"))
}
