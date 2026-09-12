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
		t.Fatal("the manual has no synopsis table")
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
