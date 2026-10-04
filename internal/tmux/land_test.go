package tmux

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLandingSearchesBackFromTheEndAndCentres(t *testing.T) {
	got := strings.Join(landSearch("%4", 2, "error"), " ")
	want := "copy-mode -t %4 ; send-keys -t %4 -X history-bottom ; send-keys -t %4 -X end-of-line ; send-keys -t %4 -X search-backward-text error ; send-keys -t %4 -X -N 2 search-again"
	if got != want {
		t.Errorf("the search runs\n%s\nwant\n%s", got, want)
	}
	if got := strings.Join(landSearch("%4", 0, "x"), " "); strings.Contains(got, "search-again") {
		t.Error("the last saying needs no search-again")
	}
	// tmux put the match three quarters down a twelve-row pane, at row
	// 9, scrolled 153 up a history of 294: the middle is row 5, so the
	// view scrolls four lines down and the line is on row 5.
	if scroll, row := centering(9, 153, 294, 12); scroll != 149 || row != 5 {
		t.Errorf("centring gives scroll %d row %d, want 149 and 5", scroll, row)
	}
	// Near the top of a short history the view cannot scroll past the
	// top, and the line sits as near the middle as it can.
	if scroll, row := centering(2, 10, 10, 12); scroll != 10 || row != 2 {
		t.Errorf("at the top of history centring gives scroll %d row %d, want 10 and 2", scroll, row)
	}
	// At the foot, with nothing below, the view cannot scroll past the
	// bottom either.
	if scroll, row := centering(10, 0, 294, 12); scroll != 0 || row != 10 {
		t.Errorf("at the foot centring gives scroll %d row %d, want 0 and 10", scroll, row)
	}
	got = strings.Join(landCenter("%4", 149, 5, "error"), " ")
	want = "send-keys -t %4 -X goto-line 149 ; send-keys -t %4 -X top-line ; send-keys -t %4 -X -N 4 cursor-down ; send-keys -t %4 -X end-of-line ; send-keys -t %4 -X search-forward-text error"
	if got != want {
		t.Errorf("centring runs\n%s\nwant\n%s", got, want)
	}
	if got := strings.Join(landCenter("%4", 10, 1, "x"), " "); strings.Contains(got, "cursor-down") || !strings.Contains(got, "end-of-line") {
		t.Errorf("a line on the second row has the first row above it: %s", got)
	}
	if got := strings.Join(landCenter("%4", 10, 0, "x"), " "); !strings.Contains(got, "start-of-line") {
		t.Errorf("a line on the first row starts from its start: %s", got)
	}
}

// The landing against tmux itself: a pane of numbered lines, landed on
// by saying, is on the line, in the middle, and still there after the
// pane is wrapped to another width. Skipped where there is no tmux.
func TestLandingOnARealPane(t *testing.T) {
	bin, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("no tmux")
	}
	dir, err := os.MkdirTemp("/tmp", "cl")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	s := &Server{Tmux: bin, Socket: filepath.Join(dir, "sock")}
	if _, err := s.Run("new-session", "-d", "-s", "t", "-x", "80", "-y", "12", "sh"); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = s.Run("kill-server") }()
	time.Sleep(300 * time.Millisecond)
	if _, err := s.Run("send-keys", "-t", "t", "seq 1 300; echo aa aa end; seq 1 3", "Enter"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	where := func() string {
		out, _ := s.Run("display-message", "-p", "-t", "t", "#{copy_cursor_line}|#{copy_cursor_y}|#{copy_cursor_x}|#{pane_in_mode}")
		return strings.TrimSpace(out)
	}
	if err := s.Land("t", 0, "150"); err != nil {
		t.Fatal(err)
	}
	if got := where(); got != "150|5|0|1" {
		t.Errorf("landed at %q, want line 150 on row 5", got)
	}
	// The second saying back of 15 is 159; the row of aa aa is one row
	// and lands on its first saying, which is two back from the end.
	if err := s.Land("t", 1, "15"); err != nil {
		t.Fatal(err)
	}
	if got := where(); !strings.HasPrefix(got, "159|5|0|") {
		t.Errorf("one saying back landed at %q, want 159", got)
	}
	if err := s.Land("t", 1, "aa"); err != nil {
		t.Fatal(err)
	}
	if got := where(); !strings.HasPrefix(got, "aa aa end|") || !strings.HasSuffix(got, "|0|1") {
		t.Errorf("the line's first saying landed at %q", got)
	}
	// Wrapped to another width, the saying is the same saying.
	s.Unmode("t")
	if _, err := s.Run("resize-window", "-t", "t", "-x", "40", "-y", "12"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := s.Land("t", 0, "150"); err != nil {
		t.Fatal(err)
	}
	if got := where(); got != "150|5|0|1" {
		t.Errorf("after a reflow landed at %q, want line 150 on row 5", got)
	}
	s.Unmode("t")
	if got := where(); !strings.HasSuffix(got, "|0") {
		t.Errorf("Unmode left the pane in a mode: %q", got)
	}
}
