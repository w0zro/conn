package main

// The processes view at rest is quiet. A machine at work carries a tree
// under every head — the shell a contact runs, the servers the contact
// started, the build under the shell, the test under the build — and a
// panel that listed all of it was a column of rows nobody could read
// for the sixteen that mattered. What matters under a head is what can
// want you: a contact, wherever it is; a row that is a fault; a row that
// is waiting; a row that is down, which wants bringing up. The rest is
// what the head is doing, and is said on the head's own row. z shows the whole tree, for when the rest is what you
// are looking for.

// fold is the projects with every row that is only what its parent is
// doing folded into the parent: a row stays when it is a head, a
// contact, a fault or waiting, and comes to stand under the nearest row
// that stayed. A shell whose rows folded says what it runs — the first
// of them that is not a shell itself, so a bash -c is looked through to
// the command it was given.
func fold(projects []project) []project {
	out := make([]project, 0, len(projects))
	for _, pl := range projects {
		kept := project{path: pl.path, note: pl.note}
		// For each depth of the tree as read, the row it stands under in
		// what is kept: the index into kept.entries, and its depth there.
		var at []int
		var depth []int
		for _, e := range pl.entries {
			d := e.depth
			if d >= len(at) {
				at = append(at, make([]int, d+1-len(at))...)
				depth = append(depth, make([]int, d+1-len(depth))...)
			}
			parent, parentDepth := -1, -1
			if d > 0 {
				parent, parentDepth = at[d-1], depth[d-1]
			}
			if d == 0 || e.kind == kindContact || e.fault || e.status == statusWaiting || e.status == statusDown {
				e.depth = parentDepth + 1
				kept.entries = append(kept.entries, e)
				at[d], depth[d] = len(kept.entries)-1, e.depth
				continue
			}
			// Folded: what stands under it stands under what it stood
			// under, and a shell it stood under says what it runs.
			at[d], depth[d] = parent, parentDepth
			if parent >= 0 {
				p := &kept.entries[parent]
				if p.kind == kindShell && (p.under == "" || p.underShell && e.kind != kindShell) {
					p.under, p.underShell = e.asTyped(), e.kind == kindShell
				}
			}
		}
		out = append(out, kept)
	}
	return out
}
