import { afterEach, describe, expect, it } from 'vitest';
import { applyLanguage, msg } from '../i18n';
import type { Stats, Transfer } from '../services/lan';
import { rejectedSummary, sharedGameTitle, transferLabel } from './lanText';

afterEach(() => applyLanguage('ru'));

function transfer(patch: Partial<Transfer> = {}): Transfer {
  return {
    id: 't1',
    infoHash: 'hash',
    peerId: 'p1',
    gameId: 'g1',
    title: 'Игра',
    downloaded: 0,
    total: 0,
    status: 'receiving',
    startedAt: '',
    updatedAt: '',
    ...patch,
  };
}

function stats(patch: Partial<Stats> = {}): Stats {
  return {
    announcesSent: 0,
    announcesReceived: 0,
    rejected: null,
    peersKnown: 0,
    offersKnown: 0,
    sharesActive: 0,
    ...patch,
  };
}

describe('transferLabel', () => {
  it('показывает процент при получении', () => {
    expect(transferLabel(transfer({ status: 'receiving', downloaded: 50, total: 200 }))).toBe('Получение… 25%');
  });

  it('не делит на ноль, когда total ещё неизвестен', () => {
    const label = transferLabel(transfer({ status: 'receiving', downloaded: 0, total: 0 }));
    expect(label).toBe('Получение…');
    expect(label).not.toContain('NaN');
    expect(label).not.toContain('Infinity');
  });

  it('сообщает о завершении', () => {
    expect(transferLabel(transfer({ status: 'completed' }))).toBe('Получено');
  });

  it('показывает причину ошибки', () => {
    const failed = transfer({ status: 'failed', error: 'typhon:lan.executable_missing: lan: executable missing after transfer: /games/test/game.exe' });
    expect(transferLabel(failed)).toBe(`Не удалось: ${msg('errSources.lanExecutableMissing')}`);
    applyLanguage('en');
    expect(transferLabel(failed)).toBe(`Failed: ${msg('errSources.lanExecutableMissing')}`);
  });

  it('подставляет заглушку без текста ошибки', () => {
    expect(transferLabel(transfer({ status: 'failed', error: '' }))).toBe('Не удалось: неизвестная ошибка');
  });

  it('сообщает об отмене', () => {
    expect(transferLabel(transfer({ status: 'cancelled' }))).toBe('Отменено');
  });
});

describe('rejectedSummary', () => {
  it('пусто без rejected', () => {
    expect(rejectedSummary(stats({ rejected: null }))).toBe('');
  });

  it('пусто для пустого словаря', () => {
    expect(rejectedSummary(stats({ rejected: {} }))).toBe('');
  });

  it('сортирует причины по убыванию количества', () => {
    expect(
      rejectedSummary(
        stats({
          rejected: { rate_limited: 2, too_large: 9, bad_json: 5 },
        }),
      ),
    ).toBe('слишком большой пакет: 9 · повреждённые данные: 5 · слишком частые сообщения: 2');
  });

  it('не роняет рендер на неизвестном ключе', () => {
    expect(rejectedSummary(stats({ rejected: { some_future_reason: 3 } }))).toBe('some_future_reason: 3');
  });
});

describe('sharedGameTitle', () => {
  const games = [{ id: 'g1', title: 'Hades' }, { id: 'g2', title: '' }];

  it('takes the title from the library', () => {
    expect(sharedGameTitle('g1', games)).toBe('Hades');
  });

  it('names a game without a title or gone from the library generically', () => {
    expect(sharedGameTitle('g2', games)).toBe(msg('ui.game'));
    expect(sharedGameTitle('gone', games)).toBe(msg('ui.game'));
  });
});
