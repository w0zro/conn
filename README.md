# conn

A terminal UI for working on projects at the command line. conn is a tmux
client: `conn` brings up a tmux server of its own under its own
configuration, attaches the terminal, and runs the navigator down the left
of the home window. The shell under the navigator's cursor is the tmux pane
beside it; the navigator lists, finds, starts and kills what runs in every
project.

## Install

```sh
curl -fsSL https://conn.w0zro.com/install.sh | sh
```

That fetches the build for this machine, checks it against the release's
checksums, and puts it in `~/.local/bin`, with the manpage under
`~/.local/share/man` — `man conn` is the reference. `CONN_INSTALL_DIR`
says where else to put the binary, `CONN_MAN_DIR` the manpage, and
`CONN_VERSION` names a release other than the latest. Once conn is
running it keeps itself current: a newer release is offered on the status
line, and `U` installs it from inside the window.

Builds are published for macOS and Linux, on both arm64 and amd64, and the
test suite runs on both. conn needs `tmux` installed, which holds and draws
the shells — and on macOS `lsof`, which is how the process list is read
there; Linux keeps its processes on `/proc`, and conn reads them off it.

## Build it yourself

```sh
go install github.com/w0zro/conn@latest
```

## The manual

The site is the manual, one page of HTML at `docs/index.html`, and the
manpage is that text in roff: `go run ./tools/man` writes `man/conn.1`
from it, and a test holds the two together. Edit the manual, run the
tool, commit both. The appendix of keys is held to the `?` popup the same
way, and the table of commands to the words the chords run.

## Releasing

Pushing a `v*` tag runs `.github/workflows/release.yml`, which tests on macOS,
cross-compiles the four builds, and publishes them with a `checksums.txt` that
`install.sh` reads. The script is served from `docs/`, with the site, which
GitHub Pages rebuilds on a push to main — but not on a push that carries a
tag along with it, so main goes first, and the tag on its own. The tag is
annotated, and its message is what the manual's record of revisions says
of the release: once the release is out, the workflow writes the row,
rewrites the manpage under the new stamp, and pushes both to main.

```sh
git push origin main
git tag -a v0.4.0 -m "Endings, tasks and the transcript" && git push origin v0.4.0
```

`go run ./tools/revisions` writes the same row by hand, from the tags.

## License

MIT; see [LICENSE](LICENSE).
