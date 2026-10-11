# A grammar over processes

Draft, 2026-10-10. Written fresh, not from today's keys.

## The idea

- Vim reduces an engineer's dealings with text to a grammar: verbs over objects, picked by motions. n verbs and m objects give n×m actions from n+m keys.
- conn does the same for an engineer's dealings with the processes working a project.
- conn ships the grammar. It does not ship per-tool features. What a verb means for a given process is resolved, not written into conn.

## Nouns

A project's processes are the objects. There are two kinds:

- **Running**: tmux panes, containers, Homebrew services, listeners on a port, contacts (agents).
- **Declared**: what the project says it can run, read from the files that already say it: `Procfile`, `compose.yaml`, `package.json` scripts, `Makefile` targets, `.conn`.

Tests, a build, a migration are declared processes that are not running. Running them is the same verb as starting a server.

Each process can have a **role**: database, web, worker, tests, build, agent. Roles name objects without moving to them, as vim's text objects do.

## Verbs

| Key | Verb | On a database | On a web server | On the tests |
|---|---|---|---|---|
| `enter` | go: its pane in front | its pane | its pane | its pane |
| `u` | up: start it | start postgres | start the server | run them |
| `x` | end it | stop postgres | stop the server | stop the run |
| `r` | restart | restart postgres | restart the server | run them again |
| `c` | client: talk to it | `psql …` | the browser at its port | the failures, rerun |
| `s` | shell: its environment | a shell in its container | a shell at its dir, its env loaded | same |
| `l` | log: what it said | its output | its output | the last run |
| `/` | find in what it said | | | |
| `y` | yank: copy its handle | the connection URL | the URL | the command |
| `i` | inspect: what it is | command, env, origin, port | same | same |
| `e` | edit: its configuration | `postgresql.conf` | the server's config | the test config |
| `a` | ask: hand it to an agent | with its output and state | same | the failures |

- `c` merges "open its client" and "open its port": the client of an HTTP server is the browser.
- `i` is how conn teaches the machine: every row can say what it is and where it came from.
- `a` is the way out of the grammar: anything the verbs do not cover goes to a contact, with the object as context.

## Objects

A verb waits for an object, as vim's operators do.

| Key | Object |
|---|---|
| the verb again | the process under the cursor (`rr`, as vim's `dd`) |
| `j` `k` `gg` `G` | a motion from the cursor |
| `d` | the project's database |
| `w` | the project's web server |
| `q` | the project's worker |
| `t` | the project's tests |
| `b` | the project's build |
| `p` | every process of the project |

`rd` restarts the database. `ut` runs the tests. `cw` opens the app in the browser. `xp` ends the project. `ad` hands the database to an agent.

- `.` repeats the last verb and object.
- Inside a process, the panel key followed by the same grammar. The object defaults to the process you are in.
- The motions and the key card stay: motions pick the row, the card shows the verbs against the objects as a table.

## Resolution

A press is resolved to an action:

1. **Known**: conn's own table of programs by name: postgres → `psql`, a container → `docker exec`. No LLM call.
2. **Saved**: an answer kept in the project's `.conn`. No LLM call.
3. **Asked**: `claude -p` with the verb, the object, and the project's context: its processes, commands, ports, declared files. The answer is shown once for confirmation and saved to `.conn` if it works.

An action is one of:

- a command in a pane conn holds (it becomes a process row)
- an existing pane brought to the front
- a signal to a process
- a value copied (`y`)
- a URL opened (`c` on an HTTP server)

That list is a starting point, not a hard constraint.

Roles are resolved the same way: known by program name, saved, or asked.

## `.conn`

Written by conn, readable and editable by the engineer, worth committing. One teammate's resolved answers spare the next teammate the LLM call.

```
role database = postgres
role web      = web
c database    = psql -h localhost -p 5433 myapp_dev
s web         = docker compose exec web bash
```

## Open

- Doubling for the current row (`rr`) costs a press for the most common case. The alternative: a verb acts on the row at once, and objects take a prefix. Vim chose doubling.
- The letters. `d` as an object rules out `d` as a verb; vim reuses letters across the two positions, and so can conn.
- When a saved answer stops working. One answer: a command that fails at once offers a re-ask, given the command and its output.
- Roles the table lacks: a cache, a queue, a proxy, a second database.
- Counts. Vim's `3dd` has no clear meaning over processes yet.
