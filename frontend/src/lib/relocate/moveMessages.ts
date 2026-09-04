import { msg } from '../i18n';
import type { MessageKey } from '../i18n';

const REASONS: [string, MessageKey][] = [
  ['игра сейчас запущена', 'transfers.moveErrGameRunning'],
  ['для игры выполняется обновление', 'transfers.moveErrUpdating'],
  ['для игры выполняется установка', 'transfers.moveErrInstalling'],
  ['для игры идёт загрузка', 'transfers.moveErrDownloading'],
  ['перенос уже выполняется', 'transfers.moveErrAlreadyRunning'],
  ['целевой каталог не пуст', 'transfers.moveErrTargetNotEmpty'],
  ['целевой каталог не может быть внутри исходного', 'transfers.moveErrTargetInsideSource'],
  ['исходный каталог не может быть внутри целевого', 'transfers.moveErrSourceInsideTarget'],
  ['целевой каталог не может быть корнем диска', 'transfers.moveErrTargetIsDriveRoot'],
  ['целевой каталог должен быть абсолютным путём', 'transfers.moveErrInvalidPath'],
  ['не указан целевой каталог', 'transfers.moveErrNoTarget'],
  ['не указан исходный каталог', 'transfers.moveErrNoSource'],
  ['не удалось определить свободное место', 'transfers.moveErrFreeSpaceUnknown'],
  ['недостаточно свободного места', 'transfers.moveErrNotEnoughSpace'],
  ['проверка перенесённых файлов не прошла', 'transfers.moveErrVerifyFailed'],
  ['у игры не задан каталог установки', 'transfers.moveErrNoInstallDir'],
  ['операция переноса не найдена', 'transfers.moveErrJobNotFound'],
  ['игра не найдена', 'transfers.moveErrGameNotFound'],
  ['восстановление переноса неоднозначно', 'transfers.moveErrAmbiguousRecovery'],
  ['диалог недоступен', 'transfers.moveErrDialogUnavailable'],
  ['сервис завершает работу', 'transfers.moveErrShuttingDown'],
  ['сервис ещё не запущен', 'transfers.moveErrNotReady'],
];

export function moveErrorText(err: unknown, fallback: string = msg('transfers.moveErrFallback')): string {
  const raw = err instanceof Error ? err.message : String(err ?? '');
  if (!raw) return fallback;
  const known = REASONS.find(([marker]) => raw.includes(marker));
  return known ? msg(known[1]) : fallback;
}
