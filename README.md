# conn

conn comes up on the loop, reads out the machine, runs its start-up checks,
and gives its verdict; a key continues to the watch, which is what is
running, by place: the processes of yours with a terminal, one standing for
each piece of work, grouped under the repository it is working in.

conn holds a tmux server of its own, on a socket under `~/.local/state/conn`
(or where `CONN_SOCKET` says), and the terminal is on it while conn is up.
Its home window is a rail on the left, which is the watch, and a slot on
the right, which is the process reached from it: `s` opens a shell at the
place under the cursor and puts it in the slot, `enter` puts the process
under the cursor there, and what leaves the slot goes back to a window of
its own, out of sight, where it keeps running. One chord, `ctrl-space -`
(`CONN_PREFIX` names another prefix, in tmux's spelling), puts focus on
the watch from anywhere in the server; tmux's own keys are unbound, so
none of them are reachable through conn. `q` detaches,
and the server keeps everything in it for the next `conn`; `conn down`
takes the server down with everything in it, and says what went, the way
`docker compose down` does. Without tmux, conn
shows the console and the watch and reaches nothing. On macOS the watch needs `lsof`, which is how a process's working
directory is read there; Linux keeps it on /proc. Everything else conn was
is in the history, and comes back piece by piece, in the form it is wanted
in.

## Build it yourself

```sh
go install github.com/w0zro/conn@latest
```

## The manual

The site is the manual, one page of HTML at `docs/index.html`, and the
manpage is that text in roff: `go run ./tools/man` writes `man/conn.1`
from it, and a test holds the two together. Edit the manual, run the
tool, commit both.

## Releasing

Pushing a `v*` tag runs `.github/workflows/release.yml`, which tests,
cross-compiles the builds, and publishes them with a `checksums.txt` that
`install.sh` reads. The script is served from `docs/`, with the site, which
GitHub Pages rebuilds on a push to main. The tag is annotated, and its
message is what the manual's record of revisions says of the release;
`go run ./tools/revisions` writes the row from the tags.

## License

MIT; see [LICENSE](LICENSE).
