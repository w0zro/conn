package main

import (
	"html"
	"os"
	"strings"
	"testing"
)

// The manual's appendix of keys is written by hand, in the manual's own
// markup; this is what keeps it the same list the ? popup shows. Every key
// the popup lists is in the appendix with the popup's words for it — the
// one exception being the popup's line about itself, which the manual
// says in its own words.
func TestTheManualsAppendixListsThePopupsKeys(t *testing.T) {
	b, err := os.ReadFile("docs/index.html")
	if err != nil {
		t.Skip(err)
	}
	page := html.UnescapeString(string(b))
	for _, k := range keyList {
		key, desc := k[0], k[1]
		if !strings.Contains(page, ">"+key+"<") {
			t.Errorf("the appendix lacks the key %q", key)
		}
		if desc == "this" {
			continue
		}
		if !strings.Contains(page, ">"+desc+"<") {
			t.Errorf("the appendix says something other than %q for %q", desc, key)
		}
	}
}

// Every page of the manual carries the classification marking top and
// bottom, the ones added after the handoff included.
func TestEveryPageOfTheManualIsMarked(t *testing.T) {
	b, err := os.ReadFile("docs/index.html")
	if err != nil {
		t.Skip(err)
	}
	pages := strings.Count(string(b), `<section class="page`)
	top := strings.Count(string(b), `class="classified top">UNCLASSIFIED<`)
	bottom := strings.Count(string(b), `class="classified bottom">UNCLASSIFIED<`)
	if pages == 0 || top != pages || bottom != pages {
		t.Errorf("%d pages, %d marked at the top and %d at the bottom; want every page marked twice", pages, top, bottom)
	}
}

// The table of commands against the server is the chords as commands,
// written by hand; every word the configuration binds is a row of it, so a
// chord added to the binary is added to the manual.
func TestTheManualsTableOfCommandsListsEveryChordWord(t *testing.T) {
	b, err := os.ReadFile("docs/index.html")
	if err != nil {
		t.Skip(err)
	}
	page := html.UnescapeString(string(b))
	_, table, ok := strings.Cut(page, `id="commands"`)
	if !ok {
		t.Fatal("the manual has no table of commands")
	}
	table, _, _ = strings.Cut(table, "</section>")
	for word := range chords {
		if !strings.Contains(table, "conn "+word) && !strings.Contains(table, ", "+word+"<") &&
			!strings.Contains(table, "<b>"+word+"</b>") {
			t.Errorf("the table of commands has no row for conn %s", word)
		}
	}
}
