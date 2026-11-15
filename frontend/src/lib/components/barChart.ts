/**
 * The arithmetic of BarChart: the axis scale, the ticks, each bar's share of the height and whether
 * there is anything to draw. Values are the caller's unit (seconds, sessions); a value that is not a
 * finite number at or above zero draws as nothing.
 */

export interface ChartBucket {
  key: string;
  /** The bucket in words ("Mon, 3 Nov"): the stand-in table and the name of a bar that can be chosen. */
  label: string;
  value: number;
  /** The bucket now is in (today, this week): drawn apart. */
  current?: boolean;
  /** Shown under the bar instead of `label` ("Mon"); an empty string shows nothing there. */
  tick?: string;
}

export interface ChartScale {
  max: number;
  step: number;
}

const usable = (value: number) => (Number.isFinite(value) && value > 0 ? value : 0);

/** A bucket's value as the bar, its label, the table and a radio's name all take it: 0 for no value. */
export const chartValue = (bucket: ChartBucket): number => usable(bucket.value);

const HOUR_STEPS = [1, 2, 3, 4, 6, 8, 10, 12, 16, 20, 24];

/** A round axis maximum in hours (1, 2, 3, 4, 6, 8, 10, 12, …) and its tick step. */
export function hourScale(maxSeconds: number): ChartScale {
  const hours = usable(maxSeconds) / 3600;
  const max = HOUR_STEPS.find((s) => s >= hours) ?? Math.ceil(hours / 4) * 4;
  const step = max <= 4 ? 1 : max <= 8 ? 2 : max <= 12 ? 4 : max / 4;
  return { max, step };
}

/** `hourScale` in seconds, for a chart whose values are seconds. */
export function secondsScale(maxSeconds: number): ChartScale {
  const { max, step } = hourScale(maxSeconds);
  return { max: max * 3600, step: step * 3600 };
}

const MAX_TICKS = 50;

/** The ticks of a scale, from 0 to its maximum; none for a scale that cannot be drawn. */
export function ticks(scale: ChartScale): number[] {
  const { max, step } = scale;
  if (!(usable(max) > 0) || !(usable(step) > 0) || max / step > MAX_TICKS) return [];
  return Array.from({ length: Math.floor(max / step + 1e-9) + 1 }, (_, i) => i * step);
}

/** The value the full height stands for: the scale's maximum, or the largest value (0 for none). */
export function chartMax(buckets: readonly ChartBucket[], scale?: ChartScale): number {
  if (scale && usable(scale.max) > 0) return scale.max;
  return Math.max(0, ...buckets.map(chartValue));
}

/** A bar's height as a part of the full height, 0…1. */
export function barShare(value: number, max: number): number {
  if (!(usable(max) > 0)) return 0;
  return Math.min(1, usable(value) / max);
}

/** Nothing to draw: no buckets, or none above zero. */
export function isEmpty(buckets: readonly ChartBucket[]): boolean {
  return buckets.every((bucket) => usable(bucket.value) === 0);
}
