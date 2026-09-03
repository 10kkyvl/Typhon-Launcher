import { describe, expect, it } from 'vitest';
import { commonGameLabel, commonGamesTitle, memberSince, mutualMore } from './profileView';

describe('commonGameLabel', () => {
  it('игра есть у обоих', () => {
    expect(commonGameLabel(true, true, 'Егор')).toBe('установлена у обоих');
  });

  it('нет у смотрящего', () => {
    expect(commonGameLabel(false, true, 'Егор')).toBe('нужно установить вам');
  });

  it('нет у владельца профиля — в подписи его имя', () => {
    expect(commonGameLabel(true, false, 'Егор')).toBe('нужно установить Егор');
  });

  it('без имени подпись всё равно осмысленная', () => {
    expect(commonGameLabel(true, false, '   ')).toBe('нужно установить ему');
  });

  it('нет ни у кого — не выдаём это за «установлена»', () => {
    expect(commonGameLabel(false, false, 'Егор')).toBe('нужно установить обоим');
  });
});

describe('commonGamesTitle', () => {
  it('склоняет число игр', () => {
    expect(commonGamesTitle(1)).toBe('1 общая игра');
    expect(commonGamesTitle(3)).toBe('3 общие игры');
    expect(commonGamesTitle(17)).toBe('17 общих игр');
    expect(commonGamesTitle(21)).toBe('21 общая игра');
  });
});

describe('memberSince', () => {
  it('форматирует дату регистрации', () => {
    expect(memberSince('2026-09-03T10:00:00Z')).toBe('Участник с 3 сентября 2026 г.');
  });

  it('пустая и битая дата не превращаются в мусор на экране', () => {
    expect(memberSince('')).toBe('');
    expect(memberSince('не дата')).toBe('');
  });
});

describe('mutualMore', () => {
  it('показывает остаток только когда он есть', () => {
    expect(mutualMore(6, 9)).toBe('+3');
    expect(mutualMore(6, 6)).toBe('');
    expect(mutualMore(6, 2)).toBe('');
  });
});
