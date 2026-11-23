import { screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { vi } from 'vitest';
import { msg } from '../../lib/i18n';
import type { Settings } from '../../lib/services/settings';
import type { Source } from '../../lib/services/sources';
import { resetHistory } from '../../lib/stores/router';
import { settings } from '../../lib/stores/settings';
import { sourceProgress } from '../../lib/stores/sourceProgress';
import { refreshingAll, sources } from '../../lib/stores/sources';
import { toasts } from '../../lib/stores/toasts';

// Fixtures of the Sources page's dom tests. Each test file mocks the services itself (vi.mock is
// hoisted per file); this holds the sources and the queries they share.

export function source(patch: Partial<Source> = {}): Source {
  return {
    id: 's1',
    name: 'Main feed',
    type: 'json',
    url: 'https://example.invalid/feed.json',
    enabled: true,
    status: 'active',
    health: 'healthy',
    lastUpdatedAt: null,
    lastError: '',
    fingerprint: '',
    feedVersion: 1,
    entries: 10,
    invalid: 0,
    matched: 7,
    review: 2,
    unmatched: 1,
    createdAt: '',
    ...patch,
  } as Source;
}

export const MAIN = source();
export const MIRROR = source({ id: 's2', name: 'Mirror', url: 'https://mirror.invalid/feed.json', matched: 3, review: 0, entries: 4 });
export const OLD = source({ id: 's3', name: 'Old feed', url: 'https://old.invalid/feed.json', enabled: false, status: 'disabled' });

export const menuButton = (name: string) => screen.getByRole('button', { name: msg('transfers.sourcesMenuFor', { name }) });

/** The row of a source: what holds its menu button. */
export function rowOf(name: string): HTMLElement {
  const row = menuButton(name).closest<HTMLElement>('[data-source-row], .row');
  if (!row) throw new Error(`no row for ${name}`);
  return row;
}

/** The names of the sources whose rows are shown, in their order (read off their menu buttons). */
export function rowNames(): string[] {
  const [before, after] = msg('transfers.sourcesMenuFor', { name: '\u0000' }).split('\u0000');
  return screen
    .queryAllByRole('button')
    .map((button) => button.getAttribute('aria-label') ?? '')
    .filter((label) => label.startsWith(before) && label.endsWith(after))
    .map((label) => label.slice(before.length, label.length - after.length));
}

/** Opens the row menu of `name` and picks `item` in it. */
export async function pick(keys: { click: (el: Element) => Promise<void> }, name: string, item: string): Promise<void> {
  await keys.click(menuButton(name));
  await keys.click(screen.getByRole('menuitem', { name: item }));
}

export const words = (el: Element | null | undefined) => (el?.textContent ?? '').replace(/\s+/g, ' ').trim();

export const toastsSaid = () => get(toasts).map((item) => [item.kind, item.message]);

export function acceptNotice(accepted: boolean): void {
  settings.set({ sourcesNoticeAccepted: accepted } as Settings);
}

/** The page's own container, as App mounts it. */
export function mountPoint(): HTMLDivElement {
  // jsdom has no ResizeObserver; the status filter measures its plate with one.
  vi.stubGlobal(
    'ResizeObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  const app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  return app;
}

/** The stores the screen reads, back as they start; the page empties. */
export function resetScreen(): void {
  vi.unstubAllGlobals();
  sources.set([]);
  refreshingAll.set(false);
  sourceProgress.set({});
  toasts.set([]);
  settings.set(null);
  resetHistory();
  document.body.replaceChildren();
}

export const dialogNamed = (name: string) => screen.getByRole('dialog', { name });
export const inDialog = (name: string) => within(dialogNamed(name));
