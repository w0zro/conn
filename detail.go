package main

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// What conn knows of a row beyond its name: how a plan's entry stands, how
// long ago a moment was, what git and ps say when asked. The buffer's
// heading and the finder's rows read these; the transcript itself is the
// buffer, and needs no reading.

// detailKey identifies the subject of a row, so the cursor can keep its
// subject across a rescan and a fold can be remembered by it.
func detailKey(r navRow) string {
	switch r.kind {
	case rowProc:
		return "proc:" + strconv.Itoa(r.node.PID)
	case rowRest:
		return "rest:" + r.rest.ID
	}
	return "repo:" + r.project.Path
}

// transcriptLines is how much of a held shell's transcript is read when
// something in it is looked for: the last file:line a run named.
const transcriptLines = 60

// entryState is what a plan entry — or the test run, or a row's own
// shell — is doing: "up" while its command runs, the exit status once it
// ended, and empty for one with no shell. At is when it ended, when the
// pane recorded that; zero otherwise.
type entryState struct {
	State   string
	At      time.Time
	Summary string   // what the run's transcript said of it, when conn read a shape it knows
	Ports   []string // what an entry that is up is listening on: up, and reachable
}

// joinWords is the words that are there, comma-separated.
func joinWords(words ...string) string {
	var out []string
	for _, w := range words {
		if w != "" {
			out = append(out, w)
		}
	}
	return strings.Join(out, ", ")
}

// ago says how long ago a moment was, or nothing for a moment unknown.
func ago(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	if age := shortAge(at); age != "now" {
		return age + " ago"
	}
	return "just now"
}

// runsShown is how many past runs a heading's strip lists.
const runsShown = 5

// runPorts is everything the processes a row stands for are listening on,
// as the scan found them.
func runPorts(run []*ProcNode, n *ProcNode) []string {
	nodes := run
	if len(nodes) == 0 {
		nodes = []*ProcNode{n}
	}

	seen := map[string]bool{}
	var ports []string
	for _, node := range nodes {
		for _, p := range node.Ports {
			if !seen[p] {
				seen[p] = true
				ports = append(ports, p)
			}
		}
	}
	sortPorts(ports)
	return ports
}

// countTree counts a node and everything beneath it.
func countTree(n *ProcNode) int {
	total := 1
	for _, c := range n.Children {
		total += countTree(c)
	}
	return total
}

// git runs a git command, reporting what git printed rather than just an exit
// status: "not a git repository" is worth showing, "exit status 128" is not.
//
// Without optional locks: these run in the background on a cadence, and a
// status that took the index lock would collide with the git the user is
// running in the shell beside it. Bounded like the scans, because a git that
// hangs on a large or unhealthy checkout would otherwise pile up behind the
// refresh the same way a hung lsof did.
func git(dir string, args ...string) (string, error) {
	out, err := listing(scanTimeout, "git",
		append([]string{"--no-optional-locks", "-C", dir}, args...)...)
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			if msg := firstLine(string(ee.Stderr)); msg != "" {
				return "", errors.New(strings.TrimPrefix(msg, "fatal: "))
			}
		}
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// firstLine returns the first non-empty line of s.
func firstLine(s string) string {
	for ln := range strings.SplitSeq(s, "\n") {
		if ln = strings.TrimSpace(ln); ln != "" {
			return ln
		}
	}
	return ""
}

func ps(pid int, format string) (string, error) {
	out, err := listing(scanTimeout, "ps", "-p", strconv.Itoa(pid), "-o", format)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}
