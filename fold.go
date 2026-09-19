package main

// The processes view at rest is quiet. A machine at work carries a tree
// under every head — the shell a contact runs, the servers the contact
// started, the build under the shell, the test under the build — and a
// panel that listed all of it was a column of rows nobody could read
// for the sixteen that mattered. What matters under a head is what can
// want you: a contact, wherever it is; a row that is a fault; a row that
// is waiting; a row that is down, which wants bringing up; and a
// service, which is a thing to reach — a port to go to, a health to
// watch — and not a step in what its compose is doing. The rest is
// what the head is doing, and is said on the head's own row. z shows
// the whole tree, for when the rest is what you are looking for.
//
// A process that listens is a service of the machine's own, and stays
// the same way: a port is a thing to reach. Where one process alone
// listens under a head, the head is the thing — npm run dev, and the
// node under it that holds the port, are one server to the operator,
// reached from the one pane — so the port is lifted onto the head and
// the listener's row folds into it; see liftListener. Where several
// listen under one head, a compose stack or a monorepo's dev, each
// stands as a row under it with its own command and port, and the
// supervisor they roll up to carries none.

// fold is the projects with every row that is only what its parent is
// doing folded into the parent: a row stays when it is a head, a
// contact, a service, a listener, a fault or waiting, and comes to
// stand under the nearest row that stayed. A shell whose rows folded
// says what it runs — the first of them that is not a shell itself, so
// a bash -c is looked through to the command it was given.
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
			if d == 0 || e.kind == kindContact || e.kind == kindService || len(e.ports) > 0 || e.fault || e.status == statusWaiting || e.status == statusDown {
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
		kept.entries = liftListener(kept.entries)
		out = append(out, kept)
	}
	return out
}

// liftListener folds a lone listener into its head. A head is any row
// kept that is not a contact and not a service: a contact files by
// what it asks of you and never by a port, and a service is a thing of
// its own. The listener is a plain process alive on a port, standing
// directly under the head in what is kept, and the only row under the
// head at any depth that has a port: a second port under the head,
// on the listener itself or beside it, is several servers, and each
// keeps its row. The head takes the ports and the sockets, and says
// whose they were, and what stood under the listener stands under the
// head.
func liftListener(rows []entry) []entry {
	drop := map[int]bool{}
	for j := range rows {
		h := &rows[j]
		if h.kind == kindContact || h.kind == kindService || len(h.ports) > 0 {
			continue
		}
		listener, ports := -1, 0
		for i := j + 1; i < len(rows) && rows[i].depth > h.depth; i++ {
			if len(rows[i].ports) == 0 {
				continue
			}
			ports++
			if rows[i].depth == h.depth+1 && plainListener(rows[i]) {
				listener = i
			}
		}
		if ports != 1 || listener < 0 {
			continue
		}
		l := rows[listener]
		h.ports, h.listener = l.ports, l.asTyped()
		h.sockets = append(append([]socket(nil), h.sockets...), l.sockets...)
		drop[listener] = true
		for i := listener + 1; i < len(rows) && rows[i].depth > rows[listener].depth; i++ {
			rows[i].depth--
		}
	}
	if len(drop) == 0 {
		return rows
	}
	out := make([]entry, 0, len(rows)-len(drop))
	for i, e := range rows {
		if !drop[i] {
			out = append(out, e)
		}
	}
	return out
}

// plainListener says whether a row is a process alive on a port and
// nothing else the panel keeps a row for: not a contact, not a
// service, not a fault, and not waiting.
func plainListener(e entry) bool {
	switch {
	case e.kind == kindContact, e.kind == kindService, e.fault, e.status == statusWaiting:
		return false
	}
	return len(e.ports) > 0
}
