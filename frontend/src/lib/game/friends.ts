import type { GameFriend, GameFriends } from '../services/social';

export interface GameFriendRow {
  friend: GameFriend;
  playing: boolean;
}

export function gameFriendRows(page: GameFriends | null | undefined): GameFriendRow[] {
  const played = page?.played ?? [];
  const playingNow = page?.playingNow ?? [];
  const byId = new Map(played.map((friend) => [friend.id, friend]));
  const live = new Set(playingNow.map((friend) => friend.id));
  const rows: GameFriendRow[] = [];
  for (const friend of playingNow) {
    rows.push({ friend: byId.get(friend.id) ?? { ...friend, status: '' }, playing: true });
  }
  for (const friend of played) {
    if (!live.has(friend.id)) rows.push({ friend, playing: false });
  }
  return rows;
}
