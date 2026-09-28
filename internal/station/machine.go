package station

import (
	"context"
	"os/exec"
	"syscall"
	"time"
)

// A Machine is what the kernel and the firmware say of the hardware and
// the system on it, as read and not as worded: names, bytes, counts and
// times. A zero field is one the platform could not read, and the
// readout leaves its line off. The platform files fill it; the parsers
// they share are in parse.go, so they can be tested off the platform.
type Machine struct {
	System      string // the system as it names itself: macOS 26.6.2, Ubuntu 24.04 LTS
	SystemBuild string // the system's build, where it has one: 25G83
	Virtual     string // container or wsl, when the system is one
	Kernel      string // the kernel's release: Darwin 25.6.0, Linux 6.8.0
	Model       string // the hardware's model
	Processor   string // the processor's name for itself
	CPUs        int    // logical processors
	PerfCores   int    // performance cores, on a machine that sorts them
	EffCores    int    // efficiency cores, likewise
	Rosetta     bool   // running translated, on Apple silicon
	Memory      uint64 // bytes
	Available   int    // percent of memory free for new work; -1 unread
	Pressure    string // the kernel's own word for how memory stands; blank where it publishes none
	SwapTotal   uint64
	SwapUsed    uint64
	SwapEncrypt bool
	Booted      time.Time
	Load        [3]float64
	LoadRead    bool // the load was read; it can honestly be zero
	Processes   int  // how many processes the machine holds, of any state
	Power       Power
	SIP         string // enabled or disabled, where the system has it
	Page        int    // the kernel's page, in bytes
}

// The kernel's own word for how memory stands, where it publishes one.
// conn reports the verdict rather than judging the numbers itself: the
// kernel is the one that acts on it, the way a contact is asked how it
// stands rather than measured. A level conn has never heard of is not
// guessed at, and leaves the field blank.
const (
	PressureNormal   = "normal"
	PressureWarning  = "warning"
	PressureCritical = "critical"
)

// Power is what the machine runs on and how the battery stands.
type Power struct {
	Source    string // battery or ac; blank unread
	Percent   int    // the battery's charge; -1 without one
	State     string // discharging, charging, charged, full, as the platform says
	Remaining string // H:MM to empty or to full, when the platform estimates
}

// A Volume is the file system under a path and its room.
type Volume struct {
	FS          string // the file system's name
	Free, Total uint64 // bytes
}

// readVolume is the volume under a path; the platform names its file
// system.
func readVolume(path string) Volume {
	var st syscall.Statfs_t
	if path == "" || syscall.Statfs(path, &st) != nil {
		return Volume{}
	}
	return Volume{
		FS:    volumeType(path),
		Free:  st.Bavail * uint64(st.Bsize),
		Total: st.Blocks * uint64(st.Bsize),
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

// LookPath is where a program is on PATH, or nothing.
func LookPath(name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return path
}

// A Tool is a program a platform needs past the kernel, and where it is.
type Tool struct {
	Name, Path string
}
