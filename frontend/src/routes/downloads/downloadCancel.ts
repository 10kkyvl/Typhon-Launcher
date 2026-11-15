import type { ConfirmPrompt } from '../../lib/confirm/prompts';
import { msg } from '../../lib/i18n';
import type { Download } from '../../lib/services/downloads';
import { bytesSize } from '../../lib/utils/format';

// Go sends both flags on a download (internal/download/model.go); the service's type does not list them yet.
type Placed = Download & { inPlace?: boolean; flat?: boolean };

/**
 * Whether Go's Cancel leaves the data on disk: `discard` deletes only what a download wrote into a
 * folder of its own, so an in-place download (an update written over the game) and a flat one (files
 * straight into the destination) keep their files. A download whose folder Go cannot name keeps them
 * too, which the frontend cannot see.
 */
export function cancelKeepsData(download: Download): boolean {
  const placed = download as Placed;
  return Boolean(placed.inPlace || placed.flat);
}

/** Cancel asks first when there is data on disk to lose; with nothing downloaded it runs at once. */
export function cancelAsks(download: Download): boolean {
  return download.downloaded > 0;
}

/** The question before Go's Cancel: names the download and says what happens to what it downloaded. */
export function cancelPrompt(download: Download): ConfirmPrompt {
  const size = bytesSize(download.downloaded);
  return {
    title: msg('downloads.queue.cancelTitle', { title: download.name }),
    text: cancelKeepsData(download)
      ? msg('downloads.queue.cancelKeeps', { size, path: download.destination })
      : msg('downloads.queue.cancelDeletes', { size }),
    confirm: msg('ui.cancelDownload'),
    busy: msg('downloads.queue.cancelling'),
    cancel: msg('ui.dontCancel'),
  };
}
