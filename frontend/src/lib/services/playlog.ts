import { Events } from '@wailsio/runtime';
import { Service as PlaylogService } from '../../../bindings/typhon/internal/playlog';
import type { DayTotal, GameTotal, LongestSession, Stats } from '../../../bindings/typhon/internal/playlog';
import { inWails } from './backend';

export type { DayTotal, GameTotal, LongestSession, Stats };

export type StatsPeriod = 'week' | 'month' | 'year';

export const STATS_PERIODS: readonly StatsPeriod[] = ['week', 'month', 'year'];

const unavailable = () => new Error('unavailable in browser');

export async function getStats(period: StatsPeriod): Promise<Stats> {
  if (!inWails) throw unavailable();
  return (await PlaylogService.Stats(period)) as Stats;
}

export function onPlaylogRecorded(handler: () => void): () => void {
  if (!inWails) return () => {};
  return Events.On('playlog:recorded', () => handler());
}
