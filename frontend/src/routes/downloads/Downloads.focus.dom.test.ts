import { screen, waitFor, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/downloads', async (actual) => ({
  ...(await actual<typeof import('../../lib/services/downloads')>()),
  cancelDownload: vi.fn(),
  resumeDownload: vi.fn(),
  getDownload: vi.fn(() => new Promise(() => {})),
}));

import { msg } from '../../lib/i18n';
import { cancelDownload, resumeDownload, type Download } from '../../lib/services/downloads';
import { downloads } from '../../lib/stores/downloads';
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
    status: 'downloading',
    progress: 0.4,
    downloaded: 0,
    total: 1000,
    downloadSpeed: 100,
    uploadSpeed: 0,
    etaSeconds: 10,
    seeders: 1,
    peers: 1,
    files: [],
    seeding: false,
    stalled: false,
    stalledSince: null,
    addedAt: '2026-09-08T00:00:00Z',
    completedAt: null,
    error: '',
    origin: {},
    ...patch,
  };
}

const goSets = (id: string, patch: Partial<Download>) => downloads.update((list) => list.map((d) => (d.id === id ? { ...d, ...patch } : d)));
const panel = (name: string) => screen.getByRole('region', { name });
const grip = (title: string) => screen.getByRole('button', { name: new RegExp(`^«${title}», `) });
const cancelX = (title: string) => screen.getByRole('button', { name: msg('downloads.queue.cancelFor', { title }) });

let scrolled: HTMLElement[];

beforeEach(() => {
  scrolled = [];
  Element.prototype.scrollIntoView = vi.fn(function (this: HTMLElement) {
    scrolled.push(this);
  });
  for (const fn of [cancelDownload, resumeDownload]) {
    vi.mocked(fn).mockReset();
    vi.mocked(fn).mockResolvedValue(undefined);
  }
});

afterEach(() => {
  downloads.set([]);
  toasts.set([]);
  resetHistory();
  document.body.replaceChildren();
});

describe('Downloads: focus goes with a download its own press moves to another section', () => {
  it('Resume that queues the download takes focus to its grip in the queue', async () => {
    vi.mocked(resumeDownload).mockImplementation(async (id) => goSets(id, { status: 'queued', downloadSpeed: 0 }));
    downloads.set([download({ status: 'paused', downloadSpeed: 0 }), download({ id: 'd2', name: 'Game Two' })]);
    const { user } = renderDom(Downloads);

    within(panel('Game One')).getByRole('button', { name: msg('downloads.panel.resumeNamed', { name: 'Game One' }) }).focus();
    await user.keyboard('{Enter}');

    await waitFor(() => expect(document.activeElement).toBe(grip('Game One')));
  });
});

describe('Downloads: the hand-off brings its target into view', () => {
  it('a cancelled queue row hands focus to the row now in its place and scrolls it into view', async () => {
    downloads.set([download({ status: 'queued' }), download({ id: 'd2', name: 'Game Two', status: 'queued' })]);
    const { user } = renderDom(Downloads);

    cancelX('Game One').focus();
    await user.keyboard('{Enter}');
    downloads.update((list) => list.filter((d) => d.id !== 'd1'));

    await waitFor(() => expect(document.activeElement).toBe(cancelX('Game Two')));
    expect(scrolled).toContain(cancelX('Game Two'));
  });
});
