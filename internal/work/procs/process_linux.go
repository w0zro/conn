package procs

import (
	"os"

	"github.com/w0zro/conn/internal/work"
)

// Read reads the process table off /proc.
func Read(uid int) ([]work.Process, error) {
	stat, err := os.ReadFile("/proc/stat")
	if err != nil {
		return nil, err
	}
	return readProcTree("/proc", parseBootTime(string(stat)), 100), nil
}
