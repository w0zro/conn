// man writes the man page from the manual. docs/index.html is the one
// text: every section of it is a heading of the page, every numbered
// paragraph a paragraph tagged with its number, every table a tagged
// list under its caption, every figure a block set as it is. The man page
// is that text in roff, for the reader who types man conn, and is never
// edited by hand: edit the manual, run this, commit both. A test holds
// the two together.
//
// The page's version and date are the manual's stamp and the latest row
// of its record; -version names another, for the release that is being
// cut and not yet recorded.
//
//	go run ./tools/man [-version v] [docs/index.html [man/conn.1]]
package main

import (
	"flag"
	"fmt"
	"html"
	"os"
	"regexp"
	"strings"
)

func main() {
	version := flag.String("version", "", "the version the page says it is for, instead of the manual's stamp")
	flag.Parse()
	in, out := "docs/index.html", "man/conn.1"
	if flag.NArg() > 0 {
		in = flag.Arg(0)
	}
	if flag.NArg() > 1 {
		out = flag.Arg(1)
	}
	page, err := os.ReadFile(in)
	if err != nil {
		fmt.Fprintln(os.Stderr, "man:", err)
		os.Exit(1)
	}
	roff, err := render(string(page), *version)
	if err != nil {
		fmt.Fprintln(os.Stderr, "man:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(out, []byte(roff), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "man:", err)
		os.Exit(1)
	}
}

var (
	stamp      = regexp.MustCompile(`REV (\d[\d.]*)<small>`)
	recordRow  = regexp.MustCompile(`<div class="row"><span>[\d.]+</span><span>([A-Z]{3}) (\d{4})</span>`)
	tagline    = regexp.MustCompile(`<p class="tagline">(.*?)</p>`)
	subline    = regexp.MustCompile(`<p class="sub">(.*?)</p>`)
	months     = "JANFEBMARAPRMAYJUNJULAUGSEPOCTNOVDEC"
	monthNames = strings.Fields("January February March April May June July August September October November December")

	// The blocks a section is made of, in the manual's own markup.
	opener = regexp.MustCompile(`<p class="kicker">|<h2>|<p class="para">|<p class="caption">|<div class="table[" ]|<div class="figure[" ]|<div class="cmd"|<div class="note">`)
	number = regexp.MustCompile(`^<span class="n">([^<]*)</span>`)
	row    = regexp.MustCompile(`<div class="row( head)?">(.*?)</div>`)
	cell   = regexp.MustCompile(`<span>(.*?)</span>`)
	pre    = regexp.MustCompile(`(?s)<pre>(.*?)</pre>`)
	code   = regexp.MustCompile(`<code>(.*?)</code>`)
	noteP  = regexp.MustCompile(`(?s)<p>(.*?)</p>\s*</div>\s*$`)
	tags   = regexp.MustCompile(`<[^>]+>`)
)

// render is the page in roff.
func render(page, version string) (string, error) {
	var b strings.Builder
	if version == "" {
		m := stamp.FindStringSubmatch(page)
		if m == nil {
			return "", fmt.Errorf("the manual has no REV stamp")
		}
		version = m[1]
	}
	date := ""
	for _, m := range recordRow.FindAllStringSubmatch(page, -1) {
		if i := strings.Index(months, m[1]); i >= 0 {
			date = monthNames[i/3] + " " + m[2]
		}
	}
	fmt.Fprintf(&b, ".\\\" Written from docs/index.html by go run ./tools/man; edit the manual, not this.\n")
	fmt.Fprintf(&b, ".TH CONN 1 \"%s\" \"conn %s\" \"User Commands\"\n", date, strings.TrimPrefix(version, "v"))

	line := tagline.FindStringSubmatch(page)
	if line == nil {
		return "", fmt.Errorf("the cover has no tagline")
	}
	name := strings.TrimSuffix(inline(line[1]), ".")
	b.WriteString(".SH NAME\nconn \\- " + strings.ToLower(name[:1]) + name[1:] + "\n")
	b.WriteString(".SH SYNOPSIS\n.B conn\n.br\n.B conn ls\n.br\n.B conn restart\n.br\n.B conn \\-h | \\-\\-help | \\-\\-version\n")

	sub := subline.FindStringSubmatch(page)
	if sub == nil {
		return "", fmt.Errorf("the cover has no description")
	}
	b.WriteString(".SH DESCRIPTION\n" + wrap(inline(sub[1])) + "\n")

	for _, s := range sections(page) {
		if err := blocks(&b, s); err != nil {
			return "", err
		}
	}
	return b.String(), nil
}

// sections is the body of every div.section on the page, in order — the
// record of revisions left out, being the tags' and not the text's.
func sections(page string) []string {
	var out []string
	for at := 0; ; {
		i := strings.Index(page[at:], `<div class="section"`)
		if i < 0 {
			return out
		}
		start := at + i
		end := divEnd(page, start)
		body := page[start:end]
		if !strings.Contains(body, `<p class="kicker">RECORD OF REVISIONS</p>`) {
			out = append(out, body)
		}
		at = end
	}
}

// divEnd is the index just past the </div> closing the div opened at
// start, counting the divs opened and closed between.
func divEnd(s string, start int) int {
	depth := 0
	for at := start; ; {
		open := strings.Index(s[at:], "<div")
		close := strings.Index(s[at:], "</div>")
		if close < 0 {
			return len(s)
		}
		if open >= 0 && open < close {
			depth++
			at += open + 4
			continue
		}
		depth--
		at += close + 6
		if depth == 0 {
			return at
		}
	}
}

// blocks writes a section's blocks in the order they come: its heading,
// its paragraphs, its captions and the tables and figures under them. A
// caption or a figure straight under the heading starts no paragraph,
// there being none to break from.
func blocks(b *strings.Builder, s string) error {
	fresh := false
	pp := func() {
		if !fresh {
			b.WriteString(".PP\n")
		}
		fresh = false
	}
	for {
		loc := opener.FindStringIndex(s)
		if loc == nil {
			return nil
		}
		s = s[loc[0]:]
		switch {
		case strings.HasPrefix(s, `<p class="kicker">`), strings.HasPrefix(s, `<p class="para">`), strings.HasPrefix(s, `<p class="caption">`):
			end := strings.Index(s, "</p>")
			body := s[strings.Index(s, ">")+1 : end]
			switch {
			case strings.HasPrefix(s, `<p class="para">`):
				tag := ""
				if m := number.FindStringSubmatch(body); m != nil {
					tag, body = m[1], body[len(m[0]):]
				}
				b.WriteString(".TP\n" + tag + "\n" + wrap(inline(body)) + "\n")
				fresh = false
			case strings.HasPrefix(s, `<p class="caption">`):
				pp()
				b.WriteString(".I \"" + inline(body) + "\"\n")
			}
			s = s[end:]
		case strings.HasPrefix(s, "<h2>"):
			end := strings.Index(s, "</h2>")
			b.WriteString(".SH " + strings.ToUpper(inline(s[4:end])) + "\n")
			fresh = true
			s = s[end:]
		case strings.HasPrefix(s, `<div class="table`):
			end := divEnd(s, 0)
			table(b, s[:end])
			fresh = false
			s = s[end:]
		case strings.HasPrefix(s, `<div class="figure`):
			end := divEnd(s, 0)
			m := pre.FindStringSubmatch(s[:end])
			if m == nil {
				return fmt.Errorf("a figure without a pre")
			}
			pp()
			block(b, m[1])
			s = s[end:]
		case strings.HasPrefix(s, `<div class="cmd"`):
			end := divEnd(s, 0)
			m := code.FindStringSubmatch(s[:end])
			if m == nil {
				return fmt.Errorf("a command without its code")
			}
			pp()
			block(b, "$ "+m[1])
			s = s[end:]
		case strings.HasPrefix(s, `<div class="note">`):
			end := divEnd(s, 0)
			m := noteP.FindStringSubmatch(s[:end])
			if m == nil {
				return fmt.Errorf("a note without its text")
			}
			b.WriteString(".TP\n\\fBNOTE\\fR\n" + wrap(inline(m[1])) + "\n")
			fresh = false
			s = s[end:]
		}
	}
}

// table writes a table as a tagged list: each row's first cell the tag,
// its last the text, and any cell between — a default — in parentheses
// after, named by the head; a head row names the columns and is not a
// row of its own.
func table(b *strings.Builder, s string) {
	var head []string
	b.WriteString(".RS\n")
	for _, r := range row.FindAllStringSubmatch(s, -1) {
		var cells []string
		for _, c := range cell.FindAllStringSubmatch(r[2], -1) {
			cells = append(cells, c[1])
		}
		if r[1] != "" {
			head = cells
			continue
		}
		if len(cells) < 2 {
			continue
		}
		text := inline(cells[len(cells)-1])
		for i := 1; i < len(cells)-1; i++ {
			if v := inline(cells[i]); v != "—" {
				label := ""
				if i < len(head) {
					label = strings.ToLower(inline(head[i])) + " "
				}
				text += " (" + label + v + ")"
			}
		}
		b.WriteString(".TP\n" + inline("<b>"+cells[0]+"</b>") + "\n" + wrap(text) + "\n")
	}
	b.WriteString(".RE\n")
}

// block writes a figure or a command as it is, set off and unfilled.
func block(b *strings.Builder, s string) {
	b.WriteString(".RS\n.nf\n")
	for line := range strings.SplitSeq(strings.TrimRight(s, "\n"), "\n") {
		line = escape(html.UnescapeString(tags.ReplaceAllString(line, "")))
		if strings.HasPrefix(line, ".") || strings.HasPrefix(line, "'") {
			line = "\\&" + line
		}
		b.WriteString(line + "\n")
	}
	b.WriteString(".fi\n.RE\n")
}

// inline is a run of the manual's text in roff: what it sets in bold,
// bold; the rest plain; the entities read; roff's own characters escaped.
// A hyphen in bold is a command's or an option's and is set as a minus,
// so it copies out as one.
func inline(s string) string {
	s = strings.NewReplacer("<b>", "\x01", "</b>", "\x02", "<code>", "\x01", "</code>", "\x02").Replace(s)
	s = escape(html.UnescapeString(tags.ReplaceAllString(s, "")))
	s = strings.Join(strings.Fields(s), " ")
	var b strings.Builder
	bold := false
	for _, r := range s {
		switch {
		case r == '\x01':
			bold = true
			b.WriteString("\\fB")
		case r == '\x02':
			bold = false
			b.WriteString("\\fR")
		case r == '-' && bold:
			b.WriteString("\\-")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// wrap breaks filled text into lines of at most 80 bytes at its spaces,
// the way a man page's source is written; roff fills them again. A line
// that would begin with a period or an apostrophe is roff's request, and
// is marked as text.
func wrap(s string) string {
	var lines []string
	line := ""
	for _, w := range strings.Fields(s) {
		if line != "" && len(line)+1+len(w) > 80 {
			lines = append(lines, line)
			line = ""
		}
		if line == "" {
			if strings.HasPrefix(w, ".") || strings.HasPrefix(w, "'") {
				w = "\\&" + w
			}
			line = w
		} else {
			line += " " + w
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// escape makes text safe to set: a backslash is roff's, and a quote would
// end a quoted argument.
func escape(s string) string {
	return strings.NewReplacer(`\`, `\e`, `"`, `\(dq`).Replace(s)
}
