package main

import (
	"cmp"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"

	tea "charm.land/bubbletea/v2"
)

// A conversation an agent was having does not end when its instance exits;
// it is suspended, and the transcript on disk is enough to pick it back up.
// The newest at rest under a place is a row under the place in the
// everything view and a resume row in the finder — its prompt, its age,
// its branch — and enter continues it, in a shell like any other. A at
// conn, or ctrl-space A from any buffer, is the same verb reaching
// further back: the finder on every conversation at rest, newest first,
// for the one that is not the newest.

// convoDirs is every directory a place's conversations could be filed under:
// its own, its sub-projects', and for a group each repository's in turn —
// a transcript is filed by the exact directory the conversation was had in.
func (m model) convoDirs(p Project) []string {
	dirs := []string{p.Path}
	for _, sp := range m.subs[p.Path] {
		dirs = append(dirs, sp.Path)
	}
	for _, rp := range m.grouped[p.Path] {
		dirs = append(dirs, rp.Path)
		for _, sp := range m.subs[rp.Path] {
			dirs = append(dirs, sp.Path)
		}
	}
	return dirs
}

// liveConversations is the id of every conversation a running instance is
// carrying — vetted against the process table, an agent counting only while
// its process is there and still running it, so a session file outliving
// its process does not hide the conversation it left behind.
func (m model) liveConversations() map[string]bool {
	live := map[string]bool{}
	for pid, a := range m.agents {
		n := m.nodes[pid]
		if n == nil || !runs(a, n) {
			continue
		}
		if id := a.id(); id != "" {
			live[id] = true
		}
	}
	return live
}

// openResume lists every conversation at rest under every place, off the
// render path, and opens the finder on that list alone. The directories
// and the live conversations are read here, where the model is; the
// transcripts are read in the command.
func (m *model) openResume() tea.Cmd {
	if m.server == nil {
		m.status, m.statusErr = "no server to hold it: "+m.serverErr, true
		return nil
	}
	type placeDirs struct {
		project Project
		dirs    []string
	}
	var places []placeDirs
	for _, p := range m.projects {
		if p.Path == globalPlace {
			continue
		}
		places = append(places, placeDirs{p, m.convoDirs(p)})
	}
	live := m.liveConversations()
	s := m.server
	return func() tea.Msg {
		type rest struct {
			project Project
			conv    conversation
		}
		var rests []rest
		seen := map[string]bool{}
		for _, pd := range places {
			for _, c := range suspendedConversations(pd.dirs, live) {
				if seen[c.ID] {
					continue
				}
				seen[c.ID] = true
				rests = append(rests, rest{pd.project, c})
			}
		}
		slices.SortStableFunc(rests, func(a, b rest) int { return byRecency(a.conv, b.conv) })
		entries := make([]finderEntry, 0, len(rests))
		for _, r := range rests {
			entries = append(entries, restEntry(r.project, r.conv))
		}
		if err := s.showRests(entries); err != nil {
			return serverErrorMsg{err: err}
		}
		return nil
	}
}

// restEntry is a conversation at rest as the finder lists it: its place
// and how long it has rested, then the last thing asked of it, its branch
// and the kind that had it.
func restEntry(p Project, c conversation) finderEntry {
	return finderEntry{Kind: "resume", Label: p.Name + " " + glyphDot + " " + shortAge(c.When),
		Dir: c.Dir, Run: resumeCommand(c),
		Facts: []segment{{`"` + cmp.Or(c.Prompt, c.Summary, c.ID) + `"`, toneQuiet},
			{c.Branch, toneQuiet}, {c.Kind, toneQuiet}}}
}

// restsPath is where the listing of every conversation at rest is kept,
// beside the finder's, for the page to read.
func restsPath() string {
	return filepath.Join(filepath.Dir(socketPath()), "rests.json")
}

// writeRests writes the listing for the page.
func writeRests(entries []finderEntry) error {
	b, err := json.Marshal(finderSnapshot{Entries: entries})
	if err != nil {
		return err
	}
	path := restsPath()
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// readRests reads it.
func readRests() (finderSnapshot, error) {
	var snap finderSnapshot
	b, err := os.ReadFile(restsPath())
	if err != nil {
		return snap, err
	}
	err = json.Unmarshal(b, &snap)
	return snap, err
}
