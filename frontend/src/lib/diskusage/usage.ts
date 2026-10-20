import type { DownloadUsage, GameUsage, GroupUsage, Leftover } from '../services/diskusage';
import { hasMessage, msg, type MessageKey } from '../i18n';

export const STALE_DAYS = 90;

const DAY_MS = 86400000;

export type GameSort = 'size' | 'played';

export type Age =
  | { kind: 'never' }
  | { kind: 'days'; count: number }
  | { kind: 'months'; count: number }
  | { kind: 'years'; count: number };

function playedAt(iso: string | null): number | null {
  if (!iso) return null;
  const at = Date.parse(iso);
  return Number.isNaN(at) ? null : at;
}

export function ageOf(iso: string | null, now: number): Age {
  const at = playedAt(iso);
  if (at === null) return { kind: 'never' };
  const days = Math.max(0, Math.floor((now - at) / DAY_MS));
  if (days < 30) return { kind: 'days', count: days };
  if (days < 365) return { kind: 'months', count: Math.floor(days / 30) };
  return { kind: 'years', count: Math.floor(days / 365) };
}

export function isStale(iso: string | null, now: number): boolean {
  const at = playedAt(iso);
  if (at === null) return true;
  return now - at > STALE_DAYS * DAY_MS;
}

function bySize(a: { sizeBytes: number; sizeUnknown: boolean }, b: { sizeBytes: number; sizeUnknown: boolean }) {
  if (a.sizeUnknown !== b.sizeUnknown) return a.sizeUnknown ? 1 : -1;
  return b.sizeBytes - a.sizeBytes;
}

export function sortGames(games: GameUsage[], by: GameSort): GameUsage[] {
  return games.toSorted((a, b) => {
    if (by === 'played') {
      const left = playedAt(a.lastPlayed);
      const right = playedAt(b.lastPlayed);
      if (left === null && right !== null) return -1;
      if (left !== null && right === null) return 1;
      if (left !== null && right !== null && left !== right) return left - right;
    }
    return bySize(a, b) || a.title.localeCompare(b.title, 'ru');
  });
}

export function gameBlockedKey(game: GameUsage): MessageKey | null {
  if (game.running) return 'diskusage.blockedRunning';
  if (game.busy) return 'diskusage.blockedBusy';
  return null;
}

export function canManageGame(game: GameUsage): boolean {
  return !game.external && !game.running && !game.busy;
}

const LEFTOVER_KEYS: Record<string, MessageKey> = {
  install_partial: 'diskusage.kindInstallPartial',
  download_part: 'diskusage.kindDownloadPart',
  relocate_staging: 'diskusage.kindRelocateStaging',
  update_staging: 'diskusage.kindUpdateStaging',
  update_previous: 'diskusage.kindUpdatePrevious',
  update_replaced: 'diskusage.kindUpdateReplaced',
  download_orphan: 'diskusage.kindDownloadOrphan',
};

export function leftoverKindKey(kind: string): MessageKey {
  return LEFTOVER_KEYS[kind] ?? 'diskusage.kindUnknown';
}

export function sizeText(
  bytes: number,
  unknown: boolean,
  format: (bytes: number) => string,
  unknownText: string,
): string {
  if (unknown && bytes <= 0) return unknownText;
  return unknown ? `≥ ${format(bytes)}` : format(bytes);
}

export function totalText(
  bytes: number,
  unknown: boolean,
  format: (bytes: number) => string,
  atLeast: (size: string) => string,
): string {
  return unknown ? atLeast(format(bytes)) : format(bytes);
}

export function sizeSorted<T extends { sizeBytes: number; sizeUnknown: boolean }>(items: T[]): T[] {
  return items.toSorted(bySize);
}

export function downloadBlocked(download: DownloadUsage): boolean {
  return download.referenced || download.seeding;
}

export function countUnknown(report: {
  games: GameUsage[];
  downloads: DownloadUsage[];
  leftovers: Leftover[];
  screenshots: GroupUsage[];
  snapshots: GroupUsage[];
}): number {
  const flags = [
    ...report.games,
    ...report.downloads,
    ...report.leftovers,
    ...report.screenshots,
    ...report.snapshots,
  ];
  return flags.filter((item) => item.sizeUnknown).length;
}

export function ageText(age: Age): string {
  switch (age.kind) {
    case 'never':
      return msg('diskusage.neverPlayed');
    case 'days':
      if (age.count <= 0) return msg('format.today');
      if (age.count === 1) return msg('format.yesterday');
      return msg('format.daysAgo', { count: age.count });
    case 'months':
      return msg('diskusage.monthsAgo', { count: age.count });
    case 'years':
      return msg('diskusage.yearsAgo', { count: age.count });
  }
}

export const REASON_CODES = [
  'owned_by_install',
  'only_copy',
  'no_record',
  'relocate_active',
  'relocate_journal',
  'holds_game',
  'update_owned',
  'symlink',
  'foreign_part',
  'owned_by_download',
  'seeding',
  'incomplete',
  'unreadable',
  'unknown',
] as const;

export function reasonLabel(code: string, fallback: string): string {
  return hasMessage(code) ? msg(code) : fallback;
}
