import { writable } from 'svelte/store';
import { getCatalogGames } from '../services/sources';
import { metadataErrorText } from '../metadata/metadataErrors';
import { msg } from '../i18n';
import { toast } from './toasts';

export const libraryGenres = writable<Record<string, string[]>>({});

const requested = new Set<string>();
const RETRY_AFTER_MS = 60_000;

export async function requestLibraryGenres(ids: string[]) {
  const missing = [...new Set(ids)].filter((id) => id && !requested.has(id));
  if (missing.length === 0) return;
  for (const id of missing) requested.add(id);
  try {
    const games = await getCatalogGames(missing);
    libraryGenres.update((current) => ({
      ...current,
      ...Object.fromEntries(games.map((game) => [game.id, game.genres ?? []])),
    }));
  } catch (err) {
    setTimeout(() => {
      for (const id of missing) requested.delete(id);
    }, RETRY_AFTER_MS);
    toast(metadataErrorText(err, msg('games.libraryLoadGamesError')), 'danger');
  }
}
