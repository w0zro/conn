package main

import "strings"

// The keys, where the decision is made. Every key conn has was
// invisible until prefix ?, which is the one thing a manual cannot
// mend: nobody opens the manual for a key they do not know is there.
// So the foot of the window carries the keys that work where the
// cursor is, a key and a word each, the way a footer says what a
// screen can do; see bar in tui.go, which chooses them, and bar.go,
// which draws them. It is the manual's rows put where the eye already
// is.

// A keyHint is a key and a word for what it does here.
type keyHint struct{ key, does string }

// The keys the bar says where the view has no cursor to ask; the
// rest are chosen by what the cursor's row can take, in bar.
var (
	moveHint     = keyHint{"j k", "Move"}
	rootsHints   = []keyHint{moveHint, {"Enter", "Saves it"}}
	consoleHints = []keyHint{{"Any key", "Continue"}}
	helpHints    = []keyHint{{"Esc", "Back"}}
)

// keyBar is the hints as the bar writes them: each key in the ink and
// bold, what it does in the gray after it, three cells between one and
// the next. The bar's ground is the band's, which the bar itself
// paints; the pieces here return to it after each color.
func keyBar(hints []keyHint, p palette) string {
	var b strings.Builder
	for i, h := range hints {
		if i > 0 {
			b.WriteString("   ")
		}
		b.WriteString(p.ink + p.bold + h.key + p.end + p.selection + " " + p.gray + h.does + p.end + p.selection)
	}
	return b.String()
}

// designation is the station's mark at the right of the key bar: the
// host in capitals, and the conn that is running.
func designation(host, version string, p palette) string {
	return p.gray + join(" · ", strings.ToUpper(host), strings.TrimSpace("conn "+version)) + p.end + p.selection
}

// visible is how many cells a string with escapes in it takes.
func visible(s string) int {
	n := 0
	in := false
	for _, r := range s {
		switch {
		case in:
			if r == 'm' {
				in = false
			}
		case r == 0x1b:
			in = true
		default:
			n++
		}
	}
	return n
}
