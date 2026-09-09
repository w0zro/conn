package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

var testReport = report{
	version: "0.7.1",
	note:    "(devel)",
	build:   "4af550d · 09-SEP-2026 · MODIFIED",
	station: "w0zro@station",
	term:    "xterm-256color · truecolor",
	clock:   "08-Sep-2026  23:58:41 Z",
	system: []fact{
		{"HOST", "station"},
		{"SYSTEM", "macOS 26.6.2 (25G83)"},
		{"CPU", "Apple M3 Pro · 11 CORES (5P + 6E)"},
		{"MEMORY", "18 GB · 77% AVAILABLE"},
		{"UPTIME", "5D 01H 15M · UP SINCE 04-Sep 00:47 Z"},
	},
	session: []fact{
		{"USER", "w0zro · UID 501 · ADMIN"},
		{"SHELL", "ZSH 5.9"},
		{"TERMINAL", "ghostty 1.3.1"},
		{"TIME ZONE", "America/Los_Angeles · UTC-07:00 · 19:28 LOCAL"},
	},
	checks: []check{
		{label: "STATE", value: "~/.local/state/conn", status: nominal},
		{label: "DISK", value: "412 GB FREE OF 994 GB", status: nominal},
		{label: "MEMORY", value: "77% OF 18 GB AVAILABLE", status: nominal},
		{label: "LOAD", value: "1.85 2.07 1.99 · 11 CORES", status: nominal},
		{label: "NET", value: "en0 192.168.68.58 · 2 UP", status: nominal},
		{label: "POWER", value: "BATTERY · 81% · DISCHARGING · 9:04 LEFT", status: nominal},
		{label: "CLOCK", value: "19:28:41 PDT · AFTER THE BUILD OF 09-SEP-2026", status: nominal},
	},
}

func texts(rows []row) string {
	var b []string
	for _, r := range rows {
		b = append(b, r.text)
	}
	return strings.Join(b, "\n")
}

// The console lays out as briefed: the station block beside the mark,
// the system and the session side by side, the checks to one status
// column flush right, and the verdict that all is nominal.
func TestConsoleLaysOut(t *testing.T) {
	rows := screen(testReport, 120, 40)
	measure, rightCol, _ := columns(120)
	text := texts(rows)
	for _, s := range []string{
		"CONN 0.7.1 (devel)", "STATION  W0ZRO@STATION", "08-SEP-2026  23:58:41 Z", "4af550d · 09-SEP-2026 · MODIFIED",
		"SYSTEM", "SESSION", "HOST ...... STATION", "USER ...... W0ZRO · UID 501 · ADMIN", "TIME ZONE . AMERICA/LOS_ANGELES",
		"START-UP CHECKS", "SCREEN .... 120×40 · XTERM-256COLOR · TRUECOLOR", "STATE ..... ~/.LOCAL/STATE/CONN",
		"CLOCK ..... 19:28:41 PDT", "NOMINAL", "ALL SYSTEMS NOMINAL",
	} {
		if !strings.Contains(text, s) {
			t.Errorf("console lacks %q:\n%s", s, text)
		}
	}
	for _, s := range []string{"NOT NOMINAL", "TMUX", "GIT", "\x1b"} {
		if strings.Contains(text, s) {
			t.Errorf("console carries %q:\n%s", s, text)
		}
	}
	if len(rows) != 40 {
		t.Errorf("120x40 has %d rows", len(rows))
	}
	for _, r := range rows {
		if strings.HasSuffix(r.text, "NOMINAL") && strings.Contains(r.text, "...") && utf8.RuneCountInString(r.text) != margin+measure {
			t.Errorf("status is not flush with column %d: %q", margin+measure, r.text)
		}
		if i := strings.Index(r.text, "USER ..."); i >= 0 && utf8.RuneCountInString(r.text[:i]) != margin+rightCol {
			t.Errorf("session column is not at %d: %q", margin+rightCol, r.text)
		}
		if w := utf8.RuneCountInString(r.text); w > 120 {
			t.Errorf("row is %d columns: %q", w, r.text)
		}
	}
	if len(wordmark) != 6 {
		t.Errorf("wordmark is %d rows", len(wordmark))
	}
}

// A fault lights: the chip flush right in the status column, and the
// count in the verdict band. A short terminal is a fault of its own.
func TestFaultsLightTheConsole(t *testing.T) {
	r := testReport
	r.checks = append([]check{}, r.checks...)
	r.checks[1] = check{label: "DISK", value: "6.9 GB FREE OF 494 GB", status: "LOW", fault: true}
	rows := screen(r, 120, 40)
	text := texts(rows)
	for _, s := range []string{" LOW", "1 SYSTEM NOT NOMINAL"} {
		if !strings.Contains(text, s) {
			t.Errorf("fault not lit, lacks %q:\n%s", s, text)
		}
	}
	// In plain text the chip's trailing space is trimmed with the row's.
	measure, _, _ := columns(120)
	for _, row := range rows {
		if strings.HasSuffix(row.text, " LOW") && utf8.RuneCountInString(row.text) != margin+measure-1 {
			t.Errorf("chip is not flush with column %d: %q", margin+measure, row.text)
		}
	}
	small := texts(screen(r, 100, 20))
	if !strings.Contains(small, " SMALL") || !strings.Contains(small, "2 SYSTEMS NOT NOMINAL") {
		t.Errorf("short terminal not a fault:\n%s", small)
	}
	piped := texts(screen(r, 80, 0))
	if !strings.Contains(piped, "SCREEN .... NO TERMINAL") || !strings.Contains(piped, " UNCHECKED") || !strings.Contains(piped, "1 SYSTEM NOT NOMINAL") {
		t.Errorf("no terminal should be unchecked, not a fault:\n%s", piped)
	}
	if rowsNeeded(testReport) != 27 {
		t.Errorf("the test report needs %d rows", rowsNeeded(testReport))
	}
}

// In color, every row is painted edge to edge on the ground and ends with
// the terminal's own colors back; the words are the plain console's.
func TestColoredConsolePaintsEveryRow(t *testing.T) {
	pal = colored()
	defer func() { pal = plain }()
	rows := screen(testReport, 120, 40)
	for i, r := range rows {
		if !strings.HasPrefix(r.text, pal.normal) || !strings.HasSuffix(r.text, pal.end) {
			t.Errorf("row %d is not painted from the ground to the end: %q", i, r.text)
		}
		if w := utf8.RuneCountInString(stripEscapes(r.text)); w != 120 {
			t.Errorf("row %d paints %d columns: %q", i, w, stripEscapes(r.text))
		}
	}
	text := stripEscapes(texts(rows))
	for _, s := range []string{"CONN 0.7.1 (devel)", "SHELL ..... ZSH 5.9", "NOMINAL", "ALL SYSTEMS NOMINAL"} {
		if !strings.Contains(text, s) {
			t.Errorf("colored console lacks %q:\n%s", s, text)
		}
	}
}

// stripEscapes drops the color sequences, leaving the cells.
func stripEscapes(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// The console comes on in stages, a key skips to the end, and the clock
// keeps time.
func TestProgramComesOnInStages(t *testing.T) {
	m := newModel()
	m.report = testReport
	m.width, m.height = 120, 40
	has := func(s string) bool { return strings.Contains(m.View().Content, s) }
	if !has("STATION  W0ZRO@STATION") || has("SYSTEM") || has("ALL SYSTEMS") {
		t.Errorf("the header alone should be up at the start:\n%s", m.View().Content)
	}
	if got := strings.Count(m.View().Content, "\n") + 1; got != 40 {
		t.Errorf("view is %d rows, not the terminal's 40", got)
	}
	next, _ := m.Update(stageMsg{})
	m = next.(model)
	if !has("HOST ...... STATION") || has("SCREEN") {
		t.Errorf("the readout should be up second:\n%s", m.View().Content)
	}
	next, _ = m.Update(stageMsg{})
	m = next.(model)
	if !has("SCREEN") || has("STATE ...") {
		t.Errorf("the screen check should be up third, alone:\n%s", m.View().Content)
	}
	for m.stage < lastStage(m.report) {
		next, _ = m.Update(stageMsg{})
		m = next.(model)
	}
	if !has("CLOCK") || !has("ALL SYSTEMS NOMINAL") {
		t.Errorf("the console did not finish:\n%s", m.View().Content)
	}
	if lastStage(m.report) != stageChecks+8 {
		t.Errorf("last stage is %d", lastStage(m.report))
	}
	next, cmd := m.Update(clockMsg{})
	m = next.(model)
	if m.report.clock == testReport.clock || cmd == nil {
		t.Errorf("the clock did not turn: %q", m.report.clock)
	}
}

// The machine can be read without a fuss, and what it says is in shape.
func TestPathsShortenFromTheMiddle(t *testing.T) {
	exe := "/VAR/FOLDERS/51/KKGWPD9J2R53TRB71LNMFP6R0000GN/T/GO-BUILD3688959642/B001/EXE/CONN"
	for _, c := range []struct {
		in   string
		w    int
		want string
	}{
		{exe + " · 5.5 MB", 90, exe + " · 5.5 MB"},
		{exe + " · 5.5 MB", 60, "/VAR/…/T/GO-BUILD3688959642/B001/EXE/CONN · 5.5 MB"},
		{exe + " · 5.5 MB", 30, "/VAR/…/B001/EXE/CONN · 5.5 MB"},
		{exe + " · 5.5 MB", 22, "/VAR/…/CONN · 5.5 MB"},
		{exe, 8, "…XE/CONN"},
		{"~/PROJECTS/W0ZRO/CONN/CONN", 20, "~/…/W0ZRO/CONN/CONN"},
		{"~/PROJECTS/W0ZRO/CONN", 40, "~/PROJECTS/W0ZRO/CONN"},
		{"APPLE M3 PRO · 11 CORES (5P + 6E)", 12, "APPLE M3 PR…"},
		{"/A/B", 1, ""},
	} {
		if got := fit(c.in, c.w); got != c.want || utf8.RuneCountInString(got) > c.w {
			t.Errorf("fit(%q, %d) = %q, want %q", c.in, c.w, got, c.want)
		}
	}
}

func TestStationReportReadsTheMachine(t *testing.T) {
	r := stationReport()
	if (r.version == "" && r.note == "") || r.station == "" || r.clock == "" {
		t.Errorf("identification incomplete: %+v", r)
	}
	if len(r.system) < 6 || len(r.session) < 8 {
		t.Errorf("readout thin: %d system, %d session\n%+v\n%+v", len(r.system), len(r.session), r.system, r.session)
	}
	for _, f := range append(r.system, r.session...) {
		if f.label == "" || strings.TrimSpace(f.value) == "" {
			t.Errorf("fact incomplete: %+v", f)
		}
	}
	if len(r.checks) != 7 {
		t.Errorf("%d checks, not 7: %+v", len(r.checks), r.checks)
	}
	for _, c := range r.checks {
		if c.label == "" || c.value == "" || c.status == "" {
			t.Errorf("check incomplete: %+v", c)
		}
		if c.label == "TMUX" || c.label == "GIT" {
			t.Errorf("a tool conn does not depend on is checked: %+v", c)
		}
	}
	if gigabytes(18<<30, 1<<30) != "18 GB" || gigabytes(994_662_584_320, 1e9) != "995 GB" || sizeShort(4_400_000) != "4.2 MB" {
		t.Errorf("sizes: %q %q %q", gigabytes(18<<30, 1<<30), gigabytes(994_662_584_320, 1e9), sizeShort(4_400_000))
	}
	if firstVersion("zsh 5.9 (arm-apple-darwin23.0.0)") != "5.9" || firstVersion("GNU bash, version 5.2.37(1)-release") != "5.2.37" {
		t.Errorf("versions: %q %q", firstVersion("zsh 5.9 (arm-apple-darwin23.0.0)"), firstVersion("GNU bash, version 5.2.37(1)-release"))
	}
	if tilde("/Users/x/p", "/Users/x") != "~/p" || tilde("/Users/xy", "/Users/x") != "/Users/xy" {
		t.Errorf("tilde: %q %q", tilde("/Users/x/p", "/Users/x"), tilde("/Users/xy", "/Users/x"))
	}
}
