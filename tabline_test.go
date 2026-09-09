package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// The tabline, the finder's listing and the kill preview: the parts the
// redesign added, tested without a server.

// heldTabs is a model holding three buffers in one place, the second an
// entry whose run ended badly.
func heldTabs(w int) model {
	m := withProcList(w, 20,
		[]Project{{Name: "demo", Path: "/p/demo"}},
		[]Proc{
			{PID: 700, PPID: 1, Command: "zsh", Dir: "/p/demo"},
			{PID: 701, PPID: 1, Command: "zsh", Dir: "/p/demo"},
			{PID: 702, PPID: 1, Command: "zsh", Dir: "/p/demo"},
		})
	m.terms = map[int]*remoteTerm{
		700: {pid: 700, dir: "/p/demo"},
		701: {pid: 701, dir: "/p/demo", name: "test", run: "go test ./...", exit: "1"},
		702: {pid: 702, dir: "/p/demo", name: "web", run: "npm run dev"},
	}
	m.rebuild()
	return m
}

func TestTheTablineNamesEachBufferForItsPlaceAndMarksItsState(t *testing.T) {
	m := heldTabs(80)
	line := tablineOf(m)
	for _, want := range []string{"everything", "demo/test ✗", "demo/web", "demo/zsh", ". toggles running · all"} {
		if !strings.Contains(line, want) {
			t.Errorf("tabline = %q, want %q", line, want)
		}
	}
	// The hint follows the view: a dead task's buffer offers the rerun.
	m.all, m.shown = false, 701
	m.keepRows()
	if line := tablineOf(m); !strings.HasSuffix(line, "r reruns") {
		t.Errorf("tabline = %q, want the rerun offered on a dead buffer", line)
	}
}

func TestTheTablineKeepsTheFocusedTabInView(t *testing.T) {
	// Tabs past the width give way from the left, so the focused one is
	// always drawn.
	m := heldTabs(30)
	m.all, m.shown = false, 702
	m.keepRows()
	line := tablineOf(m)
	if !strings.Contains(line, "demo/web") {
		t.Errorf("tabline = %q, want the focused tab in view", line)
	}
	for _, row := range strings.Split(stripANSI(m.View().Content), "\n") {
		if w := len([]rune(row)); w > 30 {
			t.Errorf("row %q is %d columns, wider than the window", row, w)
		}
	}
}

func TestTheHeadingSaysHowTheRunEndedAndWhatRanBefore(t *testing.T) {
	m := heldTabs(100)
	m.all, m.shown = false, 701
	m.history[701] = []run{{Exit: "1", Took: 120}, {Exit: "0", Took: 12}}
	m.keepRows()
	head := stripANSI(m.heading())
	for _, want := range []string{"test", "in demo", "go test ./...", "pid 701", "✗ exit 1", "past runs", "✗ 2m", "✓ 12s"} {
		if !strings.Contains(head, want) {
			t.Errorf("heading = %q, want %q", head, want)
		}
	}
	if f := footer(m); !strings.HasPrefix(f, "FAILED") {
		t.Errorf("footer = %q, want the FAILED chip while a failed buffer is shown", f)
	}
}

func TestTheFindersListingHoldsEverythingOpenable(t *testing.T) {
	m := heldTabs(80)
	m.plans = map[string]plan{"/p/demo": {Entries: []entry{{Name: "web", Run: "npm run dev"}, {Name: "api", Run: "go run ./api"}}}}
	m.roots = []string{"/p"}
	snap := m.finderSnapshot()
	byLabel := map[string][]finderEntry{}
	for _, e := range snap.Entries {
		byLabel[e.Label] = append(byLabel[e.Label], e)
	}
	// The held buffers, each once, whatever the place defines by the same
	// name; the entries not held; the place's shell.
	for label, kind := range map[string]string{"demo/zsh": "buffer", "demo/test": "buffer", "demo/web": "buffer",
		"demo/api": "entry", "demo/terminal": "place"} {
		es := byLabel[label]
		if len(es) != 1 || es[0].Kind != kind {
			t.Errorf("%s: %+v, want one %s entry", label, es, kind)
		}
	}
	if snap.Root != "/p" {
		t.Errorf("root = %q, want the first projects root, for a name that matches nothing", snap.Root)
	}
	if e := byLabel["demo/test"][0]; !strings.Contains(plainFacts(e), "✗") {
		t.Errorf("the dead test's facts = %q, want its ending", plainFacts(e))
	}
	// The listing survives its trip through the file.
	b, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	var back finderSnapshot
	if err := json.Unmarshal(b, &back); err != nil || len(back.Entries) != len(snap.Entries) {
		t.Errorf("the listing did not round-trip: %v", err)
	}
}

func TestTheFinderNarrowsByFuzzAndOffersToMakeWhatIsMissing(t *testing.T) {
	f := newFinderModel("")
	f.loaded = true
	f.snap = finderSnapshot{Root: "/p", Entries: []finderEntry{
		{Kind: "buffer", Label: "datum/tests", Facts: []segment{{"task", toneQuiet}}},
		{Kind: "buffer", Label: "datum/claude", Facts: []segment{{"● running", toneGood}}},
		{Kind: "place", Label: "conn/terminal"},
	}}
	f.query.SetValue("dat ts")
	if got := f.matches(); len(got) != 1 || got[0].Label != "datum/tests" {
		t.Fatalf("matches = %+v, want the subsequence to narrow to datum/tests", got)
	}
	page := stripANSI(f.render())
	for _, want := range []string{"› dat ts", "▸ datum/tests", "task", `no match? enter on "dat ts" makes the repo`, ", the next kind"} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q:\n%s", want, page)
		}
	}
	if strings.Contains(page, "datum/claude") {
		t.Errorf("page lists what does not answer:\n%s", page)
	}
	f.query.SetValue("")
	if got := f.matches(); len(got) != 3 {
		t.Errorf("an empty query lists %d, want everything", len(got))
	}
}

func TestTheKillPreviewShowsTheTreeAndTheConsequences(t *testing.T) {
	m := heldTabs(100)
	m.procs = append(m.procs, Proc{PID: 710, PPID: 702, Command: "node", Argv: "node vite", Dir: "/p/demo", Ports: []string{"3000"}})
	m.rebuild()
	m.all, m.shown = false, 702
	m.keepRows()
	// The cursor is on web's row; x previews from the buffer.
	for i, r := range m.rows {
		if r.kind == rowProc && r.holds(702) {
			m.cursor = i
		}
	}
	m, _ = pipeServer(t, m)
	m = press(m, "x")
	if m.shown != 0 || m.from != 702 || m.pendingKill == nil {
		t.Fatalf("shown %d, from %d, pending %v; want the buffer parked for the preview", m.shown, m.from, m.pendingKill)
	}
	page := strings.Join(strings.Fields(strings.Join(bodyRows(m), " ")), " ")
	for _, want := range []string{"web", "about to die", "✗ vite", "pid 710", "holds :3000",
		":3000 frees", "esc changes your mind", "esc keeps it"} {
		if !strings.Contains(page, want) {
			t.Errorf("preview lacks %q:\n%s", want, page)
		}
	}
	if f := footer(m); !strings.Contains(f, "CONFIRM") || !strings.Contains(f, ":3000") {
		t.Errorf("footer = %q, want CONFIRM and the port", f)
	}
	// esc gives the buffer back.
	m = press(m, "esc")
	if m.pendingKill != nil || m.shown != 702 {
		t.Errorf("after esc: pending %v, shown %d; want the buffer back", m.pendingKill, m.shown)
	}
}

func TestAOpensTheFinderOnEveryConversationAtRest(t *testing.T) {
	// The page on the conversations at rest lists them alone, offers
	// to make nothing, and says what enter does there.
	f := newFinderModel(finderRests)
	f.loaded = true
	f.snap = finderSnapshot{Place: "datum", Entries: []finderEntry{
		restEntry(Project{Name: "datum", Path: "/p/datum"}, conversation{ID: "a1", Dir: "/p/datum", Prompt: "fix the trace", Branch: "main", Kind: "claude"}),
		restEntry(Project{Name: "datum", Path: "/p/datum"}, conversation{ID: "b2", Dir: "/p/datum/api", Summary: "reading", Kind: "claude"}),
	}}
	page := stripANSI(f.render())
	for _, want := range []string{"datum ·", `"fix the trace"`, "main", `"reading"`, "every conversation at rest in datum", "enter picks it back up", "esc leaves it"} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q:\n%s", want, page)
		}
	}
	if strings.Contains(page, "makes the repo") {
		t.Errorf("page offers to make a repo from the conversations at rest:\n%s", page)
	}
	f.query.SetValue("zzz")
	if next, _ := f.act(); next.(finderModel).said != "" {
		t.Errorf("enter on no match said %q, want nothing made and nothing said", next.(finderModel).said)
	}
	if page := stripANSI(f.render()); !strings.Contains(page, "nothing at rest answers zzz") {
		t.Errorf("page = %s, want the lack said", page)
	}
}

func TestAReachesIntoTheRowsPlaceOrTheShownBuffers(t *testing.T) {
	m := heldTabs(80)
	m.all, m.shown = true, 0
	for i, r := range m.rows {
		if r.project.Name == "demo" && r.kind == rowProject {
			m.cursor = i
		}
	}
	if p, ok := m.restPlace(); !ok || p.Name != "demo" {
		t.Errorf("place = %+v, %v with the cursor on demo, want demo", p, ok)
	}
	m.all, m.shown = false, 701
	if p, ok := m.restPlace(); !ok || p.Path != m.terms[701].dir {
		t.Errorf("place = %+v, %v with 701 shown, want the buffer's place", p, ok)
	}
	m.shown = 0
	m.cursor = -1
	if _, ok := m.restPlace(); ok {
		t.Error("nothing under the cursor and nothing shown should name no place")
	}
}
