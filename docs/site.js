// conn.w0zro.com draws conn. The window is the station: the console
// comes up first, staged the way conn stages it, against the visitor's
// own browser; then the panel, and a session plays in scenes as the
// page is scrolled, each scene a screen. Everything is drawn on the
// terminal's grid in rows of cells, with the arithmetic of conn's own
// drawing: internal/console/screen.go for the console, filed.go for
// the panel, readout.go for the page, band.go and keybar.go for the
// band. The words are conn's words. The guide, a surface over the
// station, is the site's own voice, and the only one.
(() => {
  'use strict';

  // The build the site stands for. The release stamps these the way it
  // stamps the manual's cover.
  const V = { version: '0.23.0', built: '09-OCT-2026', commit: '12b688f', buildDate: new Date(2026, 9, 9) };
  const HOST = 'conn.w0zro.com';
  const INSTALL = 'curl -fsSL https://conn.w0zro.com/install.sh | sh';

  const $ = id => document.getElementById(id);
  const station = $('station'), panelEl = $('panel'), workEl = $('work');
  const b0 = $('b0'), b1 = $('b1'), scrollEl = $('scroll');
  const guideEl = $('guide'), progressEl = $('progress'), railEl = $('rail');
  const reduced = matchMedia('(prefers-reduced-motion: reduce)').matches;
  const touch = matchMedia('(hover: none)').matches;
  const arrived = new Date();

  // ---- the grid -------------------------------------------------------

  const MARGIN = 3, MIN_COLS = 80, PANEL_W = 44, FACT_COL = 12, CHECK_COL = 12, STATUS_W = 9;
  const GUIDE_PX = 420 + 72 + 22; // the guide's surface, the rail beside it, and their margins
  const guideRoom = () => Math.ceil(GUIDE_PX / cellW) + 2; // the cells it takes at the right
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
      add(cls, text) { if (!text) return l; segs.push([cls, text]); cells += width(text); return l; },
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
        const body = segs.map(([c, t]) => c ? `<span class="${c}">${esc(t)}</span>` : esc(t)).join('');
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
    const w = !one && cols - guideRoom() >= MIN_COLS ? cols - guideRoom() : cols;
    return { lines: small ? smallConsole(cols, rows, need()) : consoleBody(v, ks, w, stage), lastStage, small, compact: !small && w === cols };
  }

  // ---- the session ----------------------------------------------------

  const MARK = { contact: '☿', shell: '❯', editor: '▯', service: '◉', run: '○' };

  // The model: the projects, their rows, where the cursor is, whether
  // the keys are in a pane, and what the log has. A scene is a change
  // to it; the state of any scene is the base with every scene before
  // it applied, so scrolling back is the same picture as before.
  function base() {
    return {
      view: 'processes', cursor: 'zsh1', shown: null, inside: false, log: 0, logRows: [],
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
          { id: 'worker', kind: 'run', name: 'worker', status: 'down', word: 'DOWN' },
        ] },
      ],
      claude: 'working', answered: false, typed: 0, worker: 'down',
    };
  }
  const row = (m, id) => m.projects.flatMap(p => p.rows).find(r => r.id === id);
  const projectOf = (m, id) => m.projects.find(p => p.rows.some(r => r.id === id));

  // The scenes. enter is the state at the scene's head; beat, where a
  // scene has one, is what happens a moment after, and stands once the
  // scene is past.
  const SCENES = [
    { id: 'console', label: 'CONSOLE', title: 'The console', caption: null },
    { id: 'panel', label: '01 · THE PANEL', title: 'The panel',
      caption: 'Every process on the machine, under the project it works. A mark for what it is, and a word only where there is one to say.',
      enter(m) {} },
    { id: 'waiting', label: '02 · WAITING', title: 'A contact stops to ask',
      caption: 'The project’s rule says WAITING, the row says how long, and the band counts it.',
      enter(m) { const r = row(m, 'claude'); r.status = 'waiting'; r.age = '9 MIN'; m.log = 1;
        m.logRows.unshift({ t: '14:02', p: 'conn', n: 'claude', w: 'WAITING', note: 'Rename bay to workspace in the code?' }); } },
    { id: 'tab', label: '03 · TAB', pane: 'work', title: 'The one waiting longest', key: 'tab',
      caption: 'The cursor goes to it, and its question is on its page before you go in.',
      enter(m) { m.cursor = 'claude'; } },
    { id: 'enter', label: '04 · ENTER', pane: 'work', title: 'Go in and answer', key: 'enter',
      caption: 'The keys are in the pane. ctrl-space brings them back to the panel, whatever is in front of you.',
      enter(m) { m.inside = true; m.shown = 'claude'; },
      beat(m) { const r = row(m, 'claude'); r.status = 'working'; r.age = ''; m.answered = true; m.log = 2;
        m.logRows.unshift({ t: '14:11', p: 'conn', n: 'claude', w: 'IDLE', note: 'waited 9 min' }); } },
    { id: 'log', label: '05 · THE LOG', title: 'The log', key: 'l',
      caption: 'What changed while you were in a pane: a wait, a fault, a turn over, a pane that said FAIL. Newest first.',
      enter(m) { m.inside = false; m.shown = null; m.view = 'log'; m.log = 0; } },
    { id: 'declared', label: '06 · WHAT A PROJECT DECLARES', title: 'What a project declares', key: '.conn',
      caption: 'The file names what works a project. What is down is a row, worded so; u brings it up, x ends it.',
      enter(m) { m.view = 'processes'; m.cursor = 'worker'; },
      beat(m) { const r = row(m, 'worker'); r.status = 'working'; r.word = ''; m.worker = 'up'; } },
    { id: 'install', label: '07 · INSTALL', pane: 'work', title: 'Install', key: '$',
      caption: 'One line, on macOS or Linux. man conn is the reference; the manual is how it is worked.',
      enter(m) { m.cursor = 'zsh1'; m.inside = true; m.shown = 'zsh1'; m.typed = 0; },
      beat(m) { m.typed = INSTALL.length + 1; } },
  ];
  function stateAt(n, beaten) {
    const m = base();
    for (let i = 1; i <= n && i < SCENES.length; i++) {
      const s = SCENES[i];
      if (s.enter) s.enter(m);
      if (s.beat && (i < n || beaten)) s.beat(m);
    }
    return m;
  }

  // ---- the panel ------------------------------------------------------

  // A block's verdict: what wants you, at the end of the rule.
  function verdict(p) {
    const waiting = p.rows.filter(r => r.status === 'waiting').length;
    if (waiting > 0) return stamp(waiting > 1 ? `${waiting} WAITING` : 'WAITING', true);
    const fault = p.rows.find(r => r.status === 'stopped');
    if (fault) return stamp(fault.word, false);
    return null;
  }

  function drawPanel(m, w) {
    const measure = w - MARGIN, out = [];
    for (const p of m.projects) {
      out.push(blank());
      out.push(line().eyebrow(MARGIN, p.name, measure, verdict(p)));
      for (const r of p.rows) {
        const l = line();
        const cursored = r.id === m.cursor && !m.inside;
        const shown = r.id === m.shown;
        if (cursored || shown) l.add('acc', '▌');
        if (r.status === 'working') l.to(1).add('run spin', '⣾');
        let tone = 'faint';
        if (r.status === 'waiting') tone = 'acc b';
        else if (r.status === 'stopped') tone = 'acc';
        else if (r.status === 'working' || (r.port && r.kind !== 'contact')) tone = 'run';
        l.to(3).add(tone, MARK[r.kind]);
        l.add('', r.kind === 'contact' ? ` ${r.digit} ` : '   ');
        const over = r.status === 'down' || r.status === 'ended';
        l.add(over ? 'struck' : (r.status === 'waiting' ? 'ink b' : 'ink'), fit(r.name, measure - l.cells - 10));
        if (r.status === 'waiting') { const s = stamp(r.age || 'WAITING', true); l.to(measure - s.w); s.draw(l); }
        else if (r.status === 'stopped') { const s = stamp(r.word, false); l.to(measure - s.w); s.draw(l); }
        else if (r.word) l.to(measure - width(r.word)).add('gray', r.word);
        else if (r.port) l.to(measure - width(r.port)).add('gray', r.port);
        out.push([l, cursored ? 'chosen' : '']);
      }
    }
    return out;
  }

  // The log, as the panel shows it over time.
  function drawLog(m, w) {
    const measure = w - MARGIN, out = [], L = () => line(MARGIN);
    out.push(blank());
    out.push(L().add('par b', 'LOG').to(MARGIN + measure - 5).add('gray', `${m.logRows.length} NEW`));
    out.push(L().rule(measure));
    out.push(blank());
    out.push(line().eyebrow(MARGIN, 'TODAY', measure, null));
    const older = [
      { t: '11:41', p: 'web', n: 'go test ./...', w: 'EXIT 1', note: 'ran 2 min' },
      { t: '09:04', p: 'web', n: 'node vite', w: 'ACTIVE', note: ':5173' },
    ];
    [...m.logRows, ...older].forEach((e, i) => {
      const l = line();
      if (i === 0) l.add('acc', '▌');
      l.to(MARGIN).add('ink', e.t).add('', ' ').add('par', e.p.padEnd(5)).add('', ' ').add('ink', fit(e.n, measure - l.cells - width(e.w) - 2));
      if (e.w === 'WAITING') { const s = stamp(e.w, false); l.to(measure - s.w); s.draw(l); }
      else l.to(measure - width(e.w)).add('gray', e.w);
      out.push([l, i === 0 ? 'chosen' : '']);
      out.push(line(MARGIN + 12).add('gray', fit(e.note, measure - MARGIN - 12)));
    });
    return out;
  }

  // ---- the workspace --------------------------------------------------

  // The page: what conn knows about the row under the cursor, read
  // without entering it. A left column at the measure; the
  // specifications to its right where the pane is wide enough.
  function drawPage(m, w) {
    const r = row(m, m.cursor), p = projectOf(m, m.cursor);
    const wide = w >= 104, clear = one ? w - 2 * MARGIN : Math.max(60, w - guideRoom() - MARGIN);
    const leftW = Math.min(wide ? w - 2 * MARGIN - 34 : w - 2 * MARGIN, clear), specCol = MARGIN + leftW + 6;
    const out = [], L = () => line(MARGIN);
    const para = (text, cls = 'ink') => wrap(text, leftW).forEach(t => out.push(L().add(cls, t)));
    const procedure = steps => {
      out.push(blank()); out.push(line().eyebrow(MARGIN, 'PROCEDURE', MARGIN + leftW, null)); out.push(blank());
      steps.forEach(([k, does], i) => out.push(L().add('gray', String(i + 1)).to(MARGIN + 5).add('ink b', k).to(MARGIN + 14).add('ink', fit(does, leftW - 14))));
    };
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
    out.push(blank());
    if (r.kind === 'contact') {
      out.push(line().eyebrow(MARGIN, 'CONTACT', MARGIN + leftW, null));
      out.push(blank());
      out.push(L().add('ink b', ` ${r.digit}`).to(MARGIN + 5).add('ink', `conn-${r.digit}`).to(MARGIN + 14).add('gray', '~/projects/w0zro/conn · main'));
      out.push(L().to(MARGIN + 5).add('gray', 'Claude Code 2.1.267 · 212k of context carried'));
      out.push(blank());
      if (r.status === 'waiting') {
        box([['ink b', 'WAITING FOR YOU', '9 min'], ['', ''], ['ink', '“Rename bay to workspace in the code too?”'], ['', ''], ['gray', 'Asked with AskUserQuestion at 14:02']]);
        procedure([['enter', 'Go in and answer it'], ['tab', 'Leave it, take the next thing waiting'], ['x', 'End it — conn asks first']]);
        out.push(blank()); out.push(line().eyebrow(MARGIN, 'CAUTION', MARGIN + leftW, null)); out.push(blank());
        para('It has the terminal and is sleeping. x signals the contact, not what it runs — the process under it keeps going.');
      } else {
        box([['run b', 'WORKING', m.answered ? '1 min' : '12 min'], ['', ''], ['ink', m.answered ? 'Renaming bay to workspace in the code' : 'Reading band.go, keys.go, bay.go'], ['', ''], ['gray', m.answered ? 'You last answered “1. Yes” at 14:11' : 'You last said “bring the code to the manual” at 13:58']]);
        procedure([['enter', 'Go in'], ['tab', 'The next thing waiting'], ['x', 'End it — conn asks first']]);
      out.push(blank()); out.push(line().eyebrow(MARGIN, 'HOW THE WORK HAS GONE', MARGIN + leftW, null)); out.push(blank());
      const barW = Math.min(40, leftW - 28);
      const worked = Math.round(barW * 85 / 94);
      out.push(L().add('run', rep('█', worked)).add('acc', rep('█', barW - worked)).add('gray', `  worked 1h 25m · ${r.status === 'waiting' ? 'waiting 9m' : 'asked once'}`));
      out.push(blank());
      para(r.status === 'waiting' ? 'It came up at 12:36 and worked for 1h 25m, then stopped to ask. You last said “bring the code to the manual”.'
        : 'It came up at 12:36, worked for 1h 25m, asked once and was answered. It is at work again.', 'ink');
      }
    } else if (r.status === 'down') {
      out.push(line().eyebrow(MARGIN, 'DECLARED', MARGIN + leftW, null));
      out.push(blank());
      out.push(L().add('ink b', r.name).to(MARGIN + 10).add('gray', 'services/queue').to(MARGIN + 27).add('ink', 'make run'));
      out.push(L().add('gray', '.conn line 3 · not running'));
      procedure([['enter', 'Bring it up and go in'], ['u', 'Bring it up'], ['U', 'Bring up everything declared and down']]);
      out.push(blank()); out.push(line().eyebrow(MARGIN, 'THE FILE', MARGIN + leftW, null)); out.push(blank());
      out.push(L().add('gray', '# what works this project'));
      out.push(L().add('ink', 'web frontend:          npm run dev'));
      out.push(L().add('ink', 'api:                   go run ./cmd/api'));
      out.push(L().add('ink b', 'worker services/queue: make run'));
      out.push(L().add('ink', 'db:                    docker compose up postgres'));
      out.push(L().add('ink', 'cache:                 brew services start redis'));
      out.push(blank());
      para('A line is name [dir]: command. The name is the row. A docker compose up has its services as rows under it; a brew services start is the Homebrew service, as brew reports it.', 'ink');
    } else if (r.kind === 'shell') {
      out.push(line().eyebrow(MARGIN, 'SHELL', MARGIN + leftW, null));
      out.push(blank());
      out.push(L().add('ink b', r.name).to(MARGIN + 10).add('gray', `~/projects/w0zro/${p.name} · main`));
      out.push(L().add('gray', 'at its prompt · up 2h 14m · ttys004'));
      procedure([['enter', 'Go in'], ['s', 'A shell beside it'], ['x', 'End it — conn asks first']]);
      out.push(blank()); out.push(line().eyebrow(MARGIN, 'THE PROJECT', MARGIN + leftW, null)); out.push(blank());
      [['Branch', 'main · clean'], ['Commit', '12b688f · The front page says it in rows, not paragraphs'], ['Tracking', 'origin/main · 3 ahead'], ['Folder', `~/projects/w0zro/${p.name}`]]
        .forEach(([k, v]) => out.push(L().leader(k, MARGIN + 13, 'faint').add('ink', fit(v, leftW - 15))));
    } else {
      out.push(line().eyebrow(MARGIN, r.kind.toUpperCase(), MARGIN + leftW, null));
      out.push(blank());
      out.push(L().add('ink b', r.name).to(MARGIN + 16).add('gray', `~/projects/w0zro/${p.name}`));
      procedure([['enter', 'Go in'], ['x', 'End it — conn asks first']]);
    }
    if (wide && r.kind === 'contact') {
      // The specifications, a column to the right of the page's rows.
      const specW = w - MARGIN - specCol;
      const spec = [['Designation', `CONN-${r.digit}`], ['Given', 'Today, 12:36'], ['Worked', '1h 25m'], ['Waiting', r.status === 'waiting' ? '9 min' : '\u2014'], ['', ''],
        ['Branch', 'main \u00b7 3 changed'], ['Commit', '12b688f'], ['Tracking', 'origin/main'], ['', ''], ['Folder', '~/projects/w0zro/conn'], ['Pane', '%2'], ['Process', '49212'],
        ['Command', 'claude --resume'], ['Model', 'claude-fable-5-1']];
      const right = [line().eyebrow(0, 'SPECIFICATIONS', specW, null), blank()];
      spec.forEach(([k, v]) => right.push(k ? line().leader(k, 13, 'faint').add('ink', fit(v, specW - 15)) : blank()));
      const merged = [];
      for (let i = 0; i < Math.max(out.length, right.length + 1); i++) {
        const left = i < out.length ? out[i] : blank();
        const [l, cls] = Array.isArray(left) ? left : [left, ''];
        const rt = i >= 1 && i - 1 < right.length ? right[i - 1] : null;
        merged.push([rt ? mergeLines(l, rt, specCol) : l, cls]);
      }
      return merged;
    }
    return out;
  }

  // Two lines on one row: the left cut at the column the right starts.
  function mergeLines(left, right, col) {
    const l = line();
    for (const [c, t] of left.segs) {
      const room = col - 1 - l.cells;
      if (room <= 0) break;
      l.add(c, width(t) > room ? [...t].slice(0, room).join('') : t);
    }
    l.to(col);
    for (const [c, t] of right.segs) l.add(c, t);
    return l;
  }

  // The contact's pane: Claude Code, the question, and after the beat
  // the answer and the work going on.
  function drawClaude(m, w) {
    const out = [], L = () => line(2), tw = w - 4;
    const para = (lead, cls, text) => wrap(text, tw - 2).forEach((t, i) => out.push(i === 0 ? L().add(cls, lead).add('', ' ').add('ink', t) : L().to(4).add('ink', t)));
    out.push(blank());
    para('>', 'par', 'the manual says workspace where the code says bay. bring the code to the manual');
    out.push(blank());
    para('●', 'run', 'Read bay.go, band.go, keys.go, dress.go');
    out.push(blank());
    para('●', 'acc', 'The type is bay in four files, and the held screens under testdata name it. Renaming it is one commit, with the screens held again.');
    out.push(blank());
    out.push(L().to(4).add('ink', 'Rename bay to workspace in the code?'));
    if (!m.answered) {
      out.push(L().to(4).add('acc', '❯').add('ink', ' 1. Yes'));
      out.push(L().to(6).add('ink', '2. No, leave the code as it is'));
    } else {
      out.push(L().to(4).add('gray', '  1. Yes'));
      out.push(blank());
      para('>', 'par', '1');
      out.push(blank());
      para('●', 'run', 'Renaming bay to workspace: bay.go → workspace.go, the type and its methods, and the screens under testdata held again.');
      out.push(blank());
      out.push(L().add('run spin', '⣾').add('gray', ' Renaming… (esc to interrupt)'));
    }
    return out;
  }

  // The shell's pane: the install line typed, and what install.sh says.
  function drawShell(m, w) {
    const out = [], L = () => line(1), typed = INSTALL.slice(0, Math.min(m.typed, INSTALL.length));
    out.push(blank());
    const l = L().add('acc b', '$').add('', ' ').add('ink', typed);
    if (m.typed <= INSTALL.length) l.add('chip', ' ');
    out.push(l);
    if (m.typed > INSTALL.length) {
      out.push(L().add('gray', `downloading conn v${V.version} for darwin_arm64`));
      out.push(L().add('gray', `installed conn v${V.version} to ~/.local/bin/conn`));
      out.push(L().add('gray', 'installed conn.1 to ~/.local/share/man/man1'));
      out.push(blank());
      out.push(L().add('acc b', '$').add('', ' ').add('ink', 'conn').add('chip', ' '));
    }
    void w;
    return out;
  }

  // ---- the band -------------------------------------------------------

  function uptime() {
    const mins = Math.round((Date.now() - arrived) / 60000), h = Math.floor(mins / 60), mi = mins % 60;
    return `${pad2(h)}h ${pad2(mi)}m`;
  }
  function drawBand(m, scene) {
    const now = new Date();
    const right = `${pad2(now.getHours())}:${pad2(now.getMinutes())} · T+ ${uptime()} `;
    const l0 = line();
    if (scene > 0) {
      if (m.view === 'log' && !m.inside) l0.add('chip', ' LOG ');
      else { l0.add('ink b', ' CONN '); if (m.log > 0) l0.add('gray', `· LOG ${m.log} `); }
    }
    l0.to(cols - width(right)).add('gray', right);
    b0.innerHTML = l0.html();
    const ident = `${HOST.toUpperCase()} · conn ${V.version} `;
    let hints;
    if (scene === 0) hints = [[touch ? 'tap' : 'any key', 'Continue']];
    else if (m.inside) hints = [['ctrl-space', 'Panel'], ['ctrl-space tab', 'Next waiting'], ['ctrl-space ctrl-space', 'Last process'], ['ctrl-space ?', 'Help']];
    else if (m.view === 'log') hints = [['j k', 'Move'], ['enter', 'Go to it'], ['esc', 'Back']];
    else {
      const r = row(m, m.cursor);
      hints = [['j k', 'Move'], ['enter', r.status === 'down' ? 'Bring it up' : 'Go in']];
      if (m.projects.some(p => p.rows.some(x => x.status === 'waiting'))) hints.push(['tab', 'Next waiting']);
      if (r.status === 'down') hints.push(['u', 'Bring it up'], ['U', 'All of them']);
      else hints.push(['x', 'End it']);
      if (r.kind === 'contact') hints.push(['A', 'Sessions']);
      hints.push(['l', 'Log'], ['?', 'Help']);
    }
    // The keys that fit, from the left; the designation at the right
    // where there is room for it after them.
    const barOf = hs => { const l = line().add('', ' '); hs.forEach(([k, does], i) => { if (i) l.add('', '   '); l.add('ink b', k + ' ').add('gray', does.toLowerCase()); }); return l; };
    while (hints.length > 1 && barOf(hints).cells > cols) hints.pop();
    const l1 = barOf(hints);
    if (!one && l1.cells + width(ident) <= cols) l1.to(cols - width(ident)).add('gray', ident);
    b1.innerHTML = l1.html();
  }

  // ---- the scenes on the screen ---------------------------------------

  let scene = -1, model = base(), beatTimer = 0, typeTimer = 0, bootTimers = [], booted = false, bootStage = 0, consoleLast = 0;

  function clearTimers() {
    clearTimeout(beatTimer); clearTimeout(typeTimer);
    bootTimers.forEach(clearTimeout); bootTimers = [];
  }

  function showConsole(stage) {
    const c = drawConsole(stage);
    consoleLast = c.lastStage;
    if (c.compact !== guideCompact) { guideCompact = c.compact; drawGuide(0); }
    station.classList.add('console');
    station.classList.remove('on-panel', 'on-work');
    paint(workEl, c.lines, rows, 0);
    if (c.lines.some(l => (Array.isArray(l) ? l[0] : l).segs.some(([cls]) => cls === 'wmtext'))) workEl.insertAdjacentHTML('beforeend', wordmarkSVG(MARGIN, 1));
    drawBand(model, 0);
  }

  function showScene(n, beaten, animate) {
    model = stateAt(n, beaten);
    station.classList.remove('console');
    const panelScene = SCENES[n].pane !== 'work';
    station.classList.toggle('on-panel', one && panelScene);
    station.classList.toggle('on-work', one && !panelScene);
    const step = animate ? 28 : 0;
    paint(panelEl, model.view === 'log' ? drawLog(model, panelW) : drawPanel(model, panelW), rows, step);
    let work;
    if (model.inside && model.shown === 'claude') work = drawClaude(model, workW);
    else if (model.inside && model.shown === 'zsh1') work = drawShell(model, workW);
    else work = drawPage(model, workW);
    paint(workEl, work, rows, animate ? 14 : 0);
    drawBand(model, n);
  }

  const shortLabel = i => i === 0 ? 'CONSOLE' : SCENES[i].label.replace(' · ', ' ');
  const CLAIM = 'A console for the processes working your projects.';

  function frameFor(n) { drawGuide(n); }

  // The guide: a surface over the station. The demonstration's steps,
  // the one on screen lit, how far along, the scene's line and what
  // comes next; the links; and on the first and last scenes the
  // install line. On a phone it docks across the top and says the
  // step alone.
  let guideCompact = false; // the console has no room beside it for the steps
  function drawGuide(n) {
    const last = SCENES.length - 1, s = SCENES[n];
    guideEl.classList.toggle('compact', n === 0 && guideCompact && !one);
    guideEl.classList.toggle('docked', one);
    const nextWord = n < last
      ? `<b>${n === 0 && !touch ? 'any key' : touch ? 'scroll' : 'j'}</b>${n === 0 && !touch ? ', or scroll' : touch ? '' : ', or scroll'} <span class="arrow">\u2193</span> ${esc(shortLabel(n + 1))}`
      : `<b>g</b> <span class="arrow">\u2191</span> the console`;
    const text = n === 0 ? 'The console comes up against this browser: its screen, its font, its clock, this site. Then a session, scene by scene.' : s.caption;
    const titled = s.title, key = s.key;
    const cta = (n === 0 || n === last) && !one
      ? `<div class="cta"><span class="prompt">$</span><code>${esc(INSTALL)}</code><button class="copy" type="button" data-copy="${esc(INSTALL)}">COPY</button></div>` : '';
    guideEl.innerHTML = `
      <div class="body">
      <div class="head"><span class="title">DEMONSTRATION</span><span class="count">${pad2(n)} / ${pad2(last)}</span></div>
      <h2>${esc(titled)}${key ? `<small>${esc(key)}</small>` : ''}</h2>
      <p class="line">${esc(text)}</p>
      <div class="next">${nextWord}</div>
      ${cta}
      <div class="foot"><span class="needs">macOS \u00b7 Linux \u00b7 tmux</span><nav class="links"><a href="/manual/">MANUAL</a><a href="https://github.com/w0zro/conn/blob/main/man/conn.1">MAN</a><a class="accent" href="https://github.com/w0zro/conn">GITHUB \u2197</a></nav></div>
      </div>`;
    wireCopy();
    const pad = one ? (guideEl.offsetHeight + 14) + 'px' : '';
    panelEl.style.paddingTop = pad; workEl.style.paddingTop = pad;
    progressEl.style.width = (100 * n / last) + '%';
    drawRail(n);
  }

  // The rail: a number a scene, the one on screen under the marker.
  function drawRail(n) {
    if (!railEl.querySelector('a')) {
      railEl.insertAdjacentHTML('beforeend', SCENES.map((_, i) => `<a href="#s${i}" data-scene="${i}">${pad2(i)}</a>`).join(''));
      for (const a of railEl.querySelectorAll('a')) a.onclick = e => { e.preventDefault(); scrollTo(Number(a.dataset.scene)); };
    }
    railEl.querySelectorAll('a').forEach((a, i) => { a.classList.toggle('on', i === n); a.classList.toggle('done', i < n); });
    const step = parseFloat(getComputedStyle(railEl).getPropertyValue('--step')) || 26;
    railEl.querySelector('.marker').style.top = (5 + n * step) + 'px';
  }

  function goTo(n, animate = true) {
    n = Math.max(0, Math.min(SCENES.length - 1, n));
    if (n === scene) return;
    clearTimers();
    scene = n;
    frameFor(n);
    if (n === 0) {
      if (booted) showConsole(consoleLast + 1);
      else boot();
      return;
    }
    booted = true;
    showScene(n, false, animate);
    const s = SCENES[n];
    if (s.beat) {
      if (s.id === 'install') {
        // The install line is typed, a character at a time.
        const type = () => {
          model.typed++;
          paint(workEl, drawShell(model, workW), rows, 0);
          if (model.typed <= INSTALL.length) typeTimer = setTimeout(type, model.typed === INSTALL.length + 1 ? 400 : 28 + Math.random() * 40);
        };
        typeTimer = setTimeout(type, reduced ? 0 : 700);
        if (reduced) { model.typed = INSTALL.length + 1; paint(workEl, drawShell(model, workW), rows, 0); }
      } else {
        beatTimer = setTimeout(() => showScene(n, true, false), reduced ? 0 : (s.id === 'declared' ? 3200 : 2600));
      }
    }
  }

  // The boot: the header at once, then the readout, then each check on
  // its turn, then the verdict. Any key, a tap or a scroll ends it.
  function boot() {
    bootStage = 0;
    showConsole(0);
    if (reduced) { booted = true; showConsole(99); return; }
    const c = drawConsole(0);
    if (c.small) { booted = true; return; }
    let t = 500;
    const stages = [];
    stages.push([t, 1]); t += 420;
    for (let i = 0; i < c.lastStage - 2; i++) { stages.push([t, 2 + i]); t += 170; }
    stages.push([t + 250, c.lastStage]);
    for (const [at, st] of stages) bootTimers.push(setTimeout(() => { bootStage = st; showConsole(st); if (st === c.lastStage) booted = true; }, at));
  }

  function finishBoot() {
    if (booted) return;
    clearTimers();
    booted = true;
    showConsole(99);
  }

  // ---- the keys, the scroll, the clock --------------------------------

  const vh = () => innerHeight;
  const sceneOfScroll = () => Math.round(scrollY / vh());
  const scrollTo = n => window.scrollTo({ top: n * vh(), behavior: reduced ? 'auto' : 'smooth' });

  addEventListener('scroll', () => {
    const n = sceneOfScroll();
    if (n !== scene) { if (n > 0) finishBoot(); goTo(n); }
  }, { passive: true });

  addEventListener('keydown', e => {
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    const k = e.key;
    if (scene === 0) {
      if (!booted) { finishBoot(); e.preventDefault(); return; }
      if (k === 'k' || k === 'ArrowUp' || k === 'PageUp') return;
      scrollTo(1); e.preventDefault(); return;
    }
    const next = ['j', 'ArrowDown', ' ', 'Tab', 'Enter', 'PageDown', 'l', 'u'];
    const prev = ['k', 'ArrowUp', 'PageUp', 'Escape'];
    if (next.includes(k)) { scrollTo(scene + 1); e.preventDefault(); }
    else if (prev.includes(k)) { scrollTo(scene - 1); e.preventDefault(); }
    else if (k === 'g') { scrollTo(0); e.preventDefault(); }
    else if (k === 'G') { scrollTo(SCENES.length - 1); e.preventDefault(); }
    else if (k === '?') { location.href = '/manual/'; }
  });

  station.addEventListener('click', () => {
    if (scene === 0) { if (!booted) finishBoot(); else scrollTo(1); }
  });

  function wireCopy() {
    for (const b of document.querySelectorAll('.copy')) {
      b.onclick = async () => {
        const was = b.textContent;
        try { await navigator.clipboard.writeText(b.dataset.copy); b.textContent = 'COPIED'; b.classList.add('done'); setTimeout(() => { b.textContent = was; b.classList.remove('done'); }, 1400); }
        catch (e) { b.textContent = 'SELECT IT'; }
      };
    }
  }

  // The spinner turns on its own cadence, in every row at work.
  if (!reduced) {
    const FRAMES = ['⣾', '⣽', '⣻', '⢿', '⡿', '⣟', '⣯', '⣷'];
    let f = 0;
    setInterval(() => { f = (f + 1) % FRAMES.length; for (const s of document.querySelectorAll('.spin')) s.textContent = FRAMES[f]; }, 110);
  }
  // The band's clock, to the minute, and the mission clock with it.
  setInterval(() => drawBand(model, scene), 15000);

  // A scene can be linked: #s3 opens on the third, the console done.
  const still = new URLSearchParams(location.search).has('still'); // a linked scene held for a screenshot
  const linked = () => { const m = location.hash.match(/^#s(\d)$/); return m ? Math.min(Number(m[1]), SCENES.length - 1) : -1; };

  function layout() {
    measureGrid();
    scrollEl.innerHTML = SCENES.map(() => '<section></section>').join('');
    let n = scene < 0 ? sceneOfScroll() : scene;
    if (scene < 0 && linked() >= 0) { n = linked(); booted = true; if (!still) window.scrollTo({ top: n * vh(), behavior: 'auto' }); }
    scene = -1;
    if (n === 0) { if (booted) { scene = 0; frameFor(0); showConsole(99); } else goTo(0); }
    else { booted = true; goTo(n, false); }
  }

  let resizeTimer = 0;
  addEventListener('resize', () => { clearTimeout(resizeTimer); resizeTimer = setTimeout(layout, 120); });

  // The font first, so the grid is measured in it; the page draws in
  // the fallback before it arrives, and again when it does.
  if (document.fonts && document.fonts.ready) document.fonts.ready.then(layout);
  layout();
})();
