import { render } from 'svelte/server';
import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', () => ({ Events: { On: vi.fn(() => () => {}) }, Window: {} }));
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../stores/notifications', async (actual) => {
  const { writable } = await import('svelte/store');
  return { ...(await actual<typeof import('../stores/notifications')>()), notifications: writable([]) };
});
vi.mock('../stores/social', async (actual) => {
  const { writable } = await import('svelte/store');
  return { ...(await actual<typeof import('../stores/social')>()), incomingCount: writable(0) };
});
vi.mock('../stores/user', async (actual) => {
  const { writable } = await import('svelte/store');
  return { ...(await actual<typeof import('../stores/user')>()), isOffline: writable(false) };
});

import Topbar from './Topbar.svelte';
import { activeTheme } from '../stores/theme';
import { notifications, type Notification } from '../stores/notifications';
import { incomingCount } from '../stores/social';
import { authState, currentUser, isOffline } from '../stores/user';
import { msg } from '../i18n';
import type { Writable } from 'svelte/store';
import type { Theme } from '../services/theme';
import type { CurrentUser } from '../services/account';

type Fs = { readFileSync(path: URL, encoding: 'utf8'): string };
const fs = (globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): Fs } }).process.getBuiltinModule('node:fs');
const source = (file: string) => fs.readFileSync(new URL(file, import.meta.url), 'utf8');

const withLayout = (layout: Theme['layout']) => activeTheme.set({ id: 'x', layout } as unknown as Theme);
const html = () => render(Topbar, { props: {} } as never).body;
const ids = (body: string) => [...body.matchAll(/data-part="topbar-action" data-id="([a-z]+)"/g)].map((m) => m[1]);
const escape = (text: string) => text.replaceAll('&', '&amp;').replaceAll('"', '&quot;').replaceAll('<', '&lt;');
const notes = notifications as unknown as Writable<Notification[]>;
const incoming = incomingCount as unknown as Writable<number>;
const offline = isOffline as Writable<boolean>;

const user = {
  id: 'u1',
  username: 'neo',
  displayName: 'Neo Anderson',
  email: '',
  avatarUrl: '',
  bio: '',
  createdAt: '',
  profile: { appearance: { avatarFrame: 'neon', frameColor: '#00ff00' } },
} as unknown as CurrentUser;

afterEach(() => {
  activeTheme.set(null);
  notes.set([]);
  incoming.set(0);
  offline.set(false);
  currentUser.set(null);
  authState.set('bootstrapping');
});

// The chip: from the account part's opening tag to the end of its button.
function chip(body: string): string {
  const start = body.search(/<button[^>]*data-part="topbar-account"/);
  return start < 0 ? '' : body.slice(start, body.indexOf('</button>', start));
}

type El = { tag: string; attrs: string; noDrag: boolean };

// Every button and input in the header, with whether it or an element around it opts out of dragging.
function controls(body: string): El[] {
  const out: El[] = [];
  const stack: El[] = [];
  for (const m of body.matchAll(/<(\/?)([a-z][a-z0-9-]*)([^>]*?)(\/?)>/g)) {
    const [, close, tag, attrs, selfClose] = m;
    if (close) {
      while (stack.length > 0 && stack.pop()?.tag !== tag);
      continue;
    }
    const el = { tag, attrs, noDrag: /class="[^"]*\bno-drag\b/.test(attrs) || stack.some((e) => e.noDrag) };
    if (tag === 'button' || tag === 'input') out.push(el);
    if (!selfClose && !['input', 'img', 'br', 'hr', 'meta', 'link'].includes(tag)) stack.push(el);
  }
  return out;
}

describe('Topbar layout', () => {
  it('renders every part and every action in the default order', () => {
    activeTheme.set(null);
    const body = html();
    expect(ids(body)).toEqual(['bigpicture', 'notifications', 'friends']);
    for (const part of ['topbar', 'topbar-history', 'topbar-search', 'topbar-actions', 'topbar-account']) {
      expect(body).toContain(`data-part="${part}"`);
    }
    expect(body).not.toContain('data-part="sidebar-account"');
    expect(body).not.toMatch(/class="topbar[^"]*\bcentered\b/);
  });

  it('orders and hides actions', () => {
    withLayout({ topbar: { order: ['friends', 'bigpicture'], hidden: ['notifications'] } });
    expect(ids(html())).toEqual(['friends', 'bigpicture']);
  });

  it('centres the search', () => {
    withLayout({ topbar: { search: 'center' } });
    expect(html()).toMatch(/class="topbar[^"]*\bcentered\b/);
  });

  it('keeps the row the same with the sidebar on the right', () => {
    const left = html();
    withLayout({ sidebar: { side: 'right' } });
    expect(html()).toBe(left);
  });

  it('tabs through history, search, actions and the account chip in that order', () => {
    const body = html();
    const at = ['topbar-history', 'topbar-search', 'topbar-actions', 'topbar-account'].map((part) => body.indexOf(`data-part="${part}"`));
    expect(at.every((i) => i >= 0)).toBe(true);
    expect([...at].sort((a, b) => a - b)).toEqual(at);
    expect(body).not.toMatch(/tabindex="[1-9]/);
  });
});

describe('Topbar window behaviour', () => {
  it('leaves the window buttons to the title bar', () => {
    const body = html();
    expect(body).not.toContain('data-part="window-controls"');
    for (const label of [msg('ui.minimize'), msg('ui.maximize'), msg('common.close')]) {
      expect(body).not.toContain(`aria-label="${label}"`);
    }
  });

  it('drags the window by the row and by nothing that can be clicked', () => {
    const body = html();
    const header = /<header[^>]*data-part="topbar"[^>]*>/.exec(body)?.[0] ?? '';
    expect(header).toContain('--wails-draggable: drag');
    expect(header).not.toMatch(/\bno-drag\b/);
    const list = controls(body);
    expect(list.filter((c) => c.tag === 'button').length).toBeGreaterThanOrEqual(6);
    expect(list.filter((c) => c.tag === 'input')).toHaveLength(1);
    expect(list.filter((c) => !c.noDrag).map((c) => `<${c.tag}${c.attrs}>`)).toEqual([]);
  });
});

describe('Topbar islands', () => {
  it('draws the history, the search and the right group as islands rounded by the control radius', () => {
    const island = /\n {2}\.island \{([^}]*)\}/.exec(source('./Topbar.svelte'))?.[1] ?? '';
    expect(island).toContain('background: var(--island-fill)');
    expect(island).toContain('border-radius: var(--radius-control)');
    const body = html();
    expect(body).toMatch(/<div class="[^"]*\bisland\b[^"]*"[^>]*data-part="topbar-history"/);
    expect(body).toMatch(/<div class="[^"]*\bisland\b[^"]*"[^>]*>(<!--[^>]*-->|\s)*<div class="actions[^"]*" data-part="topbar-actions"/);
    const pill = /\n {2}\.search-wrap :global\(\.search\) \{([^}]*)\}/.exec(source('./TopbarSearch.svelte'))?.[1] ?? '';
    expect(pill).toContain('background: var(--island-fill)');
    expect(pill).toContain('border-radius: var(--radius-control)');
  });

  it('shows the badges', () => {
    notes.set([
      { id: 'a', title: 'A', text: '', route: 'library', terminal: true },
      { id: 'b', title: 'B', text: '', route: 'library', terminal: true },
    ]);
    incoming.set(3);
    const body = html();
    const slot = (id: string) => {
      const start = body.indexOf(`data-part="topbar-action" data-id="${id}"`);
      const next = body.indexOf('data-part="topbar-action"', start + 1);
      return body.slice(start, next < 0 ? undefined : next);
    };
    expect(slot('notifications')).toMatch(/<span class="badge[^"]*">2<\/span>/);
    expect(slot('friends')).toMatch(/<span class="badge[^"]*">3<\/span>/);
  });
});

describe('Topbar account chip', () => {
  it('shows a signed-in user with their frame, presence dot, name and handle', () => {
    authState.set('authenticated');
    currentUser.set(user);
    const part = chip(html());
    expect(part).toContain(escape('Neo Anderson'));
    expect(part).toContain('@neo');
    expect(part).toMatch(/class="frame frame-neon/);
    expect(part).toContain('--frame-color: #00ff00');
    expect(part).toMatch(/class="dot /);
  });

  it('marks the handle offline when the launcher is offline', () => {
    authState.set('authenticated');
    offline.set(true);
    currentUser.set(user);
    expect(chip(html())).toContain(escape(msg('ui.offlineSuffix')));
  });

  it('shows a guest as a guest without a presence dot', () => {
    authState.set('guest');
    const part = chip(html());
    expect(part).toContain(escape(msg('ui.guest')));
    expect(part).toContain(escape(msg('ui.noAccount')));
    expect(part).not.toMatch(/class="dot /);
  });

  it('rings the presence dot with the island colour', () => {
    const rule = /\n {2}\.avatar \{([^}]*)\}/.exec(source('./AccountMenu.svelte'))?.[1] ?? '';
    expect(rule).toContain('--avatar-ring: var(--bg-sidebar)');
  });

  it('opens the menu down from the chip, aligned to its right edge', () => {
    expect(source('./AccountMenu.svelte')).toMatch(/<DropdownMenu items=\{profileMenu\} placement="down" align="right"/);
  });
});
