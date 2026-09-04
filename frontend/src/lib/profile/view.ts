import type { ShowcaseKind, Visibility } from '../services/account';
import type { GameRef, ProfileStats } from '../services/profile';
import { playtime } from '../utils/format';
import { msg } from '../i18n';

export const SHOWCASE_TITLES: Record<ShowcaseKind, string> = {
  favorites: 'Любимые',
  recently_completed: 'Недавно пройденные',
  most_played: 'Больше всего сыграно',
};

export const SHOWCASE_HINTS: Record<ShowcaseKind, string> = {
  favorites: 'Отметьте игры сердцем на странице игры',
  recently_completed: 'Выберите статус «Пройдена» на странице игры',
  most_played: 'Появится после первой сыгранной сессии',
};

export const VISIBILITY_LABELS: Record<Visibility, string> = {
  public: 'Все',
  friends: 'Друзья',
  private: 'Никто',
};

export function visibilityLabel(visibility: string): string {
  return VISIBILITY_LABELS[visibility as Visibility] ?? VISIBILITY_LABELS.friends;
}

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

export function monthLine(stats: ProfileStats): string {
  const { monthSeconds, monthGames, monthCompleted } = stats;
  if (monthSeconds === 0 && monthGames === 0 && monthCompleted === 0) return '';
  const parts: string[] = [];
  if (monthSeconds > 0) parts.push(playtime(monthSeconds));
  if (monthGames > 0) parts.push(msg('profile.monthGames', { count: monthGames }));
  parts.push(msg('profile.monthCompleted', { count: monthCompleted }));
  return parts.join(' · ');
}

export function shortDate(iso: string): string {
  return new Date(iso).toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' });
}

export type StatusKind = 'playing' | 'online' | 'offline';

export function statusLine(running: GameRef[], online: boolean): { kind: StatusKind; text: string } {
  if (running.length > 0) return { kind: 'playing', text: `Играет: ${running[0].title}` };
  return online ? { kind: 'online', text: 'В сети' } : { kind: 'offline', text: 'Не в сети' };
}
