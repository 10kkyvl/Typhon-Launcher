import type { ActivityDay, GameRef } from '../services/profile';

export interface WeekDay {
  date: string;
  at: Date;
  seconds: number;
  today: boolean;
}

export interface WeekGame {
  game: GameRef;
  seconds: number;
}

export interface WeekSummary {
  days: WeekDay[];
  games: WeekGame[];
  totalSeconds: number;
  bestSeconds: number;
}

const WEEK_LENGTH = 7;
const TOP_GAMES = 3;

function dayKey(date: Date): string {
  const month = `${date.getMonth() + 1}`.padStart(2, '0');
  const day = `${date.getDate()}`.padStart(2, '0');
  return `${date.getFullYear()}-${month}-${day}`;
}

export function weekSummary(activity: ActivityDay[], now = new Date(), limit = TOP_GAMES): WeekSummary {
  const byDate = new Map(activity.map((day) => [day.date, day]));
  const totals = new Map<string, WeekGame>();
  const days: WeekDay[] = [];
  let totalSeconds = 0;
  let bestSeconds = 0;

  for (let back = WEEK_LENGTH - 1; back >= 0; back--) {
    const date = new Date(now.getFullYear(), now.getMonth(), now.getDate() - back);
    const key = dayKey(date);
    let seconds = 0;
    for (const entry of byDate.get(key)?.entries ?? []) {
      if (entry.seconds <= 0) continue;
      seconds += entry.seconds;
      const found = totals.get(entry.game.id);
      if (found) found.seconds += entry.seconds;
      else totals.set(entry.game.id, { game: entry.game, seconds: entry.seconds });
    }
    totalSeconds += seconds;
    if (seconds > bestSeconds) bestSeconds = seconds;
    days.push({ date: key, at: date, seconds, today: back === 0 });
  }

  const games = [...totals.values()]
    .sort((a, b) => b.seconds - a.seconds || a.game.title.localeCompare(b.game.title, 'ru'))
    .slice(0, limit);

  return { days, games, totalSeconds, bestSeconds };
}
