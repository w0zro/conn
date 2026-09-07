package main

import (
	"os"
	"strings"
	"testing"
)

// The man page in the tree is what this tool writes from the manual; a
// manual edited without the page rewritten fails here, with the command
// that mends it.
func TestTheManPageIsWrittenFromTheManual(t *testing.T) {
	page, err := os.ReadFile("../../docs/index.html")
	if err != nil {
		t.Skip(err)
	}
	want, err := os.ReadFile("../../man/conn.1")
	if err != nil {
		t.Skip(err)
	}
	got, err := render(string(page), "")
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("man/conn.1 is not the manual's; run go run ./tools/man")
	}
}

// Bold is bold, a hyphen in it is a minus, entities are read, and roff's
// own characters are escaped.
func TestInline(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{`<b>conn --help</b> and <b>-</b>`, `\fBconn \-\-help\fR and \fB\-\fR`},
		{`a &mdash; b, <span class="mark">&#10007;</span> &lt;uid&gt;`, `a — b, ✗ <uid>`},
		{`say "so" \ once`, `say \(dqso\(dq \e once`},
		{"two\n  lines", `two lines`},
	} {
		if got := inline(c.in); got != c.want {
			t.Errorf("inline(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Filled text is wrapped at its spaces, and a line that would begin
// with a period is marked as text rather than read as a request.
func TestWrap(t *testing.T) {
	long := strings.Repeat("word ", 20) + ".conn is a plan"
	got := wrap(long)
	for line := range strings.SplitSeq(got, "\n") {
		if len(line) > 80 {
			t.Errorf("a line of %d bytes: %q", len(line), line)
		}
	}
	if !strings.Contains(got, "\n\\&.conn") && !strings.Contains(got, " .conn") {
		t.Errorf("wrap lost .conn: %q", got)
	}
	if strings.Contains(got, "\n.conn") {
		t.Errorf("a line begins with a request: %q", got)
	}
}

// The tagged lists keep a table's default beside its text, named by the
// head, and a head row is not a row.
func TestTable(t *testing.T) {
	var b strings.Builder
	table(&b, `<div class="table three">
          <div class="row head"><span>FIELD</span><span>DEFAULT</span><span>FUNCTION</span></div>
          <div class="row"><span>navWidth</span><span>30</span><span>the width</span></div>
          <div class="row"><span>skipDirs</span><span>&mdash;</span><span>never entered</span></div>
        </div>`)
	want := ".RS\n.TP\n\\fBnavWidth\\fR\nthe width (default 30)\n.TP\n\\fBskipDirs\\fR\nnever entered\n.RE\n"
	if b.String() != want {
		t.Errorf("table:\n%s\nwant:\n%s", b.String(), want)
	}
}
