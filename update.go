package main

import (
	"errors"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// A newer conn is worth knowing about and easy to take. The navigator asks
// github which release is current — on its first paint and once a day
// after, the answer kept beside the socket so the navigator being started
// again, as every launch does, is not another ask — and a release newer
// than this build is said on the status line whenever nothing else is
// being said. U takes it: the installer runs in a popup over the window,
// puts the release where this build is, and runs conn, which gives the
// server the new build's configuration and brings the navigator back as
// the new build. The shells stay; only the navigator is replaced.

const (
	// releasesURL answers with a redirect to the latest release, which
	// names its tag: a plain request to github.com, not the API, so it
	// spends no rate limit. install.sh asks the same way.
	releasesURL = "https://github.com/w0zro/conn/releases/latest"

	// installURL is the installer, served from the site.
	installURL = "https://conn.w0zro.com/install.sh"

	// updateEvery is how long an answer from github is believed before it
	// is asked again.
	updateEvery = 24 * time.Hour

	// updateTicks is how many process polls pass between looks at whether
	// the answer has aged out: about half an hour, which is how stale a
	// day-old check can be at worst, for a navigator that stays up.
	updateTicks = 900
)

// fetchLatest asks github which release is current: the tag the redirect
// lands on. A variable so the tests can answer instead of github.
var fetchLatest = func() (string, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
		// The redirect is the answer; there is no need to follow it.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Head(releasesURL)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	tag := path.Base(resp.Header.Get("Location"))
	if resp.StatusCode/100 != 3 || tag == "" || tag == "." {
		return "", errors.New("github did not say which release is latest")
	}
	if tag == "latest" {
		return "", errors.New("there are no releases yet")
	}
	return tag, nil
}

// releaseVersion reads a release's tag, or a version as this build reports
// one, as numbers: v0.4.0 and 0.4.0 both. Anything else — (devel), a
// build past a tag, a dirty tree — is no release, and reads as none.
func releaseVersion(s string) ([3]int, bool) {
	var v [3]int
	parts := strings.Split(strings.TrimPrefix(s, "v"), ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

// newer reports whether latest is a release newer than current. A build
// that is no release is never behind one: a tree past a tag is ahead of
// it, and a build with no version at all cannot be placed.
func newer(latest, current string) bool {
	l, ok := releaseVersion(latest)
	if !ok {
		return false
	}
	c, ok := releaseVersion(current)
	if !ok {
		return false
	}
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

// stampPath is where the last answer is kept: beside the socket, in the
// state directory, as when it was given and what it was.
func stampPath() string {
	return filepath.Join(filepath.Dir(socketPath()), "latest-release")
}

// readStamp is the kept answer, if there is one.
func readStamp() (at time.Time, tag string, ok bool) {
	b, err := os.ReadFile(stampPath())
	if err != nil {
		return time.Time{}, "", false
	}
	when, tag, ok := strings.Cut(strings.TrimSpace(string(b)), "\t")
	unix, err := strconv.ParseInt(when, 10, 64)
	if !ok || err != nil || tag == "" {
		return time.Time{}, "", false
	}
	return time.Unix(unix, 0), tag, true
}

// writeStamp keeps an answer. The directory is the one the socket is
// in, and may not be there yet on a machine that has never run the server.
func writeStamp(at time.Time, tag string) error {
	if err := os.MkdirAll(filepath.Dir(stampPath()), 0o700); err != nil {
		return err
	}
	return os.WriteFile(stampPath(), []byte(strconv.FormatInt(at.Unix(), 10)+"\t"+tag+"\n"), 0o600)
}

// updateMsg carries the latest release, or why it could not be asked for.
// asked says U asked, and wants an answer either way; the daily look says
// nothing unless there is something to say.
type updateMsg struct {
	latest string
	err    error
	asked  bool
}

// checkUpdate finds the latest release: the kept answer while it is fresh,
// else github's, kept. U asks github regardless — pressing it is the way
// to ask now.
func checkUpdate(asked bool, now time.Time) tea.Cmd {
	return func() tea.Msg {
		if !asked {
			if at, tag, ok := readStamp(); ok && now.Sub(at) < updateEvery {
				return updateMsg{latest: tag}
			}
		}
		tag, err := fetchLatest()
		if err != nil {
			return updateMsg{err: err, asked: asked}
		}
		_ = writeStamp(now, tag)
		return updateMsg{latest: tag, asked: asked}
	}
}

// learnUpdate takes the answer. A newer release is remembered, for the
// status line to say; the daily look is otherwise silent, and U's look
// says what it found either way.
func (m *model) learnUpdate(msg updateMsg) {
	switch {
	case msg.err != nil:
		if msg.asked {
			m.status, m.statusErr = "could not ask for the latest conn: "+msg.err.Error(), true
		}
	case newer(msg.latest, buildVersion()):
		m.release = msg.latest
		if msg.asked {
			m.status, m.statusErr = updateNotice(msg.latest), false
		}
	case msg.asked:
		m.status, m.statusErr = "conn "+buildVersion()+" is the latest", false
	}
}

// updateNotice is what the status line says of a newer release.
func updateNotice(latest string) string {
	return "conn " + strings.TrimPrefix(latest, "v") + " is out · U installs it"
}

// updateConn is U: with a newer release known, the installer in a popup;
// without one, a look now, which says what it finds.
func (m *model) updateConn() tea.Cmd {
	if m.release == "" {
		m.status, m.statusErr = "asking for the latest conn", false
		return checkUpdate(true, time.Now())
	}
	if m.server == nil {
		m.status, m.statusErr = "no server to run the installer in: "+m.serverErr, true
		return nil
	}
	m.status, m.statusErr = "installing conn "+strings.TrimPrefix(m.release, "v"), false
	m.server.popup(" update ", updateCommand(connExe()))
	return nil
}

// updateCommand is what the popup runs: the installer, told to put the
// release where this build is, then conn — from inside the window, which
// gives the server the new configuration and the navigator back as the
// new build, and the popup closes with it. A failure holds the popup on
// its output until enter, since a popup that closed would take the reason
// with it. A build whose path is unknown is installed where the installer
// puts one, and conn is the name on the PATH.
func updateCommand(exe string) string {
	install := "curl -fsSL " + installURL + " | "
	if filepath.IsAbs(exe) {
		install += "CONN_INSTALL_DIR=" + shellQuote(filepath.Dir(exe)) + " "
	}
	install += "sh"
	return install + " && " + shellQuote(exe) + " || { printf '\\nenter closes this\\n'; read -r _; }"
}
