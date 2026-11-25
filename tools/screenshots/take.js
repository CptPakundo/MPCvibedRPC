// Takes the README screenshots (docs/images/window-main.jpg, window-privacy.jpg, window-plex.jpg) from a throwaway copy
// of the program, in a headless Edge or Chrome with a profile of its own. Node.js 22 or later, nothing to install.
//
//   node tools/screenshots/take.js <the program you built> [--out docs/images] [--quality 70]
//
// The copy gets a temporary data folder (MPRP_HOME) and opens no window (MPRP_BROWSER=none); showing on Discord,
// update checks and autostart are off, and its players point nowhere (port 1, no mpv pipe, no VLC or Kodi password),
// so it contacts nothing. window-main shows made-up playback from demo.js. The copy is quit and both temporary
// folders are deleted at the end. The browser is found in the usual places; MPRP_SHOT_BROWSER names another one.
'use strict';
const fs = require('fs');
const os = require('os');
const path = require('path');
const { spawn, execFileSync } = require('child_process');

// The program's own window: Edge's --window-size=660,800 (internal/winsys/window.go) leaves a 660 x 760 page. The
// pictures are 800 pixels wide.
const PAGE = { width: 660, height: 760 };
const WIDTH = 800;

function usage(msg) {
  if (msg) console.error(msg);
  console.error('usage: node tools/screenshots/take.js <program> [--out <folder>] [--quality <1-100>]');
  process.exit(2);
}

function parseArgs(argv) {
  const o = { out: path.join(__dirname, '..', '..', 'docs', 'images'), quality: 70 };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === '--out') o.out = argv[++i];
    else if (a === '--quality') o.quality = Number(argv[++i]);
    else if (a.startsWith('-')) usage('unknown option ' + a);
    else if (!o.program) o.program = path.resolve(a);
    else usage('one program only');
  }
  if (!o.program || !fs.existsSync(o.program)) usage('name the program you built');
  if (!(o.quality >= 1 && o.quality <= 100)) usage('--quality is 1 to 100');
  if (!o.out) usage('--out needs a folder');
  return o;
}

function findBrowser() {
  if (process.env.MPRP_SHOT_BROWSER) return process.env.MPRP_SHOT_BROWSER;
  const list = [];
  if (process.platform === 'win32') {
    for (const base of [process.env['ProgramFiles(x86)'], process.env.ProgramFiles, process.env.LOCALAPPDATA]) {
      if (!base) continue;
      list.push(path.join(base, 'Microsoft', 'Edge', 'Application', 'msedge.exe'));
      list.push(path.join(base, 'Google', 'Chrome', 'Application', 'chrome.exe'));
    }
  } else if (process.platform === 'darwin') {
    list.push('/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge');
    list.push('/Applications/Google Chrome.app/Contents/MacOS/Google Chrome');
    list.push('/Applications/Chromium.app/Contents/MacOS/Chromium');
  } else {
    for (const dir of (process.env.PATH || '').split(':')) {
      for (const name of ['microsoft-edge', 'google-chrome', 'google-chrome-stable', 'chromium', 'chromium-browser']) {
        list.push(path.join(dir, name));
      }
    }
  }
  const found = list.find((p) => fs.existsSync(p));
  if (!found) throw new Error('no Edge or Chrome found: set MPRP_SHOT_BROWSER to one');
  return found;
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function waitFor(what, fn, ms = 20000) {
  const end = Date.now() + ms;
  for (;;) {
    const v = await fn().catch(() => null);
    if (v) return v;
    if (Date.now() > end) throw new Error('timed out waiting for ' + what);
    await sleep(200);
  }
}

// rmrf deletes a folder, retrying while a process that just quit still holds files in it.
async function rmrf(dir) {
  for (let i = 0; i < 25; i++) {
    try { fs.rmSync(dir, { recursive: true, force: true }); return; } catch (_) { await sleep(400); }
  }
  console.error('could not delete ' + dir + ': delete it by hand');
}

// startCopy starts the throwaway copy and returns how to reach its API.
async function startCopy(program, home) {
  const config = {
    port: 1, mpvPipe: '', vlcPassword: '', kodiPassword: '',
    app: { autoStart: false, startPresence: false, checkUpdates: false, autoUpdate: false, welcomeSeen: true, openWindow: false },
  };
  fs.writeFileSync(path.join(home, 'config.json'), JSON.stringify(config, null, 2));
  const env = { ...process.env, MPRP_HOME: home, MPRP_BROWSER: 'none' };
  const child = spawn(program, [], { env, stdio: 'ignore' });
  const ipc = await waitFor('the copy to start', async () => JSON.parse(fs.readFileSync(path.join(home, 'ipc.json'), 'utf8')));
  const base = 'http://127.0.0.1:' + ipc.port;
  const api = async (name, body) => {
    const res = await fetch(base + '/api/' + name, {
      method: body === undefined ? 'GET' : 'POST',
      headers: { 'X-Token': ipc.token, 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    if (!res.ok) throw new Error(name + ': HTTP ' + res.status);
    return res.json();
  };
  return { child, base, token: ipc.token, api };
}

// cdp is a minimal Chrome DevTools Protocol client over the browser's WebSocket.
function cdp(url) {
  const ws = new WebSocket(url);
  let id = 0;
  const waiting = new Map();
  const listeners = [];
  ws.onmessage = (ev) => {
    const m = JSON.parse(ev.data);
    if (m.id && waiting.has(m.id)) {
      const { ok, fail } = waiting.get(m.id);
      waiting.delete(m.id);
      if (m.error) fail(new Error(m.error.message)); else ok(m.result);
    } else if (m.method) {
      listeners.forEach((l) => l(m));
    }
  };
  const open = new Promise((ok, fail) => { ws.onopen = ok; ws.onerror = () => fail(new Error('cannot reach the browser')); });
  return {
    open,
    send(method, params = {}) {
      return new Promise((ok, fail) => { const n = ++id; waiting.set(n, { ok, fail }); ws.send(JSON.stringify({ id: n, method, params })); });
    },
    on(method, fn) { listeners.push((m) => { if (m.method === method) fn(m.params); }); },
    once(method) { return new Promise((ok) => { const l = (m) => { if (m.method === method) { listeners.splice(listeners.indexOf(l), 1); ok(m.params); } }; listeners.push(l); }); },
    close() { ws.close(); },
  };
}

async function main() {
  const o = parseArgs(process.argv.slice(2));
  const browser = findBrowser();
  const home = fs.mkdtempSync(path.join(os.tmpdir(), 'mprp-shots-home-'));
  const profile = fs.mkdtempSync(path.join(os.tmpdir(), 'mprp-shots-browser-'));
  let copy, edge, page;
  try {
    copy = await startCopy(o.program, home);
    const state = await copy.api('state');
    await copy.api('save', { app: { seenVersion: state.version } }); // keeps the What's new box closed

    edge = spawn(browser, [
      '--headless=new', '--user-data-dir=' + profile, '--remote-debugging-port=0', '--no-first-run',
      '--no-default-browser-check', '--disable-extensions', 'about:blank',
    ], { stdio: 'ignore' });
    const devPort = await waitFor('the browser to start', async () => fs.readFileSync(path.join(profile, 'DevToolsActivePort'), 'utf8').split('\n')[0].trim());
    const target = await waitFor('the browser page', async () => {
      const list = await (await fetch('http://127.0.0.1:' + devPort + '/json/list')).json();
      return list.find((t) => t.type === 'page');
    });
    page = cdp(target.webSocketDebuggerUrl);
    await page.open;
    await page.send('Page.enable');
    await page.send('Emulation.setDeviceMetricsOverride', { width: PAGE.width, height: PAGE.height, deviceScaleFactor: WIDTH / PAGE.width, mobile: false });
    await page.send('Emulation.setEmulatedMedia', { features: [{ name: 'prefers-color-scheme', value: 'dark' }] });
    // The player logos come from this checkout's docs/players, not from GitHub: the pictures show the logos as they
    // are here, and nothing is fetched.
    const logos = path.join(__dirname, '..', '..', 'docs', 'players');
    page.on('Fetch.requestPaused', (p) => {
      const file = path.join(logos, path.basename(new URL(p.request.url).pathname));
      if (!fs.existsSync(file)) return page.send('Fetch.failRequest', { requestId: p.requestId, errorReason: 'FileNotFound' }).catch(() => {});
      page.send('Fetch.fulfillRequest', {
        requestId: p.requestId, responseCode: 200, responseHeaders: [{ name: 'Content-Type', value: 'image/png' }],
        body: fs.readFileSync(file).toString('base64'),
      }).catch(() => {});
    });
    await page.send('Fetch.enable', { patterns: [{ urlPattern: '*/docs/players/*.png' }] });

    const js = async (expr) => {
      const r = await page.send('Runtime.evaluate', { expression: expr, awaitPromise: true, returnByValue: true });
      if (r.exceptionDetails) throw new Error(r.exceptionDetails.text + ' in ' + expr.slice(0, 60));
      return r.result.value;
    };
    const load = async () => {
      const done = page.once('Page.loadEventFired');
      await page.send('Page.navigate', { url: copy.base + '/?t=' + copy.token });
      await done;
      await waitFor('the settings page', () => js("!!document.querySelector('#tab-privacy')"));
    };
    const settle = async () => {
      await js('document.fonts.ready.then(() => true)');
      await waitFor('the pictures', () => js('[...document.images].every((i) => i.complete)'));
      const broken = await js("[...document.images].filter((i) => i.offsetParent && !i.naturalWidth).map((i) => i.src)");
      if (broken.length) throw new Error('pictures that did not load: ' + broken.join(', '));
      await sleep(800);
    };
    const shoot = async (name) => {
      const shot = await page.send('Page.captureScreenshot', { format: 'jpeg', quality: o.quality });
      const file = path.join(o.out, name);
      fs.writeFileSync(file, Buffer.from(shot.data, 'base64'));
      console.log(file + ' (' + fs.statSync(file).size + ' bytes)');
    };
    // scroll brings an element to the top of the panel that scrolls it, or (bottom) scrolls that panel to its
    // end.
    const scroll = (selector, bottom) => js(`(() => {
      const t = ${selector};
      let s = t.parentElement;
      while (s && !(s.scrollHeight > s.clientHeight && /(auto|scroll)/.test(getComputedStyle(s).overflowY))) s = s.parentElement;
      if (!s) throw new Error('nothing scrolls');
      s.scrollTop = ${bottom ? 's.scrollHeight' : 's.scrollTop + t.getBoundingClientRect().top - s.getBoundingClientRect().top'};
      return true;
    })()`);

    // window-main: the General tab with demo.js's made-up playback.
    await load();
    await js(fs.readFileSync(path.join(__dirname, 'demo.js'), 'utf8') + ';true');
    await waitFor('the sample playback', () => js("document.body.innerText.includes('Sample Show')"));
    await settle();
    await shoot('window-main.jpg');

    // window-privacy: a fresh page (not showing on Discord), Privacy tab at "Services that may see your titles".
    await load();
    await js("document.querySelector('#tab-privacy').click(), true");
    await scroll("document.querySelector('#fl-disabledSources')", false);
    await settle();
    await shoot('window-privacy.jpg');

    // window-plex: the Players tab scrolled to its end (Plex, Stremio, Jellyfin / Emby).
    await js("document.querySelector('#tab-players').click(), true");
    await scroll("document.querySelector('#p-players')", true);
    await settle();
    await shoot('window-plex.jpg');
  } finally {
    if (page) { await page.send('Browser.close').catch(() => {}); page.close(); }
    if (edge && edge.exitCode === null) await Promise.race([new Promise((r) => edge.once('exit', r)), sleep(5000)]);
    if (edge && edge.exitCode === null) edge.kill();
    if (copy) {
      await copy.api('quit', {}).catch(() => {});
      await Promise.race([new Promise((r) => copy.child.once('exit', r)), sleep(5000)]);
      if (copy.child.exitCode === null) {
        try {
          if (process.platform === 'win32') execFileSync('taskkill', ['/PID', String(copy.child.pid), '/F'], { stdio: 'ignore' });
          else process.kill(copy.child.pid);
        } catch (_) {}
      }
    }
    await rmrf(profile);
    await rmrf(home);
  }
}

main().catch((e) => { console.error(e.message || e); process.exit(1); });
