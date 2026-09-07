package main

import (
	"fmt"
	"strings"
	"testing"
)

const sample = `<div class="table">
        <div class="row head"><span>REV</span><span>DATE</span><span>DESCRIPTION</span></div>
        <!-- revisions -->
        <div class="row"><span>0.1</span><span>MAR 2026</span><span>Initial issue.</span></div>
        <div class="row"><span>0.3</span><span>SEP 2026</span><span>Agent kinds and models.</span></div>
        <!-- /revisions -->
      </div>
    <div class="stamp">REV 0.3<small>IN EFFECT</small></div>
</section>
<!-- record -->
<!-- /record -->
    <div class="folio"><span>TM-CONN-01</span><b>1-1</b><span>CHANGE 3</span></div>
    <div class="folio"><span>TM-CONN-01</span><b>2-1</b><span>CHANGE 3</span></div>
`

func TestTheTagsBecomeRowsAndTheStampFollows(t *testing.T) {
	// A tag's message describes the release when it says more than the
	// version; otherwise the row keeps what the table already said, and a
	// tag with nothing to say and no row is a release. The stamp and every
	// change number follow the latest tag; a patch release shows its
	// patch.
	tags := parseTags("v0.1.0\t2026-08-30\tscrn v0.1.0\n" +
		"v0.2.0\t2026-09-02\tv0.2.0: scrn is called conn\n" +
		"v0.3.0\t2026-09-04\tv0.3.0\n" +
		"v0.4.0\t2026-09-08\tEndings, tasks and the transcript\n" +
		"v0.4.1\t2026-09-09\t\n" +
		"junk\t2026-09-09\tnot a tag\n")
	if len(tags) != 5 {
		t.Fatalf("parsed %d tags, want 5", len(tags))
	}
	got, err := record(sample, tags)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<span>0.1</span><span>AUG 2026</span><span>Initial issue.</span>`,
		`<span>0.2</span><span>SEP 2026</span><span>scrn is called conn.</span>`,
		`<span>0.3</span><span>SEP 2026</span><span>Agent kinds and models.</span>`,
		`<span>0.4</span><span>SEP 2026</span><span>Endings, tasks and the transcript.</span>`,
		`<span>0.4.1</span><span>SEP 2026</span><span>Release.</span>`,
		`REV 0.4.1<small>`,
		`aria-label="Page R-1"`, `<b>R-1</b>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the record lacks %q:\n%s", want, got)
		}
	}
	// The cover keeps the latest three; the record page has them all.
	coverPart := got[:strings.Index(got, "<!-- record -->")]
	if strings.Count(coverPart, `<div class="row"><span>`) != 3 || strings.Contains(coverPart, "<span>0.1</span>") {
		t.Errorf("the cover should carry the latest three rows:\n%s", coverPart)
	}
	if strings.Count(got, "<span>0.1</span>") != 1 || strings.Count(got, "<span>0.4.1</span>") != 2 {
		t.Errorf("the record should carry every row, the cover the latest:\n%s", got)
	}
	// The change number follows the latest tag on every page, the record's
	// included.
	if strings.Count(got, "<span>CHANGE 4</span>") != 3 || strings.Contains(got, "CHANGE 3") {
		t.Errorf("the change number did not follow the latest tag on every page:\n%s", got)
	}
	if strings.Contains(got, "MAR 2026") {
		t.Error("a row's date should come from the tag, not from what the table said")
	}
	// Run again on its own output: nothing changes.
	again, err := record(got, tags)
	if err != nil || again != got {
		t.Error("recording twice should be the same as once")
	}
	if _, err := record("no block here", tags); err == nil {
		t.Error("a page without the block should be refused")
	}
}

func TestTheRecordPaginatesAsItGrows(t *testing.T) {
	// A page holds a column of rows; the first fewer, under the heading.
	// Past that the record continues on pages of its own, R-2 and on.
	var listing strings.Builder
	for i := 1; i <= 40; i++ {
		fmt.Fprintf(&listing, "v0.%d.0\t2026-09-%02d\tRelease %d\n", i, 1+i%28, i)
	}
	tags := parseTags(listing.String())
	got, err := record(sample, tags)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(got, `aria-label="Page R-`) != 2 || !strings.Contains(got, "<b>R-2</b>") {
		t.Errorf("40 rows should take two record pages:\n%s", got)
	}
	first := got[strings.Index(got, `aria-label="Page R-1"`):strings.Index(got, `aria-label="Page R-2"`)]
	if n := strings.Count(first, `<div class="row"><span>`); n != firstPageRows {
		t.Errorf("the first record page holds %d rows, want %d", n, firstPageRows)
	}
	if strings.Count(got, "<h2>Revisions</h2>") != 1 {
		t.Error("only the first record page carries the heading")
	}
}
