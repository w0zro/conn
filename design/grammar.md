# A grammar over processes

Draft, 2026-10-10. Written fresh, not from today's keys. A direction being worked out, not a spec.

## The idea

- Vim reduces an engineer's dealings with text to a grammar: verbs over objects. n verbs and m objects give n×m actions from n+m keys.
- conn does the same for dealings with the processes working a project.
- conn ships the grammar, not per-tool features. What a verb means for a given process is resolved, not written into conn.
- The grammar has two users: agents, by a CLI, and the engineer, by keys. Both reach the same verbs and the same saved answers.

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

A project's processes are the objects:

- **Running**: tmux panes, containers, Homebrew services, listeners on a port, contacts.
- **Declared**: what the project says it can run — `.conn`, compose services, `Makefile` targets, `package.json` scripts. A declared process is a row whether or not it runs.

Checks — tests, a build, lint, a typecheck — are declared processes that run to completion. They are processes conn holds: on the panel, `.` reruns them, and the engineer's run and the agent's run are the same process.

## Addresses

| Address | Keys | CLI |
|---|---|---|
| The row under the cursor | the verb doubled (`rr`, as vim's `dd`) | — |
| A motion from the cursor | `j` `k` `gg` `G` | — |
| A name: declared, or the process's own | open | `conn l web` |
| A port | open | `conn x :3000` |
| Every process of the project | `p` | `conn x -p` |

- An agent has no cursor. It knows a process by what an error told it: `EADDRINUSE :3000`, a name like `vite`. conn already reads the sockets and shows the port on the row, so a port address needs no new knowledge.
- "What is on 3000" is the engineer's question too.

## Verbs

| Key | Verb | Keys do | CLI does |
|---|---|---|---|
| `u` | up: start it | starts it in a pane | starts it; for a check, waits and returns its output and exit status |
| `x` | end it | ends it | ends it |
| `r` | restart | ends it and starts it | the same, in one verb, where agents use two |
| `c` | client: talk to it | a session: `psql` in a pane, the browser on a web server | one request and its answer: `conn c db "select 1"`, `conn c web /health` |
| `l` | log: what it said | its pane's output | its output, printed |
| `i` | inspect: what it is | command, origin, port | the same, printed; `conn ls` lists them |
| `a` | ask: hand it to an agent | a contact given its output and state | — |

- `c` merges "open its client" and "open its port": the client of an HTTP server is the browser.
- A session and a request rest on one piece of knowledge: how to reach the process — host, port, user, database. That is what a resolution of `c` saves, not a command line.
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

A verb on a process is resolved to an action:

1. **Saved**: an answer kept with the project. No LLM call.
2. **Asked**: `claude -p` with the verb, the process, and the project's context: its processes, their commands, ports, directories. The answer is shown once for confirmation and saved if it works.

A saved answer is kept while it works. When it fails at once — it exits non-zero within two seconds of starting — conn asks again, giving `claude -p` the failed command and its output with the project's context. The new answer is confirmed and replaces the saved one.

An action is one of: a command in a pane conn holds, an existing pane brought to the front, a signal to a process, a request whose answer is printed, a URL opened. A starting point, not a hard constraint.

## Where answers are kept

- Written by conn, readable and editable by the engineer, worth committing. One teammate's answers spare the next the LLM call.
- `.conn` already declares a project's processes, one a line, `name [dir]: command`. Saved answers either take a syntax of their own in that file or a file of their own. Open.

## What it opens up

1. **Agents address processes.** One addressed, saved way to read, reach, start and end a project's processes, in place of `capture-pane`, `lsof` and `pkill -f` rebuilt every session.
2. **The checks are visible.** The commonest thing agents run lands on the panel, with its failures, instead of only in a transcript.
3. **Muscle memory across projects.** The same keys reach the database in a Rails app, a Go service or a compose stack; the saved answers differ by project.
4. **Runbooks as config.** The saved answers replace a README's "how to connect to the db" and "how to get a shell in the container", and stay correct because a broken answer is asked again.
5. **Sources and verbs at n+m.** A new source (Kubernetes, systemd, an SSH host) gets every verb at once: `c` on a pod resolves to `kubectl exec`. A new verb works on every source at once.
6. **Macros, in vim's spirit.** A sequence recorded once — restart the database, run the migrations, restart web — replayed on one key.
7. **The seam to vim.** A check's failures go to a quickfix list. The conjurer and quickfix-pro are the vim half; conn does not read files.
8. **Failure as intent.** A process failed, so hand it to an agent with its output. The re-ask on a fast failure is one case; `a` on a crashed row is the pattern on demand.

## Later

- **Types**: a table of programs conn knows by name (postgres → `psql`), answering before the LLM is asked.
- **Roles** as objects, as vim's text objects: `d` the database, `w` the web server, `t` the tests. Open: how a role names several processes, and which roles get letters.
- **More sources**: `Procfile`, and what else a project declares its processes in.
- **`s` shell**: its environment — a shell in its container, or at its directory with its environment loaded.
- **`y` yank** and **`e` edit**: no evidence yet, which the count could not have seen either way.

## Open

- How the keys take a name or a port.
- Where saved answers are kept, beside `.conn`'s declarations.
- How a CLI `u` on a long-running process returns: at once, or when it is listening.
