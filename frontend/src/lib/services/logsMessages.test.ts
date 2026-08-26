import { describe, expect, it } from 'vitest';
import { logsReason } from './logsMessages';

describe('logsReason', () => {
  it('translates known backend errors', () => {
    expect(logsReason(new Error('файлы журнала не найдены'))).toBe('Логов пока нет: лаунчер ещё ничего не записал.');
    expect(logsReason(new Error('downloads folder: The system cannot find the path'))).toBe(
      'Не удалось найти папку «Загрузки».',
    );
    expect(logsReason(new Error('create temp file in C:\Users: Access is denied.'))).toBe(
      'Нет прав на запись в папку «Загрузки».',
    );
  });

  it('falls back instead of showing go error text', () => {
    expect(logsReason(new Error('write entry typhon.log: some io failure'))).toBe('Не удалось сохранить логи.');
    expect(logsReason(undefined)).toBe('Не удалось сохранить логи.');
  });
});
