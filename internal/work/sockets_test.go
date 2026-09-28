package work

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A row that was listening and is not, while its process lives, says
// CLOSED: the port it had been saying does not simply go off the row.
// One reading is not enough — a listing that did not come back would
// stamp every server on the machine — and the word stands for as long
// as the process does without a listener.
func TestAListenerGoneWhileTheProcessLives(t *testing.T) {
	began := time.Now().Add(-time.Hour)
	serving := []Project{{Path: "/w/a", Entries: []Entry{
		{PID: 300, Kind: KindRun, Command: "node vite", Started: began, Status: StatusActive, Ports: []string{"5173"}},
	}}}
	was := MarkClosed(serving, nil)
	if got := was[300]; !got.Started.Equal(began) || got.Lost != 0 {
		t.Fatalf("a row that serves is held as %+v", got)
	}
	closed := func() []Project {
		return []Project{{Path: "/w/a", Entries: []Entry{
			{PID: 300, Kind: KindRun, Command: "node vite", Started: began, Status: StatusActive},
		}}}
	}
	one := closed()
	was = MarkClosed(one, was)
	if e := one[0].Entries[0]; e.Status != StatusActive || e.Fault {
		t.Errorf("one reading without the port says %q", e.Status)
	}
	two := closed()
	was = MarkClosed(two, was)
	if e := two[0].Entries[0]; e.Status != StatusClosed || !e.Fault {
		t.Errorf("two readings without the port say %q, fault %v", e.Status, e.Fault)
	}
	three := closed()
	was = MarkClosed(three, was)
	if e := three[0].Entries[0]; e.Status != StatusClosed {
		t.Errorf("the word does not hold: %q", e.Status)
	}
	// And a listener bound again is a row at work: the word goes, and
	// the count it was stamped on starts over.
	back := serving
	was = MarkClosed(back, was)
	if e := back[0].Entries[0]; e.Status != StatusActive || e.Fault {
		t.Errorf("a port bound again says %q", e.Status)
	}
	again := closed()
	MarkClosed(again, was)
	if e := again[0].Entries[0]; e.Status != StatusActive {
		t.Errorf("the first reading after the port came back says %q", e.Status)
	}
}

// lsof -nP -a -i -F pcnPT, as captured: a socket a descriptor, the
// state on a line of its own, and the same connection on two
// descriptors listed once.
func TestInternetSocketsAreParsed(t *testing.T) {
	out := "p673\ncidentityservicesd\nf7\nPUDP\nn*:*\nf33\nPTCP\nn[fe80::1]:1024->[fe80::2]:1024\nTST=ESTABLISHED\nTQR=0\nTQS=0\nf43\nPTCP\nn[fe80::1]:1024->[fe80::2]:1024\nTST=ESTABLISHED\n" +
		"p8100\ncnode\nf21\nPTCP\nn*:5173\nTST=LISTEN\nf22\nPTCP\nn127.0.0.1:5173->127.0.0.1:60322\nTST=ESTABLISHED\nf30\nPUDP\nn*:5353\n"
	got := ParseSockets(out)
	if len(got[673]) != 2 || got[673][0].Proto != "UDP" || got[673][1].State != "ESTABLISHED" {
		t.Errorf("673 holds %+v", got[673])
	}
	node := got[8100]
	if len(node) != 3 || node[0] != (Socket{"TCP", "*:5173", "LISTEN"}) || !node[0].Listening() || node[1].Listening() || !node[2].Listening() {
		t.Errorf("node holds %+v", node)
	}
	if got[673][0].Listening() {
		t.Error("a UDP socket bound nowhere is called listening")
	}
	if s := (Socket{"TCP", "*:80", "CLOSE_WAIT"}).String(); s != "TCP *:80 · close_wait" {
		t.Errorf("a socket says %q", s)
	}
	if len(ParseSockets("")) != 0 {
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
	got := FdSockets(dir, ProcSocketTables(root))
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
	if HexAddr("00000000:0000") != "*:*" || HexAddr("garbage") != "garbage" {
		t.Errorf("hexAddr: %q %q", HexAddr("00000000:0000"), HexAddr("garbage"))
	}
}

// lsof -nP -a -U -F pcn: the unix sockets with a path, once each; one
// without a path is a pair of ends nobody else can reach.
func TestUnixSocketsAreParsed(t *testing.T) {
	out := "p700\ncdocker\nf5\nn/Users/w0zro/.docker/run/docker.sock\nf6\nn/Users/w0zro/.docker/run/docker.sock\nf7\nn->0x9f2c\nf8\nn/tmp/core.sock\n"
	got := ParseUnixSockets(out)
	if len(got[700]) != 2 || got[700][0].Addr != "/Users/w0zro/.docker/run/docker.sock" || got[700][1].Addr != "/tmp/core.sock" {
		t.Errorf("700 holds %+v", got[700])
	}
}

// What conn does not watch this way: a contact, which is filed by what
// it asks of you and never by what it has open; a row that is over,
// which serves nothing; a row conn only reports, with no pid of its
// own; a pid come round again on another process; and a row that is
// already a fault or waiting, which has the word worth reading.
func TestWhatTheClosedWordLeavesAlone(t *testing.T) {
	began := time.Now().Add(-time.Hour)
	since := began.Add(time.Minute)
	served := map[int]ServingSeen{
		301: {Started: began, Lost: ClosedAfter},
		302: {Started: began, Lost: ClosedAfter},
		303: {Started: began, Lost: ClosedAfter},
		304: {Started: began, Lost: ClosedAfter},
		305: {Started: since, Lost: ClosedAfter},
		306: {Started: began, Lost: ClosedAfter},
	}
	projects := []Project{{Path: "/w/a", Entries: []Entry{
		{PID: 301, Kind: KindContact, Command: "claude", Started: began, Status: StatusWorking},
		{PID: 302, Kind: KindRun, Command: "node vite", Started: began, Status: StatusEnded, Fault: true},
		{PID: 0, Kind: KindService, Command: "web", Container: "abc", Status: StatusActive},
		{PID: 304, Kind: KindRun, Command: "node vite", Started: began, Status: StatusStopped, Fault: true},
		{PID: 305, Kind: KindRun, Command: "node vite", Started: began, Status: StatusActive},
		{PID: 306, Kind: KindContact, Command: "claude", Started: began, Status: StatusWaiting},
	}}}
	MarkClosed(projects, served)
	for _, e := range projects[0].Entries {
		if e.Status == StatusClosed {
			t.Errorf("%d says CLOSED", e.PID)
		}
	}
}
