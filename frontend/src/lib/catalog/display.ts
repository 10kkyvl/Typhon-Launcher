import type { CatalogGame } from '../services/sources';

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
