import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import type { DiscoveryResult } from '../services/discovery';

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

let scanning = false;
const scan = vi.fn();

vi.mock('../services/discovery', () => ({
  isScanning: vi.fn(async () => scanning),
  scanInstalledGames: scan,
}));

function result(patch: Partial<DiscoveryResult> = {}): DiscoveryResult {
  return {
    roots: 1,
    rootsSkipped: 0,
    candidates: 0,
    added: 0,
    updated: 0,
    known: 0,
    skipped: 0,
    errors: 0,
    cancelled: false,
    ...patch,
  };
}

async function load() {
  vi.resetModules();
  for (const key of Object.keys(handlers)) delete handlers[key];
  const store = await import('./discovery');
  await store.initDiscovery();
  return store;
}

beforeEach(() => {
  vi.clearAllMocks();
  scanning = false;
});

describe('game scan', () => {
  it('picks up a scan that is already running when the window opens', async () => {
    scanning = true;
    const store = await load();

    expect(get(store.scanning)).toBe(true);
  });

  it('resets the progress when a scan starts and remembers how much there is to do', async () => {
    const store = await load();
    store.scanProgress.set({ processed: 9, total: 9 });

    handlers['discovery:started']({ data: { processed: 0, total: 40 } });

    expect(get(store.scanning)).toBe(true);
    expect(get(store.scanProgress)).toEqual({ processed: 0, total: 40 });
  });

  it('starts from zero when the start event carries no total', async () => {
    const store = await load();

    handlers['discovery:started']({ data: undefined });

    expect(get(store.scanProgress)).toEqual({ processed: 0, total: 0 });
  });

  it('follows progress and ignores an empty progress event', async () => {
    const store = await load();
    handlers['discovery:started']({ data: { processed: 0, total: 10 } });

    handlers['discovery:progress']({ data: { processed: 4, total: 10 } });
    handlers['discovery:progress']({ data: undefined });

    expect(get(store.scanProgress)).toEqual({ processed: 4, total: 10 });
  });

  it('stores the outcome and stops scanning when the scan completes', async () => {
    const store = await load();
    handlers['discovery:started']({ data: { processed: 0, total: 10 } });

    handlers['discovery:completed']({ data: result({ candidates: 3, added: 2 }) });

    expect(get(store.scanning)).toBe(false);
    expect(get(store.lastScan)?.added).toBe(2);
  });

  it('forgets the previous outcome when a scan completes without one', async () => {
    const store = await load();
    store.lastScan.set(result({ added: 5 }));

    handlers['discovery:completed']({ data: undefined });

    expect(get(store.lastScan)).toBeNull();
  });

  it('returns what the scan found and always clears the scanning flag', async () => {
    const store = await load();
    scan.mockResolvedValueOnce(result({ candidates: 1 }));

    const pending = store.rescan();
    expect(get(store.scanning)).toBe(true);

    await expect(pending).resolves.toMatchObject({ candidates: 1 });
    expect(get(store.scanning)).toBe(false);
  });

  it('clears the scanning flag and lets the caller see the error when the scan fails', async () => {
    const store = await load();
    scan.mockRejectedValueOnce(new Error('typhon:discovery.busy: already scanning'));

    await expect(store.rescan()).rejects.toThrow('discovery.busy');

    expect(get(store.scanning)).toBe(false);
  });
});

describe('scan summary', () => {
  it('always reports what was found', async () => {
    const store = await load();

    expect(store.scanSummary(result({ candidates: 4 }))).toContain('4');
  });

  it('leaves out every count that is zero', async () => {
    const store = await load();

    const only = store.scanSummary(result({ candidates: 4 }));
    const full = store.scanSummary(
      result({ candidates: 4, added: 1, updated: 2, known: 3, skipped: 5, errors: 6 }),
    );

    expect(only.split(' · ')).toHaveLength(1);
    expect(full.split(' · ')).toHaveLength(6);
  });

  it('adds one part per non-zero count, in a stable order', async () => {
    const store = await load();

    const parts = store.scanSummary(result({ candidates: 4, errors: 2, added: 1 })).split(' · ');

    expect(parts).toHaveLength(3);
    expect(parts[1]).toContain('1');
    expect(parts[2]).toContain('2');
  });
});
