import { get } from 'svelte/store';
import type { Kind, Record } from '../../../bindings/typhon/internal/history';
import { locale as activeLocale } from '../i18n';

export interface HistoryFilterOptions {
  kinds?: Kind[];
  query?: string;
  /** The language whose case rules compare the query with the titles; the interface's when not given. */
  locale?: string;
}

export function filterHistory(records: Record[], options: HistoryFilterOptions = {}): Record[] {
  const kindSet = options.kinds && options.kinds.length > 0 ? new Set(options.kinds) : null;
  const locale = options.locale ?? get(activeLocale);
  const needle = (options.query ?? '').trim().toLocaleLowerCase(locale);
  return records.filter((record) => {
    if (kindSet && !kindSet.has(record.kind)) return false;
    if (needle && !record.title.toLocaleLowerCase(locale).includes(needle)) return false;
    return true;
  });
}
