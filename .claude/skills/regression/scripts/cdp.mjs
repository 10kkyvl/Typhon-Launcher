#!/usr/bin/env node
// Drives the real Typhon window of a qa build over the WebView2 DevTools port.
// Node 22+, no dependencies (global WebSocket and fetch).
//
//   node cdp.mjs targets [--json]
//   node cdp.mjs eval '<js>' | eval -          (expression from stdin with "-")
//   node cdp.mjs text [--limit N]
//   node cdp.mjs click '<visible text>' [--nth N] [--aria]   (--aria: exact aria-label, for switches and icon buttons)
//   node cdp.mjs fill '<css selector>' '<value>'
//   node cdp.mjs wait '<text>' [timeoutMs] [--gone]
//   node cdp.mjs shot <file.png>
//   node cdp.mjs console [ms] [--fail-on-error]
//
// Options for every command: --target <substring of title or url> picks the
// page (default: the first http page, which is the main window), --connect-ms N
// keeps retrying the connection for N ms (default 5000).
// Port: TYPHON_QA_CDP_PORT (default 9333), host is always 127.0.0.1.
// "eval" runs the given JavaScript in the launcher window by design: this is a
// test driver for a local qa build, and it connects to loopback only.
//
// Exit codes: 0 ok, 1 the check failed (no such text, timeout, js threw,
// console errors with --fail-on-error), 2 usage, 3 no DevTools endpoint or
// no matching page, 4 protocol error or timeout talking to the page.

import { mkdir, writeFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';

const EXIT_FAIL = 1;
const EXIT_USAGE = 2;
const EXIT_CONNECT = 3;
const EXIT_PROTOCOL = 4;

class Fail extends Error {
  constructor(code, message) {
    super(message);
    this.code = code;
  }
}

// Wails asks for an optional /wails/custom.js that the launcher does not ship;
// the 404 is on every start and says nothing about the build under test.
const BENIGN_LOG_URLS = ['/wails/custom.js'];

const VALUE_FLAGS = new Set(['limit', 'nth', 'target', 'connect-ms']);
const BOOL_FLAGS = new Set(['json', 'gone', 'fail-on-error', 'aria']);

function parseArgs(argv) {
  const pos = [];
  const flags = {};
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a.startsWith('--') && a.length > 2) {
      const name = a.slice(2);
      if (VALUE_FLAGS.has(name)) {
        if (i + 1 >= argv.length) throw new Fail(EXIT_USAGE, `--${name} needs a value`);
        flags[name] = argv[++i];
      } else if (BOOL_FLAGS.has(name)) {
        flags[name] = true;
      } else {
        throw new Fail(EXIT_USAGE, `unknown option --${name}`);
      }
    } else {
      pos.push(a);
    }
  }
  return { pos, flags };
}

function intFlag(flags, name, def, min) {
  if (flags[name] === undefined) return def;
  const n = Number(flags[name]);
  if (!Number.isInteger(n) || n < min) throw new Fail(EXIT_USAGE, `--${name} must be an integer >= ${min}, got "${flags[name]}"`);
  return n;
}

function intArg(raw, def, min, what) {
  if (raw === undefined) return def;
  const n = Number(raw);
  if (!Number.isInteger(n) || n < min) throw new Fail(EXIT_USAGE, `${what} must be an integer >= ${min}, got "${raw}"`);
  return n;
}

function qaPort() {
  const raw = process.env.TYPHON_QA_CDP_PORT;
  if (raw === undefined) return 9333;
  const n = Number(raw);
  if (!Number.isInteger(n) || n < 1024 || n > 65535) {
    throw new Fail(EXIT_USAGE, `TYPHON_QA_CDP_PORT="${raw}" must be an integer in 1024..65535`);
  }
  return n;
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function listTargets(port, connectMs) {
  const url = `http://127.0.0.1:${port}/json/list`;
  const deadline = Date.now() + connectMs;
  let last;
  for (;;) {
    try {
      const res = await fetch(url, { signal: AbortSignal.timeout(2000) });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const list = await res.json();
      if (Array.isArray(list) && list.some((t) => t.type === 'page')) return list;
      last = new Error('no page targets yet');
    } catch (e) {
      last = e;
    }
    if (Date.now() >= deadline) break;
    await sleep(250);
  }
  throw new Fail(
    EXIT_CONNECT,
    `no DevTools endpoint at ${url} (${last?.cause?.code ?? last?.message ?? last}); ` +
      'is the qa build running? build it with "wails3 task build:qa" and start bin/typhon.exe',
  );
}

function pickTarget(list, substr) {
  const pages = list.filter((t) => t.type === 'page' && t.webSocketDebuggerUrl);
  if (substr) {
    const hit = pages.find((t) => `${t.title} ${t.url}`.includes(substr));
    if (!hit) {
      throw new Fail(EXIT_CONNECT, `no page matches --target "${substr}"; pages: ${pages.map((t) => shortUrl(t.url)).join(', ') || 'none'}`);
    }
    return hit;
  }
  const hit = pages.find((t) => /^https?:/.test(t.url)) ?? pages[0];
  if (!hit) throw new Fail(EXIT_CONNECT, 'the DevTools endpoint lists no page targets');
  return hit;
}

function shortUrl(u, max = 80) {
  const s = String(u ?? '');
  return s.length > max ? `${s.slice(0, max)}…(${s.length} chars)` : s;
}

class Session {
  #id = 0;
  #pending = new Map();
  #handlers = new Map();
  #ws;
  #closed = false;

  constructor(ws) {
    this.#ws = ws;
    ws.addEventListener('message', (ev) => this.#onMessage(ev.data));
    ws.addEventListener('close', () => this.#onClose());
    ws.addEventListener('error', () => this.#onClose());
  }

  static open(url) {
    return new Promise((res, rej) => {
      const ws = new WebSocket(url);
      const timer = setTimeout(() => rej(new Fail(EXIT_CONNECT, `timeout opening ${url}`)), 5000);
      ws.addEventListener('open', () => {
        clearTimeout(timer);
        res(new Session(ws));
      }, { once: true });
      ws.addEventListener('error', () => {
        clearTimeout(timer);
        rej(new Fail(EXIT_CONNECT, `cannot open ${url}`));
      }, { once: true });
    });
  }

  #onMessage(data) {
    let msg;
    try {
      msg = JSON.parse(typeof data === 'string' ? data : String(data));
    } catch {
      return;
    }
    if (msg.id !== undefined) {
      const p = this.#pending.get(msg.id);
      if (!p) return;
      this.#pending.delete(msg.id);
      clearTimeout(p.timer);
      if (msg.error) p.rej(new Fail(EXIT_PROTOCOL, `${p.method}: ${msg.error.message} (${msg.error.code})`));
      else p.res(msg.result ?? {});
      return;
    }
    for (const fn of this.#handlers.get(msg.method) ?? []) fn(msg.params ?? {});
  }

  #onClose() {
    this.#closed = true;
    for (const [id, p] of this.#pending) {
      clearTimeout(p.timer);
      p.rej(new Fail(EXIT_PROTOCOL, `${p.method}: connection closed`));
      this.#pending.delete(id);
    }
  }

  send(method, params = {}, timeoutMs = 15000) {
    if (this.#closed) return Promise.reject(new Fail(EXIT_PROTOCOL, `${method}: connection closed`));
    const id = ++this.#id;
    return new Promise((res, rej) => {
      const timer = setTimeout(() => {
        this.#pending.delete(id);
        rej(new Fail(EXIT_PROTOCOL, `${method}: no answer in ${timeoutMs} ms`));
      }, timeoutMs);
      this.#pending.set(id, { res, rej, timer, method });
      this.#ws.send(JSON.stringify({ id, method, params }));
    });
  }

  on(method, fn) {
    const list = this.#handlers.get(method) ?? [];
    list.push(fn);
    this.#handlers.set(method, list);
  }

  close() {
    try {
      this.#ws.close();
    } catch {
      // already closed
    }
  }
}

async function evaluate(s, expression) {
  const r = await s.send('Runtime.evaluate', {
    expression,
    awaitPromise: true,
    returnByValue: true,
    userGesture: true,
  });
  if (r.exceptionDetails) {
    const d = r.exceptionDetails;
    const text = d.exception?.description ?? d.exception?.value ?? d.text;
    throw new Fail(EXIT_FAIL, `js threw: ${text}`);
  }
  return r.result;
}

async function pageText(s) {
  const r = await evaluate(s, 'document.body ? document.body.innerText : ""');
  return typeof r.value === 'string' ? r.value : '';
}

async function readStdin() {
  const chunks = [];
  for await (const c of process.stdin) chunks.push(c);
  return Buffer.concat(chunks).toString('utf8');
}

function formatValue(r) {
  if (r.type === 'undefined') return 'undefined';
  if ('value' in r) return typeof r.value === 'string' ? r.value : JSON.stringify(r.value, null, 2);
  if (r.unserializableValue) return r.unserializableValue;
  return r.description ?? String(r.type);
}

// Runs inside the page. Returns where to click, or why there is nothing to click.
function locate(query, nth, aria) {
  const norm = (s) => String(s ?? '').replace(/\s+/g, ' ').trim();
  const q = norm(query);
  const ql = q.toLowerCase();
  const skip = new Set(['SCRIPT', 'STYLE', 'NOSCRIPT', 'HEAD', 'META', 'LINK', 'TEMPLATE', 'HTML']);

  const visible = (el) => {
    const r = el.getBoundingClientRect();
    if (r.width < 1 || r.height < 1) return false;
    const cs = getComputedStyle(el);
    return cs.visibility === 'visible' && cs.display !== 'none' && Number(cs.opacity) !== 0 && cs.pointerEvents !== 'none';
  };
  const clickable = (el) => {
    const roles = new Set(['button', 'tab', 'link', 'menuitem', 'option', 'checkbox', 'switch', 'radio', 'treeitem']);
    const tags = new Set(['A', 'BUTTON', 'INPUT', 'SELECT', 'TEXTAREA', 'SUMMARY', 'LABEL', 'OPTION']);
    let n = el;
    for (let i = 0; n && n !== document.body && i < 6; i++, n = n.parentElement) {
      if (tags.has(n.tagName) || roles.has(n.getAttribute('role')) || n.hasAttribute('onclick')) return true;
      const ti = n.getAttribute('tabindex');
      if (ti !== null && Number(ti) >= 0) return true;
      if (getComputedStyle(n).cursor === 'pointer') return true;
    }
    return false;
  };
  const desc = (el) => {
    const id = el.id ? `#${el.id}` : '';
    const cls = typeof el.className === 'string' && el.className.trim() ? `.${el.className.trim().split(/\s+/).slice(0, 2).join('.')}` : '';
    return `${el.tagName.toLowerCase()}${id}${cls}`;
  };

  const all = [];
  for (const el of document.querySelectorAll('body, body *')) {
    if (skip.has(el.tagName)) continue;
    const label = `${el.getAttribute('aria-label') ?? ''} ${el.getAttribute('title') ?? ''}`.toLowerCase();
    if (aria) {
      if (norm(el.getAttribute('aria-label')).toLowerCase() !== ql) continue;
    } else if (!(el.textContent ?? '').toLowerCase().includes(ql) && !label.includes(ql)) continue;
    if (!visible(el)) continue;
    all.push({ el, text: norm(el.innerText ?? el.textContent), aria: norm(el.getAttribute('aria-label')), title: norm(el.getAttribute('title')) });
  }

  const tiers = aria ? [['aria', () => true]] : [
    ['exact', (c) => c.text === q],
    ['exact-ignore-case', (c) => c.text.toLowerCase() === ql],
    ['includes', (c) => c.text.toLowerCase().includes(ql)],
    ['label', (c) => c.aria.toLowerCase().includes(ql) || c.title.toLowerCase().includes(ql)],
  ];

  let tier = null;
  let matched = [];
  for (const [name, test] of tiers) {
    const hit = all.filter(test);
    const innermost = hit.filter((c) => !hit.some((o) => o !== c && c.el.contains(o.el)));
    if (innermost.length) {
      tier = name;
      matched = innermost;
      break;
    }
  }

  const rank = matched.map((c) => ({ ...c, click: clickable(c.el) }));
  rank.sort((a, b) => Number(b.click) - Number(a.click));
  const list = rank.slice(0, 10).map((c) => ({ tag: desc(c.el), text: c.text.slice(0, 60), clickable: c.click }));

  if (!rank.length) {
    const near = [];
    for (const el of document.querySelectorAll('button, a, [role=button], [role=tab], [tabindex], input[type=button], input[type=submit]')) {
      if (!visible(el)) continue;
      const t = norm(el.innerText || el.getAttribute('aria-label') || el.value);
      if (t) near.push(t.slice(0, 40));
      if (near.length >= 25) break;
    }
    return { ok: false, reason: 'no visible element with that text', visibleClickables: near };
  }
  if (nth >= rank.length) {
    return { ok: false, reason: `--nth ${nth} but only ${rank.length} candidate(s)`, candidates: list };
  }

  const pick = rank[nth];
  pick.el.scrollIntoView({ block: 'center', inline: 'center', behavior: 'instant' });
  const r = pick.el.getBoundingClientRect();
  const x = Math.round(r.left + r.width / 2);
  const y = Math.round(r.top + r.height / 2);
  const top = document.elementFromPoint(x, y);
  if (!top || !(top === pick.el || pick.el.contains(top) || top.contains(pick.el))) {
    return { ok: false, reason: `covered at (${x},${y}) by ${top ? desc(top) : 'nothing'}`, candidates: list };
  }
  return { ok: true, x, y, tier, count: rank.length, picked: { tag: desc(pick.el), text: pick.text.slice(0, 60), clickable: pick.click }, candidates: list };
}

// Runs inside the page. Svelte 5 bind:value listens to input events and reads
// the element's value, so the value goes through the native setter that React-
// and Svelte-style wrappers do not shadow.
function assign(selector, value) {
  const el = document.querySelector(selector);
  if (!el) return { ok: false, reason: `no element matches ${selector}` };
  if (el.disabled || el.readOnly) return { ok: false, reason: `${selector} is ${el.disabled ? 'disabled' : 'read-only'}` };
  el.focus();
  if (el.isContentEditable) {
    el.textContent = value;
  } else {
    const proto = el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype
      : el instanceof HTMLSelectElement ? HTMLSelectElement.prototype
        : el instanceof HTMLInputElement ? HTMLInputElement.prototype : null;
    if (!proto) return { ok: false, reason: `${selector} is a <${el.tagName.toLowerCase()}>, not a form field` };
    const setter = Object.getOwnPropertyDescriptor(proto, 'value')?.set;
    if (!setter) return { ok: false, reason: `${selector} has no value setter` };
    setter.call(el, value);
  }
  el.dispatchEvent(new Event('input', { bubbles: true }));
  el.dispatchEvent(new Event('change', { bubbles: true }));
  return { ok: true, value: el.isContentEditable ? el.textContent : el.value };
}

const commands = {
  async targets({ pos, flags, port }) {
    if (pos.length) throw new Fail(EXIT_USAGE, 'targets takes no arguments');
    const list = await listTargets(port, intFlag(flags, 'connect-ms', 5000, 0));
    const chosen = pickTarget(list, flags.target);
    if (flags.json) {
      console.log(JSON.stringify(list.map((t) => ({ ...t, url: shortUrl(t.url, 200), title: shortUrl(t.title, 200), devtoolsFrontendUrl: undefined, default: t.id === chosen.id })), null, 2));
      return 0;
    }
    for (const t of list) {
      console.log(`${t.id === chosen.id ? '*' : ' '} ${t.id}  ${t.type}  ${shortUrl(t.title, 50)}  ${shortUrl(t.url)}`);
    }
    return 0;
  },

  async eval({ pos, session }) {
    if (pos.length !== 1) throw new Fail(EXIT_USAGE, 'usage: eval <js> | eval -');
    const expression = pos[0] === '-' ? await readStdin() : pos[0];
    console.log(formatValue(await evaluate(session, expression)));
    return 0;
  },

  async text({ pos, flags, session }) {
    if (pos.length) throw new Fail(EXIT_USAGE, 'usage: text [--limit N]');
    const limit = intFlag(flags, 'limit', 4000, 1);
    const text = (await pageText(session)).trim();
    console.log(text.length > limit ? `${text.slice(0, limit)}\n… [truncated: ${text.length - limit} more chars, raise --limit]` : text);
    return 0;
  },

  async click({ pos, flags, session }) {
    if (pos.length !== 1 || !pos[0].trim()) throw new Fail(EXIT_USAGE, 'usage: click <visible text> [--nth N] [--aria]');
    const nth = intFlag(flags, 'nth', 0, 0);
    const found = (await evaluate(session, `(${locate})(${JSON.stringify(pos[0])}, ${nth}, ${Boolean(flags.aria)})`)).value;
    if (!found?.ok) {
      const extra = found?.candidates ? `\ncandidates: ${JSON.stringify(found.candidates)}` : found?.visibleClickables ? `\nvisible clickables: ${JSON.stringify(found.visibleClickables)}` : '';
      throw new Fail(EXIT_FAIL, `click "${pos[0]}": ${found?.reason ?? 'locate failed'}${extra}`);
    }
    const at = { x: found.x, y: found.y, button: 'left', pointerType: 'mouse' };
    await session.send('Input.dispatchMouseEvent', { type: 'mouseMoved', x: found.x, y: found.y, pointerType: 'mouse' });
    await session.send('Input.dispatchMouseEvent', { type: 'mousePressed', clickCount: 1, buttons: 1, ...at });
    await session.send('Input.dispatchMouseEvent', { type: 'mouseReleased', clickCount: 1, buttons: 0, ...at });
    const others = found.count > 1 ? `; ${found.count - 1} more candidate(s), use --nth to choose: ${JSON.stringify(found.candidates)}` : '';
    console.log(`clicked ${found.picked.tag} "${found.picked.text}" at (${found.x},${found.y}) [${found.tier}, ${found.picked.clickable ? 'clickable' : 'not marked clickable'}]${others}`);
    return 0;
  },

  async fill({ pos, session }) {
    if (pos.length !== 2) throw new Fail(EXIT_USAGE, 'usage: fill <css selector> <value>');
    const res = (await evaluate(session, `(${assign})(${JSON.stringify(pos[0])}, ${JSON.stringify(pos[1])})`)).value;
    if (!res?.ok) throw new Fail(EXIT_FAIL, `fill ${pos[0]}: ${res?.reason ?? 'failed'}`);
    console.log(`filled ${pos[0]} = ${JSON.stringify(res.value)}`);
    return 0;
  },

  async wait({ pos, flags, session }) {
    if (pos.length < 1 || pos.length > 2 || !pos[0]) throw new Fail(EXIT_USAGE, 'usage: wait <text> [timeoutMs] [--gone]');
    const timeout = intArg(pos[1], 10000, 1, 'timeoutMs');
    const deadline = Date.now() + timeout;
    let text = '';
    for (;;) {
      text = await pageText(session);
      if (text.includes(pos[0]) !== Boolean(flags.gone)) {
        console.log(flags.gone ? `gone: "${pos[0]}"` : `found: "${pos[0]}"`);
        return 0;
      }
      if (Date.now() >= deadline) break;
      await sleep(250);
    }
    const tail = text.trim().slice(0, 300).replace(/\s+/g, ' ');
    throw new Fail(EXIT_FAIL, `timeout after ${timeout} ms waiting for text ${flags.gone ? 'to disappear' : 'to appear'}: "${pos[0]}"; page starts with: ${tail}`);
  },

  async shot({ pos, session }) {
    if (pos.length !== 1) throw new Fail(EXIT_USAGE, 'usage: shot <file.png>');
    const file = resolve(pos[0]);
    const { data } = await session.send('Page.captureScreenshot', { format: 'png', fromSurface: true }, 20000);
    const buf = Buffer.from(data, 'base64');
    await mkdir(dirname(file), { recursive: true });
    await writeFile(file, buf);
    console.log(`${file} (${buf.length} bytes)`);
    return 0;
  },

  async console({ pos, flags, session }) {
    if (pos.length > 1) throw new Fail(EXIT_USAGE, 'usage: console [ms] [--fail-on-error]');
    const ms = intArg(pos[0], 3000, 0, 'ms');
    const lines = [];
    let errors = 0;
    const show = (a) => (a.value !== undefined ? (typeof a.value === 'string' ? a.value : JSON.stringify(a.value)) : a.description ?? a.type);
    session.on('Runtime.consoleAPICalled', (p) => {
      if (p.type === 'error' || p.type === 'assert') errors++;
      lines.push(`[${p.type}] ${p.args.map(show).join(' ')}`);
    });
    session.on('Runtime.exceptionThrown', (p) => {
      errors++;
      const d = p.exceptionDetails;
      lines.push(`[exception] ${d.exception?.description ?? d.text} (${shortUrl(d.url, 60)}:${d.lineNumber}:${d.columnNumber})`);
    });
    session.on('Log.entryAdded', (p) => {
      const benign = BENIGN_LOG_URLS.some((u) => (p.entry.url ?? '').endsWith(u));
      if (p.entry.level === 'error' && !benign) errors++;
      lines.push(`[log.${p.entry.level}${benign ? ' ignored' : ''}] ${p.entry.source}: ${p.entry.text}${p.entry.url ? ` (${shortUrl(p.entry.url, 60)})` : ''}`);
    });
    await session.send('Log.enable');
    await session.send('Runtime.enable');
    await sleep(ms);
    console.log(lines.length ? lines.join('\n') : `(no console output in ${ms} ms)`);
    if (errors && flags['fail-on-error']) throw new Fail(EXIT_FAIL, `${errors} console error(s) or exception(s)`);
    return 0;
  },
};

async function main() {
  const [cmd, ...rest] = process.argv.slice(2);
  if (!cmd || cmd === 'help' || cmd === '--help' || cmd === '-h') {
    console.error('usage: node cdp.mjs <targets|eval|text|click|fill|wait|shot|console> [args] (see the header of this file)');
    return cmd ? 0 : EXIT_USAGE;
  }
  const run = commands[cmd];
  if (!run) throw new Fail(EXIT_USAGE, `unknown command "${cmd}"`);
  const { pos, flags } = parseArgs(rest);
  const port = qaPort();
  if (cmd === 'targets') return run({ pos, flags, port });
  const list = await listTargets(port, intFlag(flags, 'connect-ms', 5000, 0));
  const target = pickTarget(list, flags.target);
  const session = await Session.open(target.webSocketDebuggerUrl);
  try {
    return await run({ pos, flags, port, session });
  } finally {
    session.close();
  }
}

try {
  process.exitCode = await main();
} catch (e) {
  if (e instanceof Fail) {
    console.error(`error: ${e.message}`);
    process.exitCode = e.code;
  } else {
    console.error(`error: ${e?.stack ?? e}`);
    process.exitCode = EXIT_PROTOCOL;
  }
}
setTimeout(() => process.exit(process.exitCode ?? 0), 1000).unref();
