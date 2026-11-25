import { errorCode, msg, type MessageKey } from '../i18n';
import { installErrorText } from '../install/installErrors';

/** The codes the disk usage service raises itself (internal/diskusage, uierr), by their words. */
export const REASONS: Record<string, MessageKey> = {
  'diskusage.not_started': 'errInstall.diskusageNotStarted',
  'diskusage.not_scanned': 'errInstall.diskusageNotScanned',
  'diskusage.not_deletable': 'errInstall.diskusageNotDeletable',
  'diskusage.unsafe_path': 'errInstall.diskusageUnsafePath',
  'diskusage.no_game': 'errInstall.diskusageNoGame',
  'diskusage.scan_cancelled': 'diskusage.scan_cancelled',
};

/**
 * The words for a failure of a disk usage call: the service's own codes here, the codes of the owners
 * it asks (an install, a download, a move) from the install table the screen has always read, else
 * `fallback`. Go's own text never reaches the window.
 */
export function diskusageErrorText(err: unknown, fallback: string): string {
  const key = REASONS[errorCode(err)];
  return key ? msg(key) : installErrorText(err, fallback);
}

/** A scan stopped by CancelScan (the page was left): not a failure to show. */
export function isScanCancelled(err: unknown): boolean {
  return errorCode(err) === 'diskusage.scan_cancelled';
}

/** A group delete that left some of its items: what went (deleted now or already gone) and what stayed. */
export function partialText(result: { deleted: number; gone: number; failed: number }): string {
  return msg('diskusage.delete_partial', { deleted: result.deleted + result.gone, failed: result.failed });
}

/** A group delete Go answered with items it could not delete: a failure of the confirmation it ran in. */
export class PartialDeleteError extends Error {
  readonly result: { deleted: number; gone: number; failed: number };

  constructor(result: { deleted: number; gone: number; failed: number }) {
    super('typhon:diskusage.delete_partial');
    this.result = result;
  }
}

/** The words for a delete that did not go through: the counts of a partial one, else its code's. */
export function deleteErrorText(err: unknown): string {
  return err instanceof PartialDeleteError ? partialText(err.result) : diskusageErrorText(err, msg('diskusage.deleteFailed'));
}
