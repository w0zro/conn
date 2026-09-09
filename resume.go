package main

// A conversation an agent was having does not end when its instance exits;
// it is suspended, and the transcript on disk is enough to pick it back up.
// The newest at rest under a place is a row under the place in the
// everything view and a resume row in the finder — its prompt, its age,
// its branch — and enter continues it, in a shell like any other.

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
