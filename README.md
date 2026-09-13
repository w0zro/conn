# conn

Operating notes for conn. The manual proper is the site, `docs/index.html`;
this document records what conn does and why it was decided, section by
section, so that a decision can be found again.

## SECTION 1. GENERAL

**1-1. PURPOSE.** conn is the station from which one operator directs the
work of many processes on one machine. A process works a project on the
operator's behalf: a shell, a contact, a run. Processes work without
direction, and stop to ask for it. The station shows what each process is
doing and which process is waiting, so that the operator's attention,
which is the one thing not in supply, goes where it is asked for.

**1-2. THE PROCESSES.** The kernel's word is process, and it is the word
here. What has changed is not the word but the thing: one kind of process
now has a mind at the other end. A contact is a process connected to a
non-human intelligence, Claude Code in a terminal, and the station only
ever sees the contact: the process, the tree under it, the transcript it
writes. A process is long-lived. It has a project it works, a tree of
processes under it, and, if it is a contact, a session it is carrying and
a state of mind that cannot be read off its processor time. Above all it
can stop and wait on the operator. A compiler never waited on anyone.
That one new fact is why the processes are the thing to watch, and why
the question is no longer whether a thing is done but whether it needs
you.

**1-3. FILES.** Tooling grew up around files and the things done to them,
because the person did the work and a process was a brief event, run and
watched to its end. The files are still where the work lands. They are
now what the processes are working on, the way they used to be what the
operator was working on, and the operator goes to them to review what a
process did or to read before answering it. An editor is somewhere
visited from the station, not where the operator lives.

**1-4. THE TWO QUESTIONS.** Everything on the screen answers one of two
questions the operator keeps asking: what is going on, and who needs me.
The processes view answers the first. The waiting word, its blink and
`tab` answer the second. Every other rule in this document, the order
that holds still, the words that are earned, the status line that stays
dark, the one orange, exists so that those two answers can be taken in at
a glance rather than read.

**1-5. THE NAME.** The conn is the Navy's word for control of a ship's
movement. The officer who has it does not work the ship; the crew does.
The officer receives reports and gives orders. conn is named for that
seat and not for the machine.

**1-6. WHAT IS SAID.** conn says only what it knows to be true, in a word
it has earned, and says nothing where it knows nothing. A guess is worse
than silence. Nothing is said twice and nothing is said of what there is
none of. A row that always says something is a row nobody reads.

## SECTION 2. COMING UP

**2-1. THE CONSOLE.** conn comes up on the console. It reads out the
machine, runs its start-up checks, and gives its verdict. Any key
continues to the processes view. The console holds until the first
reading of the processes is ready rather than putting up an empty view
and filling it: the console is a still page, and a moment more of it is
not seen, where a view assembling itself would be.

**2-2. THE VERDICT.** A fault is the count of them, as a chip that
blinks. With none, the verdict is what the checks came to: `ALL SYSTEMS
NOMINAL` where every one of them was read and passed, and otherwise a
count for each word they stand under, `9 NOMINAL · 1 UNCHECKED`. A check
with nothing to check against is `UNCHECKED` and one the machine would
not answer is `UNKNOWN`. Neither is a fault, and neither is a pass: conn
took no reading there, and a line that said every system was nominal
would be claiming one it never took. Both are said in the color
something waiting is said in, so the row and the count agree.

**2-3. WITHOUT TMUX.** Without tmux, conn shows the console and the
processes view and reaches nothing. On macOS the processes view needs
`lsof`, which is how a process's working directory is read there; Linux
keeps it on /proc.

## SECTION 3. THE PROCESSES VIEW

**3-1. WHAT IT SHOWS.** The processes view is what is running, by project:
the operator's processes with a terminal, each standing for its own work,
grouped under the project it works in. A project is a git repository, or,
under the roots where checkouts are kept, the folder that holds several,
for work that is beside the checkouts rather than in one. Everything
below a project is in it; conn's `docs` directory is conn's work, not a
project of its own.

**3-2. TITLES.** A project is titled by what tells it apart from the
others, which is what is left of its path once the root is taken off:
`~/projects/w0zro/conn` is `w0zro/conn`. The root is the same for every
project and would otherwise be read again at the head of every block, on
a panel forty-four columns wide. A project outside every root is written
from `~` and whole. There is nothing shared to take off it, and where it
is is the only thing its line has to say.

**3-3. THE TREE.** Under a project the processes read as the tree they
are: a shell, the contact it runs indented under it, the shell that
contact asked for under that, and its `go test` under that again. A whole
tree belongs to one project, the directory its root stands in, whatever a
process below it has since changed directory to. A shell is idle only
bare, at its prompt. Running anything, however far down, it is active the
way what it runs is.

**3-4. ORDER.** Everything sits where it started and stays there for as
long as it lives: a project by the work that first began there, a tree by
its own root, a row among its siblings by itself, oldest first. What is
new goes on the end, so nothing above it moves and a row read twice is in
the same spot. Sorting by the newest work was tried and rejected. It is a
true thing to say about a list and a hard one to read: a contact running
a command a second re-sorted the whole view under the eye trying to
follow it.

**3-5. WORKING AND ACTIVE.** A row reads `WORKING` when it was doing
something between one reading and the next, and `ACTIVE` when it is only
alive. A dev server waiting on a request and one answering it are not the
same thing, and the column says which.

A contact is asked rather than measured. It knows whether it is mid-turn,
and the processor time it uses says little either way: the intelligence
answering spends barely any, waiting on the operator spends none, and a
contact sitting on a test suite of its own spends none while the suite
spends plenty. That last is work conn could not see any other way, so a
contact with a command running under it is working, the same as one
mid-turn.

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
one word in the processes view that asks something of the operator. It
is a contact stopped on something it put to you and cannot go on
without: a permission, a question, a dialog sitting there unanswered. It
blinks, which is the one thing on a screen that reaches the corner of an
eye. Reading down a list of rows that all say something, the row that
wants you is the row that moves. On the dark half its cells are the
ground and nothing around them moves, the way the console's verdict goes
dark; a word that jumped its neighbours about would be worse than one
that never blinked.

Nothing else in the processes view blinks. A fault wears a chip and keeps
it, since a process you suspended yourself is not asking anything of you.
A contact whose turn is simply over is `IDLE`, the same word a shell at
its prompt gets and for the same reason: at rest, nothing pending, yours
when you want it. The difference is whether anything is held up, and only
the thing that is held up is worth a word that carries.

Nothing but a contact is ever called waiting. A server waiting on a
socket is waiting on the socket, so the word is only ever about a person.
It is no fault, so it takes a color of its own rather than a chip. Only a
contact says any of this, being the only thing that knows its own mind. A
contact conn cannot ask, another maker's, one too old to say, or one
newer than conn and using a word conn has never heard, reads as alive
like anything else. Guessing at the word is worse than having none:
`IDLE` says at rest, nothing pending, yours when you want it, and none of
that is known.

**3-7. TAB.** `tab` goes to what is waiting: the cursor to the contact
that has been held up longest, its pane into the bay, and the keys into
it, so one press has the operator answering. Each press after it goes to
the next, and round again from the end. A contact says when its status
became what it is, and Claude writes that on a change rather than on a
clock, so the stamp is the moment the wait began and the order is how
long each has actually waited. One that cannot say goes last. It is still
waiting, which is what the word is for, but it cannot claim a turn ahead
of one that can prove it waited longer. A waiting contact conn holds no
pane for is gone to on the panel, and the keys stay where they are.

**3-8. HELD AND REPORTED.** A row is also written by what conn can do with
it, and in two ways, since they are not the same kind of saying. What
conn can only report, a terminal it did not open and cannot attach to, is
dimmed, every column of it, the other columns being the quiet gray
already. What conn holds a pane for is in the ink, a row `enter` can put
in the bay. Outside its server conn holds nothing and dims nothing, the
distinction there being every row.

**3-9. THE BAY'S MARK.** The row in the bay gets a mark rather than a
tier: the kind of the head of what is in it, in the orange, and nothing
else of the row. One cell is all a mark needs, a row is a lot of orange,
and the status column is not for the orange to take. `WAITING` is
already a color near enough to it that the two together would say
neither. The row under the cursor gives a rank of the dimming back rather
than the reading, faint on the cursor's raised ground being barely there
at all.

**3-10. REACHING A TREE.** A pane holds a whole tree, so `enter` on a row
down inside one reaches the head, a pane being the only thing there is to
attach to, and the cursor goes to the head with it. The row that asked is
not the row that answered, and a cursor left where it was would pick out
the one row in the pane that is not what is in the bay.

## SECTION 4. THE READOUT

**4-1. OPENING AND CLOSING.** `i` opens the readout in the bay and `i`
again takes it away. It stands beside the processes view rather than over
it, so the row it is about stays on screen under the cursor. It then
follows that cursor: `j` and `k` walk the rows and the page walks with
them, and `tab` carries it to whatever is waiting on you. One page serves
the whole view, which is why `i` is pressed once and not once a row, and
why the second press is free to mean close.

Where the row is one conn holds, closing goes to it: the page is a
reading of that row and the row is right there in a pane, so read about
it and then be in it. What cannot be reached closes to the hold an empty
bay has instead. Closing wants no row under the cursor at all. The page
is there whatever the cursor is on, and refusing to shut it because the
view had emptied would leave it stuck. The readout is conn's own program
in a pane of the server, `conn readout`, the way the hold is, so the bay
holds it like anything else and reaching a real process is rid of it.
Focus stays on the panel: the page is a reading, not somewhere to be put,
and every key that works the view is over there.

**4-2. WHAT IT SAYS.** A row of the processes view is six columns on a
panel, and most of what conn reads of a process does not fit in that. It
is dropped rather than shortened, which is right for the view and leaves
the dropped part said nowhere. The readout is where it is said.

- The whole command, as it was written rather than in conn's upper case,
  since it is a thing somebody might retype.
- The directory the process is in, when that is not the project its tree
  belongs to.
- What the process table calls it, and whether its group holds the
  terminal.
- How long it has been up and how much processor time it has actually
  spent, which is the measure behind `WORKING`.
- What runs it, and what it runs.
- Of a contact, which session it is carrying: its id, the branch, the
  last thing it was asked. Stopped on you, what it is stopped on: `input
  needed`, `dialog open`, a `sandbox request`, the one thing the view has
  no column wide enough for. That group is written first, ahead of what
  the row even is, since the page is cut off at the pane's height rather
  than scrolled and what must not be lost goes at the top.
- Of the project, what git says of it: the branch, whether the tree is
  clean, the last commit, how far it stands from what it tracks. A row
  stands for work, and the work is in a repository.

Nothing is said twice and nothing is said of what there is none of. A
shell has no session and gets no CONTACT heading, a project that is no
repository has no git to report, and a status with no moment behind it is
not dated. The form is the console's own, a label, a dotted leader, a
value, grouped under a title, because the console says what the machine
is and the readout says what one row of it is, and they are the same
instrument speaking.

**4-3. HOW IT READS.** The processes view and the readout are separate
programs in separate panes, so the cursor travels between them as a few
bytes in a file beside the socket, written when it moves and said again
on every reading, so a file swept away comes back on the next beat. The
page reads it twenty times a second, that poll being the whole of the
wait between a key on the panel and the page changing, and reads the
process table itself only when the subject actually changes or the beat
comes round.

It does not wait on that reading. The machine takes a tenth of a second
to read, which is long enough to see, and the table read a moment ago
holds every row of the machine rather than only the row it was read for.
So the row the cursor landed on is answered out of that at once, as true
as the view beside it, and the reading on its way says it again newer.
What git said of a project and which session a row was carrying are kept
along with it, since a list walked down and back up is the same few
projects over and over and git is a process each time; a page left open
on one row asks git again only after half a minute, and reads what a
waiting contact wants again only when its status changes. A reading that
lands after the cursor has moved on is dropped rather than shown, so the
page never flicks back to a row nobody is looking at; the table it came
with is kept, being a reading of the machine and not of the row. Off the
processes view nothing is published. Going to projects to open something
does not unchoose the row you were reading, and the page goes on reading
it. `conn readout <pid>`, typed, pins the page to one process instead.

## SECTION 5. THE SERVER, THE BAY AND THE KEYS

**5-1. THE SERVER.** conn holds a tmux server of its own, on a socket
under `~/.local/state/conn` (or where `CONN_SOCKET` says), and the
terminal is on it while conn is up. Its home window is a panel on the
left, which shows the processes view, and a bay on the right, which holds
the process reached from it. conn sets the panel's width, not the
terminal: conn holds tmux to it and draws to it, so the first view is
painted in the shape the pane is about to be and the split that opens the
bay has nothing to reflow. The server keeps everything in it for the next
`conn`. When the process in the bay ends, the bay takes the next process
conn holds, from the cursor down and round again from the top. Only with
none to reach does the bay hold a placard, `VACANT`, and any key in it
puts the keys back on the panel.

**5-2. REACHING.** Coming back to the processes view from the console, the
rows of the last stay are still held, so it goes up at once with them.
`s` opens a shell at the project under the cursor and puts it in the bay,
`a` opens claude instead, and `enter` puts the process under the cursor
there. What leaves the bay goes back to a window of its own, out of
sight, where it keeps running.

**5-3. ENDING.** `x` asks to end the cursor's entry. It arms the question
rather than the ending: the next key answers it, `x`, `y` or `enter`
confirming and anything else calling it off. A bare shell, with nothing
running in it, is killed outright; asked more gently it ignores the
signal. Anything a shell runs is asked to end on its own, leaving the
shell at its prompt rather than taking it too.

**5-4. THE CHORDS.** Five chords live under `ctrl-space`; `CONN_PREFIX`
names another prefix, in tmux's spelling.

- `ctrl-space -` puts focus on the processes view from anywhere in the
  server.
- `ctrl-space p` puts it on projects from anywhere too, out of whatever
  you are working in and straight to the projects, without the processes
  view in between.
- `ctrl-space ctrl-space` goes to the process you were last in.
- `ctrl-space tab` goes to the contact that has waited longest, from
  anywhere, the way `tab` does in the processes view.
- `ctrl-space q` detaches, the same as `q` does from the processes view
  itself, without first coming back to it.

The prefix twice over is the other process. It puts back whatever was in
the bay before the thing in it now, and takes the thing in it now as the
one to come back to, so pressed twice it is where it started. It is the
key for working two things at once: a shell and the contact you are
asking about it, a build and the file it is failing on. conn's own
furniture is never somewhere you were working. A hold standing in an
empty bay and the readout are not remembered, and going back never lands
on one. `alt-o` does the same from the panel, the way `alt-p` opens
projects, and is what the chord sends. tmux's own keys are otherwise
unbound, so none of them are reachable through conn.

**5-5. DOWN.** `conn down` takes the server down with everything in it,
and says what went, a line for each window and one for the server, the
way `docker compose down` does.

## SECTION 6. THE STATUS LINE

**6-1. AN ANNUNCIATOR.** Across the foot of the window, under the panel
and the bay alike, is the status line, tmux's own. It is an annunciator
rather than a line of status: dark at rest, lit by what would be worth
turning for. It has two halves, each at a fixed position, so the eye
learns where to glance and an empty position is itself a reading.

**6-2. THE KEYS.** On the left, where the keys are. A chord hanging is
`PREFIX`, and it covers everything; whatever you were doing, the next key
is one of the four. A pane in copy mode is `COPY`, where the keys walk
the history instead. Only tmux knows those two: a conn drawing in the
panel knows nothing of the client, and no amount of drawing on the panel
will tell you either. A kill armed is `CONFIRM`, which is not a state you
are in but a question waiting on you, and takes the next key whatever it
is. Fourth is the panel view the keys are in, `PROCS`, `PROJECTS` or
`SESSIONS`, which conn knows and tmux does not.

The view's word was left off for a while, on the reasoning that a word
saying `PROCS` while you are looking at the processes view is furniture.
It is not. The three views are worked by different keys, and a letter
that narrows the rows in projects and sessions runs a command in
processes, so which of them has the keys is the same kind of state the
other three words say. A question armed comes first: while it stands the
view under it cannot be worked, and its word would be a lie. The position
is dark when the keys are in the bay, and dark on the console, which
covers the window and names itself.

Each is a block of the orange with the word knocked out of it, flush to
the edge of the screen. A block is not read but seen, and one that starts
where the screen starts is seen first. All four take the one color: all
four say the same fact, that the keys are here and doing this, and the
orange is what "you, here" is said in everywhere else in conn, the cursor
and the kind of the row the bay holds alike. The word inside says which
mode it is, and says it more plainly than a hue can.

A color apiece was tried first: copy mode in the blue, being a state of
the pane; the question in the color a thing waiting on you is said in;
the view in a teal held down to the orange's own presence. It made four
colors to learn and then read, in the one position on the screen whose
whole job is to be seen rather than read. What the position has to carry
is lit or dark. The word answers the rest.

**6-3. THE RIGHT IS EMPTY.** It carried one lamp per row of the processes
view, a strip of them in the corner of the eye, each in the color of how
its process stood and the waiting one blinking. A row of dots says how
many things are running and which one wants you. The processes view says
that in words a glance to the left, and says which row it is about, where
a dot has to be counted against the list to mean anything at all. The
list is the reading and the dots were a second, worse copy of it.

What goes with them is the one thing conn had that worked on peripheral
vision: when you are working, your eyes are in the bay and the view is
beside them unread, and a lamp that lit was seen without being looked at.
What is left of that signal is the blinking word in the view and `tab`,
which reaches the longest-waiting contact from anywhere in the server
without reading anything first.

**6-4. COST.** conn writes the one thing only it knows, where its own
keys are, into an option of its own and only when it changes, which is on
a keypress: nothing but a key moves the keys between views or arms a
question. Never on a beat, and never on a reading.

## SECTION 7. PROJECTS AND SESSIONS

**7-1. PROJECTS.** `p` opens projects: every project the roots hold,
whether anything is running in it or not. conn walks `CONN_ROOTS`, or
`~/projects` when it says nothing, for repositories, and the shape of
what it finds is the declaration. A folder holding two or more of them is
the project they collectively make, and gets a row of its own with its
repositories under it, by their own names; a folder of one stays flat.

The projects view is a line typed into, so the letters the processes view
is worked by are characters there. What is typed narrows the rows, and a
project answers by its own name and by the name of the folder that groups
it. `enter` opens a shell at the row under the cursor and comes back to
the processes view, where the shell shows; on a group row that is a shell
at the level the work is about, which is what the group row is for.
`ctrl-a` opens claude there instead; plain `a` is a letter to type into
the filter, so the view takes the chord `s` does not need. `esc` comes
back without opening anything. `alt-p` opens projects from anywhere, from
the projects view itself, sessions, the console, since `p` only means
projects in the processes view, where it is a key rather than a letter
being typed or one of the any-keys that leave the console. It is what the
prefix chord sends.

**7-2. SESSIONS.** A session does not end when claude exits; the
transcript it leaves is enough to resume it. `alt-a`, in the processes
view or projects, a group's own row and all, opens sessions over what is
suspended at that project, beside `ctrl-a`'s own chord for a fresh
session. Newest first, each answering by its branch, the last thing it
was asked, or where it was had. It is a line typed into like projects.
`enter` resumes the one under the cursor in a shell running `claude
--resume`, and `esc` comes back without resuming anything. A session a
live contact is already carrying is left off, checked against the
process table rather than trusted on the file's word alone.

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
`conn down` in between. Every pane takes the new sixteen, the panel and a
hold in the bay come back drawn on the new ground, and a pane with work
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
carries which of the sixteen it is as well as its hex, so it holds up
where sixteen is all a terminal was given, and a ground that is none of
the sixteen takes the pane's own.

`conn theme claude` writes its file, on `dark-ansi` or `light-ansi`
according to the server's own ground. The theme names one of the sixteen
wherever a token has that shape of meaning, so most of it follows the
pane's own sixteen; only the grounds none of the sixteen has a name for,
the washes under a diff and the band behind a message, are spelled out.
Claude Code's syntax coloring is not for a theme to set. It is a fixed
map onto those same sixteen, so code in a pane is conn's colors already.
conn writes the file and says where. It offers to select the theme only
when Claude Code is on one it came with, or on none; a custom theme is
somebody's own doing, and conn says what it is and leaves it. Selecting
it yourself is `/theme` in a session.

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
