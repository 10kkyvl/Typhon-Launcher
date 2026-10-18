import type { DayTotal } from '../services/playlog';

export type ChartUnit = 'day' | 'month';

export interface ChartBucket {
  key: string;
  at: Date;
  seconds: number;
  unit: ChartUnit;
}

export function parseDay(value: string): Date | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (!match) return null;
  const date = new Date(Number(match[1]), Number(match[2]) - 1, Number(match[3]));
  return Number.isNaN(date.getTime()) ? null : date;
}

export function chartUnit(period: string): ChartUnit {
  return period === 'year' ? 'month' : 'day';
}

export function chartBuckets(period: string, days: DayTotal[]): ChartBucket[] {
  const unit = chartUnit(period);
  const buckets: ChartBucket[] = [];
  const byKey = new Map<string, ChartBucket>();
  for (const day of days) {
    const at = parseDay(day.date);
    if (!at) continue;
    const seconds = Math.max(0, day.seconds);
    if (unit === 'day') {
      buckets.push({ key: day.date, at, seconds, unit });
      continue;
    }
    const key = day.date.slice(0, 7);
    const found = byKey.get(key);
    if (found) {
      found.seconds += seconds;
      continue;
    }
    const bucket: ChartBucket = { key, at: new Date(at.getFullYear(), at.getMonth(), 1), seconds, unit };
    byKey.set(key, bucket);
    buckets.push(bucket);
  }
  return buckets;
}

export function bestSeconds(buckets: ChartBucket[]): number {
  return buckets.reduce((best, bucket) => Math.max(best, bucket.seconds), 0);
}

export function barHeight(seconds: number, best: number): number {
  if (seconds <= 0 || best <= 0) return 0;
  return Math.max(4, Math.round((seconds / best) * 100));
}

export function labelEvery(period: string): number {
  return period === 'month' ? 5 : 1;
}
