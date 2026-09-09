# conn

conn comes up on the loop, reads out the machine, runs its start-up checks,
and calls hello from the conn; ctrl+c or q closes it. Everything it was is
in the history, and comes back piece by piece, in the form it is wanted in.

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
