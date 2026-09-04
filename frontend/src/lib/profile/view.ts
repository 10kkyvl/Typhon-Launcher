import type { ShowcaseKind, Visibility } from '../services/account';
import type { GameRef, ProfileStats } from '../services/profile';
import { playtime, shortDate as formatShortDate } from '../utils/format';
import { msg } from '../i18n';
import type { ProfileKey } from '../i18n/catalog/ru/profile';

const SHOWCASE_TITLE_KEYS: Record<ShowcaseKind, ProfileKey> = {
  favorites: 'profile.showcaseFavorites',
  recently_completed: 'profile.showcaseRecentlyCompleted',
  most_played: 'profile.showcaseMostPlayed',
};

export function showcaseLabel(kind: string): string {
  const key = SHOWCASE_TITLE_KEYS[kind as ShowcaseKind];
  return key ? msg(key) : kind;
}

const SHOWCASE_HINT_KEYS: Record<ShowcaseKind, ProfileKey> = {
  favorites: 'profile.showcaseHintFavorites',
  recently_completed: 'profile.showcaseHintRecentlyCompleted',
  most_played: 'profile.showcaseHintMostPlayed',
};

export function showcaseHint(kind: ShowcaseKind): string {
  return msg(SHOWCASE_HINT_KEYS[kind]);
}

const VISIBILITY_KEYS: Record<Visibility, ProfileKey> = {
  public: 'profile.visibilityPublic',
  friends: 'profile.visibilityFriends',
  private: 'profile.visibilityPrivate',
};

export function visibilityLabel(visibility: string): string {
  const key = VISIBILITY_KEYS[visibility as Visibility] ?? VISIBILITY_KEYS.friends;
  return msg(key);
}

export function recentLabel(seconds: number): string {
  return msg('profile.recentWindow', { value: playtime(seconds) });
}

export function dayLabel(isoDate: string, now = new Date()): string {
  const [y, m, d] = isoDate.split('-').map(Number);
  const date = new Date(y, m - 1, d);
  const startOfDay = (v: Date) => new Date(v.getFullYear(), v.getMonth(), v.getDate()).getTime();
  const days = Math.round((startOfDay(now) - startOfDay(date)) / 86400000);
  if (days <= 0) return msg('profile.today');
  if (days === 1) return msg('profile.yesterday');
  return formatShortDate(date);
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
  return formatShortDate(new Date(iso));
}

export type StatusKind = 'playing' | 'online' | 'offline';

export function statusLine(running: GameRef[], online: boolean): { kind: StatusKind; text: string } {
  if (running.length > 0) return { kind: 'playing', text: msg('social.playingNamed', { name: running[0].title }) };
  return online
    ? { kind: 'online', text: msg('social.presenceOnline') }
    : { kind: 'offline', text: msg('social.presenceOffline') };
}
