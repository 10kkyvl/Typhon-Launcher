import type { GameCard } from '../services/social';

type Art = Pick<GameCard, 'coverUrl'> & Partial<Pick<GameCard, 'heroUrl'>>;

/**
 * Картинка для широкого слота (16/9): сначала горизонтальный кадр от бэкенда,
 * и только если его нет — портретная обложка, которую рамка обрежет.
 */
export function wideArt(game: Art | null | undefined): string {
  if (!game) return '';
  return game.heroUrl || game.coverUrl || '';
}
