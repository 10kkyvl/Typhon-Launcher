import { describe, expect, it } from 'vitest';
import { saveBackupCodeText, saveBackupErrorText, snapshotKindLabel, snapshotWhen } from './messages';

describe('saveBackupErrorText', () => {
  it('maps a savebackup code to its message', () => {
    expect(saveBackupErrorText(new Error('typhon:savebackup.game_running: игра запущена'))).toBe(
      'Закройте игру перед восстановлением',
    );
  });

  it('maps a library code raised while locating saves', () => {
    expect(saveBackupErrorText(new Error('typhon:library.saves_path_unavailable: нет доступа'), 'x')).toBe(
      'папка сохранений недоступна',
    );
  });

  it('uses the fallback for an unknown code', () => {
    expect(saveBackupErrorText(new Error('typhon:savebackup.brand_new: ?'), 'запасной')).toBe('запасной');
  });

  it('uses the fallback when the error has no code', () => {
    expect(saveBackupErrorText(new Error('boom'), 'запасной')).toBe('запасной');
  });

  it('ignores codes outside the backup and library namespaces', () => {
    expect(saveBackupCodeText('common.cancel', 'запасной')).toBe('запасной');
  });
});

describe('snapshotKindLabel', () => {
  it('names every kind', () => {
    expect(snapshotKindLabel('manual')).toBe('Вручную');
    expect(snapshotKindLabel('session')).toBe('После игры');
    expect(snapshotKindLabel('update')).toBe('Перед обновлением');
    expect(snapshotKindLabel('pre-restore')).toBe('Перед восстановлением');
  });
});

describe('snapshotWhen', () => {
  it('falls back to the id when the date cannot be parsed', () => {
    expect(snapshotWhen({ id: 'abc', createdAt: '' })).toBe('abc');
  });

  it('formats a valid date', () => {
    expect(snapshotWhen({ id: 'abc', createdAt: '2026-09-29T10:00:00Z' })).not.toBe('abc');
  });
});
