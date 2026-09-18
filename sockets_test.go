package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// lsof -nP -a -i -F pcnPT, as captured: a socket a descriptor, the
// state on a line of its own, and the same connection on two
// descriptors listed once.
func TestInternetSocketsAreParsed(t *testing.T) {
	out := "p673\ncidentityservicesd\nf7\nPUDP\nn*:*\nf33\nPTCP\nn[fe80::1]:1024->[fe80::2]:1024\nTST=ESTABLISHED\nTQR=0\nTQS=0\nf43\nPTCP\nn[fe80::1]:1024->[fe80::2]:1024\nTST=ESTABLISHED\n" +
		"p8100\ncnode\nf21\nPTCP\nn*:5173\nTST=LISTEN\nf22\nPTCP\nn127.0.0.1:5173->127.0.0.1:60322\nTST=ESTABLISHED\nf30\nPUDP\nn*:5353\n"
	got := parseSockets(out)
	if len(got[673]) != 2 || got[673][0].proto != "UDP" || got[673][1].state != "ESTABLISHED" {
		t.Errorf("673 holds %+v", got[673])
	}
	node := got[8100]
	if len(node) != 3 || node[0] != (socket{"TCP", "*:5173", "LISTEN"}) || !node[0].listening() || node[1].listening() || !node[2].listening() {
		t.Errorf("node holds %+v", node)
	}
	if got[673][0].listening() {
		t.Error("a UDP socket bound nowhere is called listening")
	}
	if s := (socket{"TCP", "*:80", "CLOSE_WAIT"}).String(); s != "TCP *:80 · close_wait" {
		t.Errorf("a socket says %q", s)
	}
	if len(parseSockets("")) != 0 {
		t.Error("nothing parsed as something")
	}
}

// lsof -nP -a -U -F pcn: the unix sockets with a path, once each; one
// without a path is a pair of ends nobody else can reach.
func TestUnixSocketsAreParsed(t *testing.T) {
	out := "p700\ncdocker\nf5\nn/Users/w0zro/.docker/run/docker.sock\nf6\nn/Users/w0zro/.docker/run/docker.sock\nf7\nn->0x9f2c\nf8\nn/tmp/core.sock\n"
	got := parseUnixSockets(out)
	if len(got[700]) != 2 || got[700][0].addr != "/Users/w0zro/.docker/run/docker.sock" || got[700][1].addr != "/tmp/core.sock" {
		t.Errorf("700 holds %+v", got[700])
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
	if !got[0].listening() || got[1].listening() || !got[2].listening() {
		t.Errorf("listening is wrong: %+v", got)
	}
	if hexAddr("00000000:0000") != "*:*" || hexAddr("garbage") != "garbage" {
		t.Errorf("hexAddr: %q %q", hexAddr("00000000:0000"), hexAddr("garbage"))
	}
}

// The page says what a row has open: what it listens on first, then
// what it is connected to, then its unix sockets, each kind labelled
// once; a row with nothing open has no such group.
func TestThePageSaysWhatARowListensOn(t *testing.T) {
	// A run's page, which is the groups; a contact's is the sheet, and
	// a contact has nothing open to the world worth its page.
	s := readoutSubj()
	s.entry.kind, s.entry.command, s.entry.typed = kindRun, "node vite", ""
	s.entry.sockets = []socket{
		{"TCP", "127.0.0.1:5173->127.0.0.1:60322", "ESTABLISHED"},
		{"TCP", "*:5173", "LISTEN"},
		{"UDP", "*:*", ""},
		{"unix", "/tmp/dev.sock", ""},
	}
	text := texts(drawReadout(composeReadout(s, "/Users/w0zro", processesNow), 100, 60, plain))
	for _, want := range []string{"SOCKETS", "LISTENS ... TCP *:5173", "CONNECTED . TCP 127.0.0.1:5173->127.0.0.1:60322", "UNIX ...... /tmp/dev.sock"} {
		if !strings.Contains(text, want) {
			t.Errorf("the page lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "*:*") {
		t.Errorf("a UDP socket bound nowhere is on the page:\n%s", text)
	}
	if strings.Index(text, "LISTENS") > strings.Index(text, "CONNECTED") {
		t.Errorf("what listens is not said first:\n%s", text)
	}
	s.entry.sockets = nil
	if quiet := texts(drawReadout(composeReadout(s, "/Users/w0zro", processesNow), 100, 60, plain)); strings.Contains(quiet, "SOCKETS") {
		t.Errorf("a row with nothing open has a sockets group:\n%s", quiet)
	}
}
