import { describe, expect, it } from 'vitest';
import type { FriendView, PresenceView } from '../services/social';
import { dotKind, ownStatusLine, presenceDot, presenceLine, sinceLabel, sortFriends, statusDot } from './presence';

function presence(patch: Partial<PresenceView> = {}): PresenceView {
  return { status: 'online', ...patch };
}

describe('statusDot', () => {
  it('переводит статус в цвет точки', () => {
    expect(statusDot('online')).toBe('online');
    expect(statusDot('away')).toBe('away');
    expect(statusDot('busy')).toBe('busy');
    expect(statusDot('offline')).toBe('offline');
  });

  it('прячет невидимку и незнакомые статусы за «не в сети»', () => {
    expect(statusDot('invisible')).toBe('offline');
    expect(statusDot('')).toBe('offline');
    expect(statusDot('lunch')).toBe('offline');
  });
});

describe('presenceDot', () => {
  it('берёт статус присутствия', () => {
    expect(presenceDot(presence({ status: 'away' }))).toBe('away');
    expect(presenceDot(presence({ status: 'busy' }))).toBe('busy');
  });

  it('считает отсутствующее присутствие офлайном', () => {
    expect(presenceDot(undefined)).toBe('offline');
    expect(presenceDot(null)).toBe('offline');
    expect(presenceDot(presence({ status: 'offline' }))).toBe('offline');
  });
});

describe('dotKind', () => {
  it('сопоставляет точку с видом бейджа', () => {
    expect(dotKind('online')).toBe('success');
    expect(dotKind('away')).toBe('warning');
    expect(dotKind('busy')).toBe('danger');
    expect(dotKind('offline')).toBe('neutral');
  });
});

describe('presenceLine', () => {
  const now = new Date('2025-03-10T12:00:00Z');

  it('показывает игру для любого статуса, кроме офлайна', () => {
    expect(presenceLine(presence({ gameId: 1942, gameTitle: 'The Witcher 3' }), now)).toBe('Играет: The Witcher 3');
    expect(presenceLine(presence({ status: 'busy', gameId: 1942, gameTitle: 'The Witcher 3' }), now)).toBe(
      'Играет: The Witcher 3',
    );
  });

  it('игнорирует игру у офлайн-присутствия', () => {
    expect(presenceLine(presence({ status: 'offline', gameId: 1942, gameTitle: 'The Witcher 3' }), now)).toBe(
      'Не в сети',
    );
  });

  it('подписывает статусы без игры', () => {
    expect(presenceLine(presence(), now)).toBe('В сети');
    expect(presenceLine(presence({ status: 'away' }), now)).toBe('Отошёл');
    expect(presenceLine(presence({ status: 'busy' }), now)).toBe('Не беспокоить');
  });

  it('добавляет к офлайну время последнего визита', () => {
    expect(presenceLine(presence({ status: 'offline', lastSeenAt: '2025-03-10T10:00:00Z' }), now)).toBe(
      'Не в сети · 2 ч назад',
    );
  });

  it('обходится без времени последнего визита', () => {
    expect(presenceLine(presence({ status: 'offline' }), now)).toBe('Не в сети');
    expect(presenceLine(undefined, now)).toBe('Не в сети');
    expect(presenceLine(null, now)).toBe('Не в сети');
  });

  it('показывает невидимку другим как офлайн, без последнего визита', () => {
    expect(presenceLine(presence({ status: 'invisible', gameTitle: 'The Witcher 3' }), now)).toBe('Не в сети');
    expect(
      presenceLine(presence({ status: 'invisible', lastSeenAt: '2025-03-10T10:00:00Z' }), now),
    ).toBe('Не в сети');
  });

  it('называет невидимку по имени в своём профиле', () => {
    expect(
      presenceLine(presence({ status: 'invisible', lastSeenAt: '2025-03-10T10:00:00Z' }), now, true),
    ).toBe('Невидимка');
    expect(presenceLine(presence({ status: 'invisible', gameTitle: 'The Witcher 3' }), now, true)).toBe(
      'Невидимка',
    );
  });
});

describe('sinceLabel', () => {
  const now = new Date('2025-03-10T12:00:00Z');

  it('называет свежие отметки', () => {
    expect(sinceLabel('2025-03-10T11:59:40Z', now)).toBe('только что');
    expect(sinceLabel('2025-03-10T12:00:30Z', now)).toBe('только что');
  });

  it('считает минуты и часы', () => {
    expect(sinceLabel('2025-03-10T11:55:00Z', now)).toBe('5 мин назад');
    expect(sinceLabel('2025-03-10T11:01:00Z', now)).toBe('59 мин назад');
    expect(sinceLabel('2025-03-10T10:00:00Z', now)).toBe('2 ч назад');
  });

  it('считает дни', () => {
    expect(sinceLabel('2025-03-09T12:00:00Z', now)).toBe('вчера');
    expect(sinceLabel('2025-03-07T12:00:00Z', now)).toBe('3 дн. назад');
  });

  it('переходит на дату после недели', () => {
    expect(sinceLabel('2025-02-01T12:00:00Z', now)).toBe(
      new Date('2025-02-01T12:00:00Z').toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' }),
    );
  });

  it('не спотыкается о мусор', () => {
    expect(sinceLabel('', now)).toBe('');
    expect(sinceLabel('вчера вечером', now)).toBe('');
  });
});

describe('sortFriends', () => {
  function friend(id: string, displayName: string, presenceView?: PresenceView | null): FriendView {
    return {
      id,
      username: id,
      displayName,
      avatarUrl: '',
      since: '2025-01-01T00:00:00Z',
      presence: presenceView ?? null,
    };
  }

  it('поднимает играющих, затем сеть, отошёл, занят и офлайн', () => {
    const list = [
      friend('e', 'Ева', { status: 'offline' }),
      friend('d', 'Дима', { status: 'busy' }),
      friend('c', 'Соня', { status: 'away' }),
      friend('b', 'Боря', { status: 'online' }),
      friend('a', 'Аня', { status: 'away', gameId: 1942, gameTitle: 'The Witcher 3' }),
    ];
    expect(sortFriends(list).map((f) => f.id)).toEqual(['a', 'b', 'c', 'd', 'e']);
  });

  it('сортирует офлайн по последнему визиту, потом по имени', () => {
    const list = [
      friend('old', 'Яна', { status: 'offline', lastSeenAt: '2025-03-01T00:00:00Z' }),
      friend('never', 'Боря', null),
      friend('fresh', 'Юля', { status: 'offline', lastSeenAt: '2025-03-09T00:00:00Z' }),
      friend('nameless', 'Аня', null),
    ];
    expect(sortFriends(list).map((f) => f.id)).toEqual(['fresh', 'old', 'nameless', 'never']);
  });

  it('сортирует равных по имени и не трогает исходный список', () => {
    const list = [
      friend('b', 'Борис', { status: 'online' }),
      friend('a', 'Алла', { status: 'online' }),
    ];
    expect(sortFriends(list).map((f) => f.id)).toEqual(['a', 'b']);
    expect(list.map((f) => f.id)).toEqual(['b', 'a']);
  });

  it('берёт имя пользователя, когда отображаемого нет', () => {
    const list = [friend('zeta', '', { status: 'online' }), friend('alpha', '', { status: 'online' })];
    expect(sortFriends(list).map((f) => f.id)).toEqual(['alpha', 'zeta']);
  });
});

describe('ownStatusLine', () => {
  it('подписывает собственный статус', () => {
    expect(ownStatusLine('online', false)).toBe('В сети');
    expect(ownStatusLine('away', false)).toBe('Отошёл');
    expect(ownStatusLine('busy', false)).toBe('Не беспокоить');
    expect(ownStatusLine('invisible', false)).toBe('Невидимка');
    expect(ownStatusLine('', false)).toBe('В сети');
  });

  it('молчит, когда игра запущена', () => {
    expect(ownStatusLine('away', true)).toBe('');
    expect(ownStatusLine('online', true)).toBe('');
  });

  it('оставляет невидимку поверх запущенной игры', () => {
    expect(ownStatusLine('invisible', true)).toBe('Невидимка');
  });
});

describe('playing without a title', () => {
  it('still reads as playing and sorts first', () => {
    const untitled = { status: 'online', gameId: 1942 } as const;
    expect(presenceLine(untitled)).toBe('Играет');
    const list = [
      { id: 'b', username: 'b', displayName: 'B', since: '', presence: { status: 'online' } },
      { id: 'a', username: 'a', displayName: 'A', since: '', presence: untitled },
    ] as never[];
    expect(sortFriends(list).map((f: { id: string }) => f.id)).toEqual(['a', 'b']);
  });
});
