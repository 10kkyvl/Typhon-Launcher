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
import { libraryGames, runningGames } from '../stores/library';
import { shownPresence } from '../stores/presence';
import { STATUS_LABELS } from '../social/presence';
import { msg } from '../i18n';
import type { LibraryGame } from '../services/library';
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
  shownPresence.set('online');
  runningGames.set(new Set());
  libraryGames.set([]);
});

const signedIn = () => {
  authState.set('authenticated');
  currentUser.set(user);
};
const dot = (part: string) => /<span class="dot ([a-z]+)/.exec(part)?.[1];
const line = (part: string) =>
  (/<span class="chip-status[^"]*">([\s\S]*?)<\/span>/.exec(part)?.[1] ?? '').replace(/<!--[\s\S]*?-->/g, '');

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

  // The notifications badge is held by NotificationsPanel.dom.test.ts through the bell's name.
  it('shows the friend requests badge', () => {
    incoming.set(3);
    const body = html();
    const slot = (id: string) => {
      const start = body.indexOf(`data-part="topbar-action" data-id="${id}"`);
      const next = body.indexOf('data-part="topbar-action"', start + 1);
      return body.slice(start, next < 0 ? undefined : next);
    };
    expect(slot('friends')).toMatch(/<span class="badge[^"]*">3<\/span>/);
  });
});

describe('Topbar account chip', () => {
  it('shows a signed-in user online with their frame, name and presence, without the handle', () => {
    signedIn();
    const part = chip(html());
    expect(part).toContain(escape('Neo Anderson'));
    expect(line(part)).toBe(escape(STATUS_LABELS.online));
    expect(dot(part)).toBe('online');
    expect(part).not.toContain('@neo');
    expect(part).toMatch(/class="frame frame-neon/);
    expect(part).toContain('--frame-color: #00ff00');
  });

  it('shows an away user as away', () => {
    signedIn();
    shownPresence.set('away');
    const part = chip(html());
    expect(line(part)).toBe(escape(STATUS_LABELS.away));
    expect(dot(part)).toBe('away');
  });

  it('names the running game with the in-game dot, and returns to the status when it stops', () => {
    signedIn();
    shownPresence.set('away');
    libraryGames.set([{ id: 'g1', title: 'Hollow Knight' }] as LibraryGame[]);
    runningGames.set(new Set(['g1']));
    const playing = chip(html());
    expect(line(playing)).toBe(escape(msg('social.playingNamed', { name: 'Hollow Knight' })));
    expect(dot(playing)).toBe('online');
    runningGames.set(new Set());
    const stopped = chip(html());
    expect(line(stopped)).toBe(escape(STATUS_LABELS.away));
    expect(dot(stopped)).toBe('away');
  });

  it('keeps the offline mark after the presence when the launcher is offline', () => {
    signedIn();
    offline.set(true);
    const part = chip(html());
    expect(line(part)).toContain(escape(STATUS_LABELS.online));
    expect(part).toContain(escape(msg('ui.offlineSuffix')));
  });

  it('shows a guest as a guest without a presence dot', () => {
    authState.set('guest');
    const part = chip(html());
    expect(part).toContain(escape(msg('ui.guest')));
    expect(line(part)).toBe(escape(msg('ui.noAccount')));
    expect(dot(part)).toBeUndefined();
  });

  it('rings the presence dot with the island colour', () => {
    const rule = /\n {2}\.avatar \{([^}]*)\}/.exec(source('./AccountMenu.svelte'))?.[1] ?? '';
    expect(rule).toContain('--avatar-ring: var(--bg-sidebar)');
  });
});
