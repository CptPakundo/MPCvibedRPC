'use strict';
// Behaviour tests for the settings page's script (ui.js), run by TestPageBehaviour (assets_test.go) with Node.js:
//   node page_test.js <ui.html> <ui.js> <state.json> <lines.json>
// ui.js runs in a small fake DOM built from ui.html, with a stubbed fetch that plays the program's local API. It
// prints one line per check and exits 1 when one fails.
const fs = require('fs');
const vm = require('vm');

const [htmlPath, jsPath, statePath, linesPath] = process.argv.slice(2);

// ---- a small DOM: enough of it for ui.js ------------------------------------------------------------------------
const camel = (s) => s.replace(/-([a-z])/g, (m, c) => c.toUpperCase());
class TextNode {
  constructor(t) { this.nodeType = 3; this.data = String(t); this.parentNode = null; }
  get textContent() { return this.data; }
  set textContent(v) { this.data = String(v); }
  remove() { if (this.parentNode) this.parentNode.removeChild(this); }
}
class ClassList {
  constructor(el) { this.el = el; }
  get list() { return this.el.className.split(/\s+/).filter(Boolean); }
  contains(c) { return this.list.includes(c); }
  add(...cs) { const l = this.list; cs.forEach((c) => { if (!l.includes(c)) l.push(c); }); this.el.className = l.join(' '); }
  remove(...cs) { this.el.className = this.list.filter((c) => !cs.includes(c)).join(' '); }
  toggle(c, on) { const has = this.contains(c); const want = on === undefined ? !has : !!on; if (want && !has) this.add(c); if (!want && has) this.remove(c); return want; }
}
let doc = null;
class Element {
  constructor(tag) {
    this.nodeType = 1; this.tagName = tag.toUpperCase(); this.childNodes = []; this.parentNode = null;
    this.attrs = {}; this.dataset = {}; this.listeners = {}; this.style = {}; this.className = ''; this.id = '';
    this.classList = new ClassList(this); this.disabled = false; this.checked = false; this.selectionStart = null; this.selectionEnd = null;
    this._value = '';
  }
  get children() { return this.childNodes.filter((n) => n.nodeType === 1); }
  get firstChild() { return this.childNodes[0] || null; }
  get lastChild() { return this.childNodes[this.childNodes.length - 1] || null; }
  get textContent() { return this.childNodes.map((n) => n.textContent).join(''); }
  set textContent(v) { this.childNodes.forEach((n) => { n.parentNode = null; }); this.childNodes = []; if (v !== '' && v != null) this.append(String(v)); }
  set innerHTML(v) { this.textContent = ''; this._html = v; }
  get innerHTML() { return this._html || ''; }
  get value() {
    if (this.tagName === 'SELECT') { const o = this.options.find((x) => x.selected) || this.options[0]; return o ? o.value : ''; }
    return this._value;
  }
  set value(v) {
    if (this.tagName === 'SELECT') { this.options.forEach((o) => { o.selected = o.value === String(v); }); return; }
    this._value = v == null ? '' : String(v);
  }
  get options() { return this.children.filter((c) => c.tagName === 'OPTION'); }
  get isConnected() { let n = this; while (n.parentNode) n = n.parentNode; return n === doc.documentElement; }
  append(...nodes) { nodes.forEach((n) => this.appendChild(typeof n === 'string' ? new TextNode(n) : n)); }
  prepend(...nodes) { nodes.reverse().forEach((n) => { const x = typeof n === 'string' ? new TextNode(n) : n; if (x.parentNode) x.parentNode.removeChild(x); x.parentNode = this; this.childNodes.unshift(x); }); }
  appendChild(n) { if (n.parentNode) n.parentNode.removeChild(n); n.parentNode = this; this.childNodes.push(n); return n; }
  removeChild(n) { const i = this.childNodes.indexOf(n); if (i >= 0) this.childNodes.splice(i, 1); n.parentNode = null; return n; }
  remove() { if (this.parentNode) this.parentNode.removeChild(this); }
  before(n) { const p = this.parentNode; if (n.parentNode) n.parentNode.removeChild(n); p.childNodes.splice(p.childNodes.indexOf(this), 0, n); n.parentNode = p; }
  replaceWith(n) { const p = this.parentNode; if (!p) return; p.childNodes[p.childNodes.indexOf(this)] = n; n.parentNode = p; this.parentNode = null; }
  replaceChildren(...nodes) { this.textContent = ''; this.append(...nodes); }
  setAttribute(k, v) { this.attrs[k] = String(v); if (k === 'id') this.id = String(v); if (k === 'class') this.className = String(v); }
  getAttribute(k) { if (k === 'id') return this.id || null; if (k === 'class') return this.className; return k in this.attrs ? this.attrs[k] : null; }
  hasAttribute(k) { return this.getAttribute(k) !== null; }
  removeAttribute(k) { delete this.attrs[k]; }
  addEventListener(t, f) { (this.listeners[t] = this.listeners[t] || []).push(f); }
  removeEventListener(t, f) { this.listeners[t] = (this.listeners[t] || []).filter((x) => x !== f); }
  dispatch(type, extra) {
    const ev = Object.assign({ type, target: this, defaultPrevented: false, preventDefault() { this.defaultPrevented = true; }, stopPropagation() {} }, extra || {});
    (this.listeners[type] || []).slice().forEach((f) => f.call(this, ev));
    if (typeof this['on' + type] === 'function') this['on' + type](ev);
    return ev;
  }
  click() { if (this.disabled) return; this.dispatch('mousedown'); this.dispatch('click'); }
  focus() { doc.activeElement = this; this.dispatch('focus'); }
  blur() { if (doc.activeElement === this) doc.activeElement = doc.body; this.dispatch('blur'); }
  select() {}
  scrollIntoView() {}
  getClientRects() { return [{}]; }
  closest(sel) { let n = this; while (n && n.nodeType === 1) { if (matches(n, sel)) return n; n = n.parentNode; } return null; }
  matches(sel) { return matches(this, sel); }
  querySelectorAll(sel) { const out = []; walk(this, (n) => { if (n !== this && matches(n, sel, this)) out.push(n); }); return out; }
  querySelector(sel) { return this.querySelectorAll(sel)[0] || null; }
}
function walk(n, f) { n.childNodes.forEach((c) => { if (c.nodeType === 1) { f(c); walk(c, f); } }); }
// selectors: groups of compounds (tag, #id, .class, [attr] or [attr="v"]) joined by spaces (descendant)
function parseCompound(s) {
  const c = { tag: null, id: null, cls: [], attrs: [] };
  const re = /([a-zA-Z][\w-]*)|#([\w-]+)|\.([\w-]+)|\[([\w-]+)(?:="([^"]*)")?\]/y;
  for (let i = 0; i < s.length;) {
    re.lastIndex = i;
    const m = re.exec(s);
    if (!m) throw new Error('selector not understood: ' + s);
    if (m[1]) c.tag = m[1].toUpperCase(); else if (m[2]) c.id = m[2]; else if (m[3]) c.cls.push(m[3]); else c.attrs.push([m[4], m[5]]);
    i = re.lastIndex;
  }
  return c;
}
function attrOf(n, k) {
  if (k.startsWith('data-')) return n.dataset[camel(k.slice(5))];
  if (k === 'tabindex') return n.tabIndex == null ? undefined : String(n.tabIndex);
  if (k in n.attrs) return n.attrs[k];
  const p = n[k];
  return p == null || p === '' ? undefined : String(p);
}
function one(n, c) {
  if (c.tag && n.tagName !== c.tag) return false;
  if (c.id && n.id !== c.id) return false;
  if (!c.cls.every((x) => n.classList.contains(x))) return false;
  return c.attrs.every(([k, v]) => { const a = attrOf(n, k); return v === undefined ? a !== undefined : a === v; });
}
function matches(n, sel, scope) {
  return sel.split(',').some((group) => {
    const parts = group.trim().split(/\s+/).map(parseCompound);
    if (!one(n, parts[parts.length - 1])) return false;
    let cur = n.parentNode;
    for (let i = parts.length - 2; i >= 0; i--) {
      while (cur && cur.nodeType === 1 && !one(cur, parts[i])) { if (cur === scope) return false; cur = cur.parentNode; }
      if (!cur || cur.nodeType !== 1) return false;
      cur = cur.parentNode;
    }
    return true;
  });
}

// ui.html as a tree (the page's markup is simple: no unquoted attributes, no inline script)
function build(html) {
  const root = new Element('html');
  const stack = [root];
  const voids = new Set(['meta', 'link', 'img', 'input', 'br', 'hr']);
  const re = /<!--[\s\S]*?-->|<!doctype[^>]*>|<(\/?)([a-zA-Z][\w-]*)((?:\s+[\w:-]+(?:="[^"]*")?)*)\s*\/?>|([^<]+)/gi;
  let m;
  while ((m = re.exec(html))) {
    if (m[4] !== undefined) { if (m[4].trim()) stack[stack.length - 1].append(m[4]); continue; }
    if (!m[2]) continue;
    const tag = m[2].toLowerCase();
    if (m[1]) { if (tag !== 'html') stack.pop(); continue; }
    if (tag === 'html') { continue; }
    const e = new Element(tag);
    const ar = /([\w:-]+)(?:="([^"]*)")?/g;
    let a;
    while ((a = ar.exec(m[3]))) {
      const k = a[1], v = a[2] === undefined ? '' : a[2];
      if (k === 'class') e.className = v;
      else if (k === 'id') e.id = v;
      else if (k.startsWith('data-')) e.dataset[camel(k.slice(5))] = v;
      else if (k === 'type' || k === 'content' || k === 'name' || k === 'src' || k === 'href') { e[k] = v; e.attrs[k] = v; }
      else if (k === 'checked' || k === 'disabled' || k === 'readonly') e[k] = true;
      else e.attrs[k] = v;
    }
    stack[stack.length - 1].appendChild(e);
    if (!voids.has(tag)) stack.push(e);
  }
  return root;
}

const html = fs.readFileSync(htmlPath, 'utf8').replace('__TOKEN__', 'test-token');
const root = build(html);
const docListeners = {};
doc = {
  documentElement: root,
  activeElement: null,
  hidden: false,
  get body() { return root.querySelector('body'); },
  getElementById(id) { let f = null; walk(root, (n) => { if (!f && n.id === id) f = n; }); return f; },
  querySelector: (s) => root.querySelector(s),
  querySelectorAll: (s) => root.querySelectorAll(s),
  createElement: (t) => new Element(t),
  createTextNode: (t) => new TextNode(t),
  addEventListener(t, f) { (docListeners[t] = docListeners[t] || []).push(f); },
  removeEventListener(t, f) { docListeners[t] = (docListeners[t] || []).filter((x) => x !== f); },
  execCommand: () => true,
};
doc.activeElement = doc.body;
const key = (k, extra) => { const ev = Object.assign({ key: k, preventDefault() {}, shiftKey: false }, extra || {}); (docListeners.keydown || []).slice().forEach((f) => f(ev)); };

// ---- the program's API, stubbed ----------------------------------------------------------------------------------
const state = JSON.parse(fs.readFileSync(statePath, 'utf8'));
const calls = [];
let refuseNextSave = null; // the error the next save answers with
let staleSearch = false;   // the next search answers with an older request number
const opened = [], copied = [];
const SEARCH = [
  { name: 'Sample Show', year: 2019, kind: 'series', catalog: 'IMDb', entry: 'https://www.imdb.com/title/tt0000003/', poster: 'https://img.example/a.jpg' },
  { name: 'The Sample Show', year: 1998, kind: 'series', catalog: 'IMDb', entry: 'https://www.imdb.com/title/tt0000001/' },
  { name: 'Sample Show', year: 2024, kind: 'series', catalog: 'TVmaze', entry: 'https://www.tvmaze.com/shows/43/sample-show' },
];
function answer(status, body) {
  return { ok: status < 400, status, json: async () => JSON.parse(JSON.stringify(body)), body: { getReader: () => ({ read: () => new Promise(() => {}) }) } };
}
async function fakeFetch(url, init) {
  const name = url.replace(/^\/api\//, '');
  const body = init && init.body ? JSON.parse(init.body) : undefined;
  if (name === 'alive') return answer(200, {}); // held open while the program runs: never ends here
  calls.push({ name, body });
  switch (name) {
    case 'state': return answer(200, state);
    case 'status': return answer(200, state.status);
    case 'save': {
      if (refuseNextSave) { const e = refuseNextSave; refuseNextSave = null; return answer(400, { error: e }); }
      Object.assign(state.values, (body && body.settings) || {});
      Object.assign(state.app, (body && body.app) || {});
      return answer(200, state);
    }
    case 'match-search': return answer(200, { n: staleSearch ? body.n - 1 : body.n, results: (body.q || '').length >= 2 ? SEARCH : [], catalogs: ['IMDb', 'TVmaze', 'AniList'] });
    case 'match-covers': return answer(200, { n: body.n, covers: [{ url: 'https://img.example/other.jpg', catalog: 'TVmaze' }] });
    case 'match-report': return answer(200, { text: 'File name: ' + body.file + '\nThe fix: ' + (body.fix.label || body.fix.search) });
    case 'log': return answer(200, { lines: [] });
    case 'connections': return answer(200, { hosts: [] });
    default: return answer(200, {});
  }
}

// ---- run ui.js -----------------------------------------------------------------------------------------------------
const ctx = {
  document: doc, console, fetch: fakeFetch, setTimeout, clearTimeout, setInterval, clearInterval,
  history: { replaceState() {} }, location: { pathname: '/' },
  sessionStorage: { getItem: () => null, setItem() {} }, localStorage: { getItem: () => null, setItem() {} },
  navigator: { clipboard: { writeText: async (t) => { copied.push(t); } } },
  Option: function Option(text, value, defSel, sel) { const o = new Element('option'); o.textContent = text; o.value = value; o.selected = !!sel; return o; },
};
ctx.window = ctx;
ctx.window.open = (u) => { opened.push(u); };
ctx.window.close = () => {};
vm.createContext(ctx);
vm.runInContext(fs.readFileSync(jsPath, 'utf8'), ctx, { filename: 'ui.js' });

// ---- the checks ----------------------------------------------------------------------------------------------------
let failed = 0;
const check = (ok, what, detail) => { console.log((ok ? 'ok   ' : 'FAIL ') + what + (ok || detail === undefined ? '' : ': ' + JSON.stringify(detail))); if (!ok) failed++; };
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
async function until(cond, ms = 2000) { const end = Date.now() + ms; while (Date.now() < end) { if (cond()) return true; await sleep(20); } return cond(); }
const $ = (id) => doc.getElementById(id);
const field = (sel) => doc.querySelector(sel);
const saves = () => calls.filter((c) => c.name === 'save');
const lastSave = () => saves()[saves().length - 1];
const change = (el, v) => { if (el.type === 'checkbox') el.checked = v; else el.value = v; el.dispatch('change'); };

(async () => {
  // 1. the shared table of text lines
  const tab = JSON.parse(fs.readFileSync(linesPath, 'utf8'));
  const L = vm.runInContext('MPRP_LINES', ctx);
  let bad = tab.cases.filter((c) => L.expand(c.tpl, tab.values[c.values]) !== c.want);
  check(tab.cases.length >= 10 && !bad.length, 'the page fills text lines as the program does (' + tab.cases.length + ' cases)', bad.map((c) => [c.tpl, L.expand(c.tpl, tab.values[c.values]), c.want]));

  // 2. the page draws itself from the state
  check(await until(() => $('p-look') && $('p-titles')), 'the page draws the settings tabs');

  // 3. the saver: one setting at a time; a refused value goes back, with the error in the bar
  const rich = field('input[data-key="richInfo"]');
  change(rich, false);
  check(await until(() => saves().length === 1), 'a switch saves at once');
  check(JSON.stringify(lastSave().body) === '{"settings":{"richInfo":false}}', 'only that setting is sent', lastSave().body);
  check(await until(() => $('msg').textContent === 'Saved'), 'the bar says Saved', $('msg').textContent);
  refuseNextSave = 'Check every (ms): enter a number from 1000 to 60000';
  const poll = field('input[data-key="pollInterval"]');
  const before = poll.value;
  change(poll, '5');
  check(await until(() => $('msg').classList.contains('err')), 'a refused value shows its error', $('msg').textContent);
  check(poll.value === String(before) && $('msg').textContent.startsWith('Check every'), 'and goes back to what was saved', poll.value);

  // 4. the status sync: a switch made from the tray shows in the window without a save
  const n = saves().length;
  state.status.hideTitle = true;
  check(await until(() => field('input[data-key="hideTitle"]').checked, 2500), 'Hide what I\'m watching follows the status');
  check(saves().length === n, 'without saving anything');
  state.status.hideTitle = false;
  await until(() => !field('input[data-key="hideTitle"]').checked, 2500);

  // 5. Text lines: Custom shows its lines, filled from the choice before, with a sample; a line saves after a pause
  const choice = field('select[data-key="textLines"]'), custom = field('.tl-custom');
  const epRow = field('input[data-key="episodeInDetails"]').closest('.field');
  check(custom.classList.contains('hidden') && !epRow.classList.contains('hidden'), 'Standard: no custom lines, S01E02 switch shown');
  change(choice, 'custom');
  check(await until(() => saves().some((s) => s.body.settings && s.body.settings.customLines)), 'Custom saves the lines it starts with');
  const cl = saves().find((s) => s.body.settings && s.body.settings.customLines).body.settings.customLines;
  check(cl.episodeDetails === '{episodeTitle}' && cl.movieState === '{rating} · {director}' && cl.movieCover === '{title}[ ({year})]', 'filled with Standard\'s lines', cl);
  check(!custom.classList.contains('hidden') && epRow.classList.contains('hidden'), 'Custom shows the lines and hides the S01E02 switch');
  const ep = field('input[data-line="episodeDetails"]');
  check(custom.querySelector('.tl-l1').textContent === 'Sample Episode', 'the sample shows the first line', custom.querySelector('.tl-l1').textContent);
  ep.value = '{code} · {titel}';
  ep.dispatch('input');
  check(custom.querySelector('.tl-l1').textContent === 'S01E02 · {titel}' && custom.querySelector('.tl-bad').textContent === 'Unknown: {titel}', 'the sample follows the typing and names an unknown placeholder', custom.querySelector('.tl-bad').textContent);
  const chip = [...custom.querySelectorAll('button.chip')].find((b) => b.textContent === '{rating}');
  ep.focus();
  chip.click();
  check(ep.value === '{code} · {titel} · {rating}', 'a placeholder goes in with a separator', ep.value);
  const k = saves().length;
  check(!(await until(() => saves().length > k, 500)), 'no save on each key');
  check(await until(() => saves().length > k, 1500) && lastSave().body.settings.customLines.episodeDetails === '{code} · {titel} · {rating}', 'the line saves after a pause', lastSave().body);
  change(choice, 'short');
  check(await until(() => custom.classList.contains('hidden')), 'another choice hides the custom lines');

  // 6. Fix a match: the list, Remove and Undo
  const rows = () => [...$('fixes').querySelectorAll('.fix-row')];
  check(rows().length === 2, 'the list shows the fixes', rows().length);
  check(rows()[0].querySelector('.help').textContent === 'Sample Show (2019) · Show · IMDb · episodes +12' && rows()[1].querySelector('.help').textContent === 'Only this file · Your image',
    'each says what it does', rows().map((r) => r.querySelector('.help').textContent));
  rows()[1].querySelectorAll('button')[1].click();
  check(await until(() => rows().length === 1), 'Remove takes the row away');
  check(lastSave().body.settings.matchFixes.length === 1, 'and saves the list without it');
  const undo = $('msg').querySelector('button');
  check(!!undo && $('msg').textContent.startsWith('Fix removed.'), 'the bar offers Undo', $('msg').textContent);
  undo.click();
  check(await until(() => rows().length === 2) && lastSave().body.settings.matchFixes.length === 2, 'Undo brings it back');

  // 7. Fix this match from the preview: what plays, the search as you type, the pick, the cover step
  state.status.match = { file: 'sample.show.s01e02.1080p.mkv', title: 'Sample Show', season: 1, episode: 2, now: 'The Sample Show (1998)', nowEntry: 'https://www.imdb.com/title/tt0000001/' };
  await sleep(1300); // the next status poll
  $('fixMatch').click();
  const sheet = () => doc.querySelector('.sheet');
  check(await until(() => sheet()), 'Wrong title or cover? opens Fix this match');
  const from = sheet().querySelector('.from').textContent;
  check(from === 'From sample.show.s01e02.1080p.mkv, read as Sample Show, season 1, episode 2. Now matched to The Sample Show (1998).', 'it says what was read', from);
  const searches = () => calls.filter((c) => c.name === 'match-search');
  check(await until(() => searches().length === 1 && sheet().querySelectorAll('.res li[role="option"]').length === 4), 'it searches the read title at once', searches().map((c) => c.body));
  check(searches()[0].body.q === 'Sample Show', 'with the read title', searches()[0].body);
  const opts = () => [...sheet().querySelectorAll('.res li[role="option"]')];
  check(opts()[1].textContent.endsWith('Now') && opts()[3].textContent === 'Search as "Sample Show" instead (no entry pinned)', 'the current match says Now; the last row searches under the typed name', opts().map((o) => o.textContent));
  check(sheet().querySelector('.help').textContent.startsWith('Searches IMDb, TVmaze and AniList'), 'it names the catalogs it asks', sheet().querySelector('.help').textContent);
  const use = [...sheet().querySelectorAll('button')].find((b) => b.textContent === 'Use this');
  opts()[1].click();
  check(use.disabled, 'the current match cannot be used again');
  const q = sheet().querySelector('input[type="text"]');
  q.value = 'Sa'; q.dispatch('input');
  q.value = 'Sample'; q.dispatch('input');
  await sleep(150);
  check(searches().length === 1, 'no search before the pause');
  check(await until(() => searches().length === 2, 1000) && searches()[1].body.q === 'Sample' && searches()[1].body.n === 2, 'one search after the pause', searches().map((c) => c.body));
  await until(() => opts().length === 4);
  staleSearch = true;
  q.value = 'Sample Sh'; q.dispatch('input');
  await until(() => searches().length === 3, 1000);
  await sleep(100);
  check(opts().length === 0 || sheet().querySelector('.res li.wait'), 'an answer to an older keystroke is dropped', opts().length);
  staleSearch = false;
  q.value = 'Sample Show'; q.dispatch('input');
  check(await until(() => opts().length === 4, 1500), 'the next answer is drawn');
  opts()[2].click();
  check(!use.disabled, 'a pick can be used');
  const scope = sheet().querySelectorAll('.scope input');
  check(scope.length === 2 && scope[0].checked, 'the fix is for every file read as the title by default');
  use.click();
  check(await until(() => sheet() && sheet().querySelector('h2').textContent === 'Fixed'), 'Use this saves and shows Fixed');
  const saved = lastSave().body.settings.matchFixes;
  const fx = saved.find((x) => x.match === 'Sample Show');
  check(!!fx && fx.entry === 'https://www.tvmaze.com/shows/43/sample-show' && fx.label === 'Sample Show (2024) · Show · TVmaze' && fx.episodeOffset === 12 && saved.length === 2,
    'the fix pins the pick and keeps its offset', saved);
  check(await until(() => calls.some((c) => c.name === 'match-covers') && sheet().querySelectorAll('.covers img').length === 1), 'the cover step asks for the other catalogs\' covers');

  // 8. Tell the project: the report is shown, copied, and GitHub's form opens at its plain address
  [...sheet().querySelectorAll('button')].find((b) => b.textContent === 'Tell the project about this').click();
  check(await until(() => doc.querySelector('textarea.report')), 'Tell the project shows the report');
  const rep = calls.find((c) => c.name === 'match-report').body;
  check(rep.file === 'sample.show.s01e02.1080p.mkv' && rep.before === 'The Sample Show (1998)' && rep.fix.entry === fx.entry, 'built from what plays and the fix', rep);
  const reportText = doc.querySelector('textarea.report').value;
  [...doc.querySelectorAll('.dlg button')].find((b) => b.textContent === 'Copy and open GitHub').click();
  check(await until(() => opened.length === 1), 'Copy and open GitHub opens the issue form');
  check(opened[0] === 'https://github.com/CptPakundo/MPRP/issues/new/choose' && copied[0] === reportText && reportText.startsWith('File name: sample.show'), 'at its plain address, with the text copied', [opened, copied]);
  await until(() => !doc.querySelector('textarea.report') || !doc.querySelector('textarea.report').isConnected);

  // 9. Done without a change saves nothing; Escape closes the dialog
  const s9 = saves().length;
  [...sheet().querySelectorAll('button')].find((b) => b.textContent === 'Done').click();
  check(await until(() => !sheet()) && saves().length === s9, 'Done without a change closes without saving');
  $('fixMatch').click();
  await until(() => sheet());
  key('Escape');
  check(await until(() => !sheet()), 'Escape closes Fix this match');

  // 10. About (Help tab): the version the program reports, and links to the project's own pages only
  check($('aboutVer').textContent === 'Version ' + state.version, 'About shows the version', $('aboutVer').textContent);
  const aboutLinks = $('p-help').querySelectorAll('.about a').map((a) => a.attrs.href);
  check(aboutLinks.length >= 4 && aboutLinks.every((u) => u.startsWith('https://github.com/')) && aboutLinks.includes('https://github.com/CptPakundo/MPRP'),
    'About links to the project page, the licence and the credit', aboutLinks);

  console.log(failed ? failed + ' failed' : 'all passed');
  process.exit(failed ? 1 : 0);
})().catch((e) => { console.log('FAIL the test itself: ' + (e && e.stack || e)); process.exit(1); });
