import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import type { Download } from '../services/downloads';
import { goSources } from '../testing/sources';

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
vi.mock('./toasts', () => ({ toast: vi.fn(), ACTION_MS: 1 }));

let seeded: Download[] = [];

vi.mock('../services/downloads', () => ({
  listDownloads: vi.fn(async () => seeded),
  cancelDownload: vi.fn(),
  downloadNow: vi.fn(),
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
    source: '',
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

describe('download events', () => {
  it('are named as Go names them, and each has a listener', async () => {
    const { store } = await load();
    const go = goSources['internal/download/manager.go'];
    const declared = new Map([...go.matchAll(/^\tevent(\w+)\s*=\s*"(download:[a-z]+)"/gm)].map((match) => [match[1].toLowerCase(), match[2]]));

    expect(declared.size).toBeGreaterThan(5);
    for (const [kind, name] of Object.entries(store.downloadEvents)) {
      expect(declared.get(kind), kind).toBe(name);
    }
    for (const name of Object.values(store.downloadEvents)) expect(handlers[name], name).toBeTypeOf('function');
  });

  it.each(['added', 'updated', 'progress', 'completed', 'failed', 'removed'] as const)(
    'a %s event without a download id changes nothing',
    async (kind) => {
      seeded = [makeDownload()];
      const { store } = await load();
      const before = get(store.downloads);

      for (const data of [null, undefined, 'x', {}, { id: 7 }]) handlers[store.downloadEvents[kind]]({ data });

      expect(get(store.downloads)).toBe(before);
    },
  );
});
