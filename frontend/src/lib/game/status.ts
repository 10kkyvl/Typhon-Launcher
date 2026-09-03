export const GAME_STATUSES = ['playing', 'completed', 'dropped', 'backlog', 'paused'] as const;

export type GameStatus = (typeof GAME_STATUSES)[number] | '';

export const STATUS_LABELS: Record<Exclude<GameStatus, ''>, string> = {
  playing: 'Играю',
  completed: 'Пройдена',
  dropped: 'Брошена',
  backlog: 'В планах',
  paused: 'На паузе',
};

export function statusLabel(status: string | undefined): string {
  if (!status) return 'Без статуса';
  return STATUS_LABELS[status as Exclude<GameStatus, ''>] ?? 'Без статуса';
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
