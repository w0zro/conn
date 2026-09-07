package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"
)

// A run is one ending of a named shell — a plan's entry, a task — kept
// after the shell is gone: how the last five test runs went and how long
// each took is what the pane says beside the tasks, and a run that ended
// an hour ago has no shell to read it from.

// run is one ending: what it was called, where, how it ended, what its
// transcript said of it, when, and how long it took.
type run struct {
	Dir     string    `json:"dir"`
	Name    string    `json:"name"`
	Exit    string    `json:"exit"`
	Summary string    `json:"summary,omitempty"`
	At      time.Time `json:"at"`
	Took    float64   `json:"took,omitempty"` // seconds; 0 when the start was not known
}

// runsPath is where the runs are kept: one JSON line each, beside the
// server's socket.
func runsPath() string {
	return filepath.Join(filepath.Dir(socketPath()), "runs.jsonl")
}

// runsCap is how long the file is let grow before it is cut back to
// runsKeep: appending is the cheap way to record, and a file read whole
// for every pane has to stay small.
const (
	runsCap  = 4000
	runsKeep = 2000
)

// recordRun appends an ending to the file, making the directory on the
// way, and cuts the file back once it is long.
func recordRun(r run) error {
	path := runsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	line, err := json.Marshal(r)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return trimRuns(path)
}

// trimRuns rewrites the file with its last runsKeep lines when it has
// passed runsCap.
func trimRuns(path string) error {
	lines, err := readLines(path)
	if err != nil || len(lines) <= runsCap {
		return err
	}
	lines = lines[len(lines)-runsKeep:]
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, bytes.Join(append(lines, nil), []byte{'\n'}), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// readLines is the file's lines, or nothing for a file not there.
func readLines(path string) ([][]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var lines [][]byte
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		lines = append(lines, append([]byte{}, sc.Bytes()...))
	}
	return lines, sc.Err()
}

// pastRuns is the last n endings of a name at a place, newest first. A
// line that does not parse is skipped: the file is conn's own, and a
// half-written last line is the only way one gets in.
func pastRuns(dir, name string, n int) []run {
	lines, err := readLines(runsPath())
	if err != nil {
		return nil
	}
	var out []run
	for i := len(lines) - 1; i >= 0 && len(out) < n; i-- {
		var r run
		if json.Unmarshal(lines[i], &r) != nil || r.Dir != dir || r.Name != name {
			continue
		}
		out = append(out, r)
	}
	return out
}

// describeRuns is the runs in a line, newest first: each its mark and how
// long it took, ✓ 12s · ✗ 2m.
func describeRuns(runs []run) string {
	parts := make([]string, 0, len(runs))
	for _, r := range runs {
		mark := glyphDone
		if r.Exit != "0" {
			mark = glyphFailed
		}
		if r.Took > 0 {
			mark += " " + shortTook(time.Duration(r.Took*float64(time.Second)))
		}
		parts = append(parts, mark)
	}
	return joinDots(parts)
}

// joinDots is the parts with a middle dot between them.
func joinDots(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " · "
		}
		out += p
	}
	return out
}

// shortTook is a duration in the fewest characters that still say it:
// 12s, 2m, 1h.
func shortTook(d time.Duration) string {
	switch {
	case d < time.Second:
		return "<1s"
	case d < time.Minute:
		return strconv.Itoa(int(d.Seconds())) + "s"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m"
	default:
		return strconv.Itoa(int(d.Hours())) + "h"
	}
}

// startLayout is how ps prints lstart, which the scan carries as an opaque
// token and a run's length needs as a moment.
const startLayout = "Mon Jan 2 15:04:05 2006"

// parseStart is the moment a process began, from the token the scan
// carries, or zero when it cannot be read.
func parseStart(started string) time.Time {
	t, err := time.ParseInLocation(startLayout, started, time.Local)
	if err != nil {
		return time.Time{}
	}
	return t
}

// startedAt is when the listed process began, or zero for one not listed.
func (m model) startedAt(pid int) time.Time {
	i := slices.IndexFunc(m.procs, func(p Proc) bool { return p.PID == pid })
	if i < 0 {
		return time.Time{}
	}
	return parseStart(m.procs[i].Started)
}
