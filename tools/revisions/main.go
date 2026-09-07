// revisions writes the manual's record of revisions from the repository's
// release tags: one row per v* tag, oldest first — its revision, the month
// it was cut, and what it did — and the stamp and the change number on
// every page brought up to the latest. The release workflow runs it after
// a release and commits what changed, so the manual records a release the
// way a technical manual does, without anyone typing the row.
//
// The cover carries the latest few rows beside the stamp; the whole record
// is a page of its own after the cover, the way a manual's front matter
// keeps it, paginating as it grows.
//
// A row's description is the tag's message when the message says more than
// the version — git tag -a v0.4.0 -m "Endings, tasks and the transcript" —
// and otherwise whatever the table already says for that revision, so the
// rows written by hand before this tool stand. A tag with nothing to say
// and no row yet is recorded as a release.
//
//	go run ./tools/revisions [docs/index.html]
package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

func main() {
	path := "docs/index.html"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	out, err := exec.Command("git", "for-each-ref", "--sort=version:refname",
		"--format=%(refname:short)\t%(creatordate:short)\t%(contents:subject)", "refs/tags/v*").Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, "revisions: listing tags:", err)
		os.Exit(1)
	}
	page, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "revisions:", err)
		os.Exit(1)
	}
	tags := parseTags(string(out))
	if len(tags) == 0 {
		fmt.Fprintln(os.Stderr, "revisions: no v* tags")
		os.Exit(1)
	}
	next, err := record(string(page), tags)
	if err != nil {
		fmt.Fprintln(os.Stderr, "revisions:", err)
		os.Exit(1)
	}
	if next == string(page) {
		fmt.Println("revisions: the manual is current")
		return
	}
	if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "revisions:", err)
		os.Exit(1)
	}
	fmt.Printf("revisions: recorded %s\n", tags[len(tags)-1].rev)
}

// tag is one release as the table records it.
type tag struct {
	rev   string // 0.3, or 0.3.1 for a patch
	minor string // the change number: 3
	date  string // SEP 2026
	desc  string // the tag's message, when it says more than the version
}

var (
	version     = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)$`)
	saidOnly    = regexp.MustCompile(`^(?:scrn |conn )?v?\d+\.\d+\.\d+:?\s*`)
	months      = []string{"JAN", "FEB", "MAR", "APR", "MAY", "JUN", "JUL", "AUG", "SEP", "OCT", "NOV", "DEC"}
	rowsBlock   = regexp.MustCompile(`(?s)(<!-- revisions -->\n)(.*?)(\s*<!-- /revisions -->)`)
	recordBlock = regexp.MustCompile(`(?s)(<!-- record -->\n)(.*?)(<!-- /record -->)`)
	rowShape    = regexp.MustCompile(`<div class="row"><span>([\d.]+)</span><span>[^<]*</span><span>([^<]*)</span></div>`)
	stamp       = regexp.MustCompile(`REV \d[\d.]*<small>`)
	change      = regexp.MustCompile(`<span>CHANGE \d+</span>`)
)

// The cover's share of the record, and a record page's: the cover has
// room for a few rows beside the stamp; a page of its own holds a column
// of them under a heading, and the pages after it, without one, more.
const (
	coverRows     = 3
	firstPageRows = 24
	nextPageRows  = 30
)

// parseTags reads for-each-ref's lines: the tag, the date it was cut, and
// its message's first line, tab-separated, oldest first.
func parseTags(listing string) []tag {
	var tags []tag
	for line := range strings.SplitSeq(strings.TrimSpace(listing), "\n") {
		f := strings.SplitN(line, "\t", 3)
		if len(f) < 2 {
			continue
		}
		m := version.FindStringSubmatch(f[0])
		if m == nil {
			continue
		}
		t := tag{rev: m[1] + "." + m[2], minor: m[2]}
		if m[3] != "0" {
			t.rev += "." + m[3]
		}
		if d := strings.Split(f[1], "-"); len(d) == 3 {
			if mo, err := strconv.Atoi(d[1]); err == nil && mo >= 1 && mo <= 12 {
				t.date = months[mo-1] + " " + d[0]
			}
		}
		if len(f) == 3 {
			// The message describes the release when it says more than the
			// version: "v0.2.0: scrn is called conn" does, "v0.3.0" does not.
			if said := strings.TrimSpace(saidOnly.ReplaceAllString(f[2], "")); said != "" {
				if !strings.HasSuffix(said, ".") {
					said += "."
				}
				t.desc = said
			}
		}
		tags = append(tags, t)
	}
	return tags
}

// record rewrites the page's record of revisions from the tags — the
// latest few on the cover, all of them on the record pages after it —
// keeping a row's description where the tag has none, and brings the
// stamp and the change number on every page up to the latest tag.
func record(page string, tags []tag) (string, error) {
	m := rowsBlock.FindStringSubmatchIndex(page)
	if m == nil {
		return "", fmt.Errorf("the page has no <!-- revisions --> block")
	}
	rm := recordBlock.FindStringSubmatchIndex(page)
	if rm == nil {
		return "", fmt.Errorf("the page has no <!-- record --> block")
	}
	// What the table already says, from the cover and the record alike:
	// the cover keeps only the latest, the record all.
	kept := map[string]string{}
	for _, r := range rowShape.FindAllStringSubmatch(page[m[4]:m[5]]+page[rm[4]:rm[5]], -1) {
		kept[r[1]] = r[2]
	}
	for i := range tags {
		if tags[i].desc == "" {
			tags[i].desc = kept[tags[i].rev]
		}
		if tags[i].desc == "" {
			tags[i].desc = "Release."
		}
	}
	latest := tags[len(tags)-1]

	// The record pages go in first, so the cover's block is not moved by
	// the write below it.
	page = page[:rm[4]] + recordPages(tags, latest.minor) + page[rm[5]:]
	m = rowsBlock.FindStringSubmatchIndex(page)
	cover := tags
	if len(cover) > coverRows {
		cover = cover[len(cover)-coverRows:]
	}
	page = page[:m[4]] + rows(cover, "        ") + page[m[5]:]

	page = stamp.ReplaceAllString(page, "REV "+latest.rev+"<small>")
	page = change.ReplaceAllString(page, "<span>CHANGE "+latest.minor+"</span>")
	return page, nil
}

// rows is the table's rows for the tags, indented as the page indents them.
func rows(tags []tag, indent string) string {
	var b bytes.Buffer
	for _, t := range tags {
		fmt.Fprintf(&b, "%s<div class=\"row\"><span>%s</span><span>%s</span><span>%s</span></div>\n", indent, t.rev, t.date, t.desc)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// recordPages is the record of revisions as pages of the manual: the first
// under its heading, the rest continuing it, each with the table's head
// and as many rows as a page holds, folioed R-1, R-2… as front matter.
func recordPages(tags []tag, minor string) string {
	var b bytes.Buffer
	rest := tags
	for n := 1; len(rest) > 0 || n == 1; n++ {
		take := nextPageRows
		if n == 1 {
			take = firstPageRows
		}
		if take > len(rest) {
			take = len(rest)
		}
		these, remaining := rest[:take], rest[take:]
		rest = remaining
		fmt.Fprintf(&b, `<section class="page" aria-label="Page R-%d">
  <span class="hole"></span><span class="hole"></span><span class="hole"></span>
  <span class="classified top">UNCLASSIFIED</span><span class="classified bottom">UNCLASSIFIED</span>
  <div class="sheet">
    <div class="running"><b>TM-CONN-01</b><span>OPERATING MANUAL</span><span>RECORD OF REVISIONS</span></div>
    <div class="rule"></div>
    <div class="body">
      <div class="section">
`, n)
		if n == 1 {
			b.WriteString(`        <p class="kicker">RECORD OF REVISIONS</p>
        <h2>Revisions</h2>
        <p class="para">Each release of conn, as the tag that cut it described it. The stamp on the cover names the one in effect.</p>
`)
		}
		b.WriteString(`        <div class="table record">
          <div class="row head"><span>REV</span><span>DATE</span><span>DESCRIPTION</span></div>
`)
		b.WriteString(rows(these, "          "))
		fmt.Fprintf(&b, `
        </div>
      </div>
    </div>
    <div class="foot">
      <div class="rule"></div>
      <div class="folio"><span>TM-CONN-01</span><b>R-%d</b><span>CHANGE %s</span></div>
    </div>
  </div>
</section>

`, n, minor)
	}
	return b.String()
}
