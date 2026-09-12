# conn

conn comes up on the loop, reads out the machine, runs its start-up checks,
and gives its verdict; a key continues to the watch, which is what is
running, by project: the processes of yours with a terminal, each standing
for its own work, grouped under the project it works in. A project is a git
repository, or — under the roots where your checkouts are kept — the folder
that holds them, for work that is beside the checkouts rather than in one.
Everything below a project is in it: conn's `docs` directory is conn's work,
not a place of its own. Each is titled by what tells it apart from the
others, which is what is left of its path once the root is taken off:
`~/projects/w0zro/conn` is `w0zro/conn`. The root is the same for every one
of them and would be read again at the head of every block, on a rail
forty-four columns wide. A place outside every root is written from `~` and
whole — there is nothing shared to take off it, and where it is is the only
thing its line has to say.

Under a place they read as the tree they are: a shell, the agent it runs
indented under it, the shell that agent asked for under that, and its `go
test` under that again. A whole tree belongs to one place, the directory its
root stands in, whatever a process below it has since `cd`'d to. A shell is
idle only bare, at its prompt; running anything, however far down, it is
active the way what it runs is. Everything sits where it started and stays
there for as long as it lives: a place by the work that first began there, a
tree by its own root, a row among its siblings by itself, oldest first. What
is new goes on the end, so nothing above it moves and a row read twice is in
the same spot. It was the newest work anywhere in a tree bringing it and its
place to the top, which is a true thing to say about a list and a hard one to
read — an agent running a command a second re-sorted the whole watch under
the eye trying to follow it.

A row reads `WORKING` when it was doing something between one reading and the
next, and `ACTIVE` when it is only alive — a dev server waiting on a request
and one answering it are not the same thing, and the column says which. An
agent is asked rather than measured, since it knows whether it is mid-turn
and the processor time it uses says little either way: a model answering is
barely any, waiting on you is none, and an agent sitting on a test suite of
its own spends none either while the suite spends plenty. That last is work
conn could not see any other way, so an agent with a command running under
it is working, the same as one mid-turn. Everything else is read off the
processor time it spent, against how long there was to spend it in. That
span is one conn watched: the difference since its last reading, or — for a
process born since — the short life it has had. A process older than that
reading is not judged by a life conn was not there for, which is why the
first reading after conn starts calls nothing working. Otherwise a dev
server that compiled for two seconds an hour ago would come up `WORKING` and
go quiet a beat later. The work is not claimed up the tree either: a shell
whose child is working is active, and the row doing the work is the one that
says so.

`WAITING` is the other end of that question, and the one word on the watch
that asks something of you: an agent stopped on something it put to you and
cannot go on without — a permission, a question, a dialog sitting there
unanswered. An agent whose turn is simply over is `IDLE` instead, the same
word a shell at its prompt gets and for the same reason: at rest, nothing
pending, yours when you want it. The difference is whether anything is held
up, and only the thing that is held up is worth a word that carries.

`i` opens the look in the slot and `i` again takes it away — beside the
watch rather than over it, so the row it is about stays on screen under the
cursor. It then follows that
cursor: `j` and `k` walk the list and the page walks with them, and `tab`
carries it to whatever is waiting on you. One page serves the whole list,
which is why `i` is pressed once and not once a row — and why the second press
is free to mean close. Where the row is one conn holds, closing goes to it:
the page is a reading of that row and the row is right there in a pane, so
read about it and then be in it, and what cannot be reached closes to the
hold an empty slot has instead. Closing wants no row under the cursor at
all: the page is there whatever the cursor is on, and refusing to shut it
because the watch had emptied would leave it stuck. It is conn's own program
in a pane of the server, `conn look`, the way the hold is, so the slot holds
it like anything else and reaching a real process is rid of it. Focus stays
on the rail: the page is a reading, not a place to be put, and every key that
works the watch is over there.

The two are separate programs in separate panes, so the cursor travels
between them as a few bytes in a file beside the socket — written when it
moves and said again on every reading, so a file swept away comes back on the
next beat. The page reads it twenty times a second, that poll being the whole
of the wait between a key on the rail and the page changing, and reads the
table itself only when the subject actually changes or the beat comes round.
It does not wait on that reading: the machine takes a tenth of a second to
read, which is long enough to see, and the table read a moment ago holds
every row of the machine rather than only the row it was read for — so the
row the cursor landed on is answered out of that at once, as true as the
rail's own list beside it, and the reading on its way says it again newer.
What git said of a place and which conversation a row was carrying are kept
along with it, since a list walked down and back up is the same few places
over and over and git is a process each time. A reading that lands after the
cursor has moved on is dropped rather than shown: the page never flicks back
to a row nobody is looking at, though the table it came with is kept, being
a reading of the machine and not of the row. Off the watch nothing is
published — going to the list to open something does not unchoose the row you
were reading, and the page goes on reading it. `conn look <pid>` by hand pins
the page to one process instead.

A row of the watch is six columns on a rail, and most of what conn reads of a
process does not fit in that; it is dropped rather than shortened, which is
right for the watch and leaves the dropped part said nowhere. The look is
where it is said. The whole command, as it was written rather than in conn's
upper case, since it is a thing somebody might retype. The directory the
process is in, when that is not the place its tree belongs to. What the
process table calls it, and whether its group holds the terminal. How long it
has been up and how much processor time it has actually spent, which is the
measure behind `WORKING`. What runs it, and what it runs. Of an agent, which
conversation it is carrying — the session, the branch, the last thing it was
asked — and, stopped on you, what it is stopped on: `input needed`, `dialog
open`, a `sandbox request`, the one thing the watch has no column wide enough
for. That group is written first, ahead of what the row even is, since the
page is cut off at the pane's height rather than scrolled and what must not
be lost goes at the top. And of the place, what git says of it: the branch,
whether the tree is clean, the last commit, how far it stands from what it
tracks — a row stands for work, and the work is in a repository.

Nothing is said twice and nothing is said of what there is none of: a shell
has no conversation and gets no agent heading, a place that is no repository
has no git to report, and a status with no moment behind it is not dated. It
is the console's own form — a label, a dotted leader, a value, grouped under
a title — because the console says what the machine is and the look says what
one row of it is, and they are the same instrument speaking.

`tab` goes to what is waiting on you: the first press to the agent that has
been held up longest, each after it to the next, and round again from the
end. An agent says when its status became what it is, and Claude writes that
on a change rather than on a clock, so the stamp is the moment the wait began
and the order is how long each has actually waited. One that cannot say goes
last — still waiting, which is what the word is for, but it cannot claim a
turn ahead of one that can prove it waited longer.

Nothing but an agent is ever called waiting here — a server waiting on a
socket is waiting on the socket — so the word is only ever about a person. It
is no fault, so it takes a color of its own rather than a chip. Only an agent
says any of this, being the only thing that knows its own mind; an agent conn
cannot ask — another maker's, one too old to say, or one newer than conn and
using a word conn has never heard — reads as alive like anything else.
Guessing at the word is worse than having none: `IDLE` says at rest, nothing
pending, yours when you want it, and none of that is known.

A row is also written by what conn can do with it, and in two ways, since
they are not the same kind of saying. What conn can only report — a terminal
it did not open, and cannot attach to — is dimmed, every column of it, the
other columns being the quiet gray already; what conn holds a pane for is in
the ink, a row `enter` can put in the slot. Outside its server conn holds
nothing and dims nothing, the distinction there being every row.

The slot gets a mark rather than a tier: the kind of the head of what is in
it, in the orange, and nothing else of the row. One cell is all a mark needs,
a row is a lot of orange, and the status column is not the orange's to take —
`WAITING` is already a color near enough to it that the two together say
neither. The row under the cursor gives a rank of the dimming back rather
than the reading, faint on the cursor's raised ground being barely there at
all.

A pane holds a whole tree, so `enter` on a row down inside one reaches the
head — a pane is the only thing there is to attach to — and the cursor goes
to the head with it. The row that asked is not the row that answered, and a
cursor left where it was would pick out the one row in the pane that is not
what is in the slot.

Across the foot of the window, under the rail and the slot alike, is the
bar. It is tmux's status line, and it is an annunciator panel rather than a
status line: dark, saying nothing at all, until something lights it. Its
ground is the window's own, so at rest there is nothing there to tell the row
from the padding around the client.

A row that always says something is a row nobody reads, and most of what a
status line carries — where the keys are, who you are, what time it is — you
already know or do not need. What conn puts there is what you would want to
be interrupted for, and nothing else ever. On the left are lamps for what the
keys are doing, which is the half conn cannot see from inside its own pane:
`PREFIX` while a chord hangs, `COPY` in a pane in copy mode, `CONFIRM` while
a kill waits on its second key. Beside them, whatever conn has to say — the
note that went wrong reaching something, the question a kill asks — which
stands until the next key. On the right is the one question conn asks of you:
`WAITING`, the count when there is more than one, and how long the one held
up longest has waited, in its largest unit alone. A figure read from the
corner of the eye should hold still, and the watch has it to the second when
you turn to deal with it.

The bar is the only instrument conn has that works on peripheral vision. The
rail cannot catch your eye: when you are working your eyes are in the slot,
and the watch is beside them unread. A dark row that lights is seen without
being looked at, which is what the row is worth — and it is only worth it
while the row is dark the rest of the time. The waiting lamp blinks, since a
lamp that blinks is the one thing on a screen that reaches the corner of an
eye. The blinking is the clock's: the lamp is lit on an odd second and dark
on an even one, which tmux works out for itself — the status line is run
through `strftime` before the conditionals in it are read, so the format can
ask what second it is. The blink attribute was the obvious way and it does
not work: the terminfo advertises blink, tmux duly sends it, and a terminal
is free to draw it steady, as Ghostty does.

Two rows the rail used to spend on itself are the bar's: the foot, which held
a note until the next key, and the right-hand side of the head, which held
the station and the clock. Both were the session's business rather than the
list's, and the rail's forty-four columns are all list now.

conn holds a tmux server of its own, on a socket under `~/.local/state/conn`
(or where `CONN_SOCKET` says), and the terminal is on it while conn is up. Its
home window is a rail on the left, which is the watch, and a slot on the
right, which is the process reached from it. The rail's width is conn's, not
the terminal's: conn holds tmux to it and draws to it, so the first watch is
painted in the shape the pane is about to be and the split that opens the
slot has nothing to reflow. The console holds until that first reading is in
hand rather than putting an empty watch up and filling it — the console is a
still page, and a moment more of it is not seen where a watch assembling
itself is. Coming back to the watch from the console the rows of the last
stay are still in hand, so it goes up at once with them: `s` opens a shell at
the place under the cursor and puts it in the slot, `a` opens claude instead,
`enter` puts the process under the cursor there, and what leaves the slot
goes back to a window of its own, out of sight, where it keeps running. `x`
asks to end the cursor's entry, arming the question rather than the ending:
the next key answers it, `x`, `y` or `enter` confirming and anything else
calling it off. A bare shell, with nothing running in it, is killed outright
— asked more gently it just ignores the signal — and anything a shell runs
is asked to end on its own, leaving the shell at its prompt rather than
taking it too. Three chords live under `ctrl-space` (`CONN_PREFIX` names
another prefix, in tmux's spelling): `ctrl-space -` puts focus on the
watch from anywhere in the server, `ctrl-space p` puts it on the list from
anywhere too — out of whatever you are working in and straight to the
projects, without the watch in between — and `ctrl-space q` detaches, the
same as `q` does from the watch itself, without first coming back to it.
tmux's own keys are otherwise unbound, so none of them are reachable
through conn. The server keeps everything in it for the
next `conn`; `conn down` takes the server down with everything in it,
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
`alt-p` opens the list from anywhere — from the list itself, the picker, the
console — since `p` only means the list on the watch, where it is a key
rather than a letter being typed or one of the any-keys that leave the
console. It is what the prefix chord below sends.

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
the server's life, so later attaches do not each ask again. A terminal that
says nothing, or nothing conn can read, stays dark, which is what conn was
before there was a light to ask for.

`conn --light` and `conn --dark` say the ground instead of asking the
terminal, ahead of a command name if there is one (`conn --light theme
claude`). Either says it whenever it is given: a server already up is put on
the other ground where it stands, no `conn down` in between. Every pane takes
the new sixteen, the rail and a hold in the slot come back drawn on the new
ground, and a pane with work in it keeps the colors it writes itself — what
it asks for by name it gets from the new sixteen like everything else.

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
