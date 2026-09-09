package main

import (
	"encoding/binary"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// machine is what the kernel says of the hardware and itself.
type machine struct {
	system, kernel, model, processor string
	memory                           uint64
	booted                           time.Time
	load                             [3]float64
}

// readMachine asks sysctl, one name at a time; a name it will not answer
// leaves its field blank and the line off the report.
func readMachine() machine {
	var m machine
	if v, err := unix.Sysctl("kern.osproductversion"); err == nil {
		m.system = "MACOS " + v
	}
	if v, err := unix.Sysctl("kern.osrelease"); err == nil {
		m.kernel = "DARWIN " + v
	}
	m.model, _ = unix.Sysctl("hw.model")
	m.processor, _ = unix.Sysctl("machdep.cpu.brand_string")
	m.memory, _ = unix.SysctlUint64("hw.memsize")
	if tv, err := unix.SysctlTimeval("kern.boottime"); err == nil {
		m.booted = time.Unix(tv.Sec, 0)
	}
	// vm.loadavg is three fixed-point words and the scale they are in.
	if raw, err := unix.SysctlRaw("vm.loadavg"); err == nil && len(raw) >= 24 {
		scale := float64(binary.LittleEndian.Uint64(raw[16:24]))
		if scale > 0 {
			for i := range m.load {
				m.load[i] = float64(binary.LittleEndian.Uint32(raw[i*4:i*4+4])) / scale
			}
		}
	}
	m.processor = strings.TrimSpace(m.processor)
	return m
}
