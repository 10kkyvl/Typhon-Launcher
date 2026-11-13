import type { LibraryQuery } from './query';

export type Narrowing = 'search' | 'filter' | 'source' | 'status' | 'collection';

export type EmptyReason =
  | { kind: 'loading' }
  | { kind: 'failed' }
  | { kind: 'empty' }
  | { kind: 'all-hidden' }
  | { kind: 'no-match'; narrowing: Narrowing[] };

export interface EmptyInput {
  /** The library's first list has arrived. */
  loaded: boolean;
  /** The first list could not be read (and none has arrived since). */
  failed: boolean;
  /** Library games and downloads, hidden ones included. */
  entries: number;
  watched: number;
  /** What the query keeps. */
  shown: number;
  query: LibraryQuery;
}

/** The choices that narrow the list right now; "Watched" is not narrowed by source or status. */
export function narrowing(query: LibraryQuery): Narrowing[] {
  const listed = query.filter === 'watched';
  const found: Narrowing[] = [];
  if (query.search.trim()) found.push('search');
  if (query.filter !== 'all') found.push('filter');
  if (!listed && query.source !== 'all') found.push('source');
  if (!listed && query.status !== 'all') found.push('status');
  if (query.collection) found.push('collection');
  return found;
}

/** Why the list is empty, or null when it is not. */
export function emptyReason(input: EmptyInput): EmptyReason | null {
  if (input.shown > 0) return null;
  if (input.failed) return { kind: 'failed' };
  if (!input.loaded) return { kind: 'loading' };
  if (input.entries === 0 && input.watched === 0) return { kind: 'empty' };
  const by = narrowing(input.query);
  if (by.length === 0) return input.entries === 0 ? { kind: 'empty' } : { kind: 'all-hidden' };
  return { kind: 'no-match', narrowing: by };
}
