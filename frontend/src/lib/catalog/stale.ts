import type { CatalogPage } from '../services/sources';

export interface FreshPageEvent {
  key: string;
  page: CatalogPage;
}

export interface ShownPage {
  revision: number;
  snapshot: string;
  page: number;
}

export type FreshPageAction =
  | { action: 'ignore' }
  | { action: 'reload' }
  | { action: 'replace'; page: CatalogPage };

export function staleKeyOf(page: CatalogPage): string {
  return page.stale === true ? (page.key ?? '') : '';
}

export function canWarmNextPage(state: { offline: boolean; staleKey: string; loaded: number; total: number }): boolean {
  return !state.offline && state.staleKey === '' && state.loaded < state.total;
}

export function applyFreshPage(staleKey: string, shown: ShownPage, event: FreshPageEvent | undefined): FreshPageAction {
  if (!staleKey || !event || event.key !== staleKey || !event.page) return { action: 'ignore' };
  if (shown.page > 1) return { action: 'reload' };
  if (event.page.revision !== shown.revision || (event.page.snapshot ?? '') !== shown.snapshot) return { action: 'reload' };
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
      if (!event?.key || !event.page) return;
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
