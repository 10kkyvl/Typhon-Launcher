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
