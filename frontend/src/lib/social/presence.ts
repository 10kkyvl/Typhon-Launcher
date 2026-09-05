import { shortDate } from '../profile/view';
import type { PresenceStatus } from '../services/online';
import type { FriendView, PresenceView } from '../services/social';
import { msg } from '../i18n';

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

export const STATUS_LABELS: Record<PresenceStatus, string> = {
  get online() {
    return msg('social.presenceOnline');
  },
  get away() {
    return msg('social.presenceAway');
  },
  get busy() {
    return msg('social.presenceBusy');
  },
  get invisible() {
    return msg('social.presenceInvisible');
  },
};

const DOT_LABELS: Record<PresenceDot, string> = {
  get online() {
    return STATUS_LABELS.online;
  },
  get away() {
    return STATUS_LABELS.away;
  },
  get busy() {
    return STATUS_LABELS.busy;
  },
  get offline() {
    return msg('social.presenceOffline');
  },
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
  if (elapsed < MINUTE) return msg('social.presenceJustNow');
  if (elapsed < HOUR) return msg('social.presenceMinutesAgo', { count: Math.floor(elapsed / MINUTE) });
  if (elapsed < DAY) return msg('social.presenceHoursAgo', { count: Math.floor(elapsed / HOUR) });
  const days = Math.floor(elapsed / DAY);
  if (days === 1) return msg('social.presenceYesterdayLower');
  if (days < 7) return msg('social.presenceDaysAgo', { count: days });
  return shortDate(iso);
}

export function presenceLine(presence?: PresenceView | null, now = new Date(), own = false): string {
  if (presence?.status === 'invisible') return own ? STATUS_LABELS.invisible : DOT_LABELS.offline;
  const dot = presenceDot(presence);
  if (isPlaying(presence)) {
    return presence?.gameTitle ? msg('social.playingNamed', { name: presence.gameTitle }) : msg('social.playing');
  }
  if (dot !== 'offline') return DOT_LABELS[dot];
  const seen = sinceLabel(presence?.lastSeenAt, now);
  return seen ? `${DOT_LABELS.offline} · ${seen}` : DOT_LABELS.offline;
}

export function ownStatusLine(status: string, running: boolean): string {
  const label = STATUS_LABELS[status as PresenceStatus];
  if (status === 'invisible') return label;
  if (running) return '';
  return label ?? STATUS_LABELS.online;
}

const RANKS: Record<PresenceDot, number> = {
  online: 1,
  away: 2,
  busy: 3,
  offline: 4,
};

export function isPlaying(presence?: PresenceView | null): boolean {
  return presenceDot(presence) !== 'offline' && presence?.gameId != null;
}

function rank(presence?: PresenceView | null): number {
  if (isPlaying(presence)) return 0;
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
