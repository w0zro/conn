package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAReleaseIsNewerOnlyThanAReleaseBehindIt(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v0.5.0", "0.4.0", true},
		{"v0.4.1", "0.4.0", true},
		{"v1.0.0", "0.9.9", true},
		{"v0.4.0", "0.4.0", false},
		{"v0.4.0", "0.5.0", false},
		{"v0.10.0", "0.9.0", true}, // numbers, not letters
		{"v0.5.0", "(devel)", false},
		{"v0.5.0", "0.4.0-3-gabc1234-dirty", false}, // a tree past a tag is ahead of it
		{"v0.5.0", "unknown", false},
		{"latest", "0.4.0", false},
	}
	for _, c := range cases {
		if got := newer(c.latest, c.current); got != c.want {
			t.Errorf("newer(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}

// answering has github answer with a tag, counting the asks.
func answering(t *testing.T, tag string, err error) *int {
	t.Helper()
	old := fetchLatest
	t.Cleanup(func() { fetchLatest = old })
	asks := 0
	fetchLatest = func() (string, error) {
		asks++
		return tag, err
	}
	return &asks
}

// stateDir points the stamp at a directory of the test's own.
func stateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CONN_SOCKET", "")
	t.Setenv("XDG_STATE_HOME", dir)
	return dir
}

func TestTheAnswerIsKeptForADay(t *testing.T) {
	// The navigator is started again at every launch, and each start
	// looks; github is asked once a day, and the kept answer serves the
	// rest. U asks regardless: pressing it is the way to ask now.
	stateDir(t)
	asks := answering(t, "v0.5.0", nil)
	now := time.Now()

	first := checkUpdate(false, now)().(updateMsg)
	if first.latest != "v0.5.0" || *asks != 1 {
		t.Fatalf("first look = %+v after %d asks, want github asked", first, *asks)
	}
	again := checkUpdate(false, now.Add(time.Hour))().(updateMsg)
	if again.latest != "v0.5.0" || *asks != 1 {
		t.Errorf("an hour on = %+v after %d asks, want the kept answer", again, *asks)
	}
	later := checkUpdate(false, now.Add(updateEvery+time.Minute))().(updateMsg)
	if later.latest != "v0.5.0" || *asks != 2 {
		t.Errorf("a day on = %+v after %d asks, want github asked again", later, *asks)
	}
	asked := checkUpdate(true, now)().(updateMsg)
	if !asked.asked || *asks != 3 {
		t.Errorf("U = %+v after %d asks, want github asked now", asked, *asks)
	}
}

func TestAFailedLookIsSilentUnlessAsked(t *testing.T) {
	stateDir(t)
	answering(t, "", errors.New("no network"))
	m := withProcList(90, 14, []Project{{Name: "brand", Path: "/p/brand"}}, nil)

	quiet := checkUpdate(false, time.Now())().(updateMsg)
	next, _ := m.Update(quiet)
	if m := next.(model); m.status != "" || m.release != "" {
		t.Errorf("status = %q, release = %q; want nothing said of a daily look that failed", m.status, m.release)
	}
	loud := checkUpdate(true, time.Now())().(updateMsg)
	next, _ = m.Update(loud)
	if m := next.(model); !m.statusErr || !strings.Contains(m.status, "no network") {
		t.Errorf("status = %q, want the failure said when U asked", m.status)
	}
	if _, err := os.Stat(stampPath()); err == nil {
		t.Error("a failure was kept as an answer")
	}
}

func TestANewerReleaseIsSaidWheneverNothingElseIs(t *testing.T) {
	old := version
	version = "v0.4.0"
	t.Cleanup(func() { version = old })
	m := withProcList(90, 14, []Project{{Name: "brand", Path: "/p/brand"}}, nil)

	next, _ := m.Update(updateMsg{latest: "v0.5.0"})
	m = next.(model)
	if m.release != "v0.5.0" {
		t.Fatalf("release = %q, want the newer one remembered", m.release)
	}
	if st := m.statusLine(); !strings.Contains(st.msg, "conn 0.5.0 is out · U installs it") {
		t.Errorf("status = %+v, want the release offered", st)
	}
	// A report has the line while it stands; the offer is back after.
	m.status = "made site"
	if st := m.statusLine(); !strings.Contains(st.msg, "made site") || strings.Contains(st.msg, "0.5.0") {
		t.Errorf("status = %+v, want the report alone", st)
	}
	m = press(m, "j")
	if st := m.statusLine(); !strings.Contains(st.msg, "0.5.0") {
		t.Errorf("status = %+v, want the offer back once the report is gone", st)
	}

	// The same release again, or an older one, is nothing new.
	next, _ = m.Update(updateMsg{latest: "v0.4.0"})
	if m := next.(model); m.release != "v0.5.0" {
		t.Errorf("release = %q, want the newer one kept", m.release)
	}
}

func TestUInstallsTheReleaseInAPopup(t *testing.T) {
	// The installer runs over the window, told to put the release where
	// this build is, and conn after it: from inside the window, that gives
	// the server the new configuration and the navigator back as the new
	// build.
	m := withProcList(90, 14, []Project{{Name: "brand", Path: "/p/brand"}}, nil)
	m.release = "v0.5.0"
	m, asked := pipeServer(t, m)
	m = press(m, "U")
	if !strings.Contains(m.status, "installing conn 0.5.0") {
		t.Errorf("status = %q, want the install said", m.status)
	}
	got := askedForKind(t, asked, kindHelp)
	if !strings.Contains(got.Run, installURL) || !strings.Contains(got.Run, "CONN_INSTALL_DIR=") {
		t.Errorf("popup runs %q, want the installer, aimed at this build's directory", got.Run)
	}
}

func TestUWithNothingNewerAsksNow(t *testing.T) {
	stateDir(t)
	old := version
	version = "v0.5.0"
	t.Cleanup(func() { version = old })
	asks := answering(t, "v0.5.0", nil)
	m := withProcList(90, 14, []Project{{Name: "brand", Path: "/p/brand"}}, nil)

	next, cmd := m.Update(typed("U"))
	m = next.(model)
	if cmd == nil || m.status != "asking for the latest conn" {
		t.Fatalf("status = %q, cmd = %v; want a look started", m.status, cmd != nil)
	}
	next, _ = m.Update(cmd())
	m = next.(model)
	if *asks != 1 || m.status != "conn 0.5.0 is the latest" || m.release != "" {
		t.Errorf("status = %q, release = %q after %d asks; want told this is the latest", m.status, m.release, *asks)
	}
}

func TestTheInstallerIsAimedAtThisBuild(t *testing.T) {
	cmd := updateCommand("/Users/me/.local/bin/conn")
	for _, want := range []string{
		"curl -fsSL " + installURL + " | CONN_INSTALL_DIR='/Users/me/.local/bin' sh",
		"&& '/Users/me/.local/bin/conn'",
		"read -r _", // a failure holds the popup on its output
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("command = %q, want %q in it", cmd, want)
		}
	}
	// A build whose path is unknown goes where the installer puts one.
	if cmd := updateCommand("conn"); strings.Contains(cmd, "CONN_INSTALL_DIR") || !strings.Contains(cmd, "&& 'conn'") {
		t.Errorf("command = %q, want the installer's own directory and conn by name", cmd)
	}
}

func TestTheStampSurvivesARead(t *testing.T) {
	stateDir(t)
	at := time.Unix(1788716897, 0)
	if err := writeStamp(at, "v0.5.0"); err != nil {
		t.Fatal(err)
	}
	got, tag, ok := readStamp()
	if !ok || !got.Equal(at) || tag != "v0.5.0" {
		t.Errorf("stamp = %v %q %v, want what was written", got, tag, ok)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(stampPath()), "latest-release"), []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := readStamp(); ok {
		t.Error("a stamp that does not parse was believed")
	}
}
