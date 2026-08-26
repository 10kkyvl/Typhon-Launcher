const REASONS: [string, string][] = [
  ['файлы журнала не найдены', 'Логов пока нет: лаунчер ещё ничего не записал.'],
  ['downloads folder', 'Не удалось найти папку «Загрузки».'],
  ['config dir', 'Не удалось найти папку с данными лаунчера.'],
  ['Access is denied', 'Нет прав на запись в папку «Загрузки».'],
  ['permission denied', 'Нет прав на запись в папку «Загрузки».'],
  ['not enough space', 'На диске не хватает места для архива с логами.'],
  ['no space left', 'На диске не хватает места для архива с логами.'],
];

export function logsReason(err: unknown): string {
  const raw = err instanceof Error ? err.message : String(err ?? '');
  const known = REASONS.find(([marker]) => raw.includes(marker));
  return known ? known[1] : 'Не удалось сохранить логи.';
}
