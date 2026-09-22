# conn

A console for the processes working your projects. conn puts every
process on one machine under the project it works, says what each
project wants of you, and puts the one you choose in front of you.

```
   conn ────────────────────────  WAITING
▸  ✻  claude                        9 MIN
   ❯  zsh
 ⣾ ○  go test ./...

   web ─────────────────────────  STOPPED
   ❯  zsh
   ▯  vim                         STOPPED
   ◉  postgres                      :5432
   ○  node vite                     :5173
   ○  worker                         DOWN
```

```
curl -fsSL https://conn.w0zro.com/install.sh | sh
```

macOS and Linux. Needs `tmux`, and `lsof` on macOS.

conn brings up a tmux server of its own. conn is the panel on the
left; the workspace is the rest of the window. Every shell, editor,
coding agent, dev server, container and Homebrew service working a
project under your roots is a row on the panel. `enter` puts a row's
process in the workspace and the keys in it. `ctrl-space` brings the
keys back to the panel from inside any process. `tab` goes to the
agent that has waited longest for you. `esc` goes back into the
process you were in. `q` detaches, and everything keeps running.

A project's `.conn` file declares what works it, one process a line,
so conn can say what is not running and bring it up. Claude Code
sessions are contacts: conn reads their state, shows the question a
waiting one asked, and lists the sessions left suspended at a project.

`man conn`, or `?` inside conn, is the reference. The operating
manual is at [conn.w0zro.com](https://conn.w0zro.com).

---

Notes for working on the tree. Nothing here that the tree says for
itself.

## SECTION 1. GENERAL

**1-1.** conn is a station for one operator directing many processes
on one machine: a panel that lists the processes working each project,
and a workspace holding the one process the operator is in, on a tmux
server conn starts and holds. It is one Go binary with no cgo, for
macOS and Linux. The vocabulary is fixed: process, contact, session,
project, panel, workspace, status line, page. The code still says
bay for the workspace and readout for the page.

**1-2.** The whole program is one package at the root. A file is named
for the thing it is about, and its head comment says what that thing
is and why it is done the way it is; read the head before the code. The
machine is read by a file per platform, `process_darwin.go` and
`process_linux.go`, and a function only one platform calls is unused on
the other. `tools/revisions` writes the manual's record of revisions
from the tags. `docs/` is what conn.w0zro.com serves: the manual, its
stylesheet, and `install.sh`.

**1-3.** There are two texts. `man/conn.1` is the reference, written by
hand in roff, built into the binary, and shown by `?`; it is held to the
binary by a test: its synopsis names every command and flag conn
answers to and nothing else, and its keys section names the panel key
as it is bound. A key or a command added without its row fails the
build. `docs/index.html` is the operating manual: how conn is worked,
on the site's pages, pointing at the man page for every key and word.
It is held to nothing by a test, and is read again when a section it
describes changes.

## SECTION 2. BUILDING AND TESTING

**2-1.** `make build` produces `./conn` the way a release does: trimmed,
stripped, stamped with what `git describe` says. `make test` runs
`go vet` and the tests under the race detector. `make lint` checks
formatting and runs golangci-lint for this platform and for Linux.
The format check, vet, tests and lint run on every push, the tests on
macOS and Linux both.

**2-2.** The tests need a real tmux on the path. They bring up servers
of their own on scratch sockets and take them down; none touches the
operator's station. Without tmux, or under `-short`, those tests skip
and the run means less than it says. Screens are held to files under
`testdata/`, one per view and size; a change to what a view says is a
change to its file, made on purpose.

**2-3.** A commit says what changed and why, in prose: a sentence for
its subject, in sentence case, and paragraphs under it that explain the
decision so it can be found again. Nothing is prefixed. Each finished
step is its own commit.

## SECTION 3. RELEASING

**3-1.** A release is an annotated tag, `v0.11.0`, whose message is the
row the manual will record it by: `git tag -a v0.11.0 -m "What the
release did"`. Pushing it runs the tests and hands the build to
goreleaser, which produces `conn_<version>_<os>_<arch>.tar.gz` holding
the binary and the man page, stamped with the release, beside a
`checksums.txt`. Those names are what `install.sh` expects.

**3-2.** Once the release is out, the workflow writes the record of
revisions, stamps the man page, and pushes both to main. That push is
the one Pages rebuilds the site on. `goreleaser release --snapshot
--clean` rehearses the build locally without tagging.
