package main

// The keys, where the decision is made. Every key conn has was
// invisible until prefix ?, which is the one thing a manual cannot
// mend: nobody opens the manual for a key they do not know is there.
// So the foot of the window carries the keys that work where the
// cursor is, a key and a word each, the way a footer says what a
// screen can do; see keyBar in tmux.go and bar in tui.go. It is the
// manual's rows put where the eye already is.

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
