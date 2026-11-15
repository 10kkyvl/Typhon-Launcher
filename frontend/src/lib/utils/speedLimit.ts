/**
 * The download speed limit as a place on a slider: the steps in MB/s (binary megabytes, as everywhere
 * in the launcher), fine at the low end where a limit protects a slow line, coarser up to a gigabyte a
 * second, and "no limit" one place past the last step. Settings keep the limit in bytes/s, 0 for none;
 * Go reads a value below zero as none and has no upper bound. The words for a limit come from
 * `rateLimitLabel` (utils/format.ts), not from here.
 */
export const SPEED_LIMIT_STEPS_MB = [1, 2, 3, 5, 8, 10, 15, 20, 30, 50, 75, 100, 150, 200, 300, 500, 750, 1000] as const;

/** The place of "no limit": one past the last step. */
export const SPEED_LIMIT_NO_LIMIT = SPEED_LIMIT_STEPS_MB.length;

const MB = 1024 ** 2;
const LAST = SPEED_LIMIT_STEPS_MB.length - 1;

/**
 * Where a stored limit sits on the track. A value between two steps sits between them, in proportion,
 * so a limit set elsewhere (12 MB/s, 0.5 MB/s) is shown where it is and not moved to a step until the
 * player moves it. Under the first step it sits at the start, over the last at the last step.
 */
export function speedLimitPosition(bytesPerSec: number): number {
  if (!(bytesPerSec > 0)) return SPEED_LIMIT_NO_LIMIT;
  const mb = bytesPerSec / MB;
  if (mb <= SPEED_LIMIT_STEPS_MB[0]) return 0;
  for (let i = 1; i <= LAST; i++) {
    const below = SPEED_LIMIT_STEPS_MB[i - 1];
    const step = SPEED_LIMIT_STEPS_MB[i];
    if (mb <= step) return i - 1 + (mb - below) / (step - below);
  }
  return LAST;
}

/** The limit in bytes/s at a place on the track: the nearest step, 0 for "no limit". */
export function speedLimitAt(position: number): number {
  if (Number.isNaN(position)) return 0;
  const at = Math.min(SPEED_LIMIT_NO_LIMIT, Math.max(0, Math.round(position)));
  return at === SPEED_LIMIT_NO_LIMIT ? 0 : SPEED_LIMIT_STEPS_MB[at] * MB;
}
