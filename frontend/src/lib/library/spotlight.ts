import type { LibraryEntry } from './entries';
import { entryComparator, isRecent } from './query';

/** How many slides the hero carousel shows, whatever the list's sort and filters are. */
export const HERO_SLIDES = 3;

/** How many games the "Recent" row shows; "Show all" lists the rest. */
export const RECENT_LIMIT = 12;

/**
 * The hero's slides: the last played games that are on disk, launchable and not hidden, newest first,
 * a running game first. Launchable, because the hero's main action plays the game.
 */
export function buildSlides(entries: readonly LibraryEntry[], running: ReadonlySet<string>, locale: string): LibraryEntry[] {
  const candidates = entries.filter((entry) => entry.kind === 'game' && entry.installed && entry.playable && !entry.hidden);
  const live = candidates.find((entry) => running.has(entry.id));
  const played = candidates.filter((entry) => entry !== live && isRecent(entry)).toSorted(entryComparator('recent', locale));
  return [...(live ? [live] : []), ...played].slice(0, HERO_SLIDES);
}

/**
 * The "Recent" row: the games of the "Recent" filter in the same order, without the game the hero
 * shows (`shown`, while it is on screen), at most `limit`.
 */
export function recentEntries(
  entries: readonly LibraryEntry[],
  shown: string | undefined,
  locale: string,
  limit: number = RECENT_LIMIT,
): LibraryEntry[] {
  return entries
    .filter((entry) => isRecent(entry) && entry.id !== shown)
    .toSorted(entryComparator('recent', locale))
    .slice(0, limit);
}
