package procs

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/w0zro/conn/internal/work"
)

// lsof -F pcn, as captured.
func TestLsofIsParsed(t *testing.T) {
	out, err := os.ReadFile("testdata/lsof.txt")
	if err != nil {
		t.Fatal(err)
	}
	got := parseLsof(string(out))
	if len(got) != 5 || got[67032].Command != "conn" || got[67032].Cwd != "/Users/w0zro/projects/w0zro/conn" || got[409].Cwd != "/" {
		t.Errorf("lsof: %+v", got)
	}
	if got := parseLsof(""); len(got) != 0 {
		t.Errorf("nothing parsed as %+v", got)
	}
}

// /proc/<pid>/stat, with a command that holds a space and a parenthesis,
// and a tree of them read off a directory that stands in for /proc.
func TestProcIsParsed(t *testing.T) {
	boot := time.Date(2026, 9, 4, 0, 47, 0, 0, time.UTC)
	line := "70301 (go (test)) R 70300 70300 70001 34823 70300 4194304 1 0 0 0 5 1 0 0 20 0 8 0 43200000 100 200 300"
	p, ok := parseProcStat(line, boot, 100)
	// utime and stime are fields 14 and 15 — 5 and 1 here — and at a
	// hundred ticks a second that is sixty milliseconds on a processor.
	want := work.Process{PID: 70301, Command: "go (test)", State: 'R', PPID: 70300, PGID: 70300, TTY: "pts/7", Foreground: true,
		Started: boot.Add(432000 * time.Second), CPU: 60 * time.Millisecond}
	if !ok || !reflect.DeepEqual(p, want) {
		t.Errorf("stat: %+v %v, want %+v", p, ok, want)
	}
	if _, ok := parseProcStat("garbage", boot, 100); ok {
		t.Error("garbage parsed")
	}
	for nr, name := range map[int]string{0: "", 34823: "pts/7", 34816: "pts/0", 35072: "pts/256", 1025: "tty1", 5 << 8: ""} {
		if got := linuxTTY(nr); got != name {
			t.Errorf("tty %d: %q, want %q", nr, got, name)
		}
	}
	if got := parseBootTime("cpu  1 2 3\nbtime " + strconv.FormatInt(boot.Unix(), 10) + "\nprocesses 5\n"); !got.Equal(boot) {
		t.Errorf("btime: %v", got)
	}

	root := t.TempDir()
	write := func(pid, name, content string) {
		dir := filepath.Join(root, pid)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("70301", "stat", line+"\n")
	write("70301", "cmdline", "go\x00test\x00./...\x00")
	if err := os.Symlink("/home/w0zro/conn", filepath.Join(root, "70301", "cwd")); err != nil {
		t.Fatal(err)
	}
	write("70302", "stat", "broken\n")
	write("notapid", "stat", line)
	procs := readProcTree(root, boot, 100)
	if len(procs) != 1 || procs[0].Cwd != "/home/w0zro/conn" || !reflect.DeepEqual(procs[0].Args, []string{"go", "test", "./..."}) || procs[0].UID != os.Getuid() {
		t.Errorf("proc tree: %+v", procs)
	}
}

// kern.procargs2, laid out as the kernel lays it.
func TestProcargsAreParsed(t *testing.T) {
	raw := make([]byte, 4)
	binary.LittleEndian.PutUint32(raw, 3)
	raw = append(raw, "/usr/local/bin/go\x00\x00\x00\x00go\x00test\x00./...\x00HOME=/Users/w0zro\x00"...)
	if got := parseProcargs(raw); !reflect.DeepEqual(got, []string{"go", "test", "./..."}) {
		t.Errorf("procargs: %q", got)
	}
	if got := parseProcargs(raw[:3]); got != nil {
		t.Errorf("a short procargs parsed as %q", got)
	}
}

// ps prints a processor time as minutes and seconds, the minutes
// running past sixty rather than becoming hours; hours and days show up
// on other systems, and all of them read.
func TestPsTimesAreParsed(t *testing.T) {
	for _, c := range []struct {
		in   string
		want time.Duration
		ok   bool
	}{
		{"0:00.00", 0, true},
		{"0:00.39", 390 * time.Millisecond, true},
		{"12:34.56", 12*time.Minute + 34*time.Second + 560*time.Millisecond, true},
		{"583:40.70", 583*time.Minute + 40*time.Second + 700*time.Millisecond, true},
		{"1:02:03", time.Hour + 2*time.Minute + 3*time.Second, true},
		{"2-01:00:00", 49 * time.Hour, true},
		{"nonsense", 0, false},
		{"1:2:3:4", 0, false},
	} {
		got, ok := parsePsTime(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("%q: %v %v, want %v %v", c.in, got, ok, c.want, c.ok)
		}
	}
	times := parsePsTimes("    1  63:35.26\n  333  26:21.76\n\ngarbage line here\n  334   0:00.39\n")
	if len(times) != 3 || times[1] != 63*time.Minute+35*time.Second+260*time.Millisecond || times[334] != 390*time.Millisecond {
		t.Errorf("a listing reads as %v", times)
	}
}

// lsof -nP -a -i -F pcnPT, as captured: a socket a descriptor, the
// state on a line of its own, and the same connection on two
// descriptors listed once.
func TestInternetSocketsAreParsed(t *testing.T) {
	out := "p673\ncidentityservicesd\nf7\nPUDP\nn*:*\nf33\nPTCP\nn[fe80::1]:1024->[fe80::2]:1024\nTST=ESTABLISHED\nTQR=0\nTQS=0\nf43\nPTCP\nn[fe80::1]:1024->[fe80::2]:1024\nTST=ESTABLISHED\n" +
		"p8100\ncnode\nf21\nPTCP\nn*:5173\nTST=LISTEN\nf22\nPTCP\nn127.0.0.1:5173->127.0.0.1:60322\nTST=ESTABLISHED\nf30\nPUDP\nn*:5353\n"
	got := parseSockets(out)
	if len(got[673]) != 2 || got[673][0].Proto != "UDP" || got[673][1].State != "ESTABLISHED" {
		t.Errorf("673 holds %+v", got[673])
	}
	node := got[8100]
	if len(node) != 3 || node[0] != (work.Socket{Proto: "TCP", Addr: "*:5173", State: "LISTEN"}) || !node[0].Listening() || node[1].Listening() || !node[2].Listening() {
		t.Errorf("node holds %+v", node)
	}
	if got[673][0].Listening() {
		t.Error("a UDP socket bound nowhere is called listening")
	}
	if s := (work.Socket{Proto: "TCP", Addr: "*:80", State: "CLOSE_WAIT"}).String(); s != "TCP *:80 · close_wait" {
		t.Errorf("a socket says %q", s)
	}
	if len(parseSockets("")) != 0 {
		t.Error("nothing parsed as something")
	}
}

// /proc/net/tcp and friends: addresses in hex, the host's byte order,
// the state a number, the inode the key; and /proc/net/unix, the path
// where there is one. A process's descriptors point at the inodes.
func TestProcSocketsAreRead(t *testing.T) {
	root := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(root, "net"), 0o755))
	must(os.WriteFile(filepath.Join(root, "net", "tcp"), []byte(
		"  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n"+
			"   0: 00000000:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000   501        0 12345 1 0000000000000000 100 0 0 10 0\n"+
			"   1: 0100007F:1F90 0100007F:EB92 01 00000000:00000000 00:00000000 00000000   501        0 12346 1 0000000000000000 20 4 30 10 -1\n"), 0o644))
	must(os.WriteFile(filepath.Join(root, "net", "tcp6"), []byte(
		"  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n"+
			"   0: 00000000000000000000000001000000:0BB8 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000   501        0 12347 1 0000000000000000 100 0 0 10 0\n"), 0o644))
	must(os.WriteFile(filepath.Join(root, "net", "unix"), []byte(
		"Num       RefCount Protocol Flags    Type St Inode Path\n"+
			"0000000000000000: 00000002 00000000 00010000 0001 01 12348 /run/user/501/app.sock\n"+
			"0000000000000000: 00000002 00000000 00000000 0001 03 12349\n"), 0o644))
	dir := filepath.Join(root, "42")
	must(os.MkdirAll(filepath.Join(dir, "fd"), 0o755))
	for fd, target := range map[string]string{"0": "/dev/pts/3", "3": "socket:[12345]", "4": "socket:[12346]", "5": "socket:[12347]", "6": "socket:[12348]", "7": "socket:[12345]", "8": "socket:[99999]"} {
		must(os.Symlink(target, filepath.Join(dir, "fd", fd)))
	}
	got := fdSockets(dir, procSocketTables(root))
	var lines []string
	for _, s := range got {
		lines = append(lines, s.String())
	}
	if want := "TCP *:8080 TCP 127.0.0.1:8080->127.0.0.1:60306 TCP [::1]:3000 unix /run/user/501/app.sock"; strings.Join(lines, " ") != want {
		t.Errorf("the process holds %q, want %q", strings.Join(lines, " "), want)
	}
	if !got[0].Listening() || got[1].Listening() || !got[2].Listening() {
		t.Errorf("listening is wrong: %+v", got)
	}
	if hexAddr("00000000:0000") != "*:*" || hexAddr("garbage") != "garbage" {
		t.Errorf("hexAddr: %q %q", hexAddr("00000000:0000"), hexAddr("garbage"))
	}
}

// lsof -nP -a -U -F pcn: the unix sockets with a path, once each; one
// without a path is a pair of ends nobody else can reach.
func TestUnixSocketsAreParsed(t *testing.T) {
	out := "p700\ncdocker\nf5\nn/Users/w0zro/.docker/run/docker.sock\nf6\nn/Users/w0zro/.docker/run/docker.sock\nf7\nn->0x9f2c\nf8\nn/tmp/core.sock\n"
	got := parseUnixSockets(out)
	if len(got[700]) != 2 || got[700][0].Addr != "/Users/w0zro/.docker/run/docker.sock" || got[700][1].Addr != "/tmp/core.sock" {
		t.Errorf("700 holds %+v", got[700])
	}
}
