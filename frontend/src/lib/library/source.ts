import { msg } from '../i18n';
import type { LibraryGame } from '../services/library';

export type GameSource = 'typhon' | 'disk' | 'steam' | 'epic' | 'gog';

export const sourceOrder: readonly GameSource[] = ['typhon', 'steam', 'epic', 'gog', 'disk'];

const providers: readonly string[] = ['steam', 'epic', 'gog'];

export function gameSource(game: Pick<LibraryGame, 'source' | 'external'>): GameSource | null {
  if (game.external) {
    return providers.includes(game.external.provider) ? (game.external.provider as GameSource) : null;
  }
  switch (game.source) {
    case undefined:
    case '':
    case 'managed':
      return 'typhon';
    case 'discovered':
      return 'disk';
    default:
      return null;
  }
}

export function sourceLabel(source: GameSource): string {
  switch (source) {
    case 'typhon':
      return msg('games.sourceTyphon');
    case 'steam':
      return msg('games.sourceSteam');
    case 'epic':
      return msg('games.sourceEpic');
    case 'gog':
      return msg('games.sourceGog');
    case 'disk':
      return msg('games.sourceDisk');
  }
}

export function sourceTitle(source: GameSource): string {
  switch (source) {
    case 'typhon':
      return msg('games.sourceTyphonTitle');
    case 'disk':
      return msg('games.sourceDiskTitle');
    default:
      return msg('games.externalBadgeTitle', { provider: sourceLabel(source) });
  }
}
