import type { LibraryCollection } from '../services/library';
import { matchesCollection } from './collections';
import type { LibraryEntry } from './entries';
import type { LibraryFilter } from './filters';
import { sourceOrder, type GameSource } from './source';
import { matchesStatus, statusRank, type StatusFilter } from './statuses';

export type LibrarySort = 'title' | 'recent' | 'playtime' | 'size' | 'status' | 'completed' | 'added';

export const LIBRARY_SORTS: readonly LibrarySort[] = ['title', 'recent', 'playtime', 'size', 'status', 'completed', 'added'];

/** What narrows the list. `collection` is the collection itself; an id that names none narrows nothing. */
export interface LibraryQuery {
  filter: LibraryFilter;
  source: GameSource | 'all';
  status: StatusFilter;
  collection?: Pick<LibraryCollection, 'tags' | 'genres' | 'status'>;
  search: string;
}

export type Genres = Readonly<Record<string, string[] | undefined>>;

/**
 * Library games that stopped after the list was read, by stopping order (later is larger). Their
 * `lastPlayed` in the list predates the session that just ended, so they count as the last played.
 */
export type JustStopped = ReadonlyMap<string, number>;

const NONE: JustStopped = new Map();

export function playedAt(entry: Pick<LibraryEntry, 'lastPlayed'>): number {
  return timestamp(entry.lastPlayed);
}

function timestamp(value: string | null | undefined): number {
  const time = value ? Date.parse(value) : Number.NaN;
  return Number.isFinite(time) ? time : 0;
}

/** Case- and form-insensitive: full-width letters and ligatures match their plain forms. */
export function searchKey(text: string): string {
  return text.trim().normalize('NFKC').toLocaleLowerCase();
}

/** A played library game that is on disk and not hidden: what "Recent" holds, as a row or as a filter. */
export function isRecent(entry: LibraryEntry, stopped: JustStopped = NONE): boolean {
  return entry.kind === 'game' && entry.installed && !entry.hidden && (playedAt(entry) > 0 || stopped.has(entry.id));
}

/** Last played first: games that just stopped, the latest stop first, then by `lastPlayed`. */
export function byPlayOrder(stopped: JustStopped = NONE): (a: LibraryEntry, b: LibraryEntry) => number {
  return (a, b) => {
    const left = stopped.get(a.id);
    const right = stopped.get(b.id);
    if (left !== undefined && right !== undefined) return right - left;
    if (left !== undefined || right !== undefined) return left !== undefined ? -1 : 1;
    return playedAt(b) - playedAt(a);
  };
}

function passesFilter(entry: LibraryEntry, filter: LibraryFilter, stopped: JustStopped): boolean {
  switch (filter) {
    case 'installed':
      return entry.installed;
    case 'uninstalled':
      return entry.kind === 'game' && !entry.installed;
    case 'recent':
      return isRecent(entry, stopped);
    case 'favorites':
      return entry.favorite;
    default:
      return true;
  }
}

/**
 * The entries a query keeps, in their given order. "Watched" lists the watched games and nothing else;
 * source and status only narrow library games and downloads, and a collection only library games.
 * A hidden game is in "Hidden" and nowhere else.
 */
export function filterEntries(
  entries: readonly LibraryEntry[],
  watched: readonly LibraryEntry[],
  query: LibraryQuery,
  genres: Genres,
  stopped: JustStopped = NONE,
): LibraryEntry[] {
  const listed = query.filter === 'watched';
  const needle = searchKey(query.search);
  return (listed ? watched : entries).filter((entry) => {
    if (needle && !searchKey(entry.title).includes(needle)) return false;
    if (entry.hidden !== (query.filter === 'hidden')) return false;
    if (!listed && query.source !== 'all' && entry.source !== query.source) return false;
    if (!listed && query.status !== 'all' && !(entry.kind === 'game' && matchesStatus(entry.status, query.status))) return false;
    if (query.collection) {
      const own = entry.catalogId ? genres[entry.catalogId] : undefined;
      if (entry.kind !== 'game' || !matchesCollection(entry, own, query.collection)) return false;
    }
    return passesFilter(entry, query.filter, stopped);
  });
}

const collators = new Map<string, Intl.Collator>();

function collator(locale: string): Intl.Collator {
  let found = collators.get(locale);
  if (!found) {
    found = new Intl.Collator(locale);
    collators.set(locale, found);
  }
  return found;
}

type Compare = (a: LibraryEntry, b: LibraryEntry) => number;

/** Titles in the order of the interface language; equal titles by id, so the order never depends on input order. */
export function byTitle(locale: string): Compare {
  const { compare } = collator(locale);
  return (a, b) => compare(a.title, b.title) || (a.id < b.id ? -1 : a.id > b.id ? 1 : 0);
}

function completedTime(entry: LibraryEntry): number {
  return entry.completedAt ? timestamp(entry.completedAt) : 0;
}

export function entryComparator(sort: LibrarySort, locale: string, stopped: JustStopped = NONE): Compare {
  const title = byTitle(locale);
  switch (sort) {
    case 'recent': {
      const played = byPlayOrder(stopped);
      return (a, b) => played(a, b) || title(a, b);
    }
    case 'playtime':
      return (a, b) => b.playtimeSeconds - a.playtimeSeconds || title(a, b);
    case 'size':
      return (a, b) => b.sizeBytes - a.sizeBytes || title(a, b);
    case 'added':
      return (a, b) => timestamp(b.installedAt) - timestamp(a.installedAt) || title(a, b);
    case 'status':
      return (a, b) => statusRank(a.status) - statusRank(b.status) || title(a, b);
    case 'completed':
      return (a, b) => {
        const left = completedTime(a);
        const right = completedTime(b);
        if (left > 0 && right > 0 && left !== right) return right - left;
        if (left > 0 !== right > 0) return left > 0 ? -1 : 1;
        return title(a, b);
      };
    default:
      return title;
  }
}

/** "Recent" is in play order whatever sort is chosen. */
export function effectiveSort(filter: LibraryFilter, sort: LibrarySort): LibrarySort {
  return filter === 'recent' ? 'recent' : sort;
}

export function sortEntries(
  entries: readonly LibraryEntry[],
  sort: LibrarySort,
  locale: string,
  stopped: JustStopped = NONE,
): LibraryEntry[] {
  return entries.toSorted(entryComparator(sort, locale, stopped));
}

export function queryLibrary(
  entries: readonly LibraryEntry[],
  watched: readonly LibraryEntry[],
  query: LibraryQuery & { sort: LibrarySort },
  context: { genres: Genres; locale: string; stopped?: JustStopped },
): LibraryEntry[] {
  const kept = filterEntries(entries, watched, query, context.genres, context.stopped);
  return sortEntries(kept, effectiveSort(query.filter, query.sort), context.locale, context.stopped);
}

/**
 * The sources a source menu offers: those of the games the list can show, hidden games inside
 * "Hidden" and the others elsewhere, plus the selected source, so a choice that narrows the list is
 * always one the menu shows and can take back.
 */
export function sourceOptions(
  entries: readonly LibraryEntry[],
  { hidden = false, selected = 'all' }: { hidden?: boolean; selected?: GameSource | 'all' } = {},
): GameSource[] {
  const present = new Set<GameSource | 'all' | null>(entries.filter((entry) => entry.hidden === hidden).map((entry) => entry.source));
  present.add(selected);
  return sourceOrder.filter((source) => present.has(source));
}

/** Catalog genres of the library games that are not hidden, for the collection editor and the random pick. */
export function genreOptions(entries: readonly LibraryEntry[], genres: Genres): string[] {
  const found = new Set<string>();
  for (const entry of entries) {
    if (entry.kind !== 'game' || entry.hidden || !entry.catalogId) continue;
    for (const genre of genres[entry.catalogId] ?? []) found.add(genre);
  }
  return [...found];
}
