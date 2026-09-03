import { describe, expect, it } from 'vitest';
import { commonLine, friendRequestNotification, isFriendCode, relationLabel } from './view';

describe('relationLabel', () => {
  it('даёт подпись действия для каждого отношения', () => {
    expect(relationLabel('none')).toBe('Добавить в друзья');
    expect(relationLabel('outgoing')).toBe('Заявка отправлена');
    expect(relationLabel('incoming')).toBe('Принять');
    expect(relationLabel('friend')).toBe('В друзьях');
    expect(relationLabel('self')).toBe('Это вы');
    expect(relationLabel('blocked')).toBe('Недоступен');
  });

  it('неизвестное отношение не выдаёт себя за «можно добавить»', () => {
    expect(relationLabel('')).toBe('Недоступен');
    expect(relationLabel('whatever')).toBe('Недоступен');
  });
});

describe('friendRequestNotification', () => {
  it('без заявок уведомления нет', () => {
    expect(friendRequestNotification(0)).toBeNull();
    expect(friendRequestNotification(-1)).toBeNull();
  });

  it('одна заявка — текст без числа', () => {
    const item = friendRequestNotification(1);
    expect(item).toMatchObject({
      id: 'friends:incoming:1',
      title: 'Друзья',
      text: 'Заявка в друзья',
      route: 'friends',
      terminal: false,
    });
    expect(item?.refId).toBeUndefined();
  });

  it('несколько заявок — число и склонение', () => {
    expect(friendRequestNotification(3)?.text).toBe('3 заявки в друзья');
    expect(friendRequestNotification(5)?.text).toBe('5 заявок в друзья');
    expect(friendRequestNotification(21)?.text).toBe('21 заявка в друзья');
  });

  it('id меняется вместе с числом, чтобы новая заявка не считалась прочитанной', () => {
    expect(friendRequestNotification(1)?.id).not.toBe(friendRequestNotification(2)?.id);
  });
});

describe('isFriendCode', () => {
  it('узнаёт код в любом регистре и с любым набором дефисов', () => {
    expect(isFriendCode('TY-A2B3-C4D5')).toBe(true);
    expect(isFriendCode('ty-a2b3-c4d5')).toBe(true);
    expect(isFriendCode('TYA2B3C4D5')).toBe(true);
    expect(isFriendCode('A2B3-C4D5')).toBe(true);
    expect(isFriendCode('A2B3C4D5')).toBe(true);
    expect(isFriendCode('  TY-A2B3-C4D5  ')).toBe(true);
  });

  it('имя пользователя кодом не считается', () => {
    expect(isFriendCode('egor')).toBe(false);
    expect(isFriendCode('@egor')).toBe(false);
    expect(isFriendCode('')).toBe(false);
    expect(isFriendCode('A2B3-C4D')).toBe(false);
    expect(isFriendCode('A2B3-C4D5E')).toBe(false);
    expect(isFriendCode('A2B3_C4D5')).toBe(false);
  });
});

describe('commonLine', () => {
  it('обе части — через разделитель', () => {
    expect(commonLine(3, 17)).toBe('3 общих друга · 17 общих игр');
  });

  it('нулевая часть пропускается', () => {
    expect(commonLine(0, 17)).toBe('17 общих игр');
    expect(commonLine(3, 0)).toBe('3 общих друга');
  });

  it('оба нуля — пустая строка', () => {
    expect(commonLine(0, 0)).toBe('');
    expect(commonLine(-1, -2)).toBe('');
  });

  it('склоняет и прилагательное, и существительное', () => {
    expect(commonLine(1, 1)).toBe('1 общий друг · 1 общая игра');
    expect(commonLine(2, 2)).toBe('2 общих друга · 2 общие игры');
    expect(commonLine(5, 5)).toBe('5 общих друзей · 5 общих игр');
    expect(commonLine(21, 21)).toBe('21 общий друг · 21 общая игра');
  });
});
