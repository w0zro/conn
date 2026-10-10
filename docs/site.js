// conn.w0zro.com is conn, playable. The window is the station: the
// console comes up first, staged the way conn stages it, against the
// visitor's own browser; then the panel, over a machine that runs in
// this page - two projects, their processes, each with a state and a
// clock - and the visitor works it with conn's own keys. Everything is
// drawn on the terminal's grid in rows of cells, with the arithmetic of
// conn's own drawing: internal/console/screen.go for the console,
// filed.go for the panel, readout.go for the page, band.go and
// keybar.go for the band. The words are conn's words. The one thing
// the site says for itself is the station's own page, in the
// workspace before the cursor moves. Nothing is kept or sent.
(() => {
  'use strict';

  // The build the site stands for. The release stamps these the way it
  // stamps the manual's cover.
  const V = { version: '0.23.0', built: '09-OCT-2026', commit: '12b688f', buildDate: new Date(2026, 9, 9) };
  const HOST = 'conn.w0zro.com';
  const INSTALL = 'curl -fsSL https://conn.w0zro.com/install.sh | sh';

  const $ = id => document.getElementById(id);
  const station = $('station'), panelEl = $('panel'), workEl = $('work');
  const b0 = $('b0'), b1 = $('b1'), bandEl = $('band');
  const reduced = matchMedia('(prefers-reduced-motion: reduce)').matches;
  const touch = matchMedia('(hover: none)').matches;
  const arrived = new Date();

  // ---- the grid -------------------------------------------------------

  const MARGIN = 3, MIN_COLS = 80, PANEL_W = 44, FACT_COL = 12, CHECK_COL = 12, STATUS_W = 9;
  let cellW = 8.4, cellH = 20, cols = 80, rows = 24, one = false, panelW = PANEL_W, workW = 40;

  function measureGrid() {
    const probe = document.createElement('pre');
    probe.textContent = 'M'.repeat(100);
    probe.style.cssText = 'position:absolute;visibility:hidden;white-space:pre;left:0;top:0';
    document.body.appendChild(probe);
    const r = probe.getBoundingClientRect();
    cellW = r.width / 100;
    cellH = r.height || cellH;
    probe.remove();
    document.documentElement.style.setProperty('--cell-w', cellW + 'px');
    cols = Math.max(20, Math.floor(innerWidth / cellW));
    rows = Math.max(8, Math.floor(station.clientHeight / cellH) - 2); // the band's two rows
    one = cols < PANEL_W + 56;
    panelW = one ? cols : PANEL_W;
    workW = one ? cols : cols - PANEL_W;
    station.classList.toggle('one', one);
    panelEl.style.width = one ? '' : (PANEL_W * cellW) + 'px';
  }

  // ---- a line of cells -----------------------------------------------

  const esc = s => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  const width = s => [...s].length;
  const fit = (s, w) => width(s) <= w ? s : (w > 1 ? [...s].slice(0, w - 1).join('') + '…' : '');
  const rep = (ch, n) => n > 0 ? ch.repeat(n) : '';

  function line(col = 0) {
    const segs = [];
    let cells = 0;
    const l = {
      get cells() { return cells; },
      get segs() { return segs; },
      add(cls, text, href) { if (!text) return l; segs.push([cls, text, href]); cells += width(text); return l; },
      button(cls, text, copy) { segs.push([cls, text, null, copy]); cells += width(text); return l; },
      to(c) { if (c > cells) l.add('', rep(' ', c - cells)); return l; },
      // A label, a space, leaders to the end column, a space.
      leader(label, end, cls) { l.add(cls, label); l.add('', ' '); l.add(cls, rep('.', end - cells)); l.add('', ' '); return l; },
      rule(n, cls = 'bord') { return l.add(cls, rep('─', n)); },
      title(c, text) { l.to(c); return l.add('par b', text); },
      // A name, then the rule to the right edge, then a tail if any.
      eyebrow(c, name, right, tail) {
        l.to(c).add('par b', name).add('', ' ');
        const tw = tail ? tail.w : 0;
        l.rule(Math.max(right - cells - (tw ? tw + 1 : 0), 0));
        if (tail) { l.add('', ' '); tail.draw(l); }
        return l;
      },
      html(cls = '', style = '') {
        const body = segs.map(([c, t, href, copy]) => copy ? `<button class="${c}" type="button" data-copy="${esc(copy)}">${esc(t)}</button>`
          : href ? `<a class="${c}" href="${href}">${esc(t)}</a>` : c ? `<span class="${c}">${esc(t)}</span>` : esc(t)).join('');
        return `<div class="row${cls ? ' ' + cls : ''}"${style ? ` style="${style}"` : ''}>${body}</div>`;
      },
    };
    return l.to(col);
  }
  const blank = () => line();
  // A stamp: the word in a block of the accent, the ground knocked out.
  const stamp = (word, blinks) => ({ w: width(word) + 2, draw: l => l.add('chip' + (blinks && !reduced ? ' blink' : ''), ' ' + word + ' ') });

  // Lines to a pane: each drawn in on its turn, the first at once.
  function paint(el, lines, h, step) {
    const out = [];
    for (let i = 0; i < Math.min(lines.length, h); i++) {
      const [l, cls] = Array.isArray(lines[i]) ? lines[i] : [lines[i], ''];
      const delay = step && !reduced ? `animation-delay:${i * step}ms` : '';
      out.push(l.html((step && !reduced ? 'in ' : '') + cls, delay));
    }
    el.innerHTML = out.join('');
  }

  // Text wrapped to a width, on spaces.
  function wrap(text, w) {
    const lines = [];
    for (const para of text.split('\n')) {
      let cur = '';
      for (const word of para.split(' ')) {
        if (cur && width(cur) + 1 + width(word) > w) { lines.push(cur); cur = word; }
        else cur = cur ? cur + ' ' + word : word;
      }
      lines.push(cur);
    }
    return lines;
  }

  // ---- the console ----------------------------------------------------

  const pad2 = n => String(n).padStart(2, '0');
  const MONTHS = ['JAN', 'FEB', 'MAR', 'APR', 'MAY', 'JUN', 'JUL', 'AUG', 'SEP', 'OCT', 'NOV', 'DEC'];
  const stampDate = d => `${pad2(d.getUTCDate())}-${MONTHS[d.getUTCMonth()]}-${d.getUTCFullYear()}`;
  const zulu = d => `${stampDate(d)}  ${pad2(d.getUTCHours())}:${pad2(d.getUTCMinutes())}:${pad2(d.getUTCSeconds())} Z`;

  // What the browser can say for itself, and not a word more: nothing
  // is kept, nothing is sent.
  function readVisitor() {
    const ua = navigator.userAgent;
    const plat = /iPhone|iPad|iPod/.test(ua) ? 'IOS' : /Android/.test(ua) ? 'ANDROID' : /Mac/.test(ua) ? 'MACOS'
      : /CrOS/.test(ua) ? 'CHROMEOS' : /Linux|X11/.test(ua) ? 'LINUX' : /Windows/.test(ua) ? 'WINDOWS' : 'UNKNOWN';
    const phone = plat === 'IOS' || plat === 'ANDROID';
    let browser = 'UNKNOWN';
    for (const [re, name] of [[/Edg\/(\d+)/, 'EDGE'], [/OPR\/(\d+)/, 'OPERA'], [/Firefox\/(\d+)/, 'FIREFOX'], [/CriOS\/(\d+)/, 'CHROME'], [/Chrome\/(\d+)/, 'CHROME'], [/Version\/(\d+).*Safari/, 'SAFARI']]) {
      const m = ua.match(re);
      if (m) { browser = `${name} ${m[1]}`; break; }
    }
    const tz = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UNKNOWN';
    const off = -arrived.getTimezoneOffset();
    const utc = `UTC${off >= 0 ? '+' : '-'}${pad2(Math.floor(Math.abs(off) / 60))}:${pad2(Math.abs(off) % 60)}`;
    const local = `${pad2(arrived.getHours())}:${pad2(arrived.getMinutes())} LOCAL`;
    const gamut = matchMedia('(color-gamut: p3)').matches ? 'P3' : 'SRGB';
    const conn = navigator.connection && navigator.connection.effectiveType;
    let from = 'DIRECT';
    try { if (document.referrer) from = new URL(document.referrer).host.toUpperCase(); } catch (e) { /* direct */ }
    const machine = [
      ['SYSTEM', plat],
      ['SCREEN', `${screen.width}×${screen.height} · ${Math.round(devicePixelRatio * 10) / 10}X`],
      ['WINDOW', `${innerWidth}×${innerHeight} · ${cols}×${rows + 2} CELLS`],
      ['COLOR', `${screen.colorDepth}-BIT · ${gamut}`],
      ['CORES', navigator.hardwareConcurrency ? String(navigator.hardwareConcurrency) : 'UNREAD'],
      ['NETWORK', navigator.onLine ? (conn ? 'ONLINE · ' + conn.toUpperCase() : 'ONLINE') : 'OFFLINE'],
      ['POINTER', matchMedia('(pointer: coarse)').matches ? 'TOUCH' : 'MOUSE'],
      ['MOTION', reduced ? 'REDUCED' : 'FULL'],
    ];
    const session = [
      ['VISITOR', 'ANONYMOUS · NOTHING KEPT'],
      ['BROWSER', browser],
      ['LANGUAGE', (navigator.language || 'UNKNOWN').toUpperCase()],
      ['TIME ZONE', `${tz.toUpperCase()} · ${utc} · ${local}`],
      ['ARRIVED', `${from} · ${local}`],
      ['CONN', `${V.version} · ${V.built}`],
      ['NEEDS', 'TMUX · LSOF ON MACOS'],
      ['INSTALL', INSTALL],
      ['MANUAL', `${HOST}/manual`],
    ];
    return { plat, phone, machine, session };
  }

  // The start-up checks: the screen, the font, the clock, the install
  // script on this site, the platform. NOMINAL is said and left alone;
  // anything else takes the chip and blinks with the verdict.
  let installCheck = { label: 'INSTALL', value: '/install.sh', status: 'UNCHECKED', fault: true };
  fetch('/install.sh', { method: 'HEAD', cache: 'no-store' }).then(r => {
    if (!r.ok) throw new Error(String(r.status));
    const n = Number(r.headers.get('content-length'));
    installCheck = { label: 'INSTALL', value: n ? `/install.sh · ${(n / 1024).toFixed(1)} KB` : '/install.sh', status: 'NOMINAL' };
  }).catch(() => { installCheck = { label: 'INSTALL', value: '/install.sh', status: 'MISSING', fault: true }; });

  function checks(v) {
    const fontOK = document.fonts && document.fonts.check('14px "JetBrains Mono"');
    const after = arrived >= V.buildDate;
    return [
      { label: 'SCREEN', value: `${cols}×${rows + 2} · WEB · TRUECOLOR`, status: 'NOMINAL' },
      fontOK ? { label: 'FONTS', value: 'JETBRAINS MONO', status: 'NOMINAL' }
             : { label: 'FONTS', value: 'JETBRAINS MONO · FALLBACK', status: 'MISSING', fault: true },
      after ? { label: 'CLOCK', value: 'AFTER THE BUILD OF ' + V.built, status: 'NOMINAL' }
            : { label: 'CLOCK', value: 'BEFORE THE BUILD OF ' + V.built, status: 'BEHIND', fault: true },
      installCheck,
      v.phone ? { label: 'PLATFORM', value: `${v.plat} · NO TERMINAL HERE`, status: 'NOT HERE', fault: true }
      : v.plat === 'WINDOWS' ? { label: 'PLATFORM', value: 'WINDOWS · WSL IS LINUX', status: 'NOMINAL' }
      : { label: 'PLATFORM', value: `${v.plat} · MACOS AND LINUX`, status: 'NOMINAL' },
    ];
  }

  const WORDMARK = [
    ' ██████╗ ██████╗ ███╗   ██╗███╗   ██╗',
    '██╔════╝██╔═══██╗████╗  ██║████╗  ██║',
    '██║     ██║   ██║██╔██╗ ██║██╔██╗ ██║',
    '██║     ██║   ██║██║╚██╗██║██║╚██╗██║',
    '╚██████╗╚██████╔╝██║ ╚████║██║ ╚████║',
    ' ╚═════╝ ╚═════╝ ╚═╝  ╚═══╝╚═╝  ╚═══╝',
  ];

  // The wordmark, drawn: a rect a cell for a block, and the double
  // lines of the box glyphs as two thin rects each, the way a terminal
  // tiles them. A font's glyphs sit in their em box, short of the line
  // box, and leave seams between rows; the drawing has none.
  function wordmarkSVG(col, rowAt) {
    const w = cellW, h = cellH, a = 0.38, b = 0.46, c = 0.54, d = 0.62;
    const rects = [];
    const R = (x, y, rw, rh) => rects.push(`<rect x="${x.toFixed(2)}" y="${y.toFixed(2)}" width="${rw.toFixed(2)}" height="${rh.toFixed(2)}"/>`);
    WORDMARK.forEach((row, ry) => [...row].forEach((ch, rx) => {
      const x = rx * w, y = ry * h;
      const H = (y0, y1, x0, x1) => R(x + x0 * w, y + y0 * h, (x1 - x0) * w, (y1 - y0) * h);
      switch (ch) {
        case '█': R(x, y, w + 0.3, h + 0.3); break;
        case '═': H(a, b, 0, 1); H(c, d, 0, 1); break;
        case '║': H(0, 1, a, b); H(0, 1, c, d); break;
        case '╗': H(a, b, 0, d); H(a, 1, c, d); H(c, d, 0, b); H(c, 1, a, b); break;
        case '╝': H(c, d, 0, d); H(0, d, c, d); H(a, b, 0, b); H(0, b, a, b); break;
        case '╔': H(a, b, a, 1); H(a, 1, a, b); H(c, d, c, 1); H(c, 1, c, d); break;
        case '╚': H(c, d, a, 1); H(0, d, a, b); H(a, b, c, 1); H(0, b, c, d); break;
        default: break;
      }
    }));
    const W = width(WORDMARK[0]) * w, Hh = WORDMARK.length * h;
    return `<svg class="wm" style="left:${(col * w).toFixed(2)}px;top:${(rowAt * h).toFixed(2)}px" width="${W.toFixed(2)}" height="${Hh.toFixed(2)}" viewBox="0 0 ${W.toFixed(2)} ${Hh.toFixed(2)}" aria-hidden="true"><g fill="currentColor" shape-rendering="crispEdges">${rects.join('')}</g></svg>`;
  }

  // The console's body at a width, to a stage: the header at once, the
  // readout, each check in turn, the verdict last. Rows past the stage
  // are blank, so nothing moves when they come.
  function consoleBody(v, ks, w, stage) {
    const measure = w - 2 * MARGIN, rightCol = Math.floor(measure / 2), leaderEnd = measure - STATUS_W - 2;
    const out = [];
    const emit = (l, st, cls) => out.push([st <= stage ? l : blank(), cls || '']);
    const L = () => line(MARGIN);
    emit(blank(), 0);
    const stationBlock = [['ink b', 'CONN ' + V.version], ['gray', 'STATION  ' + HOST.toUpperCase()], ['gray', zulu(arrived)], ['gray', `${V.commit} · ${V.built}`]];
    const markW = width(WORDMARK[0]);
    WORDMARK.forEach((m, i) => {
      const l = L().add('wmtext', m);
      if (i < stationBlock.length) l.to(MARGIN + markW + 5).add(stationBlock[i][0], stationBlock[i][1]);
      emit(l, 0);
    });
    emit(L().rule(measure), 0);
    const fact = (l, col, w, [label, value]) => {
      l.to(MARGIN + col).leader(label, MARGIN + col + FACT_COL - 1, 'faint').add('ink', fit(value, w - FACT_COL - 2));
    };
    emit(L().title(MARGIN, 'MACHINE').title(MARGIN + rightCol, 'SESSION'), 1);
    for (let i = 0; i < Math.max(v.machine.length, v.session.length); i++) {
      const l = L();
      if (i < v.machine.length) fact(l, 0, rightCol, v.machine[i]);
      if (i < v.session.length) fact(l, rightCol, measure - rightCol, v.session[i]);
      emit(l, 1);
    }
    emit(blank(), 2);
    emit(L().title(MARGIN, 'START-UP CHECKS'), 2);
    let off = 0;
    ks.forEach((k, i) => {
      const l = L().leader(k.label, MARGIN + CHECK_COL - 1, 'gray').add('ink', fit(k.value, leaderEnd - CHECK_COL - 2)).add('', ' ');
      l.add('bord', rep('.', MARGIN + leaderEnd - l.cells));
      if (k.status === 'NOMINAL') {
        l.to(MARGIN + measure - STATUS_W + Math.floor((STATUS_W - width(k.status)) / 2)).add('gray', k.status);
      } else {
        off++;
        const word = ' ' + k.status + ' ';
        l.to(MARGIN + measure - STATUS_W + Math.floor((STATUS_W - width(word)) / 2)).add('chip' + (reduced ? '' : ' blink'), word);
      }
      emit(l, 2 + i);
    });
    const last = 2 + ks.length;
    emit(blank(), last);
    emit(L().rule(measure), last);
    const verdict = L();
    if (off === 0) verdict.add('gray', 'ALL SYSTEMS NOMINAL');
    else verdict.add('chip' + (reduced ? '' : ' blink'), off > 1 ? ` ${off} SYSTEMS NOT NOMINAL ` : ' 1 SYSTEM NOT NOMINAL ');
    emit(verdict, last, 'center');
    return out;
  }

  // The console for a window the body will not fit: the mark, and the
  // screen's check with the size it needs. Nothing is clipped, so the
  // reason is in view.
  function smallConsole(w, h, need) {
    const measure = w - 2 * MARGIN, out = [], L = () => line(MARGIN);
    out.push(blank());
    if (measure >= width(WORDMARK[0]) && h >= WORDMARK.length + 5) WORDMARK.forEach(m => out.push(L().add('wmtext', m)));
    else out.push(L().add('acc b', 'CONN'));
    out.push(blank());
    const value = `${w}×${h + 2} · WEB · TRUECOLOR`;
    const l = L().add('gray', 'SCREEN ').add('ink', fit(value, measure - 7 - 8));
    l.to(MARGIN + measure - 7).add('chip' + (reduced ? '' : ' blink'), ' SMALL ');
    out.push(l);
    out.push(L().add('gray', 'NEEDS ').add('ink', `${MIN_COLS}×${need + 2}`));
    return out;
  }

  // The console as a window of this size takes it: facts given up from
  // the foot of the readout until the body fits, the way conn joins its
  // roots, and the small console where nothing would.
  function drawConsole(stage) {
    const v = readVisitor(), ks = checks(v);
    const need = () => 1 + 6 + 1 + 1 + Math.max(v.machine.length, v.session.length) + 1 + 1 + ks.length + 1 + 1 + 1;
    while (need() > rows && Math.max(v.machine.length, v.session.length) > 3) {
      if (v.machine.length >= v.session.length) v.machine.pop(); else v.session.pop();
    }
    const small = cols < MIN_COLS || need() > rows;
    const lastStage = 2 + ks.length;
    return { lines: small ? smallConsole(cols, rows, need()) : consoleBody(v, ks, cols, stage), lastStage, small };
  }

  // ---- the machine ----------------------------------------------------

  const MARK = { contact: '☿', shell: '❯', editor: '▯', service: '◉', run: '○' };
  const QUESTION = 'Rename bay to workspace in the code too?';

  // The model: two projects and their processes, each with a state;
  // where the cursor is; whether the keys are in a pane and which; the
  // view the panel is in; what the log holds and how much of it has not
  // been looked at; a question armed; and the station's own clock. It
  // is one machine, running, and every key is a change to it.
  function fresh() {
    return {
      view: 'processes', cursor: 'zsh1', inside: false, shown: null, landing: true,
      log: 0, logRows: [], logCursor: 0, confirm: null, left: null,
      claude: 'working', answered: false, moved: false, done: {}, since: Date.now(), events: [],
      projects: [
        { name: 'conn', rows: [
          { id: 'claude', kind: 'contact', digit: '0', name: 'claude', status: 'working' },
          { id: 'zsh1', kind: 'shell', name: 'zsh', status: 'idle' },
          { id: 'test', kind: 'run', name: 'go test ./...', status: 'working' },
        ] },
        { name: 'web', rows: [
          { id: 'zsh2', kind: 'shell', name: 'zsh', status: 'idle' },
          { id: 'vim', kind: 'editor', name: 'vim', status: 'stopped', word: 'STOPPED' },
          { id: 'pg', kind: 'service', name: 'postgres', status: 'serving', port: ':5432' },
          { id: 'vite', kind: 'run', name: 'node vite', status: 'serving', port: ':5173' },
          { id: 'worker', kind: 'run', name: 'worker', status: 'down', word: 'DOWN', declared: true },
        ] },
      ],
    };
  }
  let m = fresh();
  const rowsOf = () => m.projects.flatMap(p => p.rows);
  const row = id => rowsOf().find(r => r.id === id);
  const projectOf = id => m.projects.find(p => p.rows.some(r => r.id === id));
  const clock = () => { const d = new Date(); return `${pad2(d.getHours())}:${pad2(d.getMinutes())}`; };
  const waiting = () => rowsOf().filter(r => r.status === 'waiting').sort((a, b) => a.waitedAt - b.waitedAt);
  const logLine = (r, w, note) => { m.logRows.unshift({ t: clock(), p: projectOf(r.id).name, n: r.name, w, note }); m.log++; };

  // What happens on its own, by the station's clock: the test run
  // ends badly, the contact stops to ask; and after an answer, the
  // contact works and finishes its turn. Each fires once.
  const TIMELINE = [
    [5000, 'testEnds', () => { const r = row('test'); if (r && r.status === 'working') { r.status = 'fault'; r.word = 'EXIT 1'; logLine(r, 'EXIT 1', 'ran 2 min · FAIL: TestBand'); } }],
    [8000, 'asks', () => { const r = row('claude'); if (r && r.status === 'working') { r.status = 'waiting'; r.waitedAt = Date.now(); logLine(r, 'WAITING', QUESTION); } }],
  ];
  function tick() {
    const t = Date.now() - m.since;
    let changed = false;
    for (const [at, id, fn] of TIMELINE) if (t >= at && !m.done[id]) { m.done[id] = true; fn(); changed = true; }
    // A wait's age is said to the minute, so the row changes on the minute.
    for (const r of rowsOf()) if (r.status === 'waiting') { const age = ageOf(r); if (age !== r.age) { r.age = age; changed = true; } }
    if (changed) render();
  }
  // What happens a moment later, on its own timer, and is drawn.
  const later = (ms, fn) => setTimeout(() => { if (m.view === 'console') return; fn(); render(); }, ms);
  const ageOf = r => { const min = Math.floor((Date.now() - r.waitedAt) / 60000); return min >= 1 ? `${min} MIN` : ''; };

  // ---- the panel ------------------------------------------------------

  // A block's verdict: what wants you, at the end of the rule. A wait,
  // else a fault; several faults counted.
  function verdict(p) {
    const w = p.rows.filter(r => r.status === 'waiting').length;
    if (w > 0) return stamp(w > 1 ? `${w} WAITING` : 'WAITING', true);
    const faults = p.rows.filter(r => r.status === 'fault' || r.status === 'stopped');
    if (faults.length === 1) return stamp(faults[0].word, false);
    if (faults.length > 1) return stamp(`${faults.length} FAULTS`, false);
    return null;
  }

  function drawPanel(w) {
    const measure = w - MARGIN, out = [];
    for (const p of m.projects) {
      out.push(blank());
      out.push(line().eyebrow(MARGIN, p.name, measure, verdict(p)));
      for (const r of p.rows) {
        const l = line();
        const cursored = r.id === m.cursor && !m.inside;
        const shown = m.inside && r.id === m.shown;
        if (cursored || shown) l.add('acc', '▌');
        if (r.status === 'working') l.to(1).add('run spin', '⣾');
        let tone = 'faint';
        if (r.status === 'waiting') tone = 'acc b';
        else if (r.status === 'stopped' || r.status === 'fault') tone = 'acc';
        else if (r.status === 'working' || (r.port && r.kind !== 'contact' && r.status === 'serving')) tone = 'run';
        l.to(3).add(tone, MARK[r.kind]);
        l.add('', r.kind === 'contact' ? ` ${r.digit} ` : '   ');
        const over = r.status === 'down' || r.status === 'ended';
        l.add(over ? 'struck' : (r.status === 'waiting' ? 'ink b' : 'ink'), fit(r.name, measure - l.cells - 10));
        if (r.status === 'waiting') { const s = stamp(r.age || 'WAITING', true); l.to(measure - s.w); s.draw(l); }
        else if (r.status === 'stopped' || r.status === 'fault') { const s = stamp(r.word, false); l.to(measure - s.w); s.draw(l); }
        else if (over) l.to(measure - width(r.word || 'DOWN')).add('gray', r.word || 'DOWN');
        else if (r.port && r.status === 'serving') l.to(measure - width(r.port)).add('gray', r.port);
        out.push([l, cursored ? 'chosen' : '']);
      }
    }
    return out;
  }

  // The log, as the panel shows it over time: newest first, a note a line.
  const OLDER = [
    { t: '11:41', p: 'web', n: 'go test ./...', w: 'EXIT 1', note: 'ran 2 min' },
    { t: '09:04', p: 'web', n: 'node vite', w: 'ACTIVE', note: ':5173' },
  ];
  const logEntries = () => [...m.logRows, ...OLDER];
  function drawLog(w) {
    const measure = w - MARGIN, out = [], L = () => line(MARGIN);
    out.push(blank());
    out.push(L().add('par b', 'LOG').to(MARGIN + measure - 5).add('gray', `${m.logRows.length} NEW`));
    out.push(L().rule(measure));
    out.push(blank());
    out.push(line().eyebrow(MARGIN, 'TODAY', measure, null));
    logEntries().forEach((e, i) => {
      const l = line();
      if (i === m.logCursor) l.add('acc', '▌');
      l.to(MARGIN).add('ink', e.t).add('', ' ').add('par', e.p.padEnd(5)).add('', ' ').add('ink', fit(e.n, measure - l.cells - width(e.w) - 2));
      if (e.w === 'WAITING') { const s = stamp(e.w, false); l.to(measure - s.w); s.draw(l); }
      else l.to(measure - width(e.w)).add('gray', e.w);
      out.push([l, i === m.logCursor ? 'chosen' : '']);
      out.push(line(MARGIN + 12).add('gray', fit(e.note, measure - MARGIN - 12)));
    });
    return out;
  }

  // ---- the workspace --------------------------------------------------

  // The station's own page: what the workspace holds before the cursor
  // moves. The one place the site speaks, in the page's own form.
  function drawLanding(w) {
    const leftW = Math.min(w - 2 * MARGIN, 76), out = [], L = () => line(MARGIN);
    const para = (text, cls) => wrap(text, leftW).forEach(t => out.push(L().add(cls, t)));
    const eyebrow = name => { out.push(blank()); out.push(line().eyebrow(MARGIN, name, MARGIN + leftW, null)); out.push(blank()); };
    out.push(blank());
    out.push(line().eyebrow(MARGIN, 'CONN', MARGIN + leftW, null));
    out.push(blank());
    para('A console for the processes working your projects.', 'ink b');
    out.push(blank());
    para('Every process on one machine, under the project it works. What each project wants of you. The one you choose, in front of you.', 'par');
    eyebrow('INSTALL');
    out.push(L().add('acc b', '$').add('', ' ').add('ink', INSTALL).add('', '   ').button('copy', '[copy]', INSTALL));
    out.push(L().add('gray', 'macOS and Linux · needs tmux, and lsof on macOS'));
    eyebrow('PROCEDURE');
    [['j k', 'Down the panel; the page follows the cursor'], ['tab', 'The contact that has waited longest'], ['enter', 'Go in; ctrl-space brings the keys back'], ['l  ?  c', 'The log, the keys, the console']]
      .forEach(([k, does], i) => out.push(L().add('gray', String(i + 1)).to(MARGIN + 5).add('ink b', k).to(MARGIN + 15).add('ink', fit(does, leftW - 15))));
    eyebrow('DOCUMENTS');
    [['Operating manual', `${HOST}/manual`, '/manual/'], ['Man conn', 'the reference, built in: ? inside conn', 'https://github.com/w0zro/conn/blob/main/man/conn.1'], ['Source', 'github.com/w0zro/conn', 'https://github.com/w0zro/conn']]
      .forEach(([k, v, href]) => out.push(L().leader(k, MARGIN + 19, 'faint').add('ink link', v, href)));
    eyebrow('NOTE');
    para('This station is a simulation: two projects on a machine that runs in this page, worked with conn’s own keys. The clock is yours. Nothing is kept or sent.', 'par');
    return out;
  }

  // The page: what conn knows about the row under the cursor, read
  // without entering it; the specifications to its right where the
  // pane is wide enough.
  function drawPage(w) {
    const r = row(m.cursor), p = projectOf(m.cursor);
    const wide = w >= 104, leftW = wide ? w - 2 * MARGIN - 34 : w - 2 * MARGIN, specCol = MARGIN + leftW + 6;
    const out = [], L = () => line(MARGIN);
    const para = (text, cls = 'ink') => wrap(text, leftW).forEach(t => out.push(L().add(cls, t)));
    const section = name => { out.push(blank()); out.push(line().eyebrow(MARGIN, name, MARGIN + leftW, null)); out.push(blank()); };
    const procedure = steps => { section('PROCEDURE'); steps.forEach(([k, does], i) => out.push(L().add('gray', String(i + 1)).to(MARGIN + 5).add('ink b', k).to(MARGIN + 14).add('ink', fit(does, leftW - 14)))); };
    const box = lines => {
      out.push(L().add('bord', rep('▄', leftW)));
      for (const [cls, t, right] of lines) {
        const l = L().add('bord', '▌').add('', '  ');
        if (t) l.add(cls, fit(t, leftW - 3 - (right ? width(right) + 2 : 0)));
        if (right) l.to(MARGIN + leftW - width(right)).add('gray', right);
        out.push(l);
      }
      out.push(L().add('bord', rep('▀', leftW)));
    };
    const spec = pairs => {
      if (!wide) return out;
      const specW = w - MARGIN - specCol, right = [line().eyebrow(0, 'SPECIFICATIONS', specW, null), blank()];
      pairs.forEach(([k, v]) => right.push(k ? line().leader(k, 13, 'faint').add('ink', fit(v, specW - 15)) : blank()));
      const merged = [];
      for (let i = 0; i < Math.max(out.length, right.length + 1); i++) {
        const left = i < out.length ? out[i] : blank();
        const [l, cls] = Array.isArray(left) ? left : [left, ''];
        const rt = i >= 1 && i - 1 < right.length ? right[i - 1] : null;
        merged.push([rt ? mergeLines(l, rt, specCol) : l, cls]);
      }
      return merged;
    };
    const folder = `~/projects/w0zro/${p.name}`;
    out.push(blank());
    if (r.kind === 'contact') {
      out.push(line().eyebrow(MARGIN, 'CONTACT', MARGIN + leftW, null));
      out.push(blank());
      out.push(L().add('ink b', ` ${r.digit}`).to(MARGIN + 5).add('ink', `conn-${r.digit}`).to(MARGIN + 14).add('gray', `${folder} · main`));
      out.push(L().to(MARGIN + 5).add('gray', 'Claude Code 2.1.267 · 212k of context carried'));
      out.push(blank());
      if (r.status === 'waiting') {
        box([['ink b', 'WAITING FOR YOU', r.age ? r.age.toLowerCase() : 'just now'], ['', ''], ['ink', `“${QUESTION}”`], ['', ''], ['gray', `Asked with AskUserQuestion at ${m.logRows.find(e => e.w === 'WAITING')?.t || clock()}`]]);
        procedure([['enter', 'Go in and answer it'], ['tab', 'Leave it, take the next thing waiting'], ['x', 'End it — conn asks first']]);
        section('CAUTION');
        para('It has the terminal and is sleeping. x signals the contact, not what it runs — the process under it keeps going.');
      } else if (r.status === 'working') {
        box([['run b', 'WORKING', ''], ['', ''], ['ink', m.answered ? 'Renaming bay to workspace in the code' : 'Reading band.go, keys.go, bay.go'], ['', ''], ['gray', m.answered ? 'You last answered “1. Yes”' : 'You last said “bring the code to the manual”']]);
        procedure([['enter', 'Go in'], ['tab', 'The next thing waiting'], ['x', 'End it — conn asks first']]);
      } else {
        box([['ink b', 'TURN OVER', ''], ['', ''], ['ink', 'Renamed bay to workspace in four files; the screens held again.'], ['', ''], ['gray', 'Idle at its prompt. It will carry what it has until you say more.']]);
        procedure([['enter', 'Go in and say more'], ['x', 'End it — conn asks first']]);
      }
      section('HOW THE WORK HAS GONE');
      const barW = Math.min(40, leftW - 28), worked = Math.round(barW * 85 / 94);
      out.push(L().add('run', rep('█', worked)).add('acc', rep('█', barW - worked)).add('gray', `  worked 1h 25m · ${r.status === 'waiting' ? 'waiting' : 'asked once'}`));
      out.push(blank());
      para(r.status === 'waiting' ? 'It came up at 12:36 and worked for 1h 25m, then stopped to ask. You last said “bring the code to the manual”.'
        : r.status === 'working' ? 'It came up at 12:36, worked for 1h 25m, asked once and was answered. It is at work again.'
        : 'It came up at 12:36, worked for 1h 25m, asked once, was answered, and finished its turn.');
      return spec([['Designation', `CONN-${r.digit}`], ['Given', 'Today, 12:36'], ['Worked', '1h 25m'], ['Waiting', r.status === 'waiting' ? (r.age || 'now').toLowerCase() : '—'], ['', ''],
        ['Branch', 'main · 3 changed'], ['Commit', V.commit], ['Tracking', 'origin/main'], ['', ''], ['Folder', folder], ['Pane', '%2'], ['Process', '49212'], ['Command', 'claude --resume'], ['Model', 'claude-fable-5-1']]);
    }
    if (r.status === 'down') {
      out.push(line().eyebrow(MARGIN, 'DECLARED', MARGIN + leftW, null));
      out.push(blank());
      out.push(L().add('ink b', r.name).to(MARGIN + 10).add('gray', 'services/queue').to(MARGIN + 27).add('ink', 'make run'));
      out.push(L().add('gray', '.conn line 3 · not running'));
      procedure([['enter', 'Bring it up and go in'], ['u', 'Bring it up'], ['U', 'Bring up everything declared and down']]);
      section('THE FILE');
      out.push(L().add('gray', '# what works this project'));
      out.push(L().add('ink', 'web frontend:          npm run dev'));
      out.push(L().add('ink', 'api:                   go run ./cmd/api'));
      out.push(L().add('ink b', 'worker services/queue: make run'));
      out.push(L().add('ink', 'db:                    docker compose up postgres'));
      out.push(L().add('ink', 'cache:                 brew services start redis'));
      out.push(blank());
      para('A line is name [dir]: command. The name is the row. A docker compose up has its services as rows under it; a brew services start is the Homebrew service, as brew reports it.');
      return out;
    }
    if (r.kind === 'shell') {
      out.push(line().eyebrow(MARGIN, 'SHELL', MARGIN + leftW, null));
      out.push(blank());
      out.push(L().add('ink b', r.name).to(MARGIN + 10).add('gray', `${folder} · main`));
      out.push(L().add('gray', 'at its prompt · up 2h 14m · ttys004'));
      procedure([['enter', 'Go in'], ['s', 'A shell beside it'], ['x', 'End it — conn asks first']]);
      section('THE PROJECT');
      [['Branch', 'main · clean'], ['Commit', `${V.commit} · The site is conn, playable`], ['Tracking', 'origin/main · 3 ahead'], ['Folder', folder]]
        .forEach(([k, v]) => out.push(L().leader(k, MARGIN + 13, 'faint').add('ink', fit(v, leftW - 15))));
      return out;
    }
    if (r.kind === 'editor') {
      out.push(line().eyebrow(MARGIN, 'EDITOR', MARGIN + leftW, null));
      out.push(blank());
      out.push(L().add('ink b', 'vim notes.md').to(MARGIN + 16).add('gray', folder));
      out.push(L().add('gray', 'stopped with ctrl-z · its shell has the terminal'));
      procedure([['enter', 'Go in: the shell brings it to the foreground'], ['x', 'End it — conn asks first']]);
      section('CAUTION');
      para('A stopped editor holds a buffer. Ending it is ending vim, and what is unsaved in notes.md goes with it.');
      return out;
    }
    if (r.kind === 'service') {
      out.push(line().eyebrow(MARGIN, 'SERVICE', MARGIN + leftW, null));
      out.push(blank());
      out.push(L().add('ink b', 'postgres').to(MARGIN + 16).add('gray', 'brew services · postgresql@17'));
      out.push(L().add('gray', 'ACTIVE · :5432 · 2 connections · up since 09:04'));
      procedure([['enter', 'Follow its log'], ['o', 'Open its port'], ['x', 'Ask brew to stop it']]);
      return out;
    }
    if (r.status === 'fault') {
      out.push(line().eyebrow(MARGIN, 'RUN', MARGIN + leftW, null));
      out.push(blank());
      out.push(L().add('ink b', r.name).to(MARGIN + 16).add('gray', folder));
      out.push(L().add('gray', 'ended with exit 1 · ran 2 min · its pane holds the output'));
      procedure([['enter', 'Go in and read it'], ['u', 'Run it again'], ['x', 'Close the pane']]);
      section('CAUTION');
      para('The fault word holds still; a wait blinks. A fault is a thing to look at, a wait a thing to answer.');
      return out;
    }
    out.push(line().eyebrow(MARGIN, 'RUN', MARGIN + leftW, null));
    out.push(blank());
    out.push(L().add('ink b', r.name).to(MARGIN + 16).add('gray', folder));
    out.push(L().add('gray', r.port ? `serving ${r.port} · up 3h 02m` : 'running · up 1 min'));
    procedure(r.port ? [['enter', 'Go in'], ['o', 'Open its port'], ['x', 'End it — conn asks first']] : [['enter', 'Go in'], ['x', 'End it — conn asks first']]);
    return out;
  }

  // Two lines on one row: the left cut at the column the right starts.
  function mergeLines(left, right, col) {
    const l = line();
    for (const seg of left.segs) {
      const room = col - 1 - l.cells;
      if (room <= 0) break;
      const t = seg[1];
      l.add(seg[0], width(t) > room ? [...t].slice(0, room).join('') : t, seg[2]);
    }
    l.to(col);
    for (const seg of right.segs) l.add(seg[0], seg[1], seg[2]);
    return l;
  }

  // The panes: what the workspace holds with the keys inside. The
  // contact's is Claude Code; the rest are what each process writes.
  function drawPane(w) {
    const r = row(m.shown);
    if (!r) return [];
    const out = [], L = () => line(2), tw = w - 4;
    const para = (lead, cls, text) => wrap(text, tw - 2).forEach((t, i) => out.push(i === 0 ? L().add(cls, lead).add('', ' ').add('ink', t) : L().to(4).add('ink', t)));
    const prompt = () => out.push(L().add('acc b', '$').add('', ' ').add('chip', ' '));
    out.push(blank());
    switch (r.kind) {
      case 'contact':
        if (r.fresh) {
          out.push(L().add('acc b', '✻').add('ink', ' Welcome to Claude Code'));
          out.push(blank());
          out.push(L().add('gray', `  ${projectOf(r.id) ? '~/projects/w0zro/' + projectOf(r.id).name : ''}`));
          out.push(blank());
          out.push(L().add('par', '❯ ').add('chip', ' '));
          out.push(L().add('faint', '  ? for shortcuts'));
          break;
        }
        para('>', 'par', 'the manual says workspace where the code says bay. bring the code to the manual');
        out.push(blank());
        para('●', 'run', 'Read bay.go, band.go, keys.go, dress.go');
        out.push(blank());
        para('●', 'acc', 'The type is bay in four files, and the held screens under testdata name it. Renaming it is one commit, with the screens held again.');
        out.push(blank());
        out.push(L().to(4).add('ink', QUESTION));
        if (r.status === 'waiting') {
          out.push(L().to(4).add(m.option === 2 ? 'gray' : 'acc', m.option === 2 ? '  ' : '❯ ').add('ink', '1. Yes'));
          out.push(L().to(4).add(m.option === 2 ? 'acc' : 'gray', m.option === 2 ? '❯ ' : '  ').add('ink', '2. No, leave the code as it is'));
          out.push(blank());
          out.push(L().add('faint', '  enter to answer · j k to choose · ctrl-space the panel'));
        } else if (m.answered === 'no') {
          out.push(L().to(4).add('gray', '  2. No, leave the code as it is'));
          out.push(blank());
          para('>', 'par', '2');
          out.push(blank());
          para('●', 'run', 'Leaving it. The code says bay and the manual says workspace; I have noted the seam in the README’s tree notes.');
          out.push(blank());
          out.push(L().add('par', '❯ ').add('chip', ' '));
        } else {
          out.push(L().to(4).add('gray', '  1. Yes'));
          out.push(blank());
          para('>', 'par', '1');
          out.push(blank());
          if (r.status === 'working') {
            para('●', 'run', 'Renaming bay to workspace: bay.go → workspace.go, the type and its methods, and the screens under testdata held again.');
            out.push(blank());
            out.push(L().add('run spin', '⣾').add('gray', ' Renaming… (esc to interrupt)'));
          } else {
            para('●', 'run', 'Renamed bay to workspace in bay.go → workspace.go, band.go, keys.go and dress.go; the screens under testdata held again. go test ./... passes.');
            out.push(blank());
            out.push(L().add('par', '❯ ').add('chip', ' '));
          }
        }
        break;
      case 'shell':
        out.push(L().add('acc b', '$').add('', ' ').add('ink', 'git status -sb'));
        out.push(L().add('gray', '## main...origin/main [ahead 3]'));
        out.push(L().add('acc b', '$').add('', ' ').add('ink', 'go build ./...'));
        prompt();
        break;
      case 'editor':
        out.push(L().add('ink', 'notes.md'));
        out.push(blank());
        ['# conn', '', '- the panel is by project; the rule says what it wants', '- bay → workspace in the code, as the manual says', '- the log counts what you have not seen'].forEach(t => out.push(L().add('ink', t)));
        for (let i = 0; i < 6; i++) out.push(L().add('faint', '~'));
        out.push(L().add('gray', '"notes.md" 5L, 162B').to(w - 12).add('gray', '5,1  All'));
        break;
      case 'service':
        ['LOG:  database system is ready to accept connections', 'LOG:  checkpoint starting: time', 'LOG:  checkpoint complete: wrote 12 buffers (0.1%)', 'LOG:  connection received: host=[local]', 'LOG:  connection authorized: user=web database=web_dev'].forEach(t => out.push(L().add('gray', clock() + ' ').add('ink', t)));
        break;
      default:
        if (r.id === 'test') {
          out.push(L().add('acc b', '$').add('', ' ').add('ink', 'go test ./...'));
          out.push(L().add('ink', 'ok  \tgithub.com/w0zro/conn/internal/draw\t0.214s'));
          out.push(L().add('ink', '--- FAIL: TestBand (0.00s)'));
          out.push(L().add('ink', '    band_test.go:41: the band reads "CONN · LOG 2", want "CONN · LOG 1"'));
          out.push(L().add('acc', 'FAIL'));
          out.push(L().add('acc', 'FAIL\tgithub.com/w0zro/conn\t2.118s'));
          out.push(L().add('gray', '[exit 1]'));
        } else if (r.id === 'vite') {
          out.push(L().add('run b', '  VITE v6.0.3').add('gray', '  ready in 212 ms'));
          out.push(blank());
          out.push(L().add('ink', '  ➜  Local:   ').add('run', 'http://localhost:5173/'));
          out.push(L().add('ink', '  ➜  Network: use --host to expose'));
          out.push(L().add('gray', '  ➜  press h + enter to show help'));
        } else {
          out.push(L().add('acc b', '$').add('', ' ').add('ink', 'make run'));
          out.push(L().add('gray', 'go run ./cmd/worker'));
          out.push(L().add('ink', `${clock()} worker: listening on queue "jobs"`));
          out.push(L().add('ink', `${clock()} worker: 0 jobs in flight`));
        }
    }
    return out;
  }

  // The keys card, as ? shows it: a group a column.
  const KEYS = [
    ['MOVING', [['j k', 'a row'], ['gg G', 'first row, last row'], ['tab', 'next waiting'], ['0-9', 'contact 0 to 9'], ['z', 'the whole tree']]],
    ['THE ROW', [['enter', 'go in'], ['esc', 'back where you were'], ['x', 'end it'], ['s', 'a shell there'], ['S', 'its own client'], ['o', 'open its port'], ['a', 'a new contact'], ['A', 'its sessions'], ['/', 'find in its project’s output'], ['u U', 'bring up, bring up all']]],
    ['THE STATION', [['p', 'the projects'], ['r', 'recent sessions'], ['l', 'the log'], ['c', 'the console'], [',', 'the settings'], ['?', 'these keys'], ['q', 'detach']]],
    ['IN A PROCESS', [['ctrl-space', 'the panel'], ['ctrl-space ctrl-space', 'the last process'], ['ctrl-space tab', 'next waiting'], ['ctrl-space 0-9', 'contact 0 to 9'], ['ctrl-space r', 'recent sessions'], ['ctrl-space l', 'the log'], ['ctrl-space /', 'find in the output'], ['ctrl-space ?', 'these keys']]],
  ];
  function drawKeys(w) {
    const out = [], L = () => line(MARGIN), measure = w - 2 * MARGIN;
    out.push(blank());
    out.push(line().eyebrow(MARGIN, 'THE KEYS', MARGIN + measure, null));
    out.push(blank());
    const cols2 = measure >= 100 ? 2 : 1, colW = Math.floor(measure / cols2);
    const groups = KEYS.map(([name, hints]) => { const g = [line().add('par b', name)]; hints.forEach(([k, d]) => g.push(line().add('ink b', k).to(24).add('gray', d))); g.push(blank()); return g; });
    const stacks = [];
    for (let i = 0; i < cols2; i++) stacks.push(groups.filter((_, gi) => gi % cols2 === i).flat());
    const h = Math.max(...stacks.map(s => s.length));
    for (let i = 0; i < h; i++) {
      let l = line();
      stacks.forEach((st, ci) => { const seg = st[i] || blank(); l = mergeLines(l, seg, MARGIN + ci * colW); });
      out.push(l);
    }
    out.push(L().add('gray', 'Every key, every word the panel and the console can show: man conn, or ? inside conn.'));
    out.push(L().add('gray', 'The manual, how conn is worked: ').add('ink link', `${HOST}/manual`, '/manual/'));
    return out;
  }

  // The outer shell, with conn detached: everything keeps running.
  function drawDetached(w) {
    const out = [], L = () => line(1);
    out.push(blank());
    out.push(L().add('acc b', '$').add('', ' ').add('ink', 'conn'));
    out.push(L().add('gray', '[detached (from session conn)]'));
    const l = L().add('acc b', '$').add('', ' ').add('ink', m.typedBack || '');
    l.add('chip', ' ');
    out.push(l);
    void w;
    return out;
  }

  // ---- the band -------------------------------------------------------

  function uptime() {
    const mins = Math.round((Date.now() - arrived) / 60000), h = Math.floor(mins / 60), mi = mins % 60;
    return `${pad2(h)}h ${pad2(mi)}m`;
  }

  // What to try next: the one move the station suggests, lit on the
  // bar and said on the band. Each is a step a visitor can take once.
  function objective() {
    if (m.view === 'console') return null;
    if (m.view === 'keys') return { key: 'esc' };
    if (m.view === 'log') return { key: 'esc' };
    if (m.confirm) return null;
    if (m.inside) {
      const r = row(m.shown);
      if (r && r.kind === 'contact' && r.status === 'waiting') return { key: 'enter', inside: true };
      return { key: 'ctrl-space' };
    }
    const w = waiting();
    if (w.length && m.cursor !== w[0].id) return { key: 'tab' };
    if (w.length && m.cursor === w[0].id) return { key: 'enter' };
    if (!m.moved) return { key: 'j' };
    if (m.log > 0) return { key: 'l' };
    const worker = row('worker');
    if (worker && worker.status === 'down') return m.cursor === 'worker' ? { key: 'u' } : { key: 'G' };
    if (!m.done.keys) return { key: '?' };
    if (!m.done.detached) return { key: 'q' };
    return null;
  }

  function drawBand() {
    const now = new Date(), obj = objective();
    const right = `${pad2(now.getHours())}:${pad2(now.getMinutes())} · T+ ${uptime()} `;
    const l0 = line();
    if (m.view !== 'console') {
      if (m.confirm) l0.add('chip', ' CONFIRM ');
      else if (m.view === 'keys') l0.add('chip', ' MANUAL ');
      else if (m.view === 'log') l0.add('chip', ' LOG ');
      else {
        l0.add('ink b', ' CONN ');
        if (m.log > 0) l0.add('gray', `· LOG ${m.log} `);
        if (obj && !obj.inside) l0.add('par', `· TRY ${obj.key} `);
      }
    }
    l0.to(cols - width(right)).add('gray', right);
    b0.innerHTML = l0.html();
    const ident = `${HOST.toUpperCase()} · conn ${V.version} `;
    let hints, say = '';
    if (m.view === 'console') hints = [[touch ? 'tap' : 'any key', 'Continue']];
    else if (m.confirm) { say = m.confirm.prompt; hints = [['y', 'Yes'], ['any other key', 'No']]; }
    else if (m.view === 'keys') hints = [['j k', 'Move'], ['esc', 'Back']];
    else if (m.view === 'log') hints = [['j k', 'Move'], ['enter', 'Go to it'], ['esc', 'Back']];
    else if (m.inside) {
      hints = [['ctrl-space', 'Panel']];
      if (waiting().length) hints.push(['ctrl-space tab', 'Next waiting']);
      hints.push(['ctrl-space ctrl-space', 'Last process'], ['ctrl-space ?', 'Help']);
    } else if (m.landing && one) hints = [['panel', 'The panel'], ['?', 'Help']];
    else {
      const r = row(m.cursor);
      hints = [['j k', 'Move']];
      if (r.status === 'down') hints.push(['enter', 'Bring it up'], ['u', 'Bring it up'], ['U', 'All of them']);
      else hints.push(['enter', r.status === 'fault' ? 'Read it' : 'Go in']);
      if (waiting().length) hints.push(['tab', 'Next waiting']);
      if (r.status !== 'down') hints.push(['x', r.status === 'fault' ? 'Close it' : 'End it']);
      if (r.kind === 'shell') hints.push(['s', 'A shell']);
      hints.push(['l', 'Log'], ['?', 'Help'], ['q', 'Detach']);
    }
    const barOf = hs => {
      const l = line();
      if (say) l.add('', ' ').add('par', say).add('', ' ');
      l.add('', ' ');
      hs.forEach(([k, does], i) => {
        if (i) l.add('', '   ');
        const lit = obj && !obj.inside && k === obj.key;
        l.add((lit ? 'acc b' : 'ink b') + ' key', k + ' ');
        l.add(lit ? 'par' : 'gray', does.toLowerCase());
      });
      return l;
    };
    while (hints.length > 1 && barOf(hints).cells > cols) hints.pop();
    const l1 = barOf(hints);
    if (!one && l1.cells + width(ident) <= cols) l1.to(cols - width(ident)).add('gray', ident);
    b1.innerHTML = l1.html();
    // A key on the bar can be pressed with the pointer.
    b1.querySelectorAll('.key').forEach((el, i) => { el.onclick = () => press(hints[i][0]); });
  }

  // ---- drawing the station --------------------------------------------

  let firstPanel = true;
  function render() {
    if (m.view === 'console') return;
    bandEl.style.display = m.view === 'detached' ? 'none' : '';
    station.classList.remove('console');
    station.classList.toggle('detached', m.view === 'detached');
    if (m.view === 'detached') {
      station.classList.remove('on-panel', 'on-work');
      paint(workEl, drawDetached(cols), rows, 0);
      return;
    }
    const showWork = m.inside || m.view === 'keys' || (m.landing && !m.moved);
    station.classList.toggle('on-panel', one && !showWork);
    station.classList.toggle('on-work', one && showWork);
    paint(panelEl, m.view === 'log' ? drawLog(panelW) : drawPanel(panelW), rows, firstPanel ? 28 : 0);
    firstPanel = false;
    let work;
    if (m.view === 'keys') work = drawKeys(workW);
    else if (m.inside) work = drawPane(workW);
    else if (m.landing) work = drawLanding(workW);
    else work = drawPage(workW);
    paint(workEl, work, rows, 0);
    wireCopy();
    drawBand();
  }

  // ---- the keys -------------------------------------------------------

  const flat = () => rowsOf().map(r => r.id);
  function moveCursor(d) {
    const ids = flat(), i = ids.indexOf(m.cursor);
    m.cursor = ids[Math.max(0, Math.min(ids.length - 1, i + d))];
    m.moved = true; m.landing = false;
  }
  function goIn(id) {
    const r = row(id);
    if (!r) return;
    m.cursor = id; m.landing = false; m.moved = true;
    if (r.status === 'down') bringUp(r);
    m.inside = true; m.shown = id; m.last = id;
  }
  function bringUp(r) {
    r.status = 'working'; r.word = '';
    if (r.id === 'worker') later(1200, () => { r.status = 'serving'; r.port = ':8080'; });
  }
  function back() { m.inside = false; m.shown = null; }
  function endRow(r) {
    const p = projectOf(r.id);
    if (r.declared) { r.status = 'down'; r.word = 'DOWN'; r.port = ''; logLine(r, 'DOWN', 'ended by hand'); return; }
    if (r.kind === 'service') { r.status = 'down'; r.word = 'DOWN'; r.port = ''; logLine(r, 'DOWN', 'brew services stop'); return; }
    if (r.status === 'fault') { p.rows.splice(p.rows.indexOf(r), 1); }
    else { logLine(r, 'GONE', 'ended by hand'); p.rows.splice(p.rows.indexOf(r), 1); }
    if (m.cursor === r.id) m.cursor = flat()[0];
  }
  function answer(n) {
    const r = row('claude');
    if (!r || r.status !== 'waiting') return;
    m.answered = n === 2 ? 'no' : true;
    r.status = n === 2 ? 'idle' : 'working'; r.age = ''; r.waitedAt = 0;
    logLine(r, 'IDLE', 'answered · waited ' + (ageOf({ waitedAt: r.waitedAt || Date.now() }) || 'under a minute'));
    if (n !== 2) later(7000, () => { if (r.status === 'working') { r.status = 'idle'; logLine(r, 'IDLE', 'turn over · took 7s'); } });
  }
  function detach() {
    m.view = 'detached'; m.inside = false; m.shown = null; m.typedBack = ''; m.done.detached = true;
    later(2200, function type() { m.typedBack = 'conn'.slice(0, (m.typedBack || '').length + 1); if (m.typedBack.length < 4) later(140, type); else later(500, () => { m.view = 'processes'; station.classList.remove('detached'); }); });
  }
  function newContact() {
    const p = projectOf(m.cursor), digit = String(rowsOf().filter(r => r.kind === 'contact').length);
    const r = { id: 'c' + digit, kind: 'contact', digit, name: 'claude', status: 'working', fresh: true };
    p.rows.unshift(r);
    goIn(r.id);
  }
  function newShell() {
    const p = projectOf(m.cursor), r = { id: 'sh' + Date.now(), kind: 'shell', name: 'zsh', status: 'idle' };
    const at = p.rows.findIndex(x => x.kind !== 'contact' && x.kind !== 'shell');
    p.rows.splice(at < 0 ? p.rows.length : at, 0, r);
    goIn(r.id);
  }

  // A key, by name, as the bar writes it; the keyboard and the pointer
  // both come through here.
  function press(key) {
    if (m.view === 'console') { if (!booted) finishBoot(); else leaveConsole(); return; }
    if (m.view === 'detached') return;
    if (m.confirm) {
      if (key === 'y') endRow(row(m.confirm.id));
      m.confirm = null; render(); return;
    }
    if (key === 'panel') { m.landing = false; m.moved = true; render(); return; }
    if (m.view === 'keys') { if (key === 'esc' || key === '?') m.view = m.inside ? 'processes' : 'processes'; render(); return; }
    if (m.view === 'log') {
      const n = logEntries().length;
      if (key === 'j') m.logCursor = Math.min(n - 1, m.logCursor + 1);
      else if (key === 'k') m.logCursor = Math.max(0, m.logCursor - 1);
      else if (key === 'esc' || key === 'l') m.view = 'processes';
      else if (key === 'enter') { const e = logEntries()[m.logCursor]; const r = rowsOf().find(x => x.name === e.n); m.view = 'processes'; if (r) goIn(r.id); }
      render(); return;
    }
    if (m.inside) {
      const r = row(m.shown);
      // The panel key brings the keys back at once; what is pressed
      // after it is the panel's: tab the next waiting, ctrl-space again
      // the last process, l the log, ? the keys.
      if (key === 'ctrl-space' || key === 'esc') { m.left = m.shown; back(); render(); return; }
      if (r && r.kind === 'contact' && r.status === 'waiting') {
        if (key === 'j') m.option = 2; else if (key === 'k') m.option = 1;
        else if (key === '1') answer(1); else if (key === '2') answer(2);
        else if (key === 'enter') answer(m.option === 2 ? 2 : 1);
      }
      render(); return;
    }
    const r = row(m.cursor);
    switch (key) {
      case 'j': moveCursor(1); break;
      case 'k': moveCursor(-1); break;
      case 'g': m.cursor = flat()[0]; m.moved = true; m.landing = false; break;
      case 'G': m.cursor = flat()[flat().length - 1]; m.moved = true; m.landing = false; break;
      case 'tab': { const w = waiting(); if (w.length) { m.cursor = w[0].id; m.moved = true; m.landing = false; } break; }
      case 'enter': goIn(m.cursor); break;
      case 'esc': case 'ctrl-space': if (m.left && row(m.left)) goIn(m.left); break;
      case 'x': if (r.status !== 'down') m.confirm = { id: r.id, prompt: `${r.status === 'fault' ? 'Close' : 'End'} ${r.name}?` }; break;
      case 'u': if (r.status === 'down') bringUp(r); else if (r.status === 'fault') { r.status = 'working'; r.word = ''; later(2500, () => { r.status = 'fault'; r.word = 'EXIT 1'; logLine(r, 'EXIT 1', 'ran again · FAIL: TestBand'); }); } break;
      case 'U': rowsOf().filter(x => x.status === 'down').forEach(bringUp); break;
      case 's': newShell(); break;
      case 'a': newContact(); break;
      case 'l': m.view = 'log'; m.logCursor = 0; m.log = 0; break;
      case '?': m.view = 'keys'; m.done.keys = true; break;
      case 'c': showConsole(99); m.view = 'console'; return;
      case 'q': detach(); break;
      case 'o': if (r.port) window.open(`http://localhost${r.port}/`, '_blank'); break;
      default: if (/^[0-9]$/.test(key)) { const c = rowsOf().find(x => x.kind === 'contact' && x.digit === key); if (c) { m.cursor = c.id; m.moved = true; m.landing = false; } }
    }
    render();
  }

  // The keyboard, as conn reads it.
  addEventListener('keydown', e => {
    if (e.metaKey || e.altKey) return;
    let key = null;
    if (e.ctrlKey && (e.key === ' ' || e.code === 'Space')) key = 'ctrl-space';
    else if (e.ctrlKey) return;
    else if (e.key === 'Enter') key = 'enter';
    else if (e.key === 'Escape') key = 'esc';
    else if (e.key === 'Tab') key = 'tab';
    else if (e.key === 'ArrowDown') key = 'j';
    else if (e.key === 'ArrowUp') key = 'k';
    else if (e.key.length === 1) key = e.key;
    if (key === null) return;
    if (m.view === 'console') { press(key); e.preventDefault(); return; }
    if (key === ' ' && !m.inside) return;
    e.preventDefault();
    press(key);
  });

  // The pointer, as conn's: a click on a row is the cursor to that row
  // and goes in; a click on the panel's air brings the keys back.
  panelEl.addEventListener('click', e => {
    const el = e.target.closest('.row');
    if (m.view !== 'processes') { if (m.view === 'log') { const i = [...panelEl.querySelectorAll('.row')].indexOf(el); const entry = Math.floor((i - 5) / 2); if (entry >= 0 && entry < logEntries().length) { m.logCursor = entry; press('enter'); } } return; }
    const idx = el ? [...panelEl.querySelectorAll('.row')].indexOf(el) : -1;
    const ids = [];
    for (const p of m.projects) { ids.push(null, null); for (const r of p.rows) ids.push(r.id); }
    const id = idx >= 0 ? ids[idx] : null;
    if (id) goIn(id); else if (m.inside) back();
    render();
  });
  station.addEventListener('click', e => {
    if (m.view === 'console') { if (!booted) finishBoot(); else leaveConsole(); return; }
    if (e.target.closest('.band') && m.inside && one) { back(); render(); }
  });

  function wireCopy() {
    for (const b of document.querySelectorAll('.copy')) {
      b.onclick = async ev => {
        ev.stopPropagation();
        const was = b.textContent;
        try { await navigator.clipboard.writeText(b.dataset.copy); b.textContent = '[copied]'; b.classList.add('done'); setTimeout(() => { b.textContent = was; b.classList.remove('done'); }, 1400); }
        catch (err) { b.textContent = '[select it]'; }
      };
    }
  }

  // ---- the console, and leaving it ------------------------------------

  let booted = false, bootTimers = [], consoleLast = 0;

  function showConsole(stage) {
    const c = drawConsole(stage);
    consoleLast = c.lastStage;
    station.classList.add('console');
    station.classList.remove('on-panel', 'on-work', 'detached');
    bandEl.style.display = '';
    paint(workEl, c.lines, rows, 0);
    if (c.lines.some(l => (Array.isArray(l) ? l[0] : l).segs.some(([cls]) => cls === 'wmtext'))) workEl.insertAdjacentHTML('beforeend', wordmarkSVG(MARGIN, 1));
    m.view = 'console';
    drawBand();
  }

  // The boot: the header at once, then the readout, then each check on
  // its turn, then the verdict. Any key, a tap or a scroll ends it.
  function boot() {
    showConsole(0);
    if (reduced) { booted = true; showConsole(99); return; }
    const c = drawConsole(0);
    if (c.small) { booted = true; return; }
    let t = 500;
    const stages = [[t, 1]]; t += 420;
    for (let i = 0; i < c.lastStage - 2; i++) { stages.push([t, 2 + i]); t += 170; }
    stages.push([t + 250, c.lastStage]);
    for (const [at, st] of stages) bootTimers.push(setTimeout(() => { showConsole(st); if (st === c.lastStage) booted = true; }, at));
  }
  function finishBoot() {
    if (booted) return;
    bootTimers.forEach(clearTimeout); bootTimers = [];
    booted = true;
    showConsole(99);
  }
  function leaveConsole() {
    if (!m.since || m.view === 'console' && !m.started) { m.since = Date.now(); m.started = true; }
    m.view = 'processes';
    render();
  }

  // The spinner turns on its own cadence, in every row at work.
  if (!reduced) {
    const FRAMES = ['⣾', '⣽', '⣻', '⢿', '⡿', '⣟', '⣯', '⣷'];
    let f = 0;
    setInterval(() => { f = (f + 1) % FRAMES.length; for (const s of document.querySelectorAll('.spin')) s.textContent = FRAMES[f]; }, 110);
  }
  // The machine's clock, and the band's.
  setInterval(() => { if (m.started && m.view !== 'console') tick(); }, 250);
  setInterval(() => { if (m.view !== 'detached') drawBand(); }, 15000);

  // For a look at a later moment: #t=9 opens on the panel with the
  // clock nine seconds in, and #t=9&k=tab,enter with those keys pressed.
  function layout() {
    measureGrid();
    const at = location.hash.match(/^#t=(\d+)(?:&k=([^&]*))?$/);
    if (at) {
      booted = true;
      m.since = Date.now() - Number(at[1]) * 1000; m.started = true; m.view = 'processes';
      for (const [when, id, fn] of TIMELINE) if (Number(at[1]) * 1000 >= when && !m.done[id]) { m.done[id] = true; fn(); }
      render();
      if (at[2] && !m.replayed) { m.replayed = true; for (const k of at[2].split(',')) press(k); }
      return;
    }
    if (m.view === 'console') { if (booted) showConsole(99); else boot(); }
    else render();
  }
  let resizeTimer = 0;
  addEventListener('resize', () => { clearTimeout(resizeTimer); resizeTimer = setTimeout(layout, 120); });
  // The station from the console of the browser: conn.press('tab').
  window.conn = { press, get model() { return m; } };
  if (document.fonts && document.fonts.ready) document.fonts.ready.then(layout);
  layout();
})();
