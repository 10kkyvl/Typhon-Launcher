import { CancellablePromise } from '@wailsio/runtime';
import { describe, expect, it, vi } from 'vitest';

const { browseGames } = vi.hoisted(() => ({ browseGames: vi.fn() }));

vi.mock('../../../bindings/typhon/internal/catalog', () => ({ Service: { BrowseGames: browseGames } }));
vi.mock('../../../bindings/typhon/internal/sources', () => ({ Service: {} }));
vi.mock('./backend', () => ({ inWails: true }));

import { isCancelledRequest, queryCatalogGames, searchGames } from './sources';

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
