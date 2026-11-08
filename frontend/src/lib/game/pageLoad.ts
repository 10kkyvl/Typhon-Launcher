export interface PageKeyGame {
  id: string;
  canonicalGameId?: string;
}

// The catalog entry is fetched for a page the library does not hold by id. The key
// must not look at the card found through that entry: it appears only after the
// fetch, and a key that follows it refetches forever.
export function catalogLoadKey(id: string, games: readonly PageKeyGame[]): string {
  return games.some((game) => game.id === id || game.canonicalGameId === id) ? '' : id;
}

export interface ReleaseLoadInput {
  id: string;
  canonicalId: string | undefined;
  title: string | undefined;
  sideloaded: boolean;
  external: boolean;
}

export function releaseLoadKey(input: ReleaseLoadInput): string {
  const mode = input.sideloaded ? 'sideloaded' : input.external ? 'external' : 'catalog';
  return `${input.id}|${input.canonicalId ?? ''}|${input.title ?? ''}|${mode}`;
}

export function pageTitle(
  game: { title: string } | undefined,
  viaCatalog: boolean,
  catalogTitle: string | undefined,
): string | undefined {
  return viaCatalog ? catalogTitle : (game?.title ?? catalogTitle);
}

const sameGameRelations = ['edition', 'base', 'original'];

// A release the server matched to another edition lands on a card that names that
// edition, so the page of this one has to look there as well. Remakes and ports
// are other games and stay out.
export function sameGameIds(related: readonly { game: { id: string; relation: string } }[]): string[] {
  return related.filter((item) => sameGameRelations.includes(item.game.relation)).map((item) => item.game.id);
}
