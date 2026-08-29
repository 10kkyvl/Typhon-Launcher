import { Service as HistoryService } from '../../../bindings/typhon/internal/history';
import { Kind, type Filter, type Record, type Status } from '../../../bindings/typhon/internal/history';
import { inWails } from './backend';

export { Kind };
export type { Record, Status };

export interface HistoryFilter {
  kinds?: Kind[];
  query?: string;
  limit?: number;
}

const unavailable = () => new Error('unavailable in browser');

const emptyStatus: Status = { degraded: false, message: '' };

function toFilter(filter: HistoryFilter): Filter {
  return {
    Kinds: filter.kinds && filter.kinds.length > 0 ? filter.kinds : null,
    Query: filter.query ?? '',
    Limit: filter.limit ?? 0,
  };
}

export async function listHistory(filter: HistoryFilter = {}): Promise<Record[]> {
  if (!inWails) return [];
  return ((await HistoryService.List(toFilter(filter))) ?? []) as unknown as Record[];
}

export async function recentHistory(n: number): Promise<Record[]> {
  if (!inWails) return [];
  return ((await HistoryService.Recent(n)) ?? []) as unknown as Record[];
}

export async function getHistoryStatus(): Promise<Status> {
  if (!inWails) return emptyStatus;
  return (await HistoryService.StatusOf()) as unknown as Status;
}

export async function clearHistory(): Promise<void> {
  if (!inWails) throw unavailable();
  await HistoryService.Clear();
}
