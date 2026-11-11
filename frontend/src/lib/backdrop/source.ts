import { clean } from '../game/view';
import type { LibraryGame } from '../services/library';
import type { GameArt } from '../services/metadata';

type ArtMap = Readonly<Record<string, GameArt | undefined>>;

/** How many of the last played games may ask for art up front, before a screen shows them. */
const PLAYED_ART_REQUESTS = 5;

/** The store key of a game's art: a library game's canonical id, or the id itself for a game outside the library. */
function artKey(id: string, games: readonly LibraryGame[]): { game?: LibraryGame; key: string | null } {
  const game = games.find((g) => g.id === id);
  if (game) return { game, key: game.canonicalGameId || null };
  return { key: id };
}

/**
 * The picture behind a game: its hero (the user's pick or a landscape screenshot), else its cover from
 * the art store, else the cover the library holds for it. Empty when there is none.
 */
export function backdropArt(id: string, games: readonly LibraryGame[], art: ArtMap): string {
  const { game, key } = artKey(id, games);
  const known = key ? art[key] : undefined;
  return clean(known?.hero) || clean(known?.cover) || clean(game?.cover);
}

/** The id to ask the art store for, when it has not answered for this game yet. */
export function artRequest(id: string, games: readonly LibraryGame[], art: ArtMap): string | null {
  const { key } = artKey(id, games);
  return key && !(key in art) ? key : null;
}

function played(games: readonly LibraryGame[]): LibraryGame[] {
  return games
    .map((game) => ({ game, at: game.lastPlayed && !game.hidden ? Date.parse(game.lastPlayed) : Number.NaN }))
    .filter((entry) => entry.at > 0)
    .sort((a, b) => b.at - a.at)
    .map((entry) => entry.game);
}

/** The most recently played library game that has art; null before any is played. */
export function rememberedGame(games: readonly LibraryGame[], art: ArtMap): string | null {
  return played(games).find((game) => backdropArt(game.id, games, art))?.id ?? null;
}

/** Canonical ids of the last few played games the art store has not answered for yet. */
export function playedArtRequests(games: readonly LibraryGame[], art: ArtMap): string[] {
  return played(games)
    .slice(0, PLAYED_ART_REQUESTS)
    .map((game) => artRequest(game.id, games, art))
    .filter((id): id is string => id !== null);
}
