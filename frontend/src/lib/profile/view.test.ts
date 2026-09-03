import { describe, expect, it } from 'vitest';
import { dayLabel, hoursLabel, recentLabel, statusLine } from './view';

const now = new Date(2026, 8, 3, 15, 0, 0);

describe('dayLabel', () => {
  it('names today and yesterday', () => {
    expect(dayLabel('2026-09-03', now)).toBe('Сегодня');
    expect(dayLabel('2026-09-02', now)).toBe('Вчера');
  });
  it('prints a short date otherwise', () => {
    expect(dayLabel('2026-08-31', now)).toBe('31 авг.');
  });
  it('handles year boundaries', () => {
    expect(dayLabel('2025-12-31', new Date(2026, 0, 1, 0, 30))).toBe('Вчера');
  });
});

describe('hoursLabel', () => {
  it('floors to hours with a ru separator', () => {
    expect(hoursLabel(1284 * 3600 + 1799)).toBe((1284).toLocaleString('ru-RU'));
    expect(hoursLabel(0)).toBe('0');
  });
});

describe('recentLabel', () => {
  it('appends the window', () => {
    expect(recentLabel(48 * 3600)).toBe('48 ч за 2 недели');
    expect(recentLabel(7 * 3600 + 5 * 60)).toBe('7 ч 5 мин за 2 недели');
  });
});

describe('statusLine', () => {
  it('prefers the running game', () => {
    expect(statusLine([{ id: 'x', title: 'WoW', cover: '', playtimeSeconds: 0 }], true)).toEqual({
      kind: 'playing',
      text: 'Играет: WoW',
    });
  });
  it('falls back to online state', () => {
    expect(statusLine([], true)).toEqual({ kind: 'online', text: 'В сети' });
    expect(statusLine([], false)).toEqual({ kind: 'offline', text: 'Не в сети' });
  });
});
