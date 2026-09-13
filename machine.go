package main

import (
	"context"
	"os/exec"
	"syscall"
	"time"
)

// A machine is what the kernel and the firmware say of the hardware and
// the system on it, as read and not as worded: names, bytes, counts and
// times. A zero field is one the platform could not read, and the
// readout leaves its line off. The platform files fill it; the parsers
// they share are in parse.go, so they can be tested off the platform.
type machine struct {
	system      string // the system as it names itself: macOS 26.6.2, Ubuntu 24.04 LTS
	systemBuild string // the system's build, where it has one: 25G83
	virtual     string // container or wsl, when the system is one
	kernel      string // the kernel's release: Darwin 25.6.0, Linux 6.8.0
	model       string // the hardware's model
	processor   string // the processor's name for itself
	cpus        int    // logical processors
	perfCores   int    // performance cores, on a machine that sorts them
	effCores    int    // efficiency cores, likewise
	rosetta     bool   // running translated, on Apple silicon
	memory      uint64 // bytes
	available   int    // percent of memory free for new work; -1 unread
	pressure    string // the kernel's own word for how memory stands; blank where it publishes none
	swapTotal   uint64
	swapUsed    uint64
	swapEncrypt bool
	booted      time.Time
	load        [3]float64
	processes   int // how many processes the machine holds, of any state
	power       power
	sip         string // enabled or disabled, where the system has it
	page        int    // the kernel's page, in bytes
}

// The kernel's own word for how memory stands, where it publishes one.
// conn reports the verdict rather than judging the numbers itself: the
// kernel is the one that acts on it, the way a contact is asked how it
// stands rather than measured. A level conn has never heard of is not
// guessed at, and leaves the field blank.
const (
	pressureNormal   = "normal"
	pressureWarning  = "warning"
	pressureCritical = "critical"
)

// power is what the machine runs on and how the battery stands.
type power struct {
	source    string // battery or ac; blank unread
	percent   int    // the battery's charge; -1 without one
	state     string // discharging, charging, charged, full, as the platform says
	remaining string // H:MM to empty or to full, when the platform estimates
}

// A volume is the file system under a path and its room.
type volume struct {
	fs          string // the file system's name
	free, total uint64 // bytes
}

// readVolume is the volume under a path; the platform names its file
// system.
func readVolume(path string) volume {
	var st syscall.Statfs_t
	if path == "" || syscall.Statfs(path, &st) != nil {
		return volume{}
	}
	return volume{
		fs:    volumeType(path),
		free:  st.Bavail * uint64(st.Bsize),
		total: st.Blocks * uint64(st.Bsize),
	}
}

// commandTimeout is the moment a program is given to answer; the ones
// conn asks answer from disk.
const commandTimeout = 2 * time.Second

// run is what a program prints, given a moment; missing or mute, it is
// the empty string.
func run(name string, args ...string) string {
	if _, err := exec.LookPath(name); err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// lookPath is where a program is on PATH, or nothing.
func lookPath(name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return path
}

// A tool is a program a platform needs past the kernel, and where it is.
type tool struct {
	name, path string
}
