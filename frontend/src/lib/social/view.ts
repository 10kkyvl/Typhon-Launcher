import { SHOWCASE_TITLES } from '../profile/view';
import type { ShowcaseKind } from '../services/account';
import type { Relation } from '../services/social';
import type { Notification } from '../stores/notifications';
import { plural } from '../utils/format';

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

export function friendRequestNotification(count: number): Notification | null {
  if (count <= 0) return null;
  const text =
    count === 1 ? 'Заявка в друзья' : `${count} ${plural(count, 'заявка', 'заявки', 'заявок')} в друзья`;
  return {
    id: `friends:incoming:${count}`,
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

function count(n: number, adjective: [string, string, string], noun: [string, string, string]): string {
  return `${n} ${plural(n, ...adjective)} ${plural(n, ...noun)}`;
}

export function commonGamesTitle(games: number): string {
  return count(games, ['общая', 'общие', 'общих'], ['игра', 'игры', 'игр']);
}

export function commonLine(mutual: number, games: number): string {
  const parts: string[] = [];
  if (mutual > 0) parts.push(count(mutual, ['общий', 'общих', 'общих'], ['друг', 'друга', 'друзей']));
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
