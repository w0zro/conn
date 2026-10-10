// conn.w0zro.com: a figure from the memorandum, in motion. A field of
// panes - agents, servers, shells, editors, tests, more than anyone can
// watch, two of them asking a question that nothing says - files itself
// into conn's panel as the page is scrolled: a row a process, under the
// project it works, with a word a project where something waits; one
// row opens in the workspace; then the installation procedure. Every
// piece is laid out for the scroll's position, between keyframes, so
// scrolling back is the same picture. The panes are the machine's
// processes as conn would file them, and the panel they make is drawn
// with conn's own marks, inks and words.
(() => {
  'use strict';

  const $ = id => document.getElementById(id);
  const field = $('field'), panelbg = $('panelbg'), pane = $('pane'), scroll = $('scroll');
  const reduced = matchMedia('(prefers-reduced-motion: reduce)').matches;
  const esc = s => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

  // ---- the processes --------------------------------------------------

  const MARK = { contact: '☿', shell: '❯', editor: '▯', service: '◉', run: '○' };

  // Four projects and what works them. Each is a window in the field
  // and a row in the panel. A body is what the process has written,
  // a few lines; a waiting contact carries its question.
  const PROJECTS = [
    { name: 'conn', rows: [
      { kind: 'contact', digit: '0', name: 'claude', status: 'waiting', word: '9 MIN', body: ['<span class="g">></span> bring the code to the manual', '<span class="r">●</span> Read band.go, keys.go, bay.go', '<span class="a">●</span> Rename bay to workspace in the code too?', '  <span class="a">❯</span> 1. Yes', '    2. No'] },
      { kind: 'shell', name: 'zsh', status: 'idle', body: ['<span class="a">$</span> git status -sb', '<span class="g">## main...origin/main [ahead 3]</span>', '<span class="a">$</span> <span class="cursor"></span>'] },
      { kind: 'run', name: 'go test ./...', status: 'fault', word: 'EXIT 1', body: ['<span class="g">ok  internal/draw  0.214s</span>', '<span class="a">--- FAIL: TestBand (0.00s)</span>', '    band_test.go:41: want "CONN · LOG 1"', '<span class="a">FAIL</span>'] },
      { kind: 'editor', name: 'vim', status: 'stopped', word: 'STOPPED', body: ['<span class="k"># conn</span>', '- the panel is by project', '- bay → workspace', '<span class="f">~</span>', '<span class="g">"notes.md" 5L</span>'] },
    ] },
    { name: 'web', rows: [
      { kind: 'contact', digit: '1', name: 'claude', status: 'working', body: ['<span class="g">></span> make the cart total update live', '<span class="r">●</span> Read Cart.tsx, useCart.ts', '<span class="r">⣾</span> <span class="g">Editing… (esc to interrupt)</span>'] },
      { kind: 'shell', name: 'zsh', status: 'idle', body: ['<span class="a">$</span> npm run dev', '<span class="g">> web@0.4.1 dev</span>', '<span class="a">$</span> <span class="cursor"></span>'] },
      { kind: 'service', name: 'postgres', status: 'serving', word: ':5432', body: ['<span class="g">LOG:</span>  database system is ready', '<span class="g">LOG:</span>  checkpoint starting: time', '<span class="g">LOG:</span>  connection authorized: user=web'] },
      { kind: 'run', name: 'node vite', status: 'serving', word: ':5173', body: ['<span class="r">VITE v6.0.3</span>  <span class="g">ready in 212 ms</span>', '', '  ➜  Local:   <span class="r">http://localhost:5173/</span>', '  ➜  <span class="g">press h + enter to show help</span>'] },
      { kind: 'service', name: 'redis', status: 'serving', word: ':6379', body: ['<span class="g">1:M</span> Ready to accept connections tcp', '<span class="g">1:M</span> DB saved on disk'] },
      { kind: 'run', name: 'worker', status: 'down', word: 'DOWN', body: ['<span class="f">make run</span>', '<span class="f">not running</span>'] },
    ] },
    { name: 'api', rows: [
      { kind: 'contact', digit: '2', name: 'claude', status: 'waiting', word: '23 MIN', body: ['<span class="g">></span> add the rate limit to /v1/search', '<span class="r">●</span> Read limiter.go, search.go', '<span class="a">●</span> Allow Bash: go test ./...?', '  <span class="a">❯</span> Yes', '    No'] },
      { kind: 'shell', name: 'zsh', status: 'idle', body: ['<span class="a">$</span> docker compose up', '<span class="a">$</span> <span class="cursor"></span>'] },
      { kind: 'run', name: 'docker compose', status: 'working', body: ['<span class="g">api-1  |</span> listening on :8080', '<span class="g">db-1   |</span> ready to accept connections', '<span class="g">api-1  |</span> GET /v1/search 200 12ms'] },
      { kind: 'run', name: 'go run ./cmd/api', status: 'serving', word: ':8080', body: ['<span class="g">2026/10/10 14:02:11</span> listening on :8080', '<span class="g">2026/10/10 14:02:40</span> GET /healthz 200'] },
      { kind: 'run', name: 'pytest', status: 'working', body: ['tests/test_search.py <span class="r">........</span>', 'tests/test_limit.py <span class="r">...</span><span class="g">.</span>', '<span class="r">⣾</span> <span class="g">11 passed · running</span>'] },
    ] },
    { name: 'notes', rows: [
      { kind: 'contact', digit: '3', name: 'claude', status: 'idle', body: ['<span class="g">></span> outline the october post', '<span class="r">●</span> Wrote outline to 2026-10-post.md', '<span class="p">❯</span> <span class="cursor"></span>'] },
      { kind: 'editor', name: 'vim', status: 'stopped', word: 'STOPPED', body: ['<span class="k"># October</span>', 'The shift from files to processes.', '<span class="f">~</span>', '<span class="g">"2026-10-post.md" 2L</span>'] },
      { kind: 'run', name: 'hugo server', status: 'serving', word: ':1313', body: ['<span class="g">Web Server is available at</span> <span class="r">http://localhost:1313/</span>', '<span class="g">Press Ctrl+C to stop</span>'] },
      { kind: 'shell', name: 'zsh', status: 'idle', body: ['<span class="a">$</span> <span class="cursor"></span>'] },
    ] },
  ];

  // ---- the pieces -----------------------------------------------------

  let W = innerWidth, H = innerHeight, ROW = 20, phone = false, PANEL_W = 370;
  const wins = [], eyebrows = [];
  let seed = 7;
  const rnd = () => { seed = (seed * 9301 + 49297) % 233280; return seed / 233280; };

  function build() {
    field.innerHTML = '';
    wins.length = 0; eyebrows.length = 0;
    seed = 7;
    PROJECTS.forEach((p, pi) => {
      const e = document.createElement('div');
      e.className = 'eyebrow';
      const stamp = verdict(p);
      e.innerHTML = `<span class="pname">${p.name}</span><span class="rule"></span>${stamp ? `<span class="tail ${stamp.cls}">${stamp.word}</span>` : ''}`;
      field.appendChild(e);
      eyebrows.push({ el: e, p, pi });
      p.rows.forEach((r, ri) => {
        const w = document.createElement('div');
        w.className = `win ${r.status}`;
        const wordCls = r.status === 'waiting' ? 'word stamp blink' : (r.status === 'fault' || r.status === 'stopped') ? 'word stamp' : 'word';
        w.innerHTML = `<div class="drift"><div class="head"><span class="cur"></span><span class="spin">${r.status === 'working' ? '⣾' : ''}</span><span class="mark">${MARK[r.kind]}</span>${r.kind === 'contact' ? `<span class="digit">${r.digit}</span>` : '<span class="digit"></span>'}<span class="name">${esc(r.name)}</span>${r.word ? `<span class="${wordCls}">${r.word}</span>` : ''}</div><pre class="body">${r.body.join('\n')}</pre></div>`;
        field.appendChild(w);
        const lines = r.body.length;
        wins.push({ el: w, r, p, pi, ri, lines, seed: rnd(), seed2: rnd(), seed3: rnd(), stagger: rnd() });
      });
    });
    // The pane: the first waiting contact, gone into.
    pane.innerHTML = `<div class="g">&gt; <span class="p">bring the code to the manual</span></div><br><div><span class="r">●</span> Read band.go, keys.go, bay.go, dress.go</div><br><div><span class="a">●</span> The type is bay in four files, and the held screens under testdata name it. Renaming it is one commit, with the screens held again.</div><br><div>&nbsp;&nbsp;Rename bay to workspace in the code too?</div><div>&nbsp;&nbsp;<span class="a">❯</span> 1. Yes</div><div>&nbsp;&nbsp;&nbsp;&nbsp;2. No, leave the code as it is</div><div class="band"><b> CONN </b> · LOG 3<span style="float:right">14:11 · T+ 01h 36m </span></div>`;
  }

  // A block's verdict: what wants you, at the end of its rule.
  function verdict(p) {
    const waiting = p.rows.filter(r => r.status === 'waiting').length;
    if (waiting > 0) return { word: waiting > 1 ? `${waiting} WAITING` : 'WAITING', cls: 'stamp blink' };
    const faults = p.rows.filter(r => r.status === 'fault' || r.status === 'stopped');
    if (faults.length === 1) return { word: faults[0].word, cls: 'stamp' };
    if (faults.length > 1) return { word: `${faults.length} FAULTS`, cls: 'stamp' };
    return null;
  }

  // ---- the layouts ----------------------------------------------------

  // Where everything is at each beat of the scroll:
  //   0  the field, a window a process, scattered
  //   1  gathered: a row a process, under its project, on the panel
  //   2  the words: what each project wants, at the end of its rule
  //   3  one row gone into: the panel at the left, its pane at the right
  //   4  left: the pane gone, the panel back, everything running
  //   5  the end: the panel small and dim behind the install
  const lerp = (a, b, t) => a + (b - a) * t;
  const ease = t => t < 0 ? 0 : t > 1 ? 1 : t * t * (3 - 2 * t);
  const clamp01 = t => Math.max(0, Math.min(1, t));

  function measure() {
    W = innerWidth; H = innerHeight;
    phone = W < 720;
    ROW = phone || H < 1000 ? 18 : 20;
    document.documentElement.style.setProperty('--row', ROW + 'px');
    PANEL_W = phone ? Math.min(W - 24, 360) : 370;
  }

  // The field: a window's size and place, scattered with some overlap,
  // kept off the line of type at the lower left.
  function scattered(w) {
    const ww = phone ? 170 + w.seed2 * 50 : 220 + w.seed2 * 90;
    const hh = ROW + 10 + Math.min(w.lines, 5) * 16;
    const n = phone ? Math.min(wins.length, 10) : wins.length; // a phone's field holds ten
    const cols = phone ? 2 : 5, rowsN = Math.ceil(n / cols);
    const i = wins.indexOf(w), cx = i % cols, cy = Math.floor(i / cols);
    const fieldH = H - (phone ? 330 : 340) - 60; // above the caption's ground
    const cellW = (W - 40) / cols, cellH = fieldH / rowsN;
    const x = 20 + cx * cellW + (w.seed - 0.5) * cellW * 0.5 + (cellW - ww) / 2;
    const y = 60 + cy * cellH + (w.seed3 - 0.5) * cellH * 0.4;
    return { x: Math.max(8, Math.min(W - ww - 8, x)), y: Math.max(56, Math.min(60 + fieldH - hh, y)), w: ww, h: hh, body: 1 };
  }

  // The panel: its place at a beat, and each row's slot in it.
  function panelAt(beat) {
    const rowsTotal = PROJECTS.reduce((n, p) => n + p.rows.length + 2, 0);
    const ph = rowsTotal * ROW;
    let x, y = Math.max(56, Math.min((H - ph) / 2 - 60, H - 330 - ph)), scale = 1; // above the caption's ground
    if (beat >= 3 && beat < 4) x = 0; // at the left, the pane beside it
    else if (beat >= 5) { x = (W - PANEL_W) / 2; scale = phone ? 0.8 : 0.9; }
    else x = phone ? (W - PANEL_W) / 2 : Math.max(24, (W - PANEL_W) / 2 - (phone ? 0 : 60));
    if (phone && beat >= 3 && beat < 4) x = -PANEL_W - 20;
    return { x, y, h: ph, scale };
  }
  function slot(w, beat) {
    const p = panelAt(beat);
    let yy = 0;
    for (let pi = 0; pi < w.pi; pi++) yy += (PROJECTS[pi].rows.length + 2) * ROW;
    yy += (2 + w.ri) * ROW; // the air, then the eyebrow
    return { x: p.x, y: p.y + yy, w: PANEL_W, h: ROW, body: 0 };
  }
  function eyebrowSlot(e, beat) {
    const p = panelAt(beat);
    let yy = 0;
    for (let pi = 0; pi < e.pi; pi++) yy += (PROJECTS[pi].rows.length + 2) * ROW;
    return { x: p.x, y: p.y + yy + ROW };
  }

  // ---- laying out for a position --------------------------------------

  let progress = 0, drifting = true, raf = 0;
  const chosenId = () => wins.findIndex(w => w.r.status === 'waiting');

  function layout(pr) {
    progress = pr;
    const beat = Math.floor(pr), frac = pr - beat;
    const chosen = chosenId();
    // The panel's ground.
    const gatherT = ease(clamp01((pr - 0.6) / 0.9)); // the panel fades in as the rows arrive
    const pA = panelAt(Math.max(1, Math.min(pr, 5))), pB = panelAt(Math.max(1, Math.min(Math.ceil(pr), 5)));
    const pf = beat >= 1 ? ease(clamp01((frac - 0.6) / 0.4)) : 0; // a beat holds, then moves into the next in its last part
    const px = lerp(pA.x, pB.x, pf), py = lerp(pA.y, pB.y, pf), ps = lerp(pA.scale, pB.scale, pf);
    const dim = pr >= 4.4 ? 1 - 0.85 * ease(clamp01((pr - 4.4) / 0.6)) : 1; // behind the procedure, nearly dark
    panelbg.style.width = PANEL_W + 'px'; panelbg.style.height = pA.h + 'px';
    panelbg.style.transform = `translate(${px}px, ${py}px) scale(${ps})`;
    panelbg.style.transformOrigin = '0 0';
    panelbg.style.opacity = String(gatherT * dim);
    // Each window, between its scattered place and its slot.
    wins.forEach((w, i) => {
      const t = ease(clamp01((pr - 0.15 - w.stagger * 0.5) / 0.7)); // its own moment to go
      const s = scattered(w);
      const slotNow = { ...slot(w, Math.max(1, Math.min(pr, 5))) };
      // The slot moves with the panel from beat to beat.
      const a = slot(w, Math.max(1, Math.min(pr, 5))), b = slot(w, Math.max(1, Math.min(Math.ceil(pr), 5)));
      slotNow.x = lerp(a.x, b.x, pf); slotNow.y = lerp(a.y, b.y, pf);
      const x = lerp(s.x, slotNow.x, t), y = lerp(s.y, slotNow.y, t);
      const ww = lerp(s.w, slotNow.w, t), hh = lerp(s.h, slotNow.h, t);
      // Drift in the field, stilled as it goes.
      const dr = drifting && !reduced ? (1 - t) : 0;
      const now = performance.now() / 1000;
      const dx = Math.sin(now * 0.37 + w.seed * 9) * 6 * dr, dy = Math.cos(now * 0.29 + w.seed2 * 7) * 5 * dr;
      // At the end, the panel scales down about its own origin.
      const ox = px, oy = py;
      const sx = ox + (x - ox) * ps + dx, sy = oy + (y - oy) * ps + dy;
      w.el.style.transform = `translate(${sx.toFixed(1)}px, ${sy.toFixed(1)}px) scale(${ps})`;
      w.el.style.transformOrigin = '0 0';
      w.el.style.width = ww.toFixed(1) + 'px';
      w.el.style.height = hh.toFixed(1) + 'px';
      w.el.classList.toggle('row', t > 0.85);
      w.el.style.opacity = String(dim);
      const body = w.el.querySelector('.body'); body.style.opacity = String(1 - ease(clamp01(t * 1.6)));
      // The word at the right comes with the words beat; the chosen row
      // is raised while its pane is open.
      const word = w.el.querySelector('.word'); if (word) word.style.opacity = String(ease(clamp01((pr - 1.6) / 0.4)));
      const open = ease(clamp01((pr - 2.6) / 0.4)) * (1 - ease(clamp01((pr - 3.6) / 0.4)));
      w.el.classList.toggle('chosen', i === chosen && open > 0.5);
      w.el.querySelector('.cur').textContent = i === chosen && open > 0.5 ? '▌' : '';
      w.el.style.zIndex = i === chosen ? 40 : w.r.status === 'waiting' ? 35 : 10 + Math.floor(w.seed * 20);
      // A phone's field holds only so many; the rest arrive with the rows.
      if (phone && i >= 10) w.el.style.opacity = String(dim * ease(clamp01(t * 2)));
    });
    // The eyebrows come in with the rows; their tails with the words.
    eyebrows.forEach(e => {
      const a = eyebrowSlot(e, Math.max(1, Math.min(pr, 5))), b = eyebrowSlot(e, Math.max(1, Math.min(Math.ceil(pr), 5)));
      const x = lerp(a.x, b.x, pf), y = lerp(a.y, b.y, pf);
      e.el.style.width = PANEL_W + 'px';
      e.el.style.transform = `translate(${(px + (x - px) * ps).toFixed(1)}px, ${(py + (y - py) * ps).toFixed(1)}px) scale(${ps})`;
      e.el.style.transformOrigin = '0 0';
      e.el.style.opacity = String(ease(clamp01((pr - 0.7) / 0.6)) * dim);
      const tail = e.el.querySelector('.tail'); if (tail) tail.style.opacity = String(ease(clamp01((pr - 1.6) / 0.4)));
    });
    // The pane, beside the panel while one row is gone into.
    const open = ease(clamp01((pr - 2.6) / 0.4)) * (1 - ease(clamp01((pr - 3.6) / 0.4)));
    const pw = phone ? W - 24 : Math.min(W - PANEL_W - 60, 720);
    pane.style.width = pw + 'px'; pane.style.height = pA.h + 'px';
    pane.style.transform = `translate(${(phone ? 12 : PANEL_W + 28 + (1 - open) * 30).toFixed(1)}px, ${py.toFixed(1)}px)`;
    pane.style.opacity = String(open);
    // The line of type.
    const at = pr >= 5 ? -1 : Math.min(4, Math.max(0, Math.round(pr - 0.35)));
    document.querySelectorAll('.caption').forEach(c => c.classList.toggle('on', Number(c.dataset.at) === at));
    const fig = document.getElementById('figure'); if (fig) fig.textContent = at < 0 ? 'SEC. 2' : `FIG. 1-${at + 1}`;
    drifting = pr < 1.2;
  }

  // ---- the scroll -----------------------------------------------------

  const still = location.hash.match(/^#p=([\d.]+)$/); // a moment held, for a look
  const current = () => still ? Number(still[1]) : Math.max(0, scrollY / innerHeight);
  function frame() {
    layout(current());
    if (drifting && !reduced) raf = requestAnimationFrame(frame); else raf = 0;
  }
  addEventListener('scroll', () => { if (!raf) raf = requestAnimationFrame(frame); }, { passive: true });
  addEventListener('resize', () => { measure(); if (!raf) raf = requestAnimationFrame(frame); });
  addEventListener('keydown', e => {
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    const next = ['j', 'ArrowDown', ' ', 'Enter', 'Tab', 'PageDown'], prev = ['k', 'ArrowUp', 'PageUp'];
    const n = Math.round(scrollY / innerHeight);
    if (next.includes(e.key)) { e.preventDefault(); window.scrollTo({ top: (n + 1) * innerHeight, behavior: reduced ? 'auto' : 'smooth' }); }
    else if (prev.includes(e.key)) { e.preventDefault(); window.scrollTo({ top: (n - 1) * innerHeight, behavior: reduced ? 'auto' : 'smooth' }); }
    else if (e.key === 'g') window.scrollTo({ top: 0 }); else if (e.key === 'G') window.scrollTo({ top: document.body.scrollHeight });
  });

  // The spinner turns in every row at work.
  if (!reduced) {
    const FRAMES = ['⣾', '⣽', '⣻', '⢿', '⡿', '⣟', '⣯', '⣷'];
    let f = 0;
    setInterval(() => { f = (f + 1) % FRAMES.length; for (const s of document.querySelectorAll('.spin')) if (s.textContent) s.textContent = FRAMES[f]; }, 110);
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
