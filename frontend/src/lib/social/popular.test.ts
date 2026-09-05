import { describe, expect, it } from 'vitest';
import type { FeedEvent, FriendView, GameCard } from '../services/social';
import { popularGames } from './popular';

function game(patch: Partial<GameCard> = {}): GameCard {
  return { igdbId: 7, title: 'Elden Ring', coverUrl: 'elden.jpg', ...patch };
}

function event(patch: Partial<FeedEvent> = {}): FeedEvent {
  return {
    id: 1,
    user: { id: 'u1', username: 'kirmalin', displayName: 'Кир', avatarUrl: '' },
    kind: 'completed',
    game: game(),
    createdAt: '2026-09-03T12:00:00Z',
    reactions: [],
    mine: [],
    ...patch,
  };
}

function friend(patch: Partial<FriendView> = {}): FriendView {
  return {
    id: 'u1',
    username: 'kirmalin',
    displayName: 'Кир',
    avatarUrl: '',
    since: '2026-01-01T00:00:00Z',
    presence: null,
    ...patch,
  };
}

describe('popularGames', () => {
  it('считает друзей по игре и ставит популярную выше', () => {
    const events = [
      event({ id: 1, user: { id: 'u1', username: 'kirmalin', displayName: 'Кир', avatarUrl: '' } }),
      event({ id: 2, user: { id: 'u2', username: 'nikita.afk', displayName: 'Никита', avatarUrl: '' } }),
      event({ id: 3, game: game({ igdbId: 9, title: 'Counter-Strike 2' }), user: { id: 'u3', username: 'sonya.vibe', displayName: 'Соня', avatarUrl: '' } }),
    ];

    const top = popularGames(events, []);

    expect(top.map((item) => item.game.title)).toEqual(['Elden Ring', 'Counter-Strike 2']);
    expect(top[0].count).toBe(2);
    expect(top[0].names).toEqual(['Кир', 'Никита']);
    expect(top[1].count).toBe(1);
  });

  it('считает друга один раз, сколько бы событий он ни принёс', () => {
    const events = [event({ id: 1 }), event({ id: 2, kind: 'started' }), event({ id: 3, kind: 'favorited' })];

    const top = popularGames(events, []);

    expect(top).toHaveLength(1);
    expect(top[0].count).toBe(1);
    expect(top[0].names).toEqual(['Кир']);
  });

  it('не считает события самого пользователя', () => {
    const events = [
      event({ id: 1, user: { id: 'me', username: 'egor', displayName: 'Егор', avatarUrl: '' } }),
      event({ id: 2, user: { id: 'u2', username: 'nikita.afk', displayName: 'Никита', avatarUrl: '' } }),
    ];

    const top = popularGames(events, [], 'me');

    expect(top).toHaveLength(1);
    expect(top[0].count).toBe(1);
    expect(top[0].names).toEqual(['Никита']);
  });

  it('подхватывает игру из присутствия, даже если её нет в ленте', () => {
    const friends = [
      friend({
        id: 'u2',
        username: 'nikita.afk',
        displayName: 'Никита',
        presence: { status: 'online', gameId: 42, gameTitle: 'Counter-Strike 2' },
      }),
    ];

    const top = popularGames([], friends);

    expect(top).toHaveLength(1);
    expect(top[0].game).toEqual({ igdbId: 42, title: 'Counter-Strike 2', coverUrl: '' });
    expect(top[0].count).toBe(1);
    expect(top[0].playing).toBe(1);
  });

  it('не считает играющим друга, который офлайн', () => {
    const friends = [friend({ presence: { status: 'offline', gameId: 7, gameTitle: 'Elden Ring' } })];

    expect(popularGames([], friends)).toEqual([]);
  });

  it('дополняет обложку и название из ленты для игры из присутствия', () => {
    const friends = [friend({ presence: { status: 'online', gameId: 7, gameTitle: '' } })];
    const events = [event({ user: { id: 'u2', username: 'nikita.afk', displayName: 'Никита', avatarUrl: '' } })];

    const top = popularGames(events, friends);

    expect(top).toHaveLength(1);
    expect(top[0].game).toEqual({ igdbId: 7, title: 'Elden Ring', coverUrl: 'elden.jpg' });
    expect(top[0].count).toBe(2);
    expect(top[0].playing).toBe(1);
    expect(top[0].names).toEqual(['Кир', 'Никита']);
  });

  it('при равном числе друзей выше та игра, в которую играют сейчас', () => {
    const events = [
      event({ id: 1, game: game({ igdbId: 7, title: 'Elden Ring' }) }),
      event({
        id: 2,
        game: game({ igdbId: 9, title: 'Counter-Strike 2' }),
        user: { id: 'u2', username: 'nikita.afk', displayName: 'Никита', avatarUrl: '' },
      }),
    ];
    const friends = [
      friend({
        id: 'u2',
        username: 'nikita.afk',
        displayName: 'Никита',
        presence: { status: 'online', gameId: 9, gameTitle: 'Counter-Strike 2' },
      }),
    ];

    const top = popularGames(events, friends);

    expect(top.map((item) => item.game.title)).toEqual(['Counter-Strike 2', 'Elden Ring']);
  });

  it('при прочих равных выше более свежее событие', () => {
    const events = [
      event({ id: 1, game: game({ igdbId: 7, title: 'Elden Ring' }), createdAt: '2026-09-01T12:00:00Z' }),
      event({ id: 2, game: game({ igdbId: 9, title: 'Counter-Strike 2' }), createdAt: '2026-09-04T12:00:00Z' }),
    ];

    const top = popularGames(events, []);

    expect(top.map((item) => item.game.title)).toEqual(['Counter-Strike 2', 'Elden Ring']);
  });

  it('пропускает события без игры', () => {
    const events = [event({ game: game({ igdbId: 0, title: 'Ничто' }) }), event({ id: 2 })];

    const top = popularGames(events, []);

    expect(top.map((item) => item.game.title)).toEqual(['Elden Ring']);
  });

  it('обрезает список до лимита', () => {
    const events = [1, 2, 3, 4, 5, 6].map((n) =>
      event({ id: n, game: game({ igdbId: n, title: `Игра ${n}` }), createdAt: `2026-09-0${n}T12:00:00Z` }),
    );

    expect(popularGames(events, [], '', 3)).toHaveLength(3);
    expect(popularGames(events, [])).toHaveLength(5);
  });

  it('не падает на пустых данных', () => {
    expect(popularGames([], [])).toEqual([]);
  });
});
