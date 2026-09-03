import { plural } from '../utils/format';

export function commonGameLabel(viewerOwned: boolean, targetOwned: boolean, name: string): string {
  if (viewerOwned && targetOwned) return 'установлена у обоих';
  if (!viewerOwned && !targetOwned) return 'нужно установить обоим';
  if (!viewerOwned) return 'нужно установить вам';
  return `нужно установить ${name.trim() || 'ему'}`;
}

export function commonGamesTitle(count: number): string {
  return `${count} ${plural(count, 'общая игра', 'общие игры', 'общих игр')}`;
}

export function memberSince(iso: string): string {
  if (!iso) return '';
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return '';
  const text = date.toLocaleDateString('ru-RU', { day: 'numeric', month: 'long', year: 'numeric' });
  return `Участник с ${text}`;
}

export function mutualMore(shown: number, total: number): string {
  const rest = total - shown;
  return rest > 0 ? `+${rest}` : '';
}
