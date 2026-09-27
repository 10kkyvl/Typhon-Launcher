import { get } from 'svelte/store';
import { showcaseLabel } from '../profile/view';
import type { Relation } from '../services/social';
import type { Notification } from '../stores/notifications';
import { longDate, relativeDate } from '../utils/format';
import { locale, msg } from '../i18n';
import type { SocialKey } from '../i18n/catalog/ru/social';

const RELATION_KEYS: Record<Relation, SocialKey> = {
  none: 'social.relationNone',
  outgoing: 'social.relationOutgoing',
  incoming: 'social.relationIncoming',
  friend: 'social.relationFriend',
  self: 'social.relationSelf',
  blocked: 'social.relationBlocked',
};

export function relationLabel(relation: string): string {
  return msg(RELATION_KEYS[relation as Relation] ?? RELATION_KEYS.blocked);
}

const RELATION_HINT_KEYS: Record<Relation, SocialKey | ''> = {
  none: '',
  outgoing: 'social.relationHintOutgoing',
  incoming: 'social.relationHintIncoming',
  friend: 'social.relationHintFriend',
  self: 'social.relationSelf',
  blocked: 'social.relationHintBlocked',
};

export function relationHint(relation: string): string {
  const key = RELATION_HINT_KEYS[relation as Relation] ?? RELATION_HINT_KEYS.blocked;
  return key ? msg(key) : '';
}

export function friendRequestNotification(count: number, peak: number): Notification | null {
  if (count <= 0) return null;
  const text = count === 1 ? msg('friends.requestOne') : msg('friends.requests', { count });
  return {
    id: `friends:incoming:${Math.max(peak, count)}`,
    title: msg('social.friendsTitle'),
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
  if (when === '—') return when;
  return msg('friends.sentAt', { when: when.toLocaleLowerCase(get(locale)) });
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
  if (viewerOwned && targetOwned) return msg('social.userGameInstalledBoth');
  if (!viewerOwned && !targetOwned) return msg('social.userGameInstallBoth');
  if (!viewerOwned) return msg('social.userGameInstallYou');
  const owner = name.trim();
  return owner ? msg('social.userGameInstallNamed', { name: owner }) : msg('social.userGameInstallOther');
}

export function joinDate(iso: string): string {
  if (!iso) return '';
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return '';
  return longDate(date);
}

export function memberSince(iso: string): string {
  const date = joinDate(iso);
  return date ? msg('social.memberSince', { date }) : '';
}

export function mutualMore(shown: number, total: number): string {
  const rest = total - shown;
  return rest > 0 ? `+${rest}` : '';
}

export function showcaseTitle(kind: string): string {
  return showcaseLabel(kind);
}
