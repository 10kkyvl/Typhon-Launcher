import type { LibraryEntry } from './entries';
import { entryComparator, isRecent, type JustStopped } from './query';

/** How many slides the hero carousel shows, whatever the list's sort and filters are. */
export const HERO_SLIDES = 3;

/** How many games the "Recent" row shows; "Show all" lists the rest. */
export const RECENT_LIMIT = 12;

export interface PlayState {
  running: ReadonlySet<string>;
  /** Games that stopped since the list was read (see `JustStopped`). */
  stopped?: JustStopped;
  locale: string;
}

/**
 * The hero's slides: the last played games that are on disk, launchable and not hidden, newest first,
 * a running game first. Launchable, because the hero's main action plays the game. A game that has
 * just stopped stays first until the list that records its session arrives.
 */
export function buildSlides(entries: readonly LibraryEntry[], { running, stopped, locale }: PlayState): LibraryEntry[] {
  const candidates = entries.filter((entry) => entry.kind === 'game' && entry.installed && entry.playable && !entry.hidden);
  const live = candidates.find((entry) => running.has(entry.id));
  const played = candidates
    .filter((entry) => entry !== live && isRecent(entry, stopped))
    .toSorted(entryComparator('recent', locale, stopped));
  return [...(live ? [live] : []), ...played].slice(0, HERO_SLIDES);
}

/**
 * The "Recent" row: the games of the "Recent" filter in the same order, without the game the hero
 * shows (`shown`, while it is on screen), at most `limit`.
 */
export function recentEntries(
  entries: readonly LibraryEntry[],
  { shown, stopped, locale, limit = RECENT_LIMIT }: { shown?: string; stopped?: JustStopped; locale: string; limit?: number },
): LibraryEntry[] {
  return entries
    .filter((entry) => isRecent(entry, stopped) && entry.id !== shown)
    .toSorted(entryComparator('recent', locale, stopped))
    .slice(0, limit);
}
