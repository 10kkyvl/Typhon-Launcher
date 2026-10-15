import type { LibraryGame } from '../services/library';
import { isLaunchable } from './external';

export function canPlay(game: Pick<LibraryGame, 'executable' | 'uninstalled' | 'external'>): boolean {
  return !game.uninstalled && isLaunchable(game);
}
