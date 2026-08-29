const REASONS: [string, string][] = [
  ['not authenticated', 'Сессия аккаунта истекла — войдите заново, чтобы синхронизировать данные.'],
  ['settings revision conflict', 'Настройки изменились на другом устройстве — повторите ещё раз.'],
  ['too many games in request', 'Слишком много игр для одной синхронизации — повторите чуть позже.'],
  ['device limit reached', 'Достигнут лимит устройств для синхронизации этого аккаунта.'],
  ['sync already in progress', 'Синхронизация уже выполняется.'],
  ['service is not started', 'Служба синхронизации ещё не запущена — подождите и повторите.'],
  ['network error', 'Нет связи с сервером синхронизации — проверьте интернет.'],
  ['server error, status', 'Сервер синхронизации сейчас недоступен — попробуйте позже.'],
  ['request rejected', 'Сервер отклонил запрос синхронизации.'],
  ['unavailable in browser', 'Синхронизация доступна только в desktop-сборке.'],
];

function translate(raw: string): string {
  const known = REASONS.find(([marker]) => raw.includes(marker));
  return known ? known[1] : '';
}

export function accountSyncReason(err: unknown, fallback: string): string {
  const raw = err instanceof Error ? err.message : String(err ?? '');
  return translate(raw) || fallback;
}
