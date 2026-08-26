import type { SelfUpdateOutcome, SelfUpdateStatus } from './selfupdate';

const REASONS: [string, string][] = [
  [
    'left the launcher binary unchanged',
    'Установщик не заменил файлы запущенного лаунчера. Откройте Typhon из меню Пуск или переустановите вручную.',
  ],
  ['launcher did not exit', 'Лаунчер не закрылся вовремя, обновление отменено.'],
  ['downloaded hash differs', 'Загруженный установщик повреждён. Скачайте обновление заново.'],
  ['downloaded size differs', 'Загруженный установщик повреждён. Скачайте обновление заново.'],
  ['no verified update is ready', 'Установщик не найден. Скачайте обновление заново.'],
  ['run installer', 'Установщик завершился с ошибкой.'],
  ['artifact download stalled', 'Сервер обновлений перестал отдавать данные. Проверьте соединение и попробуйте ещё раз.'],
  ['Client.Timeout', 'Сервер обновлений не ответил вовремя. Проверьте соединение и попробуйте ещё раз.'],
  ['context deadline exceeded', 'Сервер обновлений не ответил вовремя. Проверьте соединение и попробуйте ещё раз.'],
  ['context canceled', 'Загрузка обновления прервана.'],
  ['no such host', 'Не удалось связаться с сервером обновлений. Проверьте интернет.'],
  ['connection refused', 'Не удалось связаться с сервером обновлений. Проверьте интернет.'],
  ['dial tcp', 'Не удалось связаться с сервером обновлений. Проверьте интернет.'],
  ['tls:', 'Не удалось установить защищённое соединение с сервером обновлений.'],
  ['artifact endpoint returned an error status', 'Сервер обновлений вернул ошибку. Попробуйте позже.'],
  ['manifest endpoint returned an error status', 'Сервер обновлений вернул ошибку. Попробуйте позже.'],
  ['signature does not verify', 'Подпись обновления не совпала. Обновление отклонено.'],
  ['unknown key', 'Подпись обновления не совпала. Обновление отклонено.'],
  ['manifest is malformed', 'Сервер обновлений вернул повреждённые данные. Попробуйте позже.'],
  ['manifest exceeds the size limit', 'Сервер обновлений вернул повреждённые данные. Попробуйте позже.'],
  ['manifest version is not comparable', 'Сервер обновлений вернул повреждённые данные. Попробуйте позже.'],
  ['manifest has no artifact for this platform', 'Для этой системы обновление не опубликовано.'],
  ['another update operation is in progress', 'Другая операция обновления уже идёт.'],
  ['check for an update before downloading', 'Сначала проверьте обновления.'],
  ['not enough space', 'На диске не хватает места для обновления.'],
  ['Access is denied', 'Нет прав на запись в папку обновлений.'],
  ['permission denied', 'Нет прав на запись в папку обновлений.'],
];

const CODE_REASONS: Record<string, string> = {
  manifest: 'Не удалось проверить обновления.',
  artifact: 'Не удалось проверить обновления.',
  version: 'Не удалось проверить обновления.',
  download: 'Не удалось загрузить обновление.',
  apply: 'Не удалось установить обновление.',
};

function translate(raw: string): string {
  const known = REASONS.find(([marker]) => raw.includes(marker));
  return known ? known[1] : '';
}

export function outcomeReason(outcome: SelfUpdateOutcome): string {
  const raw = outcome.error ?? '';
  return translate(raw) || raw;
}

export function updateReason(err: unknown): string {
  const raw = err instanceof Error ? err.message : String(err ?? '');
  return translate(raw) || raw;
}

export function statusReason(status: SelfUpdateStatus): string {
  const raw = status.error ?? '';
  if (!raw) return '';
  return translate(raw) || CODE_REASONS[status.errorCode ?? ''] || raw;
}
