import { msg } from '../i18n';
import { genreLabel } from '../metadata/labels';
export { genreLabel } from '../metadata/labels';
import type { CatalogGame } from '../services/sources';

export interface Recommendation {
  game: CatalogGame;
  libraryId?: string;
  reason: string;
  reasonTitle?: string;
  reasonGenre?: string;
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
