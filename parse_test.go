package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The pmset parser, against captures of each form pmset takes.
func TestPmsetIsParsed(t *testing.T) {
	for _, c := range []struct {
		file string
		want power
	}{
		{"battery.txt", power{source: "battery", percent: 73, state: "discharging", remaining: "8:53"}},
		{"charged.txt", power{source: "ac", percent: 100, state: "charged"}},
		{"charging.txt", power{source: "ac", percent: 41, state: "charging", remaining: "1:12"}},
		{"charging-no-estimate.txt", power{source: "ac", percent: 41, state: "charging"}},
		{"desktop.txt", power{source: "ac", percent: -1}},
		{"low.txt", power{source: "battery", percent: 7, state: "discharging", remaining: "0:31"}},
	} {
		out, err := os.ReadFile(filepath.Join("testdata", "pmset", c.file))
		if err != nil {
			t.Fatal(err)
		}
		if got := parsePmset(string(out)); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.file, got, c.want)
		}
	}
	if got := parsePmset(""); got != (power{percent: -1}) {
		t.Errorf("no output: %+v", got)
	}
}

// The sysctl structs, laid out as the kernel lays them.
func TestSysctlStructsAreParsed(t *testing.T) {
	swap := make([]byte, 32)
	binary.LittleEndian.PutUint64(swap[0:], 5<<30)
	binary.LittleEndian.PutUint64(swap[8:], 1<<30)
	binary.LittleEndian.PutUint64(swap[16:], 4<<30)
	binary.LittleEndian.PutUint32(swap[24:], 16384)
	binary.LittleEndian.PutUint32(swap[28:], 1)
	total, used, encrypted, ok := parseSwapUsage(swap)
	if !ok || total != 5<<30 || used != 4<<30 || !encrypted {
		t.Errorf("swap: %d %d %v %v", total, used, encrypted, ok)
	}
	if _, _, _, ok := parseSwapUsage(swap[:31]); ok {
		t.Error("a short swapusage parsed")
	}
	load := make([]byte, 24)
	binary.LittleEndian.PutUint32(load[0:], 2048*250/100)
	binary.LittleEndian.PutUint32(load[4:], 2048*200/100)
	binary.LittleEndian.PutUint32(load[8:], 2048*3)
	binary.LittleEndian.PutUint64(load[16:], 2048)
	if got, ok := parseLoadavg(load); !ok || got != [3]float64{2.5, 2, 3} {
		t.Errorf("load: %v %v", got, ok)
	}
	binary.LittleEndian.PutUint64(load[16:], 0)
	if _, ok := parseLoadavg(load); ok {
		t.Error("a loadavg with no scale parsed")
	}
}

// What Linux keeps on file, read off a fixture tree: an EC2 instance
// that, for the test, also has a battery.
func TestLinuxFilesAreRead(t *testing.T) {
	m := machine{available: -1}
	readLinuxFiles("testdata/linux", &m)
	want := machine{
		system:    "Ubuntu 24.04.2 LTS",
		model:     "Amazon EC2 m6i.xlarge",
		processor: "Intel(R) Xeon(R) Platinum 8375C CPU @ 2.90GHz",
		cpus:      4,
		available: 75,
		power:     power{source: "battery", percent: 43, state: "discharging"},
	}
	if m != want {
		t.Errorf("read\n%+v\nwant\n%+v", m, want)
	}
	m = machine{available: -1}
	readLinuxFiles(t.TempDir(), &m)
	if m != (machine{available: -1, power: power{percent: -1}}) {
		t.Errorf("an empty tree read as %+v", m)
	}
}

// The parsers behind the Linux files, on the forms they meet.
func TestLinuxTextIsParsed(t *testing.T) {
	if got := parseOSRelease("NAME=Alpine\nPRETTY_NAME=\"Alpine Linux v3.20\"\n"); got != "Alpine Linux v3.20" {
		t.Errorf("os-release: %q", got)
	}
	if got := parseOSRelease("NAME=Alpine\n"); got != "" {
		t.Errorf("os-release without a pretty name: %q", got)
	}
	model, cpus := parseCPUInfo("processor\t: 0\nHardware\t: BCM2835\nprocessor\t: 1\n")
	if model != "BCM2835" || cpus != 2 {
		t.Errorf("arm cpuinfo: %q %d", model, cpus)
	}
	total, avail := parseMeminfo("MemTotal:       1000 kB\nMemAvailable:    250 kB\n")
	if total != 1024000 || avail != 256000 {
		t.Errorf("meminfo: %d %d", total, avail)
	}
	if p := readPowerSupply(filepath.Join(t.TempDir(), "none")); p != (power{percent: -1}) {
		t.Errorf("no power_supply: %+v", p)
	}
}

// vm_stat is how much of the machine's memory is free for new work on
// macOS, which is the one reading sysctl will not give whole.
func TestVMStatIsParsed(t *testing.T) {
	out, err := os.ReadFile(filepath.Join("testdata", "vm_stat.txt"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	free, ok := parseVMStat(text)
	if !ok {
		t.Fatal("a whole vm_stat was not read")
	}
	// The three classes of the capture, at its own page size.
	want := (73427 + 416068 + 25514) * uint64(16384)
	if free != want {
		t.Errorf("free memory: %d, want %d", free, want)
	}

	// A class missing is no reading at all, rather than a sum of what
	// was there: a machine that answered for two classes of three has
	// not said how much memory is free.
	short := strings.ReplaceAll(text, "Pages inactive", "Pages dormant")
	if _, ok := parseVMStat(short); ok {
		t.Error("vm_stat without an inactive count was read anyway")
	}
	if _, ok := parseVMStat("Pages free: 1.\nPages inactive: 2.\nPages speculative: 3.\n"); ok {
		t.Error("vm_stat with no page size was read anyway")
	}
	if _, ok := parseVMStat(""); ok {
		t.Error("nothing was read as a vm_stat")
	}
}

// csrutil, on each machine it answers for. A protection set piece by
// piece is the one that mattered: the word enabled is in its answer
// several times over, and none of those times is the status.
func TestCSRUtilIsParsed(t *testing.T) {
	for _, c := range []struct {
		name, text, want string
	}{
		{"on", "System Integrity Protection status: enabled.\n", "enabled"},
		{"off", "System Integrity Protection status: disabled.\n", "disabled"},
		{"custom", `System Integrity Protection status: unknown (Custom Configuration).

Configuration:
	Apple Internal: disabled
	Kext Signing: enabled
	Filesystem Protections: disabled
	Debugging Restrictions: enabled
	DTrace Restrictions: enabled
	NVRAM Protections: enabled
	BaseSystem Verification: enabled

This is an unsupported configuration, likely to break in the future and leave your machine in an unknown state.
`, "custom"},
		{"unanswered", "", ""},
		{"a word conn has never heard", "System Integrity Protection status: sideways.\n", ""},
	} {
		if got := parseCSRUtil(c.text); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

// The default route, as each kernel gives it up: the interface the
// machine reaches everything that is not local through.
func TestTheDefaultRouteIsParsed(t *testing.T) {
	darwin := `   route to: default
destination: default
       mask: default
    gateway: 172.20.10.1
  interface: en0
      flags: <UP,GATEWAY,DONE,STATIC,PRCLONING,GLOBAL>
`
	if got := parseRouteGet(darwin); got != "en0" {
		t.Errorf("route get: %q", got)
	}
	if got := parseRouteGet("route: writing to routing socket: not in table\n"); got != "" {
		t.Errorf("no route at all: %q", got)
	}

	linux := "Iface\tDestination\tGateway \tFlags\tRefCnt\tUse\tMetric\tMask\t\tMTU\tWindow\tIRTT\n" +
		"docker0\t000011AC\t00000000\t0001\t0\t0\t0\t0000FFFF\t0\t0\t0\n" +
		"eth0\t00000000\t0101A8C0\t0003\t0\t0\t100\t00000000\t0\t0\t0\n"
	if got := parseProcNetRoute(linux); got != "eth0" {
		t.Errorf("proc route: %q", got)
	}
	// A machine with links and nowhere to send what is not local.
	local := "Iface\tDestination\tGateway \tFlags\n" + "docker0\t000011AC\t00000000\t0001\n"
	if got := parseProcNetRoute(local); got != "" {
		t.Errorf("only a local route: %q", got)
	}
	if got := parseProcNetRoute(""); got != "" {
		t.Errorf("no table: %q", got)
	}
}
