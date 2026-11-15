import { render, screen } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { listDownloads, moveDownloadDown, type Download } from '../services/downloads';
import { downloads, initDownloads } from '../stores/downloads';
import { toast } from '../stores/toasts';
import { user } from '../testing/dom';
import DownloadsPage from './components/DownloadsPage.svelte';

const handlers: Record<string, (event: { data: unknown }) => void> = {};

vi.mock('@wailsio/runtime', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  Events: {
    On: vi.fn((name: string, cb: (event: { data: unknown }) => void) => {
      handlers[name] = cb;
      return () => {};
    }),
  },
}));
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../stores/toasts', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../stores/toasts')>()),
  toast: vi.fn(),
}));
vi.mock('../services/downloads', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/downloads')>()),
  listDownloads: vi.fn(),
  moveDownloadDown: vi.fn(),
  moveDownloadUp: vi.fn(),
}));

function queued(id: string): Download {
  return {
    id,
    name: `Game ${id}`,
    type: 'torrent',
    source: '',
    infoHash: `hash-${id}`,
    destination: `/games/${id}`,
    status: 'queued',
    progress: 0,
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
    completedAt: null,
    error: '',
    origin: {},
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function emit(name: string, data: unknown) {
  const handler = handlers[name];
  expect(handler, name).toBeTypeOf('function');
  handler({ data });
}

function control(key: string): HTMLButtonElement {
  const found = document.querySelector<HTMLButtonElement>(`[data-bp-focus="${key}"]`);
  expect(found, key).not.toBeNull();
  return found!;
}

const ids = () => get(downloads).map((item) => item.id);

beforeEach(async () => {
  vi.mocked(listDownloads).mockResolvedValue([queued('a'), queued('b'), queued('c')]);
  await initDownloads();
  vi.mocked(listDownloads).mockClear();
});

afterEach(() => {
  downloads.set([]);
  vi.mocked(listDownloads).mockReset();
  vi.mocked(moveDownloadDown).mockReset();
  vi.mocked(toast).mockClear();
});

function renderPage() {
  render(DownloadsPage, { props: { onback: vi.fn(), ongame: vi.fn(), requestText: vi.fn(async () => null) } });
}

describe('Big Picture downloads: moving a queued download', () => {
  it('keeps a remove and an add that land during the move, and reads no list afterwards', async () => {
    const move = deferred<void>();
    vi.mocked(moveDownloadDown).mockReturnValueOnce(move.promise);
    // A list read now would answer with what Go held before the events below arrived.
    vi.mocked(listDownloads).mockResolvedValue([queued('b'), queued('a'), queued('c')]);
    renderPage();

    await user().click(control('queue:a:down'));
    expect(moveDownloadDown).toHaveBeenCalledWith('a');

    emit('download:reordered', { ids: ['b', 'a', 'c'] });
    emit('download:removed', { id: 'b' });
    emit('download:added', queued('d'));
    move.resolve();

    await vi.waitFor(() => expect(control('queue:a:down').disabled).toBe(false));
    expect(ids()).toEqual(['a', 'c', 'd']);
    expect(listDownloads).not.toHaveBeenCalled();
  });

  it('says a failed move on the card, toasts nothing, and its Retry sends the move again', async () => {
    vi.mocked(moveDownloadDown)
      .mockRejectedValueOnce(new Error('typhon:download.not_found: gone'))
      .mockResolvedValueOnce(undefined);
    renderPage();
    const keys = user();

    await keys.click(control('queue:a:down'));
    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toContain(msg('bp.transfers.retryAction'));
    expect(toast).not.toHaveBeenCalled();

    await keys.click(control('retry:download:a:move-down'));
    await vi.waitFor(() => expect(screen.queryByRole('alert')).toBeNull());
    expect(moveDownloadDown).toHaveBeenCalledTimes(2);
    expect(listDownloads).not.toHaveBeenCalled();
  });
});
