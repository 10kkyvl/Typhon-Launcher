import type { DownloadAction } from '../downloads/actions';
import type { Download } from '../services/downloads';
import type { PlayThrottle } from '../services/playThrottle';
import type { Settings } from '../services/settings';

/** Where a panel's leaving hand-off lands among its controls (`data-panel-control`). */
export type PanelControl = 'lead' | 'more';

/** The selector of a panel's control of `kind`, for a hand-off's `prefer`. */
export const panelControl = (kind: PanelControl) => `[data-panel-control="${kind}"]`;

/**
 * How a press that may take its row away wants focus handed on: `prefer` the control (a selector) the
 * row that takes focus offers it on, and whether the download's own row in another section takes it,
 * before the rows around it (`first`) or only when none is left (`last`).
 */
export interface LeaveHow {
  follow?: 'first' | 'last';
  prefer?: string;
}

/** A press's hand-off, its row's place read at the press. */
export interface LeaveTicket {
  /** Where focus goes now by the screen's rule; for a dialog's `returnFocus`. */
  target(): HTMLElement | null;
  /** Hands focus on, if it fell, once the row has left. */
  handOn(): Promise<void>;
  /** The press failed: nothing to hand on. */
  drop(): void;
}

/** The screen's rule for where focus goes when a row leaves (routes/downloads/rowFocus.ts), called at the press. */
export type Leave = (id: string, how?: LeaveHow) => LeaveTicket;

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
