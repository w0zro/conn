# A grammar over processes

Draft, 2026-10-10. Written fresh, not from today's keys. A direction being worked out, not a spec.

## From first use

2026-10-11: the tree was cut back to the tmux layer (cb1d265) and the grammar is rebuilt one step of use at a time, each step only what it needs. What is below this section was written before the cut; what it describes as built is in the history up to f5c773a.

1. **Open the terminal, you are in conn.** `conn` from `~` opens on the projects work has happened in, read from Claude Code's transcripts and where conn's panes are, newest first. Typing finds one, or names a directory by path. Built.
2. **Get back to a conversation.** Enter on a project lists what runs in conn, then conversations running in another terminal, then the ones at rest. Enter goes to what runs, or resumes a conversation at rest in a window of its own. ctrl-space comes back to conn. Built.

3. **Start something new, and leave.** In a project, `n` starts a Claude conversation and `s` a shell, at the project's root. `q` in a project and `ctrl-c` anywhere leave conn with everything in it running. Built.

4. **Know who needs you.** A conversation reads working, waiting or idle, from what Claude Code writes for each instance. Waiting comes first, in orange, in a project and on the projects screen, where a row says `1 waiting · 2 running`; the status line says how many wait from every window. Built.

5. **Agents work the processes through conn.** `conn ls`, `conn l NAME|:PORT|PID`, `conn u [NAME] [-w] -- COMMAND`, `conn x` and `conn r`, over conn's panes and the servers listening in the project. The agents conn starts are told them. Built.

6. **Work the processes from the panel.** In a project, on the row under the cursor, `x` stops it and `r` restarts what conn started; `u` asks for a command and runs it at the project's root. One key a verb (2026-10-11): with no other object yet, doubling says nothing more. Built.

7. **Open what you started.** A project's rows say their ports; a server in the project that conn does not hold is a row; `o` opens a row's port in the browser. Built.

8. **Start the whole project.** `u` asks Claude Code what keeps running in the project, shows the answer, and enter starts each process; nothing is kept, so each `u` asks again (about ten seconds). A typed command is `!`. Built.

9. **Go to who needs you.** `tab` in conn goes to a conversation waiting on you, the next one each press; ctrl-space then tab from anywhere. Built.

10. **Find what was said.** `/` in a project searches what its panes keep; enter lands in tmux copy mode on the line. Built.

11. **The panel beside the work.** conn's window is the panel and the workspace; the workspace shows the row under the cursor, enter puts the keys in it, ctrl-space brings them back. Built.

12. **A folder of repositories.** A folder holding two or more git checkouts, below a folder directly in home, is listed with its repositories beneath it and opens as a project of all of theirs; each repository is still its own. One level. Built.

13. **What agents run is what the keys run.** Every process started under a name through conn is written down beside the socket with its role: what `conn u -w` waited on is a check, the rest is the stack. In a project, `u` brings up the stack and `t` runs the checks as each name was last run here, by whoever ran it, with no question; only where nobody has yet is Claude Code asked, the answer offered, and enter's start written down the same way. Agents write the project's dialect by using the CLI; the keys speak it. Built, 2026-10-12.

14. **What broke.** A run that ended reads done, or failed with its status, and what failed comes right after the conversations waiting on you. Built, 2026-10-12.

15. **Hand it to an agent.** `a` on a row starts a conversation in the project whose first prompt is the process: name, command, where, how it is doing, its last lines, and what to do; a check that failed goes to an agent with its failure in one key. A conversation is not handed; a server outside conn is handed by name, pid and port. Built, 2026-10-12.

16. **Agents are told the dialect.** Every conversation conn starts or picks back up is told what the project's checks and stack are as last run through conn, as `conn u` lines. What one agent established, the next is told. Built, 2026-10-12.

17. **Stop everything.** On the projects screen, `x` on a project ends every process conn holds in it, conversations included, and every server in it outside conn. Built, 2026-10-12.

18. **Get me into the database.** `c` on a row talks to its process: a conversation or a shell is gone into; a port that answers HTTP opens in the browser; anything else is asked of Claude once, the one line offered, and kept under the row's name as what `c` means for it here, a URL for the browser or a command run in a window of its own with the keys in it. `o` folds into `c`. Built, 2026-10-12.

19. **The database is a row.** A listener outside conn that the project's panes connect to is a row of the project, on the panel and in `conn ls`: a brew postgres stands under every project whose dev server talks to it. Only what conn holds is asked what it connects to. Built, 2026-10-12.

20. **What was kept is asked again when it fails at once.** What `u` and `t` start, as kept or as Claude just named it, is watched for two seconds; a run that ends in an error by then asks Claude again, given the command, its status and its last lines. A command that does not work here is offered in place of the old, under the same name, and enter's start writes it down; a command that is right and failed for the project's own reasons is said to be the project's, and nothing is offered. Built, 2026-10-14.

21. **Stop by role.** `x` waits for its object, as `r` does: `xx` the row, or on another project's line everything in it; `xs` the stack and `xt` the checks, as they stand on the panel; `xp` everything in the open project. Built, 2026-10-15.

22. **Restart by role.** `r` takes the same objects: `rs` the stack and `rt` the checks as they stand, or run as `u` and `t` run them where none stand; `rf` what failed. `rr` and `rc` as before. Built, 2026-10-15.

23. **Hand what failed.** `a` waits too: `aa` hands the row, as `a` did; `af` hands every run that failed to one conversation, each with its last lines, in a share of the size. Built, 2026-10-15.

24. **Start is a verb.** `s` waits for its object: `ss` brings up the stack and `st` runs the checks, as `u` and `t` did; `sc` starts a conversation, as `n` did. A shell is `!` with nothing typed. `n`, `u` and `t` are free; `u` is for undo. Built, 2026-10-15.

25. **Dress the server.** `,` on either screen opens the settings: the themes conn has and the two grounds, `enter` dressing the server where it stands and keeping the choice in config.json. A theme is a JSON sheet by role, three shipped and any in ~/.config/conn/themes; `conn theme claude` writes the one for Claude Code. Built, 2026-10-14.

26. **Back in.** `esc` on the panel goes back into the process the keys were last in, its project on the panel. Built, 2026-10-15.

27. **Heard from any window.** The number of conversations waiting is in the terminal's title, and the bell rings as one more starts waiting, so a tab in the background still says. Built, 2026-10-16.

28. **Whose it is.** A process started through `conn u` from a pane of conn's is that pane's project's, wherever the shell has gone: an agent that goes into another checkout to run its project's site tests there does not make a second project. The row says where it ran, and `conn ls`, `x` and `r` from the pane reach the pane's project. Built, 2026-10-16.

29. **A name is a declaration.** `conn u NAME -w` says the project runs this again, and it is written down and told to the next agent; `conn u -w -- COMMAND` with no name runs something once, a row under its command while its pane lives, answering to its pid, written down nowhere. Agents figure out a project's checks once and name them; what they run to see something, a few test files, a one-off, takes no name, so the roster stays the project's few checks and `st` starts those. Before this, one project had 113 names written down as checks, every one of them a run an agent named, and `st` started them all. Built, 2026-10-18.

Since 2026-10-11 conn reads the machine when it changes: file events on Claude Code's files, and a tmux control-mode client subscribed to every pane. Ports have no event; they are read again 1, 3 and 10 seconds after a tmux change.

Keys so far. On the projects: typing, `↑` `↓` (`ctrl-n` `ctrl-p` `ctrl-j` `ctrl-k`), `enter`, `esc`, `tab`, `x`, `,`, `?`, `ctrl-c`. In a project: `j` `k` `g` `G`, `enter`, `tab`, `esc`, `-`, `/`, `c`, `!`, `,`, `?`, `q`, and the verbs `s` `x` `r` `a` with their objects (`ss` `st` `sc`, `xx` `xs` `xt` `xp`, `rr` `rs` `rt` `rf` `rc`, `aa` `af`). `ctrl-space` from anywhere, and twice over the other process.

## The idea

- Vim reduces an engineer's dealings with text to a grammar: verbs over objects. n verbs and m objects give n×m actions from n+m keys.
- conn does the same for dealings with the processes working a project.
- conn ships the grammar, not per-tool features. What a verb means for a given process is resolved, not written into conn.
- The grammar has two users: agents, by a CLI, and the engineer, by keys. Both reach the same verbs.
- conn reads no declarations for now: no `.conn`, no compose, `Makefile` or `package.json` read by conn itself. What a project can run is resolved like any verb.
- Nothing is persisted for now: no saved answers, no restored state. The grammar and what it can do come first; speed comes after, from there. Since 2026-10-12 one thing is: what was started under a name, and as what, so the keys run what agents ran (step 13).

## The evidence

Counted 2026-10-10 from zsh history and Claude Code transcripts, by command shape.

| Intent | Count | Verb |
|---|---|---|
| Run the checks: tests, build, lint, typecheck | ≈ 3,800 (agents) | `u` on a check |
| Read a process's output | `tmux` 1,234, mostly reading other panes | `l` |
| Talk to it | `curl` 1,399, `psql` ≈ 160 | `c` |
| Find what runs, or what holds a port | `ps`, `lsof` ≈ 300 | `i` |
| End it | `pkill`, `kill` ≈ 270 | `x` |
| Bring a stack up or down | `docker compose up`/`down`, `brew services start` | `u`, `x` on `p` |

- Agents ran about 33,700 commands against processes; the engineer's shell holds about 70 that are not conn itself.
- Agents have no address for a process. They rebuild one each session — `tmux capture-pane`, `ps aux | grep`, `lsof -i`, `pkill -f` — and `pkill -f` matches by pattern, so it can end the wrong one.
- Restart never appears as one command. It appears as `pkill` and a start: agents compose it.
- Copy and edit configuration never appear. Keys pressed in conn, tmux and vim leave no history, so the count cannot see them.
- The count takes `| tail` and `| grep` filters for programs; those totals are high and mostly mean "filter output".

## Users

- **Agents** are the main operators of processes, by about 40 to 1. They reach the grammar by a CLI (`conn l web`, `conn x :3000`), or an MCP tool over the same verbs. What an agent does to a process lands on the panel and in the station log, not only in its transcript.
- **The engineer** reaches the grammar by keys on the panel.

## Nouns

- **Processes**: what runs in a project — tmux panes, containers, Homebrew services, listeners on a port, contacts — and what ran: a process conn held that ended stays a row, with its output.
- **The project**: what the processes work on; see **Projects**. The one in question comes from the working directory, for keys and CLI alike, or from the projects view; the CLI run from a pane of conn's takes the pane's, wherever the shell has gone.

What a project can run is not a list conn keeps. It is resolved when a verb asks for it, from the project's own files — compose, `Makefile`, `package.json`, a Procfile, a README — the way any verb is resolved.

Checks — tests, a build, lint, a typecheck — are processes that run to completion. Started through conn, they are processes conn holds: on the panel, `.` reruns them, and the engineer's run and the agent's run are the same process.

## Projects

A project is read from what the machine records, nothing written for conn.

- **A repository, not a checkout.** A project is a git repository, known by git's common directory. Its worktrees are checkouts of it, wherever they are on disk: `web` running in three worktrees is one project with three lines of work.
- **The folder they share.** A folder holding two or more repositories is the project they make together: `~/projects/shop/{frontend,api,infra}` is `shop`. How checkouts are laid out is the declaration, as it is today. It holds in the morning, before anything runs; a worktree joins through its repository.
- **Connections, while they run.** A process belongs to the project its working directory is in, or, started through conn from a project's pane, to that project wherever it runs. One outside any repository — a Homebrew service, a container, a listener — belongs to the projects whose processes connect to it, so a postgres shared by two repositories is in both. Two projects whose processes talk — `frontend`'s dev server proxying to `api` on :4000 — are linked while the connection stands.
- **Submodules** make a parent repository the project of its children. A monorepo is one repository and needs nothing.

Repositories that belong together but are kept apart are linked only while they run. To link them for good, the checkouts go in one folder: the layout is how it is said, not a file. Someone who keeps them apart on purpose cannot group them for good; declarations under **Later** would.

An agent's addresses reach its own worktree first, then its repository's project. An agent in one worktree does not end another worktree's `web` by name.

## The morning

- `conn` inside a project, or `enter` on it in the projects view, puts the project on the panel, with nothing running in it.
- `up` starts the project: resolved from its files to what makes up its stack, each started as a process conn holds.
- The project stays on the panel while the station is up.

## Addresses

| Address | Keys | CLI |
|---|---|---|
| The row under the cursor | the verb doubled (`rr`, as vim's `dd`) | — |
| A motion from the cursor | `j` `k` `gg` `G` | — |
| A name: the process's own, or the one it was started under | open | `conn l web` |
| A port | open | `conn x :3000` |
| Every process of the project | `p` | `conn x -p` |

- An agent has no cursor. It knows a process by what an error told it: `EADDRINUSE :3000`, a name like `vite`. conn already reads the sockets and shows the port on the row, so a port address needs no new knowledge.
- "What is on 3000" is the engineer's question too.

## Verbs

| Key | Verb | Keys do | CLI does |
|---|---|---|---|
| `u` | up: start it | starts it in a pane; on the project, its stack | starts what the agent names, as the command it gives: `conn u test -- npm test`; a check waits and returns its output and exit status |
| `x` | end it | ends it | ends it |
| `r` | restart | ends it and starts it | the same, in one verb, where agents use two |
| `c` | client: talk to it | a session: `psql` in a pane, the browser on a web server | one request and its answer: `conn c db "select 1"`, `conn c web /health` |
| `l` | log: what it said | its pane's output | its output, printed |
| `i` | inspect: what it is | command, origin, port | the same, printed; `conn ls` lists them |
| `a` | ask: hand it to an agent | a contact given its output and state | — |

- `c` merges "open its client" and "open its port": the client of an HTTP server is the browser.
- A session and a request rest on one piece of knowledge: how to reach the process — host, port, user, database. That is what a resolution of `c` finds, not a command line.
- `i` is how conn teaches the machine: every row says what it is and where it came from.
- `a` is the way out of the grammar: anything the verbs do not cover goes to a contact, with the process as context.

## Commands

`enter` and `/` act on the process under the cursor at once, as vim's `x` and `/` do.

| Key | Command |
|---|---|
| `enter` | go: its pane in front |
| `/` | find in what it said |

## The keys

- A verb waits for an object, as vim's operators do. Doubling it is the row under the cursor.
- A letter can be a verb and an object, as in vim; where it is pressed sets which.
- `.` repeats the last verb and object.
- No counts. Vim's `3dd` has no clear meaning over processes; a count comes in when one does.
- Inside a process, the panel key followed by the same grammar. The object defaults to the process you are in.
- The key card shows the verbs against the objects as a table.

## Resolution

A verb on a process is resolved to an action on every press. Nothing is kept between presses.

- **From the keys**: `claude -p` with the verb, the process, and the project's context: its processes, their commands, ports, directories.
- **From the CLI**: the caller is an agent, which resolves for itself. conn gives it the process and the project's context; it does not ask a second model.

An action is one of: a command in a pane conn holds, an existing pane brought to the front, a signal to a process, a request whose answer is printed, a URL opened. A starting point, not a hard constraint.

## What it opens up

1. **Agents address processes.** One addressed way to read, reach, start and end a project's processes, in place of `capture-pane`, `lsof` and `pkill -f` rebuilt every session.
2. **The checks are visible.** The commonest thing agents run lands on the panel, with its failures, instead of only in a transcript.
3. **Muscle memory across projects.** The same keys reach the database in a Rails app, a Go service or a compose stack; what they resolve to differs by project.
4. **Sources and verbs at n+m.** A new source (Kubernetes, systemd, an SSH host) gets every verb at once: `c` on a pod resolves to `kubectl exec`. A new verb works on every source at once.
5. **Macros, in vim's spirit.** A sequence recorded once — restart the database, run the migrations, restart web — replayed on one key.
6. **The seam to vim.** A check's failures go to a quickfix list. The conjurer and quickfix-pro are the vim half; conn does not read files.
7. **Failure as intent.** A process failed, so hand it to an agent with its output. `a` on a crashed row is the pattern on demand.

## Built

As of 2026-10-11:

- **The CLI**: `conn ls`, `conn l ADDRESS`, `conn u [NAME] [-w] -- COMMAND`, `conn x ADDRESS`, `conn r ADDRESS`. Addresses: a name (worktree, then project, then the folder of repositories), `:port`, a pid. `x` and `r` reach only the caller's project.
- **The panel's keys are the grammar**: verbs `u x r c l i s a` wait for an object — the verb again for the row, `p` the project, `j k G` a row by a motion — and `.` repeats. `enter` and `/` act at once. The station's views are behind `g`: `gp gr gl gc`.
- **Resolution, with nothing kept**: `cc` on a process with no known client opens a port that speaks the web, and otherwise asks `claude -p` and offers the answer as a question; `up` on a project with no `.conn` asks for its stack and offers it on one question.
- **Contacts are told the CLI**, naming the build by its whole path where the PATH's `conn` is another.
- **Projects**: a worktree's work is its repository's; a listener with no row of its own is a row of every project whose runs connect to it.
- **Places** (2026-10-12): a project's rows stand by role, in order: conversations, the stack, what talks to it (shells, clients), the checks, what the project talks to outside conn, conversations in terminals conn does not hold; air between places, no labels. In a place, what is written down stands as first written, so a check run again keeps its row and the cursor; the rest in start order. Wanting you is the accent where the row stands, waiting and failed alike; nothing moves for its state. A worktree repeats the places under its head.
- **Worktrees** (2026-10-12): a worktree's rows stand together under a head of their own, the branch it has out alone, in the parchment the project's head is in without its weight, after the project's own rows; in a folder the head names the repository and the branch. A label with a rule is a section of the panel, which a worktree is not. A verb starts where the cursor stands: `n`, `s` and `!` in the checkout of the row under it, and `u` and `t` run a repository's stack in that checkout where it is the repository's, at the repository otherwise. The stack is written down under the repository, never the worktree. The CLI already ran in the caller's checkout.

Not yet: linking two projects while they talk, the CLI starting the server. `.conn` and the known-programs table went with the cut.

## Later

- **Persistence, for speed**: answers saved with the project, readable and worth committing, so a press costs no LLM call and a teammate's answers spare the next. A saved answer kept while it works and asked again when it fails at once — non-zero within two seconds. Runbooks as config: the answers replace a README's "how to connect to the db". Where they are kept beside `.conn`'s declarations. Restoring what was up after a reboot.
- **Types**: a table of programs conn knows by name (postgres → `psql`), answering before the LLM is asked.
- **Roles** as objects, as vim's text objects: `d` the database, `w` the web server, `t` the tests. Open: how a role names several processes, and which roles get letters.
- **Tasks as projects**: "fix the checkout bug" across repositories, worktrees, a database and agents. Read from records that exist: the branch as identity, the contact's session as its title, what the contact started (the process tree, and its CLI calls), what those connect to, the worktrees with the branch out; ended when the branch merges, its PR closes, or the session ends with nothing running. Not recorded: work with no branch and no agent, the engineer's own processes in a shared checkout, and links across repositories beyond a shared branch name. Built on the repository and the connections above.
- **Declarations**: `.conn`, compose, `Procfile`, `Makefile`, `package.json` read by conn itself — what should run shown as down rows, and names known without asking.
- **`s` shell**: its environment — a shell in its container, or at its directory with its environment loaded.
- **`y` yank** and **`e` edit**: no evidence yet, which the count could not have seen either way.

## Open

From a day played out, 2026-10-11:

- Starting a contact in a project and handing a process to one (`a`) are two intents; the grammar has a verb for the second only.
- Whether an agent may reach another project's processes at all, beyond its own worktree and project: refused, or logged.
- The station keys — `l` log, `c` console, `p` projects, `r` recent — are verbs or objects in the grammar and need another home.
- Whether the keys show an action before running it.
- Worktrees, from the first version of 2026-10-12: a process in a worktree removed under it stats to no directory and so to no project, and runs on unseen; two worktrees' servers want one port, which the app refuses and conn only shows; a bare repository names its project after the bare directory.

From before:


- How the keys take a name or a port.
- How a CLI `u` on a long-running process returns: at once, or when it is listening.
- Whether `up` from the keys shows the stack it resolved before starting it.
