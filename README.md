# conn

Operating notes for conn. The manual proper is the site, `docs/index.html`;
this document records what conn does and why it was decided, section by
section, so that a decision can be found again.

## SECTION 1. GENERAL

**1-1. PURPOSE.** conn is the station from which one operator directs the
work of many hands on one machine. A hand is a process that works a place
on the operator's behalf: a shell, an agent, a run. Hands work without
direction, and stop to ask for it. The station shows what each hand is
doing and which hand is waiting, so that the operator's attention, which
is the one thing not in supply, goes where it is asked for.

**1-2. THE HANDS.** The kernel's word is process, and it is too small for
the thing being run. The word here is hand, and where that could be read
as the operator's own, station hand: the general worker on a station,
who does what the place needs without being stood over. A hand is
long-lived. It has a place it works, a
tree of processes under it, and, if it is an agent, a conversation it is
carrying and a state of mind that cannot be read off its processor time.
Above all it can stop and wait on the operator. A compiler never waited
on anyone. That one new fact is why the hands are the thing to watch, and
why the question is no longer whether a thing is done but whether it
needs you.

**1-3. FILES.** Tooling grew up around files and the things done to them,
because the person did the work and a process was a brief event, run and
watched to its end. The files are still where the work lands. They are
now what the hands are working on, the way they used to be what the
operator was working on, and the operator goes to them to review what a
hand did or to read before answering it. An editor is a place visited
from the station, not the place the operator lives.

**1-4. THE TWO QUESTIONS.** Everything on the screen answers one of two
questions the operator keeps asking: what is going on, and who needs me.
The watch answers the first. The waiting word, its blink, `tab`, and the
bar's lamps answer the second. Every other rule in this document, the
order that holds still, the words that are earned, the panel that stays
dark, the one orange, exists so that those two answers can be taken in at
a glance rather than read.

**1-5. THE NAME.** The conn is the Navy's word for control of a ship's
movement. The officer who has it does not work the ship; the hands do.
The officer receives reports and gives orders. conn is named for that
seat and not for the machine.

**1-6. WHAT IS SAID.** conn says only what it knows to be true, in a word
it has earned, and says nothing where it knows nothing. A guess is worse
than silence. Nothing is said twice and nothing is said of what there is
none of. A row that always says something is a row nobody reads.

## SECTION 2. COMING UP

**2-1. THE CONSOLE.** conn comes up on the console. It reads out the
machine, runs its start-up checks, and gives its verdict. Any key
continues to the watch. The console holds until the first reading of the
watch is in hand rather than putting up an empty watch and filling it:
the console is a still page, and a moment more of it is not seen, where a
watch assembling itself would be.

**2-2. WITHOUT TMUX.** Without tmux, conn shows the console and the watch
and reaches nothing. On macOS the watch needs `lsof`, which is how a
process's working directory is read there; Linux keeps it on /proc.

## SECTION 3. THE WATCH

**3-1. WHAT IT SHOWS.** The watch is what is running, by place: the
operator's processes with a terminal, each standing for its own work,
grouped under the project it works in. A project is a git repository, or,
under the roots where checkouts are kept, the folder that holds several,
for work that is beside the checkouts rather than in one. Everything
below a project is in it; conn's `docs` directory is conn's work, not a
place of its own.

**3-2. TITLES.** A place is titled by what tells it apart from the others,
which is what is left of its path once the root is taken off:
`~/projects/w0zro/conn` is `w0zro/conn`. The root is the same for every
place and would otherwise be read again at the head of every block, on a
rail forty-four columns wide. A place outside every root is written from
`~` and whole. There is nothing shared to take off it, and where it is is
the only thing its line has to say.

**3-3. THE TREE.** Under a place the hands read as the tree they are: a
shell, the agent it runs indented under it, the shell that agent asked for
under that, and its `go test` under that again. A whole tree belongs to
one place, the directory its root stands in, whatever a process below it
has since changed directory to. A shell is idle only bare, at its prompt.
Running anything, however far down, it is active the way what it runs is.

**3-4. ORDER.** Everything sits where it started and stays there for as
long as it lives: a place by the work that first began there, a tree by
its own root, a row among its siblings by itself, oldest first. What is
new goes on the end, so nothing above it moves and a row read twice is in
the same spot. Sorting by the newest work was tried and rejected. It is a
true thing to say about a list and a hard one to read: an agent running a
command a second re-sorted the whole watch under the eye trying to follow
it.

**3-5. WORKING AND ACTIVE.** A row reads `WORKING` when it was doing
something between one reading and the next, and `ACTIVE` when it is only
alive. A dev server waiting on a request and one answering it are not the
same thing, and the column says which.

An agent is asked rather than measured. It knows whether it is mid-turn,
and the processor time it uses says little either way: a model answering
spends barely any, waiting on the operator spends none, and an agent
sitting on a test suite of its own spends none while the suite spends
plenty. That last is work conn could not see any other way, so an agent
with a command running under it is working, the same as one mid-turn.

Everything else is read off the processor time it spent, against how long
there was to spend it in. That span is one conn watched: the difference
since its last reading, or, for a process born since, the short life it
has had. A process older than that reading is not judged by a life conn
was not there for, which is why the first reading after conn starts calls
nothing working. Otherwise a dev server that compiled for two seconds an
hour ago would come up `WORKING` and go quiet a beat later. Work is not
claimed up the tree: a shell whose child is working is active, and the
row doing the work is the one that says so.

**3-6. WAITING.** `WAITING` is the other end of the same question, and the
one word on the watch that asks something of the operator. It is an agent
stopped on something it put to you and cannot go on without: a
permission, a question, a dialog sitting there unanswered. It blinks,
which is the one thing on a screen that reaches the corner of an eye.
Reading down a list of rows that all say something, the row that wants
you is the row that moves. On the dark half its cells are the ground and
nothing around them moves, the way the console's verdict goes dark; a
word that jumped its neighbours about would be worse than one that never
blinked.

Nothing else on the watch blinks. A fault wears a chip and keeps it, since
a process you suspended yourself is not asking anything of you. An agent
whose turn is simply over is `IDLE`, the same word a shell at its prompt
gets and for the same reason: at rest, nothing pending, yours when you
want it. The difference is whether anything is held up, and only the
thing that is held up is worth a word that carries.

Nothing but an agent is ever called waiting. A server waiting on a socket
is waiting on the socket, so the word is only ever about a person. It is
no fault, so it takes a color of its own rather than a chip. Only an agent
says any of this, being the only thing that knows its own mind. An agent
conn cannot ask, another maker's, one too old to say, or one newer than
conn and using a word conn has never heard, reads as alive like anything
else. Guessing at the word is worse than having none: `IDLE` says at
rest, nothing pending, yours when you want it, and none of that is known.

**3-7. TAB.** `tab` goes to what is waiting on you: the first press to the
agent that has been held up longest, each after it to the next, and round
again from the end. An agent says when its status became what it is, and
Claude writes that on a change rather than on a clock, so the stamp is
the moment the wait began and the order is how long each has actually
waited. One that cannot say goes last. It is still waiting, which is what
the word is for, but it cannot claim a turn ahead of one that can prove
it waited longer.

**3-8. HELD AND REPORTED.** A row is also written by what conn can do with
it, and in two ways, since they are not the same kind of saying. What
conn can only report, a terminal it did not open and cannot attach to, is
dimmed, every column of it, the other columns being the quiet gray
already. What conn holds a pane for is in the ink, a row `enter` can put
in the slot. Outside its server conn holds nothing and dims nothing, the
distinction there being every row.

**3-9. THE SLOT'S MARK.** The row in the slot gets a mark rather than a
tier: the kind of the head of what is in it, in the orange, and nothing
else of the row. One cell is all a mark needs, a row is a lot of orange,
and the status column is not the orange's to take. `WAITING` is already a
color near enough to it that the two together would say neither. The row
under the cursor gives a rank of the dimming back rather than the
reading, faint on the cursor's raised ground being barely there at all.

**3-10. REACHING A TREE.** A pane holds a whole tree, so `enter` on a row
down inside one reaches the head, a pane being the only thing there is to
attach to, and the cursor goes to the head with it. The row that asked is
not the row that answered, and a cursor left where it was would pick out
the one row in the pane that is not what is in the slot.

## SECTION 4. THE LOOK

**4-1. OPENING AND CLOSING.** `i` opens the look in the slot and `i` again
takes it away. It stands beside the watch rather than over it, so the row
it is about stays on screen under the cursor. It then follows that
cursor: `j` and `k` walk the list and the page walks with them, and `tab`
carries it to whatever is waiting on you. One page serves the whole list,
which is why `i` is pressed once and not once a row, and why the second
press is free to mean close.

Where the row is one conn holds, closing goes to it: the page is a
reading of that row and the row is right there in a pane, so read about
it and then be in it. What cannot be reached closes to the hold an empty
slot has instead. Closing wants no row under the cursor at all. The page
is there whatever the cursor is on, and refusing to shut it because the
watch had emptied would leave it stuck. The look is conn's own program in
a pane of the server, `conn look`, the way the hold is, so the slot holds
it like anything else and reaching a real process is rid of it. Focus
stays on the rail: the page is a reading, not a place to be put, and
every key that works the watch is over there.

**4-2. WHAT IT SAYS.** A row of the watch is six columns on a rail, and
most of what conn reads of a process does not fit in that. It is dropped
rather than shortened, which is right for the watch and leaves the
dropped part said nowhere. The look is where it is said.

- The whole command, as it was written rather than in conn's upper case,
  since it is a thing somebody might retype.
- The directory the process is in, when that is not the place its tree
  belongs to.
- What the process table calls it, and whether its group holds the
  terminal.
- How long it has been up and how much processor time it has actually
  spent, which is the measure behind `WORKING`.
- What runs it, and what it runs.
- Of an agent, which conversation it is carrying: the session, the
  branch, the last thing it was asked. Stopped on you, what it is stopped
  on: `input needed`, `dialog open`, a `sandbox request`, the one thing
  the watch has no column wide enough for. That group is written first,
  ahead of what the row even is, since the page is cut off at the pane's
  height rather than scrolled and what must not be lost goes at the top.
- Of the place, what git says of it: the branch, whether the tree is
  clean, the last commit, how far it stands from what it tracks. A row
  stands for work, and the work is in a repository.

Nothing is said twice and nothing is said of what there is none of. A
shell has no conversation and gets no agent heading, a place that is no
repository has no git to report, and a status with no moment behind it is
not dated. The form is the console's own, a label, a dotted leader, a
value, grouped under a title, because the console says what the machine
is and the look says what one row of it is, and they are the same
instrument speaking.

**4-3. HOW IT READS.** The watch and the look are separate programs in
separate panes, so the cursor travels between them as a few bytes in a
file beside the socket, written when it moves and said again on every
reading, so a file swept away comes back on the next beat. The page reads
it twenty times a second, that poll being the whole of the wait between a
key on the rail and the page changing, and reads the process table itself
only when the subject actually changes or the beat comes round.

It does not wait on that reading. The machine takes a tenth of a second
to read, which is long enough to see, and the table read a moment ago
holds every row of the machine rather than only the row it was read for.
So the row the cursor landed on is answered out of that at once, as true
as the rail's own list beside it, and the reading on its way says it
again newer. What git said of a place and which conversation a row was
carrying are kept along with it, since a list walked down and back up is
the same few places over and over and git is a process each time; a page
left open on one row asks git again only after half a minute, and reads
what a waiting agent wants again only when its standing changes. A
reading that lands after the cursor has moved on is dropped rather than
shown, so the page never flicks back to a row nobody is looking at; the
table it came with is kept, being a reading of the machine and not of the
row. Off the watch nothing is published. Going to the list to open
something does not unchoose the row you were reading, and the page goes
on reading it. `conn look <pid>` by hand pins the page to one process
instead.

## SECTION 5. THE SERVER, THE SLOT AND THE KEYS

**5-1. THE SERVER.** conn holds a tmux server of its own, on a socket
under `~/.local/state/conn` (or where `CONN_SOCKET` says), and the
terminal is on it while conn is up. Its home window is a rail on the
left, which is the watch, and a slot on the right, which is the hand
reached from it. The rail's width is conn's, not the terminal's: conn
holds tmux to it and draws to it, so the first watch is painted in the
shape the pane is about to be and the split that opens the slot has
nothing to reflow. The server keeps everything in it for the next `conn`.
When the hand in the slot ends, the slot takes the next hand conn holds,
from the cursor down and round again from the top. Only with none to
reach does the slot hold a placard, `VACANT`, and any key in it hands the
keys back to the rail.

**5-2. REACHING.** Coming back to the watch from the console, the rows of
the last stay are still in hand, so it goes up at once with them. `s`
opens a shell at the place under the cursor and puts it in the slot, `a`
opens claude instead, and `enter` puts the hand under the cursor there.
What leaves the slot goes back to a window of its own, out of sight,
where it keeps running.

**5-3. ENDING.** `x` asks to end the cursor's entry. It arms the question
rather than the ending: the next key answers it, `x`, `y` or `enter`
confirming and anything else calling it off. A bare shell, with nothing
running in it, is killed outright; asked more gently it ignores the
signal. Anything a shell runs is asked to end on its own, leaving the
shell at its prompt rather than taking it too.

**5-4. THE CHORDS.** Four chords live under `ctrl-space`; `CONN_PREFIX`
names another prefix, in tmux's spelling.

- `ctrl-space -` puts focus on the watch from anywhere in the server.
- `ctrl-space p` puts it on the list from anywhere too, out of whatever
  you are working in and straight to the projects, without the watch in
  between.
- `ctrl-space ctrl-space` goes to the hand you were last in.
- `ctrl-space q` detaches, the same as `q` does from the watch itself,
  without first coming back to it.

The prefix twice over is the other hand. It puts back whatever was in the
slot before the thing in it now, and takes the thing in it now as the one
to come back to, so pressed twice it is where it started. It is the key
for working two things at once: a shell and the agent you are asking
about it, a build and the file it is failing on. conn's own furniture is
never somewhere you were working. A hold standing in an empty slot and
the look are not remembered, and going back never lands on one. `alt-o`
does the same from the rail, the way `alt-p` opens the list, and is what
the chord sends. tmux's own keys are otherwise unbound, so none of them
are reachable through conn.

**5-5. DOWN.** `conn down` takes the server down with everything in it,
and says what went, a line for each window and one for the server, the
way `docker compose down` does.

## SECTION 6. THE BAR

**6-1. AN ANNUNCIATOR.** Across the foot of the window, under the rail and
the slot alike, is the bar. It is tmux's status line, and it is an
annunciator panel rather than a status line: dark at rest, lit by what
would be worth turning for. It has two halves, each in a fixed place, so
the eye learns where to glance and an empty place is itself a reading.

**6-2. THE KEYS.** On the left, the keys, and only what cannot be seen
from the rail. A chord hanging is `PREFIX`, and it covers everything;
whatever you were doing, the next key is one of the four. A pane in copy
mode is `COPY`, its keys being its history's. Both are the client's
business and tmux's to know: a conn drawing in the rail knows nothing of
the client, and no amount of drawing on the rail will tell you either. A
kill armed is `CONFIRM`, which is not a state you are in but a question
waiting on you, and takes the next key whatever it is. Nothing else
lights the left. A word saying `WATCH` while you are looking at the watch
is furniture.

Each is a block of its color with the word knocked out of it, flush to
the edge of the screen. A block is not read but seen, and one that starts
where the screen starts is seen first. The chord takes the orange, which
is "you, here" everywhere in conn; copy mode the blue, being a state of
the pane rather than a thing you are doing; the question the color a
thing waiting on you is said in, so the two halves of the row speak one
language.

**6-3. THE LAMPS.** On the right, the hands: one lamp for each row of the
watch, in the watch's order, so a lamp's place on the row is a row's
place on the list. A lamp is a rank of gray while its hand is working,
the faintest ink while it is idle or merely active, keeping its place so
the lamps beside it do not shift, and the owed color, bold and blinking,
while an agent is stopped on something it asked of you. The terminal does
the blinking, so nothing here redraws on a beat; a terminal that will not
blink shows it steady, which is the same lamp less insistent. Faults stay
off the panel. A process you suspended yourself is not holding you up,
and the watch has the chip.

**6-4. WHY A PANEL.** The bar is the one instrument conn has that works on
peripheral vision. The rail cannot catch your eye, because when you are
working your eyes are in the slot and the watch is beside them unread. A
dark row that lights is seen without being looked at, and only while it
is dark the rest of the time.

**6-5. COST.** conn writes the two things only it knows, the question and
the lamps, each in an option of its own and each only when it changes:
the question on a keypress, the lamps when a reading finds a hand
standing differently from the last. Never on a beat.

## SECTION 7. THE LIST AND THE PICKER

**7-1. THE LIST.** `p` is the list: every project the roots hold, whether
anything is running in it or not. conn walks `CONN_ROOTS`, or
`~/projects` when it says nothing, for repositories, and the shape of
what it finds is the declaration. A folder holding two or more of them is
the project they collectively make, and gets a row of its own with its
repositories under it, by their own names; a folder of one stays flat.

The list is a line typed into, so the letters the watch is worked by are
characters there. What is typed narrows the rows, and a project answers
by its own name and by the name of the folder that groups it. `enter`
opens a shell at the row under the cursor and comes back to the watch,
where the shell shows; on a group row that is a shell at the level the
work is about, which is what the group row is for. `ctrl-a` opens claude
there instead; plain `a` is a letter to type into the filter, so the list
takes the chord `s` does not need. `esc` comes back without opening
anything. `alt-p` opens the list from anywhere, from the list itself, the
picker, the console, since `p` only means the list on the watch, where it
is a key rather than a letter being typed or one of the any-keys that
leave the console. It is what the prefix chord sends.

**7-2. THE PICKER.** A conversation does not end when claude exits; the
transcript it leaves is enough to pick it back up. `alt-a`, on the watch
or the list, a group's own row and all, opens a picker over what is
suspended at that place, beside `ctrl-a`'s own chord for a fresh
conversation. Newest first, each answering by its branch, the last thing
it was asked, or where it was had. It is a line typed into like the list.
`enter` continues the one under the cursor in a shell running `claude
--resume`, and `esc` comes back without continuing anything. A
conversation a live instance is already carrying is left off, checked
against the process table rather than trusted on the file's word alone.

## SECTION 8. COLORS

**8-1. THE SCHEME.** Every pane of the server is drawn in conn's scheme:
the ground and the ink, the cursor in the orange, and the sixteen colors
a program asks for by name. conn comes up on one of two grounds, dark or
light. The first time a server rises it asks the terminal for its own
background and keeps that answer for the server's life, so later attaches
do not each ask again. A terminal that says nothing, or nothing conn can
read, stays dark, which is what conn was before there was a light to ask
for. conn does not follow the system after.

**8-2. SAYING THE GROUND.** `conn --light` and `conn --dark` say the
ground instead of asking the terminal, ahead of a command name if there
is one (`conn --light theme claude`). Either says it whenever it is
given: a server already up is put on the other ground where it stands, no
`conn down` in between. Every pane takes the new sixteen, the rail and a
hold in the slot come back drawn on the new ground, and a pane with work
in it keeps the colors it writes itself; what it asks for by name it gets
from the new sixteen like everything else.

**8-3. THEMES.** A program that writes its own hex asks for none of the
sixteen, and tmux passes those through untouched, so conn's palette
cannot reach it. There is a theme for each of those conn knows about,
written from the same table, so none of them can drift from the palette
or from the ground the server is actually on:

```sh
conn theme claude   # ~/.claude/themes/conn.json
conn theme vim      # ~/.config/nvim/colors/conn.vim
```

nvim takes its colorscheme with `colorscheme conn`. Every color in it
carries the slot it is as well as its hex, so it holds up where sixteen
is all a terminal was given, and a ground that is no slot takes the
pane's own.

`conn theme claude` writes its file, on `dark-ansi` or `light-ansi`
according to the server's own ground. The theme names a slot wherever a
token has a slot-shaped meaning, so most of it follows the pane's own
sixteen; only the grounds no slot has a name for, the washes under a diff
and the bar behind a message, are spelled out. Claude Code's syntax
coloring is not a theme's to set. It is a fixed map onto those same
sixteen, so code in a pane is conn's colors already. conn writes the file
and says where. It offers to select the theme only when Claude Code is on
one it came with, or on none; a custom theme is somebody's own doing, and
conn says what it is and leaves it. Selecting it by hand is `/theme` in a
session.

**8-4. THE ENVIRONMENT.** The server says the terminal does truecolor
twice over: `COLORTERM`, and `CLAUDE_CODE_TMUX_TRUECOLOR` for Claude
Code, which otherwise reads `TERM=tmux-256color` and paints conn's scheme
in the 256 palette, where the warm dark end of it does not exist and a
diff's washes come out the same gray whichever way the line went. `CONN`
is set in the server too, so a program can tell where it is and dress to
match for a run: `claude --settings '{"theme":"custom:conn"}'` wears
conn's colors inside conn and leaves the theme elsewhere alone.

## SECTION 9. THE RECORD

**9-1. HISTORY.** Everything else conn was is in the history, and comes
back piece by piece, in the form it is wanted in. Each commit says what
was decided and what was read off a client to confirm it.

**9-2. BUILDING.**

```sh
go install github.com/w0zro/conn@latest
```

**9-3. THE MANUAL.** The site is the manual, one page of HTML at
`docs/index.html`, and the manpage is that text in roff: `go run
./tools/man` writes `man/conn.1` from it, and a test holds the two
together. Edit the manual, run the tool, commit both.

**9-4. RELEASING.** Pushing a `v*` tag runs `.github/workflows/release.yml`,
which tests, cross-compiles the builds, and publishes them with a
`checksums.txt` that `install.sh` reads. The script is served from
`docs/`, with the site, which GitHub Pages rebuilds on a push to main. The
tag is annotated, and its message is what the manual's record of revisions
says of the release; `go run ./tools/revisions` writes the row from the
tags.

**9-5. LICENSE.** MIT; see [LICENSE](LICENSE).
