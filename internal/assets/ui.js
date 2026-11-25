// Text lines (Look tab): the presets, the placeholders and how a line is filled, for the page's sample. The program
// fills the card's lines with the same rules (internal/core/lines.go); internal/core/testdata/lines.json holds the
// cases both must agree on (the page's test runs them through MPRP_LINES.expand).
const MPRP_LINES = (() => {
  const SEP = ' · ';
  const COVER = '{title}[ ({year})]';
  const KEYS = ['episodeDetails', 'episodeState', 'movieDetails', 'movieState', 'movieCover'];
  // episode first and second line, film first and second line, film cover text (Standard is the program's own lines;
  // these are what Custom starts from)
  const PRESETS = {
    standard: ['{episodeTitle}', '{seasonName}' + SEP + '{genres}' + SEP + '{rating}', '{year}' + SEP + '{genres}', '{rating}' + SEP + '{director}', COVER],
    short: ['{code}' + SEP + '{episodeTitle}', '', '{year}', '{director}', COVER],
    detailed: ['{code}' + SEP + '{episodeTitle}', '{genres}' + SEP + '{rating}' + SEP + '[on {player}]', '{year}' + SEP + '{genres}' + SEP + '{length}', '{rating}' + SEP + '{director}' + SEP + '[on {player}]', COVER],
  };
  const SAMPLE = {
    episode: { title: 'Sample Show', year: '', genres: 'Drama, Comedy', rating: '★ 8.3', director: '', player: 'VLC media player', length: '45:00', code: 'S01E02', season: '1', episode: '2', episodeTitle: 'Sample Episode', seasonName: '' },
    movie: { title: 'Sample Movie', year: '2020', genres: 'Drama, Comedy', rating: '★ 7.1', director: 'Dir. Sample Name', player: 'VLC media player', length: '1:52:10' },
  };
  const CHIPS = {
    episode: ['title', 'code', 'season', 'episode', 'episodeTitle', 'seasonName', 'genres', 'rating', 'player', 'length'],
    movie: ['title', 'year', 'genres', 'rating', 'director', 'player', 'length'],
  };
  const has = (v, k) => Object.prototype.hasOwnProperty.call(v, k);
  // each known {name} becomes its value (an unknown one stays as typed); a part in [ ] drops out when a placeholder in
  // it is empty; parts between " · " that end up empty go with their separator; trimmed and cut to 128 characters
  function expand(tpl, v) {
    const fill = (s) => s.replace(/\{(\w+)\}/g, (m, k) => (has(v, k) ? v[k] : m));
    let s = String(tpl || '').replace(/\[([^[\]]*)\]/g, (m, inner) => {
      let empty = false;
      inner.replace(/\{(\w+)\}/g, (x, k) => { if (has(v, k) && v[k] === '') empty = true; return x; });
      return empty ? '' : inner;
    });
    s = fill(s).split(SEP).map((p) => p.trim()).filter(Boolean).join(SEP).trim();
    if (s.length > 128) s = s.slice(0, 125) + '...';
    return s;
  }
  // the placeholders in tpls that v does not know ("" when there are none)
  function unknown(tpls, v) {
    const out = [];
    tpls.forEach((t) => String(t || '').replace(/\{(\w+)\}/g, (m, k) => { if (!has(v, k) && !out.includes(m)) out.push(m); return m; }));
    return out.length ? 'Unknown: ' + out.join(', ') : '';
  }
  return { SEP, COVER, KEYS, PRESETS, SAMPLE, CHIPS, expand, unknown };
})();

(() => {
  // The page gets its token when it is opened with a one-time code. It keeps the token for this window (a reload
  // comes without one) and takes the code, or a token, out of its address.
  let TOKEN = document.querySelector('meta[name="token"]').content;
  try {
    if (TOKEN) { sessionStorage.setItem('token', TOKEN); history.replaceState(null, '', location.pathname); }
    else TOKEN = sessionStorage.getItem('token') || '';
  } catch (_) {}
  const $ = (id) => document.getElementById(id);
  let model = null, queue = Promise.resolve(), queued = 0, clearTimer = null, saving = 0, gone = false, timers = [], running = false;

  async function api(name, body, method) {
    const res = await fetch('/api/' + name, { method: method || (body === undefined ? 'GET' : 'POST'), headers: { 'X-Token': TOKEN, 'Content-Type': 'application/json' }, body: body === undefined ? undefined : JSON.stringify(body) });
    let j = {}; try { j = await res.json(); } catch (_) {}
    if (!res.ok) throw new Error(j.error || ('HTTP ' + res.status));
    return j;
  }
  const el = (tag, props, ...kids) => { const e = Object.assign(document.createElement(tag), props || {}); kids.forEach((k) => e.append(k)); return e; };
  const help = (text) => el('div', { className: 'help', textContent: text });
  // a setting's help: a long one shows its first sentence, the rest behind "More"
  const longHelp = (text) => {
    const m = text.length > 110 && /^(.{20,}?[.:])\s+(\S.*)$/s.exec(text);
    if (!m) return help(text);
    return el('div', { className: 'help' }, document.createTextNode(m[1] + ' '), el('details', {}, el('summary', { textContent: 'More' }), document.createTextNode(m[2])));
  };
  // a switch with its label (and help) on the left
  const switchRow = (label, input, helpText, cls) => el('label', { className: 'line' + (cls ? ' ' + cls : '') }, el('span', { className: 'lbl' }, document.createTextNode(label), ...(helpText ? [cls ? help(helpText) : longHelp(helpText)] : [])), el('span', { className: 'sw' }, input, el('span')));
  // the bottom line: empty at rest, "Saving…" and "Saved" for a moment after a change, errors until the next change
  const note = (text, err) => {
    clearTimeout(clearTimer); $('msg').className = 'msg' + (err ? ' err' : ''); $('msg').textContent = text;
    if (!err) clearTimer = setTimeout(() => { $('msg').textContent = ''; }, 2500);
  };
  // one change at a time, in order
  const later = (task) => { queued++; const run = async () => { try { await task(); } finally { queued--; } }; queue = queue.then(run, run); };
  // a setting changed while something else is still under way (a sign-in can wait on its server for seconds): say at
  // once that the change was taken, it is saved when its turn comes
  const saveLater = (run) => { if (queued > 0) note('Saving…'); later(run); };
  const readValue = (i) => (i.type === 'checkbox' ? i.checked : i.value);
  const showValue = (i, v) => { if (i.type === 'checkbox') i.checked = !!v; else i.value = v == null ? '' : v; };
  const appValue = (key) => (key === 'autoStart' ? !!model.autoStart : model.app[key]);

  // Texts that differ on macOS and Linux (the page is written for Windows; the program says which system it runs on).
  const OS_TEXT = {
    darwin: {
      players: 'IINA, mpv, VLC, MPC-QT or Kodi', here: 'this Mac', autoStart: 'Start at login', copyKey: 'Cmd+C',
      help: '<b>No player is answering.</b> Open your player and play something. IINA, mpv and VLC need setting up first: the button does it.',
    },
    linux: {
      players: 'VLC, mpv, Celluloid, Haruna, SMPlayer, MPC-QT, Kodi or another video player', here: 'this computer', autoStart: 'Start at login', copyKey: 'Ctrl+C',
      help: '<b>No player is answering.</b> Open VLC, Celluloid, Haruna or another video player and play something; they need no setup. mpv needs a line in mpv.conf: the button adds it.',
    },
  };
  const osText = () => (model && model.os !== 'windows' ? OS_TEXT[model.os === 'darwin' ? 'darwin' : 'linux'] : null);
  const players = () => (osText() ? osText().players : 'MPC-HC, MPC-BE, MPC-QT, mpv, VLC, PotPlayer or Kodi');
  // an in-page dialog (the browser's confirm() and alert() would head the box with this page's address). buttons are
  // [label, class] pairs, the first being the safe choice that has the focus; it resolves to the index of the button
  // pressed, or -1 for Escape or a click outside. body is text or an element. opt.focus is the button that has the
  // focus instead of the first. opt.hold(i) runs when button i is pressed, with the dialog's buttons switched off
  // until it has answered: the dialog closes when it answers true and stays open otherwise.
  let modalCount = 0;
  const modal = (title, body, buttons, wide, opt = {}) => new Promise((resolve) => {
    const prev = document.activeElement;
    let busy = false;
    const pick = async (i) => {
      if (busy) return;
      if (opt.hold) {
        busy = true; btns.forEach((b) => { b.disabled = true; });
        let close = true;
        try { close = await opt.hold(i); } finally { busy = false; btns.forEach((b) => { b.disabled = false; }); }
        if (!close) { btns[opt.focus || 0].focus(); return; }
      }
      done(i);
    };
    const btns = buttons.map(([label, cls], i) => { const b = el('button', { textContent: label, className: cls || '' }); b.onclick = () => pick(i); return b; });
    const content = typeof body === 'string' ? el('p', { textContent: body }) : body;
    const uid = 'dlg' + (++modalCount);
    const box = el('div', { className: 'dlg' + (wide ? ' wide' : '') }, el('h2', { textContent: title, id: uid + '-title' }), content, el('div', { className: 'row' }, ...btns));
    box.setAttribute('role', 'alertdialog'); box.setAttribute('aria-modal', 'true');
    content.id = uid + '-body'; box.setAttribute('aria-labelledby', uid + '-title'); box.setAttribute('aria-describedby', uid + '-body');
    const back = el('div', { className: 'dlg-back' }, box);
    const done = (v) => { document.removeEventListener('keydown', key, true); back.remove(); if (prev && prev.focus) prev.focus(); resolve(v); };
    const key = (e) => {
      if (e.key === 'Escape') { e.preventDefault(); if (!busy) done(-1); }
      else if (e.key === 'Tab') { e.preventDefault(); const i = btns.indexOf(document.activeElement); btns[(i + (e.shiftKey ? btns.length - 1 : 1)) % btns.length].focus(); }
    };
    document.addEventListener('keydown', key, true);
    back.onmousedown = (e) => { if (e.target === back && !busy) done(-1); };
    document.body.append(back);
    btns[opt.focus || 0].focus();
  });
  // resolves once no dialog is open
  const noDialog = async () => { while (document.querySelector('.dlg-back')) await new Promise((r) => setTimeout(r, 300)); };
  // a yes/no question (Cancel first); with cancel set to null it is a message with a single button
  const ask = async (title, text, yes, danger, cancel = 'Cancel') => {
    const ok = [yes, danger ? 'danger-fill' : 'primary'];
    if (!cancel) { await modal(title, text, [ok]); return true; }
    return (await modal(title, text, [[cancel], ok])) === 1;
  };

  // the changelog: the newest version's changes, or every version's
  async function showChangelog(all) {
    const log = (model && model.changelog) || [];
    if (!log.length) return;
    const list = all ? log : log.slice(0, 1);
    const box = el('div', { className: 'chg' });
    list.forEach((v) => {
      if (list.length > 1) box.append(el('h3', { textContent: 'Version ' + v.version + ' · ' + v.date }));
      box.append(el('ul', {}, ...v.items.map((t) => el('li', { textContent: t }))));
    });
    const buttons = all || log.length < 2 ? [['Close', 'primary']] : [['Close', 'primary'], ['Earlier versions']];
    if ((await modal(all ? 'Changelog' : "What's new in " + log[0].version, box, buttons, true)) === 1) showChangelog(true);
  }

  // the counts on the Help tab (kept on this computer only)
  const dur = (ms) => { const m = Math.floor(ms / 60000), h = Math.floor(m / 60); return h ? h + ' h ' + (m % 60) + ' min' : m + ' min'; };
  function paintStats(s) {
    if (!s) return;
    const since = new Date(s.since + 'T00:00:00').toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' });
    const by = Object.entries(s.players || {}).sort((a, b) => b[1] - a[1]).map(([p, n]) => p + ' ' + n).join(', ');
    $('statsText').textContent = s.videos
      ? 'Since ' + since + ': ' + s.videos + (s.videos === 1 ? ' video' : ' videos') + ' shown on Discord, ' + s.matched + ' found in a catalog, ' + dur(s.watchedMs) + ' of playing.' + (by ? ' By player: ' + by + '.' : '')
      : 'Nothing shown on Discord yet (counting since ' + since + ').';
  }
  const autoStartName = () => (osText() ? osText().autoStart : 'Start with Windows');
  function applyOsText() {
    const t = osText();
    if (!t) return;
    $('playerHelpText').innerHTML = t.help;
    $('autoStartLabel').textContent = t.autoStart;
    if (model.tray) { // the macOS app has a menu bar icon
      $('autoStartHelp').textContent = 'Starts in the menu bar, without a window.';
      $('openWindowHelp').textContent = 'Turn off to go straight to the menu bar. Reopen the window from its icon.';
    } else {
      $('autoStartHelp').textContent = 'Starts in the background, without a window.';
      $('openWindowHelp').textContent = 'Turn off to start in the background. Open the app again to show this window.';
    }
  }

  // Every control saves on its own, one at a time and only its own setting: a toggle or a choice as soon as it changes,
  // a text box when it loses focus (or on Enter). A value that is refused goes back to what was saved.
  // "get" reads the control's value and "set" writes a value back into it (used when a value is refused).
  // an object setting (the custom text lines) is the same when its non-empty values are
  const flat = (o) => JSON.stringify(Object.keys(o || {}).sort().filter((k) => o[k] !== '' && o[k] != null).map((k) => [k, o[k]]));
  const isObj = (x) => !!x && typeof x === 'object' && !Array.isArray(x);
  const same = (a, b) => (Array.isArray(a) || Array.isArray(b) ? JSON.stringify([...(a || [])].sort()) === JSON.stringify([...(b || [])].sort())
    : isObj(a) || isObj(b) ? flat(a) === flat(b) : String(a) === String(b == null ? '' : b));
  function saver(scope, key, get, set) {
    return async () => {
      const value = get();
      const saved = scope === 'app' ? appValue(key) : model.values[key];
      if (same(value, saved)) return;
      note('Saving…');
      saving++;
      try {
        const m = await api('save', scope === 'app' ? { app: { [key]: value } } : { settings: { [key]: value } });
        model = m;
        set(scope === 'app' ? appValue(key) : m.values[key]);
        note('Saved');
      } catch (e) {
        set(saved);
        note(e.message, true);
      } finally {
        saving--;
      }
    };
  }
  function autosave(input, scope, key) {
    const run = saver(scope, key, () => readValue(input), (v) => showValue(input, v));
    input.addEventListener('change', () => saveLater(run));
    if (input.tagName === 'INPUT' && input.type !== 'checkbox') input.addEventListener('keydown', (e) => { if (e.key === 'Enter') input.blur(); });
  }

  // One control per setting. sources: a switch per online service (the stored value is the list that is OFF);
  // lines: one entry per line (stored as a list); the rest save through autosave.
  // the visible name of a control: a <label> tied to it by id, and its help text as the description
  const nameOf = (f, input, helpEl) => {
    input.id = 'f-' + f.key;
    if (helpEl) { helpEl.id = 'fh-' + f.key; input.setAttribute('aria-describedby', helpEl.id); }
    return el('label', { className: 'name', htmlFor: input.id, textContent: f.label });
  };
  function fieldFor(f, v) {
    const wrap = el('div', { className: 'field' });
    const helpEl = f.help ? (f.type === 'sources' ? help(f.help) : longHelp(f.help)) : null, h = helpEl ? [helpEl] : [];
    if (f.type === 'sources') {
      const off = new Set(v || []);
      const boxes = f.sources.map((s) => { const cb = el('input', { type: 'checkbox', checked: !off.has(s.id) }); cb.dataset.id = s.id; return cb; });
      const list = el('div', { className: 'sources' }, ...f.sources.map((s, i) => switchRow(s.label, boxes[i], s.used + ' (' + s.host + ')', 'src')));
      const name = el('div', { textContent: f.label, id: 'fl-' + f.key });
      list.setAttribute('role', 'group'); list.setAttribute('aria-labelledby', name.id);
      wrap.append(name, ...h, list);
      const run = saver('settings', f.key, () => boxes.filter((c) => !c.checked).map((c) => c.dataset.id), (ids) => { const o = new Set(ids || []); boxes.forEach((c) => { c.checked = !o.has(c.dataset.id); }); });
      boxes.forEach((c) => c.addEventListener('change', () => saveLater(run)));
      return wrap;
    }
    if (f.type === 'lines') {
      const ta = el('textarea', { value: (v || []).join('\n'), spellcheck: false, placeholder: 'One per line, e.g. Private' });
      ta.dataset.key = f.key;
      wrap.append(nameOf(f, ta, helpEl), el('div', { className: 'ctl' }, ta), ...h);
      const run = saver('settings', f.key, () => ta.value.split(/\r?\n/).map((x) => x.trim()).filter(Boolean), (arr) => { ta.value = (arr || []).join('\n'); });
      ta.addEventListener('change', () => saveLater(run));
      return wrap;
    }
    if (f.type === 'textlines') return customLinesBox(f, v);
    if (f.type === 'fixes') return fixesBox(f);
    let input;
    if (f.type === 'bool') {
      input = el('input', { type: 'checkbox', checked: !!v });
      wrap.append(switchRow(f.label, input, f.help));
    } else if (f.type === 'map') {
      // several lines: the box goes under its name
      input = el('textarea', { value: v || '', spellcheck: false });
      wrap.append(nameOf(f, input, helpEl), el('div', { className: 'ctl' }, input), ...h);
    } else {
      // a choice, a number or a line of text: the name on the left, the control on the right (under it in a narrow row)
      if (f.type === 'choice') { input = el('select'); f.options.forEach(([val, text]) => input.append(new Option(text, val, false, val === v))); }
      else if (f.type === 'int') input = el('input', { type: 'number', value: v, min: f.min, max: f.max });
      else input = el('input', { type: f.type === 'secret' ? 'password' : 'text', value: v || '', spellcheck: false, autocomplete: 'off' });
      wrap.append(el('div', { className: 'line' }, el('div', { className: 'lbl' }, nameOf(f, input, helpEl), ...h), el('div', { className: 'ctl-r' }, input)));
    }
    input.dataset.key = f.key;
    autosave(input, 'settings', f.key);
    return wrap;
  }

  // What the window adds under some group headings (sign-ins, buttons); the settings themselves come from the schema.
  const EXTRAS = {
    'players/Stremio': () => [stremioBox()],
    'players/Jellyfin / Emby': () => [jellyfinBox()],
    'players/On this computer': () => [playerSetupBox()],
    'advanced/Maintenance': () => [maintenanceBox()],
  };

  function renderFields(m) {
    document.querySelectorAll('.panel.dyn').forEach((p) => p.remove());
    for (const sec of m.schema) {
      const d = el('section', { className: 'panel dyn', id: 'p-' + sec.id });
      let group = null;
      const newGroup = (title) => { if (title) d.append(el('h3', { className: 'grp', textContent: title })); group = el('div', { className: 'group' }); d.append(group); };
      // a group that starts closed: reading, not settings
      const newFold = (title) => { group = el('details', { className: 'group fold' }, el('summary', { textContent: title })); d.append(group); };
      for (const f of sec.fields) {
        if (f.type === 'heading') {
          newGroup(f.label);
          const extra = EXTRAS[sec.id + '/' + f.label];
          if (extra) group.append(...extra());
          continue;
        }
        if (!group) newGroup('');
        if (f.key === 'plexAddress') {
          // the sign-in comes first in the Plex group; the address is rarely needed, so it is one click away (open
          // when it has a value)
          group.append(plexBox(), el('div', { className: 'field' }, el('details', { className: 'fold', open: !!m.values[f.key] }, el('summary', { textContent: 'Server address' }), fieldFor(f, m.values[f.key]))));
          continue;
        }
        group.append(fieldFor(f, m.values[f.key]));
      }
      if (sec.id === 'look') textLinesFollow(d);
      if (sec.id === 'privacy') { group.append(coverCacheBox()); dimServices(d); newFold('What else leaves ' + (osText() ? osText().here : 'this PC')); group.append(leavesBox()); newFold('Recent connections'); group.append(connsBox()); }
      d.setAttribute('role', 'tabpanel');
      $('p-help').before(d);
    }
    buildTabs();
  }

  // With "Look up cover art and titles online" off, the list of services below it is dimmed and cannot be changed
  // (it keeps its values).
  function dimServices(panel) {
    const main = panel.querySelector('input[data-key="showArtwork"]'), list = panel.querySelector('.sources');
    if (!main || !list) return;
    const apply = () => {
      list.classList.toggle('off', !main.checked);
      list.querySelectorAll('input').forEach((c) => { c.disabled = !main.checked; });
    };
    apply();
    // queued behind the save of the switch itself, so it also sees a value that was refused and put back
    main.addEventListener('change', () => later(apply));
  }

  // ---- Text lines (Look tab) ----------------------------------------------------------------------------------------
  // Custom's five lines in two groups, each with its placeholders to insert and a sample drawn like the preview. A
  // line saves when it loses focus or after a second's pause in typing, not at each key: each save sends the status.
  let customFields = null; // the custom lines' controls while the Look tab is drawn
  function customLinesBox(f, v) {
    const L = MPRP_LINES, vals = v || {}, inputs = {}, draws = [];
    const wrap = el('div', { className: 'field tl-custom' });
    let timer = null;
    const draw = () => draws.forEach((d) => d());
    const read = () => { const o = {}; L.KEYS.forEach((k) => { const s = inputs[k].value.trim(); if (s) o[k] = s; }); return o; };
    const show = (o) => { L.KEYS.forEach((k) => { inputs[k].value = (o && o[k]) || ''; }); draw(); };
    const run = saver('settings', f.key, read, show);
    const save = () => { clearTimeout(timer); saveLater(run); };
    const soon = () => { draw(); clearTimeout(timer); timer = setTimeout(save, 1000); };
    const groups = [
      ['episode', 'Episodes', [['episodeDetails', 'First line'], ['episodeState', 'Second line']]],
      ['movie', 'Films', [['movieDetails', 'First line'], ['movieState', 'Second line'], ['movieCover', 'Cover text']]],
    ];
    for (const [kind, title, lines] of groups) {
      const box = el('div', { className: 'tl-grp' }, el('div', { className: 'tl-h', textContent: title }));
      let target = null; // the field a placeholder goes into: the last one used in this group
      for (const [key, label] of lines) {
        const i = el('input', { type: 'text', value: vals[key] || '', spellcheck: false, autocomplete: 'off' });
        i.id = 'f-' + f.key + '-' + key;
        i.dataset.line = key;
        if (key === 'movieCover') i.placeholder = L.COVER;
        inputs[key] = i;
        if (!target) target = i;
        i.addEventListener('input', soon);
        i.addEventListener('change', save);
        i.addEventListener('keydown', (e) => { if (e.key === 'Enter') i.blur(); });
        i.addEventListener('focus', () => { target = i; });
        box.append(el('div', { className: 'tl-line' }, el('label', { htmlFor: i.id, textContent: label }), i));
      }
      const chips = el('div', { className: 'tl-chips' });
      chips.setAttribute('role', 'group'); chips.setAttribute('aria-label', title + ': placeholders');
      L.CHIPS[kind].forEach((k) => {
        const b = el('button', { type: 'button', className: 'chip', textContent: '{' + k + '}' });
        b.addEventListener('mousedown', (e) => e.preventDefault()); // the field keeps the focus and the cursor
        b.onclick = () => {
          const t = target, pos = t.selectionStart == null ? t.value.length : t.selectionStart;
          const before = t.value.slice(0, pos), after = t.value.slice(pos);
          const ins = (before.trim() && !before.endsWith(L.SEP) ? L.SEP : '') + '{' + k + '}';
          t.value = before + ins + after;
          t.focus();
          t.selectionStart = t.selectionEnd = (before + ins).length;
          soon();
        };
        chips.append(b);
      });
      const bad = el('div', { className: 'help tl-bad' });
      const l1 = el('div', { className: 'tl-l1' }), l2 = el('div', { className: 'tl-l2' });
      const sample = el('div', { className: 'tl-sample' }, el('div', { className: 'tl-cover' + (kind === 'movie' ? ' film' : '') }),
        el('div', { className: 'pv-text' }, el('div', { className: 'pv-name', textContent: 'Watching ' + L.SAMPLE[kind].title }), l1, l2));
      const coverNote = kind === 'movie' ? el('div', { className: 'help tl-covertext' }) : null;
      box.append(chips, bad, sample, ...(coverNote ? [coverNote] : []));
      draws.push(() => {
        const sv = L.SAMPLE[kind], keys = lines.map(([k]) => k);
        l1.textContent = L.expand(inputs[keys[0]].value, sv);
        l2.textContent = L.expand(inputs[keys[1]].value, sv);
        bad.textContent = L.unknown(keys.map((k) => inputs[k].value), sv);
        if (coverNote) coverNote.textContent = 'Shown when you point at the cover: ' + L.expand(inputs.movieCover.value.trim() || L.COVER, sv);
      });
      wrap.append(box);
    }
    wrap.append(help('Parts in [ ] drop out when a value is missing. Discord shows up to 128 characters a line.'));
    customFields = { read, show, save };
    draw();
    return wrap;
  }
  // The Text lines choice: Custom shows its five lines (filled with the lines of the choice that was on before, while
  // they are empty), and "Also show S01E02 in the title line" belongs to Standard only.
  function textLinesFollow(panel) {
    const sel = panel.querySelector('select[data-key="textLines"]'), custom = panel.querySelector('.tl-custom');
    if (!sel || !custom) return;
    const ep = panel.querySelector('input[data-key="episodeInDetails"]'), epField = ep && ep.closest('.field');
    let prev = sel.value === 'custom' ? 'standard' : sel.value;
    const apply = () => {
      custom.classList.toggle('hidden', sel.value !== 'custom');
      if (epField) epField.classList.toggle('hidden', sel.value !== 'standard');
    };
    apply();
    sel.addEventListener('change', () => {
      if (sel.value === 'custom' && customFields && !Object.keys(customFields.read()).length) {
        const p = MPRP_LINES.PRESETS[prev] || MPRP_LINES.PRESETS.standard, o = {};
        MPRP_LINES.KEYS.forEach((k, i) => { if (p[i]) o[k] = p[i]; });
        customFields.show(o);
        customFields.save();
      }
      if (sel.value !== 'custom') prev = sel.value;
      apply();
      later(apply); // after the save: a refused choice is put back
    });
  }

  // ---- Fix a match (Titles & art, and the preview's "Wrong title or cover?") ----------------------------------------
  let fixList = null, lastMatch = null; // the list while Titles & art is drawn; what plays, as the status says (Status.match)
  const fixesNow = () => (model && Array.isArray(model.values.matchFixes) ? model.values.matchFixes : []);
  const kindName = (k) => (k === 'movie' ? 'Movie' : 'Show');
  const entryLine = (r) => [r.year, kindName(r.kind), r.catalog].filter(Boolean).join(' · ');
  const entryLabel = (r) => r.name + (r.year ? ' (' + r.year + ')' : '') + ' · ' + kindName(r.kind) + ' · ' + r.catalog;
  const signed = (n) => (n > 0 ? '+' + n : String(n));
  const sameFix = (a, b) => !!a.file === !!b.file && a.match.trim().toLowerCase() === b.match.trim().toLowerCase();
  // the list with fx in place of the fix for the same title or file (or added at the end)
  const withFix = (list, fx) => { const i = list.findIndex((x) => sameFix(x, fx)); const out = list.filter((x) => !sameFix(x, fx)); out.splice(i < 0 ? out.length : i, 0, fx); return out; };
  const fixFor = (m) => fixesNow().find((x) => (x.file ? m.file && x.match.toLowerCase() === m.file.toLowerCase() : false))
    || fixesNow().find((x) => !x.file && x.match.trim().toLowerCase() === (m.title || '').trim().toLowerCase());
  // what a fix does, in the list: "Only this file · Your image · Sample Show (2019) · Show · IMDb · episodes +12"
  function fixMeta(fx) {
    const parts = [];
    if (fx.file) parts.push('Only this file');
    if (fx.cover) parts.push('Your image');
    if (fx.entry) parts.push(fx.label || fx.entry);
    else if (fx.search) parts.push('Searched as "' + fx.search + '"');
    if (fx.seasonOffset) parts.push('seasons ' + signed(fx.seasonOffset));
    if (fx.episodeOffset) parts.push('episodes ' + signed(fx.episodeOffset));
    return parts.join(' · ');
  }
  const thumb = (src, cls) => {
    if (!src) return el('span', { className: 'sq' + (cls ? ' ' + cls : '') });
    const i = el('img', { alt: '', src, className: cls || '' });
    i.onerror = () => { i.replaceWith(el('span', { className: 'sq' + (cls ? ' ' + cls : '') })); };
    return i;
  };
  function fixesBox() {
    fixList = el('div', { className: 'fixes', id: 'fixes' });
    drawFixes();
    const add = el('button', { textContent: 'Add a fix' });
    add.onclick = () => openFix({ add: true });
    return el('div', { className: 'field' }, fixList, el('div', { className: 'row actions' }, add, el('span', { className: 'help', textContent: 'A fix applies to every file read as that title, or to one file.' })));
  }
  function drawFixes() {
    if (!fixList) return;
    fixList.textContent = '';
    const list = fixesNow();
    if (!list.length) {
      fixList.append(el('div', { className: 'help fix-empty' }, document.createTextNode('No fixes yet. When the card shows the wrong title or cover, choose '), el('b', { textContent: 'Wrong title or cover?' }), document.createTextNode(' under it.')));
      return;
    }
    list.forEach((fx) => {
      const edit = el('button', { className: 'quiet', textContent: 'Edit' }), rm = el('button', { className: 'quiet', textContent: 'Remove' });
      edit.setAttribute('aria-label', 'Edit the fix for ' + fx.match); rm.setAttribute('aria-label', 'Remove the fix for ' + fx.match);
      edit.onclick = () => openFix({ fix: fx });
      rm.onclick = async () => {
        const before = fixesNow();
        if (await saveFixes(before.filter((x) => !sameFix(x, fx)))) noteUndo('Fix removed.', () => saveFixes(before));
      };
      fixList.append(el('div', { className: 'fix-row' }, thumb(fx.cover || fx.poster),
        el('div', { className: 't' }, el('div', { className: 'n', textContent: fx.match }), el('div', { className: 'help', textContent: fixMeta(fx) })), edit, rm));
    });
  }
  // saves Fix a match's list, in turn like every change, and draws it again; resolves to whether it was saved
  const saveFixes = (next) => new Promise((resolve) => later(async () => {
    note('Saving…');
    saving++;
    try { model = await api('save', { settings: { matchFixes: next } }); note('Saved'); resolve(true); } catch (e) { note(e.message, true); resolve(false); } finally { saving--; drawFixes(); }
  }));
  // a line in the bottom bar with an Undo button, for a few seconds
  function noteUndo(text, undo) {
    clearTimeout(clearTimer);
    const m = $('msg'), b = el('button', { className: 'link', textContent: 'Undo' });
    m.className = 'msg'; m.textContent = '';
    b.onclick = () => { m.textContent = ''; undo(); };
    m.append(document.createTextNode(text + ' '), b);
    clearTimer = setTimeout(() => { m.textContent = ''; }, 8000);
  }
  // a dialog with content of its own (modal() is for a question with buttons). Escape and a click outside close it;
  // Tab stays inside it.
  function sheet(title) {
    const prev = document.activeElement, uid = 'dlg' + (++modalCount);
    const h = el('h2', { textContent: title, id: uid + '-title' }), body = el('div');
    const box = el('div', { className: 'dlg wide sheet' }, h, body);
    box.setAttribute('role', 'dialog'); box.setAttribute('aria-modal', 'true'); box.setAttribute('aria-labelledby', h.id);
    const back = el('div', { className: 'dlg-back' }, box);
    const top = () => [...document.querySelectorAll('.dlg-back')].pop() === back;
    const close = () => { document.removeEventListener('keydown', key, true); back.remove(); if (prev && prev.focus) prev.focus(); };
    const key = (e) => {
      if (!top()) return;
      if (e.key === 'Escape') { e.preventDefault(); close(); return; }
      if (e.key !== 'Tab') return;
      const all = [...box.querySelectorAll('button, input, select, textarea, summary, li[tabindex]')].filter((x) => !x.disabled && x.getClientRects().length);
      if (!all.length) return;
      const i = all.indexOf(document.activeElement);
      if (e.shiftKey ? i <= 0 : i === all.length - 1) { e.preventDefault(); all[e.shiftKey ? all.length - 1 : 0].focus(); }
    };
    document.addEventListener('keydown', key, true);
    back.onmousedown = (e) => { if (e.target === back) close(); };
    document.body.append(back);
    return { box, h, body, close };
  }
  // Fix this match. ctx.match: what plays (Status.match); ctx.fix: a fix to edit; ctx.add: a fix for a title that is not
  // playing. Step 1 searches the catalogs and saves the pick; step 2 (fixed) offers the cover and the offsets.
  function openFix(ctx) {
    const playing = ctx.match || null;
    const fix = ctx.fix || (playing ? fixFor(playing) : null) || null;
    const s = sheet('Fix this match');
    let cats = [], sel = null, n = 0, timer = null, scopeFile = !!(fix && fix.file);
    const from = el('p', { className: 'from' });
    const b = (t) => el('b', { textContent: t }), tx = (t) => document.createTextNode(t);
    let readIn = null;
    if (playing) {
      const nums = (playing.season ? ', season ' + playing.season : '') + (playing.episode ? ', episode ' + playing.episode : '');
      from.append(tx('From '), b(playing.file || 'this video'), tx(', read as '), b(playing.title), tx(nums + '. '), tx(playing.now ? 'Now matched to ' + playing.now + '.' : 'Nothing matched it yet.'));
    } else if (fix) {
      from.append(...(fix.file ? [tx('For the file '), b(fix.match), tx('.')] : [tx('For every file read as '), b(fix.match), tx('.')]));
    } else {
      readIn = el('input', { type: 'text', spellcheck: false, autocomplete: 'off', placeholder: 'Sample Show' });
      readIn.id = 'fixRead';
      from.append(el('label', { htmlFor: readIn.id, textContent: 'The title as it is read from the file name (the preview shows it while it plays):' }), readIn);
    }
    const q = el('input', { type: 'text', spellcheck: false, autocomplete: 'off', value: (fix && fix.search) || (playing ? playing.title : fix ? (fix.label ? fix.label.split(' · ')[0].replace(/ \(\d{4}\)$/, '') : fix.match) : '') });
    q.setAttribute('aria-label', 'Search the catalogs');
    const qHelp = help('');
    const res = el('ul', { className: 'res' });
    res.setAttribute('role', 'listbox'); res.setAttribute('aria-label', 'Results');
    const use = el('button', { className: 'primary', textContent: 'Use this', disabled: true });
    const cancel = el('button', { className: 'quiet', textContent: 'Cancel' });
    cancel.onclick = s.close;
    let scope = el('span', { className: 'grow' });
    if (playing && playing.file) {
      const name = 'scope' + modalCount;
      const r1 = el('input', { type: 'radio', name, checked: !scopeFile }), r2 = el('input', { type: 'radio', name, checked: scopeFile });
      r1.onchange = () => { scopeFile = false; }; r2.onchange = () => { scopeFile = true; };
      scope = el('div', { className: 'scope' }, el('label', {}, r1, el('span', {}, tx('Every file read as '), b(playing.title))), el('label', {}, r2, el('span', {}, tx('Only this file ('), b(playing.file), tx(')'))));
      scope.setAttribute('role', 'radiogroup'); scope.setAttribute('aria-label', 'Apply the fix to');
    }
    s.body.append(from, q, qHelp, res, el('div', { className: 'foot' }, scope, cancel, use));
    const nowEntry = playing && playing.nowEntry ? playing.nowEntry : fix && fix.entry ? fix.entry : '';
    const rows = () => [...res.querySelectorAll('li[role="option"]')];
    const choose = (li, r) => {
      rows().forEach((x) => x.setAttribute('aria-selected', 'false'));
      li.setAttribute('aria-selected', 'true');
      sel = r; use.disabled = !!r.now;
    };
    const row = (li, r) => {
      li.setAttribute('role', 'option'); li.setAttribute('aria-selected', 'false'); li.tabIndex = -1;
      li.onclick = () => choose(li, r);
      li.ondblclick = () => { choose(li, r); if (!use.disabled) use.click(); };
      res.append(li);
    };
    function render(list, err) {
      res.textContent = ''; sel = null; use.disabled = true;
      qHelp.textContent = (cats.length ? 'Searches ' + cats.join(', ').replace(/, ([^,]*)$/, ' and $1') + ' (the catalogs on under Privacy).' : 'All catalogs are off (Privacy).') + (err ? ' ' + err : '');
      const term = q.value.trim();
      if (cats.length && term.length >= 2 && !list.length) res.append(el('li', { className: 'wait', textContent: err ? 'No answer.' : 'Nothing found. Try another spelling.' }));
      list.forEach((r) => {
        const now = !!nowEntry && r.entry === nowEntry;
        row(el('li', {}, thumb(r.poster), el('div', { className: 't' }, el('div', { className: 'n', textContent: r.name }), el('div', { className: 'm', textContent: entryLine(r) })),
          ...(now ? [el('span', { className: 'now', textContent: 'Now' })] : [])), Object.assign({}, r, { now }));
      });
      if (term) row(el('li', { className: 'as' }, el('span', { className: 'sq' }), el('div', { className: 't', textContent: 'Search as "' + term + '" instead (no entry pinned)' })), { search: term });
    }
    async function search(first) {
      clearTimeout(timer);
      const term = q.value.trim(), my = ++n;
      if (!first && term.length < 2) { render([], ''); return; }
      res.textContent = '';
      res.append(el('li', { className: 'wait', textContent: 'Searching…' }));
      try {
        const r = await api('match-search', { q: term, n: my });
        if (r.n !== my || !s.box.isConnected) return; // an answer to an older keystroke
        cats = r.catalogs || [];
        render(r.results || [], r.error || '');
      } catch (e) { if (my === n) render([], e.message); }
    }
    q.addEventListener('input', () => { clearTimeout(timer); timer = setTimeout(() => search(false), 350); });
    q.addEventListener('keydown', (e) => {
      const all = rows(), i = all.findIndex((x) => x.getAttribute('aria-selected') === 'true');
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        e.preventDefault();
        const to = all[Math.max(0, Math.min(all.length - 1, i + (e.key === 'ArrowDown' ? 1 : -1)))];
        if (to) { to.click(); to.scrollIntoView({ block: 'nearest' }); }
      } else if (e.key === 'Enter' && !use.disabled) use.click();
    });
    use.onclick = async () => {
      if (!sel || sel.now) return;
      let match, file = false;
      if (readIn) match = readIn.value.trim();
      else if (playing && playing.file && scopeFile) { match = playing.file; file = true; }
      else if (playing) match = playing.title;
      else { match = fix.match; file = !!fix.file; }
      if (!match) { qHelp.textContent = 'Type the title as it is read from the file name first.'; readIn.focus(); return; }
      const fx = { match };
      if (file) fx.file = true;
      if (sel.entry) { fx.entry = sel.entry; fx.label = entryLabel(sel); if (sel.poster) fx.poster = sel.poster; } else fx.search = sel.search;
      if (fix && sameFix(fix, fx)) ['cover', 'seasonOffset', 'episodeOffset'].forEach((k) => { if (fix[k]) fx[k] = fix[k]; });
      const before = fixesNow();
      let next = fix && !sameFix(fix, fx) && !playing ? before.filter((x) => !sameFix(x, fix)) : before;
      next = withFix(next, fx);
      use.disabled = true;
      if (await saveFixes(next)) fixed(fx, before, sel); else use.disabled = false;
    };
    // step 2: the fix is saved; its cover and its offsets
    function fixed(fx, before, picked) {
      s.h.textContent = 'Fixed';
      s.body.textContent = '';
      const name = picked.entry ? picked.name : 'Searched as "' + fx.search + '"';
      const head = el('div', { className: 'done-head' }, thumb(fx.cover || fx.poster, 'big'), el('div', {}, el('div', { className: 'n', textContent: name }),
        help(picked.entry ? entryLine(picked) : 'No entry pinned'), help(picked.entry ? 'The card shows it in a moment. Episode titles come from this entry.' : 'The card shows it in a moment.')));
      const group = 'cov' + modalCount;
      const rCat = el('input', { type: 'radio', name: group, checked: !fx.cover, id: group + '-cat' });
      const rOwn = el('input', { type: 'radio', name: group, id: group + '-own' });
      const own = el('input', { type: 'text', value: fx.cover || '', placeholder: 'https://example.com/poster.jpg', spellcheck: false, autocomplete: 'off' });
      own.setAttribute('aria-label', 'Image address');
      const covers = el('div', { className: 'covers' }), capt = help('Covers of this title from the other catalogs on under Privacy:');
      capt.classList.add('hidden');
      let picks = [];
      const pick = (url) => { picks.forEach((x) => x.classList.toggle('sel', x.dataset.url === url)); };
      rCat.onchange = () => pick(''); rOwn.onchange = () => pick('');
      own.addEventListener('focus', () => { rOwn.checked = true; pick(''); });
      if (fx.cover) rOwn.checked = true;
      const bad = el('div', { className: 'help bad hidden' });
      const sOff = el('input', { type: 'number', value: fx.seasonOffset || 0, min: -500, max: 500 }), eOff = el('input', { type: 'number', value: fx.episodeOffset || 0, min: -500, max: 500 });
      sOff.setAttribute('aria-label', 'Season offset'); eOff.setAttribute('aria-label', 'Episode offset');
      const fold = el('details', { className: 'fold', open: !!(fx.seasonOffset || fx.episodeOffset) }, el('summary', { textContent: "Episodes don't line up?" }),
        el('div', { className: 'offs' }, tx('Season '), sOff, tx(' episode '), eOff),
        help('Added to the numbers read from the file before the catalogs are asked (0 and 0: no change). Example: a file numbered S01E02 with +12 episodes is looked up as S01E14.'));
      const undo = el('button', { className: 'link', textContent: 'Undo' }), tell = el('button', { className: 'link', textContent: 'Tell the project about this' });
      const done = el('button', { className: 'primary', textContent: 'Done' });
      s.body.append(head,
        el('div', { className: 'opt' }, rCat, el('label', { htmlFor: rCat.id, textContent: picked.entry ? "This entry's cover" : 'The cover the search finds' })),
        capt, covers,
        el('div', { className: 'opt' }, rOwn, el('label', { htmlFor: rOwn.id, textContent: 'An image address' }), own),
        bad, fold, el('div', { className: 'foot' }, undo, el('span', { className: 'help', textContent: '·' }), tell, el('span', { className: 'grow' }), done));
      if (picked.entry) {
        api('match-covers', { name: picked.name, year: picked.year || 0, kind: picked.kind, catalog: picked.catalog, n: 1 }).then((r) => {
          (r.covers || []).forEach((c) => {
            const i = el('img', { alt: 'Cover from ' + c.catalog, src: c.url, title: c.catalog, tabIndex: 0 });
            i.dataset.url = c.url;
            const choose = () => { rCat.checked = false; rOwn.checked = false; pick(c.url); };
            i.onclick = choose;
            i.onkeydown = (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); choose(); } };
            i.onerror = () => i.remove();
            picks.push(i); covers.append(i);
          });
          capt.classList.toggle('hidden', !picks.length);
          if (fx.cover) pick(fx.cover);
        }).catch(() => {});
      }
      const current = () => {
        const chosen = picks.find((x) => x.classList.contains('sel'));
        const cover = rCat.checked ? '' : rOwn.checked ? own.value.trim() : chosen ? chosen.dataset.url : '';
        const out = Object.assign({}, fx);
        ['cover', 'seasonOffset', 'episodeOffset'].forEach((k) => { delete out[k]; });
        if (cover) out.cover = cover;
        const so = Math.trunc(Number(sOff.value) || 0), eo = Math.trunc(Number(eOff.value) || 0);
        if (so) out.seasonOffset = so;
        if (eo) out.episodeOffset = eo;
        return out;
      };
      undo.onclick = async () => { if (await saveFixes(before)) { s.close(); note('Fix undone.'); } };
      tell.onclick = () => tellProject(current(), playing);
      done.onclick = async () => {
        const out = current();
        if (out.cover && !/^https?:\/\/\S+$/.test(out.cover)) { bad.textContent = 'Enter a web address of an image (starting with https).'; bad.classList.remove('hidden'); own.focus(); return; }
        if (JSON.stringify(out) === JSON.stringify(fx)) { s.close(); return; }
        done.disabled = true;
        if (await saveFixes(withFix(fixesNow(), out))) s.close(); else done.disabled = false;
      };
      done.focus();
    }
    search(true);
    (readIn || q).focus();
  }
  // "Tell the project about this": the report is shown to be looked over, copied, and posted by the user on GitHub's
  // issue form (opened at its plain address: no file name goes into a web address). The program sends nothing.
  async function tellProject(fx, playing) {
    let text = '';
    try {
      text = (await api('match-report', { fix: fx, file: (playing && playing.file) || '', title: (playing && playing.title) || '', season: (playing && playing.season) || 0, episode: (playing && playing.episode) || 0, before: (playing && playing.now) || '' })).text;
    } catch (e) { note(e.message, true); return; }
    const ta = el('textarea', { value: text, spellcheck: false, className: 'report' });
    ta.setAttribute('aria-label', 'Report text');
    const body = el('div', {}, el('p', { textContent: 'If the program should have found this by itself, a report helps it learn the pattern. The program sends nothing: you copy the text, look it over, and post it on GitHub yourself.' }),
      ta, help("Folders, players' addresses and sign-ins are left out. You can edit the text before posting."));
    await modal('Tell the project about this', body, [['Cancel'], ['Copy and open GitHub', 'primary']], true, {
      focus: 1,
      hold: async (i) => {
        if (i !== 1) return true;
        let ok = false;
        try { await navigator.clipboard.writeText(ta.value); ok = true; } catch (_) { ta.focus(); ta.select(); try { ok = document.execCommand('copy'); } catch (_) {} }
        window.open('https://github.com/CptPakundo/MPRP/issues/new/choose', '_blank', 'noopener');
        note(ok ? 'Copied. Paste it into the new issue and look it over before you post it.' : 'Select the text and copy it, then paste it into the new issue.');
        return true;
      },
    });
  }

  // "Set up the player connection" at the top of the Players tab: the same call and confirmation as the card that
  // shows when no player is answering
  function playerSetupBox() {
    const msg = el('span', { className: 'help' });
    const btn = el('button', { textContent: 'Set up the player connection' });
    btn.onclick = () => setUpPlayers(btn, msg);
    return el('div', { className: 'field' }, el('div', { className: 'row' }, btn, msg),
      help('Switches on what each player needs. MPC-HC, MPC-BE and VLC are restarted after you confirm.'));
  }

  // the bottom of Advanced: the folder with the settings, and Restore defaults
  function maintenanceBox() {
    const folder = el('button', { textContent: 'Open settings folder' });
    folder.onclick = () => api('open-folder', {}).catch(() => {});
    const reset = el('button', { textContent: 'Restore defaults', className: 'danger' });
    reset.onclick = restoreDefaults;
    return el('div', { className: 'field' }, el('div', { className: 'row' }, folder, reset));
  }

  // the parts of the Privacy tab that are not settings: clearing the cover cache, and what else leaves this PC
  function coverCacheBox() {
    const msg = el('span', { className: 'help' });
    const btn = el('button', { textContent: 'Clear cover cache' });
    btn.onclick = () => later(async () => {
      btn.disabled = true; msg.textContent = '';
      try { await api('clear-cache', {}); msg.textContent = 'Cleared.'; note('Cover cache cleared.'); } catch (e) { msg.textContent = e.message; }
      btn.disabled = false;
    });
    return el('div', { className: 'field' }, el('div', { textContent: 'Cover cache' }),
      help('Remembers what was found for titles you watched, so they show up faster next time. Clearing it makes the next lookups ask the services again.'),
      el('div', { className: 'row actions' }, btn, msg));
  }
  function leavesBox() {
    const all = el('li', {}, document.createTextNode('Every address the program reaches is listed in '),
      el('a', { href: 'https://github.com/CptPakundo/MPRP/blob/master/SECURITY.md#where-it-connects', target: '_blank', rel: 'noopener', textContent: 'SECURITY.md' }),
      document.createTextNode('.'));
    // MPC-HC is Windows-only; its web interface answers every web page (no password, any origin)
    const mpc = osText() ? [] : ["MPC-HC's web interface has no password, and web pages you open can read it and control the player while it is on. Switch it off in MPC-HC when you stop using this program."];
    return el('div', { className: 'field' }, el('ul', { className: 'help leaves' }, ...[
      'Discord: your status. That is the point of the program.',
      'GitHub: asked about new versions while Check for updates is on.',
      'Plex: only while you are signed in. plex.tv is asked for your servers, and your server tells this program what you play.',
      'Stremio: only while you are signed in. It is asked every 20 seconds which library entries changed.',
      'Jellyfin or Emby: only while you are signed in. Your own server is asked every 5 seconds what plays.',
      'Kodi: only once its password is set (Players tab). It is asked at the address there, this computer unless you change it, and listened to on port 9090 there for its pause and play notifications.',
      "The preview at the top loads the cover image from the service that hosts it, or the player's logo from GitHub. Discord loads them the same way for your status.",
      'A file that is just an episode number uses its folder\'s name as the title when "Use the folder name" is on; that name is looked up like any title.',
      'The player (' + players() + ') and Discord are only reached on this computer. No analytics, your stats stay here, and file paths are never sent.',
      ...mpc,
    ].map((t) => el('li', { textContent: t })), all));
  }

  // every computer outside this one the program asked since it started: host, how often, when last (never what for)
  function connsBox() {
    return el('div', { className: 'field' },
      help('Since the program started: every address outside this computer it sent a request to, how often and when last. Never what it asked.'),
      el('ul', { className: 'help leaves', id: 'conns' }, el('li', { textContent: 'None yet.' })));
  }
  function ago(ms) {
    const s = Math.max(0, Math.round((Date.now() - ms) / 1000));
    if (s < 60) return 'just now';
    if (s < 3600) return Math.round(s / 60) + ' min ago';
    return 'at ' + new Date(ms).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }
  async function pollConns() {
    const box = $('conns');
    if (tab !== 'privacy' || !box || document.hidden || gone) return;
    try {
      const { hosts } = await api('connections');
      const rows = (hosts || []).map((h) => [h.host, ' (' + (h.label || 'an address you entered') + '): ' + h.count + (h.count === 1 ? ' time, ' : ' times, ') + 'last ' + ago(h.last) + '.']);
      const sig = rows.map((r) => r.join('')).join('\n');
      if (box.dataset.sig === sig) return; // unchanged: leave the list alone, so a selection in it survives
      box.dataset.sig = sig;
      box.textContent = '';
      if (!rows.length) { box.append(el('li', { textContent: 'None yet.' })); return; }
      rows.forEach(([host, rest]) => box.append(el('li', {}, el('b', { textContent: host }), document.createTextNode(rest))));
    } catch (_) {}
  }

  // A sign-in on the Players tab. The program keeps the sign-in: the page never sees a token or key. o.extra is an
  // element shown above the buttons, o.onShow(state) and o.onBusy(on) let it follow the box.
  function signInBox(name, o) {
    const text = help(''), err = el('div', { className: 'help bad hidden' });
    const btn = el('button');
    const again = el('button', { textContent: 'Open the sign-in page again', className: 'hidden' });
    const out = el('button', { textContent: 'Sign out', className: 'hidden' });
    const box = el('div', { className: 'field' }, text, err, ...(o.extra ? [o.extra] : []), el('div', { className: 'row actions' }, btn, again, out));
    let cur = null, timer = null;
    const fail = (m) => { err.textContent = m || ''; err.classList.toggle('hidden', !m); };
    const show = (p) => {
      cur = p;
      clearTimeout(timer);
      fail(p.problem);
      again.classList.toggle('hidden', !p.waiting);
      out.classList.toggle('hidden', !p.signedIn || p.waiting);
      btn.className = p.signedIn && !p.waiting ? 'hidden' : ''; // signing in is optional: a plain button
      if (p.waiting) {
        text.textContent = 'Approve the sign-in on the page that opened in your browser.';
        btn.textContent = 'Cancel';
        timer = setTimeout(refresh, 2000);
      } else if (p.signedIn) text.textContent = o.signedIn(p);
      else { text.textContent = o.intro; btn.textContent = 'Sign in to ' + o.service; }
      if (o.onShow) o.onShow(p);
    };
    const refresh = async () => { if (!box.isConnected) return; try { show(await api(name)); } catch (_) { timer = setTimeout(refresh, 4000); } };
    const busy = (on) => { btn.disabled = out.disabled = on; if (o.onBusy) o.onBusy(on); };
    const act = (call, body) => later(async () => { busy(true); try { show(await api(call, body || {})); } catch (e) { fail(e.message); } busy(false); });
    btn.onclick = () => act(name + (cur && cur.waiting ? '-cancel' : '-signin'));
    again.onclick = () => { if (cur && cur.url) window.open(cur.url, '_blank', 'noopener'); };
    out.onclick = async () => { if (await ask('Sign out of ' + o.service + '?', o.signedOut + ' You can sign in again at any time.', 'Sign out')) act(name + '-signout'); };
    setTimeout(refresh); // once the box is on the page (a box from an earlier render stops asking)
    return { box, act, fail };
  }

  // "Sign in to Plex", then the server to follow.
  function plexBox() {
    const sel = el('select', { className: 'hidden' });
    sel.setAttribute('aria-label', 'Plex server');
    let listed = false;
    const listServers = async (cur) => {
      listed = true;
      try {
        const { servers } = await api('plex-servers', {});
        sel.textContent = '';
        servers.forEach((s) => sel.append(new Option(s.name + (s.owned ? '' : ' (shared with you)'), s.id, false, s.id === cur.server)));
        if (!servers.length) sel.append(new Option('No Plex server in this account', '', true, true));
        sel.disabled = servers.length < 2;
      } catch (e) { listed = false; s.fail(e.message); }
    };
    const s = signInBox('plex', {
      service: 'Plex',
      intro: 'Shows what you play on Plex on any device. Signing in opens plex.tv; this app never sees your password.',
      signedIn: (p) => 'Signed in as ' + (p.user || 'your Plex account') + '. Server:',
      signedOut: 'Discord stops showing what you play on Plex.',
      extra: el('div', { className: 'ctl' }, sel),
      onShow: (p) => {
        const on = p.signedIn && !p.waiting;
        sel.classList.toggle('hidden', !on);
        if (!on) listed = false;
        else if (!listed) { sel.textContent = ''; sel.append(new Option(p.serverName || 'Loading…', p.server || '', true, true)); listServers(p); }
      },
      onBusy: (on) => { sel.disabled = on || sel.options.length < 2; },
    });
    sel.onchange = () => { if (sel.value) s.act('plex-server', { id: sel.value }); };
    return s.box;
  }

  function stremioBox() {
    return signInBox('stremio', {
      service: 'Stremio',
      intro: 'Shows what you watch in Stremio on any device. Signing in opens stremio.com; this app never sees your password.',
      signedIn: () => 'Signed in. Pausing takes the status down; it comes back when you resume.',
      signedOut: 'Discord stops showing what you watch in Stremio.',
    }).box;
  }

  // The Jellyfin / Emby sign-in: the server's address, then a Quick Connect code to approve in a Jellyfin app, or a user
  // name and password (Emby, or Quick Connect off). The password is passed on to the server once and never kept.
  function jellyfinBox() {
    const input = (type, placeholder) => { const i = el('input', { type, spellcheck: false, autocomplete: 'off', placeholder }); i.setAttribute('aria-label', placeholder); return i; };
    const text = help(''), err = el('div', { className: 'help bad hidden' });
    const addr = input('text', 'http://192.168.1.20:8096');
    addr.setAttribute('aria-label', 'Server address');
    const user = input('text', 'User name'), pass = input('password', 'Password');
    const code = el('div', { className: 'jf-code hidden' });
    const btn = el('button');
    const cancel = el('button', { textContent: 'Cancel', className: 'hidden' });
    const usePw = el('button', { textContent: 'Use a password instead', className: 'hidden' });
    const out = el('button', { textContent: 'Sign out', className: 'hidden' });
    const addrRow = el('div', { className: 'ctl' }, addr);
    const pwRow = el('div', { className: 'ctl jf-pw hidden' }, user, pass);
    const box = el('div', { className: 'field' }, text, err, addrRow, code, pwRow, el('div', { className: 'row actions' }, btn, usePw, cancel, out));
    let cur = null, timer = null, pwMode = false;
    const fail = (m) => { err.textContent = m || ''; err.classList.toggle('hidden', !m); };
    const show = (p) => {
      cur = p;
      clearTimeout(timer);
      fail(p.problem);
      const pw = !p.signedIn && !p.waiting && (p.password || pwMode);
      const name = (p.server ? '"' + p.server + '"' : 'your server') + (p.product ? ' (' + p.product + ')' : '');
      addrRow.classList.toggle('hidden', p.signedIn || p.waiting || pw);
      pwRow.classList.toggle('hidden', !pw);
      code.classList.toggle('hidden', !p.waiting);
      out.classList.toggle('hidden', !p.signedIn);
      btn.classList.toggle('hidden', p.signedIn || p.waiting);
      cancel.classList.toggle('hidden', !p.waiting && !pw);
      usePw.classList.toggle('hidden', !p.waiting);
      if (p.waiting) {
        code.textContent = p.code;
        text.textContent = 'Enter this code in Jellyfin: your profile > Quick Connect.';
        timer = setTimeout(refresh, 2000);
      } else if (p.signedIn) {
        text.textContent = 'Signed in as ' + (p.user || 'your user') + ' on ' + name + '.';
        pwMode = false;
      } else if (pw) {
        text.textContent = 'Sign in to ' + name + ' with your user name and password. The password goes to your server once and is not kept.';
        btn.textContent = 'Sign in';
        if (!user.value) setTimeout(() => user.focus());
      } else {
        text.textContent = 'Shows what you play on your Jellyfin or Emby server on any device. Enter the address you open it at.';
        btn.textContent = 'Sign in to Jellyfin / Emby';
        if (p.address && !addr.value) addr.value = p.address.replace(/\/emby$/, '');
      }
    };
    const refresh = async () => { if (!box.isConnected) return; try { show(await api('jellyfin')); } catch (_) { timer = setTimeout(refresh, 4000); } };
    const act = (name, body) => later(async () => {
      [btn, out, cancel, usePw].forEach((b) => { b.disabled = true; });
      fail('');
      try { show(await api(name, body || {})); } catch (e) { fail(e.message); }
      [btn, out, cancel, usePw].forEach((b) => { b.disabled = false; });
      pass.value = '';
    });
    const address = () => (cur && cur.address && (cur.password || pwMode) ? cur.address : addr.value);
    btn.onclick = () => {
      if (!pwRow.classList.contains('hidden')) act('jellyfin-password', { address: address(), user: user.value, password: pass.value });
      else act('jellyfin-signin', { address: addr.value });
    };
    [addr, user, pass].forEach((i) => i.addEventListener('keydown', (e) => { if (e.key === 'Enter') btn.click(); }));
    cancel.onclick = () => { pwMode = false; act('jellyfin-cancel'); };
    usePw.onclick = () => { const a = cur.address; act('jellyfin-cancel'); later(() => { pwMode = true; show(Object.assign({}, cur, { address: a, password: true })); }); };
    out.onclick = async () => { if (await ask('Sign out of ' + ((cur && cur.product) || 'Jellyfin') + '?', 'Discord stops showing what you play there. You can sign in again at any time.', 'Sign out')) act('jellyfin-signout'); };
    setTimeout(refresh); // once the box is on the page (a box from an earlier render stops asking)
    return box;
  }

  let tab = 'general';
  try { tab = localStorage.getItem('tab') || 'general'; } catch (_) {}
  let updFlag = false;
  function buildTabs() {
    const list = [['general', 'General']];
    model.schema.forEach((s) => list.push([s.id, s.title])); // the tab names come from store/schema.go
    list.push(['help', 'Help']);
    if (!list.some((x) => x[0] === tab)) tab = 'general';
    const bar = $('tabs'); bar.textContent = '';
    list.forEach(([id, label]) => {
      const b = el('button', { className: 'tab', textContent: label, id: 'tab-' + id });
      b.setAttribute('role', 'tab'); b.setAttribute('aria-controls', 'p-' + id); b.setAttribute('aria-selected', 'false'); b.dataset.tab = id;
      b.onclick = () => showTab(id);
      b.onkeydown = (e) => { // Left and Right (and Home, End) move between the tabs
        const all = [...bar.querySelectorAll('.tab')], i = all.indexOf(b);
        const to = e.key === 'ArrowRight' ? all[(i + 1) % all.length] : e.key === 'ArrowLeft' ? all[(i + all.length - 1) % all.length] : e.key === 'Home' ? all[0] : e.key === 'End' ? all[all.length - 1] : null;
        if (!to) return;
        e.preventDefault(); to.focus(); to.click();
      };
      bar.append(b);
    });
    document.querySelectorAll('.panel').forEach((p) => { if (!p.hasAttribute('aria-labelledby')) p.setAttribute('aria-labelledby', 'tab-' + p.id.slice(2)); });
    if (updFlag) flagUpdate();
    showTab(tab);
  }
  function showTab(id) {
    tab = id;
    try { localStorage.setItem('tab', id); } catch (_) {}
    document.querySelectorAll('.tab').forEach((b) => { const on = b.dataset.tab === id; b.setAttribute('aria-selected', String(on)); b.tabIndex = on ? 0 : -1; });
    document.querySelectorAll('.panel').forEach((p) => p.classList.toggle('active', p.id === 'p-' + id));
    if (id === 'help') pollLog();
    if (id === 'privacy') pollConns();
    if (id === 'help') api('stats').then(paintStats).catch(() => {});
  }
  function flagUpdate() { updFlag = true; const b = $('tab-general'); if (b) b.classList.add('has'); }

  function fill(m) {
    model = m;
    $('ver').textContent = 'Version ' + m.version;
    $('ver').setAttribute('aria-label', 'Changelog (version ' + m.version + ')');
    if (!$('updText').textContent) $('updText').textContent = 'Version ' + m.version; // until a check says more
    $('autoStart').checked = !!m.autoStart; $('startPresence').checked = !!m.app.startPresence; $('openWindow').checked = !!m.app.openWindow; $('minimizeToTray').checked = !!m.app.minimizeToTray;
    $('minimizeRow').classList.toggle('hidden', m.os !== 'windows'); // only Windows can tell a minimised window and has a tray for it
    $('checkUpdates').checked = !!m.app.checkUpdates;
    $('autoUpdate').checked = !!m.app.autoUpdate;
    $('autoUpdate').disabled = !m.app.checkUpdates; // installing needs the checks
    $('autoUpdateRow').classList.toggle('hidden', m.os === 'darwin'); // macOS: the app can't replace itself
    applyOsText();
    renderFields(m);
    paintStats(m.stats);
    $('welcome').classList.toggle('hidden', !!m.app.welcomeSeen);
  }
  ['autoStart', 'startPresence', 'openWindow', 'minimizeToTray', 'checkUpdates', 'autoUpdate'].forEach((id) => autosave($(id), 'app', id));
  $('checkUpdates').addEventListener('change', () => { $('autoUpdate').disabled = !$('checkUpdates').checked; });

  const mmss = (sec) => { sec = Math.max(0, Math.floor(sec)); const h = Math.floor(sec / 3600), m = Math.floor((sec % 3600) / 60), x = sec % 60; return (h ? h + ':' + String(m).padStart(2, '0') : m) + ':' + String(x).padStart(2, '0'); };
  // a picture of the Discord card, built from the activity the program actually sent
  function paintPreview(p) {
    const box = $('preview');
    box.classList.toggle('hidden', !p);
    if (!p) return;
    $('pvName').textContent = (p.listening ? 'Listening to ' : p.watching ? 'Watching ' : 'Playing ') + p.name;
    $('pvDetails').textContent = p.details || '';
    $('pvDetails').classList.toggle('pv-link', !!p.linked);
    $('pvDetails').title = p.linked ? 'Opens the catalog page on Discord (the cover too)' : '';
    $('pvState').textContent = p.state || '';
    const img = $('pvImg'); const src = p.largeImage || '/icon.ico';
    if (img.dataset.src !== src) { img.dataset.src = src; img.onerror = () => { img.onerror = null; img.src = '/icon.ico'; }; img.src = src; }
    img.title = p.largeText || '';
    // the player's logo in the cover's corner, as Discord draws it
    const small = $('pvSmall'), badge = $('pvBadge');
    badge.classList.toggle('hidden', !p.smallImage);
    if (p.smallImage && small.dataset.src !== p.smallImage) { small.dataset.src = p.smallImage; small.onerror = () => { badge.classList.add('hidden'); }; small.src = p.smallImage; }
    badge.title = p.smallText || '';
    const bar = $('pvBar'); const timed = p.start && p.end && p.end > p.start;
    bar.classList.toggle('hidden', !timed);
    if (timed) { const total = (p.end - p.start) / 1000; const done = Math.min(total, Math.max(0, (Date.now() - p.start) / 1000)); $('pvElapsed').textContent = mmss(done); $('pvTotal').textContent = mmss(total); $('pvFill').style.width = (100 * done / total).toFixed(1) + '%'; }
    const btn = $('pvButton'); btn.textContent = p.button || ''; btn.classList.toggle('hidden', !p.button);
    const btn2 = $('pvButton2'); btn2.textContent = p.button2 || ''; btn2.classList.toggle('hidden', !p.button2);
    $('fixMatch').classList.toggle('hidden', !!p.listening); // songs are not looked up
  }

  function paint(s) {
    paintPreview(s.preview);
    lastMatch = s.match || null;
    if (typeof s.hideTitle === 'boolean' && model && !saving && model.values.hideTitle !== s.hideTitle) {
      model.values.hideTitle = s.hideTitle; // switched from the tray or the menu bar
      const box = document.querySelector('input[data-key="hideTitle"]');
      if (box) box.checked = s.hideTitle;
    }
    if (typeof s.streamTitle === 'string' && model && !saving && model.values.streamTitle !== s.streamTitle) {
      model.values.streamTitle = s.streamTitle; // switched from the tray or the menu bar
      const sel = document.querySelector('select[data-key="streamTitle"]');
      if (sel && [...sel.options].some((o) => o.value === s.streamTitle)) sel.value = s.streamTitle;
    }
    // One line says whether the status is on; while it is, a second says how Discord and the player answer (a word as
    // well as a dot), and a third what plays when the preview does not already show it.
    running = !!s.running;
    $('stMain').textContent = s.running ? 'Showing on Discord' : 'Not showing on Discord';
    $('stSub').classList.toggle('hidden', !s.running);
    const d = $('pDiscord'), p = $('pMpc');
    d.textContent = s.discord === 'connected' ? 'Discord connected' : s.discord === 'waiting' ? 'Waiting for Discord' : s.discord === 'standby' ? 'Discord on standby' : 'Discord idle';
    d.title = s.discord === 'standby' ? 'Discord is contacted as soon as something plays.' : s.discord === 'waiting' ? 'Discord was not found yet. Is the desktop app running?' : '';
    d.className = s.discord === 'waiting' ? 'warn' : '';
    p.textContent = s.playerAnswers ? (s.player || 'Player') + ' detected' : 'No player found';
    p.className = s.playerAnswers ? '' : 'warn';
    $('stDot').className = 'dot ' + (s.discord === 'waiting' || !s.playerAnswers ? 'warn' : s.discord === 'connected' ? 'good' : '');
    const now = $('now');
    let line = '';
    if (!s.running) { /* the first line says it */ }
    else if (s.hidden) line = 'The file playing is on your Never show list, so nothing is shown on Discord.';
    else if (s.nowPlaying) {
      const sub = s.paused ? (s.pauseCleared ? 'Paused (hidden on Discord until you play again)' : 'Paused') : 'Playing';
      // while the Discord preview is showing, the title is already there: keep only a note that adds something
      line = s.preview ? (s.paused ? sub : '') : sub + ': ' + s.nowPlaying;
    }
    else if (s.playerAnswers) line = 'Nothing playing. Play something in ' + players() + '.';
    now.textContent = line;
    now.classList.toggle('hidden', !line);
    const t = $('toggle'); t.textContent = s.running ? 'Stop showing' : 'Show on Discord'; t.className = s.running ? '' : 'primary';
    // a short line for screen readers, changed only when the state changes (the status itself changes every second)
    const live = s.running ? d.textContent + '. ' + p.textContent + '.' : 'Not showing on Discord.';
    if ($('liveStatus').textContent !== live) $('liveStatus').textContent = live;
    const dp = $('discordProblem'); dp.textContent = s.discordProblem || ''; dp.classList.toggle('hidden', !s.discordProblem);
    $('playerHelp').classList.toggle('hidden', !(s.running && !s.playerAnswers));
    const hint = $('playerHint'); hint.textContent = s.hint || ''; hint.classList.toggle('hidden', !s.hint);
    if (s.update && s.update.newer && s.update.latest !== updShown && $('installUpd').disabled === false) showUpdate(s.update, false);
    paintUpdBanner(s.update);
  }
  // a one-line notice at the top while an update is available; its button brings the update's dialog back
  let updInfo = null;
  function paintUpdBanner(u) {
    const on = !!(u && u.newer);
    updInfo = on ? u : null;
    $('updBanner').classList.toggle('hidden', !on);
    if (on) $('updBannerText').textContent = 'MPRP ' + u.latest + ' is available.';
  }
  $('updBannerBtn').onclick = async () => { if (!await askUpdate(updInfo, true)) { showTab('general'); $('updates').scrollIntoView({ block: 'start' }); } };

  // The program has quit: nothing is asked any more (a window the browser does not let close would go on asking a
  // port that another program, or the next start, may listen on by then).
  function closeWindow() {
    gone = true; timers.forEach(clearInterval);
    document.body.innerHTML = '<main><p>MPRP has quit. You can close this window.</p></main>';
    try { window.close(); } catch (_) {}
  }
  let polling = false;
  async function poll() {
    if (document.hidden || polling || gone) return;
    polling = true; // one at a time: a slow answer must not pile requests up
    try { paint(await api('status')); } catch (_) {}
    polling = false;
  }
  // A connection the program holds open while it runs: when it ends the program has quit, so the window closes at once.
  // Unless only the connection broke: a program that still answers gets a new one.
  async function watchAlive() {
    for (;;) {
      try {
        const res = await fetch('/api/alive', { headers: { 'X-Token': TOKEN } });
        if (!res.ok) break; // another start of the program answers there now: this window is not its window
        const reader = res.body.getReader();
        while (!(await reader.read()).done) {}
      } catch (_) {}
      if (gone) return;
      try { if (!(await fetch('/api/ping')).ok) break; } catch (_) { break; }
      await new Promise((r) => setTimeout(r, 1000));
    }
    closeWindow();
  }
  async function pollLog() {
    if (tab !== 'help' || !$('logFold').open || document.hidden || gone) return;
    try {
      const { lines } = await api('log'); const box = $('log'); const sig = lines.join('\n');
      if (box.dataset.sig === sig) return; // unchanged: leave the list alone, so a selection in it survives
      box.dataset.sig = sig;
      const stick = box.scrollTop + box.clientHeight >= box.scrollHeight - 30;
      box.textContent = '';
      lines.forEach((l) => { const m = / (INFO|WARN|ERROR) /.exec(l); box.append(el('div', { className: m ? 'L-' + m[1] : '', textContent: l })); });
      if (stick) box.scrollTop = box.scrollHeight;
    } catch (_) {}
  }

  // Help > Run a check: the program says what works. Every word comes from the program and goes in as text. The state
  // is a word as well as a colour.
  $('runCheck').onclick = async () => {
    const btn = $('runCheck'), list = $('checkList'), msg = $('checkMsg');
    btn.disabled = true; btn.textContent = 'Checking…';
    try {
      const { items } = await api('selfcheck', {});
      list.textContent = '';
      (items || []).forEach((it) => {
        const mark = { ok: 'OK', problem: 'Problem' }[it.state] || 'Note';
        list.append(el('li', { className: it.state }, el('span', { className: 'mark', textContent: mark }),
          el('span', { className: 'what' }, el('b', { textContent: it.group + ': ' }), document.createTextNode(it.text))));
      });
      list.classList.remove('hidden');
      msg.classList.remove('bad'); msg.textContent = 'Checked just now.';
    } catch (e) { msg.classList.add('bad'); msg.textContent = e.message; }
    btn.disabled = false; btn.textContent = 'Run a check';
  };

  // the welcome card: dismissing it is remembered, so it only ever greets a new install
  $('welcomeOk').onclick = () => {
    $('welcome').classList.add('hidden');
    later(async () => { try { model = await api('save', { app: { welcomeSeen: true } }); } catch (e) { note(e.message, true); } });
  };

  $('toggle').onclick = async () => { $('toggle').disabled = true; try { paint(await api(model && running ? 'stop' : 'start', {})); } catch (e) { await ask('Could not change this', e.message, 'OK', false, null); } $('toggle').disabled = false; };
  async function restoreDefaults() {
    if (!await ask('Restore defaults?', 'Every setting goes back to its default. This also turns off ' + autoStartName() + ', signs you out of Plex, Stremio, Jellyfin and Emby, and clears the TMDB key, your fixes (Fix a match) and your Never show list.', 'Restore', true)) return;
    later(async () => {
      try { fill(await api('reset', {})); note('Defaults restored.'); } catch (e) { note(e.message, true); }
    });
  }
  // "Set up the player connection": used by the card shown when no player is answering and by the Players tab.
  // msgEl shows the progress and the result.
  async function setUpPlayers(btn, msgEl) {
    btn.disabled = true;
    msgEl.textContent = 'Working…';
    try {
      let r = await api('player-setup', { closePlayers: false });
      if (r.needsClose) { if (!await ask('Close the player for a moment?', (r.message || 'The player is open and has to be closed for a moment (it will reopen).') + ' Continue?', 'Continue')) { msgEl.textContent = ''; btn.disabled = false; return; } r = await api('player-setup', { closePlayers: true }); }
      msgEl.textContent = r.message || (r.ok ? 'Done.' : 'Could not change it.');
      if (r.settingsChanged) { // the mpv name or VLC's password may have been taken over; the window is drawn again, so say it here too
        // in the queue, like every other change of the window's values: an answer that left before a save still
        // under way must not draw the older values over it
        later(async () => { try { fill(await api('state')); note(msgEl.textContent); } catch (_) {} });
      }
    } catch (e) { msgEl.textContent = e.message; }
    btn.disabled = false;
  }
  $('enableWeb').onclick = () => setUpPlayers($('enableWeb'), $('webMsg'));
  $('logFold').addEventListener('toggle', () => { if ($('logFold').open) { pollLog(); setTimeout(() => { $('log').scrollTop = $('log').scrollHeight; }, 50); } });
  // Release notes as GitHub writes them (Markdown), shown with their paragraphs, bullet points, bold and code. Built
  // from text nodes only, so nothing in the notes can act as HTML.
  function notesOf(md) {
    const box = el('div', { className: 'notes' });
    const inline = (text) => {
      const out = el('span');
      text.replace(/\[([^\]]+)\]\([^)]*\)/g, '$1').split(/(\*\*[^*]+\*\*|`[^`]+`)/).forEach((part) => {
        if (/^\*\*[^*]+\*\*$/.test(part)) out.append(el('b', { textContent: part.slice(2, -2) }));
        else if (/^`[^`]+`$/.test(part)) out.append(el('code', { textContent: part.slice(1, -1) }));
        else if (part) out.append(document.createTextNode(part));
      });
      return out;
    };
    let list = null, para = null;
    for (const raw of String(md || '').replace(/\r\n/g, '\n').split('\n')) {
      const line = raw.trim();
      const item = /^[-*] +(.*)$/.exec(line);
      if (!line) { list = para = null; continue; }
      if (item) {
        if (!list) { list = el('ul'); box.append(list); para = null; }
        list.append(el('li', {}, inline(item[1])));
        continue;
      }
      const head = /^#{1,6} +(.*)$/.exec(line);
      if (head) { list = para = null; box.append(el('p', {}, el('b', { textContent: head[1] }))); continue; }
      if (list && /^\s{2,}/.test(raw)) { list.lastChild.append(document.createTextNode(' '), inline(line)); continue; }
      list = null;
      if (!para) { para = el('p'); box.append(para); } else para.append(document.createTextNode(' '));
      para.append(inline(line));
    }
    return box;
  }
  function setNotes(md) { const n = $('updNotes'); n.textContent = ''; if (md) n.append(notesOf(md)); }
  // A newer version: shown once per version, whether the window found it (Check now) or the program did by itself
  // (as it starts, then every six hours) while the window is open.
  let updShown = '';
  function showUpdate(u, explain) {
    if (!u || !u.newer) return;
    updShown = u.latest;
    $('updText').textContent = 'Version ' + u.latest + ' is available (you have ' + u.current + ').';
    setNotes(u.notes);
    offerUpdate(u, explain);
    flagUpdate();
    paintUpdBanner(u);
    if (!explain) askUpdate(u, false); // found by the program: ask at once (Check now shows its answer in its card)
  }
  // The dialog for a newer version: what is new in it, and Update now or Not now. It comes up by itself once for each
  // version (after Not now the notice at the top and the Updates card keep offering it), and from the notice's
  // button. It answers whether it was shown.
  let updAsked = '', updAsking = false;
  try { updAsked = localStorage.getItem('updAsked') || ''; } catch (_) {}
  async function askUpdate(u, byHand) {
    if (!u || !u.newer || updAsking || $('installUpd').disabled) return false;
    if (!byHand && updAsked === u.latest) return false;
    const install = !!(u.canInstall || (!osText() && u.url));
    if (!install && !u.page) return false; // nothing to press: the card says why
    updAsking = true;
    try {
      await noDialog(); // what's new in this version, or a question, goes first
      updAsked = u.latest;
      try { localStorage.setItem('updAsked', u.latest); } catch (_) {}
      const say = el('p', { textContent: 'You have ' + u.current + '.' });
      const body = el('div', {}, say);
      if (u.notes) body.append(el('div', { className: 'chg' }, notesOf(u.notes)));
      const self = install && model && model.os !== 'darwin' && model.app.checkUpdates && model.app.autoUpdate;
      const hint = el('p', { textContent: 'Not now: it installs by itself later, once nothing is playing and the app runs without this window.' });
      if (self) body.append(hint);
      await modal('MPRP ' + u.latest + ' is available', body, [['Not now'], [install ? 'Update now' : 'Open the download page', 'primary']], true, {
        focus: 1,
        hold: async (i) => {
          if (i !== 1) return true;
          if (!install) { window.open(u.page, '_blank', 'noopener'); return true; }
          hint.remove();
          if (!await installUpdate((t) => { say.textContent = t; $('updText').textContent = t; })) return false;
          return new Promise(() => {}); // the program restarts and closes this window
        },
      });
    } finally { updAsking = false; }
    return true;
  }
  // Install: download, check, swap, restart. say gets the progress, and the reason when it fails.
  async function installUpdate(say) {
    $('installUpd').disabled = true; say('Downloading… MPRP restarts by itself.');
    try { await api('update-install', {}); return true; } catch (e) { say('Could not update: ' + e.message); $('installUpd').disabled = false; return false; }
  }
  $('checkUpd').onclick = async () => {
    $('updText').textContent = 'Checking…'; $('installUpd').classList.add('hidden'); $('pageUpd').classList.add('hidden'); $('updNotes').textContent = '';
    try {
      await queue; // a change still being saved goes first
      const u = await api('update-check', {});
      if (!u.configured) $('updText').textContent = 'No update source is set (updateRepo in the settings file).';
      else if (u.error) $('updText').textContent = u.error;
      else if (u.newer) showUpdate(u, true);
      else $('updText').textContent = 'You are up to date (' + u.current + ').';
    } catch (e) { $('updText').textContent = 'Could not check: ' + e.message; }
  };
  // Install where the program can update itself; on a Mac (an app bundle) the release page is offered instead.
  let updPage = '';
  function offerUpdate(u, explain) {
    if (u.canInstall || (!osText() && u.url)) $('installUpd').classList.remove('hidden');
    else if (osText() && u.page) { updPage = u.page; $('pageUpd').classList.remove('hidden'); }
    else if (explain) $('updNotes').textContent = 'That release has no program file attached.';
  }
  $('pageUpd').onclick = () => { if (updPage) window.open(updPage, '_blank', 'noopener'); };
  $('installUpd').onclick = () => installUpdate((t) => { $('updText').textContent = t; });
  $('copyDiag').onclick = async () => {
    const msg = $('diagMsg'), box = $('diagText');
    try {
      const { text } = await api('diagnostics');
      box.value = text; box.classList.remove('hidden');
      let ok = false;
      try { await navigator.clipboard.writeText(text); ok = true; } catch (_) { box.focus(); box.select(); try { ok = document.execCommand('copy'); } catch (_) {} }
      msg.textContent = ok ? 'Copied. Look it over before you post it: it names what you played (folder paths, sign-ins and keys are left out).' : 'Select the text below and copy it (' + (osText() ? osText().copyKey : 'Ctrl+C') + ').';
    } catch (e) { msg.textContent = 'Could not build it: ' + e.message; }
  };
  $('reportBtn').onclick = () => window.open('https://github.com/CptPakundo/MPRP/issues/new/choose', '_blank', 'noopener');
  $('ver').onclick = () => showChangelog(true);
  // the preview's "Wrong title or cover?": the Fix this match dialog for what plays, else the list of fixes
  $('fixMatch').onclick = () => {
    if (lastMatch) { openFix({ match: lastMatch }); return; }
    showTab('titles');
    const h = [...document.querySelectorAll('#p-titles h3.grp')].find((x) => x.textContent === 'Fix a match');
    if (h) h.scrollIntoView({ block: 'start' });
  };
  $('statsReset').onclick = async () => {
    if (!await ask('Reset your stats?', 'The counts start again from today.', 'Reset', true)) return;
    try { paintStats(await api('stats-reset', {})); } catch (e) { note(e.message, true); }
  };
  $('quit').onclick = async () => { if (await ask('Quit MPRP?', 'Discord stops showing what you watch.', 'Quit', true)) { try { await api('quit', {}); } catch (_) {} closeWindow(); } };

  if (!TOKEN) { document.body.replaceChildren(el('main', {}, el('p', { textContent: 'Open MPRP from its tray icon or by launching the program again.' }))); return; }
  watchAlive();
  api('state').then((m) => {
    fill(m); paint(m.status);
    // the changelog shows once for each new version (a new install too); the version button shows it any time
    if (m.app.seenVersion !== m.version) { showChangelog(false); later(async () => { try { model = await api('save', { app: { seenVersion: m.version } }); } catch (_) {} }); }
    showUpdate(m.update, false);
  }).catch((e) => { document.body.replaceChildren(el('main', {}, el('p', { textContent: 'Could not reach the program: ' + e.message }))); });
  timers = [setInterval(poll, 1000), setInterval(pollLog, 3000), setInterval(pollConns, 5000)];
  // a hidden window asks for nothing; when it shows again it catches up at once
  document.addEventListener('visibilitychange', () => { if (!document.hidden) { poll(); pollLog(); pollConns(); } });
})();
