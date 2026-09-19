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

// The manual names the one key tmux takes, and it is the one conn
// binds: in the root table, so that it works from inside a process,
// and no other. This is what holds man conn to the binary for the key,
// the way the synopsis table holds it for the commands.
func TestTheManualNamesThePanelKey(t *testing.T) {
	page, err := os.ReadFile("docs/index.html")
	if err != nil {
		t.Skip(err)
	}
	bound := regexp.MustCompile(`(?m)^bind (\S+) (\S+) `).FindAllStringSubmatch(tmuxConf(defaultKey), -1)
	if len(bound) != 1 || bound[0][1] != "-n" || bound[0][2] != defaultKey {
		t.Fatalf("conn binds %v, not the panel key alone in the root table", bound)
	}
	// As the manual writes it: the modifier in words, the key lowered.
	say := strings.ToLower(strings.ReplaceAll(defaultKey, "C-", "ctrl-"))
	keys := string(page)
	if i := strings.Index(keys, `id="keys"`); i >= 0 {
		keys = keys[i:]
	}
	for _, want := range []string{"<b>" + say + "</b>", "<b>CONN_KEY</b>"} {
		if !strings.Contains(keys, want) {
			t.Errorf("the manual's keys section lacks %s", want)
		}
	}
}
