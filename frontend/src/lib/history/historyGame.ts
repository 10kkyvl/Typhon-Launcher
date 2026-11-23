import { Kind, type Record } from '../../../bindings/typhon/internal/history';
import type { LibraryGame } from '../services/library';
import type { GameArt } from '../services/metadata';

/** The library game a history record is about, if the library still holds it. */
export type HistoryGameLookup = (gameId: string | undefined) => LibraryGame | undefined;

/**
 * The one join of a history record to the library. Go records either the game's library id (an
 * install, a removal, a move, a copy over the local network) or its catalog id (a download, a failed
 * install, an update, a failed update, a rollback), so a record finds its game by the library id first, then by the
 * catalog id. Two library games of one catalog game: the first in the library's order.
 */
export function historyGames(games: readonly LibraryGame[]): HistoryGameLookup {
  const byId = new Map(games.map((game) => [game.id, game]));
  const byCatalog = new Map<string, LibraryGame>();
  for (const game of games) {
    if (game.canonicalGameId && !byCatalog.has(game.canonicalGameId)) byCatalog.set(game.canonicalGameId, game);
  }
  return (gameId) => (gameId ? (byId.get(gameId) ?? byCatalog.get(gameId)) : undefined);
}

// The kinds Go records with the game's catalog id (see historyGames).
const CATALOG_KINDS: ReadonlySet<Kind> = new Set([
  Kind.KindDownloaded,
  Kind.KindInstallFailed,
  Kind.KindUpdated,
  Kind.KindUpdateFailed,
  Kind.KindRolledBack,
]);

/**
 * The picture of a record's game: `key` is the catalog id the art store knows it by (what the screen asks
 * `requestArt` for), '' when there is none, as for a removed game whose record holds its library id;
 * `cover` the art store's cover, else the library's own, else ''; `seed` names the game for the generated
 * art shown without a cover.
 */
export function historyArt(
  record: Pick<Record, 'kind' | 'gameId'>,
  game: Pick<LibraryGame, 'id' | 'cover' | 'canonicalGameId'> | undefined,
  art: { readonly [id: string]: GameArt },
): { key: string; cover: string; seed: string } {
  const key = game ? (game.canonicalGameId ?? '') : CATALOG_KINDS.has(record.kind) ? (record.gameId ?? '') : '';
  const cover = (key && art[key]?.cover) || game?.cover || '';
  return { key, cover, seed: key || game?.id || record.gameId || '' };
}
