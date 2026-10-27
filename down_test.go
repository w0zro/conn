package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/w0zro/conn/internal/theme"
	"github.com/w0zro/conn/internal/tmux"
)

// A process of a project's stack that is written down and not running,
// as after a reboot or conn down, is a dim "down" row on the open
// project, after whatever runs; under the cursor, the workspace gives
// its name, "down" and its command. The projects screen counts them
// beside what runs, "3 down" or "1 running · 2 down". ss lists them
// and enter starts them; xx forgets one, for good. What a check or a
// name taken off wrote down is no stack process, and another project's
// is not this one's. Read from the screen as the person has it: the
// built conn in a tmux of its own, its keys sent, its screen captured.
func TestAStackProcessNotRunningIsADownRow(t *testing.T) {
	bin, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("no tmux")
	}
	shots := os.Getenv("SHOTS")
	g := theme.Dark

	// Not under /tmp, which conn takes for scratch that is no project.
	scratch, err := os.MkdirTemp("/var/tmp", "cs-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(scratch) })
	scratch, _ = filepath.EvalSymlinks(scratch)
	repo := func(name string) string {
		dir := filepath.Join(scratch, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		git(t, dir, "init", "-q")
		git(t, dir, "commit", "-q", "--allow-empty", "-m", "first")
		return dir
	}
	web, shop := repo("web"), repo("shop")
	conn := filepath.Join(t.TempDir(), "conn")
	build := exec.Command("go", "build", "-o", conn, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	// What was last written down, as a reboot leaves it.
	socket := socketPath(scratch)
	if err := os.MkdirAll(filepath.Dir(socket), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, r := range []started{
		{Root: web, Name: "api", Command: "sleep 1001", Role: roleStack},
		{Root: web, Name: "worker", Command: "sleep 1002", Role: roleStack},
		{Root: web, Name: "css", Command: "sleep 1003", Role: roleStack},
		{Root: web, Name: "unit", Command: "echo ok", Role: roleCheck},
		{Root: web, Name: "old", Command: "sleep 1005", Role: roleStack},
		{Root: web, Name: "old", Command: "sleep 1005", Role: roleOff},
		{Root: shop, Name: "db", Command: "sleep 1004", Role: roleStack},
	} {
		if err := recordStart(startedPath(socket), r); err != nil {
			t.Fatal(err)
		}
	}

	env := []string{"HOME=" + scratch, "CLAUDE_CONFIG_DIR=" + filepath.Join(scratch, ".claude"), "LC_ALL=C.UTF-8"}
	clean := func() []string {
		var out []string
		for _, e := range os.Environ() {
			if k, _, _ := strings.Cut(e, "="); k != "TMUX" && k != "TMUX_PANE" && k != "XDG_STATE_HOME" && k != "CONN_SOCKET" {
				out = append(out, e)
			}
		}
		return append(out, env...)
	}
	outer := filepath.Join(scratch, "outer.sock")
	tm := func(args ...string) string {
		cmd := exec.Command(bin, append([]string{"-S", outer}, args...)...)
		cmd.Env = clean()
		out, _ := cmd.Output()
		return string(out)
	}
	inner := &tmux.Server{Tmux: bin, Socket: socket}
	t.Cleanup(func() { tm("kill-server"); _ = inner.Down() })
	// run is conn's command line, as an agent at a shell in dir has it.
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command(conn, args...)
		cmd.Dir, cmd.Env = dir, clean()
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("conn %v: %v\n%s", args, err, out)
		}
	}

	// The screen as lines, plain or dressed, and each as its panel and
	// its workspace, split at the rule between them.
	screen := func(dressed bool) []string {
		args := []string{"capture-pane", "-p", "-t", "outer"}
		if dressed {
			args = append(args, "-e")
		}
		return strings.Split(strings.TrimRight(tm(args...), "\n"), "\n")
	}
	halves := func(lines []string) (panel []string, space string) {
		var right []string
		for _, l := range lines {
			left, r, _ := strings.Cut(l, "│")
			panel = append(panel, strings.TrimRight(left, " "))
			right = append(right, strings.TrimSpace(r))
		}
		return panel, strings.Join(right, "\n")
	}
	wait := func(what string, ok func(panel []string, space string) bool) []string {
		t.Helper()
		var lines []string
		for end := time.Now().Add(6 * time.Second); time.Now().Before(end); time.Sleep(60 * time.Millisecond) {
			lines = screen(false)
			if panel, space := halves(lines); ok(panel, space) {
				return lines
			}
		}
		t.Fatalf("never saw %s; the screen is:\n%s", what, strings.Join(lines, "\n"))
		return nil
	}
	keys := func(k ...string) { tm(append([]string{"send-keys", "-t", "outer"}, k...)...) }
	frame := func(name string) {
		if shots == "" {
			return
		}
		var b strings.Builder
		fmt.Fprintf(&b, "\x1b]10;%s\x1b\\\x1b]11;%s\x1b\\\x1b[2J\x1b[H", theme.Hex(g.Ink), g.Surface)
		b.WriteString(strings.Join(screen(true), "\x1b[0m\r\n") + "\x1b[0m")
		if err := os.WriteFile(filepath.Join(shots, name), []byte(b.String()), 0o644); err != nil {
			t.Error(err)
		}
	}

	// at is the line of the panel where a word stands on its own.
	at := func(panel []string, word string) int {
		for i, l := range panel {
			if regexp.MustCompile(`(^|\s)` + word + `(\s|$)`).MatchString(l) {
				return i
			}
		}
		return -1
	}
	// downAt is a name's line, where it says down.
	downAt := func(panel []string, name string) int {
		if i := at(panel, name); i >= 0 && regexp.MustCompile(`\bdown\b`).MatchString(panel[i]) {
			return i
		}
		return -1
	}
	// stands is whether the panel lists a name with no "down" on it.
	runsAt := func(panel []string, name string) int {
		if i := at(panel, name); i >= 0 && !strings.Contains(panel[i], "down") {
			return i
		}
		return -1
	}
	// project is a project's line on the projects screen.
	project := func(panel []string, name, says string) bool {
		for _, l := range panel {
			if regexp.MustCompile(`^\S?\s+` + name + `\s.*\s` + says + `$`).MatchString(l) {
				return true
			}
		}
		return false
	}
	onProjects := func(webSays, shopSays string) func(panel []string, space string) bool {
		return func(panel []string, _ string) bool {
			return project(panel, "web", webSays) && project(panel, "shop", shopSays)
		}
	}
	open := func(name string) {
		t.Helper()
		keys(name, "Enter")
		wait("the open "+name, func(panel []string, _ string) bool {
			return at(panel, "find") < 0 && regexp.MustCompile(`(?m)^\s+`+name+`\s+~/`+name+`$`).MatchString(strings.Join(panel, "\n"))
		})
	}
	// dimmed is whether the words stand on a captured line in the
	// theme's gray or faint ink, or dimmed, and not the plain ink.
	rgb := func(hex string) string {
		var c [3]int64
		for i := range c {
			c[i], _ = strconv.ParseInt(hex[1+2*i:3+2*i], 16, 0)
		}
		return fmt.Sprintf("%d;%d;%d", c[0], c[1], c[2])
	}
	quiet := map[string]bool{rgb(g.Gray): true, rgb(g.Faint): true}
	dimmed := func(raw string, words ...string) bool {
		raw, _, _ = strings.Cut(raw, "│")
		for _, word := range words {
			seen, loud := false, false
			fg, dim, last := "", false, 0
			segment := func(to int) {
				if regexp.MustCompile(`\b` + word + `\b`).MatchString(raw[last:to]) {
					seen = true
					loud = loud || !quiet[fg] && !dim
				}
			}
			for _, m := range regexp.MustCompile("\x1b\\[([0-9;]*)m").FindAllStringSubmatchIndex(raw, -1) {
				segment(m[0])
				last = m[1]
				p := strings.Split(raw[m[2]:m[3]], ";")
				for i := 0; i < len(p); i++ {
					switch p[i] {
					case "", "0":
						fg, dim = "", false
					case "2":
						dim = true
					case "22":
						dim = false
					case "39":
						fg = ""
					case "38", "48":
						// A color: 2 and its three parts, or 5 and its one.
						n := 2
						if i+1 < len(p) && p[i+1] == "2" {
							n = 4
						}
						if p[i] == "38" && i+n < len(p) {
							fg = strings.Join(p[i+1:i+n+1], ";")
							if n == 4 {
								fg = strings.Join(p[i+2:i+5], ";")
							}
						}
						i += n
					}
				}
			}
			segment(len(raw))
			if !seen || loud {
				return false
			}
		}
		return true
	}

	start := func() {
		t.Helper()
		tm("new-session", "-d", "-s", "outer", "-x", "120", "-y", "36", "env "+strings.Join(env, " ")+" "+conn)
		// A key skips the console to its end, and a key there goes on.
		wait("the console", func(panel []string, _ string) bool { return strings.Contains(strings.Join(panel, "\n"), "any key") })
		for end := time.Now().Add(5 * time.Second); time.Now().Before(end) && strings.Contains(strings.Join(screen(false), "\n"), "any key"); {
			keys("Enter")
			time.Sleep(400 * time.Millisecond)
		}
		wait("the projects", func(panel []string, _ string) bool { return at(panel, "find") >= 0 })
	}

	// After a reboot: the projects say how many are down in each.
	start()
	wait("web saying 3 down and shop 1 down", onProjects("3 down", "1 down"))
	frame("projects-120x36.ans")

	// Open, web lists its stack as down rows, in the order it was
	// written down; a check, a name taken off and another project's
	// are not among them. The cursor is on the first, and the
	// workspace gives its name, "down" and its command.
	open("web")
	lines := wait("api, worker and css as down rows", func(panel []string, _ string) bool {
		return downAt(panel, "api") >= 0 && downAt(panel, "worker") >= 0 && downAt(panel, "css") >= 0
	})
	panel, space := halves(lines)
	if a, w, c := downAt(panel, "api"), downAt(panel, "worker"), downAt(panel, "css"); !(a < w && w < c) {
		t.Errorf("the down rows stand api at %d, worker at %d, css at %d; want them in the order written down", a, w, c)
	}
	for _, other := range []string{"unit", "old", "db"} {
		if at(panel, other) >= 0 {
			t.Errorf("web lists %s, which is no stack process of web's:\n%s", other, strings.Join(lines, "\n"))
		}
	}
	for _, want := range []string{`\bapi\b`, `\bdown\b`, `sleep 1001`} {
		if !regexp.MustCompile(want).MatchString(space) {
			t.Errorf("the workspace on the down row api does not say %s:\n%s", want, strings.Join(lines, "\n"))
		}
	}
	raw := screen(true)
	for _, name := range []string{"worker", "css"} {
		if i := downAt(panel, name); i < 0 || i >= len(raw) || !dimmed(raw[i], name, "down") {
			t.Errorf("the down row %s is not drawn dim:\n%s", name, strings.Join(raw, "\n"))
		}
	}
	frame("down-120x36.ans")

	// Running, css is first, and the down rows come after it; web
	// says what runs and what is down.
	run(web, "u", "css", "--", "sleep 1003")
	lines = wait("css running and api and worker down", func(panel []string, _ string) bool {
		return runsAt(panel, "css") >= 0 && downAt(panel, "api") >= 0 && downAt(panel, "worker") >= 0
	})
	keys("g")
	panel, _ = halves(lines)
	if c, a, w := runsAt(panel, "css"), downAt(panel, "api"), downAt(panel, "worker"); !(c < a && a < w) {
		t.Errorf("css runs at %d, api is down at %d, worker at %d; want what runs first, the down rows after it", c, a, w)
	}
	wait("the cursor on css", func(_ []string, space string) bool { return !strings.Contains(space, "sleep 1001") })
	raw = screen(true)
	panel, _ = halves(raw)
	frame("mixed-120x36.ans")
	if i := runsAt(panel, "css"); i < 0 || dimmed(raw[i], "css") {
		t.Errorf("the row css, which runs, is drawn dim: %q", raw[i])
	}
	keys("-")
	wait("web saying 1 running · 2 down", onProjects("1 running · 2 down", "1 down"))
	frame("projects-mixed-120x36.ans")

	// xx on a down row forgets it and stops nothing.
	open("web")
	keys("j")
	wait("the workspace on api", func(_ []string, space string) bool {
		return regexp.MustCompile(`\bapi\b`).MatchString(space) && strings.Contains(space, "sleep 1001")
	})
	keys("x", "x")
	lines = wait("api gone", func(panel []string, _ string) bool { return at(panel, "api") < 0 && downAt(panel, "worker") >= 0 })
	if panel, _ = halves(lines); runsAt(panel, "css") < 0 {
		t.Errorf("xx on api stopped css:\n%s", strings.Join(lines, "\n"))
	}
	keys("-")
	wait("web saying 1 running · 1 down", onProjects("1 running · 1 down", "1 down"))

	// ss lists what is down and no longer what was forgotten; enter
	// starts it, and web says nothing is down.
	open("web")
	keys("s", "s")
	lines = wait("ss listing worker", func(_ []string, space string) bool {
		return strings.Contains(space, "Start these in web?") && regexp.MustCompile(`worker\s+sleep 1002`).MatchString(space)
	})
	if _, space := halves(lines); regexp.MustCompile(`\bapi\b`).MatchString(space) {
		t.Errorf("ss lists api, which xx forgot:\n%s", strings.Join(lines, "\n"))
	}
	keys("Enter")
	wait("worker started", func(_ []string, space string) bool { return strings.Contains(space, "worker is up") })
	keys("Escape")
	lines = wait("worker running", func(panel []string, _ string) bool { return runsAt(panel, "worker") >= 0 })
	if panel, _ = halves(lines); strings.Contains(strings.Join(panel, "\n"), "down") {
		t.Errorf("web still has something down after enter:\n%s", strings.Join(lines, "\n"))
	}
	keys("-")
	wait("web saying 2 running and shop 1 down", onProjects("2 running", "1 down"))

	// conn down puts them all down again, the forgotten one not
	// among them.
	run(web, "down")
	tm("kill-server")
	start()
	wait("web saying 2 down after conn down", onProjects("2 down", "1 down"))
	open("web")
	lines = wait("css and worker down", func(panel []string, _ string) bool {
		return downAt(panel, "css") >= 0 && downAt(panel, "worker") >= 0
	})
	if panel, _ = halves(lines); at(panel, "api") >= 0 {
		t.Errorf("after conn down web lists api, which was forgotten:\n%s", strings.Join(lines, "\n"))
	}
}
