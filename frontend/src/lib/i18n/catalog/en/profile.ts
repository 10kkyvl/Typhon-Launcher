import type { Message } from '../../types';
import type { ProfileKey } from '../ru/profile';

export const profile: Record<ProfileKey, Message> = {
  'profile.monthGames': { one: '{count} game', other: '{count} games' },
  'profile.monthCompleted': { one: '{count} finished', other: '{count} finished' },
};
