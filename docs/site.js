// conn.w0zro.com: the panel, worked. As the page opens, a field of
// panes - conversations, servers, shells, checks, more than anyone can
// watch - gathers once into conn's panel: the project you are in on
// top, a row a process, and under it a line for every other project
// with something running. Beside the panel stand the verbs against
// their objects, and the keys pressed on this page work the panel as
// they work conn: a verb waits for its object, the foot says what it
// waits for, and the rows answer. The panel is drawn with conn's own
// marks, inks and words.
(() => {
  'use strict';

  const $ = id => document.getElementById(id);
  const reduced = matchMedia('(prefers-reduced-motion: reduce)').matches;
  const esc = s => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  // A moment held, for a look: #p=N holds the gathering at N (past 1,
  // the page as gathered), and k= presses keys on it.
  const look = new URLSearchParams(location.hash.slice(1));
  const held = look.has('p') ? [null, look.get('p')] : null;
  const wait = (ms, fn) => setTimeout(fn, reduced ? 0 : ms);

  // ---- the processes --------------------------------------------------

  // A row's mark is its role: a conversation, the stack, what talks to
  // it, a check and its verdict, a listener outside conn.
  const MARK = { conversation: '❝', stack: '○', client: '❯', check: '◦', pass: '✓', fail: '✗', outside: '◌' };
  const PLACES = ['conversation', 'stack', 'client', 'check', 'outside'];

  // The open project and what works it, then three more. Each process
  // is a pane in the field; the open project's become its rows, the
  // others' gather into their project's line. A body is what the
  // process has written, a few lines. A hand is the title the
  // conversation gets when the row is handed to an agent.
  const OPEN = { name: 'conn', path: '~/projects/w0zro/conn', rows: [
    { role: 'conversation', name: 'bring the code to the manual', status: 'working', ctx: '262k', body: ['<span class="g">></span> bring the code to the manual', '<span class="r">●</span> Read panel.go, cli.go, server.go', '<span class="r">⣾</span> <span class="g">Editing… (esc to interrupt)</span>'] },
    { role: 'conversation', name: 'a grammar over processes', status: 'waiting', ctx: '118k', cursor: true, body: ['<span class="g">></span> make s a verb that waits for its object', '<span class="r">●</span> Read panel.go, conn_test.go', '<span class="a">●</span> Free n, u and t once sc, ss and st are in?', '  <span class="a">❯</span> 1. Yes', '    2. No'] },
    { role: 'stack', name: 'demo', status: 'serving', port: ':8123', hand: 'the demo server on :8123', body: ['<span class="g">Serving HTTP on :: port 8123</span>', '<span class="g">::1 - -</span> "GET / HTTP/1.1" 200 -', '<span class="g">::1 - -</span> "GET /manual/ HTTP/1.1" 200 -'] },
    { role: 'client', name: 'zsh', status: 'idle', hand: 'the shell at the root', body: ['<span class="a">$</span> git status -sb', '<span class="g">## main...origin/main [ahead 3]</span>', '<span class="a">$</span> <span class="cursor"></span>'] },
    { role: 'check', name: 'test', status: 'passed', age: '4m', hand: 'the tests pass; look them over', body: ['<span class="g">ok  github.com/w0zro/conn  2.814s</span>', '<span class="g">ok  internal/theme  0.112s</span>', '<span class="g">ok  internal/tmux  0.391s</span>', '<span class="f">[test exited 0]</span>'] },
    { role: 'check', name: 'lint', status: 'failed', age: '2m', hand: 'fix the unused nameFor in panel.go', body: ['<span class="a">panel.go:2206:4:</span> unused: nameFor', '<span class="g">2 issues:</span>', '<span class="g">* unused: 2</span>', '<span class="f">[lint exited 1]</span>'] },
    { role: 'outside', name: 'postgres', status: 'serving', port: ':5432', hand: 'postgres on :5432', body: ['<span class="g">LOG:</span>  database system is ready', '<span class="g">LOG:</span>  checkpoint starting: time', '<span class="g">LOG:</span>  connection authorized: user=conn'] },
  ] };
  const OTHERS = [
    { name: 'web', standing: '1 waiting · 3 running', waiting: true, rows: [
      { role: 'conversation', name: 'make the cart total update live', status: 'waiting', body: ['<span class="g">></span> make the cart total update live', '<span class="r">●</span> Read Cart.tsx, useCart.ts', '<span class="a">●</span> Allow Bash: conn u test -w -- npm test?', '  <span class="a">❯</span> Yes', '    No'] },
      { role: 'stack', name: 'vite', status: 'serving', body: ['<span class="r">VITE v6.0.3</span>  <span class="g">ready in 212 ms</span>', '', '  ➜  Local:   <span class="r">http://localhost:5173/</span>', '  ➜  <span class="g">press h + enter to show help</span>'] },
      { role: 'client', name: 'zsh', status: 'idle', body: ['<span class="a">$</span> conn ls', '<span class="g">~/projects/web</span>', 'vite  41822  running  5173  npm run dev', '<span class="a">$</span> <span class="cursor"></span>'] },
    ] },
    { name: 'api', standing: '3 running', turning: true, rows: [
      { role: 'conversation', name: 'add the rate limit to /v1/search', status: 'working', body: ['<span class="g">></span> add the rate limit to /v1/search', '<span class="r">●</span> Read limiter.go, search.go', '<span class="r">⣾</span> <span class="g">Running conn u test -w -- go test ./…</span>'] },
      { role: 'stack', name: 'api', status: 'serving', body: ['<span class="g">2026/10/16 14:02:11</span> listening on :8080', '<span class="g">2026/10/16 14:02:40</span> GET /healthz 200'] },
      { role: 'check', name: 'test', status: 'running', body: ['<span class="g">ok  api/limiter  0.214s</span>', '<span class="g">ok  api/search   1.102s</span>', '<span class="r">⣾</span> <span class="g">api/store</span>'] },
    ] },
    { name: 'notes', standing: '2 running', rows: [
      { role: 'conversation', name: 'outline the october post', status: 'idle', body: ['<span class="g">></span> outline the october post', '<span class="r">●</span> Wrote outline to 2026-10-post.md', '<span class="p">❯</span> <span class="cursor"></span>'] },
      { role: 'stack', name: 'hugo', status: 'serving', body: ['<span class="g">Web Server is available at</span> <span class="r">http://localhost:1313/</span>', '<span class="g">Press Ctrl+C to stop</span>'] },
    ] },
  ];
  // The conversations at rest, for rc.
  const REST = [
    { name: 'the lines typed into edit as readline does', when: '2h' },
    { name: 'a finished conversation says so', when: '5h' },
    { name: 'what a conversation is told reaches it through a file', when: '1d' },
  ];

  // The keys each screen says on the foot, as conn says them.
  const HINTS = {
    project: [['enter', 'go'], ['/', 'search'], ['s', 'start'], ['x', 'stop'], ['r', 'restart'], ['a', 'agent'], ['c', 'talk'], ['!', 'run'], ['?', 'keys']],
    s: [['s', 'the stack'], ['t', 'the checks'], ['c', 'a conversation'], ['esc', 'cancel']],
    x: [['x', 'the row'], ['s', 'the stack'], ['t', 'the checks'], ['p', 'the project'], ['esc', 'cancel']],
    r: [['r', 'the row'], ['s', 'the stack'], ['t', 'the checks'], ['f', 'what failed'], ['c', 'a conversation at rest'], ['esc', 'cancel']],
    a: [['a', 'the row'], ['f', 'what failed'], ['esc', 'cancel']],
    list: [['type', 'to find'], ['↑ ↓', 'move'], ['enter', 'picks it back up'], ['esc', 'back']],
  };

  // ---- the panel ------------------------------------------------------

  let rows = OPEN.rows.map(r => ({ ...r, id: Math.random() }));
  let cursor = Math.max(0, rows.findIndex(r => r.cursor));
  let verb = '', note = '', listing = false, listAt = 0;
  const rowsEl = $('rows'), linesEl = $('lines'), footEl = $('foot'), panelEl = $('panel');

  function markOf(r) {
    if (r.role === 'check') return r.status === 'passed' ? MARK.pass : r.status === 'failed' ? MARK.fail : MARK.check;
    return MARK[r.role];
  }
  function spinOf(r) {
    if (r.status === 'working' || r.status === 'running' || r.status === 'starting') return '⣾';
    if (r.status === 'serving') return '⣿';
    return '';
  }
  // The figure at a row's right: a conversation's context and whether
  // it waits, a server's port, a check's age.
  function figOf(r) {
    switch (r.role) {
      case 'conversation': return r.status === 'waiting' ? `${r.ctx}  waiting` : r.status === 'working' ? r.ctx : '';
      case 'stack': case 'outside': return r.status === 'serving' ? r.port : '';
      case 'check': return r.status === 'running' ? '' : r.age;
      default: return '';
    }
  }
  const rowHTML = (r, at) => `<div class="row ${r.status} ${r.role}${at ? ' at' : ''}" data-id="${r.id}"><span class="cur">▌</span><span class="spin">${spinOf(r)}</span><span class="mark">${markOf(r)}</span><span class="name">${esc(r.name)}</span><span class="fig">${esc(figOf(r))}</span></div>`;

  function render() {
    if (listing) {
      rowsEl.innerHTML = REST.map((c, i) => `<div class="row rest${i === listAt ? ' at' : ''}"><span class="cur">▌</span><span class="spin"></span><span class="mark">${MARK.conversation}</span><span class="name">${esc(c.name)}</span><span class="fig">${c.when}</span></div>`).join('');
    } else {
      // Rows stand by place, air between places, nothing moving for
      // its state.
      cursor = Math.max(0, Math.min(cursor, rows.length - 1));
      let html = '', last = -1;
      rows.forEach((r, i) => {
        const place = PLACES.indexOf(r.role);
        if (last >= 0 && place !== last) html += '<div class="air"></div>';
        last = place;
        html += rowHTML(r, i === cursor);
      });
      rowsEl.innerHTML = html;
    }
    linesEl.innerHTML = OTHERS.map(p => `<div class="pline${p.waiting ? ' waiting' : ''}"><span class="spin">${p.turning ? '⣾' : ''}</span><span class="pname">${esc(p.name)}</span><span class="fig">${esc(p.standing)}</span></div>`).join('');
    const hints = listing ? HINTS.list : verb ? HINTS[verb] : HINTS.project;
    footEl.className = 'foot' + (note ? ' say' : '');
    footEl.innerHTML = note ? esc(note) : hints.map(([k, d]) => `<i><b>${esc(k)}</b> ${esc(d)}</i>`).join('');
    for (const th of document.querySelectorAll('.verbs th[data-verb]')) th.classList.toggle('pending', th.dataset.verb === verb);
    for (const td of document.querySelectorAll('.verbs td[data-key]')) td.classList.toggle('pending', !!verb && td.dataset.key[0] === verb);
  }

  // ---- the verbs ------------------------------------------------------

  // The stack and the checks as written down: what ss and st bring
  // back once they are gone.
  const written = role => OPEN.rows.filter(r => r.role === role).map(r => ({ ...r, id: Math.random() }));
  const of = role => rows.filter(r => r.role === role);
  const sorted = () => { rows.sort((a, b) => PLACES.indexOf(a.role) - PLACES.indexOf(b.role)); };

  // A row started: it turns in the margin, then is what it is.
  function start(r, done) {
    r.status = r.role === 'check' ? 'running' : 'starting';
    render();
    wait(r.role === 'check' ? 1400 : 900, () => {
      r.status = r.role === 'check' ? (r.name === 'lint' ? 'failed' : 'passed') : 'serving';
      if (r.role === 'check') r.age = 'now';
      render();
      if (done) done();
    });
  }
  function startAll(list) { list.forEach((r, i) => wait(i * 120, () => start(r))); }
  // A role brought up: what stands, or what is written down where
  // nothing stands.
  function bring(role) {
    let list = of(role);
    if (!list.length) { list = written(role); rows.push(...list); sorted(); }
    startAll(list);
  }
  function stop(list) { rows = rows.filter(r => !list.includes(r)); render(); }

  // A process handed to an agent: a new conversation at work on it,
  // which in a while stops to ask, as any does.
  function hand(title) {
    const c = { role: 'conversation', name: title, status: 'working', ctx: '12k', id: Math.random() };
    rows.push(c); sorted(); render();
    wait(4500, () => { if (rows.includes(c)) { c.status = 'waiting'; c.ctx = '31k'; render(); } });
  }

  function act(key) {
    note = '';
    if (listing) {
      if (key === 'esc') listing = false;
      else if (key === 'j' || key === 'down') listAt = (listAt + 1) % REST.length;
      else if (key === 'k' || key === 'up') listAt = (listAt + REST.length - 1) % REST.length;
      else if (key === 'enter') { listing = false; const c = { role: 'conversation', name: REST[listAt].name, status: 'idle', id: Math.random() }; rows.push(c); sorted(); cursor = rows.indexOf(c); }
      render(); return;
    }
    const at = rows[cursor];
    if (verb) {
      const v = verb; verb = '';
      lit(v + key);
      switch (v + key) {
        case 'ss': bring('stack'); break;
        case 'st': bring('check'); break;
        case 'sc': { const c = { role: 'conversation', name: 'new conversation', status: 'idle', id: Math.random() }; rows.push(c); sorted(); cursor = rows.indexOf(c); break; }
        case 'xx': if (at) stop([at]); break;
        case 'xs': stop(of('stack')); break;
        case 'xt': stop(of('check')); break;
        case 'xp': stop(rows.slice()); break;
        case 'rr': if (at) { if (at.role === 'conversation') at.status = 'working'; else start(at); } break;
        case 'rs': bring('stack'); break;
        case 'rt': bring('check'); break;
        case 'rf': { const f = rows.filter(r => r.status === 'failed'); if (f.length) startAll(f); else note = 'Nothing failed here.'; break; }
        case 'rc': listing = true; listAt = 0; break;
        case 'aa': if (at) { if (at.role === 'conversation') note = 'That is a conversation already; enter goes to it.'; else hand(at.hand); } break;
        case 'af': { const f = rows.filter(r => r.status === 'failed'); if (f.length) hand(f.length === 1 ? f[0].hand : `${f.length} runs failed`); else note = 'Nothing failed here.'; break; }
        case v + 'esc': break;
        default: verb = v; return; // not an object: the verb still waits
      }
      render(); return;
    }
    switch (key) {
      case 's': case 'x': case 'r': case 'a': verb = key; break;
      case 'j': case 'down': cursor = (cursor + 1) % Math.max(1, rows.length); break;
      case 'k': case 'up': cursor = (cursor + rows.length - 1) % Math.max(1, rows.length); break;
      case 'g': cursor = 0; break;
      case 'G': cursor = rows.length - 1; break;
      case 'tab': {
        const waiting = rows.map((r, i) => r.status === 'waiting' ? i : -1).filter(i => i >= 0);
        if (!waiting.length) { note = 'Nothing is waiting on you.'; break; }
        cursor = waiting.find(i => i > cursor) ?? waiting[0];
        lit('tab');
        break;
      }
      case 'esc': break;
      default: return;
    }
    render();
  }
  // The cell of the table a key lights, for a moment.
  function lit(key) {
    const el = document.querySelector(`[data-key="${key}"]`);
    if (!el) return;
    el.classList.add('lit');
    setTimeout(() => el.classList.remove('lit'), 500);
  }

  const panelOnScreen = () => { const b = panelEl.getBoundingClientRect(); return b.bottom > 80 && b.top < innerHeight; };
  addEventListener('keydown', e => {
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    if (e.target && /^(INPUT|TEXTAREA|SELECT|BUTTON)$/.test(e.target.tagName)) return;
    const key = { Escape: 'esc', Enter: 'enter', Tab: 'tab', ArrowDown: 'down', ArrowUp: 'up' }[e.key] || e.key;
    if (!panelOnScreen()) return; // the browser's, off the panel
    if (!/^(esc|enter|tab|down|up|j|k|g|G|s|x|r|a|t|f|c|p)$/.test(key)) return;
    if (!verb && !listing && /^(enter|t|f|c|p)$/.test(key)) return; // theirs to do elsewhere
    e.preventDefault();
    act(key);
  });
  for (const td of document.querySelectorAll('[data-key]')) {
    td.addEventListener('click', () => { const k = td.dataset.key; if (k.length === 2 && HINTS[k[0]]) { act(k[0]); act(k[1]); } else act(k); });
  }

  // ---- the line -------------------------------------------------------

  const typedEl = $('typed'), cursorEl = $('cursor');
  const LINE = 'Take the conn.';
  function type(n) {
    typedEl.textContent = LINE.slice(0, n);
    if (n < LINE.length) wait(n === 0 ? 300 : 55 + Math.random() * 50, () => type(n + 1));
    else wait(1800, () => cursorEl.classList.add('off'));
  }

  // ---- the gathering --------------------------------------------------

  const field = $('field');
  const lerp = (a, b, t) => a + (b - a) * t;
  const ease = t => t < 0 ? 0 : t > 1 ? 1 : t * t * (3 - 2 * t);
  const clamp01 = t => Math.max(0, Math.min(1, t));
  const wins = [];
  let seed = 7;
  const rnd = () => { seed = (seed * 9301 + 49297) % 233280; return seed / 233280; };

  function buildField() {
    const all = [[OPEN, 0], ...OTHERS.map((p, i) => [p, i + 1])];
    all.forEach(([p, pi]) => p.rows.forEach((r, ri) => {
      const w = document.createElement('div');
      w.className = `win ${r.status} ${r.role}`;
      w.innerHTML = `<div class="head"><span class="cur"></span><span class="spin">${spinOf(r)}</span><span class="mark">${markOf(r)}</span><span class="name">${esc(r.name)}</span></div><pre class="body">${r.body.join('\n')}</pre>`;
      field.appendChild(w);
      wins.push({ el: w, r, pi, ri, lines: r.body.length, seed: rnd(), seed2: rnd(), seed3: rnd(), stagger: rnd() });
    }));
  }
  // The field: a pane's size and place, scattered with some overlap.
  function scattered(w) {
    const W = innerWidth, H = innerHeight, phone = W < 720;
    const longest = Math.max(...w.r.body.map(l => l.replace(/<[^>]+>/g, '').length), w.r.name.length + 6);
    const ww = Math.min(phone ? 230 : 330, Math.max(phone ? 160 : 190, longest * 6.6 + 24));
    const hh = 20 + 10 + Math.min(w.lines, 5) * 16;
    const n = phone ? Math.min(wins.length, 10) : wins.length;
    const rowsN = Math.ceil(n / (phone ? 2 : 5)), base = Math.floor(n / rowsN), extra = n % rowsN;
    const i = wins.indexOf(w);
    let cy = 0, at = i;
    while (at >= base + (cy < extra ? 1 : 0)) { at -= base + (cy < extra ? 1 : 0); cy++; }
    const cols = base + (cy < extra ? 1 : 0), cx = at;
    const top = 64, fieldH = H - top - 48;
    const cellW = (W - 32) / cols, cellH = fieldH / rowsN;
    const x = 16 + cx * cellW + (cellW - ww) / 2 + (w.seed - 0.5) * Math.max(0, cellW - ww) * 0.6;
    const y = top + cy * cellH + (cellH - hh) / 2 + (w.seed3 - 0.5) * Math.max(0, cellH - hh) * 0.6;
    return { x: Math.max(8, Math.min(W - ww - 8, x)), y: Math.max(top, Math.min(top + fieldH - hh, y)), w: ww, h: hh };
  }
  // A pane's slot on the panel: the open project's row, or its
  // project's line, where it stands on the screen now.
  function slot(w) {
    const r = w.pi === 0 ? rows[w.ri] : null;
    const el = w.pi === 0 ? (r && rowsEl.querySelector(`[data-id="${r.id}"]`)) : linesEl.children[w.pi - 1];
    const b = (el || panelEl).getBoundingClientRect();
    return { x: b.left, y: b.top, w: b.width, h: b.height };
  }

  // One frame of the gathering, at a progress: drifting apart before
  // half, each pane going to its slot in its own moment after.
  function frame(pr) {
    const now = performance.now() / 1000;
    const phone = innerWidth < 720;
    wins.forEach((w, i) => {
      const t = ease(clamp01((pr - 0.5 - w.stagger * 0.3) / 0.35));
      const s = scattered(w), d = slot(w);
      const x = lerp(s.x, d.x, t), y = lerp(s.y, d.y, t), ww = lerp(s.w, d.w, t), hh = lerp(s.h, d.h, t);
      const dr = 1 - t;
      const dx = Math.sin(now * 0.37 + w.seed * 9) * 6 * dr, dy = Math.cos(now * 0.29 + w.seed2 * 7) * 5 * dr;
      w.el.style.transform = `translate(${(x + dx).toFixed(1)}px, ${(y + dy).toFixed(1)}px)`;
      w.el.style.width = ww.toFixed(1) + 'px';
      w.el.style.height = hh.toFixed(1) + 'px';
      w.el.classList.toggle('row', t > 0.85);
      w.el.querySelector('.body').style.opacity = String(1 - ease(clamp01(t * 1.6)));
      w.el.style.zIndex = w.r.status === 'waiting' ? 35 : 10 + Math.floor(w.seed * 20);
      let o = 1;
      if (w.pi > 0) o = 1 - ease(clamp01((t - 0.7) / 0.3));
      if (phone && i >= 10) o = Math.min(o, ease(clamp01(t * 2)));
      w.el.style.opacity = String(o);
    });
  }
  function landed() {
    field.remove();
    panelEl.classList.add('on');
    $('verbs').closest('.grammar').classList.add('on');
    type(0);
  }
  function gather() {
    const from = performance.now(), dur = 3400;
    const step = now => {
      const pr = clamp01((now - from) / dur);
      frame(pr);
      if (pr < 1) requestAnimationFrame(step); else landed();
    };
    requestAnimationFrame(step);
  }

  // ---- the agent's pane -----------------------------------------------

  // Scrolled into view, the agent runs the checks through conn, and
  // conn says what came of it.
  const CMD = 'conn u test -w -- go test -count=1 ./...';
  const OUT = ['ok  github.com/w0zro/conn  2.814s', 'ok  github.com/w0zro/conn/internal/theme  0.112s', 'ok  github.com/w0zro/conn/internal/tmux  0.391s', 'test exited 0'];
  function agent() {
    const cmd = $('agentcmd'), out = $('agentout'), cur = $('agentcur');
    const typeCmd = n => {
      cmd.textContent = CMD.slice(0, n);
      if (n < CMD.length) wait(28 + Math.random() * 30, () => typeCmd(n + 1));
      else wait(700, () => { cur.classList.add('off'); say(0); });
    };
    const say = n => {
      out.textContent = OUT.slice(0, n).join('\n');
      if (n < OUT.length) wait(n === 0 ? 300 : 420, () => say(n + 1));
    };
    typeCmd(0);
  }
  const shell = $('shell');
  if ('IntersectionObserver' in window && !held) {
    const io = new IntersectionObserver(es => { if (es.some(e => e.isIntersecting)) { io.disconnect(); agent(); } }, { threshold: 0.6 });
    io.observe(shell);
  } else {
    $('agentcmd').textContent = CMD; $('agentcur').classList.add('off'); $('agentout').textContent = OUT.join('\n');
  }

  // ---- the rest -------------------------------------------------------

  // The spinner turns in the margin of every row at work.
  if (!reduced) {
    const FRAMES = ['⣾', '⣽', '⣻', '⢿', '⡿', '⣟', '⣯', '⣷'];
    let f = 0;
    setInterval(() => { f = (f + 1) % FRAMES.length; for (const s of document.querySelectorAll('.spin')) if (s.textContent && s.textContent !== '⣿') s.textContent = FRAMES[f]; }, 110);
  }
  for (const b of document.querySelectorAll('.copy')) {
    b.addEventListener('click', async () => {
      try { await navigator.clipboard.writeText(b.dataset.copy); b.textContent = '[copied]'; b.classList.add('done'); setTimeout(() => { b.textContent = '[copy]'; b.classList.remove('done'); }, 1400); }
      catch (err) { b.textContent = '[select it]'; }
    });
  }

  // ---- opening --------------------------------------------------------

  render();
  // The face, or a moment and a half for it: a font that never settles
  // does not hold the page.
  const ready = Promise.race([document.fonts && document.fonts.ready || Promise.resolve(), new Promise(r => setTimeout(r, 1500))]);
  ready.then(() => {
    if (held && Number(held[1]) >= 1 || reduced) {
      // The page as it stands once gathered.
      field.remove();
      panelEl.classList.add('hold');
      $('verbs').closest('.grammar').classList.add('hold');
      typedEl.textContent = LINE; cursorEl.classList.add('off');
      for (const k of (look.get('k') || '').split(',').filter(Boolean)) act(k);
      return;
    }
    buildField();
    if (held) { frame(Number(held[1])); return; } // the field, held
    let raf = 0;
    const drift = () => { frame(0); raf = requestAnimationFrame(drift); };
    drift();
    wait(900, () => { cancelAnimationFrame(raf); gather(); });
  });
})();
