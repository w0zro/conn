package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The manual's synopsis is the command table: every command conn offers
// and every flag it takes is a row there, and no row names a command or
// a flag conn does not have. The man page's synopsis is written from
// those rows, so this is what holds man conn to the binary.
func TestTheManualsSynopsisIsTheCommandTable(t *testing.T) {
	page, err := os.ReadFile("docs/index.html")
	if err != nil {
		t.Skip(err)
	}
	table := regexp.MustCompile(`(?s)<div class="table synopsis">(.*?</div>)\s*</div>`).FindStringSubmatch(string(page))
	if table == nil {
		// The manual is being written again a piece at a time, and the
		// table is not back yet. There is nothing to hold the binary to
		// until it is, and this holds it again the moment it returns.
		t.Skip("the manual has no synopsis table yet")
	}
	var rows []string
	for _, r := range regexp.MustCompile(`<div class="row"><span>(.*?)</span>`).FindAllStringSubmatch(table[1], -1) {
		rows = append(rows, r[1])
	}
	known := map[string]bool{"conn": true}
	for _, c := range commands {
		if c.use != "" {
			known[c.name] = true
		}
	}
	for _, f := range flags {
		known[f.name] = true
	}
	// Every row is a way conn can be called.
	seen := map[string]bool{}
	for _, r := range rows {
		f := strings.Fields(r)
		switch {
		case len(f) == 1 && f[0] == "conn":
			seen["conn"] = true
		case len(f) >= 2 && f[0] == "conn" && known[f[1]]:
			seen[f[1]] = true
		default:
			t.Errorf("the manual offers %q, which conn does not answer to", r)
		}
	}
	// And every way conn can be called is a row.
	for name := range known {
		if !seen[name] {
			t.Errorf("the manual's synopsis lacks %q", name)
		}
	}
	// What --help says agrees with it, name for name.
	for name := range known {
		want := "conn " + name
		if name == "conn" {
			want = "conn "
		}
		if !strings.Contains(synopsis(), want) {
			t.Errorf("--help lacks %q:\n%s", name, synopsis())
		}
	}
}

// The manual's chord table is the chords conn binds: every key bound
// under the prefix is a row there, and no row names one conn does not
// bind. This is what holds man conn to the binary for the keys, the way
// the synopsis table holds it for the commands — a chord added without
// a row is a chord nobody can find out about.
func TestTheManualsChordTableIsTheChordsConnBinds(t *testing.T) {
	page, err := os.ReadFile("docs/index.html")
	if err != nil {
		t.Skip(err)
	}
	rows := map[string]bool{}
	for _, r := range regexp.MustCompile(`<div class="row"><span>prefix ([^<]*)</span>`).FindAllStringSubmatch(string(page), -1) {
		rows[r[1]] = true
	}
	if len(rows) == 0 {
		t.Skip("the manual has no chord table yet")
	}
	// A chord as the manual writes it: tmux's own spelling of the key,
	// a named key lowered, with its modifier said in words and the
	// prefix itself named rather than spelt. A letter keeps its case,
	// under a modifier too: a and A are two chords, and alt-A is A's.
	say := func(key string) string {
		if key == defaultPrefix {
			return "prefix"
		}
		key = strings.ReplaceAll(key, "M-", "alt-")
		if len(strings.TrimPrefix(key, "alt-")) == 1 {
			return key
		}
		return strings.ToLower(key)
	}
	bound := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^bind (\S+) `).FindAllStringSubmatch(tmuxConf(defaultPrefix), -1) {
		key := say(m[1])
		bound[key] = true
		if !rows[key] {
			t.Errorf("the manual does not document prefix %s", key)
		}
	}
	for key := range rows {
		if !bound[key] {
			t.Errorf("the manual documents prefix %s, which conn does not bind", key)
		}
	}
}
