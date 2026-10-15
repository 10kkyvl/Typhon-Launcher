import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import type { Alert, Entry, Status } from '../services/watchlist';

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

let entries: Entry[] = [];
let alerts: Alert[] = [];
const status: Status = { degraded: false, message: '' };
const watch = vi.fn();
const unwatch = vi.fn();
const dismiss = vi.fn();
const dismissAll = vi.fn();

vi.mock('../services/watchlist', () => ({
  listWatched: vi.fn(async () => entries),
  listAlerts: vi.fn(async () => alerts),
  getWatchlistStatus: vi.fn(async () => status),
  watchGame: watch,
  unwatchGame: unwatch,
  dismissWatchAlert: dismiss,
  dismissWatchAlerts: dismissAll,
}));

function entry(gameId: string): Entry {
  return { gameId, title: gameId, addedAt: '2026-01-01T00:00:00Z', known: [] } as unknown as Entry;
}

function alert(id: string, gameId: string): Alert {
  return { id, gameId, title: gameId, releaseId: 'r', releaseTitle: 'R', version: '', sourceId: 's', at: '2026-01-01T00:00:00Z' } as unknown as Alert;
}

async function load() {
  vi.resetModules();
  for (const key of Object.keys(handlers)) delete handlers[key];
  const store = await import('./watchlist');
  const toasts = await import('./toasts');
  await store.initWatchlist();
  return { store, toast: vi.mocked(toasts.toast) };
}

beforeEach(() => {
  vi.clearAllMocks();
  entries = [];
  alerts = [];
  watch.mockImplementation(async (id: string) => entry(id));
  unwatch.mockResolvedValue(undefined);
  dismiss.mockResolvedValue(undefined);
  dismissAll.mockResolvedValue(undefined);
});

describe('watchlist store', () => {
  it('loads entries and exposes watched ids', async () => {
    entries = [entry('g1'), entry('g2')];
    const { store } = await load();
    expect([...get(store.watchedIds)]).toEqual(['g1', 'g2']);
  });

  it('toggleWatch adds then removes a game and its alerts', async () => {
    alerts = [alert('a1', 'g1')];
    const { store } = await load();
    await store.toggleWatch('g1');
    expect(get(store.watchedIds).has('g1')).toBe(true);
    await store.toggleWatch('g1');
    expect(get(store.watchedIds).has('g1')).toBe(false);
    expect(get(store.watchAlerts)).toEqual([]);
  });

  it('shows a toast and keeps the state when watching fails', async () => {
    watch.mockRejectedValue(new Error('boom'));
    const { store, toast } = await load();
    await store.toggleWatch('g1');
    expect(get(store.watchedIds).has('g1')).toBe(false);
    expect(toast).toHaveBeenCalledWith(expect.any(String), 'danger');
  });

  it('keeps the alert when dismissing fails', async () => {
    alerts = [alert('a1', 'g1')];
    dismiss.mockRejectedValue(new Error('boom'));
    const { store, toast } = await load();
    await store.dismissAlert('a1');
    expect(get(store.watchAlerts)).toHaveLength(1);
    expect(toast).toHaveBeenCalledWith(expect.any(String), 'danger');
  });

  it('dismissAlerts drops every alert of the game', async () => {
    alerts = [alert('a1', 'g1'), alert('a2', 'g2')];
    const { store } = await load();
    await store.dismissAlerts('g1');
    expect(get(store.watchAlerts).map((a) => a.id)).toEqual(['a2']);
  });

  it('adds alerts from the event without duplicates and toasts each', async () => {
    alerts = [alert('a1', 'g1')];
    const { store, toast } = await load();
    handlers['watchlist:alert']({ data: [alert('a1', 'g1'), alert('a2', 'g2')] });
    expect(get(store.watchAlerts).map((a) => a.id)).toEqual(['a1', 'a2']);
    expect(toast).toHaveBeenCalledTimes(2);
  });

  it('replaces entries on watchlist:updated', async () => {
    entries = [entry('g1')];
    const { store } = await load();
    handlers['watchlist:updated']({ data: [entry('g3')] });
    expect([...get(store.watchedIds)]).toEqual(['g3']);
  });
});
