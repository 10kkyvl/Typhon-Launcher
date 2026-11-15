import { readable, type Readable } from 'svelte/store';

/** The downloads list's first read: whether a list has arrived, and why the first read failed. */
export interface DownloadsListLoad {
  loaded: boolean;
  /** Why the first read failed; null once a list has arrived or while it is under way. */
  error: unknown;
}

/**
 * The first read as the page sees it, the one place the page learns it. `stores/downloads` keeps no
 * such state: until it exports one (`downloadsLoaded`, `downloadsLoadError`), the list counts as read
 * from the start, and an empty list before Go answers shows as an empty list, as it always has.
 */
export const downloadsListLoad: Readable<DownloadsListLoad> = readable({ loaded: true, error: null });

/**
 * Reads the list again after a failed first read; answers whether a list is there. Nothing reports a
 * failure yet (see `downloadsListLoad`), so there is nothing to read again and the list is there.
 */
export async function reloadDownloadsList(): Promise<boolean> {
  return true;
}
