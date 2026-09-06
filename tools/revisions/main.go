// revisions writes the manual's record of revisions from the repository's
// release tags: one row per v* tag, oldest first — its revision, the month
// it was cut, and what it did — and the stamp and the change number on
// every page brought up to the latest. The release workflow runs it after
// a release and commits what changed, so the manual records a release the
// way a technical manual does, without anyone typing the row.
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
	version   = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)$`)
	saidOnly  = regexp.MustCompile(`^(?:scrn |conn )?v?\d+\.\d+\.\d+:?\s*`)
	months    = []string{"JAN", "FEB", "MAR", "APR", "MAY", "JUN", "JUL", "AUG", "SEP", "OCT", "NOV", "DEC"}
	rowsBlock = regexp.MustCompile(`(?s)(<!-- revisions -->\n)(.*?)(\s*<!-- /revisions -->)`)
	rowShape  = regexp.MustCompile(`<div class="row"><span>([\d.]+)</span><span>[^<]*</span><span>([^<]*)</span></div>`)
	stamp     = regexp.MustCompile(`REV \d[\d.]*<small>`)
	change    = regexp.MustCompile(`<span>CHANGE \d+</span>`)
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

// record rewrites the page's record of revisions from the tags, keeping a
// row's description where the tag has none, and brings the stamp and the
// change number on every page up to the latest tag.
func record(page string, tags []tag) (string, error) {
	m := rowsBlock.FindStringSubmatchIndex(page)
	if m == nil {
		return "", fmt.Errorf("the page has no <!-- revisions --> block")
	}
	old := page[m[4]:m[5]]
	kept := map[string]string{}
	for _, r := range rowShape.FindAllStringSubmatch(old, -1) {
		kept[r[1]] = r[2]
	}
	var b bytes.Buffer
	for _, t := range tags {
		desc := t.desc
		if desc == "" {
			desc = kept[t.rev]
		}
		if desc == "" {
			desc = "Release."
		}
		fmt.Fprintf(&b, "        <div class=\"row\"><span>%s</span><span>%s</span><span>%s</span></div>\n", t.rev, t.date, desc)
	}
	rows := strings.TrimSuffix(b.String(), "\n")
	page = page[:m[4]] + rows + page[m[5]:]
	latest := tags[len(tags)-1]
	page = stamp.ReplaceAllString(page, "REV "+latest.rev+"<small>")
	page = change.ReplaceAllString(page, "<span>CHANGE "+latest.minor+"</span>")
	return page, nil
}
