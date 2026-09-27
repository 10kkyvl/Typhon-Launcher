import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { get } from 'svelte/store';

vi.mock('@wailsio/runtime', () => ({
  Events: { On: vi.fn(() => vi.fn()) },
  Call: { ByID: vi.fn() },
  CancellablePromise: class {},
}));
vi.mock('./toasts', () => ({ toast: vi.fn() }));
vi.mock('../services/account', () => {
  class AccountError extends Error {
    code: string;
    field: string;
    constructor(code: string, field = '') {
      super(code);
      this.name = 'AccountError';
      this.code = code;
      this.field = field;
    }
  }
  return { AccountError };
});
vi.mock('../services/social', () => ({
  feed: vi.fn(async () => ({ events: [], next: 0 })),
  react: vi.fn(async () => {}),
  unreact: vi.fn(async () => {}),
  setNote: vi.fn(async () => {}),
}));

function event(id: number, emoji = 'fire', count = 0) {
  return {
    id,
    user: { id: `u${id}`, username: `u${id}`, displayName: `u${id}`, avatarUrl: '' },
    kind: 'completed',
    game: { igdbId: 1, title: 'Game', coverUrl: '' },
    createdAt: '2026-09-03T10:00:00Z',
    reactions: count > 0 ? [{ emoji, count }] : [],
    mine: [] as string[],
    note: '',
  };
}

async function load() {
  vi.resetModules();
  const feed = await import('./feed');
  const { feed: fetchFeed } = await import('../services/social');
  return { ...feed, fetchFeed: vi.mocked(fetchFeed) };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.useFakeTimers();
  vi.setSystemTime(new Date('2026-09-03T12:00:00Z'));
});

afterEach(() => {
  vi.useRealTimers();
});

it('late response cannot repopulate feed after logout reset', async () => {
 const f = await load();
 let resolve: (v: any) => void = () => {};
 f.fetchFeed.mockImplementationOnce(() => new Promise(r => { resolve = r; }));
 const request = f.loadFeed();
 f.resetFeed();
 expect(get(f.feedEvents)).toEqual([]);
 resolve({ events: [event(777)], next: 0 });
 await request;
 expect(get(f.feedEvents).map(e => e.id)).toEqual([]);
});
