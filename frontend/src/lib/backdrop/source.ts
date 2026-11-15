import { clean, pickHero } from '../game/view';
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
 * Where a game's colour (stores/gameTone.ts) is read from: kept by the art key, so a catalog card, the
 * library card and the game page of one game agree, or by the library id of a game with no catalog match.
 * The pictures go the covers first, the one a card shows before the
 * library's own, then the hero. Null while the art store has not answered for the game: a picture read
 * now could be replaced by the answer, and the colour would change twice.
 */
export function toneSource(id: string, games: readonly LibraryGame[], art: ArtMap, answered: ReadonlySet<string>): { key: string; pictures: string[] } | null {
  const { game, key } = artKey(id, games);
  if (key && !(key in art) && !answered.has(key)) return null;
  const known = key ? art[key] : undefined;
  const own = game ?? (key ? games.find((g) => g.canonicalGameId === key) : undefined);
  return { key: key ?? id, pictures: [...new Set([clean(known?.cover), clean(own?.cover), clean(known?.hero)].filter(Boolean))] };
}

/** Whether a game has a wide picture for the stage: known yes (with it), known no, or not known yet. */
export type WideArt = { state: 'yes'; src: string } | { state: 'no' } | { state: 'unknown' };

/**
 * The wide picture a game stands on the stage with: Go's hero choice by the one rule (`pickHero`: a
 * poster is the cover again, and the stage would crop it). Not known yet until the art store has heard
 * back about the game (`answered`: the backend leaves a game without art out of its answer); known
 * missing for a library game without a catalog match, for which no art will come. A "no" can still turn
 * to "yes" when art the backend went to fetch arrives later.
 */
export function wideArt(id: string, games: readonly LibraryGame[], art: ArtMap, answered: ReadonlySet<string> = new Set()): WideArt {
  const { key } = artKey(id, games);
  if (!key) return { state: 'no' };
  if (!(key in art)) return answered.has(key) ? { state: 'no' } : { state: 'unknown' };
  const src = pickHero(art[key]);
  return src ? { state: 'yes', src } : { state: 'no' };
}

/**
 * The game a game page claims: a library game by its own id, else the catalog game by its canonical id,
 * whether or not the art store knows it yet (the backdrop asks for the art and stays bare meanwhile).
 * Nothing while the page shows no game: an id it could not find or load is not a game to ask art for.
 */
export function pageAmbient(libraryId: string | undefined, canonicalId: string | null | undefined, shown: boolean): string | null {
  if (!shown) return null;
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
