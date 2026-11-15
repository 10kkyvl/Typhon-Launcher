import { playGame, stopGame, type LibraryGame } from '../../lib/services/library';
import type { GameArt } from '../../lib/services/metadata';
import { toast } from '../../lib/stores/toasts';
import { bytesSize } from '../../lib/utils/format';
import { errorCode, hasMessage, msg } from '../../lib/i18n';

// What the parts of the Installed screen share about one game: its words, its pictures, Play and Stop.

export function libraryErrorText(err: unknown, fallback: string): string {
  const code = errorCode(err);
  return hasMessage(code) ? msg(code) : fallback;
}

export type Sort = 'recent' | 'alpha' | 'size';

export function sizeLabel(game: LibraryGame) {
  return game.sizeBytes > 0 ? bytesSize(game.sizeBytes) : '';
}

export function updateLabel(update?: { available: boolean; kind: string }) {
  if (!update?.available) return '';
  return update.kind === 'update' ? msg('games.updateKindUpdate') : msg('games.updateKindNewRelease');
}

export function coverFor(game: LibraryGame, art: Record<string, GameArt>) {
  return (game.canonicalGameId && art[game.canonicalGameId]?.cover) || game.cover;
}

export function heroFor(game: LibraryGame, art: Record<string, GameArt>) {
  return (game.canonicalGameId && art[game.canonicalGameId]?.hero) || coverFor(game, art);
}

export async function play(game: LibraryGame) {
  try {
    await playGame(game.id);
  } catch (err) {
    toast(libraryErrorText(err, msg('games.errorPlayFailed')), 'danger');
  }
}

export async function stop(game: LibraryGame) {
  try {
    await stopGame(game.id);
  } catch {
    toast(msg('games.errorStopFailed'), 'danger');
  }
}
