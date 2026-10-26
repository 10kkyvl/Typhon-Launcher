import { msg } from '../i18n';
import { mediumDate } from '../utils/format';

export const GAME_STATUSES = ['playing', 'completed', 'mastered', 'dropped', 'backlog', 'paused'] as const;

export type GameStatus = (typeof GAME_STATUSES)[number] | '';

export const STATUS_LABELS: Record<Exclude<GameStatus, ''>, string> = {
  get playing() {
    return msg('games.statusPlaying');
  },
  get completed() {
    return msg('games.statusCompleted');
  },
  get mastered() {
    return msg('games.statusMastered');
  },
  get dropped() {
    return msg('games.statusDropped');
  },
  get backlog() {
    return msg('games.statusBacklog');
  },
  get paused() {
    return msg('games.statusPaused');
  },
};

export function statusLabel(status: string | undefined): string {
  if (!status) return msg('games.statusNone');
  return STATUS_LABELS[status as Exclude<GameStatus, ''>] ?? msg('games.statusNone');
}

export function friendStatusLabel(status: string | undefined): string {
  if (!status) return '';
  if (status === 'playing') return msg('games.statusPlayingOther');
  return STATUS_LABELS[status as Exclude<GameStatus, ''>] ?? '';
}

export function statusBadgeKind(status: string | undefined): 'success' | 'accent' | 'neutral' | 'warning' {
  switch (status) {
    case 'completed':
    case 'mastered':
      return 'success';
    case 'playing':
      return 'accent';
    case 'dropped':
      return 'warning';
    default:
      return 'neutral';
  }
}

export function isFinished(status: string | undefined): boolean {
  return status === 'completed' || status === 'mastered';
}

export function completionLine(status: string | undefined, completedAt: string | null | undefined): string {
  if (!isFinished(status)) return '';
  if (!completedAt) return msg('games.completedUnknown');
  const date = new Date(completedAt);
  if (Number.isNaN(date.getTime())) return msg('games.completedUnknown');
  return msg('games.completedOn', { date: mediumDate(date) });
}
