export interface LiveEntry {
  refId?: string;
  terminal: boolean;
}

export interface HistoryEntry {
  refId?: string;
}

export function mergeNotifications<T extends LiveEntry>(live: T[], history: HistoryEntry[]): T[] {
  const recorded = new Set(history.map((item) => item.refId).filter((id): id is string => Boolean(id)));
  return live.filter((item) => !(item.terminal && item.refId && recorded.has(item.refId)));
}
