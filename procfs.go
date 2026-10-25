package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Where lsof is not installed, Linux itself records what listens: the
// listening sockets in /proc/net/tcp and tcp6 by inode, and in each
// process's /proc/<pid>/fd the sockets it holds open.

// hasLsof says lsof is installed.
func hasLsof() bool {
	_, err := exec.LookPath("lsof")
	return err == nil
}

// listeningSockets is the port of each TCP socket in the listen state,
// by its inode.
func listeningSockets() map[string]string {
	m := map[string]string{}
	for _, table := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		data, _ := os.ReadFile(table)
		for _, l := range strings.Split(string(data), "\n") {
			// sl local_address rem_address st … inode
			f := strings.Fields(l)
			if len(f) < 10 || f[3] != "0A" {
				continue
			}
			i := strings.LastIndexByte(f[1], ':')
			if port, err := strconv.ParseUint(f[1][i+1:], 16, 16); i >= 0 && err == nil {
				m[f[9]] = strconv.FormatUint(port, 10)
			}
		}
	}
	return m
}

// procListening is the listening ports each process holds a socket for,
// and the ports of listening sockets no process conn can read holds.
func procListening() (held map[int][]string, unowned []string) {
	sockets := listeningSockets()
	held = map[int][]string{}
	owned := map[string]bool{}
	dirs, _ := filepath.Glob("/proc/[0-9]*/fd")
	for _, dir := range dirs {
		pid, _ := strconv.Atoi(strings.Split(dir, "/")[2])
		fds, _ := os.ReadDir(dir)
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(dir, fd.Name()))
			if err != nil || !strings.HasPrefix(link, "socket:[") {
				continue
			}
			inode := strings.TrimSuffix(strings.TrimPrefix(link, "socket:["), "]")
			if port, ok := sockets[inode]; ok {
				held[pid] = append(held[pid], port)
				owned[inode] = true
			}
		}
	}
	for inode, port := range sockets {
		if !owned[inode] {
			unowned = append(unowned, port)
		}
	}
	return held, unowned
}

// procDescribe is where each process runs and what it is, from /proc.
func procDescribe(pids []int) (cwd, comm map[int]string) {
	cwd, comm = map[int]string{}, map[int]string{}
	for _, pid := range pids {
		root := filepath.Join("/proc", strconv.Itoa(pid))
		if dir, err := os.Readlink(filepath.Join(root, "cwd")); err == nil {
			cwd[pid] = dir
		}
		if name, err := os.ReadFile(filepath.Join(root, "comm")); err == nil {
			comm[pid] = strings.TrimSpace(string(name))
		}
	}
	return cwd, comm
}

// unseenPorts is the ports something listens on that conn cannot
// attribute to a process, as one owned by another user cannot be read;
// with lsof there are none to name.
func unseenPorts() []string {
	if hasLsof() {
		return nil
	}
	_, unowned := procListening()
	return unowned
}
