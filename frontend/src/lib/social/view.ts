import { SHOWCASE_TITLES } from '../profile/view';
import type { ShowcaseKind } from '../services/account';
import type { Relation } from '../services/social';
import type { Notification } from '../stores/notifications';
import { relativeDate } from '../utils/format';
import { msg } from '../i18n';

const RELATION_LABELS: Record<Relation, string> = {
  none: 'Добавить в друзья',
  outgoing: 'Заявка отправлена',
  incoming: 'Принять',
  friend: 'В друзьях',
  self: 'Это вы',
  blocked: 'Недоступен',
};

export function relationLabel(relation: string): string {
  return RELATION_LABELS[relation as Relation] ?? RELATION_LABELS.blocked;
}

const RELATION_HINTS: Record<Relation, string> = {
  none: '',
  outgoing: 'Заявка уже отправлена',
  incoming: 'Этот пользователь уже отправил вам заявку — примите её во вкладке «Заявки»',
  friend: 'Вы уже друзья',
  self: 'Это вы',
  blocked: 'Пользователь недоступен',
};

export function relationHint(relation: string): string {
  return RELATION_HINTS[relation as Relation] ?? RELATION_HINTS.blocked;
}

export function friendRequestNotification(count: number, peak: number): Notification | null {
  if (count <= 0) return null;
  const text = count === 1 ? msg('friends.requestOne') : msg('friends.requests', { count });
  return {
    id: `friends:incoming:${Math.max(peak, count)}`,
    title: 'Друзья',
    text,
    route: 'friends',
    terminal: false,
  };
}

const FRIEND_CODE = /^(?:ty-?[2-9a-hj-np-tv-z]{8}|(?:ty-?)?[2-9a-hj-np-tv-z]{4}-[2-9a-hj-np-tv-z]{4})$/i;

export function isFriendCode(input: string): boolean {
  const value = input.trim();
  if (value.startsWith('@')) return false;
  return FRIEND_CODE.test(value);
}

export function sentAt(iso: string | null): string {
  const when = relativeDate(iso);
  return when === '—' ? when : `Отправлена ${when.toLocaleLowerCase('ru-RU')}`;
}

export function commonGamesTitle(games: number): string {
  return msg('friends.commonGames', { count: games });
}

export function commonLine(mutual: number, games: number): string {
  const parts: string[] = [];
  if (mutual > 0) parts.push(msg('friends.commonFriends', { count: mutual }));
  if (games > 0) parts.push(commonGamesTitle(games));
  return parts.join(' · ');
}

export function commonGameLabel(viewerOwned: boolean, targetOwned: boolean, name: string): string {
  if (viewerOwned && targetOwned) return 'установлена у обоих';
  if (!viewerOwned && !targetOwned) return 'нужно установить обоим';
  if (!viewerOwned) return 'нужно установить вам';
  const owner = name.trim();
  return owner ? `нужно установить: ${owner}` : 'нужно установить ему';
}

export function joinDate(iso: string): string {
  if (!iso) return '';
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return '';
  return date.toLocaleDateString('ru-RU', { day: 'numeric', month: 'long', year: 'numeric' });
}

export function memberSince(iso: string): string {
  const date = joinDate(iso);
  return date ? `Участник с ${date}` : '';
}

export function mutualMore(shown: number, total: number): string {
  const rest = total - shown;
  return rest > 0 ? `+${rest}` : '';
}

export function showcaseTitle(kind: string): string {
  return SHOWCASE_TITLES[kind as ShowcaseKind] ?? kind;
}
