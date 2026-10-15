import type { GameTotal, LongestSession, Stats } from '../services/playlog';
import { parseDay } from './buckets';

export interface LongestView {
  gameId: string;
  title: string;
  seconds: number;
  at: Date | null;
}

export interface TopView {
  gameId: string;
  title: string;
  seconds: number;
  sessions: number;
  share: number;
}

export interface StatsSummary {
  totalSeconds: number;
  sessions: number;
  longest: LongestView | null;
  top: TopView[];
  empty: boolean;
}

function longestView(longest: LongestSession | null): LongestView | null {
  if (!longest || longest.seconds <= 0) return null;
  return { gameId: longest.gameId, title: longest.title, seconds: longest.seconds, at: parseDay(longest.date) };
}

function topView(top: GameTotal[], totalSeconds: number): TopView[] {
  return top
    .filter((item) => item.seconds > 0)
    .map((item) => ({
      gameId: item.gameId,
      title: item.title,
      seconds: item.seconds,
      sessions: item.sessions,
      share: totalSeconds > 0 ? Math.min(1, item.seconds / totalSeconds) : 0,
    }));
}

export function summarize(stats: Stats): StatsSummary {
  const totalSeconds = Math.max(0, stats.totalSeconds);
  return {
    totalSeconds,
    sessions: Math.max(0, stats.sessions),
    longest: longestView(stats.longest),
    top: topView(stats.top ?? [], totalSeconds),
    empty: totalSeconds === 0 && stats.sessions <= 0,
  };
}

export function gameTitle(title: string, libraryTitle: string | undefined, deleted: string): string {
  return title.trim() || libraryTitle?.trim() || deleted;
}
