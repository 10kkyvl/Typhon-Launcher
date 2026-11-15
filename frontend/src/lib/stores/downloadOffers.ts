import { derived, type Readable } from 'svelte/store';
import { downloadActions, type DownloadAction } from '../downloads/actions';
import { waitsForSchedule } from '../downloads/schedule';
import type { Download } from '../services/downloads';
import type { PlayThrottle } from '../services/playThrottle';
import { downloads } from './downloads';
import { playThrottle } from './playThrottle';

// Apart from stores/downloads.ts so that the downloads store does not pull in the play-throttle state
// and its formatting for every reader of the list.
//
// Both stores change the list four times a second per moving download, on every progress tick, but
// what they answer depends only on a download's status, its ignoreSchedule and whether the schedule
// blocks. They notify only when one of those changed, and keep the answer for an unchanged download
// the same object, so a row reading it does not re-evaluate on a tick.

/** What an answer for one download depends on. */
function inputsOf(download: Download, throttle: PlayThrottle): string {
  return `${download.status}|${Boolean(download.ignoreSchedule)}|${Boolean(throttle.schedule?.blocked)}`;
}

/** The ids of the downloads the schedule holds back now (`waitsForSchedule`). */
export const waitingForSchedule: Readable<ReadonlySet<string>> = (() => {
  let current: ReadonlySet<string> = new Set();
  return derived(
    [downloads, playThrottle],
    ([$downloads, $playThrottle], set) => {
      const next = new Set($downloads.filter((d) => waitsForSchedule(d, $playThrottle)).map((d) => d.id));
      if (next.size === current.size && [...next].every((id) => current.has(id))) return;
      current = next;
      set(current);
    },
    current,
  );
})();

/** What each download offers, by id (`downloadActions`): the one answer for every surface. */
export const downloadOffers: Readable<ReadonlyMap<string, readonly DownloadAction[]>> = (() => {
  let current: ReadonlyMap<string, readonly DownloadAction[]> = new Map();
  let inputs = new Map<string, string>();
  return derived(
    [downloads, playThrottle],
    ([$downloads, $playThrottle], set) => {
      const nextInputs = new Map<string, string>();
      const next = new Map<string, readonly DownloadAction[]>();
      let changed = $downloads.length !== current.size;
      for (const d of $downloads) {
        const key = inputsOf(d, $playThrottle);
        nextInputs.set(d.id, key);
        const kept = inputs.get(d.id) === key ? current.get(d.id) : undefined;
        if (!kept) changed = true;
        next.set(d.id, kept ?? downloadActions(d, $playThrottle));
      }
      if (!changed) return;
      current = next;
      inputs = nextInputs;
      set(current);
    },
    current,
  );
})();
