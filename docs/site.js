// conn.w0zro.com: the gathering. A field of panes - conversations,
// servers, shells, checks, more than anyone can watch, one of them
// asking a question that nothing says - files itself into conn's panel
// on one scroll: the project you are in on top, a row a process, and
// under it a line for every other project with something running.
// Beside the panel, once the words are in, one line is typed: Take the
// conn. Then the install. Every piece is laid out for the scroll's
// position, so scrolling back is the same picture; left alone, the
// page gathers on its own. The panel is drawn with conn's own marks,
// inks and words.
(() => {
  'use strict';

  const $ = id => document.getElementById(id);
  const field = $('field'), panelbg = $('panelbg');
  const reduced = matchMedia('(prefers-reduced-motion: reduce)').matches;
  const esc = s => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

  // ---- the processes --------------------------------------------------

  // A row's mark is its role: a conversation, the stack, what talks to
  // it, a check and its verdict, a listener outside conn.
  const MARK = { conversation: '❝', stack: '○', client: '❯', check: '◦', pass: '✓', fail: '✗', outside: '◌' };

  // The open project and what works it, then three more. Each process
  // is a pane in the field; the open project's become its rows, the
  // others' gather into their project's line. A body is what the
  // process has written, a few lines; a waiting conversation carries
  // its question. The figure is what the row says at its right.
  const PROJECTS = [
    { name: 'conn', path: '~/projects/w0zro/conn', rows: [
      { role: 'conversation', name: 'bring the code to the manual', status: 'working', fig: '262k', body: ['<span class="g">></span> bring the code to the manual', '<span class="r">●</span> Read panel.go, cli.go, server.go', '<span class="r">⣾</span> <span class="g">Editing… (esc to interrupt)</span>'] },
      { role: 'conversation', name: 'a grammar over processes', status: 'waiting', fig: '118k  waiting', cursor: true, body: ['<span class="g">></span> make s a verb that waits for its object', '<span class="r">●</span> Read panel.go, conn_test.go', '<span class="a">●</span> Free n, u and t once sc, ss and st are in?', '  <span class="a">❯</span> 1. Yes', '    2. No'] },
      { role: 'stack', name: 'demo', status: 'serving', fig: ':8123', body: ['<span class="g">Serving HTTP on :: port 8123</span>', '<span class="g">::1 - -</span> "GET / HTTP/1.1" 200 -', '<span class="g">::1 - -</span> "GET /manual/ HTTP/1.1" 200 -'] },
      { role: 'client', name: 'zsh', status: 'idle', body: ['<span class="a">$</span> git status -sb', '<span class="g">## main...origin/main [ahead 3]</span>', '<span class="a">$</span> <span class="cursor"></span>'] },
      { role: 'check', name: 'test', status: 'passed', fig: '4m', body: ['<span class="g">ok  github.com/w0zro/conn  2.814s</span>', '<span class="g">ok  internal/theme  0.112s</span>', '<span class="g">ok  internal/tmux  0.391s</span>', '<span class="f">[test exited 0]</span>'] },
      { role: 'check', name: 'lint', status: 'failed', fig: '2m', body: ['<span class="a">panel.go:2206:4:</span> unused: nameFor', '<span class="g">2 issues:</span>', '<span class="g">* unused: 2</span>', '<span class="f">[lint exited 1]</span>'] },
      { role: 'outside', name: 'postgres', status: 'serving', fig: ':5432', body: ['<span class="g">LOG:</span>  database system is ready', '<span class="g">LOG:</span>  checkpoint starting: time', '<span class="g">LOG:</span>  connection authorized: user=conn'] },
    ] },
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

  // ---- the pieces -----------------------------------------------------

  let W = innerWidth, H = innerHeight, ROW = 20, phone = false, PANEL_W = 370;
  const wins = [], pieces = [];
  let seed = 7;
  const rnd = () => { seed = (seed * 9301 + 49297) % 233280; return seed / 233280; };

  // The panel's rows, top to bottom: air, the open project's head, air,
  // its rows, air, the projects label, a line a project, air.
  const open = PROJECTS[0];
  const ROWS_AT = 3;
  const LABEL_AT = ROWS_AT + open.rows.length + 1;
  const LINES_AT = LABEL_AT + 1;
  const PANEL_ROWS = LINES_AT + PROJECTS.length - 1 + 1;

  function markOf(r) {
    if (r.role === 'check') return r.status === 'passed' ? MARK.pass : r.status === 'failed' ? MARK.fail : MARK.check;
    return MARK[r.role];
  }
  function spinOf(r) {
    if (r.status === 'working' || (r.role === 'check' && r.status === 'running')) return '⣾';
    if (r.status === 'serving') return '⣿';
    return '';
  }

  function build() {
    field.innerHTML = '';
    wins.length = 0; pieces.length = 0;
    seed = 7;
    const piece = (cls, html, at) => {
      const e = document.createElement('div');
      e.className = cls;
      e.innerHTML = html;
      field.appendChild(e);
      pieces.push({ el: e, at });
    };
    piece('phead', `<span class="pname">${esc(open.name)}</span><span class="ppath">${esc(open.path)}</span>`, 1);
    piece('plabel', `<span class="pname">projects</span><span class="rule"></span>`, LABEL_AT);
    PROJECTS.forEach((p, pi) => {
      if (pi > 0) piece(`pline${p.waiting ? ' waiting' : ''}`, `<span class="spin">${p.turning ? '⣾' : ''}</span><span class="pname">${esc(p.name)}</span><span class="fig">${esc(p.standing)}</span>`, LINES_AT + pi - 1);
      p.rows.forEach((r, ri) => {
        const w = document.createElement('div');
        w.className = `win ${r.status} ${r.role}${r.cursor ? ' cursor' : ''}`;
        w.innerHTML = `<div class="drift"><div class="head"><span class="cur">${r.cursor ? '▌' : ''}</span><span class="spin">${spinOf(r)}</span><span class="mark">${markOf(r)}</span><span class="name">${esc(r.name)}</span>${r.fig ? `<span class="fig">${esc(r.fig)}</span>` : ''}</div><pre class="body">${r.body.join('\n')}</pre></div>`;
        field.appendChild(w);
        const lines = r.body.length;
        wins.push({ el: w, r, p, pi, ri, lines, seed: rnd(), seed2: rnd(), seed3: rnd(), stagger: rnd() });
      });
    });
  }

  // ---- the layout ----------------------------------------------------

  // The scroll is one screen: from the field, a pane a process, to the
  // panel, a row a process, with the line and the install beside it.
  const lerp = (a, b, t) => a + (b - a) * t;
  const ease = t => t < 0 ? 0 : t > 1 ? 1 : t * t * (3 - 2 * t);
  const clamp01 = t => Math.max(0, Math.min(1, t));
  const claimEl = $('claim'), typedEl = $('typed'), cursorEl = claimEl.querySelector('.cursor');
  const LINE = 'Take the conn.';

  function measure() {
    W = innerWidth; H = innerHeight;
    phone = W < 720;
    ROW = phone || H < 900 ? 18 : 20;
    document.documentElement.style.setProperty('--row', ROW + 'px');
    PANEL_W = phone ? Math.min(W - 24, 360) : 370;
  }

  // The field: a pane's size and place, scattered with some overlap.
  function scattered(w) {
    const longest = Math.max(...w.r.body.map(l => l.replace(/<[^>]+>/g, '').length), w.r.name.length + 6);
    const ww = Math.min(phone ? 230 : 330, Math.max(phone ? 160 : 190, longest * 6.6 + 24));
    const hh = ROW + 10 + Math.min(w.lines, 5) * 16;
    // Rows as even as they can be: fifteen panes in rows of five.
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

  // The panel: left of the middle, the line beside it; on a phone, at
  // the top, the line under it.
  const panelH = () => PANEL_ROWS * ROW;
  function panelAt() {
    const ph = panelH();
    if (phone) return { x: (W - PANEL_W) / 2, y: Math.max(56, Math.min(80, (H - ph - 200) / 2)), h: ph };
    const gap = 72, total = PANEL_W + gap + 520;
    const x = Math.max(24, (W - total) / 2);
    return { x, y: Math.max(56, (H - ph) / 2), h: ph, lineX: x + PANEL_W + gap };
  }
  // A pane's slot: the open project's row, or its project's line.
  function slot(w) {
    const p = panelAt();
    const row = w.pi === 0 ? ROWS_AT + w.ri : LINES_AT + w.pi - 1;
    return { x: p.x, y: p.y + row * ROW, w: PANEL_W, h: ROW };
  }
  function pieceSlot(e) {
    const p = panelAt();
    return { x: p.x, y: p.y + e.at * ROW };
  }

  // ---- laying out for a position --------------------------------------

  let progress = 0, drifting = true, raf = 0, typing = 0, typed = 0;

  function layout(pr) {
    progress = pr = clamp01(pr);
    const p = panelAt();
    const gatherT = ease(clamp01((pr - 0.5) / 0.35));
    panelbg.style.width = PANEL_W + 'px'; panelbg.style.height = p.h + 'px';
    panelbg.style.transform = `translate(${p.x}px, ${p.y}px)`;
    panelbg.style.opacity = String(gatherT);
    const now = performance.now() / 1000;
    wins.forEach((w, i) => {
      const t = ease(clamp01((pr - 0.08 - w.stagger * 0.45) / 0.45)); // its own moment to go
      const s = scattered(w), d = slot(w);
      const x = lerp(s.x, d.x, t), y = lerp(s.y, d.y, t), ww = lerp(s.w, d.w, t), hh = lerp(s.h, d.h, t);
      const dr = drifting && !reduced ? (1 - t) : 0;
      const dx = Math.sin(now * 0.37 + w.seed * 9) * 6 * dr, dy = Math.cos(now * 0.29 + w.seed2 * 7) * 5 * dr;
      w.el.style.transform = `translate(${(x + dx).toFixed(1)}px, ${(y + dy).toFixed(1)}px)`;
      w.el.style.width = ww.toFixed(1) + 'px';
      w.el.style.height = hh.toFixed(1) + 'px';
      w.el.classList.toggle('row', t > 0.85);
      w.el.querySelector('.body').style.opacity = String(1 - ease(clamp01(t * 1.6)));
      const fig = w.el.querySelector('.fig'); if (fig) fig.style.opacity = String(ease(clamp01((pr - 0.84) / 0.14)));
      w.el.style.zIndex = w.r.status === 'waiting' ? 35 : 10 + Math.floor(w.seed * 20);
      // Another project's panes gather into its line, and are gone by
      // the time the line is there: the line says what runs in it.
      let o = 1;
      if (w.pi > 0) o = 1 - ease(clamp01((t - 0.7) / 0.3));
      if (phone && i >= 10) o = Math.min(o, ease(clamp01(t * 2)));
      w.el.style.opacity = String(o);
    });
    pieces.forEach(e => {
      const d = pieceSlot(e);
      e.el.style.width = PANEL_W + 'px';
      e.el.style.transform = `translate(${d.x.toFixed(1)}px, ${d.y.toFixed(1)}px)`;
      e.el.style.opacity = String(ease(clamp01((pr - 0.55) / 0.3)));
      const fig = e.el.querySelector('.fig'); if (fig) fig.style.opacity = String(ease(clamp01((pr - 0.84) / 0.14)));
    });
    // The line, beside the panel, typed once the words are in.
    const arrived = pr >= 0.96;
    if (phone) { claimEl.style.left = '18px'; claimEl.style.right = '18px'; claimEl.style.top = (p.y + p.h + 28) + 'px'; }
    else { claimEl.style.left = p.lineX + 'px'; claimEl.style.right = ''; claimEl.style.top = (p.y + Math.max(0, (p.h - 260) / 2)) + 'px'; }
    claimEl.style.opacity = String(ease(clamp01((pr - 0.9) / 0.1)));
    claimEl.classList.toggle('on', arrived);
    if (arrived && !typing && typed < LINE.length) type();
    if (!arrived && pr < 0.8) { clearTimeout(typing); typing = 0; typed = 0; typedEl.textContent = ''; cursorEl.classList.remove('off'); }
    drifting = pr < 0.6;
  }
  function type() {
    typing = setTimeout(() => {
      typed = Math.min(LINE.length, typed + 1);
      typedEl.textContent = LINE.slice(0, typed);
      typing = 0;
      if (typed < LINE.length) type(); else setTimeout(() => cursorEl.classList.add('off'), 1800);
    }, reduced ? 0 : (typed === 0 ? 350 : 55 + Math.random() * 50));
  }

  // ---- the scroll -----------------------------------------------------

  const still = location.hash.match(/^#p=([\d.]+)$/); // a moment held, for a look
  const current = () => still ? Number(still[1]) : Math.max(0, scrollY / innerHeight);
  let touched = false;
  function frame() {
    layout(current());
    if (drifting && !reduced) raf = requestAnimationFrame(frame); else raf = 0;
  }
  addEventListener('scroll', () => { if (!raf) raf = requestAnimationFrame(frame); }, { passive: true });
  addEventListener('wheel', () => { touched = true; }, { passive: true });
  addEventListener('touchstart', () => { touched = true; }, { passive: true });
  addEventListener('resize', () => { measure(); if (!raf) raf = requestAnimationFrame(frame); });
  addEventListener('keydown', e => {
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    if (['j', 'ArrowDown', ' ', 'Enter', 'Tab', 'PageDown'].includes(e.key)) { e.preventDefault(); touched = true; window.scrollTo({ top: innerHeight, behavior: reduced ? 'auto' : 'smooth' }); }
    else if (['k', 'ArrowUp', 'PageUp', 'g'].includes(e.key)) { e.preventDefault(); touched = true; window.scrollTo({ top: 0, behavior: reduced ? 'auto' : 'smooth' }); }
  });
  // Left alone, the page gathers on its own, at a pace of its own:
  // the scroll is driven over two seconds, the snap held off meanwhile.
  function gatherAlone() {
    if (touched || scrollY > 10) return;
    if (reduced) { window.scrollTo({ top: innerHeight }); return; }
    const html = document.documentElement, from = performance.now(), dur = 2200;
    html.style.scrollSnapType = 'none';
    const step = now => {
      if (touched) { html.style.scrollSnapType = ''; return; }
      const t = ease(clamp01((now - from) / dur));
      window.scrollTo(0, t * innerHeight);
      if (t < 1) requestAnimationFrame(step); else html.style.scrollSnapType = '';
    };
    requestAnimationFrame(step);
  }
  if (!still) setTimeout(gatherAlone, 2400);

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

  measure();
  build();
  frame();
  if (document.fonts && document.fonts.ready) document.fonts.ready.then(() => { measure(); layout(current()); });
})();
