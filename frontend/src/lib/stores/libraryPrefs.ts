/**
 * The library's view choices. Grid or list and the sort stay across restarts (localStorage); the
 * filter, source, status, collection, search and the hidden hero stay for the session
 * (sessionStorage), so opening a game and coming back finds them as they were. Browser storage holds
 * these per-viewer conveniences only: it may throw or be missing, and then the choices live in memory.
 */
import { writable, type Writable } from 'svelte/store';
import type { LibraryFilter } from '../library/filters';
import { LIBRARY_SORTS, type LibrarySort } from '../library/query';
import { sourceOrder, type GameSource } from '../library/source';
import { STATUS_FILTERS, type StatusFilter } from '../library/statuses';

export interface LibraryChoices {
  filter: LibraryFilter;
  source: GameSource | 'all';
  status: StatusFilter;
  /** The id of the selected collection, '' for none. */
  collection: string;
  search: string;
  heroHidden: boolean;
}

export const DEFAULT_CHOICES: Readonly<LibraryChoices> = Object.freeze({
  filter: 'all',
  source: 'all',
  status: 'all',
  collection: '',
  search: '',
  heroHidden: false,
});

/** The filters the desktop page offers; a stored one it does not offer falls back to "All". */
const FILTERS: readonly LibraryFilter[] = ['all', 'installed', 'recent', 'favorites', 'hidden', 'watched'];
const MAX_TEXT = 200;

type Area = 'localStorage' | 'sessionStorage';

function storage(area: Area): Storage | undefined {
  try {
    return globalThis[area] ?? undefined;
  } catch {
    // Reading the property itself throws where storage is blocked: no storage, choices stay in memory.
    return undefined;
  }
}

function readStored(area: Area, key: string): unknown {
  try {
    const text = storage(area)?.getItem(key);
    return text ? JSON.parse(text) : undefined;
  } catch {
    // Unreadable or not JSON: the caller falls back to its default.
    return undefined;
  }
}

function writeStored(area: Area, key: string, value: unknown) {
  try {
    storage(area)?.setItem(key, JSON.stringify(value));
  } catch {
    // Full or blocked storage: the choice stays in memory for this run.
  }
}

/** A writable that starts from what `area` holds under `key` (through `parse`) and writes every change back. */
export function storedValue<T>(area: Area, key: string, parse: (raw: unknown) => T): Writable<T> {
  const store = writable(parse(readStored(area, key)));
  store.subscribe((value) => writeStored(area, key, value));
  return store;
}

function oneOf<T extends string>(list: readonly T[], value: unknown, fallback: T): T {
  return typeof value === 'string' && (list as readonly string[]).includes(value) ? (value as T) : fallback;
}

function text(value: unknown): string {
  return typeof value === 'string' && value.length <= MAX_TEXT ? value : '';
}

function parseChoices(raw: unknown): LibraryChoices {
  const stored = raw !== null && typeof raw === 'object' ? (raw as Record<string, unknown>) : {};
  return {
    filter: oneOf(FILTERS, stored.filter, DEFAULT_CHOICES.filter),
    source: oneOf<GameSource | 'all'>(['all', ...sourceOrder], stored.source, DEFAULT_CHOICES.source),
    status: oneOf(STATUS_FILTERS, stored.status, DEFAULT_CHOICES.status),
    collection: text(stored.collection),
    search: text(stored.search),
    heroHidden: stored.heroHidden === true,
  };
}

export const libraryChoices = storedValue('sessionStorage', 'typhon:library:choices', parseChoices);

export const librarySort = storedValue<LibrarySort>('localStorage', 'typhon:library:sort', (raw) => oneOf(LIBRARY_SORTS, raw, 'title'));
