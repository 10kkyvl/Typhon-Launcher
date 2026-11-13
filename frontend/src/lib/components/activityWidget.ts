import { msg, type MessageKey } from '../i18n';
import type { LibraryGame } from '../services/library';
import type { ActivityItem, ActivityKind, ActivityTone } from '../stores/activity';
import type { Route } from '../stores/router';
import { progressPercent } from '../utils/format';

/**
 * What the sidebar's activity widget shows: the items of the activity store (downloads, installs,
 * verifications, a move) and the running games, as one list of rows. Pure: the component feeds it the
 * stores and the clock.
 */

export type WidgetKind = ActivityKind | 'game';

export interface WidgetItem {
  key: string;
  kind: WidgetKind;
  /** What the row acts on and opens: the download (an install's download), the game, the move. */
  id: string;
  name: string;
  status: string;
  detail: string;
  /** 0..1; null for an item without a bar (a running game). */
  progress: number | null;
  indeterminate: boolean;
  tone: ActivityTone;
  attention: boolean;
  pausable: boolean;
  resumable: boolean;
  /** A running game: its cover. */
  cover?: string;
  /** A running game this launcher saw start: when, in ms since the epoch. */
  startedAt?: number;
}

export type Outcome = 'done' | 'failed' | null;

function fromActivity(item: ActivityItem): WidgetItem {
  return {
    key: item.key,
    kind: item.kind,
    id: item.downloadId,
    name: item.name,
    status: item.status,
    detail: item.detail,
    progress: item.progress,
    indeterminate: Boolean(item.indeterminate),
    tone: item.tone,
    attention: item.attention,
    pausable: item.pausable,
    resumable: item.resumable,
  };
}

function fromGame(id: string, game: LibraryGame | undefined, startedAt: number | undefined): WidgetItem {
  return {
    key: `game:${id}`,
    kind: 'game',
    id,
    name: game?.title || msg('ui.game'),
    status: msg('ui.activityPlaying'),
    detail: '',
    progress: null,
    indeterminate: false,
    tone: 'accent',
    attention: false,
    pausable: false,
    resumable: false,
    cover: game?.cover ?? '',
    startedAt,
  };
}

/** Every item going on, in the activity store's order (one that needs the user first), games last. */
export function widgetItems(
  activity: readonly ActivityItem[],
  running: ReadonlySet<string>,
  starts: ReadonlyMap<string, number>,
  games: readonly LibraryGame[],
): WidgetItem[] {
  const byId = new Map(games.map((game) => [game.id, game]));
  return [...activity.map(fromActivity), ...[...running].map((id) => fromGame(id, byId.get(id), starts.get(id)))];
}

/**
 * The rows that do not repeat the page in front of the user: the Downloads page lists the downloads
 * and the installs, a game's page shows that the game runs and its verification.
 */
export function shownItems(items: readonly WidgetItem[], route: Pick<Route, 'name' | 'params'>): WidgetItem[] {
  if (route.name === 'downloads') return items.filter((item) => item.kind !== 'download' && item.kind !== 'install');
  if (route.name === 'game') {
    const id = route.params.id;
    return items.filter((item) => !((item.kind === 'game' || item.kind === 'verify') && item.id === id));
  }
  return [...items];
}

/** The item the compact line shows: one that needs the user, else one that moves, else the first. */
export function leadOf(items: readonly WidgetItem[]): WidgetItem | null {
  return (
    items.find((item) => item.attention) ??
    items.find((item) => item.progress !== null && !item.indeterminate && !item.resumable) ??
    items[0] ??
    null
  );
}

/** A session's length as a clock: 4:07, 1:02:09. */
export function sessionClock(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000));
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const seconds = String(total % 60).padStart(2, '0');
  return hours > 0 ? `${hours}:${String(minutes).padStart(2, '0')}:${seconds}` : `${minutes}:${seconds}`;
}

/** What stands on an item's title line after its name: the percent, its state instead, or the session time. */
export function reading(item: WidgetItem, now: number): string {
  if (item.kind === 'game') return item.startedAt === undefined ? '' : sessionClock(now - item.startedAt);
  if (item.attention || item.resumable || item.indeterminate || item.progress === null) return item.status;
  return `${progressPercent(item.progress)}%`;
}

/** The line under a row's title in the panel. */
export function detailOf(item: WidgetItem, now: number): string {
  if (item.kind === 'game') {
    return item.startedAt === undefined ? msg('ui.activityPlaying') : msg('ui.activityPlayingFor', { time: sessionClock(now - item.startedAt) });
  }
  return [item.status, item.detail].filter(Boolean).join(' · ');
}

/** The value of a bar in words, when a percent alone would not say it: a paused or queued download. */
export function valueText(item: WidgetItem): string | undefined {
  if (!item.resumable || item.progress === null) return undefined;
  return msg('ui.activityPausedValue', { status: item.status, percent: progressPercent(item.progress) });
}

const APPEARED: Record<WidgetKind, MessageKey> = {
  download: 'ui.activityLiveDownload',
  install: 'ui.activityLiveInstall',
  verify: 'ui.activityLiveVerify',
  move: 'ui.activityLiveMove',
  game: 'ui.activityLiveGame',
};

/**
 * What the live region says between two states of the list: an item that appeared, one that came to
 * need the user, one that ended done or failed (`outcome` tells which, from the stores). Progress and a
 * changed status say nothing.
 */
export function liveMessages(before: ReadonlyMap<string, WidgetItem>, after: readonly WidgetItem[], outcome: (item: WidgetItem) => Outcome): string[] {
  const out: string[] = [];
  const now = new Set(after.map((item) => item.key));
  for (const [key, item] of before) {
    if (now.has(key)) continue;
    const ended = outcome(item);
    if (ended) out.push(msg(ended === 'done' ? 'ui.activityLiveDone' : 'ui.activityLiveFailed', { name: item.name }));
  }
  for (const item of after) {
    const was = before.get(item.key);
    if (item.attention && !was?.attention) out.push(msg('ui.activityLiveAttention', { name: item.name }));
    else if (!was) out.push(msg(APPEARED[item.kind], { name: item.name }));
  }
  return out;
}
