import { CancellablePromise } from '@wailsio/runtime';
import { describe, expect, it, vi } from 'vitest';

const { browseGames, getGame, getReleaseBinding } = vi.hoisted(() => ({
  browseGames: vi.fn(),
  getGame: vi.fn(),
  getReleaseBinding: vi.fn(),
}));

vi.mock('../../../bindings/typhon/internal/catalog', () => ({ Service: { BrowseGames: browseGames, GetGame: getGame } }));
vi.mock('../../../bindings/typhon/internal/sources', () => ({ Service: { GetRelease: getReleaseBinding } }));
vi.mock('./backend', () => ({ inWails: true }));

import { getCatalogGame, getRelease, isCancelledRequest, queryCatalogGames, searchGames } from './sources';

const emptyPage = { items: [], total: 0, page: 1, pageSize: 20 };

describe('catalog query filtering defaults', () => {
  it('does not hide dismissed games for generic searches', async () => {
    browseGames.mockResolvedValueOnce(emptyPage);

    await searchGames('game', 20);

    expect(browseGames).toHaveBeenCalledWith(expect.objectContaining({
      search: 'game',
      kind: 'all',
      hideNotInterested: false,
    }));
  });

  it('keeps an explicit catalog hide preference', async () => {
    browseGames.mockResolvedValueOnce(emptyPage);

    await queryCatalogGames({ search: 'game', hideNotInterested: true });

    expect(browseGames).toHaveBeenCalledWith(expect.objectContaining({ hideNotInterested: true }));
  });
});

describe('catalog stale pages', () => {
  it('passes stale and key through from the binding', async () => {
    browseGames.mockResolvedValueOnce({ ...emptyPage, stale: true, key: 'cache-key' });

    const page = await queryCatalogGames({ search: 'game' });

    expect(page.stale).toBe(true);
    expect(page.key).toBe('cache-key');
  });

  it('defaults to a fresh page when the binding omits the fields', async () => {
    browseGames.mockResolvedValueOnce(emptyPage);

    const page = await queryCatalogGames({ search: 'game' });

    expect(page.stale).toBe(false);
    expect(page.key).toBe('');
  });
});

describe('catalog stale opt-in', () => {
  it('sends allowStale only when the caller sets it', async () => {
    browseGames.mockClear();
    browseGames.mockResolvedValue(emptyPage);

    await queryCatalogGames({ search: 'game', allowStale: true });
    await queryCatalogGames({ search: 'game' });
    await queryCatalogGames({ search: 'game', allowStale: false });

    expect(browseGames.mock.calls[0][0]).toHaveProperty('allowStale', true);
    expect(browseGames.mock.calls[1][0]).not.toHaveProperty('allowStale');
    expect(browseGames.mock.calls[2][0]).not.toHaveProperty('allowStale');
    browseGames.mockReset();
  });
});

describe('catalog request cancellation', () => {
  it('cancels the underlying BrowseGames call when the signal aborts', async () => {
    const call = new CancellablePromise<typeof emptyPage>((resolve) => { setTimeout(() => resolve(emptyPage), 50); });
    browseGames.mockReturnValueOnce(call);
    const controller = new AbortController();

    const pending = queryCatalogGames({ search: 'game' }, controller.signal);
    controller.abort();

    await expect(pending).rejects.toSatisfy(isCancelledRequest);
  });

  it('recognises a cancelled request by type, not by its error text', () => {
    expect(isCancelledRequest(new Error('typhon:catalog.changed'))).toBe(false);
    expect(isCancelledRequest('Promise cancelled.')).toBe(false);
  });
});

describe('single record lookups', () => {
  it('returns null for a game the catalog does not have', async () => {
    getGame.mockRejectedValueOnce(new Error('typhon:catalog.game_not_found'));

    await expect(getCatalogGame('g1')).resolves.toBeNull();
  });

  it('lets a catalog failure reach the caller instead of reporting a missing game', async () => {
    const failure = new Error('typhon:catalog.save_failed');
    getGame.mockRejectedValueOnce(failure);

    await expect(getCatalogGame('g1')).rejects.toBe(failure);
  });

  it('returns null for a release the source does not have', async () => {
    getReleaseBinding.mockRejectedValueOnce(new Error('typhon:sources.release_not_found'));

    await expect(getRelease('r1')).resolves.toBeNull();
  });

  it('lets a release lookup failure reach the caller', async () => {
    const failure = new Error('boom');
    getReleaseBinding.mockRejectedValueOnce(failure);

    await expect(getRelease('r1')).rejects.toBe(failure);
  });
});
