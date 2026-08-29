import type { Kind, Record } from '../../../bindings/typhon/internal/history';

export interface HistoryFilterOptions {
  kinds?: Kind[];
  query?: string;
}

export function filterHistory(records: Record[], options: HistoryFilterOptions = {}): Record[] {
  const kindSet = options.kinds && options.kinds.length > 0 ? new Set(options.kinds) : null;
  const needle = (options.query ?? '').trim().toLocaleLowerCase('ru');
  return records.filter((record) => {
    if (kindSet && !kindSet.has(record.kind)) return false;
    if (needle && !record.title.toLocaleLowerCase('ru').includes(needle)) return false;
    return true;
  });
}
