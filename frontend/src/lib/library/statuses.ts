export const STATUS_ORDER = ['playing', 'backlog', 'paused', 'completed', 'mastered', 'dropped'] as const;

export type StatusFilter = 'all' | 'none' | (typeof STATUS_ORDER)[number];

export const STATUS_FILTERS: readonly StatusFilter[] = ['all', ...STATUS_ORDER, 'none'];

export function matchesStatus(status: string | undefined, filter: StatusFilter): boolean {
  if (filter === 'all') return true;
  if (filter === 'none') return !status || !(STATUS_ORDER as readonly string[]).includes(status);
  return status === filter;
}

export function statusRank(status: string | undefined): number {
  const index = (STATUS_ORDER as readonly string[]).indexOf(status ?? '');
  return index < 0 ? STATUS_ORDER.length : index;
}
