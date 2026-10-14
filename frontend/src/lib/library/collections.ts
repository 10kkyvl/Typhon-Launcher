import type { LibraryCollection, LibraryGame } from '../services/library';

type GameFields = Partial<Pick<LibraryGame, 'tags' | 'status'>>;
type CollectionFields = Partial<Pick<LibraryCollection, 'tags' | 'genres' | 'status'>>;

const fold = (value: string) => value.normalize('NFKC').trim().toLocaleLowerCase();

export function matchesCollection(
  game: GameFields,
  catalogGenres: string[] | undefined,
  collection: CollectionFields,
): boolean {
  if (collection.status && game.status !== collection.status) return false;
  const own = new Set((game.tags ?? []).map(fold));
  if ((collection.tags ?? []).some((tag) => !own.has(fold(tag)))) return false;
  const wanted = collection.genres ?? [];
  if (wanted.length === 0) return true;
  const have = new Set((catalogGenres ?? []).map(fold));
  return wanted.some((genre) => have.has(fold(genre)));
}

export function collectionShelfId(id: string): `collection:${string}` {
  return `collection:${id}`;
}
