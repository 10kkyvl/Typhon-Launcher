import type { FeedAchievement, FeedEvent, FeedKind, ReactionCount } from '../services/social';
import { relativeDate } from '../utils/format';
import { msg } from '../i18n';
import type { SocialKey } from '../i18n/catalog/ru/social';

export type ReactionId = 'fire' | 'salute' | 'heart' | 'clap' | 'skull' | 'party' | 'eyes' | 'joy';

export const REACTIONS: ReactionId[] = ['fire', 'salute', 'heart', 'clap', 'skull', 'party', 'eyes', 'joy'];

const REACTION_KEYS: Record<ReactionId, SocialKey> = {
  fire: 'social.reactionFire',
  salute: 'social.reactionSalute',
  heart: 'social.reactionHeart',
  clap: 'social.reactionClap',
  skull: 'social.reactionSkull',
  party: 'social.reactionParty',
  eyes: 'social.reactionEyes',
  joy: 'social.reactionJoy',
};

const KIND_KEYS: Record<FeedKind, SocialKey> = {
  completed: 'social.feedCompleted',
  started: 'social.feedStarted',
  favorited: 'social.feedFavorited',
  played: 'social.feedPlayed',
  achievement: 'social.feedAchievement',
  reviewed: 'social.feedReviewed',
  planned: 'social.feedPlanned',
};

export const FEED_KIND_LABELS: Record<FeedKind, SocialKey> = {
  completed: 'social.feedKindCompleted',
  started: 'social.feedKindStarted',
  favorited: 'social.feedKindFavorited',
  played: 'social.feedKindPlayed',
  achievement: 'social.feedKindAchievement',
  reviewed: 'social.feedKindReviewed',
  planned: 'social.feedKindPlanned',
};

export interface FeedLabelSource {
  kind: string;
  seconds?: number;
  achievements?: FeedAchievement[];
}

export interface FeedDayGroup {
  key: string;
  label: string;
  events: FeedEvent[];
}

export function reactionLabel(emoji: string): string {
  const key = REACTION_KEYS[emoji as ReactionId];
  return key ? msg(key) : emoji;
}

export function isReaction(emoji: string): emoji is ReactionId {
  return emoji in REACTION_KEYS;
}

export function kindLabel(kind: string): string {
  if (kind === 'played' || kind === 'achievement') return '';
  const key = (KIND_KEYS as Record<string, SocialKey>)[kind];
  return key ? msg(key) : '';
}

export function eventLabel(event: FeedLabelSource): string {
  if (event.kind === 'played') {
    const hours = Math.floor((event.seconds ?? 0) / 3600);
    return hours > 0 ? msg('social.feedPlayed', { hours }) : '';
  }
  if (event.kind === 'achievement') {
    const list = event.achievements ?? [];
    if (list.length === 0) return '';
    if (list.length === 1) return msg('social.feedAchievement', { name: list[0].name || list[0].key });
    return msg('social.feedAchievements', { count: list.length });
  }
  return kindLabel(event.kind);
}

export function eventLine(event: FeedLabelSource & { game: { title: string } }): string {
  const label = eventLabel(event);
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
