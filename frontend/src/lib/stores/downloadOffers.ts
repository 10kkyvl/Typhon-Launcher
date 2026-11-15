import { derived, type Readable } from 'svelte/store';
import { downloadActions, type DownloadAction } from '../downloads/actions';
import { waitsForSchedule } from '../downloads/schedule';
import { downloads } from './downloads';
import { playThrottle } from './playThrottle';

// Apart from stores/downloads.ts so that the downloads store does not pull in the play-throttle state
// and its formatting for every reader of the list.

/** The ids of the downloads the schedule holds back now (`waitsForSchedule`). */
export const waitingForSchedule: Readable<ReadonlySet<string>> = derived(
  [downloads, playThrottle],
  ([$downloads, $playThrottle]) => new Set($downloads.filter((d) => waitsForSchedule(d, $playThrottle)).map((d) => d.id)),
);

/** What each download offers, by id (`downloadActions`): the one answer for every surface. */
export const downloadOffers: Readable<ReadonlyMap<string, readonly DownloadAction[]>> = derived(
  [downloads, playThrottle],
  ([$downloads, $playThrottle]) => new Map($downloads.map((d) => [d.id, downloadActions(d, $playThrottle)])),
);
