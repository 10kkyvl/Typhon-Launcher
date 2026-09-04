import type { FeedEvent, ReactionCount } from '../services/social';
import { relativeDate } from '../utils/format';

export type ReactionId = 'fire' | 'salute' | 'heart' | 'clap' | 'skull' | 'party' | 'eyes' | 'joy';

export const REACTIONS: ReactionId[] = ['fire', 'salute', 'heart', 'clap', 'skull', 'party', 'eyes', 'joy'];

const REACTION_LABELS: Record<ReactionId, string> = {
  fire: 'Огонь',
  salute: 'Салют',
  heart: 'Сердце',
  clap: 'Аплодисменты',
  skull: 'Череп',
  party: 'Праздник',
  eyes: 'Глаза',
  joy: 'Смех',
};

const KIND_LABELS: Record<string, string> = {
  completed: 'Пройдена',
  started: 'Новая игра',
  favorited: 'В любимых',
};

export interface FeedDayGroup {
  key: string;
  label: string;
  events: FeedEvent[];
}

export function reactionLabel(emoji: string): string {
  return REACTION_LABELS[emoji as ReactionId] ?? emoji;
}

export function isReaction(emoji: string): emoji is ReactionId {
  return emoji in REACTION_LABELS;
}

export function kindLabel(kind: string): string {
  return KIND_LABELS[kind] ?? '';
}

export function eventLine(event: { kind: string; game: { title: string } }): string {
  const label = kindLabel(event.kind);
  const title = event.game.title.trim();
  if (!label) return title;
  if (!title) return label;
  return `${label}: ${title}`;
}

function dayKey(iso: string): string {
  const date = new Date(iso);
  if (!iso || Number.isNaN(date.getTime())) return 'unknown';
  return `${date.getFullYear()}-${date.getMonth() + 1}-${date.getDate()}`;
}

export function feedDayGroups(events: FeedEvent[]): FeedDayGroup[] {
  const groups = new Map<string, FeedDayGroup>();
  for (const event of events) {
    const key = dayKey(event.createdAt);
    const group = groups.get(key);
    if (group) {
      group.events.push(event);
      continue;
    }
    groups.set(key, { key, label: relativeDate(event.createdAt), events: [event] });
  }
  return [...groups.values()];
}

export function reactionCount(event: FeedEvent, emoji: string): number {
  return event.reactions.find((item) => item.emoji === emoji)?.count ?? 0;
}

export function hasReacted(event: FeedEvent, emoji: string): boolean {
  return event.mine.includes(emoji);
}

function withReaction(reactions: ReactionCount[], emoji: string, delta: number): ReactionCount[] {
  const next: ReactionCount[] = [];
  let seen = false;
  for (const item of reactions) {
    if (item.emoji !== emoji) {
      next.push(item);
      continue;
    }
    seen = true;
    const count = item.count + delta;
    if (count > 0) next.push({ emoji, count });
  }
  if (!seen && delta > 0) next.push({ emoji, count: delta });
  return next;
}

export function toggleReaction(event: FeedEvent, emoji: string): FeedEvent {
  if (!isReaction(emoji)) return event;
  const mine = hasReacted(event, emoji);
  return {
    ...event,
    reactions: withReaction(event.reactions, emoji, mine ? -1 : 1),
    mine: mine ? event.mine.filter((item) => item !== emoji) : [...event.mine, emoji],
  };
}
