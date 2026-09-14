package main

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// What git says of a project, for the readout. A row stands for work,
// and the work is in a repository; the branch it is on and whether the
// tree is clean are the first two things anyone asks of it, and neither
// is anywhere else in conn.
//
// git is a process like any other and a slow disk makes it a slow one,
// so every reading here is given a deadline and comes back empty rather
// than late. A project that is not a repository at all is the ordinary
// case for a row running somewhere else, and says nothing.

// gitStatus is a project's own state as git tells it.
type gitStatus struct {
	repo     bool // the project is a work tree at all
	branch   string
	detached bool   // on no branch, at a commit
	dirty    int    // paths changed, staged or not
	commit   string // the short hash at the head
	subject  string // what that commit said of itself
	when     time.Time
	ahead    int
	behind   int
	upstream string
	read     time.Time // when git was asked, for the readout to know when to ask again
	// Why there is no reading, where conn asked and got none: git not
	// on the machine, or git not answering in time. Blank where git
	// answered, which covers a directory that is simply no repository —
	// that is an answer, and the answer is that there is nothing here.
	problem string
}

// gitWait is how long any one git reading is given. The readout redraws
// on a tick, so a reading that does not land in time is one the next
// tick takes again rather than one anybody waits for.
const gitWait = 2 * time.Second

// readGit is what git says of a directory. Everything is read in one
// call where git will give it in one, since each is a process.
func readGit(dir string) gitStatus {
	if dir == "" {
		return gitStatus{}
	}
	var g gitStatus
	// Whether conn could ask at all. A machine with no git answers
	// nothing about any project on it, and saying nothing of a
	// repository because the tool is missing is a different silence
	// from saying nothing because there is no repository.
	if lookPath("git") == "" {
		g.problem = "NOT ON PATH"
		return g
	}

	// One call for the head: the branch, the hash, the subject and the
	// date. %D is empty on a detached head, which is how that is known.
	out, err, late := gitOut(dir, "log", "-1", "--no-color", "--format=%h%x00%s%x00%cI%x00%D")
	if late {
		g.problem = "NO ANSWER IN " + brief(gitWait)
		return g
	}
	if err != nil {
		// git answered, and the answer is that this is no repository.
		return gitStatus{}
	}
	g.repo = true
	if f := strings.Split(strings.TrimRight(out, "\n"), "\x00"); len(f) == 4 {
		g.commit, g.subject = f[0], f[1]
		g.when, _ = time.Parse(time.RFC3339, f[2])
	}
	// The branch from git itself rather than teased out of %D, which
	// names tags and remotes in the same breath.
	if b, err, _ := gitOut(dir, "symbolic-ref", "--short", "HEAD"); err == nil {
		g.branch = strings.TrimSpace(b)
	} else {
		g.detached = true
	}

	// How far the tree has moved past the head. --porcelain is one line
	// a path, staged and unstaged alike, which is the count wanted:
	// how much is not committed.
	if st, err, _ := gitOut(dir, "status", "--porcelain"); err == nil {
		for _, l := range strings.Split(strings.TrimRight(st, "\n"), "\n") {
			if l != "" {
				g.dirty++
			}
		}
	}

	// And how far it has moved from what it tracks, when it tracks
	// anything. A branch with no upstream is not behind by nothing —
	// there is nothing for it to be behind — so it says neither.
	if up, err, _ := gitOut(dir, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}"); err == nil {
		g.upstream = strings.TrimSpace(up)
		if counts, err, _ := gitOut(dir, "rev-list", "--left-right", "--count", "HEAD..."+g.upstream); err == nil {
			if f := strings.Fields(counts); len(f) == 2 {
				g.ahead, _ = strconv.Atoi(f[0])
				g.behind, _ = strconv.Atoi(f[1])
			}
		}
	}
	return g
}

// gitOut runs one git reading in a directory, under the deadline. The
// deadline is the context's: WaitDelay alone would not bound the run,
// since it only starts counting once the context is already done.
//
// It says whether it was the deadline
// that ended it rather than git itself. A git that did not answer and a
// git that answered "no repository here" both come back as a failure,
// and the readout has to tell the reader which it was.
func gitOut(dir string, args ...string) (out string, err error, late bool) {
	ctx, cancel := context.WithTimeout(context.Background(), gitWait)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	b, err := cmd.Output()
	return string(b), err, ctx.Err() != nil
}
