import { render } from 'svelte/server';
import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', () => ({ Events: { On: vi.fn(() => () => {}) }, Window: {} }));
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../stores/user', async (actual) => {
  const { writable } = await import('svelte/store');
  return { ...(await actual<typeof import('../stores/user')>()), isOffline: writable(false) };
});

import Sidebar from './Sidebar.svelte';
import { activeTheme } from '../stores/theme';
import { cloudStatus } from '../stores/cloudsaves';
import { libraryGames } from '../stores/library';
import { selfUpdateDownloading, selfUpdateStatus } from '../stores/selfupdate';
import { authState, isOffline } from '../stores/user';
import { msg } from '../i18n';
import type { Writable } from 'svelte/store';
import type { Theme } from '../services/theme';
import type { CloudStatus } from '../services/cloudsaves';
import type { LibraryGame } from '../services/library';
import type { SelfUpdateStatus } from '../services/selfupdate';

type Fs = { readFileSync(path: URL, encoding: 'utf8'): string };
const fs = (globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): Fs } }).process.getBuiltinModule('node:fs');

const withLayout = (layout: Theme['layout']) => activeTheme.set({ id: 'x', layout } as unknown as Theme);
const html = () => render(Sidebar, { props: {} } as never).body;
const ids = (body: string) => [...body.matchAll(/data-part="sidebar-item" data-id="([a-z]+)"/g)].map((m) => m[1]);
const escape = (text: string) => text.replaceAll('&', '&amp;').replaceAll('"', '&quot;').replaceAll('<', '&lt;');
const offline = isOffline as Writable<boolean>;
const escapeRe = (text: string) => text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

afterEach(() => {
  activeTheme.set(null);
  cloudStatus.set(null);
  libraryGames.set([]);
  selfUpdateStatus.set({ state: 'idle', currentVersion: '' });
  selfUpdateDownloading.set(false);
  offline.set(false);
  authState.set('bootstrapping');
});

describe('Sidebar layout', () => {
  it('renders the grouped default order with the bottom items', () => {
    activeTheme.set(null);
    const body = html();
    expect(ids(body)).toEqual([
      'library', 'catalog', 'downloads', 'friends', 'activity', 'stats',
      'installed', 'diskusage', 'sources', 'history',
      'support', 'settings',
    ]);
    expect(body.match(/data-part="sidebar-separator"/g)).toHaveLength(1);
    expect(body).toContain('data-part="sidebar-nav"');
    expect(body).toContain('data-part="sidebar-bottom"');
    expect(body).toContain('data-part="sidebar-account"');
    expect(body).not.toMatch(/class="sidebar[^"]*\bright\b/);
  });

  it('names every nav item for assistive tech, also when the rail hides its label', () => {
    const items = html().match(/<button[^>]*data-part="sidebar-item"[^>]*>/g) ?? [];
    expect(items.length).toBeGreaterThan(10);
    for (const tag of items) expect(tag).toMatch(/aria-label="[^"]+"/);
  });

  it('follows a theme order, hides items and drops support', () => {
    withLayout({ sidebar: { order: ['stats', 'library'], hidden: ['catalog', 'support'] } });
    const list = ids(html());
    expect(list.slice(0, 2)).toEqual(['stats', 'library']);
    expect(list).not.toContain('catalog');
    expect(list).not.toContain('support');
    expect(list.at(-1)).toBe('settings');
  });

  it('marks the right side', () => {
    withLayout({ sidebar: { side: 'right' } });
    const body = html();
    expect(body).toMatch(/class="sidebar[^"]*\bright\b/);
    expect(body.match(/class="plate[ "]/g)).toHaveLength(1);
  });
});

describe('Sidebar island', () => {
  it('sets the brand name as the wordmark next to the logo', () => {
    const body = html();
    expect(body).toMatch(/<img[^>]*class="logo-mark/);
    expect(body).toMatch(/<span class="wordmark[^"]*">Typhon<\/span>/);
  });

  it('marks the active item with one plate for every item', () => {
    const body = html();
    expect(body.match(/class="plate[ "]/g)).toHaveLength(1);
    expect(body.match(/class="marker/g)).toBeNull();
  });

  it('draws the island from the island tokens', () => {
    const source = fs.readFileSync(new URL('./Sidebar.svelte', import.meta.url), 'utf8');
    const rule = /\n {2}\.sidebar \{([^}]*)\}/.exec(source)?.[1] ?? '';
    expect(rule).toContain('background: var(--island-fill)');
    expect(rule).toContain('border-radius: var(--radius-xl)');
    expect(rule).toMatch(/margin: var\(--island-gap\)/);
    expect(rule).toContain('width: var(--sidebar-w)');
  });

  // A short window (750px, a larger interface scale) must not push the bottom block or the status row
  // out of the island: the nav alone gives way and scrolls.
  it('lets only the nav shrink and scroll; the brand, the bottom block and the status row keep their height', () => {
    const sidebar = fs.readFileSync(new URL('./Sidebar.svelte', import.meta.url), 'utf8');
    const status = fs.readFileSync(new URL('./SidebarStatus.svelte', import.meta.url), 'utf8');
    const body = (source: string, selector: string) =>
      new RegExp(`\\n {2}${selector.replaceAll('.', '\\.')} \\{([^}]*)\\}`).exec(source)?.[1] ?? '';
    const nav = body(sidebar, 'nav');
    expect(nav).toMatch(/flex: 1 1 0/);
    expect(nav).toMatch(/min-height: 0/);
    expect(nav).toMatch(/overflow-y: scroll/);
    expect(nav).toMatch(/position: relative/);
    for (const [source, selector] of [[sidebar, '.logo'], [sidebar, '.bottom'], [status, '.status']] as const) {
      expect(body(source, selector), selector).toMatch(/flex: none/);
    }
    expect(body(sidebar, '.plate')).toMatch(/position: absolute/);
    expect(body(sidebar, '.sidebar')).not.toMatch(/overflow/);
  });
});

const games = [{ id: 'g1', title: 'Hollow' }, { id: 'g2', title: 'Celeste' }] as LibraryGame[];

function cloud(over: Partial<CloudStatus>): CloudStatus {
  return { enabled: true, signedIn: true, usedBytes: 0, quotaBytes: 0, maxBytes: 0, error: '', games: [], ...over };
}

const conflict = (gameId: string) => ({ gameId, state: 'conflict' as const, code: '', message: '', at: '', syncedAt: '' });

const cloudStates: { kind: string; status: CloudStatus; offline?: boolean; guest?: boolean; name: string; button: boolean }[] = [
  { kind: 'off', status: cloud({ enabled: false }), name: msg('cloud.disabled'), button: true },
  { kind: 'signIn', status: cloud({ signedIn: false }), guest: true, name: msg('cloud.not_signed_in'), button: true },
  { kind: 'account', status: cloud({ signedIn: false, error: 'cloud.token' }), name: msg('cloud.token'), button: false },
  { kind: 'error', status: cloud({ error: 'cloud.server' }), name: msg('cloud.server'), button: false },
  { kind: 'offline', status: cloud({}), offline: true, name: msg('ui.statusCloudOffline'), button: false },
  { kind: 'synced', status: cloud({}), name: msg('ui.statusCloudSynced'), button: false },
  { kind: 'pending', status: cloud({ pending: true }), name: msg('cloudsaves.statePending'), button: false },
  { kind: 'conflict', status: cloud({ games: [conflict('g1')] }), name: msg('ui.statusCloudConflict', { title: 'Hollow' }), button: true },
  {
    kind: 'conflict',
    status: cloud({ games: [conflict('g1'), conflict('g2')] }),
    name: msg('ui.statusCloudConflicts', { count: 2 }),
    button: true,
  },
  {
    kind: 'failed',
    status: cloud({ games: [{ ...conflict('g2'), state: 'failed' }] }),
    name: msg('ui.statusCloudFailed', { title: 'Celeste' }),
    button: true,
  },
];

const updateStates: { kind: string; status: SelfUpdateStatus; name: string; button: boolean }[] = [
  { kind: 'version', status: { state: 'idle', currentVersion: '1.4.1' }, name: msg('ui.versionLabel', { version: '1.4.1' }), button: false },
  {
    kind: 'available',
    status: { state: 'available', currentVersion: '1.4.1', availableVersion: '1.5.0' },
    name: msg('ui.statusUpdateDownload', { version: '1.5.0' }),
    button: true,
  },
  {
    kind: 'downloading',
    status: { state: 'downloading', currentVersion: '1.4.1', availableVersion: '1.5.0', totalBytes: 200, downloadedBytes: 50 },
    name: `${msg('ui.downloadingUpdate', { version: '1.5.0' })} · 25%`,
    button: false,
  },
  { kind: 'ready', status: { state: 'ready', currentVersion: '1.4.1', availableVersion: '1.5.0' }, name: msg('ui.restartAndUpdate'), button: true },
  {
    kind: 'failed',
    status: { state: 'available', currentVersion: '1.4.1', availableVersion: '1.5.0', error: 'x', errorCode: 'download' },
    name: msg('ui.statusUpdateRetry', { version: '1.5.0' }),
    button: true,
  },
];

// From the state's element up to the end of its screen-reader name.
function part(body: string, side: 'cloud' | 'update'): string {
  const start = body.search(new RegExp(`<(button|span)[^>]*data-${side}="`));
  if (start < 0) return '';
  const name = body.indexOf('<span class="sr-only', start);
  return body.slice(start, body.indexOf('</span>', name) + '</span>'.length);
}

describe('Sidebar status row', () => {
  for (const c of cloudStates) {
    for (const u of updateStates) {
      it(`shows cloud ${c.kind} (${c.name}) next to update ${u.kind}`, () => {
        libraryGames.set(games);
        cloudStatus.set(c.status);
        offline.set(Boolean(c.offline));
        authState.set(c.guest ? 'guest' : 'authenticated');
        selfUpdateStatus.set(u.status);
        const body = html();
        expect(body).toMatch(/class="status[^"]*"[^>]*role="group"/);

        const left = part(body, 'cloud');
        expect(left).toContain(`data-cloud="${c.kind}"`);
        expect(left.startsWith('<button')).toBe(c.button);
        expect(left).toContain(`title="${escape(c.name)}"`);
        expect(left).toMatch(new RegExp(`<span class="sr-only[^"]*">${escapeRe(escape(c.name))}</span>`));

        const right = part(body, 'update');
        expect(right).toContain(`data-update="${u.kind}"`);
        expect(right.startsWith('<button')).toBe(u.button);
        expect(right).toMatch(new RegExp(`<span class="sr-only[^"]*">${escapeRe(escape(u.name))}</span>`));
        expect(right).toMatch(/title="[^"]+"/);
      });
    }
  }

  it('shows no cloud state before the cloud reports one, and no version before it is known', () => {
    const body = html();
    expect(body).not.toContain('data-cloud=');
    expect(body).not.toContain('data-update=');
  });

  it('keeps every name when the island collapses to icons: only the short words go', () => {
    const source = fs.readFileSync(new URL('./SidebarStatus.svelte', import.meta.url), 'utf8');
    const collapsed = /@media \(max-width: 1140px\) \{([\s\S]*)\}\s*<\/style>/.exec(source)?.[1] ?? '';
    expect(collapsed).toMatch(/\.short[^{]*\{[^}]*display: none/);
    expect(collapsed).not.toMatch(/\.sr-only[^{]*\{[^}]*display: none/);
    expect(collapsed).not.toMatch(/\[data-(cloud|update)[^{]*\{[^}]*display: none/);
  });
});
