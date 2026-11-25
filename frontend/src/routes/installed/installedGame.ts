import { sizeWords } from '../../lib/components/diskMeter';
import { playGame, stopGame, type LibraryGame } from '../../lib/services/library';
import type { GameArt } from '../../lib/services/metadata';
import { toast } from '../../lib/stores/toasts';
import { bytesSize } from '../../lib/utils/format';
import { errorCode, hasMessage, msg } from '../../lib/i18n';

// What the parts of the Installed screen share about one game: its words, its pictures, Play and Stop.
// Play and Stop never throw: a failure is said in the catalogue's words for its code.

export function libraryErrorText(err: unknown, fallback: string): string {
  const code = errorCode(err);
  return hasMessage(code) ? msg(code) : fallback;
}

export type Sort = 'recent' | 'alpha' | 'size';

// A size not fully measured is a lower bound when some bytes were counted, unknown otherwise: said in
// the disk meter's words. A size of 0 that is not flagged says nothing.
export function sizeLabel(game: LibraryGame) {
  if (!game.sizeUnknown) return game.sizeBytes > 0 ? bytesSize(game.sizeBytes) : '';
  return sizeWords(game.sizeBytes > 0 ? { kind: 'atLeast', bytes: game.sizeBytes } : { kind: 'unknown' }, bytesSize, {
    atLeast: (size) => msg('common.meterAtLeast', { size }),
    atMost: (size) => msg('common.meterAtMost', { size }),
    unknown: msg('common.meterSizeUnknown'),
  });
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
  } catch (err) {
    toast(libraryErrorText(err, msg('games.errorStopFailed')), 'danger');
  }
}
