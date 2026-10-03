package tmux

import (
	"strings"
	"testing"
)

func TestLandingPutsTheLineAboveTheMatchAtTheTop(t *testing.T) {
	// Twenty history lines and a screen under them: a match on line 15
	// puts line 14 at the top, which is six lines up from the end of the
	// history, the cursor on its end, and the search forward from there.
	got := strings.Join(landArgs("%4", 20, 15, "error"), " ")
	want := "copy-mode -t %4 ; send-keys -t %4 -X goto-line 6 ; send-keys -t %4 -X top-line ; send-keys -t %4 -X end-of-line ; send-keys -t %4 -X search-forward-text error"
	if got != want {
		t.Errorf("landing runs\n%s\nwant\n%s", got, want)
	}
	// A match on the screen itself scrolls nothing back past it, and
	// one on the first line of all has no line above: the cursor starts
	// at the top's start.
	if got := strings.Join(landArgs("%4", 20, 25, "x"), " "); !strings.Contains(got, "goto-line 0 ") {
		t.Errorf("a match below the history scrolls to %s", got)
	}
	if got := strings.Join(landArgs("%4", 20, 0, "x"), " "); !strings.Contains(got, "goto-line 20 ;") || !strings.Contains(got, "start-of-line") {
		t.Errorf("the first line lands as %s", got)
	}
}
