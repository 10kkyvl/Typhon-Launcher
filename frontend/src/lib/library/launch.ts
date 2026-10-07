import type { LibraryGame } from '../services/library';

export function canPlay(game: Pick<LibraryGame, 'executable' | 'uninstalled'>): boolean {
  return !game.uninstalled && Boolean(game.executable);
}
