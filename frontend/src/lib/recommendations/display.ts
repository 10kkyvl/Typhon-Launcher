import { msg, type MessageKey } from '../i18n';
import type { CatalogGame } from '../services/sources';

export interface Recommendation {
  game: CatalogGame;
  libraryId?: string;
  reason: string;
  reasonTitle?: string;
  reasonGenre?: string;
}

const genreKeys: Record<string, MessageKey> = {
  Action: 'games.genreAction', RPG: 'games.genreRPG', 'Role-playing (RPG)': 'games.genreRPG',
  Shooter: 'games.genreShooter', Adventure: 'games.genreAdventure', Strategy: 'games.genreStrategy', Indie: 'games.genreIndie',
  'Экшены': 'games.genreAction', 'Ролевые игры': 'games.genreRPG', 'Приключенческие игры': 'games.genreAdventure',
  'Экшен': 'games.genreAction', 'Ролевые': 'games.genreRPG', 'Шутеры': 'games.genreShooter',
  'Приключения': 'games.genreAdventure', 'Стратегии': 'games.genreStrategy', 'Инди': 'games.genreIndie',
};

export function genreLabel(genre: string): string {
  return genreKeys[genre] ? msg(genreKeys[genre]) : genre;
}

export function recommendationReason(item: Recommendation): string {
  switch (item.reason) {
    case 'similar': return item.reasonTitle ? msg('games.recommendationSimilar', { title: item.reasonTitle }) : '';
    case 'favorite': return msg('games.recommendationFavorite');
    case 'unplayed': return msg('games.recommendationUnplayed');
    case 'return': return msg('games.recommendationReturn');
    case 'popular': return msg('games.recommendationPopular');
    case 'category': return item.reasonGenre ? msg('games.recommendationCategory', { genre: genreLabel(item.reasonGenre) }) : '';
    case 'genre': return item.reasonGenre ? msg('games.recommendationGenre', { genre: genreLabel(item.reasonGenre) }) : '';
    default: return '';
  }
}
