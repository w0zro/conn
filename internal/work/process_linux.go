package work

import (
	"os"
)

// ReadProcesses reads the process table off /proc.
func ReadProcesses(uid int) ([]Process, error) {
	stat, err := os.ReadFile("/proc/stat")
	if err != nil {
		return nil, err
	}
	return ReadProcTree("/proc", ParseBootTime(string(stat)), 100), nil
}
