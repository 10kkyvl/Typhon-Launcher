import { describe, expect, it } from 'vitest';
import type { Offer, Stats, Transfer } from '../services/lan';
import { offerLabel, rejectedSummary, transferLabel } from './lanText';

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

function offer(patch: Partial<Offer> = {}): Offer {
  return {
    peerId: 'p1',
    host: 'DESKTOP-ABC',
    addr: '192.168.1.5',
    port: 41234,
    gameId: 'g1',
    title: 'Игра',
    version: '',
    exe: 'game.exe',
    sizeBytes: 0,
    infoHash: 'hash',
    lastSeen: '',
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
    expect(transferLabel(transfer({ status: 'failed', error: 'нет места на диске' }))).toBe(
      'Не удалось: нет места на диске',
    );
  });

  it('подставляет заглушку без текста ошибки', () => {
    expect(transferLabel(transfer({ status: 'failed', error: '' }))).toBe('Не удалось: неизвестная ошибка');
  });

  it('сообщает об отмене', () => {
    expect(transferLabel(transfer({ status: 'cancelled' }))).toBe('Отменено');
  });
});

describe('offerLabel', () => {
  it('включает версию, когда она известна', () => {
    expect(offerLabel(offer({ title: 'Portal 2', version: '1.0', sizeBytes: 1024 * 1024 * 1024, host: 'PC-1' }))).toBe(
      'Portal 2 · 1.0 · 1,0 ГБ · с ПК PC-1',
    );
  });

  it('пропускает сегмент версии, когда она пустая', () => {
    expect(offerLabel(offer({ title: 'Portal 2', version: '', sizeBytes: 1024 * 1024 * 1024, host: 'PC-1' }))).toBe(
      'Portal 2 · 1,0 ГБ · с ПК PC-1',
    );
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
