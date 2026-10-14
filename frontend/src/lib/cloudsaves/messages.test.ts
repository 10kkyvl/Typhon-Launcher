import { describe, expect, it } from 'vitest';
import { cloudCodeText, cloudErrorText, cloudStateKind, cloudStateLabel } from './messages';

describe('cloudErrorText', () => {
  it('maps a cloud code to its message', () => {
    expect(cloudErrorText(new Error('typhon:cloud.game_running: игра запущена'))).toBe(
      'Игра запущена, синхронизация подождёт',
    );
  });

  it('uses the fallback for an unknown cloud code', () => {
    expect(cloudErrorText(new Error('typhon:cloud.brand_new: ?'), 'запасной')).toBe('запасной');
  });

  it('uses the fallback when the error has no code', () => {
    expect(cloudErrorText(new Error('boom'), 'запасной')).toBe('запасной');
  });

  it('ignores codes outside the cloud namespace', () => {
    expect(cloudCodeText('common.cancel', 'запасной')).toBe('запасной');
  });

  it('says why a game without a catalog id has no cloud copy', () => {
    expect(cloudCodeText('cloud.no_canonical_id', 'x')).toContain('не опознана');
  });
});

describe('cloudStateLabel', () => {
  it('names every state', () => {
    expect(cloudStateLabel('synced')).toBe('Синхронизировано');
    expect(cloudStateLabel('pending')).toBe('Ожидает синхронизации');
    expect(cloudStateLabel('conflict')).toBe('Конфликт');
    expect(cloudStateLabel('failed')).toBe('Ошибка');
    expect(cloudStateLabel('skipped')).toBe('Пропущено');
  });
});

describe('cloudStateKind', () => {
  it('marks conflicts as warnings and failures as danger', () => {
    expect(cloudStateKind('conflict')).toBe('warning');
    expect(cloudStateKind('failed')).toBe('danger');
    expect(cloudStateKind('synced')).toBe('success');
  });
});
