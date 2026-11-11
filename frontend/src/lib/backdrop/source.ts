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
 * The pictures behind a game, in the order they are tried: its hero (the user's pick or a landscape
 * screenshot), its cover from the art store, the cover the library holds for it. Empty when there is none.
 */
export function backdropSources(id: string, games: readonly LibraryGame[], art: ArtMap): string[] {
  const { game, key } = artKey(id, games);
  const known = key ? art[key] : undefined;
  return [...new Set([clean(known?.hero), clean(known?.cover), clean(game?.cover)].filter(Boolean))];
}

/**
 * The game a game page claims: a library game by its own id, else the catalog game by its canonical id,
 * whether or not the art store knows it yet (the backdrop asks for the art and stays bare meanwhile).
 */
export function pageAmbient(libraryId: string | undefined, canonicalId: string | null | undefined): string | null {
  return libraryId || canonicalId || null;
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
  return played(games).find((game) => backdropSources(game.id, games, art).length > 0)?.id ?? null;
}

/** Canonical ids of the last few played games the art store has not answered for yet. */
export function playedArtRequests(games: readonly LibraryGame[], art: ArtMap): string[] {
  return played(games)
    .slice(0, PLAYED_ART_REQUESTS)
    .map((game) => artRequest(game.id, games, art))
    .filter((id): id is string => id !== null);
}
