import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { SearchOverlay } from './overlay';
import type { OverlayState } from './overlay';
import type { GameHit, ReleaseHit, SearchResult } from '../services/search';

function makeResult(games: GameHit[], releases: ReleaseHit[], moreGames = 0, moreReleases = 0): SearchResult {
  return { query: '', games, releases, moreGames, moreReleases };
}

function makeGame(id: string): GameHit {
  return {
    id,
    title: id,
    installed: false,
    releases: 1,
    sources: 1,
    score: 1,
  };
}

function makeRelease(id: string): ReleaseHit {
  return {
    id,
    sourceId: 'src',
    sourceName: 'Source',
    title: id,
    size: 0,
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

const gameA = makeGame('game-a');
const gameB = makeGame('game-b');
const releaseA = makeRelease('release-a');
const releaseB = makeRelease('release-b');

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

function track() {
  let state: OverlayState | undefined;
  const onState = vi.fn((s: OverlayState) => {
    state = s;
  });
  return { onState, get state() { return state as OverlayState; } };
}

describe('SearchOverlay debounce', () => {
  it('does not call search before the delay elapses', async () => {
    const search = vi.fn().mockResolvedValue(makeResult([], []));
    const overlay = new SearchOverlay({ search, onState: () => {}, delay: 200 });
    overlay.setQuery('cy');
    expect(search).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(199);
    expect(search).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    expect(search).toHaveBeenCalledTimes(1);
    expect(search).toHaveBeenCalledWith('cy');
  });

  it('collapses rapid setQuery calls into a single search with the latest query', async () => {
    const search = vi.fn().mockResolvedValue(makeResult([], []));
    const overlay = new SearchOverlay({ search, onState: () => {}, delay: 200 });
    overlay.setQuery('c');
    overlay.setQuery('cy');
    overlay.setQuery('cyb');
    overlay.setQuery('cybe');
    await vi.advanceTimersByTimeAsync(200);
    expect(search).toHaveBeenCalledTimes(1);
    expect(search).toHaveBeenCalledWith('cybe');
  });
});

describe('SearchOverlay stale response protection', () => {
  it('keeps the newer result when the older search resolves later', async () => {
    const first = deferred<SearchResult>();
    const second = deferred<SearchResult>();
    const search = vi.fn().mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const tracker = track();
    const { onState } = tracker;
    const overlay = new SearchOverlay({ search, onState, delay: 100 });

    overlay.setQuery('alpha');
    await vi.advanceTimersByTimeAsync(100);
    expect(search).toHaveBeenNthCalledWith(1, 'alpha');

    overlay.setQuery('beta');
    await vi.advanceTimersByTimeAsync(100);
    expect(search).toHaveBeenNthCalledWith(2, 'beta');

    second.resolve(makeResult([gameB], []));
    await second.promise;
    expect(tracker.state.games).toEqual([gameB]);

    first.resolve(makeResult([gameA], []));
    await first.promise;
    expect(tracker.state.games).toEqual([gameB]);
    expect(tracker.state.games).not.toEqual([gameA]);
  });
});

describe('SearchOverlay empty query', () => {
  it('clears state, never calls search, and reports not searched', () => {
    const search = vi.fn();
    const tracker = track();
    const { onState } = tracker;
    const overlay = new SearchOverlay({ search, onState, delay: 50 });

    overlay.setQuery('');

    expect(search).not.toHaveBeenCalled();
    expect(tracker.state.loading).toBe(false);
    expect(tracker.state.searched).toBe(false);
    expect(tracker.state.games).toEqual([]);
    expect(tracker.state.releases).toEqual([]);
  });
});

describe('SearchOverlay minimum query length', () => {
  it('triggers a search for a single character query', async () => {
    const search = vi.fn().mockResolvedValue(makeResult([], []));
    const overlay = new SearchOverlay({ search, onState: () => {}, delay: 50 });
    overlay.setQuery('c');
    await vi.advanceTimersByTimeAsync(50);
    expect(search).toHaveBeenCalledTimes(1);
    expect(search).toHaveBeenCalledWith('c');
  });
});

describe('SearchOverlay focus', () => {
  it('opens the overlay without clearing the query and avoids a duplicate search', async () => {
    const search = vi.fn().mockResolvedValue(makeResult([gameA], []));
    const tracker = track();
    const { onState } = tracker;
    const overlay = new SearchOverlay({ search, onState, delay: 50 });

    overlay.setQuery('halo');
    await vi.advanceTimersByTimeAsync(50);
    expect(search).toHaveBeenCalledTimes(1);

    overlay.close();
    expect(tracker.state.open).toBe(false);
    expect(tracker.state.query).toBe('halo');

    overlay.focus();
    expect(tracker.state.open).toBe(true);
    expect(tracker.state.query).toBe('halo');

    overlay.focus();
    expect(tracker.state.open).toBe(true);
    expect(tracker.state.query).toBe('halo');

    await vi.advanceTimersByTimeAsync(1000);
    expect(search).toHaveBeenCalledTimes(1);
  });
});

describe('SearchOverlay close', () => {
  it('hides the overlay and resets active, but keeps query and results', async () => {
    const search = vi.fn().mockResolvedValue(makeResult([gameA], [releaseA]));
    const tracker = track();
    const { onState } = tracker;
    const overlay = new SearchOverlay({ search, onState, delay: 50 });

    overlay.setQuery('doom');
    await vi.advanceTimersByTimeAsync(50);
    overlay.move(1);
    expect(tracker.state.active).toBe(0);

    overlay.close();
    expect(tracker.state.open).toBe(false);
    expect(tracker.state.active).toBe(-1);
    expect(tracker.state.query).toBe('doom');
    expect(tracker.state.games).toEqual([gameA]);
    expect(tracker.state.releases).toEqual([releaseA]);
  });
});

describe('SearchOverlay keyboard navigation', () => {
  it('moves forward across games then releases and wraps to the start', async () => {
    const search = vi.fn().mockResolvedValue(makeResult([gameA, gameB], [releaseA, releaseB]));
    const tracker = track();
    const { onState } = tracker;
    const overlay = new SearchOverlay({ search, onState, delay: 50 });

    overlay.setQuery('quake');
    await vi.advanceTimersByTimeAsync(50);
    expect(tracker.state.active).toBe(-1);

    overlay.move(1);
    expect(tracker.state.active).toBe(0);
    expect(overlay.activeHit()).toEqual({ kind: 'game', hit: gameA });

    overlay.move(1);
    expect(tracker.state.active).toBe(1);
    expect(overlay.activeHit()).toEqual({ kind: 'game', hit: gameB });

    overlay.move(1);
    expect(tracker.state.active).toBe(2);
    expect(overlay.activeHit()).toEqual({ kind: 'release', hit: releaseA });

    overlay.move(1);
    expect(tracker.state.active).toBe(3);
    expect(overlay.activeHit()).toEqual({ kind: 'release', hit: releaseB });

    overlay.move(1);
    expect(tracker.state.active).toBe(0);
    expect(overlay.activeHit()).toEqual({ kind: 'game', hit: gameA });
  });

  it('moves backward from the initial position to the last index', async () => {
    const search = vi.fn().mockResolvedValue(makeResult([gameA, gameB], [releaseA, releaseB]));
    const tracker = track();
    const { onState } = tracker;
    const overlay = new SearchOverlay({ search, onState, delay: 50 });

    overlay.setQuery('quake');
    await vi.advanceTimersByTimeAsync(50);
    expect(tracker.state.active).toBe(-1);

    overlay.move(-1);
    expect(tracker.state.active).toBe(3);
    expect(overlay.activeHit()).toEqual({ kind: 'release', hit: releaseB });
  });
});

describe('SearchOverlay error path', () => {
  it('surfaces the error message and clears result lists', async () => {
    const search = vi.fn().mockRejectedValue(new Error('network down'));
    const tracker = track();
    const { onState } = tracker;
    const overlay = new SearchOverlay({ search, onState, delay: 50 });

    overlay.setQuery('portal');
    await vi.advanceTimersByTimeAsync(50);

    expect(tracker.state.loading).toBe(false);
    expect(tracker.state.searched).toBe(true);
    expect(tracker.state.error).toBe('network down');
    expect(tracker.state.games).toEqual([]);
    expect(tracker.state.releases).toEqual([]);
  });

  it('falls back to errorText when the rejection carries no message', async () => {
    const search = vi.fn().mockRejectedValue(new Error());
    const tracker = track();
    const { onState } = tracker;
    const overlay = new SearchOverlay({ search, onState, delay: 50, errorText: 'Search unavailable' });

    overlay.setQuery('portal');
    await vi.advanceTimersByTimeAsync(50);

    expect(tracker.state.error).toBe('Search unavailable');
  });
});

describe('SearchOverlay destroy', () => {
  it('prevents a pending debounce timer from firing a search', async () => {
    const search = vi.fn().mockResolvedValue(makeResult([], []));
    const overlay = new SearchOverlay({ search, onState: () => {}, delay: 50 });

    overlay.setQuery('unreal');
    overlay.destroy();
    await vi.advanceTimersByTimeAsync(1000);

    expect(search).not.toHaveBeenCalled();
  });

  it('ignores an in-flight response that resolves after destroy', async () => {
    const pending = deferred<SearchResult>();
    const search = vi.fn().mockReturnValue(pending.promise);
    const onState = vi.fn();
    const overlay = new SearchOverlay({ search, onState, delay: 50 });

    overlay.setQuery('unreal');
    await vi.advanceTimersByTimeAsync(50);
    expect(search).toHaveBeenCalledTimes(1);

    const callsBeforeDestroy = onState.mock.calls.length;
    overlay.destroy();

    pending.resolve(makeResult([gameA], []));
    await pending.promise;

    expect(onState.mock.calls.length).toBe(callsBeforeDestroy);
  });
});
