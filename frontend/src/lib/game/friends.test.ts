import { describe, expect, it } from 'vitest';
import { gameFriendRows } from './friends';
import type { GameFriend, GameFriends } from '../services/social';

function friend(id: string, extra: Partial<GameFriend> = {}): GameFriend {
  return {
    id,
    username: id,
    displayName: id,
    avatarUrl: '',
    status: '',
    ...extra,
  };
}

function page(part: Partial<GameFriends>): GameFriends {
  return { played: [], playingNow: [], ...part };
}

describe('gameFriendRows', () => {
  it('lists a friend playing right now once, keeping the library entry', () => {
    const rows = gameFriendRows(
      page({
        played: [friend('a', { status: 'playing', playtimeSeconds: 720 })],
        playingNow: [friend('a')],
      }),
    );
    expect(rows).toHaveLength(1);
    expect(rows[0].playing).toBe(true);
    expect(rows[0].friend.playtimeSeconds).toBe(720);
  });

  it('puts the friends in the game above the rest', () => {
    const rows = gameFriendRows(
      page({
        played: [friend('a', { status: 'completed' }), friend('b', { status: 'playing' })],
        playingNow: [friend('b')],
      }),
    );
    expect(rows.map((row) => [row.friend.id, row.playing])).toEqual([
      ['b', true],
      ['a', false],
    ]);
  });

  it('keeps a friend who plays without sharing the library', () => {
    const rows = gameFriendRows(page({ playingNow: [friend('a')] }));
    expect(rows).toEqual([{ friend: friend('a'), playing: true }]);
  });

  it('returns nothing without a page', () => {
    expect(gameFriendRows(null)).toEqual([]);
  });
});
