import { describe, expect, it } from 'vitest';
import { friendRequestNotification, relationLabel } from './view';

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
