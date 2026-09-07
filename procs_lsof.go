//go:build !linux

package main

import (
	"strconv"
	"strings"
)

// On macOS there is no /proc to walk, and lsof is the only way to read
// another process's working directory; ps says the rest. Processes owned
// by other users are reported as permission errors on stderr and simply do
// not appear, which is the behavior we want.

// procsBut is runningProcs for a conn of the given pid: neither that
// process nor its children are work happening in a repository.
func procsBut(self int) ([]Proc, error) {
	// One call asks for every process's working directory and every
	// listening TCP socket together — without -a the selections are
	// unioned — which is a few milliseconds over asking for the
	// directories alone, where a second call per row would be that much
	// again for every row drawn. -nP keeps the addresses numeric: a lookup
	// per socket is what makes lsof slow.
	out, err := listing(scanTimeout, "lsof", "-nP", "-d", "cwd", "-iTCP", "-sTCP:LISTEN", "-F", "pcRfn")
	if err != nil && len(out) == 0 {
		return nil, err
	}

	// What each process was run with and when it began, in one call. Asking
	// per process is milliseconds each, which is fine for the one row being
	// inspected and far too slow for a list being redrawn.
	procs, err := parseScan(out, self, psTable())
	if err != nil {
		return nil, err
	}
	return procs, nil
}

// startedOf is when each of the given processes began, asked freshly of
// ps, in the scan's own terms. nil when ps could not answer at all, which
// callers read as the check being unavailable rather than every process
// being gone.
func startedOf(pids []int) map[int]string {
	args := []string{"-o", "pid=,lstart="}
	for _, pid := range pids {
		args = append(args, "-p", strconv.Itoa(pid))
	}
	// ps exits nonzero when any asked-for pid is gone, while still listing
	// the rest; only nothing printed at all means it could not answer.
	out, err := listing(scanTimeout, "ps", args...)
	if err != nil && len(strings.TrimSpace(string(out))) == 0 {
		return nil
	}

	table := map[int]string{}
	for line := range strings.SplitSeq(string(out), "\n") {
		pid, rest := cutField(line)
		n, err := strconv.Atoi(pid)
		if err != nil {
			continue
		}
		table[n] = strings.Join(strings.Fields(rest), " ")
	}
	return table
}
