import type { DownloadUsage, GameUsage, GroupUsage, Leftover, LeftoverKind, Warning } from '../services/diskusage';
import { hasMessage, msg, type MessageKey } from '../i18n';
import { clockTime, longDate } from '../utils/format';

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
  if (at === null) return false;
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

/** What holds a game beyond the scan's flags: a move of it or of the whole library, its emulation. */
export interface GameHolds {
  moving: 'game' | 'library' | null;
  emulated: boolean;
}

/**
 * Why Move and Remove of a game row are off, each null when on. A move under way holds both (the
 * scan's `busy` may predate it); Go refuses to move any emulated game (library.CheckRelocatable:
 * emulators.game_unsupported), so its Move is off while Remove follows the other gates.
 */
export function gameGates(game: GameUsage, holds: GameHolds): { move: MessageKey | null; remove: MessageKey | null } {
  const held: MessageKey | null =
    holds.moving === 'library' ? 'diskusage.blockedLibraryMoving' : holds.moving === 'game' ? 'diskusage.blockedMoving' : gameBlockedKey(game);
  return { move: held ?? (holds.emulated ? 'diskusage.blockedEmulated' : null), remove: held };
}

/** The name of the game a group or a copy belongs to; "Unknown game" when nothing knows it. */
export function gameTitle(item: { title?: string; titleUnknown?: boolean }): string {
  const title = item.title?.trim() ?? '';
  return item.titleUnknown || !title ? msg('diskusage.unknownGame') : title;
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
  relocate_copy_staging: 'diskusage.kindRelocateCopyStaging',
  relocate_copy_target: 'diskusage.kindRelocateCopyTarget',
  relocate_copy_source: 'diskusage.kindRelocateCopySource',
  removal: 'diskusage.kindRemoval',
} satisfies Record<LeftoverKind, MessageKey>;

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
  return download.referenced;
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

/**
 * When a report was measured (its walk started), as the History screen says a time: the clock time in
 * words, the date and the time for the hint and for assistive tech. Null for a time it cannot read.
 */
export function measuredWhen(iso: string): { words: string; exact: string } | null {
  const at = new Date(iso);
  if (!iso || Number.isNaN(at.getTime())) return null;
  const time = clockTime(at);
  return { words: msg('diskusage.measuredAt', { time }), exact: msg('transfers.historyExactTime', { date: longDate(at), time }) };
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

/** Every reason code a report can carry: internal/diskusage/codes.go::reasonCodes (usage.test.ts reads it). */
export const REASON_CODES = [
  'diskusage.reason.owned_by_install',
  'diskusage.reason.only_copy',
  'diskusage.reason.no_record',
  'diskusage.reason.relocate_active',
  'diskusage.reason.relocate_journal',
  'diskusage.reason.holds_game',
  'diskusage.reason.update_owned',
  'diskusage.reason.symlink',
  'diskusage.reason.foreign_part',
  'diskusage.reason.owned_by_download',
  'diskusage.reason.seeding',
  'diskusage.reason.incomplete',
  'diskusage.reason.unreadable',
  'diskusage.reason.unknown',
  'diskusage.reason.games_unchecked',
  'diskusage.reason.relocate_copy',
  'diskusage.reason.relocate_unjournaled',
  'diskusage.reason.removal_pending',
  'diskusage.reason.removal_unjournaled',
  'diskusage.reason.removals_unreadable',
  'diskusage.reason.folder_held',
] as const satisfies readonly MessageKey[];

const REASONS: ReadonlySet<string> = new Set(REASON_CODES);

/** Why a row cannot be deleted, in words: its code's, or the row's plain "cannot be deleted". */
export function reasonLabel(code: string): string {
  return REASONS.has(code) && hasMessage(code) ? msg(code) : msg('diskusage.cannotDelete');
}

// A folder that may be the only copy of a game, or holds an installed game: the player is warned.
const WARNING_REASONS: ReadonlySet<string> = new Set(['diskusage.reason.only_copy', 'diskusage.reason.holds_game']);

/** How a row's reason reads: a warning for a folder that matters to a game, plain otherwise. */
export function reasonTone(code: string): 'warning' | 'plain' {
  return WARNING_REASONS.has(code) ? 'warning' : 'plain';
}

// The kinds that belong to one game: a move's copies and a removal's folder.
const GAME_KINDS: ReadonlySet<string> = new Set(['relocate_copy_staging', 'relocate_copy_target', 'relocate_copy_source', 'removal']);

/** Whether a leftover's row says whose game it is (Go names the game of a move's copy, when it knows it). */
export function leftoverOfGame(leftover: Leftover): boolean {
  return GAME_KINDS.has(leftover.kind) || leftover.kind.startsWith('relocate_copy_') || Boolean(leftover.gameId || leftover.title);
}

/** Every warning code a report can carry: internal/diskusage/codes.go::Warn* (usage.test.ts reads it). */
const WARNING_CODES = [
  'diskusage.warning.size_not_saved',
  'diskusage.warning.folder_unreadable',
  'diskusage.warning.moves_unchecked',
  'diskusage.warning.removals_unreadable',
  'diskusage.warning.screenshots_no_folder',
  'diskusage.warning.screenshots_unreadable',
  'diskusage.warning.snapshots_disabled',
  'diskusage.warning.snapshots_unreadable',
] as const satisfies readonly MessageKey[];

const WARNINGS: ReadonlySet<string> = new Set(WARNING_CODES);

/** A warning in words: its code's, or the plain "part of the report could not be measured". */
export function warningText(warning: Warning): string {
  return WARNINGS.has(warning.code) && hasMessage(warning.code) ? msg(warning.code) : msg('diskusage.warningUnknown');
}
