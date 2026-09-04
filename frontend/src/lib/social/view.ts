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

export function commonLine(mutual: number, games: number): string {
  const parts: string[] = [];
  if (mutual > 0) parts.push(count(mutual, ['общий', 'общих', 'общих'], ['друг', 'друга', 'друзей']));
  if (games > 0) parts.push(count(games, ['общая', 'общие', 'общих'], ['игра', 'игры', 'игр']));
  return parts.join(' · ');
}
