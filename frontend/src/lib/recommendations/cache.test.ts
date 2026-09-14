import { describe, expect, it, vi } from 'vitest';
import { createDiscoveryCache } from './cache';
import type { DiscoveryResult } from '../services/recommendations';

function result(id: string, fallback = false): DiscoveryResult {
  return {
    items: [{ game: { id, title: id, sortTitle: id, createdAt: '' }, reason: 'popular' }],
    profile: { defaultSort: 'popular', confidence: 0, evidenceGames: 0, evidencePlaytimeSeconds: 0 }, fallback,
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}

describe('discovery cache', () => {
  it('shows fresh picks without another request, then shows expired picks while one shared refresh runs', async () => {
    let now = 0;
    const cache = createDiscoveryCache(() => now);
    const initial = result('old');
    await cache.get('same-context', async () => initial).refreshed;
    const request = deferred<DiscoveryResult>();
    const load = vi.fn(() => request.promise);
    now = 119_999;
    expect(cache.get('same-context', load)).toEqual({ cached: initial, refreshed: undefined });
    expect(load).not.toHaveBeenCalled();

    now = 120_000;
    const first = cache.get('same-context', load);
    const second = cache.get('same-context', load);
    expect(first.cached).toBe(initial);
    expect(second.refreshed).toBe(first.refreshed);
    expect(load).toHaveBeenCalledTimes(1);
    request.resolve(result('new'));
    await first.refreshed;
    expect(cache.get('same-context', load).cached?.items[0].game.id).toBe('new');
    expect(load).toHaveBeenCalledTimes(1);
  });

  it('retains remote picks after an offline fallback or rejected refresh and retries next time', async () => {
    let now = 0;
    const cache = createDiscoveryCache(() => now);
    await cache.get('key', async () => result('remote')).refreshed;
    now = 120_000;
    const offline = await cache.get('key', async () => result('local', true)).refreshed;
    expect(offline?.items[0].game.id).toBe('remote');
    expect(offline?.fallback).toBe(true);
    const failure = cache.get('key', async () => { throw new Error('network'); });
    await expect(failure.refreshed).rejects.toThrow('network');
    const retry = cache.get('key', async () => result('recovered'));
    expect(retry.cached?.items[0].game.id).toBe('remote');
    await retry.refreshed;
    expect(cache.get('key', async () => result('unused')).cached?.items[0].game.id).toBe('recovered');
  });

  it('never shares picks between different filters or personal evidence', async () => {
    const cache = createDiscoveryCache();
    await cache.get('rpg,profile-a', async () => result('rpg')).refreshed;
    const request = deferred<DiscoveryResult>();
    const changed = cache.get('strategy,profile-b', () => request.promise);
    expect(changed.cached).toBeUndefined();
    request.resolve(result('strategy'));
    await changed.refreshed;
    expect(cache.get('rpg,profile-a', async () => result('unused')).cached?.items[0].game.id).toBe('rpg');
  });

  it('refreshes on demand within the TTL and ignores an older request completing last', async () => {
    const cache = createDiscoveryCache();
    await cache.get('key', async () => result('initial')).refreshed;
    const older = deferred<DiscoveryResult>();
    const first = cache.get('key', () => older.promise, true);
    const second = cache.get('key', async () => result('latest'), true);
    expect(second.cached?.items[0].game.id).toBe('initial');
    await second.refreshed;
    older.resolve(result('obsolete'));
    await first.refreshed;
    expect(cache.get('key', async () => result('unused')).cached?.items[0].game.id).toBe('latest');
  });
});
