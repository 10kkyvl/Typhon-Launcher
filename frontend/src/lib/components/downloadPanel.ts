import type { DownloadAction } from '../downloads/actions';
import type { Download } from '../services/downloads';
import type { PlayThrottle } from '../services/playThrottle';
import type { Settings } from '../services/settings';

/** The attribute naming a panel's download (DownloadItem.svelte); focus is handed on by it, not by index. */
export const PANEL_ATTR = 'data-download-panel';

/** The attribute marking the controls a hand-off may land on inside a panel. */
export const CONTROL_ATTR = 'data-panel-control';

/** Which control of a panel takes focus: the lead button, or the "more" menu's, which every panel has. */
export type PanelControl = 'lead' | 'more';

/** The action of the panel's one lead button, the first Go offers: "Download now", else Pause, else Resume. */
export function leadAction(offers: readonly DownloadAction[]): 'downloadNow' | 'pause' | 'resume' | null {
  if (offers.includes('downloadNow')) return 'downloadNow';
  if (offers.includes('pause')) return 'pause';
  if (offers.includes('resume')) return 'resume';
  return null;
}

/** The download speed limit in force, bytes/s, 0 for none: the play limit while a game runs in that mode, else the user's. */
export function speedLimit(settings: Pick<Settings, 'downloadRateLimit'> | null, throttle: Pick<PlayThrottle, 'mode' | 'downloadLimit'>): number {
  if (throttle.mode === 'limit') return Math.max(0, throttle.downloadLimit);
  return Math.max(0, settings?.downloadRateLimit ?? 0);
}

/**
 * Whether Go would hold this download back if it started now: a running game in the pause mode, or the
 * schedule, unless "Download now" lifted it (internal/download `pausedLocked`). A forced start under a
 * hold sets the status to downloading with the engine gated, which would read as a stalled download.
 */
export function heldBack(download: Pick<Download, 'ignoreSchedule'>, throttle: Pick<PlayThrottle, 'mode' | 'schedule'>): boolean {
  if (throttle.mode === 'pause') return true;
  return Boolean(throttle.schedule?.blocked) && !download.ignoreSchedule;
}

/**
 * Whether Go's Cancel leaves this download's files on disk: an in-place download (an update or a repair
 * written into the game's folder) and a flat one, whose folder Go cannot tell apart (manager.go `discard`,
 * `removalRoot`). Go sends both flags in the record; the TS type does not declare them yet.
 */
export function cancelKeepsFiles(download: Download): boolean {
  const flags = download as Download & { inPlace?: boolean; flat?: boolean };
  return flags.inPlace === true || flags.flat === true;
}

function panels(): HTMLElement[] {
  return [...document.querySelectorAll<HTMLElement>(`[${PANEL_ATTR}]`)];
}

/** The ids of the panels on the page, in their order. */
export function panelOrder(): string[] {
  return panels().map((panel) => panel.getAttribute(PANEL_ATTR) ?? '');
}

function controlIn(panel: HTMLElement, prefer: PanelControl): HTMLElement | null {
  return panel.querySelector<HTMLElement>(`[${CONTROL_ATTR}="${prefer}"]`) ?? panel.querySelector<HTMLElement>(`[${CONTROL_ATTR}="more"]`);
}

/**
 * Where focus goes when the panel of `id` lost it: the same panel when it `stays` (a download that came
 * back under a new node), else the panel now in its place, else the one above, else `away` (the page's
 * section heading or its own heading). `order` is the panels' order at the press.
 */
export function handOffTarget(
  order: readonly string[],
  id: string,
  options: { stays: boolean; prefer: PanelControl; away?: () => HTMLElement | null | undefined },
): HTMLElement | null | undefined {
  const shown = new Map(panels().map((panel) => [panel.getAttribute(PANEL_ATTR) ?? '', panel]));
  const own = shown.get(id);
  if (options.stays && own) return controlIn(own, options.prefer);
  const at = order.indexOf(id);
  const candidates = at < 0 ? [] : [...order.slice(at + 1), ...order.slice(0, at).reverse()];
  for (const other of candidates) {
    const panel = shown.get(other);
    if (panel) return controlIn(panel, options.prefer);
  }
  return options.away?.();
}
