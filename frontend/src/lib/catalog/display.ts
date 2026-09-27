import type { CatalogGame } from '../services/sources';
import type { Recommendation } from '../recommendations/display';

// Shelf membership changes independently of server pages. Keep the original
// page sequence for continuation and hide only proven canonical aliases here.
export function catalogWithoutDiscovery(items: CatalogGame[], discovery: Recommendation[]): CatalogGame[] {
  const ids = (game: CatalogGame) => [game.id, game.serverId, ...(game.aliasIds ?? [])].filter(Boolean);
  const picks = new Set(discovery.flatMap((item) => ids(item.game)));
  return items.filter((game) => !ids(game).some((id) => picks.has(id)));
}

// Metadata events can contain an older partial game snapshot. Keep the fresh
// Browse fields visible when such a snapshot has no developer value.
export function mergeCatalogDisplay(game: CatalogGame, metadata?: CatalogGame): CatalogGame {
  if (!metadata) return game;
  return {
    ...game,
    ...metadata,
    developer: metadata.developer || game.developer,
  };
}
