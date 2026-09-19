package main

import (
	_ "embed"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// The manual conn carries: man/conn.1, written by hand and built into
// the binary rather than looked for on the machine. A conn run from a
// build directory has no installed page, and an installed conn may have
// one from an older release beside a newer binary. The manual a conn
// shows is the manual that conn was built with, which is the only one
// guaranteed to describe it.
//
//go:embed man/conn.1
var manPage []byte

// manPath is where conn puts the page for man to read, beside its own
// state. It is written on each showing rather than kept current,
// because it costs nothing and a stale copy is the thing this exists to
// avoid.
func manPath(home string) string {
	return filepath.Join(stateHome(home), "conn", "conn.1")
}

// writeManPage puts the manual where man can be pointed at it.
func writeManPage(home string) (string, error) {
	path := manPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, manPage, 0o644)
}

// manText is the manual as lines of text at a width, read by running
// man over the page conn just wrote. man is asked for the text and not
// for a pager: conn does the paging itself, in the pane it holds, so
// the keys in the manual are conn's own and not less's.
func manText(path string, width int) []string {
	cmd := exec.Command("man", path)
	cmd.Env = append(os.Environ(),
		"MANPAGER=cat", "PAGER=cat",
		"MANWIDTH="+strconv.Itoa(max(width, minCols)))
	out, err := cmd.Output()
	if err != nil {
		return []string{"the manual could not be read: " + err.Error()}
	}
	return strings.Split(strings.TrimRight(string(out), "\n"), "\n")
}

// A run of the manual's text: what it says, and whether man set it
// apart. man says bold and underline by overstriking — the character,
// a backspace, and the character again — which is how a page was
// emphasised on a printer and is still how the text comes out.
type manRun struct {
	text string
	bold bool
}

// manLine reads one line of man's output into its runs, the overstrike
// taken off and kept as the emphasis it stood for.
func manLine(s string) []manRun {
	r := []rune(s)
	var out []manRun
	var cur strings.Builder
	curBold := false
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, manRun{text: cur.String(), bold: curBold})
			cur.Reset()
		}
	}
	for i := 0; i < len(r); i++ {
		ch, bold := r[i], false
		// Overstruck more than once for bold and underline together;
		// the last character written is the one that shows.
		for i+2 < len(r)+1 && i+2 <= len(r)-1 && r[i+1] == '\b' {
			ch, bold = r[i+2], true
			i += 2
		}
		if bold != curBold {
			flush()
			curBold = bold
		}
		cur.WriteRune(ch)
	}
	flush()
	return out
}
