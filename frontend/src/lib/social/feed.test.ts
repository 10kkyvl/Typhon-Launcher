import { describe, expect, it } from 'vitest';
import type { FeedEvent } from '../services/social';
import { REACTIONS, eventLabel, eventLine, feedDayGroups, kindLabel, reactionLabel, toggleReaction } from './feed';

function event(patch: Partial<FeedEvent> = {}): FeedEvent {
  return {
    id: 1,
    user: { id: 'u1', username: 'alex', displayName: 'Alex', avatarUrl: '' },
    kind: 'completed',
    game: { igdbId: 7, title: 'Elden Ring', coverUrl: '' },
    createdAt: '2026-09-03T12:00:00Z',
    seconds: 0,
    achievements: [],
    review: null,
    reactions: [],
    mine: [],
    note: '',
    ...patch,
  };
}

function iso(daysAgo: number, hour = 12): string {
  const now = new Date();
  const date = new Date(now.getFullYear(), now.getMonth(), now.getDate() - daysAgo, hour, 0, 0);
  return date.toISOString();
}

describe('kindLabel', () => {
  it('переводит вид события без глагола', () => {
    expect(kindLabel('completed')).toBe('Пройдена');
    expect(kindLabel('started')).toBe('Новая игра');
    expect(kindLabel('favorited')).toBe('В любимых');
  });

  it('не выдумывает подпись для незнакомого вида', () => {
    expect(kindLabel('exploded')).toBe('');
    expect(kindLabel('')).toBe('');
  });
});

describe('eventLabel', () => {
  it('names the static kinds', () => {
    expect(eventLabel(event({ kind: 'reviewed' }))).toBe('Написан отзыв');
    expect(eventLabel(event({ kind: 'planned' }))).toBe('В планах');
  });

  it('shows whole hours for played and nothing when there are none', () => {
    expect(eventLabel(event({ kind: 'played', seconds: 3600 * 5 + 1200 }))).toBe('Наиграно 5 ч за неделю');
    expect(eventLabel(event({ kind: 'played', seconds: 0 }))).toBe('');
  });

  it('names a single achievement and counts several with a plural', () => {
    const one = { key: 'A', name: 'First win', iconUrl: '' };
    expect(eventLabel(event({ kind: 'achievement', achievements: [one] }))).toBe('Получено достижение «First win»');
    expect(eventLabel(event({ kind: 'achievement', achievements: [one, one] }))).toBe('Получено 2 достижения');
    expect(eventLabel(event({ kind: 'achievement', achievements: Array(5).fill(one) }))).toBe('Получено 5 достижений');
    expect(eventLabel(event({ kind: 'achievement', achievements: [] }))).toBe('');
  });

  it('gives nothing for an unknown kind', () => {
    expect(eventLabel(event({ kind: 'exploded' }))).toBe('');
  });
});

describe('eventLine', () => {
  it('собирает строку из подписи и названия', () => {
    expect(eventLine(event())).toBe('Пройдена: Elden Ring');
    expect(eventLine(event({ kind: 'started', game: { igdbId: 2, title: "Baldur's Gate 3", coverUrl: '' } }))).toBe(
      "Новая игра: Baldur's Gate 3",
    );
    expect(eventLine(event({ kind: 'favorited', game: { igdbId: 3, title: 'Helldivers 2', coverUrl: '' } }))).toBe(
      'В любимых: Helldivers 2',
    );
  });

  it('незнакомый вид оставляет только название', () => {
    expect(eventLine(event({ kind: 'exploded' }))).toBe('Elden Ring');
  });

  it('без названия остаётся одна подпись', () => {
    expect(eventLine(event({ game: { igdbId: 0, title: '', coverUrl: '' } }))).toBe('Пройдена');
    expect(eventLine(event({ kind: 'exploded', game: { igdbId: 0, title: '  ', coverUrl: '' } }))).toBe('');
  });
});

describe('reactionLabel', () => {
  it('даёт русскую подпись каждой реакции пака', () => {
    expect(REACTIONS.map(reactionLabel)).toEqual([
      'Огонь',
      'Салют',
      'Сердце',
      'Аплодисменты',
      'Череп',
      'Праздник',
      'Глаза',
      'Смех',
    ]);
  });

  it('незнакомую реакцию возвращает как есть', () => {
    expect(reactionLabel('poop')).toBe('poop');
  });
});

describe('REACTIONS', () => {
  it('содержит ровно восемь идентификаторов пака', () => {
    expect(REACTIONS).toEqual(['fire', 'salute', 'heart', 'clap', 'skull', 'party', 'eyes', 'joy']);
  });
});

describe('feedDayGroups', () => {
  it('группирует события по дню и подписывает группу', () => {
    const today = event({ id: 1, createdAt: iso(0, 9) });
    const later = event({ id: 2, createdAt: iso(0, 20) });
    const yesterday = event({ id: 3, createdAt: iso(1) });
    const groups = feedDayGroups([later, today, yesterday]);
    expect(groups).toHaveLength(2);
    expect(groups[0].label).toBe('Сегодня');
    expect(groups[0].events.map((e) => e.id)).toEqual([2, 1]);
    expect(groups[1].label).toBe('Вчера');
    expect(groups[1].events.map((e) => e.id)).toEqual([3]);
  });

  it('сохраняет порядок групп по первому событию', () => {
    const groups = feedDayGroups([event({ id: 1, createdAt: iso(2) }), event({ id: 2, createdAt: iso(0) })]);
    expect(groups.map((g) => g.events[0].id)).toEqual([1, 2]);
  });

  it('пустой список даёт пустой результат', () => {
    expect(feedDayGroups([])).toEqual([]);
  });

  it('битую дату не теряет и складывает в отдельную группу', () => {
    const groups = feedDayGroups([event({ id: 1, createdAt: 'не дата' }), event({ id: 2, createdAt: '' })]);
    expect(groups).toHaveLength(1);
    expect(groups[0].label).toBe('—');
    expect(groups[0].events.map((e) => e.id)).toEqual([1, 2]);
  });

  it('ключи групп различаются', () => {
    const groups = feedDayGroups([event({ id: 1, createdAt: iso(0) }), event({ id: 2, createdAt: iso(1) })]);
    expect(new Set(groups.map((g) => g.key)).size).toBe(2);
  });
});

describe('toggleReaction', () => {
  it('ставит реакцию, которой ещё не было', () => {
    const next = toggleReaction(event(), 'fire');
    expect(next.mine).toEqual(['fire']);
    expect(next.reactions).toEqual([{ emoji: 'fire', count: 1 }]);
  });

  it('увеличивает счётчик существующей реакции', () => {
    const next = toggleReaction(event({ reactions: [{ emoji: 'fire', count: 2 }] }), 'fire');
    expect(next.mine).toEqual(['fire']);
    expect(next.reactions).toEqual([{ emoji: 'fire', count: 3 }]);
  });

  it('снимает свою реакцию и уменьшает счётчик', () => {
    const next = toggleReaction(event({ reactions: [{ emoji: 'fire', count: 3 }], mine: ['fire'] }), 'fire');
    expect(next.mine).toEqual([]);
    expect(next.reactions).toEqual([{ emoji: 'fire', count: 2 }]);
  });

  it('убирает реакцию, когда счётчик дошёл до нуля', () => {
    const next = toggleReaction(event({ reactions: [{ emoji: 'fire', count: 1 }], mine: ['fire'] }), 'fire');
    expect(next.mine).toEqual([]);
    expect(next.reactions).toEqual([]);
  });

  it('не трогает соседние реакции', () => {
    const source = event({ reactions: [{ emoji: 'heart', count: 4 }, { emoji: 'fire', count: 1 }], mine: ['heart'] });
    const next = toggleReaction(source, 'fire');
    expect(next.reactions).toEqual([
      { emoji: 'heart', count: 4 },
      { emoji: 'fire', count: 2 },
    ]);
    expect(next.mine).toEqual(['heart', 'fire']);
  });

  it('не меняет исходное событие', () => {
    const source = event({ reactions: [{ emoji: 'fire', count: 1 }] });
    toggleReaction(source, 'fire');
    expect(source.reactions).toEqual([{ emoji: 'fire', count: 1 }]);
    expect(source.mine).toEqual([]);
  });

  it('незнакомую реакцию оставляет событие без изменений', () => {
    const source = event();
    expect(toggleReaction(source, 'poop')).toBe(source);
  });
});
