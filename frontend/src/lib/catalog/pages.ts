import { errorCode } from '../i18n/errors';
import type { CatalogGame, CatalogPage, CatalogQuery, CompatInfo } from '../services/sources';

// Reject a mismatched continuation before publishing any of it to the view.
// Previous items remain intact so retry can restart the query explicitly.
export function appendCatalogPage(previous: CatalogGame[], page: CatalogPage, revision: number): CatalogGame[] {
  if (revision && page.revision !== revision) throw new Error('typhon:catalog.changed: catalog revision changed');
  const seen = new Set(previous.map((g) => g.serverId || g.id));
  for (const game of page.items) {
    const id = game.serverId || game.id;
    if (seen.has(id)) throw new Error('typhon:catalog.changed: catalog_duplicate_page');
    seen.add(id);
  }
  return [...previous, ...page.items];
}

function explicitAliasMatch(left: CatalogGame, right: CatalogGame): boolean {
  if (left.id === right.id) return true;
  if (left.serverId && right.serverId && left.serverId === right.serverId) return true;
  const leftIDs = new Set([left.id, ...(left.aliasIds ?? [])]);
  const rightIDs = new Set([right.id, ...(right.aliasIds ?? [])]);
  for (const id of leftIDs) if (rightIDs.has(id)) return true;
  return false;
}

function canonicalEvidenceScore(game: CatalogGame): number {
  return Number(Boolean(game.externalIds?.igdb)) * 4
    + (game.providerLinks?.igdb?.length ?? 0) * 2
    + Number(Boolean(game.developer));
}

// Offline page membership can come from separate cache files. The server may
// have learned an alias after those files were written, so only explicit IDs
// in aliasIds may fold rows across pages. Titles and provider IDs are not
// enough evidence for this fallback path.
export function appendOfflineCatalogPage(previous: CatalogGame[], page: CatalogPage, revision: number): CatalogGame[] {
  if (revision && page.revision !== revision) throw new Error('typhon:catalog.changed: catalog revision changed');
  const items = [...previous];
  for (const game of page.items) {
    const duplicate = items.findIndex((existing) => explicitAliasMatch(existing, game));
    if (duplicate < 0) {
      items.push(game);
      continue;
    }
    if (canonicalEvidenceScore(game) > canonicalEvidenceScore(items[duplicate])) items[duplicate] = game;
  }
  return items;
}

function appendCatalogContinuation(
  previous: CatalogGame[],
  page: CatalogPage,
  revision: number,
  offline: boolean,
): CatalogGame[] {
  return offline || page.offline
    ? appendOfflineCatalogPage(previous, page, revision)
    : appendCatalogPage(previous, page, revision);
}

export async function reloadCatalogPrefix(
  query: CatalogQuery,
  pageCount: number,
  load: (query: CatalogQuery) => Promise<CatalogPage>,
  active: () => boolean = () => true,
): Promise<{ result: CatalogPage; items: CatalogGame[]; compat: Record<string, CompatInfo> }> {
  const targetPage = Math.max(1, pageCount);
  let lastError: unknown;

  // A metadata update can race one backend revision change. Retry the complete
  // prefix once, then leave the current view intact for a later user action.
  for (let attempt = 0; attempt < 2; attempt++) {
    try {
      let result = await load({ ...query, page: 1, revision: 0 });
      let items = [...result.items];
      let compat = { ...(result.compat ?? {}) };
      let offline = result.offline === true;
      for (let page = 2; page <= targetPage && items.length < result.total; page++) {
        if (!active()) throw new Error('catalog.refresh_cancelled');
        const next = await load({ ...query, page, revision: result.revision ?? 0 });
        items = appendCatalogContinuation(items, next, result.revision ?? 0, offline);
        compat = { ...compat, ...(next.compat ?? {}) };
        offline ||= next.offline === true;
        result = next;
      }
      return { result: { ...result, offline }, items, compat };
    } catch (err) {
      lastError = err;
      if (errorCode(err) !== 'catalog.changed' || attempt === 1 || !active()) throw err;
    }
  }

  throw lastError instanceof Error ? lastError : new Error('catalog.refresh_failed');
}

// Publish a refreshed prefix only after all of its pages share one revision.
export async function loadCatalogContinuation(
  query: CatalogQuery,
  previous: CatalogGame[],
  load: (query: CatalogQuery) => Promise<CatalogPage>,
  initial: () => Promise<CatalogPage>,
  active: () => boolean = () => true,
  offline = false,
): Promise<{ result: CatalogPage; items: CatalogGame[]; refreshed: boolean }> {
  try {
    const result = await initial();
    const items = query.page === 1
      ? result.items
      : appendCatalogContinuation(previous, result, query.revision ?? 0, offline);
    return { result: { ...result, offline: result.offline || (query.page !== 1 && offline) }, items, refreshed: false };
  } catch (err) {
    if (errorCode(err) !== 'catalog.changed' || (query.page ?? 1) <= 1 || !active()) throw err;
    let result = await load({ ...query, page: 1, revision: 0 });
    let items = result.items;
    const compat = { ...result.compat };
    let offline = result.offline === true;
    for (let page = 2; page <= (query.page ?? 1) && items.length < result.total; page++) {
      if (!active()) throw err;
      const next = await load({ ...query, page, revision: result.revision });
      items = appendCatalogContinuation(items, next, result.revision ?? 0, offline);
      Object.assign(compat, next.compat);
      offline ||= next.offline === true;
      result = next;
    }
    return { result: { ...result, compat, offline }, items, refreshed: true };
  }
}
