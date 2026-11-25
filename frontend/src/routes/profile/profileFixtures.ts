import { DEFAULT_PROFILE, type CurrentUser, type ProfileSettings } from '../../lib/services/account';
import { EMPTY_SNAPSHOT, type GameRef, type ProfileSnapshot } from '../../lib/services/profile';

// The players and the profile figures of the profile screen's tests; made up.

export function player(profile: Partial<ProfileSettings> = {}, extra: Partial<CurrentUser> = {}): CurrentUser {
  return {
    id: 'u1',
    username: 'ann',
    displayName: 'Ann',
    email: 'ann@example.com',
    avatarUrl: '',
    bio: '',
    profile: { ...DEFAULT_PROFILE, ...profile },
    createdAt: '2025-01-01T00:00:00Z',
    ...extra,
  };
}

export const ref = (id: string, title: string, extra: Partial<GameRef> = {}): GameRef => ({
  id,
  title,
  cover: `${id}.jpg`,
  canonicalGameId: `c-${id}`,
  playtimeSeconds: 3600,
  status: '',
  ...extra,
});

export const SNAPSHOT: ProfileSnapshot = {
  ...EMPTY_SNAPSHOT,
  stats: { ...EMPTY_SNAPSHOT.stats, games: 12, hours: 340, completed: 5 },
  playing: [{ game: ref('g2', 'Celeste'), recentSeconds: 60 }],
};
