import { describe, expect, it } from 'vitest';
import { loadCatalogContinuation } from './pages';
import { applyFreshPage, canWarmNextPage, createFreshPageBuffer, staleKeyOf } from './stale';
import type { CatalogPage } from '../services/sources';

function page(patch: Partial<CatalogPage> = {}): CatalogPage {
  return { items: [], total: 0, page: 1, pageSize: 60, revision: 7, snapshot: 's1', ...patch };
}

const shown = { revision: 7, page: 1, fallback: false };

describe('applyFreshPage', () => {
  it('replaces in place when revision and fallback match', () => {
    const fresh = page();
    expect(applyFreshPage('k', shown, { key: 'k', page: fresh })).toEqual({ action: 'replace', page: fresh });
  });

  it('reloads when the revision differs', () => {
    expect(applyFreshPage('k', shown, { key: 'k', page: page({ revision: 8 }) })).toEqual({ action: 'reload' });
  });

  it('replaces when only the frozen snapshot differs', () => {
    const fresh = page({ snapshot: 's2' });
    expect(applyFreshPage('k', shown, { key: 'k', page: fresh })).toEqual({ action: 'replace', page: fresh });
  });

  it('reloads when the personalization fallback differs', () => {
    expect(applyFreshPage('k', shown, { key: 'k', page: page({ personalizationFallback: true }) })).toEqual({ action: 'reload' });
    expect(applyFreshPage('k', { ...shown, fallback: true }, { key: 'k', page: page() })).toEqual({ action: 'reload' });
  });

  it('ignores a same-revision page once the user paged beyond the first page', () => {
    expect(applyFreshPage('k', { ...shown, page: 2 }, { key: 'k', page: page() })).toEqual({ action: 'ignore' });
  });

  it('reloads a different revision once the user paged beyond the first page', () => {
    expect(applyFreshPage('k', { ...shown, page: 2 }, { key: 'k', page: page({ revision: 8 }) })).toEqual({ action: 'reload' });
  });

  it('reports a failed background refresh with its error', () => {
    expect(applyFreshPage('k', shown, { key: 'k', page: page(), failed: true, error: 'catalog.changed' })).toEqual({ action: 'failed', error: 'catalog.changed' });
    expect(applyFreshPage('k', { ...shown, page: 3 }, { key: 'k', page: page(), failed: true })).toEqual({ action: 'failed', error: '' });
  });

  it('ignores a failure for another key', () => {
    expect(applyFreshPage('k', shown, { key: 'other', page: page(), failed: true })).toEqual({ action: 'ignore' });
  });

  it('ignores an event for another key', () => {
    expect(applyFreshPage('k', shown, { key: 'other', page: page() })).toEqual({ action: 'ignore' });
  });

  it('ignores every event when the shown page is not stale', () => {
    expect(applyFreshPage('', shown, { key: '', page: page() })).toEqual({ action: 'ignore' });
  });

  it('ignores a missing payload', () => {
    expect(applyFreshPage('k', shown, undefined)).toEqual({ action: 'ignore' });
  });
});

describe('stale pages and the next-page prefetch', () => {
  it('reads the key only from a stale page', () => {
    expect(staleKeyOf(page({ stale: true, key: 'k' }))).toBe('k');
    expect(staleKeyOf(page({ stale: false, key: 'k' }))).toBe('');
    expect(staleKeyOf(page())).toBe('');
  });

  it('does not warm the next page while the shown page is stale', () => {
    expect(canWarmNextPage({ offline: false, staleKey: 'k', loaded: 60, total: 200 })).toBe(false);
  });

  it('warms the next page for a fresh page with more items', () => {
    expect(canWarmNextPage({ offline: false, staleKey: '', loaded: 60, total: 200 })).toBe(true);
  });

  it('does not warm offline or when everything is loaded', () => {
    expect(canWarmNextPage({ offline: true, staleKey: '', loaded: 60, total: 200 })).toBe(false);
    expect(canWarmNextPage({ offline: false, staleKey: '', loaded: 200, total: 200 })).toBe(false);
  });

  it('keeps stale and key through the first page continuation', async () => {
    const stale = page({ stale: true, key: 'k', total: 1 });
    const out = await loadCatalogContinuation(
      { page: 1, pageSize: 60, stable: true },
      [],
      async () => stale,
      async () => stale,
    );
    expect(staleKeyOf(out.result)).toBe('k');
  });
});

describe('createFreshPageBuffer', () => {
  it('hands back an event that arrived before the stale page it refreshes', () => {
    const buffer = createFreshPageBuffer();
    const event = { key: 'k', page: page() };
    buffer.remember(event);
    expect(buffer.take('k')).toBe(event);
    expect(buffer.take('k')).toBeUndefined();
  });

  it('ignores events without a key or a page', () => {
    const buffer = createFreshPageBuffer();
    buffer.remember(undefined);
    buffer.remember({ key: '', page: page() });
    buffer.remember({ key: 'k', page: undefined as unknown as CatalogPage });
    expect(buffer.take('k')).toBeUndefined();
    expect(buffer.take('')).toBeUndefined();
  });

  it('keeps only the newest events up to the limit', () => {
    const buffer = createFreshPageBuffer(2);
    buffer.remember({ key: 'a', page: page() });
    buffer.remember({ key: 'b', page: page() });
    buffer.remember({ key: 'c', page: page() });
    expect(buffer.take('a')).toBeUndefined();
    expect(buffer.take('b')).toBeDefined();
    expect(buffer.take('c')).toBeDefined();
  });

  it('keeps a failure that arrived before the stale response', () => {
    const buffer = createFreshPageBuffer();
    const event = { key: 'k', page: page(), failed: true, error: 'catalog.backend_outdated' };
    buffer.remember(event);
    expect(buffer.take('k')).toBe(event);
  });

  it('forgets everything on clear', () => {
    const buffer = createFreshPageBuffer();
    buffer.remember({ key: 'k', page: page() });
    buffer.clear();
    expect(buffer.take('k')).toBeUndefined();
  });
});
