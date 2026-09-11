import { errorCode } from '../i18n/errors';
import type { CatalogGame, CatalogPage, CatalogQuery } from '../services/sources';

// Reject a mismatched continuation before publishing any of it to the view.
// Previous items remain intact so retry can restart the query explicitly.
export function appendCatalogPage(previous: CatalogGame[], page: CatalogPage, revision: number): CatalogGame[] {
  if (revision && page.revision !== revision) throw new Error('typhon:catalog.changed: catalog revision changed');
  const seen = new Set(previous.map((g) => g.serverId || g.id));
  for (const game of page.items) {
    const id = game.serverId || game.id;
    if (seen.has(id)) throw new Error('catalog_duplicate_page');
    seen.add(id);
  }
  return [...previous, ...page.items];
}

// Publish a refreshed prefix only after all of its pages share one revision.
export async function loadCatalogContinuation(
  query: CatalogQuery,
  previous: CatalogGame[],
  load: (query: CatalogQuery) => Promise<CatalogPage>,
  initial: () => Promise<CatalogPage>,
  active: () => boolean = () => true,
): Promise<{ result: CatalogPage; items: CatalogGame[]; refreshed: boolean }> {
  try {
    const result = await initial();
    const items = query.page === 1 ? result.items : appendCatalogPage(previous, result, query.revision ?? 0);
    return { result, items, refreshed: false };
  } catch (err) {
    if (errorCode(err) !== 'catalog.changed' || (query.page ?? 1) <= 1 || !active()) throw err;
    let result = await load({ ...query, page: 1, revision: 0 });
    let items = result.items;
    const compat = { ...result.compat };
    for (let page = 2; page <= (query.page ?? 1) && items.length < result.total; page++) {
      if (!active()) throw err;
      const next = await load({ ...query, page, revision: result.revision });
      items = appendCatalogPage(items, next, result.revision ?? 0);
      Object.assign(compat, next.compat);
      result = next;
    }
    return { result: { ...result, compat }, items, refreshed: true };
  }
}
