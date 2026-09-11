import { describe, it, expect } from 'vitest';
import { appendCatalogPage, appendOfflineCatalogPage, loadCatalogContinuation, reloadCatalogPrefix } from './pages';
import type { CatalogGame, CatalogPage } from '../services/sources';
const game = (id: string, patch: Partial<CatalogGame> = {}): CatalogGame => ({
  id,
  title: id,
  sortTitle: id,
  createdAt: '',
  ...patch,
});
const page = (items: CatalogGame[], revision = 3): CatalogPage => ({ items, revision, total: 3, page: 2, pageSize: 1 });
describe('catalog continuation', () => {
  it('preserves previous position and appends a stable page', () => {
    const previous = [game('a')];
    expect(appendCatalogPage(previous, page([game('b')]), 3).map((g) => g.id)).toEqual(['a', 'b']);
    expect(previous).toHaveLength(1);
  });
  it('keeps the old page intact when the server revision changed', () => {
    const previous = [game('a')];
    expect(() => appendCatalogPage(previous, page([game('b')], 4), 3)).toThrow('catalog.changed');
    expect(previous).toEqual([game('a')]);
  });
  it('rejects repeated canonical identities even under different local IDs', () => {
    expect(() => appendCatalogPage([{ ...game('old'), serverId: 'same' }], page([{ ...game('new'), serverId: 'same' }]), 3)).toThrow('catalog_duplicate_page');
  });
});

import { vi } from 'vitest';
const changed = () => new Error('typhon:catalog.changed: reload');
const fresh = (n: number, revision = 4): CatalogPage => ({ ...page([game(`fresh-${n}`)], revision), page: n });
describe('catalog revision recovery', () => {
  it('rebuilds the visible prefix and the requested page with a fresh revision', async () => {
    const load = vi.fn().mockResolvedValueOnce(fresh(1)).mockResolvedValueOnce(fresh(2)).mockResolvedValueOnce(fresh(3));
    const old = [game('old-1'), game('old-2')];
    const result = await loadCatalogContinuation({page: 3, revision: 3, search: 'test'}, old, load, () => Promise.reject(changed()));
    expect(result.items.map(g => g.id)).toEqual(['fresh-1','fresh-2','fresh-3']);
    expect(result.result.page).toBe(3);
    expect(load.mock.calls.map(([q]) => [q.page,q.revision,q.search])).toEqual([[1,0,'test'],[2,4,'test'],[3,4,'test']]);
    expect(old.map(g => g.id)).toEqual(['old-1','old-2']);
  });
  it('does not loop or mix versions when the catalog changes again during recovery', async () => {
    const load = vi.fn().mockResolvedValueOnce(fresh(1)).mockResolvedValueOnce(fresh(2,5));
    await expect(loadCatalogContinuation({page: 3,revision: 3}, [], load, () => Promise.reject(changed()))).rejects.toThrow('catalog.changed');
    expect(load).toHaveBeenCalledTimes(2);
  });
  it('does not restart for network errors or a superseded search', async () => {
    const load = vi.fn();
    await expect(loadCatalogContinuation({page: 3}, [], load, () => Promise.reject(new Error('offline')))).rejects.toThrow('offline');
    await expect(loadCatalogContinuation({page: 3}, [], load, () => Promise.reject(changed()), () => false)).rejects.toThrow('catalog.changed');
    expect(load).not.toHaveBeenCalled();
  });
});

describe('catalog duplicate recovery', () => {
  it('rebuilds a prefix after compatibility filtering shifts a game between pages without a revision change', async () => {
    const old = [game('a')];
    const load = vi.fn().mockResolvedValueOnce({ ...page([game('b')]), page: 1 }).mockResolvedValueOnce(page([game('a')]));
    const result = await loadCatalogContinuation({ page: 2, revision: 3, compat: 'working' }, old, load, async () => page([game('a')]));
    expect(result.items.map(g => g.id)).toEqual(['b', 'a']);
    expect(result.refreshed).toBe(true);
    expect(result.result.page).toBe(2);
    expect(old.map(g => g.id)).toEqual(['a']);
  });
  it('stops after one recovery attempt when the server repeats a duplicate', async () => {
    const load = vi.fn().mockResolvedValueOnce({ ...page([game('a')]), page: 1 }).mockResolvedValueOnce(page([game('a')]));
    await expect(loadCatalogContinuation({ page: 2, revision: 3 }, [game('a')], load, async () => page([game('a')]))).rejects.toThrow('catalog_duplicate_page');
    expect(load).toHaveBeenCalledTimes(2);
  });

  it('folds explicit aliases across offline pages and keeps canonical metadata', async () => {
    const previous = [game('steam', { title: 'Same', aliasIds: ['igdb'] })];
    const canonical = game('igdb', {
      title: 'Same',
      developer: 'Author',
      externalIds: { igdb: '42' },
      providerLinks: { igdb: ['42'] },
      aliasIds: ['steam'],
    });
    const offlinePage: CatalogPage = {
      items: [canonical],
      total: 2,
      page: 2,
      pageSize: 1,
      revision: 3,
      offline: true,
    };

    const result = await loadCatalogContinuation(
      { page: 2, revision: 3 },
      previous,
      vi.fn(),
      async () => offlinePage,
    );

    expect(result.items).toHaveLength(1);
    expect(result.items[0].id).toBe('igdb');
    expect(result.items[0].developer).toBe('Author');
  });

  it('does not fold same-title offline homonyms without aliases', () => {
    const previous = [game('first', { title: 'Same' })];
    const next: CatalogPage = {
      ...page([game('second', { title: 'Same' })]),
      offline: true,
    };

    expect(appendOfflineCatalogPage(previous, next, 3).map((item) => item.id)).toEqual(['first', 'second']);
  });

  it('keeps online continuation on strict server identity checks', async () => {
    const previous = [game('steam', { aliasIds: ['igdb'] })];
    const canonical = game('igdb', { aliasIds: ['steam'] });
    const onlinePage: CatalogPage = { ...page([canonical]), offline: false };

    const result = await loadCatalogContinuation(
      { page: 2, revision: 3 },
      previous,
      vi.fn(),
      async () => onlinePage,
    );

    expect(result.items.map((item) => item.id)).toEqual(['steam', 'igdb']);
  });
});

describe('catalog prefix refresh', () => {
  it('keeps same-title games separate while rebuilding the loaded pages', async () => {
    const load = vi.fn(async (query): Promise<CatalogPage> => {
      const pageNumber = query.page ?? 1;
      return {
        items: [{ ...game(`fresh-${pageNumber}`), title: 'Same title' }],
        total: 3,
        page: pageNumber,
        pageSize: 1,
        revision: 7,
      };
    });

    const result = await reloadCatalogPrefix({ search: 'same', pageSize: 1 }, 3, load);

    expect(result.items.map((item) => item.id)).toEqual(['fresh-1', 'fresh-2', 'fresh-3']);
    expect(result.items.map((item) => item.title)).toEqual(['Same title', 'Same title', 'Same title']);
    expect(load.mock.calls.map(([query]) => [query.page, query.revision])).toEqual([[1, 0], [2, 7], [3, 7]]);
  });

  it('retries the complete prefix once after a changed revision', async () => {
    let first = true;
    const load = vi.fn(async (query): Promise<CatalogPage> => {
      if (first) {
        first = false;
        throw changed();
      }
      const pageNumber = query.page ?? 1;
      return { ...page([game(`fresh-${pageNumber}`)], 8), page: pageNumber, total: 2 };
    });

    const result = await reloadCatalogPrefix({ pageSize: 1 }, 2, load);

    expect(result.items.map((item) => item.id)).toEqual(['fresh-1', 'fresh-2']);
    expect(load).toHaveBeenCalledTimes(3);
    expect(load.mock.calls.map(([query]) => [query.page, query.revision])).toEqual([[1, 0], [1, 0], [2, 8]]);
  });
});
