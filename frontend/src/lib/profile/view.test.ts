import { describe, expect, it } from 'vitest';
import { coverOf, dayLabel, monthLine, recentLabel, shortDate, statusLine, visibilityLabel } from './view';

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

describe('recentLabel', () => {
  it('appends the window', () => {
    expect(recentLabel(48 * 3600)).toBe('48 ч за 2 недели');
    expect(recentLabel(7 * 3600 + 5 * 60)).toBe('7 ч 5 мин за 2 недели');
  });
});

describe('statusLine', () => {
  it('prefers the running game', () => {
    expect(
      statusLine([{ id: 'x', title: 'WoW', cover: '', playtimeSeconds: 0, status: '' }], true)
    ).toEqual({
      kind: 'playing',
      text: 'Играет: WoW',
    });
  });
  it('falls back to online state', () => {
    expect(statusLine([], true)).toEqual({ kind: 'online', text: 'В сети' });
    expect(statusLine([], false)).toEqual({ kind: 'offline', text: 'Не в сети' });
  });
});

describe('monthLine', () => {
  it('is empty when nothing happened this month', () => {
    expect(monthLine({ games: 0, hours: 0, completed: 0, playing: 0, monthSeconds: 0, monthGames: 0, monthCompleted: 0 })).toBe('');
  });
  it('lists hours, games and completions', () => {
    expect(
      monthLine({ games: 0, hours: 0, completed: 0, playing: 0, monthSeconds: 42 * 3600, monthGames: 6, monthCompleted: 2 })
    ).toBe('42 ч · 6 игр · 2 пройдено');
  });
  it('omits the hours part when there are no seconds', () => {
    expect(
      monthLine({ games: 0, hours: 0, completed: 0, playing: 0, monthSeconds: 0, monthGames: 1, monthCompleted: 1 })
    ).toBe('1 игра · 1 пройдена');
  });
  it('omits the games part when no game was played this month', () => {
    expect(
      monthLine({ games: 0, hours: 0, completed: 0, playing: 0, monthSeconds: 0, monthGames: 0, monthCompleted: 2 })
    ).toBe('2 пройдено');
  });
  it('keeps a zero completion count', () => {
    expect(
      monthLine({
        games: 0,
        hours: 0,
        completed: 0,
        playing: 0,
        monthSeconds: 3600 * 7 + 300,
        monthGames: 2,
        monthCompleted: 0,
      })
    ).toBe('7 ч 5 мин · 2 игры · 0 пройдено');
  });
});

describe('shortDate', () => {
  it('prints a day and short month', () => {
    expect(shortDate('2026-08-22T10:00:00Z')).toBe('22 авг.');
  });
});

describe('visibilityLabel', () => {
  it('names each level', () => {
    expect(visibilityLabel('public')).toBe('Все');
    expect(visibilityLabel('friends')).toBe('Друзья');
    expect(visibilityLabel('private')).toBe('Никто');
  });
  it('falls back to friends for anything unknown', () => {
    expect(visibilityLabel('')).toBe('Друзья');
    expect(visibilityLabel('everyone')).toBe('Друзья');
  });
});

describe('coverOf', () => {
  const game = { id: 'local', title: '007 First Light', canonicalGameId: 'bcc', cover: '/media/games/bcc/stale.jpg', playtimeSeconds: 0, status: '' };

  it('prefers the catalog art over the snapshot stored in the library', () => {
    expect(coverOf(game, { bcc: { cover: '/media/games/bcc/fresh.jpg', hero: '' } })).toBe('/media/games/bcc/fresh.jpg');
  });

  it('falls back to the stored cover when the catalog has no art', () => {
    expect(coverOf(game, {})).toBe('/media/games/bcc/stale.jpg');
  });

  it('survives a game with no canonical id', () => {
    expect(coverOf({ ...game, canonicalGameId: undefined }, {})).toBe('/media/games/bcc/stale.jpg');
  });
});
