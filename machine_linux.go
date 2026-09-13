package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// readMachine reads uname and sysinfo, and the files under /etc, /proc
// and /sys. What cannot be read leaves its field zero.
func readMachine() machine {
	m := machine{available: -1, power: power{percent: -1}, page: os.Getpagesize()}
	var u syscall.Utsname
	if err := syscall.Uname(&u); err == nil {
		m.kernel = "Linux " + cstring(u.Release[:])
	}
	var si syscall.Sysinfo_t
	if err := syscall.Sysinfo(&si); err == nil {
		unit := uint64(si.Unit)
		m.memory = si.Totalram * unit
		m.swapTotal = si.Totalswap * unit
		m.swapUsed = (si.Totalswap - si.Freeswap) * unit
		m.booted = time.Now().Add(-time.Duration(si.Uptime) * time.Second)
		for i := range m.load {
			m.load[i] = float64(si.Loads[i]) / 65536
		}
		m.loadRead = true
		m.processes = int(si.Procs)
	}
	readLinuxFiles("/", &m)
	return m
}

// cstring is the string in a NUL-terminated array.
func cstring(a []int8) string {
	var b strings.Builder
	for _, c := range a {
		if c == 0 {
			break
		}
		b.WriteByte(byte(c))
	}
	return b.String()
}

// volumeType is the file system holding a path, by the magic the kernel
// reports for it.
func volumeType(path string) string {
	var st unix.Statfs_t
	if unix.Statfs(path, &st) != nil {
		return ""
	}
	switch uint64(st.Type) {
	case 0xEF53:
		return "ext4"
	case 0x9123683E:
		return "btrfs"
	case 0x58465342:
		return "xfs"
	case 0x2FC12FC1:
		return "zfs"
	case 0xF2F52010:
		return "f2fs"
	case 0x01021994:
		return "tmpfs"
	case 0x794C7630:
		return "overlayfs"
	case 0x6969:
		return "nfs"
	case 0x65735546:
		return "fuse"
	case 0xF15F:
		return "ecryptfs"
	default:
		return fmt.Sprintf("fs 0x%X", st.Type)
	}
}

// defaultRoute is the interface the machine reaches everything else
// through, and whether the table could be read at all. The kernel
// keeps it in a file, so there is no process to run.
func defaultRoute() (string, bool) {
	out, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return "", false
	}
	return parseProcNetRoute(string(out)), true
}
