import type { Message } from '../../types';
import type { ProfileKey } from '../ru/profile';

export const profile: Record<ProfileKey, Message> = {
  'profile.monthGames': { one: '{count} game', other: '{count} games' },
  'profile.monthCompleted': { one: '{count} finished', other: '{count} finished' },
  'profile.showcaseFavorites': 'Favorites',
  'profile.showcaseRecentlyCompleted': 'Recently completed',
  'profile.showcaseMostPlayed': 'Most played',
  'profile.showcaseHintFavorites': 'Mark games with a heart on the game page',
  'profile.showcaseHintRecentlyCompleted': 'Choose the "Completed" status on the game page',
  'profile.showcaseHintMostPlayed': 'Appears after your first play session',
  'profile.visibilityPublic': 'Everyone',
  'profile.visibilityFriends': 'Friends',
  'profile.visibilityPrivate': 'Nobody',
  'profile.today': 'Today',
  'profile.yesterday': 'Yesterday',
  'profile.recentWindow': '{value} in the last 2 weeks',
};
