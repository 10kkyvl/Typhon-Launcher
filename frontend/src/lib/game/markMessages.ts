import { errorCode, hasMessage, msg } from '../i18n';

export function markError(err: unknown, fallback: string): string {
  const code = errorCode(err);
  if (code === 'library.too_many_favorites') return msg('games.markFavoritesLimit');
  return hasMessage(code) ? msg(code) : fallback;
}
