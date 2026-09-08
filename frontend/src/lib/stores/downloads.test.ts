import { describe, it, expect, vi, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import type { Download } from '../services/downloads';

const handlers: Record<string, (event: { data: unknown }) => void> = {};

vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: vi.fn((name: string, cb: (event: { data: unknown }) => void) => {
      handlers[name] = cb;
      return vi.fn();
    }),
  },
}));

vi.mock('../services/backend', () => ({ inWails: true }));

vi.mock('../install/installErrors', () => ({ installErrorText: vi.fn((err: unknown) => String(err)) }));

vi.mock('../i18n', () => ({ msg: vi.fn((key: string) => key) }));

vi.mock('./toasts', () => ({ toast: vi.fn() }));

let seeded: Download[] = [];

vi.mock('../services/downloads', () => ({
  listDownloads: vi.fn(async () => seeded),
  cancelDownload: vi.fn(),
  forceStartDownload: vi.fn(),
  moveDownloadDown: vi.fn(),
  moveDownloadUp: vi.fn(),
  pauseDownload: vi.fn(),
  removeDownload: vi.fn(),
  resumeDownload: vi.fn(),
}));

function makeDownload(patch: Partial<Download> = {}): Download {
  return {
    id: 'd1',
    name: 'Game One',
    type: 'torrent',
    source: 'magnet:?xt=urn:btih:hash1',
    infoHash: 'hash1',
    destination: '/games/d1',
    status: 'downloading',
    progress: 0,
    downloaded: 0,
    total: 1000,
    downloadSpeed: 0,
    uploadSpeed: 0,
    etaSeconds: -1,
    seeders: 0,
    peers: 0,
    files: [
      { path: 'game/data.bin', size: 900, selected: true, bytesDone: 0 },
      { path: 'game/readme.txt', size: 100, selected: true, bytesDone: 0 },
    ],
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

async function load() {
  vi.resetModules();
  for (const key of Object.keys(handlers)) delete handlers[key];
  const store = await import('./downloads');
  await store.initDownloads();
  return { store };
}

beforeEach(() => {
  vi.clearAllMocks();
  seeded = [];
});

describe('download:progress merging', () => {
  it('merges a lightweight progress tick into the matching download without losing its file list', async () => {
    seeded = [makeDownload()];
    const { store } = await load();

    handlers['download:progress']({
      data: {
        id: 'd1',
        status: 'downloading',
        progress: 0.5,
        downloaded: 500,
        downloadSpeed: 2048,
        uploadSpeed: 0,
        etaSeconds: 30,
        seeders: 4,
        peers: 7,
        stalled: false,
      },
    });

    const list = get(store.downloads);
    expect(list).toHaveLength(1);
    const updated = list[0];
    expect(updated.downloaded).toBe(500);
    expect(updated.progress).toBe(0.5);
    expect(updated.downloadSpeed).toBe(2048);
    expect(updated.seeders).toBe(4);
    expect(updated.peers).toBe(7);
    expect(updated.etaSeconds).toBe(30);
    // The heavy fields a progress tick never carries must survive untouched.
    expect(updated.files).toHaveLength(2);
    expect(updated.files[0].path).toBe('game/data.bin');
    expect(updated.name).toBe('Game One');
    expect(updated.destination).toBe('/games/d1');
  });

  it('carries a stall flip through the merge', async () => {
    seeded = [makeDownload()];
    const { store } = await load();

    handlers['download:progress']({
      data: {
        id: 'd1',
        status: 'downloading',
        progress: 0.5,
        downloaded: 500,
        downloadSpeed: 0,
        uploadSpeed: 0,
        etaSeconds: -1,
        seeders: 0,
        peers: 0,
        stalled: true,
        stalledSince: '2026-09-08T00:05:00Z',
      },
    });

    const updated = get(store.downloads)[0];
    expect(updated.stalled).toBe(true);
    expect(updated.stalledSince).toBe('2026-09-08T00:05:00Z');
  });

  it('ignores a progress tick for a download the store has not seen a full record for yet', async () => {
    seeded = [makeDownload({ id: 'd1' })];
    const { store } = await load();

    handlers['download:progress']({
      data: {
        id: 'ghost',
        status: 'downloading',
        progress: 0.9,
        downloaded: 900,
        downloadSpeed: 100,
        uploadSpeed: 0,
        etaSeconds: 1,
        seeders: 1,
        peers: 1,
        stalled: false,
      },
    });

    const list = get(store.downloads);
    // No half-filled card for the unknown id, and the known one is untouched.
    expect(list).toHaveLength(1);
    expect(list[0].id).toBe('d1');
    expect(list.find((d) => d.id === 'ghost')).toBeUndefined();
  });

  it('still applies a full download:updated snapshot as before, files included', async () => {
    seeded = [makeDownload()];
    const { store } = await load();

    handlers['download:updated']({
      data: makeDownload({ status: 'paused', downloadSpeed: 0, files: [{ path: 'only.bin', size: 10, selected: true, bytesDone: 10 }] }),
    });

    const updated = get(store.downloads)[0];
    expect(updated.status).toBe('paused');
    expect(updated.files).toHaveLength(1);
    expect(updated.files[0].path).toBe('only.bin');
  });
});
