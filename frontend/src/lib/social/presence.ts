import { shortDate } from '../profile/view';
import type { FriendView, PresenceView } from '../services/social';

export type PresenceDot = 'online' | 'away' | 'busy' | 'offline';

export type PresenceKind = 'success' | 'warning' | 'danger' | 'neutral';

const DOTS: Record<string, PresenceDot> = {
  online: 'online',
  away: 'away',
  busy: 'busy',
};

const KINDS: Record<PresenceDot, PresenceKind> = {
  online: 'success',
  away: 'warning',
  busy: 'danger',
  offline: 'neutral',
};

const LINES: Record<PresenceDot, string> = {
  online: 'В сети',
  away: 'Отошёл',
  busy: 'Не беспокоить',
  offline: 'Не в сети',
};

const OWN_LINES: Record<string, string> = {
  away: 'Отошёл',
  busy: 'Не беспокоить',
  invisible: 'Невидимка',
};

const MINUTE = 60000;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

export function statusDot(status: string): PresenceDot {
  return DOTS[status] ?? 'offline';
}

export function presenceDot(presence?: PresenceView | null): PresenceDot {
  return statusDot(presence?.status ?? '');
}

export function dotKind(dot: PresenceDot): PresenceKind {
  return KINDS[dot];
}

export function sinceLabel(iso: string | null | undefined, now = new Date()): string {
  if (!iso) return '';
  const seen = new Date(iso);
  if (Number.isNaN(seen.getTime())) return '';
  const elapsed = now.getTime() - seen.getTime();
  if (elapsed < MINUTE) return 'только что';
  if (elapsed < HOUR) return `${Math.floor(elapsed / MINUTE)} мин назад`;
  if (elapsed < DAY) return `${Math.floor(elapsed / HOUR)} ч назад`;
  const days = Math.floor(elapsed / DAY);
  if (days === 1) return 'вчера';
  if (days < 7) return `${days} дн. назад`;
  return shortDate(iso);
}

export function presenceLine(presence?: PresenceView | null, now = new Date()): string {
  const dot = presenceDot(presence);
  if (playing(presence)) return presence?.gameTitle ? `Играет: ${presence.gameTitle}` : 'Играет';
  if (dot !== 'offline') return LINES[dot];
  const seen = sinceLabel(presence?.lastSeenAt, now);
  return seen ? `${LINES.offline} · ${seen}` : LINES.offline;
}

export function ownStatusLine(status: string, running: boolean): string {
  if (running) return '';
  return OWN_LINES[status] ?? LINES.online;
}

const RANKS: Record<PresenceDot, number> = {
  online: 1,
  away: 2,
  busy: 3,
  offline: 4,
};

function playing(presence?: PresenceView | null): boolean {
  return presenceDot(presence) !== 'offline' && presence?.gameId != null;
}

function rank(presence?: PresenceView | null): number {
  if (playing(presence)) return 0;
  return RANKS[presenceDot(presence)];
}

function seenAt(presence?: PresenceView | null): number {
  if (!presence?.lastSeenAt) return 0;
  const seen = new Date(presence.lastSeenAt).getTime();
  return Number.isNaN(seen) ? 0 : seen;
}

function label(friend: FriendView): string {
  return friend.displayName || friend.username;
}

export function sortFriends(list: FriendView[]): FriendView[] {
  return [...list].sort((a, b) => {
    const byRank = rank(a.presence) - rank(b.presence);
    if (byRank !== 0) return byRank;
    const bySeen = seenAt(b.presence) - seenAt(a.presence);
    if (bySeen !== 0) return bySeen;
    return label(a).localeCompare(label(b), 'ru');
  });
}
