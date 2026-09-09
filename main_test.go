package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

var testReport = report{
	version: "0.7.1",
	note:    "(devel)",
	station: "w0zro@station",
	term:    "xterm-256color · truecolor",
	clock:   "08-Sep-2026  23:58:41 Z",
	facts: []fact{
		{label: "HOST", value: "station"},
		{label: "SYSTEM", value: "macOS 26.6.2 · arm64"},
		{label: "CPU", value: "Apple M3 Pro · 11 cores"},
		{label: "USER", value: "w0zro · /bin/zsh"},
		{label: "MEMORY", value: "18 GB"},
		{label: "UPTIME", value: "5D 01H 15M"},
		{label: "LOAD", value: "4.55 2.09 1.87"},
		{label: "NET", value: "en0 192.168.68.58"},
	},
	checks: []check{
		{label: "TMUX", value: "3.5a", status: nominal},
		{label: "GIT", value: "2.45.2 · lsof 4.91 · docker 27.3", status: nominal},
		{label: "STATE", value: "~/.local/state/conn", status: nominal},
		{label: "DISK", value: "412 GB FREE OF 994 GB", status: nominal},
	},
}

func texts(rows []row) string {
	var b []string
	for _, r := range rows {
		b = append(b, r.text)
	}
	return strings.Join(b, "\n")
}

// The console lays out as handed off: the station block beside the mark,
// eight facts in two columns, five checks to one status column, and the
// greeting alone when all is nominal.
func TestConsoleLaysOutAsHandedOff(t *testing.T) {
	rows := screen(testReport, 100, 30)
	text := texts(rows)
	for _, s := range []string{
		"CONN 0.7.1 (devel)", "STATION  W0ZRO@STATION", "08-SEP-2026  23:58:41 Z",
		"SYSTEM", "HOST .... STATION", "MEMORY .. 18 GB", "USER .... W0ZRO · /BIN/ZSH", "NET ..... EN0 192.168.68.58",
		"START-UP CHECKS", "TERMINAL .. XTERM-256COLOR · TRUECOLOR · 100×30", "TMUX ...... 3.5A",
		"GIT ....... 2.45.2 · LSOF 4.91 · DOCKER 27.3", "NOMINAL", "***  HELLO FROM THE CONN  ***",
	} {
		if !strings.Contains(text, s) {
			t.Errorf("console lacks %q:\n%s", s, text)
		}
	}
	if strings.Contains(text, "NOT NOMINAL") {
		t.Errorf("a verdict chip with nothing wrong:\n%s", text)
	}
	if strings.Contains(text, "\x1b") {
		t.Errorf("console carries escapes off a terminal:\n%q", text)
	}
	for _, r := range rows {
		if i := strings.Index(r.text, "NOMINAL"); i >= 0 && utf8.RuneCountInString(r.text[:i]) != margin+statusCol {
			t.Errorf("status is not in column %d: %q", margin+statusCol, r.text)
		}
		if w := utf8.RuneCountInString(r.text); w > 100 {
			t.Errorf("row is %d columns: %q", w, r.text)
		}
	}
	for _, r := range rows {
		if i := strings.Index(r.text, "MEMORY"); i >= 0 && utf8.RuneCountInString(r.text[:i]) != margin+rightCol {
			t.Errorf("second column is not at %d: %q", margin+rightCol, r.text)
		}
	}
	if len(wordmark) != 6 {
		t.Errorf("wordmark is %d rows", len(wordmark))
	}
	for i, m := range wordmark {
		if utf8.RuneCountInString(m) != utf8.RuneCountInString(wordmark[0]) {
			t.Errorf("wordmark row %d is a different width", i)
		}
	}
}

// A fault lights: the chip in the status column, the count and the
// consequence in the verdict band, and the anomaly echoed in the facts.
func TestFaultsLightTheConsole(t *testing.T) {
	r := testReport
	r.checks = append([]check{}, r.checks...)
	r.checks[3] = check{label: "DISK", value: "6.9 GB FREE OF 494 GB", status: "LOW", fault: true, consequence: "DISK LOW — CONN RUNS, MIND YOUR BUILDS"}
	r.facts = append([]fact{}, r.facts...)
	r.facts[4].anomaly = "6.9 GB DISK FREE"
	text := texts(screen(r, 100, 30))
	for _, s := range []string{" LOW ", "1 SYSTEM NOT NOMINAL", "DISK LOW — CONN RUNS, MIND YOUR BUILDS", "MEMORY .. 18 GB · 6.9 GB DISK FREE"} {
		if !strings.Contains(text, s) {
			t.Errorf("fault not lit, lacks %q:\n%s", s, text)
		}
	}
	small := texts(screen(r, 60, 20))
	if !strings.Contains(small, " SMALL ") || !strings.Contains(small, "2 SYSTEMS NOT NOMINAL") {
		t.Errorf("small terminal not a fault:\n%s", small)
	}
}

// In color, every row is painted edge to edge on the ground and ends with
// the terminal's own colors back; the words are the plain console's.
func TestColoredConsolePaintsEveryRow(t *testing.T) {
	pal = colored()
	defer func() { pal = plain }()
	rows := screen(testReport, 120, 40)
	if len(rows) != 40 {
		t.Errorf("120x40 has %d rows", len(rows))
	}
	for i, r := range rows {
		if !strings.HasPrefix(r.text, pal.normal) || !strings.HasSuffix(r.text, pal.end) {
			t.Errorf("row %d is not painted from the ground to the end: %q", i, r.text)
		}
		if w := utf8.RuneCountInString(stripEscapes(r.text)); w != 120 {
			t.Errorf("row %d paints %d columns: %q", i, w, stripEscapes(r.text))
		}
	}
	text := stripEscapes(texts(rows))
	for _, s := range []string{"CONN 0.7.1 (devel)", "TMUX ...... 3.5A", "NOMINAL", "***  HELLO FROM THE CONN  ***"} {
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
	m.width, m.height = 100, 30
	has := func(s string) bool { return strings.Contains(m.View().Content, s) }
	if !has("STATION  W0ZRO@STATION") || has("SYSTEM") || has("HELLO") {
		t.Errorf("the header alone should be up at the start:\n%s", m.View().Content)
	}
	if got := strings.Count(m.View().Content, "\n") + 1; got != 30 {
		t.Errorf("view is %d rows, not the terminal's 30", got)
	}
	next, _ := m.Update(stageMsg{})
	m = next.(model)
	if !has("HOST .... STATION") || has("TMUX") {
		t.Errorf("the system block should be up second:\n%s", m.View().Content)
	}
	for m.stage < lastStage {
		next, _ = m.Update(stageMsg{})
		m = next.(model)
	}
	if !has("DISK") || !has("HELLO FROM THE CONN") {
		t.Errorf("the console did not finish:\n%s", m.View().Content)
	}
	m.stage = stageSystem
	next, _ = m.Update(stageMsg{}) // a message is not a key
	m = next.(model)
	if m.stage != stageChecks {
		t.Errorf("stage went to %d", m.stage)
	}
	next, cmd := m.Update(clockMsg{})
	m = next.(model)
	if m.report.clock == testReport.clock || cmd == nil {
		t.Errorf("the clock did not turn: %q", m.report.clock)
	}
}

// The machine can be read without a fuss, and what it says is in shape.
func TestStationReportReadsTheMachine(t *testing.T) {
	r := stationReport()
	if (r.version == "" && r.note == "") || r.station == "" || r.clock == "" {
		t.Errorf("identification incomplete: %+v", r)
	}
	if len(r.facts) != 8 {
		t.Errorf("%d facts, not 8: %+v", len(r.facts), r.facts)
	}
	if len(r.checks) != checkCount-1 {
		t.Errorf("%d checks, not %d: %+v", len(r.checks), checkCount-1, r.checks)
	}
	for _, c := range r.checks {
		if c.label == "" || c.value == "" || c.status == "" || (c.fault && c.consequence == "") {
			t.Errorf("check incomplete: %+v", c)
		}
	}
	if gigabytes(18<<30, 1<<30) != "18 GB" || gigabytes(994_662_584_320, 1e9) != "995 GB" {
		t.Errorf("sizes: %q %q", gigabytes(18<<30, 1<<30), gigabytes(994_662_584_320, 1e9))
	}
	if firstVersion("tmux 3.5a") != "3.5a" || firstVersion("git version 2.50.1") != "2.50.1" {
		t.Errorf("versions: %q %q", firstVersion("tmux 3.5a"), firstVersion("git version 2.50.1"))
	}
	if join(" · ", "a", "", " b ") != "a · b" {
		t.Errorf("join: %q", join(" · ", "a", "", " b "))
	}
}
