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
