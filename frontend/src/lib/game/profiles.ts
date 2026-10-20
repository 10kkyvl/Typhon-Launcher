import { msg } from '../i18n';
import type { LibraryGame } from '../services/library';

export interface ProfileChoice {
  id: string;
  name: string;
}

export const playProfilePrefix = 'play-profile:';

type ProfileSource = Pick<LibraryGame, 'profiles' | 'lastProfile'>;

export function profileChoices(game: ProfileSource): ProfileChoice[] {
  const extra = game.profiles ?? [];
  if (extra.length === 0) return [];
  return [{ id: '', name: msg('games.profileDefault') }, ...extra.map((p) => ({ id: p.id, name: p.name }))];
}

export function lastProfileId(game: ProfileSource): string {
  const id = game.lastProfile ?? '';
  return id !== '' && (game.profiles ?? []).some((p) => p.id === id) ? id : '';
}

export function playLabel(game: ProfileSource): string {
  const id = lastProfileId(game);
  if (id === '') return msg('games.play');
  const name = (game.profiles ?? []).find((p) => p.id === id)?.name ?? '';
  return msg('games.playProfile', { name });
}

export function playProfileItems(game: ProfileSource): { id: string; label: string }[] {
  return profileChoices(game).map((p) => ({
    id: playProfilePrefix + p.id,
    label: msg('games.actionPlayProfile', { name: p.name }),
  }));
}

export function profileIdFromItem(itemId: string): string | null {
  return itemId.startsWith(playProfilePrefix) ? itemId.slice(playProfilePrefix.length) : null;
}

export function withProfileItems<T extends { id: string }>(
  items: T[],
  game: ProfileSource,
): (T | { id: string; label: string })[] {
  const at = items.findIndex((item) => item.id === 'play');
  const extra = playProfileItems(game);
  if (at < 0 || extra.length === 0) return items;
  return [...items.slice(0, at + 1), ...extra, ...items.slice(at + 1)];
}
