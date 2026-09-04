import { describe, expect, it } from 'vitest';
import {
  commonGameLabel,
  commonGamesTitle,
  commonLine,
  friendRequestNotification,
  isFriendCode,
  joinDate,
  memberSince,
  mutualMore,
  relationLabel,
  sentAt,
  showcaseTitle,
} from './view';

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
    expect(isFriendCode('TY-84K2-9WFC')).toBe(true);
    expect(isFriendCode('ty-84k2-9wfc')).toBe(true);
    expect(isFriendCode('TY84K29WFC')).toBe(true);
    expect(isFriendCode('ty84k29wfc')).toBe(true);
    expect(isFriendCode('84K2-9WFC')).toBe(true);
    expect(isFriendCode('84k2-9wfc')).toBe(true);
    expect(isFriendCode('  TY-84K2-9WFC  ')).toBe(true);
  });

  it('собачка всегда означает имя пользователя', () => {
    expect(isFriendCode('@maxpayne')).toBe(false);
    expect(isFriendCode('@84k2-9wfc')).toBe(false);
    expect(isFriendCode('@TY-84K2-9WFC')).toBe(false);
  });

  it('имя пользователя из восьми букв кодом не считается', () => {
    expect(isFriendCode('maxpayne')).toBe(false);
    expect(isFriendCode('egorripa')).toBe(false);
    expect(isFriendCode('egor')).toBe(false);
    expect(isFriendCode('')).toBe(false);
  });

  it('символы вне алфавита кода отбраковываются', () => {
    expect(isFriendCode('TY-84K2-91FC')).toBe(false);
    expect(isFriendCode('TY-84K2-9OFC')).toBe(false);
    expect(isFriendCode('TY-84I2-9WFC')).toBe(false);
    expect(isFriendCode('84K2_9WFC')).toBe(false);
  });

  it('длина должна совпадать с форматом', () => {
    expect(isFriendCode('84K2-9WF')).toBe(false);
    expect(isFriendCode('84K2-9WFCD')).toBe(false);
    expect(isFriendCode('TY84K29WF')).toBe(false);
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

describe('commonGamesTitle', () => {
  it('склоняет число игр', () => {
    expect(commonGamesTitle(1)).toBe('1 общая игра');
    expect(commonGamesTitle(3)).toBe('3 общие игры');
    expect(commonGamesTitle(17)).toBe('17 общих игр');
    expect(commonGamesTitle(21)).toBe('21 общая игра');
  });
});

describe('commonGameLabel', () => {
  it('игра есть у обоих', () => {
    expect(commonGameLabel(true, true, 'Егор')).toBe('установлена у обоих');
  });

  it('нет у смотрящего', () => {
    expect(commonGameLabel(false, true, 'Егор')).toBe('нужно установить вам');
  });

  it('нет у владельца профиля — имя отделено двоеточием', () => {
    expect(commonGameLabel(true, false, 'Егор')).toBe('нужно установить: Егор');
  });

  it('без имени подпись всё равно осмысленная', () => {
    expect(commonGameLabel(true, false, '   ')).toBe('нужно установить ему');
  });

  it('нет ни у кого — не выдаём это за «установлена»', () => {
    expect(commonGameLabel(false, false, 'Егор')).toBe('нужно установить обоим');
  });
});

describe('joinDate', () => {
  it('форматирует дату регистрации', () => {
    expect(joinDate('2026-09-03T10:00:00Z')).toBe('3 сентября 2026 г.');
  });

  it('пустая и битая дата не превращаются в мусор на экране', () => {
    expect(joinDate('')).toBe('');
    expect(joinDate('не дата')).toBe('');
  });
});

describe('memberSince', () => {
  it('дописывает подпись к дате', () => {
    expect(memberSince('2026-09-03T10:00:00Z')).toBe('Участник с 3 сентября 2026 г.');
  });

  it('без даты подписи тоже нет', () => {
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

describe('showcaseTitle', () => {
  it('берёт заголовки из таблицы витрины своего профиля', () => {
    expect(showcaseTitle('favorites')).toBe('Любимые');
    expect(showcaseTitle('recently_completed')).toBe('Недавно пройденные');
    expect(showcaseTitle('most_played')).toBe('Больше всего сыграно');
  });

  it('неизвестный вид показывает как есть, а не пустотой', () => {
    expect(showcaseTitle('whatever')).toBe('whatever');
  });
});

describe('sentAt', () => {
  const today = new Date().toISOString();

  it('дата в подписи со строчной буквы', () => {
    expect(sentAt(today)).toBe('Отправлена сегодня');
  });

  it('без даты остаётся прочерк, а не «Отправлена —»', () => {
    expect(sentAt(null)).toBe('—');
    expect(sentAt('')).toBe('—');
    expect(sentAt('не дата')).toBe('—');
  });
});
