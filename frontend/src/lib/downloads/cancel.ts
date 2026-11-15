import type { Download } from '../services/downloads';

/**
 * Whether Go's Cancel leaves this download's data on disk: `discard` deletes only what a download wrote
 * into a folder of its own, so an in-place download (an update or a repair written into the game's
 * folder) and a flat one (files straight into the destination) keep theirs (internal/download
 * manager.go `discard`, `removalRoot`). A download whose folder Go cannot name keeps them too, which
 * the record does not say.
 */
export function cancelKeepsFiles(download: Pick<Download, 'inPlace' | 'flat'>): boolean {
  return download.inPlace === true || download.flat === true;
}

/** Cancel asks first when there is data on disk; with nothing downloaded it runs at once. */
export function cancelAsks(download: Pick<Download, 'downloaded'>): boolean {
  return download.downloaded > 0;
}
