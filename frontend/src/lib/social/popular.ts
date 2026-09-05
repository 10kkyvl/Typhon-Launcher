import type { FeedEvent, FriendView, GameCard } from '../services/social';
import { isPlaying } from './presence';

export interface PopularGame {
  game: GameCard;
  count: number;
  playing: number;
  names: string[];
}

interface Entry {
  game: GameCard;
  playing: number;
  latest: number;
  users: Set<string>;
  names: string[];
}

const POPULAR_LIMIT = 5;

function timestamp(iso: string): number {
  const at = new Date(iso).getTime();
  return Number.isNaN(at) ? 0 : at;
}

function entryFor(entries: Map<number, Entry>, game: GameCard): Entry {
  const found = entries.get(game.igdbId);
  if (!found) {
    const entry: Entry = { game: { ...game }, playing: 0, latest: 0, users: new Set(), names: [] };
    entries.set(game.igdbId, entry);
    return entry;
  }
  if (!found.game.title) found.game.title = game.title;
  if (!found.game.coverUrl) found.game.coverUrl = game.coverUrl;
  return found;
}

function addUser(entry: Entry, id: string, name: string) {
  if (entry.users.has(id)) return;
  entry.users.add(id);
  entry.names.push(name);
}

export function popularGames(
  events: FeedEvent[],
  friends: FriendView[],
  selfId = '',
  limit = POPULAR_LIMIT,
): PopularGame[] {
  const entries = new Map<number, Entry>();

  for (const friend of friends) {
    const gameId = friend.presence?.gameId;
    if (!isPlaying(friend.presence) || !gameId) continue;
    const entry = entryFor(entries, { igdbId: gameId, title: friend.presence?.gameTitle ?? '', coverUrl: '' });
    entry.playing += 1;
    addUser(entry, friend.id, friend.displayName || friend.username);
  }

  for (const event of events) {
    if (!event.game?.igdbId || event.user.id === selfId) continue;
    const entry = entryFor(entries, event.game);
    entry.latest = Math.max(entry.latest, timestamp(event.createdAt));
    addUser(entry, event.user.id, event.user.displayName || event.user.username);
  }

  return [...entries.values()]
    .sort((a, b) => {
      const byCount = b.users.size - a.users.size;
      if (byCount !== 0) return byCount;
      const byPlaying = b.playing - a.playing;
      if (byPlaying !== 0) return byPlaying;
      const byLatest = b.latest - a.latest;
      if (byLatest !== 0) return byLatest;
      return a.game.title.localeCompare(b.game.title, 'ru');
    })
    .slice(0, limit)
    .map((entry) => ({ game: entry.game, count: entry.users.size, playing: entry.playing, names: entry.names }));
}
