import { describe, expect, it } from 'vitest';
import { historyLabel } from './historyText';
import { Kind, type Record } from '../../../bindings/typhon/internal/history';

const record = (over: Partial<Record> = {}): Record => ({
  id: 'r1',
  kind: Kind.KindInstalled,
  at: '2026-01-01T00:00:00Z',
  title: 'Cyberpunk 2077',
  bytesKnown: false,
  ...over,
});

describe('historyLabel', () => {
  it('formats an install', () => {
    expect(historyLabel(record({ kind: Kind.KindInstalled }))).toEqual({
      title: 'Cyberpunk 2077 установлен',
      detail: '',
    });
  });

  it('formats an update with the version jump', () => {
    expect(
      historyLabel(
        record({ title: 'Doom Eternal', kind: Kind.KindUpdated, fromVersion: '1.2', toVersion: '1.3' }),
      ),
    ).toEqual({
      title: 'Doom Eternal обновлён',
      detail: '1.2 → 1.3',
    });
  });

  it('drops the version detail when either version is missing', () => {
    expect(historyLabel(record({ kind: Kind.KindUpdated, toVersion: '1.3' })).detail).toBe('');
  });

  it('formats a removal with a known size', () => {
    expect(
      historyLabel(record({ title: 'Hades', kind: Kind.KindRemoved, bytesKnown: true, bytes: 15032385536 })),
    ).toEqual({
      title: 'Hades удалён',
      detail: 'освобождено 14,0 ГБ',
    });
  });

  it('omits the size entirely when it is unknown, never showing 0 Б', () => {
    expect(historyLabel(record({ kind: Kind.KindRemoved, bytesKnown: false })).detail).toBe('');
  });

  it('shows a known zero size instead of hiding it', () => {
    expect(historyLabel(record({ kind: Kind.KindUninstalled, bytesKnown: true, bytes: 0 })).detail).toBe(
      'освобождено 0 Б',
    );
  });

  it('formats a failed install with the failure detail', () => {
    expect(historyLabel(record({ kind: Kind.KindInstallFailed, detail: 'нет места на диске' }))).toEqual({
      title: 'Cyberpunk 2077 не установился',
      detail: 'нет места на диске',
    });
  });

  it('formats a failed update with the failure detail', () => {
    expect(historyLabel(record({ kind: Kind.KindUpdateFailed, detail: 'обрыв соединения' }))).toEqual({
      title: 'Cyberpunk 2077 не обновился',
      detail: 'обрыв соединения',
    });
  });

  it('formats a rollback to the restored version', () => {
    expect(historyLabel(record({ kind: Kind.KindRolledBack, fromVersion: '1.3', toVersion: '1.2' }))).toEqual({
      title: 'Cyberpunk 2077 откачен к 1.2',
      detail: '',
    });
  });

  it('formats a download', () => {
    expect(historyLabel(record({ kind: Kind.KindDownloaded }))).toEqual({
      title: 'Cyberpunk 2077 загружен',
      detail: '',
    });
  });

  it('formats a move using the backend-provided detail', () => {
    expect(
      historyLabel(record({ kind: Kind.KindMoved, detail: 'C:\\Games → D:\\Games' })),
    ).toEqual({
      title: 'Cyberpunk 2077 перемещён',
      detail: 'C:\\Games → D:\\Games',
    });
  });

  it('formats a LAN receive', () => {
    expect(historyLabel(record({ kind: Kind.KindLanReceived }))).toEqual({
      title: 'Cyberpunk 2077 получен из локальной сети',
      detail: '',
    });
  });

  it('falls back to a generic label for an unknown kind instead of an empty string', () => {
    const label = historyLabel(record({ kind: '' as Kind }));
    expect(label.title).not.toBe('');
    expect(label.title).toContain('Cyberpunk 2077');
  });
});
