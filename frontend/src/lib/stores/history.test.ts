import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import type { Record, Status } from '../services/history';

const handlers: { [name: string]: (event: { data: unknown }) => void } = {};

vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: vi.fn((name: string, cb: (event: { data: unknown }) => void) => {
      handlers[name] = cb;
      return vi.fn();
    }),
  },
}));

vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('./toasts', () => ({ toast: vi.fn() }));

let seeded: Record[] = [];
let status: Status = { degraded: false, message: '' };
const clear = vi.fn(async () => {});

vi.mock('../services/history', () => ({
  listHistory: vi.fn(async () => seeded),
  getHistoryStatus: vi.fn(async () => status),
  clearHistory: clear,
}));

function record(id: string, at: string): Record {
  return { id, kind: 'installed', at, title: id, bytesKnown: false } as unknown as Record;
}

async function load() {
  vi.resetModules();
  for (const key of Object.keys(handlers)) delete handlers[key];
  const store = await import('./history');
  const toasts = await import('./toasts');
  await store.initHistory();
  return { store, toast: vi.mocked(toasts.toast) };
}

beforeEach(() => {
  vi.clearAllMocks();
  seeded = [];
  status = { degraded: false, message: '' };
  clear.mockResolvedValue(undefined);
});

describe('history store', () => {
  it('shows the newest record first, whatever order the backend returns', async () => {
    seeded = [
      record('old', '2026-09-01T10:00:00Z'),
      record('newest', '2026-09-03T10:00:00Z'),
      record('middle', '2026-09-02T10:00:00Z'),
    ];
    const { store } = await load();

    expect(get(store.history).map((item) => item.id)).toEqual(['newest', 'middle', 'old']);
  });

  it('keeps the twenty newest records for the recent list', async () => {
    seeded = Array.from({ length: 30 }, (_, index) => record(`r${index}`, new Date(Date.UTC(2026, 8, 1, 0, index)).toISOString()));
    const { store } = await load();

    const recent = get(store.historyRecent);
    expect(recent).toHaveLength(20);
    expect(recent[0].id).toBe('r29');
    expect(recent[19].id).toBe('r10');
  });

  it('puts a freshly recorded entry on top and does not duplicate one it already has', async () => {
    seeded = [record('a', '2026-09-01T10:00:00Z')];
    const { store } = await load();

    handlers['history:recorded']({ data: record('b', '2026-09-02T10:00:00Z') });
    handlers['history:recorded']({ data: record('b', '2026-09-02T10:00:00Z') });

    expect(get(store.history).map((item) => item.id)).toEqual(['b', 'a']);
  });

  it('replaces the whole list from history:updated', async () => {
    seeded = [record('a', '2026-09-01T10:00:00Z')];
    const { store } = await load();

    handlers['history:updated']({ data: [record('x', '2026-09-05T10:00:00Z'), record('y', '2026-09-06T10:00:00Z')] });

    expect(get(store.history).map((item) => item.id)).toEqual(['y', 'x']);
  });

  it('flags the journal as degraded and clears the flag once it records again', async () => {
    const { store } = await load();

    handlers['history:degraded']({ data: { degraded: true, message: 'disk is full' } });
    expect(get(store.historyStatus)).toEqual({ degraded: true, message: 'disk is full' });

    handlers['history:recorded']({ data: record('a', '2026-09-01T10:00:00Z') });
    expect(get(store.historyStatus)).toEqual({ degraded: false, message: '' });
  });

  it('starts with the status the backend reports', async () => {
    status = { degraded: true, message: 'read only' };
    const { store } = await load();

    expect(get(store.historyStatus)).toEqual({ degraded: true, message: 'read only' });
  });
});

describe('clearing the history', () => {
  it('empties the list once the backend has cleared it', async () => {
    seeded = [record('a', '2026-09-01T10:00:00Z')];
    const { store, toast } = await load();

    await store.clearHistory();

    expect(clear).toHaveBeenCalledTimes(1);
    expect(get(store.history)).toEqual([]);
    expect(toast).not.toHaveBeenCalled();
  });

  it('keeps the records and tells the player when the backend could not clear them', async () => {
    seeded = [record('a', '2026-09-01T10:00:00Z')];
    clear.mockRejectedValueOnce(new Error('typhon:history.clear_failed: disk is read only'));
    const { store, toast } = await load();

    await store.clearHistory();

    expect(get(store.history)).toHaveLength(1);
    expect(toast).toHaveBeenCalledTimes(1);
    expect(toast.mock.calls[0][1]).toBe('danger');
    expect(String(toast.mock.calls[0][0])).not.toContain('typhon:');
  });

  it('gives a generic message for a failure without a code', async () => {
    clear.mockRejectedValueOnce(new Error('boom'));
    const { store, toast } = await load();

    await store.clearHistory();

    expect(toast).toHaveBeenCalledTimes(1);
    expect(String(toast.mock.calls[0][0])).not.toContain('boom');
  });
});
