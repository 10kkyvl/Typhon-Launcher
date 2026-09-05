import { msg } from '../i18n';

export function markError(err: unknown, fallback: string): string {
  const raw = err instanceof Error ? err.message : String(err ?? '');
  if (raw.includes('favorites limit reached')) return msg('games.markFavoritesLimit');
  return fallback;
}
