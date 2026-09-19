package main

import (
	"fmt"
	"strings"
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
	moveHint     = keyHint{"j k", "Move"}
	rootsHints   = []keyHint{moveHint, {"enter", "Saves it"}}
	consoleHints = []keyHint{{"any key", "Continue"}}
	helpHints    = []keyHint{{"esc", "Back"}}
)

// keyBar is the hints as the bar writes them: each key in the ink and
// bold, what it does in the gray after it and in the lower case, so
// the key is the one thing that stands up in the row; three cells
// between one and the next, a cell in from the edge, on the surface,
// which is the bar's ground.
func keyBar(hints []keyHint) string {
	var b strings.Builder
	b.WriteString(" ")
	for i, h := range hints {
		if i > 0 {
			b.WriteString("   ")
		}
		fmt.Fprintf(&b, "#[bg=%s fg=%s bold]%s #[nobold fg=%s]%s", surfaceHex, hex(inkColor), h.key, grayHex, strings.ToLower(h.does))
	}
	return b.String()
}

// designation is the station's mark at the right of the key bar: the
// host in capitals, and the conn that is running.
func designation(host, version string) string {
	return fmt.Sprintf("#[bg=%s fg=%s nobold]%s ", surfaceHex, grayHex,
		strings.ReplaceAll(join(" · ", strings.ToUpper(host), strings.TrimSpace("conn "+version)), "#", "##"))
}
