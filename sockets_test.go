package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	for _, want := range []string{"SOCKETS", "Listens ... TCP *:5173", "Connected . TCP 127.0.0.1:5173->127.0.0.1:60322", "Unix ...... /tmp/dev.sock"} {
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

// A row that was listening and is not, while its process lives, says
// CLOSED: the port it had been saying does not simply go off the row.
// One reading is not enough — a listing that did not come back would
// stamp every server on the machine — and the word stands for as long
// as the process does without a listener.
func TestAListenerGoneWhileTheProcessLives(t *testing.T) {
	began := time.Now().Add(-time.Hour)
	serving := []project{{path: "/w/a", entries: []entry{
		{pid: 300, kind: kindRun, command: "node vite", started: began, status: statusActive, ports: []string{"5173"}},
	}}}
	was := markClosed(serving, nil)
	if got := was[300]; !got.started.Equal(began) || got.lost != 0 {
		t.Fatalf("a row that serves is held as %+v", got)
	}
	closed := func() []project {
		return []project{{path: "/w/a", entries: []entry{
			{pid: 300, kind: kindRun, command: "node vite", started: began, status: statusActive},
		}}}
	}
	one := closed()
	was = markClosed(one, was)
	if e := one[0].entries[0]; e.status != statusActive || e.fault {
		t.Errorf("one reading without the port says %q", e.status)
	}
	two := closed()
	was = markClosed(two, was)
	if e := two[0].entries[0]; e.status != statusClosed || !e.fault {
		t.Errorf("two readings without the port say %q, fault %v", e.status, e.fault)
	}
	three := closed()
	was = markClosed(three, was)
	if e := three[0].entries[0]; e.status != statusClosed {
		t.Errorf("the word does not hold: %q", e.status)
	}
	// And a listener bound again is a row at work: the word goes, and
	// the count it was stamped on starts over.
	back := serving
	was = markClosed(back, was)
	if e := back[0].entries[0]; e.status != statusActive || e.fault {
		t.Errorf("a port bound again says %q", e.status)
	}
	again := closed()
	markClosed(again, was)
	if e := again[0].entries[0]; e.status != statusActive {
		t.Errorf("the first reading after the port came back says %q", e.status)
	}
}

// A listener that has closed keeps its row on the panel. Its port is
// what had lifted it onto the head that runs it; with the port gone
// the row would have folded into that head and taken the fault with
// it, leaving a project whose dev server is unreachable saying nothing
// at all.
func TestAClosedListenerKeepsItsRow(t *testing.T) {
	began := time.Now().Add(-time.Hour)
	rows := func(ports []string) []project {
		return []project{{path: "/w/a", entries: []entry{
			{pid: 300, kind: kindShell, command: "npm run dev", started: began, status: statusActive, depth: 0},
			{pid: 301, kind: kindRun, command: "node vite", started: began, status: statusActive, depth: 1, ports: ports},
		}}}
	}
	was := markClosed(rows([]string{"5173"}), nil)
	for range closedAfter {
		was = markClosed(rows(nil), was)
	}
	gone := rows(nil)
	markClosed(gone, was)
	folded := fold(gone)
	if len(folded[0].entries) != 2 {
		t.Fatalf("the fold left %d rows: %+v", len(folded[0].entries), folded[0].entries)
	}
	if e := folded[0].entries[1]; e.status != statusClosed || !e.fault {
		t.Errorf("the listener's row says %q, fault %v", e.status, e.fault)
	}
	// And the project's block says the fault at the end of its rule.
	b := composeProcesses(folded, nil, "", nil, func(string) bool { return true }, "/h", time.Now(), "", false, true)
	if word, stamped, _ := verdict(b.projects[0].rows); word != statusClosed || !stamped {
		t.Errorf("the block says %q, stamped %v", word, stamped)
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
	served := map[int]servingSeen{
		301: {started: began, lost: closedAfter},
		302: {started: began, lost: closedAfter},
		303: {started: began, lost: closedAfter},
		304: {started: began, lost: closedAfter},
		305: {started: since, lost: closedAfter},
		306: {started: began, lost: closedAfter},
	}
	projects := []project{{path: "/w/a", entries: []entry{
		{pid: 301, kind: kindContact, command: "claude", started: began, status: statusWorking},
		{pid: 302, kind: kindRun, command: "node vite", started: began, status: statusEnded, fault: true},
		{pid: 0, kind: kindService, command: "web", container: "abc", status: statusActive},
		{pid: 304, kind: kindRun, command: "node vite", started: began, status: statusStopped, fault: true},
		{pid: 305, kind: kindRun, command: "node vite", started: began, status: statusActive},
		{pid: 306, kind: kindContact, command: "claude", started: began, status: statusWaiting},
	}}}
	markClosed(projects, served)
	for _, e := range projects[0].entries {
		if e.status == statusClosed {
			t.Errorf("%d says CLOSED", e.pid)
		}
	}
}
