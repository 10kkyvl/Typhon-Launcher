import { screen, waitFor, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/downloads', async (actual) => ({
  ...(await actual<typeof import('../../lib/services/downloads')>()),
  cancelDownload: vi.fn(),
  removeDownload: vi.fn(),
  resumeDownload: vi.fn(),
  getDownload: vi.fn(() => new Promise(() => {})),
}));

import { msg } from '../../lib/i18n';
import { cancelDownload, removeDownload, resumeDownload, type Download } from '../../lib/services/downloads';
import { downloads } from '../../lib/stores/downloads';
import { installations } from '../../lib/stores/install';
import { resetHistory } from '../../lib/stores/router';
import { toasts } from '../../lib/stores/toasts';
import { renderDom } from '../../lib/testing/dom';
import Downloads from './Downloads.svelte';

function download(patch: Partial<Download> = {}): Download {
  return {
    id: 'd1',
    name: 'Game One',
    type: 'torrent',
    source: 'magnet:?xt=urn:btih:hash1',
    infoHash: 'hash1',
    destination: 'C:\\Games\\d1',
    status: 'completed',
    progress: 1,
    downloaded: 0,
    total: 1000,
    downloadSpeed: 0,
    uploadSpeed: 0,
    etaSeconds: -1,
    seeders: 0,
    peers: 0,
    files: [],
    seeding: false,
    stalled: false,
    stalledSince: null,
    addedAt: '2026-09-08T00:00:00Z',
    completedAt: '2026-09-08T01:00:00Z',
    error: '',
    origin: {},
    ...patch,
  };
}

const goRemoves = (id: string) => downloads.update((list) => list.filter((d) => d.id !== id));
const goSets = (id: string, patch: Partial<Download>) => downloads.update((list) => list.map((d) => (d.id === id ? { ...d, ...patch } : d)));

/** The screen's one live region and what it says now, one node per saying. */
function region(): HTMLElement {
  const found = [...document.querySelectorAll<HTMLElement>('[aria-live="polite"]')].filter((el) => !el.closest('.notice, .toast'));
  expect(found).toHaveLength(1);
  return found[0];
}

async function saidOnce(text: string) {
  await waitFor(() => expect(region().textContent?.trim()).toBe(text));
  expect(region().children).toHaveLength(1);
}

async function pick(user: ReturnType<typeof renderDom>['user'], trigger: HTMLElement, item: string) {
  trigger.focus();
  await user.keyboard('{Enter}');
  const entry = screen.getByRole('menuitem', { name: item });
  entry.focus();
  await user.keyboard('{Enter}');
}

beforeEach(() => {
  for (const fn of [cancelDownload, removeDownload, resumeDownload]) {
    vi.mocked(fn).mockReset();
    vi.mocked(fn).mockResolvedValue(undefined);
  }
});

afterEach(() => {
  downloads.set([]);
  installations.set([]);
  toasts.set([]);
  resetHistory();
  document.body.replaceChildren();
});

describe('Downloads: every part speaks in the screen’s one live region', () => {
  it('an active panel: a cancel is said once', async () => {
    vi.mocked(cancelDownload).mockImplementation(async (id) => goRemoves(id));
    downloads.set([download({ status: 'downloading', progress: 0, completedAt: null })]);
    const { user } = renderDom(Downloads);

    const panel = screen.getByRole('region', { name: 'Game One' });
    await pick(user, within(panel).getByRole('button', { name: msg('downloads.panel.more', { name: 'Game One' }) }), msg('downloads.panel.cancel'));

    await saidOnce(msg('downloads.cancel.done', { name: 'Game One' }));
  });

  it('the queue: a cancel is said once', async () => {
    vi.mocked(cancelDownload).mockImplementation(async (id) => goRemoves(id));
    downloads.set([download({ status: 'queued', progress: 0, completedAt: null })]);
    const { user } = renderDom(Downloads);

    await user.click(screen.getByRole('button', { name: msg('downloads.queue.cancelFor', { title: 'Game One' }) }));

    await saidOnce(msg('downloads.cancel.done', { name: 'Game One' }));
  });

  it('the queue: its menu offers the details, which open the details dialog', async () => {
    downloads.set([download({ status: 'queued', progress: 0, completedAt: null })]);
    const { user } = renderDom(Downloads);

    await pick(user, screen.getByRole('button', { name: msg('transfers.downloadsQueueActionsFor', { name: 'Game One' }) }), msg('downloads.queue.details'));

    expect(await screen.findByRole('dialog', { name: 'Game One' })).toBeTruthy();
  });

  it('the failed list: a retry is said once', async () => {
    vi.mocked(resumeDownload).mockImplementation(async (id) => goSets(id, { status: 'queued', error: '' }));
    downloads.set([download({ status: 'failed', error: 'typhon:download.no_free_space', completedAt: null })]);
    const { user } = renderDom(Downloads);

    await user.click(screen.getByRole('button', { name: msg('downloads.failed.retryFor', { title: 'Game One' }) }));

    await saidOnce(msg('downloads.failed.retried', { title: 'Game One' }));
  });

  it('the completed section: a removal from the list is said once', async () => {
    vi.mocked(removeDownload).mockImplementation(async (id) => goRemoves(id));
    downloads.set([download(), download({ id: 'd2', name: 'Game Two' })]);
    const { user } = renderDom(Downloads);

    await pick(user, screen.getByRole('button', { name: msg('downloads.panel.more', { name: 'Game One' }) }), msg('downloads.done.remove'));

    await saidOnce(msg('downloads.done.removed', { name: 'Game One' }));
  });
});
