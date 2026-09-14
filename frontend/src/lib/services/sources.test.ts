import { describe, expect, it, vi } from 'vitest';

const { browseGames } = vi.hoisted(() => ({ browseGames: vi.fn() }));

vi.mock('../../../bindings/typhon/internal/catalog', () => ({ Service: { BrowseGames: browseGames } }));
vi.mock('../../../bindings/typhon/internal/sources', () => ({ Service: {} }));
vi.mock('./backend', () => ({ inWails: true }));

import { queryCatalogGames, searchGames } from './sources';

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
