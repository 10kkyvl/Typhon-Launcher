import { describe, expect, it } from 'vitest';
import type { BackupEvent } from '../services/savebackup';
import {
  backupNotice,
  saveBackupCodeText,
  saveBackupErrorText,
  snapshotKindLabel,
  snapshotPlayedAfter,
  snapshotWhen,
} from './messages';

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
    expect(snapshotKindLabel('launch')).toBe('Перед запуском');
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

describe('snapshotPlayedAfter', () => {
  it('shows nothing when the playtime is unknown', () => {
    expect(snapshotPlayedAfter({ playedAfterKnown: false, playedAfterSeconds: 5000 })).toBe('');
    expect(snapshotPlayedAfter({})).toBe('');
  });

  it('formats the known playtime', () => {
    expect(snapshotPlayedAfter({ playedAfterKnown: true, playedAfterSeconds: 4800 })).toBe('Играл после: 1 ч 20 мин');
  });

  it('says so when the game was not played after the snapshot', () => {
    expect(snapshotPlayedAfter({ playedAfterKnown: true, playedAfterSeconds: 0 })).toBe('После этого не играл');
  });
});

describe('backupNotice', () => {
  const event = (patch: Partial<BackupEvent>): BackupEvent => ({
    gameId: 'g1',
    kind: 'launch',
    snapshot: null,
    status: 'created',
    code: '',
    error: '',
    ...patch,
  });

  it('is silent for a launch backup that was created or skipped', () => {
    expect(backupNotice(event({ status: 'created' }), 'Hades')).toBeNull();
    expect(backupNotice(event({ status: 'skipped', code: 'savebackup.saves_ambiguous' }), 'Hades')).toBeNull();
  });

  it('warns that the game started without a copy when the launch backup timed out', () => {
    const notice = backupNotice(event({ status: 'failed', code: 'savebackup.launch_timeout' }), 'Hades');
    expect(notice?.kind).toBe('danger');
    expect(notice?.text).toContain('Hades');
    expect(notice?.text).toContain('запущена без копии');
    expect(notice?.text).toContain('не успела');
  });

  it('uses the generic reason for an unknown launch failure code', () => {
    const notice = backupNotice(event({ status: 'failed', code: 'x.unknown' }), 'Hades');
    expect(notice?.text).toContain('запущена без копии');
    expect(notice?.text).not.toContain('x.unknown');
  });

  it('keeps the session wording for a failed session backup', () => {
    const notice = backupNotice(event({ kind: 'session', status: 'failed', code: 'savebackup.no_free_space' }), 'Hades');
    expect(notice?.text).toContain('Не удалось создать копию');
  });

  it('stays quiet for manual backups', () => {
    expect(backupNotice(event({ kind: 'manual', status: 'failed' }), 'Hades')).toBeNull();
  });
});
