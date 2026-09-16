# conn

Notes for working on conn. The manual is the site, `docs/index.html`,
and it says what conn does for the operator. This document says how
the tree is built, tested, and released, and nothing the tree already
says for itself.

## SECTION 1. GENERAL

**1-1.** conn is a station for one operator directing many processes
on one machine: a panel that lists the processes working each project,
and a workspace holding the one process the operator is in, on a tmux
server conn starts and holds. It is one Go binary with no cgo, for
macOS and Linux. The vocabulary is fixed: process, contact, session,
project, panel, workspace, status line, readout. The code still says
bay for the workspace.

**1-2.** The whole program is one package at the root. A file is named
for the thing it is about, and its head comment says what that thing
is and why it is done the way it is; read the head before the code. The
machine is read by a file per platform, `process_darwin.go` and
`process_linux.go`, and a function only one platform calls is unused on
the other. `tools/man` writes the man page from the manual and
`tools/revisions` writes the manual's record of revisions from the
tags. `docs/` is what conn.w0zro.com serves: the manual, its
stylesheet, and `install.sh`.

**1-3.** The manual is the one text. `man/conn.1` is written from it by
`go run ./tools/man`, built into the binary, and shown by `prefix ?`;
it is never edited by hand. A test fails when the page in the tree is
not the manual's. Two more hold the manual to the binary: every command
and flag conn answers to is a row of the synopsis table and nothing
else is, and every chord bound under the prefix is a row of the chord
table and nothing else is. A key or a command added without its row
fails the build.

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
the binary and the man page, beside a `checksums.txt`. Those names are
what `install.sh` expects.

**3-2.** Once the release is out, the workflow writes the record of
revisions and the man page and pushes both to main. That push is the
one Pages rebuilds the site on. `goreleaser release --snapshot --clean`
rehearses the build locally without tagging.
