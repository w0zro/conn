//go:build linux

package main

// On Linux the process list is read off /proc (procfs.go): every process
// the user may read, with its directory, its command line, its group, its
// state, its start time and its listening ports, and no lsof or ps to
// have installed.

// procsBut is runningProcs for a conn of the given pid: neither that
// process nor its children are work happening in a repository.
func procsBut(self int) ([]Proc, error) {
	procs, err := procfsScan("/proc", self)
	if err != nil {
		return nil, err
	}
	// The containers docker runs for a place, filed under the compose
	// that runs them where one is in the list.
	return attachContainers(procs, containers()), nil
}

// startedOf is when each of the given processes began, asked freshly of
// /proc, in the scan's own terms.
func startedOf(pids []int) map[int]string {
	return procfsStarted("/proc", pids)
}
