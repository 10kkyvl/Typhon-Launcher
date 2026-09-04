import { msg } from '../i18n';

export const GAME_STATUSES = ['playing', 'completed', 'dropped', 'backlog', 'paused'] as const;

export type GameStatus = (typeof GAME_STATUSES)[number] | '';

export const STATUS_LABELS: Record<Exclude<GameStatus, ''>, string> = {
  get playing() {
    return msg('games.statusPlaying');
  },
  get completed() {
    return msg('games.statusCompleted');
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

export function statusBadgeKind(status: string | undefined): 'success' | 'accent' | 'neutral' | 'warning' {
  switch (status) {
    case 'completed':
      return 'success';
    case 'playing':
      return 'accent';
    case 'dropped':
      return 'warning';
    default:
      return 'neutral';
  }
}
