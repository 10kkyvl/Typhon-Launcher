import type { ShowcaseKind } from '../services/account';
import type { GameRef } from '../services/profile';
import { playtime } from '../utils/format';

export const SHOWCASE_TITLES: Record<ShowcaseKind, string> = {
  favorites: 'Любимые',
  recently_completed: 'Недавно пройденные',
  most_played: 'Больше всего сыграно',
};

export const SHOWCASE_HINTS: Record<ShowcaseKind, string> = {
  favorites: 'Отметьте игры сердцем на странице игры',
  recently_completed: 'Отметьте пройденные игры в меню игры',
  most_played: 'Появится после первой сыгранной сессии',
};

export function recentLabel(seconds: number): string {
  return `${playtime(seconds)} за 2 недели`;
}

export function dayLabel(isoDate: string, now = new Date()): string {
  const [y, m, d] = isoDate.split('-').map(Number);
  const date = new Date(y, m - 1, d);
  const startOfDay = (v: Date) => new Date(v.getFullYear(), v.getMonth(), v.getDate()).getTime();
  const days = Math.round((startOfDay(now) - startOfDay(date)) / 86400000);
  if (days <= 0) return 'Сегодня';
  if (days === 1) return 'Вчера';
  return date.toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' });
}

export type StatusKind = 'playing' | 'online' | 'offline';

export function statusLine(running: GameRef[], online: boolean): { kind: StatusKind; text: string } {
  if (running.length > 0) return { kind: 'playing', text: `Играет: ${running[0].title}` };
  return online ? { kind: 'online', text: 'В сети' } : { kind: 'offline', text: 'Не в сети' };
}
