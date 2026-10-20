# conn

A console for the processes working your projects. conn puts the
project you are in on one panel, a row a process, the one you choose
in the workspace beside it, and under them a line for every other
project with something running, so what is waiting on you is never
out of sight.

```
   conn  ~/projects/w0zro/conn

 ⣾ ❝ bring the code to the manual          262k
▌  ❝ a grammar over processes               118k
 ⣿ ○ demo                                 :8123
   ❯ zsh
   ✓ test                                    4m
   ✗ lint                                    2m
   ◌ postgres                             :5432

   projects ──────────────────────────────────
   web                            1 waits · 2 runs
   notes                                    1 runs
```

```
curl -fsSL https://conn.w0zro.com/install.sh | sh
```

macOS and Linux. Needs `tmux` and `git`; `lsof` for the ports and
the servers outside conn; `claude` for conversations.

conn brings up a tmux server of its own, on a console of the machine
and the start-up checks; any key goes on. conn is the panel on the
left; the workspace is the rest of the window, and shows the process
under the cursor. `conn` opens on your projects: every git checkout
under your home, the newest work first, read from where Claude Code
has worked and where conn's own panes are. Typing finds one, or names
a directory by path; `enter` opens it. A project's panel is what runs
in it: conversations with Claude Code, the stack, shells, the checks,
and the servers outside conn the project talks to, each marked for
what it is. `enter` puts the row's process in the workspace and the
keys in it. `ctrl-space` brings the keys back from inside any process;
pressed on the panel, it goes to the other process. `tab` goes to the
conversation that has waited longest. `esc` goes back into the process
you were in. `-` goes up to the projects. `q` detaches, and everything
keeps running.

The verbs are `s` start, `x` stop, `r` restart and `a` hand to an
agent, and each waits for its object, as an operator does in vim: the
verb again is the row (`xx`, `rr`, `aa`), `s` the stack, `t` the
checks, `f` what failed, `p` the project, `c` a conversation. `ss`
brings the stack up and `st` runs the checks as they were last run
here, listing them first: `x` takes a line off for good, `enter`
runs what is left, `esc` runs nothing; where nobody has run them yet, Claude Code is asked what they
are and the answer is offered on one key. `sc` starts a conversation
and `rc` picks one back up. `c` talks to a row's process: a
conversation or a shell is gone into, a web server opens in the
browser, anything else is asked of Claude Code once and the answer
kept. `!` runs a command, or opens a shell with nothing typed. `/`
searches what the project's processes have written. `?` lists the
keys.

Agents reach the same verbs from a shell, and are told so when conn
starts them: `conn ls`, `conn l web`, `conn u test -w -- go test ./...`,
`conn x :3000`, `conn r api`; `conn down` takes the server and everything
in it down. What is started under a name through
conn is written down as what the project runs: what waited is a check,
the rest the stack, and the keys run them again as they were run. A
run under no name, `conn u -w -- node --test a.test.mjs`, is a row
while it runs and is not kept.

`man conn` is the reference. The operating manual is at
[conn.w0zro.com/manual](https://conn.w0zro.com/manual/).

---

Notes for working on the tree. Nothing here that the tree says for
itself.

## SECTION 1. GENERAL

**1-1.** conn is a console for one operator directing many processes
on one machine: a panel that lists the processes working the open
project, and a workspace holding the one process the operator is in,
on a tmux server conn starts and holds. It is one Go binary with no
cgo, for macOS and Linux. The words are the code's: project, process,
conversation, row, place (conversations, the stack, clients, checks,
outside conn, elsewhere), role (stack, check, client), panel,
workspace, hold.

**1-2.** The program is one package at the root, with three under
`internal/`: `shell` quotes a word for a shell line, `theme` is the
sheets conn ships and the theme it writes for Claude Code, `tmux` is
how conn talks to its server. A file is named for the thing it is
about, and its head comment says what that thing is and why it is
done the way it is; read the head before the code. The machine is read
through `lsof`, `ps` and `git` on both platforms. `tools/revisions`
writes the manual's record of revisions from the tags. `docs/` is what
conn.w0zro.com serves: the front page, the manual under
`docs/manual/`, and `install.sh`. `.devcontainer/` brings up Go with
tmux and golangci-lint for a Codespace.

**1-3.** There are two texts. `man/conn.1` is the reference, written
by hand in roff and shipped in the release archive beside the binary,
where `install.sh` puts it for `man`. Nothing holds it to the binary;
a key or a command added is a row added by hand. `?` inside conn lists
the keys of the screen it is pressed on, and a test holds every key on
the bar to that list. `docs/manual/index.html` is the operating
manual: how conn is worked, on the site's pages. It is held to nothing
by a test, and is read again when a section it describes changes.

## SECTION 2. BUILDING AND TESTING

**2-1.** `make build` produces `./conn` the way a release does:
trimmed and stripped. The version `git describe` says is passed to the
linker, and nothing in the binary reads it since the tree was cut
back; there is no `conn --version`. `make test` runs `go vet` and the
tests under the race detector. `make lint` checks formatting and runs
golangci-lint for this platform and for Linux. The format check, vet,
tests and lint run on every push and pull request, the tests on macOS
and Linux both.

**2-2.** The tests need a real tmux on the path. They bring up servers
of their own on scratch sockets and take them down; none touches the
operator's station. Without tmux, or under `-short`, those tests skip
and the run means less than it says. The sheets conn ships are held to
copies of their sources under `internal/theme/testdata`; refreshing a
theme is copying those again.

**2-3.** A commit says what changed and why, in prose: a sentence for
its subject, in sentence case, and paragraphs under it that explain the
decision so it can be found again. Nothing is prefixed. Each finished
step is its own commit.

## SECTION 3. RELEASING

**3-1.** A release is an annotated tag, `v0.24.0`, whose message is the
row the manual will record it by: `git tag -a v0.24.0 -m "What the
release did"`. Pushing it runs the tests and hands the build to
goreleaser, which produces `conn_<version>_<os>_<arch>.tar.gz` holding
the binary and the man page, stamped with the release, beside a
`checksums.txt`. Those names are what `install.sh` expects.

**3-2.** Once the release is out, the workflow writes the record of
revisions, stamps the man page, and pushes both to main. That push is
the one Pages rebuilds the site on. `goreleaser release --snapshot
--clean` rehearses the build locally without tagging.
