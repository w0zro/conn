# A grammar over processes

Draft, 2026-10-10. Written fresh, not from today's keys.

## The idea

- Vim reduces an engineer's dealings with text to a grammar: verbs over objects, picked by motions. n verbs and m objects give n×m actions from n+m keys.
- conn does the same for an engineer's dealings with the processes working a project.
- conn ships the grammar. It does not ship per-tool features. What a verb means for a given process is resolved, not written into conn.

## First step: processes in general

The first step treats every process alike: no types, no roles, no distinction by where a process comes from. A process is a row, and a verb on it is resolved for that process. Specialization comes later, under **Later**.

## Verbs

| Key | Verb | For example |
|---|---|---|
| `u` | up: start it | bring up a server that ended |
| `x` | end it | stop it |
| `r` | restart | end it and start it again |
| `c` | client: talk to it | `psql …` on postgres, the browser on a web server |
| `s` | shell: its environment | a shell in its container, or at its directory |
| `l` | log: what it said | its output |
| `y` | yank: copy its handle | its URL, its command |
| `i` | inspect: what it is | its command, environment, origin, port |
| `e` | edit: its configuration | `postgresql.conf` |
| `a` | ask: hand it to an agent | a contact given its output and state |

- `c` merges "open its client" and "open its port": the client of an HTTP server is the browser.
- `i` is how conn teaches the machine: every row can say what it is and where it came from.
- `a` is the way out of the grammar: anything the verbs do not cover goes to a contact, with the process as context.

## Commands

`enter` and `/` act on the process under the cursor at once, as vim's `x` and `/` do.

| Key | Command |
|---|---|
| `enter` | go: its pane in front |
| `/` | find in what it said |

## Objects

A verb waits for an object, as vim's operators do.

| Key | Object |
|---|---|
| the verb again | the process under the cursor (`rr`, as vim's `dd`) |
| `j` `k` `gg` `G` | a motion from the cursor |
| `p` | every process of the project |

`rr` restarts the process under the cursor. `xj` ends the one below it. `rp` restarts the project.

- A letter can be a verb and an object, as in vim: what it means is set by where it is pressed. A verb's own letter, as its object, means the process under the cursor.
- `.` repeats the last verb and object.
- No counts. Vim's `3dd` has no clear meaning over processes; a count comes in when one does.
- Inside a process, the panel key followed by the same grammar. The object defaults to the process you are in.
- The key card shows the verbs against the objects as a table.

## Resolution

A press is resolved to an action:

1. **Saved**: an answer kept in the project's `.conn`. No LLM call.
2. **Asked**: `claude -p` with the verb, the process, and the project's context: its processes, their commands, ports, directories. The answer is shown once for confirmation and saved to `.conn` if it works.

A saved answer is kept while it works. When it fails at once — it exits non-zero within two seconds of starting — conn re-asks, giving `claude -p` the failed command and its output along with the project's context. The new answer is confirmed, and it replaces the saved one.

An action is one of:

- a command in a pane conn holds (it becomes a process row)
- an existing pane brought to the front
- a signal to a process
- a value copied (`y`)
- a URL opened (`c` on an HTTP server)

That list is a starting point, not a hard constraint.

## `.conn`

Written by conn, readable and editable by the engineer, worth committing. One teammate's resolved answers spare the next teammate the LLM call. An answer is kept by verb and by the process as its row names it.

```
c postgres = psql -h localhost -p 5433 myapp_dev
s web      = docker compose exec web bash
```

## Later

Specialization, once the general grammar works:

- **Types**: a table of programs conn knows by name (postgres → `psql`), answering before the LLM is asked.
- **Roles** as objects, as vim's text objects: `d` the database, `w` the web server, `t` the tests. `rd` restarts the database without moving to it. Open: how a role letter names several processes of the role, and which roles get letters.
- **Sources**: declared processes read from the files that already say them — `Procfile`, `compose.yaml`, `package.json` scripts, `Makefile` targets — so tests, a build or a migration are processes `u` starts, running or not.
