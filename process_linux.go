package main

import (
	"os"
)

// readProcesses reads the process table off /proc.
func readProcesses(uid int) ([]process, error) {
	stat, err := os.ReadFile("/proc/stat")
	if err != nil {
		return nil, err
	}
	return readProcTree("/proc", parseBootTime(string(stat)), 100), nil
}

// readTools is what conn needs on this platform past the kernel: tmux,
// to hold the work.
func readTools() []tool {
	return []tool{{name: "tmux", path: lookPath("tmux")}}
}
