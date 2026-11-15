import type { Translate } from '../../lib/i18n';
import type { Download } from '../../lib/services/downloads';
import type { Installation } from '../../lib/services/install';
import type { LibraryGame } from '../../lib/services/library';
import { installActive } from '../../lib/stores/install';
import type { Format } from '../../lib/utils/format';

/**
 * What became of a completed download: its installation is under way, waits for the player, failed,
 * was cancelled or interrupted, or finished; no installation took it yet (ready); or it is no download
 * an install takes (an update or a repair, which the updates service applies itself: Go's install
 * service skips every download whose purpose is not a release).
 */
export type CompletedKind = 'installing' | 'waiting' | 'failed' | 'stopped' | 'installed' | 'ready' | 'none';

/** The section a completed download is listed in. */
export type CompletedPart = 'installing' | 'completed';

/** The one classification of a completed download; the row and both sections read it. */
export function completedKind(
  download: Pick<Download, 'origin'>,
  installation: Pick<Installation, 'status'> | undefined,
): CompletedKind {
  if (!installation) return download.origin.purpose ? 'none' : 'ready';
  const { status } = installation;
  if (installActive(status)) return 'installing';
  if (status === 'waiting_for_user') return 'waiting';
  if (status === 'failed') return 'failed';
  if (status === 'completed') return 'installed';
  return 'stopped';
}

export function completedPart(kind: CompletedKind): CompletedPart {
  return kind === 'installing' || kind === 'waiting' ? 'installing' : 'completed';
}

/**
 * Whether "Remove from list" is offered: not while the installation runs or waits for the player. The
 * install reads the download's record, and a waiting row holds the screen's only Continue.
 */
export function canRemove(kind: CompletedKind): boolean {
  return completedPart(kind) === 'completed';
}

/**
 * Whether "Remove from list" asks first: on a download the screen can still install (ready, its
 * install failed or stopped). Go's install and its retry read the download's record
 * (internal/install/service.go `completedDownload`), so without it the game cannot be installed from
 * here, though the files stay.
 */
export function removeAsks(kind: CompletedKind): boolean {
  return kind === 'ready' || kind === 'failed' || kind === 'stopped';
}

/**
 * Whether "Clear the list" takes the download: only one that needs nothing more from the screen (an
 * installed one, update or repair files the updates apply), and not while it seeds, which only its own
 * row stops.
 */
export function canClear(download: Pick<Download, 'seeding'>, kind: CompletedKind): boolean {
  return (kind === 'installed' || kind === 'none') && !download.seeding;
}

/** "Clear the list" asks first when more than this many downloads would leave the list. */
export const CLEAR_ASKS_ABOVE = 5;

type DateFormat = Pick<Format, 'clockTime' | 'shortDate' | 'mediumDate' | 'longDate'>;

const startOfDay = (date: Date) => new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime();

/**
 * When a download finished: in words for the row ("today, 18:06", "yesterday, 21:44", "28 Oct, 18:02",
 * the year added for another year) and in full for assistive tech and the hint. Null without a time.
 */
export function completedWhen(
  iso: string | null | undefined,
  now: Date,
  format: DateFormat,
  t: Translate,
): { words: string; exact: string } | null {
  if (!iso) return null;
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return null;
  const time = format.clockTime(date);
  // Rounded: a day across a daylight-saving change is 23 or 25 hours long.
  const days = Math.round((startOfDay(now) - startOfDay(date)) / 86_400_000);
  let words: string;
  if (days <= 0) words = t('downloads.done.today', { time });
  else if (days === 1) words = t('downloads.done.yesterday', { time });
  else if (date.getFullYear() === now.getFullYear()) words = t('downloads.done.on', { date: format.shortDate(date), time });
  else words = t('downloads.done.on', { date: format.mediumDate(date), time });
  return { words, exact: t('downloads.done.exact', { date: format.longDate(date), time }) };
}

/** The library game an installation added, if the library lists it; `games` is `stores/library.ts::libraryGamesById`. */
export function installedGame(
  installation: Pick<Installation, 'gameId'> | undefined,
  games: ReadonlyMap<string, LibraryGame>,
): LibraryGame | undefined {
  const id = installation?.gameId;
  return id ? games.get(id) : undefined;
}
