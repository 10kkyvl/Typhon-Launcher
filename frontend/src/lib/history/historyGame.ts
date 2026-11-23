import type { LibraryGame } from '../services/library';

/** The library game a history record is about, if the library still holds it. */
export type HistoryGameLookup = (gameId: string | undefined) => LibraryGame | undefined;

/**
 * The one join of a history record to the library. Go records either the game's library id (an
 * install, a removal, a move, a copy over the local network) or its catalog id (a download, a failed
 * install, an update, a rollback), so a record finds its game by the library id first, then by the
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
