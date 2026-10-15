import type { LibraryGame } from '../services/library';

export function isExternal(game: Pick<LibraryGame, 'external'> | null | undefined): boolean {
  return Boolean(game?.external);
}

export function externalLabel(game: Pick<LibraryGame, 'external'> | null | undefined): string {
  switch (game?.external?.provider) {
    case 'steam':
      return 'Steam';
    case 'gog':
      return 'GOG';
    case 'epic':
      return 'Epic';
    default:
      return '';
  }
}

export function isLaunchable(game: Pick<LibraryGame, 'executable' | 'external'>): boolean {
  return Boolean(game.executable) || Boolean(game.external?.launchUri);
}
