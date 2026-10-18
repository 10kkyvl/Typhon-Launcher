import type { CatalogPage } from '../services/sources';

export interface FreshPageEvent {
  key: string;
  page: CatalogPage;
  failed?: boolean;
  error?: string;
}

export interface ShownPage {
  revision: number;
  page: number;
  fallback: boolean;
}

export type FreshPageAction =
  | { action: 'ignore' }
  | { action: 'reload' }
  | { action: 'failed'; error: string }
  | { action: 'replace'; page: CatalogPage };

export function staleKeyOf(page: CatalogPage): string {
  return page.stale === true ? (page.key ?? '') : '';
}

export function canWarmNextPage(state: { offline: boolean; staleKey: string; loaded: number; total: number }): boolean {
  return !state.offline && state.staleKey === '' && state.loaded < state.total;
}

export function applyFreshPage(staleKey: string, shown: ShownPage, event: FreshPageEvent | undefined): FreshPageAction {
  if (!staleKey || !event || event.key !== staleKey) return { action: 'ignore' };
  if (event.failed) return { action: 'failed', error: event.error ?? '' };
  if (!event.page) return { action: 'ignore' };
  const sameRevision = event.page.revision === shown.revision;
  if (shown.page > 1) return sameRevision ? { action: 'ignore' } : { action: 'reload' };
  if (!sameRevision || (event.page.personalizationFallback ?? false) !== shown.fallback) return { action: 'reload' };
  return { action: 'replace', page: event.page };
}

export interface FreshPageBuffer {
  remember(event: FreshPageEvent | undefined): void;
  take(key: string): FreshPageEvent | undefined;
  clear(): void;
}

export function createFreshPageBuffer(limit = 4): FreshPageBuffer {
  const events = new Map<string, FreshPageEvent>();
  return {
    remember(event) {
      if (!event?.key || (!event.page && !event.failed)) return;
      events.delete(event.key);
      events.set(event.key, event);
      while (events.size > limit) {
        const oldest = events.keys().next().value;
        if (oldest === undefined) break;
        events.delete(oldest);
      }
    },
    take(key) {
      const event = events.get(key);
      if (event) events.delete(key);
      return event;
    },
    clear() {
      events.clear();
    },
  };
}
