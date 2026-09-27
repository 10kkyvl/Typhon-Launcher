import { describe, expect, it } from 'vitest';
import { catalogWithoutDiscovery } from './display';
import { appendCatalogPage } from './pages';
import type { CatalogGame, CatalogPage } from '../services/sources';
import type { Recommendation } from '../recommendations/display';

const game = (id: string, extra: Partial<CatalogGame> = {}): CatalogGame => ({ id, title: id, sortTitle: id, createdAt: '', ...extra });
const pick = (game: CatalogGame): Recommendation => ({ game, reason: 'popular' });

describe('catalog and independently loaded discovery', () => {
  it('hides canonical and legacy aliases without merging different editions by title or provider alone', () => {
    const items = [game('same'), game('local', { serverId: 'server' }), game('redirect', { aliasIds: ['old'] }),
      game('left', { aliasIds: ['common'] }), game('edition', { title: 'same', externalIds: { steam: '42' } })];
    const picks = [pick(game('same', { externalIds: { steam: '42' } })), pick(game('server')),
      pick(game('old')), pick(game('right', { aliasIds: ['common'] }))];
    expect(catalogWithoutDiscovery(items, picks).map((item) => item.id)).toEqual(['edition']);
    expect(items).toHaveLength(5);
  });

  it('keeps raw pages intact while late picks and refreshed picks move between the shelf and grid', () => {
    const first = [game('a'), game('b'), game('c')];
    const next: CatalogPage = { items: [game('d'), game('e'), game('f')], total: 6, page: 2, pageSize: 3, revision: 7 };
    expect(catalogWithoutDiscovery(first, [])).toEqual(first);
    expect(catalogWithoutDiscovery(first, [pick(game('b'))]).map((item) => item.id)).toEqual(['a', 'c']);
    const all = appendCatalogPage(first, next, 7);
    expect(catalogWithoutDiscovery(all, [pick(game('b')), pick(game('e'))]).map((item) => item.id)).toEqual(['a', 'c', 'd', 'f']);
    expect(catalogWithoutDiscovery(all, [pick(game('d'))]).map((item) => item.id)).toEqual(['a', 'b', 'c', 'e', 'f']);
    expect(all.map((item) => item.id)).toEqual(['a', 'b', 'c', 'd', 'e', 'f']);
    expect(first).toHaveLength(3);
  });
});
