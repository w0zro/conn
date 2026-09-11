# conn

conn comes up on the loop, reads out the machine, runs its start-up checks,
and gives its verdict; a key continues to the watch, which is what is
running, by place: the processes of yours with a terminal, one standing for
each piece of work, grouped under the place it works in — the repository, or
the app or service inside it that carries a manifest of its own.

conn holds a tmux server of its own, on a socket under `~/.local/state/conn`
(or where `CONN_SOCKET` says), and the terminal is on it while conn is up. Its
home window is a rail on the left, which is the watch, and a slot on the
right, which is the process reached from it: `s` opens a shell at the place
under the cursor and puts it in the slot, `a` opens claude there instead,
`enter` puts the process under the cursor there, and what leaves the slot
goes back to a window of its own, out of sight, where it keeps running. `x`
asks to end the cursor's entry, arming the question rather than the ending:
the next key answers it, `x`, `y` or `enter` confirming and anything else
calling it off. A bare shell, with nothing running in it, is killed outright
— asked more gently it just ignores the signal — and anything a shell runs
is asked to end on its own, leaving the shell at its prompt rather than
taking it too. One chord, `ctrl-space -` (`CONN_PREFIX`
names another prefix, in tmux's spelling), puts focus on the watch from
anywhere in the server; tmux's own keys are unbound, so none of them are
reachable through conn. `q` detaches, and the server keeps everything in it
for the next `conn`; `conn down` takes the server down with everything in it,
and says what went, the way `docker compose down` does. Without tmux, conn
shows the console and the watch and reaches nothing. On macOS the watch needs
`lsof`, which is how a process's working directory is read there; Linux keeps
it on /proc. Everything else conn was is in the history, and comes back piece
by piece, in the form it is wanted in.

`p` is the list: every project the roots hold, whether anything is running in
it or not. conn walks `CONN_ROOTS` — or `~/projects`, when it says nothing —
for repositories, and the shape of what it finds is the declaration: a folder
holding two or more of them is the project they collectively make, and gets a
row of its own with its repositories under it, by their own names; a folder of
one stays flat. The list is a line typed into, so the letters the watch is
worked by are characters there: what is typed narrows the rows, and a project
answers by its own name and by the name of the folder that groups it. `enter`
opens a shell at the row under the cursor and comes back to the watch, where
the shell shows; on a group row that is a shell at the level the work is
about, which is what the group row is for. `ctrl-a` opens claude there
instead — plain `a` is a letter to type into the filter, so the list takes
the chord `s` does not need. `esc` comes back without opening anything.

A conversation does not end when claude exits: the transcript it leaves is
enough to pick it back up. `alt-a`, on the watch or the list — a group's
own row and all — opens a picker over what is suspended at that place,
beside `ctrl-a`'s own chord for a fresh conversation: newest first, each
answering by its branch, the last thing it was asked, or where it was
had. It is a line typed into like the list, `enter` continues the one
under the cursor in a shell running `claude --resume`, and `esc` comes
back without continuing anything. A conversation a live instance is
already carrying is left off, checked against the process table rather
than trusted on the file's word alone.

Every pane of the server is drawn in conn's scheme: the ground and the ink,
the cursor in the orange, and the sixteen colors a program asks for by name.
conn comes up on one of two grounds, dark or light: the first time a server
rises it asks the terminal for its own background and keeps that answer for
the server's life. `conn down` and a relaunch is how it is asked again; a
terminal that says nothing, or nothing conn can read, stays dark, which is
what conn was before there was a light to ask for.

`conn --light` and `conn --dark` say the ground instead of asking the
terminal, ahead of a command name if there is one (`conn --light theme
claude`). Either still only takes hold the first time a server rises; a
server already up keeps what it came up on regardless, the same as the
terminal's own answer would.

A program that writes its own hex instead asks for none of the sixteen, and
tmux passes those through untouched, so conn's palette cannot reach it.
There is a theme for each of those conn knows about, written from the same
table, so none of them can drift from the palette, or from the ground the
server is actually on:

```sh
conn theme claude   # ~/.claude/themes/conn.json
conn theme vim      # ~/.config/nvim/colors/conn.vim
```

nvim takes its colorscheme with `colorscheme conn`; every color in it
carries the slot it is as well as its hex, so it holds up where sixteen is
all a terminal was given, and a ground that is no slot takes the pane's
own. conn picks one ground when a server rises and holds it for that
server's life; it does not follow the system after.

`conn theme claude` writes its file, on `dark-ansi` or `light-ansi`
according to the server's own ground. The theme names a slot wherever a
token has a slot-shaped meaning, so most of it follows the pane's own
sixteen; only the grounds no slot has a name for — the washes under a
diff, the bar behind a message — are spelled out. Claude Code's syntax
coloring is not a theme's to set: it is a fixed map onto those same
sixteen, so code in a pane is conn's colors already.

conn writes the file and says where. It offers to select the theme only
when Claude Code is on one it came with, or on none; a custom theme is
somebody's own doing, and conn says what it is and leaves it. Selecting it
by hand is `/theme` in a session.

The server also says the terminal does truecolor twice over: `COLORTERM`,
and `CLAUDE_CODE_TMUX_TRUECOLOR` for Claude Code, which otherwise reads
`TERM=tmux-256color` and paints conn's scheme in the 256 palette — where
the warm dark end of it does not exist, and a diff's washes come out the
same gray whichever way the line went.

`CONN` is set in the server too, so a program can tell where it is and
dress to match for a run: `claude --settings '{"theme":"custom:conn"}'`
wears conn's colors inside conn and leaves the theme elsewhere alone.

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
