export const STATUS_ORDER = ['playing', 'backlog', 'paused', 'completed', 'mastered', 'dropped'] as const;

export type StatusFilter = 'all' | 'none' | (typeof STATUS_ORDER)[number];

export const STATUS_FILTERS: readonly StatusFilter[] = ['all', ...STATUS_ORDER, 'none'];

interface Ranked {
  title: string;
  status?: string;
}

interface Dated extends Ranked {
  completedAt?: string | null;
}

export function matchesStatus(status: string | undefined, filter: StatusFilter): boolean {
  if (filter === 'all') return true;
  if (filter === 'none') return !status || !(STATUS_ORDER as readonly string[]).includes(status);
  return status === filter;
}

export function statusRank(status: string | undefined): number {
  const index = (STATUS_ORDER as readonly string[]).indexOf(status ?? '');
  return index < 0 ? STATUS_ORDER.length : index;
}

function byTitle(a: Ranked, b: Ranked): number {
  return a.title.localeCompare(b.title, 'ru');
}

export function compareByStatus(a: Ranked, b: Ranked): number {
  return statusRank(a.status) - statusRank(b.status) || byTitle(a, b);
}

function completedTime(entry: Dated): number {
  if (!entry.completedAt) return Number.NaN;
  return Date.parse(entry.completedAt);
}

export function compareByCompleted(a: Dated, b: Dated): number {
  const left = completedTime(a);
  const right = completedTime(b);
  const hasLeft = !Number.isNaN(left);
  const hasRight = !Number.isNaN(right);
  if (hasLeft && hasRight && left !== right) return right - left;
  if (hasLeft !== hasRight) return hasLeft ? -1 : 1;
  return byTitle(a, b);
}
